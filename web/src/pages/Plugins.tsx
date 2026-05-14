import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Settings2, Trash2, X, FlaskConical } from 'lucide-react'
import {
  api,
  type ConfigSchema,
  type Plugin,
  type PluginType,
  type PluginConfigResponse,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { ConfigForm, defaultsForSchema, stripEmptyPasswords } from '@/components/ConfigForm'
import { useToasts } from '@/lib/toasts'

function statusTone(status: string): string {
  const s = status.toLowerCase()
  if (s === 'connected') return 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border-emerald-500/30'
  if (s === 'connecting' || s === 'reconnecting' || s === 'starting') {
    return 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/30'
  }
  if (s.includes('fail') || s.includes('error') || s === 'auth_failed') {
    return 'bg-destructive/10 text-destructive border-destructive/30'
  }
  return 'bg-muted text-muted-foreground border-border'
}

type Modal =
  | { kind: 'closed' }
  | { kind: 'add' }
  | { kind: 'edit'; pluginId: string }

export default function PluginsPage() {
  const qc = useQueryClient()
  const pushToast = useToasts((s) => s.push)

  const { data: plugins, isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ['plugins'],
    queryFn: api.plugins,
    refetchInterval: 5_000,
  })
  const { data: types } = useQuery({
    queryKey: ['plugin-types'],
    queryFn: api.pluginTypes,
  })

  const typesByName = useMemo(() => {
    const m = new Map<string, PluginType>()
    for (const t of types ?? []) m.set(t.type, t)
    return m
  }, [types])

  const [modal, setModal] = useState<Modal>({ kind: 'closed' })

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deletePlugin(id),
    onSuccess: () => {
      pushToast('Plugin deleted', 'success')
      void qc.invalidateQueries({ queryKey: ['plugins'] })
    },
    onError: (e: unknown) =>
      pushToast(`Delete failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  const onDelete = (p: Plugin) => {
    if (!confirm(`Delete plugin "${p.id}"? Bridges using it will be removed.`)) return
    deleteMut.mutate(p.id)
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Plugins</h1>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => void refetch()}
            className="text-xs text-muted-foreground hover:text-foreground border rounded px-2 py-1"
            disabled={isFetching}
          >
            {isFetching ? 'Refreshing…' : 'Refresh'}
          </button>
          <Button size="sm" onClick={() => setModal({ kind: 'add' })}>
            <Plus className="h-3.5 w-3.5" /> Add plugin
          </Button>
        </div>
      </div>

      {isLoading ? (
        <p className="text-muted-foreground text-sm">Loading plugins…</p>
      ) : error ? (
        <p className="text-destructive text-sm">Failed to load plugins.</p>
      ) : !plugins || plugins.length === 0 ? (
        <div className="border border-dashed rounded-lg p-6 text-sm text-muted-foreground">
          No plugins configured yet. Click <strong>Add plugin</strong> to get started.
        </div>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="text-left px-3 py-2 font-medium">ID</th>
                <th className="text-left px-3 py-2 font-medium w-40">Type</th>
                <th className="text-left px-3 py-2 font-medium w-40">Status</th>
                <th className="w-32" />
              </tr>
            </thead>
            <tbody>
              {plugins.map((p) => (
                <tr key={p.id} className="border-b last:border-0">
                  <td className="px-3 py-2.5 font-mono">{p.id}</td>
                  <td className="px-3 py-2.5 text-muted-foreground">
                    {typesByName.get(p.type)?.displayName ?? p.type}
                  </td>
                  <td className="px-3 py-2.5">
                    <span
                      className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium ${statusTone(
                        p.status,
                      )}`}
                    >
                      <span className="h-1.5 w-1.5 rounded-full bg-current opacity-70" />
                      {p.status}
                    </span>
                  </td>
                  <td className="px-2 py-2 text-right whitespace-nowrap">
                    <Button
                      size="xs"
                      variant="ghost"
                      onClick={() => setModal({ kind: 'edit', pluginId: p.id })}
                      title="Configure"
                    >
                      <Settings2 className="h-3 w-3" /> Configure
                    </Button>
                    <Button
                      size="xs"
                      variant="ghost"
                      onClick={() => onDelete(p)}
                      className="text-destructive ml-1"
                      title="Delete"
                    >
                      <Trash2 className="h-3 w-3" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {modal.kind === 'add' && (
        <AddPluginModal
          types={types ?? []}
          onClose={() => setModal({ kind: 'closed' })}
          onCreated={() => {
            setModal({ kind: 'closed' })
            void qc.invalidateQueries({ queryKey: ['plugins'] })
          }}
        />
      )}
      {modal.kind === 'edit' && (
        <EditPluginModal
          pluginId={modal.pluginId}
          types={typesByName}
          onClose={() => setModal({ kind: 'closed' })}
          onSaved={() => {
            setModal({ kind: 'closed' })
            void qc.invalidateQueries({ queryKey: ['plugins'] })
          }}
        />
      )}
    </div>
  )
}

