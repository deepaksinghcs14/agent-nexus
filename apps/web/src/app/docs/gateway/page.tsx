import { DocPage, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'Nexus Gateway — Docs' }

export default function GatewayDoc() {
  return (
    <DocPage
      title="Nexus Gateway"
      subtitle="Connect agents to inbound messaging channels — WhatsApp, HTTP, and more — so they can receive and respond to real-world messages."
    >
      <h2>What is the Gateway?</h2>
      <p>
        The Nexus Gateway bridges external messaging channels and your agents. Instead of invoking
        an agent via the API or playground, messages arrive from users on WhatsApp (or any HTTP
        channel), the gateway dispatches a run, and the agent&apos;s reply is sent back automatically.
      </p>
      <p>
        Every gateway channel is linked to a default agent. You can override the agent per contact,
        giving different users different AI assistants on the same WhatsApp number.
      </p>

      <h2>Supported channel types</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Type</th><th>Transport</th><th>Notes</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><code>whatsapp</code></td>
            <td>WhatsApp Web adapter (Node.js)</td>
            <td>Requires <code>WHATSAPP_ADAPTER_URL</code> and the adapter service to be running</td>
          </tr>
          <tr>
            <td><code>http</code></td>
            <td>Generic HTTP POST</td>
            <td>Any caller can POST <code>{`{"input":"..."}`}</code> to <code>/gateway/http/{'{channelId}'}</code></td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>How it works — HTTP channel</h2>
      <p>
        An HTTP channel is the simplest way to connect any application to an agent. There is no
        pairing, no adapter service, and no authentication — just a plain HTTP POST.
      </p>
      <ol>
        <li>
          <strong>Create a channel</strong> at <a href="/gateway">/gateway</a>. Choose{' '}
          <em>HTTP</em>, pick a default agent, and save.
        </li>
        <li>
          <strong>Copy the webhook URL</strong> from the channel Overview tab. It looks like:
          <pre><code>{`POST https://your-api.example.com/gateway/http/<channelId>`}</code></pre>
        </li>
        <li>
          <strong>POST a message</strong> to the URL with a JSON body:
          <pre><code>{`{
  "input": "What is the weather today?",
  "session_id": "user-123"
}`}</code></pre>
          <code>input</code> is required. <code>session_id</code> is optional — the same value groups
          multiple requests into one conversation thread. Omit it to start a fresh conversation on
          every call.
        </li>
        <li>
          The gateway returns <strong>202 Accepted</strong> immediately with a run reference:
          <pre><code>{`{
  "run_id": "...",
  "session_id": "...",
  "conversation_id": "...",
  "status": "running"
}`}</code></pre>
          The agent run proceeds asynchronously. Poll <code>GET /api/v1/runs/{'{run_id}'}</code> or
          stream events at <code>GET /api/v1/runs/{'{run_id}'}/stream</code> to get the reply.
        </li>
      </ol>

      <Callout type="tip">
        Use the <strong>Send a test message</strong> panel on the channel Overview tab to try the
        webhook directly from the dashboard without writing any code.
      </Callout>

      <Callout type="info">
        The HTTP inbound endpoint has no built-in authentication. Keep the channel ID secret or
        place the API behind a reverse proxy with access controls if you need to restrict access.
      </Callout>

      <h2>How it works — WhatsApp</h2>
      <ol>
        <li>
          <strong>Create a channel</strong> at <a href="/gateway">/gateway</a>. Choose{' '}
          <em>WhatsApp</em>, pick a default agent, and save.
        </li>
        <li>
          <strong>Scan the QR code</strong> from the channel detail page to pair the WhatsApp account
          with the adapter. The adapter stores session credentials at{' '}
          <code>WHATSAPP_AUTH_ROOT</code> so the session survives restarts.
        </li>
        <li>
          <strong>Add contacts</strong> with their phone numbers and roles (see below). Only contacts
          are sent to the agent unless <em>Bot Mode</em> is on.
        </li>
        <li>
          <strong>Inbound messages</strong> arrive at the adapter, which forwards them to{' '}
          <code>POST /gateway/whatsapp/{'{channelId}'}</code>. The gateway matches the sender to a
          contact, dispatches an agent run, and sends the reply back via WhatsApp.
        </li>
      </ol>

      <Callout type="info">
        Messages sent <strong>by the WhatsApp account itself</strong> (from_me events) are
        automatically dropped unless <em>Self-Chat</em> is enabled. Owner commands (e.g.{' '}
        <code>stop Alice</code>, <code>approve CODE</code>) are still processed even when
        Self-Chat is off.
      </Callout>

      <h2>Channel configuration options</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Option</th><th>Description</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><strong>Auto-Reply</strong></td>
            <td>
              When enabled, every eligible inbound message triggers an agent run automatically.
              When disabled, the gateway receives messages but does not invoke the agent — useful
              for pausing the assistant without disconnecting the channel.
            </td>
          </tr>
          <tr>
            <td><strong>Self-Chat</strong></td>
            <td>
              When enabled, messages that the WhatsApp account sends <em>to itself</em> (self-chat)
              are processed as user messages and trigger agent runs. Useful for testing the agent
              from your own phone without adding yourself as a contact.
            </td>
          </tr>
          <tr>
            <td><strong>Bot Mode</strong></td>
            <td>
              When enabled, messages from any sender are forwarded to the agent, even if the sender
              is not in the contacts list. Equivalent to <em>DM Policy: open</em> set at the channel
              level via owner command.
            </td>
          </tr>
          <tr>
            <td><strong>Default Agent</strong></td>
            <td>
              The agent that handles inbound messages when no contact-level override is set. You can
              override the agent per contact to give different users different assistants on the same
              WhatsApp number.
            </td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>Contact roles</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Role</th><th>Behaviour</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><strong>owner</strong></td>
            <td>
              Can send owner commands (start / stop contacts, enable / disable the assistant, manage
              approvals). Messages from owners also trigger agent runs if <em>Auto-Reply</em> is on.
            </td>
          </tr>
          <tr>
            <td><strong>trusted</strong></td>
            <td>Messages trigger agent runs when <em>Auto-Reply</em> is on.</td>
          </tr>
          <tr>
            <td><strong>blocked</strong></td>
            <td>All messages are silently dropped.</td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>Owner commands</h2>
      <p>
        Any contact with the <strong>owner</strong> role can send WhatsApp messages to control the
        gateway without touching the dashboard:
      </p>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Command</th><th>Effect</th></tr>
        </thead>
        <tbody>
          <tr><td><code>start Alice</code></td><td>Enable auto-reply for a contact named Alice</td></tr>
          <tr><td><code>stop Alice</code></td><td>Disable auto-reply for Alice</td></tr>
          <tr><td><code>stop assistant</code></td><td>Disable the assistant for the entire channel</td></tr>
          <tr><td><code>start assistant</code></td><td>Re-enable the assistant</td></tr>
          <tr><td><code>enable bot mode</code></td><td>Auto-approve unknown senders</td></tr>
          <tr><td><code>disable bot mode</code></td><td>Require contacts to be in the list</td></tr>
          <tr><td><code>enable approvals</code></td><td>Turn on in-chat escalation approvals</td></tr>
          <tr><td><code>disable approvals</code></td><td>Turn off in-chat escalation approvals</td></tr>
          <tr><td><code>approve CODE</code></td><td>Approve a pending escalation</td></tr>
          <tr><td><code>reject CODE</code></td><td>Reject a pending escalation</td></tr>
          <tr><td><code>pending approvals</code></td><td>List outstanding escalation codes</td></tr>
        </tbody>
      </table>
      </div>

      <h2>DM policy</h2>
      <p>
        The <strong>DM Policy</strong> controls which senders can reach the agent when they are{' '}
        <em>not</em> in the contacts list:
      </p>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Policy</th><th>Behaviour</th></tr>
        </thead>
        <tbody>
          <tr><td><strong>pairing</strong> (default)</td><td>Unknown senders must be approved via the Pairing panel before the agent replies.</td></tr>
          <tr><td><strong>open</strong></td><td>Any sender can reach the agent without prior approval.</td></tr>
          <tr><td><strong>allowlist</strong></td><td>Only phone numbers / JIDs in the explicit allow list are accepted.</td></tr>
        </tbody>
      </table>
      </div>

      <h2>Escalation and approval</h2>
      <p>
        Agents can call the native <code>whatsapp_request_owner_approval</code> tool when an action
        is risky or ambiguous. This creates an escalation with a short approval code, notifies all
        owner contacts via WhatsApp, and pauses the run until an owner replies{' '}
        <code>approve CODE</code> or <code>reject CODE</code>.
      </p>
      <p>
        Escalations are visible in the <em>Escalations</em> tab of each channel and can also be
        resolved from the dashboard.
      </p>

      <Callout type="warning">
        The WhatsApp adapter is a separate Node.js service (<code>services/api/whatsapp-adapter/</code>).
        Set <code>WHATSAPP_ADAPTER_URL</code> to point at it. Without the adapter running, WhatsApp
        channels will show as disconnected.
      </Callout>

      <h2>Environment variables</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Variable</th><th>Description</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><code>WHATSAPP_ADAPTER_URL</code></td>
            <td>Base URL of the WhatsApp adapter service (default: <code>http://127.0.0.1:18901</code>)</td>
          </tr>
          <tr>
            <td><code>WHATSAPP_AUTH_ROOT</code></td>
            <td>Directory where adapter session credentials are persisted</td>
          </tr>
        </tbody>
      </table>
      </div>
    </DocPage>
  )
}
