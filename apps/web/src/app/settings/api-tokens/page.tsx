'use client'

import { useState, useEffect } from 'react'
import { Key, Plus, Trash2, Copy, Check, AlertCircle } from 'lucide-react'
import { apiTokensAPI } from '@/lib/api'
import type { APIToken, CreatedAPIToken } from '@/types'

function formatDate(s: string | null) {
  if (!s) return '—'
  return new Date(s).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
}

export default function APITokensPage() {
  const [tokens, setTokens] = useState<APIToken[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [newToken, setNewToken] = useState<CreatedAPIToken | null>(null)
  const [copied, setCopied] = useState(false)

  // Create form state
  const [name, setName] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState('')

  const load = () => {
    apiTokensAPI.list()
      .then((r) => setTokens(r.data ?? []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleCreate = async () => {
    if (!name.trim()) { setCreateError('Name is required'); return }
    setCreating(true)
    setCreateError('')
    try {
      const created = await apiTokensAPI.create({
        name: name.trim(),
        expires_at: expiresAt || null,
        scopes: [],
      })
      setNewToken(created)
      setShowCreate(false)
      setName('')
      setExpiresAt('')
      load()
    } catch {
      setCreateError('Failed to create token')
    } finally {
      setCreating(false)
    }
  }

  const handleRevoke = async (id: string) => {
    if (!confirm('Revoke this token? Any integrations using it will stop working.')) return
    await apiTokensAPI.revoke(id).catch(() => {})
    load()
  }

  const handleCopy = () => {
    if (!newToken) return
    navigator.clipboard.writeText(newToken.token)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="max-w-2xl mx-auto py-10 px-6">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">API Tokens</h1>
          <p className="text-sm text-gray-500 mt-1">
            Use tokens to access the Agent Nexus API from scripts, CI/CD, or integrations.
          </p>
        </div>
        <button
          onClick={() => { setShowCreate(true); setNewToken(null) }}
          className="flex items-center gap-2 px-4 py-2 rounded-md bg-[#534AB7] hover:bg-[#4a42a3] text-white text-sm font-medium transition-colors"
        >
          <Plus className="w-4 h-4" />
          New token
        </button>
      </div>

      {/* New token revealed */}
      {newToken && (
        <div className="mb-6 p-4 rounded-lg border border-green-200 bg-green-50">
          <div className="flex items-start gap-3">
            <AlertCircle className="w-5 h-5 text-green-600 shrink-0 mt-0.5" />
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-green-800 mb-2">
                Token created — copy it now. You won&apos;t see it again.
              </p>
              <div className="flex items-center gap-2">
                <code className="flex-1 px-3 py-1.5 rounded bg-white border border-green-200 text-sm font-mono text-gray-800 truncate">
                  {newToken.token}
                </code>
                <button
                  onClick={handleCopy}
                  className="shrink-0 p-1.5 rounded hover:bg-green-100 text-green-700"
                >
                  {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Create form */}
      {showCreate && (
        <div className="mb-6 p-4 rounded-lg border border-gray-200 bg-white space-y-4">
          <h3 className="font-medium text-gray-900">New API Token</h3>
          <div>
            <label className="block text-sm text-gray-600 mb-1">Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. CI Pipeline, Slack Bot"
              className="w-full px-3 py-2 rounded border border-gray-300 text-sm focus:outline-none focus:ring-2 focus:ring-[#534AB7]/30 focus:border-[#534AB7]"
            />
          </div>
          <div>
            <label className="block text-sm text-gray-600 mb-1">
              Expiration <span className="text-gray-400">(optional — leave blank for no expiry)</span>
            </label>
            <input
              type="date"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value ? new Date(e.target.value).toISOString() : '')}
              className="px-3 py-2 rounded border border-gray-300 text-sm focus:outline-none focus:ring-2 focus:ring-[#534AB7]/30 focus:border-[#534AB7]"
            />
          </div>
          {createError && <p className="text-sm text-red-500">{createError}</p>}
          <div className="flex gap-2">
            <button
              onClick={handleCreate}
              disabled={creating}
              className="px-4 py-2 rounded-md bg-[#534AB7] hover:bg-[#4a42a3] text-white text-sm font-medium transition-colors disabled:opacity-50"
            >
              {creating ? 'Creating…' : 'Create token'}
            </button>
            <button
              onClick={() => setShowCreate(false)}
              className="px-4 py-2 rounded-md border border-gray-300 text-sm text-gray-600 hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Token list */}
      {loading ? (
        <div className="py-10 text-center text-sm text-gray-400">Loading…</div>
      ) : tokens.length === 0 ? (
        <div className="py-12 text-center">
          <Key className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-sm text-gray-500">No API tokens yet. Create one to get started.</p>
        </div>
      ) : (
        <div className="rounded-lg border border-gray-200 overflow-hidden divide-y divide-gray-100">
          {tokens.map((t) => (
            <div key={t.id} className="flex items-center justify-between px-4 py-3 bg-white">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <p className="text-sm font-medium text-gray-900">{t.name}</p>
                  <code className="text-xs text-gray-400 font-mono">{t.token_prefix}…</code>
                </div>
                <p className="text-xs text-gray-400 mt-0.5">
                  Created {formatDate(t.created_at)}
                  {t.last_used_at ? ` · Last used ${formatDate(t.last_used_at)}` : ' · Never used'}
                  {t.expires_at ? ` · Expires ${formatDate(t.expires_at)}` : ''}
                </p>
              </div>
              <button
                onClick={() => handleRevoke(t.id)}
                className="ml-4 p-1.5 rounded text-gray-400 hover:text-red-500 hover:bg-red-50 transition-colors"
                title="Revoke token"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>
      )}

      <div className="mt-6 p-4 rounded-lg bg-gray-50 border border-gray-200">
        <p className="text-sm text-gray-600">
          Use your token in the{' '}
          <a href="/docs/api-tokens" className="text-[#534AB7] hover:underline">
            interactive documentation
          </a>{' '}
          to test API calls directly in the browser.
        </p>
      </div>
    </div>
  )
}
