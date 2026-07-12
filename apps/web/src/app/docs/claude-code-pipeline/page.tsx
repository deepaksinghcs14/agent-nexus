import { DocPage, Badge, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'Claude Code Pipeline — Docs' }

export default function ClaudeCodePipelineDoc() {
  return (
    <DocPage
      title="Autonomous Jira → PR Pipeline"
      subtitle="A Jira ticket labeled for automation is driven to reviewed pull requests: repo selection over your repo catalog, headless Claude Code sessions in the runner service, an automated review pass, PR creation, and Jira updates."
    >
      <Callout type="info">
        The pipeline&apos;s controls are split across two pages: <strong>Settings → Claude Code</strong> holds
        credentials only (Claude account, GitHub token, Jira), and the <strong>Claude Code</strong> page
        (Build section of the sidebar) holds everything else — the repo allowlist, the readiness
        checklist, and recent session activity.
      </Callout>

      <h2>How a ticket becomes a PR</h2>
      <pre><code>{`Jira webhook ─▶ Orchestrator agent run
  (label filter)   │ 1. read ticket (Atlassian MCP, optional)
                   │ 2. search the repo catalog → pick target repo(s)
                   │ 3. launch a coding session ─▶ runner service
                   │      run parks durably (session_wait)   │ clone (workspace GitHub token)
                   │ ◀── completion callback ────────────────┘ claude -p …, push nexus/<ticket>
                   │ 4. branch diff → Code Review Agent
                   │ 5. open pull request
                   └ 6. Jira comment / transition`}</code></pre>

      <h2>The three system agents</h2>
      <p>
        Every workspace is seeded with three protected (non-deletable, fully editable) agents:
        the <strong>Jira Pipeline Orchestrator</strong> (drives tickets end to end),
        the <strong>Code Review Agent</strong> (reviews branch diffs before PRs open), and
        the <strong>Docs Map Maintainer</strong> (keeps per-repo <code>llms.txt</code> maps fresh
        after merges). You never create pipeline agents — adjust their model or instructions
        from the Agents page if needed.
      </p>

      <h2>Repositories: indexed vs. writeable</h2>
      <p>These are deliberately different trust levels:</p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Level</th><th>How a repo gets there</th><th>What it allows</th></tr></thead>
        <tbody>
          <tr>
            <td><Badge label="indexed" color="gray" /></td>
            <td>Syncing a GitHub connector auto-adopts every synced repo into the catalog</td>
            <td>The orchestrator can <em>search</em> its docs to select repos — read-only</td>
          </tr>
          <tr>
            <td><Badge label="sessions on" color="green" /></td>
            <td>Flip the <em>enable sessions</em> toggle (Claude Code → Repositories tab), or onboard explicitly via <em>Add repository</em></td>
            <td>Coding sessions may clone it, write code on a <code>nexus/&lt;ticket&gt;</code> branch, push, and open PRs</td>
          </tr>
        </tbody>
      </table>
      </div>
      <Callout type="warning">
        <strong>The enabled set is the blast radius.</strong> A session can only ever target a
        repo with sessions enabled — the launch tool enforces this mechanically, listing the
        allowed repos in its error. Review the enabled set before switching the runner to real mode.
      </Callout>

      <h2>Credentials (Settings → Claude Code)</h2>
      <p>
        Both are per-workspace, AES-256-GCM encrypted, never returned by the API, and injected
        per session — the runner holds no static secrets.
      </p>
      <ul>
        <li>
          <strong>Claude account</strong> — run <code>claude setup-token</code> in your terminal
          (the familiar browser login), paste the <code>sk-ant-oat…</code> token. Coding sessions
          bill your Claude subscription. Fallback: <code>ANTHROPIC_API_KEY</code> on the runner.
        </li>
        <li>
          <strong>GitHub token</strong> — a PAT with access to the workspace&apos;s repositories;
          used for clone/push in sessions and by the PR/diff tools. A workspace token always
          takes precedence over any instance-level <code>GITHUB_TOKEN</code>.
        </li>
      </ul>

      <h2>Runner modes</h2>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Mode</th><th>Behaviour</th></tr></thead>
        <tbody>
          <tr>
            <td><Badge label="stub" color="amber" /></td>
            <td>Sessions are <strong>simulated</strong> — no clone, no code, fake branch names.
                For validating pipeline plumbing without credentials. The readiness panel and
                every session summary say so explicitly.</td>
          </tr>
          <tr>
            <td><Badge label="claude" color="green" /></td>
            <td>Real headless Claude Code sessions: clone → code → commit → push. Set
                <code> RUNNER_EXECUTOR=claude</code> on the runner service.</td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>Code quality &amp; repo lessons</h2>
      <p>
        Two mechanisms keep session output from degrading over time:
      </p>
      <ul>
        <li>
          <strong>Quality system prompt</strong> — every coding session runs with an enforced
          set of rules appended to its system prompt: write the shortest working diff, reuse
          the repo&apos;s existing helpers before writing new ones, no speculative abstractions,
          fix root causes rather than symptoms, match the surrounding code&apos;s idiom, and leave
          a minimal check for non-trivial logic.
        </li>
        <li>
          <strong>Repo lessons</strong> — when a review session completes, its findings
          (blocking issues and notes from the verdict JSON) are automatically distilled into a
          dated <em>lessons</em> log on the repo&apos;s catalog entry. Every future coding session
          targeting that repo gets these lessons injected into its task, so a mistake the
          reviewer caught once is warned against on every subsequent ticket.
        </li>
      </ul>
      <p>
        Lessons are plain text — open a repo&apos;s <strong>lessons</strong> pill on the
        Claude Code → Repositories tab to review, prune, or add house rules by hand (e.g.
        &quot;migrations must exist in both <code>infra/migrations</code> and the embedded SQL
        dir&quot;). The log is capped; the oldest entries fall off as new reviews land.
      </p>
      <Callout type="info">
        Lesson capture depends on the <strong>Code Review Agent</strong>&apos;s instructions keeping
        the JSON verdict contract. If you edit that agent, keep the{' '}
        <code>{`{"verdict", "blocking_issues", "non_blocking_notes"}`}</code> response shape —
        free-text verdicts still drive the pipeline, but nothing can be distilled into lessons.
      </Callout>

      <h2>Durable waits</h2>
      <p>
        A run waiting on a coding session (<Badge label="session_wait" color="blue" />) or a tool
        approval (<Badge label="approval_wait" color="purple" />) persists its loop state and
        survives API restarts — the runner&apos;s completion callback or the approval decision
        resumes it exactly where it parked. Runner crashes mid-session are journaled and
        reported as <code>crashed</code>, and a watchdog resumes any session run whose callback
        never arrives, so no ticket is ever silently stranded.
      </p>

      <h2>Setup checklist</h2>
      <p>The readiness strip at the top of the Claude Code page tracks all of this live:</p>
      <ol>
        <li>Runner reachable and in the mode you expect (stub vs claude)</li>
        <li>Claude account + GitHub token connected</li>
        <li>Repos onboarded and session-enabled</li>
        <li>Webhook triggers created — run <code>infra/scripts/setup_pipeline.sh</code>, then point
            a Jira webhook (issue created/updated) and a GitHub webhook (pull_request events) at
            the printed URLs</li>
        <li>Optional: connect Atlassian&apos;s hosted MCP server (MCP Servers → Connect (OAuth)).
            The orchestrator attaches the synced Jira tools it needs by itself at run time —
            without a connected server the pipeline works but skips Jira commenting</li>
      </ol>

      <h2>Testing from the playground</h2>
      <p>
        Open the playground with the <strong>Jira Pipeline Orchestrator</strong> and paste a
        ticket-shaped message — no webhook needed:
      </p>
      <pre><code>{`Ticket NEX-101: Add rate limiting to the webhook ingress endpoint.
Limit to 60 requests/minute per trigger; return 429 with a Retry-After header.`}</code></pre>
      <p>
        Watch the trace: catalog retrieval → session launch → <code>session_wait</code> →
        the session result → review → PR. In stub mode the result is clearly marked as simulated.
      </p>
    </DocPage>
  )
}
