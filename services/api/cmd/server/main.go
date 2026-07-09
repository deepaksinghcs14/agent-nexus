package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/handler"
	"github.com/deepaksingh/agent-nexus/services/api/internal/api/router"
	"github.com/deepaksingh/agent-nexus/services/api/internal/catalog"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
	"github.com/deepaksingh/agent-nexus/services/api/internal/migrate"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider/catalogsync"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/logstream"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
)

func main() {
	logHub := logstream.NewHub()

	// Structured logger
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(logstream.NewHandler(logHub, logLevel, logstream.SourceAPI)))

	// Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	// Database pool
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database pool init failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database connected", "url", maskURL(cfg.DatabaseURL))

	// Apply pending database migrations before accepting traffic
	if err := migrate.Run(ctx, pool); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	// Ensure at least one platform admin exists. If no admin is present, promote
	// the earliest registered user — handles instances where the first user
	// signed up before this logic was added.
	if _, err := pool.Exec(ctx, `
		UPDATE users SET is_admin = true
		WHERE id = (SELECT id FROM users ORDER BY created_at ASC LIMIT 1)
		  AND NOT EXISTS (SELECT 1 FROM users WHERE is_admin = true)
	`); err != nil {
		slog.Warn("bootstrap admin promotion failed", "error", err)
	}

	// Mark orphaned runs as failed. Any run still in running/pending/user_input_wait
	// at startup was left stranded by a previous server crash or restart — their
	// goroutines are gone. Runs in approval_wait/session_wait survive the restart
	// when their loop state is persisted in run_wait_states: they resume on the
	// approval decision or the runner's session callback (see handler.ResumeApprovedRun
	// / handler.ResumeSessionRun). Only waiting runs WITHOUT persisted state
	// (e.g. nested sub-runs, whose parent stack cannot be restored) are failed.
	// Top-level workflow runs (conversation has a workflow_id, no parent_run_id)
	// are excluded from the blanket fail — handler.RecoverInterruptedWorkflowRuns
	// below resumes them from their workflow_checkpoints row instead.
	if t, err := pool.Exec(ctx, `
		UPDATE runs
		SET status='failed', completed_at=NOW(), error_message='Server restarted while run was active'
		WHERE (
			status IN ('running','pending','user_input_wait')
			OR (status IN ('approval_wait','session_wait') AND NOT EXISTS (
			      SELECT 1 FROM run_wait_states w WHERE w.run_id = runs.id))
		)
		AND NOT (
			status = 'running' AND parent_run_id IS NULL AND EXISTS (
				SELECT 1 FROM conversations c WHERE c.id = runs.conversation_id AND c.workflow_id IS NOT NULL
			)
		)
	`); err != nil {
		slog.Warn("failed to mark orphaned runs as failed", "error", err)
	} else if t.RowsAffected() > 0 {
		slog.Info("marked orphaned runs as failed", "count", t.RowsAffected())
	}

	// Build tool registry and seed native tools into DB.
	// In demo mode, skip http_request and write_file to prevent SSRF and disk abuse.
	reg := tools.NewRegistry()
	for _, t := range native.All(pool, cfg) {
		reg.Register(t)
	}
	if err := reg.SeedDB(ctx, pool); err != nil {
		slog.Warn("failed to seed native tools", "error", err)
	} else {
		slog.Info("native tools seeded", "count", len(reg.All()), "demo_mode", cfg.DemoMode)
	}
	if err := syncRequiredSkillTools(ctx, pool); err != nil {
		slog.Warn("failed to sync required skill tools", "error", err)
	}
	// Keep model pricing/context/deprecation data fresh from models.dev.
	catalogsync.Start(ctx)

	// Seed the protected Jira→PR pipeline agents into every workspace.
	// Runs after tool seeding so agent_tools name lookups resolve.
	if err := handler.SeedPipelineAgents(ctx, pool); err != nil {
		slog.Warn("failed to seed pipeline agents", "error", err)
	}
	// Adopt repos already synced by GitHub connectors into the session
	// allowlist (covers connectors synced before adoption existed or while
	// the API was down; per-sync adoption handles the steady state).
	if n, err := catalog.AdoptAllGithubConnectors(ctx, pool); err != nil {
		slog.Warn("failed to adopt connector repos", "error", err)
	} else if n > 0 {
		slog.Info("repos adopted into session allowlist at startup", "repos", n)
	}
	// Card healing must not depend on a GitHub connector existing: allowlist
	// rows outlive a deleted connector, and each needs a searchable card.
	if err := catalog.EnsureCardsForAllWorkspaces(ctx, pool); err != nil {
		slog.Warn("failed to ensure catalog cards", "error", err)
	}
	if err := attachExistingWhatsAppCapabilities(ctx, pool); err != nil {
		slog.Warn("failed to attach whatsapp capabilities", "error", err)
	}
	// Encrypt provider credentials still stored as plaintext in connector
	// configs (rows created before config secrets were encrypted at rest).
	if n, err := connector.EncryptLegacyConfigs(ctx, pool, []byte(cfg.EncryptionKey)); err != nil {
		slog.Warn("failed to encrypt legacy connector config secrets", "error", err)
	} else if n > 0 {
		slog.Info("encrypted plaintext connector config secrets", "connectors", n)
	}
	// Same for MCP server configs (HTTP auth tokens, stdio env credentials).
	if n, err := handler.EncryptLegacyMCPConfigs(ctx, pool, []byte(cfg.EncryptionKey)); err != nil {
		slog.Warn("failed to encrypt legacy mcp config secrets", "error", err)
	} else if n > 0 {
		slog.Info("encrypted plaintext mcp config secrets", "servers", n)
	}
	// Embed chunks that were indexed while the embedder was unreachable —
	// syncs skip unchanged documents, so these never self-heal otherwise.
	handler.StartEmbeddingBackfill(ctx, pool, cfg)
	exec := tools.NewExecutor(reg)

	// Wire handlers — runs needs invokeH set post-construction to avoid circular init.
	runs := handler.NewRunsHandler(pool, cfg, reg, exec)
	invoke := handler.NewInvokeHandler(pool, cfg, runs, reg, exec)
	runs.SetInvokeHandler(invoke)
	nexusAI := handler.NewNexusAIHandler(pool, cfg, runs)
	nexusAI.SetInvokeHandler(invoke)
	evals := handler.NewEvalHandler(pool, cfg, invoke, runs)
	h := &handler.Handlers{
		Auth:            handler.NewAuthHandler(pool, cfg),
		Providers:       handler.NewProvidersHandler(pool, cfg),
		Agents:          handler.NewAgentsHandler(pool, cfg),
		Tools:           handler.NewToolsHandler(pool, cfg),
		MCP:             handler.NewMCPHandler(pool, cfg),
		Connectors:      handler.NewConnectorsHandler(pool, cfg),
		Conversations:   handler.NewConversationsHandler(pool, cfg),
		Workspace:       handler.NewWorkspaceHandler(pool, cfg),
		Runs:            runs,
		Memory:          handler.NewMemoryHandler(pool, cfg),
		Workflows:       handler.NewWorkflowsHandler(pool, cfg),
		Admin:           handler.NewAdminHandler(pool, cfg, logHub),
		APITokens:       handler.NewAPITokensHandler(pool, cfg),
		Invoke:          invoke,
		WebhookTriggers: handler.NewWebhookTriggerHandler(pool, cfg),
		WebhookIngress:  handler.NewWebhookIngressHandler(pool, invoke),
		Gateway:         handler.NewGatewayHandler(pool, cfg, invoke),
		Skills:          handler.NewSkillsHandler(pool, cfg),
		Config:          handler.NewConfigHandler(cfg),
		NexusAI:         nexusAI,
		Observability:   handler.NewObservabilityHandler(pool),
		WhatsAppCreds:   handler.NewWhatsAppCredsHandler(pool, cfg),
		Evals:           evals,
	}
	h.Agents.SetEvalHandler(evals)

	// HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router.New(cfg, h, pool),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // disabled — SSE streams can run indefinitely
		IdleTimeout:  0, // disabled — keep SSE connections open as long as needed
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Re-push callbackUrl to the WhatsApp adapter for every active channel.
	// The adapter runs in the same container but loses in-memory state on restart.
	go func() {
		time.Sleep(3 * time.Second) // wait for adapter subprocess to be ready
		h.Gateway.SyncAllAdapters(context.Background())
	}()

	// Resume any connector syncs that were in-flight when the pod restarted.
	// Runs after a short delay to ensure DB connections are settled.
	go func() {
		time.Sleep(2 * time.Second)
		h.Connectors.RecoverInterruptedSyncs(context.Background())
	}()

	// Resume workflow runs that were in-flight when the pod restarted, using
	// their last persisted workflow_checkpoints row.
	go func() {
		time.Sleep(2 * time.Second)
		invoke.RecoverInterruptedWorkflowRuns(context.Background())
	}()

	// Fire pending WhatsApp reminders as they come due.
	go h.Gateway.StartReminderDispatcher(context.Background())

	// Auto-reconnect stale WhatsApp connections (e.g. after fetchProps timeout).
	go h.Gateway.StartConnectionWatchdog(context.Background())

	// Deliver scheduled outbound messages as they come due.
	go h.Gateway.StartScheduledMessageDispatcher(context.Background())

	// Resume session_wait runs whose runner callback never arrived.
	go invoke.StartSessionWaitWatchdog(context.Background())

	<-quit
	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

func attachExistingWhatsAppCapabilities(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT DISTINCT agent_id::text FROM gateway_channels WHERE channel_type='whatsapp'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var agentID string
		if err := rows.Scan(&agentID); err != nil {
			return err
		}
		if err := handler.AttachWhatsAppCapabilities(ctx, pool, agentID); err != nil {
			return err
		}
	}
	return rows.Err()
}

func syncRequiredSkillTools(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO agent_tools(agent_id, tool_id, enabled)
		SELECT DISTINCT ask.agent_id, t.id, true
		FROM agent_skills ask
		JOIN skills s ON s.id=ask.skill_id AND ask.enabled=true AND s.enabled=true
		JOIN LATERAL unnest(s.required_tool_names) required_tool_name(name) ON true
		JOIN tools t ON t.name=required_tool_name.name AND t.enabled=true
		ON CONFLICT (agent_id, tool_id) DO UPDATE SET enabled=true
	`)
	return err
}

// maskURL hides the password in a DB URL for logging.
func maskURL(url string) string {
	// Simple mask: show scheme and host only
	if len(url) > 30 {
		return url[:15] + "***"
	}
	return "***"
}
