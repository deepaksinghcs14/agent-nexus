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

      <h2>Contact roles</h2>
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

      <h2>Owner commands</h2>
      <p>
        Any contact with the <strong>owner</strong> role can send WhatsApp messages to control the
        gateway without touching the dashboard:
      </p>
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

      <h2>DM policy</h2>
      <p>
        The <strong>DM Policy</strong> controls which senders can reach the agent when they are{' '}
        <em>not</em> in the contacts list:
      </p>
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
    </DocPage>
  )
}
