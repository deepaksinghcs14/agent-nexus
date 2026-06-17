import { DocPage, Callout } from '@/components/docs/DocPage'

export const metadata = { title: 'Skills — Docs' }

export default function SkillsDoc() {
  return (
    <DocPage
      title="Skills"
      subtitle="Reusable instruction blocks that are injected into an agent's system prompt at run time — centralise expertise and attach it to any agent."
    >
      <h2>What are Skills?</h2>
      <p>
        A skill is a named block of text (markdown or plain prose) that describes a specific
        capability, persona, or set of rules. When a skill is attached to an agent, its content
        is appended to the agent&apos;s system prompt every time a run starts — no manual copy-paste,
        no drift between agents.
      </p>
      <p>
        Skills are managed at <a href="/skills">/skills</a>. Each workspace can create its own
        custom skills. Platform-managed skills (marked <em>managed</em>) are read-only and provided
        by Agent Nexus.
      </p>

      <h2>How skills are injected</h2>
      <p>
        When the invoke engine starts a run, it loads all skills attached to the agent and appends
        each one to the system prompt in a labelled block:
      </p>
      <pre><code>{`[Skill: WhatsApp Owner Escalation]
When a WhatsApp request is risky or ambiguous, call whatsapp_request_owner_approval ...

[Skill: Tone Guidelines]
Always respond in a friendly, concise tone. ...`}</code></pre>
      <p>
        The skills are injected <em>after</em> the agent&apos;s own instructions so they can extend
        or refine the base prompt without overriding it.
      </p>

      <Callout type="info">
        Skills are injected into the system prompt at run time, not stored in the agent config.
        Updating a skill immediately affects every agent it is attached to — no need to edit each
        agent individually.
      </Callout>

      <h2>Skill sources</h2>
      <table>
        <thead>
          <tr><th>Source</th><th>Description</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><strong>managed</strong></td>
            <td>
              Provided by Agent Nexus (e.g. <em>WhatsApp Owner Escalation</em>). Read-only — content
              is updated with platform releases. Cannot be deleted.
            </td>
          </tr>
          <tr>
            <td><strong>custom</strong></td>
            <td>Created by workspace members. Fully editable and deletable.</td>
          </tr>
        </tbody>
      </table>

      <h2>Attaching skills to an agent</h2>
      <ol>
        <li>Open the agent in the <a href="/agents">Agent Builder</a>.</li>
        <li>Go to the <strong>Skills</strong> tab.</li>
        <li>Select one or more skills from the list and save.</li>
      </ol>
      <p>
        Detaching a skill stops it from being injected on future runs. Runs already in progress are
        not affected.
      </p>

      <h2>Creating a skill</h2>
      <ol>
        <li>Go to <a href="/skills">/skills</a> and click <strong>New Skill</strong>.</li>
        <li>Give it a name and description (used to explain its purpose to teammates).</li>
        <li>Write the skill content — this is the text injected into the system prompt.</li>
        <li>Save. The skill is now available to attach to any agent in the workspace.</li>
      </ol>

      <h2>Built-in managed skills</h2>
      <table>
        <thead>
          <tr><th>Skill</th><th>Purpose</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><strong>WhatsApp Owner Escalation</strong></td>
            <td>
              Instructs the agent to call <code>whatsapp_request_owner_approval</code> before taking
              risky or ambiguous actions in a WhatsApp gateway context. Automatically included for
              agents used in gateway channels.
            </td>
          </tr>
        </tbody>
      </table>

      <Callout type="info">
        Managed skills are updated automatically with platform releases. If you need different
        behaviour, create a custom skill — do not try to duplicate a managed skill.
      </Callout>
    </DocPage>
  )
}
