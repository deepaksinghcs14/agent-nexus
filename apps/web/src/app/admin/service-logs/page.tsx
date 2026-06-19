'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronDown, Play, Search, Square, Trash2 } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import type { ServiceLogEntry } from '@/types'

const MAX_ROWS = 1500
const LEVELS = ['all', 'trace', 'debug', 'info', 'warn', 'error', 'fatal']
const SOURCES = ['all', 'api', 'whatsapp-adapter']

function levelClass(level: string) {
  switch (level.toLowerCase()) {
    case 'fatal':
    case 'error':
      return 'text-red-300'
    case 'warn':
      return 'text-amber-300'
    case 'debug':
    case 'trace':
      return 'text-cyan-300'
    default:
      return 'text-emerald-300'
  }
}

function formatTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--:--:--'
  return date.toLocaleTimeString(undefined, { hour12: false })
}

function logText(log: ServiceLogEntry) {
  const attrs = log.attrs && Object.keys(log.attrs).length > 0 ? ` ${JSON.stringify(log.attrs)}` : ''
  return `${log.ts} ${log.source} ${log.level} ${log.message}${attrs}`.toLowerCase()
}

export default function AdminServiceLogsPage() {
  const [logs, setLogs] = useState<ServiceLogEntry[]>([])
  const [running, setRunning] = useState(false)
  const [source, setSource] = useState('all')
  const [level, setLevel] = useState('all')
  const [query, setQuery] = useState('')
  const [autoScroll, setAutoScroll] = useState(true)
  const [streamError, setStreamError] = useState('')
  const eventSourceRef = useRef<EventSource | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return logs.filter((log) => {
      if (source !== 'all' && log.source !== source) return false
      if (level !== 'all' && log.level.toLowerCase() !== level) return false
      if (q && !logText(log).includes(q)) return false
      return true
    })
  }, [logs, source, level, query])

  useEffect(() => {
    if (autoScroll) bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [filtered.length, autoScroll])

  useEffect(() => {
    if (!running) return

    const stream = adminAPI.serviceLogStream()
    eventSourceRef.current = stream
    setStreamError('')

    stream.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data) as ServiceLogEntry
        setLogs((current) => [...current, parsed].slice(-MAX_ROWS))
      } catch {
        setLogs((current) => [
          ...current,
          {
            ts: new Date().toISOString(),
            source: 'console',
            level: 'warn',
            message: 'Unable to parse log event',
            attrs: { data: event.data },
          },
        ].slice(-MAX_ROWS))
      }
    }

    stream.onerror = () => {
      setStreamError('Stream disconnected')
      setRunning(false)
    }

    return () => {
      stream.close()
      if (eventSourceRef.current === stream) eventSourceRef.current = null
    }
  }, [running])

  const stop = () => {
    eventSourceRef.current?.close()
    eventSourceRef.current = null
    setRunning(false)
  }

  return (
    <div className="p-6 h-full flex flex-col min-h-0">
      <div className="flex items-center justify-between mb-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Service logs</h1>
          <p className="text-[11px] text-gray-400 mt-0.5">{filtered.length.toLocaleString()} visible / {logs.length.toLocaleString()} captured</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setAutoScroll((v) => !v)}
            className={`inline-flex items-center gap-1.5 text-[12px] px-3 py-1.5 rounded-lg border ${
              autoScroll ? 'bg-gray-900 text-white border-gray-900' : 'bg-white text-gray-600 border-gray-200'
            }`}
          >
            <ChevronDown className="w-3.5 h-3.5" />
            Auto-scroll
          </button>
          <button
            onClick={() => setLogs([])}
            className="inline-flex items-center gap-1.5 text-[12px] px-3 py-1.5 rounded-lg border border-gray-200 bg-white text-gray-600 hover:border-gray-300"
          >
            <Trash2 className="w-3.5 h-3.5" />
            Clear
          </button>
          {running ? (
            <button
              onClick={stop}
              className="inline-flex items-center gap-1.5 text-[12px] px-3 py-1.5 rounded-lg bg-red-600 text-white hover:bg-red-700"
            >
              <Square className="w-3.5 h-3.5" />
              Stop
            </button>
          ) : (
            <button
              onClick={() => setRunning(true)}
              className="inline-flex items-center gap-1.5 text-[12px] px-3 py-1.5 rounded-lg bg-emerald-600 text-white hover:bg-emerald-700"
            >
              <Play className="w-3.5 h-3.5" />
              Start
            </button>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2 mb-3">
        <div className="relative w-80">
          <Search className="w-3.5 h-3.5 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search visible logs"
            className="w-full text-[12px] pl-8 pr-3 py-1.5 border border-gray-200 rounded-lg bg-white"
          />
        </div>
        <select value={source} onChange={(event) => setSource(event.target.value)} className="text-[12px] px-2.5 py-1.5 border border-gray-200 rounded-lg bg-white">
          {SOURCES.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select value={level} onChange={(event) => setLevel(event.target.value)} className="text-[12px] px-2.5 py-1.5 border border-gray-200 rounded-lg bg-white">
          {LEVELS.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <span className={`ml-auto text-[11px] px-2 py-1 rounded-full ${running ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-500'}`}>
          {running ? 'streaming' : 'stopped'}
        </span>
      </div>

      {streamError && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-3">
          {streamError}
        </div>
      )}

      <div className="flex-1 min-h-[520px] bg-[#0b0f14] border border-gray-900 rounded-lg overflow-auto font-mono text-[12px] leading-5">
        {filtered.length === 0 ? (
          <div className="h-full flex items-center justify-center text-gray-500">
            {running ? 'Waiting for log output…' : 'Start the stream to watch service logs.'}
          </div>
        ) : (
          <div className="py-3">
            {filtered.map((log, index) => (
              <div key={`${log.ts}-${index}`} className="grid grid-cols-[72px_120px_64px_minmax(0,1fr)] gap-3 px-4 hover:bg-white/[0.04]">
                <span className="text-gray-500">{formatTime(log.ts)}</span>
                <span className="text-sky-300 truncate">{log.source}</span>
                <span className={`uppercase ${levelClass(log.level)}`}>{log.level}</span>
                <span className="text-gray-200 whitespace-pre-wrap break-words">
                  {log.message}
                  {log.attrs && Object.keys(log.attrs).length > 0 && (
                    <span className="text-gray-500"> {JSON.stringify(log.attrs)}</span>
                  )}
                </span>
              </div>
            ))}
            <div ref={bottomRef} />
          </div>
        )}
      </div>
    </div>
  )
}
