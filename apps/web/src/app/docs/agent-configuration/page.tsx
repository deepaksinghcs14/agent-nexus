import { DocPage, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'Agent Configuration — Docs' }

const FIELDS = [
  { field: 'name', type: 'string', desc: 'Display name for the agent.' },
  { field: 'description', type: 'string', desc: 'Short description of what the agent does. Shown in the agent list.' },
  { field: 'instructions', type: 'string', desc: "System prompt. Defines the agent's personality, role, and constraints." },
  { field: 'provider', type: 'anthropic | openai | gemini | ollama', desc: 'LLM provider. Must have a configured API key for the provider in your workspace.' },
  { field: 'model', type: 'string', desc: 'Model ID (e.g. claude-sonnet-4-6, gpt-4o). Must match the chosen provider.' },
  { field: 'temperature', type: 'float 0–1', desc: 'Controls response randomness. 0 = deterministic, 1 = very creative.' },
  { field: 'max_tokens', type: 'integer', desc: 'Maximum tokens the model can generate in a single response.' },
  { field: 'memory_enabled', type: 'boolean', desc: 'Whether to retrieve and store memories from previous runs.' },
  { field: 'memory_scope', type: 'conversation | agent | workspace', desc: 'Scope of memories retrieved and stored. conversation = not persisted across runs; agent = private to this agent; workspace = shared across all agents in the workspace.' },
  { field: 'max_memories', type: 'integer (default 5)', desc: 'Maximum number of past memories injected into the prompt per run. Higher values give more context but use more tokens.' },
  { field: 'min_relevance_score', type: 'float 0–1 (default 0.70)', desc: 'Only memories with a relevance score at or above this threshold are injected. Set to 0 to disable filtering.' },
  { field: 'context_retrieval_enabled', type: 'boolean', desc: 'Whether to retrieve relevant document chunks from connected connectors (RAG).' },
  { field: 'max_steps', type: 'integer (default 10)', desc: 'Maximum number of model+tool call cycles per run. Prevents infinite loops.' },
  { field: 'max_tool_calls', type: 'integer (default 20)', desc: 'Maximum tool calls per run across all steps.' },
  { field: 'max_duration_secs', type: 'integer (default 300)', desc: 'Hard timeout for the run in seconds. Run is cancelled when exceeded.' },
  { field: 'status', type: 'active | paused | archived', desc: 'Only active agents can be invoked.' },
]

export default function AgentConfigurationDoc() {
  return (
    <DocPage
      title="Agent Configuration"
      subtitle="Reference for every field in the agent builder. Create or update agents via the UI or the REST API."
    >
      <h2>REST API</h2>
      <pre><code>{`# Create an agent
POST /api/v1/agents

# Update an agent
PUT /api/v1/agents/:id`}</code></pre>

      <h2>Field Reference</h2>
      <table>
        <thead>
          <tr>
            <th>Field</th>
            <th>Type</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          {FIELDS.map((f) => (
            <tr key={f.field}>
              <td><code>{f.field}</code></td>
              <td style={{ whiteSpace: 'nowrap', color: '#6b7280', fontSize: '0.8125rem' }}>{f.type}</td>
              <td>{f.desc}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>Example Request</h2>
      <pre><code>{`POST /api/v1/agents
{
  "name": "Support Agent",
  "description": "Handles tier-1 customer support queries.",
  "instructions": "You are a helpful customer support agent. Be concise and friendly.",
  "provider": "anthropic",
  "model": "claude-sonnet-4-6",
  "temperature": 0.3,
  "max_tokens": 1024,
  "memory_enabled": true,
  "memory_scope": "agent",
  "max_memories": 5,
  "min_relevance_score": 0.70,
  "context_retrieval_enabled": false,
  "max_steps": 10,
  "max_tool_calls": 20,
  "max_duration_secs": 300,
  "status": "active"
}`}</code></pre>

      <Callout type="tip">
        You can also create and configure agents through the UI at{' '}
        <a href="/agents/new">Agents → New Agent</a>. The builder form maps directly to these fields.
      </Callout>

      <h2>Agent Self-Management tools</h2>
      <p>
        Enable the <strong>Agent Self-Management</strong> skill to give an agent the ability to
        orchestrate other agents, create resources at runtime, and clean up after itself — without
        a pre-configured workflow canvas.
      </p>
      <table>
        <thead>
          <tr><th>Tool</th><th>What it does</th></tr>
        </thead>
        <tbody>
          <tr><td><code>native_list_agents</code></td><td>List agents available in the workspace.</td></tr>
          <tr><td><code>native_call_agent</code></td><td>Delegate a task to another agent and return its output. Issue multiple calls in one response to run sub-agents in parallel.</td></tr>
          <tr><td><code>native_create_agent</code></td><td>Create a new agent at runtime. Set <code>ephemeral=true</code> to auto-delete it when the root run ends.</td></tr>
          <tr><td><code>native_delete_agent</code></td><td>Delete an agent created by the current run.</td></tr>
          <tr><td><code>native_list_skills</code></td><td>List skills available in the workspace.</td></tr>
          <tr><td><code>native_create_skill</code></td><td>Create a skill at runtime. Use <code>attach_to_self=true</code> to inject its content into the calling agent&apos;s own context immediately.</td></tr>
          <tr><td><code>native_delete_skill</code></td><td>Delete a skill created by the current run.</td></tr>
          <tr><td><code>native_list_http_tools</code></td><td>List HTTP tools in the workspace.</td></tr>
          <tr><td><code>native_create_http_tool</code></td><td>Register an external API endpoint as a callable tool. Auto-attached to the calling agent so it is available on the next turn.</td></tr>
          <tr><td><code>native_delete_tool</code></td><td>Delete an HTTP tool created by the current run.</td></tr>
        </tbody>
      </table>
      <p>
        Sub-agent chains are capped at <strong>3 levels deep</strong>. Resources created with
        <code>ephemeral=true</code> are deleted automatically after the root run completes.
        Agents can only delete resources they created in the current run.
      </p>
    </DocPage>
  )
}
