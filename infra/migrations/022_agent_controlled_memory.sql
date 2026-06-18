-- Add agent-controlled memory save policies and reviewable memory candidates.

ALTER TABLE agents
  ADD COLUMN IF NOT EXISTS memory_save_mode text NOT NULL DEFAULT 'hybrid',
  ADD COLUMN IF NOT EXISTS memory_review_policy text NOT NULL DEFAULT 'uncertain',
  ADD COLUMN IF NOT EXISTS memory_min_importance float8 NOT NULL DEFAULT 0.70,
  ADD COLUMN IF NOT EXISTS memory_dedupe_threshold float8 NOT NULL DEFAULT 0.88;

ALTER TABLE memories
  ADD COLUMN IF NOT EXISTS conversation_id uuid REFERENCES conversations(id) ON DELETE CASCADE,
  ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active',
  ADD COLUMN IF NOT EXISTS importance_score float8 NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS save_source text NOT NULL DEFAULT 'extractor',
  ADD COLUMN IF NOT EXISTS metadata jsonb NOT NULL DEFAULT '{}';

UPDATE memories
SET importance_score = relevance_score
WHERE importance_score = 0 AND relevance_score > 0;

CREATE INDEX IF NOT EXISTS memories_status_idx ON memories(status);
CREATE INDEX IF NOT EXISTS memories_conversation_idx ON memories(conversation_id);
CREATE INDEX IF NOT EXISTS memories_save_source_idx ON memories(save_source);
