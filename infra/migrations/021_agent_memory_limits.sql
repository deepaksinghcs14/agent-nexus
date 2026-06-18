-- Add max_memories and min_relevance_score to agents for configurable memory retrieval.
ALTER TABLE agents
  ADD COLUMN IF NOT EXISTS max_memories       integer NOT NULL DEFAULT 5,
  ADD COLUMN IF NOT EXISTS min_relevance_score float8  NOT NULL DEFAULT 0.70;
