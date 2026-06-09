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
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
)

func main() {
	// Structured logger
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

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

	// Build tool registry and seed native tools into DB
	reg := tools.NewRegistry()
	reg.Register(native.NewReadFileTool(cfg.StoragePath))
	reg.Register(native.NewWriteFileTool(cfg.StoragePath))
	reg.Register(native.NewWebSearchTool())
	reg.Register(native.NewHTTPRequestTool())
	if err := reg.SeedDB(ctx, pool); err != nil {
		slog.Warn("failed to seed native tools", "error", err)
	} else {
		slog.Info("native tools seeded", "count", len(reg.All()))
	}
	exec := tools.NewExecutor(reg)

	// Wire handlers
	runs := handler.NewRunsHandler(pool, cfg, reg, exec)
	h := &handler.Handlers{
		Auth:          handler.NewAuthHandler(pool, cfg),
		Providers:     handler.NewProvidersHandler(pool, cfg),
		Agents:        handler.NewAgentsHandler(pool, cfg),
		Tools:         handler.NewToolsHandler(pool, cfg),
		MCP:           handler.NewMCPHandler(pool, cfg),
		Connectors:    handler.NewConnectorsHandler(pool, cfg),
		Conversations: handler.NewConversationsHandler(pool, cfg),
		Workspace:     handler.NewWorkspaceHandler(pool, cfg),
		Runs:          runs,
		Memory:        handler.NewMemoryHandler(pool, cfg),
		Groups:        handler.NewGroupsHandler(pool, cfg),
		Admin:         handler.NewAdminHandler(pool, cfg),
		APITokens:     handler.NewAPITokensHandler(pool, cfg),
		Invoke:        handler.NewInvokeHandler(pool, cfg, runs, reg, exec),
	}

	// HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router.New(cfg, h, pool),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second, // long for SSE streams
		IdleTimeout:  60 * time.Second,
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

	<-quit
	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	slog.Info("server stopped")
}

// maskURL hides the password in a DB URL for logging.
func maskURL(url string) string {
	// Simple mask: show scheme and host only
	if len(url) > 30 {
		return url[:15] + "***"
	}
	return "***"
}
