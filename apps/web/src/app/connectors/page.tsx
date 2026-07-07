'use client'

import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BookOpen, ChevronRight, Database, ExternalLink, FileText, FolderOpen, Github,
  HardDrive, MessageSquare, Plus, RefreshCw, Trash2, Trello, X,
} from 'lucide-react'
import { connectorsAPI, filesystemAPI } from '@/lib/api'
import { relativeTime, statusColor } from '@/lib/utils'
import { useDemoMode } from '@/context/demo-mode'
import type { Connector, ConnectorBrowseResult, ConnectorDocument, ConnectorDocumentGroup, ConnectorSyncJob } from '@/types'

// ─── provider definitions ─────────────────────────────────────────────────────

const PROVIDERS = [
  { id: 'filesystem', label: 'Filesystem',   Icon: FolderOpen,    description: 'Index local files and directories',  available: true },
  { id: 'github',     label: 'GitHub',       Icon: Github,        description: 'Index repository files and code',    available: true },
  { id: 'confluence', label: 'Confluence',   Icon: BookOpen,      description: 'Index pages and spaces',             available: true },
  { id: 'slack',      label: 'Slack',        Icon: MessageSquare, description: 'Index messages and channels',        available: false },
  { id: 'jira',       label: 'Jira',         Icon: Trello,        description: 'Index issues and comments',          available: false },
  { id: 'gdrive',     label: 'Google Drive', Icon: HardDrive,     description: 'Index documents and sheets',         available: false },
]

// ─── folder browser modal ─────────────────────────────────────────────────────

