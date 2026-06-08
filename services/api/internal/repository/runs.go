package repository

import (
	"context"
	"github.com/agentNexus/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RunRepository struct{ pool *pgxpool.Pool }

func NewRunRepository(p *pgxpool.Pool) *RunRepository { return &RunRepository{p} }

const runCols = `id::text,workspace_id::text,agent_id::text,conversation_id::text,user_id::text,input,output,status,started_at,completed_at,total_input_tokens,total_output_tokens,cost_estimate,error_message`

func scanRun(r interface{ Scan(...any) error }) (domain.Run, error) {
	var x domain.Run
	e := r.Scan(&x.ID, &x.WorkspaceID, &x.AgentID, &x.ConversationID, &x.UserID, &x.Input, &x.Output, &x.Status, &x.StartedAt, &x.CompletedAt, &x.TotalInputTokens, &x.TotalOutputTokens, &x.CostEstimate, &x.ErrorMessage)
	return x, e
}
func (r *RunRepository) Create(c context.Context, x *domain.Run) error {
	_, e := r.pool.Exec(c, `INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,output,status,total_input_tokens,total_output_tokens,cost_estimate,error_message)VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,$7,$8,$9,$10,$11,$12)`, x.ID, x.WorkspaceID, x.AgentID, x.ConversationID, x.UserID, x.Input, x.Output, x.Status, x.TotalInputTokens, x.TotalOutputTokens, x.CostEstimate, x.ErrorMessage)
	return e
}
func (r *RunRepository) Get(c context.Context, id string) (*domain.Run, error) {
	x, e := scanRun(r.pool.QueryRow(c, `SELECT `+runCols+` FROM runs WHERE id=$1::uuid`, id))
	return &x, e
}
func (r *RunRepository) Update(c context.Context, x *domain.Run) error {
	_, e := r.pool.Exec(c, `UPDATE runs SET output=$2,status=$3,completed_at=$4,total_input_tokens=$5,total_output_tokens=$6,cost_estimate=$7,error_message=$8 WHERE id=$1::uuid`, x.ID, x.Output, x.Status, x.CompletedAt, x.TotalInputTokens, x.TotalOutputTokens, x.CostEstimate, x.ErrorMessage)
	return e
}
func (r *RunRepository) list(c context.Context, q string, arg string) ([]domain.Run, error) {
	rows, e := r.pool.Query(c, `SELECT `+runCols+` FROM runs WHERE `+q+`=$1::uuid ORDER BY started_at DESC`, arg)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	a := []domain.Run{}
	for rows.Next() {
		x, e := scanRun(rows)
		if e != nil {
			return nil, e
		}
		a = append(a, x)
	}
	return a, rows.Err()
}
func (r *RunRepository) List(c context.Context, id string) ([]domain.Run, error) {
	return r.list(c, "workspace_id", id)
}
func (r *RunRepository) ListByConversation(c context.Context, id string) ([]domain.Run, error) {
	return r.list(c, "conversation_id", id)
}
func (r *RunRepository) CreateStep(c context.Context, s *domain.RunStep) error {
	_, e := r.pool.Exec(c, `INSERT INTO run_steps(id,run_id,step_type,input,output,latency_ms,tokens_used,tool_name,error)VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9)`, s.ID, s.RunID, s.StepType, s.Input, s.Output, s.LatencyMs, s.TokensUsed, s.ToolName, s.Error)
	return e
}
func (r *RunRepository) ListSteps(c context.Context, id string) ([]domain.RunStep, error) {
	rows, e := r.pool.Query(c, `SELECT id::text,run_id::text,step_type,input,output,latency_ms,tokens_used,tool_name,error,created_at FROM run_steps WHERE run_id=$1::uuid ORDER BY created_at`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	a := []domain.RunStep{}
	for rows.Next() {
		var s domain.RunStep
		if e := rows.Scan(&s.ID, &s.RunID, &s.StepType, &s.Input, &s.Output, &s.LatencyMs, &s.TokensUsed, &s.ToolName, &s.Error, &s.CreatedAt); e != nil {
			return nil, e
		}
		a = append(a, s)
	}
	return a, rows.Err()
}
