ALTER TABLE gateway_reminders
  ADD COLUMN IF NOT EXISTS use_agent boolean NOT NULL DEFAULT false;
