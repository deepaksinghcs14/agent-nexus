'use client'

import { useState } from 'react'
import { Play, Loader2, ChevronDown, ChevronUp } from 'lucide-react'
import { useDocsStore } from '@/store/docs'

interface PathParam {
  name: string
  label: string
  placeholder?: string
}

interface ApiPlaygroundProps {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  path: string
  defaultBody?: Record<string, unknown>
  pathParams?: PathParam[]
  description?: string
}

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

const METHOD_COLOR: Record<string, string> = {
  GET: 'text-green-400 bg-green-400/10',
  POST: 'text-blue-400 bg-blue-400/10',
  PUT: 'text-amber-400 bg-amber-400/10',
  PATCH: 'text-orange-400 bg-orange-400/10',
  DELETE: 'text-red-400 bg-red-400/10',
}

export default function ApiPlayground({ method, path, defaultBody, pathParams = [], description }: ApiPlaygroundProps) {
  const tokenRaw = useDocsStore((s) => s.selectedTokenRaw)
  const [paramValues, setParamValues] = useState<Record<string, string>>(
    Object.fromEntries(pathParams.map((p) => [p.name, '']))
  )
  const [body, setBody] = useState(defaultBody ? JSON.stringify(defaultBody, null, 2) : '')
  const [bodyError, setBodyError] = useState('')
  const [response, setResponse] = useState<{ status: number; data: unknown } | null>(null)
  const [loading, setLoading] = useState(false)
  const [expanded, setExpanded] = useState(true)

  const resolvedPath = pathParams.reduce(
    (p, param) => p.replace(`{${param.name}}`, paramValues[param.name] || `{${param.name}}`),
    path
  )

  const handleSend = async () => {
    if (!tokenRaw) return
    setBodyError('')

    let parsedBody: unknown = undefined
    if (body.trim() && method !== 'GET' && method !== 'DELETE') {
      try {
        parsedBody = JSON.parse(body)
      } catch {
        setBodyError('Invalid JSON body')
        return
      }
    }

    setLoading(true)
    setResponse(null)
    try {
      const res = await fetch(`${API_URL}/api/v1${resolvedPath}`, {
        method,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${tokenRaw}`,
        },
        body: parsedBody !== undefined ? JSON.stringify(parsedBody) : undefined,
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
    <div className="my-6 rounded-lg border border-white/10 overflow-hidden">
      {/* Header */}
      <div
        className="flex items-center justify-between px-4 py-3 bg-white/3 cursor-pointer select-none"
        onClick={() => setExpanded((e) => !e)}
      >
        <div className="flex items-center gap-3">
          <span className={`text-xs font-bold px-2 py-0.5 rounded font-mono ${METHOD_COLOR[method]}`}>
            {method}
          </span>
          <code className="text-sm text-white/70 font-mono">{resolvedPath}</code>
        </div>
        <div className="flex items-center gap-2">
          {!tokenRaw && (
            <span className="text-xs text-white/30">select a token above to enable</span>
          )}
          {expanded ? <ChevronUp className="w-4 h-4 text-white/40" /> : <ChevronDown className="w-4 h-4 text-white/40" />}
        </div>
      </div>

      {expanded && (
        <div className="p-4 space-y-4 bg-black/20">
          {description && <p className="text-sm text-white/50">{description}</p>}

          {/* Path params */}
          {pathParams.length > 0 && (
            <div className="space-y-2">
              <p className="text-xs font-medium text-white/40 uppercase tracking-wider">Path Parameters</p>
              {pathParams.map((p) => (
                <div key={p.name} className="flex items-center gap-3">
                  <label className="w-32 text-sm text-white/60 font-mono shrink-0">{p.name}</label>
                  <input
                    type="text"
                    value={paramValues[p.name]}
                    onChange={(e) => setParamValues((v) => ({ ...v, [p.name]: e.target.value }))}
                    placeholder={p.placeholder ?? `Enter ${p.label}`}
                    className="flex-1 px-3 py-1.5 text-sm rounded border border-white/10 bg-black/30 text-white/80 placeholder-white/20 focus:outline-none focus:border-[#534AB7] font-mono"
                  />
                </div>
              ))}
            </div>
          )}

          {/* Body editor */}
          {method !== 'GET' && method !== 'DELETE' && body !== '' && (
            <div className="space-y-1">
              <p className="text-xs font-medium text-white/40 uppercase tracking-wider">Request Body</p>
              <textarea
                value={body}
                onChange={(e) => setBody(e.target.value)}
                rows={Math.min(12, body.split('\n').length + 1)}
                spellCheck={false}
                className="w-full px-3 py-2 text-sm rounded border border-white/10 bg-black/30 text-white/80 font-mono focus:outline-none focus:border-[#534AB7] resize-none"
              />
              {bodyError && <p className="text-xs text-red-400">{bodyError}</p>}
            </div>
          )}

          {/* Send button */}
          <button
            onClick={handleSend}
            disabled={!tokenRaw || loading}
            className="flex items-center gap-2 px-4 py-2 rounded-md bg-[#534AB7] hover:bg-[#4a42a3] text-white text-sm font-medium transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
            Send Request
          </button>

          {/* Response panel */}
          {response && (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <p className="text-xs font-medium text-white/40 uppercase tracking-wider">Response</p>
                <span
                  className={`text-xs font-mono px-2 py-0.5 rounded ${
                    response.status >= 200 && response.status < 300
                      ? 'bg-green-400/10 text-green-400'
                      : response.status === 0
                      ? 'bg-red-400/10 text-red-400'
                      : 'bg-amber-400/10 text-amber-400'
                  }`}
                >
                  {response.status === 0 ? 'Network Error' : `HTTP ${response.status}`}
                </span>
              </div>
              <pre className="p-3 rounded border border-white/10 bg-black/40 text-sm text-white/70 font-mono overflow-auto max-h-80 whitespace-pre-wrap">
                {JSON.stringify(response.data, null, 2)}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
