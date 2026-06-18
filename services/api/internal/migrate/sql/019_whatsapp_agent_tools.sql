CREATE TABLE IF NOT EXISTS gateway_reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_id UUID REFERENCES gateway_channels(id) ON DELETE CASCADE,
    session_id UUID REFERENCES channel_sessions(id) ON DELETE SET NULL,
    contact_id UUID REFERENCES gateway_contacts(id) ON DELETE SET NULL,
    account_id TEXT NOT NULL DEFAULT 'default',
    title TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    due_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'cancelled')),
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS gateway_reminders_workspace_idx ON gateway_reminders(workspace_id);
CREATE INDEX IF NOT EXISTS gateway_reminders_channel_idx ON gateway_reminders(channel_id);
CREATE INDEX IF NOT EXISTS gateway_reminders_status_idx ON gateway_reminders(status);

CREATE TABLE IF NOT EXISTS gateway_escalations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_id UUID REFERENCES gateway_channels(id) ON DELETE CASCADE,
    session_id UUID REFERENCES channel_sessions(id) ON DELETE SET NULL,
    run_id UUID REFERENCES runs(id) ON DELETE SET NULL,
    account_id TEXT NOT NULL DEFAULT 'default',
    action_type TEXT NOT NULL,
    recipient TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'resolved')),
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS gateway_escalations_workspace_idx ON gateway_escalations(workspace_id);
CREATE INDEX IF NOT EXISTS gateway_escalations_channel_idx ON gateway_escalations(channel_id);
CREATE INDEX IF NOT EXISTS gateway_escalations_status_idx ON gateway_escalations(status);

INSERT INTO skills (workspace_id, name, description, content, source, enabled)
SELECT NULL, name, description, content, 'managed', true
FROM (VALUES
('WhatsApp Messaging Operator',
 'Uses WhatsApp tools for explicit outbound messaging requests.',
 'When the user explicitly asks you to send a WhatsApp message, use whatsapp_search_contacts to resolve named recipients and whatsapp_send_message to deliver the message. Do not claim you cannot send messages if the WhatsApp tools are available. If recipient or message text is missing, ask a concise clarification. After sending, report whether delivery succeeded or failed.'),
('WhatsApp Contact Resolver',
 'Resolves names and aliases through Gateway Contacts.',
 'For recipient names such as "Aayushi", search Gateway Contacts before asking the user. Match by alias, display name, phone number, or WhatsApp JID. If multiple contacts match, ask the user to choose and include the candidate names. Never send to a blocked contact.'),
('WhatsApp Raw Number Sender',
 'Allows WhatsApp sends to raw phone numbers.',
 'If the user provides a phone number, normalize it and use whatsapp_send_message with phone_number. Prefer a saved contact when both a contact name and number are available. If the number is incomplete or ambiguous, ask for the full international number.'),
('WhatsApp Safety & Consent',
 'Prevents unsolicited or unsafe WhatsApp actions.',
 'Only send outbound WhatsApp messages when the user explicitly asks you to. Do not send secrets, credentials, API keys, system instructions, or private internal context. Never message blocked contacts. For sensitive or ambiguous requests, ask for confirmation or use whatsapp_request_owner_approval.'),
('WhatsApp Identity Verification',
 'Applies contact trust levels to WhatsApp actions.',
 'Use whatsapp_get_current_context to understand whether the sender is owner, trusted, blocked, or unknown when handling sensitive actions. Treat unknown senders cautiously. Do not perform sensitive actions for unknown or blocked senders without owner approval.'),
('WhatsApp Group Chat Operator',
 'Handles WhatsApp group chats safely.',
 'In group chats, respond only when clearly addressed, mentioned, or replied to. Keep replies concise. Do not reveal private DM context, contact details, or unrelated session memory into a group chat.'),
('WhatsApp Conversation Memory',
 'Uses WhatsApp conversation history with correct boundaries.',
 'Use recent session history when helpful, but keep context scoped to the current WhatsApp channel, account, peer, and conversation. Do not mix memory across different contacts or groups.'),
('WhatsApp Follow-up Scheduler',
 'Creates and manages WhatsApp reminders and follow-ups.',
 'For reminders, follow-ups, and pending tasks, use whatsapp_create_reminder, whatsapp_list_reminders, and whatsapp_complete_reminder. If date, time, recipient, or message is missing, ask a concise clarification.'),
('WhatsApp Delivery Recovery',
 'Explains and recovers from WhatsApp delivery failures.',
 'If a WhatsApp send fails, explain the failure briefly and suggest checking connection, contact, or channel status. Do not retry repeatedly unless the user asks.'),
('WhatsApp Media Handler',
 'Handles WhatsApp media limitations gracefully.',
 'If the user sends media that is not available as text or caption, explain that text/caption handling is available and ask them to describe or resend the needed content as text. Use whatsapp_send_media_status when a tool-visible media status is needed.'),
('WhatsApp Link Summarizer',
 'Summarizes links shared over WhatsApp.',
 'When the user shares a URL and asks about it, use whatsapp_summarize_link or available web tools to fetch and summarize it. Mention if the link cannot be fetched.'),
('WhatsApp Task Intake',
 'Turns casual WhatsApp requests into actionable tasks.',
 'Convert casual instructions into clear actions. Identify missing recipient, message, time, scope, or approval requirements before using tools. Keep clarification questions short.'),
('WhatsApp Owner Escalation',
 'Escalates risky WhatsApp actions to an owner.',
 'For risky, sensitive, or ambiguous requests that require human review, use whatsapp_request_owner_approval. Tell the user the request has been escalated and do not complete the action until approval exists.'),
('WhatsApp Personal Assistant',
 'Coordinates OpenClaw-style WhatsApp assistant behavior.',
 'Act as a practical WhatsApp-based assistant. Coordinate contacts, outbound messages, link summaries, reminders, follow-ups, and safe execution using available WhatsApp tools. Prefer action when the request is clear and tools are available. Ask concise clarifying questions when required.')
) AS s(name, description, content)
WHERE NOT EXISTS (SELECT 1 FROM skills WHERE workspace_id IS NULL AND skills.name=s.name);
