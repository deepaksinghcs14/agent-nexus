INSERT INTO skills (id, name, description, content, source, enabled, required_tool_names)
VALUES (
  gen_random_uuid(),
  'WhatsApp Message Scheduler',
  'Guides agents to schedule one-time or recurring WhatsApp messages, including AI-generated dynamic content.',
  $skill$
## WhatsApp Message Scheduler

Use `whatsapp_schedule_message` to send a WhatsApp message at a future time, optionally on a recurring schedule.

### When to use
- User asks to send a message to a contact at a specific time or date
- User wants to set up recurring messages (daily, weekly, monthly, yearly, weekdays)
- User wants the agent to generate a personalized message at send time (`use_agent=true`)

### Key parameters
- `message` — text to send (or prompt for the agent when `use_agent=true`)
- `send_at` — ISO datetime or 'YYYY-MM-DD HH:MM:SS UTC' (e.g. '2026-06-23 09:00:00 UTC')
- `to` — contact name, alias, or phone number (or use `contact_id` / `whatsapp_jid`)
- `recurrence_frequency` — `"daily"`, `"weekly"`, `"monthly"`, `"yearly"`, or `"weekdays"` for repeating messages
- `recurrence_interval` — every N frequency-units (e.g. 2 + weekly = every 2 weeks; default 1)
- `recurrence_end_at` / `recurrence_max_occurrences` — stop condition for recurring messages
- `use_agent` — set `true` to have the contact's assigned agent generate the message dynamically at send time; the `message` field becomes the prompt

### Dynamic messages with use_agent
When `use_agent=true`:
- The `message` field is a **prompt**, not the literal text to send
- At send time the contact's assigned agent runs the prompt and its response is delivered to the contact
- Use this for personalized greetings, weekly reports, creative content, follow-ups, or any message that should vary each time

### Examples
Schedule a one-time static message:
`whatsapp_schedule_message(to="Alice", message="Don't forget our meeting tomorrow!", send_at="2026-06-23 09:00:00 UTC")`

Schedule a weekly AI-generated motivation message:
`whatsapp_schedule_message(to="Alice", message="Write an uplifting Monday motivation message for Alice.", send_at="2026-06-23 08:00:00 UTC", recurrence_frequency="weekly", use_agent=true)`

Schedule a yearly birthday greeting:
`whatsapp_schedule_message(to="Bob", message="Write a warm birthday greeting for Bob.", send_at="2026-07-15 10:00:00 UTC", recurrence_frequency="yearly", use_agent=true)`

### Rules
- Always call the tool immediately — do not say "I will schedule it" without calling the tool first
- If the time is ambiguous, ask once for clarification before scheduling
- Prefer `contact_id` when the contact UUID is known; fall back to `to` for name/phone lookup
- Use `use_agent=true` whenever the user asks for a personalized, creative, or varying message
$skill$,
  'managed', true, ARRAY['whatsapp_schedule_message']
)
ON CONFLICT DO NOTHING;
