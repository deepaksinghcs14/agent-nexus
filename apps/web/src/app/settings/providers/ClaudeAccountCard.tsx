'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Terminal, Unplug } from 'lucide-react'
import { runnerCredsAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'

type RunnerCreds = { connected: boolean; updated_at?: string }

// Claude account for repo coding sessions (Jira→PR pipeline). Authenticates
// the runner's headless Claude Code against a subscription instead of API
// credits: the user runs `claude setup-token` once locally (same browser
// login as the CLI) and pastes the resulting token here.
export function ClaudeAccountCard() {
  const queryClient = useQueryClient()
  const [token, setToken] = useState('')
  const [error, setError] = useState('')

  const { data } = useQuery({
    queryKey: ['runner-credentials'],
    queryFn: () => runnerCredsAPI.get() as Promise<RunnerCreds>,
  })

  const save = useMutation({
    mutationFn: () => runnerCredsAPI.put(token.trim()),
    onSuccess: () => {
      setToken('')
      setError('')
      queryClient.invalidateQueries({ queryKey: ['runner-credentials'] })
    },
    onError: (err: Error) => setError(err.message),
  })
  const disconnect = useMutation({
    mutationFn: () => runnerCredsAPI.delete(),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['runner-credentials'] }),
  })

  const connected = data?.connected ?? false

  return (
    <div className="border border-amber-100 bg-amber-50/60 rounded-xl px-4 py-3 mb-4">
      <div className="flex flex-wrap items-center gap-3 justify-between">
        <div className="flex items-center gap-2.5">
          <Terminal size={16} className="text-amber-700" />
          <div>
            <p className="text-sm font-medium text-amber-900">Claude account — repo coding sessions</p>
            {connected ? (
              <p className="text-xs text-green-700 flex items-center gap-1">
                <Check size={11} /> Connected · subscription billing
                {data?.updated_at && <span className="text-amber-600/70">· updated {relativeTime(data.updated_at)}</span>}
              </p>
            ) : (
              <p className="text-xs text-amber-700">
                Run <code className="bg-amber-100 px-1 rounded">claude setup-token</code> in your terminal (opens the
                usual Claude login), then paste the token below
              </p>
            )}
          </div>
        </div>
        {connected && (
          <button
            onClick={() => disconnect.mutate()}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 border border-amber-200 text-xs text-amber-800 rounded-lg hover:bg-amber-100"
          >
            <Unplug size={12} /> Disconnect
          </button>
        )}
      </div>

      {!connected && (
        <div className="flex flex-wrap items-center gap-2 mt-2.5">
          <input
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder="sk-ant-oat…"
            className="flex-1 min-w-[220px] px-2.5 py-1.5 border border-amber-200 rounded-lg text-xs font-mono bg-white focus:outline-none focus:ring-1 focus:ring-amber-400"
          />
          <button
            onClick={() => save.mutate()}
            disabled={!token.trim() || save.isPending}
            className="px-3 py-1.5 bg-amber-700 text-white text-xs rounded-lg font-medium disabled:opacity-50"
          >
            {save.isPending ? 'Saving…' : 'Connect'}
          </button>
        </div>
      )}
      {error && <p className="text-xs text-red-600 mt-2">{error}</p>}
      <p className="text-[11px] text-amber-700/70 mt-2">
        Used only to authenticate autonomous coding sessions in the runner. Stored encrypted; never shown again.
        Without it, the runner falls back to its own ANTHROPIC_API_KEY.
      </p>
    </div>
  )
}
