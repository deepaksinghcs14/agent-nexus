package native

import (
	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// All returns every native tool the platform ships, ready to register. New
// tools are added here — the composition root just loops. In demo mode the
// http-request and write-file tools are omitted to prevent SSRF and disk
// abuse on shared instances.
func All(pool *pgxpool.Pool, cfg *config.Config) []tools.NativeTool {
	out := []tools.NativeTool{
		NewReadFileTool(cfg.StoragePath),
		NewSaveMemoryTool(pool),
		NewListMemoriesTool(),
		NewRequestMemoryTool(),
		NewListToolsTool(),
		NewRequestToolTool(),
		NewListAgentSkillsTool(),
		NewRequestSkillTool(),
		// Agent self-management
		NewListAgentsTool(pool),
		NewCallAgentTool(pool),
		NewCreateAgentTool(pool),
		NewDeleteAgentTool(pool),
		NewListSkillsTool(pool),
		NewCreateSkillTool(pool),
		NewDeleteSkillTool(pool),
		NewListHttpToolsTool(pool),
		NewCreateHttpToolTool(pool),
		NewDeleteToolTool(pool),
		// Expanded self-management
		NewUpdateAgentTool(pool),
		NewAttachSkillTool(pool),
		NewDetachSkillTool(pool),
		NewUpdateSkillTool(pool),
		NewListWorkspaceToolsTool(pool),
		NewAttachToolTool(pool),
		NewDetachToolTool(pool),
		NewAttachConnectorTool(pool),
		NewDetachConnectorTool(pool),
		NewCreateCodeToolTool(pool),
		NewListWorkflowsTool(pool),
		NewCreateWorkflowTool(pool),
		NewSaveWorkflowGraphTool(pool),
		NewRunWorkflowTool(pool),
		NewDeleteWorkflowTool(pool),
		NewPromoteResourceTool(pool),
		NewRetrieveContextTool(pool),
		NewSendMessageTool(),
		NewAskUserTool(),
		NewLaunchRepoSessionTool(pool, cfg),
		NewLaunchReviewSessionTool(pool, cfg),
		NewCreatePullRequestTool(pool, cfg),
		NewGetBranchDiffTool(pool, cfg),
		NewListReposTool(pool),
	}
	out = append(out, NewWhatsAppTools(pool, cfg)...)
	if !cfg.DemoMode {
		out = append(out, NewWriteFileTool(cfg.StoragePath), NewHTTPRequestTool(cfg.HTTPToolAllowHosts))
	}
	return out
}
