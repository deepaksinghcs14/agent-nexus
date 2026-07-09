'use client'

// Workflow Studio — visual builder for workflow graphs.
// - @xyflow/react v12 canvas, house design tokens (Paper & Phosphor)
// - workflowsAPI.getGraph / saveGraph hit /api/v1/workflows/:id/graph
// - invokeAPI.workflow hits /api/v1/invoke/workflows/:id with { input, stream: true }
// - SSE node events carry node_id, node_name, node_type, result fields

import React, { use, useCallback, useEffect, useMemo, useRef, useState, DragEvent } from 'react'
import { createPortal } from 'react-dom'
import { useRouter } from 'next/navigation'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ReactFlow,
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  addEdge,
  useNodesState,
  useEdgesState,
  ReactFlowProvider,
  Handle,
  Position,
  MarkerType,
  type Node,
  type Edge,
  type Connection,
  type ReactFlowInstance,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  ArrowLeft,
  Save,
  Play,
  GitBranch,
  GitMerge,
  RefreshCcw,
  X,
  ChevronRight,
  Loader2,
  LayoutTemplate,
  BookOpen,
  Zap,
  Crown,
  Bot,
  Wrench,
  Webhook as WebhookIcon,
  MessagesSquare,
  Split,
  Flag,
  CircleDot,
  Send,
} from 'lucide-react'
import { TriggersPanel } from './TriggersPanel'
import { workflowsAPI, agentsAPI, invokeAPI, toolsAPI, gatewayAPI, providersAPI } from '@/lib/api'
import type { Workflow, Agent, Tool, GatewayChannel, WorkflowGraph, WorkflowNode, WorkflowEdge, WorkflowNodeType, SSEEvent } from '@/types'

// ---------------------------------------------------------------------------
// Node theme — one accent per node type, used for handles, icons, minimap.
// Hex (not CSS vars) because React Flow needs concrete values in JS.
// ---------------------------------------------------------------------------
const NODE_THEME: Record<string, { accent: string; icon: React.ElementType; title: string; blurb: string }> = {
  start:      { accent: '#10b981', icon: Play,           title: 'Start',      blurb: 'Entry point of every run' },
  end:        { accent: '#f43f5e', icon: Flag,           title: 'End',        blurb: 'Finish; optionally deliver the result' },
  agent:      { accent: '#534AB7', icon: Bot,            title: 'Agent',      blurb: 'Run an agent on the current input' },
  supervisor: { accent: '#d97706', icon: Crown,          title: 'Supervisor', blurb: 'Coordinates team agents as tools' },
  condition:  { accent: '#f59e0b', icon: Split,          title: 'Condition',  blurb: 'Branch on the previous output' },
  parallel:   { accent: '#64748b', icon: GitBranch,      title: 'Parallel',   blurb: 'Fan out into concurrent branches' },
  join:       { accent: '#64748b', icon: GitMerge,       title: 'Join',       blurb: 'Merge parallel branches' },
  loop:       { accent: '#8b5cf6', icon: RefreshCcw,     title: 'Loop',       blurb: 'Repeat until a condition is met' },
  tool:       { accent: '#14b8a6', icon: Wrench,         title: 'Tool',       blurb: 'Call one workspace tool directly' },
  webhook:    { accent: '#0ea5e9', icon: WebhookIcon,    title: 'Webhook',    blurb: 'POST the output to an external URL' },
  gateway:    { accent: '#ec4899', icon: MessagesSquare, title: 'Gateway',    blurb: 'Send the output as a chat message' },
}

// ---------------------------------------------------------------------------
// Type augmentation for node data
// ---------------------------------------------------------------------------
interface NodeData extends Record<string, unknown> {
  label: string
  node_type: WorkflowNodeType
  agent_id?: string | null
  agent_name?: string
  agent_model?: string
  agent_provider?: string
  config: Record<string, unknown>
  status?: 'idle' | 'running' | 'done' | 'error'
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------
function toRFNode(n: WorkflowNode, agents: Agent[]): Node<NodeData> {
  const agent = agents.find((a) => a.id === n.agent_id)
  return {
    id: n.id,
    type: n.node_type,
    position: { x: n.position_x, y: n.position_y },
    data: {
      label: (n.config.label as string) || agent?.name || n.node_type,
      agent_id: n.agent_id,
      agent_name: agent?.name,
      agent_model: agent?.model,
      agent_provider: agent?.provider,
      config: n.config,
      node_type: n.node_type,
      status: 'idle',
    },
  }
}

function edgeColor(label?: string | null) {
  if (label === 'yes') return '#10b981'
  if (label === 'no') return '#f43f5e'
  if (label === 'loop') return '#8b5cf6'
  if (label === 'exit') return '#10b981'
  if (label === 'delegate') return '#d97706'
  return '#94a3b8'
}

function labelToSourceHandle(label?: string | null): string | undefined {
  if (label === 'yes') return 'yes'
  if (label === 'no') return 'no'
  if (label === 'loop') return 'continue'
  if (label === 'exit') return 'exit'
  if (label === 'delegate') return 'delegate'
  return undefined
}

// mkEdgeProps centralises the visual treatment of an edge for a given label
// so every creation path (load, connect, template, relabel) looks identical.
function mkEdgeProps(label?: string | null): Partial<Edge> {
  const color = edgeColor(label)
  const isDelegate = label === 'delegate'
  return {
    type: 'smoothstep',
    animated: true,
    zIndex: 1000,
    label: label || undefined,
    markerEnd: { type: MarkerType.ArrowClosed, color, width: 16, height: 16 },
    style: { stroke: color, strokeWidth: isDelegate ? 1.5 : 2, strokeDasharray: isDelegate ? '6 3' : undefined, opacity: 0.9 },
    labelStyle: { fontSize: 10, fill: color, fontWeight: 700, fontFamily: 'var(--font-mono, monospace)' },
    labelBgStyle: { fill: 'var(--wf-label-bg)', fillOpacity: 0.95 },
    labelBgPadding: [4, 2] as [number, number],
    labelBgBorderRadius: 4,
  }
}

function toRFEdge(e: WorkflowEdge): Edge {
  return {
    id: e.id || `e-${e.source_node_id}-${e.target_node_id}`,
    source: e.source_node_id,
    target: e.target_node_id,
    sourceHandle: labelToSourceHandle(e.label),
    ...mkEdgeProps(e.label),
  }
}

// ---------------------------------------------------------------------------
// Custom node components
// ---------------------------------------------------------------------------

const HANDLE_CLS = '!w-3 !h-3 !border-2 !border-[var(--wf-label-bg)]'

function statusRing(status?: string) {
  if (status === 'running') return 'ring-2 ring-accent/60 shadow-[0_0_0_4px_rgba(83,74,183,0.12)]'
  if (status === 'done') return 'ring-2 ring-good/60'
  if (status === 'error') return 'ring-2 ring-crit/60'
  return ''
}

function StatusPip({ status }: { status?: string }) {
  if (status === 'done') {
    return (
      <span className="absolute -top-2 -right-2 grid h-5 w-5 place-items-center rounded-full bg-good text-[10px] font-bold text-white shadow">✓</span>
    )
  }
  if (status === 'running') {
    return (
      <span className="absolute -top-2 -right-2 grid h-5 w-5 place-items-center rounded-full bg-accent shadow">
        <Loader2 size={11} className="animate-spin text-white" />
      </span>
    )
  }
  if (status === 'error') {
    return (
      <span className="absolute -top-2 -right-2 grid h-5 w-5 place-items-center rounded-full bg-crit text-[10px] font-bold text-white shadow">!</span>
    )
  }
  return null
}

// Shared card body for rectangular nodes.
function NodeCard({
  type, title, subtitle, status, selected, children, minWidth = 168,
}: {
  type: string; title: string; subtitle?: React.ReactNode; status?: string; selected?: boolean
  children?: React.ReactNode; minWidth?: number
}) {
  const theme = NODE_THEME[type] ?? NODE_THEME.agent
  const Icon = theme.icon
  return (
    <div
      className={`relative rounded-xl border bg-surface shadow-card transition-shadow ${statusRing(status)} ${selected ? 'border-accent/60' : 'border-border'}`}
      style={{ minWidth }}
    >
      <StatusPip status={status} />
      <div className="flex items-center gap-2.5 px-3.5 py-2.5">
        <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg" style={{ background: `${theme.accent}18`, color: theme.accent }}>
          <Icon size={14} />
        </span>
        <div className="min-w-0">
          <div className="truncate text-xs font-semibold text-foreground">{title}</div>
          {subtitle && <div className="mt-0.5 truncate text-[10px] text-muted-foreground">{subtitle}</div>}
        </div>
      </div>
      {children}
    </div>
  )
}

function StartNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  const theme = NODE_THEME.start
  const hasDelivery = false
  void hasDelivery
  return (
    <div className={`relative flex items-center gap-2 rounded-full border bg-surface py-2 pl-2.5 pr-4 shadow-card ${statusRing(d.status)} ${selected ? 'border-accent/60' : 'border-border'}`}>
      <StatusPip status={d.status} />
      <span className="grid h-7 w-7 place-items-center rounded-full" style={{ background: `${theme.accent}18`, color: theme.accent }}>
        <Play size={13} />
      </span>
      <span className="text-xs font-semibold text-foreground">Start</span>
      <Handle type="source" position={Position.Right} className={HANDLE_CLS} style={{ background: theme.accent }} />
    </div>
  )
}

function EndNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  const theme = NODE_THEME.end
  const cfg = d.config || {}
  const deliversWebhook = !!(cfg.webhook_url as string)
  const deliversGateway = !!(cfg.gateway_channel_id as string)
  return (
    <div className={`relative flex items-center gap-2 rounded-full border bg-surface py-2 pl-2.5 pr-4 shadow-card ${statusRing(d.status)} ${selected ? 'border-accent/60' : 'border-border'}`}>
      <StatusPip status={d.status} />
      <span className="grid h-7 w-7 place-items-center rounded-full" style={{ background: `${theme.accent}18`, color: theme.accent }}>
        <Flag size={13} />
      </span>
      <span className="text-xs font-semibold text-foreground">End</span>
      {(deliversWebhook || deliversGateway) && (
        <span className="flex items-center gap-1 text-muted-foreground">
          {deliversWebhook && <WebhookIcon size={11} style={{ color: NODE_THEME.webhook.accent }} />}
          {deliversGateway && <MessagesSquare size={11} style={{ color: NODE_THEME.gateway.accent }} />}
        </span>
      )}
      <Handle type="target" position={Position.Left} className={HANDLE_CLS} style={{ background: theme.accent }} />
    </div>
  )
}

function AgentNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  return (
    <NodeCard
      type="agent"
      title={d.label}
      status={d.status}
      selected={selected}
      subtitle={d.agent_model
        ? <span className="font-mono">{d.agent_model}{d.agent_provider ? ` · ${d.agent_provider}` : ''}</span>
        : <span className="text-warn">no agent assigned</span>}
    >
      <Handle type="target" position={Position.Left} className={HANDLE_CLS} style={{ background: NODE_THEME.agent.accent }} />
      <Handle type="source" position={Position.Right} className={HANDLE_CLS} style={{ background: NODE_THEME.agent.accent }} />
    </NodeCard>
  )
}

function SupervisorNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  return (
    <NodeCard
      type="supervisor"
      title={d.label}
      status={d.status}
      selected={selected}
      subtitle={d.agent_model
        ? <span className="font-mono">{d.agent_model}{d.agent_provider ? ` · ${d.agent_provider}` : ''}</span>
        : <span className="text-warn">no agent assigned</span>}
    >
      <Handle type="target" position={Position.Left} className={HANDLE_CLS} style={{ background: NODE_THEME.supervisor.accent }} />
      <Handle type="source" id="forward" position={Position.Right} className={HANDLE_CLS} style={{ background: NODE_THEME.supervisor.accent }} />
      <Handle type="source" id="delegate" position={Position.Bottom} className={HANDLE_CLS} style={{ background: '#92400e' }} />
      <div className="pointer-events-none absolute -bottom-5 left-1/2 -translate-x-1/2 whitespace-nowrap font-mono text-[9px] font-bold" style={{ color: NODE_THEME.supervisor.accent }}>
        delegate ↓
      </div>
    </NodeCard>
  )
}

function ConditionNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  const expr = (d.config?.expression as string) || 'no expression'
  return (
    <NodeCard type="condition" title={d.label === 'condition' ? 'Condition' : d.label} status={d.status} selected={selected}
      subtitle={<span className="font-mono">{expr}</span>} minWidth={160}>
      <Handle type="target" position={Position.Left} className={HANDLE_CLS} style={{ background: NODE_THEME.condition.accent }} />
      <Handle type="source" id="yes" position={Position.Right} className={HANDLE_CLS} style={{ background: '#10b981' }} />
      <Handle type="source" id="no" position={Position.Bottom} className={HANDLE_CLS} style={{ background: '#f43f5e' }} />
      <div className="pointer-events-none absolute -right-7 top-1/2 -translate-y-1/2 font-mono text-[9px] font-bold text-good">yes</div>
      <div className="pointer-events-none absolute -bottom-4.5 left-1/2 -translate-x-1/2 font-mono text-[9px] font-bold text-crit" style={{ bottom: -16 }}>no</div>
    </NodeCard>
  )
}

function ParallelNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  return (
    <NodeCard type="parallel" title="Parallel" subtitle="fan out" status={d.status} selected={selected} minWidth={132}>
      <Handle type="target" position={Position.Left} className={HANDLE_CLS} style={{ background: NODE_THEME.parallel.accent }} />
      <Handle type="source" id="s1" position={Position.Right} className={HANDLE_CLS} style={{ top: '30%', background: NODE_THEME.parallel.accent }} />
      <Handle type="source" id="s2" position={Position.Right} className={HANDLE_CLS} style={{ top: '55%', background: NODE_THEME.parallel.accent }} />
      <Handle type="source" id="s3" position={Position.Right} className={HANDLE_CLS} style={{ top: '80%', background: NODE_THEME.parallel.accent }} />
    </NodeCard>
  )
}

function JoinNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  return (
    <NodeCard type="join" title="Join" subtitle="merge branches" status={d.status} selected={selected} minWidth={132}>
      <Handle type="target" id="t1" position={Position.Left} className={HANDLE_CLS} style={{ top: '30%', background: NODE_THEME.join.accent }} />
      <Handle type="target" id="t2" position={Position.Left} className={HANDLE_CLS} style={{ top: '55%', background: NODE_THEME.join.accent }} />
      <Handle type="target" id="t3" position={Position.Left} className={HANDLE_CLS} style={{ top: '80%', background: NODE_THEME.join.accent }} />
      <Handle type="source" position={Position.Right} className={HANDLE_CLS} style={{ background: NODE_THEME.join.accent }} />
    </NodeCard>
  )
}

function LoopNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  const expr = (d.config?.exit_condition as string) || 'no exit condition'
  const maxIter = (d.config?.max_iterations as number) || 5
  return (
    <NodeCard type="loop" title="Loop" status={d.status} selected={selected}
      subtitle={<span className="font-mono">{expr} · max {maxIter}</span>} minWidth={160}>
      <Handle type="target" position={Position.Top} className={HANDLE_CLS} style={{ background: NODE_THEME.loop.accent }} />
      <Handle type="source" id="continue" position={Position.Left} className={HANDLE_CLS} style={{ background: NODE_THEME.loop.accent }} />
      <Handle type="source" id="exit" position={Position.Right} className={HANDLE_CLS} style={{ background: '#10b981' }} />
      <div className="pointer-events-none absolute -left-8 top-1/2 -translate-y-1/2 font-mono text-[9px] font-bold" style={{ color: NODE_THEME.loop.accent }}>↩ loop</div>
      <div className="pointer-events-none absolute -right-8 top-1/2 -translate-y-1/2 font-mono text-[9px] font-bold text-good">exit →</div>
    </NodeCard>
  )
}

function ToolNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  const toolName = (d.config?.tool_name as string) || ''
  return (
    <NodeCard type="tool" title={d.label && d.label !== 'tool' ? d.label : 'Tool'} status={d.status} selected={selected}
      subtitle={toolName ? <span className="font-mono">{toolName}</span> : <span className="text-warn">no tool selected</span>}>
      <Handle type="target" position={Position.Left} className={HANDLE_CLS} style={{ background: NODE_THEME.tool.accent }} />
      <Handle type="source" position={Position.Right} className={HANDLE_CLS} style={{ background: NODE_THEME.tool.accent }} />
    </NodeCard>
  )
}

function hostOf(url?: string): string {
  if (!url) return ''
  try { return new URL(url).host } catch { return url.slice(0, 32) }
}

function WebhookNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  const url = (d.config?.url as string) || ''
  return (
    <NodeCard type="webhook" title={d.label && d.label !== 'webhook' ? d.label : 'Webhook'} status={d.status} selected={selected}
      subtitle={url ? <span className="font-mono">{((d.config?.method as string) || 'POST').toUpperCase()} {hostOf(url)}</span> : <span className="text-warn">no URL configured</span>}>
      <Handle type="target" position={Position.Left} className={HANDLE_CLS} style={{ background: NODE_THEME.webhook.accent }} />
      <Handle type="source" position={Position.Right} className={HANDLE_CLS} style={{ background: NODE_THEME.webhook.accent }} />
    </NodeCard>
  )
}

function GatewayNode({ data, selected }: NodeProps) {
  const d = data as NodeData
  const peer = (d.config?.peer_id as string) || ''
  return (
    <NodeCard type="gateway" title={d.label && d.label !== 'gateway' ? d.label : 'Gateway'} status={d.status} selected={selected}
      subtitle={peer ? <span className="font-mono flex items-center gap-1"><Send size={9} /> {peer}</span> : <span className="text-warn">no recipient set</span>}>
      <Handle type="target" position={Position.Left} className={HANDLE_CLS} style={{ background: NODE_THEME.gateway.accent }} />
      <Handle type="source" position={Position.Right} className={HANDLE_CLS} style={{ background: NODE_THEME.gateway.accent }} />
    </NodeCard>
  )
}

const nodeTypes = {
  start: StartNode,
  end: EndNode,
  agent: AgentNode,
  supervisor: SupervisorNode,
  condition: ConditionNode,
  parallel: ParallelNode,
  join: JoinNode,
  loop: LoopNode,
  tool: ToolNode,
  webhook: WebhookNode,
  gateway: GatewayNode,
}

// ---------------------------------------------------------------------------
// Palette — grouped, with one-line blurbs
// ---------------------------------------------------------------------------
const PALETTE_GROUPS: { name: string; items: WorkflowNodeType[] }[] = [
  { name: 'Flow',         items: ['start', 'end'] },
  { name: 'Run',          items: ['agent', 'supervisor', 'tool'] },
  { name: 'Logic',        items: ['condition', 'parallel', 'join', 'loop'] },
  { name: 'Integrations', items: ['webhook', 'gateway'] },
]

// ---------------------------------------------------------------------------
// Template gallery types + data
// ---------------------------------------------------------------------------
interface WorkflowTemplate {
  id: string
  name: string
  description: string
  category: string
  nodes: Array<{ key: string; type: WorkflowNodeType; label: string; x: number; y: number; config?: Record<string, unknown> }>
  edges: Array<{ from: string; to: string; label?: string }>
  guide: {
    overview: string
    agents: Array<{ name: string; role: string; systemPrompt: string; tools?: string[]; model?: string }>
    steps: string[]
  }
}

