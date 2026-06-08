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
  { field: 'memory_scope', type: 'conversation | agent | workspace', desc: 'Scope of memories retrieved: per-conversation, per-agent, or shared across the workspace.' },
  { field: 'context_retrieval_enabled', type: 'boolean', desc: 'Whether to retrieve relevant document chunks from connected connectors.' },
  { field: 'max_steps', type: 'integer', desc: 'Maximum number of model+tool call cycles per run. Prevents infinite loops.' },
  { field: 'max_tool_calls', type: 'integer', desc: 'Maximum tool calls per run across all steps.' },
  { field: 'max_duration_secs', type: 'integer', desc: 'Hard timeout for the run in seconds.' },
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
  "memory_scope": "conversation",
  "context_retrieval_enabled": false,
  "max_steps": 5,
  "status": "active"
}`}</code></pre>

      <Callout type="tip">
        You can also create and configure agents through the UI at{' '}
        <a href="/agents/new">Agents → New Agent</a>. The builder form maps directly to these fields.
      </Callout>
    </DocPage>
  )
}
