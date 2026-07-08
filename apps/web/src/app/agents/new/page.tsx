'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, ChevronDown, Save, X, Check, Search, GripVertical } from 'lucide-react'
import { agentsAPI, connectorsAPI, providersAPI, skillsAPI, toolsAPI } from '@/lib/api'
import type { Agent, AgentSkill, Connector, ModelInfo, ProviderCredential, Skill, Tool } from '@/types'
import { NexusDraftPanel, type AgentDraft } from '@/components/NexusDraftPanel'
import { InfoTip } from '@/components/ui/InfoTip'
import { FIELD_HELP } from '@/lib/field-help'

const TABS = ['Basics', 'Model', 'Instructions', 'Skills', 'Tools', 'Context', 'Memory', 'Guardrails'] as const
type Tab = typeof TABS[number]

const PROVIDERS = ['anthropic', 'openai', 'gemini', 'ollama']

const TOOL_GROUP_ORDER = ['Built-in', 'WhatsApp', 'MCP', 'HTTP', 'Code'] as const
type ToolGroup = typeof TOOL_GROUP_ORDER[number]

function getToolGroup(tool: Tool): ToolGroup {
  if (tool.name.startsWith('whatsapp_')) return 'WhatsApp'
  if (tool.name.startsWith('mcp_')) return 'MCP'
  if (tool.type === 'http') return 'HTTP'
  if ((tool.type as string) === 'code' || tool.name.startsWith('code_')) return 'Code'
  return 'Built-in'
}

const riskBadge = (r: string) => {
  const map: Record<string, string> = {
    low: 'bg-info/10 text-info', medium: 'bg-warn/10 text-warn',
    high: 'bg-warn/10 dark:bg-warn/10 text-warn dark:text-warn', critical: 'bg-crit/10 text-crit dark:text-crit',
  }
  return `text-[10px] font-medium px-2 py-0.5 rounded-full ${map[r] ?? 'bg-muted text-muted-foreground'}`
}

