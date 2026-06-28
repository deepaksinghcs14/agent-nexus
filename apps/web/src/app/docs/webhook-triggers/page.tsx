import { DocPage, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'Webhook Triggers — Docs' }

export default function WebhookTriggersDoc() {
  return (
    <DocPage
      title="Webhook Triggers"
      subtitle="Run agents and workflows automatically whenever an inbound HTTP event arrives — no polling, no scheduled jobs."
    >
      <h2>What are Webhook Triggers?</h2>
      <p>
        A webhook trigger is a persistent HTTP endpoint tied to a specific agent or workflow.
        When any external system POSTs to that URL — GitHub, Stripe, Slack, a form submission,
        or your own backend — Agent Nexus fires a run automatically and returns a{' '}
        <code>202 Accepted</code> response with the <code>run_id</code>.
      </p>
      <p>
        Triggered runs appear in the{' '}
        <a href="/runs">Runs</a> view exactly like any other run, with full step traces and output.
      </p>

      <h2>How it works</h2>
      <ol>
        <li>
          <strong>Create a trigger</strong> at <a href="/triggers">/triggers</a> — pick a target agent
          or workflow, optionally set a secret for HMAC verification, and save.
        </li>
        <li>
          <strong>Copy the webhook URL</strong> from the trigger list. It looks like:
          <pre><code>{`POST https://your-instance.example.com/webhook/<trigger-id>`}</code></pre>
        </li>
        <li>
          <strong>POST to the URL</strong> from your external system. The request body is passed
          through the <em>input template</em> (see below) to produce the agent input.
        </li>
        <li>
          <strong>Agent Nexus fires the run asynchronously.</strong> The HTTP response is immediate
          (<code>202</code>) — your caller is never blocked waiting for the agent to finish.
        </li>
        <li>
          <strong>Observe the run</strong> at <a href="/runs">/runs</a> or poll{' '}
          <code>GET /api/v1/runs/:id</code> with an API token.
        </li>
      </ol>

      <Callout type="info">
        Webhook endpoints do <strong>not</strong> require an API token — they are public URLs.
        Use the HMAC secret (below) to restrict who can fire them.
      </Callout>

      <h2>Input Template Reference</h2>
      <p>
        The <strong>input template</strong> is a{' '}
        <a href="https://pkg.go.dev/text/template" target="_blank" rel="noreferrer">Go text/template</a>{' '}
        evaluated against the inbound request. The result becomes the agent&apos;s input string.
        The default template <code>{'{{.RawBody}}'}</code> passes the full JSON body verbatim.
      </p>

      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Variable</th><th>Type</th><th>Description</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><code>{'{{.RawBody}}'}</code></td>
            <td>string</td>
            <td>Full request body as a raw string (default — works for any content type)</td>
          </tr>
          <tr>
            <td><code>{'{{.Body.field}}'}</code></td>
            <td>any</td>
            <td>Parsed JSON body field — e.g. <code>{'{{.Body.message}}'}</code> extracts the <code>message</code> key</td>
          </tr>
          <tr>
            <td><code>{'{{.Headers.X-Custom}}'}</code></td>
            <td>string</td>
            <td>First value of the named request header</td>
          </tr>
          <tr>
            <td><code>{'{{.Query.param}}'}</code></td>
            <td>string</td>
            <td>URL query string parameter — e.g. <code>{'{{.Query.repo}}'}</code></td>
          </tr>
        </tbody>
      </table>
      </div>

      <h3>Example templates</h3>
      <pre><code>{`# Pass the entire JSON body as the agent input (default):
{{.RawBody}}

# Extract a specific field — e.g. a Slack event payload:
{{.Body.event.text}}

# Compose a human-readable prompt from multiple fields:
New PR opened: {{.Body.pull_request.title}} by {{.Body.sender.login}}
Description: {{.Body.pull_request.body}}`}</code></pre>

      <h2>Securing with a Secret</h2>
      <p>
        When you set a <strong>secret</strong> on a trigger, Agent Nexus rejects any request that
        does not include a valid{' '}
        <code>X-Hub-Signature-256</code> header — the same scheme used by GitHub webhooks.
      </p>
      <p>
        The header value must be <code>sha256=&lt;hex&gt;</code>, where the hex is the HMAC-SHA256
        of the raw request body using your secret as the key.
      </p>
      <pre><code>{`# Sign a payload with openssl (for testing):
BODY='{"message":"hello"}'
SECRET="my-webhook-secret"
SIG=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" | sed 's/.* //')

curl -X POST https://your-instance.example.com/webhook/<trigger-id> \\
  -H "Content-Type: application/json" \\
  -H "X-Hub-Signature-256: sha256=$SIG" \\
  -d "$BODY"`}</code></pre>

      <Callout type="tip">
        GitHub, Stripe, and most major webhook providers use exactly this scheme and can set the
        secret automatically — just paste the same secret you set in Agent Nexus into their webhook
        configuration.
      </Callout>

      <h2>Response Format</h2>
      <p>
        A successful request returns <code>202 Accepted</code> immediately. The run executes
        asynchronously in the background.
      </p>
      <pre><code>{`HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "run_id": "550e8400-e29b-41d4-a716-446655440000",
  "conversation_id": "a2fb4c8d-...",
  "status": "running"
}`}</code></pre>

      <h2>Error Responses</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Status</th><th>Condition</th></tr>
        </thead>
        <tbody>
          <tr><td><code>404 Not Found</code></td><td>Trigger ID not found or trigger is inactive</td></tr>
          <tr><td><code>401 Unauthorized</code></td><td>Secret is set but signature header is missing or invalid</td></tr>
          <tr><td><code>400 Bad Request</code></td><td>Request body unreadable, or template renders to an empty string</td></tr>
          <tr><td><code>500 Internal Server Error</code></td><td>Failed to create the run record or start execution</td></tr>
        </tbody>
      </table>
      </div>

      <h2>Management API</h2>
      <p>
        All trigger CRUD operations require an{' '}
        <a href="/docs/api-tokens">API token</a> in the <code>Authorization</code> header.
      </p>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Method</th><th>Path</th><th>Description</th></tr>
        </thead>
        <tbody>
          <tr><td><code>GET</code></td><td><code>/api/v1/webhook-triggers</code></td><td>List all triggers in the workspace</td></tr>
          <tr><td><code>POST</code></td><td><code>/api/v1/webhook-triggers</code></td><td>Create a new trigger</td></tr>
          <tr><td><code>GET</code></td><td><code>/api/v1/webhook-triggers/:id</code></td><td>Get a trigger by ID</td></tr>
          <tr><td><code>PUT</code></td><td><code>/api/v1/webhook-triggers/:id</code></td><td>Update a trigger</td></tr>
          <tr><td><code>DELETE</code></td><td><code>/api/v1/webhook-triggers/:id</code></td><td>Delete a trigger</td></tr>
          <tr><td><code>POST</code></td><td><code>/webhook/:id</code></td><td>Inbound trigger endpoint (public, no auth)</td></tr>
        </tbody>
      </table>
      </div>

      <h3>Create trigger request body</h3>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Field</th><th>Type</th><th>Required</th><th>Description</th></tr>
        </thead>
        <tbody>
          <tr><td><code>name</code></td><td>string</td><td>Yes</td><td>Human-readable name</td></tr>
          <tr><td><code>description</code></td><td>string</td><td>No</td><td>Optional description</td></tr>
          <tr><td><code>target_type</code></td><td><code>&quot;agent&quot; | &quot;workflow&quot;</code></td><td>Yes</td><td>Whether to run an agent or workflow</td></tr>
          <tr><td><code>target_id</code></td><td>UUID</td><td>Yes</td><td>ID of the agent or workflow to run</td></tr>
          <tr><td><code>input_template</code></td><td>string</td><td>No</td><td>Go text/template for the agent input (default: <code>{'{{.RawBody}}'}</code>)</td></tr>
          <tr><td><code>secret</code></td><td>string</td><td>No</td><td>HMAC-SHA256 shared secret for signature verification</td></tr>
          <tr><td><code>is_active</code></td><td>boolean</td><td>No</td><td>Whether the trigger accepts requests (default: <code>true</code>)</td></tr>
        </tbody>
      </table>
      </div>

      <Callout type="info">
        After a run is created, track it with{' '}
        <a href="/docs/run-states">Run States</a> or stream its output with{' '}
        <a href="/docs/sse-events">SSE Events</a>.
      </Callout>
    </DocPage>
  )
}
