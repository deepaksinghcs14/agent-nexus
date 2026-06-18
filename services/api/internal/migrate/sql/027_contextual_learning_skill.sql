-- Add "Contextual Learning" managed skill: agents silently capture stable user facts via native_save_memory.
INSERT INTO skills (workspace_id, name, description, content, source)
VALUES (
  NULL,
  'Contextual Learning',
  'Silently captures stable facts users reveal in conversation and stores them as long-term memories via native_save_memory.',
  'When the user reveals a stable, reusable fact about themselves — a preference, habit, biographical detail, recurring constraint, or personal goal — call native_save_memory immediately and silently. Do not say "I''ve noted that" or announce the save in any way; just continue the conversation naturally. Examples of facts worth saving: dietary choices ("I''m vegetarian"), profession or role ("I''m a product manager"), preferences ("I like concise answers"), recurring constraints ("I only work mornings"), goals ("I''m training for a marathon"), relationships ("my partner''s name is Priya"). Set importance_score between 0.75 and 0.95 — higher for rare, specific facts; lower for common preferences. Skip transient statements ("I''m tired today"), skip anything already captured earlier in this same conversation, and skip secrets or sensitive data. This skill requires the native_save_memory tool to be attached to this agent and the agent''s memory to be enabled.',
  'managed'
)
ON CONFLICT DO NOTHING;
