import Link from 'next/link'
import { DocPage, Callout, Badge } from '@/components/docs/DocPage'

export const metadata = { title: 'What is a Workflow — Docs' }

export default function WhatIsAnAgentGroupDoc() {
  return (
    <DocPage
      title="What is a Workflow?"
      subtitle="Workflows let you chain multiple agents together so each one handles a specific part of a complex task."
    >
      <p>
        A single agent is great for focused tasks. But some workflows are too broad for one agent
        to handle well — writing code, then reviewing it, then writing tests requires different
        expertise and different tool access. <strong>Workflows</strong> solve this by running
        a sequence of specialised agents, each passing its output to the next.
      </p>

      <h2>Pipeline Mode</h2>
      <p>
        In <strong>pipeline mode</strong> (the default in v0.1), agents execute sequentially:
      </p>
      <pre><code>{`Input → Agent A → Agent B → Agent C → Final Output`}</code></pre>
      <p>
        Each agent receives the original input <em>plus</em> the output of all previous agents as
        additional context. The final output of the last agent becomes the workflow&apos;s output.
      </p>

      <Callout type="info">
        Supervisor mode (where one agent decides which agents to call and in what order) is coming
        in v0.2. For now, pipeline is the only mode available.
      </Callout>

      <h2>When to Use a Workflow</h2>
      <ul>
        <li><strong>Multi-step reasoning</strong> — research → analyse → summarise, each step with a fresh agent context window.</li>
        <li><strong>Separation of concerns</strong> — one agent with filesystem access writes code; another (with no tools) reviews it for safety.</li>
        <li><strong>Quality gate</strong> — a critic agent at the end of the pipeline flags issues before the result reaches the user.</li>
        <li><strong>Different models per step</strong> — use a fast cheap model for extraction, a powerful model for synthesis.</li>
      </ul>

      <h2>Creating a Workflow</h2>
      <p>
        Go to <Link href="/workflows/new">Workflows → New Workflow</Link>. You configure:
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Field</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td><strong>Name</strong></td><td>Identifies the workflow in the UI and API</td></tr>
          <tr><td><strong>Description</strong></td><td>Explains what the workflow does — shown in the docs playground</td></tr>
          <tr><td><strong>Mode</strong></td><td><Badge label="pipeline" color="blue" /> Sequential execution</td></tr>
          <tr><td><strong>Agents</strong></td><td>Ordered list of existing agents. Drag to reorder.</td></tr>
        </tbody>
      </table>
      </div>

      <Callout type="tip">
        Each agent in a workflow should be configured for its specific role. Give it a tight system prompt,
        only the tools it needs, and memory disabled if its context is fully provided by the previous step.
      </Callout>

      <h2>How the Pipeline Runs</h2>
      <p>
        When you invoke a workflow (via the playground or API), the runtime:
      </p>
      <ol>
        <li>Creates a <strong>workflow run record</strong> with a unique run ID.</li>
        <li>Starts <strong>Agent A</strong> with the user&apos;s input.</li>
        <li>When Agent A completes, its output becomes part of the prompt for <strong>Agent B</strong> (injected as a prior message).</li>
        <li>Repeats for each agent in order.</li>
        <li>Returns the final agent&apos;s output as the workflow&apos;s result.</li>
      </ol>

      <h2>Workflow Runs vs. Agent Runs</h2>
      <div className="table-scroll">
      <table>
        <thead><tr><th></th><th>Single Agent Run</th><th>Workflow Run</th></tr></thead>
        <tbody>
          <tr><td><strong>Run ID</strong></td><td>Single run record</td><td>One run record per agent + one workflow record</td></tr>
          <tr><td><strong>Status</strong></td><td>Tracks one agent</td><td>Tracks across all agents in pipeline</td></tr>
          <tr><td><strong>Invoke endpoint</strong></td><td><code>/invoke/agents/:id</code></td><td><code>/invoke/workflows/:id</code></td></tr>
          <tr><td><strong>Output</strong></td><td>Final model response</td><td>Final agent in pipeline&apos;s response</td></tr>
          <tr><td><strong>Tracing</strong></td><td>Steps for one agent</td><td>Steps for all agents, nested by agent</td></tr>
        </tbody>
      </table>
      </div>

      <h2>Next Steps</h2>
      <ul>
        <li><Link href="/workflows/new">Create your first workflow</Link></li>
        <li><Link href="/docs/invoke-api">Invoke a workflow via the API</Link></li>
        <li><Link href="/docs/run-states">Understand workflow run states</Link></li>
      </ul>
    </DocPage>
  )
}
