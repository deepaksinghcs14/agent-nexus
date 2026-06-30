package handler

import (
	"encoding/json"
	"fmt"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/internal/runtime/logstream"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strings"
	"time"
)

type AdminHandler struct {
	pool   *pgxpool.Pool
	cfg    *config.Config
	logHub *logstream.Hub
}

func NewAdminHandler(p *pgxpool.Pool, c *config.Config, logHub *logstream.Hub) *AdminHandler {
	return &AdminHandler{pool: p, cfg: c, logHub: logHub}
}
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	a, e := repository.NewUserRepository(h.pool).List(r.Context())
	if e != nil {
		errs.Write(w, errs.Internal("failed to list users"))
		return
	}
	if a == nil {
		a = []domain.User{}
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	u, e := repository.NewUserRepository(h.pool).GetByID(r.Context(), chi.URLParam(r, "id"))
	if e != nil {
		errs.Write(w, errs.NotFound("user not found"))
		return
	}
	errs.WriteJSON(w, 200, u)
}
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	repo := repository.NewUserRepository(h.pool)
	u, e := repo.GetByID(r.Context(), chi.URLParam(r, "id"))
	if e != nil {
		errs.Write(w, errs.NotFound("user not found"))
		return
	}
	var q struct {
		FullName *string `json:"full_name"`
		IsActive *bool   `json:"is_active"`
		IsAdmin  *bool   `json:"is_admin"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if q.FullName != nil {
		u.FullName = *q.FullName
	}
	if q.IsActive != nil {
		u.IsActive = *q.IsActive
	}
	if q.IsAdmin != nil {
		u.IsAdmin = *q.IsAdmin
	}
	if repo.Update(r.Context(), u) != nil {
		errs.Write(w, errs.Internal("failed to update user"))
		return
	}
	errs.WriteJSON(w, 200, u)
}
func (h *AdminHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), `
		SELECT w.id::text, w.name, w.display_name, w.owner_id::text, w.settings, w.workspace_type, w.created_at, w.updated_at,
		       COALESCE(a.cnt,0), COALESCE(r.cnt,0), COALESCE(r.tokens,0)
		FROM workspaces w
		LEFT JOIN (SELECT workspace_id, COUNT(*) AS cnt FROM agents WHERE ephemeral=false GROUP BY workspace_id) a ON a.workspace_id=w.id
		LEFT JOIN (SELECT workspace_id, COUNT(*) AS cnt, COALESCE(SUM(total_input_tokens+total_output_tokens),0) AS tokens FROM runs GROUP BY workspace_id) r ON r.workspace_id=w.id
		ORDER BY w.created_at DESC`)
	if e != nil {
		errs.Write(w, errs.Internal("failed to list workspaces"))
		return
	}
	defer rows.Close()
	type wsRow struct {
		domain.Workspace
		AgentCount  int64 `json:"agent_count"`
		RunCount    int64 `json:"run_count"`
		TotalTokens int64 `json:"total_tokens"`
	}
	a := []wsRow{}
	for rows.Next() {
		var x wsRow
		if rows.Scan(&x.ID, &x.Name, &x.DisplayName, &x.OwnerID, &x.Settings, &x.WorkspaceType, &x.CreatedAt, &x.UpdatedAt, &x.AgentCount, &x.RunCount, &x.TotalTokens) != nil {
			errs.Write(w, errs.Internal("failed to read workspaces"))
			return
		}
		a = append(a, x)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}

func (h *AdminHandler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var memberCount int
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM workspace_members WHERE workspace_id=$1::uuid`, id).Scan(&memberCount)
	if memberCount > 1 {
		errs.Write(w, errs.BadRequest("workspace still has members — remove all members before deleting"))
		return
	}
	tag, e := h.pool.Exec(r.Context(), `DELETE FROM workspaces WHERE id=$1::uuid`, id)
	if e != nil || tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("workspace not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *AdminHandler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	var q struct {
		DisplayName string          `json:"display_name"`
		Settings    json.RawMessage `json:"settings"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if len(q.Settings) == 0 {
		q.Settings = json.RawMessage(`{}`)
	}
	var x domain.Workspace
	e := h.pool.QueryRow(r.Context(), `UPDATE workspaces SET display_name=COALESCE(NULLIF($2,''),display_name),settings=$3,updated_at=NOW() WHERE id=$1::uuid RETURNING id::text,name,display_name,owner_id::text,settings,workspace_type,created_at,updated_at`, chi.URLParam(r, "id"), q.DisplayName, q.Settings).Scan(&x.ID, &x.Name, &x.DisplayName, &x.OwnerID, &x.Settings, &x.WorkspaceType, &x.CreatedAt, &x.UpdatedAt)
	if e != nil {
		errs.Write(w, errs.NotFound("workspace not found"))
		return
	}
	errs.WriteJSON(w, 200, x)
}
func (h *AdminHandler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resourceType := q.Get("resource_type")
	actorEmail := q.Get("actor_email")
	rows, e := h.pool.Query(r.Context(),
		`SELECT id::text,COALESCE(workspace_id::text,''),COALESCE(actor_id::text,''),actor_email,action,resource_type,resource_id,metadata,ip_address,created_at
		 FROM admin_audit_logs
		 WHERE ($1='' OR resource_type=$1) AND ($2='' OR actor_email ILIKE '%'||$2||'%')
		 ORDER BY created_at DESC LIMIT 200`,
		resourceType, actorEmail)
	if e != nil {
		errs.Write(w, errs.Internal("failed to list audit logs"))
		return
	}
	defer rows.Close()
	a := []domain.AuditLog{}
	for rows.Next() {
		var x domain.AuditLog
		if rows.Scan(&x.ID, &x.WorkspaceID, &x.ActorID, &x.ActorEmail, &x.Action, &x.ResourceType, &x.ResourceID, &x.Metadata, &x.IPAddress, &x.CreatedAt) != nil {
			errs.Write(w, errs.Internal("failed to read audit logs"))
			return
		}
		a = append(a, x)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}

func (h *AdminHandler) ServiceLogStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		errs.Write(w, errs.Internal("streaming is not supported"))
		return
	}
	if h.logHub == nil {
		errs.Write(w, errs.Internal("log stream is not configured"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.logHub.Subscribe()
	defer h.logHub.Unsubscribe(ch)

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (h *AdminHandler) IngestServiceLog(w http.ResponseWriter, r *http.Request) {
	if h.cfg.LogStreamIngestToken == "" {
		errs.Write(w, errs.Forbidden("log ingest is not configured"))
		return
	}
	token := r.Header.Get("X-Log-Stream-Token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if token != h.cfg.LogStreamIngestToken {
		errs.Write(w, errs.Forbidden("invalid log ingest token"))
		return
	}
	if h.logHub == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var entry logstream.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		errs.Write(w, errs.BadRequest("invalid log entry"))
		return
	}
	if entry.Source == "" {
		entry.Source = "process"
	}
	entry.Level = strings.ToLower(entry.Level)
	if entry.Level == "" {
		entry.Level = "info"
	}
	h.logHub.Publish(entry)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) Usage(w http.ResponseWriter, r *http.Request) {
	var totalRuns, totalTokens int
	var totalCost float64
	var webhookTriggers, totalWorkspaces, totalAgents, totalConnectors, totalGatewayChannels, totalEvalSuites int
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*),COALESCE(SUM(total_input_tokens+total_output_tokens),0),COALESCE(SUM(cost_estimate),0) FROM runs`).Scan(&totalRuns, &totalTokens, &totalCost)
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM webhook_triggers`).Scan(&webhookTriggers)
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM workspaces`).Scan(&totalWorkspaces)
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM agents WHERE ephemeral=false`).Scan(&totalAgents)
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM connectors`).Scan(&totalConnectors)
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM gateway_channels`).Scan(&totalGatewayChannels)
	_ = h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM eval_suites`).Scan(&totalEvalSuites)

	// Top 5 workspaces by run count
	type topWs struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"display_name"`
		Runs        int     `json:"runs"`
		Tokens      int64   `json:"tokens"`
		Cost        float64 `json:"cost"`
	}
	topRows, _ := h.pool.Query(r.Context(), `
		SELECT w.id::text, w.display_name, COUNT(r.id), COALESCE(SUM(r.total_input_tokens+r.total_output_tokens),0), COALESCE(SUM(r.cost_estimate),0)
		FROM workspaces w
		LEFT JOIN runs r ON r.workspace_id=w.id
		GROUP BY w.id, w.display_name
		ORDER BY COUNT(r.id) DESC
		LIMIT 5`)
	topWorkspaces := []topWs{}
	if topRows != nil {
		defer topRows.Close()
		for topRows.Next() {
			var t topWs
			if topRows.Scan(&t.ID, &t.DisplayName, &t.Runs, &t.Tokens, &t.Cost) == nil {
				topWorkspaces = append(topWorkspaces, t)
			}
		}
	}

	errs.WriteJSON(w, 200, map[string]any{
		"runs":                  totalRuns,
		"tokens":                totalTokens,
		"cost":                  totalCost,
		"webhook_triggers":      webhookTriggers,
		"total_workspaces":      totalWorkspaces,
		"total_agents":          totalAgents,
		"total_connectors":      totalConnectors,
		"total_gateway_channels": totalGatewayChannels,
		"total_eval_suites":     totalEvalSuites,
		"top_workspaces":        topWorkspaces,
	})
}
func (h *AdminHandler) GetPolicies(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), `SELECT id::text,COALESCE(workspace_id::text,''),key,value,created_at,updated_at FROM policies ORDER BY key`)
	if e != nil {
		errs.Write(w, errs.Internal("failed to list policies"))
		return
	}
	defer rows.Close()
	a := []domain.Policy{}
	for rows.Next() {
		var x domain.Policy
		if rows.Scan(&x.ID, &x.WorkspaceID, &x.Key, &x.Value, &x.CreatedAt, &x.UpdatedAt) != nil {
			errs.Write(w, errs.Internal("failed to read policies"))
			return
		}
		a = append(a, x)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
func (h *AdminHandler) SetPolicies(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Policies []struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		} `json:"policies"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	tx, e := h.pool.Begin(r.Context())
	if e != nil {
		errs.Write(w, errs.Internal("failed to save policies"))
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	for _, p := range q.Policies {
		_, e = tx.Exec(r.Context(), `INSERT INTO policies(workspace_id,key,value)VALUES(NULL,$1,$2)ON CONFLICT(workspace_id,key)DO UPDATE SET value=$2,updated_at=NOW()`, p.Key, p.Value)
		if e != nil {
			errs.Write(w, errs.Internal("failed to save policies"))
			return
		}
	}
	if tx.Commit(r.Context()) != nil {
		errs.Write(w, errs.Internal("failed to save policies"))
		return
	}
	errs.WriteJSON(w, 200, map[string]any{"status": "saved"})
}
