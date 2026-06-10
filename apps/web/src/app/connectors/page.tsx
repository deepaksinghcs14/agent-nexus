'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BookOpen, ChevronRight, Database, FileText, FolderOpen, Github,
  HardDrive, MessageSquare, Plus, RefreshCw, Trash2, Trello, X,
} from 'lucide-react'
import { connectorsAPI, filesystemAPI } from '@/lib/api'
import { relativeTime, statusColor } from '@/lib/utils'
import type { Connector, ConnectorDocument, ConnectorSyncJob } from '@/types'

// ─── provider definitions ─────────────────────────────────────────────────────

const PROVIDERS = [
  { id: 'filesystem', label: 'Filesystem', Icon: FolderOpen, description: 'Index local files and directories', available: true },
  { id: 'slack',      label: 'Slack',      Icon: MessageSquare, description: 'Index messages and channels',    available: false },
  { id: 'jira',       label: 'Jira',       Icon: Trello,        description: 'Index issues and comments',      available: false },
  { id: 'confluence', label: 'Confluence', Icon: BookOpen,      description: 'Index pages and spaces',         available: false },
  { id: 'github',     label: 'GitHub',     Icon: Github,        description: 'Index issues and pull requests', available: false },
  { id: 'gdrive',     label: 'Google Drive', Icon: HardDrive,   description: 'Index documents and sheets',     available: false },
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
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md">
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-100">
          <p className="text-sm font-medium text-gray-900">Browse filesystem</p>
          <button onClick={onClose}><X size={15} className="text-gray-400" /></button>
        </div>
        <div className="px-4 py-2 bg-gray-50 border-b border-gray-100">
          <p className="text-[11px] font-mono text-gray-600 truncate">{path}</p>
        </div>
        <div className="overflow-y-auto max-h-72">
          {isLoading && <div className="py-8 text-center text-sm text-gray-400">Loading…</div>}
          {error && <div className="py-4 px-4 text-[12px] text-red-600">Cannot read directory.</div>}
          {data && (
            <>
              {data.parent !== '' && (
                <button
                  onClick={() => setPath(data.parent)}
                  className="w-full flex items-center gap-2 px-4 py-2.5 text-[12px] text-gray-500 hover:bg-gray-50 border-b border-gray-50"
                >
                  <FolderOpen size={13} className="text-gray-400" />
                  <span>.. (parent directory)</span>
                </button>
              )}
              {data.entries.length === 0 && (
                <div className="py-6 text-center text-[12px] text-gray-400">Empty directory</div>
              )}
              {data.entries.map((entry) => (
                <button
                  key={entry.path}
                  onClick={() => { if (entry.is_dir) setPath(entry.path) }}
                  disabled={!entry.is_dir}
                  className={`w-full flex items-center gap-2 px-4 py-2.5 text-[12px] border-b border-gray-50 last:border-b-0 ${
                    entry.is_dir
                      ? 'text-gray-800 hover:bg-purple-50 cursor-pointer'
                      : 'text-gray-400 cursor-default'
                  }`}
                >
                  <FolderOpen size={13} className={entry.is_dir ? 'text-amber-400' : 'text-gray-300'} />
                  <span className="text-left truncate flex-1">{entry.name}</span>
                  {entry.is_dir && <ChevronRight size={11} className="text-gray-400 flex-shrink-0" />}
                </button>
              ))}
            </>
          )}
        </div>
        <div className="flex items-center gap-2 px-4 py-3 border-t border-gray-100">
          <button onClick={onClose} className="px-3 py-1.5 text-[12px] text-gray-500 hover:text-gray-800">
            Cancel
          </button>
          <button
            onClick={() => { onSelect(path); onClose() }}
            className="ml-auto px-4 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg font-medium"
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
  const [form, setForm] = useState({ name: '', path: '' })
  const [formError, setFormError] = useState('')
  const [browserOpen, setBrowserOpen] = useState(false)

  const handleSubmit = () => {
    if (!form.name.trim()) { setFormError('Connector name is required'); return }
    if (!form.path.trim()) { setFormError('Directory path is required'); return }
    if (!form.path.startsWith('/')) { setFormError('Path must be absolute and start with /'); return }
    setFormError('')
    onCreate({ name: form.name.trim(), provider: 'filesystem', type: 'native', auth_type: 'none', config: { path: form.path.trim() } })
  }

  return (
    <div className="border border-gray-200 rounded-xl mb-6 overflow-hidden bg-white">
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-100 bg-gray-50">
        <p className="text-sm font-medium text-gray-800">Connect a data source</p>
        <button onClick={onClose}><X size={14} className="text-gray-400" /></button>
      </div>

      <div className="p-4">
        <p className="text-[11px] font-medium text-gray-400 uppercase tracking-wide mb-3">Choose a source type</p>
        <div className="grid grid-cols-3 gap-2 mb-5">
          {PROVIDERS.map(({ id, label, Icon, description, available }) => {
            const active = provider === id
            return (
              <button
                key={id}
                disabled={!available}
                onClick={() => available && setProvider(id)}
                className={`relative flex flex-col items-start gap-1.5 p-3 rounded-xl border text-left transition-all ${
                  active
                    ? 'border-purple-300 bg-purple-50'
                    : available
                    ? 'border-gray-200 hover:border-gray-300 bg-white'
                    : 'border-gray-100 bg-gray-50 opacity-50 cursor-not-allowed'
                }`}
              >
                <Icon size={15} className={active ? 'text-purple-600' : available ? 'text-gray-500' : 'text-gray-400'} />
                <span className={`text-[12px] font-medium ${active ? 'text-purple-700' : 'text-gray-700'}`}>{label}</span>
                <span className="text-[10px] text-gray-400 leading-tight">{description}</span>
                {!available && (
                  <span className="absolute top-2 right-2 text-[9px] px-1.5 py-0.5 bg-gray-200 text-gray-500 rounded-full">v0.2</span>
                )}
              </button>
            )
          })}
        </div>

        {provider === 'filesystem' && (
          <div className="border-t border-gray-100 pt-4">
            <p className="text-[11px] font-medium text-gray-400 uppercase tracking-wide mb-3">Configure filesystem connector</p>
            {formError && (
              <p className="text-[12px] text-red-600 bg-red-50 border border-red-100 rounded px-3 py-2 mb-3">{formError}</p>
            )}
            <div className="grid grid-cols-2 gap-3 mb-3">
              <div>
                <label className="block text-[11px] font-medium text-gray-600 mb-1">Connector name *</label>
                <input
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="e.g. Engineering Wiki"
                  className="w-full text-sm px-3 py-2 border border-gray-200 rounded-lg"
                />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-gray-600 mb-1">Directory path *</label>
                <div className="flex gap-1.5">
                  <input
                    value={form.path}
                    onChange={(e) => setForm({ ...form, path: e.target.value })}
                    placeholder="/home/user/documents"
                    className="flex-1 text-sm px-3 py-2 border border-gray-200 rounded-lg font-mono min-w-0"
                  />
                  <button
                    type="button"
                    onClick={() => setBrowserOpen(true)}
                    className="px-3 py-2 border border-gray-200 rounded-lg text-[11px] text-gray-600 hover:bg-gray-50 flex-shrink-0"
                  >
                    Browse
                  </button>
                </div>
              </div>
            </div>
            <p className="text-[11px] text-gray-400 mb-4">
              Agent Nexus will recursively index all files in this directory. Run a sync after connecting to start indexing.
            </p>
            <div className="flex gap-2">
              <button
                onClick={handleSubmit}
                disabled={isPending}
                className="px-4 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg font-medium disabled:opacity-50"
              >
                {isPending ? 'Connecting…' : 'Connect'}
              </button>
              <button onClick={onClose} className="px-3 py-1.5 text-[12px] text-gray-500 hover:text-gray-800">
                Cancel
              </button>
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
  const [showCreate, setShowCreate] = useState(false)
  const [selected, setSelected] = useState('')
  const [detailTab, setDetailTab] = useState<'documents' | 'sync-history'>('documents')
  const [actionError, setActionError] = useState('')

  const { data, isLoading, error } = useQuery({
    queryKey: ['connectors'],
    queryFn: () => connectorsAPI.list() as Promise<{ data: Connector[] }>,
  })
  const connectors = data?.data ?? []
  const selectedConnector = connectors.find((c) => c.id === selected)

  const { data: docsData, isLoading: docsLoading } = useQuery({
    queryKey: ['connector-docs', selected],
    queryFn: () => connectorsAPI.documents(selected) as Promise<{ data: ConnectorDocument[] }>,
    enabled: !!selected,
  })
  const { data: jobsData } = useQuery({
    queryKey: ['connector-jobs', selected],
    queryFn: () => connectorsAPI.syncJobs(selected) as Promise<{ data: ConnectorSyncJob[] }>,
    enabled: !!selected,
  })

  const create = useMutation({
    mutationFn: (body: unknown) => connectorsAPI.create(body),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['connectors'] }); setShowCreate(false); setActionError('') },
    onError: (err: Error) => setActionError(err.message),
  })
  const sync = useMutation({
    mutationFn: (id: string) => connectorsAPI.sync(id),
    onSuccess: (_, id) => { queryClient.invalidateQueries({ queryKey: ['connectors'] }); queryClient.invalidateQueries({ queryKey: ['connector-jobs', id] }) },
    onError: (err: Error) => setActionError(err.message),
  })
  const remove = useMutation({
    mutationFn: (id: string) => connectorsAPI.delete(id),
    onSuccess: () => { setSelected(''); queryClient.invalidateQueries({ queryKey: ['connectors'] }) },
    onError: (err: Error) => setActionError(err.message),
  })

  const docs = docsData?.data ?? []
  const jobs = jobsData?.data ?? []

  return (
    <div className="p-6 max-w-4xl">
      {/* Header */}
      <div className="flex items-center justify-between mb-5">
        <div>
          <h1 className="text-base font-medium text-gray-900">Context connectors</h1>
          <p className="text-[12px] text-gray-400 mt-0.5">
            Connect data sources so agents can retrieve relevant context when they run.
          </p>
        </div>
        {connectors.length > 0 && !showCreate && (
          <button
            onClick={() => { setShowCreate(true); setActionError('') }}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg"
          >
            <Plus size={13} /> Connect source
          </button>
        )}
      </div>

      {(error || actionError) && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
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

      {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading…</div>}

      {/* Empty state */}
      {!isLoading && !error && connectors.length === 0 && !showCreate && (
        <div className="grid grid-cols-2 gap-8 border border-dashed border-gray-200 rounded-xl p-8">
          <div>
            <Database size={22} className="text-purple-300 mb-3" />
            <p className="text-sm font-medium text-gray-800 mb-1">What are connectors?</p>
            <p className="text-[12px] text-gray-500 leading-relaxed mb-5">
              Connectors index documents from external sources so agents can retrieve relevant information at runtime — grounding responses in your actual data, not just model training.
            </p>
            <div className="space-y-3">
              {[
                { n: '1', title: 'Connect a source', desc: 'Point to a directory or integration' },
                { n: '2', title: 'Sync documents',   desc: 'Agent Nexus indexes and embeds the content' },
                { n: '3', title: 'Agents use context', desc: 'Relevant chunks are injected at run time' },
              ].map((s) => (
                <div key={s.n} className="flex items-start gap-2.5">
                  <span className="w-5 h-5 rounded-full bg-purple-100 text-purple-600 text-[10px] font-bold flex items-center justify-center flex-shrink-0 mt-0.5">
                    {s.n}
                  </span>
                  <div>
                    <p className="text-[12px] font-medium text-gray-700">{s.title}</p>
                    <p className="text-[11px] text-gray-400">{s.desc}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
          <div className="flex flex-col items-center justify-center border-l border-dashed border-gray-200 pl-8">
            <FolderOpen size={28} className="text-gray-300 mb-3" />
            <p className="text-sm font-medium text-gray-600 mb-1">Connect your first source</p>
            <p className="text-[11px] text-gray-400 mb-5 text-center">
              Start with a local directory — index your docs, codebase, or any file collection.
            </p>
            <button
              onClick={() => { setShowCreate(true); setActionError('') }}
              className="px-4 py-2 bg-purple-600 text-white text-[12px] rounded-lg font-medium"
            >
              Connect source
            </button>
          </div>
        </div>
      )}

      {/* Connector cards */}
      {connectors.length > 0 && (
        <div className="grid grid-cols-3 gap-3 mb-6">
          {connectors.map((connector) => {
            const cfg = connector.config as { path?: string } | undefined
            const isSyncing = sync.isPending && (sync.variables as string) === connector.id
            return (
              <div
                key={connector.id}
                onClick={() => { setSelected(connector.id === selected ? '' : connector.id); setDetailTab('documents') }}
                className={`bg-white border rounded-xl p-3.5 cursor-pointer transition-all ${
                  selected === connector.id
                    ? 'border-purple-300 ring-1 ring-purple-100'
                    : 'border-gray-100 hover:border-gray-200'
                }`}
              >
                <div className="flex items-start justify-between mb-2">
                  <div className="flex items-center gap-1.5 min-w-0">
                    <FolderOpen size={13} className="text-amber-400 flex-shrink-0" />
                    <p className="text-[13px] font-medium text-gray-900 truncate">{connector.name}</p>
                  </div>
                  <span className={`text-[10px] px-2 py-0.5 rounded-full flex-shrink-0 ml-1 ${statusColor(connector.status)}`}>
                    {connector.status}
                  </span>
                </div>
                {cfg?.path && (
                  <p className="text-[10px] font-mono text-gray-400 truncate mb-2" title={cfg.path}>{cfg.path}</p>
                )}
                <p className="text-[10px] text-gray-400 mb-3">Filesystem</p>
                <div className="flex gap-1.5" onClick={(e) => e.stopPropagation()}>
                  <button
                    onClick={() => sync.mutate(connector.id)}
                    disabled={isSyncing}
                    className="inline-flex items-center gap-1 px-2 py-1 border border-gray-200 text-[11px] text-gray-600 rounded-md hover:bg-gray-50 disabled:opacity-50"
                  >
                    <RefreshCw size={11} className={isSyncing ? 'animate-spin' : ''} />
                    {isSyncing ? 'Syncing…' : 'Sync'}
                  </button>
                  <button
                    onClick={() => { if (confirm('Delete this connector and all indexed documents?')) remove.mutate(connector.id) }}
                    className="ml-auto p-1 text-gray-300 hover:text-red-500"
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
        <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
          <div className="flex items-center border-b border-gray-100 px-1">
            {(['documents', 'sync-history'] as const).map((tab) => (
              <button
                key={tab}
                onClick={() => setDetailTab(tab)}
                className={`px-4 py-2.5 text-[12px] font-medium border-b-2 -mb-px transition-colors ${
                  detailTab === tab
                    ? 'border-purple-600 text-purple-700'
                    : 'border-transparent text-gray-500 hover:text-gray-700'
                }`}
              >
                {tab === 'documents' ? 'Documents' : 'Sync history'}
              </button>
            ))}
          </div>

          {detailTab === 'documents' && (
            <div>
              {docsLoading && <div className="py-8 text-center text-sm text-gray-400">Loading documents…</div>}
              {!docsLoading && docs.length === 0 && (
                <div className="py-10 text-center">
                  <FileText size={20} className="text-gray-300 mx-auto mb-2" />
                  <p className="text-[12px] text-gray-400">No documents indexed yet.</p>
                  <p className="text-[11px] text-gray-400 mt-1">Click Sync on the connector card to start indexing.</p>
                </div>
              )}
              {docs.length > 0 && (
                <table className="w-full">
                  <thead>
                    <tr className="border-b border-gray-50">
                      <th className="text-left text-[10px] font-medium text-gray-400 px-4 py-2">Title</th>
                      <th className="text-left text-[10px] font-medium text-gray-400 px-4 py-2">Source</th>
                      <th className="text-left text-[10px] font-medium text-gray-400 px-4 py-2">Indexed</th>
                    </tr>
                  </thead>
                  <tbody>
                    {docs.map((doc) => (
                      <tr key={doc.id} className="border-b last:border-b-0 border-gray-50 hover:bg-gray-50/50">
                        <td className="px-4 py-2.5">
                          <p className="text-[12px] font-medium text-gray-800 truncate max-w-xs">{doc.title || '(untitled)'}</p>
                        </td>
                        <td className="px-4 py-2.5">
                          <p className="text-[11px] font-mono text-gray-400 truncate max-w-xs">{doc.source}</p>
                        </td>
                        <td className="px-4 py-2.5">
                          <p className="text-[11px] text-gray-400">{doc.indexed_at ? relativeTime(doc.indexed_at) : 'not indexed'}</p>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          )}

          {detailTab === 'sync-history' && (
            <div>
              {jobs.length === 0 && (
                <div className="py-10 text-center">
                  <p className="text-[12px] text-gray-400">No sync jobs recorded.</p>
                  <p className="text-[11px] text-gray-400 mt-1">Use the Sync button to trigger a sync.</p>
                </div>
              )}
              {jobs.map((job) => (
                <div key={job.id} className="flex items-center gap-3 px-4 py-3 border-b last:border-b-0 border-gray-50">
                  <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(job.status === 'completed' ? 'connected' : job.status)}`}>
                    {job.status}
                  </span>
                  <p className="text-[12px] text-gray-600">
                    {job.documents_indexed} of {job.documents_found} documents indexed
                  </p>
                  {job.error_message && (
                    <p className="text-[11px] text-red-500 truncate flex-1">{job.error_message}</p>
                  )}
                  <p className="text-[11px] text-gray-400 ml-auto flex-shrink-0">{relativeTime(job.created_at)}</p>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="mt-6 p-4 rounded-lg bg-gray-50 border border-gray-200">
        <p className="text-sm text-gray-600">
          Learn how to index external data sources and use them for RAG in the{' '}
          <a href="/docs/what-is-a-connector" className="text-[#534AB7] hover:underline">
            documentation
          </a>.
        </p>
      </div>
    </div>
  )
}
