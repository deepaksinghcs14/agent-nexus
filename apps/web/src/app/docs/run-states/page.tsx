import { DocPage, Badge, Callout } from '@/components/docs/DocPage'
import ApiPlayground from '@/components/docs/ApiPlayground'

export const metadata = { title: 'Run States — Docs' }

type BadgeColor = 'gray' | 'amber' | 'purple' | 'green' | 'red' | 'blue'

const STATES: { state: string; color: BadgeColor; desc: string }[] = [
  { state: 'pending',       color: 'gray',   desc: 'Queued, not yet started. Poll again shortly.' },
  { state: 'running',       color: 'amber',  desc: 'Active. Model is generating a response. Poll for updates or subscribe via SSE.' },
  { state: 'approval_wait', color: 'purple', desc: 'Paused. An agent tool call requires human approval before continuing. Survives API restarts — approving resumes the run.' },
  { state: 'session_wait',  color: 'blue',   desc: 'Paused. An autonomous coding session is executing in the runner service (minutes to hours). Resumes automatically on the session completion callback; survives API restarts.' },
  { state: 'success',       color: 'green',  desc: 'Completed. The output field contains the final response.' },
  { state: 'failed',        color: 'red',    desc: 'Error occurred. The error_message field has details.' },
  { state: 'cancelled',     color: 'gray',   desc: 'Cancelled by user. Terminal state.' },
]

export default function RunStatesDoc() {
  return (
    <DocPage
      title="Run States"
      subtitle="Every agent run moves through a well-defined lifecycle. Understanding run states lets you build robust integrations that handle all outcomes."
    >
      <h2>State Reference</h2>
      <div className="table-scroll">
      <table>
        <thead><tr><th>State</th><th>Description</th></tr></thead>
        <tbody>
          {STATES.map((s) => (
            <tr key={s.state}>
              <td><Badge label={s.state} color={s.color} /></td>
              <td>{s.desc}</td>
            </tr>
          ))}
        </tbody>
      </table>
      </div>

      <h2>Recommended Polling Pattern</h2>
      <pre><code>{`const TERMINAL = new Set(['success', 'failed', 'cancelled'])

async function waitForRun(runId, token) {
  while (true) {
    const res = await fetch(\`/api/v1/runs/\${runId}\`, {
      headers: { Authorization: \`Bearer \${token}\` }
    })
    const { run } = await res.json()

    if (run.status === 'approval_wait') {
      const decision = await askUser(run.approval_request)
      await fetch(\`/api/v1/runs/\${runId}/approve\`, {
        method: 'POST',
        headers: { Authorization: \`Bearer \${token}\`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ decision })
      })
    }

    if (TERMINAL.has(run.status)) return run
    await new Promise(r => setTimeout(r, 1000))
  }
}`}</code></pre>

      <h2>Approval Gates</h2>
      <p>
        When an agent tool is configured with <code>requires_approval: true</code>, the run pauses at{' '}
        <code>approval_wait</code>. The <code>GET /runs/:id</code> response includes an{' '}
        <code>approval_request</code> object with the tool name and proposed inputs.
      </p>
      <p>Submit your decision with:</p>

      <ApiPlayground
        method="POST"
        path="/runs/{runId}/approve"
        description="Submit an approval decision for a paused run."
        pathParams={[{ name: 'runId', label: 'Run ID', placeholder: 'run-id-in-approval_wait' }]}
        defaultBody={{ decision: 'approved', input: {} }}
      />

      <Callout type="tip">
        You can pass a modified <code>input</code> object when approving to override what the tool
        receives — useful for correcting a file path or adjusting an HTTP body before execution.
      </Callout>

      <ApiPlayground
        method="POST"
        path="/runs/{runId}/cancel"
        description="Cancel a run in pending, running, approval_wait, or session_wait state."
        pathParams={[{ name: 'runId', label: 'Run ID' }]}
      />

      <h2>Try It: Get Run Status</h2>
      <ApiPlayground
        method="GET"
        path="/runs/{runId}"
        description="Fetch the current state of a run including steps and any pending approval request."
        pathParams={[{ name: 'runId', label: 'Run ID', placeholder: 'paste-a-run-id' }]}
      />
    </DocPage>
  )
}
