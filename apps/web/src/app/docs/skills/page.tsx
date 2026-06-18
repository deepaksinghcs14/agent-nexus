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

      <h2>Required tools</h2>
      <p>
        A skill can declare a list of <strong>required tool names</strong>. When you enable such a skill
        on an agent, those tools are automatically attached to the agent — no need to find and check
        them individually in the Tools tab.
      </p>
      <p>
        In the Agent Builder, tools added this way are shown with an <em>enabled by skill</em> label
        and cannot be manually unchecked while the skill is active. Disabling the skill removes them
        (unless another enabled skill also requires the same tool).
      </p>

      <Callout type="info">
        The built-in <strong>Agent Self-Management</strong> skill uses this mechanism — enabling it
        auto-attaches all 10 self-management tools and injects the capabilities guide into the system prompt.
      </Callout>

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
      <p>
        These skills are provided by Agent Nexus and are available in every workspace. They are
        read-only — attach them to any agent directly from the Skills tab.
      </p>
      <table>
        <thead>
          <tr><th>Skill</th><th>Auto-attaches tools</th><th>Purpose</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><strong>Agent Self-Management</strong></td>
            <td>10 native tools</td>
            <td>
              Enables the agent to call other agents, create ephemeral agents/skills/HTTP tools at
              runtime, and clean up after itself. Parallel sub-agent execution is supported — issue
              multiple <code>native_call_agent</code> calls in one response to run them concurrently.
              See the <a href="/docs/agent-configuration">Agent Configuration</a> page for the full
              tool reference.
            </td>
          </tr>
          <tr>
            <td><strong>WhatsApp Owner Escalation</strong></td>
            <td><code>whatsapp_request_owner_approval</code></td>
            <td>
              Instructs the agent to call <code>whatsapp_request_owner_approval</code> before taking
              risky or ambiguous actions in a WhatsApp gateway context. Recommended for all agents
              used in WhatsApp channels.
            </td>
          </tr>
          <tr>
            <td><strong>WhatsApp Formatter</strong></td>
            <td>—</td>
            <td>
              Formats responses for WhatsApp — avoids markdown headers, uses *bold* for emphasis,
              and keeps replies under 1600 characters. Attach to any agent that sends WhatsApp messages.
            </td>
          </tr>
          <tr>
            <td><strong>Human Persona</strong></td>
            <td>—</td>
            <td>
              Makes the agent reply like a real person texting — casual, short, no AI filler phrases
              (&ldquo;Certainly!&rdquo;, &ldquo;Of course!&rdquo;), no unnecessary bullet points. Ideal for WhatsApp and
              conversational contexts where you want replies to feel natural.
            </td>
          </tr>
          <tr>
            <td><strong>Language Mirror</strong></td>
            <td>—</td>
            <td>
              Detects the language of the user&apos;s message and always replies in the same language.
              Useful for multilingual audiences.
            </td>
          </tr>
          <tr>
            <td><strong>Concise Responder</strong></td>
            <td>—</td>
            <td>
              Keeps responses under 300 words unless the user explicitly asks for more detail. Reduces
              token usage and improves readability for everyday queries.
            </td>
          </tr>
          <tr>
            <td><strong>Safety Guardrail</strong></td>
            <td>—</td>
            <td>
              Prevents the agent from revealing its system instructions, API keys, or internal
              configuration, even if asked directly.
            </td>
          </tr>
          <tr>
            <td><strong>Professional Tone</strong></td>
            <td>—</td>
            <td>
              Maintains a professional, direct, and respectful tone. Avoids slang and excessive
              informality. Complements <em>Human Persona</em> when you want warmth without being too casual.
            </td>
          </tr>
        </tbody>
      </table>

      <Callout type="tip">
        For WhatsApp agents, a good starting stack is: <strong>WhatsApp Formatter</strong> +{' '}
        <strong>Human Persona</strong> + <strong>WhatsApp Owner Escalation</strong>. Adjust as needed.
      </Callout>

      <Callout type="info">
        Managed skills are updated with platform releases. If you need different behaviour, create
        a custom skill — do not duplicate a managed skill.
      </Callout>
    </DocPage>
  )
}
