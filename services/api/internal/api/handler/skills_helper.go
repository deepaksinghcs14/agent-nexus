package handler

import (
	"context"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type agentSkillSet struct {
	Always         []string
	AlwaysRequired map[string]bool
	OnDemand       map[string]domain.Skill
}

func loadAgentSkills(ctx context.Context, pool *pgxpool.Pool, agentID string) (agentSkillSet, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.id::text, s.name, s.description, s.content, COALESCE(s.required_tool_names, '{}'), ask.activation_mode
		FROM agent_skills ask
		JOIN skills s ON s.id=ask.skill_id
		WHERE ask.agent_id=$1::uuid AND ask.enabled=true AND s.enabled=true
		ORDER BY ask.order_index ASC, ask.created_at ASC`, agentID)
	if err != nil {
		return agentSkillSet{}, err
	}
	defer rows.Close()
	out := agentSkillSet{Always: []string{}, AlwaysRequired: map[string]bool{}, OnDemand: map[string]domain.Skill{}}
	for rows.Next() {
		var s domain.Skill
		var mode string
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Content, &s.RequiredToolNames, &mode); err != nil {
			return agentSkillSet{}, err
		}
		if mode == "on_demand" {
			out.OnDemand[s.Name] = s
		} else if s.Content != "" {
			out.Always = append(out.Always, s.Content)
			for _, name := range s.RequiredToolNames {
				out.AlwaysRequired[name] = true
			}
		}
	}
	return out, rows.Err()
}

// loadWorkspaceSkillCatalog returns every enabled skill visible in the workspace, keyed by name,
// regardless of whether it's attached to any particular agent via agent_skills. Used to populate
// discovery (native_list_agent_skills) and to let native_request_skill activate a workspace skill
// the agent wasn't pre-wired with — loadAgentSkills stays attachment-scoped since it also drives
// which skills are auto-injected ("always") into the system prompt.
func loadWorkspaceSkillCatalog(ctx context.Context, pool *pgxpool.Pool, workspaceID string) (map[string]domain.Skill, error) {
	rows, err := pool.Query(ctx, `
		SELECT id::text, name, COALESCE(description,''), COALESCE(content,''), COALESCE(required_tool_names,'{}')
		FROM skills
		WHERE (workspace_id=$1::uuid OR workspace_id IS NULL) AND enabled=true
		ORDER BY name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]domain.Skill{}
	for rows.Next() {
		var s domain.Skill
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Content, &s.RequiredToolNames); err != nil {
			return nil, err
		}
		out[s.Name] = s
	}
	return out, rows.Err()
}
