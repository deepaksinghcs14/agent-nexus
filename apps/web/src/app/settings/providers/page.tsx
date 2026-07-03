'use client'

import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, Check, Key, Chrome } from 'lucide-react'
import Link from 'next/link'
import { providersAPI } from '@/lib/api'
import type { ProviderCredential } from '@/types'

const PROVIDER_OPTIONS = [
  { value: 'anthropic', label: 'Anthropic', color: 'bg-amber-50 text-amber-700' },
  { value: 'openai', label: 'OpenAI', color: 'bg-green-50 text-green-700' },
  { value: 'gemini', label: 'Google Gemini', color: 'bg-blue-50 text-blue-700' },
  { value: 'ollama', label: 'Ollama (local)', color: 'bg-purple-50 text-purple-700' },
]

function providerColor(provider: string) {
  return PROVIDER_OPTIONS.find((p) => p.value === provider)?.color ?? 'bg-gray-50 text-gray-600'
}

function tokenExpiryLabel(expiry?: string) {
  if (!expiry) return null
  const diff = new Date(expiry).getTime() - Date.now()
  if (diff <= 0) return 'Token expired — will auto-refresh on next run'
  const hours = Math.floor(diff / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (hours > 0) return `Token valid · expires in ${hours}h ${mins}m · auto-refreshes`
  return `Token valid · expires in ${mins}m · auto-refreshes`
}

export default function ProvidersPage() {
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState({ provider: 'anthropic', display_name: '', api_key: '', base_url: '' })
  const [formError, setFormError] = useState('')
  const [saved, setSaved] = useState(false)
  const [oauthStatus, setOauthStatus] = useState<'success' | 'error' | null>(null)

  // Read ?oauth= query param on mount
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const status = params.get('oauth')
    if (status === 'success') {
      setOauthStatus('success')
      // Remove the query param without reloading
      window.history.replaceState({}, '', '/settings/providers')
    } else if (status === 'error') {
      setOauthStatus('error')
      window.history.replaceState({}, '', '/settings/providers')
    }
  }, [])

  const { data, isLoading } = useQuery({
    queryKey: ['providers'],
    queryFn: () => providersAPI.list() as Promise<{ data: ProviderCredential[] }>,
  })

  const createMutation = useMutation({
    mutationFn: () => providersAPI.create({
      provider: form.provider,
      display_name: form.display_name || form.provider,
      api_key: form.api_key,
      base_url: form.base_url,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['providers'] })
      setAdding(false)
      setForm({ provider: 'anthropic', display_name: '', api_key: '', base_url: '' })
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    },
    onError: (err: Error) => setFormError(err.message),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => providersAPI.delete(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['providers'] }),
  })

  async function handleGoogleOAuth() {
    const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
    const token = localStorage.getItem('access_token')
    try {
      const res = await fetch(`${apiUrl}/api/v1/providers/oauth/google/authorize`, {
        headers: { Authorization: `Bearer ${token ?? ''}` },
        credentials: 'include',
      })
      if (!res.ok) throw new Error('Failed to start OAuth flow')
      const { auth_url } = await res.json()
      window.location.href = auth_url
    } catch {
      setOauthStatus('error')
    }
  }

  const providers = data?.data ?? []

  return (
    <div className="p-4 sm:p-6 max-w-2xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Providers</h1>
          <p className="text-sm text-gray-500 mt-0.5">API keys are encrypted at rest with AES-256-GCM</p>
        </div>
        <button
          onClick={() => { setAdding(true); setFormError('') }}
          className="flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700"
        >
          <Plus size={13} /> Add Key
        </button>
      </div>

      {oauthStatus === 'success' && (
        <div className="flex items-center gap-2 text-green-700 bg-green-50 border border-green-200 rounded-lg px-3 py-2 mb-4 text-sm">
          <Check size={14} /> Connected to Google Gemini via OAuth
        </div>
      )}
      {oauthStatus === 'error' && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2 mb-4">
          Google OAuth failed. Please try again.
        </div>
      )}

      {saved && (
        <div className="flex items-center gap-2 text-green-700 bg-green-50 border border-green-200 rounded-lg px-3 py-2 mb-4 text-sm">
          <Check size={14} /> API key saved successfully
        </div>
      )}

      {/* Pipeline credentials (Claude account, GitHub token) moved to their own tab */}
      <p className="text-[11px] text-gray-400 mb-4">
        Looking for repo coding session credentials?{' '}
        <Link href="/settings/claude-code" className="text-purple-600 hover:underline">Settings → Claude Code</Link>
      </p>

      {/* Google OAuth quick-connect for Gemini */}
      {!providers.find((p) => p.provider === 'gemini') && (
        <div className="flex flex-wrap items-center gap-3 justify-between border border-blue-100 bg-blue-50 rounded-xl px-4 py-3 mb-4">
          <div className="flex items-center gap-2.5">
            <Chrome size={16} className="text-blue-600" />
            <div>
              <p className="text-sm font-medium text-blue-900">Connect Google Gemini</p>
              <p className="text-xs text-blue-600">Use your Google account — no API key needed</p>
            </div>
          </div>
          <button
            onClick={handleGoogleOAuth}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 text-white text-xs rounded-lg hover:bg-blue-700"
          >
            <Chrome size={12} /> Login with Google
          </button>
        </div>
      )}

      {adding && (
        <div className="border border-gray-200 rounded-xl p-4 mb-4 bg-gray-50 space-y-3">
          <h3 className="text-sm font-medium text-gray-700">Add API Key</h3>
          {formError && (
            <div className="text-xs text-red-600 bg-red-50 border border-red-200 rounded px-2 py-1">{formError}</div>
          )}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-gray-500 mb-1">Provider</label>
              <select
                value={form.provider}
                onChange={e => setForm(f => ({ ...f, provider: e.target.value }))}
                className="w-full text-xs px-2 py-1.5 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400"
              >
                {PROVIDER_OPTIONS.map(p => <option key={p.value} value={p.value}>{p.label}</option>)}
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Display name (optional)</label>
              <input
                type="text"
                value={form.display_name}
                onChange={e => setForm(f => ({ ...f, display_name: e.target.value }))}
                placeholder="e.g. Production key"
                className="w-full text-xs px-2 py-1.5 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400"
              />
            </div>
          </div>
          {form.provider === 'gemini' ? (
            <div className="space-y-2">
              <div>
                <label className="block text-xs text-gray-500 mb-1">API Key</label>
                <input
                  type="password"
                  value={form.api_key}
                  onChange={e => setForm(f => ({ ...f, api_key: e.target.value }))}
                  placeholder="AIza..."
                  className="w-full text-xs px-2 py-1.5 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400"
                />
              </div>
              <div className="flex items-center gap-2 pt-1">
                <span className="text-xs text-gray-400">or</span>
                <button
                  type="button"
                  onClick={handleGoogleOAuth}
                  className="flex items-center gap-1.5 px-3 py-1.5 border border-blue-200 text-blue-700 bg-blue-50 text-xs rounded-lg hover:bg-blue-100"
                >
                  <Chrome size={12} /> Login with Google instead
                </button>
              </div>
            </div>
          ) : (
            <div>
              <label className="block text-xs text-gray-500 mb-1">API Key</label>
              <input
                type="password"
                value={form.api_key}
                onChange={e => setForm(f => ({ ...f, api_key: e.target.value }))}
                placeholder="sk-..."
                className="w-full text-xs px-2 py-1.5 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400"
              />
              {form.provider !== 'gemini' && form.provider !== 'ollama' && (
                <p className="text-[10px] text-gray-400 mt-1">API key required — OAuth is not supported for {form.provider}.</p>
              )}
            </div>
          )}
          {(form.provider === 'ollama') && (
            <div>
              <label className="block text-xs text-gray-500 mb-1">Base URL</label>
              <input
                type="text"
                value={form.base_url}
                onChange={e => setForm(f => ({ ...f, base_url: e.target.value }))}
                placeholder="http://localhost:11434"
                className="w-full text-xs px-2 py-1.5 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400"
              />
            </div>
          )}
          <div className="flex gap-2 pt-1">
            <button
              onClick={() => createMutation.mutate()}
              disabled={createMutation.isPending || (!form.api_key && form.provider !== 'ollama')}
              className="px-3 py-1.5 bg-purple-600 text-white text-xs rounded-lg hover:bg-purple-700 disabled:opacity-50"
            >
              {createMutation.isPending ? 'Saving…' : 'Save Key'}
            </button>
            <button
              onClick={() => { setAdding(false); setFormError('') }}
              className="px-3 py-1.5 border border-gray-200 text-gray-600 text-xs rounded-lg hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {isLoading && <div className="text-sm text-gray-400 py-8 text-center">Loading…</div>}

      {!isLoading && providers.length === 0 && !adding && (
        <div className="border border-dashed border-gray-200 rounded-xl p-8 text-center">
          <Key size={28} className="mx-auto text-gray-300 mb-3" />
          <p className="text-gray-500 text-sm">No API keys configured yet.</p>
        </div>
      )}

      <div className="space-y-2">
        {providers.map((p) => (
          <div key={p.id} className="flex flex-wrap items-center gap-3 justify-between border border-gray-100 rounded-xl p-4 bg-white">
            <div className="flex items-center gap-3">
              <span className={`text-[11px] font-medium px-2 py-0.5 rounded-full ${providerColor(p.provider)}`}>
                {p.provider}
              </span>
              <div>
                <p className="text-sm font-medium text-gray-800">{p.display_name}</p>
                {p.auth_type === 'oauth' ? (
                  <p className="text-xs text-blue-600">{tokenExpiryLabel(p.oauth_token_expiry) ?? 'Connected via Google OAuth'}</p>
                ) : (
                  <p className="text-xs text-gray-400">••••••••••••••••</p>
                )}
              </div>
            </div>
            <div className="flex items-center gap-2">
              {p.auth_type === 'oauth' && (
                <span className="text-[11px] px-2 py-0.5 rounded-full bg-blue-50 text-blue-700">OAuth</span>
              )}
              <span className={`text-[11px] px-2 py-0.5 rounded-full ${p.is_active ? 'bg-green-50 text-green-700' : 'bg-gray-50 text-gray-500'}`}>
                {p.is_active ? 'active' : 'inactive'}
              </span>
              <button
                onClick={() => { if (confirm('Delete this credential?')) deleteMutation.mutate(p.id) }}
                className="p-1.5 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded-lg"
              >
                <Trash2 size={14} />
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
