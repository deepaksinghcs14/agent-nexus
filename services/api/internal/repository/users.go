package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Register creates a user and their default workspace atomically.
func (r *UserRepository) Register(ctx context.Context, u *domain.User, passwordHash string, ws *domain.Workspace) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, full_name, is_active, is_admin, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		u.ID, u.Email, passwordHash, u.FullName, u.IsActive, u.IsAdmin)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	wsType := ws.WorkspaceType
	if wsType == "" {
		wsType = "personal"
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO workspaces (id, name, display_name, owner_id, workspace_type, settings, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, '{}', NOW(), NOW())`,
		ws.ID, ws.Name, ws.DisplayName, u.ID, wsType)
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}
	if err := ensureWorkspaceRolesTx(ctx, tx, ws.ID, u.ID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, string, error) {
	var u domain.User
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, email, password_hash, full_name, avatar_url, is_active, is_admin, created_at, updated_at
		 FROM users WHERE email = $1`, email).
		Scan(&u.ID, &u.Email, &hash, &u.FullName, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, "", fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, "", fmt.Errorf("query user: %w", err)
	}
	return &u, hash, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, email, full_name, avatar_url, is_active, is_admin, created_at, updated_at
		 FROM users WHERE id = $1::uuid`, id).
		Scan(&u.ID, &u.Email, &u.FullName, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetWorkspaceByOwner(ctx context.Context, ownerID string) (*domain.Workspace, error) {
	var ws domain.Workspace
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, name, display_name, owner_id::text, workspace_type, created_at, updated_at
		 FROM workspaces WHERE owner_id = $1::uuid LIMIT 1`, ownerID).
		Scan(&ws.ID, &ws.Name, &ws.DisplayName, &ws.OwnerID, &ws.WorkspaceType, &ws.CreatedAt, &ws.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("workspace not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query workspace: %w", err)
	}
	return &ws, nil
}

func (r *UserRepository) GetWorkspaceForUser(ctx context.Context, userID string) (*domain.Workspace, string, error) {
	var ws domain.Workspace
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT w.id::text, w.name, w.display_name, w.owner_id::text, w.settings, w.workspace_type, w.created_at, w.updated_at, ro.name
		 FROM user_roles ur
		 JOIN roles ro ON ro.id = ur.role_id
		 JOIN workspaces w ON w.id = ur.workspace_id
		 WHERE ur.user_id = $1::uuid
		 ORDER BY CASE ro.name WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'member' THEN 2 ELSE 3 END, w.created_at
		 LIMIT 1`, userID).
		Scan(&ws.ID, &ws.Name, &ws.DisplayName, &ws.OwnerID, &ws.Settings, &ws.WorkspaceType, &ws.CreatedAt, &ws.UpdatedAt, &role)
	if err == nil {
		return &ws, role, nil
	}
	if err != pgx.ErrNoRows {
		return nil, "", fmt.Errorf("query workspace membership: %w", err)
	}
	ownerWS, ownerErr := r.GetWorkspaceByOwner(ctx, userID)
	if ownerErr != nil {
		return nil, "", ownerErr
	}
	return ownerWS, "owner", nil
}

func (r *UserRepository) GetWorkspaceByID(ctx context.Context, workspaceID string) (*domain.Workspace, error) {
	var ws domain.Workspace
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, name, display_name, owner_id::text, settings, workspace_type, created_at, updated_at
		 FROM workspaces WHERE id = $1::uuid`, workspaceID).
		Scan(&ws.ID, &ws.Name, &ws.DisplayName, &ws.OwnerID, &ws.Settings, &ws.WorkspaceType, &ws.CreatedAt, &ws.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("workspace not found")
	}
	return &ws, err
}

func (r *UserRepository) GetWorkspacesForUser(ctx context.Context, userID string) ([]domain.WorkspaceWithRole, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT w.id::text, w.name, w.display_name, w.owner_id::text, w.settings, w.workspace_type, w.created_at, w.updated_at, ro.name
		 FROM user_roles ur
		 JOIN roles ro ON ro.id = ur.role_id
		 JOIN workspaces w ON w.id = ur.workspace_id
		 WHERE ur.user_id = $1::uuid
		 ORDER BY CASE ro.name WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'member' THEN 2 ELSE 3 END, w.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("query workspaces: %w", err)
	}
	defer rows.Close()
	var result []domain.WorkspaceWithRole
	for rows.Next() {
		var wwr domain.WorkspaceWithRole
		if err := rows.Scan(&wwr.ID, &wwr.Name, &wwr.DisplayName, &wwr.OwnerID, &wwr.Settings, &wwr.WorkspaceType, &wwr.CreatedAt, &wwr.UpdatedAt, &wwr.Role); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		result = append(result, wwr)
	}
	return result, rows.Err()
}

func (r *UserRepository) CountOwnedWorkspaces(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM workspaces WHERE owner_id = $1::uuid`, userID).Scan(&count)
	return count, err
}

func (r *UserRepository) CreateWorkspace(ctx context.Context, ws *domain.Workspace, ownerID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	wsType := ws.WorkspaceType
	if wsType == "" {
		wsType = "personal"
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO workspaces (id, name, display_name, owner_id, workspace_type, settings, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, '{}', NOW(), NOW())`,
		ws.ID, ws.Name, ws.DisplayName, ownerID, wsType)
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}
	if err := ensureWorkspaceRolesTx(ctx, tx, ws.ID, ownerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserRepository) CreateRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1::uuid, $2, $3)`,
		userID, tokenHash, expiresAt)
	return err
}

func (r *UserRepository) GetUserByRefreshToken(ctx context.Context, tokenHash string) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx,
		`SELECT u.id::text, u.email, u.full_name, u.avatar_url, u.is_active, u.is_admin, u.created_at, u.updated_at
		 FROM refresh_tokens rt
		 JOIN users u ON u.id = rt.user_id
		 WHERE rt.token_hash = $1 AND rt.expires_at > NOW()`, tokenHash).
		Scan(&u.ID, &u.Email, &u.FullName, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("token not found or expired")
	}
	if err != nil {
		return nil, fmt.Errorf("query refresh token: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, tokenHash)
	return err
}

func (r *UserRepository) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, email, full_name, avatar_url, is_active, is_admin, created_at, updated_at
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.AvatarURL, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UserRepository) Update(ctx context.Context, u *domain.User) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET full_name=$2, is_active=$3, is_admin=$4, updated_at=NOW() WHERE id=$1::uuid`,
		u.ID, u.FullName, u.IsActive, u.IsAdmin)
	return err
}

type txExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func ensureWorkspaceRolesTx(ctx context.Context, tx txExecutor, workspaceID, ownerID string) error {
	roles := []string{"owner", "admin", "member", "viewer"}
	for _, role := range roles {
		if _, err := tx.Exec(ctx, `INSERT INTO roles(workspace_id,name,permissions) VALUES($1::uuid,$2,'[]'::jsonb) ON CONFLICT(workspace_id,name) DO NOTHING`, workspaceID, role); err != nil {
			return fmt.Errorf("ensure role %s: %w", role, err)
		}
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO user_roles(user_id,workspace_id,role_id)
		 SELECT $1::uuid,$2::uuid,id FROM roles WHERE workspace_id=$2::uuid AND name='owner'
		 ON CONFLICT(user_id,workspace_id) DO NOTHING`, ownerID, workspaceID)
	if err != nil {
		return fmt.Errorf("ensure owner role: %w", err)
	}
	return nil
}
