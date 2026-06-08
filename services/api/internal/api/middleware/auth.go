package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentNexus/agent-nexus/services/api/pkg/errs"
)

type contextKey string

const (
	ContextKeyUserID      contextKey = "user_id"
	ContextKeyWorkspaceID contextKey = "workspace_id"
	ContextKeyIsAdmin     contextKey = "is_admin"
	ContextKeyEmail       contextKey = "email"
)

type Claims struct {
	UserID      string `json:"sub"`
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	IsAdmin     bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

// Authenticate validates either a JWT or an API token (anx_... prefix) from the Authorization header.
func Authenticate(jwtSecret string, pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
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

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
