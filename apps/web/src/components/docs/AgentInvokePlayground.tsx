'use client'

import { useState, useEffect } from 'react'
import { Play, Loader2, Plus, RefreshCw } from 'lucide-react'
import { useDocsStore } from '@/store/docs'

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

interface Resource { id: string; name: string }

type Mode = 'agent' | 'group'

async function fetchResources(token: string, mode: Mode): Promise<Resource[]> {
  const path = mode === 'agent' ? '/agents' : '/workflows'
  const res = await fetch(`${API_URL}/api/v1${path}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) return []
  const json = await res.json()
  return (json.data ?? []).map((r: { id: string; name: string }) => ({ id: r.id, name: r.name }))
}

export default function AgentInvokePlayground() {
  const tokenRaw = useDocsStore((s) => s.selectedTokenRaw)
  const [mode, setMode] = useState<Mode>('agent')
  const [resources, setResources] = useState<Resource[]>([])
  const [selectedId, setSelectedId] = useState('')
  const [input, setInput] = useState('Hello! What can you do?')
  const [stream, setStream] = useState(false)
  const [loading, setLoading] = useState(false)
  const [fetching, setFetching] = useState(false)
  const [response, setResponse] = useState<{ status: number; data: unknown } | null>(null)

  const loadResources = async () => {
    if (!tokenRaw) return
    setFetching(true)
    const list = await fetchResources(tokenRaw, mode).catch(() => [])
    setResources(list)
    setSelectedId(list[0]?.id ?? '')
    setFetching(false)
  }

  useEffect(() => {
    setResources([])
    setSelectedId('')
    setResponse(null)
    if (tokenRaw) loadResources()
  }, [tokenRaw, mode])

  const handleSend = async () => {
    if (!tokenRaw || !selectedId || !input.trim()) return
    setLoading(true)
    setResponse(null)
    const path = mode === 'agent'
      ? `/invoke/agents/${selectedId}`
      : `/invoke/workflows/${selectedId}`
    try {
      const res = await fetch(`${API_URL}/api/v1${path}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${tokenRaw}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ input: input.trim(), stream }),
      })
      const data = await res.json().catch(() => null)
      setResponse({ status: res.status, data })
    } catch (err) {
      setResponse({ status: 0, data: { error: String(err) } })
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      border: '1px solid #e5e7eb', borderRadius: 10,
      overflow: 'hidden', margin: '1.5rem 0', background: '#fff',
    }}>
      {/* Header */}
      <div style={{ background: '#f9fafb', borderBottom: '1px solid #e5e7eb', padding: '0.875rem 1.25rem', display: 'flex', alignItems: 'center', gap: 12 }}>
        <span style={{ background: '#dbeafe', color: '#1d4ed8', fontSize: '0.7rem', fontWeight: 700, padding: '0.2em 0.6em', borderRadius: 4, letterSpacing: '0.03em' }}>
          POST
        </span>
        <code style={{ fontSize: '0.8125rem', color: '#374151', fontFamily: 'monospace' }}>
          {mode === 'agent' ? '/api/v1/invoke/agents/{agentId}' : '/api/v1/invoke/workflows/{workflowId}'}
        </code>
      </div>

      <div style={{ padding: '1.25rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
        {!tokenRaw && (
          <div style={{ padding: '0.875rem 1rem', background: '#fafafa', border: '1px solid #e5e7eb', borderRadius: 8, fontSize: '0.875rem', color: '#6b7280', textAlign: 'center' }}>
            Select an API token in the header above to enable this playground.
          </div>
        )}

        {tokenRaw && (
          <>
            {/* Mode toggle */}
            <div style={{ display: 'flex', gap: 8 }}>
              {(['agent', 'group'] as Mode[]).map((m) => (
                <button
                  key={m}
                  onClick={() => setMode(m)}
                  style={{
                    padding: '0.35rem 0.875rem', borderRadius: 6, fontSize: '0.8125rem', fontWeight: 500,
                    border: mode === m ? '1px solid #534AB7' : '1px solid #e5e7eb',
                    background: mode === m ? '#EEEDFE' : '#fff',
                    color: mode === m ? '#534AB7' : '#6b7280',
                    cursor: 'pointer', transition: 'all 0.15s',
                  }}
                >
                  {m === 'agent' ? 'Single Agent' : 'Workflow'}
                </button>
              ))}
            </div>

            {/* Resource selector */}
            <div>
              <label style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: '#374151', marginBottom: 6 }}>
                {mode === 'agent' ? 'Agent' : 'Workflow'}
              </label>
              {fetching ? (
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: '0.875rem', color: '#9ca3af' }}>
                  <RefreshCw size={14} style={{ animation: 'spin 1s linear infinite' }} /> Loading…
                </div>
              ) : resources.length === 0 ? (
                <div style={{ padding: '0.875rem', background: '#fafafa', border: '1px solid #e5e7eb', borderRadius: 8, fontSize: '0.875rem' }}>
                  <span style={{ color: '#9ca3af' }}>No {mode === 'agent' ? 'agents' : 'workflows'} found in this workspace. </span>
                  <a
                    href={mode === 'agent' ? '/agents/new' : '/workflows/new'}
                    style={{ color: '#534AB7', fontWeight: 500, textDecoration: 'none' }}
                  >
                    <Plus size={12} style={{ display: 'inline', verticalAlign: 'middle' }} /> Create one
                  </a>
                </div>
              ) : (
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <select
                    value={selectedId}
                    onChange={(e) => setSelectedId(e.target.value)}
                    style={{
                      flex: 1, padding: '0.5rem 0.75rem', borderRadius: 8,
                      border: '1px solid #d1d5db', background: '#fff',
                      fontSize: '0.875rem', color: '#111827', cursor: 'pointer',
                      outline: 'none', appearance: 'auto',
                    }}
                  >
                    {resources.map((r) => (
                      <option key={r.id} value={r.id}>{r.name}</option>
                    ))}
                  </select>
                  <button
                    onClick={loadResources}
                    title="Refresh"
                    style={{ padding: '0.5rem', borderRadius: 8, border: '1px solid #e5e7eb', background: '#fff', cursor: 'pointer', color: '#6b7280' }}
                  >
                    <RefreshCw size={14} />
                  </button>
                </div>
              )}
              {selectedId && (
                <p style={{ marginTop: 4, fontSize: '0.75rem', color: '#9ca3af', fontFamily: 'monospace' }}>
                  ID: {selectedId}
                </p>
              )}
            </div>

            {/* Input */}
            <div>
              <label style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: '#374151', marginBottom: 6 }}>
                Input message
              </label>
              <textarea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                rows={3}
                style={{
                  width: '100%', padding: '0.625rem 0.875rem',
                  border: '1px solid #d1d5db', borderRadius: 8,
                  fontSize: '0.875rem', color: '#111827', resize: 'vertical',
                  outline: 'none', boxSizing: 'border-box',
                  fontFamily: 'inherit', lineHeight: 1.5,
                }}
              />
            </div>

            {/* Stream toggle */}
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', userSelect: 'none' }}>
              <input
                type="checkbox"
                checked={stream}
                onChange={(e) => setStream(e.target.checked)}
                style={{ accentColor: '#534AB7', width: 15, height: 15 }}
              />
              <span style={{ fontSize: '0.875rem', color: '#4b5563' }}>
                Streaming mode (SSE) — keep connection open for real-time deltas
              </span>
            </label>

            {/* Send */}
            <button
              onClick={handleSend}
              disabled={loading || !selectedId}
              style={{
                display: 'flex', alignItems: 'center', gap: 8,
                padding: '0.625rem 1.125rem', borderRadius: 8,
                background: loading || !selectedId ? '#9ca3af' : '#534AB7',
                color: '#fff', fontSize: '0.875rem', fontWeight: 600,
                border: 'none', cursor: loading || !selectedId ? 'not-allowed' : 'pointer',
                alignSelf: 'flex-start', transition: 'background 0.15s',
              }}
            >
              {loading ? <Loader2 size={15} style={{ animation: 'spin 1s linear infinite' }} /> : <Play size={15} />}
              Send Request
            </button>
          </>
        )}

        {/* Response */}
        {response && (
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
              <span style={{ fontSize: '0.8125rem', fontWeight: 500, color: '#374151' }}>Response</span>
              <span style={{
                fontSize: '0.75rem', fontWeight: 600, padding: '0.15em 0.5em', borderRadius: 4,
                background: response.status >= 200 && response.status < 300 ? '#f0fdf4' : '#fff1f2',
                color: response.status >= 200 && response.status < 300 ? '#166534' : '#be123c',
              }}>
                {response.status === 0 ? 'Network error' : `HTTP ${response.status}`}
              </span>
            </div>
            <pre style={{
              background: '#0d0d14', color: '#e2e8f0', borderRadius: 8,
              padding: '1rem 1.25rem', fontSize: '0.8125rem', fontFamily: 'monospace',
              lineHeight: 1.65, overflowX: 'auto', maxHeight: 320,
              margin: 0, border: '1px solid rgba(255,255,255,0.06)',
            }}>
              {JSON.stringify(response.data, null, 2)}
            </pre>
            {response.status === 202 && (
              <p style={{ marginTop: 8, fontSize: '0.8125rem', color: '#6b7280' }}>
                Run started. Use the <code style={{ background: '#EEEDFE', color: '#534AB7', padding: '0.1em 0.4em', borderRadius: 4 }}>run_id</code> above
                with <code style={{ background: '#EEEDFE', color: '#534AB7', padding: '0.1em 0.4em', borderRadius: 4 }}>GET /runs/:id</code> to poll for results.
              </p>
            )}
          </div>
        )}
      </div>

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
