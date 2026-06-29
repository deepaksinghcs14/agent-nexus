package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
)

type EvalRepository struct {
	pool *pgxpool.Pool
}

func NewEvalRepository(pool *pgxpool.Pool) *EvalRepository {
	return &EvalRepository{pool: pool}
}

func (r *EvalRepository) ListSuites(ctx context.Context, ws string) ([]domain.EvalSuite, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			es.id::text, es.workspace_id::text, es.agent_id::text,
			COALESCE(a.name, ''), es.name, es.description, es.grading_mode, es.auto_run,
			COUNT(ec.id)::int AS case_count,
			MAX(er.created_at) AS last_run_at,
			(SELECT score FROM eval_runs WHERE suite_id=es.id ORDER BY created_at DESC LIMIT 1) AS last_score,
			es.created_at, es.updated_at
		FROM eval_suites es
		LEFT JOIN agents a ON a.id = es.agent_id
		LEFT JOIN eval_cases ec ON ec.suite_id = es.id
		LEFT JOIN eval_runs er ON er.suite_id = es.id
		WHERE es.workspace_id = $1::uuid
		GROUP BY es.id, a.name
		ORDER BY es.created_at DESC
	`, ws)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var suites []domain.EvalSuite
	for rows.Next() {
		var s domain.EvalSuite
		var lastRunAt *time.Time
		var lastScore *float64
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.AgentID, &s.AgentName, &s.Name, &s.Description, &s.GradingMode, &s.AutoRun,
			&s.CaseCount, &lastRunAt, &lastScore, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.LastRunAt = lastRunAt
		s.LastScore = lastScore
		suites = append(suites, s)
	}
	return suites, rows.Err()
}

func (r *EvalRepository) GetSuite(ctx context.Context, id, ws string) (*domain.EvalSuite, error) {
	var s domain.EvalSuite
	err := r.pool.QueryRow(ctx, `
		SELECT
			es.id::text, es.workspace_id::text, es.agent_id::text,
			COALESCE(a.name, ''), es.name, es.description, es.grading_mode, es.auto_run,
			es.created_at, es.updated_at
		FROM eval_suites es
		LEFT JOIN agents a ON a.id = es.agent_id
		WHERE es.id = $1::uuid AND es.workspace_id = $2::uuid
	`, id, ws).Scan(&s.ID, &s.WorkspaceID, &s.AgentID, &s.AgentName, &s.Name, &s.Description, &s.GradingMode, &s.AutoRun,
		&s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("eval suite not found")
	}
	return &s, err
}

func (r *EvalRepository) CreateSuite(ctx context.Context, s *domain.EvalSuite) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO eval_suites(id, workspace_id, agent_id, name, description, grading_mode, auto_run) VALUES($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7)`,
		s.ID, s.WorkspaceID, s.AgentID, s.Name, s.Description, s.GradingMode, s.AutoRun)
	return err
}

func (r *EvalRepository) UpdateSuite(ctx context.Context, s *domain.EvalSuite) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE eval_suites SET name=$3, description=$4, grading_mode=$5, auto_run=$6, updated_at=NOW() WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		s.ID, s.WorkspaceID, s.Name, s.Description, s.GradingMode, s.AutoRun)
	return err
}

func (r *EvalRepository) ListAutoRunSuitesForAgent(ctx context.Context, agentID string) ([]domain.EvalSuite, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, workspace_id::text, agent_id::text, name, grading_mode FROM eval_suites WHERE agent_id=$1::uuid AND auto_run=true`,
		agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var suites []domain.EvalSuite
	for rows.Next() {
		var s domain.EvalSuite
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.AgentID, &s.Name, &s.GradingMode); err != nil {
			return nil, err
		}
		suites = append(suites, s)
	}
	return suites, rows.Err()
}

func (r *EvalRepository) DeleteSuite(ctx context.Context, id, ws string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM eval_suites WHERE id=$1::uuid AND workspace_id=$2::uuid`, id, ws)
	return err
}

func (r *EvalRepository) ListCases(ctx context.Context, suiteID string) ([]domain.EvalCase, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, suite_id::text, input, expected_output, grading_criteria, created_at, updated_at FROM eval_cases WHERE suite_id=$1::uuid ORDER BY created_at ASC`,
		suiteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cases []domain.EvalCase
	for rows.Next() {
		var c domain.EvalCase
		if err := rows.Scan(&c.ID, &c.SuiteID, &c.Input, &c.ExpectedOutput, &c.GradingCriteria, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, rows.Err()
}

func (r *EvalRepository) CreateCase(ctx context.Context, c *domain.EvalCase) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO eval_cases(id, suite_id, input, expected_output, grading_criteria) VALUES($1::uuid, $2::uuid, $3, $4, $5)`,
		c.ID, c.SuiteID, c.Input, c.ExpectedOutput, c.GradingCriteria)
	return err
}

