import { DocPage, Callout, Badge } from '@/components/docs/DocPage'

export const metadata = { title: 'What is a Tool — Docs' }

export default function WhatIsAToolDoc() {
  return (
    <DocPage
      title="What is a Tool?"
      subtitle="Tools give agents the ability to take action — read files, call APIs, search the web — beyond generating text."
    >
      <p>
        By default, an LLM can only produce text. A <strong>tool</strong> extends an agent with the
        ability to perform real-world actions: read a file, make an HTTP request, search the web.
        Tools are defined in the registry and enabled per-agent.
      </p>
      <p>
        When the model decides a tool is needed, the runtime pauses, executes the tool, feeds the
        result back as a message, and the model continues. This loop repeats until the model produces
        a final answer without calling any more tools.
      </p>

      <h2>Native Tools</h2>
      <p>Agent Nexus ships these built-in tools:</p>
      <table>
        <thead><tr><th>Tool</th><th>Risk</th><th>What it does</th></tr></thead>
        <tbody>
          <tr>
            <td><code>read_file</code></td>
            <td><Badge label="low" color="blue" /></td>
            <td>Read file contents from the agent&apos;s allowed storage path</td>
          </tr>
          <tr>
            <td><code>write_file</code></td>
            <td><Badge label="high" color="red" /></td>
            <td>Write or overwrite a file. Requires approval by default.</td>
          </tr>
          <tr>
            <td><code>web_search</code></td>
            <td><Badge label="medium" color="amber" /></td>
            <td>Query a search engine and return a list of results with snippets</td>
          </tr>
          <tr>
            <td><code>http_request</code></td>
            <td><Badge label="high" color="red" /></td>
            <td>Make an outbound HTTP request to any URL. Requires approval by default.</td>
          </tr>
        </tbody>
      </table>

      <h2>Risk Levels and Approval Gates</h2>
      <p>
        Every tool has a <strong>risk level</strong> that determines whether a human must approve the
        tool call before it executes. This lets you give agents powerful capabilities while keeping
        destructive or external actions under control.
      </p>
      <table>
        <thead><tr><th>Risk Level</th><th>Default Behaviour</th><th>Example Tools</th></tr></thead>
        <tbody>
          <tr>
            <td><Badge label="low" color="blue" /></td>
            <td>Auto-approved, executes immediately</td>
            <td>read_file, list_directory</td>
          </tr>
          <tr>
            <td><Badge label="medium" color="amber" /></td>
            <td>Auto-approved by default; configurable per agent</td>
            <td>web_search, query</td>
          </tr>
          <tr>
            <td><Badge label="high" color="red" /></td>
            <td>Requires human approval before executing</td>
            <td>write_file, http_request</td>
          </tr>
          <tr>
            <td><Badge label="critical" color="red" /></td>
            <td>Always requires approval, no override</td>
            <td>delete operations, shell execution (blocked in v0.1)</td>
          </tr>
        </tbody>
      </table>

      <Callout type="warning">
        Shell execution (<code>bash</code>, <code>exec</code>) is globally blocked and never exposed
        through the tool registry. Agent Nexus intentionally has no arbitrary code execution.
      </Callout>

      <h2>How Approval Works</h2>
      <p>
        When a tool requiring approval is called:
      </p>
      <ol>
        <li>The run status changes to <code>approval_wait</code>.</li>
        <li>An SSE event <code>approval_required</code> is emitted with the tool name and proposed input.</li>
        <li>In the playground, a UI card appears asking you to approve or reject.</li>
        <li>Via the API, poll <code>GET /runs/:id</code> — it returns an <code>approval_request</code> object.</li>
        <li>Submit your decision via <code>POST /runs/:id/approve</code>.</li>
        <li>The run resumes with the result.</li>
      </ol>

      <Callout type="tip">
        You can override the tool input when approving — useful for correcting a proposed file path or
        adjusting an HTTP body before execution.
      </Callout>

      <h2>MCP Tools</h2>
      <p>
        Beyond native tools, you can connect <a href="/docs/mcp-servers">MCP servers</a> to give agents
        access to tools from any MCP-compatible service. MCP tools go through the same risk check →
        approval gate → trace log path as native tools — they are never called directly.
      </p>

      <h2>Enabling Tools on an Agent</h2>
      <p>
        On the <strong>Tools</strong> tab of the Agent Builder, you see all tools in the registry.
        Toggle each one on or off for that specific agent. The risk level and approval badge are shown
        next to each toggle so you know exactly what you&apos;re enabling.
      </p>

      <h2>Next Steps</h2>
      <ul>
        <li><a href="/tools">View the tool registry</a></li>
        <li><a href="/docs/mcp-servers">Connect an MCP server</a></li>
        <li><a href="/docs/run-states">Understand approval_wait and how to respond</a></li>
      </ul>
    </DocPage>
  )
}
