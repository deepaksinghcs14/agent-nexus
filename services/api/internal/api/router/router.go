package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/handler"
	mw "github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
)

func New(cfg *config.Config, h *handler.Handlers, pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
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

	// Public config — returns feature flags like demo_mode (no auth required)
	r.Get("/api/v1/config", h.Config.Get)

	// Internal process log ingestion, protected by LOG_STREAM_INGEST_TOKEN.
	r.Post("/internal/service-logs/ingest", h.Admin.IngestServiceLog)

	// Internal WhatsApp credential persistence — adapter-only, no JWT.
	r.Get("/internal/whatsapp/{accountId}/credentials", h.WhatsAppCreds.Get)
	r.Put("/internal/whatsapp/{accountId}/credentials", h.WhatsAppCreds.Put)
	r.Delete("/internal/whatsapp/{accountId}/credentials", h.WhatsAppCreds.Delete)
	r.Put("/internal/whatsapp/{accountId}/lid-map", h.WhatsAppCreds.PutLIDMap)

	// Runner session completion callbacks — verified by RUNNER_CALLBACK_SECRET.
	r.Post("/internal/sessions/callback", h.Invoke.SessionCallback)

	// Public webhook inbound endpoint (no auth — verified by per-trigger HMAC secret)
	r.Post("/webhook/{webhookId}", h.WebhookIngress.Receive)
	r.Post("/gateway/whatsapp/{channelId}", h.Gateway.WhatsAppReceive)
	r.Post("/gateway/http/{channelId}", h.Gateway.HTTPReceive)

	r.Route("/api/v1", func(r chi.Router) {
		// Public auth routes
		r.Post("/auth/register", h.Auth.Register)
		r.Post("/auth/login", h.Auth.Login)
		r.Post("/auth/refresh", h.Auth.Refresh)

		// Google OAuth callback is public — Google redirects here with no JWT
		r.Get("/providers/oauth/google/callback", h.Providers.OAuthGoogleCallback)

		// MCP OAuth browser redirect — unauthenticated; verified by the state
		// nonce persisted when the flow started.
		r.Get("/mcp-servers/oauth/callback", h.MCP.OAuthCallback)

		// Authenticated routes (accept JWT or API token)
		r.Group(func(r chi.Router) {
			r.Use(mw.Authenticate(cfg.JWTSecret, pool))

			r.Post("/auth/logout", h.Auth.Logout)
			r.Get("/auth/me", h.Auth.Me)

			// Workspace (current workspace, scoped by JWT)
			r.Get("/workspace", h.Workspace.Get)
			r.Patch("/workspace", h.Workspace.Update)
			r.Get("/workspace/runner-credentials", h.Workspace.GetRunnerCredentials)
			r.Put("/workspace/runner-credentials", h.Workspace.PutRunnerCredentials)
			r.Delete("/workspace/runner-credentials", h.Workspace.DeleteRunnerCredentials)
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

			// Invoke (stateless agent/workflow execution via API token)
			r.Post("/invoke/agents/{agentId}", h.Invoke.Agent)
			r.Post("/invoke/workflows/{workflowId}", h.Invoke.Group)

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
			r.Post("/agents/import", h.Agents.Import)
			r.Get("/agents/{id}", h.Agents.Get)
			r.Put("/agents/{id}", h.Agents.Update)
			r.Delete("/agents/{id}", h.Agents.Delete)
			r.Get("/agents/{id}/export", h.Agents.Export)
			r.Get("/agents/{id}/tools", h.Agents.ListTools)
			r.Put("/agents/{id}/tools", h.Agents.SetTools)
			r.Get("/agents/{id}/connectors", h.Agents.ListConnectors)
			r.Put("/agents/{id}/connectors", h.Agents.SetConnectors)

			// Tools
			r.Get("/tools", h.Tools.List)
			r.Post("/tools", h.Tools.Create)
			r.Post("/tools/test-code", h.Tools.TestCode)
			r.Get("/tools/{id}", h.Tools.Get)
			r.Put("/tools/{id}", h.Tools.Update)
			r.Delete("/tools/{id}", h.Tools.Delete)

			// MCP Servers
			r.Get("/mcp-servers", h.MCP.List)
			r.Post("/mcp-servers", h.MCP.Create)
			r.Get("/mcp-servers/{id}", h.MCP.Get)
			r.Delete("/mcp-servers/{id}", h.MCP.Delete)
			r.Post("/mcp-servers/{id}/sync", h.MCP.Sync)
			r.Post("/mcp-servers/{id}/oauth/start", h.MCP.OAuthStart)
			r.Get("/mcp-servers/{id}/tools", h.MCP.ListTools)
			r.Patch("/mcp-servers/{id}/tools/{toolId}", h.MCP.UpdateToolRisk)

			// Connectors
			r.Get("/connectors", h.Connectors.List)
			r.Post("/connectors", h.Connectors.Create)
			r.Get("/connectors/{id}", h.Connectors.Get)
			r.Delete("/connectors/{id}", h.Connectors.Delete)
			r.Post("/connectors/{id}/sync", h.Connectors.Sync)
			r.Get("/connectors/{id}/documents", h.Connectors.ListDocuments)
			r.Get("/connectors/{id}/browse", h.Connectors.BrowseDocuments)
			r.Get("/connectors/{id}/document-groups", h.Connectors.ListDocumentGroups)
			r.Get("/connectors/{id}/sync-jobs", h.Connectors.ListSyncJobs)
			r.Get("/filesystem/browse", h.Connectors.BrowseFilesystem)

			// Conversations
			r.Get("/conversations", h.Conversations.List)
			r.Post("/conversations", h.Conversations.Create)
			r.Delete("/conversations", h.Conversations.DeleteAll)
			r.Get("/conversations/{id}", h.Conversations.Get)
			r.Delete("/conversations/{id}", h.Conversations.Delete)
			r.Post("/conversations/{id}/runs", h.Runs.Start) // SSE stream
			r.Get("/conversations/{id}/runs", h.Runs.ListByConversation)

			// Runs
			r.Get("/runs", h.Runs.List)
			r.Get("/runs/stats", h.Runs.Stats)
			r.Get("/runs/{id}", h.Runs.Get)
			r.Get("/runs/{id}/children", h.Runs.ListChildren)
			r.Post("/runs/{id}/approve", h.Runs.Approve)
			r.Post("/runs/{id}/input", h.Runs.SubmitUserInput)
			r.Post("/runs/{id}/cancel", h.Runs.Cancel)

			// Memory
			r.Get("/memory", h.Memory.List)
			r.Patch("/memory/{id}/approve", h.Memory.Approve)
			r.Patch("/memory/{id}/reject", h.Memory.Reject)
			r.Delete("/memory/{id}", h.Memory.Delete)
			r.Delete("/memory", h.Memory.BulkDelete)

			// Webhook triggers
			r.Get("/webhook-triggers", h.WebhookTriggers.List)
			r.Post("/webhook-triggers", h.WebhookTriggers.Create)
			r.Get("/webhook-triggers/{id}", h.WebhookTriggers.Get)
			r.Put("/webhook-triggers/{id}", h.WebhookTriggers.Update)
			r.Delete("/webhook-triggers/{id}", h.WebhookTriggers.Delete)

			// Gateway channels
			r.Route("/gateway", func(r chi.Router) {
				r.Get("/channels", h.Gateway.ListChannels)
				r.Post("/channels", h.Gateway.CreateChannel)
				r.Get("/channels/{id}", h.Gateway.GetChannel)
				r.Put("/channels/{id}", h.Gateway.UpdateChannel)
				r.Delete("/channels/{id}", h.Gateway.DeleteChannel)
				r.Get("/channels/{id}/adapter/status", h.Gateway.AdapterStatus)
				r.Post("/channels/{id}/adapter/login/start", h.Gateway.AdapterLoginStart)
				r.Get("/channels/{id}/adapter/login/qr", h.Gateway.AdapterQR)
				r.Post("/channels/{id}/adapter/login/pairing-code", h.Gateway.AdapterPairingCode)
				r.Post("/channels/{id}/adapter/logout", h.Gateway.AdapterLogout)
				r.Post("/channels/{id}/adapter/sync-lids", h.Gateway.AdapterSyncLIDs)
				r.Post("/channels/{id}/adapter/sync-contacts", h.Gateway.AdapterSyncContacts)
				r.Get("/sessions", h.Gateway.ListSessions)
				r.Delete("/sessions/{id}", h.Gateway.DeleteSession)
				r.Get("/events", h.Gateway.ListEvents)
				r.Get("/pairings", h.Gateway.ListPairings)
				r.Post("/pairings/{id}/approve", h.Gateway.ApprovePairing)
				r.Post("/pairings/{id}/reject", h.Gateway.RejectPairing)
				r.Get("/outbox", h.Gateway.ListOutbox)
				r.Get("/reminders", h.Gateway.ListReminders)
				r.Get("/scheduled-messages", h.Gateway.ListScheduledMessages)
				r.Post("/scheduled-messages", h.Gateway.CreateScheduledMessage)
				r.Get("/scheduled-messages/{id}", h.Gateway.GetScheduledMessage)
				r.Delete("/scheduled-messages/{id}", h.Gateway.DeleteScheduledMessage)
				r.Get("/escalations", h.Gateway.ListEscalations)
				r.Post("/escalations/{id}/approve", h.Gateway.ApproveEscalation)
				r.Post("/escalations/{id}/reject", h.Gateway.RejectEscalation)
				r.Get("/contacts", h.Gateway.ListContacts)
				r.Post("/contacts", h.Gateway.CreateContact)
				r.Put("/contacts/{id}", h.Gateway.UpdateContact)
				r.Delete("/contacts/{id}", h.Gateway.DeleteContact)
			})

			// Skills
			r.Get("/skills", h.Skills.List)
			r.Post("/skills", h.Skills.Create)
			r.Get("/skills/{id}", h.Skills.Get)
			r.Put("/skills/{id}", h.Skills.Update)
			r.Delete("/skills/{id}", h.Skills.Delete)
			r.Get("/agents/{id}/skills", h.Skills.ListForAgent)
			r.Put("/agents/{id}/skills", h.Skills.SetForAgent)

			// Workflows
			r.Get("/workflows", h.Workflows.List)
			r.Post("/workflows", h.Workflows.Create)
			r.Get("/workflows/{id}", h.Workflows.Get)
			r.Put("/workflows/{id}", h.Workflows.Update)
			r.Delete("/workflows/{id}", h.Workflows.Delete)
			r.Post("/workflows/{id}/runs", h.Workflows.Run)
			r.Get("/workflows/{id}/graph", h.Workflows.GetGraph)
			r.Put("/workflows/{id}/graph", h.Workflows.SaveGraph)

			// Observability
			r.Get("/observability/latency", h.Observability.Latency)

			// Nexus AI meta-agent
			r.Post("/nexus-ai/chat", h.NexusAI.Chat)

			// Eval framework
			r.Route("/evals", func(r chi.Router) {
				r.Get("/suites", h.Evals.ListSuites)
				r.Post("/suites", h.Evals.CreateSuite)
				r.Get("/suites/{id}", h.Evals.GetSuite)
				r.Put("/suites/{id}", h.Evals.UpdateSuite)
				r.Delete("/suites/{id}", h.Evals.DeleteSuite)
				r.Post("/suites/{id}/cases", h.Evals.CreateCase)
				r.Put("/suites/{id}/cases/{caseId}", h.Evals.UpdateCase)
				r.Delete("/suites/{id}/cases/{caseId}", h.Evals.DeleteCase)
				r.Post("/suites/{id}/generate-cases", h.Evals.GenerateCases)
				r.Get("/suites/{id}/cases/export", h.Evals.ExportCases)
				r.Post("/suites/{id}/cases/import", h.Evals.ImportCases)
				r.Post("/suites/{id}/runs", h.Evals.TriggerRun)
				r.Get("/suites/{id}/runs", h.Evals.ListRuns)
				r.Get("/runs/{runId}", h.Evals.GetRun)
				r.Post("/runs/{runId}/analyze", h.Evals.AnalyzeRun)
				r.Patch("/runs/{runId}/results/{resultId}", h.Evals.OverrideResult)
				r.Post("/runs/{runId}/results/{resultId}/fix-case", h.Evals.FixCase)
			})

			// Admin (requires is_admin flag)
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireAdmin)

				r.Get("/admin/users", h.Admin.ListUsers)
				r.Get("/admin/users/{id}", h.Admin.GetUser)
				r.Patch("/admin/users/{id}", h.Admin.UpdateUser)
				r.Get("/admin/workspaces", h.Admin.ListWorkspaces)
				r.Patch("/admin/workspaces/{id}", h.Admin.UpdateWorkspace)
				r.Delete("/admin/workspaces/{id}", h.Admin.DeleteWorkspace)
				r.Get("/admin/audit-logs", h.Admin.AuditLogs)
				r.Get("/admin/service-logs/stream", h.Admin.ServiceLogStream)
				r.Get("/admin/usage", h.Admin.Usage)
				r.Get("/admin/policies", h.Admin.GetPolicies)
				r.Put("/admin/policies", h.Admin.SetPolicies)
			})
		})
	})

	return r
}
