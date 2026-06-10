'use client'

import { useState, useMemo, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  Handle,
  Position,
  type Node,
  type Edge,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  Brain, Database, Sparkles, Wrench, Clock as ClockIcon, Check, AlertCircle,
  GitBranch, Layers, X, ChevronDown, ChevronRight, Zap,
} from 'lucide-react'
import { runsAPI, workflowsAPI } from '@/lib/api'
import { statusColor } from '@/lib/utils'
import type { Run, RunStep, StepType, WorkflowGraph, WorkflowNode, WorkflowEdge } from '@/types'

// ─── Colour / icon helpers ──────────────────────────────────────────────────

const STEP_STYLE: Record<StepType, { bg: string; border: string; icon: React.ReactNode; label: string }> = {
  memory_retrieval:  { bg: '#f5f3ff', border: '#a78bfa', icon: <Brain size={12} />,     label: 'Memory' },
  context_retrieval: { bg: '#eff6ff', border: '#60a5fa', icon: <Database size={12} />,  label: 'Context' },
  model_call:        { bg: '#eef2ff', border: '#6366f1', icon: <Sparkles size={12} />,  label: 'Model' },
  tool_call:         { bg: '#fff7ed', border: '#fb923c', icon: <Wrench size={12} />,    label: 'Tool' },
  mcp_call:          { bg: '#fff7ed', border: '#fb923c', icon: <Wrench size={12} />,    label: 'MCP' },
  approval_wait:     { bg: '#fefce8', border: '#facc15', icon: <ClockIcon size={12} />, label: 'Approval' },
  final_response:    { bg: '#f0fdf4', border: '#4ade80', icon: <Check size={12} />,     label: 'Final' },
  error:             { bg: '#fef2f2', border: '#f87171', icon: <AlertCircle size={12} />, label: 'Error' },
}

function stepLabel(step: RunStep): string {
  const base = STEP_STYLE[step.step_type]?.label ?? step.step_type
  return step.tool_name ? `${base}: ${step.tool_name}` : base
}

// ─── Custom node — single step ───────────────────────────────────────────────

type StepNodeData = { step: RunStep; selected: boolean }

