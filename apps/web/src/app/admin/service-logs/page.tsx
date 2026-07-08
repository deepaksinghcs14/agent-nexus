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
      return 'text-crit'
    case 'warn':
      return 'text-warn'
    case 'debug':
    case 'trace':
      return 'text-info'
    default:
      return 'text-good'
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
    <div className="p-4 sm:p-6 h-full flex flex-col min-h-0">
      <div className="flex flex-wrap items-start gap-3 justify-between mb-4">
        <div>
          <span className="eyebrow block mb-1">Admin</span>
          <h1 className="text-[22px] font-bold tracking-tight text-foreground">Service logs</h1>
          <p className="text-[11px] text-faint mt-0.5">{filtered.length.toLocaleString()} visible / {logs.length.toLocaleString()} captured</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setAutoScroll((v) => !v)}
            className={`inline-flex items-center gap-1.5 text-[12px] px-3 py-1.5 rounded-lg border ${
              autoScroll ? 'bg-accent text-white border-gray-900' : 'bg-surface text-muted-foreground border-border-strong'
            }`}
          >
            <ChevronDown className="w-3.5 h-3.5" />
            Auto-scroll
          </button>
          <button
            onClick={() => setLogs([])}
            className="inline-flex items-center gap-1.5 text-[12px] px-3 py-1.5 rounded-lg border border-border-strong bg-surface text-muted-foreground hover:border-border-strong"
          >
            <Trash2 className="w-3.5 h-3.5" />
            Clear
          </button>
          {running ? (
            <button
              onClick={stop}
              className="inline-flex items-center gap-1.5 text-[12px] px-3 py-1.5 rounded-lg bg-crit text-white hover:bg-crit"
            >
              <Square className="w-3.5 h-3.5" />
              Stop
            </button>
          ) : (
            <button
              onClick={() => setRunning(true)}
              className="inline-flex items-center gap-1.5 text-[12px] px-3 py-1.5 rounded-lg bg-good text-white hover:bg-good"
            >
              <Play className="w-3.5 h-3.5" />
              Start
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 mb-3">
        <div className="relative w-full sm:w-80">
          <Search className="w-3.5 h-3.5 text-faint absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search visible logs"
            className="w-full text-[12px] pl-8 pr-3 py-1.5 border border-border-strong rounded-lg bg-surface"
          />
        </div>
        <select value={source} onChange={(event) => setSource(event.target.value)} className="text-[12px] px-2.5 py-1.5 border border-border-strong rounded-lg bg-surface">
          {SOURCES.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select value={level} onChange={(event) => setLevel(event.target.value)} className="text-[12px] px-2.5 py-1.5 border border-border-strong rounded-lg bg-surface">
          {LEVELS.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <span className={`ml-auto text-[11px] px-2 py-1 rounded-full ${running ? 'bg-good/10 text-good' : 'bg-muted text-muted-foreground'}`}>
          {running ? 'streaming' : 'stopped'}
        </span>
      </div>

      {streamError && (
        <div className="text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg p-3 mb-3">
          {streamError}
        </div>
      )}

      <div className="flex-1 min-h-[520px] bg-[#0b0f14] border border-gray-900 rounded-lg overflow-auto font-mono text-[12px] leading-5">
        {filtered.length === 0 ? (
          <div className="h-full flex items-center justify-center text-muted-foreground">
            {running ? 'Waiting for log output…' : 'Start the stream to watch service logs.'}
          </div>
        ) : (
          <div className="py-3">
            {filtered.map((log, index) => (
              <div key={`${log.ts}-${index}`} className="grid grid-cols-[72px_120px_64px_minmax(0,1fr)] gap-3 px-4 hover:bg-white/[0.04]">
                <span className="text-muted-foreground">{formatTime(log.ts)}</span>
                <span className="text-sky-300 truncate">{log.source}</span>
                <span className={`uppercase ${levelClass(log.level)}`}>{log.level}</span>
                <span className="text-faint whitespace-pre-wrap break-words">
                  {log.message}
                  {log.attrs && Object.keys(log.attrs).length > 0 && (
                    <span className="text-muted-foreground"> {JSON.stringify(log.attrs)}</span>
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
