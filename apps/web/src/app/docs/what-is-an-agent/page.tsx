import { DocPage, Callout, Badge } from '@/components/docs/DocPage'

export const metadata = { title: 'What is an Agent — Docs' }

export default function WhatIsAnAgentDoc() {
  return (
    <DocPage
      title="What is an Agent?"
      subtitle="An agent is an AI that can reason, use tools, access memory, and take multi-step actions — not just respond to a single prompt."
    >
      <p>
        In Agent Nexus, an <strong>agent</strong> is a configurable unit that combines a large language
        model with instructions, tools, memory, and context retrieval. Instead of a one-shot prompt →
        response, an agent can loop: call a tool, inspect the result, call another tool, and produce
        a final answer — all in a single run.
      </p>

      <Callout type="info">
        Every agent is backed by exactly one LLM provider and model. You can create as many agents as
        you want, each tuned differently for a specific task.
      </Callout>

      <h2>How a Run Works</h2>
      <p>
        When you send a message to an agent, the runtime executes this loop:
      </p>
      <ol>
        <li><strong>Memory retrieval</strong> — relevant memories from past conversations are retrieved and injected into the system prompt.</li>
        <li><strong>Context retrieval</strong> — if the agent has connectors attached, related documents are fetched by vector similarity and included as context.</li>
        <li><strong>Model call</strong> — the LLM receives the full prompt (instructions + memory + context + chat history + user message) and responds.</li>
        <li><strong>Tool calls</strong> — if the model requests a tool, the runtime executes it (with approval gating if needed) and loops back to the model call with the result.</li>
        <li><strong>Final response</strong> — when the model stops calling tools, the run completes and the output is returned.</li>
      </ol>

      <h2>Agent Configuration</h2>
      <p>
        Agents are created through the <a href="/agents/new">Agent Builder</a>, which has seven tabs:
      </p>

      <div className="table-scroll">
      <table>
        <thead><tr><th>Tab</th><th>What you configure</th></tr></thead>
        <tbody>
          <tr>
            <td><strong>Basics</strong></td>
            <td>Name, description, and status (active / paused / archived)</td>
          </tr>
          <tr>
            <td><strong>Model</strong></td>
            <td>
              Provider (Anthropic, OpenAI, Gemini, Ollama), model, temperature, max tokens,
              and whether to stream output
            </td>
          </tr>
          <tr>
            <td><strong>Instructions</strong></td>
            <td>The system prompt — tells the agent who it is, what it can do, and how to behave</td>
          </tr>
          <tr>
            <td><strong>Tools</strong></td>
            <td>
              Enable / disable individual tools. Each tool shows its risk level and whether it
              requires human approval before execution
            </td>
          </tr>
          <tr>
            <td><strong>Context</strong></td>
            <td>Toggle context retrieval, select which connectors to query, and set relevance thresholds</td>
          </tr>
          <tr>
            <td><strong>Memory</strong></td>
            <td>Toggle memory, set scope (conversation / agent / workspace), and retrieval settings</td>
          </tr>
          <tr>
            <td><strong>Guardrails</strong></td>
            <td>Max tool calls, max steps, max duration, and approval requirements per action type</td>
          </tr>
        </tbody>
      </table>
      </div>

      <h2>Providers and Models</h2>
      <p>
        Agent Nexus is model-agnostic. You bring your own API keys — one per provider per workspace —
        and each agent picks its provider and model independently.
      </p>
      <div className="table-scroll">
      <table>
        <thead><tr><th>Provider</th><th>Example Models</th></tr></thead>
        <tbody>
          <tr><td>Anthropic</td><td>claude-opus-4, claude-sonnet-4-5, claude-haiku-4-5</td></tr>
          <tr><td>OpenAI</td><td>gpt-4o, gpt-4o-mini, o1, o3</td></tr>
          <tr><td>Gemini</td><td>gemini-2.0-flash, gemini-1.5-pro</td></tr>
          <tr><td>Ollama</td><td>Any locally-hosted model (llama3.2, mistral, etc.)</td></tr>
        </tbody>
      </table>
      </div>

      <h2>System Prompt Tips</h2>
      <p>
        A well-written system prompt is the most important part of an agent. Here are patterns that work well:
      </p>
      <ul>
        <li>State the agent&apos;s role clearly: <em>&quot;You are a senior Go code reviewer.&quot;</em></li>
        <li>List what the agent should and should not do.</li>
        <li>Tell it how to use its tools: <em>&quot;Use <code>read_file</code> to read source files before suggesting changes.&quot;</em></li>
        <li>Set output format expectations: <em>&quot;Respond with a markdown diff block.&quot;</em></li>
        <li>Keep it focused — a narrow, specific agent outperforms a generic one.</li>
      </ul>

      <Callout type="tip">
        <strong>Start simple.</strong> Create an agent with just a system prompt and no tools first.
        Add tools and memory once the base behaviour is correct.
      </Callout>

      <h2>Agent Status</h2>
      <p>
        An agent can be in one of three states:
      </p>
      <ul>
        <li><Badge label="active" color="green" /> — accepts new runs from the playground and the API.</li>
        <li><Badge label="paused" color="amber" /> — visible but cannot be invoked. Useful during reconfiguration.</li>
        <li><Badge label="archived" color="gray" /> — hidden from normal lists. Old runs are still accessible.</li>
      </ul>

      <h2>Next Steps</h2>
      <ul>
        <li><a href="/agents/new">Create your first agent</a></li>
        <li><a href="/docs/what-is-a-tool">Learn about tools and risk levels</a></li>
        <li><a href="/docs/invoke-api">Invoke an agent via the API</a></li>
      </ul>
    </DocPage>
  )
}
