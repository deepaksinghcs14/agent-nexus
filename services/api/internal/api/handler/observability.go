package handler

import (
	"net/http"
	"strconv"

	"github.com/deepaksingh/agent-nexus/services/api/internal/api/middleware"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ObservabilityHandler struct {
	pool *pgxpool.Pool
}

func NewObservabilityHandler(pool *pgxpool.Pool) *ObservabilityHandler {
	return &ObservabilityHandler{pool: pool}
}

func (h *ObservabilityHandler) Latency(w http.ResponseWriter, r *http.Request) {
	ws := middleware.WorkspaceIDFromCtx(r.Context())

	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 && v <= 90 {
			days = v
		}
	}
	daysStr := strconv.Itoa(days)

	// ── By Agent ─────────────────────────────────────────────────────────────
	type agentRow struct {
		ID               string  `json:"id"`
		Name             string  `json:"name"`
		Provider         string  `json:"provider"`
		Model            string  `json:"model"`
		RunCount         int     `json:"run_count"`
		SuccessCount     int     `json:"success_count"`
		P50Secs          float64 `json:"p50_secs"`
		P95Secs          float64 `json:"p95_secs"`
		AvgInputTokens   float64 `json:"avg_input_tokens"`
		AvgOutputTokens  float64 `json:"avg_output_tokens"`
	}

	agentRows, err := h.pool.Query(r.Context(), `
		SELECT
		  a.id::text, a.name, a.provider, a.model,
		  COUNT(*),
		  COUNT(*) FILTER (WHERE r.status = 'success'),
		  COALESCE(ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (
		      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3), 0),
		  COALESCE(ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (
		      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3), 0),
		  COALESCE(ROUND(AVG(r.total_input_tokens)::numeric, 0), 0),
		  COALESCE(ROUND(AVG(r.total_output_tokens)::numeric, 0), 0)
		FROM runs r
		JOIN agents a ON a.id = r.agent_id
		WHERE r.workspace_id = $1::uuid
		  AND r.completed_at IS NOT NULL
		  AND r.started_at > NOW() - ($2 || ' days')::interval
		GROUP BY a.id, a.name, a.provider, a.model
		ORDER BY COUNT(*) DESC
		LIMIT 20
	`, ws, daysStr)
	if err != nil {
		errs.Write(w, errs.Internal("failed to query agent latency"))
		return
	}
	defer agentRows.Close()

	byAgent := []agentRow{}
	for agentRows.Next() {
		var row agentRow
		if err := agentRows.Scan(&row.ID, &row.Name, &row.Provider, &row.Model,
			&row.RunCount, &row.SuccessCount, &row.P50Secs, &row.P95Secs,
			&row.AvgInputTokens, &row.AvgOutputTokens); err != nil {
			continue
		}
		byAgent = append(byAgent, row)
	}

	// ── By Model ─────────────────────────────────────────────────────────────
	type modelRow struct {
		Provider     string  `json:"provider"`
		Model        string  `json:"model"`
		RunCount     int     `json:"run_count"`
		P50Secs      float64 `json:"p50_secs"`
		P95Secs      float64 `json:"p95_secs"`
		TokensPerSec float64 `json:"tokens_per_sec"`
	}

	modelRows, err := h.pool.Query(r.Context(), `
		SELECT
		  a.provider, a.model,
		  COUNT(*),
		  COALESCE(ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (
		      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3), 0),
		  COALESCE(ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (
		      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3), 0),
		  COALESCE(ROUND(AVG(
		      r.total_output_tokens::float /
		      NULLIF(EXTRACT(EPOCH FROM (r.completed_at - r.started_at)), 0)
		  )::numeric, 1), 0)
		FROM runs r
		JOIN agents a ON a.id = r.agent_id
		WHERE r.workspace_id = $1::uuid
		  AND r.completed_at IS NOT NULL
		  AND r.status = 'success'
		  AND r.started_at > NOW() - ($2 || ' days')::interval
		GROUP BY a.provider, a.model
		ORDER BY COUNT(*) DESC
	`, ws, daysStr)
	if err != nil {
		errs.Write(w, errs.Internal("failed to query model latency"))
		return
	}
	defer modelRows.Close()

	byModel := []modelRow{}
	for modelRows.Next() {
		var row modelRow
		if err := modelRows.Scan(&row.Provider, &row.Model, &row.RunCount,
			&row.P50Secs, &row.P95Secs, &row.TokensPerSec); err != nil {
			continue
		}
		byModel = append(byModel, row)
	}

	// ── Daily Trend ───────────────────────────────────────────────────────────
	type trendRow struct {
		Day      string  `json:"day"`
		RunCount int     `json:"run_count"`
		P50Secs  float64 `json:"p50_secs"`
		P95Secs  float64 `json:"p95_secs"`
	}

	trendRows, err := h.pool.Query(r.Context(), `
		SELECT
		  DATE_TRUNC('day', r.started_at AT TIME ZONE 'UTC')::date::text,
		  COUNT(*),
		  COALESCE(ROUND(PERCENTILE_CONT(0.5) WITHIN GROUP (
		      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3), 0),
		  COALESCE(ROUND(PERCENTILE_CONT(0.95) WITHIN GROUP (
		      ORDER BY EXTRACT(EPOCH FROM (r.completed_at - r.started_at)))::numeric, 3), 0)
		FROM runs r
		WHERE r.workspace_id = $1::uuid
		  AND r.completed_at IS NOT NULL
		  AND r.status = 'success'
		  AND r.started_at > NOW() - ($2 || ' days')::interval
		GROUP BY 1
		ORDER BY 1
	`, ws, daysStr)
	if err != nil {
		errs.Write(w, errs.Internal("failed to query latency trend"))
		return
	}
	defer trendRows.Close()

	trend := []trendRow{}
	for trendRows.Next() {
		var row trendRow
		if err := trendRows.Scan(&row.Day, &row.RunCount, &row.P50Secs, &row.P95Secs); err != nil {
			continue
		}
		trend = append(trend, row)
	}

	errs.WriteJSON(w, http.StatusOK, map[string]any{
		"by_agent": byAgent,
		"by_model": byModel,
		"trend":    trend,
		"days":     days,
	})
}
