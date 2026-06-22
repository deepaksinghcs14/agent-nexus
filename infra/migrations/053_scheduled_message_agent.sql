ALTER TABLE gateway_scheduled_messages
  ADD COLUMN IF NOT EXISTS use_agent boolean NOT NULL DEFAULT false;
