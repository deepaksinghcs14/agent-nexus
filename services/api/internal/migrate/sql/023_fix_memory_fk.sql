-- Fix memories.source_run_id FK: RESTRICT → SET NULL so conversations can be deleted
ALTER TABLE memories DROP CONSTRAINT IF EXISTS memories_source_run_id_fkey;
ALTER TABLE memories ADD CONSTRAINT memories_source_run_id_fkey
    FOREIGN KEY (source_run_id) REFERENCES runs(id) ON DELETE SET NULL;
