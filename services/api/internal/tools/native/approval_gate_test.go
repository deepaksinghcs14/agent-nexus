package native

import (
	"testing"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
)

// Both tools are RiskLevel "high" but ran with no approval gate because
// RequiresApproval was omitted (zero-valued to false) and SeedDB rewrites the
// DB column from these definitions on every boot — so the definition is the
// only place the flag can be set. Default stays false to keep the unattended
// Jira→PR pipeline working; REQUIRE_SESSION_APPROVAL turns the gate on.
func TestSessionToolsApprovalFollowsConfig(t *testing.T) {
	for _, requireApproval := range []bool{false, true} {
		cfg := &config.Config{RequireSessionApproval: requireApproval}

		if got := NewLaunchRepoSessionTool(nil, cfg).Definition().RequiresApproval; got != requireApproval {
			t.Errorf("launch_repo_session RequiresApproval = %v, want %v", got, requireApproval)
		}
		if got := NewCreatePullRequestTool(nil, cfg).Definition().RequiresApproval; got != requireApproval {
			t.Errorf("create_pull_request RequiresApproval = %v, want %v", got, requireApproval)
		}
	}
}
