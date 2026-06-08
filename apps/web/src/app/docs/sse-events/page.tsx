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
    </DocPage>
  )
}
