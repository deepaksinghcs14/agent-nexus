package handler

import (
	"context"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
)

// clientIP returns the caller's address as resolved by the trusted-proxy
// middleware in the router. Reading X-Forwarded-For directly would take the
// whole client-controlled header, letting a caller forge audit entries (and,
// where this feeds a rate limiter, mint unlimited buckets).
func clientIP(r *http.Request) string {
	if ip := chimw.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

// writeAudit inserts an audit log row best-effort (errors are silently ignored).
func writeAudit(r *http.Request, pool *pgxpool.Pool, action, resourceType, resourceID string) {
	ctx := r.Context()
	ip := clientIP(r)
	_, _ = pool.Exec(ctx,
		`INSERT INTO admin_audit_logs(id,workspace_id,actor_id,actor_email,action,resource_type,resource_id,ip_address)
		 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8)`,
		uuid.NewString(),
		middleware.WorkspaceIDFromCtx(ctx),
		middleware.UserIDFromCtx(ctx),
		middleware.EmailFromCtx(ctx),
		action,
		resourceType,
		resourceID,
		ip,
	)
}

// writeSystemAudit is like writeAudit but for unauthenticated paths where workspace/actor
// context is not available from middleware (e.g. the public webhook ingress endpoint).
func writeSystemAudit(ctx context.Context, pool *pgxpool.Pool, workspaceID, actorID, actorEmail, action, resourceType, resourceID, ip string) {
	_, _ = pool.Exec(ctx,
		`INSERT INTO admin_audit_logs(id,workspace_id,actor_id,actor_email,action,resource_type,resource_id,ip_address)
		 VALUES($1::uuid,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8)`,
		uuid.NewString(), workspaceID, actorID, actorEmail, action, resourceType, resourceID, ip,
	)
}
