package admin

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	panic("not implemented")
}

func (s *Service) GetUser(ctx context.Context, id string) (*domain.User, error) {
	panic("not implemented")
}

func (s *Service) UpdateUser(ctx context.Context, u *domain.User) error {
	panic("not implemented")
}

func (s *Service) ListWorkspaces(ctx context.Context) ([]domain.Workspace, error) {
	panic("not implemented")
}

func (s *Service) UpdateWorkspace(ctx context.Context, w *domain.Workspace) error {
	panic("not implemented")
}

func (s *Service) ListAuditLogs(ctx context.Context, workspaceID string, limit, offset int) ([]domain.AuditLog, error) {
	panic("not implemented")
}

func (s *Service) GetPolicies(ctx context.Context, workspaceID string) ([]domain.Policy, error) {
	panic("not implemented")
}

func (s *Service) SetPolicy(ctx context.Context, p *domain.Policy) error {
	panic("not implemented")
}
