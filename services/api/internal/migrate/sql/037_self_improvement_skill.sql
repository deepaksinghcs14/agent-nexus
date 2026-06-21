-- Add a managed skill for retaining verified operational learnings across runs.
INSERT INTO skills (workspace_id, name, description, content, source, enabled, required_tool_names)
SELECT
  NULL,
  'Self-Improvement',
  'Captures verified corrections and reusable operating lessons so the agent improves across future runs.',
  E'## Self-Improvement\n\nImprove future work from verified evidence. When the user corrects you, a tool or integration exposes a non-obvious failure mode, or you confirm a better repeatable approach, first apply the lesson in the current response. Then call `native_save_memory` once with a compact, self-contained prevention rule.\n\nSave only durable lessons that are likely to help on a future run: project conventions, user preferences, reliable tool behaviour, validated constraints, or a correction that prevents the same mistake. Include the relevant context and the correct action, for example: "For this workspace, use migration files in both infra/migrations and services/api/internal/migrate/sql; the API embeds the latter at build time."\n\nDo not save transient retries, raw logs, unverified hypotheses, ordinary task state, full transcripts, credentials, tokens, private keys, sensitive personal data, or information that is already clearly captured in the current conversation. Do not use memory as a debugging diary.\n\nSet `importance_score` to 0.75–0.95 for durable corrections or high-impact repeatable lessons, and 0.60–0.74 for lower-impact conventions. Use a short `reason` that identifies the triggering correction or verified result. Do not claim that a lesson was remembered unless the tool call succeeds, and do not announce saves unless the user asks.\n\nIf the lesson is uncertain, verify it before saving. If it is too broad or could change over time, keep it out of memory and use it only for the current task.',
  'managed',
  true,
  ARRAY['native_save_memory']
WHERE NOT EXISTS (
  SELECT 1 FROM skills WHERE workspace_id IS NULL AND name = 'Self-Improvement'
);