const TEMPLATES: WorkflowTemplate[] = [
  {
    id: 'content-pipeline',
    name: 'Content Pipeline',
    description: 'Research → quality loop → parallel write/SEO → editorial review',
    category: 'Marketing',
    nodes: [
      { key: 'start',    type: 'start',     label: 'Start',          x: 50,   y: 260 },
      { key: 'research', type: 'agent',     label: 'Research Agent', x: 200,  y: 260 },
      { key: 'loop',     type: 'loop',      label: 'Quality Loop',   x: 430,  y: 260, config: { exit_condition: 'contains:STATUS: complete', max_iterations: 3 } },
      { key: 'cond',     type: 'condition', label: 'Quality Check',  x: 660,  y: 260, config: { expression: 'contains:APPROVED' } },
      { key: 'par',      type: 'parallel',  label: 'Parallel',       x: 890,  y: 260 },
      { key: 'writer',   type: 'agent',     label: 'Writer Agent',   x: 1070, y: 150, config: { label: 'Writer Agent' } },
      { key: 'seo',      type: 'agent',     label: 'SEO Analyst',    x: 1070, y: 370, config: { label: 'SEO Analyst' } },
      { key: 'join',     type: 'join',      label: 'Join',           x: 1270, y: 260 },
      { key: 'editor',   type: 'agent',     label: 'Editor Agent',   x: 1450, y: 260, config: { label: 'Editor Agent' } },
      { key: 'end',      type: 'end',       label: 'End',            x: 1640, y: 260 },
    ],
    edges: [
      { from: 'start',    to: 'research' },
      { from: 'research', to: 'loop' },
      { from: 'loop',     to: 'research', label: 'loop' },
      { from: 'loop',     to: 'cond',     label: 'exit' },
      { from: 'cond',     to: 'par',      label: 'yes' },
      { from: 'cond',     to: 'research', label: 'no' },
      { from: 'par',      to: 'writer' },
      { from: 'par',      to: 'seo' },
      { from: 'writer',   to: 'join' },
      { from: 'seo',      to: 'join' },
      { from: 'join',     to: 'editor' },
      { from: 'editor',   to: 'end' },
    ],
    guide: {
      overview: 'A multi-stage content production pipeline. Research Agent gathers facts, a loop ensures quality before branching Writer and SEO agents run in parallel, then an Editor does a final pass.',
      agents: [
        {
          name: 'Research Agent',
          role: 'Gather facts, sources and key points on the topic',
          systemPrompt: `You are a professional research assistant. When given a topic, research it thoroughly and produce a structured brief with:
- 3-5 key facts
- Target audience insights
- Relevant angles / hooks
- Suggested outline

End your response with one of:
- STATUS: complete (research is solid)
- STATUS: needs_more (needs follow-up)`,
          tools: ['web_search'],
          model: 'claude-opus-4-8',
        },
        {
          name: 'Writer Agent',
          role: 'Write the first draft from the research brief',
          systemPrompt: `You are a skilled content writer. Given a research brief, write a well-structured blog post or article that:
- Has a compelling headline
- Opens with a hook
- Covers all key points from the brief
- Uses clear, engaging language
- Ends with a strong call-to-action

Return the full draft.`,
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'SEO Analyst',
          role: 'Generate SEO metadata and keyword recommendations',
          systemPrompt: `You are an SEO specialist. Given a research brief or draft content, produce:
- Primary keyword (1)
- Secondary keywords (3-5)
- Meta title (≤60 chars)
- Meta description (≤160 chars)
- Recommended H2 headings (3-5)
- Internal linking suggestions

Return as structured markdown.`,
          model: 'claude-haiku-4-5-20251001',
        },
        {
          name: 'Editor Agent',
          role: 'Final editorial review combining writer draft and SEO recommendations',
          systemPrompt: `You are a senior editor. You receive a writer's draft and SEO recommendations. Your job is to:
- Integrate SEO keywords naturally into the content
- Fix grammar, clarity and flow issues
- Ensure the meta title and description match the content
- Tighten the copy

Return the final polished article with the SEO meta block at the top.`,
          model: 'claude-sonnet-4-6',
        },
      ],
      steps: [
        'Create all 4 agents (Research, Writer, SEO Analyst, Editor) in the Agents page',
        'For each Agent node on the canvas, open Node Config and assign the corresponding agent',
        'Set the Loop node exit condition to "contains:STATUS: complete" and max iterations to 3',
        'Set the Condition node expression to "contains:APPROVED" — the Research Agent must output APPROVED to proceed',
        'Save the workflow, then run it with a topic like "The future of AI agents in enterprise software"',
      ],
    },
  },
  {
    id: 'support-triage',
    name: 'Customer Support Triage',
    description: 'Classify tickets → route to Tier-1 or escalation team automatically',
    category: 'Support',
    nodes: [
      { key: 'start',     type: 'start',     label: 'Start',              x: 50,  y: 200 },
      { key: 'classify',  type: 'agent',     label: 'Classifier Agent',   x: 200, y: 200 },
      { key: 'cond',      type: 'condition', label: 'Needs Escalation?',  x: 440, y: 200, config: { expression: 'contains:ESCALATE' } },
      { key: 'escalate',  type: 'agent',     label: 'Escalation Handler', x: 660, y: 80,  config: { label: 'Escalation Handler' } },
      { key: 'tier1',     type: 'agent',     label: 'Tier-1 Support',     x: 660, y: 320, config: { label: 'Tier-1 Support' } },
      { key: 'join',      type: 'join',      label: 'Join',               x: 880, y: 200 },
      { key: 'end',       type: 'end',       label: 'End',                x: 1040, y: 200 },
    ],
    edges: [
      { from: 'start',    to: 'classify' },
      { from: 'classify', to: 'cond' },
      { from: 'cond',     to: 'escalate', label: 'yes' },
      { from: 'cond',     to: 'tier1',    label: 'no' },
      { from: 'escalate', to: 'join' },
      { from: 'tier1',    to: 'join' },
      { from: 'join',     to: 'end' },
    ],
    guide: {
      overview: 'Automatically triage incoming support tickets. A Classifier agent categorises severity; a Condition node routes critical tickets to an escalation specialist and routine tickets to Tier-1.',
      agents: [
        {
          name: 'Classifier Agent',
          role: 'Classify the support ticket by severity and category',
          systemPrompt: `You are a support ticket classifier. Analyse the incoming ticket and output a structured triage report:

CATEGORY: [billing | technical | account | general]
SEVERITY: [low | medium | high | critical]
SUMMARY: One-sentence summary of the issue.
NEXT_ACTION: [ESCALATE | TIER1]

Include ESCALATE if: payment disputes, data loss, service outages, security issues, or angry/threatening tone.
Include TIER1 for all other cases.

Always end with the NEXT_ACTION line so routing works correctly.`,
          model: 'claude-haiku-4-5-20251001',
        },
        {
          name: 'Escalation Handler',
          role: 'Craft a professional escalation response and internal notes',
          systemPrompt: `You are a senior support specialist handling escalated tickets. Given a ticket triage report, produce:

1. CUSTOMER RESPONSE: A professional, empathetic response to the customer that acknowledges urgency and sets expectations.
2. INTERNAL NOTES: Key details for the engineering or billing team.
3. SUGGESTED RESOLUTION: Recommended next steps.

Tone: calm, professional, solution-focused.`,
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'Tier-1 Support',
          role: 'Resolve routine tickets with a helpful response',
          systemPrompt: `You are a friendly Tier-1 support agent. Given a support ticket and triage classification, write a helpful customer response that:
- Directly addresses the reported issue
- Provides a clear step-by-step solution or workaround
- Includes relevant documentation links (use placeholders like [docs link])
- Ends with "Is there anything else I can help you with?"

Keep the tone warm and concise.`,
          model: 'claude-haiku-4-5-20251001',
        },
      ],
      steps: [
        'Create 3 agents: Classifier Agent, Escalation Handler, Tier-1 Support',
        'Assign agents to the corresponding nodes using Node Config',
        'The Condition node expression is "contains:ESCALATE" — the Classifier must include ESCALATE in output to trigger escalation',
        'Save and test with a ticket like "My payment was charged twice" (should escalate) or "How do I reset my password?" (should go to Tier-1)',
      ],
    },
  },
  {
    id: 'research-report',
    name: 'Research Report',
    description: 'Deep research → fact-check loop → structured report writing',
    category: 'Research',
    nodes: [
      { key: 'start',       type: 'start',     label: 'Start',            x: 50,  y: 200 },
      { key: 'researcher',  type: 'agent',     label: 'Researcher',       x: 200, y: 200 },
      { key: 'factchecker', type: 'agent',     label: 'Fact Checker',     x: 440, y: 200 },
      { key: 'cond',        type: 'condition', label: 'Facts Verified?',  x: 680, y: 200, config: { expression: 'contains:VERIFIED' } },
      { key: 'writer',      type: 'agent',     label: 'Report Writer',    x: 900, y: 200, config: { label: 'Report Writer' } },
      { key: 'end',         type: 'end',       label: 'End',              x: 1090, y: 200 },
    ],
    edges: [
      { from: 'start',       to: 'researcher' },
      { from: 'researcher',  to: 'factchecker' },
      { from: 'factchecker', to: 'cond' },
      { from: 'cond',        to: 'writer',     label: 'yes' },
      { from: 'cond',        to: 'researcher', label: 'no' },
      { from: 'writer',      to: 'end' },
    ],
    guide: {
      overview: 'A rigorous research workflow. Researcher gathers information, Fact Checker verifies claims, and if any facts fail verification it loops back for more research. Once verified, Report Writer produces the final structured report.',
      agents: [
        {
          name: 'Researcher',
          role: 'Gather comprehensive information on the topic',
          systemPrompt: `You are a thorough research analyst. Given a research question or topic, produce a detailed research document including:

- BACKGROUND: Context and definitions
- KEY FINDINGS: 5-10 bullet points with specific facts, statistics, and claims
- SOURCES: Reference the type of sources you'd cite (academic, industry reports, news)
- GAPS: Note any areas needing more investigation

Be specific and include verifiable claims where possible.`,
          tools: ['web_search'],
          model: 'claude-opus-4-8',
        },
        {
          name: 'Fact Checker',
          role: 'Verify claims in the research document',
          systemPrompt: `You are a rigorous fact-checker. Review the research document and assess each claim:

For each key claim:
- Mark as ✓ CONFIRMED, ⚠ UNCERTAIN, or ✗ DISPUTED
- Note any logical inconsistencies or missing evidence

Conclude with one of:
- VERIFIED: All major claims are supported
- NEEDS_REVISION: [list specific claims that need re-research]

Always end your response with either VERIFIED or NEEDS_REVISION on its own line.`,
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'Report Writer',
          role: 'Write a polished report from verified research',
          systemPrompt: `You are a professional report writer. Given verified research findings, produce a well-structured report with:

# Executive Summary (2-3 sentences)

## Key Findings
(numbered list with evidence)

## Analysis
(interpretation and implications)

## Recommendations
(3-5 actionable recommendations)

## Conclusion

Use clear headings, concise language, and cite specific data points.`,
          model: 'claude-sonnet-4-6',
        },
      ],
      steps: [
        'Create 3 agents: Researcher, Fact Checker, Report Writer',
        'Assign each agent to its node via Node Config',
        'The Condition node checks "contains:VERIFIED" — the Fact Checker must output VERIFIED to proceed to writing',
        'If not verified, the workflow loops back to Researcher for another pass (max once without a Loop node; add a Loop node before Fact Checker for multiple retries)',
        'Run with a question like "What is the current state of quantum computing adoption in enterprise?"',
      ],
    },
  },
  {
    id: 'code-review',
    name: 'Code Review Pipeline',
    description: 'Static analysis + security scan in parallel → AI code reviewer',
    category: 'Engineering',
    nodes: [
      { key: 'start',    type: 'start',     label: 'Start',            x: 50,  y: 220 },
      { key: 'par',      type: 'parallel',  label: 'Parallel',         x: 200, y: 220 },
      { key: 'linter',   type: 'agent',     label: 'Static Analysis',  x: 400, y: 100, config: { label: 'Static Analysis' } },
      { key: 'security', type: 'agent',     label: 'Security Scan',    x: 400, y: 340, config: { label: 'Security Scan' } },
      { key: 'join',     type: 'join',      label: 'Join',             x: 620, y: 220 },
      { key: 'reviewer', type: 'agent',     label: 'Code Reviewer',    x: 820, y: 220, config: { label: 'Code Reviewer' } },
      { key: 'cond',     type: 'condition', label: 'Approved?',        x: 1040, y: 220, config: { expression: 'contains:APPROVED' } },
      { key: 'end',      type: 'end',       label: 'End',              x: 1220, y: 120 },
      { key: 'fix',      type: 'agent',     label: 'Fix Suggestions',  x: 1220, y: 340, config: { label: 'Fix Suggestions' } },
    ],
    edges: [
      { from: 'start',    to: 'par' },
      { from: 'par',      to: 'linter' },
      { from: 'par',      to: 'security' },
      { from: 'linter',   to: 'join' },
      { from: 'security', to: 'join' },
      { from: 'join',     to: 'reviewer' },
      { from: 'reviewer', to: 'cond' },
      { from: 'cond',     to: 'end',  label: 'yes' },
      { from: 'cond',     to: 'fix',  label: 'no' },
    ],
    guide: {
      overview: 'Automated code review pipeline. Static Analysis and Security Scan run in parallel, then a Code Reviewer synthesises both reports. If issues are found, a Fix Suggestions agent produces actionable remediation steps.',
      agents: [
        {
          name: 'Static Analysis',
          role: 'Analyse code for style, complexity and maintainability issues',
          systemPrompt: `You are a static code analysis tool. Review the provided code and report:

## Code Quality Issues
- List each issue with: FILE (if provided), LINE RANGE, SEVERITY (low/medium/high), DESCRIPTION, SUGGESTED FIX

## Complexity Assessment
- Cyclomatic complexity concerns
- Long functions or deeply nested logic
- Code duplication

## Summary
Total issues found: X (Y high, Z medium, W low)

Be specific with line references. If no issues found, say "No issues detected."`,
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'Security Scan',
          role: 'Identify security vulnerabilities in the code',
          systemPrompt: `You are a security-focused code auditor trained on OWASP Top 10 and common CVEs. Review the code for:

## Vulnerabilities Found
For each issue: SEVERITY (critical/high/medium/low), TYPE (e.g. SQL Injection, XSS, IDOR), LOCATION, DESCRIPTION, REMEDIATION

## Security Checklist
- [ ] Input validation
- [ ] Authentication / authorisation checks
- [ ] Secrets / credentials in code
- [ ] Dependency risks
- [ ] Data exposure risks

## Verdict
CRITICAL_ISSUES_FOUND / MINOR_ISSUES_FOUND / CLEAN`,
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'Code Reviewer',
          role: 'Synthesise analysis reports and make final review decision',
          systemPrompt: `You are a senior engineering reviewer. You receive two reports: a static analysis report and a security scan report. Synthesise them into a final code review:

## Review Summary
- Overall quality assessment (1-10)
- Key strengths
- Critical blockers (must fix before merge)
- Nice-to-have improvements

## Decision
End with exactly one of:
- APPROVED: Code is ready to merge
- CHANGES_REQUESTED: [brief reason]

The APPROVED or CHANGES_REQUESTED line determines workflow routing.`,
          model: 'claude-opus-4-8',
        },
        {
          name: 'Fix Suggestions',
          role: 'Generate concrete code fixes for review blockers',
          systemPrompt: `You are a senior developer helping a team fix code review blockers. Given the code review report, produce:

For each blocking issue:
1. **Issue**: [restate the problem]
2. **Fix**: Show the corrected code snippet
3. **Explanation**: Why this fix addresses the concern

Prioritise critical security issues first, then high-severity quality issues.
End with a checklist the developer can use to verify all fixes are applied.`,
          model: 'claude-sonnet-4-6',
        },
      ],
      steps: [
        'Create 4 agents: Static Analysis, Security Scan, Code Reviewer, Fix Suggestions',
        'Assign agents to nodes: Parallel branches → linter and security nodes; then Code Reviewer and Fix Suggestions',
        'The Condition checks "contains:APPROVED" — the Code Reviewer must output APPROVED to end cleanly',
        'If changes are requested, the Fix Suggestions agent provides remediation steps (you can loop Fix Suggestions back to Code Reviewer with a Loop node for multi-pass reviews)',
        'Run the workflow by pasting code into the input field',
      ],
    },
  },
  {
    id: 'supervisor-intelligence',
    name: 'Enterprise Intelligence Pipeline',
    description: 'Full-spectrum research pipeline: parallel ingestion → supervisor-coordinated analysis team → quality loop → gated executive briefing',
    category: 'Supervisor',
    nodes: [
      // ── Phase 1: Pre-processing ──────────────────────────────────────────
      { key: 'start',        type: 'start',      label: 'Start',                 x: 50,   y: 300 },
      { key: 'preprocess',   type: 'agent',      label: 'Query Preprocessor',    x: 260,  y: 300, config: { label: 'Query Preprocessor' } },
      // ── Phase 2: Parallel research ingestion ─────────────────────────────
      { key: 'parallel',     type: 'parallel',   label: 'Parallel',              x: 490,  y: 300 },
      { key: 'web_intel',    type: 'agent',       label: 'Web Intelligence',      x: 710,  y: 120, config: { label: 'Web Intelligence' } },
      { key: 'domain_exp',   type: 'agent',       label: 'Domain Expert',         x: 710,  y: 480, config: { label: 'Domain Expert' } },
      { key: 'join1',        type: 'join',        label: 'Join',                  x: 950,  y: 300 },
      // ── Phase 3: Supervisor + specialist team (below the main axis) ──────
      { key: 'supervisor',   type: 'supervisor',  label: 'Intelligence Supervisor', x: 1170, y: 300 },
      { key: 'data_analyst', type: 'agent',       label: 'Data Analyst',          x: 970,  y: 580, config: { label: 'Data Analyst' } },
      { key: 'fact_verify',  type: 'agent',       label: 'Fact Verifier',         x: 1130, y: 580, config: { label: 'Fact Verifier' } },
      { key: 'src_eval',     type: 'agent',       label: 'Source Evaluator',      x: 1290, y: 580, config: { label: 'Source Evaluator' } },
      { key: 'insight_gen',  type: 'agent',       label: 'Insight Generator',     x: 1450, y: 580, config: { label: 'Insight Generator' } },
      // ── Phase 4: Quality control loop ────────────────────────────────────
      { key: 'loop',         type: 'loop',        label: 'Quality Loop',          x: 1430, y: 300, config: { exit_condition: 'contains:QUALITY_PASS', max_iterations: 3 } },
      // ── Phase 5: Approval gate ────────────────────────────────────────────
      { key: 'gate',         type: 'condition',   label: 'Executive Approval?',   x: 1680, y: 300, config: { expression: 'contains:EXECUTIVE_APPROVED' } },
      { key: 'exec_brief',   type: 'agent',       label: 'Executive Briefing',    x: 1920, y: 120, config: { label: 'Executive Briefing' } },
      { key: 'gap_report',   type: 'agent',       label: 'Gap Analysis Report',   x: 1920, y: 480, config: { label: 'Gap Analysis Report' } },
      // ── Phase 6: Final merge ─────────────────────────────────────────────
      { key: 'join2',        type: 'join',        label: 'Join',                  x: 2160, y: 300 },
      { key: 'end',          type: 'end',         label: 'End',                   x: 2360, y: 300 },
    ],
    edges: [
      // Pre-processing
      { from: 'start',       to: 'preprocess' },
      { from: 'preprocess',  to: 'parallel' },
      // Parallel branches
      { from: 'parallel',    to: 'web_intel' },
      { from: 'parallel',    to: 'domain_exp' },
      { from: 'web_intel',   to: 'join1' },
      { from: 'domain_exp',  to: 'join1' },
      // Into supervisor
      { from: 'join1',       to: 'supervisor' },
      // Supervisor → main flow
      { from: 'supervisor',  to: 'loop' },
      // Supervisor → team members (dashed delegate edges)
      { from: 'supervisor',  to: 'data_analyst', label: 'delegate' },
      { from: 'supervisor',  to: 'fact_verify',  label: 'delegate' },
      { from: 'supervisor',  to: 'src_eval',     label: 'delegate' },
      { from: 'supervisor',  to: 'insight_gen',  label: 'delegate' },
      // Quality loop
      { from: 'loop',        to: 'supervisor',   label: 'loop' },
      { from: 'loop',        to: 'gate',         label: 'exit' },
      // Approval gate branches
      { from: 'gate',        to: 'exec_brief',   label: 'yes' },
      { from: 'gate',        to: 'gap_report',   label: 'no' },
      // Final merge
      { from: 'exec_brief',  to: 'join2' },
      { from: 'gap_report',  to: 'join2' },
      { from: 'join2',       to: 'end' },
    ],
    guide: {
      overview: `A production-grade intelligence pipeline that demonstrates every workflow node type working together.

Phase 1 — Preprocessing: A Query Preprocessor structures the raw input into a precise research brief.

Phase 2 — Parallel ingestion: Two specialist agents run concurrently. Web Intelligence searches the web for current data while Domain Expert applies deep domain knowledge. Their outputs are merged by a Join node.

Phase 3 — Supervisor coordination: The Intelligence Supervisor orchestrates four team agents (via dashed delegate edges). It delegates to its team agents as tools — deciding when and in what order. The supervisor synthesises their outputs and marks the result QUALITY_PASS if the analysis is complete.

Phase 4 — Quality loop: The Loop node checks for QUALITY_PASS in the supervisor's output. If not found, it retries the supervisor (up to 3 times). If found, it exits to the approval gate.

Phase 5 — Executive gate: A Condition node checks for EXECUTIVE_APPROVED. If present, the Executive Briefing agent produces a polished summary. If not, the Gap Analysis Report agent identifies what still needs work.

Phase 6 — Final join: Both gate branches merge into a Join node before the End.`,
      agents: [
        {
          name: 'Query Preprocessor',
          role: 'Transform raw user input into a structured research brief',
          systemPrompt: `You are a research query preprocessor. Transform the user's input into a structured brief that downstream agents can act on precisely.

## Research Question
[Clear, precise formulation of the investigation]

## Scope & Constraints
- Domain: [primary field]
- Depth required: [overview / detailed / exhaustive]
- Key assumptions: [list any]

## Sub-questions to Answer
1. [specific sub-question]
2. [specific sub-question]
3. [specific sub-question]

## Success Criteria
[What a complete, high-quality answer looks like — be specific]

Always output this exact structure. Be concise but precise.`,
          model: 'claude-haiku-4-5-20251001',
        },
        {
          name: 'Web Intelligence',
          role: 'Search the web and gather current, relevant intelligence',
          systemPrompt: `You are a web intelligence specialist. Given a research brief, gather current and relevant information.

## Web Intelligence Report

### Summary
[2-3 sentence synthesis of what you found]

### Key Findings
- [finding 1] — [context/source type]
- [finding 2] — [context/source type]
- [finding 3] — [context/source type]

### Current Trends & Developments
[What is happening right now in this space]

### Data Points & Statistics
[Any quantitative data found]

### Intelligence Gaps
[What could not be confirmed or remains unclear]

Be factual, specific, and cite source types (news article, research paper, industry report, etc.).`,
          tools: ['web_search'],
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'Domain Expert',
          role: 'Apply deep domain expertise and contextual knowledge',
          systemPrompt: `You are a domain knowledge expert. Apply deep expertise to provide authoritative context on the research topic.

## Domain Expert Report

### Domain Context
[Background and foundational knowledge essential to this topic]

### Expert Analysis
[What practitioners and academics know about this — draw on deep knowledge]

### Frameworks & Mental Models
[Key frameworks, theories or models that apply]

### Domain-Specific Nuances
[Edge cases, pitfalls, or subtleties that a non-expert would miss]

### Confidence Assessment
[Where your domain knowledge is strong vs. where you're less certain]

Be authoritative and specific. Flag any areas where the question crosses into adjacent domains.`,
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'Intelligence Supervisor',
          role: 'Orchestrate the specialist team and synthesise a verified intelligence report',
          systemPrompt: `You are a senior intelligence supervisor coordinating a team of specialist agents. Your available team agents and their exact tool names are injected automatically at runtime — do not reference tool names in these instructions.

Orchestration strategy:
1. Delegate to the fact-verification and source-evaluation specialists first — they can work on the raw ingested data immediately.
2. Delegate to the quantitative data analyst with the research findings to extract metrics and patterns.
3. Delegate to the insight-generation specialist last, providing the full verified and analysed findings.

After reviewing all team outputs, produce a comprehensive intelligence report:

# Intelligence Report

## Executive Summary
[3-4 sentence high-level answer]

## Verified Key Findings
[List verified, high-confidence findings with evidence]

## Data & Metrics
[Quantitative insights from data_analyst]

## Strategic Insights
[Synthesised insights and implications from insight_generator]

## Confidence & Caveats
[Overall confidence level and important limitations]

## Recommendations
[3-5 concrete, prioritised recommendations]

---
If the team has produced a complete, high-quality verified analysis:
→ End with: QUALITY_PASS | EXECUTIVE_APPROVED

If there are significant gaps, low-confidence findings, or the team flagged issues:
→ End with: QUALITY_FAIL (and note what's missing)`,
          model: 'claude-opus-4-8',
        },
        {
          name: 'Data Analyst',
          role: 'Extract quantitative patterns, metrics and statistical insights',
          systemPrompt: `You are a quantitative data analyst. Analyse the research findings for measurable patterns and data-driven insights.

## Data Analysis Report

### Key Metrics Identified
- [metric 1]: [value/range] — [significance]
- [metric 2]: [value/range] — [significance]
- [metric 3]: [value/range] — [significance]

### Patterns & Correlations
[Statistical or logical patterns in the data]

### Trend Analysis
[Direction and velocity of key trends]

### Anomalies & Outliers
[Unusual data points that warrant attention]

### Data Quality Assessment
- Coverage: HIGH / MEDIUM / LOW
- Recency: CURRENT / DATED / MIXED
- Confidence: [1-10 with brief justification]

Be precise with numbers. If data is unavailable, state that explicitly rather than estimating.`,
          model: 'claude-haiku-4-5-20251001',
        },
        {
          name: 'Fact Verifier',
          role: 'Cross-check accuracy of claims and flag unverified assertions',
          systemPrompt: `You are a rigorous fact-checker. Systematically verify every major claim in the research findings.

## Fact Verification Report

### Claim Verification
For each key claim in the research:
- ✓ CONFIRMED: Strong supporting evidence
- ⚠ UNCERTAIN: Partially supported or conflicting signals
- ✗ DISPUTED: Evidence contradicts this claim

### Verification Summary
- Confirmed claims: [count]
- Uncertain claims: [count] — [brief description]
- Disputed claims: [count] — [brief description]

### High-Risk Assertions
[Claims that appear confident but have weak evidence — flag these prominently]

### Overall Reliability Rating
RELIABLE / PARTIALLY_RELIABLE / UNRELIABLE — [one sentence justification]

Be strict. Flag speculation presented as fact. Note where claims require caveats.`,
          model: 'claude-haiku-4-5-20251001',
        },
        {
          name: 'Source Evaluator',
          role: 'Assess credibility, bias and quality of research sources',
          systemPrompt: `You are a source quality evaluator. Assess the credibility and reliability of all sources referenced in the research.

## Source Evaluation Report

### Source Inventory
List each distinct source type found in the research with a quality rating:
- [Source type]: ★★★★☆ — [brief credibility note]
- [Source type]: ★★★☆☆ — [brief credibility note]

### Bias Assessment
[Potential ideological, commercial or selection biases in the source pool]

### Coverage Gaps
[Important perspectives or source types that are missing]

### Source Diversity Score
EXCELLENT / GOOD / ADEQUATE / POOR — [one sentence justification]

### Recommended Additional Sources
[2-3 specific source types that would strengthen the analysis]

Be direct. Poor sources should be clearly labelled as such.`,
          model: 'claude-haiku-4-5-20251001',
        },
        {
          name: 'Insight Generator',
          role: 'Synthesise verified findings into strategic insights and recommendations',
          systemPrompt: `You are a strategic intelligence analyst. Transform verified research findings into actionable insights for decision-makers.

## Strategic Intelligence Synthesis

### Top Insights
1. **[Insight title]**: [What this means and why it matters — 2-3 sentences]
2. **[Insight title]**: [What this means and why it matters]
3. **[Insight title]**: [What this means and why it matters]

### Implications
[What these insights mean for the organisation / decision-maker]

### Risk Factors
[Key risks or threats surfaced by the analysis]

### Opportunities
[Clear opportunities identified in the findings]

### Prioritised Recommendations
1. [Action] — Impact: HIGH | Urgency: HIGH/MEDIUM/LOW
2. [Action] — Impact: MEDIUM | Urgency: HIGH/MEDIUM/LOW
3. [Action] — Impact: MEDIUM | Urgency: HIGH/MEDIUM/LOW

### Synthesis Confidence
EXECUTIVE_APPROVED — [brief rationale]
OR
NEEDS_REVISION — [specific gaps that prevent executive sign-off]`,
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'Executive Briefing',
          role: 'Produce a polished, executive-ready intelligence brief',
          systemPrompt: `You are an executive communications specialist. Transform the intelligence report into a crisp, decision-ready executive brief.

# EXECUTIVE INTELLIGENCE BRIEF

**Classification**: Internal Research | **Date**: [Today]

## Situation Summary
[3 sentences maximum — what's happening and why it matters NOW]

## Key Takeaways
• [Takeaway 1 — one sentence, direct]
• [Takeaway 2 — one sentence, direct]
• [Takeaway 3 — one sentence, direct]

## Decision Points
[What decisions need to be made and by when]

## Recommended Actions
| Action | Owner | Timeline | Priority |
|--------|-------|----------|----------|
| [action] | [team] | [timeframe] | HIGH |
| [action] | [team] | [timeframe] | MEDIUM |

## Risk if No Action
[Clear consequence of inaction — one sentence]

## Confidence Level
[HIGH / MEDIUM / LOW] — [brief justification]

Keep it to one page. Executives need clarity, not comprehensiveness.`,
          model: 'claude-sonnet-4-6',
        },
        {
          name: 'Gap Analysis Report',
          role: 'Identify and document what is missing or unresolved in the analysis',
          systemPrompt: `You are a research quality auditor. The analysis did not reach executive approval. Identify precisely what is missing or needs correction.

# GAP ANALYSIS REPORT

## Why Executive Approval Was Not Granted
[Direct statement of the primary reason]

## Critical Gaps
For each gap that must be addressed:
- **Gap**: [description]
  - **Impact**: [why this matters]
  - **Resolution**: [specific action needed]
  - **Priority**: CRITICAL / HIGH / MEDIUM

## Data Deficiencies
[Specific data points that are missing, outdated or unreliable]

## Unverified Claims
[Claims that require additional validation before executive presentation]

## Recommended Next Steps
1. [Concrete next step with owner and timeline]
2. [Concrete next step]
3. [Concrete next step]

## Estimated Gap Closure Effort
[LOW (< 1 day) / MEDIUM (1-3 days) / HIGH (> 3 days)]

Be specific and constructive. This report is used to brief the team on what to fix.`,
          model: 'claude-haiku-4-5-20251001',
        },
      ],
      steps: [
        'Click "Create agents & load" — this creates all 9 agents and maps them to the canvas automatically',
        'Open the Intelligence Supervisor node (gold/amber, 👑 crown) → Node Config → verify the agent is assigned',
        'The 4 dashed amber edges from the Supervisor to team agents are delegate edges — those agents run as tools',
        'The Loop node is pre-configured with exit_condition "contains:QUALITY_PASS" and max 3 iterations',
        'The Condition node checks "contains:EXECUTIVE_APPROVED" — the Insight Generator must output this for the executive branch to fire',
        'Run the workflow with an intelligence question, e.g. "Analyse the competitive landscape for enterprise AI agent platforms in 2025"',
        'Watch the canvas — nodes light up as they run; the Supervisor calls team agents as tools and shows delegation events',
        'If the supervisor produces QUALITY_PASS + EXECUTIVE_APPROVED, the Executive Briefing agent fires; otherwise Gap Analysis runs',
      ],
    },
  },
]

