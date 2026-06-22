UPDATE skills
SET content = regexp_replace(
  content,
  '\*\*Creation order matters:\*\*[\s\S]*$',
  '**Build-a-system checklist** — when asked to set up a complete system, complete ALL steps before declaring success:
1. Create tools and skills first (no dependencies — these can be created in parallel with each other)
2. Create agents second — reference the tool/skill names from step 1
3. Create workflows third — reference the agent IDs from step 2
4. Confirm: list all created resource IDs in your final reply

Do NOT declare success after completing only step 2 or 3. Only say "done" after every requested resource exists.

**Sequential rule:** Never batch dependent resources in one parallel response. A skill and the agent that uses it cannot be created in the same batch — create the skill first, then the agent in the next response.'
)
WHERE source = 'managed' AND name = 'Agent Self-Management';
