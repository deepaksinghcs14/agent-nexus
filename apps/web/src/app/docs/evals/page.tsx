import { DocPage, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'Eval Framework — Docs' }

export default function EvalsDoc() {
  return (
    <DocPage
      title="Eval Framework"
      subtitle="Test your agents systematically — define cases, run them in parallel, grade with an LLM judge, and get AI-generated fix suggestions for every failure."
    >
      <h2>Overview</h2>
      <p>
        The eval framework lets you build a library of test cases for any agent, run them on demand
        or automatically after agent updates, and see exactly where the agent fails — along with
        actionable suggestions for fixing the prompt, adding a tool, or enabling a skill.
      </p>
      <p>
        Navigate to <a href="/evals">/evals</a> to manage eval suites.
      </p>

      <h2>Core concepts</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Term</th><th>Description</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><strong>Suite</strong></td>
            <td>
              A named collection of test cases for a single agent. Each suite has a grading mode
              and an optional auto-run flag.
            </td>
          </tr>
          <tr>
            <td><strong>Case</strong></td>
            <td>
              One test: an <code>input</code> message sent to the agent, an optional{' '}
              <code>expected_output</code>, and a <code>grading_criteria</code> description.
            </td>
          </tr>
          <tr>
            <td><strong>Run</strong></td>
            <td>
              An execution of all cases in a suite. Each run produces a per-case result with a
              pass/fail grade, score, latency, and judge reasoning. Runs are stored permanently so
              you can compare results over time.
            </td>
          </tr>
          <tr>
            <td><strong>Analysis</strong></td>
            <td>
              An LLM-generated summary of failure patterns and specific fix suggestions, produced
              automatically after any run that has failures.
            </td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>Grading modes</h2>
      <div className="table-scroll">
      <table>
        <thead>
          <tr><th>Mode</th><th>How it grades</th><th>When to use</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><code>llm_judge</code></td>
            <td>
              Calls the agent&apos;s own LLM at temperature 0.1 with the input, expected output,
              grading criteria, and actual response. Returns pass/fail, a score 0–1, and a one-sentence
              reasoning.
            </td>
            <td>
              Most cases. Handles varied phrasing, partial credit, and nuanced criteria that exact
              or substring matching can&apos;t capture.
            </td>
          </tr>
          <tr>
            <td><code>contains</code></td>
            <td>
              Case-insensitive substring check: does the actual output contain the expected output?
            </td>
            <td>
              Simple factual answers where the response must include a specific word, ID, or phrase.
            </td>
          </tr>
          <tr>
            <td><code>exact</code></td>
            <td>
              Case-insensitive equality after trimming whitespace.
            </td>
            <td>
              Classification tasks, yes/no questions, or any case where the output must be a
              specific fixed string.
            </td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>Parallel execution</h2>
      <p>
        Cases run up to <strong>5 at a time</strong> concurrently. Both the agent call and the LLM
        judge call are parallelised — wall-clock time is dominated by the slowest batch, not the
        sum of all cases. A 10-case suite typically completes in the time it takes to run 2 cases
        sequentially.
      </p>

      <Callout type="info">
        Each case gets its own fresh conversation so cases are fully isolated and cannot influence
        each other.
      </Callout>

      <h2>Creating a suite</h2>
      <ol>
        <li>Go to <a href="/evals">/evals</a> and click <strong>New Suite</strong>.</li>
        <li>Select the agent to test and choose a grading mode.</li>
        <li>
          Add cases manually, or click <strong>Generate Cases</strong> to have an LLM create
          realistic test inputs, expected outputs, and grading criteria automatically.
        </li>
        <li>Click <strong>Run</strong> to execute all cases.</li>
      </ol>

      <h2>AI case generation</h2>
      <p>
        Click <strong>Generate Cases</strong> in a suite to have an LLM write test cases based on
        the agent&apos;s name, description, system prompt, tools, skills, and connectors. The generator
        creates a mix of:
      </p>
      <ul>
        <li>Core happy-path cases — does the agent do its primary job?</li>
        <li>Edge cases — unusual inputs, boundary conditions</li>
        <li>Multi-step requests — cases that require tool use or memory</li>
        <li>Ambiguous inputs — cases where the agent must ask for clarification</li>
        <li>Error handling — cases where the agent should gracefully refuse or redirect</li>
      </ul>
      <p>
        You can specify the count (up to 20) and optionally use a different provider/model than the
        agent&apos;s own model for generation. Cases are saved immediately and can be edited before
        running.
      </p>

      <h2>AI run analysis</h2>
      <p>
        After any run that has one or more failures, an LLM automatically analyses all failed cases
        and produces an <strong>Analysis</strong> card on the run detail page. The analysis contains:
      </p>
      <ul>
        <li>
          <strong>Issues</strong> — 1–2 sentences identifying the root cause pattern (e.g. &ldquo;The
          agent consistently fails to call the search tool before answering factual questions&rdquo;).
        </li>
        <li>
          <strong>Fixes</strong> — 1–3 specific, actionable suggestions, each with a type:
          <ul>
            <li>
              <code>prompt</code> — exact text to append to the system prompt; an <strong>Apply</strong>{' '}
              button patches the agent in one click.
            </li>
            <li>
              <code>tool</code> — a tool the agent is missing and why it would help.
            </li>
            <li>
              <code>skill</code> — a skill to enable and why it addresses the failure.
            </li>
          </ul>
        </li>
      </ul>
      <p>
        Analysis runs automatically in the background. It also appears the moment the page loads
        if the run already completed. You can re-trigger it manually with the <strong>Retry
        analysis</strong> button.
      </p>

      <Callout type="tip">
        Apply a prompt fix, then immediately re-run the suite to see whether the score improves.
        This loop — run → analyse → fix → re-run — is the fastest way to iterate on agent quality.
      </Callout>

      <h2>Manual overrides</h2>
      <p>
        LLM judges are not perfect. Use the thumbs up / thumbs down buttons on any result card to
        mark false positives (agent actually failed but was graded as passed) or false negatives
        (agent succeeded but was graded as failed). Overrides are reflected in the effective
        pass rate immediately.
      </p>

      <h2>AI case fix</h2>
      <p>
        After overriding a result, click <strong>Fix case with AI</strong>. The LLM refines the
        <code>expected_output</code> and <code>grading_criteria</code> for that specific case based
        on your verdict — tightening criteria for false positives, relaxing them for false negatives.
        The fix is applied to the underlying case so future runs grade it correctly.
      </p>

      <h2>Export and import</h2>
      <p>
        Download all cases from a suite as a JSON file (<strong>Export cases</strong> button in the
        suite detail page). Import them into any suite on any instance. The format is:
      </p>
      <pre><code>{`[
  {
    "input": "What is the capital of France?",
    "expected_output": "Paris",
    "grading_criteria": "Response must state that Paris is the capital of France."
  }
]`}</code></pre>
      <p>
        Import accepts up to 200 cases per file. Existing cases in the target suite are not
        overwritten — imports always append.
      </p>

      <h2>Auto-run on agent save</h2>
      <p>
        Enable <strong>Auto-run</strong> on a suite (Suite Settings → toggle) to trigger a new eval
        run automatically every time the agent&apos;s configuration is saved. This catches regressions
        before they affect real users.
      </p>

      <Callout type="info">
        Auto-run fires asynchronously — it does not block the agent save response. The new run
        appears in the Runs tab within a few seconds.
      </Callout>

      <h2>Run scoring</h2>
      <p>
        Each run shows a <strong>score</strong> — the proportion of cases that passed (using the
        effective result, which takes manual overrides into account):
      </p>
      <pre><code>{`score = passed_cases / (passed_cases + failed_cases)`}</code></pre>
      <p>
        The score is shown as a percentage in the suite list (last run score), the run list, and the
        run detail header.
      </p>
    </DocPage>
  )
}
