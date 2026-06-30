'use client'

import { useState } from 'react'
import { CheckCircle, XCircle, AlertCircle, RefreshCw, Copy, Check, Terminal, ExternalLink } from 'lucide-react'
import { adminAPI } from '@/lib/api'

function CodeBlock({ code }: { code: string }) {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <div className="relative group bg-gray-900 rounded-lg px-4 py-3 text-sm font-mono text-gray-200 overflow-x-auto">
      <code>{code}</code>
      <button
        onClick={copy}
        className="absolute top-2 right-2 p-1 rounded text-gray-500 hover:text-gray-300 opacity-0 group-hover:opacity-100 transition-opacity"
      >
        {copied ? <Check size={14} className="text-green-400" /> : <Copy size={14} />}
      </button>
    </div>
  )
}

interface StatusResult {
  claude_installed: boolean
  claude_authenticated: boolean
  claude_version?: string
  anthropic_key_set: boolean
  git_installed: boolean
  error?: string
}

export default function ClaudeCodeAdminPage() {
  const [status, setStatus] = useState<StatusResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [checked, setChecked] = useState(false)

  const checkStatus = async () => {
    setLoading(true)
    try {
      const res = await adminAPI.claudeCodeStatus() as { data: StatusResult }
      setStatus(res.data)
    } catch {
      setStatus({ claude_installed: false, claude_authenticated: false, anthropic_key_set: false, git_installed: false, error: 'Failed to reach API' })
    } finally {
      setLoading(false)
      setChecked(true)
    }
  }

  function StatusBadge({ ok, label }: { ok: boolean; label: string }) {
    return (
      <div className="flex items-center gap-2 text-sm">
        {ok
          ? <CheckCircle size={16} className="text-green-500 flex-shrink-0" />
          : <XCircle size={16} className="text-red-400 flex-shrink-0" />}
        <span className={ok ? 'text-gray-700' : 'text-red-600'}>{label}</span>
      </div>
    )
  }

  const allGood = status?.claude_installed && status?.claude_authenticated && status?.git_installed

  return (
    <div className="p-4 sm:p-6 max-w-3xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-6">
        <div>
          <h1 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Terminal size={20} className="text-purple-600" />
            Claude Code Setup
          </h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Configure the API server to run Claude Code for automated coding tasks.
          </p>
        </div>
        <button
          onClick={checkStatus}
          disabled={loading}
          className="flex items-center gap-2 px-3 py-2 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700 disabled:opacity-60"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          {checked ? 'Re-check status' : 'Check status'}
        </button>
      </div>

      {/* Status panel */}
      {checked && status && (
        <div className={`border rounded-xl p-4 mb-6 ${allGood ? 'border-green-200 bg-green-50' : 'border-amber-200 bg-amber-50'}`}>
          <div className="flex items-center gap-2 mb-3">
            {allGood
              ? <CheckCircle size={18} className="text-green-600" />
              : <AlertCircle size={18} className="text-amber-600" />}
            <span className="font-medium text-sm text-gray-800">
              {allGood ? 'All checks passed — ready to use native_run_claude_code' : 'Setup incomplete'}
            </span>
          </div>
          <div className="space-y-1.5">
            <StatusBadge ok={status.git_installed} label={status.git_installed ? 'git is installed' : 'git not found in PATH'} />
            <StatusBadge ok={status.claude_installed} label={status.claude_installed ? `claude CLI installed${status.claude_version ? ` (${status.claude_version})` : ''}` : 'claude CLI not found in PATH'} />
            <StatusBadge
              ok={status.claude_authenticated || status.anthropic_key_set}
              label={
                status.claude_authenticated
                  ? 'Authenticated via claude login (OAuth)'
                  : status.anthropic_key_set
                    ? 'ANTHROPIC_API_KEY env var is set'
                    : 'Not authenticated — run claude login or set ANTHROPIC_API_KEY'
              }
            />
          </div>
          {status.error && (
            <p className="mt-2 text-xs text-red-600">{status.error}</p>
          )}
        </div>
      )}

      {/* Step 1 */}
      <div className="mb-6">
        <h2 className="text-sm font-semibold text-gray-800 mb-2 flex items-center gap-2">
          <span className="w-5 h-5 rounded-full bg-purple-100 text-purple-700 text-xs flex items-center justify-center font-bold">1</span>
          Install Claude Code CLI on the API server
        </h2>
        <p className="text-sm text-gray-600 mb-2">
          The CLI must be on the <code className="bg-gray-100 px-1 rounded text-xs">$PATH</code> of
          the process running the Agent Nexus API.
        </p>
        <CodeBlock code="npm install -g @anthropic-ai/claude-code" />
        <p className="text-xs text-gray-400 mt-1">
          Requires Node.js 18+. Verify with <code className="bg-gray-100 px-1 rounded">claude --version</code>.
        </p>
      </div>

      {/* Step 2 */}
      <div className="mb-6">
        <h2 className="text-sm font-semibold text-gray-800 mb-2 flex items-center gap-2">
          <span className="w-5 h-5 rounded-full bg-purple-100 text-purple-700 text-xs flex items-center justify-center font-bold">2</span>
          Authenticate
        </h2>
        <p className="text-sm text-gray-600 mb-3">
          Choose one method. Option A is recommended — it stores OAuth credentials on the server that
          the API process inherits automatically.
        </p>

        <div className="space-y-3">
          <div className="border border-gray-200 rounded-lg p-3">
            <p className="text-xs font-medium text-gray-700 mb-1.5">Option A — OAuth login (recommended for Docker/Railway)</p>
            <CodeBlock code="docker exec -it agent-nexus-api sh" />
            <div className="mt-1.5">
              <CodeBlock code="claude login" />
            </div>
            <p className="text-xs text-gray-400 mt-1.5">
              Follow the browser link printed in the terminal. Credentials are stored in
              <code className="bg-gray-100 px-1 rounded mx-0.5">~/.config/claude/</code>
              inside the container.
            </p>
          </div>

          <div className="border border-gray-200 rounded-lg p-3">
            <p className="text-xs font-medium text-gray-700 mb-1.5">Option B — API key env var</p>
            <p className="text-xs text-gray-500 mb-1.5">
              Add to <code className="bg-gray-100 px-1 rounded">services/api/.env</code> (local) or Railway env vars (production):
            </p>
            <CodeBlock code="ANTHROPIC_API_KEY=sk-ant-..." />
          </div>
        </div>
      </div>

      {/* Step 3 */}
      <div className="mb-6">
        <h2 className="text-sm font-semibold text-gray-800 mb-2 flex items-center gap-2">
          <span className="w-5 h-5 rounded-full bg-purple-100 text-purple-700 text-xs flex items-center justify-center font-bold">3</span>
          Connect GitHub repositories
        </h2>
        <p className="text-sm text-gray-600 mb-2">
          Create a <strong>GitHub Connector</strong> for each repository your agents may modify.
          The connector indexes the codebase for code search and securely stores the GitHub token —
          agents pass the connector ID instead of a raw token.
        </p>
        <a
          href="/connectors"
          className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-gray-900 text-white text-xs rounded-lg hover:bg-gray-700"
        >
          Open Connectors <ExternalLink size={12} />
        </a>
      </div>

      {/* Step 4 */}
      <div className="mb-6">
        <h2 className="text-sm font-semibold text-gray-800 mb-2 flex items-center gap-2">
          <span className="w-5 h-5 rounded-full bg-purple-100 text-purple-700 text-xs flex items-center justify-center font-bold">4</span>
          Attach the tool and test
        </h2>
        <p className="text-sm text-gray-600 mb-2">
          The <code className="bg-gray-100 px-1 rounded text-xs">native_run_claude_code</code> tool
          is already available in your workspace. Attach it to any agent and test it from the
          Playground with a small task on a test repository.
        </p>
        <a
          href="/tools"
          className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-gray-900 text-white text-xs rounded-lg hover:bg-gray-700"
        >
          Open Tools <ExternalLink size={12} />
        </a>
        <span className="mx-2 text-gray-300">·</span>
        <a
          href="/docs/claude-code-tool"
          className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-gray-200 text-gray-700 text-xs rounded-lg hover:bg-gray-50"
        >
          Full documentation <ExternalLink size={12} />
        </a>
      </div>

      {/* Docker note */}
      <div className="border border-blue-100 bg-blue-50 rounded-xl p-4 text-sm text-blue-800">
        <p className="font-medium mb-1">Running in Docker?</p>
        <p className="text-xs text-blue-700 mb-2">
          OAuth credentials stored via <code className="bg-blue-100 px-1 rounded">claude login</code>{' '}
          are written inside the container and will be lost on rebuild. To make them persistent,
          mount a volume for <code className="bg-blue-100 px-1 rounded">~/.config/claude/</code> or
          use the <code className="bg-blue-100 px-1 rounded">ANTHROPIC_API_KEY</code> env var approach instead.
        </p>
        <CodeBlock code={`# In infra/docker-compose.yml, under the api service:
volumes:
  - claude-config:/root/.config/claude

volumes:
  claude-config:`} />
      </div>
    </div>
  )
}
