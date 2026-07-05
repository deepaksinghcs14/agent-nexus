import { DocPage, ConceptGrid } from '@/components/docs/DocPage'

export const metadata = { title: 'Documentation — Agent Nexus' }

export default function DocsOverview() {
  return (
    <DocPage
      title="Agent Nexus Documentation"
      subtitle="Everything you need to build with Agent Nexus — from running agents in the UI to invoking them programmatically."
    >
      <h2>Platform Concepts</h2>
      <ConceptGrid items={[
        {
          title: 'What is an Agent?',
          href: '/docs/what-is-an-agent',
          desc: 'An AI that can reason, use tools, access memory, and take multi-step actions.',
          badge: 'Concept',
        },
        {
          title: 'What is a Workflow?',
          href: '/docs/what-is-an-agent-group',
          desc: 'Chain multiple specialised agents together in a pipeline.',
          badge: 'Concept',
        },
        {
          title: 'Workflow Node Types',
          href: '/docs/workflows',
          desc: 'Start, Agent, Supervisor, Condition, Parallel, Join, and Loop nodes, plus save-time validation and crash-resumable runs.',
          badge: 'Reference',
        },
        {
          title: 'What is a Tool?',
          href: '/docs/what-is-a-tool',
          desc: 'Give agents the ability to take action — read files, call APIs, search the web.',
          badge: 'Concept',
        },
        {
          title: 'What is a Connector?',
          href: '/docs/what-is-a-connector',
          desc: 'Bring external knowledge into agents via RAG — filesystem, Slack, GitHub, and more.',
          badge: 'Concept',
        },
        {
          title: 'MCP Servers',
          href: '/docs/mcp-servers',
          desc: 'Connect any MCP-compatible server to expose additional tools to agents.',
          badge: 'Concept',
        },
        {
          title: 'Skills',
          href: '/docs/skills',
          desc: 'Reusable instruction blocks injected into agent system prompts — share expertise across agents.',
          badge: 'Concept',
        },
      ]} />

      <h2>Autonomous Coding</h2>
      <ConceptGrid items={[
        {
          title: 'Claude Code Pipeline',
          href: '/docs/claude-code-pipeline',
          desc: 'Jira tickets driven to reviewed pull requests — repo catalog, coding sessions, review agent, durable waits.',
          badge: 'Pipeline',
        },
      ]} />

      <h2>Testing & Quality</h2>
      <ConceptGrid items={[
        {
          title: 'Eval Framework',
          href: '/docs/evals',
          desc: 'Test agents with named suites, LLM judge grading, AI failure analysis, and one-click prompt fixes.',
          badge: 'Evals',
        },
      ]} />

      <h2>Messaging Channels</h2>
      <ConceptGrid items={[
        {
          title: 'Nexus Gateway',
          href: '/docs/gateway',
          desc: 'Connect agents to WhatsApp and HTTP channels so they can receive and reply to real-world messages.',
          badge: 'Gateway',
        },
      ]} />

      <h2>API Integration</h2>
      <ConceptGrid items={[
        {
          title: 'API Tokens',
          href: '/docs/api-tokens',
          desc: 'Create long-lived tokens for programmatic access to your agents.',
          badge: 'Auth',
        },
        {
          title: 'Invoke API',
          href: '/docs/invoke-api',
          desc: 'Invoke agents and workflows with a single HTTP request.',
          badge: 'API',
        },
        {
          title: 'Webhook Triggers',
          href: '/docs/webhook-triggers',
          desc: 'Run agents and workflows automatically from inbound HTTP events.',
          badge: 'API',
        },
        {
          title: 'Run States',
          href: '/docs/run-states',
          desc: 'Understand the run lifecycle and handle approval gates.',
          badge: 'Reference',
        },
        {
          title: 'Agent Configuration',
          href: '/docs/agent-configuration',
          desc: 'Every field on an agent — model, memory, guardrails, tools, and context retrieval.',
          badge: 'Reference',
        },
        {
          title: 'SSE Events',
          href: '/docs/sse-events',
          desc: 'Stream real-time events from a running agent.',
          badge: 'Reference',
        },
      ]} />

      <h2>System Design</h2>
      <ConceptGrid items={[
        {
          title: 'Architecture Diagram',
          href: '/architecture',
          desc: 'Animated component overview — frontend, API server, database, AI providers, MCP, and webhook flows.',
          badge: 'Visual',
        },
      ]} />

      <h2>Quick Start</h2>
      <p>The fastest way to invoke an agent from your code:</p>
      <pre><code>{`curl -X POST https://your-instance.example.com/api/v1/invoke/agents/AGENT_ID \\
  -H "Authorization: Bearer anx_your_token_here" \\
  -H "Content-Type: application/json" \\
  -d '{"input": "Summarise the Q3 report"}'`}</code></pre>
      <p>
        The response includes a <code>run_id</code>. Poll{' '}
        <code>GET /api/v1/runs/:id</code> to check status and retrieve output.
        Or set <code>&quot;stream&quot;: true</code> to receive SSE events in real time.
      </p>

      <h2>Base URL</h2>
      <p>All API endpoints are relative to your Agent Nexus instance. For local development:</p>
      <pre><code>{`http://localhost:8080/api/v1`}</code></pre>
    </DocPage>
  )
}
