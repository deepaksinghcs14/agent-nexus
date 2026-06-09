package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentNexus/agent-nexus/services/api/internal/api/handler"
	mw "github.com/agentNexus/agent-nexus/services/api/internal/api/middleware"
	"github.com/agentNexus/agent-nexus/services/api/internal/config"
)

func New(cfg *config.Config, h *handler.Handlers, pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(mw.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check (no auth)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	r.Route("/api/v1", func(r chi.Router) {
		// Public auth routes
		r.Post("/auth/register", h.Auth.Register)
		r.Post("/auth/login", h.Auth.Login)
		r.Post("/auth/refresh", h.Auth.Refresh)

		// Google OAuth callback is public — Google redirects here with no JWT
		r.Get("/providers/oauth/google/callback", h.Providers.OAuthGoogleCallback)

		// Authenticated routes (accept JWT or API token)
		r.Group(func(r chi.Router) {
			r.Use(mw.Authenticate(cfg.JWTSecret, pool))

			r.Post("/auth/logout", h.Auth.Logout)
			r.Get("/auth/me", h.Auth.Me)

			// Workspace (current workspace, scoped by JWT)
			r.Get("/workspace", h.Workspace.Get)
			r.Patch("/workspace", h.Workspace.Update)
			r.Get("/workspace/members", h.Workspace.ListMembers)
			r.Post("/workspace/members", h.Workspace.AddMember)
			r.Patch("/workspace/members/{id}", h.Workspace.UpdateMember)
			r.Delete("/workspace/members/{id}", h.Workspace.RemoveMember)

			// Workspaces (user's full list + create/switch)
			r.Get("/workspaces", h.Workspace.List)
			r.Post("/workspaces", h.Workspace.Create)
			r.Delete("/workspaces/{id}", h.Workspace.Delete)
			r.Post("/workspaces/switch", h.Auth.Switch)

			// API tokens
			r.Get("/api-tokens", h.APITokens.List)
			r.Post("/api-tokens", h.APITokens.Create)
			r.Delete("/api-tokens/{id}", h.APITokens.Revoke)

			// Invoke (stateless agent/group execution via API token)
			r.Post("/invoke/agents/{agentId}", h.Invoke.Agent)
			r.Post("/invoke/groups/{groupId}", h.Invoke.Group)

			// Providers
			r.Get("/providers", h.Providers.List)
			r.Post("/providers", h.Providers.Create)
			r.Put("/providers/{id}", h.Providers.Update)
			r.Delete("/providers/{id}", h.Providers.Delete)
			r.Get("/providers/{id}/models", h.Providers.ListModels)
			r.Get("/providers/oauth/google/authorize", h.Providers.OAuthGoogleAuthorize)

			// Agents
			r.Get("/agents", h.Agents.List)
			r.Post("/agents", h.Agents.Create)
			r.Get("/agents/{id}", h.Agents.Get)
			r.Put("/agents/{id}", h.Agents.Update)
			r.Delete("/agents/{id}", h.Agents.Delete)
			r.Get("/agents/{id}/tools", h.Agents.ListTools)
			r.Put("/agents/{id}/tools", h.Agents.SetTools)
			r.Get("/agents/{id}/connectors", h.Agents.ListConnectors)
			r.Put("/agents/{id}/connectors", h.Agents.SetConnectors)

			// Tools
			r.Get("/tools", h.Tools.List)
			r.Post("/tools", h.Tools.Create)
			r.Get("/tools/{id}", h.Tools.Get)
			r.Put("/tools/{id}", h.Tools.Update)
			r.Delete("/tools/{id}", h.Tools.Delete)

			// MCP Servers
			r.Get("/mcp-servers", h.MCP.List)
			r.Post("/mcp-servers", h.MCP.Create)
			r.Get("/mcp-servers/{id}", h.MCP.Get)
			r.Delete("/mcp-servers/{id}", h.MCP.Delete)
			r.Post("/mcp-servers/{id}/sync", h.MCP.Sync)
			r.Get("/mcp-servers/{id}/tools", h.MCP.ListTools)
			r.Patch("/mcp-servers/{id}/tools/{toolId}", h.MCP.UpdateToolRisk)

			// Connectors
			r.Get("/connectors", h.Connectors.List)
			r.Post("/connectors", h.Connectors.Create)
			r.Get("/connectors/{id}", h.Connectors.Get)
			r.Delete("/connectors/{id}", h.Connectors.Delete)
			r.Post("/connectors/{id}/sync", h.Connectors.Sync)
			r.Get("/connectors/{id}/documents", h.Connectors.ListDocuments)
			r.Get("/connectors/{id}/sync-jobs", h.Connectors.ListSyncJobs)
			r.Get("/filesystem/browse", h.Connectors.BrowseFilesystem)

			// Conversations
			r.Get("/conversations", h.Conversations.List)
			r.Post("/conversations", h.Conversations.Create)
			r.Get("/conversations/{id}", h.Conversations.Get)
			r.Delete("/conversations/{id}", h.Conversations.Delete)
			r.Post("/conversations/{id}/runs", h.Runs.Start) // SSE stream
			r.Get("/conversations/{id}/runs", h.Runs.ListByConversation)

			// Runs
			r.Get("/runs", h.Runs.List)
			r.Get("/runs/{id}", h.Runs.Get)
			r.Post("/runs/{id}/approve", h.Runs.Approve)
			r.Post("/runs/{id}/cancel", h.Runs.Cancel)

			// Memory
			r.Get("/memory", h.Memory.List)
			r.Delete("/memory/{id}", h.Memory.Delete)
			r.Delete("/memory", h.Memory.BulkDelete)

			// Agent Groups
			r.Get("/agent-groups", h.Groups.List)
			r.Post("/agent-groups", h.Groups.Create)
			r.Get("/agent-groups/{id}", h.Groups.Get)
			r.Put("/agent-groups/{id}", h.Groups.Update)
			r.Delete("/agent-groups/{id}", h.Groups.Delete)
			r.Post("/agent-groups/{id}/runs", h.Groups.Run)
			r.Get("/agent-groups/{id}/graph", h.Groups.GetGraph)
			r.Put("/agent-groups/{id}/graph", h.Groups.SaveGraph)

			// Admin (requires is_admin flag)
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireAdmin)

				r.Get("/admin/users", h.Admin.ListUsers)
				r.Get("/admin/users/{id}", h.Admin.GetUser)
				r.Patch("/admin/users/{id}", h.Admin.UpdateUser)
				r.Get("/admin/workspaces", h.Admin.ListWorkspaces)
				r.Patch("/admin/workspaces/{id}", h.Admin.UpdateWorkspace)
				r.Get("/admin/audit-logs", h.Admin.AuditLogs)
				r.Get("/admin/usage", h.Admin.Usage)
				r.Get("/admin/policies", h.Admin.GetPolicies)
				r.Put("/admin/policies", h.Admin.SetPolicies)
			})
		})
	})

	return r
}
