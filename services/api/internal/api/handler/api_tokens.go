package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type APITokensHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewAPITokensHandler(pool *pgxpool.Pool, cfg *config.Config) *APITokensHandler {
	return &APITokensHandler{pool: pool, cfg: cfg}
}

type apiToken struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      []string   `json:"scopes"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (h *APITokensHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	rows, err := h.pool.Query(r.Context(),
		`SELECT id::text, name, token_prefix, scopes, last_used_at, expires_at, created_at
		 FROM api_tokens
		 WHERE workspace_id=$1::uuid AND user_id=$2::uuid AND revoked_at IS NULL
		 ORDER BY created_at DESC`,
		wsID, uid)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list API tokens"))
		return
	}
	defer rows.Close()

	list := []apiToken{}
	for rows.Next() {
		var t apiToken
		var scopes []string
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenPrefix, &scopes, &t.LastUsedAt, &t.ExpiresAt, &t.CreatedAt); err != nil {
			errs.Write(w, errs.Internal("failed to read API token"))
			return
		}
		t.Scopes = scopes
		if t.Scopes == nil {
			t.Scopes = []string{}
		}
		list = append(list, t)
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
}

func (h *APITokensHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.cfg.DemoMode {
		errs.Write(w, errs.Forbidden("API token creation is not available in demo mode"))
		return
	}
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	var req struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
		Scopes    []string   `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		errs.Write(w, errs.BadRequest("name is required"))
		return
	}
	if req.Scopes == nil {
		req.Scopes = []string{}
	}

	// Generate "anx_" + 40 hex chars (20 random bytes)
	rawBytes := make([]byte, 20)
	if _, err := rand.Read(rawBytes); err != nil {
		errs.Write(w, errs.Internal("failed to generate token"))
		return
	}
	rawToken := "anx_" + hex.EncodeToString(rawBytes)
	h256 := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(h256[:])
	tokenPrefix := rawToken[:12] // "anx_" + 8 hex chars

	id := uuid.NewString()
	var createdAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO api_tokens(id, workspace_id, user_id, name, token_hash, token_prefix, scopes, expires_at)
		 VALUES($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8)
		 RETURNING created_at`,
		id, wsID, uid, req.Name, tokenHash, tokenPrefix, req.Scopes, req.ExpiresAt,
	).Scan(&createdAt)
	if err != nil {
		errs.Write(w, errs.Internal("failed to create API token"))
		return
	}

	writeAudit(r, h.pool, "api_token.created", "api_token", id)

	// Return the raw token exactly once — not stored, cannot be recovered
	errs.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":           id,
		"token":        rawToken,
		"name":         req.Name,
		"token_prefix": tokenPrefix,
		"scopes":       req.Scopes,
		"expires_at":   req.ExpiresAt,
		"created_at":   createdAt,
	})
}

func (h *APITokensHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "id")
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE api_tokens SET revoked_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid AND user_id=$3::uuid AND revoked_at IS NULL`,
		tokenID, wsID, uid)
	if err != nil {
		errs.Write(w, errs.Internal("failed to revoke token"))
		return
	}
	if tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("token not found"))
		return
	}

	writeAudit(r, h.pool, "api_token.revoked", "api_token", tokenID)
	w.WriteHeader(http.StatusNoContent)
}
