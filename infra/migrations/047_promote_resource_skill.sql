UPDATE skills
SET content = content || '

## Resource Lifetime Control
- `native_promote_resource(type, id)` — convert an ephemeral resource to permanent after creation. Call this once you know the resource is worth keeping. Supports type: "agent", "skill", "tool", "workflow".

**Guidance:** All resources are ephemeral by default (auto-delete at run end). Set `ephemeral=false` at creation time if you already know you want to keep it, OR call `native_promote_resource` at any point during the run to decide after seeing the result.'
WHERE source = 'managed' AND name = 'Agent Self-Management';
