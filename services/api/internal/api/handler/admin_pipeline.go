package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// PipelineOverview handles GET /api/v1/admin/pipeline — instance-wide
// pipeline health for platform admins: runner reachability/mode, every run
// currently parked in a durable wait (with workspace and age, across all
// workspaces), and 24h run outcomes. This is the "is anything stuck?" view;
// per-workspace readiness lives in Settings → Claude Code.
func (h *AdminHandler) PipelineOverview(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"runner_configured": h.cfg.RunnerURL != "",
		"runner_reachable":  false,
		"runner_executor":   "",
	}
	if h.cfg.RunnerURL != "" {
		client := &http.Client{Timeout: 3 * time.Second}
		if res, err := client.Get(strings.TrimRight(h.cfg.RunnerURL, "/") + "/healthz"); err == nil {
			defer res.Body.Close()
			if res.StatusCode == http.StatusOK {
				out["runner_reachable"] = true
				var hz struct {
					Executor string `json:"executor"`
				}
				if b, err := io.ReadAll(io.LimitReader(res.Body, 4096)); err == nil {
					_ = json.Unmarshal(b, &hz)
					out["runner_executor"] = hz.Executor
				}
			}
		}
	}

	type waitingRun struct {
		RunID        string    `json:"run_id"`
		Workspace    string    `json:"workspace"`
		AgentName    string    `json:"agent_name"`
		Status       string    `json:"status"`
		WaitType     string    `json:"wait_type"`
		WaitingSince time.Time `json:"waiting_since"`
	}
	waiting := []waitingRun{}
	rows, err := h.pool.Query(r.Context(), `
		SELECT r.id::text, w.display_name, COALESCE(a.name,''), r.status,
		       COALESCE(ws.wait_type,'(none — in-process only)'), COALESCE(ws.created_at, r.started_at)
		FROM runs r
		JOIN workspaces w ON w.id = r.workspace_id
		LEFT JOIN agents a ON a.id = r.agent_id
		LEFT JOIN run_wait_states ws ON ws.run_id = r.id
		WHERE r.status IN ('approval_wait','session_wait')
		ORDER BY COALESCE(ws.created_at, r.started_at)`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var wr waitingRun
			if rows.Scan(&wr.RunID, &wr.Workspace, &wr.AgentName, &wr.Status, &wr.WaitType, &wr.WaitingSince) == nil {
				waiting = append(waiting, wr)
			}
		}
	}
	out["waiting_runs"] = waiting

	dayStats := map[string]int{}
	if rows, err := h.pool.Query(r.Context(),
		`SELECT status, COUNT(*) FROM runs WHERE started_at > NOW() - INTERVAL '24 hours' GROUP BY status`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var n int
			if rows.Scan(&status, &n) == nil {
				dayStats[status] = n
			}
		}
	}
	out["runs_24h"] = dayStats

	errs.WriteJSON(w, http.StatusOK, out)
}
