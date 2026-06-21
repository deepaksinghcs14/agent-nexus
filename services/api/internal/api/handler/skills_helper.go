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
