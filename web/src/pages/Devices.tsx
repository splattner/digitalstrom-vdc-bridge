import { useEffect, useMemo, useRef, useState, Fragment } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Search,
  X,
  ChevronRight,
  ChevronDown,
  Lightbulb,
  Home,
  Copy,
  Info,
  MoreVertical,
} from 'lucide-react'
import { api, connectEvents, type Device, type Mapping, type WsEvent } from '@/api/client'
import { Button } from '@/components/ui/button'
import { useToasts } from '@/lib/toasts'

// ── helpers ──────────────────────────────────────────────────────────────────

interface ChannelState  { value: number; age: number }
interface ChannelDesc   { name: string; siunit?: string; symbol?: string; min?: number; max?: number }
interface SensorState   { value: number; age: number; error?: number }
interface SensorDesc    { name?: string; siunit?: string; symbol?: string; sensorType?: number; min?: number; max?: number }

function getChannelStates(d: Device): Record<string, ChannelState> {
  const cs = d.channelStates
  if (cs && typeof cs === 'object' && !Array.isArray(cs)) {
    return cs as Record<string, ChannelState>
  }
  return {}
}

function getChannelDescs(d: Device): Record<string, ChannelDesc> {
  const cd = d.channelDescriptions
  if (cd && typeof cd === 'object' && !Array.isArray(cd)) {
    return cd as Record<string, ChannelDesc>
  }
  return {}
}

function getSensorStates(d: Device): Record<string, SensorState> {
  const s = d.sensorStates
  if (s && typeof s === 'object' && !Array.isArray(s)) return s as Record<string, SensorState>
  return {}
}

function getSensorDescs(d: Device): Record<string, SensorDesc> {
  const s = d.sensorDescriptions
  if (s && typeof s === 'object' && !Array.isArray(s)) return s as Record<string, SensorDesc>
  return {}
}

function isSensorDevice(d: Device): boolean {
  return String(d.outputType ?? '').toLowerCase() === 'sensor'
}

// ── digitalSTROM primary group → dot colour + label ─────────────────────────
// Values from p44vdc/vdc_common/dsdefs.h (DsGroup enum). Tailwind classes use
// inline styles for the official dS colour-code dots.
interface DsGroupInfo { name: string; color: string }
const DS_GROUPS: Record<number, DsGroupInfo> = {
  0:  { name: 'undefined',           color: '#9ca3af' }, // grey-400
  1:  { name: 'yellow · light',      color: '#facc15' },
  2:  { name: 'grey · shadow',       color: '#6b7280' },
  3:  { name: 'blue · heating',      color: '#3b82f6' },
  4:  { name: 'cyan · audio',        color: '#06b6d4' },
  5:  { name: 'magenta · video',     color: '#d946ef' },
  6:  { name: 'red · security',      color: '#ef4444' },
  7:  { name: 'green · access',      color: '#22c55e' },
  8:  { name: 'black · joker',       color: '#111827' },
  9:  { name: 'blue · cooling',      color: '#60a5fa' },
  10: { name: 'blue · ventilation',  color: '#60a5fa' },
  11: { name: 'blue · windows',      color: '#60a5fa' },
  12: { name: 'blue · air recirc.',  color: '#60a5fa' },
  48: { name: 'room temp. control',  color: '#f97316' },
  49: { name: 'room vent. control',  color: '#0ea5e9' },
}

function dsGroupInfo(g?: number): DsGroupInfo {
  if (g == null) return { name: 'unknown', color: '#9ca3af' }
  return DS_GROUPS[g] ?? { name: `group ${g}`, color: '#9ca3af' }
}

/** Format a channel value as a compact string with unit. */
function fmtChannel(val: number, desc?: ChannelDesc): string {
  const unit = desc?.symbol ?? desc?.siunit ?? ''
  return `${val % 1 === 0 ? val.toFixed(0) : val.toFixed(1)}${unit}`
}

/** Bar fill 0–100 based on min/max from description. */
function barPct(val: number, desc?: ChannelDesc): number {
  const min = desc?.min ?? 0
  const max = desc?.max ?? 100
  if (max === min) return 0
  return Math.max(0, Math.min(100, ((val - min) / (max - min)) * 100))
}

