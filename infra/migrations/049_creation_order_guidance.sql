UPDATE skills
SET content = content || '

**Creation order matters:** When resources depend on each other, create them sequentially (one response per step), not in parallel:
- Create tools and skills FIRST (they have no dependencies)
- Create agents SECOND (reference skills/tools by name from the step above)
- Create workflows THIRD (reference agent IDs from the step above)
Batching dependent resources in one parallel response causes failures — the downstream resource gets created before the upstream one exists.'
WHERE source = 'managed' AND name = 'Agent Self-Management';
