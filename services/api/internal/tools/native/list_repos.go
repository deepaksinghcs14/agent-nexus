package native

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
)

// listRepoMapPreview caps how much of a repo's cached map is echoed back per
// repo in the list — the full map (up to 4000 chars) times several repos
// would bloat a single tool call; a preview is enough to pick a target.
const listRepoMapPreview = 800

// ListReposTool exposes repo_catalog for repo-selection decisions — cheaper
// and more direct than semantic search over indexed file chunks
// (native_retrieve_context) when a repo already has a cached architecture map
// describing what it's responsible for.
type ListReposTool struct {
	tools.RequiresRunContext
	pool *pgxpool.Pool
}

func NewListReposTool(pool *pgxpool.Pool) *ListReposTool { return &ListReposTool{pool: pool} }

func (t *ListReposTool) Definition() domain.Tool {
	return domain.Tool{
		Name: "native_list_repos",
		Description: "List repositories onboarded for coding sessions in this workspace (only these may be targeted by " +
			"native_launch_repo_session), each with its cached architecture map — a short summary of directory layout, " +
			"key modules, and conventions — when one has been generated. Use this first when deciding which repo a " +
			"ticket belongs to; fall back to native_retrieve_context for a deeper, file-level check when the maps " +
			"alone don't make it clear.",
		Type:             "native",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "low",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *ListReposTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, _ map[string]any) (any, error) {
	rows, err := t.pool.Query(ctx,
		`SELECT repo, repo_map FROM repo_catalog WHERE workspace_id=$1::uuid AND sessions_enabled ORDER BY repo`,
		execCtx.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type entry struct {
		Repo string `json:"repo"`
		Map  string `json:"map,omitempty"`
	}
	var out []entry
	for rows.Next() {
		var e entry
		if rows.Scan(&e.Repo, &e.Map) != nil {
			continue
		}
		if len(e.Map) > listRepoMapPreview {
			e.Map = strings.TrimSpace(e.Map[:listRepoMapPreview]) + "…"
		}
		out = append(out, e)
	}
	if out == nil {
		out = []entry{}
	}
	return map[string]any{"repos": out, "count": len(out)}, nil
}
