'use client'

import { useState } from 'react'
import { Play } from 'lucide-react'
import { mcpAPI } from '@/lib/api'

// buildSkeleton turns a JSON Schema's top-level properties into a starter
// input object — one key per property, defaulted by declared type. Anything
// nested (arrays of objects, oneOf, etc.) is left to the user, who has the
// schema itself shown alongside the textarea.
function buildSkeleton(schema: Record<string, unknown> | undefined): string {
  const props = (schema?.properties ?? {}) as Record<string, { type?: string }>
  const skeleton: Record<string, unknown> = {}
  for (const [key, def] of Object.entries(props)) {
    switch (def?.type) {
      case 'number': case 'integer': skeleton[key] = 0; break
      case 'boolean': skeleton[key] = false; break
      case 'array': skeleton[key] = []; break
      case 'object': skeleton[key] = {}; break
      default: skeleton[key] = ''
    }
  }
  return JSON.stringify(skeleton, null, 2)
}

export function McpToolTester({ serverId, name, riskLevel, inputSchema }: {
  serverId: string
  name: string
  riskLevel: string
  inputSchema?: Record<string, unknown>
}) {
  const [input, setInput] = useState(() => buildSkeleton(inputSchema))
  const [result, setResult] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  const run = async () => {
    if ((riskLevel === 'high' || riskLevel === 'critical') &&
        !confirm(`"${name}" is a ${riskLevel}-risk tool and this call runs against the real MCP server — continue?`)) {
      return
    }
    setRunning(true)
    setResult(null)
    try {
      const parsed = JSON.parse(input)
      const res = await mcpAPI.testTool(serverId, name, parsed)
      setResult(res.error
        ? `Error: ${res.error}`
        : `${JSON.stringify(res.output, null, 2)}\n\n${res.latency_ms}ms`)
    } catch (e) {
      setResult(`Error: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="border border-border rounded-lg p-4 bg-muted">
      <p className="text-[11px] font-medium text-muted-foreground mb-2">Test this tool</p>
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div>
          <label className="block text-[10px] text-muted-foreground mb-1">Arguments (JSON)</label>
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            rows={6}
            className="w-full text-[12px] font-mono px-3 py-2 border border-border-strong rounded-lg bg-surface resize-none"
          />
        </div>
        <div>
          <label className="block text-[10px] text-muted-foreground mb-1">Input schema</label>
          <pre className="h-[132px] text-[11px] font-mono px-3 py-2 border border-border-strong rounded-lg bg-surface overflow-auto whitespace-pre-wrap text-faint">
            {inputSchema ? JSON.stringify(inputSchema, null, 2) : '—'}
          </pre>
        </div>
        <div>
          <label className="block text-[10px] text-muted-foreground mb-1">Result</label>
          <pre className={`h-[132px] text-[12px] font-mono px-3 py-2 border rounded-lg overflow-auto whitespace-pre-wrap ${
            result?.startsWith('Error:')
              ? 'bg-crit/10 border-crit/30 text-crit dark:text-crit'
              : 'bg-surface border-border-strong text-foreground'
          }`}>
            {result ?? <span className="text-faint">— run test to see output —</span>}
          </pre>
        </div>
      </div>
      <button
        onClick={run}
        disabled={running}
        className="mt-2 inline-flex items-center gap-1.5 px-3 py-1.5 bg-accent hover:bg-accent-hover text-white text-[12px] rounded-lg disabled:opacity-50 transition-colors"
      >
        <Play size={11} /> {running ? 'Running…' : 'Run test'}
      </button>
    </div>
  )
}
