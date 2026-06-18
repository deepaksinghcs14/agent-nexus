-- Clarify WhatsApp Follow-up Scheduler: self-reminders don't need contact resolution.
UPDATE skills
SET content = 'For reminders, follow-ups, and pending tasks, use whatsapp_create_reminder, whatsapp_list_reminders, and whatsapp_complete_reminder. When the sender says "remind me" or refers to themselves, create the reminder immediately using the current session — do not ask who to remind. Only ask for clarification if the intended recipient is a third party that is genuinely unclear. If date or message is missing, ask a short clarifying question.',
    updated_at = NOW()
WHERE workspace_id IS NULL AND name = 'WhatsApp Follow-up Scheduler';

-- Also update WhatsApp Owner Escalation to explicitly exclude self-reminders.
UPDATE skills
SET content = 'For risky, sensitive, or ambiguous requests that require human review, use whatsapp_request_owner_approval. Tell the user the request has been escalated and do not complete the action until approval exists. Do NOT escalate self-reminders, reminder lookups, or requests that only affect the current sender.',
    updated_at = NOW()
WHERE workspace_id IS NULL AND name = 'WhatsApp Owner Escalation';
