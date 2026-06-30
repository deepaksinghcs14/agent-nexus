import { DocPage, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'Claude Code Tool — Docs' }

export default function ClaudeCodeToolDoc() {
  return (
    <DocPage
      title="native_run_claude_code"
      subtitle="Give agents the ability to write and ship code — clone a GitHub repo, implement a task with Claude Code, push a branch, and open a pull request, all in one tool call."
    >
      <h2>Overview</h2>
      <p>
        <code>native_run_claude_code</code> bridges the gap between an AI conversation and a real code change.
        When an agent calls this tool it:
      </p>
      <ol>
        <li>Clones the target GitHub repository into a temporary working directory.</li>
        <li>Checks out a new branch (or an existing one for review-cycle iterations).</li>
        <li>Runs <code>claude --print --dangerously-skip-permissions</code> with the task as the prompt.</li>
        <li>Claude Code reads, edits, and tests files autonomously until the task is done, then commits.</li>
        <li>Pushes the branch to GitHub.</li>
        <li>Creates a pull request via the GitHub API and returns the PR URL.</li>
      </ol>

      <Callout type="info">
        Claude Code&apos;s own internal agent loop handles test-fix-retry cycles within a single call — you
        don&apos;t need to retry from the Nexus side. The tool only fails if Claude Code exits with an error
        code after exhausting its own retries.
      </Callout>

      <h2>Prerequisites</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Requirement</th><th>How to set up</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><strong>Claude Code CLI</strong></td>
            <td>Install via <code>npm install -g @anthropic-ai/claude-code</code> on the server running the API.</td>
          </tr>
          <tr>
            <td><strong>Claude Code auth</strong></td>
            <td>
              Run <code>claude login</code> once on the server to store OAuth credentials. Alternatively,
              set <code>ANTHROPIC_API_KEY</code> in your environment (the tool inherits the server&apos;s env).
            </td>
          </tr>
          <tr>
            <td><strong>GitHub access</strong></td>
            <td>
              A GitHub personal access token with <code>repo</code> scope, OR a connected GitHub
              Connector (preferred — credentials are looked up automatically from the connector config).
            </td>
          </tr>
          <tr>
            <td><strong>git CLI</strong></td>
            <td>Must be on the <code>$PATH</code> of the API server process.</td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>Input parameters</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Parameter</th><th>Type</th><th>Required</th><th>Description</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><code>task</code></td>
            <td>string</td>
            <td>Yes</td>
            <td>Full description of what to implement. Be specific — include files, functions, and expected behaviour.</td>
          </tr>
          <tr>
            <td><code>github_connector_id</code></td>
            <td>string (UUID)</td>
            <td>One of these</td>
            <td>UUID of a connected GitHub Connector. Repo URL and token are looked up automatically.</td>
          </tr>
          <tr>
            <td><code>repo_url</code></td>
            <td>string</td>
            <td>Or these two</td>
            <td>Full GitHub URL, e.g. <code>https://github.com/org/repo</code></td>
          </tr>
          <tr>
            <td><code>github_token</code></td>
            <td>string</td>
            <td></td>
            <td>GitHub PAT with <code>repo</code> write permission.</td>
          </tr>
          <tr>
            <td><code>branch_name</code></td>
            <td>string</td>
            <td>No</td>
            <td>Branch to create. Defaults to <code>ai/nexus-{'{'}timestamp{'}'}</code>.</td>
          </tr>
          <tr>
            <td><code>base_branch</code></td>
            <td>string</td>
            <td>No</td>
            <td>Branch to clone from. Defaults to <code>main</code>.</td>
          </tr>
          <tr>
            <td><code>pull_existing</code></td>
            <td>boolean</td>
            <td>No</td>
            <td>
              If <code>true</code>, checks out an existing branch and pulls instead of creating a new one.
              Use for PR review-cycle iterations.
            </td>
          </tr>
          <tr>
            <td><code>create_pr</code></td>
            <td>boolean</td>
            <td>No</td>
            <td>
              If <code>false</code>, pushes the branch but skips PR creation. The branch&apos;s commits will
              automatically appear in an existing open PR. Defaults to <code>true</code>.
            </td>
          </tr>
          <tr>
            <td><code>pr_title</code></td>
            <td>string</td>
            <td>No</td>
            <td>PR title. Defaults to the first line of <code>task</code>.</td>
          </tr>
          <tr>
            <td><code>pr_body</code></td>
            <td>string</td>
            <td>No</td>
            <td>PR description. Auto-generated if omitted.</td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>Output</h2>
      <pre><code>{`{
  "output":  "Claude Code's text output from the session",
  "branch":  "ai/nexus-1234567890",
  "repo":    "org/repo",
  "pr_url":  "https://github.com/org/repo/pull/42"
}`}</code></pre>
      <p>If <code>create_pr=false</code>, <code>pr_url</code> is omitted and a <code>note</code> explains that the branch was pushed.</p>

      <h2>Jira → PR pipeline</h2>
      <p>
        The recommended setup uses the <strong>Nexus Orchestrator</strong> agent to build and run the
        pipeline on demand — no manual workflow creation needed.
      </p>

      <h3>1. Connect GitHub repos as Connectors</h3>
      <p>
        Go to <a href="/connectors">/connectors</a> and create a GitHub Connector for each repository
        your agents may need to modify. The Connector indexes the codebase for RAG and stores the
        GitHub token securely. When an agent calls <code>native_retrieve_context</code>, results
        include a <code>[github:owner/repo]</code> tag so the agent knows which repo to work in.
      </p>

      <h3>2. Create Jira HTTP tools</h3>
      <p>Create three HTTP tools at <a href="/tools">/tools</a>:</p>
      <pre><code>{`GET  https://yourco.atlassian.net/rest/api/3/issue/{{issue_key}}
POST https://yourco.atlassian.net/rest/api/3/issue/{{issue_key}}/comment
POST https://yourco.atlassian.net/rest/api/3/issue/{{issue_key}}/transitions`}</code></pre>
      <p>
        Set <code>Authorization: Basic {'{'}base64(email:api_token){'}'}</code> as a header on each tool.
      </p>

      <h3>3. Create a webhook trigger</h3>
      <p>Go to <a href="/triggers">/triggers</a> and create a trigger pointed at your Nexus Orchestrator.
      Use this input template to extract the Jira issue fields:</p>
      <pre><code>{`Issue {{.Body.issue.key}}: {{.Body.issue.fields.summary}}
Project: {{.Body.issue.fields.project.key}}
Description: {{.Body.issue.fields.description}}`}</code></pre>
      <p>
        In Jira, add a webhook under <strong>Settings → System → Webhooks</strong> that fires on
        &ldquo;Issue updated&rdquo; events when <code>status = In Progress</code>.
      </p>

      <h3>4. Attach to Nexus Orchestrator</h3>
      <p>
        The Nexus Orchestrator agent (seeded automatically in every workspace) already has tools to
        create and run workflows on demand. Attach <code>native_run_claude_code</code> and your Jira
        HTTP tools to it. When a Jira issue moves to In Progress, the Orchestrator will:
      </p>
      <ol>
        <li>Create an ephemeral Analyst agent that fetches the full issue and identifies the repo via <code>native_retrieve_context</code>.</li>
        <li>Create an ephemeral Developer agent that calls <code>native_run_claude_code</code>.</li>
        <li>Create an ephemeral Reporter agent that posts the PR URL to Jira and transitions the ticket.</li>
        <li>Wire them into a supervisor workflow and run it via <code>native_run_workflow</code>.</li>
      </ol>

      <Callout type="tip">
        You can also attach the Nexus Orchestrator to a Gateway channel (HTTP or WhatsApp) so
        team members can trigger coding tasks by sending a message — &ldquo;Fix the login bug in the
        frontend repo&rdquo; — and get the PR URL back in the same conversation.
      </Callout>

      <h2>Review cycle loop</h2>
      <p>
        When a reviewer requests changes on GitHub, fire a second webhook trigger (GitHub{' '}
        <code>pull_request_review</code> event) and call <code>native_run_claude_code</code> with:
      </p>
      <pre><code>{`{
  "pull_existing": true,   // checkout the existing branch
  "create_pr":     false,  // PR already open — new commits appear automatically
  "task":          "Address review feedback:\\n{comments}"
}`}</code></pre>
      <p>
        GitHub PRs update automatically when new commits are pushed to the branch, so no second PR
        creation is needed.
      </p>

      <h2>Troubleshooting</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Error</th><th>Fix</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><code>'claude' CLI not found in PATH</code></td>
            <td>Run <code>npm install -g @anthropic-ai/claude-code</code> on the API server and ensure the install location is in <code>$PATH</code>.</td>
          </tr>
          <tr>
            <td><code>claude exited with error: not authenticated</code></td>
            <td>Run <code>claude login</code> on the server, or set <code>ANTHROPIC_API_KEY</code> in the API environment.</td>
          </tr>
          <tr>
            <td><code>git clone: authentication failed</code></td>
            <td>Check that the GitHub token has <code>repo</code> scope and that the connector or token is for the correct account.</td>
          </tr>
          <tr>
            <td><code>context deadline exceeded</code></td>
            <td>The task took longer than 10 minutes. Break it into smaller tasks, or increase the agent&apos;s tool timeout override.</td>
          </tr>
          <tr>
            <td><code>pull request already exists</code></td>
            <td>A PR for this branch is already open. Use <code>pull_existing=true, create_pr=false</code> to push new commits to the existing PR.</td>
          </tr>
        </tbody>
      </table>
      </div>
    </DocPage>
  )
}
