package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// PipelineStatus handles GET /api/v1/pipeline/status — live infrastructure
// state for the Settings → Claude Code readiness panel. Credentials and
// repos come from their own endpoints; this covers what only the server can
// see: whether the runner is configured, reachable, and in which executor
// mode (stub sessions are simulations — surfacing that here prevents the
// "session succeeded but nothing happened" confusion).
func (h *WorkspaceHandler) PipelineStatus(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"runner_configured": h.cfg.RunnerURL != "",
		"runner_reachable":  false,
		"runner_executor":   "",
	}
	if h.cfg.RunnerURL != "" {
		client := &http.Client{Timeout: 3 * time.Second}
		res, err := client.Get(strings.TrimRight(h.cfg.RunnerURL, "/") + "/healthz")
		if err == nil {
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
	errs.WriteJSON(w, http.StatusOK, out)
}
