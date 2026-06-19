'use client'

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Timer } from 'lucide-react'
import { observabilityAPI } from '@/lib/api'
import type { LatencyData, LatencyByAgent, LatencyByModel, LatencyTrendDay } from '@/types'

function latencyColor(secs: number): string {
  if (secs < 2) return 'bg-green-100 text-green-700'
  if (secs < 5) return 'bg-amber-100 text-amber-700'
  return 'bg-red-100 text-red-700'
}

function successColor(rate: number): string {
  if (rate >= 95) return 'text-green-600'
  if (rate >= 80) return 'text-amber-600'
  return 'text-red-600'
}

function fmt(secs: number): string {
  return secs.toFixed(2) + 's'
}

function LatencyPill({ secs }: { secs: number }) {
  return (
    <span className={`text-[11px] px-1.5 py-0.5 rounded font-mono ${latencyColor(secs)}`}>
      {fmt(secs)}
    </span>
  )
}

const DAYS_OPTIONS = [7, 14, 30]

export default function ObservabilityPage() {
  const [days, setDays] = useState(7)

  const { data, isLoading, error } = useQuery({
    queryKey: ['observability-latency', days],
    queryFn: () => observabilityAPI.latency(days) as Promise<LatencyData>,
  })

  const byAgent: LatencyByAgent[] = data?.by_agent ?? []
  const byModel: LatencyByModel[] = data?.by_model ?? []
  const trend: LatencyTrendDay[] = data?.trend ?? []

  const maxP95 = Math.max(...trend.map((d) => d.p95_secs), 1)

  return (
    <div className="p-6 max-w-5xl">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Observability</h1>
          <p className="text-sm text-gray-500 mt-0.5">Latency and performance metrics for your agents</p>
        </div>
        <div className="flex gap-1">
          {DAYS_OPTIONS.map((d) => (
            <button
              key={d}
              onClick={() => setDays(d)}
              className={`text-[12px] px-3 py-1 rounded-md border transition-colors ${
                days === d
                  ? 'bg-gray-900 text-white border-gray-900'
                  : 'bg-white text-gray-500 border-gray-200 hover:border-gray-400'
              }`}
            >
              {d}d
            </button>
          ))}
        </div>
      </div>

      {error && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
          {(error as Error).message}
        </div>
      )}

      {isLoading && (
        <div className="py-12 text-center text-sm text-gray-400">Loading latency data…</div>
      )}

      {!isLoading && !error && byAgent.length === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl py-16 text-center">
          <Timer className="mx-auto text-gray-300 mb-3 w-8 h-8" />
          <p className="text-sm text-gray-500">No completed runs in this period.</p>
        </div>
      )}

      {!isLoading && byAgent.length > 0 && (
        <div className="space-y-8">
          {/* By Agent */}
          <section>
            <h2 className="text-[11px] font-medium text-gray-400 uppercase tracking-wider mb-3">By Agent</h2>
            <div className="border border-gray-100 rounded-xl overflow-hidden">
              <table className="w-full text-[13px]">
                <thead>
                  <tr className="bg-gray-50 border-b border-gray-100">
                    <th className="text-left px-4 py-2.5 font-medium text-gray-500 text-[11px]">Agent</th>
                    <th className="text-left px-4 py-2.5 font-medium text-gray-500 text-[11px]">Provider / Model</th>
                    <th className="text-center px-4 py-2.5 font-medium text-gray-500 text-[11px]">P50</th>
                    <th className="text-center px-4 py-2.5 font-medium text-gray-500 text-[11px]">P95</th>
                    <th className="text-center px-4 py-2.5 font-medium text-gray-500 text-[11px]">Success</th>
                    <th className="text-right px-4 py-2.5 font-medium text-gray-500 text-[11px]">Runs</th>
                    <th className="text-right px-4 py-2.5 font-medium text-gray-500 text-[11px]">Avg in tok</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50">
                  {byAgent.map((row) => {
                    const successRate = row.run_count > 0 ? (row.success_count / row.run_count) * 100 : 0
                    return (
                      <tr key={row.id} className="hover:bg-gray-50/50">
                        <td className="px-4 py-2.5 font-medium text-gray-900">{row.name}</td>
                        <td className="px-4 py-2.5 text-gray-500">
                          <span className="capitalize">{row.provider}</span>
                          <span className="text-gray-300 mx-1">/</span>
                          <span className="text-[12px] text-gray-400">{row.model}</span>
                        </td>
                        <td className="px-4 py-2.5 text-center">
                          <LatencyPill secs={row.p50_secs} />
                        </td>
                        <td className="px-4 py-2.5 text-center">
                          <LatencyPill secs={row.p95_secs} />
                        </td>
                        <td className={`px-4 py-2.5 text-center text-[12px] font-medium ${successColor(successRate)}`}>
                          {successRate.toFixed(0)}%
                        </td>
                        <td className="px-4 py-2.5 text-right text-gray-500">{row.run_count.toLocaleString()}</td>
                        <td className="px-4 py-2.5 text-right text-gray-400">{Math.round(row.avg_input_tokens).toLocaleString()}</td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </section>

          {/* By Model */}
          {byModel.length > 0 && (
            <section>
              <h2 className="text-[11px] font-medium text-gray-400 uppercase tracking-wider mb-3">By Model</h2>
              <div className="border border-gray-100 rounded-xl overflow-hidden">
                <table className="w-full text-[13px]">
                  <thead>
                    <tr className="bg-gray-50 border-b border-gray-100">
                      <th className="text-left px-4 py-2.5 font-medium text-gray-500 text-[11px]">Provider</th>
                      <th className="text-left px-4 py-2.5 font-medium text-gray-500 text-[11px]">Model</th>
                      <th className="text-center px-4 py-2.5 font-medium text-gray-500 text-[11px]">P50</th>
                      <th className="text-center px-4 py-2.5 font-medium text-gray-500 text-[11px]">P95</th>
                      <th className="text-center px-4 py-2.5 font-medium text-gray-500 text-[11px]">Tokens / sec</th>
                      <th className="text-right px-4 py-2.5 font-medium text-gray-500 text-[11px]">Runs</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-50">
                    {byModel.map((row) => (
                      <tr key={`${row.provider}/${row.model}`} className="hover:bg-gray-50/50">
                        <td className="px-4 py-2.5 capitalize text-gray-900">{row.provider}</td>
                        <td className="px-4 py-2.5 text-gray-500 text-[12px] font-mono">{row.model}</td>
                        <td className="px-4 py-2.5 text-center">
                          <LatencyPill secs={row.p50_secs} />
                        </td>
                        <td className="px-4 py-2.5 text-center">
                          <LatencyPill secs={row.p95_secs} />
                        </td>
                        <td className="px-4 py-2.5 text-center text-gray-500">{row.tokens_per_sec.toFixed(1)}</td>
                        <td className="px-4 py-2.5 text-right text-gray-500">{row.run_count.toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          )}

          {/* Daily Trend */}
          {trend.length > 0 && (
            <section>
              <h2 className="text-[11px] font-medium text-gray-400 uppercase tracking-wider mb-4">Daily Trend (P50 / P95)</h2>
              <div className="border border-gray-100 rounded-xl p-4">
                {/* Legend */}
                <div className="flex gap-4 mb-4 text-[11px] text-gray-400">
                  <span className="flex items-center gap-1.5">
                    <span className="w-2.5 h-2.5 rounded-sm bg-blue-400 inline-block" />
                    P50
                  </span>
                  <span className="flex items-center gap-1.5">
                    <span className="w-2.5 h-2.5 rounded-sm bg-purple-400 opacity-70 inline-block" />
                    P95
                  </span>
                </div>
                {/* Bars */}
                <div className="flex items-end gap-2 overflow-x-auto pb-2">
                  {trend.map((d) => (
                    <div key={d.day} className="flex flex-col items-center gap-0.5 min-w-[28px]">
                      <div className="flex items-end gap-0.5 h-24">
                        <div
                          title={`P50: ${fmt(d.p50_secs)} | ${d.run_count} runs`}
                          className="w-3 bg-blue-400 rounded-t cursor-default"
                          style={{ height: `${Math.max((d.p50_secs / maxP95) * 100, 2)}%` }}
                        />
                        <div
                          title={`P95: ${fmt(d.p95_secs)} | ${d.run_count} runs`}
                          className="w-3 bg-purple-400 rounded-t opacity-70 cursor-default"
                          style={{ height: `${Math.max((d.p95_secs / maxP95) * 100, 2)}%` }}
                        />
                      </div>
                      <span className="text-[9px] text-gray-400 whitespace-nowrap">
                        {new Date(d.day + 'T00:00:00').toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                      </span>
                    </div>
                  ))}
                </div>
                {/* Y-axis hint */}
                <p className="text-[10px] text-gray-300 mt-1">
                  Max: {fmt(maxP95)}
                </p>
              </div>
            </section>
          )}
        </div>
      )}
    </div>
  )
}