// Template agents specify an intended capability tier via their model hint;
// at creation time the hint is resolved against the provider actually
// connected in this workspace, so templates work whether the workspace runs
// Anthropic, OpenAI, Gemini, or a local Ollama.
type ModelTier = 'heavy' | 'balanced' | 'light'

function tierForHint(hint?: string): ModelTier {
  const h = (hint ?? '').toLowerCase()
  if (/opus|o1-pro|gpt-4-turbo/.test(h)) return 'heavy'
  if (/haiku|mini|flash|3\.5/.test(h)) return 'light'
  return 'balanced'
}

const TIER_PATTERNS: Record<string, Record<ModelTier, RegExp[]>> = {
  anthropic: { heavy: [/opus/], balanced: [/sonnet/], light: [/haiku/] },
  openai: { heavy: [/^o1(?!-mini)/, /gpt-4o(?!-mini)/], balanced: [/gpt-4o(?!-mini)/, /gpt-4/], light: [/mini/, /gpt-3\.5/] },
  gemini: { heavy: [/pro/], balanced: [/flash/], light: [/flash-lite/, /flash/] },
}

function pickModelForTier(providerName: string, models: { id: string }[], tier: ModelTier): string | undefined {
  for (const re of TIER_PATTERNS[providerName]?.[tier] ?? []) {
    const hit = models.find((m) => re.test(m.id.toLowerCase()))
    if (hit) return hit.id
  }
  return models[0]?.id
}

