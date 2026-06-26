package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
	"github.com/deepaksingh/agent-nexus/services/api/internal/connector/providers/confluence"
	"github.com/deepaksingh/agent-nexus/services/api/internal/connector/providers/filesystem"
	githubprovider "github.com/deepaksingh/agent-nexus/services/api/internal/connector/providers/github"
	"github.com/deepaksingh/agent-nexus/services/api/internal/connector/syncstate"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

func getConnectorProvider(p string) connector.Provider {
	switch p {
	case "filesystem":
		return filesystem.New()
	case "github":
		return githubprovider.New()
	case "confluence":
		return confluence.New()
	default:
		return nil
	}
}

type ConnectorsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewConnectorsHandler(p *pgxpool.Pool, c *config.Config) *ConnectorsHandler {
	return &ConnectorsHandler{p, c}
}

const connectorSelect = `SELECT id::text,workspace_id::text,name,provider,type,auth_type,status,config,created_by::text,created_at,updated_at FROM connectors`

func scanConnector(row interface{ Scan(...any) error }) (domain.Connector, error) {
	var c domain.Connector
	e := row.Scan(&c.ID, &c.WorkspaceID, &c.Name, &c.Provider, &c.Type, &c.AuthType, &c.Status, &c.Config, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	return c, e
}

func (h *ConnectorsHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), connectorSelect+` WHERE workspace_id=$1::uuid ORDER BY created_at DESC`, middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to list connectors"))
		return
	}
	defer rows.Close()
	a := []domain.Connector{}
	for rows.Next() {
		c, e := scanConnector(rows)
		if e != nil {
			errs.Write(w, errs.Internal("failed to read connectors"))
			return
		}
		a = append(a, c)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}

func (h *ConnectorsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.cfg.DemoMode {
		errs.Write(w, errs.Forbidden("connector creation is not available in demo mode"))
		return
	}
	var c domain.Connector
	if json.NewDecoder(r.Body).Decode(&c) != nil || c.Name == "" || c.Provider == "" {
		errs.Write(w, errs.BadRequest("name and provider are required"))
		return
	}
	c.ID = uuid.NewString()
	c.WorkspaceID = middleware.WorkspaceIDFromCtx(r.Context())
	c.CreatedBy = middleware.UserIDFromCtx(r.Context())
	if c.Type == "" {
		c.Type = "native"
	}
	if c.AuthType == "" {
		c.AuthType = "none"
	}
	c.Status = "disconnected"
	if len(c.Config) == 0 {
		c.Config = json.RawMessage(`{}`)
	}
	e := h.pool.QueryRow(r.Context(), `INSERT INTO connectors(id,workspace_id,name,provider,type,auth_type,status,config,created_by)VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::uuid)RETURNING created_at,updated_at`, c.ID, c.WorkspaceID, c.Name, c.Provider, c.Type, c.AuthType, c.Status, c.Config, c.CreatedBy).Scan(&c.CreatedAt, &c.UpdatedAt)
	if e != nil {
		errs.Write(w, errs.Internal("failed to create connector"))
		return
	}
	writeAudit(r, h.pool, "connector.created", "connector", c.ID)
	errs.WriteJSON(w, 201, c)
}

func (h *ConnectorsHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, e := scanConnector(h.pool.QueryRow(r.Context(), connectorSelect+` WHERE id=$1::uuid AND workspace_id=$2::uuid`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context())))
	if e != nil {
		errs.Write(w, errs.NotFound("connector not found"))
		return
	}
	errs.WriteJSON(w, 200, c)
}

func (h *ConnectorsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	connID := chi.URLParam(r, "id")
	t, e := h.pool.Exec(r.Context(), `DELETE FROM connectors WHERE id=$1::uuid AND workspace_id=$2::uuid`, connID, middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to delete connector"))
		return
	}
	if t.RowsAffected() == 0 {
		errs.Write(w, errs.NotFound("connector not found"))
		return
	}
	writeAudit(r, h.pool, "connector.deleted", "connector", connID)
	w.WriteHeader(204)
}

