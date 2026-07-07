// Plain-language explanations for agent-builder form fields, keyed by the exact
// label text used in the form. The shared Field component looks up this map so
// every field gets an inline "?" tooltip with no per-field wiring. Copy is
// sourced from the /docs pages so the two stay consistent.
export const FIELD_HELP: Record<string, string> = {
  'Agent name': 'A short, human-friendly name. Used in lists, playground, and when other agents call this one.',
  'Description': "One sentence on what this agent does. Shown to supervisors and in pickers — it's also the tool description when this agent is called by another.",
  'Provider': 'Which LLM vendor powers this agent (Anthropic, OpenAI, Gemini, Ollama). Must have credentials configured in Settings → Providers.',
  'Model': 'The specific model to run. Newer/larger models are more capable but cost more and can be slower.',
  'Status': 'Active agents can be run and called. Paused agents are hidden from execution; archived ones are kept for reference only.',
  'System prompt': "The agent's core instructions: its role, how it should behave step by step, which tools to use, output format, and edge cases. The single biggest lever on quality.",
  'Temperature': 'Randomness of responses (0–2). Low (0–0.3) = focused and deterministic; higher = more creative and varied. 0.7 is a balanced default.',
  'Max tokens': 'Upper bound on the length of a single model reply, measured in tokens (~¾ of a word each).',
  'Memory scope': 'How widely saved memories are shared: conversation (this chat only), agent (all chats with this agent), or workspace (shared across every agent).',
  'Save mode': 'How memories get written: tool (the agent decides via a save tool), extractor (a background pass extracts them), or hybrid (both).',
  'Review policy': 'When saved memories need confirmation: none (auto-save), uncertain (confirm only low-confidence ones), or all (confirm everything).',
  'Min save importance': 'Only memories scored at least this important (0–1) are kept. Higher = fewer, more significant memories.',
  'Dedupe threshold': 'Similarity (0–1) above which a new memory is treated as a duplicate of an existing one and skipped. Higher = stricter matching.',
  'Max memories per run': 'How many relevant memories to load into context at the start of each run.',
  'Min relevance score': 'Minimum similarity (0–1) a memory must have to the current query before it is retrieved.',
  'Retrieval strategy': 'How connected knowledge is used: a fixed pre-run injection, or agentic RAG where the agent decides when and what to retrieve during the run.',
  'Max chunks per run': 'Maximum number of document snippets pulled from connectors into context per run (fixed-injection mode).',
  'Max steps': 'Maximum reasoning/tool iterations in a single run before it stops. Prevents runaway loops.',
  'Max tool calls per run': 'Hard cap on total tool invocations in one run.',
  'Max run duration (sec)': 'Wall-clock limit for a run. It is terminated if it exceeds this.',
  'Max history messages': 'How many prior conversation messages are kept in context before older ones are trimmed or compacted.',
  'Compact after N messages': 'Once the conversation exceeds this many messages, older turns are summarized to save context space.',
  'Compact after N input tokens': 'Once input context passes this many tokens, older turns are compacted into a running summary.',
  'Lazy tool loading': 'Start each run with only meta-tools visible; the agent requests a tool’s full schema when it needs it. Keeps token cost low when many tools are attached.',
  'Category': 'A functional grouping (Communication, Dev & Code, …) so this is easy to find and filter alongside similar items.',
  'Required tools': 'Tools this skill needs. With lazy loading, they are auto-activated whenever the skill is requested.',
}
