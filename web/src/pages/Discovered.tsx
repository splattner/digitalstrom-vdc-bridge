import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowDownUp, ArrowDown, ArrowUp, Search, X,
  Activity, Antenna, BatteryMedium, Bell, ChevronDown, ChevronLeft, ChevronRight,
  CircleDot, Droplet, Gauge,
  Layers, Lightbulb, Link2, Link2Off, MousePointerClick, Palette, Sparkles, Sun,
  Thermometer, ToggleLeft, Unlink, Zap,
} from 'lucide-react'
import {
  api,
  connectEvents,
  type DiscoveredEntity, type Plugin, type WsEvent,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { useToasts } from '@/lib/toasts'
import { clusterSiblings, siblingKeyOf, type SiblingInfo } from '@/lib/siblings'

// ── Types ─────────────────────────────────────────────────────────────────────

interface Row extends DiscoveredEntity {
  pluginId: string
  pluginStatus: string
}

type SortKey = 'name' | 'remoteId' | 'plugin' | 'kind' | 'mapped'
type SortDir = 'asc' | 'desc'

const KIND_ORDER: Record<string, number> = {
  colorlight: 0, dimmer: 1, light: 2, sensor: 3, binary: 4, button: 5,
}

const ROW_KEY = (r: Pick<Row, 'pluginId' | 'id'>) => `${r.pluginId}\u0000${r.id}`

// ── Visual helpers ────────────────────────────────────────────────────────────

function kindBadge(kind: string): string {
  switch (kind) {
    case 'colorlight': return 'bg-fuchsia-500/10 text-fuchsia-700 dark:text-fuchsia-300 border-fuchsia-500/30'
    case 'dimmer':     return 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/30'
    case 'light':      return 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-300 border-yellow-500/30'
    case 'sensor':     return 'bg-sky-500/10 text-sky-700 dark:text-sky-300 border-sky-500/30'
    case 'binary':     return 'bg-slate-500/10 text-slate-700 dark:text-slate-300 border-slate-500/30'
    case 'button':     return 'bg-violet-500/10 text-violet-700 dark:text-violet-300 border-violet-500/30'
    default:           return 'bg-muted text-muted-foreground border-border'
  }
}

function pluginBadge(status: string): string {
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

// Icon picked from kind, refined with light keyword sniffing on entity-id /
// attributes (used for the sensor sub-types). The colour matches the kind.
function deviceIconFor(kind: string, id: string, attrs?: Record<string, unknown>):
  { Icon: typeof Lightbulb; tone: string } {
  const tone = ((): string => {
    switch (kind) {
      case 'colorlight': return 'bg-fuchsia-500/10 text-fuchsia-600 dark:text-fuchsia-400'
      case 'dimmer':     return 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
      case 'light':      return 'bg-yellow-500/10 text-yellow-600 dark:text-yellow-400'
      case 'sensor':     return 'bg-sky-500/10 text-sky-600 dark:text-sky-400'
      case 'binary':     return 'bg-slate-500/10 text-slate-600 dark:text-slate-400'
      case 'button':     return 'bg-violet-500/10 text-violet-600 dark:text-violet-400'
      default:           return 'bg-muted text-muted-foreground'
    }
  })()
  if (kind === 'colorlight') return { Icon: Palette, tone }
  if (kind === 'dimmer' || kind === 'light') return { Icon: Lightbulb, tone }
  if (kind === 'binary') return { Icon: ToggleLeft, tone }
  if (kind === 'button') return { Icon: MousePointerClick, tone }
  if (kind === 'sensor') {
    const hay = (id + ' ' + Object.values(attrs ?? {}).join(' ')).toLowerCase()
    if (/battery|batt/.test(hay)) return { Icon: BatteryMedium, tone }
    if (/motion|occupancy|presence|pir/.test(hay)) return { Icon: Activity, tone }
    if (/temp/.test(hay)) return { Icon: Thermometer, tone }
    if (/humid/.test(hay)) return { Icon: Droplet, tone }
    if (/power|energy|watt|consumption/.test(hay)) return { Icon: Zap, tone }
    if (/illum|lux|light_level/.test(hay)) return { Icon: Sun, tone }
    if (/door|window|contact|open/.test(hay)) return { Icon: Bell, tone }
    return { Icon: Gauge, tone }
  }
  return { Icon: CircleDot, tone }
}

// ── Toolbar pieces ────────────────────────────────────────────────────────────

function Chip({
  active, label, count, onClick,
}: { active: boolean; label: string; count?: number; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs transition-colors ${
        active
          ? 'border-primary bg-primary text-primary-foreground'
          : 'border-border bg-background hover:bg-muted text-muted-foreground'
      }`}
    >
      <span>{label}</span>
      {typeof count === 'number' && (
        <span className={`tabular-nums text-[10px] ${active ? 'opacity-80' : 'opacity-60'}`}>{count}</span>
      )}
    </button>
  )
}

function SortHeader({
  label, columnKey, sort, setSort, className,
}: {
  label: string
  columnKey: SortKey
  sort: { key: SortKey; dir: SortDir }
  setSort: (s: { key: SortKey; dir: SortDir }) => void
  className?: string
}) {
  const active = sort.key === columnKey
  const Icon = !active ? ArrowDownUp : sort.dir === 'asc' ? ArrowUp : ArrowDown
  return (
    <th className={`text-left px-3 py-2 font-medium ${className ?? ''}`}>
      <button
        type="button"
        onClick={() => setSort({
          key: columnKey,
          dir: active && sort.dir === 'asc' ? 'desc' : 'asc',
        })}
        className={`inline-flex items-center gap-1.5 select-none -ml-1 px-1 rounded hover:bg-muted ${active ? 'text-foreground' : 'text-muted-foreground'}`}
      >
        <span>{label}</span>
        <Icon className="size-3" />
      </button>
    </th>
  )
}

// ── KPI card ──────────────────────────────────────────────────────────────────

function KpiCard({
  label, value, hint, Icon, tone, accent,
}: {
  label: string
  value: number | string
  hint?: string
  Icon: typeof Lightbulb
  tone: string
  /** Tailwind border-color class for the left accent stripe, e.g. 'border-l-sky-500'. */
  accent: string
}) {
  return (
    <div
      className={`relative overflow-hidden rounded-lg border border-l-4 ${accent} bg-card px-3 py-2.5 flex items-center gap-3`}
    >
      {/* Faded background icon */}
      <Icon
        aria-hidden
        className={`absolute -right-2 -bottom-2 size-14 opacity-[0.06] ${tone.split(' ').filter((c) => c.startsWith('text-')).join(' ')}`}
      />
      <div className={`shrink-0 rounded-md p-1.5 ${tone}`}>
        <Icon className="size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-[10px] uppercase tracking-wider text-muted-foreground leading-none">{label}</div>
        <div className="mt-0.5 text-2xl font-semibold tabular-nums leading-none">{value}</div>
        {hint && (
          <div className="mt-1 text-[11px] text-muted-foreground truncate leading-none">{hint}</div>
        )}
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function DiscoveredPage() {
  const qc = useQueryClient()
  const pushToast = useToasts((s) => s.push)

  // Plugins (every 5 s).
  const { data: plugins, isLoading: loadingPlugins } = useQuery({
    queryKey: ['plugins'],
    queryFn: api.plugins,
    refetchInterval: 5_000,
  })

  // Discovered entities for every plugin (one parallel query each). The
  // interval here is just a resilience backstop for a missed WS event (e.g.
  // a reconnect gap) — normally a plugin's list is refreshed immediately via
  // the discoveryChanged event below.
  const discoveredQueries = useQueries({
    queries: (plugins ?? []).map((p: Plugin) => ({
      queryKey: ['discovered', p.id],
      queryFn: () => api.discovered(p.id),
      refetchInterval: 60_000,
      retry: false,
    })),
  })

  // Refresh a plugin's discovered list immediately when it reports a change,
  // instead of waiting for the backstop poll above.
  useEffect(() => {
    return connectEvents((e: WsEvent) => {
      if (e.type === 'discoveryChanged') {
        const pluginId = (e.data as { pluginId?: string } | undefined)?.pluginId
        void qc.invalidateQueries({ queryKey: pluginId ? ['discovered', pluginId] : ['discovered'] })
      }
    })
  }, [qc])

  // Currently active bridges (for resolving DSUID on un-bridge).
  const { data: bridges } = useQuery({
    queryKey: ['bridges'],
    queryFn: api.bridges,
  })

  const dsuidByRemote = useMemo(() => {
    const map = new Map<string, string>()
    for (const b of bridges ?? []) map.set(`${b.pluginId}\u0000${b.remoteEntityId}`, b.dsuid)
    return map
  }, [bridges])

  // Flat row list (sort handled below).
  const rows: Row[] = useMemo(() => {
    if (!plugins) return []
    const out: Row[] = []
    plugins.forEach((p, i) => {
      const q = discoveredQueries[i]
      const data = q?.data
      if (!data) return
      for (const e of data) {
        out.push({ ...e, pluginId: p.id, pluginStatus: p.status })
      }
    })
    return out
  }, [plugins, discoveredQueries])

  // ── Filters / sort / selection state ─────────────────────────────────────
  const [query, setQuery] = useState('')
  const [pluginFilter, setPluginFilter] = useState<Set<string>>(new Set())
  const [kindFilter, setKindFilter] = useState<Set<string>>(new Set())
  const [areaFilter, setAreaFilter] = useState<Set<string>>(new Set())
  const [statusFilter, setStatusFilter] = useState<'all' | 'mapped' | 'unmapped'>('all')
  const [sort, setSort] = useState<{ key: SortKey; dir: SortDir }>({ key: 'name', dir: 'asc' })
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [groupBy, setGroupBy] = useState<'none' | 'plugin' | 'kind'>('none')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [confirmUnbridgeOpen, setConfirmUnbridgeOpen] = useState(false)
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState<number>(50)

  // ── Counts (computed from the unfiltered set, for chip badges) ───────────
  const counts = useMemo(() => {
    const byPlugin: Record<string, number> = {}
    const byKind: Record<string, number> = {}
    const byArea: Record<string, number> = {}
    let mapped = 0
    for (const r of rows) {
      byPlugin[r.pluginId] = (byPlugin[r.pluginId] ?? 0) + 1
      byKind[r.kind] = (byKind[r.kind] ?? 0) + 1
      const area = typeof (r.attributes as Record<string, unknown> | undefined)?.area === 'string'
        ? (r.attributes as Record<string, string>).area
        : ''
      if (area) byArea[area] = (byArea[area] ?? 0) + 1
      if (r.mapped) mapped++
    }
    return { byPlugin, byKind, byArea, mapped, total: rows.length, unmapped: rows.length - mapped }
  }, [rows])

  // ── Filter + sort pipeline ───────────────────────────────────────────────
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const list = rows.filter((r) => {
      if (pluginFilter.size > 0 && !pluginFilter.has(r.pluginId)) return false
      if (kindFilter.size > 0 && !kindFilter.has(r.kind)) return false
      if (areaFilter.size > 0) {
        const area = typeof (r.attributes as Record<string, unknown> | undefined)?.area === 'string'
          ? (r.attributes as Record<string, string>).area
          : ''
        if (!areaFilter.has(area)) return false
      }
      if (statusFilter === 'mapped' && !r.mapped) return false
      if (statusFilter === 'unmapped' && r.mapped) return false
      if (q && !r.name.toLowerCase().includes(q) && !r.id.toLowerCase().includes(q)) {
        return false
      }
      return true
    })
    const dir = sort.dir === 'asc' ? 1 : -1
    list.sort((a, b) => {
      switch (sort.key) {
        case 'name':   return a.name.localeCompare(b.name) * dir
        case 'remoteId': return a.id.localeCompare(b.id) * dir
        case 'plugin': return (a.pluginId.localeCompare(b.pluginId) || a.name.localeCompare(b.name)) * dir
        case 'kind': {
          const ak = KIND_ORDER[a.kind] ?? 99
          const bk = KIND_ORDER[b.kind] ?? 99
          if (ak !== bk) return (ak - bk) * dir
          return a.name.localeCompare(b.name) * dir
        }
        case 'mapped':
          if (a.mapped !== b.mapped) return (a.mapped ? -1 : 1) * dir
          return a.name.localeCompare(b.name)
      }
    })
    return list
  }, [rows, query, pluginFilter, kindFilter, areaFilter, statusFilter, sort])

  // Cluster sibling rows (e.g. Z2M split-button entities sharing an IEEE) so
  // they always appear adjacent regardless of the active sort/group choice.
  // Done before pagination so siblings never get split across pages.
  const { ordered: filteredOrdered, siblingInfo } = useMemo(
    () => clusterSiblings(
      filtered,
      (r) => siblingKeyOf({ pluginId: r.pluginId, remoteEntityId: r.id }),
      (r) => ROW_KEY(r),
    ),
    [filtered],
  )

  // Selections for rows no longer visible under the current filter are left
  // in `selected` rather than pruned — every display/count below reads
  // selectedRows (selected ∩ filtered) instead of `selected` directly, so a
  // stale entry never affects the UI, and it naturally rejoins the count if
  // the filter changes back.

  // Reset to page 1 when the visible set changes meaningfully. Done as a
  // render-time state adjustment (see https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes)
  // rather than an effect, since it's just resetting local state in response
  // to other state changing.
  const pageResetKey = JSON.stringify([query, pluginFilter, kindFilter, areaFilter, statusFilter, groupBy, pageSize, sort.key, sort.dir])
  const [prevPageResetKey, setPrevPageResetKey] = useState(pageResetKey)
  if (pageResetKey !== prevPageResetKey) {
    setPrevPageResetKey(pageResetKey)
    setPage(0)
  }

  // ── Mutations ────────────────────────────────────────────────────────────
  const createMut = useMutation({
    mutationFn: api.createBridge,
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['discovered', vars.pluginId] })
      void qc.invalidateQueries({ queryKey: ['bridges'] })
      void qc.invalidateQueries({ queryKey: ['devices'] })
    },
    onError: (e: unknown) =>
      pushToast(`Bridge failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  const deleteMut = useMutation({
    mutationFn: api.deleteBridge,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['discovered'] })
      void qc.invalidateQueries({ queryKey: ['bridges'] })
      void qc.invalidateQueries({ queryKey: ['devices'] })
    },
    onError: (e: unknown) =>
      pushToast(`Un-bridge failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  // Bulk worker — runs sequentially so we never overwhelm a plugin.
  const [bulkBusy, setBulkBusy] = useState(false)

  async function bulkBridge() {
    const targets = filtered.filter((r) => selected.has(ROW_KEY(r)) && !r.mapped)
    if (targets.length === 0) return
    setBulkBusy(true)
    let ok = 0, fail = 0
    for (const r of targets) {
      try {
        await api.createBridge({
          pluginId: r.pluginId, remoteEntityId: r.id, name: r.name, kind: r.kind,
        })
        ok++
      } catch {
        fail++
      }
    }
    setBulkBusy(false)
    setSelected(new Set())
    void qc.invalidateQueries({ queryKey: ['discovered'] })
    void qc.invalidateQueries({ queryKey: ['bridges'] })
    void qc.invalidateQueries({ queryKey: ['devices'] })
    pushToast(
      fail === 0 ? `Bridged ${ok} ${ok === 1 ? 'entity' : 'entities'}`
                 : `Bridged ${ok}, ${fail} failed`,
      fail === 0 ? 'success' : 'error',
    )
  }

  function bulkUnbridge() {
    const count = filtered.filter((r) => selected.has(ROW_KEY(r)) && r.mapped).length
    if (count === 0) return
    setConfirmUnbridgeOpen(true)
  }

  function toggleExpanded(key: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key); else next.add(key)
      return next
    })
  }

  async function executeBulkUnbridge() {
    const targets = filtered.filter((r) => selected.has(ROW_KEY(r)) && r.mapped)
    if (targets.length === 0) return
    setBulkBusy(true)
    let ok = 0, fail = 0
    for (const r of targets) {
      const dsuid = dsuidByRemote.get(ROW_KEY(r))
      if (!dsuid) { fail++; continue }
      try {
        await api.deleteBridge(dsuid)
        ok++
      } catch {
        fail++
      }
    }
    setBulkBusy(false)
    setSelected(new Set())
    setConfirmUnbridgeOpen(false)
    void qc.invalidateQueries({ queryKey: ['discovered'] })
    void qc.invalidateQueries({ queryKey: ['bridges'] })
    void qc.invalidateQueries({ queryKey: ['devices'] })
    pushToast(
      fail === 0 ? `Un-bridged ${ok} ${ok === 1 ? 'entity' : 'entities'}`
                 : `Un-bridged ${ok}, ${fail} failed`,
      fail === 0 ? 'success' : 'error',
    )
  }

  // ── Selection helpers ────────────────────────────────────────────────────
  const visibleKeys = useMemo(() => filtered.map(ROW_KEY), [filtered])
  const allVisibleSelected = visibleKeys.length > 0 && visibleKeys.every((k) => selected.has(k))
  const someVisibleSelected = !allVisibleSelected && visibleKeys.some((k) => selected.has(k))

  function toggleAllVisible() {
    setSelected((prev) => {
      const next = new Set(prev)
      if (allVisibleSelected) {
        for (const k of visibleKeys) next.delete(k)
      } else {
        for (const k of visibleKeys) next.add(k)
      }
      return next
    })
  }

  function toggleRow(key: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key); else next.add(key)
      return next
    })
  }

  const selectedRows = useMemo(
    () => filtered.filter((r) => selected.has(ROW_KEY(r))),
    [filtered, selected],
  )
  const selBridgeable = selectedRows.filter((r) => !r.mapped).length
  const selUnbridgeable = selectedRows.filter((r) => r.mapped).length

  function toggleSet<T>(set: Set<T>, value: T): Set<T> {
    const next = new Set(set)
    if (next.has(value)) next.delete(value)
    else next.add(value)
    return next
  }

  const anyDiscoveryError = discoveredQueries.some((q) => q.error)
  const allDiscoveryLoading = (plugins?.length ?? 0) > 0 && discoveredQueries.every((q) => q.isLoading)
  const filtersActive =
    query.length > 0 || pluginFilter.size > 0 || kindFilter.size > 0 || areaFilter.size > 0 || statusFilter !== 'all'

  // ── Render ───────────────────────────────────────────────────────────────
  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-baseline justify-between">
        <h1 className="text-lg font-semibold text-foreground">Discovered</h1>
      </div>

      {/* KPI row */}
      <div className="grid grid-cols-3 gap-2">
        <KpiCard
          label="Total discovered"
          value={counts.total}
          hint="All entities found"
          Icon={Antenna}
          tone="bg-sky-500/10 text-sky-600 dark:text-sky-400"
          accent="border-l-sky-500"
        />
        <KpiCard
          label="New"
          value={counts.unmapped}
          hint={counts.unmapped > 0 ? 'Awaiting bridging' : 'Nothing new'}
          Icon={Sparkles}
          tone="bg-blue-500/10 text-blue-600 dark:text-blue-400"
          accent="border-l-blue-500"
        />
        <KpiCard
          label="Bridged"
          value={counts.mapped}
          hint={counts.total > 0 ? `${Math.round((counts.mapped / counts.total) * 100)}% of total` : 'None yet'}
          Icon={Link2}
          tone="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
          accent="border-l-emerald-500"
        />
      </div>

      {/* Toolbar */}
      <div className="rounded-lg border bg-card/30 p-3 space-y-2.5">
        {/* Row 1: search + status segmented control */}
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative flex-1 min-w-[200px]">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search by name or remote id…"
              className="w-full rounded-md border bg-background pl-7 pr-7 py-1.5 text-sm outline-none focus:border-ring focus:ring-3 focus:ring-ring/30"
            />
            {query && (
              <button
                type="button"
                onClick={() => setQuery('')}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                aria-label="Clear search"
              >
                <X className="size-3.5" />
              </button>
            )}
          </div>

          <div className="inline-flex rounded-md border overflow-hidden text-xs">
            {(['all', 'unmapped', 'mapped'] as const).map((v) => (
              <button
                key={v}
                type="button"
                onClick={() => setStatusFilter(v)}
                className={`px-2.5 py-1 transition-colors ${
                  statusFilter === v
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-background text-muted-foreground hover:bg-muted'
                }`}
              >
                {v === 'all' ? `All (${counts.total})`
                  : v === 'mapped' ? `Bridged (${counts.mapped})`
                  : `New (${counts.unmapped})`}
              </button>
            ))}
          </div>

          <div className="inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs">
            <Layers className="size-3 text-muted-foreground" />
            <span className="text-muted-foreground">Group:</span>
            {(['none', 'plugin', 'kind'] as const).map((v) => (
              <button
                key={v}
                type="button"
                onClick={() => setGroupBy(v)}
                className={`rounded px-1.5 py-0.5 transition-colors ${
                  groupBy === v
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-muted'
                }`}
              >
                {v === 'none' ? 'None' : v === 'plugin' ? 'Plugin' : 'Kind'}
              </button>
            ))}
          </div>

          {filtersActive && (
            <button
              type="button"
              onClick={() => {
                setQuery('')
                setPluginFilter(new Set())
                setKindFilter(new Set())
                setAreaFilter(new Set())
                setStatusFilter('all')
              }}
              className="text-xs text-muted-foreground hover:text-foreground border rounded px-2 py-1"
            >
              Reset
            </button>
          )}
        </div>

        {/* Row 2: plugin chips */}
        {plugins && plugins.length > 0 && (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-[10px] uppercase tracking-wider text-muted-foreground mr-1">Plugin</span>
            {[...(plugins ?? [])].sort((a, b) => a.id.localeCompare(b.id)).map((p) => (
              <Chip
                key={p.id}
                label={p.id}
                count={counts.byPlugin[p.id] ?? 0}
                active={pluginFilter.has(p.id)}
                onClick={() => setPluginFilter((s) => toggleSet(s, p.id))}
              />
            ))}
          </div>
        )}

        {/* Row 3: kind chips */}
        {Object.keys(counts.byKind).length > 0 && (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-[10px] uppercase tracking-wider text-muted-foreground mr-1">Kind</span>
            {Object.entries(counts.byKind)
              .sort(([a], [b]) => (KIND_ORDER[a] ?? 99) - (KIND_ORDER[b] ?? 99))
              .map(([kind, n]) => (
                <Chip
                  key={kind}
                  label={kind}
                  count={n}
                  active={kindFilter.has(kind)}
                  onClick={() => setKindFilter((s) => toggleSet(s, kind))}
                />
              ))}
          </div>
        )}

        {/* Row 4: area chips (only when any plugin reports areas) */}
        {Object.keys(counts.byArea).length > 0 && (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-[10px] uppercase tracking-wider text-muted-foreground mr-1">Area</span>
            {Object.entries(counts.byArea)
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([area, n]) => (
                <Chip
                  key={area}
                  label={area}
                  count={n}
                  active={areaFilter.has(area)}
                  onClick={() => setAreaFilter((s) => toggleSet(s, area))}
                />
              ))}
          </div>
        )}
      </div>

      {/* Bulk action bar (sticky above the table when something is selected).
          Counts use selectedRows (selected ∩ filtered) rather than raw
          selected.size, so stale selections for rows hidden by the current
          filter don't inflate the count. */}
      {selectedRows.length > 0 && (
        <div className="sticky top-0 z-10 flex flex-wrap items-center gap-2 rounded-lg border bg-primary/10 border-primary/30 px-3 py-2 text-sm">
          <span className="font-medium">{selectedRows.length} selected</span>
          <span className="text-xs text-muted-foreground">
            ({selBridgeable} new · {selUnbridgeable} bridged)
          </span>
          <div className="flex-1" />
          <Button
            size="sm"
            disabled={bulkBusy || selBridgeable === 0}
            onClick={() => void bulkBridge()}
          >
            <Link2 className="size-3.5" /> Bridge selected ({selBridgeable})
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={bulkBusy || selUnbridgeable === 0}
            onClick={() => void bulkUnbridge()}
          >
            <Unlink className="size-3.5" /> Un-bridge selected ({selUnbridgeable})
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={bulkBusy}
            onClick={() => setSelected(new Set())}
          >
            Clear
          </Button>
        </div>
      )}

      {/* Body */}
      {loadingPlugins || allDiscoveryLoading ? (
        <SkeletonTable />
      ) : !plugins || plugins.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No plugins configured. See the <span className="font-medium">Plugins</span> page for an example
          <code className="font-mono mx-1 px-1 py-0.5 bg-muted rounded text-xs">plugins.json</code>.
        </p>
      ) : rows.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No entities discovered yet.{' '}
          {anyDiscoveryError && '(Some plugins reported errors — see the Plugins page.)'}
        </p>
      ) : filtered.length === 0 ? (
        <p className="text-muted-foreground text-sm">No entities match the current filters.</p>
      ) : (() => {
        const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize))
        const safePage = Math.min(page, totalPages - 1)
        const start = safePage * pageSize
        const paged = filteredOrdered.slice(start, start + pageSize)

        // Build group buckets in iteration order of `paged` so sort is preserved.
        const groups: { key: string; label: string; items: Row[] }[] = []
        const groupIdx = new Map<string, number>()
        for (const r of paged) {
          const k = groupBy === 'plugin' ? r.pluginId : groupBy === 'kind' ? r.kind : '__all'
          let idx = groupIdx.get(k)
          if (idx === undefined) {
            idx = groups.length
            groupIdx.set(k, idx)
            groups.push({ key: k, label: k, items: [] })
          }
          groups[idx].items.push(r)
        }

        const busy = createMut.isPending || deleteMut.isPending || bulkBusy

        return (
          <>
            <div className="border rounded-lg overflow-hidden">
              <table className="w-full text-sm table-fixed">
                <colgroup>
                  <col className="w-9" />
                  <col className="w-9" />
                  <col />
                  <col className="w-56" />
                  <col className="w-36 hidden sm:table-column" />
                  <col className="w-24" />
                  <col className="w-28" />
                  <col className="w-28" />
                </colgroup>
                <thead>
                  <tr className="border-b bg-muted/50">
                    <th className="px-2 py-2">
                      <input
                        type="checkbox"
                        aria-label="Select all visible"
                        checked={allVisibleSelected}
                        ref={(el) => { if (el) el.indeterminate = someVisibleSelected }}
                        onChange={toggleAllVisible}
                        className="cursor-pointer"
                      />
                    </th>
                    <th className="px-1 py-2" />
                    <SortHeader label="Name" columnKey="name" sort={sort} setSort={setSort} />
                    <SortHeader label="Remote ID" columnKey="remoteId" sort={sort} setSort={setSort} />
                    <SortHeader label="Plugin" columnKey="plugin" sort={sort} setSort={setSort} className="hidden sm:table-cell" />
                    <SortHeader label="Kind" columnKey="kind" sort={sort} setSort={setSort} />
                    <SortHeader label="Status" columnKey="mapped" sort={sort} setSort={setSort} />
                    <th className="px-3 py-2 text-right font-medium text-muted-foreground">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {groups.map((g) => (
                    <GroupBlock
                      key={g.key}
                      group={g}
                      groupBy={groupBy}
                      counts={counts}
                      selected={selected}
                      expanded={expanded}
                      busy={busy}
                      dsuidByRemote={dsuidByRemote}
                      siblingInfo={siblingInfo}
                      onToggleRow={toggleRow}
                      onToggleExpand={toggleExpanded}
                      onBridgeOne={(r) => createMut.mutate({
                        pluginId: r.pluginId, remoteEntityId: r.id, name: r.name, kind: r.kind,
                      })}
                      onUnbridgeOne={(r) => {
                        const dsuid = dsuidByRemote.get(ROW_KEY(r))
                        if (dsuid) deleteMut.mutate(dsuid)
                      }}
                      onBridgeAllNew={(items) => {
                        const targets = items.filter((r) => !r.mapped)
                        for (const r of targets) {
                          createMut.mutate({
                            pluginId: r.pluginId, remoteEntityId: r.id, name: r.name, kind: r.kind,
                          })
                        }
                      }}
                    />
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination footer */}
            <div className="flex flex-wrap items-center justify-between gap-2 px-1 text-xs text-muted-foreground">
              <div>
                Showing <span className="tabular-nums text-foreground">{start + 1}</span>–
                <span className="tabular-nums text-foreground">{Math.min(start + pageSize, filtered.length)}</span>{' '}
                of <span className="tabular-nums text-foreground">{filtered.length}</span>
                {filtered.length !== counts.total && (
                  <span className="ml-1">(filtered from {counts.total})</span>
                )}
              </div>
              <div className="flex items-center gap-2">
                <label className="flex items-center gap-1.5">
                  <span>Per page</span>
                  <select
                    value={pageSize}
                    onChange={(e) => setPageSize(Number(e.target.value))}
                    className="rounded border bg-background px-1.5 py-0.5 text-xs"
                  >
                    {[25, 50, 100, 200].map((n) => (
                      <option key={n} value={n}>{n}</option>
                    ))}
                  </select>
                </label>
                <div className="inline-flex items-center rounded-md border overflow-hidden">
                  <button
                    type="button"
                    onClick={() => setPage((p) => Math.max(0, p - 1))}
                    disabled={safePage === 0}
                    className="px-2 py-1 hover:bg-muted disabled:opacity-40 disabled:hover:bg-transparent"
                    aria-label="Previous page"
                  >
                    <ChevronLeft className="size-3.5" />
                  </button>
                  <span className="px-2 py-1 tabular-nums border-x">
                    {safePage + 1} / {totalPages}
                  </span>
                  <button
                    type="button"
                    onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
                    disabled={safePage >= totalPages - 1}
                    className="px-2 py-1 hover:bg-muted disabled:opacity-40 disabled:hover:bg-transparent"
                    aria-label="Next page"
                  >
                    <ChevronRight className="size-3.5" />
                  </button>
                </div>
              </div>
            </div>
          </>
        )
      })()}

      {/* Confirm un-bridge modal */}
      {confirmUnbridgeOpen && (
        <ConfirmUnbridgeModal
          targets={selectedRows.filter((r) => r.mapped)}
          busy={bulkBusy}
          onCancel={() => setConfirmUnbridgeOpen(false)}
          onConfirm={() => void executeBulkUnbridge()}
        />
      )}
    </div>
  )
}

// ── Group block & row ────────────────────────────────────────────────────────

function GroupBlock({
  group, groupBy, counts, selected, expanded, busy, dsuidByRemote, siblingInfo,
  onToggleRow, onToggleExpand, onBridgeOne, onUnbridgeOne, onBridgeAllNew,
}: {
  group: { key: string; label: string; items: Row[] }
  groupBy: 'none' | 'plugin' | 'kind'
  counts: { byPlugin: Record<string, number>; byKind: Record<string, number> }
  selected: Set<string>
  expanded: Set<string>
  busy: boolean
  dsuidByRemote: Map<string, string>
  siblingInfo: Map<string, SiblingInfo>
  onToggleRow: (key: string) => void
  onToggleExpand: (key: string) => void
  onBridgeOne: (r: Row) => void
  onUnbridgeOne: (r: Row) => void
  onBridgeAllNew: (items: Row[]) => void
}) {
  const newCount = group.items.filter((r) => !r.mapped).length
  const bridgedCount = group.items.length - newCount
  const total = groupBy === 'plugin'
    ? counts.byPlugin[group.key] ?? group.items.length
    : groupBy === 'kind'
      ? counts.byKind[group.key] ?? group.items.length
      : group.items.length

  return (
    <>
      {groupBy !== 'none' && (
        <tr className="bg-muted/30 border-b">
          <td colSpan={8} className="px-3 py-1.5">
            <div className="flex items-center gap-2 text-xs">
              <span className="text-[10px] uppercase tracking-wider text-muted-foreground">
                {groupBy}
              </span>
              <span className="font-medium">{group.label}</span>
              <span className="text-muted-foreground tabular-nums">
                · {group.items.length}{group.items.length !== total ? ` / ${total}` : ''}
              </span>
              <span className="text-muted-foreground">
                ({newCount} new · {bridgedCount} bridged)
              </span>
              <div className="flex-1" />
              {newCount > 0 && (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => onBridgeAllNew(group.items)}
                >
                  <Link2 className="size-3.5" /> Bridge all new ({newCount})
                </Button>
              )}
            </div>
          </td>
        </tr>
      )}
      {group.items.map((r) => (
        <EntityRow
          key={ROW_KEY(r)}
          row={r}
          selected={selected.has(ROW_KEY(r))}
          expanded={expanded.has(ROW_KEY(r))}
          busy={busy}
          dsuidByRemote={dsuidByRemote}
          sibling={siblingInfo.get(ROW_KEY(r))}
          onToggleSelect={() => onToggleRow(ROW_KEY(r))}
          onToggleExpand={() => onToggleExpand(ROW_KEY(r))}
          onBridge={() => onBridgeOne(r)}
          onUnbridge={() => onUnbridgeOne(r)}
        />
      ))}
    </>
  )
}

function EntityRow({
  row, selected, expanded, busy, dsuidByRemote, sibling,
  onToggleSelect, onToggleExpand, onBridge, onUnbridge,
}: {
  row: Row
  selected: boolean
  expanded: boolean
  busy: boolean
  dsuidByRemote: Map<string, string>
  sibling?: SiblingInfo
  onToggleSelect: () => void
  onToggleExpand: () => void
  onBridge: () => void
  onUnbridge: () => void
}) {
  const { Icon: DevIcon, tone: devTone } = deviceIconFor(row.kind, row.id, row.attributes)
  const a = (row.attributes ?? {}) as Record<string, unknown>
  const str = (k: string) => {
    const v = a[k]
    return typeof v === 'string' || typeof v === 'number' ? String(v) : ''
  }
  const device = str('device')
  const area   = str('area')
  const model  = str('model')
  const ip     = str('ip') || str('addr')
  const sw     = str('sw') || str('ver')
  const summaryParts: string[] = []
  if (device && device !== row.name) summaryParts.push(device)
  if (area) summaryParts.push(area)
  if (model) summaryParts.push(model)
  if (ip) summaryParts.push(ip)
  if (sw) summaryParts.push(`v${sw.replace(/^v/i, '')}`)

  const dsuid = dsuidByRemote.get(ROW_KEY(row))

  return (
    <>
      <tr
        className={`border-b last:border-0 hover:bg-muted/30 transition-colors cursor-pointer ${
          row.mapped ? 'bg-emerald-500/[0.04]' : ''
        } ${selected ? 'bg-primary/[0.06]' : ''} ${expanded ? 'bg-muted/40' : ''}`}
        onClick={onToggleExpand}
      >
        <td
          className="pl-3 pr-1 py-2 align-middle"
          style={sibling ? { boxShadow: `inset 4px 0 0 ${sibling.color}` } : undefined}
          title={sibling ? `Part of ${sibling.prefix} (${sibling.size} entities on the same physical hardware)` : undefined}
          onClick={(e) => e.stopPropagation()}
        >
          <input
            type="checkbox"
            aria-label={`Select ${row.name}`}
            checked={selected}
            onChange={onToggleSelect}
            className="cursor-pointer"
          />
        </td>
        <td className="px-1 py-2 align-middle">
          <div className={`size-7 rounded-md grid place-items-center ${devTone}`}>
            <DevIcon className="size-4" />
          </div>
        </td>
        <td className="px-3 py-2 min-w-0">
          <div className="flex items-center gap-1.5 min-w-0">
            <ChevronDown
              className={`size-3.5 shrink-0 text-muted-foreground transition-transform ${
                expanded ? '' : '-rotate-90'
              }`}
            />
            <div className="font-medium truncate" title={row.name}>{row.name}</div>
            {sibling && (
              <span
                className="inline-flex items-center gap-0.5 rounded px-1 py-0.5 text-[10px] font-mono leading-none shrink-0"
                style={{ color: sibling.color, backgroundColor: `${sibling.color}1a` }}
                title={`Part of ${sibling.prefix} — ${sibling.size} entities on the same physical device`}
              >
                <Link2 className="size-3" />
                {sibling.index + 1}/{sibling.size}
              </span>
            )}
          </div>
          {summaryParts.length > 0 && (
            <div className="ml-5 text-[11px] text-muted-foreground truncate">
              {summaryParts.join(' · ')}
            </div>
          )}
        </td>
        <td className="px-3 py-2">
          <div className="font-mono text-[11px] text-muted-foreground truncate" title={row.id}>
            {row.id}
          </div>
        </td>
        <td className="px-3 py-2 hidden sm:table-cell">
          <span
            className={`inline-block max-w-full truncate rounded-md border px-1.5 py-0.5 text-[11px] font-mono ${pluginBadge(row.pluginStatus)}`}
            title={`${row.pluginId} · ${row.pluginStatus}`}
          >
            {row.pluginId}
          </span>
        </td>
        <td className="px-3 py-2">
          <span className={`inline-block rounded-md border px-1.5 py-0.5 text-[11px] ${kindBadge(row.kind)}`}>
            {row.kind}
          </span>
        </td>
        <td className="px-3 py-2">
          {row.mapped ? (
            <span className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border-emerald-500/30">
              <span className="size-1.5 rounded-full bg-current opacity-70" />
              Bridged
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] bg-blue-500/10 text-blue-700 dark:text-blue-300 border-blue-500/30">
              <span className="size-1.5 rounded-full bg-current opacity-70" />
              New
            </span>
          )}
        </td>
        <td
          className="px-3 py-2 text-right"
          onClick={(e) => e.stopPropagation()}
        >
          {row.mapped ? (
            <Button
              size="sm"
              variant="ghost"
              className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
              disabled={busy}
              onClick={onUnbridge}
              title="Un-bridge"
            >
              <Link2Off className="h-4 w-4" />
            </Button>
          ) : (
            <Button
              size="sm"
              variant="ghost"
              className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
              disabled={busy}
              onClick={onBridge}
              title="Bridge"
            >
              <Link2 className="h-4 w-4" />
            </Button>
          )}
        </td>
      </tr>
      {expanded && (
        <tr className="border-b bg-muted/20">
          <td colSpan={8} className="px-4 py-3">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
              <div>
                <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-1.5">
                  Identity
                </div>
                <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
                  <dt className="text-muted-foreground">Plugin</dt>
                  <dd className="font-mono">{row.pluginId}</dd>
                  <dt className="text-muted-foreground">Remote ID</dt>
                  <dd className="font-mono break-all">{row.id}</dd>
                  <dt className="text-muted-foreground">Kind</dt>
                  <dd>{row.kind}</dd>
                  <dt className="text-muted-foreground">Status</dt>
                  <dd>{row.mapped ? 'Bridged' : 'New'}</dd>
                  {row.mapped && dsuid && (
                    <>
                      <dt className="text-muted-foreground">DSUID</dt>
                      <dd className="font-mono break-all">{dsuid}</dd>
                    </>
                  )}
                </dl>
              </div>
              <div>
                <div className="text-[10px] uppercase tracking-wider text-muted-foreground mb-1.5">
                  Attributes
                </div>
                {Object.keys(a).length === 0 ? (
                  <div className="text-muted-foreground italic">None reported.</div>
                ) : (
                  <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
                    {Object.entries(a).map(([k, v]) => (
                      <span key={k} className="contents">
                        <dt className="text-muted-foreground font-mono text-[11px]">{k}</dt>
                        <dd className="font-mono text-[11px] break-all">
                          {v == null
                            ? <span className="italic text-muted-foreground">null</span>
                            : typeof v === 'object'
                              ? JSON.stringify(v)
                              : String(v)}
                        </dd>
                      </span>
                    ))}
                  </dl>
                )}
              </div>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

// ── Skeleton loading ─────────────────────────────────────────────────────────

function SkeletonTable() {
  return (
    <div className="border rounded-lg overflow-hidden">
      <div className="border-b bg-muted/50 h-9" />
      <div className="divide-y">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="flex items-center gap-3 px-3 py-2.5">
            <div className="size-4 rounded bg-muted animate-pulse" />
            <div className="size-7 rounded-md bg-muted animate-pulse" />
            <div className="flex-1 space-y-1.5">
              <div className="h-3 w-40 max-w-[60%] rounded bg-muted animate-pulse" />
              <div className="h-2.5 w-56 max-w-[40%] rounded bg-muted/70 animate-pulse" />
            </div>
            <div className="h-3 w-32 rounded bg-muted animate-pulse hidden sm:block" />
            <div className="h-5 w-16 rounded bg-muted animate-pulse" />
            <div className="h-5 w-20 rounded-full bg-muted animate-pulse" />
            <div className="h-7 w-20 rounded-md bg-muted animate-pulse" />
          </div>
        ))}
      </div>
    </div>
  )
}

// ── Confirm un-bridge modal ──────────────────────────────────────────────────

function ConfirmUnbridgeModal({
  targets, busy, onCancel, onConfirm,
}: {
  targets: Row[]
  busy: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  const previewLimit = 10
  const preview = targets.slice(0, previewLimit)
  const more = targets.length - preview.length

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 p-4 overflow-y-auto">
      <div className="bg-background border rounded-lg shadow-lg w-full max-w-md my-8 flex flex-col max-h-[calc(100vh-4rem)]">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <h2 className="text-base font-semibold flex items-center gap-2">
            <Link2Off className="size-4 text-amber-500" />
            Un-bridge {targets.length} {targets.length === 1 ? 'entity' : 'entities'}?
          </h2>
          <button
            onClick={onCancel}
            disabled={busy}
            className="text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="px-4 py-4 overflow-y-auto flex-1 space-y-3">
          <p className="text-sm text-muted-foreground">
            The following {targets.length === 1 ? 'device' : 'devices'} will be removed from
            digitalSTROM. The remote {targets.length === 1 ? 'entity' : 'entities'} will remain
            in {targets.length === 1 ? 'its' : 'their'} source plugin and can be re-bridged later.
          </p>
          <ul className="rounded-md border divide-y bg-muted/20 text-sm max-h-64 overflow-y-auto">
            {preview.map((r) => {
              const { Icon, tone } = deviceIconFor(r.kind, r.id, r.attributes)
              return (
                <li key={ROW_KEY(r)} className="flex items-center gap-2 px-3 py-1.5">
                  <div className={`size-6 rounded grid place-items-center ${tone}`}>
                    <Icon className="size-3.5" />
                  </div>
                  <span className="font-medium truncate flex-1" title={r.name}>{r.name}</span>
                  <span className="text-[11px] font-mono text-muted-foreground truncate max-w-[40%]" title={r.pluginId}>
                    {r.pluginId}
                  </span>
                </li>
              )
            })}
            {more > 0 && (
              <li className="px-3 py-1.5 text-xs text-muted-foreground italic">
                …and {more} more
              </li>
            )}
          </ul>
        </div>
        <div className="flex items-center justify-end gap-2 border-t px-4 py-3">
          <Button variant="ghost" size="sm" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={onConfirm}
            disabled={busy}
          >
            <Unlink className="size-3.5" />
            {busy ? 'Un-bridging…' : `Un-bridge ${targets.length}`}
          </Button>
        </div>
      </div>
    </div>
  )
}