// ── ChannelPill ───────────────────────────────────────────────────────────────

function ChannelPill({ idx, state, desc }: { idx: string; state: ChannelState; desc?: ChannelDesc }) {
  const label = desc?.name ?? `ch${idx}`
  const pct = barPct(state.value, desc)

  return (
    <div className="flex flex-col gap-0.5 min-w-[60px]">
      <div className="flex items-baseline justify-between gap-1">
        <span className="text-[10px] text-muted-foreground capitalize leading-none">{label}</span>
        <span className="text-xs font-semibold tabular-nums leading-none">{fmtChannel(state.value, desc)}</span>
      </div>
      {/* progress bar */}
      <div className="h-1 rounded-full bg-muted overflow-hidden">
        <div
          className="h-full rounded-full bg-primary transition-all duration-300"
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  )
}

// ── ChannelBadges – compact inline list used in the table row ─────────────────

function ChannelBadges({ device }: { device: Device }) {
  if (isSensorDevice(device)) {
    const states = getSensorStates(device)
    const descs  = getSensorDescs(device)
    const keys   = Object.keys(states).sort()
    if (keys.length === 0) return <span className="text-muted-foreground/50 text-xs">—</span>
    return (
      <div className="flex flex-wrap gap-x-3 gap-y-1">
        {keys.map((k) => (
          <ChannelPill
            key={k}
            idx={k}
            state={{ value: states[k].value, age: states[k].age }}
            desc={{
              name: descs[k]?.name ?? `sensor${k}`,
              siunit: descs[k]?.siunit,
              symbol: descs[k]?.symbol,
              min: descs[k]?.min,
              max: descs[k]?.max,
            }}
          />
        ))}
      </div>
    )
  }
  const states = getChannelStates(device)
  const descs  = getChannelDescs(device)
  const keys   = Object.keys(states).sort()
  if (keys.length === 0) return <span className="text-muted-foreground/50 text-xs">—</span>

  return (
    <div className="flex flex-wrap gap-x-3 gap-y-1">
      {keys.map((k) => (
        <ChannelPill key={k} idx={k} state={states[k]} desc={descs[k]} />
      ))}
    </div>
  )
}

// ── DevicesPage ───────────────────────────────────────────────────────────────

