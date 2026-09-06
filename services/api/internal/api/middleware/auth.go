package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type contextKey string

const (
	ContextKeyUserID      contextKey = "user_id"
	ContextKeyWorkspaceID contextKey = "workspace_id"
	ContextKeyIsAdmin     contextKey = "is_admin"
	ContextKeyEmail       contextKey = "email"
	ContextKeyRole        contextKey = "role"
)

type Claims struct {
	UserID      string `json:"sub"`
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	IsAdmin     bool   `json:"is_admin"`
	Role        string `json:"role"`
	jwt.RegisteredClaims
}

// Authenticate validates either a JWT or an API token (anx_... prefix) from the Authorization header.
// Native EventSource cannot set headers, so GET streams may pass the same token
// as a query parameter.
func Authenticate(jwtSecret string, pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" && r.Method == http.MethodGet {
				if token := r.URL.Query().Get("token"); token != "" {
					authHeader = "Bearer " + token
				}
			}
			if authHeader == "" {
				errs.Write(w, errs.Unauthorized("missing authorization header"))
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				errs.Write(w, errs.Unauthorized("invalid authorization header format"))
				return
			}

			rawToken := parts[1]

			if strings.HasPrefix(rawToken, "anx_") {
				h := sha256.Sum256([]byte(rawToken))
				tokenHash := hex.EncodeToString(h[:])

				var userID, workspaceID, email string
				var isAdmin bool
				err := pool.QueryRow(r.Context(), `
					SELECT u.id::text, at.workspace_id::text, u.email, u.is_admin
					FROM api_tokens at
					JOIN users u ON u.id = at.user_id
					WHERE at.token_hash = $1
					  AND at.revoked_at IS NULL
					  AND (at.expires_at IS NULL OR at.expires_at > NOW())
					  AND u.is_active = true
				`, tokenHash).Scan(&userID, &workspaceID, &email, &isAdmin)
				if err != nil {
					errs.Write(w, errs.Unauthorized("invalid or expired API token"))
					return
				}

				go pool.Exec(context.Background(), //nolint:errcheck
					`UPDATE api_tokens SET last_used_at=NOW() WHERE token_hash=$1`, tokenHash)

				ctx := context.WithValue(r.Context(), ContextKeyUserID, userID)
				ctx = context.WithValue(ctx, ContextKeyWorkspaceID, workspaceID)
				ctx = context.WithValue(ctx, ContextKeyIsAdmin, isAdmin)
				ctx = context.WithValue(ctx, ContextKeyEmail, email)
				ctx = context.WithValue(ctx, ContextKeyRole, workspaceRole(r.Context(), pool, userID, workspaceID))
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			token, err := jwt.ParseWithClaims(rawToken, &Claims{},
				func(t *jwt.Token) (any, error) {
					if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, errs.Unauthorized("unexpected signing method")
					}
					return []byte(jwtSecret), nil
				})
			if err != nil || !token.Valid {
				errs.Write(w, errs.Unauthorized("invalid or expired token"))
				return
			}

			claims, ok := token.Claims.(*Claims)
			if !ok {
				errs.Write(w, errs.Unauthorized("invalid token claims"))
				return
			}

			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyWorkspaceID, claims.WorkspaceID)
			ctx = context.WithValue(ctx, ContextKeyIsAdmin, claims.IsAdmin)
			ctx = context.WithValue(ctx, ContextKeyEmail, claims.Email)
			ctx = context.WithValue(ctx, ContextKeyRole, claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// workspaceRole looks up userID's role in workspaceID (owner/admin/member/viewer),
// falling back to "owner" when no explicit row exists but the user owns the
// workspace — same fallback WorkspaceHandler.roleFor and AuthHandler.Me use.
// A lookup failure or missing row for anyone else returns "" (unknown), which
// BlockViewerWrites treats as unrestricted — never lock out a caller who was
// never explicitly assigned "viewer".
func workspaceRole(ctx context.Context, pool *pgxpool.Pool, userID, workspaceID string) string {
	var role string
	err := pool.QueryRow(ctx,
		`SELECT ro.name FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id
		 WHERE ur.user_id=$1::uuid AND ur.workspace_id=$2::uuid`, userID, workspaceID).Scan(&role)
	if err != nil {
		var isOwner bool
		_ = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id=$1::uuid AND owner_id=$2::uuid)`,
			workspaceID, userID).Scan(&isOwner)
		if isOwner {
			return "owner"
		}
	}
	return role
}

// RequireAdmin rejects requests where the authenticated user is not an admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAdmin, _ := r.Context().Value(ContextKeyIsAdmin).(bool)
		if !isAdmin {
			errs.Write(w, errs.Forbidden("admin access required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BlockViewerWrites rejects mutating requests (any method but GET/HEAD/OPTIONS)
// from a caller whose workspace role is explicitly "viewer". A caller with no
// row in user_roles (role == "") is left unrestricted — see workspaceRole.
func BlockViewerWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RoleFromCtx(r.Context()) == "viewer" &&
			r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			errs.Write(w, errs.Forbidden("viewers have read-only access to this workspace"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserIDFromCtx extracts the authenticated user ID from context.
func UserIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(ContextKeyUserID).(string)
	return id
}

// WorkspaceIDFromCtx extracts the workspace ID from context.
func WorkspaceIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(ContextKeyWorkspaceID).(string)
	return id
}

func IsAdminFromCtx(ctx context.Context) bool {
	isAdmin, _ := ctx.Value(ContextKeyIsAdmin).(bool)
	return isAdmin
}

func EmailFromCtx(ctx context.Context) string {
	email, _ := ctx.Value(ContextKeyEmail).(string)
	return email
}

// RoleFromCtx returns the caller's workspace role (owner/admin/member/viewer),
// or "" if unknown (never assigned a role explicitly — treated as unrestricted).
func RoleFromCtx(ctx context.Context) string {
	role, _ := ctx.Value(ContextKeyRole).(string)
	return role
}
