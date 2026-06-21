-- Allow Self-Improvement to promote repeated, verified patterns into reusable skills or code tools.
UPDATE skills
SET
  description = 'Captures verified errors, learnings, and feature requests as durable memory, then promotes recurring patterns into reusable skills or code tools.',
  content = content || E'\n\n## Automatic Promotion\n\nPromote a memory only when it is verified, non-sensitive, and either recurs across at least three independent cases or the user explicitly asks to preserve it as reusable capability. Before creating anything, call `native_list_skills` or `native_list_workspace_tools` to avoid duplicating an existing resource.\n\n- **Promote to a skill** when the pattern is guidance, a workflow, a decision rule, or domain knowledge. Call `native_create_skill` with a concise name, description, and self-contained instructions; set `attach_to_self=true` and `ephemeral=false`. Include the trigger, required checks, and the prevention rule.\n- **Promote to a code tool** only when the pattern is a deterministic, pure transformation that has already been validated against representative inputs. Call `native_create_code_tool` with `ephemeral=false`; keep the implementation small, give it an explicit input schema, and rely only on the sandboxed JavaScript input and return value.\n\nDo not promote a one-off incident, a vague preference, a speculative fix, unsafe automation, credentials, or behavior requiring network, filesystem, or external side effects. Do not automatically create HTTP tools; they need an explicit user-provided endpoint and approval because they can affect external systems. If promotion fails, retain the compact memory and continue the task. Do not announce a promotion unless the user asks or it materially changes the current work.' ,
  required_tool_names = ARRAY[
    'native_save_memory',
    'native_list_skills',
    'native_create_skill',
    'native_list_workspace_tools',
    'native_create_code_tool'
  ],
  updated_at = NOW()
WHERE workspace_id IS NULL AND name = 'Self-Improvement';
