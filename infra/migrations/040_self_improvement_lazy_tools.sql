-- Make Self-Improvement compatible with lazy tool loading.
UPDATE skills
SET
  content = content || E'\n\n## Lazy Tool Loading\n\nIf lazy tool loading is enabled, tool schemas are not available until requested. Before using a Self-Improvement tool, call `native_list_tools` if you need to confirm its exact name, then call `native_request_tool` with only the tool needed for the next operation. On the following turn, call the requested tool.\n\nRequest `native_save_memory` to record an error, learning, feature request, or resolution. Request `native_list_skills` before checking for a skill duplicate, `native_create_skill` only when promoting a qualifying pattern, `native_list_workspace_tools` before checking for a code-tool duplicate, and `native_create_code_tool` only when promoting a validated deterministic transformation. Do not request all promotion tools pre-emptively. Once a requested tool is active, use it only as needed for the current run.',
  updated_at = NOW()
WHERE workspace_id IS NULL AND name = 'Self-Improvement';