func (r *EvalRepository) UpdateCase(ctx context.Context, c *domain.EvalCase) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE eval_cases SET input=$3, expected_output=$4, grading_criteria=$5, updated_at=NOW() WHERE id=$1::uuid AND suite_id=$2::uuid`,
		c.ID, c.SuiteID, c.Input, c.ExpectedOutput, c.GradingCriteria)
	return err
}

func (r *EvalRepository) DeleteCase(ctx context.Context, id, suiteID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM eval_cases WHERE id=$1::uuid AND suite_id=$2::uuid`, id, suiteID)
	return err
}

func (r *EvalRepository) CreateRun(ctx context.Context, run *domain.EvalRun) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO eval_runs(id, suite_id, workspace_id, status, total_cases) VALUES($1::uuid, $2::uuid, $3::uuid, $4, $5)`,
		run.ID, run.SuiteID, run.WorkspaceID, run.Status, run.TotalCases)
	return err
}

func (r *EvalRepository) UpdateRun(ctx context.Context, run *domain.EvalRun) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE eval_runs SET status=$2, passed=$3, failed=$4, error_count=$5, score=$6, started_at=$7, completed_at=$8 WHERE id=$1::uuid`,
		run.ID, run.Status, run.Passed, run.Failed, run.ErrorCount, run.Score, run.StartedAt, run.CompletedAt)
	return err
}

func (r *EvalRepository) GetRun(ctx context.Context, id, ws string) (*domain.EvalRun, error) {
	var run domain.EvalRun
	err := r.pool.QueryRow(ctx,
		`SELECT id::text, suite_id::text, workspace_id::text, status, total_cases, passed, failed, error_count, score, started_at, completed_at, created_at FROM eval_runs WHERE id=$1::uuid AND workspace_id=$2::uuid`,
		id, ws).Scan(&run.ID, &run.SuiteID, &run.WorkspaceID, &run.Status, &run.TotalCases, &run.Passed, &run.Failed, &run.ErrorCount, &run.Score, &run.StartedAt, &run.CompletedAt, &run.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("eval run not found")
	}
	return &run, err
}

func (r *EvalRepository) ListRuns(ctx context.Context, suiteID string) ([]domain.EvalRun, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, suite_id::text, workspace_id::text, status, total_cases, passed, failed, error_count, score, started_at, completed_at, created_at FROM eval_runs WHERE suite_id=$1::uuid ORDER BY created_at DESC LIMIT 20`,
		suiteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []domain.EvalRun
	for rows.Next() {
		var run domain.EvalRun
		if err := rows.Scan(&run.ID, &run.SuiteID, &run.WorkspaceID, &run.Status, &run.TotalCases, &run.Passed, &run.Failed, &run.ErrorCount, &run.Score, &run.StartedAt, &run.CompletedAt, &run.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *EvalRepository) CreateResult(ctx context.Context, res *domain.EvalResult) error {
	var runIDParam interface{}
	if res.RunID != nil {
		runIDParam = *res.RunID
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO eval_results(id, eval_run_id, case_id, run_id, actual_output, passed, score, judge_reasoning, error, latency_ms)
		 VALUES($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, $9, $10)`,
		res.ID, res.EvalRunID, res.CaseID, runIDParam, res.ActualOutput, res.Passed, res.Score, res.JudgeReasoning, res.Error, res.LatencyMs)
	return err
}

func (r *EvalRepository) ListResults(ctx context.Context, runID string) ([]domain.EvalResult, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			er.id::text, er.eval_run_id::text, er.case_id::text, er.run_id::text,
			ec.input, ec.expected_output, ec.grading_criteria,
			er.actual_output, er.passed, er.score, er.judge_reasoning, er.error, er.latency_ms, er.created_at
		FROM eval_results er
		JOIN eval_cases ec ON ec.id = er.case_id
		WHERE er.eval_run_id = $1::uuid
		ORDER BY er.created_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []domain.EvalResult
	for rows.Next() {
		var res domain.EvalResult
		if err := rows.Scan(&res.ID, &res.EvalRunID, &res.CaseID, &res.RunID,
			&res.Input, &res.ExpectedOutput, &res.GradingCriteria,
			&res.ActualOutput, &res.Passed, &res.Score, &res.JudgeReasoning, &res.Error, &res.LatencyMs, &res.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, rows.Err()
}
