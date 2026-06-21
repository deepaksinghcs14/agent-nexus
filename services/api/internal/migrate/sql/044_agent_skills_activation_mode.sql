ALTER TABLE agent_skills
    ADD COLUMN IF NOT EXISTS activation_mode TEXT NOT NULL DEFAULT 'always'
        CHECK (activation_mode IN ('always', 'on_demand'));

CREATE INDEX IF NOT EXISTS agent_skills_activation_idx
    ON agent_skills(agent_id, activation_mode)
    WHERE enabled = true;
