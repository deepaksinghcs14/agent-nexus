package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"text/template"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	mw "github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// ============================================================
// WebhookTriggerHandler — authenticated CRUD
// ============================================================

type WebhookTriggerHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewWebhookTriggerHandler(pool *pgxpool.Pool, cfg *config.Config) *WebhookTriggerHandler {
	return &WebhookTriggerHandler{pool: pool, cfg: cfg}
}

func (h *WebhookTriggerHandler) List(w http.ResponseWriter, r *http.Request) {
	ws := mw.WorkspaceIDFromCtx(r.Context())
	repo := repository.NewWebhookTriggerRepository(h.pool)
	list, err := repo.List(r.Context(), ws)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list webhook triggers"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *WebhookTriggerHandler) Create(w http.ResponseWriter, r *http.Request) {
	ws := mw.WorkspaceIDFromCtx(r.Context())
	uid := mw.UserIDFromCtx(r.Context())

	var body struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		TargetType    string `json:"target_type"`
		TargetID      string `json:"target_id"`
		InputTemplate string `json:"input_template"`
		Secret        string `json:"secret"`
		IsActive      *bool  `json:"is_active"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" || body.TargetType == "" || body.TargetID == "" {
		errs.Write(w, errs.BadRequest("name, target_type, and target_id are required"))
		return
	}
	if body.TargetType != "agent" && body.TargetType != "workflow" {
		errs.Write(w, errs.BadRequest("target_type must be 'agent' or 'workflow'"))
		return
	}

	tpl := body.InputTemplate
	if tpl == "" {
		tpl = "{{.RawBody}}"
	}
	if _, err := template.New("").Parse(tpl); err != nil {
		errs.Write(w, errs.BadRequest("invalid input_template: "+err.Error()))
		return
	}

	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	t := &domain.WebhookTrigger{
		ID:            uuid.NewString(),
		WorkspaceID:   ws,
		Name:          body.Name,
		Description:   body.Description,
		TargetType:    body.TargetType,
		TargetID:      body.TargetID,
		InputTemplate: tpl,
		Secret:        body.Secret,
		IsActive:      isActive,
		CreatedBy:     uid,
	}
	repo := repository.NewWebhookTriggerRepository(h.pool)
	if err := repo.Create(r.Context(), t); err != nil {
		errs.Write(w, errs.Internal("failed to create webhook trigger"))
		return
	}
	writeAudit(r, h.pool, "webhook_trigger.created", "webhook_trigger", t.ID)
	errs.WriteJSON(w, http.StatusCreated, t)
}

func (h *WebhookTriggerHandler) Get(w http.ResponseWriter, r *http.Request) {
	ws := mw.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	repo := repository.NewWebhookTriggerRepository(h.pool)
	t, err := repo.Get(r.Context(), id, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("webhook trigger not found"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, t)
}

func (h *WebhookTriggerHandler) Update(w http.ResponseWriter, r *http.Request) {
	ws := mw.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")

	repo := repository.NewWebhookTriggerRepository(h.pool)
	t, err := repo.Get(r.Context(), id, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("webhook trigger not found"))
		return
	}

	var body struct {
		Name          *string `json:"name"`
		Description   *string `json:"description"`
		TargetType    *string `json:"target_type"`
		TargetID      *string `json:"target_id"`
		InputTemplate *string `json:"input_template"`
		Secret        *string `json:"secret"`
		IsActive      *bool   `json:"is_active"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}

	if body.Name != nil {
		t.Name = *body.Name
	}
	if body.Description != nil {
		t.Description = *body.Description
	}
	if body.TargetType != nil {
		if *body.TargetType != "agent" && *body.TargetType != "workflow" {
			errs.Write(w, errs.BadRequest("target_type must be 'agent' or 'workflow'"))
			return
		}
		t.TargetType = *body.TargetType
	}
	if body.TargetID != nil {
		t.TargetID = *body.TargetID
	}
	if body.InputTemplate != nil {
		if _, err := template.New("").Parse(*body.InputTemplate); err != nil {
			errs.Write(w, errs.BadRequest("invalid input_template: "+err.Error()))
			return
		}
		t.InputTemplate = *body.InputTemplate
	}
	if body.Secret != nil {
		t.Secret = *body.Secret
	}
	if body.IsActive != nil {
		t.IsActive = *body.IsActive
	}

	if err := repo.Update(r.Context(), &t); err != nil {
		errs.Write(w, errs.Internal("failed to update webhook trigger"))
		return
	}
	writeAudit(r, h.pool, "webhook_trigger.updated", "webhook_trigger", id)
	errs.WriteJSON(w, http.StatusOK, t)
}

func (h *WebhookTriggerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ws := mw.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	repo := repository.NewWebhookTriggerRepository(h.pool)
	if err := repo.Delete(r.Context(), id, ws); err != nil {
		errs.Write(w, errs.Internal("failed to delete webhook trigger"))
		return
	}
	writeAudit(r, h.pool, "webhook_trigger.deleted", "webhook_trigger", id)
	w.WriteHeader(http.StatusNoContent)
}

// ============================================================
// WebhookIngressHandler — public, no auth
// ============================================================

type WebhookIngressHandler struct {
	pool   *pgxpool.Pool
	invoke *InvokeHandler
}

func NewWebhookIngressHandler(pool *pgxpool.Pool, invoke *InvokeHandler) *WebhookIngressHandler {
	return &WebhookIngressHandler{pool: pool, invoke: invoke}
}

