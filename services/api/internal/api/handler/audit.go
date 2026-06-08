package handler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentNexus/agent-nexus/services/api/internal/api/middleware"
)

// writeAudit inserts an audit log row best-effort (errors are silently ignored).
func writeAudit(r *http.Request, pool *pgxpool.Pool, action, resourceType, resourceID string) {
	ctx := r.Context()
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
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
