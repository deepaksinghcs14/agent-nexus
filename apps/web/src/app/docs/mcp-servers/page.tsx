import { DocPage, Badge, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'MCP Servers — Docs' }

export default function MCPServersDoc() {
  return (
    <DocPage
      title="MCP Servers"
      subtitle="Connect Model Context Protocol servers to give agents access to external tools. Agent Nexus discovers and manages tools automatically."
    >
      <h2>What is MCP?</h2>
      <p>
        The Model Context Protocol is an open standard that allows language models to call external tools
        through a standardised interface. You can connect any MCP-compatible server — filesystem access,
        database queries, web search, custom business logic — and its tools become available to all agents
        in your workspace.
      </p>
      <p>
        MCP tools go through the same <strong>risk check → approval gate → trace log</strong> path as
        native tools. They are never called directly, bypassing the executor.
      </p>

      <h2>Supported Transports</h2>
      <table>
        <thead><tr><th>Transport</th><th>When to use</th><th>Status</th></tr></thead>
        <tbody>
          <tr>
            <td><code>http</code></td>
            <td>Connect to a running MCP server over HTTP. Supports direct JSON-RPC POST (Streamable HTTP) and SSE responses.</td>
            <td><Badge label="v0.1" color="green" /></td>
          </tr>
          <tr>
            <td><code>stdio</code></td>
            <td>Spawn a local MCP process on the same host as Agent Nexus. Agent Nexus starts the process and communicates via stdin/stdout.</td>
            <td><Badge label="v0.1" color="green" /></td>
          </tr>
        </tbody>
      </table>

      <h2>HTTP Transport</h2>
      <p>
        Agent Nexus sends JSON-RPC 2.0 messages via <code>POST</code> to the server URL.
        The server responds with JSON or an SSE stream — both are handled automatically.
      </p>
      <ol>
        <li>Go to <a href="/mcp-servers">MCP Servers</a> in the sidebar.</li>
        <li>Click <strong>Add server</strong> and choose the <strong>HTTP + SSE</strong> transport.</li>
        <li>Enter the server&apos;s base URL (e.g. <code>https://mcp.example.com</code>).</li>
        <li>Click <strong>Add server</strong>, then click <strong>Sync tools</strong> on the card to discover available tools.</li>
      </ol>

      <Callout type="tip">
        Agent Nexus runs the MCP <code>initialize</code> handshake automatically before every <code>tools/list</code>
        or <code>tools/call</code> request. You do not need to do anything special on the server side.
      </Callout>

      <h2>stdio Transport</h2>
      <p>
        For servers that run as local processes, use the <code>stdio</code> transport.
        Agent Nexus spawns the command, communicates over stdin/stdout using newline-delimited JSON-RPC,
        and kills the process when the call completes.
      </p>
      <ol>
        <li>Click <strong>Add server</strong> and choose the <strong>stdio</strong> transport.</li>
        <li>
          In the <strong>Command</strong> field, enter the full command to run the server, including
          any arguments. For example:
          <pre><code>{`npx @modelcontextprotocol/server-filesystem /home/user/data`}</code></pre>
        </li>
        <li>Click <strong>Add server</strong>, then <strong>Sync tools</strong> to discover tools.</li>
      </ol>

      <Callout type="warning">
        The command runs on the Agent Nexus host with the same privileges as the API process.
        Only connect stdio servers you trust. For Docker deployments, make sure the required
        runtime (e.g. <code>node</code>, <code>python</code>) is installed in the API container.
      </Callout>

      <h2>How Sync Works</h2>
      <p>
        When you click <strong>Sync tools</strong> (or call <code>POST /api/v1/mcp-servers/:id/sync</code>):
      </p>
      <ol>
        <li>Agent Nexus connects to the server and runs the <code>initialize</code> handshake.</li>
        <li>It calls <code>tools/list</code> to fetch the server&apos;s current tool catalogue.</li>
        <li>All previously discovered tools for that server are replaced with the new list.</li>
        <li>The server&apos;s status is updated to <code>connected</code> and <code>tools_synced_at</code> is stamped.</li>
      </ol>
      <p>
        Re-sync any time the server adds, removes, or changes tools.
      </p>

      <h2>Tool Risk Levels</h2>
      <p>
        Newly discovered MCP tools default to <Badge label="medium" color="amber" /> risk.
        You can override the risk level per tool in the tool registry.
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
      <pre><code>{`# Add an HTTP server
POST /api/v1/mcp-servers
{"name": "My Tools", "url": "https://mcp.example.com", "transport": "http"}

# Add a stdio server
POST /api/v1/mcp-servers
{"name": "Filesystem MCP", "url": "npx @modelcontextprotocol/server-filesystem /data", "transport": "stdio"}

# Sync tools (connects to server and updates the tool list)
POST /api/v1/mcp-servers/:id/sync

# List tools discovered on a server
GET /api/v1/mcp-servers/:id/tools

# Remove a server
DELETE /api/v1/mcp-servers/:id`}</code></pre>

      <h2>Security</h2>
      <ul>
        <li>MCP server config is stored encrypted at rest using AES-256-GCM.</li>
        <li>The server URL is never exposed to browser clients — all calls go through the Agent Nexus backend.</li>
        <li>stdio commands run as the API process user — apply OS-level restrictions as needed.</li>
      </ul>

      <h2>Next Steps</h2>
      <ul>
        <li><a href="/mcp-servers">Connect an MCP server</a></li>
        <li><a href="/docs/what-is-a-tool">Tool risk levels and approval gates</a></li>
      </ul>
    </DocPage>
  )
}