function FolderBrowser({ onSelect, onClose }: { onSelect: (path: string) => void; onClose: () => void }) {
  const [path, setPath] = useState('/')

  const { data, isLoading, error } = useQuery({
    queryKey: ['filesystem-browse', path],
    queryFn: () => filesystemAPI.browse(path),
  })

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
      <div className="bg-surface rounded-xl shadow-xl w-full max-w-md">
        <div className="flex items-center justify-between px-4 py-3 border-b border-border">
          <p className="text-sm font-medium text-foreground">Browse filesystem</p>
          <button onClick={onClose}><X size={15} className="text-faint" /></button>
        </div>
        <div className="px-4 py-2 bg-muted border-b border-border">
          <p className="text-[11px] font-mono text-muted-foreground truncate">{path}</p>
        </div>
        <div className="overflow-y-auto max-h-72">
          {isLoading && <div className="py-8 text-center text-sm text-faint">Loading…</div>}
          {error && <div className="py-4 px-4 text-[12px] text-red-600 dark:text-red-300">Cannot read directory.</div>}
          {data && (
            <>
              {data.parent !== '' && (
                <button
                  onClick={() => setPath(data.parent)}
                  className="w-full flex items-center gap-2 px-4 py-2.5 text-[12px] text-muted-foreground hover:bg-muted border-b border-gray-50"
                >
                  <FolderOpen size={13} className="text-faint" />
                  <span>.. (parent directory)</span>
                </button>
              )}
              {data.entries.length === 0 && (
                <div className="py-6 text-center text-[12px] text-faint">Empty directory</div>
              )}
              {data.entries.map((entry) => (
                <button
                  key={entry.path}
                  onClick={() => { if (entry.is_dir) setPath(entry.path) }}
                  disabled={!entry.is_dir}
                  className={`w-full flex items-center gap-2 px-4 py-2.5 text-[12px] border-b border-gray-50 last:border-b-0 ${
                    entry.is_dir
                      ? 'text-foreground hover:bg-purple-50 cursor-pointer'
                      : 'text-faint cursor-default'
                  }`}
                >
                  <FolderOpen size={13} className={entry.is_dir ? 'text-amber-400' : 'text-faint'} />
                  <span className="text-left truncate flex-1">{entry.name}</span>
                  {entry.is_dir && <ChevronRight size={11} className="text-faint flex-shrink-0" />}
                </button>
              ))}
            </>
          )}
        </div>
        <div className="flex items-center gap-2 px-4 py-3 border-t border-border">
          <button onClick={onClose} className="px-3 py-1.5 text-[12px] text-muted-foreground hover:text-gray-800 dark:hover:text-gray-200">
            Cancel
          </button>
          <button
            onClick={() => { onSelect(path); onClose() }}
            className="ml-auto px-4 py-1.5 bg-accent text-white text-[12px] rounded-lg font-medium"
          >
            Select this folder
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── create panel ─────────────────────────────────────────────────────────────

function CreatePanel({
  onClose,
  onCreate,
  isPending,
}: {
  onClose: () => void
  onCreate: (body: unknown) => void
  isPending: boolean
}) {
  const [provider, setProvider] = useState<string | null>(null)
  const [form, setForm] = useState({ name: '', path: '', token: '', owner: '', repo: '', branch: '', url: '', username: '', spaceKeys: '' })
  const [formError, setFormError] = useState('')
  const [browserOpen, setBrowserOpen] = useState(false)

  const handleSubmit = () => {
    if (!form.name.trim()) { setFormError('Connector name is required'); return }
    if (provider === 'filesystem') {
      if (!form.path.trim()) { setFormError('Directory path is required'); return }
      if (!form.path.startsWith('/')) { setFormError('Path must be absolute and start with /'); return }
      setFormError('')
      onCreate({ name: form.name.trim(), provider: 'filesystem', type: 'native', auth_type: 'none', config: { path: form.path.trim() } })
    } else if (provider === 'github') {
      if (!form.token.trim()) { setFormError('Personal access token is required'); return }
      setFormError('')
      const cfg: Record<string, string> = { token: form.token.trim() }
      if (form.owner.trim()) cfg.owner = form.owner.trim()
      if (form.repo.trim()) cfg.repo = form.repo.trim()
      if (form.branch.trim()) cfg.branch = form.branch.trim()
      onCreate({ name: form.name.trim(), provider: 'github', type: 'native', auth_type: 'api_key', config: cfg })
    } else if (provider === 'confluence') {
      if (!form.url.trim()) { setFormError('Confluence URL is required'); return }
      if (!form.username.trim()) { setFormError('Username (email) is required'); return }
      if (!form.token.trim()) { setFormError('API token is required'); return }
      setFormError('')
      onCreate({ name: form.name.trim(), provider: 'confluence', type: 'native', auth_type: 'api_key', config: { url: form.url.trim(), username: form.username.trim(), token: form.token.trim(), space_keys: form.spaceKeys.trim() } })
    }
  }

  return (
    <div className="border border-border-strong rounded-xl mb-6 overflow-hidden bg-surface">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-muted">
        <p className="text-sm font-medium text-foreground">Connect a data source</p>
        <button onClick={onClose}><X size={14} className="text-faint" /></button>
      </div>

      <div className="p-4">
        <p className="text-[11px] font-medium text-faint uppercase tracking-wide mb-3">Choose a source type</p>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 mb-5">
          {PROVIDERS.map(({ id, label, Icon, description, available }) => {
            const active = provider === id
            return (
              <button
                key={id}
                disabled={!available}
                onClick={() => available && setProvider(id)}
                className={`relative flex flex-col items-start gap-1.5 p-3 rounded-xl border text-left transition-all ${
                  active
                    ? 'border-purple-300 bg-accent/10'
                    : available
                    ? 'border-border-strong hover:border-gray-300 bg-surface'
                    : 'border-border bg-muted opacity-50 cursor-not-allowed'
                }`}
              >
                <Icon size={15} className={active ? 'text-accent dark:text-accent-bright' : available ? 'text-muted-foreground' : 'text-faint'} />
                <span className={`text-[12px] font-medium ${active ? 'text-accent dark:text-accent-bright' : 'text-foreground'}`}>{label}</span>
                <span className="text-[10px] text-faint leading-tight">{description}</span>
                {!available && (
                  <span className="absolute top-2 right-2 text-[9px] px-1.5 py-0.5 bg-gray-200 text-muted-foreground rounded-full">v0.2</span>
                )}
              </button>
            )
          })}
        </div>

        {provider === 'filesystem' && (
          <div className="border-t border-border pt-4">
            <p className="text-[11px] font-medium text-faint uppercase tracking-wide mb-3">Configure filesystem connector</p>
            {formError && (
              <p className="text-[12px] text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-100 rounded px-3 py-2 mb-3">{formError}</p>
            )}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Connector name *</label>
                <input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="e.g. Engineering Wiki"
                  className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg"
                />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Directory path *</label>
                <div className="flex gap-1.5">
                  <input
                    value={form.path}
                    onChange={(e) => setForm({ ...form, path: e.target.value })}
                    placeholder="/home/user/documents"
                    className="flex-1 text-sm px-3 py-2 border border-border-strong rounded-lg font-mono min-w-0"
                  />
                  <button
                    type="button"
                    onClick={() => setBrowserOpen(true)}
                    className="px-3 py-2 border border-border-strong rounded-lg text-[11px] text-muted-foreground hover:bg-muted flex-shrink-0"
                  >
                    Browse
                  </button>
                </div>
              </div>
            </div>
            <p className="text-[11px] text-faint mb-4">
              Agent Nexus will recursively index all files in this directory. Run a sync after connecting to start indexing.
            </p>
            <div className="flex gap-2">
              <button
                onClick={handleSubmit}
                disabled={isPending}
                className="px-4 py-1.5 bg-accent text-white text-[12px] rounded-lg font-medium disabled:opacity-50"
              >
                {isPending ? 'Connecting…' : 'Connect'}
              </button>
              <button onClick={onClose} className="px-3 py-1.5 text-[12px] text-muted-foreground hover:text-gray-800 dark:hover:text-gray-200">
                Cancel
              </button>
            </div>
          </div>
        )}

        {provider === 'github' && (
          <div className="border-t border-border pt-4">
            <p className="text-[11px] font-medium text-faint uppercase tracking-wide mb-3">Configure GitHub connector</p>
            {formError && (
              <p className="text-[12px] text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-100 rounded px-3 py-2 mb-3">{formError}</p>
            )}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Connector name *</label>
                <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="e.g. agent-nexus repo" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Personal access token *</label>
                <input type="password" value={form.token} onChange={(e) => setForm({ ...form, token: e.target.value })} placeholder="ghp_..." className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg font-mono" />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Owner <span className="text-faint">(optional — leave blank to use token owner)</span></label>
                <input value={form.owner} onChange={(e) => setForm({ ...form, owner: e.target.value })} placeholder="org or username" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Repository <span className="text-faint">(optional — leave blank to index all repos)</span></label>
                <input value={form.repo} onChange={(e) => setForm({ ...form, repo: e.target.value })} placeholder="repo-name" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
              </div>
              <div className="col-span-2">
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Branch <span className="text-faint">(optional — defaults to repo default)</span></label>
                <input value={form.branch} onChange={(e) => setForm({ ...form, branch: e.target.value })} placeholder="main" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
              </div>
            </div>
            <p className="text-[11px] text-faint mb-4">Indexes all text files in the repository. Requires a PAT with <code className="bg-muted px-1 rounded">repo</code> scope (read-only).</p>
            <div className="flex gap-2">
              <button onClick={handleSubmit} disabled={isPending} className="px-4 py-1.5 bg-accent text-white text-[12px] rounded-lg font-medium disabled:opacity-50">{isPending ? 'Connecting…' : 'Connect'}</button>
              <button onClick={onClose} className="px-3 py-1.5 text-[12px] text-muted-foreground hover:text-gray-800 dark:hover:text-gray-200">Cancel</button>
            </div>
          </div>
        )}

        {provider === 'confluence' && (
          <div className="border-t border-border pt-4">
            <p className="text-[11px] font-medium text-faint uppercase tracking-wide mb-3">Configure Confluence connector</p>
            {formError && (
              <p className="text-[12px] text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-100 rounded px-3 py-2 mb-3">{formError}</p>
            )}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Connector name *</label>
                <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="e.g. Engineering Confluence" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Confluence URL *</label>
                <input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="https://yourorg.atlassian.net" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Username (email) *</label>
                <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} placeholder="you@company.com" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">API token *</label>
                <input type="password" value={form.token} onChange={(e) => setForm({ ...form, token: e.target.value })} placeholder="Atlassian API token" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
              </div>
              <div className="col-span-2">
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Space keys <span className="text-faint">(optional — leave blank for all spaces)</span></label>
                <input value={form.spaceKeys} onChange={(e) => setForm({ ...form, spaceKeys: e.target.value })} placeholder="ENG,OPS,HR" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg font-mono" />
              </div>
            </div>
            <p className="text-[11px] text-faint mb-4">Indexes pages from your Confluence Cloud space(s). Create an API token at <span className="text-accent dark:text-accent-bright">id.atlassian.com</span>.</p>
            <div className="flex gap-2">
              <button onClick={handleSubmit} disabled={isPending} className="px-4 py-1.5 bg-accent text-white text-[12px] rounded-lg font-medium disabled:opacity-50">{isPending ? 'Connecting…' : 'Connect'}</button>
              <button onClick={onClose} className="px-3 py-1.5 text-[12px] text-muted-foreground hover:text-gray-800 dark:hover:text-gray-200">Cancel</button>
            </div>
          </div>
        )}
      </div>

      {browserOpen && (
        <FolderBrowser
          onSelect={(path) => setForm((f) => ({ ...f, path }))}
          onClose={() => setBrowserOpen(false)}
        />
      )}
    </div>
  )
}

