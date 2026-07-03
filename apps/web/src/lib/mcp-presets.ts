// Preset catalog for the MCP server portal: popular servers configurable
// with an API token instead of OAuth (for orgs whose IdP blocks MCP OAuth).
// Each preset pins a stdio command; the portal collects the env vars and the
// API stores them encrypted (AES-256-GCM) and injects them into the spawned
// process. Adding a preset here needs no backend change.

export type MCPPresetEnvField = {
  key: string // environment variable name
  label: string
  placeholder?: string
  secret?: boolean // render as password input
  required?: boolean
}

export type MCPPreset = {
  id: string
  label: string
  description: string
  command: string // stdio command line
  env: MCPPresetEnvField[]
  docsUrl?: string
  note?: string
}

export const MCP_PRESETS: MCPPreset[] = [
  {
    id: 'atlassian',
    label: 'Atlassian (Jira + Confluence)',
    description: 'Jira issues, JQL, sprints, Confluence pages — via API token, no OAuth needed.',
    command: 'uvx mcp-atlassian',
    env: [
      { key: 'JIRA_URL', label: 'Jira URL', placeholder: 'https://yourorg.atlassian.net', required: true },
      { key: 'JIRA_USERNAME', label: 'Jira email', placeholder: 'you@org.com (leave empty for Data Center PAT)' },
      { key: 'JIRA_API_TOKEN', label: 'Jira API token', secret: true, required: true },
      { key: 'CONFLUENCE_URL', label: 'Confluence URL', placeholder: 'https://yourorg.atlassian.net/wiki (optional)' },
      { key: 'CONFLUENCE_USERNAME', label: 'Confluence email', placeholder: 'you@org.com (optional)' },
      { key: 'CONFLUENCE_API_TOKEN', label: 'Confluence API token', secret: true },
    ],
    docsUrl: 'https://github.com/sooperset/mcp-atlassian',
    note: 'Cloud: create an API token at id.atlassian.com → Security. Data Center: use a PAT and leave the email fields empty. First sync downloads the package and may take a minute.',
  },
  {
    id: 'github',
    label: 'GitHub',
    description: 'Repos, issues, PRs, file contents, and code search via a personal access token.',
    command: 'npx -y @modelcontextprotocol/server-github',
    env: [
      { key: 'GITHUB_PERSONAL_ACCESS_TOKEN', label: 'Personal access token', placeholder: 'ghp_… or github_pat_…', secret: true, required: true },
    ],
    docsUrl: 'https://github.com/modelcontextprotocol/servers-archived/tree/main/src/github',
  },
  {
    id: 'slack',
    label: 'Slack',
    description: 'Read channels, post messages, and manage threads via a bot token.',
    command: 'npx -y @modelcontextprotocol/server-slack',
    env: [
      { key: 'SLACK_BOT_TOKEN', label: 'Bot token', placeholder: 'xoxb-…', secret: true, required: true },
      { key: 'SLACK_TEAM_ID', label: 'Team ID', placeholder: 'T01234567', required: true },
    ],
    docsUrl: 'https://github.com/modelcontextprotocol/servers-archived/tree/main/src/slack',
  },
  {
    id: 'notion',
    label: 'Notion',
    description: 'Search, read, and write Notion pages and databases via an integration token.',
    command: 'npx -y @notionhq/notion-mcp-server',
    env: [
      { key: 'NOTION_TOKEN', label: 'Integration token', placeholder: 'ntn_… / secret_…', secret: true, required: true },
    ],
    docsUrl: 'https://github.com/makenotion/notion-mcp-server',
  },
  {
    id: 'brave-search',
    label: 'Brave Search',
    description: 'Web and local search through the Brave Search API.',
    command: 'npx -y @modelcontextprotocol/server-brave-search',
    env: [
      { key: 'BRAVE_API_KEY', label: 'API key', secret: true, required: true },
    ],
    docsUrl: 'https://github.com/modelcontextprotocol/servers-archived/tree/main/src/brave-search',
  },
]
