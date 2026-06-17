ALTER TABLE gateway_escalations
    ADD COLUMN IF NOT EXISTS approval_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS resolved_by_sender_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS gateway_escalations_pending_code_unique
    ON gateway_escalations(channel_id, approval_code)
    WHERE approval_code <> '' AND status = 'pending';

UPDATE skills
SET content = 'When a WhatsApp request is risky, ambiguous, or could affect another person, call whatsapp_request_owner_approval before taking action. The tool creates an approval code and notifies owner contacts. Tell the requester that owner approval is pending. Owners can reply in WhatsApp with approve CODE or reject CODE. Owners can also reply disable approvals to turn off chat approvals, and enable approvals to turn them back on.'
WHERE workspace_id IS NULL
  AND name = 'WhatsApp Owner Escalation';
