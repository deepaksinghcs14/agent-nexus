package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkspaceHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

type workspaceMember struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FullName  string `json:"full_name"`
	AvatarURL string `json:"avatar_url"`
	IsActive  bool   `json:"is_active"`
	Role      string `json:"role"`
	JoinedAt  string `json:"joined_at"`
}

func NewWorkspaceHandler(p *pgxpool.Pool, c *config.Config) *WorkspaceHandler {
	return &WorkspaceHandler{p, c}
}

func (h *WorkspaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	ws, err := repository.NewUserRepository(h.pool).GetWorkspaceByID(r.Context(), wsID)
	if err != nil {
		errs.Write(w, errs.NotFound("workspace not found"))
		return
	}
	role, _ := h.roleFor(r.Context(), middleware.UserIDFromCtx(r.Context()), wsID)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"workspace": ws, "role": role})
}

func (h *WorkspaceHandler) Update(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	if !h.canManage(r.Context(), middleware.UserIDFromCtx(r.Context()), wsID) {
		errs.Write(w, errs.Forbidden("workspace admin access required"))
		return
	}
	var q struct {
		DisplayName   string          `json:"display_name"`
		WorkspaceType string          `json:"workspace_type"`
		Settings      json.RawMessage `json:"settings"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if strings.TrimSpace(q.DisplayName) == "" {
		errs.Write(w, errs.BadRequest("workspace name is required"))
		return
	}
	if !validWorkspaceTypes[q.WorkspaceType] {
		q.WorkspaceType = ""
	}
	if len(q.Settings) == 0 {
		q.Settings = json.RawMessage(`{}`)
	}
	var ws domain.Workspace
	err := h.pool.QueryRow(r.Context(),
		`UPDATE workspaces
		 SET display_name=$2,
		     workspace_type=CASE WHEN $3='' THEN workspace_type ELSE $3 END,
		     settings=$4, updated_at=NOW()
		 WHERE id=$1::uuid
		 RETURNING id::text,name,display_name,owner_id::text,settings,workspace_type,created_at,updated_at`,
		wsID, strings.TrimSpace(q.DisplayName), q.WorkspaceType, q.Settings).
		Scan(&ws.ID, &ws.Name, &ws.DisplayName, &ws.OwnerID, &ws.Settings, &ws.WorkspaceType, &ws.CreatedAt, &ws.UpdatedAt)
	if err != nil {
		errs.Write(w, errs.Internal("failed to update workspace"))
		return
	}
	writeAudit(r, h.pool, "workspace.updated", "workspace", wsID)
	errs.WriteJSON(w, http.StatusOK, ws)
}

func (h *WorkspaceHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	if err := h.ensureRoles(r.Context(), wsID); err != nil {
		errs.Write(w, errs.Internal("failed to prepare workspace roles"))
		return
	}
	rows, err := h.pool.Query(r.Context(),
		`SELECT u.id::text,u.email,u.full_name,u.avatar_url,u.is_active,ro.name,ur.created_at::text
		 FROM user_roles ur
		 JOIN users u ON u.id=ur.user_id
		 JOIN roles ro ON ro.id=ur.role_id
		 WHERE ur.workspace_id=$1::uuid
		 ORDER BY CASE ro.name WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'member' THEN 2 ELSE 3 END, u.email`, wsID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list members"))
		return
	}
	defer rows.Close()
	members := []workspaceMember{}
	for rows.Next() {
		var m workspaceMember
		if err := rows.Scan(&m.ID, &m.Email, &m.FullName, &m.AvatarURL, &m.IsActive, &m.Role, &m.JoinedAt); err != nil {
			errs.Write(w, errs.Internal("failed to read members"))
			return
		}
		members = append(members, m)
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
}

func (h *WorkspaceHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	if !h.canManage(r.Context(), middleware.UserIDFromCtx(r.Context()), wsID) {
		errs.Write(w, errs.Forbidden("workspace admin access required"))
		return
	}
	var q struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	q.Email = strings.ToLower(strings.TrimSpace(q.Email))
	q.Role = normalizeWorkspaceRole(q.Role)
	if q.Email == "" || q.Role == "" {
		errs.Write(w, errs.BadRequest("email and role are required"))
		return
	}
	if q.Role == "owner" {
		errs.Write(w, errs.BadRequest("owner role cannot be assigned here"))
		return
	}
	user, _, err := repository.NewUserRepository(h.pool).GetByEmail(r.Context(), q.Email)
	if err != nil {
		errs.Write(w, errs.NotFound("user must register before being added to a workspace"))
		return
	}
	if err := h.ensureRoles(r.Context(), wsID); err != nil {
		errs.Write(w, errs.Internal("failed to prepare workspace roles"))
		return
	}
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO user_roles(user_id,workspace_id,role_id)
		 SELECT $1::uuid,$2::uuid,id FROM roles WHERE workspace_id=$2::uuid AND name=$3
		 ON CONFLICT(user_id,workspace_id) DO UPDATE SET role_id=EXCLUDED.role_id`,
		user.ID, wsID, q.Role)
	if err != nil {
		errs.Write(w, errs.Internal("failed to add member"))
		return
	}
	writeAudit(r, h.pool, "workspace.member_added", "workspace", wsID)
	errs.WriteJSON(w, http.StatusCreated, map[string]any{"status": "added"})
}