// Receive handles POST /webhook/:webhookId — the public inbound endpoint.
func (h *WebhookIngressHandler) Receive(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "webhookId")

	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		errs.Write(w, errs.BadRequest("failed to read request body"))
		return
	}

	repo := repository.NewWebhookTriggerRepository(h.pool)
	trig, err := repo.GetByID(r.Context(), webhookID)
	if err != nil || !trig.IsActive {
		errs.Write(w, errs.NotFound("webhook not found"))
		return
	}

	// HMAC-SHA256 verification when a secret is configured.
	if trig.Secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyHMAC(rawBody, trig.Secret, sig) {
			errs.Write(w, errs.Unauthorized("invalid signature"))
			return
		}
	}

	// Build template context.
	var bodyMap map[string]any
	_ = json.Unmarshal(rawBody, &bodyMap) // best-effort; nil if non-JSON body

	headers := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	query := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			query[k] = v[0]
		}
	}

	tplCtx := map[string]any{
		"RawBody": string(rawBody),
		"Body":    bodyMap,
		"Headers": headers,
		"Query":   query,
	}

	input, err := renderTemplate(trig.InputTemplate, tplCtx)
	if err != nil {
		errs.Write(w, errs.BadRequest("input_template render failed: "+err.Error()))
		return
	}
	input = strings.TrimSpace(input)
	if input == "" {
		// An empty render is the template's filter mechanism (e.g. a Jira
		// trigger that only fires for a specific label) — acknowledge the
		// webhook without dispatching a run so the sender doesn't retry.
		errs.WriteJSON(w, http.StatusOK, map[string]any{"status": "skipped", "reason": "input template rendered empty"})
		return
	}

	runID, convID, invokeErr := h.dispatchRun(trig, input)
	if invokeErr != nil {
		errs.Write(w, errs.Internal("failed to dispatch run"))
		return
	}

	// Fire-and-forget — don't block the HTTP response on DB update.
	go repo.IncrementTriggerCount(context.Background(), trig.ID) //nolint:errcheck

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	go writeSystemAudit(context.Background(), h.pool, trig.WorkspaceID, trig.CreatedBy, "", "webhook_trigger.fired", "webhook_trigger", trig.ID, ip)

	errs.WriteJSON(w, http.StatusAccepted, map[string]any{
		"run_id":          runID,
		"conversation_id": convID,
		"status":          "running",
	})
}

// dispatchRun creates the conversation + run records and spawns an async goroutine.
func (h *WebhookIngressHandler) dispatchRun(trig domain.WebhookTrigger, input string) (runID, convID string, err error) {
	ctx := context.Background()
	runID = uuid.NewString()
	convID = uuid.NewString()

	if trig.TargetType == "agent" {
		agentRepo := repository.NewAgentRepository(h.pool)
		a, e := agentRepo.Get(ctx, trig.TargetID, trig.WorkspaceID)
		if e != nil {
			return "", "", e
		}
		if _, e := h.pool.Exec(ctx,
			`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'Webhook Trigger')`,
			convID, trig.WorkspaceID, a.ID, trig.CreatedBy); e != nil {
			return "", "", e
		}
		if _, e := h.pool.Exec(ctx,
			`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
			uuid.NewString(), convID, input); e != nil {
			return "", "", e
		}
		if _, e := h.pool.Exec(ctx,
			`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status,trigger_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running',$7::uuid)`,
			runID, trig.WorkspaceID, a.ID, convID, trig.CreatedBy, input, trig.ID); e != nil {
			return "", "", e
		}
		go h.invoke.executeRun(ctx, a, trig.WorkspaceID, trig.CreatedBy, runID, convID, input, nil, nil, invokeOpts{})
		return runID, convID, nil
	}

	// workflow target
	var gName string
	if e := h.pool.QueryRow(ctx,
		`SELECT name FROM workflows WHERE id=$1::uuid AND workspace_id=$2::uuid AND status='active'`,
		trig.TargetID, trig.WorkspaceID).Scan(&gName); e != nil {
		return "", "", e
	}
	if _, e := h.pool.Exec(ctx,
		`INSERT INTO conversations(id,workspace_id,user_id,title,workflow_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid)`,
		convID, trig.WorkspaceID, trig.CreatedBy, "Webhook: "+gName, trig.TargetID); e != nil {
		return "", "", e
	}
	if _, e := h.pool.Exec(ctx,
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
		uuid.NewString(), convID, input); e != nil {
		return "", "", e
	}
	if _, e := h.pool.Exec(ctx,
		`INSERT INTO runs(id,workspace_id,conversation_id,user_id,input,status,trigger_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,'running',$6::uuid)`,
		runID, trig.WorkspaceID, convID, trig.CreatedBy, input, trig.ID); e != nil {
		return "", "", e
	}
	go h.invoke.executeGroupRun(ctx, trig.TargetID, trig.WorkspaceID, trig.CreatedBy, runID, convID, input, nil, nil)
	return runID, convID, nil
}

// verifyHMAC checks "sha256=<hex>" against HMAC-SHA256(secret, body).
func verifyHMAC(body []byte, secret, header string) bool {
	expected := strings.TrimPrefix(header, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	got := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(got), []byte(expected))
}

// renderTemplate executes a Go text/template with the given context.
func renderTemplate(tpl string, data any) (string, error) {
	t, err := template.New("").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
