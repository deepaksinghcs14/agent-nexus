import { DocPage, Callout } from '@/components/docs/DocPage'
import AgentInvokePlayground from '@/components/docs/AgentInvokePlayground'
import ApiPlayground from '@/components/docs/ApiPlayground'

export const metadata = { title: 'Invoke API — Docs' }

export default function InvokeApiDoc() {
  return (
    <DocPage
      title="Invoke API"
      subtitle="Run agents and workflows with a single HTTP request. Supports both streaming (SSE) and polling modes."
    >
      <h2>Invoke an Agent or Workflow</h2>
      <p>
        Select an agent or workflow below and send a message to it directly from the browser.
        Returns a <code>run_id</code> immediately; the agent runs asynchronously in polling mode,
        or streams SSE events in real time when streaming is enabled.
      </p>

      <AgentInvokePlayground />

      <h2>Streaming Mode</h2>
      <p>
        Set <code>stream: true</code> to receive the response as an SSE stream. The connection stays
        open until the run completes. Each event is a JSON object on a <code>data:</code> line.
      </p>
      <pre><code>{`curl -X POST /api/v1/invoke/agents/AGENT_ID \\
  -H "Authorization: Bearer anx_..." \\
  -H "Content-Type: application/json" \\
  -d '{"input":"Hello","stream":true}'

# Response (SSE stream):
data: {"type":"run_started","run_id":"..."}
data: {"type":"delta","content":"Hello"}
data: {"type":"delta","content":"! How can I help?"}
data: {"type":"run_completed","run_id":"...","usage":{"input":42,"output":16},"cost":0.001}`}</code></pre>

      <h2>Conversation Context</h2>
      <p>
        To maintain conversation history across multiple invocations, pass the same{' '}
        <code>conversation_id</code> in subsequent requests. Omitting it creates a new conversation each time.
      </p>
      <pre><code>{`// First turn — creates a new conversation
POST /api/v1/invoke/agents/AGENT_ID
{"input": "What is the capital of France?"}
→ {"run_id": "r1", "conversation_id": "c1", "status": "running"}

// Second turn — continues the same conversation
POST /api/v1/invoke/agents/AGENT_ID
{"input": "And what language do they speak there?", "conversation_id": "c1"}
→ {"run_id": "r2", "conversation_id": "c1", "status": "running"}`}</code></pre>

      <h2>Poll Run Status</h2>
      <p>
        After invoking, poll <code>GET /runs/:id</code> until status is <code>success</code> or{' '}
        <code>failed</code>. If the run needs human approval for a tool call, status becomes{' '}
        <code>approval_wait</code> — see <a href="/docs/run-states">Run States</a> for handling it.
      </p>

      <ApiPlayground
        method="GET"
        path="/runs/{runId}"
        description="Get the current status, output, steps, and any pending approval request for a run."
        pathParams={[{ name: 'runId', label: 'Run ID', placeholder: 'paste-run-id-from-invoke' }]}
      />

      <Callout type="tip">
        <strong>Tip:</strong> Poll with a 1–2 second interval. Most short agent runs complete in under
        5 seconds; complex runs with tool calls may take 30+ seconds.
      </Callout>

      <h2>Request Reference</h2>

      <h3>Invoke Agent</h3>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Field</th><th>Type</th><th>Required</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td><code>input</code></td><td>string</td><td>Yes</td><td>The message to send to the agent</td></tr>
          <tr><td><code>conversation_id</code></td><td>string (UUID)</td><td>No</td><td>Continue an existing conversation</td></tr>
          <tr><td><code>stream</code></td><td>boolean</td><td>No</td><td>Return SSE stream instead of polling response (default: false)</td></tr>
        </tbody>
      </table>
      </div>

      <h3>Invoke Group</h3>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Field</th><th>Type</th><th>Required</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td><code>input</code></td><td>string</td><td>Yes</td><td>The initial input passed to the first agent in the pipeline</td></tr>
          <tr><td><code>stream</code></td><td>boolean</td><td>No</td><td>Return SSE stream (default: false)</td></tr>
        </tbody>
      </table>
      </div>
    </DocPage>
  )
}