func (h *WorkspaceHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	if !h.canManage(r.Context(), middleware.UserIDFromCtx(r.Context()), wsID) {
		errs.Write(w, errs.Forbidden("workspace admin access required"))
		return
	}
	memberID := chi.URLParam(r, "id")
	var q struct {
		Role string `json:"role"`
	}
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	q.Role = normalizeWorkspaceRole(q.Role)
	if q.Role == "" || q.Role == "owner" {
		errs.Write(w, errs.BadRequest("valid role is required"))
		return
	}
	if h.isOwner(r.Context(), memberID, wsID) {
		errs.Write(w, errs.BadRequest("workspace owner role cannot be changed"))
		return
	}
	if err := h.ensureRoles(r.Context(), wsID); err != nil {
		errs.Write(w, errs.Internal("failed to prepare workspace roles"))
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE user_roles SET role_id=(SELECT id FROM roles WHERE workspace_id=$2::uuid AND name=$3)
		 WHERE user_id=$1::uuid AND workspace_id=$2::uuid`, memberID, wsID, q.Role)
	if err != nil || tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("member not found"))
		return
	}
	writeAudit(r, h.pool, "workspace.member_role_changed", "workspace", wsID)
	errs.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (h *WorkspaceHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceIDFromCtx(r.Context())
	if !h.canManage(r.Context(), middleware.UserIDFromCtx(r.Context()), wsID) {
		errs.Write(w, errs.Forbidden("workspace admin access required"))
		return
	}
	memberID := chi.URLParam(r, "id")
	if h.isOwner(r.Context(), memberID, wsID) {
		errs.Write(w, errs.BadRequest("workspace owner cannot be removed"))
		return
	}
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM user_roles WHERE user_id=$1::uuid AND workspace_id=$2::uuid`, memberID, wsID)
	if err != nil || tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("member not found"))
		return
	}
	writeAudit(r, h.pool, "workspace.member_removed", "workspace", wsID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	workspaces, err := repository.NewUserRepository(h.pool).GetWorkspacesForUser(r.Context(), userID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list workspaces"))
		return
	}
	if workspaces == nil {
		workspaces = []domain.WorkspaceWithRole{}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": workspaces})
}

var validWorkspaceTypes = map[string]bool{
	"personal": true, "team": true, "organization": true, "project": true, "sandbox": true,
}

func (h *WorkspaceHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	var req struct {
		DisplayName   string `json:"display_name"`
		WorkspaceType string `json:"workspace_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		errs.Write(w, errs.BadRequest("display_name is required"))
		return
	}
	if !validWorkspaceTypes[req.WorkspaceType] {
		req.WorkspaceType = "personal"
	}

	repo := repository.NewUserRepository(h.pool)
	count, err := repo.CountOwnedWorkspaces(r.Context(), userID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to check workspace count"))
		return
	}
	if count >= 5 {
		errs.Write(w, errs.New(http.StatusUnprocessableEntity, "workspace limit reached (max 5 owned workspaces)"))
		return
	}

	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, req.DisplayName)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	wsName := slug + "-" + userID[:8]

	ws := &domain.Workspace{
		ID:            uuid.New().String(),
		Name:          wsName,
		DisplayName:   req.DisplayName,
		OwnerID:       userID,
		WorkspaceType: req.WorkspaceType,
	}
	if err := repo.CreateWorkspace(r.Context(), ws, userID); err != nil {
		errs.Write(w, errs.Internal("failed to create workspace"))
		return
	}
	writeAudit(r, h.pool, "workspace.created", "workspace", ws.ID)
	errs.WriteJSON(w, http.StatusCreated, ws)
}

func (h *WorkspaceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := chi.URLParam(r, "id")
	userID := middleware.UserIDFromCtx(r.Context())
	if !h.isOwner(r.Context(), userID, wsID) {
		errs.Write(w, errs.Forbidden("only the workspace owner can delete it"))
		return
	}
	repo := repository.NewUserRepository(h.pool)
	count, err := repo.CountOwnedWorkspaces(r.Context(), userID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to check workspace count"))
		return
	}
	if count <= 1 {
		errs.Write(w, errs.BadRequest("cannot delete your last workspace"))
		return
	}
	tag, err := h.pool.Exec(r.Context(), `DELETE FROM workspaces WHERE id=$1::uuid AND owner_id=$2::uuid`, wsID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("workspace not found"))
		return
	}
	writeAudit(r, h.pool, "workspace.deleted", "workspace", wsID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) ensureRoles(ctx context.Context, workspaceID string) error {
	var ownerID string
	if err := h.pool.QueryRow(ctx, `SELECT owner_id::text FROM workspaces WHERE id=$1::uuid`, workspaceID).Scan(&ownerID); err != nil {
		return err
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	roles := []string{"owner", "admin", "member", "viewer"}
	for _, role := range roles {
		if _, err := tx.Exec(ctx, `INSERT INTO roles(workspace_id,name,permissions) VALUES($1::uuid,$2,'[]'::jsonb) ON CONFLICT(workspace_id,name) DO NOTHING`, workspaceID, role); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_roles(user_id,workspace_id,role_id)
		 SELECT $1::uuid,$2::uuid,id FROM roles WHERE workspace_id=$2::uuid AND name='owner'
		 ON CONFLICT(user_id,workspace_id) DO NOTHING`, ownerID, workspaceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *WorkspaceHandler) canManage(ctx context.Context, userID, workspaceID string) bool {
	if middleware.IsAdminFromCtx(ctx) {
		return true
	}
	role, err := h.roleFor(ctx, userID, workspaceID)
	return err == nil && (role == "owner" || role == "admin")
}

func (h *WorkspaceHandler) roleFor(ctx context.Context, userID, workspaceID string) (string, error) {
	if err := h.ensureRoles(ctx, workspaceID); err != nil {
		return "", err
	}
	var role string
	err := h.pool.QueryRow(ctx,
		`SELECT ro.name
		 FROM user_roles ur
		 JOIN roles ro ON ro.id=ur.role_id
		 WHERE ur.user_id=$1::uuid AND ur.workspace_id=$2::uuid`, userID, workspaceID).Scan(&role)
	if err == pgx.ErrNoRows && h.isOwner(ctx, userID, workspaceID) {
		return "owner", nil
	}
	return role, err
}

func (h *WorkspaceHandler) isOwner(ctx context.Context, userID, workspaceID string) bool {
	var ok bool
	_ = h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id=$1::uuid AND owner_id=$2::uuid)`, workspaceID, userID).Scan(&ok)
	return ok
}

func normalizeWorkspaceRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin", "member", "viewer", "owner":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return ""
	}
}