export default function DevicesPage() {
  const qc = useQueryClient()
  const pushToast = useToasts((s) => s.push)
  const { data: devices, isLoading, error } = useQuery({
    queryKey: ['devices'],
    queryFn: api.devices,
  })
  const { data: bridges } = useQuery({
    queryKey: ['bridges'],
    queryFn: api.bridges,
  })
  const bridgeByDSUID = new Map<string, Mapping>((bridges ?? []).map((m) => [m.dsuid, m]))

  const unbridgeMut = useMutation({
    mutationFn: api.deleteBridge,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['devices'] })
      void qc.invalidateQueries({ queryKey: ['bridges'] })
      void qc.invalidateQueries({ queryKey: ['discovered'] })
    },
    onError: (e: unknown) => pushToast(`Un-bridge failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  const [selected, setSelected] = useState<Device | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [groupFilter, setGroupFilter] = useState<number | null>(null)
  const [activeOnly, setActiveOnly] = useState(false)
  const wsCleanup = useRef<(() => void) | null>(null)

  useEffect(() => {
    wsCleanup.current = connectEvents((e: WsEvent) => {
      if (e.type === 'stateChange') {
        void qc.invalidateQueries({ queryKey: ['devices'] })
        if (selected && e.dsuid === selected.dSUID) {
          void qc.invalidateQueries({ queryKey: ['device', selected.dSUID] })
        }
      }
    })
    return () => wsCleanup.current?.()
  }, [qc, selected])

  const rows = Object.values(devices ?? {})

  const presentGroups = useMemo(() => {
    const seen = new Set<number>()
    for (const d of rows) { if (d.primaryGroup != null) seen.add(d.primaryGroup) }
    return [...seen].sort((a, b) => a - b)
  }, [rows])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return rows.filter((d) => {
      if (activeOnly && !d.active) return false
      if (groupFilter != null && d.primaryGroup !== groupFilter) return false
      if (q) {
        const name = String(d.name ?? '').toLowerCase()
        const dsuid = String(d.dSUID ?? '').toLowerCase()
        if (!name.includes(q) && !dsuid.includes(q)) return false
      }
      return true
    })
  }, [rows, search, groupFilter, activeOnly])

  if (isLoading) return <p className="text-muted-foreground">Loading devices…</p>
  if (error)     return <p className="text-destructive">Failed to load devices.</p>

  const copyToClipboard = (text: string, label = 'Copied') => {
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      void navigator.clipboard.writeText(text)
      pushToast(`${label}: ${text.slice(0, 24)}${text.length > 24 ? '…' : ''}`, 'info')
    }
  }

  return (
    <div className="space-y-4">
      {/* ── Page header ── */}
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">Devices</h1>
            <span className="inline-flex items-center justify-center rounded-md bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground tabular-nums">
              {filtered.length !== rows.length ? `${filtered.length} / ${rows.length}` : rows.length}
            </span>
          </div>
          <p className="text-sm text-muted-foreground mt-1">
            Overview of all devices known to this VDC.
          </p>
        </div>

        {/* search + filters */}
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
            <input
              className="border rounded-md pl-8 pr-7 py-1.5 text-sm w-56 bg-background focus:outline-none focus:ring-1 focus:ring-ring"
              placeholder="Search name or dSUID…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            {search && (
              <button
                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                onClick={() => setSearch('')}
                aria-label="Clear search"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
          <button
            className={`rounded-md border px-3 py-1.5 text-xs font-medium transition-colors ${
              activeOnly
                ? 'bg-emerald-500/15 border-emerald-500/40 text-emerald-700 dark:text-emerald-300'
                : 'border-border text-muted-foreground hover:border-foreground/30 bg-background'
            }`}
            onClick={() => setActiveOnly((v) => !v)}
          >
            Active only
          </button>
        </div>
      </div>

      {/* group filter chips */}
      {presentGroups.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          {presentGroups.map((g) => {
            const info = dsGroupInfo(g)
            const active = groupFilter === g
            return (
              <button
                key={g}
                className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs transition-colors ${
                  active
                    ? 'border-foreground/40 bg-muted text-foreground'
                    : 'border-border text-muted-foreground hover:border-foreground/30 bg-background'
                }`}
                onClick={() => setGroupFilter(active ? null : g)}
                title={`Filter: ${info.name}`}
              >
                <span
                  className="inline-block h-2 w-2 rounded-full ring-1 ring-black/10 dark:ring-white/10 shrink-0"
                  style={{ backgroundColor: info.color }}
                />
                {info.name.split(' · ')[1] ?? info.name}
              </button>
            )
          })}
          {(search || groupFilter != null || activeOnly) && (
            <button
              className="text-xs text-muted-foreground hover:text-foreground underline-offset-2 hover:underline"
              onClick={() => { setSearch(''); setGroupFilter(null); setActiveOnly(false) }}
            >
              Clear filters
            </button>
          )}
        </div>
      )}

      {/* ── Devices table card ── */}
      {rows.length === 0 ? (
        <div className="rounded-xl border bg-card p-8 text-center text-sm text-muted-foreground">
          No devices announced.
        </div>
      ) : (
        <div className="rounded-xl border bg-card overflow-hidden shadow-sm">
          <div className="overflow-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/30">
                  <th className="w-8 px-2 py-3"></th>
                  <th className="text-left px-3 py-3 font-medium text-xs uppercase tracking-wider text-muted-foreground">Name</th>
                  <th className="text-left px-3 py-3 font-medium text-xs uppercase tracking-wider text-muted-foreground w-16">Active</th>
                  <th className="text-left px-3 py-3 font-medium text-xs uppercase tracking-wider text-muted-foreground">Channels / Sensors</th>
                  <th className="text-left px-3 py-3 font-medium text-xs uppercase tracking-wider text-muted-foreground w-40 hidden md:table-cell">Source</th>
                  <th className="w-10 px-2 py-3"></th>
                </tr>
              </thead>
              <tbody>
                {filtered.length === 0 && (
                  <tr>
                    <td colSpan={6} className="px-3 py-6 text-center text-sm text-muted-foreground">
                      No devices match the current filters.
                    </td>
                  </tr>
                )}
                {filtered.map((d) => {
                  const isOpen = expanded === d.dSUID
                  const groupInfo = dsGroupInfo(d.primaryGroup)
                  const bridge = bridgeByDSUID.get(d.dSUID)

                  return (
                    <Fragment key={d.dSUID}>
                      <tr
                        className={`border-b last:border-0 hover:bg-muted/30 cursor-pointer transition-colors ${
                          isOpen ? 'bg-muted/40' : ''
                        }`}
                        onClick={() => setExpanded(isOpen ? null : d.dSUID)}
                      >
                        <td className="px-2 py-3 text-muted-foreground">
                          {isOpen
                            ? <ChevronDown className="h-4 w-4" />
                            : <ChevronRight className="h-4 w-4" />}
                        </td>
                        <td className="px-3 py-3">
                          <div className="flex items-center gap-2.5">
                            <span
                              className="flex h-7 w-7 items-center justify-center rounded-md ring-1 ring-black/5 dark:ring-white/10 shrink-0"
                              style={{ backgroundColor: `${groupInfo.color}1f`, color: groupInfo.color }}
                              title={groupInfo.name}
                            >
                              <Lightbulb className="h-3.5 w-3.5" />
                            </span>
                            <div className="min-w-0">
                              <div className="font-medium truncate">{String(d.name ?? '—')}</div>
                              <div className="text-[11px] text-muted-foreground truncate">
                                {groupInfo.name}
                              </div>
                            </div>
                          </div>
                        </td>
                        <td className="px-3 py-3">
                          <span
                            className={`inline-block h-2 w-2 rounded-full ${
                              d.active ? 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.6)]' : 'bg-muted-foreground/30'
                            }`}
                            title={d.active ? 'active' : 'inactive'}
                          />
                        </td>
                        <td className="px-3 py-3">
                          <ChannelBadges device={d} />
                        </td>
                        <td className="px-3 py-3 hidden md:table-cell">
                          {bridge ? (
                            <span className="inline-flex items-center gap-1 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0.5 text-xs text-emerald-700 dark:text-emerald-300 font-mono">
                              <Home className="h-3 w-3" />
                              {bridge.pluginId}
                            </span>
                          ) : (
                            <span className="text-muted-foreground/50 text-xs">native</span>
                          )}
                        </td>
                        <td className="pr-2 py-3 text-right">
                          <Button
                            size="sm"
                            variant="ghost"
                            className="h-7 w-7 p-0 text-muted-foreground"
                            onClick={(ev: React.MouseEvent) => { ev.stopPropagation(); setSelected(d) }}
                            title="More"
                          >
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </td>
                      </tr>
                      {isOpen && (
                        <tr className="border-b last:border-0 bg-muted/20">
                          <td colSpan={6} className="p-0">
                            <ExpandedRow
                              device={d}
                              bridge={bridge}
                              onClose={() => setExpanded(null)}
                              onUnbridge={() => {
                                if (bridge) {
                                  unbridgeMut.mutate(d.dSUID)
                                  setExpanded(null)
                                }
                              }}
                              unbridgePending={unbridgeMut.isPending}
                              onCopy={copyToClipboard}
                            />
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Optional side drawer (kept for kebab “More” action) ── */}
      {selected && (
        <div
          className="fixed inset-0 z-40 flex justify-end bg-black/30"
          onClick={() => setSelected(null)}
        >
          <aside
            className="w-80 h-full bg-card border-l overflow-auto flex flex-col shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between px-4 py-3 border-b">
              <div className="min-w-0">
                <p className="font-semibold leading-tight truncate">{String(selected.name ?? '—')}</p>
                <p className="text-xs text-muted-foreground font-mono mt-0.5 truncate">{String(selected.dSUID)}</p>
              </div>
              <Button size="sm" variant="ghost" className="h-7 w-7 p-0 shrink-0" onClick={() => setSelected(null)}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="px-4 py-3 flex-1">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-2">Properties</p>
              <dl className="text-sm space-y-1.5">
                {Object.entries(selected)
                  .filter(([k]) => !['channelStates','channelDescriptions','active','zoneID','name','dSUID','outputType'].includes(k))
                  .map(([k, v]) => (
                    <div key={k} className="grid grid-cols-[auto_1fr] gap-x-2 items-start">
                      <dt className="text-muted-foreground text-xs whitespace-nowrap">{k}</dt>
                      <dd className="font-mono text-xs text-right truncate">
                        {typeof v === 'object' ? JSON.stringify(v) : String(v)}
                      </dd>
                    </div>
                  ))}
              </dl>
            </div>
          </aside>
        </div>
      )}
    </div>
  )
}

// ── ExpandedRow – inline foldable detail panel ───────────────────────────────

type DetailTab = 'overview' | 'channels' | 'sensors' | 'metadata'

function ExpandedRow({
  device,
  bridge,
  onClose,
  onUnbridge,
  unbridgePending,
  onCopy,
}: {
  device: Device
  bridge: Mapping | undefined
  onClose: () => void
  onUnbridge: () => void
  unbridgePending: boolean
  onCopy: (text: string, label?: string) => void
}) {
  const [tab, setTab] = useState<DetailTab>('overview')
  const groupInfo = dsGroupInfo(device.primaryGroup)
  const channelCount = Object.keys(getChannelStates(device)).length
  const sensorCount = Object.keys(getSensorStates(device)).length

  const tabs: { id: DetailTab; label: string; count?: number }[] = [
    { id: 'overview', label: 'Overview' },
    { id: 'channels', label: 'Channels', count: channelCount },
    { id: 'sensors',  label: 'Sensors',  count: sensorCount },
    { id: 'metadata', label: 'Metadata' },
  ]

  return (
    <div className="px-6 py-5">
      <div className="grid grid-cols-1 lg:grid-cols-[180px_1fr] gap-6">
        {/* ── Tabs nav ── */}
        <nav className="flex lg:flex-col gap-1 overflow-x-auto">
          {tabs.map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={`flex items-center justify-between gap-2 px-3 py-1.5 rounded-md text-xs font-medium text-left whitespace-nowrap transition-colors ${
                tab === t.id
                  ? 'bg-foreground/10 text-foreground'
                  : 'text-muted-foreground hover:bg-foreground/5'
              }`}
            >
              <span>{t.label}</span>
              {t.count != null && t.count > 0 && (
                <span className="inline-flex items-center justify-center rounded bg-muted px-1.5 text-[10px] tabular-nums">
                  {t.count}
                </span>
              )}
            </button>
          ))}
        </nav>

        {/* ── Tab content ── */}
        <div className="min-w-0">
          {tab === 'overview' && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {/* Device information */}
              <div className="space-y-2">
                <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Device information</p>
                <dl className="text-xs space-y-1.5">
                  <KV label="Name" value={String(device.name ?? '—')} />
                  <KV
                    label="dSUID"
                    value={String(device.dSUID)}
                    mono
                    action={
                      <button
                        className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground"
                        onClick={() => onCopy(String(device.dSUID), 'dSUID copied')}
                        title="Copy dSUID"
                      >
                        <Copy className="h-3 w-3" />
                      </button>
                    }
                  />
                  <KV label="Output type" value={String(device.outputType ?? '—')} mono />
                  <KV
                    label="Primary group"
                    value={
                      <span className="inline-flex items-center gap-1.5">
                        <span
                          className="inline-block h-2 w-2 rounded-full ring-1 ring-black/10 dark:ring-white/10"
                          style={{ backgroundColor: groupInfo.color }}
                        />
                        {groupInfo.name}
                      </span>
                    }
                  />
                  <KV label="Zone" value={device.zoneID != null && device.zoneID !== 0 ? String(device.zoneID) : '—'} />
                  <KV
                    label="Active"
                    value={
                      <span className="inline-flex items-center gap-1.5">
                        <span className={`h-1.5 w-1.5 rounded-full ${device.active ? 'bg-emerald-500' : 'bg-muted-foreground/30'}`} />
                        {device.active ? 'yes' : 'no'}
                      </span>
                    }
                  />
                </dl>
              </div>

              {/* Integration */}
              <div className="space-y-2">
                <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Integration</p>
                <div className="rounded-lg border bg-background p-3 space-y-2 text-xs">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-muted-foreground">Connected via</span>
                    {bridge ? (
                      <span className="inline-flex items-center gap-1 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0.5 text-emerald-700 dark:text-emerald-300 font-mono">
                        <Home className="h-3 w-3" />
                        {bridge.pluginId}
                      </span>
                    ) : (
                      <span className="text-muted-foreground">native</span>
                    )}
                  </div>
                  {bridge && (
                    <>
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-muted-foreground">Source entity</span>
                        <span className="font-mono truncate" title={bridge.remoteEntityId}>
                          {bridge.remoteEntityId}
                        </span>
                      </div>
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-muted-foreground">Kind</span>
                        <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]">{bridge.kind}</span>
                      </div>
                      <div className="pt-2 border-t border-border/60 flex items-center gap-2">
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-7 px-2 text-xs"
                          disabled={unbridgePending}
                          onClick={onUnbridge}
                        >
                          Un-bridge
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-7 px-2 text-xs ml-auto"
                          onClick={onClose}
                        >
                          Close
                        </Button>
                      </div>
                    </>
                  )}
                  {!bridge && (
                    <div className="flex items-start gap-1.5 pt-1 text-[11px] text-muted-foreground">
                      <Info className="h-3 w-3 mt-0.5 shrink-0" />
                      <span>This device is announced natively by the vDC.</span>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}

          {(tab === 'channels' || tab === 'sensors') && (
            <ChannelsList
              keys={tab === 'channels' ? Object.keys(getChannelStates(device)).sort() : Object.keys(getSensorStates(device)).sort()}
              states={tab === 'channels' ? getChannelStates(device) : getSensorStates(device)}
              descs={tab === 'channels' ? getChannelDescs(device) : getSensorDescs(device)}
              kind={tab}
            />
          )}

          {tab === 'metadata' && (
            <div className="space-y-2">
              <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Raw properties</p>
              <dl className="text-xs grid grid-cols-1 md:grid-cols-2 gap-x-6 gap-y-1.5">
                {Object.entries(device)
                  .filter(([k]) => ![
                    'channelStates','channelDescriptions','sensorStates','sensorDescriptions',
                    'active','zoneID','name','dSUID','outputType','primaryGroup',
                  ].includes(k))
                  .map(([k, v]) => (
                    <div key={k} className="flex items-start justify-between gap-3">
                      <dt className="text-muted-foreground shrink-0">{k}</dt>
                      <dd className="font-mono text-right truncate min-w-0">
                        {typeof v === 'object' ? JSON.stringify(v) : String(v)}
                      </dd>
                    </div>
                  ))}
              </dl>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function ChannelsList({
  keys,
  states,
  descs,
  kind,
}: {
  keys: string[]
  states: Record<string, ChannelState> | Record<string, SensorState>
  descs: Record<string, ChannelDesc> | Record<string, SensorDesc>
  kind: 'channels' | 'sensors'
}) {
  if (keys.length === 0) {
    return <p className="text-xs text-muted-foreground">No {kind} reported.</p>
  }
  return (
    <div className="space-y-2">
      <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        {kind === 'sensors' ? 'Sensors' : 'Channels'}
      </p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {keys.map((k) => {
          const st   = (states as Record<string, ChannelState>)[k]
          const desc = (descs as Record<string, ChannelDesc>)[k]
          const pct  = barPct(st.value, desc)
          const fallbackName = kind === 'sensors' ? `sensor${k}` : `ch${k}`
          return (
            <div key={k} className="rounded-lg border bg-background p-3 space-y-1.5">
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-xs text-muted-foreground capitalize truncate">
                  {desc?.name ?? fallbackName}
                </span>
                <span className="text-sm font-semibold tabular-nums">{fmtChannel(st.value, desc)}</span>
              </div>
              <div className="h-1.5 rounded-full bg-muted overflow-hidden">
                <div
                  className="h-full rounded-full bg-emerald-500 transition-all duration-300"
                  style={{ width: `${pct}%` }}
                />
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function KV({
  label,
  value,
  mono,
  action,
}: {
  label: string
  value: React.ReactNode
  mono?: boolean
  action?: React.ReactNode
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <dt className="text-muted-foreground shrink-0">{label}</dt>
      <dd className={`text-right truncate flex items-center gap-1.5 min-w-0 ${mono ? 'font-mono' : ''}`}>
        <span className="truncate">{value}</span>
        {action}
      </dd>
    </div>
  )
}