// ---------------------------------------------------------------------------
// Template gallery modal
// ---------------------------------------------------------------------------
function TemplateGalleryModal({
  onSelect,
  onClose,
}: {
  onSelect: (tpl: WorkflowTemplate, createdAgents?: Record<string, Agent>) => void
  onClose: () => void
}) {
  const [selected, setSelected] = useState<WorkflowTemplate>(TEMPLATES[0])
  const [guideTab, setGuideTab] = useState<'overview' | 'agents' | 'steps'>('overview')
  const [creating, setCreating] = useState(false)
  const [createProgress, setCreateProgress] = useState<string[]>([])
  const [createError, setCreateError] = useState<string | null>(null)

  // Resolve models against the provider connected in this workspace instead
  // of hardcoding Anthropic.
  const { data: provData } = useQuery({
    queryKey: ['providers'],
    queryFn: () => providersAPI.list() as Promise<{ data: { id: string; provider: string; is_active: boolean }[] }>,
  })
  const providerCreds = provData?.data ?? []
  const activeCred = providerCreds.find((c) => c.is_active) ?? providerCreds[0]
  const { data: modelsData } = useQuery({
    queryKey: ['provider-models', activeCred?.id],
    queryFn: () => providersAPI.models(activeCred!.id) as Promise<{ data: { id: string }[] }>,
    enabled: !!activeCred,
  })
  const availableModels = modelsData?.data ?? []
  const resolveModel = (hint?: string) =>
    activeCred ? pickModelForTier(activeCred.provider, availableModels, tierForHint(hint)) : undefined

  async function handleCreateAndLoad() {
    if (!activeCred) {
      setCreateError('No LLM provider connected — add one under Settings → Providers before creating template agents.')
      return
    }
    setCreating(true)
    setCreateError(null)
    setCreateProgress([])
    const createdAgents: Record<string, Agent> = {}
    try {
      for (const agentDef of selected.guide.agents) {
        setCreateProgress((prev) => [...prev, `Creating ${agentDef.name}…`])
        const created = await agentsAPI.create({
          name: agentDef.name,
          description: agentDef.role,
          instructions: agentDef.systemPrompt,
          provider: activeCred.provider,
          model: resolveModel(agentDef.model) ?? agentDef.model ?? 'claude-sonnet-4-6',
          temperature: 0.7,
          max_tokens: 4096,
          memory_enabled: false,
          memory_scope: 'conversation',
          context_retrieval_enabled: false,
          max_steps: 10,
          max_tool_calls: 5,
          max_duration_secs: 300,
          status: 'active',
        }) as Agent
        createdAgents[agentDef.name] = created
        setCreateProgress((prev) => [...prev.slice(0, -1), `✓ ${agentDef.name}`])
      }
      onSelect(selected, createdAgents)
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create agents')
      setCreating(false)
    }
  }

  const categoryHue: Record<string, string> = {
    Marketing:   '#ec4899',
    Support:     '#0ea5e9',
    Research:    '#10b981',
    Engineering: '#8b5cf6',
    Supervisor:  '#d97706',
  }

  return (
    <div
      className="fixed inset-0 z-[200] flex items-center justify-center bg-black/50 p-4 backdrop-blur-[2px]"
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div className="flex h-[85vh] w-[95vw] max-w-4xl flex-col overflow-hidden rounded-2xl border border-border bg-surface shadow-2xl">
        {/* Header */}
        <div className="flex flex-shrink-0 items-center justify-between border-b border-border px-5 py-4">
          <div className="flex items-center gap-2.5">
            <span className="grid h-7 w-7 place-items-center rounded-lg bg-accent-light text-accent dark:bg-accent/15 dark:text-accent-bright">
              <BookOpen size={14} />
            </span>
            <div>
              <div className="text-sm font-semibold text-foreground">Workflow templates</div>
              <div className="text-[11px] text-muted-foreground">Proven patterns to start from</div>
            </div>
          </div>
          <button onClick={onClose} className="text-faint transition-colors hover:text-foreground" aria-label="Close templates">
            <X size={16} />
          </button>
        </div>

        {/* Body: card list + detail panel */}
        <div className="flex min-h-0 flex-1 flex-nowrap overflow-hidden">
          {/* Left: template cards */}
          <div className="flex w-56 min-w-44 flex-shrink-0 flex-col gap-2 overflow-y-auto border-r border-border p-3">
            {TEMPLATES.map((tpl) => {
              const hue = categoryHue[tpl.category] ?? '#64748b'
              const isActive = selected.id === tpl.id
              return (
                <button
                  key={tpl.id}
                  onClick={() => { setSelected(tpl); setGuideTab('overview') }}
                  className={`rounded-xl border p-3 text-left transition-colors ${
                    isActive ? 'border-accent/60 bg-accent-light/60 dark:bg-accent/10' : 'border-border bg-surface hover:bg-muted'
                  }`}
                >
                  <div className="mb-1.5 flex items-center justify-between gap-2">
                    <span className={`text-xs font-semibold ${isActive ? 'text-accent dark:text-accent-bright' : 'text-foreground'}`}>{tpl.name}</span>
                    <span className="rounded-full px-1.5 py-0.5 font-mono text-[9px] font-semibold" style={{ background: `${hue}1a`, color: hue }}>
                      {tpl.category}
                    </span>
                  </div>
                  <p className="m-0 text-[11px] leading-snug text-muted-foreground">{tpl.description}</p>
                  <p className="mt-1.5 font-mono text-[10px] text-faint">
                    {tpl.nodes.filter((n) => n.type === 'agent').length} agents · {tpl.nodes.length} nodes
                  </p>
                </button>
              )
            })}
          </div>

          {/* Right: guide detail */}
          <div className="flex flex-1 flex-col overflow-hidden">
            {/* Tab bar */}
            <div className="flex flex-shrink-0 border-b border-border px-5">
              {(['overview', 'agents', 'steps'] as const).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setGuideTab(tab)}
                  className={`border-b-2 px-4 py-3 text-xs transition-colors ${
                    guideTab === tab
                      ? 'border-accent font-semibold text-accent dark:text-accent-bright'
                      : 'border-transparent font-medium text-muted-foreground hover:text-foreground'
                  }`}
                >
                  {tab === 'overview' ? 'Overview' : tab === 'agents' ? `Agents (${selected.guide.agents.length})` : 'Setup guide'}
                </button>
              ))}
            </div>

            {/* Tab content */}
            <div className="flex-1 overflow-y-auto p-5">
              {guideTab === 'overview' && (
                <div className="flex flex-col gap-4">
                  <p className="m-0 whitespace-pre-line text-[13px] leading-relaxed text-foreground">{selected.guide.overview}</p>
                  <div>
                    <div className="mb-2 font-mono text-[10px] font-semibold uppercase tracking-wider text-faint">Workflow nodes</div>
                    <div className="flex flex-wrap gap-1.5">
                      {selected.nodes.map((n) => {
                        const accent = (NODE_THEME[n.type] ?? NODE_THEME.agent).accent
                        return (
                          <span key={n.key} className="rounded-md border px-2 py-0.5 text-[11px] font-medium"
                            style={{ background: `${accent}12`, color: accent, borderColor: `${accent}30` }}>
                            {n.label}
                          </span>
                        )
                      })}
                    </div>
                  </div>
                </div>
              )}

              {guideTab === 'agents' && (
                <div className="flex flex-col gap-4">
                  {selected.guide.agents.map((agent, i) => (
                    <div key={i} className="overflow-hidden rounded-xl border border-border">
                      <div className="border-b border-border bg-muted/50 px-3.5 py-2.5">
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-[13px] font-semibold text-foreground">{agent.name}</span>
                          {(resolveModel(agent.model) ?? agent.model) && (
                            <span className="rounded-md bg-accent-light px-1.5 py-0.5 font-mono text-[10px] font-semibold text-accent dark:bg-accent/15 dark:text-accent-bright">{resolveModel(agent.model) ?? agent.model}</span>
                          )}
                        </div>
                        <p className="m-0 mt-1 text-[11px] text-muted-foreground">{agent.role}</p>
                      </div>
                      <div className="px-3.5 py-3">
                        <div className="mb-1.5 font-mono text-[10px] font-semibold uppercase tracking-wider text-faint">System prompt</div>
                        <pre className="m-0 max-h-48 overflow-y-auto whitespace-pre-wrap break-words rounded-lg border border-border bg-muted/40 px-3 py-2.5 font-mono text-[11px] leading-relaxed text-foreground">{agent.systemPrompt}</pre>
                        {agent.tools && agent.tools.length > 0 && (
                          <div className="mt-2 flex items-center gap-1.5">
                            <span className="font-mono text-[10px] font-semibold text-faint">TOOLS:</span>
                            {agent.tools.map((t) => (
                              <span key={t} className="rounded-md bg-warn/10 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-warn">{t}</span>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {guideTab === 'steps' && (
                <div className="flex flex-col gap-3">
                  <p className="m-0 mb-1 text-xs leading-relaxed text-muted-foreground">
                    Follow these steps to set up this workflow:
                  </p>
                  {selected.guide.steps.map((step, i) => (
                    <div key={i} className="flex items-start gap-3">
                      <span className="grid h-6 w-6 flex-shrink-0 place-items-center rounded-full bg-accent-light font-mono text-[11px] font-bold text-accent dark:bg-accent/15 dark:text-accent-bright">
                        {i + 1}
                      </span>
                      <p className="m-0 text-xs leading-relaxed text-foreground">{step}</p>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Action bar */}
            <div className="flex-shrink-0 border-t border-border px-5 py-3">
              {createError && (
                <div className="mb-2.5 rounded-lg border border-crit/30 bg-crit/10 px-3 py-2 text-[11px] text-crit">
                  {createError}
                </div>
              )}
              {provData && !activeCred && (
              <div className="mb-2.5 rounded-lg border border-warn/30 bg-warn/10 px-3 py-2 text-[11px] text-warn">
                No LLM provider connected — template agents will use the provider you add under Settings → Providers.
              </div>
            )}
            {createProgress.length > 0 && (
                <div className="mb-2.5 flex flex-col gap-0.5 rounded-lg border border-accent/30 bg-accent-light/60 px-3 py-2 dark:bg-accent/10">
                  {createProgress.map((line, i) => (
                    <span key={i} className="font-mono text-[11px] text-accent dark:text-accent-bright">{line}</span>
                  ))}
                </div>
              )}
              <div className="flex justify-end gap-2">
                <button
                  onClick={() => onSelect(selected)}
                  disabled={creating}
                  className="flex items-center gap-1.5 rounded-lg border border-border bg-surface px-3.5 py-2 text-xs font-medium text-foreground transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <LayoutTemplate size={13} /> Load template only
                </button>
                <button
                  onClick={handleCreateAndLoad}
                  disabled={creating || !activeCred}
                  title={activeCred ? undefined : 'Connect an LLM provider first'}
                  className="flex items-center gap-1.5 rounded-[10px] bg-gradient-to-br from-accent to-accent-ink px-4 py-2 text-xs font-semibold text-white shadow-card transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {creating
                    ? <><Loader2 size={13} className="animate-spin" /> Creating…</>
                    : <><Play size={13} /> Create agents & load</>
                  }
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main page (inner — inside ReactFlowProvider)
// ---------------------------------------------------------------------------
function WorkflowBuilderInner({ groupId }: { groupId: string }) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const reactFlowWrapper = useRef<HTMLDivElement>(null)
  const [rfInstance, setRfInstance] = useState<ReactFlowInstance | null>(null)

  const [nodes, setNodes, onNodesChangeRaw] = useNodesState<any>([])
  const [edges, setEdges, onEdgesChangeRaw] = useEdgesState<Edge>([])
  const [selectedNode, setSelectedNode] = useState<Node<NodeData> | null>(null)
  const [selectedEdge, setSelectedEdge] = useState<Edge | null>(null)
  const [runPanelOpen, setRunPanelOpen] = useState(false)
  const [triggersPanelOpen, setTriggersPanelOpen] = useState(false)
  const [runInput, setRunInput] = useState('')
  const [runOutput, setRunOutput] = useState<Record<string, string>>({})
  const [runStatus, setRunStatus] = useState<'idle' | 'running' | 'done'>('idle')
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [saveWarnings, setSaveWarnings] = useState<string[]>([])
  const [runError, setRunError] = useState<string | null>(null)
  const [dirty, setDirty] = useState(false)

  // Structural canvas changes mark the graph dirty; selection churn and
  // in-flight drags don't.
  const onNodesChange = useCallback((changes: Parameters<typeof onNodesChangeRaw>[0]) => {
    onNodesChangeRaw(changes)
    if (changes.some((c) => c.type === 'remove' || c.type === 'add' || (c.type === 'position' && c.dragging === false))) {
      setDirty(true)
    }
  }, [onNodesChangeRaw])
  const onEdgesChange = useCallback((changes: Parameters<typeof onEdgesChangeRaw>[0]) => {
    onEdgesChangeRaw(changes)
    if (changes.some((c) => c.type === 'remove' || c.type === 'add')) setDirty(true)
  }, [onEdgesChangeRaw])

  // Fetch group
  const { data: groupData } = useQuery({
    queryKey: ['workflow', groupId],
    queryFn: () => workflowsAPI.get(groupId) as Promise<Workflow>,
  })

  // Fetch agents list for config panel
  const { data: agentsData } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })
  const agents = agentsData?.data ?? []

  // Workspace tools (tool-node picker) and gateway channels (gateway/end
  // delivery pickers). Fetched lazily-ish: cheap lists, cached by react-query.
  const { data: toolsData } = useQuery({
    queryKey: ['tools'],
    queryFn: () => toolsAPI.list() as Promise<{ data: Tool[] }>,
  })
  const workspaceTools = useMemo(
    () => (toolsData?.data ?? []).filter((t) => t.enabled),
    [toolsData],
  )
  const { data: channelsData } = useQuery({
    queryKey: ['gateway-channels'],
    queryFn: () => gatewayAPI.listChannels() as Promise<{ data: GatewayChannel[] }>,
  })
  const gatewayChannels = channelsData?.data ?? []

  // Fetch graph — TanStack Query v5 removed onSuccess; use useEffect instead
  const { data: graphData, isLoading: graphLoading } = useQuery({
    queryKey: ['workflow-graph', groupId],
    queryFn: () => workflowsAPI.getGraph(groupId),
  })

  const [graphInitialised, setGraphInitialised] = useState(false)
  useEffect(() => {
    if (graphInitialised || !graphData || !agentsData) return
    if (graphData.nodes?.length) {
      setNodes(graphData.nodes.map((n: WorkflowNode) => toRFNode(n, agents)))
      setEdges(graphData.edges.map(toRFEdge))
    }
    setGraphInitialised(true)
  }, [graphData, agentsData]) // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-label edges from condition/loop handles so the executor can route them.
  // handle id "yes"/"no" → edge label "yes"/"no"
  // handle id "continue"/"loop" → edge label "loop" (loop-back edge)
  // handle id "exit" → edge label "exit" (loop exit edge)
  const onConnect = useCallback(
    (connection: Connection) => {
      let label: string | undefined
      if (connection.sourceHandle === 'yes') label = 'yes'
      else if (connection.sourceHandle === 'no') label = 'no'
      else if (connection.sourceHandle === 'continue') label = 'loop'
      else if (connection.sourceHandle === 'exit') label = 'exit'
      else if (connection.sourceHandle === 'delegate') label = 'delegate'

      setEdges((eds) => addEdge({ ...connection, ...mkEdgeProps(label) }, eds))
      setDirty(true)
    },
    [setEdges]
  )

  const [templateGalleryOpen, setTemplateGalleryOpen] = useState(false)

  const loadTemplate = useCallback((tpl: WorkflowTemplate, createdAgents?: Record<string, Agent>) => {
    if (nodes.length > 0 && !confirm('Replace the current canvas with this template?')) return
    const idMap: Record<string, string> = {}
    const getId = (key: string) => { if (!idMap[key]) idMap[key] = crypto.randomUUID(); return idMap[key] }
    setNodes(tpl.nodes.map((n) => {
      const agent = createdAgents?.[n.label]
      return {
        id: getId(n.key),
        type: n.type,
        position: { x: n.x, y: n.y },
        data: {
          label: n.label,
          node_type: n.type,
          config: { label: n.label, ...n.config },
          status: 'idle' as const,
          ...(agent ? { agent_id: agent.id, agent_name: agent.name, agent_model: agent.model, agent_provider: agent.provider } : {}),
        },
      }
    }))
    setEdges(tpl.edges.map((e) => ({
      id: `e-${getId(e.from)}-${getId(e.to)}`,
      source: getId(e.from), target: getId(e.to),
      sourceHandle: labelToSourceHandle(e.label),
      ...mkEdgeProps(e.label),
    })))
    if (createdAgents) {
      queryClient.invalidateQueries({ queryKey: ['agents'] })
    }
    setDirty(true)
    setTemplateGalleryOpen(false)
  }, [nodes.length, setNodes, setEdges, queryClient])

  const onDragOver = useCallback((e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }, [])

  const onDrop = useCallback(
    (e: DragEvent<HTMLDivElement>) => {
      e.preventDefault()
      const type = e.dataTransfer.getData('application/reactflow') as WorkflowNodeType
      if (!type || !rfInstance) return
      const position = rfInstance.screenToFlowPosition({ x: e.clientX, y: e.clientY })
      const newNode: Node<NodeData> = {
        id: crypto.randomUUID(),
        type,
        position,
        data: { label: type, node_type: type, config: {}, status: 'idle' },
      }
      setNodes((nds) => [...nds, newNode])
      setDirty(true)
    },
    [rfInstance, setNodes]
  )

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedNode(node as Node<NodeData>)
    setSelectedEdge(null)
  }, [])

  const handleEdgeClick = useCallback((_: React.MouseEvent, edge: Edge) => {
    setSelectedEdge(edge)
    setSelectedNode(null)
  }, [])

  const handlePaneClick = useCallback(() => {
    setSelectedNode(null)
    setSelectedEdge(null)
  }, [])

  const updateEdgeLabel = useCallback((edgeId: string, newLabel: string) => {
    setEdges((eds) => eds.map((e) => e.id === edgeId ? { ...e, ...mkEdgeProps(newLabel || undefined) } : e))
    setSelectedEdge((prev) => prev?.id === edgeId ? { ...prev, label: newLabel || undefined } : prev)
    setDirty(true)
  }, [setEdges])

  const deleteEdge = useCallback((edgeId: string) => {
    setEdges((eds) => eds.filter((e) => e.id !== edgeId))
    setSelectedEdge(null)
    setDirty(true)
  }, [setEdges])

  const updateNodeConfig = useCallback(
    (nodeId: string, patch: Partial<NodeData>) => {
      setNodes((nds) =>
        nds.map((n) =>
          n.id === nodeId
            ? { ...n, data: { ...n.data, ...patch } }
            : n
        )
      )
      setSelectedNode((prev) =>
        prev?.id === nodeId ? { ...prev, data: { ...prev.data, ...patch } } : prev
      )
    },
    [setNodes]
  )

  // Save
  const handleSave = async () => {
    setSaveStatus('saving')
    try {
      const graph: WorkflowGraph = {
        nodes: nodes.map((n) => ({
          id: n.id,
          node_type: n.data.node_type,
          agent_id: (n.data.agent_id as string | null) ?? null,
          position_x: n.position.x,
          position_y: n.position.y,
          config: n.data.config as Record<string, unknown>,
        })),
        edges: edges.map((e) => ({
          id: e.id,
          source_node_id: e.source,
          target_node_id: e.target,
          label: (e.label as string) ?? null,
        })),
      }
      const res = await workflowsAPI.saveGraph(groupId, graph)
      // Node ids are stable across saves (the server upserts by id), so the
      // canvas doesn't need to discard local state and re-sync from a refetch.
      setSaveWarnings(res.warnings ?? [])
      queryClient.invalidateQueries({ queryKey: ['workflow-graph', groupId] })
      setSaveStatus('saved')
      setDirty(false)
      setTimeout(() => setSaveStatus('idle'), 2000)
    } catch {
      setSaveStatus('error')
      setTimeout(() => setSaveStatus('idle'), 3000)
    }
  }

  // Cmd/Ctrl+S saves; warn before leaving with unsaved changes.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
        e.preventDefault()
        handleSave()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })
  useEffect(() => {
    if (!dirty) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => { e.preventDefault() }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [dirty])

  // Run via SSE
  // Streamed text accumulates in a ref and flushes to state at most ~12x/s.
  // A setState per token froze the tab on large supervisor runs: every delta
  // re-rendered the whole studio (canvas included), with cost growing as the
  // output grew.
  const pendingOutput = useRef<Record<string, string>>({})
  const flushTimer = useRef<number | null>(null)
  const queueOutput = useCallback((key: string, chunk: string) => {
    pendingOutput.current[key] = (pendingOutput.current[key] ?? '') + chunk
    if (flushTimer.current == null) {
      flushTimer.current = window.setTimeout(() => {
        const pending = pendingOutput.current
        pendingOutput.current = {}
        flushTimer.current = null
        setRunOutput((prev) => {
          const next = { ...prev }
          for (const k in pending) next[k] = (next[k] ?? '') + pending[k]
          return next
        })
      }, 80)
    }
  }, [])

  // Output console follows the stream while the user is near the bottom;
  // scrolling up to read pauses the follow until they return to the bottom.
  const outputScrollRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = outputScrollRef.current
    if (!el) return
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 140
    if (nearBottom) el.scrollTop = el.scrollHeight
  }, [runOutput])

  const handleRun = async () => {
    if (!runInput.trim()) return
    setRunStatus('running')
    pendingOutput.current = {}
    setRunOutput({})
    setRunError(null)

    // Reset node statuses
    setNodes((nds) => nds.map((n) => ({ ...n, data: { ...n.data, status: 'idle' as const } })))

    try {
      const token = typeof window !== 'undefined' ? localStorage.getItem('access_token') : null
      const url = new URL(`${process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'}/api/v1/invoke/workflows/${groupId}`)
      if (token) url.searchParams.set('token', token)

      const res = await fetch(url.toString(), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
        body: JSON.stringify({ input: runInput, stream: true }),
        credentials: 'include',
      })

      if (!res.ok) {
        const errBody = await res.json().catch(() => ({ error: `HTTP ${res.status}` }))
        setRunError(errBody.error ?? errBody.message ?? `HTTP ${res.status}`)
        setRunStatus('done')
        return
      }
      if (!res.body) {
        setRunError('No response body from server')
        setRunStatus('done')
        return
      }

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''

      // Liveness guard: the engine emits a ping every 15s, so a silent minute
      // means the stream is dead (server restart, dropped connection). Without
      // this, reader.read() blocks forever and the panel spins until the tab
      // is closed.
      let lastEvent = Date.now()
      const liveness = window.setInterval(() => {
        if (Date.now() - lastEvent > 60_000) {
          reader.cancel().catch(() => {})
        }
      }, 5_000)

      try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        lastEvent = Date.now()
        buf += decoder.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() ?? ''
        for (const line of lines) {
          if (!line.startsWith('data:')) continue
          const raw = line.slice(5).trim()
          if (!raw || raw === '[DONE]') continue
          try {
            const evt = JSON.parse(raw) as SSEEvent & { target?: string; ok?: boolean }
            if (evt.type === 'node_started' && evt.node_id) {
              setNodes((nds) => nds.map((n) => n.id === evt.node_id ? { ...n, data: { ...n.data, status: 'running' as const } } : n))
            } else if (evt.type === 'node_completed' && evt.node_id) {
              setNodes((nds) => nds.map((n) => n.id === evt.node_id ? { ...n, data: { ...n.data, status: 'done' as const } } : n))
              if (evt.result) {
                queueOutput(evt.node_id!, evt.result)
              }
            } else if (evt.type === 'node_delivery' && evt.node_id) {
              const note = evt.ok
                ? `\n▸ delivered via ${evt.target}`
                : `\n▸ ${evt.target} delivery failed: ${evt.error ?? 'unknown error'}`
              queueOutput(evt.node_id!, note)
            } else if (evt.type === 'delta' && evt.node_id) {
              queueOutput(evt.node_id, evt.content ?? '')
            } else if (evt.type === 'delta' && evt.content) {
              queueOutput('__main__', evt.content)
            } else if (evt.type === 'run_completed') {
              setRunStatus('done')
            } else if (evt.type === 'error') {
              setRunError(evt.error ?? evt.message ?? 'Workflow execution error')
              setRunStatus('done')
            }
          } catch {
            // non-json line, ignore
          }
        }
      }
      } finally {
        window.clearInterval(liveness)
      }
      setRunStatus((prev) => {
        if (prev === 'running') {
          setRunError((e) => e ?? 'Stream ended without completing — the run may still be executing server-side. Check the Runs page.')
        }
        return 'done'
      })
    } catch (err) {
      setRunError(err instanceof Error ? err.message : 'Network error — could not reach the server')
      setRunStatus('done')
    }
  }

  const hasNodes = nodes.length > 0
  const selectedTheme = selectedNode ? (NODE_THEME[selectedNode.data.node_type] ?? NODE_THEME.agent) : null

  const inputCls = 'w-full rounded-lg border border-border bg-background px-3 py-2 text-xs text-foreground outline-none transition-colors focus:border-accent focus:ring-2 focus:ring-accent/20 placeholder:text-faint'
  const hintCls = 'mt-1.5 text-[10px] leading-relaxed text-faint'
  const infoBoxCls = 'rounded-lg border border-border bg-muted/50 px-3 py-2.5 text-[11px] leading-relaxed text-muted-foreground'

  // Patch one key inside the node's config object.
  const patchConfig = (key: string, value: unknown) => {
    if (!selectedNode) return
    updateNodeConfig(selectedNode.id, { config: { ...selectedNode.data.config, [key]: value } })
  }

  const selectedToolMeta = selectedNode?.data.node_type === 'tool'
    ? workspaceTools.find((t) => t.name === (selectedNode.data.config?.tool_name as string))
    : undefined

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <style>{`
        :root { --wf-label-bg: #ffffff; --wf-dots: #d6d6de; }
        .dark { --wf-label-bg: #1b1926; --wf-dots: #2e2c3a; }
        .react-flow__controls { border-radius: 10px; overflow: hidden; border: 1px solid hsl(var(--border)); box-shadow: 0 1px 2px rgba(21,26,31,.06); }
        .react-flow__controls-button { background: hsl(var(--surface)); border-bottom: 1px solid hsl(var(--border)); color: hsl(var(--muted-foreground)); width: 26px; height: 26px; }
        .react-flow__controls-button:hover { background: hsl(var(--muted)); }
        .react-flow__controls-button svg { fill: currentColor; }
        .react-flow__minimap { border-radius: 10px; }
        .react-flow__edge-textbg { rx: 4px; }
      `}</style>

      {/* Top bar */}
      <div className="flex min-h-12 flex-shrink-0 items-center gap-2 overflow-x-auto border-b border-border bg-surface px-3">
        <button
          onClick={() => router.push('/workflows')}
          className="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <ArrowLeft size={14} /> Workflows
        </button>
        <div className="h-5 w-px bg-border" />
        <span className="truncate text-sm font-semibold text-foreground">{groupData?.name ?? 'Workflow'}</span>
        {groupData?.mode && (
          <span className="rounded-full border border-border bg-muted px-2 py-0.5 font-mono text-[10px] uppercase tracking-wide text-muted-foreground">
            {groupData.mode}
          </span>
        )}
        {dirty && (
          <span className="flex items-center gap-1 font-mono text-[10px] text-warn">
            <CircleDot size={9} /> unsaved
          </span>
        )}
        <div className="flex-1" />
        <button
          onClick={() => setTemplateGalleryOpen(true)}
          title="Browse workflow templates"
          className="flex items-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted"
        >
          <LayoutTemplate size={13} /> Templates
        </button>
        <button
          onClick={() => { setTriggersPanelOpen(v => !v); setSelectedNode(null); setSelectedEdge(null) }}
          title="Webhook triggers that start this workflow"
          className={`flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors ${
            triggersPanelOpen
              ? 'border-accent/40 bg-accent-light text-accent dark:bg-accent/15 dark:text-accent-bright'
              : 'border-border bg-surface text-foreground hover:bg-muted'
          }`}
        >
          <Zap size={13} /> Triggers
        </button>
        <button
          onClick={handleSave}
          disabled={saveStatus === 'saving'}
          title="Save (⌘S)"
          className={`flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors ${
            saveStatus === 'saved' ? 'border-good/40 bg-good/10 text-good'
            : saveStatus === 'error' ? 'border-crit/40 bg-crit/10 text-crit'
            : 'border-border bg-surface text-foreground hover:bg-muted'
          }`}
        >
          {saveStatus === 'saving' ? <Loader2 size={13} className="animate-spin" /> : <Save size={13} />}
          {saveStatus === 'saving' ? 'Saving…' : saveStatus === 'saved' ? 'Saved' : saveStatus === 'error' ? 'Save failed' : 'Save'}
        </button>
        <button
          onClick={() => setRunPanelOpen(true)}
          className="flex items-center gap-1.5 rounded-[10px] bg-gradient-to-br from-accent to-accent-ink px-4 py-1.5 text-xs font-semibold text-white shadow-card transition-opacity hover:opacity-90"
        >
          <Play size={13} /> Run
        </button>
      </div>

      {/* Save validation warnings */}
      {saveWarnings.length > 0 && (
        <div className="flex flex-shrink-0 flex-wrap items-start gap-2 border-b border-warn/30 bg-warn/10 px-4 py-2">
          <span className="flex-shrink-0 text-xs font-semibold text-warn">Save warnings:</span>
          <ul className="m-0 flex min-w-0 flex-1 list-none flex-col gap-0.5 p-0">
            {saveWarnings.map((warning, i) => (
              <li key={i} className="text-xs text-warn">{warning}</li>
            ))}
          </ul>
          <button
            onClick={() => setSaveWarnings([])}
            className="flex-shrink-0 text-xs font-semibold text-warn hover:underline"
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Body */}
      <div className="flex flex-1 overflow-hidden">
        {/* Palette */}
        <div className="w-56 flex-shrink-0 space-y-4 overflow-y-auto border-r border-border bg-surface p-3">
          {PALETTE_GROUPS.map((group) => (
            <div key={group.name}>
              <div className="mb-1.5 px-1 font-mono text-[10px] font-semibold uppercase tracking-wider text-faint">{group.name}</div>
              <div className="space-y-0.5">
                {group.items.map((type) => {
                  const theme = NODE_THEME[type]
                  const Icon = theme.icon
                  return (
                    <div
                      key={type}
                      draggable
                      onDragStart={(e) => {
                        e.dataTransfer.setData('application/reactflow', type)
                        e.dataTransfer.effectAllowed = 'move'
                      }}
                      className="flex cursor-grab select-none items-center gap-2.5 rounded-lg border border-transparent px-2 py-1.5 transition-colors hover:border-border hover:bg-muted active:cursor-grabbing"
                      title={theme.blurb}
                    >
                      <span className="grid h-6 w-6 flex-shrink-0 place-items-center rounded-md" style={{ background: `${theme.accent}18`, color: theme.accent }}>
                        <Icon size={12.5} />
                      </span>
                      <div className="min-w-0">
                        <div className="text-xs font-medium text-foreground">{theme.title}</div>
                        <div className="truncate text-[10px] leading-tight text-faint">{theme.blurb}</div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          ))}
          <div className="rounded-lg border border-dashed border-border px-2.5 py-2 text-[10px] leading-relaxed text-faint">
            Drag a node onto the canvas, then connect handles to draw edges. Press ⌫ to delete a selection.
          </div>
        </div>

        {/* Canvas */}
        <div
          ref={reactFlowWrapper}
          className="relative h-full flex-1"
          onDragOver={onDragOver}
          onDrop={onDrop}
        >
          {graphLoading && (
            <div className="absolute inset-0 z-10 grid place-items-center bg-background/70 backdrop-blur-[1px]">
              <Loader2 size={24} className="animate-spin text-accent" />
            </div>
          )}
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeClick={handleNodeClick}
            onEdgeClick={handleEdgeClick}
            onPaneClick={handlePaneClick}
            onInit={setRfInstance}
            nodeTypes={nodeTypes}
            fitView
            elevateEdgesOnSelect
            className="!bg-background"
            deleteKeyCode="Backspace"
          >
            <Background variant={BackgroundVariant.Dots} color="var(--wf-dots)" gap={18} size={1.4} />
            <Controls style={{ bottom: 20, left: 20 }} showInteractive={false} />
            <MiniMap
              pannable
              zoomable
              nodeColor={(n) => (NODE_THEME[(n as Node<NodeData>).data?.node_type ?? 'agent'] ?? NODE_THEME.agent).accent}
              nodeStrokeWidth={0}
              style={{ bottom: 20, right: 20, width: 160, height: 110, border: '1px solid hsl(var(--border))', borderRadius: 10, background: 'hsl(var(--surface))' }}
              maskColor="hsl(var(--muted) / 0.7)"
            />
            {!hasNodes && !graphLoading && (
              <div className="pointer-events-none absolute inset-0 z-[5] grid place-items-center">
                <div className="pointer-events-auto flex max-w-sm flex-col items-center rounded-2xl border border-dashed border-border bg-surface/80 px-8 py-8 text-center shadow-card backdrop-blur-sm">
                  <span className="mb-3 grid h-11 w-11 place-items-center rounded-xl bg-accent-light text-accent dark:bg-accent/15 dark:text-accent-bright">
                    <GitBranch size={20} />
                  </span>
                  <p className="text-sm font-semibold text-foreground">Design your workflow</p>
                  <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                    Drag nodes from the left panel and connect their handles — or start from a proven template.
                  </p>
                  <button
                    onClick={() => setTemplateGalleryOpen(true)}
                    className="mt-4 flex items-center gap-1.5 rounded-[10px] bg-gradient-to-br from-accent to-accent-ink px-4 py-2 text-xs font-semibold text-white shadow-card transition-opacity hover:opacity-90"
                  >
                    <LayoutTemplate size={13} /> Browse templates
                  </button>
                </div>
              </div>
            )}
          </ReactFlow>
        </div>

        {/* Edge config panel */}
        {selectedEdge && !selectedNode && (
          <div className="flex w-80 flex-shrink-0 flex-col gap-4 overflow-y-auto border-l border-border bg-surface p-4">
            <div className="flex items-center justify-between">
              <span className="font-mono text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Edge</span>
              <button onClick={() => setSelectedEdge(null)} className="text-faint transition-colors hover:text-foreground" aria-label="Close edge panel">
                <X size={14} />
              </button>
            </div>

            <ConfigField label="Label">
              <input
                value={(selectedEdge.label as string) || ''}
                onChange={(e) => updateEdgeLabel(selectedEdge.id, e.target.value)}
                className={inputCls}
                placeholder="e.g. yes, no, loop, exit"
              />
              <div className="mt-2 flex flex-wrap gap-1.5">
                {['yes', 'no', 'loop', 'exit', 'delegate'].map((preset) => (
                  <button
                    key={preset}
                    onClick={() => updateEdgeLabel(selectedEdge.id, preset)}
                    className="rounded-md px-2 py-0.5 font-mono text-[10px] font-semibold transition-opacity hover:opacity-80"
                    style={selectedEdge.label === preset
                      ? { background: edgeColor(preset), color: '#fff' }
                      : { background: `${edgeColor(preset)}1a`, color: edgeColor(preset) }}
                  >
                    {preset}
                  </button>
                ))}
                <button
                  onClick={() => updateEdgeLabel(selectedEdge.id, '')}
                  className="rounded-md bg-muted px-2 py-0.5 font-mono text-[10px] text-muted-foreground hover:text-foreground"
                >
                  clear
                </button>
              </div>
            </ConfigField>

            <div className={infoBoxCls}>
              <strong>yes / no</strong> — condition routing<br />
              <strong>loop</strong> — loop-back edge · <strong>exit</strong> — loop exit<br />
              <strong>delegate</strong> — supervisor → team agent (dashed)<br />
              <strong>(empty)</strong> — default / unconditional
            </div>

            <button
              onClick={() => deleteEdge(selectedEdge.id)}
              className="mt-auto flex items-center justify-center gap-1.5 rounded-lg border border-crit/30 px-3 py-2 text-xs font-medium text-crit transition-colors hover:bg-crit/10"
            >
              <X size={12} /> Remove edge
            </button>
          </div>
        )}

        {/* Triggers panel */}
        {triggersPanelOpen && !selectedNode && !selectedEdge && (
          <TriggersPanel
            workflowId={groupId}
            onClose={() => setTriggersPanelOpen(false)}
          />
        )}

        {/* Node config panel */}
        {selectedNode && selectedTheme && (
          <div className="flex w-80 flex-shrink-0 flex-col gap-4 overflow-y-auto border-l border-border bg-surface p-4">
            <div className="flex items-center justify-between">
              <span className="flex items-center gap-2">
                <span className="grid h-6 w-6 place-items-center rounded-md" style={{ background: `${selectedTheme.accent}18`, color: selectedTheme.accent }}>
                  <selectedTheme.icon size={12.5} />
                </span>
                <span className="font-mono text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                  {selectedNode.data.node_type}
                </span>
              </span>
              <button onClick={() => setSelectedNode(null)} className="text-faint transition-colors hover:text-foreground" aria-label="Close node panel">
                <X size={14} />
              </button>
            </div>

            {/* Label */}
            {selectedNode.data.node_type !== 'start' && (
              <ConfigField label="Label">
                <input
                  value={(selectedNode.data.config?.label as string) || selectedNode.data.label || ''}
                  onChange={(e) => updateNodeConfig(selectedNode.id, {
                    label: e.target.value,
                    config: { ...selectedNode.data.config, label: e.target.value },
                  })}
                  className={inputCls}
                  placeholder="Node label"
                />
              </ConfigField>
            )}

            {/* Agent / Supervisor node: agent picker */}
            {(selectedNode.data.node_type === 'agent' || selectedNode.data.node_type === 'supervisor') && (
              <ConfigField label={selectedNode.data.node_type === 'supervisor' ? 'Supervisor agent' : 'Agent'}>
                <select
                  value={(selectedNode.data.agent_id as string) || ''}
                  onChange={(e) => {
                    const agent = agents.find((a) => a.id === e.target.value)
                    updateNodeConfig(selectedNode.id, {
                      agent_id: e.target.value || null,
                      label: agent?.name ?? selectedNode.data.label,
                      agent_name: agent?.name,
                      agent_model: agent?.model,
                      agent_provider: agent?.provider,
                    })
                  }}
                  className={inputCls}
                >
                  <option value="">Select agent…</option>
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>{a.name}</option>
                  ))}
                </select>
                {selectedNode.data.agent_model && (
                  <div className="mt-2 flex gap-1.5">
                    <span className="rounded-md bg-accent-light px-1.5 py-0.5 font-mono text-[10px] text-accent dark:bg-accent/15 dark:text-accent-bright">{selectedNode.data.agent_model as string}</span>
                    <span className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">{selectedNode.data.agent_provider as string}</span>
                  </div>
                )}
                {selectedNode.data.node_type === 'supervisor' && (
                  <div className={`${hintCls} !text-warn`}>
                    Runs in an agentic loop, calling team agents (dashed delegate edges) as tools. Use a capable model.
                  </div>
                )}
              </ConfigField>
            )}

            {/* Condition node */}
            {selectedNode.data.node_type === 'condition' && (
              <ConfigField label="Expression">
                <input
                  value={(selectedNode.data.config?.expression as string) || ''}
                  onChange={(e) => patchConfig('expression', e.target.value)}
                  className={`${inputCls} font-mono`}
                  placeholder="contains:APPROVED"
                />
                <div className={hintCls}>
                  Evaluated against the previous node&apos;s output: <code>contains:</code>, <code>not_contains:</code>, <code>equals:</code>
                </div>
              </ConfigField>
            )}

            {/* Loop node */}
            {selectedNode.data.node_type === 'loop' && (
              <>
                <ConfigField label="Exit condition">
                  <input
                    value={(selectedNode.data.config?.exit_condition as string) || ''}
                    onChange={(e) => patchConfig('exit_condition', e.target.value)}
                    className={`${inputCls} font-mono`}
                    placeholder="contains:done"
                  />
                  <div className={hintCls}>Loops while the condition is NOT met. Same syntax as the condition node.</div>
                </ConfigField>
                <ConfigField label="Max iterations">
                  <input
                    type="number"
                    min={1}
                    max={100}
                    value={(selectedNode.data.config?.max_iterations as number) || 5}
                    onChange={(e) => patchConfig('max_iterations', parseInt(e.target.value, 10) || 5)}
                    className={inputCls}
                  />
                </ConfigField>
                <div className={infoBoxCls}>
                  Drag from <strong>↩ loop</strong> back to the node to repeat (edge auto-labelled <code>loop</code>), and from <strong>exit →</strong> to the forward path.
                </div>
              </>
            )}

            {/* Tool node */}
            {selectedNode.data.node_type === 'tool' && (
              <>
                <ConfigField label="Tool">
                  <select
                    value={(selectedNode.data.config?.tool_name as string) || ''}
                    onChange={(e) => {
                      const t = workspaceTools.find((wt) => wt.name === e.target.value)
                      updateNodeConfig(selectedNode.id, {
                        label: (selectedNode.data.config?.label as string) || t?.name || selectedNode.data.label,
                        config: { ...selectedNode.data.config, tool_name: e.target.value },
                      })
                    }}
                    className={`${inputCls} font-mono`}
                  >
                    <option value="">Select tool…</option>
                    {workspaceTools.map((t) => (
                      <option key={t.id} value={t.name}>{t.name} ({t.type})</option>
                    ))}
                  </select>
                  {selectedToolMeta?.description && <div className={hintCls}>{selectedToolMeta.description}</div>}
                  {selectedToolMeta?.requires_approval && (
                    <div className={`${hintCls} !text-warn`}>
                      This tool requires approval — workflow tool nodes run unattended, so the run will refuse it. Clear the approval flag or call it from an agent node instead.
                    </div>
                  )}
                </ConfigField>
                <ConfigField label="Arguments (JSON, optional)">
                  <textarea
                    value={(selectedNode.data.config?.args_text as string) ?? (selectedNode.data.config?.args ? JSON.stringify(selectedNode.data.config.args, null, 2) : '')}
                    onChange={(e) => {
                      const text = e.target.value
                      let next: Record<string, unknown> = { ...selectedNode.data.config, args_text: text }
                      if (!text.trim()) {
                        const { args: _drop, args_text: _drop2, ...rest } = next
                        next = rest
                      } else {
                        try { next.args = JSON.parse(text) } catch { /* keep last valid args */ }
                      }
                      updateNodeConfig(selectedNode.id, { config: next })
                    }}
                    rows={5}
                    className={`${inputCls} resize-y font-mono`}
                    placeholder={'{\n  "query": "{{input}}"\n}'}
                  />
                  <div className={hintCls}>
                    String values may use <code>{'{{input}}'}</code> (previous output) and <code>{'{{original_input}}'}</code>. Without arguments the tool receives <code>{'{"input": …}'}</code>.
                  </div>
                  {typeof selectedNode.data.config?.args_text === 'string' && (selectedNode.data.config.args_text as string).trim() !== '' && (() => {
                    try { JSON.parse(selectedNode.data.config.args_text as string); return null } catch { return <div className={`${hintCls} !text-crit`}>Invalid JSON — the last valid value will be used.</div> }
                  })()}
                </ConfigField>
              </>
            )}

            {/* Webhook node */}
            {selectedNode.data.node_type === 'webhook' && (
              <>
                <ConfigField label="URL">
                  <input
                    value={(selectedNode.data.config?.url as string) || ''}
                    onChange={(e) => patchConfig('url', e.target.value)}
                    className={`${inputCls} font-mono`}
                    placeholder="https://example.com/hook"
                  />
                </ConfigField>
                <ConfigField label="Method">
                  <select
                    value={((selectedNode.data.config?.method as string) || 'POST').toUpperCase()}
                    onChange={(e) => patchConfig('method', e.target.value)}
                    className={inputCls}
                  >
                    {['POST', 'PUT', 'PATCH', 'GET'].map((m) => <option key={m} value={m}>{m}</option>)}
                  </select>
                </ConfigField>
                <ConfigField label="Payload template (optional)">
                  <textarea
                    value={(selectedNode.data.config?.payload_template as string) || ''}
                    onChange={(e) => patchConfig('payload_template', e.target.value || undefined)}
                    rows={4}
                    className={`${inputCls} resize-y font-mono`}
                    placeholder={'{"text": "{{input}}"}'}
                  />
                  <div className={hintCls}>
                    Default payload: JSON with <code>workflow_id</code>, <code>run_id</code>, <code>node_id</code> and <code>input</code>. The response body becomes this node&apos;s output.
                  </div>
                </ConfigField>
              </>
            )}

            {/* Gateway node */}
            {selectedNode.data.node_type === 'gateway' && (
              <>
                <ConfigField label="Channel">
                  <select
                    value={(selectedNode.data.config?.channel_id as string) || ''}
                    onChange={(e) => patchConfig('channel_id', e.target.value)}
                    className={inputCls}
                  >
                    <option value="">Select channel…</option>
                    {gatewayChannels.map((c) => (
                      <option key={c.id} value={c.id}>{c.name} ({c.channel_type})</option>
                    ))}
                  </select>
                  {gatewayChannels.length === 0 && (
                    <div className={hintCls}>No gateway channels yet — create one on the Gateway page.</div>
                  )}
                </ConfigField>
                <ConfigField label="Recipient">
                  <input
                    value={(selectedNode.data.config?.peer_id as string) || ''}
                    onChange={(e) => patchConfig('peer_id', e.target.value)}
                    className={`${inputCls} font-mono`}
                    placeholder="+15551234567 or JID"
                  />
                </ConfigField>
                <ConfigField label="Message template (optional)">
                  <textarea
                    value={(selectedNode.data.config?.message_template as string) || ''}
                    onChange={(e) => patchConfig('message_template', e.target.value || undefined)}
                    rows={3}
                    className={`${inputCls} resize-y`}
                    placeholder="Workflow update: {{input}}"
                  />
                  <div className={hintCls}>Defaults to the previous node&apos;s output. The output passes through unchanged.</div>
                </ConfigField>
              </>
            )}

            {/* Start node: inbound attachments */}
            {selectedNode.data.node_type === 'start' && (
              <>
                <div className={infoBoxCls}>
                  Entry point — every run begins here with the trigger&apos;s input.
                </div>
                <ConfigField label="Inbound webhook">
                  <button
                    onClick={() => { setSelectedNode(null); setSelectedEdge(null); setTriggersPanelOpen(true) }}
                    className="flex w-full items-center justify-center gap-1.5 rounded-lg border border-border px-3 py-2 text-xs font-medium text-foreground transition-colors hover:bg-muted"
                  >
                    <Zap size={12} /> Manage webhook triggers
                  </button>
                  <div className={hintCls}>
                    Webhook triggers start this workflow from external events (Jira, GitHub, custom systems) with a templated input.
                  </div>
                </ConfigField>
              </>
            )}

            {/* End node: delivery config */}
            {selectedNode.data.node_type === 'end' && (
              <>
                <div className={infoBoxCls}>
                  Terminal node. Optionally deliver the final output when the run finishes here.
                </div>
                <ConfigField label="Deliver to webhook (optional)">
                  <input
                    value={(selectedNode.data.config?.webhook_url as string) || ''}
                    onChange={(e) => patchConfig('webhook_url', e.target.value || undefined)}
                    className={`${inputCls} font-mono`}
                    placeholder="https://example.com/hook"
                  />
                  <div className={hintCls}>POSTs JSON with <code>workflow_id</code>, <code>run_id</code> and the final output.</div>
                </ConfigField>
                <ConfigField label="Deliver to gateway (optional)">
                  <select
                    value={(selectedNode.data.config?.gateway_channel_id as string) || ''}
                    onChange={(e) => patchConfig('gateway_channel_id', e.target.value || undefined)}
                    className={inputCls}
                  >
                    <option value="">No gateway delivery</option>
                    {gatewayChannels.map((c) => (
                      <option key={c.id} value={c.id}>{c.name} ({c.channel_type})</option>
                    ))}
                  </select>
                  {!!(selectedNode.data.config?.gateway_channel_id as string) && (
                    <input
                      value={(selectedNode.data.config?.gateway_peer_id as string) || ''}
                      onChange={(e) => patchConfig('gateway_peer_id', e.target.value || undefined)}
                      className={`${inputCls} mt-2 font-mono`}
                      placeholder="Recipient: +15551234567 or JID"
                    />
                  )}
                </ConfigField>
              </>
            )}

            {/* Parallel / Join: info */}
            {(selectedNode.data.node_type === 'parallel' || selectedNode.data.node_type === 'join') && (
              <div className={infoBoxCls}>
                {selectedNode.data.node_type === 'parallel' && 'Fan-out: each outgoing edge runs concurrently. Use a Join node to collect the branch outputs.'}
                {selectedNode.data.node_type === 'join' && 'Merges outputs from parallel branches into a single concatenated output before continuing.'}
              </div>
            )}

            {selectedNode.data.node_type === 'supervisor' && !selectedNode.data.agent_id && (
              <div className={infoBoxCls}>
                <strong>How supervisor works:</strong> pick a coordinator agent, drag from the right handle to the next pipeline node, and from the bottom handle to each team agent (auto-labelled <code>delegate</code>). At runtime team agents become callable tools.
              </div>
            )}

            <button
              onClick={() => {
                setNodes((nds) => nds.filter((n) => n.id !== selectedNode.id))
                setEdges((eds) => eds.filter((e) => e.source !== selectedNode.id && e.target !== selectedNode.id))
                setSelectedNode(null)
                setDirty(true)
              }}
              className="mt-auto flex items-center justify-center gap-1.5 rounded-lg border border-crit/30 px-3 py-2 text-xs font-medium text-crit transition-colors hover:bg-crit/10"
            >
              <X size={12} /> Remove node
            </button>
          </div>
        )}
      </div>

      {/* Template gallery modal */}
      {templateGalleryOpen && (
        <TemplateGalleryModal
          onSelect={loadTemplate}
          onClose={() => setTemplateGalleryOpen(false)}
        />
      )}

      {/* Run panel — portaled to <body> so ancestor overflow/transform styles
          in the app shell can never clip or re-anchor the fixed drawer. */}
      {runPanelOpen && createPortal(
        <div className="fixed inset-x-0 bottom-0 z-50 flex max-h-[50vh] min-h-60 flex-col border-t border-border bg-surface shadow-[0_-8px_24px_-12px_rgba(21,26,31,.18)]">
          <div className="flex flex-shrink-0 items-center gap-3 border-b border-border px-4 py-2.5">
            <span className="grid h-6 w-6 place-items-center rounded-md bg-accent-light text-accent dark:bg-accent/20 dark:text-accent-bright">
              <Play size={12} />
            </span>
            <span className="text-xs font-semibold text-foreground">Run workflow</span>
            {runStatus === 'running' && (
              <span className="flex items-center gap-1.5 font-mono text-[10px] text-accent dark:text-accent-bright">
                <Loader2 size={10} className="animate-spin" /> running
              </span>
            )}
            {runStatus === 'done' && !runError && (
              <span className="font-mono text-[10px] text-good">completed</span>
            )}
            <div className="flex-1" />
            <button
              onClick={() => { setRunPanelOpen(false); setRunStatus('idle'); setRunError(null) }}
              className="text-faint transition-colors hover:text-foreground"
              aria-label="Close run panel"
            >
              <X size={14} />
            </button>
          </div>

          <div className="flex flex-1 flex-wrap overflow-hidden">
            {/* Input area */}
            <div className="flex min-h-0 w-2/5 min-w-52 flex-shrink-0 flex-col gap-2 overflow-hidden border-r border-border p-3">
              <textarea
                value={runInput}
                onChange={(e) => setRunInput(e.target.value)}
                onKeyDown={(e) => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') handleRun() }}
                placeholder="Describe the task for this run… (⌘⏎ to run)"
                className="flex-1 resize-none rounded-lg border border-border bg-background px-3 py-2 text-xs text-foreground outline-none placeholder:text-faint focus:border-accent focus:ring-2 focus:ring-accent/20"
              />
              {runError && (
                <div className="rounded-lg border border-crit/30 bg-crit/10 px-3 py-2 text-[11px] leading-snug text-crit">
                  {runError}
                </div>
              )}
              <button
                onClick={handleRun}
                disabled={!runInput.trim() || runStatus === 'running'}
                className="flex items-center justify-center gap-1.5 rounded-lg bg-gradient-to-br from-accent to-accent-ink px-4 py-2 text-xs font-semibold text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
              >
                {runStatus === 'running'
                  ? <><Loader2 size={12} className="animate-spin" /> Running…</>
                  : <><Play size={12} /> Run workflow</>
                }
              </button>
            </div>

            {/* Output area */}
            <div ref={outputScrollRef} className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-3">
              {Object.keys(runOutput).length === 0 && (
                <div className="mt-8 text-center font-mono text-[11px] text-faint">
                  {runStatus === 'running' ? 'Waiting for the first node…' : 'Node outputs appear here as the run progresses.'}
                </div>
              )}
              {Object.entries(runOutput).map(([nodeId, text]) => {
                const node = nodes.find((n) => n.id === nodeId)
                const label = nodeId === '__main__' ? 'Output' : node?.data?.label || nodeId
                return (
                  <div key={nodeId} className="rounded-lg border border-border bg-muted/40 px-3 py-2.5">
                    <div className="mb-1.5 flex items-center gap-1.5">
                      <ChevronRight size={11} className="text-accent dark:text-accent-bright" />
                      <span className="text-[11px] font-semibold text-accent dark:text-accent-bright">{label}</span>
                    </div>
                    <pre className="m-0 whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed text-foreground/90">{text}</pre>
                  </div>
                )
              })}
            </div>
          </div>
        </div>,
        document.body,
      )}
    </div>
  )
}

function ConfigField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="mb-1.5 font-mono text-[10px] font-semibold uppercase tracking-wider text-faint">{label}</div>
      {children}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Page export — wrapped in ReactFlowProvider
// ---------------------------------------------------------------------------
export default function WorkflowBuilderPage({ params }: { params: Promise<{ workflowId: string }> }) {
  const { workflowId } = use(params)
  return (
    <ReactFlowProvider>
      <WorkflowBuilderInner groupId={workflowId} />
    </ReactFlowProvider>
  )
}