function StepNode({ data }: NodeProps & { data: StepNodeData }) {
  const { step } = data
  const style = STEP_STYLE[step.step_type] ?? STEP_STYLE.model_call
  return (
    <>
      <Handle type="target" position={Position.Top} style={{ background: style.border, width: 7, height: 7 }} />
      <div style={{
        background: data.selected ? style.border : style.bg,
        border: `1.5px solid ${style.border}`,
        borderRadius: 8,
        padding: '6px 10px',
        minWidth: 160,
        cursor: 'pointer',
        boxShadow: data.selected ? `0 0 0 2px ${style.border}40` : 'none',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <span style={{ color: data.selected ? '#fff' : style.border }}>{style.icon}</span>
          <span style={{ fontSize: 11, fontWeight: 600, color: data.selected ? '#fff' : '#111827', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {stepLabel(step)}
          </span>
        </div>
        <div style={{ display: 'flex', gap: 6, marginTop: 3 }}>
          {step.latency_ms > 0 && (
            <span style={{ fontSize: 9, color: data.selected ? '#ffffff99' : '#6b7280' }}>{step.latency_ms}ms</span>
          )}
          {step.tokens_used > 0 && (
            <span style={{ fontSize: 9, color: data.selected ? '#ffffff99' : '#6b7280' }}>{step.tokens_used} tok</span>
          )}
          {step.error && (
            <span style={{ fontSize: 9, color: data.selected ? '#fca5a5' : '#ef4444' }}>error</span>
          )}
        </div>
      </div>
      <Handle type="source" position={Position.Bottom} style={{ background: style.border, width: 7, height: 7 }} />
    </>
  )
}

// ─── Custom node — workflow agent node (read-only trace overlay) ─────────────

type WorkflowTraceNodeData = {
  wfNode: WorkflowNode
  subRun?: Run
  agentName?: string
  selected: boolean
  onSelectNode: (id: string) => void
}

function WorkflowTraceNode({ data }: NodeProps & { data: WorkflowTraceNodeData }) {
  const { wfNode, subRun, agentName, selected } = data
  const status = subRun?.status ?? 'pending'

  const borderColor =
    status === 'success' ? '#4ade80' :
    status === 'failed'  ? '#f87171' :
    status === 'running' ? '#60a5fa' :
    selected             ? '#534AB7' : '#d1d5db'

  const bg = selected ? '#f1f0ff' : '#fff'

  const typeLabel: Record<string, string> = {
    start: 'Start', end: 'End', agent: agentName ?? 'Agent', supervisor: 'Supervisor',
    condition: 'Condition', parallel: 'Parallel', join: 'Join', loop: 'Loop',
  }

  const typeIcon: Record<string, React.ReactNode> = {
    start: <Zap size={11} color="#534AB7" />,
    end: <Check size={11} color="#22c55e" />,
    agent: <Sparkles size={11} color="#6366f1" />,
    supervisor: <Sparkles size={11} color="#534AB7" />,
    condition: <GitBranch size={11} color="#f59e0b" />,
    parallel: <Layers size={11} color="#3b82f6" />,
    join: <Layers size={11} color="#3b82f6" />,
    loop: <ClockIcon size={11} color="#8b5cf6" />,
  }

  return (
    <>
      <Handle type="target" position={Position.Left} style={{ background: borderColor, width: 8, height: 8 }} />
      <div style={{
        background: bg,
        border: `2px solid ${borderColor}`,
        borderRadius: 10,
        padding: '8px 12px',
        minWidth: 140,
        cursor: 'pointer',
        position: 'relative',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, marginBottom: 4 }}>
          {typeIcon[wfNode.node_type] ?? null}
          <span style={{ fontSize: 11, fontWeight: 700, color: '#111827', flex: 1 }}>
            {typeLabel[wfNode.node_type] ?? wfNode.node_type}
          </span>
          {subRun && (
            <span style={{
              fontSize: 8, fontWeight: 700, padding: '1px 5px', borderRadius: 999,
              textTransform: 'uppercase',
              ...statusBadgeStyle(status),
            }}>
              {status}
            </span>
          )}
        </div>
        {subRun && (
          <div style={{ display: 'flex', gap: 6 }}>
            {subRun.completed_at && (
              <span style={{ fontSize: 9, color: '#6b7280' }}>
                {Math.round((new Date(subRun.completed_at).getTime() - new Date(subRun.started_at).getTime()) / 1000 * 10) / 10}s
              </span>
            )}
            {(subRun.total_input_tokens + subRun.total_output_tokens) > 0 && (
              <span style={{ fontSize: 9, color: '#6b7280' }}>
                {subRun.total_input_tokens + subRun.total_output_tokens} tok
              </span>
            )}
          </div>
        )}
      </div>
      <Handle type="source" position={Position.Right} style={{ background: borderColor, width: 8, height: 8 }} />
    </>
  )
}

function statusBadgeStyle(s: string): React.CSSProperties {
  switch (s) {
    case 'success': return { background: '#dcfce7', color: '#166534' }
    case 'failed':  return { background: '#fee2e2', color: '#991b1b' }
    case 'running': return { background: '#dbeafe', color: '#1e40af' }
    default:        return { background: '#f3f4f6', color: '#6b7280' }
  }
}

// ─── JSON Viewer (collapsible) ────────────────────────────────────────────────

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  const [open, setOpen] = useState(false)
  const text = JSON.stringify(value, null, 2)
  if (!text || text === '{}' || text === 'null') return null
  return (
    <div style={{ marginTop: 8 }}>
      <button
        onClick={() => setOpen(v => !v)}
        style={{ display: 'flex', alignItems: 'center', gap: 4, background: 'none', border: 'none', cursor: 'pointer', padding: 0, fontSize: 11, fontWeight: 600, color: '#374151' }}
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        {label}
      </button>
      {open && (
        <pre style={{
          marginTop: 4, background: '#f8f8fb', border: '1px solid #e5e7eb',
          borderRadius: 6, padding: '8px 10px', fontSize: 10, color: '#374151',
          maxHeight: 160, overflowY: 'auto', overflowX: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-all',
        }}>
          {text}
        </pre>
      )}
    </div>
  )
}

// ─── Step detail panel ────────────────────────────────────────────────────────

function StepDetailPanel({ step, onClose }: { step: RunStep; onClose: () => void }) {
  const style = STEP_STYLE[step.step_type] ?? STEP_STYLE.model_call
  return (
    <div style={{
      position: 'absolute', bottom: 0, left: 0, right: 0,
      background: '#fff', borderTop: `2px solid ${style.border}`,
      padding: '12px 14px', borderRadius: '0 0 8px 8px',
      maxHeight: 260, overflowY: 'auto', zIndex: 10,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <span style={{ background: style.bg, border: `1px solid ${style.border}`, borderRadius: 5, padding: '2px 6px', fontSize: 10, fontWeight: 700, color: '#374151' }}>
            {step.step_type.replaceAll('_', ' ')}
          </span>
          {step.tool_name && <span style={{ fontSize: 11, color: '#6b7280' }}>{step.tool_name}</span>}
          {step.latency_ms > 0 && <span style={{ fontSize: 10, color: '#9ca3af' }}>{step.latency_ms}ms</span>}
          {step.tokens_used > 0 && <span style={{ fontSize: 10, color: '#9ca3af' }}>{step.tokens_used} tok</span>}
        </div>
        <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#9ca3af', padding: 2 }}>
          <X size={14} />
        </button>
      </div>
      {step.error && (
        <p style={{ fontSize: 11, color: '#dc2626', background: '#fef2f2', border: '1px solid #fca5a5', borderRadius: 5, padding: '4px 8px', marginBottom: 6 }}>
          {step.error}
        </p>
      )}
      <JsonBlock label="Input" value={step.input} />
      <JsonBlock label="Output" value={step.output} />
    </div>
  )
}

// ─── Sub-run steps panel (workflow mode click) ────────────────────────────────

function SubRunStepsPanel({ subRun, onClose }: { subRun: Run; onClose: () => void }) {
  const { data, isLoading } = useQuery({
    queryKey: ['run', subRun.id],
    queryFn: () => runsAPI.get(subRun.id) as Promise<{ run: Run; steps: RunStep[] }>,
  })
  const steps = data?.steps ?? []

  return (
    <div style={{
      position: 'absolute', bottom: 0, left: 0, right: 0,
      background: '#fff', borderTop: '2px solid #534AB7',
      padding: '12px 14px', borderRadius: '0 0 8px 8px',
      maxHeight: 280, overflowY: 'auto', zIndex: 10,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <span style={{ fontSize: 11, fontWeight: 700, color: '#374151' }}>
            {subRun.agent_id ? 'Agent steps' : 'Steps'}
          </span>
          <span style={{ ...statusBadgeStyle(subRun.status), fontSize: 9, fontWeight: 700, padding: '1px 5px', borderRadius: 999, textTransform: 'uppercase' as const }}>
            {subRun.status}
          </span>
        </div>
        <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#9ca3af', padding: 2 }}>
          <X size={14} />
        </button>
      </div>
      {isLoading && <p style={{ fontSize: 11, color: '#9ca3af', textAlign: 'center', padding: '8px 0' }}>Loading…</p>}
      {!isLoading && steps.length === 0 && <p style={{ fontSize: 11, color: '#9ca3af' }}>No steps recorded.</p>}
      {steps.map((step) => {
        const st = STEP_STYLE[step.step_type] ?? STEP_STYLE.model_call
        return (
          <div key={step.id} style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '5px 0', borderBottom: '1px solid #f3f4f6' }}>
            <span style={{ color: st.border }}>{st.icon}</span>
            <span style={{ fontSize: 11, color: '#374151', flex: 1 }}>{stepLabel(step)}</span>
            {step.latency_ms > 0 && <span style={{ fontSize: 9, color: '#9ca3af' }}>{step.latency_ms}ms</span>}
            {step.error && <span style={{ fontSize: 9, color: '#ef4444' }}>error</span>}
          </div>
        )
      })}
    </div>
  )
}

// ─── Main TraceGraph ──────────────────────────────────────────────────────────

export type RunWithWorkflow = Run & { steps?: RunStep[]; workflow_id?: string }

interface Props {
  run: RunWithWorkflow
  steps: RunStep[]
}

const NODE_TYPES = { stepNode: StepNode, workflowTraceNode: WorkflowTraceNode }

export function TraceGraph({ run, steps }: Props) {
  const isWorkflowRoot = !run.agent_id && !run.parent_run_id

  const [selectedStepId, setSelectedStepId] = useState<string | null>(null)
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)

  const { data: childrenData } = useQuery({
    queryKey: ['run-children', run.id],
    queryFn: () => runsAPI.listChildren(run.id) as Promise<{ data: Run[] }>,
    enabled: isWorkflowRoot,
  })
  const children = childrenData?.data ?? []

  const { data: graphData } = useQuery({
    queryKey: ['workflow-graph', run.workflow_id],
    queryFn: () => workflowsAPI.getGraph(run.workflow_id!) as Promise<WorkflowGraph>,
    enabled: isWorkflowRoot && !!run.workflow_id,
  })

  // ── Single-agent: build step nodes in a vertical chain ──────────────────
  const { singleNodes, singleEdges } = useMemo(() => {
    const nodes: Node[] = steps.map((step, i) => ({
      id: step.id,
      type: 'stepNode',
      position: { x: 40, y: i * 90 },
      data: { step, selected: selectedStepId === step.id },
      selectable: true,
    }))
    const edges: Edge[] = steps.slice(1).map((step, i) => ({
      id: `e-${steps[i].id}-${step.id}`,
      source: steps[i].id,
      target: step.id,
      style: { stroke: '#d1d5db', strokeWidth: 1.5 },
      animated: false,
    }))
    return { singleNodes: nodes, singleEdges: edges }
  }, [steps, selectedStepId])

  // ── Workflow: build nodes from saved positions + children overlay ────────
  const { wfNodes, wfEdges, agentNameMap } = useMemo(() => {
    if (!graphData) return { wfNodes: [], wfEdges: [], agentNameMap: {} }

    const childByNode: Record<string, Run> = {}
    for (const c of children) {
      if (c.workflow_node_id) childByNode[c.workflow_node_id] = c
    }

    const nodes: Node[] = graphData.nodes.map((n: WorkflowNode) => ({
      id: n.id,
      type: 'workflowTraceNode',
      position: { x: n.position_x, y: n.position_y },
      data: {
        wfNode: n,
        subRun: childByNode[n.id],
        agentName: (n.config as Record<string, string>)?.agent_name ?? undefined,
        selected: selectedNodeId === n.id,
      } as WorkflowTraceNodeData,
      selectable: true,
    }))

    const edges: Edge[] = graphData.edges.map((e: WorkflowEdge) => ({
      id: e.id,
      source: e.source_node_id,
      target: e.target_node_id,
      label: e.label ?? undefined,
      style: { stroke: '#d1d5db', strokeWidth: 1.5 },
      labelStyle: { fontSize: 9, fill: '#6b7280' },
    }))

    return { wfNodes: nodes, wfEdges: edges, agentNameMap: childByNode }
  }, [graphData, children, selectedNodeId])

  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    if (isWorkflowRoot) {
      setSelectedNodeId(id => id === node.id ? null : node.id)
      setSelectedStepId(null)
    } else {
      setSelectedStepId(id => id === node.id ? null : node.id)
      setSelectedNodeId(null)
    }
  }, [isWorkflowRoot])

  const selectedStep = steps.find(s => s.id === selectedStepId)
  const selectedChild = selectedNodeId ? children.find(c => c.workflow_node_id === selectedNodeId) : undefined
  const hasDetail = !!selectedStep || !!selectedChild

  const GRAPH_H = hasDetail ? 340 : 500

  if (!isWorkflowRoot && steps.length === 0) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: 200, gap: 8 }}>
        <p style={{ fontSize: 12, color: '#9ca3af' }}>No steps recorded for this run.</p>
      </div>
    )
  }

  const nodes = isWorkflowRoot ? wfNodes : singleNodes
  const edges = isWorkflowRoot ? wfEdges : singleEdges

  return (
    <div style={{ position: 'relative', height: hasDetail ? 600 : 500, borderRadius: 8, overflow: 'hidden', border: '1px solid #e5e7eb' }}>
      <div style={{ height: GRAPH_H }}>
        <ReactFlowProvider>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={NODE_TYPES}
          onNodeClick={onNodeClick}
          fitView
          fitViewOptions={{ padding: 0.3 }}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable={true}
          panOnDrag={true}
          zoomOnScroll={true}
          minZoom={0.2}
          maxZoom={2}
        >
          <Background color="#f0f0f5" gap={16} size={0.5} />
          <Controls showInteractive={false} style={{ bottom: 8, left: 8 }} />
        </ReactFlow>
        </ReactFlowProvider>
      </div>

      {selectedStep && (
        <StepDetailPanel step={selectedStep} onClose={() => setSelectedStepId(null)} />
      )}
      {selectedChild && (
        <SubRunStepsPanel subRun={selectedChild} onClose={() => setSelectedNodeId(null)} />
      )}
      {!hasDetail && isWorkflowRoot && children.length === 0 && (
        <div style={{
          position: 'absolute', bottom: 0, left: 0, right: 0,
          background: '#fff', borderTop: '1px solid #f3f4f6',
          padding: '8px 12px', fontSize: 11, color: '#9ca3af', textAlign: 'center',
        }}>
          Click an agent node to see its steps
        </div>
      )}
      {!hasDetail && !isWorkflowRoot && steps.length > 0 && (
        <div style={{
          position: 'absolute', bottom: 0, left: 0, right: 0,
          background: '#fff', borderTop: '1px solid #f3f4f6',
          padding: '8px 12px', fontSize: 11, color: '#9ca3af', textAlign: 'center',
        }}>
          Click a node to see step details
        </div>
      )}
    </div>
  )
}
