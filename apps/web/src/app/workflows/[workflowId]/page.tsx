'use client'

// Assumptions:
// - @xyflow/react v12 is installed
// - workflowsAPI.getGraph / saveGraph hit /api/v1/workflows/:id/graph
// - invokeAPI.workflow hits /api/v1/invoke/workflows/:id with { input, stream: true }
// - SSE node events carry node_id, node_name, node_type, result fields (added to SSEEvent type)

import React, { use, useCallback, useEffect, useRef, useState, DragEvent } from 'react'
import { useParams, useRouter } from 'next/navigation'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ReactFlow,
  Background,
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
  Merge,
  RefreshCcw,
  X,
  ChevronRight,
  Loader2,
  LayoutTemplate,
  ChevronDown,
  ChevronUp,
  BookOpen,
  Zap,
} from 'lucide-react'
import { TriggersPanel } from './TriggersPanel'
import { workflowsAPI, agentsAPI, invokeAPI } from '@/lib/api'
import type { Workflow, Agent, WorkflowGraph, WorkflowNode, WorkflowEdge, WorkflowNodeType, SSEEvent } from '@/types'

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
  if (label === 'yes') return '#22c55e'
  if (label === 'no') return '#ef4444'
  if (label === 'loop') return '#8b5cf6'
  if (label === 'delegate') return '#d97706'
  return '#534AB7'
}

function labelToSourceHandle(label?: string | null): string | undefined {
  if (label === 'yes') return 'yes'
  if (label === 'no') return 'no'
  if (label === 'loop') return 'continue'
  if (label === 'exit') return 'exit'
  if (label === 'delegate') return 'delegate'
  return undefined
}

function toRFEdge(e: WorkflowEdge): Edge {
  const color = edgeColor(e.label)
  const sourceHandle = labelToSourceHandle(e.label)
  const isDelegate = e.label === 'delegate'
  return {
    id: e.id || `e-${e.source_node_id}-${e.target_node_id}`,
    source: e.source_node_id,
    target: e.target_node_id,
    sourceHandle: sourceHandle,
    label: e.label || undefined,
    type: 'smoothstep',
    animated: true,
    zIndex: 1000,
    markerEnd: { type: MarkerType.ArrowClosed, color, width: 18, height: 18 },
    style: { stroke: color, strokeWidth: isDelegate ? 2 : 2.5, strokeDasharray: isDelegate ? '6 3' : undefined },
    labelStyle: { fontSize: 10, fill: color, fontWeight: 700 },
    labelBgStyle: { fill: '#fff', fillOpacity: 0.9 },
  }
}

// ---------------------------------------------------------------------------
// Custom node components
// ---------------------------------------------------------------------------

function EndNode({ data }: NodeProps) {
  const nodeData = data as NodeData
  const glow = nodeData.status === 'running' ? '0 0 0 3px rgba(239,68,68,0.5)' : nodeData.status === 'done' ? '0 0 0 2px #22c55e' : 'none'
  return (
    <div style={{
      width: 64, height: 64, borderRadius: '50%', background: '#ef4444',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      boxShadow: glow, position: 'relative', color: '#fff', fontSize: 11, fontWeight: 700,
      fontFamily: 'Inter, sans-serif',
    }}>
      {nodeData.status === 'done' && (
        <div style={{ position: 'absolute', top: -6, right: -6, background: '#22c55e', borderRadius: '50%', width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, color: '#fff' }}>✓</div>
      )}
      End
      <Handle type="target" position={Position.Left} style={{ background: '#ef4444', width: 10, height: 10, border: '2px solid #fff' }} />
    </div>
  )
}

