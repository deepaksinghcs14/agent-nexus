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
		AutoRun     bool   `json:"auto_run"`
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
	suite.AutoRun = req.AutoRun
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

// GenerateCases handles POST /evals/suites/{id}/generate-cases
func (h *EvalHandler) GenerateCases(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	suiteID := chi.URLParam(r, "id")
	var req struct {
		Count    int    `json:"count"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
	if req.Count <= 0 {
		req.Count = 10
	}
	if req.Count > 20 {
		req.Count = 20
	}

	repo := repository.NewEvalRepository(h.pool)
	suite, err := repo.GetSuite(r.Context(), suiteID, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}

	// Fetch the agent
	agentRepo := repository.NewAgentRepository(h.pool)
	agent, err := agentRepo.Get(r.Context(), suite.AgentID, ws)
	if err != nil {
		errs.Write(w, errs.Internal("agent not found"))
		return
	}

	// Fetch tools
	var toolLines []string
	toolRows, _ := h.pool.Query(r.Context(),
		`SELECT t.name, t.description FROM agent_tools at JOIN tools t ON t.id=at.tool_id WHERE at.agent_id=$1::uuid ORDER BY t.name`,
		agent.ID)
	if toolRows != nil {
		defer toolRows.Close()
		for toolRows.Next() {
			var name, desc string
			toolRows.Scan(&name, &desc) //nolint:errcheck
			if desc != "" {
				toolLines = append(toolLines, name+" — "+desc)
			} else {
				toolLines = append(toolLines, name)
			}
		}
	}
	toolsText := "None"
	if len(toolLines) > 0 {
		toolsText = strings.Join(toolLines, "\n")
	}

	// Fetch skills
	var skillNames []string
	skillRows, _ := h.pool.Query(r.Context(),
		`SELECT s.name FROM agent_skills ask JOIN skills s ON s.id=ask.skill_id WHERE ask.agent_id=$1::uuid ORDER BY ask.order_index`,
		agent.ID)
	if skillRows != nil {
		defer skillRows.Close()
		for skillRows.Next() {
			var name string
			skillRows.Scan(&name) //nolint:errcheck
			skillNames = append(skillNames, name)
		}
	}
	skillsText := "None"
	if len(skillNames) > 0 {
		skillsText = strings.Join(skillNames, ", ")
	}

	// Fetch connectors
	var connectorNames []string
	connRows, _ := h.pool.Query(r.Context(),
		`SELECT c.name FROM agent_connectors ac JOIN connectors c ON c.id=ac.connector_id WHERE ac.agent_id=$1::uuid AND ac.enabled=true ORDER BY c.name`,
		agent.ID)
	if connRows != nil {
		defer connRows.Close()
		for connRows.Next() {
			var name string
			connRows.Scan(&name) //nolint:errcheck
			connectorNames = append(connectorNames, name)
		}
	}
	connectorsText := "None"
	if len(connectorNames) > 0 {
		connectorsText = strings.Join(connectorNames, ", ")
	}

	// Resolve LLM provider — use request override if supplied, fall back to agent's provider
	providerName := agent.Provider
	modelName := agent.Model
	if req.Provider != "" {
		providerName = req.Provider
	}
	if req.Model != "" {
		modelName = req.Model
	}
	llm, err := h.runsH.providerFor(r.Context(), ws, providerName)
	if err != nil || llm == nil {
		errs.Write(w, errs.Internal("provider not available: "+providerName))
		return
	}

	prompt := fmt.Sprintf(`You are a QA engineer testing an AI agent. Generate %d test cases that thoroughly cover its capabilities.

=== AGENT ===
Name: %s
Description: %s

System prompt:
%s

Tools:
%s

Skills: %s
Knowledge bases (connectors): %s

=== TASK ===
Generate %d diverse test cases. Cover: core use cases, edge cases, multi-step requests, and error handling.

Rules:
- "input": the user message to send (realistic, varied length)
- "expected_output": ONE short phrase or keyword the response must contain — or empty string "" if you are using grading_criteria instead. Do NOT write full responses here.
- "grading_criteria": what the LLM judge should check for (be specific, 1-2 sentences)

Return ONLY a valid JSON array with no markdown fences, no commentary, nothing else:
[{"input":"...","expected_output":"...","grading_criteria":"..."}]`,
		req.Count, agent.Name, agent.Description, agent.Instructions,
		toolsText, skillsText, connectorsText, req.Count)

	ch, err := llm.Complete(r.Context(), provider.CompletionRequest{
		Model:       modelName,
		Temperature: 0.4,
		MaxTokens:   6000,
		Messages:    []provider.Message{{Role: "user", Content: prompt}},
	})
	if err != nil {
		errs.Write(w, errs.Internal("LLM call failed: "+err.Error()))
		return
	}

	var reply strings.Builder
	var streamErr error
	for event := range ch {
		switch event.Type {
		case provider.EventDelta:
			reply.WriteString(event.Delta)
		case provider.EventError:
			streamErr = event.Error
		}
	}
	if streamErr != nil {
		errs.Write(w, errs.Internal("LLM error: "+streamErr.Error()))
		return
	}

	raw := strings.TrimSpace(reply.String())
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	if raw == "" {
		errs.Write(w, errs.Internal("LLM returned an empty response"))
		return
	}

	var generated []struct {
		Input           string `json:"input"`
		ExpectedOutput  string `json:"expected_output"`
		GradingCriteria string `json:"grading_criteria"`
	}
	if err := json.Unmarshal([]byte(raw), &generated); err != nil {
		errs.Write(w, errs.Internal("failed to parse LLM response as JSON: "+raw[:min(len(raw), 120)]))
		return
	}

	var cases []domain.EvalCase
	for _, g := range generated {
		if g.Input == "" {
			continue
		}
		c := domain.EvalCase{
			ID:              uuid.NewString(),
			SuiteID:         suiteID,
			Input:           g.Input,
			ExpectedOutput:  g.ExpectedOutput,
			GradingCriteria: g.GradingCriteria,
		}
		if err := repo.CreateCase(r.Context(), &c); err == nil {
			cases = append(cases, c)
		}
	}

	errs.WriteJSON(w, http.StatusCreated, map[string]any{"cases": cases, "count": len(cases)})
}

// ExportCases handles GET /evals/suites/{id}/cases/export
func (h *EvalHandler) ExportCases(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	suiteID := chi.URLParam(r, "id")
	repo := repository.NewEvalRepository(h.pool)
	suite, err := repo.GetSuite(r.Context(), suiteID, ws)
	if err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}
	cases, err := repo.ListCases(r.Context(), suiteID)
	if err != nil {
		errs.Write(w, errs.Internal("failed to list cases"))
		return
	}

	type exportCase struct {
		Input           string `json:"input"`
		ExpectedOutput  string `json:"expected_output"`
		GradingCriteria string `json:"grading_criteria"`
	}
	out := make([]exportCase, len(cases))
	for i, c := range cases {
		out[i] = exportCase{Input: c.Input, ExpectedOutput: c.ExpectedOutput, GradingCriteria: c.GradingCriteria}
	}

	filename := strings.ReplaceAll(strings.ToLower(suite.Name), " ", "-") + "-cases.json"
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out) //nolint:errcheck
}

// ImportCases handles POST /evals/suites/{id}/cases/import
func (h *EvalHandler) ImportCases(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())
	suiteID := chi.URLParam(r, "id")
	var req struct {
		Cases []struct {
			Input           string `json:"input"`
			ExpectedOutput  string `json:"expected_output"`
			GradingCriteria string `json:"grading_criteria"`
		} `json:"cases"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		errs.Write(w, errs.BadRequest("invalid request body"))
		return
	}
	if len(req.Cases) == 0 {
		errs.Write(w, errs.BadRequest("cases array is empty"))
		return
	}
	if len(req.Cases) > 200 {
		errs.Write(w, errs.BadRequest("too many cases — maximum 200 per import"))
		return
	}

	repo := repository.NewEvalRepository(h.pool)
	if _, err := repo.GetSuite(r.Context(), suiteID, ws); err != nil {
		errs.Write(w, errs.NotFound("eval suite not found"))
		return
	}

	imported := 0
	for _, item := range req.Cases {
		if item.Input == "" {
			continue
		}
		c := &domain.EvalCase{
			ID:              uuid.NewString(),
			SuiteID:         suiteID,
			Input:           item.Input,
			ExpectedOutput:  item.ExpectedOutput,
			GradingCriteria: item.GradingCriteria,
		}
		if err := repo.CreateCase(r.Context(), c); err == nil {
			imported++
		}
	}

	errs.WriteJSON(w, http.StatusCreated, map[string]any{"imported": imported})
}

// TriggerAutoRunsForAgent fires eval runs for all auto_run suites of a given agent.
// Called from AgentsHandler.Update — runs async, never blocks the HTTP response.
func (h *EvalHandler) TriggerAutoRunsForAgent(wsID, agentID, uid string) {
	ctx := context.Background()
	repo := repository.NewEvalRepository(h.pool)
	suites, err := repo.ListAutoRunSuitesForAgent(ctx, agentID)
	if err != nil || len(suites) == 0 {
		return
	}
	for _, suite := range suites {
		suite := suite
		cases, err := repo.ListCases(ctx, suite.ID)
		if err != nil || len(cases) == 0 {
			continue
		}
		run := &domain.EvalRun{
			ID:          uuid.NewString(),
			SuiteID:     suite.ID,
			WorkspaceID: wsID,
			Status:      "pending",
			TotalCases:  len(cases),
		}
		if err := repo.CreateRun(ctx, run); err != nil {
			continue
		}
		go h.executeEvalRun(ctx, &suite, cases, run, uid)
	}
}