function Toggle({ on, onToggle, disabled }: { on: boolean; onToggle: () => void; disabled?: boolean }) {
  return (
    <button onClick={onToggle} disabled={disabled}
      className={`w-8 h-4.5 rounded-full transition-colors relative flex-shrink-0 ${on ? 'bg-accent' : 'bg-muted'} ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
      style={{ width: 32, height: 18 }}>
      <span className={`absolute top-0.5 w-3.5 h-3.5 bg-surface rounded-full transition-all ${on ? 'left-[14px]' : 'left-0.5'}`} />
    </button>
  )
}

export default function AgentBuilderPage({ agentId }: { agentId?: string }) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const isEdit = !!agentId
  const [tab, setTab] = useState<Tab>('Basics')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [status, setStatus] = useState('active')
  const [instructions, setInstructions] = useState('')
  const [maxTokens, setMaxTokens] = useState(4096)
  const [provider, setProvider] = useState('anthropic')
  const [model, setModel] = useState('')
  const [temperature, setTemperature] = useState(0.7)
  const [memoryEnabled, setMemoryEnabled] = useState(false)
  const [memoryScope, setMemoryScope] = useState('conversation')
  const [memorySaveMode, setMemorySaveMode] = useState<'tool' | 'extractor' | 'hybrid'>('hybrid')
  const [memoryReviewPolicy, setMemoryReviewPolicy] = useState<'none' | 'uncertain' | 'all'>('uncertain')
  const [maxMemories, setMaxMemories] = useState(5)
  const [minRelevanceScore, setMinRelevanceScore] = useState(0.70)
  const [memoryMinImportance, setMemoryMinImportance] = useState(0.70)
  const [memoryDedupeThreshold, setMemoryDedupeThreshold] = useState(0.88)
  const [contextEnabled, setContextEnabled] = useState(false)
  const [maxSteps, setMaxSteps] = useState(10)
  const [maxToolCalls, setMaxToolCalls] = useState(20)
  const [maxDurationSecs, setMaxDurationSecs] = useState(300)
  const [maxHistoryMessages, setMaxHistoryMessages] = useState(20)
  const [lazyToolLoading, setLazyToolLoading] = useState(false)
  const [compactionThreshold, setCompactionThreshold] = useState(6)
  const [compactionTokenThreshold, setCompactionTokenThreshold] = useState(3000)
  const [enabledTools, setEnabledTools] = useState<Record<string, boolean>>({ read_file: true })
  const [enabledSkills, setEnabledSkills] = useState<Record<string, boolean>>({})
  const [skillOrder, setSkillOrder] = useState<Record<string, number>>({})
  const [skillActivationMode, setSkillActivationMode] = useState<Record<string, 'always' | 'on_demand'>>({})
  const [enabledConnectors, setEnabledConnectors] = useState<Record<string, boolean>>({})
  const [maxChunks, setMaxChunks] = useState(8)
  const [minScore, setMinScore] = useState(0.5)
  const [agenticRAG, setAgenticRAG] = useState(false)
  const [saveError, setSaveError] = useState('')

  // Search & collapse state for Tools and Skills tabs
  const [toolSearch, setToolSearch] = useState('')
  const [skillSearch, setSkillSearch] = useState('')
  const [collapsedToolGroups, setCollapsedToolGroups] = useState<Set<ToolGroup>>(new Set())
  const [collapsedSkillGroups, setCollapsedSkillGroups] = useState<Set<string>>(new Set())

  const { data: existing, isLoading: isLoadingAgent } = useQuery({
    queryKey: ['agent', agentId],
    queryFn: () => agentsAPI.get(agentId!) as Promise<Agent>,
    enabled: isEdit,
  })

  const { data: agentTools } = useQuery({
    queryKey: ['agent-tools', agentId],
    queryFn: () => agentsAPI.getTools(agentId!) as Promise<{ data: Tool[] }>,
    enabled: isEdit,
  })
  const { data: agentConnectorsData } = useQuery({
    queryKey: ['agent-connectors', agentId],
    queryFn: () => agentsAPI.getConnectors(agentId!) as Promise<{ data: Connector[]; max_chunks: number; min_score: number }>,
    enabled: isEdit,
  })
  const { data: agentSkillsData } = useQuery({
    queryKey: ['agent-skills', agentId],
    queryFn: () => skillsAPI.listForAgent(agentId!) as Promise<{ data: AgentSkill[] }>,
    enabled: isEdit,
  })
  const { data: toolsData } = useQuery({
    queryKey: ['tools'],
    queryFn: () => toolsAPI.list() as Promise<{ data: Tool[] }>,
  })
  const { data: connectorsData } = useQuery({
    queryKey: ['connectors'],
    queryFn: () => connectorsAPI.list() as Promise<{ data: Connector[] }>,
  })
  const { data: skillsData } = useQuery({
    queryKey: ['skills'],
    queryFn: () => skillsAPI.list() as Promise<{ data: Skill[] }>,
  })

  const { data: providersData } = useQuery({
    queryKey: ['providers'],
    queryFn: () => providersAPI.list() as Promise<{ data: ProviderCredential[] }>,
  })

  const activeCred = providersData?.data?.find((p) => p.provider === provider && p.is_active)

  const { data: modelsData, isLoading: isLoadingModels } = useQuery({
    queryKey: ['provider-models', activeCred?.id],
    queryFn: () => providersAPI.models(activeCred!.id) as Promise<{ data: ModelInfo[] }>,
    enabled: !!activeCred?.id,
    staleTime: 5 * 60 * 1000,
  })

  const availableModels: ModelInfo[] = modelsData?.data ?? []
  const availableTools = toolsData?.data ?? []
  const availableSkills = skillsData?.data ?? []
  const availableConnectors = connectorsData?.data ?? []

  // Tools locked by an enabled skill
  const skillLockedTools = new Map<string, string>()
  availableSkills.forEach(s => {
    if (enabledSkills[s.id] && s.required_tool_names) {
      s.required_tool_names.forEach(n => skillLockedTools.set(n, s.name))
    }
  })

  // Derived counts for tab badges
  const enabledToolCount = Object.values(enabledTools).filter(Boolean).length
  const enabledSkillCount = Object.values(enabledSkills).filter(Boolean).length

  // Filtered + grouped tools
  const q = toolSearch.toLowerCase()
  const filteredTools = availableTools.filter(t =>
    !q || t.name.toLowerCase().includes(q) || t.description.toLowerCase().includes(q)
  )
  const toolsByGroup = Object.fromEntries(
    TOOL_GROUP_ORDER.map(g => [g, filteredTools.filter(t => getToolGroup(t) === g)])
  ) as Record<ToolGroup, Tool[]>

  // Filtered + grouped skills
  const sq = skillSearch.toLowerCase()
  const filteredSkills = availableSkills.filter(s =>
    !sq || s.name.toLowerCase().includes(sq) || s.description.toLowerCase().includes(sq)
  )
  const managedSkills = filteredSkills.filter(s => s.source === 'managed')
  const customSkills = filteredSkills.filter(s => s.source !== 'managed')

  useEffect(() => {
    if (!existing) return
    setName(existing.name)
    setDescription(existing.description ?? '')
    setStatus(existing.status)
    setInstructions(existing.instructions ?? '')
    setMaxTokens(existing.max_tokens)
    setProvider(existing.provider)
    setModel(existing.model)
    setTemperature(existing.temperature)
    setMemoryEnabled(existing.memory_enabled)
    setMemoryScope(existing.memory_scope)
    setMemorySaveMode(existing.memory_save_mode ?? 'hybrid')
    setMemoryReviewPolicy(existing.memory_review_policy ?? 'uncertain')
    setMaxMemories(existing.max_memories ?? 5)
    setMinRelevanceScore(existing.min_relevance_score ?? 0.70)
    setMemoryMinImportance(existing.memory_min_importance ?? 0.70)
    setMemoryDedupeThreshold(existing.memory_dedupe_threshold ?? 0.88)
    setContextEnabled(existing.context_retrieval_enabled)
    setAgenticRAG(existing.agentic_rag ?? false)
    setMaxSteps(existing.max_steps)
    setMaxToolCalls(existing.max_tool_calls)
    setMaxDurationSecs(existing.max_duration_secs ?? 300)
    setMaxHistoryMessages(existing.max_history_messages ?? 20)
    setLazyToolLoading(existing.lazy_tool_loading ?? false)
    setCompactionThreshold(existing.compaction_threshold ?? 6)
    setCompactionTokenThreshold(existing.compaction_token_threshold ?? 3000)
  }, [existing])

  useEffect(() => {
    if (agentTools?.data) {
      setEnabledTools(Object.fromEntries(agentTools.data.map((tool) => [tool.name, true])))
    }
  }, [agentTools])

  useEffect(() => {
    if (agentConnectorsData?.data) {
      setEnabledConnectors(Object.fromEntries(agentConnectorsData.data.map((c) => [c.id, true])))
      if (agentConnectorsData.max_chunks > 0) setMaxChunks(agentConnectorsData.max_chunks)
      if (agentConnectorsData.min_score > 0) setMinScore(agentConnectorsData.min_score)
    }
  }, [agentConnectorsData])

  useEffect(() => {
    if (agentSkillsData?.data) {
      setEnabledSkills(Object.fromEntries(agentSkillsData.data.map((s) => [s.skill_id, s.enabled])))
      setSkillOrder(Object.fromEntries(agentSkillsData.data.map((s) => [s.skill_id, s.order_index])))
      setSkillActivationMode(Object.fromEntries(
        agentSkillsData.data.map((s) => [s.skill_id, (s.activation_mode === 'on_demand' ? 'on_demand' : 'always') as 'always' | 'on_demand'])
      ))
    }
  }, [agentSkillsData])

  useEffect(() => {
    if (availableModels.length > 0 && !availableModels.find((m) => m.id === model)) {
      setModel(availableModels[0].id)
    }
  }, [availableModels, provider])

  const saveMutation = useMutation({
    mutationFn: async () => {
      const body = {
        name, description, instructions, status,
        provider, model, temperature,
        max_tokens: maxTokens,
        memory_enabled: memoryEnabled,
        memory_scope: memoryScope,
        memory_save_mode: memorySaveMode,
        memory_review_policy: memoryReviewPolicy,
        max_memories: maxMemories,
        min_relevance_score: minRelevanceScore,
        memory_min_importance: memoryMinImportance,
        memory_dedupe_threshold: memoryDedupeThreshold,
        context_retrieval_enabled: contextEnabled,
        agentic_rag: agenticRAG,
        max_steps: maxSteps,
        max_tool_calls: maxToolCalls,
        max_duration_secs: maxDurationSecs,
        max_history_messages: maxHistoryMessages,
        lazy_tool_loading: lazyToolLoading,
        compaction_threshold: compactionThreshold,
        compaction_token_threshold: compactionTokenThreshold,
      }
      const saved = isEdit ? await agentsAPI.update(agentId!, body) : await agentsAPI.create(body)
      const id = isEdit ? agentId! : (saved as Agent).id
      await agentsAPI.setTools(id, { tool_names: Object.entries(enabledTools).filter(([, enabled]) => enabled).map(([tool]) => tool) })
      await agentsAPI.setConnectors(id, {
        connector_ids: Object.entries(enabledConnectors).filter(([, enabled]) => enabled).map(([cid]) => cid),
        max_chunks: maxChunks,
        min_score: minScore,
      })
      await agentsAPI.setSkills(id, {
        skills: Object.entries(enabledSkills)
          .filter(([, enabled]) => enabled)
          .map(([skill_id], i) => ({
            skill_id,
            enabled: true,
            activation_mode: skillActivationMode[skill_id] ?? 'always',
            order_index: skillOrder[skill_id] ?? i,
          })),
      })
      return saved
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] })
      queryClient.invalidateQueries({ queryKey: ['agent', agentId] })
      queryClient.invalidateQueries({ queryKey: ['agent-tools', agentId] })
      queryClient.invalidateQueries({ queryKey: ['agent-skills', agentId] })
      router.push('/agents')
    },
    onError: (err: Error) => setSaveError(err.message),
  })

  function toggleSkill(skill: Skill) {
    const turningOn = !enabledSkills[skill.id]
    setEnabledSkills(prev => ({ ...prev, [skill.id]: turningOn }))
    if (skill.required_tool_names && skill.required_tool_names.length > 0) {
      if (turningOn) {
        setEnabledTools(prev => {
          const next = { ...prev }
          skill.required_tool_names!.forEach(n => { next[n] = true })
          return next
        })
      } else {
        const stillRequired = new Set<string>()
        availableSkills.forEach(s => {
          if (s.id !== skill.id && enabledSkills[s.id] && s.required_tool_names) {
            s.required_tool_names.forEach(n => stillRequired.add(n))
          }
        })
        setEnabledTools(prev => {
          const next = { ...prev }
          skill.required_tool_names!.forEach(n => { if (!stillRequired.has(n)) delete next[n] })
          return next
        })
      }
    }
  }

  function toggleToolGroup(g: ToolGroup) {
    setCollapsedToolGroups(prev => {
      const next = new Set(prev)
      next.has(g) ? next.delete(g) : next.add(g)
      return next
    })
  }

  function toggleSkillGroup(g: string) {
    setCollapsedSkillGroups(prev => {
      const next = new Set(prev)
      next.has(g) ? next.delete(g) : next.add(g)
      return next
    })
  }

  // applyDraft pre-fills the whole form from a Nexus AI agent draft. Selection
  // maps: tools are keyed by name, skills and connectors by id (matching the
  // form's existing state shape).
  function applyDraft(d: AgentDraft) {
    if (d.name) setName(d.name)
    if (d.description !== undefined) setDescription(d.description)
    if (d.instructions) setInstructions(d.instructions)
    if (d.provider) setProvider(d.provider)
    if (d.model) setModel(d.model)
    if (typeof d.temperature === 'number') setTemperature(d.temperature)
    if (typeof d.max_tokens === 'number') setMaxTokens(d.max_tokens)
    if (typeof d.max_steps === 'number') setMaxSteps(d.max_steps)
    if (typeof d.max_tool_calls === 'number') setMaxToolCalls(d.max_tool_calls)
    if (typeof d.max_duration_secs === 'number') setMaxDurationSecs(d.max_duration_secs)
    if (typeof d.memory_enabled === 'boolean') setMemoryEnabled(d.memory_enabled)
    if (d.memory_scope) setMemoryScope(d.memory_scope)
    if (typeof d.context_retrieval_enabled === 'boolean') setContextEnabled(d.context_retrieval_enabled)
    if (typeof d.agentic_rag === 'boolean') setAgenticRAG(d.agentic_rag)
    if (typeof d.max_chunks === 'number') setMaxChunks(d.max_chunks)
    if (typeof d.min_score === 'number') setMinScore(d.min_score)
    if (typeof d.lazy_tool_loading === 'boolean') setLazyToolLoading(d.lazy_tool_loading)
    if (d.status) setStatus(d.status)
    if (d.tools) setEnabledTools(Object.fromEntries(d.tools.map((t) => [t.name, true])))
    if (d.skills) setEnabledSkills(Object.fromEntries(d.skills.map((s) => [s.id, true])))
    if (d.connectors) setEnabledConnectors(Object.fromEntries(d.connectors.map((c) => [c.id, true])))
  }

  return (
    <div className="p-4 sm:p-6 max-w-3xl">
      {isEdit && isLoadingAgent && <div className="text-sm text-faint py-8 text-center">Loading agent…</div>}
      {/* Header */}
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div className="flex items-center gap-2 text-[12px] text-faint">
          <span className="hover:text-muted-foreground dark:hover:text-faint cursor-pointer" onClick={() => router.push('/agents')}>Agents</span>
          <ChevronRight className="w-3 h-3" />
          <span className="text-foreground font-medium">{isEdit ? 'Edit agent' : 'New agent'}</span>
        </div>
        <div className="flex gap-2">
          <button onClick={() => router.push('/agents')}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-border-strong text-muted-foreground text-[12px] rounded-lg hover:bg-muted">
            <X className="w-3.5 h-3.5" /> Discard
          </button>
          <button
            onClick={() => { if (!name.trim()) { setSaveError('Agent name is required'); setTab('Basics'); return; } saveMutation.mutate() }}
            disabled={saveMutation.isPending}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-accent text-white text-[12px] rounded-lg hover:bg-accent-hover disabled:opacity-60">
            <Save className="w-3.5 h-3.5" /> {saveMutation.isPending ? 'Saving…' : 'Save Agent'}
          </button>
        </div>
      </div>

      {/* Nexus AI fast-track — describe the agent and let Nexus draft it (create only) */}
      {!isEdit && <NexusDraftPanel onDraft={applyDraft} />}

      {/* Tab bar */}
      <div className="flex border-b border-border mb-5 bg-muted rounded-t-lg overflow-x-auto"
        style={{ scrollbarWidth: 'none' }}>
        {TABS.map(t => {
          const badge = t === 'Tools' && enabledToolCount > 0 ? enabledToolCount
            : t === 'Skills' && enabledSkillCount > 0 ? enabledSkillCount
            : null
          return (
            <button key={t} onClick={() => setTab(t)}
              className={`px-4 py-2 text-[12px] transition-colors whitespace-nowrap flex items-center gap-1.5
                ${tab === t ? 'bg-surface text-accent dark:text-accent-bright font-medium border-b-2 border-accent' : 'text-muted-foreground hover:text-foreground dark:hover:text-faint'}`}>
              {t}
              {badge !== null && (
                <span className="text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-accent/15 text-accent dark:text-accent-bright leading-none">{badge}</span>
              )}
            </button>
          )
        })}
      </div>

      {/* ── BASICS ── */}
      {tab === 'Basics' && (
        <div className="space-y-4">
          {saveError && (
            <div className="text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg px-3 py-2">{saveError}</div>
          )}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Field label="Agent name" hint="Shown in the sidebar and run history">
              <input type="text" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Backend Architect"
                className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none focus:ring-1 focus:ring-accent" />
            </Field>
            <Field label="Status">
              <select value={status} onChange={e => setStatus(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none">
                <option value="active">Active</option><option value="paused">Paused</option><option value="archived">Archived</option>
              </select>
            </Field>
          </div>
          <Field label="Description" hint="Brief summary shown on agent cards">
            <input type="text" value={description} onChange={e => setDescription(e.target.value)} placeholder="What does this agent do?"
              className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none focus:ring-1 focus:ring-accent" />
          </Field>
        </div>
      )}

      {/* ── MODEL ── */}
      {tab === 'Model' && (
        <div className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Field label="Provider">
              <select value={provider} onChange={e => { setProvider(e.target.value); setModel('') }}
                className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none">
                {PROVIDERS.map(p => <option key={p} value={p}>{p.charAt(0).toUpperCase() + p.slice(1)}</option>)}
              </select>
            </Field>
            <Field label="Model" hint={!activeCred ? `Add a ${provider} API key in Settings → Providers` : undefined}>
              <select value={model} onChange={e => setModel(e.target.value)}
                disabled={isLoadingModels || availableModels.length === 0}
                className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none disabled:opacity-50">
                {isLoadingModels && <option>Loading models…</option>}
                {!isLoadingModels && availableModels.length === 0 && !activeCred && (
                  <option value="">No provider configured</option>
                )}
                {availableModels.map(m => <option key={m.id} value={m.id}>{m.name || m.id}</option>)}
              </select>
            </Field>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Field label={`Temperature — ${temperature.toFixed(1)}`} hint="Lower = more deterministic">
              <input type="range" min="0" max="2" step="0.1" value={temperature}
                onChange={e => setTemperature(parseFloat(e.target.value))}
                className="w-full accent-[#534AB7]" />
            </Field>
            <Field label="Max tokens">
              <input type="number" value={maxTokens} onChange={e => setMaxTokens(Number(e.target.value))} min={256} max={128000}
                className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
            </Field>
          </div>
          <Field label="Model capabilities">
            <div className="flex gap-2 flex-wrap mt-1">
              {['Tool calling', 'Streaming', 'JSON mode'].map(c => (
                <span key={c} className="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full bg-good/10 text-good">
                  <Check className="w-3 h-3" />{c}
                </span>
              ))}
              <span className="text-[11px] px-2 py-0.5 rounded-full bg-muted text-muted-foreground">Vision</span>
            </div>
          </Field>
        </div>
      )}

      {/* ── INSTRUCTIONS ── */}
      {tab === 'Instructions' && (
        <div className="space-y-3">
          <Field label="System prompt" hint="This is the core instruction sent to the model on every run.">
            <textarea rows={12} value={instructions} onChange={e => setInstructions(e.target.value)}
              placeholder="You are a ..."
              className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none focus:ring-1 focus:ring-accent font-mono resize-y" />
          </Field>
        </div>
      )}

      {/* ── SKILLS ── */}
      {tab === 'Skills' && (
        <div className="space-y-3">
          <p className="text-[12px] text-muted-foreground">Attach reusable prompt instructions injected after the system prompt. <span className="text-faint">Always-on skills inject their content every run; on-demand skills are listed as callable tools the model can invoke when relevant.</span></p>

          {/* Search + summary + bulk action */}
          <div className="flex flex-wrap items-center gap-3">
            <div className="relative flex-1">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-faint pointer-events-none" />
              <input
                type="text" value={skillSearch} onChange={e => setSkillSearch(e.target.value)}
                placeholder="Search skills…"
                className="w-full text-[12px] pl-8 pr-3 py-1.5 border border-border-strong rounded-lg bg-surface focus:outline-none focus:ring-1 focus:ring-accent" />
            </div>
            {enabledSkillCount > 0 && (
              <>
                <span className="text-[11px] text-accent dark:text-accent-bright font-medium whitespace-nowrap">
                  {enabledSkillCount} of {availableSkills.length} enabled
                </span>
                <button
                  type="button"
                  onClick={() => {
                    const next: Record<string, 'always' | 'on_demand'> = {}
                    Object.entries(enabledSkills).forEach(([id, on]) => {
                      if (on) next[id] = 'on_demand'
                    })
                    setSkillActivationMode(prev => ({ ...prev, ...next }))
                  }}
                  className="text-[11px] px-2.5 py-1 rounded-lg border border-warn/30 bg-warn/10 text-warn hover:bg-warn/15 whitespace-nowrap transition-colors">
                  All on-demand
                </button>
                <button
                  type="button"
                  onClick={() => {
                    const next: Record<string, 'always' | 'on_demand'> = {}
                    Object.entries(enabledSkills).forEach(([id, on]) => {
                      if (on) next[id] = 'always'
                    })
                    setSkillActivationMode(prev => ({ ...prev, ...next }))
                  }}
                  className="text-[11px] px-2.5 py-1 rounded-lg border border-border-strong bg-surface text-muted-foreground hover:bg-muted whitespace-nowrap transition-colors">
                  All always
                </button>
              </>
            )}
          </div>

          {availableSkills.length === 0 && (
            <div className="p-8 text-center text-[12px] text-faint bg-surface border border-border rounded-xl">No skills available.</div>
          )}

          {/* Managed skills */}
          {managedSkills.length > 0 && (
            <SkillSection
              label="Managed"
              description="Platform-provided skills"
              skills={managedSkills}
              enabledSkills={enabledSkills}
              activationModes={skillActivationMode}
              collapsed={collapsedSkillGroups.has('managed')}
              onToggleCollapse={() => toggleSkillGroup('managed')}
              onToggleSkill={toggleSkill}
              onToggleMode={(skill) => setSkillActivationMode(prev => ({
                ...prev,
                [skill.id]: prev[skill.id] === 'on_demand' ? 'always' : 'on_demand',
              }))}
            />
          )}

          {/* Custom skills */}
          {customSkills.length > 0 && (
            <SkillSection
              label="Custom"
              description="Created in this workspace"
              skills={customSkills}
              enabledSkills={enabledSkills}
              activationModes={skillActivationMode}
              collapsed={collapsedSkillGroups.has('custom')}
              onToggleCollapse={() => toggleSkillGroup('custom')}
              onToggleSkill={toggleSkill}
              onToggleMode={(skill) => setSkillActivationMode(prev => ({
                ...prev,
                [skill.id]: prev[skill.id] === 'on_demand' ? 'always' : 'on_demand',
              }))}
            />
          )}

          {availableSkills.length > 0 && filteredSkills.length === 0 && (
            <div className="p-6 text-center text-[12px] text-faint bg-surface border border-border rounded-xl">No skills match your search.</div>
          )}
        </div>
      )}

      {/* ── TOOLS ── */}
      {tab === 'Tools' && (
        <div className="space-y-3">
          <p className="text-[12px] text-muted-foreground">Enable tools this agent can use. High-risk tools require approval by default.</p>

          {/* Search + summary */}
          <div className="flex items-center gap-3">
            <div className="relative flex-1">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-faint pointer-events-none" />
              <input
                type="text" value={toolSearch} onChange={e => setToolSearch(e.target.value)}
                placeholder="Search tools…"
                className="w-full text-[12px] pl-8 pr-3 py-1.5 border border-border-strong rounded-lg bg-surface focus:outline-none focus:ring-1 focus:ring-accent" />
            </div>
            {enabledToolCount > 0 && (
              <span className="text-[11px] text-accent dark:text-accent-bright font-medium whitespace-nowrap">
                {enabledToolCount} of {availableTools.length} enabled
              </span>
            )}
          </div>

          {availableTools.length === 0 && (
            <div className="p-8 text-center text-[12px] text-faint bg-surface border border-border rounded-xl">No tools registered.</div>
          )}

          {TOOL_GROUP_ORDER.map(group => {
            const tools = toolsByGroup[group]
            if (tools.length === 0) return null
            const groupEnabled = tools.filter(t => enabledTools[t.name] || skillLockedTools.has(t.name)).length
            const collapsed = collapsedToolGroups.has(group)
            return (
              <div key={group} className="bg-surface border border-border rounded-xl overflow-hidden">
                {/* Group header */}
                <button
                  onClick={() => toggleToolGroup(group)}
                  className="w-full flex items-center justify-between px-4 py-2.5 bg-muted hover:bg-muted transition-colors border-b border-border">
                  <div className="flex items-center gap-2">
                    <span className="text-[12px] font-semibold text-foreground">{group}</span>
                    <span className="text-[10px] text-faint">{groupEnabled > 0 ? `${groupEnabled} / ${tools.length} enabled` : `${tools.length} tools`}</span>
                  </div>
                  <ChevronDown className={`w-3.5 h-3.5 text-faint transition-transform ${collapsed ? '-rotate-90' : ''}`} />
                </button>

                {/* Tool rows */}
                {!collapsed && tools.map((tool, i) => {
                  const lockedBySkill = skillLockedTools.get(tool.name)
                  return (
                    <div key={tool.id} className={`flex items-center justify-between px-4 py-3 ${i < tools.length - 1 ? 'border-b border-border' : ''}`}>
                      <div className="min-w-0 mr-3">
                        <p className="text-[13px] font-medium text-foreground">{tool.name}</p>
                        <p className="text-[11px] text-muted-foreground truncate">{tool.description}</p>
                        {lockedBySkill && <p className="text-[10px] text-accent dark:text-accent-bright mt-0.5">Required by: {lockedBySkill}</p>}
                      </div>
                      <div className="flex items-center gap-2 shrink-0">
                        <span className={riskBadge(tool.risk_level)}>{tool.risk_level}</span>
                        {tool.requires_approval && (
                          <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-accent/10 text-accent dark:text-accent-bright">approval</span>
                        )}
                        <Toggle
                          on={!!enabledTools[tool.name] || !!lockedBySkill}
                          disabled={!!lockedBySkill}
                          onToggle={() => { if (!lockedBySkill) setEnabledTools(prev => ({ ...prev, [tool.name]: !prev[tool.name] })) }}
                        />
                      </div>
                    </div>
                  )
                })}
              </div>
            )
          })}

          {availableTools.length > 0 && filteredTools.length === 0 && (
            <div className="p-6 text-center text-[12px] text-faint bg-surface border border-border rounded-xl">No tools match your search.</div>
          )}
        </div>
      )}

      {/* ── CONTEXT ── */}
      {tab === 'Context' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between p-3 bg-muted border border-border rounded-lg">
            <div>
              <p className="text-[13px] font-medium text-foreground">Enable context retrieval</p>
              <p className="text-[11px] text-muted-foreground">Retrieve relevant docs from connected sources before each run</p>
            </div>
            <Toggle on={contextEnabled} onToggle={() => setContextEnabled(v => !v)} />
          </div>
          {contextEnabled && (
            <>
              <p className="text-[12px] font-medium text-foreground">Allowed sources</p>
              <div className="bg-surface border border-border rounded-xl overflow-hidden">
                {availableConnectors.map((connector, i) => (
                  <div key={connector.id} className={`flex items-center justify-between px-4 py-2.5 ${i < availableConnectors.length - 1 ? 'border-b border-border' : ''}`}>
                    <p className="text-[13px] text-foreground">{connector.name}</p>
                    <Toggle on={!!enabledConnectors[connector.id]}
                      onToggle={() => setEnabledConnectors(prev => ({ ...prev, [connector.id]: !prev[connector.id] }))} />
                  </div>
                ))}
                {availableConnectors.length === 0 && <div className="p-8 text-center text-[12px] text-faint">No connectors configured.</div>}
              </div>
              <div className="flex items-center justify-between p-3 bg-muted border border-border rounded-lg">
                <div>
                  <p className="text-[13px] font-medium text-foreground">Agentic RAG</p>
                  <p className="text-[11px] text-muted-foreground">Let the agent decide when and what to retrieve — uses <code className="bg-muted px-1 rounded">native_retrieve_context</code> tool instead of pre-run injection</p>
                </div>
                <Toggle on={agenticRAG} onToggle={() => setAgenticRAG(v => !v)} />
              </div>
              {!agenticRAG && (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <Field label="Max chunks per run">
                    <input type="number" value={maxChunks} min={1} max={100}
                      onChange={e => setMaxChunks(Math.max(1, parseInt(e.target.value) || 1))}
                      className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
                  </Field>
                  <Field label="Min relevance score">
                    <input type="number" value={minScore} step={0.05} min={0} max={1}
                      onChange={e => setMinScore(Math.min(1, Math.max(0, parseFloat(e.target.value) || 0)))}
                      className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
                  </Field>
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* ── MEMORY ── */}
      {tab === 'Memory' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between p-3 bg-muted border border-border rounded-lg">
            <div>
              <p className="text-[13px] font-medium text-foreground">Enable memory</p>
              <p className="text-[11px] text-muted-foreground">Store and retrieve long-term memories between runs</p>
            </div>
            <Toggle on={memoryEnabled} onToggle={() => setMemoryEnabled(v => !v)} />
          </div>
          {memoryEnabled && (
            <>
              <Field label="Memory scope">
                <select value={memoryScope} onChange={e => setMemoryScope(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none">
                  <option value="agent">Agent memory (this agent only)</option>
                  <option value="workspace">Workspace memory (shared across agents)</option>
                  <option value="conversation">Conversation only (no persistence)</option>
                </select>
              </Field>
              <Field label="Retrieval strategy">
                <select className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none">
                  <option>Vector search + summary</option>
                  <option>Vector search only</option>
                  <option>Summary only</option>
                </select>
              </Field>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Field label="Save mode">
                  <select value={memorySaveMode} onChange={e => setMemorySaveMode(e.target.value as 'tool' | 'extractor' | 'hybrid')} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none">
                    <option value="hybrid">Hybrid</option>
                    <option value="tool">Tool only</option>
                    <option value="extractor">Extractor only</option>
                  </select>
                </Field>
                <Field label="Review policy">
                  <select value={memoryReviewPolicy} onChange={e => setMemoryReviewPolicy(e.target.value as 'none' | 'uncertain' | 'all')} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none">
                    <option value="uncertain">Review uncertain</option>
                    <option value="all">Review all saves</option>
                    <option value="none">No review</option>
                  </select>
                </Field>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Field label="Max memories per run">
                  <input type="number" value={maxMemories} onChange={e => setMaxMemories(Number(e.target.value))} min={1} max={50} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
                </Field>
                <Field label="Min relevance score">
                  <input type="number" value={minRelevanceScore} onChange={e => setMinRelevanceScore(Number(e.target.value))} step={0.05} min={0} max={1} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
                </Field>
                <Field label="Min save importance">
                  <input type="number" value={memoryMinImportance} onChange={e => setMemoryMinImportance(Number(e.target.value))} step={0.05} min={0} max={1} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
                </Field>
                <Field label="Dedupe threshold">
                  <input type="number" value={memoryDedupeThreshold} onChange={e => setMemoryDedupeThreshold(Number(e.target.value))} step={0.01} min={0} max={1} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
                </Field>
              </div>
            </>
          )}
        </div>
      )}

      {/* ── GUARDRAILS ── */}
      {tab === 'Guardrails' && (
        <div className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Field label="Max steps">
              <input type="number" value={maxSteps} onChange={e => setMaxSteps(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
            </Field>
            <Field label="Max tool calls per run">
              <input type="number" value={maxToolCalls} onChange={e => setMaxToolCalls(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
            </Field>
            <Field label="Max run duration (sec)">
              <input type="number" value={maxDurationSecs} onChange={e => setMaxDurationSecs(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
            </Field>
            <Field label="Max history messages" hint="Verbatim turns kept when compaction is active. Default 20 (no compaction).">
              <input type="number" min={1} max={100} value={maxHistoryMessages} onChange={e => setMaxHistoryMessages(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
            </Field>
            <Field label="Compact after N messages" hint="Older turns are summarised once this many messages accumulate. Default 6.">
              <input type="number" min={2} value={compactionThreshold} onChange={e => setCompactionThreshold(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
            </Field>
            <Field label="Compact after N input tokens" hint="Compacts on first run where input tokens exceed this value. Default 3000.">
              <input type="number" min={500} value={compactionTokenThreshold} onChange={e => setCompactionTokenThreshold(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface focus:outline-none" />
            </Field>
          </div>
          <div className="bg-surface border border-border rounded-xl overflow-hidden mt-2">
            <div className="flex items-center justify-between px-4 py-3">
              <div>
                <p className="text-[13px] text-foreground">Lazy tool loading</p>
                <p className="text-[11px] text-faint mt-0.5">Tools are activated on demand — reduces token cost, adds 1 turn latency on first tool use per run.</p>
              </div>
              <Toggle on={lazyToolLoading} onToggle={() => setLazyToolLoading(v => !v)} />
            </div>
          </div>
          <p className="text-[12px] font-medium text-foreground mt-2">Require approval for</p>
          <div className="bg-surface border border-border rounded-xl overflow-hidden">
            {['File write operations', 'Outbound HTTP requests', 'Shell / command execution', 'Email / send actions'].map((item, i, arr) => (
              <div key={item} className={`flex items-center justify-between px-4 py-3 ${i < arr.length - 1 ? 'border-b border-border' : ''}`}>
                <p className="text-[13px] text-foreground">{item}</p>
                <Toggle on={item !== 'Shell / command execution' ? true : false} onToggle={() => {}} />
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function SkillSection({
  label, description, skills, enabledSkills, activationModes, collapsed, onToggleCollapse, onToggleSkill, onToggleMode,
}: {
  label: string
  description: string
  skills: Skill[]
  enabledSkills: Record<string, boolean>
  activationModes: Record<string, 'always' | 'on_demand'>
  collapsed: boolean
  onToggleCollapse: () => void
  onToggleSkill: (skill: Skill) => void
  onToggleMode: (skill: Skill) => void
}) {
  const enabledCount = skills.filter(s => enabledSkills[s.id]).length
  return (
    <div className="bg-surface border border-border rounded-xl overflow-hidden">
      <button
        onClick={onToggleCollapse}
        className="w-full flex items-center justify-between px-4 py-2.5 bg-muted hover:bg-muted transition-colors border-b border-border">
        <div className="flex items-center gap-2">
          <span className="text-[12px] font-semibold text-foreground">{label}</span>
          <span className="text-[10px] text-faint">{description}</span>
          {enabledCount > 0 && (
            <span className="text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-accent/15 text-accent dark:text-accent-bright leading-none">{enabledCount} on</span>
          )}
        </div>
        <ChevronDown className={`w-3.5 h-3.5 text-faint transition-transform ${collapsed ? '-rotate-90' : ''}`} />
      </button>
      {!collapsed && skills.map((skill, i) => {
        const isEnabled = !!enabledSkills[skill.id]
        const mode = activationModes[skill.id] ?? 'always'
        return (
          <div key={skill.id} className={`flex items-center justify-between gap-3 px-4 py-3 ${i < skills.length - 1 ? 'border-b border-border' : ''}`}>
            <div className="flex items-center gap-2 min-w-0">
              <GripVertical className="w-3.5 h-3.5 text-faint shrink-0" />
              <div className="min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <p className="text-[13px] font-medium text-foreground">{skill.name}</p>
                  {skill.required_tool_names && skill.required_tool_names.length > 0 && (
                    <span className="text-[10px] text-warn dark:text-warn">Requires: {skill.required_tool_names.join(', ')}</span>
                  )}
                </div>
                <p className="text-[11px] text-muted-foreground truncate">{skill.description}</p>
              </div>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              {isEnabled && (
                <button
                  type="button"
                  onClick={() => onToggleMode(skill)}
                  title={mode === 'on_demand' ? 'On-demand: model calls this skill as a tool' : 'Always: injected into every run'}
                  className={`text-[10px] font-medium px-2 py-0.5 rounded-full border transition-colors ${
                    mode === 'on_demand'
                      ? 'bg-warn/10 border-warn/30 text-warn hover:bg-warn/15'
                      : 'bg-info/10 border-info/30 text-info hover:bg-info/15'
                  }`}>
                  {mode === 'on_demand' ? 'on-demand' : 'always'}
                </button>
              )}
              <Toggle on={isEnabled} onToggle={() => onToggleSkill(skill)} />
            </div>
          </div>
        )
      })}
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  const help = FIELD_HELP[label]
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-[11px] font-medium text-muted-foreground flex items-center gap-1">
        {label}
        {help && <InfoTip text={help} />}
      </label>
      {hint && <p className="text-[10px] text-faint -mt-1">{hint}</p>}
      {children}
    </div>
  )
}
