package repository

import (
	"context"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConnectorRepository struct{ pool *pgxpool.Pool }

func NewConnectorRepository(p *pgxpool.Pool) *ConnectorRepository { return &ConnectorRepository{p} }

const connectorCols = `id::text,workspace_id::text,name,provider,type,auth_type,status,config,created_by::text,created_at,updated_at`

func scanConnector(r interface{ Scan(...any) error }) (domain.Connector, error) {
	var x domain.Connector
	e := r.Scan(&x.ID, &x.WorkspaceID, &x.Name, &x.Provider, &x.Type, &x.AuthType, &x.Status, &x.Config, &x.CreatedBy, &x.CreatedAt, &x.UpdatedAt)
	return x, e
}
func (r *ConnectorRepository) List(c context.Context, w string) ([]domain.Connector, error) {
	rows, e := r.pool.Query(c, `SELECT `+connectorCols+` FROM connectors WHERE workspace_id=$1::uuid ORDER BY created_at DESC`, w)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	a := []domain.Connector{}
	for rows.Next() {
		x, e := scanConnector(rows)
		if e != nil {
			return nil, e
		}
		a = append(a, x)
	}
	return a, rows.Err()
}
func (r *ConnectorRepository) Create(c context.Context, x *domain.Connector) error {
	_, e := r.pool.Exec(c, `INSERT INTO connectors(id,workspace_id,name,provider,type,auth_type,status,config,created_by)VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::uuid)`, x.ID, x.WorkspaceID, x.Name, x.Provider, x.Type, x.AuthType, x.Status, x.Config, x.CreatedBy)
	return e
}
func (r *ConnectorRepository) Get(c context.Context, id string) (*domain.Connector, error) {
	x, e := scanConnector(r.pool.QueryRow(c, `SELECT `+connectorCols+` FROM connectors WHERE id=$1::uuid`, id))
	return &x, e
}
func (r *ConnectorRepository) Delete(c context.Context, id string) error {
	_, e := r.pool.Exec(c, `DELETE FROM connectors WHERE id=$1::uuid`, id)
	return e
}
func (r *ConnectorRepository) ListDocuments(c context.Context, id string) ([]domain.ConnectorDocument, error) {
	rows, e := r.pool.Query(c, `SELECT id::text,connector_id::text,workspace_id::text,source,source_document_id,title,url,author,content_hash,last_modified_at,indexed_at,metadata FROM connector_documents WHERE connector_id=$1::uuid`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	a := []domain.ConnectorDocument{}
	for rows.Next() {
		var x domain.ConnectorDocument
		if e := rows.Scan(&x.ID, &x.ConnectorID, &x.WorkspaceID, &x.Source, &x.SourceDocumentID, &x.Title, &x.URL, &x.Author, &x.ContentHash, &x.LastModifiedAt, &x.IndexedAt, &x.Metadata); e != nil {
			return nil, e
		}
		a = append(a, x)
	}
	return a, rows.Err()
}
func (r *ConnectorRepository) ListSyncJobs(c context.Context, id string) ([]domain.ConnectorSyncJob, error) {
	rows, e := r.pool.Query(c, `SELECT id::text,connector_id::text,status,started_at,completed_at,documents_found,documents_indexed,error_message,created_at FROM connector_sync_jobs WHERE connector_id=$1::uuid ORDER BY created_at DESC`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	a := []domain.ConnectorSyncJob{}
	for rows.Next() {
		var x domain.ConnectorSyncJob
		if e := rows.Scan(&x.ID, &x.ConnectorID, &x.Status, &x.StartedAt, &x.CompletedAt, &x.DocumentsFound, &x.DocumentsIndexed, &x.ErrorMessage, &x.CreatedAt); e != nil {
			return nil, e
		}
		a = append(a, x)
	}
	return a, rows.Err()
}
func (r *ConnectorRepository) UpsertDocument(c context.Context, x *domain.ConnectorDocument) error {
	_, e := r.pool.Exec(c, `INSERT INTO connector_documents(id,connector_id,workspace_id,source,source_document_id,title,url,author,content_hash,last_modified_at,indexed_at,metadata)VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12)ON CONFLICT(connector_id,source_document_id)DO UPDATE SET title=$6,url=$7,author=$8,content_hash=$9,last_modified_at=$10,indexed_at=$11,metadata=$12`, x.ID, x.ConnectorID, x.WorkspaceID, x.Source, x.SourceDocumentID, x.Title, x.URL, x.Author, x.ContentHash, x.LastModifiedAt, x.IndexedAt, x.Metadata)
	return e
}
func (r *ConnectorRepository) CreateSyncJob(c context.Context, x *domain.ConnectorSyncJob) error {
	_, e := r.pool.Exec(c, `INSERT INTO connector_sync_jobs(id,connector_id,status,started_at,completed_at,documents_found,documents_indexed,error_message)VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8)`, x.ID, x.ConnectorID, x.Status, x.StartedAt, x.CompletedAt, x.DocumentsFound, x.DocumentsIndexed, x.ErrorMessage)
	return e
}
func (r *ConnectorRepository) UpdateSyncJob(c context.Context, x *domain.ConnectorSyncJob) error {
	_, e := r.pool.Exec(c, `UPDATE connector_sync_jobs SET status=$2,started_at=$3,completed_at=$4,documents_found=$5,documents_indexed=$6,error_message=$7 WHERE id=$1::uuid`, x.ID, x.Status, x.StartedAt, x.CompletedAt, x.DocumentsFound, x.DocumentsIndexed, x.ErrorMessage)
	return e
}

func (r *ConnectorRepository) SetAgentConnectors(ctx context.Context, agentID string, connectorIDs []string, maxChunks int, minScore float64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM agent_connectors WHERE agent_id=$1::uuid`, agentID); err != nil {
		return err
	}
	for _, cid := range connectorIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO agent_connectors(agent_id, connector_id, max_chunks, min_score, enabled)
			 VALUES($1::uuid, $2::uuid, $3, $4, true)
			 ON CONFLICT(agent_id, connector_id) DO UPDATE SET max_chunks=$3, min_score=$4, enabled=true`,
			agentID, cid, maxChunks, minScore); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *ConnectorRepository) ListAgentConnectors(ctx context.Context, agentID string) ([]domain.Connector, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.id::text, c.workspace_id::text, c.name, c.provider, c.type, c.auth_type, c.status, c.config, c.created_by::text, c.created_at, c.updated_at
		 FROM agent_connectors ac
		 JOIN connectors c ON c.id = ac.connector_id
		 WHERE ac.agent_id=$1::uuid AND ac.enabled=true
		 ORDER BY c.name`,
		agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Connector
	for rows.Next() {
		x, err := scanConnector(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
