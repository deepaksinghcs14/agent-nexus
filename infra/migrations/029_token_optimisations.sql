-- Token optimisation columns on agents:
--   max_history_messages: cap how many conversation turns are loaded per run (default 20)
--   lazy_tool_loading:    only send meta-tools upfront; activate full schemas on demand

ALTER TABLE agents
  ADD COLUMN IF NOT EXISTS max_history_messages INT NOT NULL DEFAULT 20,
  ADD COLUMN IF NOT EXISTS lazy_tool_loading BOOLEAN NOT NULL DEFAULT false;
