package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/provider"
	"github.com/deepaksingh/agent-nexus/services/api/internal/repository"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

type EvalHandler struct {
	pool    *pgxpool.Pool
	cfg     *config.Config
	invokeH *InvokeHandler
	runsH   *RunsHandler
}

func NewEvalHandler(pool *pgxpool.Pool, cfg *config.Config, invokeH *InvokeHandler, runsH *RunsHandler) *EvalHandler {
	return &EvalHandler{pool: pool, cfg: cfg, invokeH: invokeH, runsH: runsH}
}

// ListSuites handles GET /evals/suites
func (h *EvalHandler) ListSuites(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	repo := repository.NewEvalRepository(h.pool)
	suites, err := repo.ListSuites(r.Context(), ws)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list eval suites"))
		return
	}
	if suites == nil {
		suites = []domain.EvalSuite{}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": suites})
}

// CreateSuite handles POST /evals/suites
func (h *EvalHandler) CreateSuite(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	var req struct {
		AgentID     string `json:"agent_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		GradingMode string `json:"grading_mode"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" || req.AgentID == "" {
		errs.Write(w, errs.BadRequest("name and agent_id are required"))
		return
	}
	mode := req.GradingMode
	if mode == "" {
		mode = "llm_judge"
	}
	suite := &domain.EvalSuite{
		ID:          uuid.NewString(),
		WorkspaceID: ws,
		AgentID:     req.AgentID,
		Name:        req.Name,
		Description: req.Description,
		GradingMode: mode,
	}
	repo := repository.NewEvalRepository(h.pool)
	if err := repo.CreateSuite(r.Context(), suite); err != nil {
		errs.Write(w, errs.Internal("failed to create eval suite"))
		return
	}
	errs.WriteJSON(w, http.StatusCreated, suite)
}

// GetSuite handles GET /evals/suites/{id}
func (h *EvalHandler) GetSuite(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	repo := repository.NewEvalRepository(h.pool)
	suite, err := repo.GetSuite(r.Context(), id, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}
	cases, _ := repo.ListCases(r.Context(), id)
	if cases == nil {
		cases = []domain.EvalCase{}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"suite": suite, "cases": cases})
}

// UpdateSuite handles PUT /evals/suites/{id}
func (h *EvalHandler) UpdateSuite(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		GradingMode string `json:"grading_mode"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" {
		errs.Write(w, errs.BadRequest("name is required"))
		return
	}
	repo := repository.NewEvalRepository(h.pool)
	suite, err := repo.GetSuite(r.Context(), id, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}
	suite.Name = req.Name
	suite.Description = req.Description
	if req.GradingMode != "" {
		suite.GradingMode = req.GradingMode
	}
	if err := repo.UpdateSuite(r.Context(), suite); err != nil {
		errs.Write(w, errs.Internal("failed to update eval suite"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, suite)
}

// DeleteSuite handles DELETE /evals/suites/{id}
func (h *EvalHandler) DeleteSuite(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	id := chi.URLParam(r, "id")
	repo := repository.NewEvalRepository(h.pool)
	if err := repo.DeleteSuite(r.Context(), id, ws); err != nil {
		errs.Write(w, errs.Internal("failed to delete eval suite"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateCase handles POST /evals/suites/{id}/cases
func (h *EvalHandler) CreateCase(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	suiteID := chi.URLParam(r, "id")
	var req struct {
		Input           string `json:"input"`
		ExpectedOutput  string `json:"expected_output"`
		GradingCriteria string `json:"grading_criteria"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Input == "" {
		errs.Write(w, errs.BadRequest("input is required"))
		return
	}
	repo := repository.NewEvalRepository(h.pool)
	if _, err := repo.GetSuite(r.Context(), suiteID, ws); err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}
	c := &domain.EvalCase{
		ID:              uuid.NewString(),
		SuiteID:         suiteID,
		Input:           req.Input,
		ExpectedOutput:  req.ExpectedOutput,
		GradingCriteria: req.GradingCriteria,
	}
	if err := repo.CreateCase(r.Context(), c); err != nil {
		errs.Write(w, errs.Internal("failed to create eval case"))
		return
	}
	errs.WriteJSON(w, http.StatusCreated, c)
}

