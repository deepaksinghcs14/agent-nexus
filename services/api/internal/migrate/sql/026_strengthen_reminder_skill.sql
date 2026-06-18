-- Make reminder/scheduler skill instruction forceful enough for Gemini models.
UPDATE skills
SET content = 'IMPORTANT: Whenever someone asks to be reminded of anything — in any language, casual phrasing, or tense — you MUST call whatsapp_create_reminder immediately. Never say "I will remind you" or "I''ll remind you" without first calling the tool. If you respond with a promise to remind someone without calling the tool, the reminder is silently lost. When the sender says "remind me", use the current session as the recipient — do not ask who to remind. Only ask a single short clarifying question if the message or time is completely missing.',
    updated_at = NOW()
WHERE workspace_id IS NULL AND name = 'WhatsApp Follow-up Scheduler';
