UPDATE skills
SET content = regexp_replace(
  content,
  '\n## Resource Lifetime Control[\s\S]*$',
  '
## Resource Lifetime Control
- `native_promote_resource(type, id)` — convert an ephemeral resource to permanent after creation. Supports type: "agent", "skill", "tool", "workflow".

**Default rule:** Resources are ephemeral by default (auto-delete at run end). Apply this rule to decide:
- User intent is to **build, create, set up, or establish** something → always set `ephemeral=false` at creation time. The user expects to find it in the UI after the run.
- User asks for a **temporary, test, or disposable** resource → keep the default (ephemeral=true).
- **When in doubt:** if the user would reasonably want the resource to exist after this conversation, set `ephemeral=false`.
- Use `native_promote_resource` when you need to see the result FIRST before deciding — call it at the end of the run for any resource that proved worth keeping.',
  ''
)
WHERE source = 'managed' AND name = 'Agent Self-Management';
