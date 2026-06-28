'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, ChevronDown, Save, X, Check, Search, GripVertical } from 'lucide-react'
import { agentsAPI, connectorsAPI, providersAPI, skillsAPI, toolsAPI } from '@/lib/api'
import type { Agent, AgentSkill, Connector, ModelInfo, ProviderCredential, Skill, Tool } from '@/types'

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
    low: 'bg-blue-50 text-blue-700', medium: 'bg-amber-50 text-amber-800',
    high: 'bg-orange-50 text-orange-800', critical: 'bg-red-50 text-red-700',
  }
  return `text-[10px] font-medium px-2 py-0.5 rounded-full ${map[r] ?? 'bg-gray-100 text-gray-600'}`
}

function Toggle({ on, onToggle, disabled }: { on: boolean; onToggle: () => void; disabled?: boolean }) {
  return (
    <button onClick={onToggle} disabled={disabled}
      className={`w-8 h-4.5 rounded-full transition-colors relative flex-shrink-0 ${on ? 'bg-purple-600' : 'bg-gray-200'} ${disabled ? 'opacity-50 cursor-not-allowed' : ''}`}
      style={{ width: 32, height: 18 }}>
      <span className={`absolute top-0.5 w-3.5 h-3.5 bg-white rounded-full transition-all ${on ? 'left-[14px]' : 'left-0.5'}`} />
    </button>
  )
}

