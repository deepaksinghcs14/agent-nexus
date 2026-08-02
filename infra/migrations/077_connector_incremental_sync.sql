-- Durable "last successful sync" watermark for incremental fetching.
-- Distinct from connector_sync_jobs.checkpoint: that column is crash-resume
-- state, cleared on every successful completion (syncstate.Reporter.Complete)
-- so it can never carry cross-sync incremental state. This one persists
-- across successful syncs and lets each provider skip unchanged
-- files/repos/pages instead of re-fetching everything every time.
ALTER TABLE connectors ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ;