// ── Modal shell ───────────────────────────────────────────────────────────────

function ModalShell({
  title,
  onClose,
  children,
  footer,
}: {
  title: string
  onClose: () => void
  children: React.ReactNode
  footer: React.ReactNode
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 p-4 overflow-y-auto">
      <div className="bg-background border rounded-lg shadow-lg w-full max-w-lg my-8 flex flex-col max-h-[calc(100vh-4rem)]">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <h2 className="text-base font-semibold">{title}</h2>
          <button
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="px-4 py-4 overflow-y-auto flex-1">{children}</div>
        <div className="flex items-center justify-end gap-2 border-t px-4 py-3">{footer}</div>
      </div>
    </div>
  )
}

// ── Add modal ─────────────────────────────────────────────────────────────────

function AddPluginModal({
  types,
  onClose,
  onCreated,
}: {
  types: PluginType[]
  onClose: () => void
  onCreated: () => void
}) {
  const pushToast = useToasts((s) => s.push)
  const [typeName, setTypeName] = useState<string>(types[0]?.type ?? '')
  const [id, setId] = useState('')
  const selected = types.find((t) => t.type === typeName)
  const schema: ConfigSchema = selected?.schema ?? { fields: [] }

  const [config, setConfig] = useState<Record<string, unknown>>(() => defaultsForSchema(schema))
  useEffect(() => {
    setConfig(defaultsForSchema(schema))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [typeName])

  const createMut = useMutation({
    mutationFn: () =>
      api.createPlugin({
        id: id.trim(),
        type: typeName,
        config: stripEmptyPasswords(config, schema),
      }),
    onSuccess: () => {
      pushToast('Plugin created', 'success')
      onCreated()
    },
    onError: (e: unknown) =>
      pushToast(`Create failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  const probeMut = useMutation({
    mutationFn: () => api.probePluginType(typeName, stripEmptyPasswords(config, schema)),
    onSuccess: (r) =>
      pushToast(
        r.ok ? 'Connection OK' : `Connection failed: ${r.error ?? 'unknown'}`,
        r.ok ? 'success' : 'error',
      ),
    onError: (e: unknown) =>
      pushToast(`Probe failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  const canSave = id.trim() !== '' && typeName !== ''

  return (
    <ModalShell
      title="Add plugin"
      onClose={onClose}
      footer={
        <>
          {selected?.hasProbe && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => probeMut.mutate()}
              disabled={probeMut.isPending}
            >
              <FlaskConical className="h-3.5 w-3.5" />
              {probeMut.isPending ? 'Testing…' : 'Test connection'}
            </Button>
          )}
          <div className="flex-1" />
          <Button size="sm" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={() => createMut.mutate()}
            disabled={!canSave || createMut.isPending}
          >
            {createMut.isPending ? 'Creating…' : 'Create'}
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="space-y-1">
          <label className="text-xs font-medium" htmlFor="plugin-type">
            Type
          </label>
          <select
            id="plugin-type"
            className="w-full border rounded px-2 py-1 text-sm bg-background"
            value={typeName}
            onChange={(e) => setTypeName(e.target.value)}
          >
            {types.map((t) => (
              <option key={t.type} value={t.type}>
                {t.displayName}
              </option>
            ))}
          </select>
          {selected?.description && (
            <p className="text-xs text-muted-foreground">{selected.description}</p>
          )}
        </div>
        <div className="space-y-1">
          <label className="text-xs font-medium" htmlFor="plugin-id">
            ID
          </label>
          <input
            id="plugin-id"
            className="w-full border rounded px-2 py-1 text-sm bg-background font-mono"
            value={id}
            onChange={(e) => setId(e.target.value)}
            placeholder="my-broker"
            autoComplete="off"
          />
          <p className="text-xs text-muted-foreground">
            Unique identifier — referenced by other plugins and persisted in <code>plugins.json</code>.
          </p>
        </div>
        <ConfigForm schema={schema} value={config} onChange={setConfig} />
      </div>
    </ModalShell>
  )
}

// ── Edit modal ────────────────────────────────────────────────────────────────

function EditPluginModal({
  pluginId,
  types,
  onClose,
  onSaved,
}: {
  pluginId: string
  types: Map<string, PluginType>
  onClose: () => void
  onSaved: () => void
}) {
  const pushToast = useToasts((s) => s.push)
  const { data, isLoading, error } = useQuery({
    queryKey: ['plugin-config', pluginId],
    queryFn: () => api.pluginConfig(pluginId),
  })
  const [config, setConfig] = useState<Record<string, unknown> | null>(null)

  useEffect(() => {
    if (data) setConfig(data.config)
  }, [data])

  const typeInfo = data ? types.get(data.type) : undefined
  const schema: ConfigSchema = typeInfo?.schema ?? { fields: [] }

  const saveMut = useMutation({
    mutationFn: () => {
      if (!config) throw new Error('not loaded')
      return api.updatePluginConfig(pluginId, stripEmptyPasswords(config, schema))
    },
    onSuccess: () => {
      pushToast('Plugin updated', 'success')
      onSaved()
    },
    onError: (e: unknown) =>
      pushToast(`Update failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  const probeMut = useMutation({
    mutationFn: () =>
      api.probePlugin(pluginId, config ? stripEmptyPasswords(config, schema) : undefined),
    onSuccess: (r) =>
      pushToast(
        r.ok ? 'Connection OK' : `Connection failed: ${r.error ?? 'unknown'}`,
        r.ok ? 'success' : 'error',
      ),
    onError: (e: unknown) =>
      pushToast(`Probe failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  return (
    <ModalShell
      title={`Configure ${pluginId}`}
      onClose={onClose}
      footer={
        <>
          {typeInfo?.hasProbe && (
            <Button
              size="sm"
              variant="outline"
              disabled={!config || probeMut.isPending}
              onClick={() => probeMut.mutate()}
            >
              <FlaskConical className="h-3.5 w-3.5" />
              {probeMut.isPending ? 'Testing…' : 'Test connection'}
            </Button>
          )}
          <div className="flex-1" />
          <Button size="sm" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={() => saveMut.mutate()}
            disabled={!config || saveMut.isPending}
          >
            {saveMut.isPending ? 'Saving…' : 'Save & restart'}
          </Button>
        </>
      }
    >
      {isLoading ? (
        <p className="text-muted-foreground text-sm">Loading config…</p>
      ) : error || !data ? (
        <p className="text-destructive text-sm">Failed to load plugin config.</p>
      ) : (
        <div className="space-y-3">
          <div className="text-xs text-muted-foreground">
            Type: <span className="font-medium text-foreground">{typeInfo?.displayName ?? data.type}</span>
          </div>
          <PluginConfigBody response={data} schema={schema} value={config} onChange={setConfig} />
        </div>
      )}
    </ModalShell>
  )
}

function PluginConfigBody({
  response,
  schema,
  value,
  onChange,
}: {
  response: PluginConfigResponse
  schema: ConfigSchema
  value: Record<string, unknown> | null
  onChange: (v: Record<string, unknown>) => void
}) {
  if (!value) return null
  return <ConfigForm schema={schema} value={value} secrets={response.secrets} onChange={onChange} />
}
