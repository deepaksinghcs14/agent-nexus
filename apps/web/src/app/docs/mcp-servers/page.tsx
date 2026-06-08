import { DocPage, Badge, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'MCP Servers — Docs' }

export default function MCPServersDoc() {
  return (
    <DocPage
      title="MCP Servers"
      subtitle="Connect Model Context Protocol (MCP) servers to expose additional tools to your agents. Agent Nexus discovers available tools automatically on connection."
    >
      <h2>What is MCP?</h2>
      <p>
        The Model Context Protocol is an open standard that allows language models to call external tools
        through a standardised interface. You can connect any MCP-compatible server — from filesystem
        access to database queries to custom business logic — and its tools become available to all agents
        in your workspace.
      </p>
      <p>
        MCP tools go through the same <strong>risk check → approval gate → trace log</strong> path as
        native tools. They are never called directly, bypassing the executor.
      </p>

      <h2>Supported Transports</h2>
      <table>
        <thead><tr><th>Transport</th><th>Description</th><th>Availability</th></tr></thead>
        <tbody>
          <tr>
            <td><code>http</code></td>
            <td>HTTP+SSE transport. The default. Server URL must be reachable from the Agent Nexus instance.</td>
            <td><Badge label="v0.1" color="green" /></td>
          </tr>
          <tr>
            <td><code>stdio</code></td>
            <td>Stdio transport for servers running as local processes on the same host.</td>
            <td><Badge label="v0.2" color="gray" /></td>
          </tr>
        </tbody>
      </table>

      <h2>Connecting a Server</h2>
      <ol>
        <li>Go to <a href="/mcp-servers">MCP Servers</a> in the sidebar.</li>
        <li>Click <strong>Add Server</strong>.</li>
        <li>Enter a name and the server&apos;s HTTP URL.</li>
        <li>Click <strong>Save</strong>. Agent Nexus will connect and call <code>tools/list</code> to discover available tools.</li>
        <li>Assign discovered tools to agents via the <strong>Tools tab</strong> of the Agent Builder.</li>
      </ol>

      <Callout type="tip">
        Re-sync a server&apos;s tools at any time using the <strong>Sync</strong> button or via
        <code>POST /api/v1/mcp-servers/:id/sync</code>. This is useful after the server adds or removes tools.
      </Callout>

      <h2>Tool Risk Levels</h2>
      <p>
        Each MCP tool is assigned a risk level that controls whether tool calls require human approval.
        You can override the default risk level per tool in the tool registry.
      </p>
      <table>
        <thead><tr><th>Risk Level</th><th>Default Behaviour</th></tr></thead>
        <tbody>
          <tr>
            <td><Badge label="low" color="blue" /></td>
            <td>Read-only or safe operations. Executes automatically.</td>
          </tr>
          <tr>
            <td><Badge label="medium" color="amber" /></td>
            <td>Modifies data. May require approval depending on agent policy.</td>
          </tr>
          <tr>
            <td><Badge label="high" color="red" /></td>
            <td>Significant side effects. Approval recommended.</td>
          </tr>
          <tr>
            <td><Badge label="critical" color="red" /></td>
            <td>Destructive or irreversible. Always requires approval.</td>
          </tr>
        </tbody>
      </table>

      <h2>REST API</h2>
      <pre><code>{`# Add a server
POST /api/v1/mcp-servers
{"name": "My Tools", "url": "https://my-mcp-server.example.com", "transport": "http"}

# List tools discovered on a server
GET /api/v1/mcp-servers/:id/tools

# Re-discover tools (after server updates)
POST /api/v1/mcp-servers/:id/sync

# Remove a server
DELETE /api/v1/mcp-servers/:id`}</code></pre>

      <h2>Security</h2>
      <p>
        MCP server credentials are stored encrypted at rest using AES-256-GCM. Server-to-server
        communication happens from the Agent Nexus backend — the MCP server URL is never exposed
        to browser clients.
      </p>

      <h2>Next Steps</h2>
      <ul>
        <li><a href="/mcp-servers">Connect an MCP server</a></li>
        <li><a href="/docs/what-is-a-tool">Learn about tool risk levels and approval gates</a></li>
      </ul>
    </DocPage>
  )
}
