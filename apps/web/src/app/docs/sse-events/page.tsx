import Link from 'next/link'
import { DocPage, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'SSE Events — Docs' }

const EVENTS = [
  {
    type: 'run_started',
    payload: '{"type":"run_started","run_id":"r_abc123"}',
    desc: 'Emitted once when the run begins executing. Use the run_id for subsequent polling.',
  },
  {
    type: 'delta',
    payload: '{"type":"delta","content":"Hello"}',
    desc: "A token or chunk of the model's response. Concatenate all deltas to get the full reply.",
  },
  {
    type: 'step_completed',
    payload: '{"type":"step_completed","step":{"type":"memory_retrieval","latency_ms":38}}',
    desc: 'Emitted after each internal step: memory_retrieval, context_retrieval, model_call, tool_call.',
  },
  {
    type: 'tool_call',
    payload: '{"type":"tool_call","tool":"read_file","input":{"path":"report.md"}}',
    desc: 'An agent tool call is about to be executed.',
  },
  {
    type: 'approval_required',
    payload: '{"type":"approval_required","tool":"write_file","input":{...},"approval_id":"ar_xyz"}',
    desc: 'The run is paused waiting for approval. Submit POST /runs/:id/approve to continue.',
  },
  {
    type: 'run_completed',
    payload: '{"type":"run_completed","run_id":"r_abc123","usage":{"input":1200,"output":340},"cost":0.004}',
    desc: 'Terminal event. The run finished successfully. Contains token usage and estimated cost.',
  },
  {
    type: 'error',
    payload: '{"type":"error","error":"model returned an empty response"}',
    desc: 'An error occurred. The run has been marked as failed. This is the final event.',
  },
]

const WORKFLOW_EVENTS = [
  {
    type: 'node_started',
    payload: '{"type":"node_started","node_id":"n_1","node_type":"agent","node_name":"Researcher"}',
    desc: 'A workflow node has started executing. Use node_id to highlight the corresponding node on the canvas.',
  },
  {
    type: 'node_completed',
    payload: '{"type":"node_completed","node_id":"n_1","node_name":"Researcher"}',
    desc: 'A workflow node finished. Its output is checkpointed at this point — if the API process restarts after this event, the node will not be re-run when the workflow resumes.',
  },
  {
    type: 'node_resumed',
    payload: '{"type":"node_resumed","node_id":"n_1","node_name":"Researcher"}',
    desc: 'The workflow run was resumed after a server restart, and this node’s output was replayed from its last checkpoint instead of being re-executed.',
  },
  {
    type: 'node_routed',
    payload: '{"type":"node_routed","node_id":"n_2","result":"yes","next_node_id":"n_3"}',
    desc: 'A condition or loop node evaluated its expression and chose the next node. Loop nodes also include "iteration" and "max".',
  },
]

export default function SSEEventsDoc() {
  return (
    <DocPage
      title="SSE Events"
      subtitle={`When you invoke an agent with stream: true, the response is a Server-Sent Events stream. Each event is a JSON object preceded by data:.`}
    >
      <h2>Connecting to the Stream</h2>
      <pre><code>{`// Browser
const res = await fetch('/api/v1/invoke/agents/AGENT_ID', {
  method: 'POST',
  headers: {
    Authorization: 'Bearer anx_...',
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({ input: 'Hello', stream: true }),
})

const reader = res.body.getReader()
const decoder = new TextDecoder()
let buffer = ''

while (true) {
  const { done, value } = await reader.read()
  if (done) break
  buffer += decoder.decode(value)

  const lines = buffer.split('\\n\\n')
  buffer = lines.pop() ?? ''

  for (const line of lines) {
    if (line.startsWith('data: ')) {
      const event = JSON.parse(line.slice(6))
      console.log(event.type, event)
    }
  }
}`}</code></pre>

      <Callout type="tip">
        For Node.js or server-side environments, use the{' '}
        <code>eventsource</code> npm package or the native <code>EventSource</code> API (Node 22+).
      </Callout>

      <h2>Event Reference</h2>

      {EVENTS.map((e) => (
        <div key={e.type}>
          <h3><code>{e.type}</code></h3>
          <p>{e.desc}</p>
          <pre><code>{e.payload}</code></pre>
        </div>
      ))}

      <h2>Workflow-Specific Events</h2>
      <p>
        Invoking a workflow (<code>POST /api/v1/invoke/workflows/:id</code> with <code>stream: true</code>)
        emits these events in addition to the ones above — <code>delta</code>, <code>tool_call</code>, and
        <code>step_completed</code> events from each node&apos;s underlying agent run are also forwarded,
        tagged with <code>node_id</code>/<code>node_name</code> so you can attribute them to the right node.
      </p>

      {WORKFLOW_EVENTS.map((e) => (
        <div key={e.type}>
          <h3><code>{e.type}</code></h3>
          <p>{e.desc}</p>
          <pre><code>{e.payload}</code></pre>
        </div>
      ))}

      <Callout type="info">
        Workflow runs checkpoint their progress after every <code>node_completed</code> event. If the API
        process restarts mid-run, it resumes automatically from the last checkpoint on boot instead of
        losing progress — see <Link href="/docs/workflows">Workflows &amp; Node Types</Link> for details.
      </Callout>
    </DocPage>
  )
}
