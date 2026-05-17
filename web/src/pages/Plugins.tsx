import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Plus, Settings2, Trash2, X, FlaskConical, ScrollText,
  Home, Wifi, Cpu, Lightbulb, Antenna, Box,
  Boxes, Link2, Monitor, AlertTriangle,
  RefreshCw, Radar, Power, PowerOff, Pause, Play, Terminal,
} from 'lucide-react'
import {
  api,
  connectEvents,
  type ConfigSchema,
  type LogLevel,
  type Plugin,
  type PluginEvent,
  type PluginType,
  type PluginConfigResponse,
  type WsEvent,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { ConfigForm, defaultsForSchema, stripEmptyPasswords } from '@/components/ConfigForm'
import { useToasts } from '@/lib/toasts'

// ── Plugin-type visual mapping ────────────────────────────────────────────────
// Static map of plugin type → icon + colour tones. Falls back to a generic
// box icon for unknown types (custom or future plugins).

interface PluginVisual {
  Icon: typeof Home
  tone: string   // tailwind classes for the icon tile background+colour
  dot: string    // hex colour used for the small status dot accent
}

const PLUGIN_VISUAL: Record<string, PluginVisual> = {
  homeassistant: { Icon: Home,      tone: 'bg-indigo-500/10 text-indigo-600 dark:text-indigo-400',   dot: '#6366f1' },
  mqtt:          { Icon: Wifi,      tone: 'bg-violet-500/10 text-violet-600 dark:text-violet-400',   dot: '#a855f7' },
  tasmota:       { Icon: Cpu,       tone: 'bg-sky-500/10 text-sky-600 dark:text-sky-400',            dot: '#0ea5e9' },
  wled:          { Icon: Lightbulb, tone: 'bg-fuchsia-500/10 text-fuchsia-600 dark:text-fuchsia-400', dot: '#d946ef' },
  zigbee2mqtt:   { Icon: Antenna,   tone: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',      dot: '#f59e0b' },
  externaldevice:{ Icon: Terminal,  tone: 'bg-teal-500/10 text-teal-600 dark:text-teal-400',         dot: '#14b8a6' },
}

const DEFAULT_VISUAL: PluginVisual = {
  Icon: Box,
  tone: 'bg-slate-500/10 text-slate-600 dark:text-slate-400',
  dot: '#64748b',
}

function visualForType(type: string): PluginVisual {
  return PLUGIN_VISUAL[type] ?? DEFAULT_VISUAL
}

function statusTone(status: string): string {
  const s = status.toLowerCase()
  if (s === 'connected') return 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border-emerald-500/30'
  if (s === 'connecting' || s === 'reconnecting' || s === 'starting') {
    return 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/30'
  }
  if (s === 'disabled' || s === 'stopped') {
    return 'bg-slate-500/10 text-slate-600 dark:text-slate-300 border-slate-500/30'
  }
  if (s.includes('fail') || s.includes('error') || s === 'auth_failed') {
    return 'bg-destructive/10 text-destructive border-destructive/30'
  }
  return 'bg-muted text-muted-foreground border-border'
}

function relativeTime(iso?: string): string {
  if (!iso) return '—'
  const t = new Date(iso).getTime()
  if (!Number.isFinite(t)) return '—'
  const diff = Date.now() - t
  if (diff < 0) return 'just now'
  const sec = Math.floor(diff / 1000)
  if (sec < 5) return 'just now'
  if (sec < 60) return `${sec}s ago`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr}h ago`
  const day = Math.floor(hr / 24)
  return `${day}d ago`
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
  const { data: bridges } = useQuery({
    queryKey: ['bridges'],
    queryFn: api.bridges,
  })

  const typesByName = useMemo(() => {
    const m = new Map<string, PluginType>()
    for (const t of types ?? []) m.set(t.type, t)
    return m
  }, [types])

  // Devices per plugin id, derived from bridges.
  const devicesByPlugin = useMemo(() => {
    const m = new Map<string, number>()
    for (const b of bridges ?? []) m.set(b.pluginId, (m.get(b.pluginId) ?? 0) + 1)
    return m
  }, [bridges])

  // Live in-memory event ring shared with the logs panel and used for KPIs.
  const [events, setEvents] = useState<PluginEvent[]>([])
  const { data: initialEvents } = useQuery({
    queryKey: ['plugin-events-global'],
    queryFn: () => api.pluginEventsGlobal({ limit: 500 }),
    staleTime: Infinity,
    gcTime: 0,
  })
  useEffect(() => {
    if (initialEvents) setEvents(initialEvents)
  }, [initialEvents])
  useEffect(() => {
    return connectEvents((e: WsEvent) => {
      if (e.type === 'pluginEvent') {
        setEvents((prev) => {
          const next = [...prev, e.data as PluginEvent]
          // Cap the ring at 1000 to bound memory.
          return next.length > 1000 ? next.slice(next.length - 1000) : next
        })
      } else if (e.type === 'pluginAdded' || e.type === 'pluginRemoved' || e.type === 'pluginUpdated') {
        void qc.invalidateQueries({ queryKey: ['plugins'] })
      }
    })
  }, [qc])

  // Per-plugin "last activity" timestamp + recent error/warn counts, derived
  // from the live event buffer.
  const lastEventByPlugin = useMemo(() => {
    const m = new Map<string, string>()
    for (const ev of events) m.set(ev.pluginId, ev.time)
    return m
  }, [events])

  // KPI: warnings/errors in the last hour.
  const recentCounts = useMemo(() => {
    const cutoff = Date.now() - 60 * 60 * 1000
    let warn = 0, err = 0
    for (const ev of events) {
      const t = new Date(ev.time).getTime()
      if (!Number.isFinite(t) || t < cutoff) continue
      if (ev.level === 'warn') warn++
      else if (ev.level === 'error') err++
    }
    return { warn, err }
  }, [events])

  const totalPlugins = plugins?.length ?? 0
  const connectedPlugins = (plugins ?? []).filter((p) => p.status === 'connected').length
  const totalDevices = bridges?.length ?? 0
  const connectedPct = totalPlugins === 0 ? 0 : Math.round((connectedPlugins / totalPlugins) * 100)

  const [modal, setModal] = useState<Modal>({ kind: 'closed' })

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deletePlugin(id),
    onSuccess: () => {
      pushToast('Plugin deleted', 'success')
      void qc.invalidateQueries({ queryKey: ['plugins'] })
      void qc.invalidateQueries({ queryKey: ['bridges'] })
    },
    onError: (e: unknown) =>
      pushToast(`Delete failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })
  const restartMut = useMutation({
    mutationFn: (id: string) => api.restartPlugin(id),
    onSuccess: (r, id) => {
      if (r.ok) pushToast(`Plugin "${id}" restarted`, 'success')
      else pushToast(`Restart failed: ${r.error ?? 'unknown'}`, 'error')
      void qc.invalidateQueries({ queryKey: ['plugins'] })
    },
    onError: (e: unknown) =>
      pushToast(`Restart failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })
  const enableMut = useMutation({
    mutationFn: (p: Plugin) => (p.enabled ? api.disablePlugin(p.id) : api.enablePlugin(p.id)),
    onSuccess: (r, p) => {
      if (!r.ok) pushToast(`${p.enabled ? 'Disable' : 'Enable'} failed: ${r.error ?? 'unknown'}`, 'error')
      else pushToast(`Plugin "${p.id}" ${r.enabled ? 'enabled' : 'disabled'}`, 'success')
      void qc.invalidateQueries({ queryKey: ['plugins'] })
      void qc.invalidateQueries({ queryKey: ['bridges'] })
    },
    onError: (e: unknown) =>
      pushToast(`Toggle failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })
  const discoverMut = useMutation({
    mutationFn: (id: string) => api.rediscoverPlugin(id),
    onSuccess: (entities, id) => {
      pushToast(`Discovery on "${id}" returned ${entities.length} entities`, 'success')
      void qc.invalidateQueries({ queryKey: ['discovered'] })
    },
    onError: (e: unknown) =>
      pushToast(`Discovery failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  const onDelete = (p: Plugin) => {
    if (!confirm(`Delete plugin "${p.id}"? Bridges using it will also be removed.`)) return
    deleteMut.mutate(p.id)
  }
  const onToggleEnabled = (p: Plugin) => {
    if (p.enabled && !confirm(`Disable plugin "${p.id}"? Its bridges will stop receiving updates until re-enabled.`)) return
    enableMut.mutate(p)
  }

  const sortedPlugins = useMemo(
    () => [...(plugins ?? [])].sort((a, b) => a.id.localeCompare(b.id)),
    [plugins],
  )

  return (
    <div className="space-y-4">
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

      {/* KPI strip */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <KpiCard
          icon={<Boxes className="h-4 w-4" />}
          tone="bg-indigo-500/10 text-indigo-600 dark:text-indigo-400"
          label="Total plugins"
          value={totalPlugins}
          sub="All configured plugins"
        />
        <KpiCard
          icon={<Link2 className="h-4 w-4" />}
          tone="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
          label="Connected plugins"
          value={connectedPlugins}
          sub={totalPlugins > 0 ? `${connectedPct}% of plugins connected` : 'No plugins yet'}
        />
        <KpiCard
          icon={<Monitor className="h-4 w-4" />}
          tone="bg-sky-500/10 text-sky-600 dark:text-sky-400"
          label="Active devices tracked"
          value={totalDevices}
          sub="Across all plugins"
        />
        <KpiCard
          icon={<AlertTriangle className="h-4 w-4" />}
          tone={
            recentCounts.err > 0
              ? 'bg-destructive/10 text-destructive'
              : recentCounts.warn > 0
                ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
                : 'bg-slate-500/10 text-slate-500'
          }
          label="Warnings / errors"
          value={recentCounts.warn + recentCounts.err}
          sub={`${recentCounts.warn} warn · ${recentCounts.err} err (last 1h)`}
        />
      </div>

      {/* Plugins table */}
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
              <tr className="border-b bg-muted/50 text-left">
                <th className="px-3 py-2 font-medium">Plugin</th>
                <th className="px-3 py-2 font-medium w-32 hidden md:table-cell">Type</th>
                <th className="px-3 py-2 font-medium w-32">Status</th>
                <th className="px-3 py-2 font-medium w-32" title="Discovered devices / actively bridged">
                  Devices
                </th>
                <th className="px-3 py-2 font-medium w-32 hidden lg:table-cell">Last activity</th>
                <th className="px-3 py-2 font-medium w-56 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {sortedPlugins.map((p) => {
                const visual = visualForType(p.type)
                const Icon = visual.Icon
                const typeInfo = typesByName.get(p.type)
                const deviceCount = devicesByPlugin.get(p.id) ?? 0
                const lastTime = lastEventByPlugin.get(p.id)
                const busy =
                  (restartMut.isPending && restartMut.variables === p.id) ||
                  (enableMut.isPending && enableMut.variables?.id === p.id) ||
                  (discoverMut.isPending && discoverMut.variables === p.id) ||
                  (deleteMut.isPending && deleteMut.variables === p.id)
                return (
                  <tr key={p.id} className={`border-b last:border-0 ${p.enabled ? '' : 'opacity-60'}`}>
                    <td className="px-3 py-2.5">
                      <div className="flex items-center gap-2.5">
                        <span className={`flex h-9 w-9 items-center justify-center rounded-md shrink-0 ${visual.tone}`}>
                          <Icon className="h-4 w-4" />
                        </span>
                        <div className="min-w-0">
                          <div className="font-medium font-mono truncate">{p.id}</div>
                          <div className="text-[11px] text-muted-foreground truncate">
                            {typeInfo?.displayName ?? p.type}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td className="px-3 py-2.5 text-muted-foreground hidden md:table-cell">
                      {typeInfo?.displayName ?? p.type}
                    </td>
                    <td className="px-3 py-2.5">
                      <span
                        className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium ${statusTone(p.status)}`}
                      >
                        <span className="h-1.5 w-1.5 rounded-full bg-current opacity-70" />
                        {p.status}
                      </span>
                    </td>
                    <td className="px-3 py-2.5 tabular-nums">
                      {(() => {
                        const bridged = deviceCount
                        const discovered = p.stats?.discovered
                        if (discovered === undefined) {
                          return (
                            <span
                              className="inline-flex items-center gap-1 text-muted-foreground"
                              title={`${bridged} bridged device(s)`}
                            >
                              <Monitor className="h-3 w-3" />
                              {bridged}
                            </span>
                          )
                        }
                        return (
                          <span
                            className="inline-flex items-center gap-1 text-muted-foreground"
                            title={`${discovered} discovered · ${bridged} bridged`}
                          >
                            <Monitor className="h-3 w-3" />
                            <span>{discovered}</span>
                            <span className="opacity-50">/</span>
                            <span className="text-foreground font-medium">{bridged}</span>
                          </span>
                        )
                      })()}
                    </td>
                    <td className="px-3 py-2.5 text-muted-foreground hidden lg:table-cell" title={lastTime}>
                      {relativeTime(lastTime)}
                    </td>
                    <td className="px-2 py-2 text-right whitespace-nowrap">
                      <IconAction
                        title="Configure"
                        onClick={() => setModal({ kind: 'edit', pluginId: p.id })}
                      >
                        <Settings2 className="h-3.5 w-3.5" />
                      </IconAction>
                      <IconAction
                        title="Restart"
                        disabled={!p.enabled || busy}
                        onClick={() => restartMut.mutate(p.id)}
                      >
                        <RefreshCw className={`h-3.5 w-3.5 ${restartMut.isPending && restartMut.variables === p.id ? 'animate-spin' : ''}`} />
                      </IconAction>
                      <IconAction
                        title="Run discovery"
                        disabled={!p.enabled || p.status !== 'connected' || busy}
                        onClick={() => discoverMut.mutate(p.id)}
                      >
                        <Radar className={`h-3.5 w-3.5 ${discoverMut.isPending && discoverMut.variables === p.id ? 'animate-pulse' : ''}`} />
                      </IconAction>
                      <IconAction
                        title={p.enabled ? 'Disable (keep config)' : 'Enable'}
                        disabled={busy}
                        onClick={() => onToggleEnabled(p)}
                        className={p.enabled ? '' : 'text-emerald-600 dark:text-emerald-400'}
                      >
                        {p.enabled ? <PowerOff className="h-3.5 w-3.5" /> : <Power className="h-3.5 w-3.5" />}
                      </IconAction>
                      <IconAction
                        title="Delete"
                        disabled={busy}
                        onClick={() => onDelete(p)}
                        className="text-destructive"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </IconAction>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {plugins && plugins.length > 0 && (
        <AllLogsPanel plugins={sortedPlugins} events={events} />
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

// ── Small UI building blocks ──────────────────────────────────────────────────

function KpiCard({
  icon, tone, label, value, sub,
}: {
  icon: React.ReactNode
  tone: string
  label: string
  value: number | string
  sub?: string
}) {
  return (
    <div className="border rounded-lg p-3 bg-card flex items-start gap-3">
      <span className={`flex h-9 w-9 items-center justify-center rounded-md shrink-0 ${tone}`}>
        {icon}
      </span>
      <div className="min-w-0">
        <div className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</div>
        <div className="text-2xl font-semibold leading-tight tabular-nums">{value}</div>
        {sub && <div className="text-[11px] text-muted-foreground truncate">{sub}</div>}
      </div>
    </div>
  )
}

function IconAction({
  title, onClick, children, disabled, className,
}: {
  title: string
  onClick: () => void
  children: React.ReactNode
  disabled?: boolean
  className?: string
}) {
  return (
    <button
      type="button"
      title={title}
      aria-label={title}
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center justify-center h-7 w-7 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40 disabled:hover:bg-transparent ml-0.5 ${className ?? ''}`}
    >
      {children}
    </button>
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
          <PluginConfigBody response={data} schema={schema} value={config} onChange={setConfig} pluginId={pluginId} />
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
  pluginId,
}: {
  response: PluginConfigResponse
  schema: ConfigSchema
  value: Record<string, unknown> | null
  onChange: (v: Record<string, unknown>) => void
  pluginId?: string
}) {
  if (!value) return null
  return (
    <ConfigForm
      schema={schema}
      value={value}
      secrets={response.secrets}
      onChange={onChange}
      pluginId={pluginId}
    />
  )
}

// ── Shared log helpers ─────────────────────────────────────────────────────────

const LOG_LEVELS: LogLevel[] = ['debug', 'info', 'warn', 'error']

function levelOrder(l: LogLevel): number {
  return LOG_LEVELS.indexOf(l)
}

function levelBadge(level: LogLevel): string {
  switch (level) {
    case 'debug': return 'bg-slate-500/10 text-slate-500 border-slate-500/30'
    case 'info':  return 'bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/30'
    case 'warn':  return 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/30'
    case 'error': return 'bg-destructive/10 text-destructive border-destructive/30'
  }
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso)
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 })
  } catch {
    return iso
  }
}

// ── All-plugins log panel ──────────────────────────────────────────────────────

function AllLogsPanel({ plugins, events }: { plugins: Plugin[]; events: PluginEvent[] }) {
  const [levelFilter, setLevelFilter] = useState<LogLevel | ''>('')
  const [pluginFilter, setPluginFilter] = useState<Set<string>>(new Set())
  const [autoScroll, setAutoScroll] = useState(true)
  const [paused, setPaused] = useState(false)
  // Snapshot of events captured when paused so the list freezes.
  const [pausedSnapshot, setPausedSnapshot] = useState<PluginEvent[] | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  // When paused toggles on, capture; when toggled off, clear so live resumes.
  useEffect(() => {
    if (paused) setPausedSnapshot(events)
    else setPausedSnapshot(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [paused])

  const liveEvents = paused && pausedSnapshot ? pausedSnapshot : events

  // Auto-scroll
  useEffect(() => {
    if (!paused && autoScroll && bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [liveEvents, autoScroll, paused])

  const onScroll = () => {
    const el = listRef.current
    if (!el) return
    setAutoScroll(el.scrollHeight - el.scrollTop - el.clientHeight < 40)
  }

  function togglePlugin(id: string) {
    setPluginFilter((s) => {
      const next = new Set(s)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }

  const displayed = liveEvents.filter((ev) => {
    if (pluginFilter.size > 0 && !pluginFilter.has(ev.pluginId)) return false
    if (levelFilter && levelOrder(ev.level) < levelOrder(levelFilter as LogLevel)) return false
    return true
  })

  const sevChips: Array<{ label: string; value: LogLevel | '' }> = [
    { label: 'All',   value: '' },
    { label: 'Info',  value: 'info' },
    { label: 'Warn',  value: 'warn' },
    { label: 'Error', value: 'error' },
    { label: 'Debug', value: 'debug' },
  ]

  return (
    <div className="border rounded-lg overflow-hidden flex flex-col bg-card" style={{ height: '28rem' }}>
      {/* toolbar */}
      <div className="flex items-center gap-2 px-3 py-2 border-b bg-muted/30 shrink-0 flex-wrap">
        <ScrollText className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        <span className="text-xs font-semibold mr-1">Logs</span>

        {/* severity chips */}
        <div className="inline-flex items-center gap-0.5 rounded-md border bg-background p-0.5">
          {sevChips.map((c) => (
            <button
              key={c.value || 'all'}
              type="button"
              onClick={() => setLevelFilter(c.value)}
              className={`px-2 py-0.5 text-[11px] rounded ${
                levelFilter === c.value
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {c.label}
            </button>
          ))}
        </div>

        {/* plugin chips */}
        <div className="flex items-center gap-1 flex-wrap">
          {plugins.map((p) => {
            const v = visualForType(p.type)
            const active = pluginFilter.has(p.id)
            return (
              <button
                key={p.id}
                type="button"
                onClick={() => togglePlugin(p.id)}
                className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] transition-colors ${
                  active
                    ? 'border-primary bg-primary text-primary-foreground'
                    : 'border-border bg-background hover:bg-muted text-muted-foreground'
                }`}
                title={`Filter to ${p.id}`}
              >
                <span className="h-1.5 w-1.5 rounded-full" style={{ background: v.dot }} />
                {p.id}
              </button>
            )
          })}
        </div>

        <div className="flex-1" />
        <span className="text-[11px] text-muted-foreground tabular-nums">{displayed.length} events</span>
        <button
          type="button"
          onClick={() => setPaused((p) => !p)}
          className={`inline-flex items-center gap-1 text-[11px] rounded-full border px-2 py-0.5 ${
            paused
              ? 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300'
              : 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
          }`}
          title={paused ? 'Resume live updates' : 'Pause live updates'}
        >
          {paused ? <><Play className="h-3 w-3" /> Paused</> : <><span className="h-1.5 w-1.5 rounded-full bg-current animate-pulse" /> Live</>}
        </button>
        {!autoScroll && !paused && (
          <button
            type="button"
            className="text-[11px] underline text-muted-foreground hover:text-foreground"
            onClick={() => {
              setAutoScroll(true)
              bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
            }}
          >
            ↓ bottom
          </button>
        )}
        {paused && (
          <button
            type="button"
            onClick={() => setPaused(false)}
            className="text-[11px] underline text-muted-foreground hover:text-foreground inline-flex items-center gap-0.5"
          >
            <Pause className="h-3 w-3" /> resume
          </button>
        )}
      </div>

      {/* log list */}
      <div
        ref={listRef}
        onScroll={onScroll}
        className="flex-1 overflow-y-auto font-mono text-xs leading-relaxed"
      >
        {displayed.length === 0 ? (
          <p className="text-muted-foreground p-4">No events match the current filters.</p>
        ) : (
          <table className="w-full border-collapse">
            <tbody>
              {displayed.map((ev) => {
                const v = visualForType(plugins.find((p) => p.id === ev.pluginId)?.type ?? '')
                const rail =
                  ev.level === 'error' ? 'inset 3px 0 0 var(--destructive,#ef4444)'
                  : ev.level === 'warn' ? 'inset 3px 0 0 #f59e0b'
                  : undefined
                return (
                  <tr key={ev.seq} className="border-b border-border/40 hover:bg-muted/40">
                    <td className="px-3 py-1.5 whitespace-nowrap text-muted-foreground w-24" style={{ boxShadow: rail }}>
                      {formatTime(ev.time)}
                    </td>
                    <td className="px-1 py-1.5 w-16">
                      <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide border ${levelBadge(ev.level)}`}>
                        {ev.level}
                      </span>
                    </td>
                    <td className="px-2 py-1.5 whitespace-nowrap w-32 font-sans">
                      <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] ${v.tone}`}>
                        <span className="h-1 w-1 rounded-full" style={{ background: v.dot }} />
                        {ev.pluginId}
                      </span>
                    </td>
                    <td className="px-2 py-1.5 text-muted-foreground whitespace-nowrap w-40">
                      {ev.code}
                    </td>
                    <td className="px-2 py-1.5 break-all">
                      <span className="text-foreground">{ev.message}</span>
                      {ev.fields && Object.keys(ev.fields).length > 0 && (
                        <span className="ml-2 inline-flex flex-wrap gap-1 align-middle">
                          {Object.entries(ev.fields).map(([k, val]) => (
                            <span
                              key={k}
                              className="inline-flex items-center gap-0.5 rounded bg-muted text-muted-foreground px-1.5 py-px text-[10px]"
                            >
                              <span className="opacity-70">{k}:</span>
                              <span className="font-mono">
                                {typeof val === 'string' ? val : JSON.stringify(val)}
                              </span>
                            </span>
                          ))}
                        </span>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
