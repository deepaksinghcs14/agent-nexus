package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	authpkg "github.com/deepaksingh/agent-nexus/services/api/internal/auth"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type AuthHandler struct {
	pool  *pgxpool.Pool
	cfg   *config.Config
	users *repository.UserRepository
}

func NewAuthHandler(pool *pgxpool.Pool, cfg *config.Config) *AuthHandler {
	return &AuthHandler{pool: pool, cfg: cfg, users: repository.NewUserRepository(pool)}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" || req.FullName == "" {
		errs.Write(w, errs.BadRequest("email, password, and full_name are required"))
		return
	}

	hash, err := authpkg.HashPassword(req.Password)
	if err != nil {
		errs.Write(w, errs.Internal("failed to hash password"))
		return
	}

	userID := uuid.New().String()
	wsID := uuid.New().String()
	slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(req.FullName), " ", "-"))
	// Append first 8 chars of userID to guarantee workspace name uniqueness
	wsSlug := slug + "-" + userID[:8]

	user := &domain.User{
		ID:       userID,
		Email:    req.Email,
		FullName: req.FullName,
		IsActive: true,
		IsAdmin:  false,
	}
	ws := &domain.Workspace{
		ID:          wsID,
		Name:        wsSlug,
		DisplayName: req.FullName + "'s Workspace",
		OwnerID:     userID,
	}

	if err := h.users.Register(r.Context(), user, hash, ws); err != nil {
		msg := err.Error()
		if (strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")) && strings.Contains(msg, "users_email") {
			errs.Write(w, errs.Conflict("email already registered"))
		} else if strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique") {
			errs.Write(w, errs.Internal("registration failed — please try again"))
		} else {
			errs.Write(w, errs.Internal("registration failed"))
		}
		return
	}

	h.issueTokens(w, r, user, wsID)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}

	user, hash, err := h.users.GetByEmail(r.Context(), strings.ToLower(req.Email))
	if err != nil {
		errs.Write(w, errs.Unauthorized("invalid credentials"))
		return
	}
	if err := authpkg.CheckPassword(hash, req.Password); err != nil {
		errs.Write(w, errs.Unauthorized("invalid credentials"))
		return
	}

	ws, _, err := h.users.GetWorkspaceForUser(r.Context(), user.ID)
	if err != nil {
		errs.Write(w, errs.Internal("workspace lookup failed"))
		return
	}

	h.issueTokens(w, r, user, ws.ID)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		errs.Write(w, errs.Unauthorized("missing refresh token"))
		return
	}

	tokenHash := sha256Hex(cookie.Value)
	user, err := h.users.GetUserByRefreshToken(r.Context(), tokenHash)
	if err != nil {
		errs.Write(w, errs.Unauthorized("invalid or expired refresh token"))
		return
	}

	ws, _, err := h.users.GetWorkspaceForUser(r.Context(), user.ID)
	if err != nil {
		errs.Write(w, errs.Internal("workspace lookup failed"))
		return
	}

	_ = h.users.DeleteRefreshToken(r.Context(), tokenHash)
	h.issueTokens(w, r, user, ws.ID)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("refresh_token"); err == nil {
		_ = h.users.DeleteRefreshToken(r.Context(), sha256Hex(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "has_session", Value: "", MaxAge: -1, Path: "/"})
	errs.WriteJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromCtx(r.Context())
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		errs.Write(w, errs.NotFound("user not found"))
		return
	}
	ws, err := h.users.GetWorkspaceByID(r.Context(), middleware.WorkspaceIDFromCtx(r.Context()))
	if err != nil {
		errs.Write(w, errs.NotFound("workspace not found"))
		return
	}
	role := ""
	_ = h.pool.QueryRow(r.Context(),
		`SELECT ro.name FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id WHERE ur.user_id=$1::uuid AND ur.workspace_id=$2::uuid`,
		userID, ws.ID).Scan(&role)
	if role == "" && ws.OwnerID == userID {
		role = "owner"
	}
	workspaces, _ := h.users.GetWorkspacesForUser(r.Context(), userID)
	if workspaces == nil {
		workspaces = []domain.WorkspaceWithRole{}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{
		"user":           user,
		"workspace":      ws,
		"workspace_role": role,
		"workspaces":     workspaces,
	})
}

func (h *AuthHandler) Switch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.WorkspaceID == "" {
		errs.Write(w, errs.BadRequest("workspace_id is required"))
		return
	}
	userID := middleware.UserIDFromCtx(r.Context())
	var ok bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(
		   SELECT 1 FROM user_roles WHERE user_id=$1::uuid AND workspace_id=$2::uuid
		   UNION ALL
		   SELECT 1 FROM workspaces WHERE id=$2::uuid AND owner_id=$1::uuid
		 )`, userID, req.WorkspaceID).Scan(&ok)
	if !ok {
		errs.Write(w, errs.Forbidden("not a member of this workspace"))
		return
	}
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to load user"))
		return
	}
	accessToken, err := authpkg.SignAccessToken(h.cfg.JWTSecret, user.ID, req.WorkspaceID, user.Email, user.IsAdmin)
	if err != nil {
		errs.Write(w, errs.Internal("failed to sign token"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"workspace_id": req.WorkspaceID,
	})
}

func (h *AuthHandler) issueTokens(w http.ResponseWriter, r *http.Request, user *domain.User, workspaceID string) {
	accessToken, err := authpkg.SignAccessToken(h.cfg.JWTSecret, user.ID, workspaceID, user.Email, user.IsAdmin)
	if err != nil {
		errs.Write(w, errs.Internal("failed to sign access token"))
		return
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		errs.Write(w, errs.Internal("failed to generate refresh token"))
		return
	}
	rawToken := hex.EncodeToString(raw)
	tokenHash := sha256Hex(rawToken)
	expiresAt := time.Now().Add(30 * 24 * time.Hour)

	if err := h.users.CreateRefreshToken(r.Context(), user.ID, tokenHash, expiresAt); err != nil {
		errs.Write(w, errs.Internal("failed to store refresh token"))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    rawToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "has_session",
		Value:    "1",
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})

	errs.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"user":         user,
		"workspace_id": workspaceID,
	})

	// Write audit log for login event (best-effort)
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	_, _ = h.pool.Exec(r.Context(),
		`INSERT INTO admin_audit_logs(id,workspace_id,actor_id,actor_email,action,resource_type,resource_id,ip_address)
		 VALUES($1::uuid,$2::uuid,$3::uuid,$4,'user.login','user',$3::uuid,$5)`,
		uuid.NewString(), workspaceID, user.ID, user.Email, ip)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
