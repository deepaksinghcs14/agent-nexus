import { DocPage, Callout, Badge } from '@/components/docs/DocPage'

export const metadata = { title: 'Workflows & Node Types — Docs' }

export default function WhatIsAnAgentGroupDoc() {
  return (
    <DocPage
      title="Workflows & Node Types"
      subtitle="Build complex multi-agent pipelines using a visual graph of typed nodes — each with distinct execution semantics."
    >
      <p>
        A <strong>Workflow</strong> (formerly Agent Group) is a directed graph of typed nodes. When you
        run a workflow, Agent Nexus walks the graph from the{' '}
        <code>start</code> node to the <code>end</code> node, executing each node&apos;s logic in order.
        Edges control routing — some carry labels (<code>yes</code>, <code>no</code>, <code>loop</code>,{' '}
        <code>delegate</code>) that change how the runtime routes execution.
      </p>

      <Callout type="info">
        Two workflow modes exist: <strong>Pipeline</strong> (the default, sequential BFS traversal)
        and <strong>Supervisor</strong> (a coordinator agent calls team agents as tools). Both use the
        same visual builder — the mode determines how a Supervisor node executes.
      </Callout>

      <h2>Node Types</h2>

      {/* START */}
      <h3>🟢 Start</h3>
      <p>
        The mandatory entry point. Every workflow must have exactly one Start node. The user&apos;s
        input is passed as <code>lastOutput</code> into the Start node, which immediately forwards
        it to all connected downstream nodes.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Config</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td>None</td><td>No configuration required</td></tr>
        </tbody>
      </table>
      </div>
      <p><strong>Connections:</strong> One outgoing edge to the first processing node.</p>

      {/* END */}
      <h3>🔴 End</h3>
      <p>
        Terminal node — captures the final output of the workflow run. The runtime reads the output
        of the node that fed into <code>end</code> and stores it as the group run&apos;s result.
        Every workflow must reach an End node.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Config</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td>None</td><td>No configuration required</td></tr>
        </tbody>
      </table>
      </div>

      {/* AGENT */}
      <h3>🟣 Agent</h3>
      <p>
        Runs a single configured AI agent. The node receives the current <code>lastOutput</code>{' '}
        (combined with the original user input for context) as its input prompt, calls the agent&apos;s
        LLM with its full tool set and memory, and passes the agent&apos;s response to the next node.
      </p>
      <p>
        Each Agent node creates its own sub-conversation and sub-run record, so you can trace
        exactly what each agent produced in the Runs page.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Config</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td><strong>Agent</strong></td><td>Select which agent runs at this node. The agent&apos;s provider, model, tools, and memory settings all apply.</td></tr>
          <tr><td><strong>Label</strong></td><td>Display name shown on the canvas node.</td></tr>
        </tbody>
      </table>
      </div>
      <Callout type="tip">
        Give each agent a tight system prompt scoped to its specific role. Avoid general-purpose
        agents in pipelines — specialist agents produce better outputs at lower cost.
      </Callout>

      {/* SUPERVISOR */}
      <h3>👑 Supervisor</h3>
      <p>
        The most powerful node type. A Supervisor runs an assigned coordinator agent in an{' '}
        <strong>agentic delegation loop</strong>: team agents connected via dashed{' '}
        <Badge label="delegate" color="amber" /> edges are exposed to the supervisor&apos;s LLM as{' '}
        <strong>callable tools</strong>. The LLM decides when to call each agent, what task to give it,
        and how to synthesise the results — without being told a fixed sequence.
      </p>
      <pre><code>{`Supervisor LLM
  ├── calls data_analyst("Extract key metrics from the research")
  ├── calls fact_verifier("Verify the top 5 claims")
  ├── calls report_writer("Write a 500-word synthesis")
  └── produces final answer`}</code></pre>
      <p>
        Each delegation creates a full sub-run for the team agent, visible in the Runs page.
        The supervisor loops until it produces a final answer with no more tool calls, or until
        <code>max_steps</code> is reached.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Config</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td><strong>Supervisor Agent</strong></td><td>The coordinator — must be a capable model (Sonnet or Opus recommended). Its system prompt should explain the team&apos;s capabilities and when to delegate.</td></tr>
          <tr><td><strong>Label</strong></td><td>Display name on the canvas.</td></tr>
        </tbody>
      </table>
      </div>
      <p><strong>Edges from a Supervisor node:</strong></p>
      <ul>
        <li>
          <Badge label="delegate" color="amber" /> edges (dashed, amber) → <strong>team agent nodes</strong>.
          These agents become tool definitions for the supervisor&apos;s LLM. Draw one delegate edge per
          team member.
        </li>
        <li>
          <strong>Normal edges</strong> → the next sequential node (e.g. a Loop or End node). This is
          where the supervisor&apos;s synthesised output goes after it finishes delegating.
        </li>
      </ul>
      <Callout type="info">
        Team agents (connected via delegate edges) are skipped in BFS traversal — they only run when
        the supervisor explicitly calls them as tools. They never execute sequentially on their own.
      </Callout>

      {/* CONDITION */}
      <h3>🟡 Condition</h3>
      <p>
        Routes execution based on whether the current output matches an expression. Draw a{' '}
        <Badge label="yes" color="green" /> edge for the branch to take when the condition is true,
        and a <Badge label="no" color="red" /> edge for the false branch.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Config</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td><strong>Expression</strong></td><td>Evaluated against the current output text (case-insensitive)</td></tr>
        </tbody>
      </table>
      </div>
      <p><strong>Supported expression syntax:</strong></p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Expression</th><th>Matches when output…</th></tr></thead>
        <tbody>
          <tr><td><code>contains:approved</code></td><td>Contains the word &quot;approved&quot;</td></tr>
          <tr><td><code>not_contains:error</code></td><td>Does not contain &quot;error&quot;</td></tr>
          <tr><td><code>startswith:yes</code></td><td>Starts with &quot;yes&quot;</td></tr>
          <tr><td><code>endswith:done</code></td><td>Ends with &quot;done&quot;</td></tr>
          <tr><td><code>*</code> or empty</td><td>Always true (wildcard / default)</td></tr>
        </tbody>
      </table>
      </div>
      <Callout type="tip">
        Design your agent prompts to output a known keyword as the last word (e.g. <code>APPROVED</code>,{' '}
        <code>ESCALATE</code>, <code>QUALITY_PASS</code>). This makes condition routing reliable and
        avoids fragile substring matching inside long outputs.
      </Callout>

      {/* PARALLEL */}
      <h3>⫼ Parallel</h3>
      <p>
        Fan-out node. All outgoing edges fire simultaneously as independent goroutines — the connected
        nodes run concurrently. Use a <strong>Join</strong> node to collect their outputs once all
        branches complete.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Config</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td>None</td><td>Draw up to 3 outgoing edges (one per source handle)</td></tr>
        </tbody>
      </table>
      </div>
      <p>
        Each branch receives the same <code>lastOutput</code> as input. The Parallel node has
        three source handles (top 25%, middle 50%, bottom 75%) — drag from each to create separate
        branches.
      </p>
      <Callout type="info">
        Parallel branches run in separate goroutines so they do not share state. Their outputs are
        independent until merged by a Join node.
      </Callout>

      {/* JOIN */}
      <h3>⇥ Join</h3>
      <p>
        Barrier node that waits for all incoming parallel branches to complete, then concatenates their
        outputs (separated by <code>---</code>) and forwards the combined result to the next node.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Config</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td>None</td><td>Up to 3 target handles (top, middle, bottom). Connect each branch to a separate handle.</td></tr>
        </tbody>
      </table>
      </div>

      {/* LOOP */}
      <h3>↩ Loop</h3>
      <p>
        Retry / iteration node. On each pass it evaluates an exit condition against the current output.
        If the condition is met (or the max iteration count is reached), execution exits via the{' '}
        <strong>exit →</strong> handle. Otherwise it re-queues the node connected to the{' '}
        <strong>↩ cont</strong> handle for another iteration.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Config</th><th>Description</th></tr></thead>
        <tbody>
          <tr><td><strong>Exit Condition</strong></td><td>Same expression syntax as Condition node. The loop exits when this is true.</td></tr>
          <tr><td><strong>Max Iterations</strong></td><td>Hard cap on retries (default 5). Prevents infinite loops.</td></tr>
        </tbody>
      </table>
      </div>
      <p><strong>Drawing the loop edges:</strong></p>
      <ul>
        <li>Drag from the <strong>↩ cont</strong> handle (left) to the node to re-run (e.g. an Agent or Supervisor node). The edge is auto-labeled <code>loop</code>.</li>
        <li>Drag from the <strong>exit →</strong> handle (right) to the node that follows when done. The edge is auto-labeled <code>exit</code>.</li>
      </ul>
      <Callout type="tip">
        Combine Loop with a Supervisor: the supervisor produces output with a quality marker (e.g.{' '}
        <code>QUALITY_PASS</code>), and the loop exits only when that marker appears. This creates a
        self-improving research loop without manual retries.
      </Callout>

      <h2>Edge Types</h2>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Label</th><th>Color</th><th>When to use</th></tr></thead>
        <tbody>
          <tr><td><em>(none)</em></td><td>Purple</td><td>Default sequential flow between any two nodes</td></tr>
          <tr><td><code>yes</code></td><td>Green</td><td>True branch from a Condition node</td></tr>
          <tr><td><code>no</code></td><td>Red</td><td>False branch from a Condition node</td></tr>
          <tr><td><code>loop</code></td><td>Purple</td><td>Loop-back edge from Loop node ↩ cont handle</td></tr>
          <tr><td><code>exit</code></td><td>Purple</td><td>Forward edge from Loop node exit → handle</td></tr>
          <tr><td><code>delegate</code></td><td>Amber (dashed)</td><td>Supervisor → team agent. Makes the agent a callable tool.</td></tr>
        </tbody>
      </table>
      </div>

      <h2>Execution Model</h2>
      <p>
        The runtime uses a BFS (breadth-first) walk starting from the Start node. At each step:
      </p>
      <ol>
        <li>Dequeue the current node from the BFS queue.</li>
        <li>Execute the node&apos;s logic (agent call, condition check, fan-out, etc.).</li>
        <li>Enqueue successor nodes based on the node&apos;s logic and edge labels.</li>
        <li>Emit SSE <code>node_started</code> / <code>node_completed</code> events so the canvas updates in real time.</li>
      </ol>
      <p>
        A global circuit breaker stops execution after 100 total node visits to prevent runaway graphs.
      </p>

      <h2>Example Workflow Patterns</h2>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Pattern</th><th>Nodes used</th><th>Use case</th></tr></thead>
        <tbody>
          <tr><td>Sequential pipeline</td><td>Start → Agent → Agent → End</td><td>Research → summarise</td></tr>
          <tr><td>Quality gate</td><td>… → Condition → (yes) End / (no) Agent → End</td><td>Approve or revise</td></tr>
          <tr><td>Retry loop</td><td>Start → Agent → Loop → (exit) End / (loop) → Agent</td><td>Retry until quality threshold</td></tr>
          <tr><td>Parallel analysis</td><td>… → Parallel → [A, B, C] → Join → Agent → End</td><td>Multi-source ingestion</td></tr>
          <tr><td>Supervisor team</td><td>… → Supervisor → (delegate) [A, B, C] + (normal) End</td><td>Coordinator with specialists</td></tr>
          <tr><td>Full intelligence pipeline</td><td>All node types</td><td>Enterprise research with quality loop</td></tr>
        </tbody>
      </table>
      </div>

      <h2>Tips for Building Effective Workflows</h2>
      <ul>
        <li><strong>One concern per node</strong> — tightly scoped agents produce better outputs than a single general-purpose agent.</li>
        <li><strong>Use Supervisor for dynamic ordering</strong> — when the sequence of steps depends on the content (e.g. call fact-checker only if needed), use a Supervisor rather than a fixed pipeline.</li>
        <li><strong>Design for known keywords</strong> — condition and loop nodes match against text output. Build your agent prompts to end with a predictable keyword so routing is deterministic.</li>
        <li><strong>Parallel + Join for independent tasks</strong> — tasks that don&apos;t depend on each other (web search + domain knowledge) should run in parallel to reduce total latency.</li>
        <li><strong>Cap your loops</strong> — always set a Max Iterations limit. An LLM that never outputs your exit keyword would loop forever without it.</li>
        <li><strong>Use the template gallery</strong> — the Enterprise Intelligence Pipeline template demonstrates every node type working together. Load it, study the structure, then customise.</li>
      </ul>

      <h2>Next Steps</h2>
      <ul>
        <li><a href="/workflows/new">Create a workflow</a></li>
        <li><a href="/docs/invoke-api">Invoke a workflow via the API</a></li>
        <li><a href="/docs/run-states">Understand run states and trace steps</a></li>
        <li><a href="/docs/sse-events">SSE events reference</a></li>
      </ul>
    </DocPage>
  )
}