function StartNode({ data }: NodeProps) {
  const nodeData = data as NodeData
  const glow = nodeData.status === 'running' ? '0 0 0 3px rgba(83,74,183,0.6)' : nodeData.status === 'done' ? '0 0 0 2px #22c55e' : 'none'
  return (
    <div style={{
      width: 64, height: 64, borderRadius: '50%', background: '#22c55e',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      boxShadow: glow, position: 'relative', color: '#fff', fontSize: 11, fontWeight: 700,
      fontFamily: 'Inter, sans-serif',
    }}>
      {nodeData.status === 'done' && (
        <div style={{ position: 'absolute', top: -6, right: -6, background: '#22c55e', borderRadius: '50%', width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, color: '#fff' }}>✓</div>
      )}
      Start
      <Handle type="source" position={Position.Right} style={{ background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
    </div>
  )
}

function AgentNode({ data }: NodeProps) {
  const nodeData = data as NodeData
  const borderColor = nodeData.status === 'running' ? '#534AB7' : nodeData.status === 'done' ? '#22c55e' : nodeData.status === 'error' ? '#ef4444' : '#534AB7'
  const glow = nodeData.status === 'running' ? '0 0 0 3px rgba(83,74,183,0.45)' : 'none'
  return (
    <div style={{
      minWidth: 160, background: '#fff', border: `2px solid ${borderColor}`,
      borderRadius: 10, padding: '10px 14px', fontFamily: 'Inter, sans-serif',
      boxShadow: glow, position: 'relative',
    }}>
      {nodeData.status === 'done' && (
        <div style={{ position: 'absolute', top: -7, right: -7, background: '#22c55e', borderRadius: '50%', width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, color: '#fff' }}>✓</div>
      )}
      {nodeData.status === 'running' && (
        <div style={{ position: 'absolute', top: -7, right: -7, background: '#534AB7', borderRadius: '50%', width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <div style={{ width: 8, height: 8, border: '1.5px solid #fff', borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.7s linear infinite' }} />
        </div>
      )}
      <Handle type="target" position={Position.Left} style={{ background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
      <div style={{ fontSize: 12, fontWeight: 600, color: '#1a1825', marginBottom: 4 }}>{nodeData.label}</div>
      {nodeData.agent_model && (
        <div style={{ display: 'flex', gap: 4 }}>
          <span style={{ fontSize: 10, background: '#f1f0ff', color: '#534AB7', borderRadius: 4, padding: '1px 6px' }}>{nodeData.agent_model}</span>
          {nodeData.agent_provider && <span style={{ fontSize: 10, background: '#f4f4f5', color: '#71717a', borderRadius: 4, padding: '1px 6px' }}>{nodeData.agent_provider}</span>}
        </div>
      )}
      <Handle type="source" position={Position.Right} style={{ background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
    </div>
  )
}

function ConditionNode({ data }: NodeProps) {
  const nodeData = data as NodeData
  const expr = (nodeData.config?.expression as string) || 'condition'
  const glow = nodeData.status === 'running' ? '0 0 0 3px rgba(245,158,11,0.5)' : 'none'
  return (
    <div style={{ position: 'relative', width: 100, height: 100, fontFamily: 'Inter, sans-serif' }}>
      <div style={{
        position: 'absolute', inset: 0,
        transform: 'rotate(45deg)', background: '#fff',
        border: '2px solid #f59e0b', borderRadius: 6,
        boxShadow: glow,
      }} />
      <div style={{
        position: 'absolute', inset: 0,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        flexDirection: 'column', padding: 8,
      }}>
        <div style={{ fontSize: 10, fontWeight: 700, color: '#92400e', marginBottom: 2 }}>COND</div>
        <div style={{ fontSize: 9, color: '#b45309', textAlign: 'center', maxWidth: 70, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{expr}</div>
      </div>
      <Handle type="target" position={Position.Left} style={{ top: '50%', background: '#f59e0b', width: 10, height: 10, border: '2px solid #fff' }} />
      <Handle type="source" id="yes" position={Position.Right} style={{ top: '50%', background: '#22c55e', width: 10, height: 10, border: '2px solid #fff' }} />
      <Handle type="source" id="no" position={Position.Bottom} style={{ left: '50%', background: '#ef4444', width: 10, height: 10, border: '2px solid #fff' }} />
      <div style={{ position: 'absolute', right: -22, top: '44%', fontSize: 9, color: '#22c55e', fontWeight: 700 }}>yes</div>
      <div style={{ position: 'absolute', bottom: -18, left: '44%', fontSize: 9, color: '#ef4444', fontWeight: 700 }}>no</div>
    </div>
  )
}

function ParallelNode({ data }: NodeProps) {
  const nodeData = data as NodeData
  const glow = nodeData.status === 'running' ? '0 0 0 3px rgba(83,74,183,0.45)' : 'none'
  return (
    <div style={{
      width: 130, height: 44, background: '#f1f0ff', border: '2px solid #534AB7',
      borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center',
      gap: 8, fontFamily: 'Inter, sans-serif', boxShadow: glow,
    }}>
      <Handle type="target" position={Position.Left} style={{ background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#534AB7" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
        <line x1="6" y1="3" x2="6" y2="15" /><path d="M21 3H3" /><path d="M12 3v18" /><path d="M3 21l4-4" /><path d="M21 21l-4-4" />
      </svg>
      <span style={{ fontSize: 11, fontWeight: 700, color: '#534AB7' }}>PARALLEL</span>
      <Handle type="source" id="s1" position={Position.Right} style={{ top: '25%', background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
      <Handle type="source" id="s2" position={Position.Right} style={{ top: '50%', background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
      <Handle type="source" id="s3" position={Position.Right} style={{ top: '75%', background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
    </div>
  )
}

function JoinNode({ data }: NodeProps) {
  const nodeData = data as NodeData
  const glow = nodeData.status === 'running' ? '0 0 0 3px rgba(83,74,183,0.45)' : 'none'
  return (
    <div style={{
      width: 130, height: 44, background: '#f1f0ff', border: '2px solid #534AB7',
      borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center',
      gap: 8, fontFamily: 'Inter, sans-serif', boxShadow: glow,
    }}>
      <Handle type="target" id="t1" position={Position.Left} style={{ top: '25%', background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
      <Handle type="target" id="t2" position={Position.Left} style={{ top: '50%', background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
      <Handle type="target" id="t3" position={Position.Left} style={{ top: '75%', background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#534AB7" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M21 12H3" /><path d="M3 3l4 4" /><path d="M3 21l4-4" /><line x1="18" y1="3" x2="18" y2="21" />
      </svg>
      <span style={{ fontSize: 11, fontWeight: 700, color: '#534AB7' }}>JOIN</span>
      <Handle type="source" position={Position.Right} style={{ background: '#534AB7', width: 10, height: 10, border: '2px solid #fff' }} />
    </div>
  )
}

function LoopNode({ data }: NodeProps) {
  const nodeData = data as NodeData
  const expr = (nodeData.config?.exit_condition as string) || 'exit condition'
  const glow = nodeData.status === 'running' ? '0 0 0 3px rgba(83,74,183,0.45)' : 'none'
  return (
    <div style={{
      minWidth: 150, background: '#fff', border: '2px solid #8b5cf6',
      borderRadius: 10, padding: '10px 14px', fontFamily: 'Inter, sans-serif',
      boxShadow: glow, position: 'relative',
    }}>
      <Handle type="target" position={Position.Top} style={{ left: '50%', background: '#8b5cf6', width: 10, height: 10, border: '2px solid #fff' }} />
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#8b5cf6" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 12a9 9 0 11-9-9c2.52 0 4.93 1 6.74 2.74L21 8" /><path d="M21 3v5h-5" />
        </svg>
        <span style={{ fontSize: 11, fontWeight: 700, color: '#8b5cf6' }}>LOOP</span>
      </div>
      <div style={{ fontSize: 9, color: '#a78bfa', maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{expr}</div>
      <Handle type="source" id="continue" position={Position.Left} style={{ top: '50%', background: '#8b5cf6', width: 10, height: 10, border: '2px solid #fff' }} />
      <Handle type="source" id="exit" position={Position.Right} style={{ top: '50%', background: '#22c55e', width: 10, height: 10, border: '2px solid #fff' }} />
      <div style={{ position: 'absolute', left: -28, top: '44%', fontSize: 9, color: '#8b5cf6' }}>↩ cont</div>
      <div style={{ position: 'absolute', right: -28, top: '44%', fontSize: 9, color: '#22c55e' }}>exit →</div>
    </div>
  )
}

function SupervisorNode({ data }: NodeProps) {
  const nodeData = data as NodeData
  const borderColor = nodeData.status === 'done' ? '#22c55e' : nodeData.status === 'error' ? '#ef4444' : '#d97706'
  const glow = nodeData.status === 'running' ? '0 0 0 3px rgba(217,119,6,0.45)' : 'none'
  return (
    <div style={{
      minWidth: 160, background: '#fffbeb', border: `2px solid ${borderColor}`,
      borderRadius: 10, padding: '10px 14px', fontFamily: 'Inter, sans-serif',
      boxShadow: glow, position: 'relative',
    }}>
      {nodeData.status === 'done' && (
        <div style={{ position: 'absolute', top: -7, right: -7, background: '#22c55e', borderRadius: '50%', width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, color: '#fff' }}>✓</div>
      )}
      {nodeData.status === 'running' && (
        <div style={{ position: 'absolute', top: -7, right: -7, background: '#d97706', borderRadius: '50%', width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <div style={{ width: 8, height: 8, border: '1.5px solid #fff', borderTopColor: 'transparent', borderRadius: '50%', animation: 'spin 0.7s linear infinite' }} />
        </div>
      )}
      <Handle type="target" position={Position.Left} style={{ background: '#d97706', width: 10, height: 10, border: '2px solid #fff' }} />
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
        <span style={{ fontSize: 14, lineHeight: 1 }}>👑</span>
        <div style={{ fontSize: 12, fontWeight: 600, color: '#92400e' }}>{nodeData.label}</div>
      </div>
      {nodeData.agent_model && (
        <div style={{ display: 'flex', gap: 4 }}>
          <span style={{ fontSize: 10, background: '#fef3c7', color: '#d97706', borderRadius: 4, padding: '1px 6px' }}>{nodeData.agent_model}</span>
          {nodeData.agent_provider && <span style={{ fontSize: 10, background: '#f4f4f5', color: '#71717a', borderRadius: 4, padding: '1px 6px' }}>{nodeData.agent_provider}</span>}
        </div>
      )}
      {/* Forward handle — connects to next pipeline node */}
      <Handle type="source" id="forward" position={Position.Right} style={{ background: '#d97706', width: 10, height: 10, border: '2px solid #fff' }} />
      {/* Delegate handle — drag from here to team agent nodes; auto-labeled "delegate" */}
      <Handle type="source" id="delegate" position={Position.Bottom} style={{ background: '#92400e', width: 10, height: 10, border: '2px solid #fff' }} />
      <div style={{ position: 'absolute', bottom: -18, left: '50%', transform: 'translateX(-50%)', fontSize: 9, color: '#92400e', fontWeight: 700, whiteSpace: 'nowrap' }}>delegate ↓</div>
    </div>
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
}

// ---------------------------------------------------------------------------
// Palette items
// ---------------------------------------------------------------------------
const PALETTE_ITEMS: { type: WorkflowNodeType; label: string; icon: React.ReactNode; color: string }[] = [
  { type: 'start',      label: 'Start',      icon: <div style={{ width: 20, height: 20, borderRadius: '50%', background: '#22c55e' }} />, color: '#22c55e' },
  { type: 'end',        label: 'End',        icon: <div style={{ width: 20, height: 20, borderRadius: '50%', background: '#ef4444' }} />, color: '#ef4444' },
  { type: 'agent',      label: 'Agent',      icon: <div style={{ width: 20, height: 14, border: '2px solid #534AB7', borderRadius: 4 }} />, color: '#534AB7' },
  { type: 'supervisor', label: 'Supervisor', icon: <span style={{ fontSize: 16, lineHeight: 1 }}>👑</span>, color: '#d97706' },
  { type: 'condition',  label: 'Condition',  icon: <div style={{ width: 16, height: 16, border: '2px solid #f59e0b', transform: 'rotate(45deg)' }} />, color: '#f59e0b' },
  { type: 'parallel',   label: 'Parallel',   icon: <GitBranch size={16} color="#534AB7" />, color: '#534AB7' },
  { type: 'join',       label: 'Join',       icon: <Merge size={16} color="#534AB7" />, color: '#534AB7' },
  { type: 'loop',       label: 'Loop',       icon: <RefreshCcw size={16} color="#8b5cf6" />, color: '#8b5cf6' },
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

function modelToProvider(model?: string): string {
  if (!model) return 'anthropic'
  if (model.startsWith('claude')) return 'anthropic'
  if (model.startsWith('gpt') || model.startsWith('o1') || model.startsWith('o3')) return 'openai'
  if (model.startsWith('gemini')) return 'gemini'
  return 'anthropic'
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

  async function handleCreateAndLoad() {
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
          provider: modelToProvider(agentDef.model),
          model: agentDef.model ?? 'claude-sonnet-4-6',
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

  const categoryColors: Record<string, { bg: string; text: string }> = {
    Marketing:   { bg: '#fce7f3', text: '#9d174d' },
    Support:     { bg: '#dbeafe', text: '#1e40af' },
    Research:    { bg: '#d1fae5', text: '#065f46' },
    Engineering: { bg: '#ede9fe', text: '#5b21b6' },
    Supervisor:  { bg: '#fef3c7', text: '#92400e' },
  }

  return (
    <div style={{
      position: 'fixed', inset: 0, zIndex: 200,
      background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center',
    }}
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
    >
      <div style={{
        background: '#fff', borderRadius: 16, width: '95vw', maxWidth: 900, height: '85vh',
        display: 'flex', flexDirection: 'column', overflow: 'hidden',
        boxShadow: '0 20px 60px rgba(0,0,0,0.25)',
      }}>
        {/* Header */}
        <div style={{
          padding: '16px 20px', borderBottom: '1px solid #e5e7eb',
          display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexShrink: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <BookOpen size={16} color="#534AB7" />
            <span style={{ fontSize: 14, fontWeight: 700, color: '#111827' }}>Workflow Templates</span>
            <span style={{ fontSize: 11, color: '#9ca3af' }}>— select a template to get started</span>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#9ca3af' }}>
            <X size={16} />
          </button>
        </div>

        {/* Body: card grid + detail panel */}
        <div style={{ flex: 1, display: 'flex', overflow: 'hidden', flexWrap: 'wrap' }}>
          {/* Left: template cards */}
          <div style={{
            width: 220, minWidth: 180, flexShrink: 0, borderRight: '1px solid #e5e7eb',
            padding: '12px', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 8,
          }}>
            {TEMPLATES.map((tpl) => {
              const cat = categoryColors[tpl.category] ?? { bg: '#f3f4f6', text: '#374151' }
              const isActive = selected.id === tpl.id
              return (
                <button
                  key={tpl.id}
                  onClick={() => { setSelected(tpl); setGuideTab('overview') }}
                  style={{
                    padding: '12px', borderRadius: 10, textAlign: 'left', cursor: 'pointer',
                    border: `2px solid ${isActive ? '#534AB7' : '#e5e7eb'}`,
                    background: isActive ? '#f1f0ff' : '#fff',
                    transition: 'border-color 0.15s',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
                    <span style={{ fontSize: 12, fontWeight: 700, color: isActive ? '#534AB7' : '#111827' }}>{tpl.name}</span>
                    <span style={{
                      fontSize: 9, padding: '2px 6px', borderRadius: 4, fontWeight: 700,
                      background: cat.bg, color: cat.text,
                    }}>{tpl.category}</span>
                  </div>
                  <p style={{ fontSize: 11, color: '#6b7280', margin: 0, lineHeight: 1.4 }}>{tpl.description}</p>
                  <p style={{ fontSize: 10, color: '#9ca3af', marginTop: 6 }}>
                    {tpl.nodes.filter((n) => n.type === 'agent').length} agents · {tpl.nodes.length} nodes
                  </p>
                </button>
              )
            })}
          </div>

          {/* Right: guide detail */}
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
            {/* Tab bar */}
            <div style={{
              display: 'flex', gap: 0, borderBottom: '1px solid #e5e7eb', padding: '0 20px', flexShrink: 0,
            }}>
              {(['overview', 'agents', 'steps'] as const).map((tab) => (
                <button
                  key={tab}
                  onClick={() => setGuideTab(tab)}
                  style={{
                    padding: '12px 16px', fontSize: 12, fontWeight: guideTab === tab ? 700 : 500,
                    color: guideTab === tab ? '#534AB7' : '#6b7280',
                    borderBottom: `2px solid ${guideTab === tab ? '#534AB7' : 'transparent'}`,
                    background: 'none', border: 'none', borderRadius: 0, cursor: 'pointer',
                    textTransform: 'capitalize',
                  }}
                >
                  {tab === 'overview' ? 'Overview' : tab === 'agents' ? `Agents (${selected.guide.agents.length})` : 'Setup Guide'}
                </button>
              ))}
            </div>

            {/* Tab content */}
            <div style={{ flex: 1, overflowY: 'auto', padding: '20px' }}>
              {guideTab === 'overview' && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <p style={{ fontSize: 13, color: '#374151', lineHeight: 1.6, margin: 0 }}>{selected.guide.overview}</p>
                  <div>
                    <div style={{ fontSize: 11, fontWeight: 700, color: '#9ca3af', letterSpacing: 0.6, textTransform: 'uppercase', marginBottom: 8 }}>Workflow nodes</div>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                      {selected.nodes.map((n) => {
                        const color = n.type === 'start' ? '#22c55e' : n.type === 'end' ? '#ef4444' : n.type === 'condition' ? '#f59e0b' : n.type === 'loop' ? '#8b5cf6' : n.type === 'supervisor' ? '#d97706' : '#534AB7'
                        return (
                          <span key={n.key} style={{
                            fontSize: 11, padding: '3px 8px', borderRadius: 6,
                            background: `${color}15`, color, border: `1px solid ${color}30`, fontWeight: 500,
                          }}>
                            {n.label}
                          </span>
                        )
                      })}
                    </div>
                  </div>
                </div>
              )}

              {guideTab === 'agents' && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  {selected.guide.agents.map((agent, i) => (
                    <div key={i} style={{ border: '1px solid #e5e7eb', borderRadius: 10, overflow: 'hidden' }}>
                      <div style={{ padding: '10px 14px', background: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                          <span style={{ fontSize: 13, fontWeight: 700, color: '#111827' }}>{agent.name}</span>
                          {agent.model && (
                            <span style={{ fontSize: 10, padding: '2px 6px', background: '#ede9fe', color: '#534AB7', borderRadius: 4, fontWeight: 600 }}>{agent.model}</span>
                          )}
                        </div>
                        <p style={{ fontSize: 11, color: '#6b7280', margin: '4px 0 0' }}>{agent.role}</p>
                      </div>
                      <div style={{ padding: '10px 14px' }}>
                        <div style={{ fontSize: 10, fontWeight: 700, color: '#9ca3af', letterSpacing: 0.5, textTransform: 'uppercase', marginBottom: 6 }}>System Prompt</div>
                        <pre style={{
                          fontSize: 11, color: '#374151', background: '#f9fafb', borderRadius: 6,
                          padding: '10px 12px', margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                          border: '1px solid #e5e7eb', lineHeight: 1.5, fontFamily: 'monospace',
                          maxHeight: 200, overflowY: 'auto',
                        }}>{agent.systemPrompt}</pre>
                        {agent.tools && agent.tools.length > 0 && (
                          <div style={{ marginTop: 8, display: 'flex', alignItems: 'center', gap: 6 }}>
                            <span style={{ fontSize: 10, color: '#9ca3af', fontWeight: 600 }}>TOOLS:</span>
                            {agent.tools.map((t) => (
                              <span key={t} style={{ fontSize: 10, padding: '2px 6px', background: '#fef3c7', color: '#92400e', borderRadius: 4, fontWeight: 600 }}>{t}</span>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {guideTab === 'steps' && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                  <p style={{ fontSize: 12, color: '#6b7280', margin: '0 0 8px', lineHeight: 1.5 }}>
                    Follow these steps to set up this workflow in Agent Nexus:
                  </p>
                  {selected.guide.steps.map((step, i) => (
                    <div key={i} style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
                      <div style={{
                        width: 24, height: 24, borderRadius: '50%', background: '#f1f0ff',
                        color: '#534AB7', fontSize: 11, fontWeight: 700,
                        display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
                      }}>
                        {i + 1}
                      </div>
                      <p style={{ fontSize: 12, color: '#374151', margin: 0, lineHeight: 1.6 }}>{step}</p>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Action bar */}
            <div style={{ padding: '12px 20px', borderTop: '1px solid #e5e7eb', flexShrink: 0 }}>
              {createError && (
                <div style={{ marginBottom: 10, padding: '8px 12px', background: '#fef2f2', border: '1px solid #fecaca', borderRadius: 8, fontSize: 11, color: '#dc2626' }}>
                  {createError}
                </div>
              )}
              {createProgress.length > 0 && (
                <div style={{ marginBottom: 10, padding: '8px 12px', background: '#f1f0ff', border: '1px solid #c4b5fd', borderRadius: 8, display: 'flex', flexDirection: 'column', gap: 2 }}>
                  {createProgress.map((line, i) => (
                    <span key={i} style={{ fontSize: 11, color: '#534AB7' }}>{line}</span>
                  ))}
                </div>
              )}
              <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
                <button
                  onClick={() => onSelect(selected)}
                  disabled={creating}
                  style={{
                    padding: '9px 16px', background: '#fff', color: '#534AB7',
                    border: '1px solid #c4b5fd', borderRadius: 8, fontSize: 12, fontWeight: 600,
                    cursor: creating ? 'not-allowed' : 'pointer', opacity: creating ? 0.5 : 1,
                    display: 'flex', alignItems: 'center', gap: 6,
                  }}
                >
                  <LayoutTemplate size={13} /> Load template only
                </button>
                <button
                  onClick={handleCreateAndLoad}
                  disabled={creating}
                  style={{
                    padding: '9px 20px', background: creating ? '#a5b4fc' : '#534AB7', color: '#fff',
                    border: 'none', borderRadius: 8, fontSize: 13, fontWeight: 700,
                    cursor: creating ? 'not-allowed' : 'pointer',
                    display: 'flex', alignItems: 'center', gap: 6,
                  }}
                >
                  {creating
                    ? <><Loader2 size={13} style={{ animation: 'spin 0.7s linear infinite' }} /> Creating…</>
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

  const [nodes, setNodes, onNodesChange] = useNodesState<any>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
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

      const color = edgeColor(label)
      const isDelegate = label === 'delegate'
      setEdges((eds) =>
        addEdge({
          ...connection,
          type: 'smoothstep',
          animated: true,
          zIndex: 1000,
          label: label ?? undefined,
          markerEnd: { type: MarkerType.ArrowClosed, color, width: 18, height: 18 },
          style: { stroke: color, strokeWidth: isDelegate ? 2 : 2.5, strokeDasharray: isDelegate ? '6 3' : undefined },
          labelStyle: { fontSize: 10, fill: color, fontWeight: 700 },
          labelBgStyle: { fill: '#fff', fillOpacity: 0.9 },
        }, eds)
      )
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
    setEdges(tpl.edges.map((e) => {
      const color = edgeColor(e.label)
      const isDelegate = e.label === 'delegate'
      return {
        id: `e-${getId(e.from)}-${getId(e.to)}`,
        source: getId(e.from), target: getId(e.to),
        type: 'smoothstep', animated: true,
        zIndex: 1000,
        label: e.label ?? undefined,
        sourceHandle: e.label === 'yes' ? 'yes' : e.label === 'no' ? 'no' : e.label === 'loop' ? 'continue' : e.label === 'exit' ? 'exit' : e.label === 'delegate' ? 'delegate' : undefined,
        markerEnd: { type: MarkerType.ArrowClosed, color, width: 18, height: 18 },
        style: { stroke: color, strokeWidth: isDelegate ? 2 : 2.5, strokeDasharray: isDelegate ? '6 3' : undefined },
        labelStyle: { fontSize: 10, fill: color, fontWeight: 700 },
        labelBgStyle: { fill: '#fff', fillOpacity: 0.9 },
      }
    }))
    if (createdAgents) {
      queryClient.invalidateQueries({ queryKey: ['agents'] })
    }
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
    setEdges((eds) => eds.map((e) => {
      if (e.id !== edgeId) return e
      const color = edgeColor(newLabel || undefined)
      const isDelegate = newLabel === 'delegate'
      return {
        ...e,
        label: newLabel || undefined,
        style: { stroke: color, strokeWidth: isDelegate ? 2 : 2.5, strokeDasharray: isDelegate ? '6 3' : undefined },
        markerEnd: { type: MarkerType.ArrowClosed, color, width: 18, height: 18 },
        labelStyle: { fontSize: 10, fill: color, fontWeight: 700 },
      }
    }))
    setSelectedEdge((prev) => prev?.id === edgeId ? { ...prev, label: newLabel || undefined } : prev)
  }, [setEdges])

  const deleteEdge = useCallback((edgeId: string) => {
    setEdges((eds) => eds.filter((e) => e.id !== edgeId))
    setSelectedEdge(null)
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
      // Node ids are stable across saves now (the server upserts by id
      // instead of reminting), so the canvas doesn't need to discard local
      // state (selection, panel) and re-sync from a refetch.
      setSaveWarnings(res.warnings ?? [])
      queryClient.invalidateQueries({ queryKey: ['workflow-graph', groupId] })
      setSaveStatus('saved')
      setTimeout(() => setSaveStatus('idle'), 2000)
    } catch {
      setSaveStatus('error')
      setTimeout(() => setSaveStatus('idle'), 3000)
    }
  }

  // Run via SSE
  const handleRun = async () => {
    if (!runInput.trim()) return
    setRunStatus('running')
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

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() ?? ''
        for (const line of lines) {
          if (!line.startsWith('data:')) continue
          const raw = line.slice(5).trim()
          if (!raw || raw === '[DONE]') continue
          try {
            const evt = JSON.parse(raw) as SSEEvent
            if (evt.type === 'node_started' && evt.node_id) {
              setNodes((nds) => nds.map((n) => n.id === evt.node_id ? { ...n, data: { ...n.data, status: 'running' as const } } : n))
            } else if (evt.type === 'node_completed' && evt.node_id) {
              setNodes((nds) => nds.map((n) => n.id === evt.node_id ? { ...n, data: { ...n.data, status: 'done' as const } } : n))
              if (evt.result) {
                setRunOutput((prev) => ({ ...prev, [evt.node_id!]: (prev[evt.node_id!] ?? '') + evt.result }))
              }
            } else if (evt.type === 'delta' && evt.node_id) {
              setRunOutput((prev) => ({ ...prev, [evt.node_id!]: (prev[evt.node_id!] ?? '') + (evt.content ?? '') }))
            } else if (evt.type === 'delta' && evt.content) {
              setRunOutput((prev) => ({ ...prev, __main__: (prev.__main__ ?? '') + evt.content }))
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
      setRunStatus('done')
    } catch (err) {
      setRunError(err instanceof Error ? err.message : 'Network error — could not reach the server')
      setRunStatus('done')
    }
  }

  const hasNodes = nodes.length > 0
  const hasRightPanel = !!(selectedNode || selectedEdge) || triggersPanelOpen
  const canvasWidth = hasRightPanel ? 'calc(100% - 460px)' : '100%'

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden', fontFamily: 'Inter, sans-serif', background: '#f8f8fb' }}>
      {/* Spin animation */}
      <style>{`
        @keyframes spin { to { transform: rotate(360deg); } }
        .rf-node-running div { animation: spin 0.7s linear infinite; }
      `}</style>

      {/* Top bar */}
      <div style={{
        minHeight: 48, borderBottom: '1px solid #e5e7eb', background: '#fff',
        display: 'flex', alignItems: 'center', gap: 8, padding: '0 12px',
        flexShrink: 0, overflowX: 'auto',
      }}>
        <button
          onClick={() => router.push('/workflows')}
          style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#6b7280', fontSize: 13, background: 'none', border: 'none', cursor: 'pointer' }}
        >
          <ArrowLeft size={14} /> Back
        </button>
        <div style={{ width: 1, height: 20, background: '#e5e7eb' }} />
        <span style={{ fontSize: 14, fontWeight: 600, color: '#111827' }}>{groupData?.name ?? 'Workflow'}</span>
        {groupData?.mode && (
          <span style={{
            fontSize: 10, fontWeight: 700, letterSpacing: 0.5,
            padding: '2px 8px', borderRadius: 10,
            background: groupData.mode === 'pipeline' ? '#f1f0ff' : '#fef3c7',
            color: groupData.mode === 'pipeline' ? '#534AB7' : '#92400e',
            textTransform: 'uppercase',
          }}>
            {groupData.mode}
          </span>
        )}
        <div style={{ flex: 1 }} />
        <button
          onClick={() => setTemplateGalleryOpen(true)}
          title="Browse workflow templates"
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '6px 14px', borderRadius: 8, fontSize: 12, fontWeight: 600, cursor: 'pointer',
            background: '#fff', color: '#534AB7', border: '1px solid #c4b5fd',
          }}
        >
          <LayoutTemplate size={13} /> Templates
        </button>
        <button
          onClick={() => { setTriggersPanelOpen(v => !v); setSelectedNode(null); setSelectedEdge(null) }}
          title="Webhook triggers for this workflow"
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '6px 14px', borderRadius: 8, fontSize: 12, fontWeight: 600, cursor: 'pointer',
            background: triggersPanelOpen ? '#f1f0ff' : '#fff',
            color: triggersPanelOpen ? '#534AB7' : '#374151',
            border: `1px solid ${triggersPanelOpen ? '#c4b5fd' : '#e5e7eb'}`,
          }}
        >
          <Zap size={13} /> Triggers
        </button>
        <button
          onClick={handleSave}
          disabled={saveStatus === 'saving'}
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '6px 14px', borderRadius: 8, fontSize: 12, fontWeight: 600, cursor: 'pointer',
            background: saveStatus === 'saved' ? '#22c55e' : saveStatus === 'error' ? '#ef4444' : '#fff',
            color: saveStatus === 'saved' || saveStatus === 'error' ? '#fff' : '#374151',
            border: '1px solid #e5e7eb',
          }}
        >
          {saveStatus === 'saving' ? <Loader2 size={13} style={{ animation: 'spin 0.7s linear infinite' }} /> : <Save size={13} />}
          {saveStatus === 'saving' ? 'Saving…' : saveStatus === 'saved' ? 'Saved' : saveStatus === 'error' ? 'Error' : 'Save'}
        </button>
        <button
          onClick={() => setRunPanelOpen(true)}
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '6px 14px', borderRadius: 8, fontSize: 12, fontWeight: 600, cursor: 'pointer',
            background: '#534AB7', color: '#fff', border: 'none',
          }}
        >
          <Play size={13} /> Run
        </button>
      </div>

      {/* Save validation warnings */}
      {saveWarnings.length > 0 && (
        <div style={{
          flexShrink: 0, background: '#fffbeb', borderBottom: '1px solid #fde68a',
          padding: '8px 16px', display: 'flex', alignItems: 'flex-start', gap: 8,
          flexWrap: 'wrap',
        }}>
          <span style={{ fontSize: 12, fontWeight: 700, color: '#92400e', flexShrink: 0 }}>Save warnings:</span>
          <ul style={{ margin: 0, padding: 0, listStyle: 'none', flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 2 }}>
            {saveWarnings.map((warning, i) => (
              <li key={i} style={{ fontSize: 12, color: '#92400e' }}>{warning}</li>
            ))}
          </ul>
          <button
            onClick={() => setSaveWarnings([])}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#92400e', fontSize: 12, fontWeight: 600, flexShrink: 0 }}
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Body */}
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        {/* Palette */}
        <div style={{
          width: 180, flexShrink: 0, background: '#fff', borderRight: '1px solid #e5e7eb',
          padding: '16px 12px', display: 'flex', flexDirection: 'column', gap: 4,
          overflowY: 'auto',
        }}>
          <div style={{ fontSize: 10, fontWeight: 700, color: '#9ca3af', letterSpacing: 0.8, marginBottom: 8, textTransform: 'uppercase' }}>Nodes</div>
          {PALETTE_ITEMS.map((item) => (
            <div
              key={item.type}
              draggable
              onDragStart={(e) => {
                e.dataTransfer.setData('application/reactflow', item.type)
                e.dataTransfer.effectAllowed = 'move'
              }}
              style={{
                display: 'flex', alignItems: 'center', gap: 10,
                padding: '8px 10px', borderRadius: 8, cursor: 'grab',
                border: `1px solid ${item.color}22`, background: `${item.color}08`,
                userSelect: 'none',
              }}
              onMouseEnter={(e) => { (e.currentTarget as HTMLDivElement).style.background = `${item.color}18` }}
              onMouseLeave={(e) => { (e.currentTarget as HTMLDivElement).style.background = `${item.color}08` }}
            >
              <div style={{ width: 24, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>{item.icon}</div>
              <span style={{ fontSize: 12, fontWeight: 500, color: '#374151' }}>{item.label}</span>
            </div>
          ))}
          <div style={{ marginTop: 16, padding: '10px', background: '#f9fafb', borderRadius: 8, fontSize: 10, color: '#9ca3af', lineHeight: 1.5 }}>
            Drag a node onto the canvas to add it to your workflow.
          </div>
        </div>

        {/* Canvas */}
        <div
          ref={reactFlowWrapper}
          style={{ flex: 1, height: '100%', position: 'relative' }}
          onDragOver={onDragOver}
          onDrop={onDrop}
        >
          {graphLoading && (
            <div style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 10, background: 'rgba(255,255,255,0.8)' }}>
              <Loader2 size={24} style={{ animation: 'spin 0.7s linear infinite', color: '#534AB7' }} />
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
            style={{ background: '#f8f8fb' }}
            deleteKeyCode="Backspace"
          >
            <Background color="#e5e7eb" gap={20} />
            <Controls style={{ bottom: 20, left: 20 }} />
            <MiniMap
              nodeColor={(n) => {
                const t = (n as Node<NodeData>).data?.node_type
                if (t === 'start') return '#22c55e'
                if (t === 'end') return '#ef4444'
                if (t === 'condition') return '#f59e0b'
                if (t === 'loop') return '#8b5cf6'
                return '#534AB7'
              }}
              style={{ bottom: 20, right: 20, border: '1px solid #e5e7eb', borderRadius: 8 }}
            />
            {!hasNodes && (
              <div style={{
                position: 'absolute', inset: 0, display: 'flex',
                alignItems: 'center', justifyContent: 'center',
                pointerEvents: 'none', zIndex: 5,
              }}>
                <div style={{ textAlign: 'center', color: '#9ca3af' }}>
                  <GitBranch size={36} style={{ margin: '0 auto 12px', opacity: 0.3 }} />
                  <p style={{ fontSize: 13, fontWeight: 500 }}>Drag nodes from the left panel to start building your workflow</p>
                  <p style={{ fontSize: 11, marginTop: 4, opacity: 0.7 }}>Connect nodes by dragging from a handle to another</p>
                </div>
              </div>
            )}
          </ReactFlow>
        </div>

        {/* Edge config panel */}
        {selectedEdge && !selectedNode && (
          <div style={{
            width: 280, flexShrink: 0, background: '#fff', borderLeft: '1px solid #e5e7eb',
            padding: '16px', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 14,
          }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span style={{ fontSize: 12, fontWeight: 700, color: '#374151', textTransform: 'uppercase', letterSpacing: 0.5 }}>
                Edge Config
              </span>
              <button onClick={() => setSelectedEdge(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#9ca3af' }}>
                <X size={14} />
              </button>
            </div>

            <div>
              <div style={{ fontSize: 10, fontWeight: 600, color: '#6b7280', marginBottom: 4, textTransform: 'uppercase', letterSpacing: 0.4 }}>Label</div>
              <input
                value={(selectedEdge.label as string) || ''}
                onChange={(e) => updateEdgeLabel(selectedEdge.id, e.target.value)}
                style={{ width: '100%', padding: '6px 8px', fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box' }}
                placeholder="e.g. yes, no, loop, exit"
              />
              <div style={{ marginTop: 6, display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                {['yes', 'no', 'loop', 'exit', 'delegate'].map((preset) => (
                  <button
                    key={preset}
                    onClick={() => updateEdgeLabel(selectedEdge.id, preset)}
                    style={{
                      padding: '2px 8px', fontSize: 10, borderRadius: 4, cursor: 'pointer', border: 'none',
                      background: selectedEdge.label === preset ? edgeColor(preset) : '#f3f4f6',
                      color: selectedEdge.label === preset ? '#fff' : '#374151',
                      fontWeight: 600,
                    }}
                  >
                    {preset}
                  </button>
                ))}
                <button
                  onClick={() => updateEdgeLabel(selectedEdge.id, '')}
                  style={{ padding: '2px 8px', fontSize: 10, borderRadius: 4, cursor: 'pointer', border: 'none', background: '#f3f4f6', color: '#9ca3af' }}
                >
                  clear
                </button>
              </div>
            </div>

            <div style={{ fontSize: 11, color: '#9ca3af', background: '#f9fafb', borderRadius: 8, padding: '8px 10px' }}>
              <strong>yes / no</strong> — condition routing<br />
              <strong>loop</strong> — loop back edge<br />
              <strong>exit</strong> — loop exit edge<br />
              <strong>delegate</strong> — supervisor → team agent (dashed)<br />
              <strong>(empty)</strong> — default / unconditional
            </div>

            <button
              onClick={() => deleteEdge(selectedEdge.id)}
              style={{
                marginTop: 'auto', padding: '7px 12px', fontSize: 12, borderRadius: 8,
                background: '#fff', color: '#ef4444', border: '1px solid #fecaca',
                cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6,
              }}
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
        {selectedNode && (
          <div style={{
            width: 280, flexShrink: 0, background: '#fff', borderLeft: '1px solid #e5e7eb',
            padding: '16px', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 14,
          }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span style={{ fontSize: 12, fontWeight: 700, color: '#374151', textTransform: 'uppercase', letterSpacing: 0.5 }}>
                Node Config
              </span>
              <button onClick={() => setSelectedNode(null)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#9ca3af' }}>
                <X size={14} />
              </button>
            </div>

            <div>
              <div style={{ fontSize: 10, color: '#9ca3af', marginBottom: 4, fontWeight: 600 }}>TYPE</div>
              <span style={{
                fontSize: 11, padding: '2px 8px', borderRadius: 6,
                background: selectedNode.data.node_type === 'supervisor' ? '#fef3c7' : '#f1f0ff',
                color: selectedNode.data.node_type === 'supervisor' ? '#92400e' : '#534AB7',
                fontWeight: 700,
              }}>
                {selectedNode.data.node_type === 'supervisor' ? '👑 supervisor' : selectedNode.data.node_type as string}
              </span>
            </div>

            {/* Label */}
            <ConfigField label="Label">
              <input
                value={(selectedNode.data.config?.label as string) || selectedNode.data.label || ''}
                onChange={(e) => updateNodeConfig(selectedNode.id, {
                  label: e.target.value,
                  config: { ...selectedNode.data.config, label: e.target.value },
                })}
                style={{ width: '100%', padding: '6px 8px', fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box' }}
                placeholder="Node label"
              />
            </ConfigField>

            {/* Agent / Supervisor node: agent picker */}
            {(selectedNode.data.node_type === 'agent' || selectedNode.data.node_type === 'supervisor') && (
              <ConfigField label={selectedNode.data.node_type === 'supervisor' ? 'Supervisor Agent' : 'Agent'}>
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
                  style={{ width: '100%', padding: '6px 8px', fontSize: 12, border: `1px solid ${selectedNode.data.node_type === 'supervisor' ? '#fde68a' : '#e5e7eb'}`, borderRadius: 6, background: '#fff', outline: 'none', boxSizing: 'border-box' }}
                >
                  <option value="">Select agent…</option>
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>{a.name}</option>
                  ))}
                </select>
                {selectedNode.data.agent_model && (
                  <div style={{ marginTop: 4, display: 'flex', gap: 4 }}>
                    <span style={{ fontSize: 10, background: selectedNode.data.node_type === 'supervisor' ? '#fef3c7' : '#f1f0ff', color: selectedNode.data.node_type === 'supervisor' ? '#d97706' : '#534AB7', borderRadius: 4, padding: '1px 6px' }}>{selectedNode.data.agent_model as string}</span>
                    <span style={{ fontSize: 10, background: '#f4f4f5', color: '#71717a', borderRadius: 4, padding: '1px 6px' }}>{selectedNode.data.agent_provider as string}</span>
                  </div>
                )}
                {selectedNode.data.node_type === 'supervisor' && (
                  <div style={{ fontSize: 10, color: '#d97706', marginTop: 6, background: '#fffbeb', border: '1px solid #fde68a', borderRadius: 6, padding: '6px 8px', lineHeight: 1.5 }}>
                    This agent runs in an agentic loop, calling team agents (connected via dashed delegate edges) as tools. Use a capable model (e.g. claude-sonnet or opus).
                  </div>
                )}
              </ConfigField>
            )}

            {/* Condition node */}
            {selectedNode.data.node_type === 'condition' && (
              <ConfigField label="Expression">
                <input
                  value={(selectedNode.data.config?.expression as string) || ''}
                  onChange={(e) => updateNodeConfig(selectedNode.id, {
                    config: { ...selectedNode.data.config, expression: e.target.value },
                  })}
                  style={{ width: '100%', padding: '6px 8px', fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box' }}
                  placeholder="e.g. contains:approved"
                />
                <div style={{ fontSize: 10, color: '#9ca3af', marginTop: 4 }}>
                  e.g. contains:approved, not_contains:error, equals:done
                </div>
              </ConfigField>
            )}

            {/* Loop node */}
            {selectedNode.data.node_type === 'loop' && (
              <>
                <ConfigField label="Exit Condition">
                  <input
                    value={(selectedNode.data.config?.exit_condition as string) || ''}
                    onChange={(e) => updateNodeConfig(selectedNode.id, {
                      config: { ...selectedNode.data.config, exit_condition: e.target.value },
                    })}
                    style={{ width: '100%', padding: '6px 8px', fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box' }}
                    placeholder="e.g. contains:done"
                  />
                  <div style={{ fontSize: 10, color: '#9ca3af', marginTop: 4 }}>
                    Loops while condition is NOT met. Same syntax as condition node.
                  </div>
                </ConfigField>
                <ConfigField label="Max Iterations">
                  <input
                    type="number"
                    min={1}
                    max={100}
                    value={(selectedNode.data.config?.max_iterations as number) || 5}
                    onChange={(e) => updateNodeConfig(selectedNode.id, {
                      config: { ...selectedNode.data.config, max_iterations: parseInt(e.target.value, 10) || 5 },
                    })}
                    style={{ width: '100%', padding: '6px 8px', fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box' }}
                  />
                </ConfigField>
                <div style={{ fontSize: 11, color: '#9ca3af', background: '#f9fafb', borderRadius: 8, padding: '8px 10px' }}>
                  Draw an edge from the <strong>↩ cont</strong> handle to the node you want to loop back to.
                  It will be auto-labeled <code style={{ background: '#ede9fe', padding: '1px 4px', borderRadius: 3 }}>loop</code>.
                  Draw from <strong>exit →</strong> for the forward path.
                </div>
              </>
            )}

            {/* Start / Parallel / Join / Supervisor: info */}
            {(selectedNode.data.node_type === 'start' || selectedNode.data.node_type === 'parallel' || selectedNode.data.node_type === 'join') && (
              <div style={{ fontSize: 11, color: '#9ca3af', background: '#f9fafb', borderRadius: 8, padding: '8px 10px', lineHeight: 1.6 }}>
                {selectedNode.data.node_type === 'start' && 'Entry point of the workflow. Every run begins here. Draw one edge from the right handle to the first node.'}
                {selectedNode.data.node_type === 'parallel' && 'Fan-out: connects to multiple downstream nodes that run concurrently. Use a Join node to collect their outputs when done.'}
                {selectedNode.data.node_type === 'join' && 'Merges outputs from parallel branches into a single concatenated output before continuing.'}
              </div>
            )}

            {selectedNode.data.node_type === 'supervisor' && !selectedNode.data.agent_id && (
              <div style={{ fontSize: 11, color: '#92400e', background: '#fffbeb', border: '1px solid #fde68a', borderRadius: 8, padding: '8px 10px', lineHeight: 1.6 }}>
                <strong>How supervisor works:</strong><br />
                1. Select a coordinator agent above.<br />
                2. Drag from the <strong>→ right handle</strong> to the next pipeline node (forward flow).<br />
                3. Drag from the <strong>↓ bottom handle</strong> to each team agent node — edges are auto-labeled <code style={{ background: '#fde68a', padding: '1px 3px', borderRadius: 2 }}>delegate</code>.<br />
                4. At runtime, team agents become callable tools for the supervisor&apos;s LLM.
              </div>
            )}

            <button
              onClick={() => {
                setNodes((nds) => nds.filter((n) => n.id !== selectedNode.id))
                setEdges((eds) => eds.filter((e) => e.source !== selectedNode.id && e.target !== selectedNode.id))
                setSelectedNode(null)
              }}
              style={{
                marginTop: 'auto', padding: '7px 12px', fontSize: 12, borderRadius: 8,
                background: '#fff', color: '#ef4444', border: '1px solid #fecaca',
                cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6,
              }}
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

      {/* Run panel */}
      {runPanelOpen && (
        <div style={{
          position: 'fixed', bottom: 0, left: 0, right: 0,
          background: '#13111f', borderTop: '1px solid #2d2b3d',
          display: 'flex', flexDirection: 'column', zIndex: 50,
          maxHeight: '50vh', minHeight: 240,
        }}>
          <div style={{
            display: 'flex', alignItems: 'center', gap: 12, padding: '10px 16px',
            borderBottom: '1px solid #2d2b3d', flexShrink: 0,
          }}>
            <Play size={13} color="#534AB7" />
            <span style={{ fontSize: 12, fontWeight: 700, color: '#e2e0ff' }}>Run Workflow</span>
            <div style={{ flex: 1 }} />
            <button
              onClick={() => { setRunPanelOpen(false); setRunStatus('idle'); setRunError(null) }}
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#6b7280' }}
            >
              <X size={14} />
            </button>
          </div>

          <div style={{ display: 'flex', flex: 1, overflow: 'hidden', flexWrap: 'wrap' }}>
            {/* Input area */}
            <div style={{ minWidth: 200, width: '40%', flexShrink: 0, padding: '12px', display: 'flex', flexDirection: 'column', gap: 8, borderRight: '1px solid #2d2b3d' }}>
              <textarea
                value={runInput}
                onChange={(e) => setRunInput(e.target.value)}
                placeholder="Enter input for the pipeline…"
                style={{
                  flex: 1, resize: 'none', background: '#1e1c2e', border: '1px solid #3d3b52',
                  borderRadius: 8, padding: '8px 10px', fontSize: 12, color: '#e5e7eb',
                  outline: 'none', fontFamily: 'Inter, sans-serif',
                }}
              />
              {runError && (
                <div style={{
                  background: '#3d1a1a', border: '1px solid #7f1d1d', borderRadius: 8,
                  padding: '8px 10px', fontSize: 11, color: '#fca5a5', lineHeight: 1.4,
                }}>
                  {runError}
                </div>
              )}
              <button
                onClick={handleRun}
                disabled={!runInput.trim() || runStatus === 'running'}
                style={{
                  padding: '8px 16px', background: runStatus === 'running' ? '#3d3b52' : '#534AB7',
                  color: '#fff', border: 'none', borderRadius: 8, fontSize: 12, fontWeight: 600,
                  cursor: runStatus === 'running' ? 'not-allowed' : 'pointer',
                  display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
                }}
              >
                {runStatus === 'running'
                  ? <><Loader2 size={12} style={{ animation: 'spin 0.7s linear infinite' }} /> Running…</>
                  : <><Play size={12} /> Execute Pipeline</>
                }
              </button>
            </div>

            {/* Output area */}
            <div style={{ flex: 1, overflowY: 'auto', padding: '12px', display: 'flex', flexDirection: 'column', gap: 8 }}>
              {Object.keys(runOutput).length === 0 && runStatus === 'idle' && (
                <div style={{ color: '#4b5563', fontSize: 12, marginTop: 20, textAlign: 'center' }}>
                  Output will appear here once you execute the pipeline.
                </div>
              )}
              {Object.entries(runOutput).map(([nodeId, text]) => {
                const node = nodes.find((n) => n.id === nodeId)
                const label = nodeId === '__main__' ? 'Output' : node?.data?.label || nodeId
                return (
                  <div key={nodeId} style={{ background: '#1e1c2e', borderRadius: 8, padding: '10px 12px', border: '1px solid #2d2b3d' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
                      <ChevronRight size={11} color="#534AB7" />
                      <span style={{ fontSize: 11, fontWeight: 700, color: '#a5b4fc' }}>{label}</span>
                    </div>
                    <pre style={{ fontSize: 11, color: '#d1d5db', margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: 'monospace' }}>{text}</pre>
                  </div>
                )
              })}
              {runStatus === 'done' && Object.keys(runOutput).length > 0 && (
                <div style={{ fontSize: 11, color: '#22c55e', textAlign: 'center', padding: '4px 0' }}>Pipeline completed.</div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function ConfigField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div style={{ fontSize: 10, fontWeight: 600, color: '#6b7280', marginBottom: 4, textTransform: 'uppercase', letterSpacing: 0.4 }}>{label}</div>
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
