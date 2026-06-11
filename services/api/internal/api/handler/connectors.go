package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
	"github.com/deepaksingh/agent-nexus/services/api/internal/connector/providers/filesystem"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

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

	var ok bool
	if h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM connectors WHERE id=$1::uuid AND workspace_id=$2::uuid)`, id, ws).Scan(&ok) != nil || !ok {
		errs.Write(w, errs.NotFound("connector not found"))
		return
	}

	// Mark connector as syncing and create a running job record
	h.pool.Exec(r.Context(), `UPDATE connectors SET status='syncing', updated_at=NOW() WHERE id=$1::uuid`, id) //nolint:errcheck

	jobID := uuid.NewString()
	now := time.Now()
	j := domain.ConnectorSyncJob{
		ID:          jobID,
		ConnectorID: id,
		Status:      "running",
		StartedAt:   &now,
	}
	repo := repository.NewConnectorRepository(h.pool)
	if err := repo.CreateSyncJob(r.Context(), &j); err != nil {
		errs.Write(w, errs.Internal("failed to create sync job"))
		return
	}

	// Get best-effort embedder; nil means chunks will be stored without embeddings
	embedder, _ := buildAnyProvider(r.Context(), h.pool, h.cfg, ws)

	go func() {
		ctx := context.Background()
		pipeline := connector.NewPipeline(h.pool, h.cfg)
		err := pipeline.Sync(ctx, id, ws, filesystem.New(), embedder)

		completed := time.Now()
		j.CompletedAt = &completed
		if err != nil {
			j.Status = "failed"
			j.ErrorMessage = err.Error()
			h.pool.Exec(ctx, `UPDATE connectors SET status='error', updated_at=NOW() WHERE id=$1::uuid`, id) //nolint:errcheck
		} else {
			j.Status = "completed"
			// Count indexed documents
			h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM connector_documents WHERE connector_id=$1::uuid`, id).Scan(&j.DocumentsIndexed) //nolint:errcheck
			j.DocumentsFound = j.DocumentsIndexed
		}
		repo.UpdateSyncJob(ctx, &j) //nolint:errcheck
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(j) //nolint:errcheck
}

func (h *ConnectorsHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	rows, e := h.pool.Query(r.Context(), `SELECT d.id::text,d.connector_id::text,d.workspace_id::text,d.source,d.source_document_id,d.title,d.url,d.author,d.content_hash,d.last_modified_at,d.indexed_at,d.metadata FROM connector_documents d JOIN connectors c ON c.id=d.connector_id WHERE d.connector_id=$1::uuid AND c.workspace_id=$2::uuid ORDER BY d.indexed_at DESC NULLS LAST`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
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
	errs.WriteJSON(w, 200, map[string]any{"data": a})
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
	rows, e := h.pool.Query(r.Context(), `SELECT j.id::text,j.connector_id::text,j.status,j.started_at,j.completed_at,j.documents_found,j.documents_indexed,j.error_message,j.created_at FROM connector_sync_jobs j JOIN connectors c ON c.id=j.connector_id WHERE j.connector_id=$1::uuid AND c.workspace_id=$2::uuid ORDER BY j.created_at DESC`, chi.URLParam(r, "id"), middleware.WorkspaceIDFromCtx(r.Context()))
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
