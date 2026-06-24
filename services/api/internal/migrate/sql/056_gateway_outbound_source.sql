-- Track why an outbound WhatsApp message was sent so final gateway replies can
-- avoid repeating a direct message back to the person who requested it.
ALTER TABLE gateway_outbound_messages
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'assistant_reply';

CREATE INDEX IF NOT EXISTS gateway_outbound_run_source_idx
    ON gateway_outbound_messages(run_id, source);
