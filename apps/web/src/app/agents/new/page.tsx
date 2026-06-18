'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, Save, X, Check } from 'lucide-react'
import { agentsAPI, connectorsAPI, providersAPI, skillsAPI, toolsAPI } from '@/lib/api'
import type { Agent, AgentSkill, Connector, ModelInfo, ProviderCredential, Skill, Tool } from '@/types'

const TABS = ['Basics', 'Model', 'Instructions', 'Skills', 'Tools', 'Context', 'Memory', 'Guardrails'] as const
type Tab = typeof TABS[number]

const PROVIDERS = ['anthropic', 'openai', 'gemini', 'ollama']

const riskBadge = (r: string) => {
  const map: Record<string, string> = {
    low: 'bg-blue-50 text-blue-700', medium: 'bg-amber-50 text-amber-800',
    high: 'bg-orange-50 text-orange-800', critical: 'bg-red-50 text-red-700',
  }
  return `text-[10px] font-medium px-2 py-0.5 rounded-full ${map[r] ?? 'bg-gray-100 text-gray-600'}`
}

function Toggle({ on, onToggle }: { on: boolean; onToggle: () => void }) {
  return (
    <button onClick={onToggle}
      className={`w-8 h-4.5 rounded-full transition-colors relative flex-shrink-0 ${on ? 'bg-purple-600' : 'bg-gray-200'}`}
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
  const [enabledTools, setEnabledTools] = useState<Record<string, boolean>>({ read_file: true })
  const [enabledSkills, setEnabledSkills] = useState<Record<string, boolean>>({})
  const [skillOrder, setSkillOrder] = useState<Record<string, number>>({})
  const [enabledConnectors, setEnabledConnectors] = useState<Record<string, boolean>>({})
  const [maxChunks, setMaxChunks] = useState(8)
  const [minScore, setMinScore] = useState(0.5)
  const [saveError, setSaveError] = useState('')

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
    queryFn: () => agentsAPI.getConnectors(params!.id!) as Promise<{ data: Connector[] }>,
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

  // Tools locked by an enabled skill (cannot be manually removed)
  const skillLockedTools = new Set<string>()
  availableSkills.forEach(s => {
    if (enabledSkills[s.id] && s.required_tool_names) {
      s.required_tool_names.forEach(n => skillLockedTools.add(n))
    }
  })

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
    setMaxSteps(existing.max_steps)
    setMaxToolCalls(existing.max_tool_calls)
    setMaxDurationSecs(existing.max_duration_secs ?? 300)
    setMaxHistoryMessages(existing.max_history_messages ?? 20)
    setLazyToolLoading(existing.lazy_tool_loading ?? false)
  }, [existing])

  useEffect(() => {
    if (agentTools?.data) {
      setEnabledTools(Object.fromEntries(agentTools.data.map((tool) => [tool.name, true])))
    }
  }, [agentTools])

  useEffect(() => {
    if (agentConnectorsData?.data) {
      setEnabledConnectors(Object.fromEntries(agentConnectorsData.data.map((c) => [c.id, true])))
    }
  }, [agentConnectorsData])

  useEffect(() => {
    if (agentSkillsData?.data) {
      setEnabledSkills(Object.fromEntries(agentSkillsData.data.map((s) => [s.skill_id, s.enabled])))
      setSkillOrder(Object.fromEntries(agentSkillsData.data.map((s) => [s.skill_id, s.order_index])))
    }
  }, [agentSkillsData])

  // Auto-select first available model when models load or provider changes
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
      max_steps: maxSteps,
      max_tool_calls: maxToolCalls,
      max_duration_secs: maxDurationSecs,
      max_history_messages: maxHistoryMessages,
      lazy_tool_loading: lazyToolLoading,
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
          .map(([skill_id], i) => ({ skill_id, enabled: true, order_index: skillOrder[skill_id] ?? i })),
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

  return (
    <div className="p-6 max-w-3xl">
      {isEdit && isLoadingAgent && <div className="text-sm text-gray-400 py-8 text-center">Loading agent…</div>}
      {/* Header */}
      <div className="flex items-center justify-between mb-5">
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
      <div className="flex border-b border-gray-100 mb-5 bg-gray-50 rounded-t-lg overflow-hidden">
        {TABS.map(t => (
          <button key={t} onClick={() => setTab(t)}
            className={`px-4 py-2 text-[12px] transition-colors whitespace-nowrap
              ${tab === t ? 'bg-white text-purple-600 font-medium border-b-2 border-purple-500' : 'text-gray-500 hover:text-gray-700'}`}>
            {t}
          </button>
        ))}
      </div>

      {/* ── BASICS ── */}
      {tab === 'Basics' && (
        <div className="space-y-4">
          {saveError && (
            <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{saveError}</div>
          )}
          <div className="grid grid-cols-2 gap-4">
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
          <div className="grid grid-cols-2 gap-4">
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
          <div className="grid grid-cols-2 gap-4">
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
        <div>
          <p className="text-[12px] text-gray-500 mb-3">Attach reusable prompt instructions. They are injected after the agent system prompt.</p>
          <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
            {availableSkills.map((skill, i) => (
              <div key={skill.id} className={`flex items-center justify-between gap-4 px-4 py-3 ${i < availableSkills.length - 1 ? 'border-b border-gray-50' : ''}`}>
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <p className="text-[13px] font-medium text-gray-900">{skill.name}</p>
                    <span className={`text-[10px] font-medium px-2 py-0.5 rounded-full ${skill.source === 'managed' ? 'bg-purple-50 text-purple-700' : 'bg-blue-50 text-blue-700'}`}>{skill.source}</span>
                  </div>
                  <p className="text-[11px] text-gray-500 truncate">{skill.description}</p>
                  {skill.required_tool_names && skill.required_tool_names.length > 0 && (
                    <p className="text-[10px] text-amber-600 mt-0.5">Requires: {skill.required_tool_names.join(', ')}</p>
                  )}
                </div>
                <div className="flex items-center gap-3 shrink-0">
                  <input
                    type="number"
                    value={skillOrder[skill.id] ?? i}
                    onChange={(e) => setSkillOrder((prev) => ({ ...prev, [skill.id]: Number(e.target.value) }))}
                    className="w-16 text-[12px] px-2 py-1 border border-gray-200 rounded"
                    title="Order"
                  />
                  <Toggle on={!!enabledSkills[skill.id]}
                    onToggle={() => {
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
                          // Collect tools still required by other enabled skills
                          const stillRequired = new Set<string>()
                          availableSkills.forEach(s => {
                            if (s.id !== skill.id && enabledSkills[s.id] && s.required_tool_names) {
                              s.required_tool_names.forEach(n => stillRequired.add(n))
                            }
                          })
                          setEnabledTools(prev => {
                            const next = { ...prev }
                            skill.required_tool_names!.forEach(n => {
                              if (!stillRequired.has(n)) delete next[n]
                            })
                            return next
                          })
                        }
                      }
                    }} />
                </div>
              </div>
            ))}
            {availableSkills.length === 0 && <div className="p-8 text-center text-[12px] text-gray-400">No skills available.</div>}
          </div>
        </div>
      )}

      {/* ── TOOLS ── */}
      {tab === 'Tools' && (
        <div>
          <p className="text-[12px] text-gray-500 mb-3">Enable tools this agent can use. High-risk tools require approval by default.</p>
          <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
            {availableTools.map((tool, i) => {
              const locked = skillLockedTools.has(tool.name)
              return (
                <div key={tool.id} className={`flex items-center justify-between px-4 py-3 ${i < availableTools.length - 1 ? 'border-b border-gray-50' : ''}`}>
                  <div>
                    <p className="text-[13px] font-medium text-gray-900">{tool.name}</p>
                    <p className="text-[11px] text-gray-500">{tool.description}</p>
                    {locked && <p className="text-[10px] text-purple-600 mt-0.5">enabled by skill</p>}
                  </div>
                  <div className="flex items-center gap-3">
                    <span className={riskBadge(tool.risk_level)}>{tool.risk_level} risk</span>
                    {tool.requires_approval && (
                      <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-purple-50 text-purple-700">approval req.</span>
                    )}
                    <Toggle on={!!enabledTools[tool.name] || locked}
                      onToggle={() => { if (!locked) setEnabledTools(prev => ({ ...prev, [tool.name]: !prev[tool.name] })) }} />
                  </div>
                </div>
              )
            })}
            {availableTools.length === 0 && <div className="p-8 text-center text-[12px] text-gray-400">No tools registered.</div>}
          </div>
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
              <div className="grid grid-cols-2 gap-4">
                <Field label="Max chunks per run">
                  <input type="number" defaultValue={8} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
                </Field>
                <Field label="Min relevance score">
                  <input type="number" defaultValue={0.75} step={0.05} min={0} max={1} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
                </Field>
              </div>
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
              <div className="grid grid-cols-2 gap-4">
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
              <div className="grid grid-cols-2 gap-4">
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
          <div className="grid grid-cols-2 gap-4">
            <Field label="Max steps">
              <input type="number" value={maxSteps} onChange={e => setMaxSteps(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
            <Field label="Max tool calls per run">
              <input type="number" value={maxToolCalls} onChange={e => setMaxToolCalls(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
            <Field label="Max run duration (sec)">
              <input type="number" value={maxDurationSecs} onChange={e => setMaxDurationSecs(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
            </Field>
            <Field label="Max history messages" hint="Older turns are dropped. Lower = fewer input tokens. Default 20.">
              <input type="number" min={1} max={100} value={maxHistoryMessages} onChange={e => setMaxHistoryMessages(Number(e.target.value))} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white focus:outline-none" />
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

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-[11px] font-medium text-gray-600">{label}</label>
      {hint && <p className="text-[10px] text-gray-400 -mt-1">{hint}</p>}
      {children}
    </div>
  )
}
