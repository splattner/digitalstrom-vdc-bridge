import { useMemo, useState } from 'react'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowDownUp, ArrowDown, ArrowUp, CheckCircle2, Circle, Search, X,
} from 'lucide-react'
import {
  api, type DiscoveredEntity, type Plugin,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { useToasts } from '@/lib/toasts'

// ── Types ─────────────────────────────────────────────────────────────────────

interface Row extends DiscoveredEntity {
  pluginId: string
  pluginStatus: string
}

type SortKey = 'name' | 'plugin' | 'kind' | 'mapped'
type SortDir = 'asc' | 'desc'

const KIND_ORDER: Record<string, number> = {
  colorlight: 0, dimmer: 1, light: 2, sensor: 3, binary: 4,
}

// ── Visual helpers ────────────────────────────────────────────────────────────

function kindBadge(kind: string): string {
  switch (kind) {
    case 'colorlight': return 'bg-fuchsia-500/10 text-fuchsia-700 dark:text-fuchsia-300 border-fuchsia-500/30'
    case 'dimmer':     return 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/30'
    case 'light':      return 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-300 border-yellow-500/30'
    case 'sensor':     return 'bg-sky-500/10 text-sky-700 dark:text-sky-300 border-sky-500/30'
    case 'binary':     return 'bg-slate-500/10 text-slate-700 dark:text-slate-300 border-slate-500/30'
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

  // Discovered entities for every plugin (one parallel query each).
  const discoveredQueries = useQueries({
    queries: (plugins ?? []).map((p: Plugin) => ({
      queryKey: ['discovered', p.id],
      queryFn: () => api.discovered(p.id),
      refetchInterval: 10_000,
      retry: false,
    })),
  })

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

  // ── Filters / sort state ─────────────────────────────────────────────────
  const [query, setQuery] = useState('')
  const [showRemoteId, setShowRemoteId] = useState(false)
  const [pluginFilter, setPluginFilter] = useState<Set<string>>(new Set())
  const [kindFilter, setKindFilter] = useState<Set<string>>(new Set())
  const [areaFilter, setAreaFilter] = useState<Set<string>>(new Set())
  const [statusFilter, setStatusFilter] = useState<'all' | 'mapped' | 'unmapped'>('all')
  const [sort, setSort] = useState<{ key: SortKey; dir: SortDir }>({ key: 'name', dir: 'asc' })

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
        <h1 className="text-lg font-semibold">
          Discovered{' '}
          <span className="ml-1 text-sm font-normal text-muted-foreground">
            ({filtered.length}{filtered.length !== counts.total ? ` / ${counts.total}` : ''})
          </span>
        </h1>
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span>{counts.mapped} bridged</span>
          <span aria-hidden>·</span>
          <span>{counts.unmapped} new</span>
        </div>
      </div>

      {/* Toolbar */}
      <div className="rounded-lg border bg-card/30 p-3 space-y-2.5">
        {/* Row 1: search + status segmented control + remote-id toggle */}
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

          <label className="inline-flex items-center gap-1.5 text-xs text-muted-foreground cursor-pointer select-none">
            <input
              type="checkbox"
              checked={showRemoteId}
              onChange={(e) => setShowRemoteId(e.target.checked)}
              className="accent-primary"
            />
            show remote id
          </label>

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
            {plugins.map((p) => (
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

      {/* Body */}
      {loadingPlugins || allDiscoveryLoading ? (
        <p className="text-muted-foreground text-sm">Loading…</p>
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
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm table-fixed">
            <colgroup>
              <col className="w-7" />
              <col />
              <col className="w-36 hidden sm:table-column" />
              <col className="w-28" />
              <col className="w-28" />
            </colgroup>
            <thead>
              <tr className="border-b bg-muted/50">
                <th className="px-2 py-2" />
                <SortHeader label="Name" columnKey="name" sort={sort} setSort={setSort} />
                <SortHeader label="Plugin" columnKey="plugin" sort={sort} setSort={setSort} className="hidden sm:table-cell" />
                <SortHeader label="Kind" columnKey="kind" sort={sort} setSort={setSort} />
                <th className="px-3 py-2 text-right font-medium text-muted-foreground">Action</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((r) => {
                const busy = createMut.isPending || deleteMut.isPending
                return (
                  <tr
                    key={`${r.pluginId}\u0000${r.id}`}
                    className={`border-b last:border-0 hover:bg-muted/30 transition-colors ${
                      r.mapped ? 'bg-emerald-500/[0.04]' : ''
                    }`}
                  >
                    <td className="pl-3 pr-1 py-2 align-middle">
                      {r.mapped ? (
                        <CheckCircle2 className="size-4 text-emerald-600 dark:text-emerald-400" aria-label="Bridged" />
                      ) : (
                        <Circle className="size-4 text-muted-foreground/40" aria-label="Not bridged" />
                      )}
                    </td>
                    <td className="px-3 py-2 min-w-0">
                      <div className="font-medium truncate" title={r.name}>{r.name}</div>
                      {(() => {
                        const a = (r.attributes ?? {}) as Record<string, unknown>
                        const str = (k: string) => {
                          const v = a[k]
                          return typeof v === 'string' || typeof v === 'number' ? String(v) : ''
                        }
                        const device = str('device')
                        const area   = str('area')
                        const model  = str('model')
                        const ip     = str('ip') || str('addr')
                        const sw     = str('sw') || str('ver')
                        const parts: string[] = []
                        if (device && device !== r.name) parts.push(device)
                        if (area) parts.push(area)
                        if (model) parts.push(model)
                        if (ip) parts.push(ip)
                        if (sw) parts.push(`v${sw.replace(/^v/i, '')}`)
                        if (parts.length === 0) return null
                        const tooltip = Object.entries(a)
                          .filter(([, v]) => v != null && v !== '')
                          .map(([k, v]) => `${k}: ${String(v)}`)
                          .join('\n')
                        return (
                          <div className="text-[11px] text-muted-foreground truncate" title={tooltip}>
                            {parts.join(' · ')}
                          </div>
                        )
                      })()}
                      {showRemoteId && (
                        <div className="font-mono text-[10.5px] text-muted-foreground truncate" title={r.id}>
                          {r.id}
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-2 hidden sm:table-cell">
                      <span
                        className={`inline-block max-w-full truncate rounded-md border px-1.5 py-0.5 text-[11px] font-mono ${pluginBadge(r.pluginStatus)}`}
                        title={`${r.pluginId} · ${r.pluginStatus}`}
                      >
                        {r.pluginId}
                      </span>
                    </td>
                    <td className="px-3 py-2">
                      <span className={`inline-block rounded-md border px-1.5 py-0.5 text-[11px] ${kindBadge(r.kind)}`}>
                        {r.kind}
                      </span>
                    </td>
                    <td className="px-3 py-2 text-right">
                      {r.mapped ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={busy}
                          onClick={() => {
                            const dsuid = dsuidByRemote.get(`${r.pluginId}\u0000${r.id}`)
                            if (dsuid) deleteMut.mutate(dsuid)
                          }}
                        >
                          Un-bridge
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          disabled={busy}
                          onClick={() =>
                            createMut.mutate({
                              pluginId: r.pluginId,
                              remoteEntityId: r.id,
                              name: r.name,
                              kind: r.kind,
                            })
                          }
                        >
                          Bridge
                        </Button>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