func (h *ConnectorsHandler) Sync(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ws := middleware.WorkspaceIDFromCtx(r.Context())

	conn, e := scanConnector(h.pool.QueryRow(r.Context(), connectorSelect+` WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, ws))
	if e != nil {
		errs.Write(w, errs.NotFound("connector not found"))
		return
	}

	prov := getConnectorProvider(conn.Provider)
	if prov == nil {
		errs.Write(w, errs.BadRequest("unsupported connector provider: "+conn.Provider))
		return
	}

	h.pool.Exec(r.Context(), `UPDATE connectors SET status='syncing', updated_at=NOW() WHERE id=$1::uuid`, id) //nolint:errcheck

	jobID := uuid.NewString()
	now := time.Now()
	j := domain.ConnectorSyncJob{
		ID: jobID, ConnectorID: id,
		Status: "running", StartedAt: &now,
	}
	repo := repository.NewConnectorRepository(h.pool)
	if err := repo.CreateSyncJob(r.Context(), &j); err != nil {
		errs.Write(w, errs.Internal("failed to create sync job"))
		return
	}

	embedder := buildEmbedder(h.cfg)
	rep := syncstate.New(h.pool, jobID, id)

	go func() {
		ctx := context.Background()
		slog.Info("connector sync started", "connector_id", id, "provider", conn.Provider, "job_id", jobID)
		pipeline := connector.NewPipeline(h.pool, h.cfg)
		err := pipeline.Sync(ctx, id, ws, prov, embedder, rep)
		if err != nil {
			slog.Error("connector sync failed", "connector_id", id, "job_id", jobID, "error", err)
			rep.Fail(err)
		} else {
			rep.Complete()
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(j) //nolint:errcheck
}

// RecoverInterruptedSyncs is called at server startup to handle syncs that were in-flight
// when the pod was restarted. It marks orphaned running jobs as 'interrupted' (preserving
// their checkpoint) and auto-resumes connectors that have a valid checkpoint to resume from.
func (h *ConnectorsHandler) RecoverInterruptedSyncs(ctx context.Context) {
	t, err := h.pool.Exec(ctx, `
		UPDATE connector_sync_jobs
		SET status='interrupted', completed_at=NOW(), error_message='Interrupted by pod restart'
		WHERE status='running'
	`)
	if err != nil {
		slog.Warn("connector recovery: failed to mark interrupted jobs", "error", err)
		return
	}
	if t.RowsAffected() == 0 {
		return
	}
	slog.Info("connector recovery: marked interrupted sync jobs", "count", t.RowsAffected())

	// Reset connector statuses for anything still marked 'syncing'.
	h.pool.Exec(ctx, `UPDATE connectors SET status='error', updated_at=NOW() WHERE status='syncing'`) //nolint:errcheck

	// Find connectors whose last interrupted job has a non-empty checkpoint — those can resume.
	rows, err := h.pool.Query(ctx, `
		SELECT DISTINCT ON (j.connector_id)
			c.id::text, c.workspace_id::text, c.provider
		FROM connector_sync_jobs j
		JOIN connectors c ON c.id = j.connector_id
		WHERE j.status = 'interrupted'
		  AND j.checkpoint != '{}'::jsonb
		  AND j.completed_at >= NOW() - INTERVAL '1 hour'
		ORDER BY j.connector_id, j.created_at DESC
	`)
	if err != nil {
		slog.Warn("connector recovery: failed to query resumable connectors", "error", err)
		return
	}
	defer rows.Close()

	type resumable struct{ id, wsID, providerName string }
	var candidates []resumable
	for rows.Next() {
		var r resumable
		if rows.Scan(&r.id, &r.wsID, &r.providerName) == nil {
			candidates = append(candidates, r)
		}
	}
	rows.Close()

	for _, r := range candidates {
		prov := getConnectorProvider(r.providerName)
		if prov == nil {
			continue
		}
		slog.Info("connector recovery: resuming sync", "connector_id", r.id, "provider", r.providerName)
		h.launchSync(ctx, r.id, r.wsID, prov)
	}
}

// launchSync creates a new sync job and starts the sync goroutine.
func (h *ConnectorsHandler) launchSync(ctx context.Context, connID, wsID string, prov connector.Provider) {
	jobID := uuid.NewString()
	now := time.Now()
	j := domain.ConnectorSyncJob{
		ID: jobID, ConnectorID: connID,
		Status: "running", StartedAt: &now,
	}
	repo := repository.NewConnectorRepository(h.pool)
	if err := repo.CreateSyncJob(ctx, &j); err != nil {
		slog.Warn("connector recovery: failed to create job", "connector_id", connID, "error", err)
		return
	}
	h.pool.Exec(ctx, `UPDATE connectors SET status='syncing', updated_at=NOW() WHERE id=$1::uuid`, connID) //nolint:errcheck

	embedder := buildEmbedder(h.cfg)
	rep := syncstate.New(h.pool, jobID, connID)

	go func() {
		slog.Info("connector recovery: sync goroutine started", "connector_id", connID, "job_id", jobID)
		pipeline := connector.NewPipeline(h.pool, h.cfg)
		err := pipeline.Sync(context.Background(), connID, wsID, prov, embedder, rep)
		if err != nil {
			slog.Error("connector recovery: sync failed", "connector_id", connID, "error", err)
			rep.Fail(err)
		} else {
			rep.Complete()
		}
	}()
}

// ListDocuments returns paginated documents for a connector (20 per page).
func (h *ConnectorsHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	const pageSize = 20
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	offset := (page - 1) * pageSize

	connID := chi.URLParam(r, "id")
	ws := middleware.WorkspaceIDFromCtx(r.Context())

	var total int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM connector_documents d JOIN connectors c ON c.id=d.connector_id WHERE d.connector_id=$1::uuid AND c.workspace_id=$2::uuid`,
		connID, ws,
	).Scan(&total)

	rows, e := h.pool.Query(r.Context(),
		`SELECT d.id::text,d.connector_id::text,d.workspace_id::text,d.source,d.source_document_id,d.title,d.url,d.author,d.content_hash,d.last_modified_at,d.indexed_at,d.metadata
		 FROM connector_documents d JOIN connectors c ON c.id=d.connector_id
		 WHERE d.connector_id=$1::uuid AND c.workspace_id=$2::uuid
		 ORDER BY d.indexed_at DESC NULLS LAST
		 LIMIT $3 OFFSET $4`,
		connID, ws, pageSize, offset)
	if e != nil {
		errs.Write(w, errs.Internal("failed to list documents"))
		return
	}
	defer rows.Close()
	a := []domain.ConnectorDocument{}
	for rows.Next() {
		var d domain.ConnectorDocument
		if rows.Scan(&d.ID, &d.ConnectorID, &d.WorkspaceID, &d.Source, &d.SourceDocumentID, &d.Title, &d.URL, &d.Author, &d.ContentHash, &d.LastModifiedAt, &d.IndexedAt, &d.Metadata) != nil {
			errs.Write(w, errs.Internal("failed to read documents"))
			return
		}
		a = append(a, d)
	}
	errs.WriteJSON(w, 200, map[string]any{
		"data":       a,
		"total":      total,
		"page":       page,
		"page_size":  pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	})
}

func (h *ConnectorsHandler) BrowseFilesystem(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")
	if raw == "" {
		raw = "/"
	}
	clean := filepath.Clean(raw)
	entries, err := os.ReadDir(clean)
	if err != nil {
		errs.Write(w, errs.BadRequest("cannot read directory: "+err.Error()))
		return
	}
	type entry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	result := []entry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		result = append(result, entry{Name: e.Name(), Path: filepath.Join(clean, e.Name()), IsDir: e.IsDir()})
	}
	parent := ""
	if clean != "/" {
		parent = filepath.Dir(clean)
	}
	errs.WriteJSON(w, 200, map[string]any{"path": clean, "parent": parent, "entries": result})
}

func (h *ConnectorsHandler) ListSyncJobs(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(),
		`SELECT j.id::text,j.connector_id::text,j.status,j.started_at,j.completed_at,j.documents_found,j.documents_indexed,j.error_message,j.created_at
		 FROM connector_sync_jobs j JOIN connectors c ON c.id=j.connector_id
		 WHERE j.connector_id=$1::uuid AND c.workspace_id=$2::uuid
		 ORDER BY j.created_at DESC`,
		chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
	if e != nil {
		errs.Write(w, errs.Internal("failed to list sync jobs"))
		return
	}
	defer rows.Close()
	a := []domain.ConnectorSyncJob{}
	for rows.Next() {
		var j domain.ConnectorSyncJob
		if rows.Scan(&j.ID, &j.ConnectorID, &j.Status, &j.StartedAt, &j.CompletedAt, &j.DocumentsFound, &j.DocumentsIndexed, &j.ErrorMessage, &j.CreatedAt) != nil {
			errs.Write(w, errs.Internal("failed to read sync jobs"))
			return
		}
		a = append(a, j)
	}
	errs.WriteJSON(w, 200, map[string]any{"data": a})
}
