package handler

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func loadAgentSkills(ctx context.Context, pool *pgxpool.Pool, agentID string) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.content
		FROM agent_skills ask
		JOIN skills s ON s.id=ask.skill_id
		WHERE ask.agent_id=$1::uuid AND ask.enabled=true AND s.enabled=true
		ORDER BY ask.order_index ASC, ask.created_at ASC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		if c != "" {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}
