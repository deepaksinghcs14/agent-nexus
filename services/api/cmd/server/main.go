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
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/migrate"
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

	// Build tool registry and seed native tools into DB.
	// In demo mode, skip http_request and write_file to prevent SSRF and disk abuse.
	reg := tools.NewRegistry()
	reg.Register(native.NewReadFileTool(cfg.StoragePath))
	reg.Register(native.NewWebSearchTool())
	reg.Register(native.NewSaveMemoryTool(pool))
	reg.Register(native.NewListToolsTool())
	reg.Register(native.NewRequestToolTool())
	for _, t := range native.NewWhatsAppTools(pool, cfg) {
		reg.Register(t)
	}
	if !cfg.DemoMode {
		reg.Register(native.NewWriteFileTool(cfg.StoragePath))
		reg.Register(native.NewHTTPRequestTool())
	}
	if err := reg.SeedDB(ctx, pool); err != nil {
		slog.Warn("failed to seed native tools", "error", err)
	} else {
		slog.Info("native tools seeded", "count", len(reg.All()), "demo_mode", cfg.DemoMode)
	}
	if err := attachExistingWhatsAppCapabilities(ctx, pool); err != nil {
		slog.Warn("failed to attach whatsapp capabilities", "error", err)
	}
	exec := tools.NewExecutor(reg)

	// Wire handlers
	runs := handler.NewRunsHandler(pool, cfg, reg, exec)
	invoke := handler.NewInvokeHandler(pool, cfg, runs, reg, exec)
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
		NexusAI:         handler.NewNexusAIHandler(pool, cfg, runs),
		Observability:   handler.NewObservabilityHandler(pool),
	}

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

	// Fire pending WhatsApp reminders as they come due.
	go h.Gateway.StartReminderDispatcher(context.Background())

	// Auto-reconnect stale WhatsApp connections (e.g. after fetchProps timeout).
	go h.Gateway.StartConnectionWatchdog(context.Background())

	// Deliver scheduled outbound messages as they come due.
	go h.Gateway.StartScheduledMessageDispatcher(context.Background())

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

// maskURL hides the password in a DB URL for logging.
func maskURL(url string) string {
	// Simple mask: show scheme and host only
	if len(url) > 30 {
		return url[:15] + "***"
	}
	return "***"
}