export default function AgentBuilderPage({ params }: { params?: { id?: string } }) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const isEdit = !!params?.id
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
    queryKey: ['agent', params?.id],
    queryFn: () => agentsAPI.get(params!.id!) as Promise<Agent>,
    enabled: isEdit,
  })

  const { data: agentTools } = useQuery({
    queryKey: ['agent-tools', params?.id],
    queryFn: () => agentsAPI.getTools(params!.id!) as Promise<{ data: Tool[] }>,
    enabled: isEdit,
  })
  const { data: agentConnectorsData } = useQuery({
    queryKey: ['agent-connectors', params?.id],
    queryFn: () => agentsAPI.getConnectors(params!.id!) as Promise<{ data: Connector[]; max_chunks: number; min_score: number }>,
    enabled: isEdit,
  })
  const { data: agentSkillsData } = useQuery({
    queryKey: ['agent-skills', params?.id],
    queryFn: () => skillsAPI.listForAgent(params!.id!) as Promise<{ data: AgentSkill[] }>,
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
      const saved = isEdit ? await agentsAPI.update(params!.id!, body) : await agentsAPI.create(body)
      const id = isEdit ? params!.id! : (saved as Agent).id
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
      queryClient.invalidateQueries({ queryKey: ['agent', params?.id] })
      queryClient.invalidateQueries({ queryKey: ['agent-tools', params?.id] })
      queryClient.invalidateQueries({ queryKey: ['agent-skills', params?.id] })
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

  return (
    <div className="p-4 sm:p-6 max-w-3xl">
      {isEdit && isLoadingAgent && <div className="text-sm text-gray-400 py-8 text-center">Loading agent…</div>}
      {/* Header */}
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div className="flex items-center gap-2 text-[12px] text-gray-400">
          <span className="hover:text-gray-600 cursor-pointer" onClick={() => router.push('/agents')}>Agents</span>
          <ChevronRight className="w-3 h-3" />
          <span className="text-gray-700 font-medium">{isEdit ? 'Edit agent' : 'New agent'}</span>
        </div>
        <div className="flex gap-2">
          <button onClick={() => router.push('/agents')}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-gray-200 text-gray-600 text-[12px] rounded-lg hover:bg-gray-50">
            <X className="w-3.5 h-3.5" /> Discard
          </button>
          <button
            onClick={() => { if (!name.trim()) { setSaveError('Agent name is required'); setTab('Basics'); return; } saveMutation.mutate() }}
            disabled={saveMutation.isPending}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg hover:bg-purple-700 disabled:opacity-60">
            <Save className="w-3.5 h-3.5" /> {saveMutation.isPending ? 'Saving…' : 'Save Agent'}
          </button>
        </div>
      </div>

      {/* Tab bar */}
      <div className="flex border-b border-gray-100 mb-5 bg-gray-50 rounded-t-lg overflow-x-auto"
        style={{ scrollbarWidth: 'none' }}>
        {TABS.map(t => {
          const badge = t === 'Tools' && enabledToolCount > 0 ? enabledToolCount
            : t === 'Skills' && enabledSkillCount > 0 ? enabledSkillCount
            : null
          return (
            <button key={t} onClick={() => setTab(t)}
              className={`px-4 py-2 text-[12px] transition-colors whitespace-nowrap flex items-center gap-1.5
                ${tab === t ? 'bg-white text-purple-600 font-medium border-b-2 border-purple-500' : 'text-gray-500 hover:text-gray-700'}`}>
              {t}
              {badge !== null && (
                <span className="text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-purple-100 text-purple-700 leading-none">{badge}</span>
              )}
            </button>
          )
        })}
      </div>

      {/* ── BASICS ── */}
      {tab === 'Basics' && (
        <div className="space-y-4">
          {saveError && (
            <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{saveError}</div>
          )}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Field label="Agent name" hint="Shown in the sidebar and run history">
              <input type="text" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Backend Architect"
                className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400" />
            </Field>
            <Field label="Status">
              <select value={status} onChange={e => setStatus(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none">
                <option value="active">Active</option><option value="paused">Paused</option><option value="archived">Archived</option>
              </select>
            </Field>
          </div>
          <Field label="Description" hint="Brief summary shown on agent cards">
            <input type="text" value={description} onChange={e => setDescription(e.target.value)} placeholder="What does this agent do?"
              className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400" />
          </Field>
        </div>
      )}

      {/* ── MODEL ── */}
      {tab === 'Model' && (
        <div className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Field label="Provider">
              <select value={provider} onChange={e => { setProvider(e.target.value); setModel('') }}
                className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none">
                {PROVIDERS.map(p => <option key={p} value={p}>{p.charAt(0).toUpperCase() + p.slice(1)}</option>)}
              </select>
            </Field>
            <Field label="Model" hint={!activeCred ? `Add a ${provider} API key in Settings → Providers` : undefined}>
              <select value={model} onChange={e => setModel(e.target.value)}
                disabled={isLoadingModels || availableModels.length === 0}
                className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none disabled:opacity-50">
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
                className="w-full accent-purple-600" />
            </Field>
            <Field label="Max tokens">
              <input type="number" value={maxTokens} onChange={e => setMaxTokens(Number(e.target.value))} min={256} max={128000}
                className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
          </div>
          <Field label="Model capabilities">
            <div className="flex gap-2 flex-wrap mt-1">
              {['Tool calling', 'Streaming', 'JSON mode'].map(c => (
                <span key={c} className="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-full bg-green-50 text-green-700">
                  <Check className="w-3 h-3" />{c}
                </span>
              ))}
              <span className="text-[11px] px-2 py-0.5 rounded-full bg-gray-100 text-gray-500">Vision</span>
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
              className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400 font-mono resize-y" />
          </Field>
        </div>
      )}

      {/* ── SKILLS ── */}
      {tab === 'Skills' && (
        <div className="space-y-3">
          <p className="text-[12px] text-gray-500">Attach reusable prompt instructions injected after the system prompt. <span className="text-gray-400">Always-on skills inject their content every run; on-demand skills are listed as callable tools the model can invoke when relevant.</span></p>

          {/* Search + summary + bulk action */}
          <div className="flex flex-wrap items-center gap-3">
            <div className="relative flex-1">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-400 pointer-events-none" />
              <input
                type="text" value={skillSearch} onChange={e => setSkillSearch(e.target.value)}
                placeholder="Search skills…"
                className="w-full text-[12px] pl-8 pr-3 py-1.5 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400" />
            </div>
            {enabledSkillCount > 0 && (
              <>
                <span className="text-[11px] text-purple-700 font-medium whitespace-nowrap">
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
                  className="text-[11px] px-2.5 py-1 rounded-lg border border-amber-200 bg-amber-50 text-amber-700 hover:bg-amber-100 whitespace-nowrap transition-colors">
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
                  className="text-[11px] px-2.5 py-1 rounded-lg border border-gray-200 bg-white text-gray-600 hover:bg-gray-50 whitespace-nowrap transition-colors">
                  All always
                </button>
              </>
            )}
          </div>

          {availableSkills.length === 0 && (
            <div className="p-8 text-center text-[12px] text-gray-400 bg-white border border-gray-100 rounded-xl">No skills available.</div>
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
            <div className="p-6 text-center text-[12px] text-gray-400 bg-white border border-gray-100 rounded-xl">No skills match your search.</div>
          )}
        </div>
      )}

      {/* ── TOOLS ── */}
      {tab === 'Tools' && (
        <div className="space-y-3">
          <p className="text-[12px] text-gray-500">Enable tools this agent can use. High-risk tools require approval by default.</p>

          {/* Search + summary */}
          <div className="flex items-center gap-3">
            <div className="relative flex-1">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-400 pointer-events-none" />
              <input
                type="text" value={toolSearch} onChange={e => setToolSearch(e.target.value)}
                placeholder="Search tools…"
                className="w-full text-[12px] pl-8 pr-3 py-1.5 border border-gray-200 rounded-lg bg-white focus:outline-none focus:ring-1 focus:ring-purple-400" />
            </div>
            {enabledToolCount > 0 && (
              <span className="text-[11px] text-purple-700 font-medium whitespace-nowrap">
                {enabledToolCount} of {availableTools.length} enabled
              </span>
            )}
          </div>

          {availableTools.length === 0 && (
            <div className="p-8 text-center text-[12px] text-gray-400 bg-white border border-gray-100 rounded-xl">No tools registered.</div>
          )}

          {TOOL_GROUP_ORDER.map(group => {
            const tools = toolsByGroup[group]
            if (tools.length === 0) return null
            const groupEnabled = tools.filter(t => enabledTools[t.name] || skillLockedTools.has(t.name)).length
            const collapsed = collapsedToolGroups.has(group)
            return (
              <div key={group} className="bg-white border border-gray-100 rounded-xl overflow-hidden">
                {/* Group header */}
                <button
                  onClick={() => toggleToolGroup(group)}
                  className="w-full flex items-center justify-between px-4 py-2.5 bg-gray-50 hover:bg-gray-100 transition-colors border-b border-gray-100">
                  <div className="flex items-center gap-2">
                    <span className="text-[12px] font-semibold text-gray-700">{group}</span>
                    <span className="text-[10px] text-gray-400">{groupEnabled > 0 ? `${groupEnabled} / ${tools.length} enabled` : `${tools.length} tools`}</span>
                  </div>
                  <ChevronDown className={`w-3.5 h-3.5 text-gray-400 transition-transform ${collapsed ? '-rotate-90' : ''}`} />
                </button>

                {/* Tool rows */}
                {!collapsed && tools.map((tool, i) => {
                  const lockedBySkill = skillLockedTools.get(tool.name)
                  return (
                    <div key={tool.id} className={`flex items-center justify-between px-4 py-3 ${i < tools.length - 1 ? 'border-b border-gray-50' : ''}`}>
                      <div className="min-w-0 mr-3">
                        <p className="text-[13px] font-medium text-gray-900">{tool.name}</p>
                        <p className="text-[11px] text-gray-500 truncate">{tool.description}</p>
                        {lockedBySkill && <p className="text-[10px] text-purple-600 mt-0.5">Required by: {lockedBySkill}</p>}
                      </div>
                      <div className="flex items-center gap-2 shrink-0">
                        <span className={riskBadge(tool.risk_level)}>{tool.risk_level}</span>
                        {tool.requires_approval && (
                          <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-purple-50 text-purple-700">approval</span>
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
            <div className="p-6 text-center text-[12px] text-gray-400 bg-white border border-gray-100 rounded-xl">No tools match your search.</div>
          )}
        </div>
      )}

      {/* ── CONTEXT ── */}
      {tab === 'Context' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between p-3 bg-gray-50 border border-gray-100 rounded-lg">
            <div>
              <p className="text-[13px] font-medium text-gray-900">Enable context retrieval</p>
              <p className="text-[11px] text-gray-500">Retrieve relevant docs from connected sources before each run</p>
            </div>
            <Toggle on={contextEnabled} onToggle={() => setContextEnabled(v => !v)} />
          </div>
          {contextEnabled && (
            <>
              <p className="text-[12px] font-medium text-gray-700">Allowed sources</p>
              <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
                {availableConnectors.map((connector, i) => (
                  <div key={connector.id} className={`flex items-center justify-between px-4 py-2.5 ${i < availableConnectors.length - 1 ? 'border-b border-gray-50' : ''}`}>
                    <p className="text-[13px] text-gray-800">{connector.name}</p>
                    <Toggle on={!!enabledConnectors[connector.id]}
                      onToggle={() => setEnabledConnectors(prev => ({ ...prev, [connector.id]: !prev[connector.id] }))} />
                  </div>
                ))}
                {availableConnectors.length === 0 && <div className="p-8 text-center text-[12px] text-gray-400">No connectors configured.</div>}
              </div>
              <div className="flex items-center justify-between p-3 bg-gray-50 border border-gray-100 rounded-lg">
                <div>
                  <p className="text-[13px] font-medium text-gray-900">Agentic RAG</p>
                  <p className="text-[11px] text-gray-500">Let the agent decide when and what to retrieve — uses <code className="bg-gray-100 px-1 rounded">native_retrieve_context</code> tool instead of pre-run injection</p>
                </div>
                <Toggle on={agenticRAG} onToggle={() => setAgenticRAG(v => !v)} />
              </div>
              {!agenticRAG && (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <Field label="Max chunks per run">
                    <input type="number" value={maxChunks} min={1} max={100}
                      onChange={e => setMaxChunks(Math.max(1, parseInt(e.target.value) || 1))}
                      className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
                  </Field>
                  <Field label="Min relevance score">
                    <input type="number" value={minScore} step={0.05} min={0} max={1}
                      onChange={e => setMinScore(Math.min(1, Math.max(0, parseFloat(e.target.value) || 0)))}
                      className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
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
          <div className="flex items-center justify-between p-3 bg-gray-50 border border-gray-100 rounded-lg">
            <div>
              <p className="text-[13px] font-medium text-gray-900">Enable memory</p>
              <p className="text-[11px] text-gray-500">Store and retrieve long-term memories between runs</p>
            </div>
            <Toggle on={memoryEnabled} onToggle={() => setMemoryEnabled(v => !v)} />
          </div>
          {memoryEnabled && (
            <>
              <Field label="Memory scope">
                <select value={memoryScope} onChange={e => setMemoryScope(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none">
                  <option value="agent">Agent memory (this agent only)</option>
                  <option value="workspace">Workspace memory (shared across agents)</option>
                  <option value="conversation">Conversation only (no persistence)</option>
                </select>
              </Field>
              <Field label="Retrieval strategy">
                <select className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none">
                  <option>Vector search + summary</option>
                  <option>Vector search only</option>
                  <option>Summary only</option>
                </select>
              </Field>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Field label="Save mode">
                  <select value={memorySaveMode} onChange={e => setMemorySaveMode(e.target.value as 'tool' | 'extractor' | 'hybrid')} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none">
                    <option value="hybrid">Hybrid</option>
                    <option value="tool">Tool only</option>
                    <option value="extractor">Extractor only</option>
                  </select>
                </Field>
                <Field label="Review policy">
                  <select value={memoryReviewPolicy} onChange={e => setMemoryReviewPolicy(e.target.value as 'none' | 'uncertain' | 'all')} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none">
                    <option value="uncertain">Review uncertain</option>
                    <option value="all">Review all saves</option>
                    <option value="none">No review</option>
                  </select>
                </Field>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Field label="Max memories per run">
                  <input type="number" value={maxMemories} onChange={e => setMaxMemories(Number(e.target.value))} min={1} max={50} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
                </Field>
                <Field label="Min relevance score">
                  <input type="number" value={minRelevanceScore} onChange={e => setMinRelevanceScore(Number(e.target.value))} step={0.05} min={0} max={1} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
                </Field>
                <Field label="Min save importance">
                  <input type="number" value={memoryMinImportance} onChange={e => setMemoryMinImportance(Number(e.target.value))} step={0.05} min={0} max={1} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
                </Field>
                <Field label="Dedupe threshold">
                  <input type="number" value={memoryDedupeThreshold} onChange={e => setMemoryDedupeThreshold(Number(e.target.value))} step={0.01} min={0} max={1} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
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
              <input type="number" value={maxSteps} onChange={e => setMaxSteps(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
            <Field label="Max tool calls per run">
              <input type="number" value={maxToolCalls} onChange={e => setMaxToolCalls(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
            <Field label="Max run duration (sec)">
              <input type="number" value={maxDurationSecs} onChange={e => setMaxDurationSecs(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
            <Field label="Max history messages" hint="Verbatim turns kept when compaction is active. Default 20 (no compaction).">
              <input type="number" min={1} max={100} value={maxHistoryMessages} onChange={e => setMaxHistoryMessages(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
            <Field label="Compact after N messages" hint="Older turns are summarised once this many messages accumulate. Default 6.">
              <input type="number" min={2} value={compactionThreshold} onChange={e => setCompactionThreshold(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
            <Field label="Compact after N input tokens" hint="Compacts on first run where input tokens exceed this value. Default 3000.">
              <input type="number" min={500} value={compactionTokenThreshold} onChange={e => setCompactionTokenThreshold(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
          </div>
          <div className="bg-white border border-gray-100 rounded-xl overflow-hidden mt-2">
            <div className="flex items-center justify-between px-4 py-3">
              <div>
                <p className="text-[13px] text-gray-800">Lazy tool loading</p>
                <p className="text-[11px] text-gray-400 mt-0.5">Tools are activated on demand — reduces token cost, adds 1 turn latency on first tool use per run.</p>
              </div>
              <Toggle on={lazyToolLoading} onToggle={() => setLazyToolLoading(v => !v)} />
            </div>
          </div>
          <p className="text-[12px] font-medium text-gray-700 mt-2">Require approval for</p>
          <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
            {['File write operations', 'Outbound HTTP requests', 'Shell / command execution', 'Email / send actions'].map((item, i, arr) => (
              <div key={item} className={`flex items-center justify-between px-4 py-3 ${i < arr.length - 1 ? 'border-b border-gray-50' : ''}`}>
                <p className="text-[13px] text-gray-800">{item}</p>
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
    <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
      <button
        onClick={onToggleCollapse}
        className="w-full flex items-center justify-between px-4 py-2.5 bg-gray-50 hover:bg-gray-100 transition-colors border-b border-gray-100">
        <div className="flex items-center gap-2">
          <span className="text-[12px] font-semibold text-gray-700">{label}</span>
          <span className="text-[10px] text-gray-400">{description}</span>
          {enabledCount > 0 && (
            <span className="text-[10px] font-medium px-1.5 py-0.5 rounded-full bg-purple-100 text-purple-700 leading-none">{enabledCount} on</span>
          )}
        </div>
        <ChevronDown className={`w-3.5 h-3.5 text-gray-400 transition-transform ${collapsed ? '-rotate-90' : ''}`} />
      </button>
      {!collapsed && skills.map((skill, i) => {
        const isEnabled = !!enabledSkills[skill.id]
        const mode = activationModes[skill.id] ?? 'always'
        return (
          <div key={skill.id} className={`flex items-center justify-between gap-3 px-4 py-3 ${i < skills.length - 1 ? 'border-b border-gray-50' : ''}`}>
            <div className="flex items-center gap-2 min-w-0">
              <GripVertical className="w-3.5 h-3.5 text-gray-300 shrink-0" />
              <div className="min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <p className="text-[13px] font-medium text-gray-900">{skill.name}</p>
                  {skill.required_tool_names && skill.required_tool_names.length > 0 && (
                    <span className="text-[10px] text-amber-600">Requires: {skill.required_tool_names.join(', ')}</span>
                  )}
                </div>
                <p className="text-[11px] text-gray-500 truncate">{skill.description}</p>
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
                      ? 'bg-amber-50 border-amber-200 text-amber-700 hover:bg-amber-100'
                      : 'bg-blue-50 border-blue-200 text-blue-700 hover:bg-blue-100'
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
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-[11px] font-medium text-gray-600">{label}</label>
      {hint && <p className="text-[10px] text-gray-400 -mt-1">{hint}</p>}
      {children}
    </div>
  )
}