// UpdateCase handles PUT /evals/suites/{id}/cases/{caseId}
func (h *EvalHandler) UpdateCase(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	suiteID := chi.URLParam(r, "id")
	caseID := chi.URLParam(r, "caseId")
	var req struct {
		Input           string `json:"input"`
		ExpectedOutput  string `json:"expected_output"`
		GradingCriteria string `json:"grading_criteria"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Input == "" {
		errs.Write(w, errs.BadRequest("input is required"))
		return
	}
	repo := repository.NewEvalRepository(h.pool)
	if _, err := repo.GetSuite(r.Context(), suiteID, ws); err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}
	c := &domain.EvalCase{
		ID:              caseID,
		SuiteID:         suiteID,
		Input:           req.Input,
		ExpectedOutput:  req.ExpectedOutput,
		GradingCriteria: req.GradingCriteria,
	}
	if err := repo.UpdateCase(r.Context(), c); err != nil {
		errs.Write(w, errs.Internal("failed to update eval case"))
		return
	}
	errs.WriteJSON(w, http.StatusOK, c)
}

// DeleteCase handles DELETE /evals/suites/{id}/cases/{caseId}
func (h *EvalHandler) DeleteCase(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	suiteID := chi.URLParam(r, "id")
	caseID := chi.URLParam(r, "caseId")
	repo := repository.NewEvalRepository(h.pool)
	if _, err := repo.GetSuite(r.Context(), suiteID, ws); err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}
	if err := repo.DeleteCase(r.Context(), caseID, suiteID); err != nil {
		errs.Write(w, errs.Internal("failed to delete eval case"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TriggerRun handles POST /evals/suites/{id}/runs
func (h *EvalHandler) TriggerRun(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	uid := middleware.UserIDFromCtx(r.Context())
	suiteID := chi.URLParam(r, "id")
	repo := repository.NewEvalRepository(h.pool)
	suite, err := repo.GetSuite(r.Context(), suiteID, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}
	cases, err := repo.ListCases(r.Context(), suiteID)
	if err != nil || len(cases) == 0 {
		errs.Write(w, errs.BadRequest("suite has no cases"))
		return
	}
	run := &domain.EvalRun{
		ID:          uuid.NewString(),
		SuiteID:     suiteID,
		WorkspaceID: ws,
		Status:      "pending",
		TotalCases:  len(cases),
	}
	if err := repo.CreateRun(r.Context(), run); err != nil {
		errs.Write(w, errs.Internal("failed to create eval run"))
		return
	}
	errs.WriteJSON(w, http.StatusAccepted, run)
	go h.executeEvalRun(context.Background(), suite, cases, run, uid)
}

// ListRuns handles GET /evals/suites/{id}/runs
func (h *EvalHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	suiteID := chi.URLParam(r, "id")
	repo := repository.NewEvalRepository(h.pool)
	if _, err := repo.GetSuite(r.Context(), suiteID, ws); err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}
	runs, err := repo.ListRuns(r.Context(), suiteID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list eval runs"))
		return
	}
	if runs == nil {
		runs = []domain.EvalRun{}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"data": runs})
}

// GetRun handles GET /evals/runs/{runId}
func (h *EvalHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	runID := chi.URLParam(r, "runId")
	repo := repository.NewEvalRepository(h.pool)
	run, err := repo.GetRun(r.Context(), runID, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("eval run not found"))
		return
	}
	results, _ := repo.ListResults(r.Context(), runID)
	if results == nil {
		results = []domain.EvalResult{}
	}
	errs.WriteJSON(w, http.StatusOK, map[string]any{"run": run, "results": results})
}

func (h *EvalHandler) executeEvalRun(ctx context.Context, suite *domain.EvalSuite, cases []domain.EvalCase, run *domain.EvalRun, uid string) {
	repo := repository.NewEvalRepository(h.pool)
	agents := repository.NewAgentRepository(h.pool)

	now := time.Now()
	run.Status = "running"
	run.StartedAt = &now
	repo.UpdateRun(ctx, run) //nolint:errcheck

	agent, err := agents.Get(ctx, suite.AgentID, suite.WorkspaceID)
	if err != nil {
		t := time.Now()
		run.Status = "failed"
		run.CompletedAt = &t
		repo.UpdateRun(ctx, run) //nolint:errcheck
		return
	}

	var llm provider.Provider
	if suite.GradingMode == "llm_judge" {
		llm, _ = h.runsH.providerFor(ctx, suite.WorkspaceID, agent.Provider)
	}

	for i, c := range cases {
		start := time.Now()

		convID := uuid.NewString()
		title := fmt.Sprintf("Eval: %s / Case %d", suite.Name, i+1)
		h.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
			convID, suite.WorkspaceID, agent.ID, uid, title)
		h.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
			uuid.NewString(), convID, c.Input)

		output, agentRunID, runErr := h.invokeH.RunAgentSync(ctx, agent, suite.WorkspaceID, uid, convID, c.Input)
		latencyMs := int(time.Since(start).Milliseconds())

		result := &domain.EvalResult{
			ID:        uuid.NewString(),
			EvalRunID: run.ID,
			CaseID:    c.ID,
			LatencyMs: latencyMs,
		}
		if agentRunID != "" {
			result.RunID = &agentRunID
		}

		if runErr != nil {
			result.Error = runErr.Error()
			run.ErrorCount++
			run.Failed++
		} else {
			result.ActualOutput = output
			passed, score, reasoning := h.grade(ctx, suite.GradingMode, c, output, agent.Model, llm)
			result.Passed = &passed
			result.Score = score
			result.JudgeReasoning = reasoning
			if passed {
				run.Passed++
			} else {
				run.Failed++
			}
		}

		repo.CreateResult(ctx, result) //nolint:errcheck
	}

	total := run.Passed + run.Failed
	if total > 0 {
		run.Score = float64(run.Passed) / float64(total)
	}
	t := time.Now()
	run.Status = "completed"
	run.CompletedAt = &t
	repo.UpdateRun(ctx, run) //nolint:errcheck
}

func (h *EvalHandler) grade(ctx context.Context, mode string, c domain.EvalCase, actual, model string, llm provider.Provider) (passed bool, score float64, reasoning string) {
	switch mode {
	case "exact":
		passed = strings.TrimSpace(strings.ToLower(actual)) == strings.TrimSpace(strings.ToLower(c.ExpectedOutput))
		if passed {
			score = 1.0
			reasoning = "Exact match"
		} else {
			reasoning = "Output did not exactly match expected"
		}
	case "contains":
		passed = strings.Contains(strings.ToLower(actual), strings.ToLower(c.ExpectedOutput))
		if passed {
			score = 1.0
			reasoning = "Output contains expected text"
		} else {
			reasoning = "Output did not contain expected text"
		}
	default: // llm_judge
		if llm == nil {
			return false, 0, "no LLM provider available for judging"
		}
		passed, score, reasoning = h.llmJudge(ctx, c, actual, model, llm)
	}
	return
}

func (h *EvalHandler) llmJudge(ctx context.Context, c domain.EvalCase, actual, model string, llm provider.Provider) (passed bool, score float64, reasoning string) {
	var parts []string
	if c.GradingCriteria != "" {
		parts = append(parts, "Evaluation criteria:\n"+c.GradingCriteria)
	}
	if c.ExpectedOutput != "" {
		parts = append(parts, "Reference/expected output:\n"+c.ExpectedOutput)
	}
	criteria := strings.Join(parts, "\n\n")
	if criteria == "" {
		criteria = "Evaluate whether the response is helpful, accurate, and relevant to the input."
	}

	judgePrompt := fmt.Sprintf(`You are an AI evaluator. Grade an agent's response.

Input given to the agent:
%s

%s

Agent's actual response:
%s

Respond with JSON only (no other text):
{"passed": true/false, "score": 0.0-1.0, "reasoning": "one brief sentence"}`,
		c.Input, criteria, actual)

	ch, err := llm.Complete(ctx, provider.CompletionRequest{
		Model:       model,
		Temperature: 0.1,
		MaxTokens:   300,
		Messages:    []provider.Message{{Role: "user", Content: judgePrompt}},
	})
	if err != nil {
		return false, 0, "judge LLM call failed: " + err.Error()
	}

	var reply strings.Builder
	for event := range ch {
		if event.Type == provider.EventError {
			return false, 0, "judge LLM error: " + event.Error.Error()
		}
		if event.Type == provider.EventDelta {
			reply.WriteString(event.Delta)
		}
	}

	raw := strings.TrimSpace(reply.String())
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	var result struct {
		Passed    bool    `json:"passed"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return false, 0, "failed to parse judge response"
	}
	return result.Passed, result.Score, result.Reasoning
}