// ─── page ─────────────────────────────────────────────────────────────────────

export default function ConnectorsPage() {
  const queryClient = useQueryClient()
  const demoMode = useDemoMode()
  const [showCreate, setShowCreate] = useState(false)
  const [selected, setSelected] = useState('')
  const [detailTab, setDetailTab] = useState<'documents' | 'sync-history'>('documents')
  const [actionError, setActionError] = useState('')
  const [docsPage, setDocsPage] = useState(1)
  const [selectedGroup, setSelectedGroup] = useState<string | null>(null)
  const [browsePath, setBrowsePath] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [searchQuery, setSearchQuery] = useState('')

  // Debounce search input by 300ms.
  useEffect(() => {
    const t = setTimeout(() => setSearchQuery(searchInput), 300)
    return () => clearTimeout(t)
  }, [searchInput])

  // Clear search when navigating to a different level.
  useEffect(() => {
    setSearchInput('')
    setSearchQuery('')
  }, [browsePath])

  const { data, isLoading, error } = useQuery({
    queryKey: ['connectors'],
    queryFn: () => connectorsAPI.list() as Promise<{ data: Connector[] }>,
    refetchInterval: (query) => {
      const connectors = (query.state.data as { data: Connector[] } | undefined)?.data ?? []
      return connectors.some((c) => c.status === 'syncing') ? 3000 : false
    },
  })
  const connectors = data?.data ?? []
  const selectedConnector = connectors.find((c) => c.id === selected)
  const isSyncing = selectedConnector?.status === 'syncing'

  const { data: docsData, isLoading: docsLoading } = useQuery({
    queryKey: ['connector-docs', selected, docsPage, selectedGroup],
    queryFn: () => connectorsAPI.documents(selected, docsPage, selectedGroup ?? undefined) as Promise<{ data: ConnectorDocument[]; total: number; page: number; page_size: number; total_pages: number }>,
    enabled: !!selected,
  })
  const { data: jobsData } = useQuery({
    queryKey: ['connector-jobs', selected],
    queryFn: () => connectorsAPI.syncJobs(selected) as Promise<{ data: ConnectorSyncJob[] }>,
    enabled: !!selected,
    refetchInterval: isSyncing ? 2000 : false,
  })
  const { data: groupsData } = useQuery({
    queryKey: ['connector-doc-groups', selected],
    queryFn: () => connectorsAPI.documentGroups(selected) as Promise<{ data: ConnectorDocumentGroup[] }>,
    enabled: !!selected && (selectedConnector?.provider === 'github' || selectedConnector?.provider === 'confluence'),
    refetchInterval: isSyncing ? 3000 : false,
  })
  const { data: browseData, isLoading: browseLoading } = useQuery({
    queryKey: ['connector-browse', selected, browsePath, searchQuery],
    queryFn: () => connectorsAPI.browse(selected, browsePath, searchQuery) as Promise<ConnectorBrowseResult>,
    enabled: !!selected && (selectedConnector?.provider === 'github' || selectedConnector?.provider === 'confluence'),
    refetchInterval: isSyncing ? 3000 : false,
  })

  const create = useMutation({
    mutationFn: (body: unknown) => connectorsAPI.create(body),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['connectors'] }); setShowCreate(false); setActionError('') },
    onError: (err: Error) => setActionError(err.message),
  })
  const sync = useMutation({
    mutationFn: (id: string) => connectorsAPI.sync(id),
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: ['connectors'] })
      queryClient.invalidateQueries({ queryKey: ['connector-jobs', id] })
      setDetailTab('sync-history')
    },
    onError: (err: Error) => setActionError(err.message),
  })
  const remove = useMutation({
    mutationFn: (id: string) => connectorsAPI.delete(id),
    onSuccess: () => { setSelected(''); queryClient.invalidateQueries({ queryKey: ['connectors'] }) },
    onError: (err: Error) => setActionError(err.message),
  })

  const docGroups = groupsData?.data ?? []
  const docs = docsData?.data ?? []
  const docsTotalPages = docsData?.total_pages ?? 1
  const docsTotal = docsData?.total ?? 0
  const jobs = jobsData?.data ?? []
  const activeJob = jobs.find((j) => j.status === 'running')

  return (
    <div className="p-4 sm:p-6 max-w-4xl">
      {/* Header */}
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div>
          <h1 className="text-xl font-semibold text-foreground">Connectors</h1>
          <p className="text-[12px] text-faint mt-0.5">
            Connect data sources so agents can retrieve relevant context when they run.
          </p>
        </div>
        {connectors.length > 0 && !showCreate && (
          <button
            onClick={() => { setShowCreate(true); setActionError('') }}
            disabled={demoMode}
            title={demoMode ? 'Not available in demo mode' : undefined}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-accent text-white text-[12px] rounded-lg disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Plus size={13} /> Connect source
          </button>
        )}
      </div>

      {(error || actionError) && (
        <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3 mb-4">
          {actionError || (error as Error).message}
        </div>
      )}

      {showCreate && (
        <CreatePanel
          onClose={() => setShowCreate(false)}
          onCreate={(body) => create.mutate(body)}
          isPending={create.isPending}
        />
      )}

      {isLoading && <div className="py-12 text-center text-sm text-faint">Loading…</div>}

      {/* Empty state */}
      {!isLoading && !error && connectors.length === 0 && !showCreate && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-8 border border-dashed border-border-strong rounded-xl p-8">
          <div>
            <Database size={22} className="text-purple-300 mb-3" />
            <p className="text-sm font-medium text-foreground mb-1">What are connectors?</p>
            <p className="text-[12px] text-muted-foreground leading-relaxed mb-5">
              Connectors index documents from external sources so agents can retrieve relevant information at runtime — grounding responses in your actual data, not just model training.
            </p>
            <div className="space-y-3">
              {[
                { n: '1', title: 'Connect a source', desc: 'Point to a directory or integration' },
                { n: '2', title: 'Sync documents',   desc: 'Agent Nexus indexes and embeds the content' },
                { n: '3', title: 'Agents use context', desc: 'Relevant chunks are injected at run time' },
              ].map((s) => (
                <div key={s.n} className="flex items-start gap-2.5">
                  <span className="w-5 h-5 rounded-full bg-purple-100 text-accent dark:text-accent-bright text-[10px] font-bold flex items-center justify-center flex-shrink-0 mt-0.5">
                    {s.n}
                  </span>
                  <div>
                    <p className="text-[12px] font-medium text-foreground">{s.title}</p>
                    <p className="text-[11px] text-faint">{s.desc}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
          <div className="flex flex-col items-center justify-center sm:border-l border-t sm:border-t-0 border-dashed border-border-strong pt-8 sm:pt-0 sm:pl-8">
            <FolderOpen size={28} className="text-faint mb-3" />
            <p className="text-sm font-medium text-muted-foreground mb-1">Connect your first source</p>
            <p className="text-[11px] text-faint mb-5 text-center">
              Start with a local directory — index your docs, codebase, or any file collection.
            </p>
            <button
              onClick={() => { setShowCreate(true); setActionError('') }}
              disabled={demoMode}
              title={demoMode ? 'Not available in demo mode' : undefined}
              className="px-4 py-2 bg-accent text-white text-[12px] rounded-lg font-medium disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {demoMode ? 'Not available in demo' : 'Connect source'}
            </button>
          </div>
        </div>
      )}

      {/* Connector cards */}
      {connectors.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 mb-6">
          {connectors.map((connector) => {
            const cfg = connector.config as { path?: string; owner?: string; repo?: string; url?: string; space_keys?: string } | undefined
            const isSyncing = sync.isPending && (sync.variables as string) === connector.id
            const providerMeta = PROVIDERS.find((p) => p.id === connector.provider)
            const ProviderIcon = providerMeta?.Icon ?? FolderOpen
            const providerLabel = providerMeta?.label ?? connector.provider
            const subtitle = connector.provider === 'github' && cfg?.owner && cfg?.repo
              ? `${cfg.owner}/${cfg.repo}`
              : connector.provider === 'confluence' && cfg?.url
              ? cfg.url.replace(/^https?:\/\//, '') + (cfg.space_keys ? ` · ${cfg.space_keys}` : '')
              : cfg?.path ?? ''
            return (
              <div
                key={connector.id}
                onClick={() => { setSelected(connector.id === selected ? '' : connector.id); setDetailTab('documents'); setDocsPage(1); setSelectedGroup(null); setBrowsePath(''); setSearchInput(''); setSearchQuery('') }}
                className={`bg-surface border rounded-xl p-3.5 cursor-pointer transition-all ${
                  selected === connector.id
                    ? 'border-purple-300 ring-1 ring-accent/20'
                    : 'border-border hover:border-gray-200'
                }`}
              >
                <div className="flex items-start justify-between mb-2">
                  <div className="flex items-center gap-1.5 min-w-0">
                    <ProviderIcon size={13} className="text-muted-foreground flex-shrink-0" />
                    <p className="text-[13px] font-medium text-foreground truncate">{connector.name}</p>
                  </div>
                  <span className={`text-[10px] px-2 py-0.5 rounded-full flex-shrink-0 ml-1 ${statusColor(connector.status)}`}>
                    {connector.status}
                  </span>
                </div>
                {subtitle && (
                  <p className="text-[10px] font-mono text-faint truncate mb-2" title={subtitle}>{subtitle}</p>
                )}
                <p className="text-[10px] text-faint mb-3">{providerLabel}</p>
                <div className="flex gap-1.5" onClick={(e) => e.stopPropagation()}>
                  <button
                    onClick={() => sync.mutate(connector.id)}
                    disabled={isSyncing}
                    className="inline-flex items-center gap-1 px-2 py-1 border border-border-strong text-[11px] text-muted-foreground rounded-md hover:bg-muted disabled:opacity-50"
                  >
                    <RefreshCw size={11} className={isSyncing ? 'animate-spin' : ''} />
                    {isSyncing ? 'Syncing…' : 'Sync'}
                  </button>
                  <button
                    onClick={() => { if (confirm('Delete this connector and all indexed documents?')) remove.mutate(connector.id) }}
                    className="ml-auto p-1 text-faint hover:text-red-500"
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* Detail panel */}
      {selectedConnector && (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          <div className="flex items-center border-b border-border px-1 overflow-x-auto">
            {(['documents', 'sync-history'] as const).map((tab) => (
              <button
                key={tab}
                onClick={() => setDetailTab(tab)}
                className={`px-4 py-2.5 text-[12px] font-medium border-b-2 -mb-px transition-colors ${
                  detailTab === tab
                    ? 'border-purple-600 text-accent dark:text-accent-bright'
                    : 'border-transparent text-muted-foreground hover:text-gray-700 dark:hover:text-gray-300'
                }`}
              >
                {tab === 'documents' ? 'Documents' : 'Sync history'}
              </button>
            ))}
          </div>

          {/* Live progress banner shown while sync is running */}
          {isSyncing && activeJob && (
            <div className="flex items-center gap-3 px-4 py-2.5 bg-blue-50 dark:bg-blue-500/10 border-b border-blue-100">
              <RefreshCw size={12} className="animate-spin text-blue-500 flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-[11px] font-medium text-blue-700 dark:text-blue-300">Syncing…</span>
                  <span className="text-[11px] text-blue-600 dark:text-blue-300">
                    {activeJob.documents_indexed} indexed / {activeJob.documents_found} found
                  </span>
                </div>
                {activeJob.documents_found > 0 && (
                  <div className="h-1 bg-blue-100 rounded-full overflow-hidden">
                    <div
                      className="h-full bg-blue-500 rounded-full transition-all duration-500"
                      style={{ width: `${Math.round((activeJob.documents_indexed / activeJob.documents_found) * 100)}%` }}
                    />
                  </div>
                )}
              </div>
            </div>
          )}

          {detailTab === 'documents' && (selectedConnector?.provider === 'github' || selectedConnector?.provider === 'confluence') && (
            <div>
              {/* Breadcrumb + search */}
              <div className="flex flex-wrap items-center gap-1 px-4 py-2 border-b border-gray-50 bg-gray-50/50">
                <button
                  onClick={() => setBrowsePath('')}
                  className={`text-[12px] flex-shrink-0 ${browsePath === '' ? 'text-foreground font-medium' : 'text-accent dark:text-accent-bright hover:underline'}`}
                >
                  All
                </button>
                {(browseData?.breadcrumb ?? []).map((crumb, i) => {
                  const isLast = i === (browseData?.breadcrumb.length ?? 0) - 1
                  return (
                    <span key={crumb.path} className="flex items-center gap-1 flex-shrink-0">
                      <ChevronRight size={11} className="text-faint" />
                      <button
                        onClick={() => setBrowsePath(crumb.path)}
                        className={`text-[12px] ${isLast ? 'text-foreground font-medium' : 'text-accent dark:text-accent-bright hover:underline'}`}
                      >
                        {crumb.name}
                      </button>
                    </span>
                  )
                })}
                <div className="ml-auto flex items-center gap-1.5 flex-shrink-0">
                  {searchInput && (
                    <button onClick={() => setSearchInput('')} className="text-faint hover:text-gray-500">
                      <X size={12} />
                    </button>
                  )}
                  <input
                    value={searchInput}
                    onChange={(e) => setSearchInput(e.target.value)}
                    placeholder="Search…"
                    className="text-[12px] px-2.5 py-1 border border-border-strong rounded-lg w-36 focus:outline-none focus:border-purple-300 bg-surface"
                  />
                </div>
              </div>
              {searchQuery && !browseLoading && (
                <div className="px-4 py-1.5 border-b border-gray-50 bg-purple-50/50">
                  <span className="text-[11px] text-accent dark:text-accent-bright">
                    {(browseData?.dirs.length ?? 0) + (browseData?.files.length ?? 0)} result{((browseData?.dirs.length ?? 0) + (browseData?.files.length ?? 0)) !== 1 ? 's' : ''} for &ldquo;{searchQuery}&rdquo;
                  </span>
                </div>
              )}

              {/* Browser entries */}
              {browseLoading && <div className="py-8 text-center text-sm text-faint">Loading…</div>}
              {!browseLoading && browseData && browseData.dirs.length === 0 && browseData.files.length === 0 && (
                <div className="py-10 text-center">
                  <FileText size={20} className="text-faint mx-auto mb-2" />
                  <p className="text-[12px] text-faint">No documents indexed yet.</p>
                  <p className="text-[11px] text-faint mt-1">Click Sync on the connector card to start indexing.</p>
                </div>
              )}
              {!browseLoading && browseData && (browseData.dirs.length > 0 || browseData.files.length > 0) && (
                <div>
                  {browseData.dirs.map((dir) => (
                    <button
                      key={dir.path}
                      onClick={() => setBrowsePath(dir.path)}
                      className="w-full flex items-center gap-3 px-4 py-2.5 border-b border-gray-50 hover:bg-amber-50/40 text-left group transition-colors"
                    >
                      <FolderOpen size={14} className="text-amber-400 flex-shrink-0" />
                      <span className="text-[12px] font-medium text-foreground flex-1 truncate group-hover:text-amber-700">{dir.label}</span>
                      {(dir.count ?? 0) > 0 && (
                        <span className="text-[11px] text-faint flex-shrink-0">{dir.count!.toLocaleString()} files</span>
                      )}
                      <ChevronRight size={12} className="text-faint flex-shrink-0 group-hover:text-amber-400" />
                    </button>
                  ))}
                  {browseData.files.map((file) => (
                    <div key={file.id} className="flex items-center gap-3 px-4 py-2.5 border-b last:border-b-0 border-gray-50 hover:bg-gray-50/60 group">
                      <FileText size={13} className="text-faint flex-shrink-0" />
                      <span className="text-[12px] text-foreground flex-1 truncate min-w-0" title={file.source_document_id}>
                        {file.title || file.source_document_id}
                      </span>
                      {file.indexed_at && (
                        <span className="text-[11px] text-faint flex-shrink-0">{relativeTime(file.indexed_at)}</span>
                      )}
                      {file.url && (
                        <a
                          href={file.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          onClick={(e) => e.stopPropagation()}
                          className="text-faint hover:text-purple-500 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
                        >
                          <ExternalLink size={12} />
                        </a>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {detailTab === 'documents' && selectedConnector?.provider !== 'github' && selectedConnector?.provider !== 'confluence' && (
            <div>
              {docsLoading && <div className="py-8 text-center text-sm text-faint">Loading documents…</div>}
              {!docsLoading && docs.length === 0 && (
                <div className="py-10 text-center">
                  <FileText size={20} className="text-faint mx-auto mb-2" />
                  <p className="text-[12px] text-faint">No documents indexed yet.</p>
                  <p className="text-[11px] text-faint mt-1">Click Sync on the connector card to start indexing.</p>
                </div>
              )}
              {docs.length > 0 && (
                <>
                  <div className="overflow-x-auto"><table className="w-full min-w-[480px]">
                    <thead>
                      <tr className="border-b border-gray-50">
                        <th className="text-left text-[10px] font-medium text-faint px-4 py-2">Title</th>
                        <th className="text-left text-[10px] font-medium text-faint px-4 py-2">Source</th>
                        <th className="text-left text-[10px] font-medium text-faint px-4 py-2">Indexed</th>
                      </tr>
                    </thead>
                    <tbody>
                      {docs.map((doc) => (
                        <tr key={doc.id} className="border-b last:border-b-0 border-gray-50 hover:bg-gray-50/50">
                          <td className="px-4 py-2.5">
                            {doc.url ? (
                              <a href={doc.url} target="_blank" rel="noopener noreferrer" className="text-[12px] font-medium text-accent dark:text-accent-bright hover:underline truncate block max-w-xs">{doc.title || '(untitled)'}</a>
                            ) : (
                              <p className="text-[12px] font-medium text-foreground truncate max-w-xs">{doc.title || '(untitled)'}</p>
                            )}
                          </td>
                          <td className="px-4 py-2.5">
                            <p className="text-[11px] font-mono text-faint truncate max-w-xs">{doc.source}</p>
                          </td>
                          <td className="px-4 py-2.5">
                            <p className="text-[11px] text-faint">{doc.indexed_at ? relativeTime(doc.indexed_at) : 'not indexed'}</p>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table></div>
                  {docsTotalPages > 1 && (
                    <div className="flex items-center justify-between px-4 py-2.5 border-t border-gray-50">
                      <p className="text-[11px] text-faint">{docsTotal} documents total</p>
                      <div className="flex items-center gap-1">
                        <button onClick={() => setDocsPage((p) => Math.max(1, p - 1))} disabled={docsPage <= 1} className="px-2 py-1 text-[11px] border border-border-strong rounded disabled:opacity-40 hover:bg-muted">←</button>
                        <span className="text-[11px] text-muted-foreground px-2">{docsPage} / {docsTotalPages}</span>
                        <button onClick={() => setDocsPage((p) => Math.min(docsTotalPages, p + 1))} disabled={docsPage >= docsTotalPages} className="px-2 py-1 text-[11px] border border-border-strong rounded disabled:opacity-40 hover:bg-muted">→</button>
                      </div>
                    </div>
                  )}
                </>
              )}
            </div>
          )}

          {detailTab === 'sync-history' && (
            <div>
              {jobs.length === 0 && (
                <div className="py-10 text-center">
                  <p className="text-[12px] text-faint">No sync jobs recorded.</p>
                  <p className="text-[11px] text-faint mt-1">Use the Sync button to trigger a sync.</p>
                </div>
              )}
              {jobs.map((job) => (
                <div key={job.id} className="px-4 py-3 border-b last:border-b-0 border-gray-50">
                  <div className="flex items-center gap-3">
                    {job.status === 'running'
                      ? <RefreshCw size={11} className="animate-spin text-blue-500 flex-shrink-0" />
                      : null}
                    <span className={`text-[10px] px-2 py-0.5 rounded-full flex-shrink-0 ${
                      job.status === 'completed' ? statusColor('connected')
                      : job.status === 'running' ? 'bg-blue-50 dark:bg-blue-500/10 text-blue-700 dark:text-blue-300'
                      : job.status === 'interrupted' ? 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300'
                      : statusColor(job.status)
                    }`}>
                      {job.status}
                    </span>
                    <p className="text-[12px] text-foreground font-medium">
                      {job.documents_indexed.toLocaleString()} indexed
                      {job.documents_found > 0 ? ` of ${job.documents_found.toLocaleString()} found` : ''}
                    </p>
                    {job.error_message && (
                      <p className="text-[11px] text-red-500 truncate flex-1">{job.error_message}</p>
                    )}
                    <p className="text-[11px] text-faint ml-auto flex-shrink-0">{relativeTime(job.created_at)}</p>
                  </div>
                  {job.status === 'running' && job.documents_found > 0 && (
                    <div className="mt-2 ml-5">
                      <div className="flex items-center justify-between mb-0.5">
                        <span className="text-[10px] text-blue-600 dark:text-blue-300">
                          {Math.round((job.documents_indexed / job.documents_found) * 100)}% complete
                        </span>
                        <span className="text-[10px] text-faint">
                          {(job.documents_found - job.documents_indexed).toLocaleString()} remaining
                        </span>
                      </div>
                      <div className="h-1.5 bg-blue-100 rounded-full overflow-hidden">
                        <div
                          className="h-full bg-blue-500 rounded-full transition-all duration-500"
                          style={{ width: `${Math.round((job.documents_indexed / job.documents_found) * 100)}%` }}
                        />
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="mt-6 p-4 rounded-lg bg-muted border border-border-strong">
        <p className="text-sm text-muted-foreground">
          Learn how to index external data sources and use them for RAG in the{' '}
          <a href="/docs/what-is-a-connector" className="text-accent dark:text-accent-bright hover:underline">
            documentation
          </a>.
        </p>
      </div>
    </div>
  )
}
