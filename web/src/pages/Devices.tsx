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
  MousePointerClick,
  Link2,
} from 'lucide-react'
import { api, connectEvents, type Device, type Mapping, type WsEvent } from '@/api/client'
import { Button } from '@/components/ui/button'
import { useToasts } from '@/lib/toasts'
import { clusterSiblings, siblingKeyOf } from '@/lib/siblings'

// ── helpers ──────────────────────────────────────────────────────────────────

interface ChannelState  { value: number; age: number }
interface ChannelDesc   { name: string; siunit?: string; symbol?: string; min?: number; max?: number }
interface SensorState   { value: number; age: number; error?: number }
interface SensorDesc    { name?: string; siunit?: string; symbol?: string; sensorType?: number; min?: number; max?: number }
interface ButtonState   { value: number; age: number; action?: string }
interface ButtonDesc    { name?: string; buttonID?: number }

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

function getButtonStates(d: Device): Record<string, ButtonState> {
  const s = d.buttonInputStates
  if (s && typeof s === 'object' && !Array.isArray(s)) return s as Record<string, ButtonState>
  return {}
}

function getButtonDescs(d: Device): Record<string, ButtonDesc> {
  const s = d.buttonInputDescriptions
  if (s && typeof s === 'object' && !Array.isArray(s)) return s as Record<string, ButtonDesc>
  return {}
}

interface ButtonSettings { group?: number; mode?: number; function?: number; setsLocalPriority?: boolean; callsPresent?: boolean }
function getButtonSettings(d: Device): Record<string, ButtonSettings> {
  const s = d.buttonInputSettings
  if (s && typeof s === 'object' && !Array.isArray(s)) return s as Record<string, ButtonSettings>
  return {}
}

function isButtonDevice(d: Device): boolean {
  return String(d.outputType ?? '').toLowerCase() === 'button'
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

// ── ActionLabel – human-readable click type label ─────────────────────────────

const ACTION_LABELS: Record<string, string> = {
  tip:   '1×',
  tip2:  '2×',
  tip3:  '3×',
  tip4:  '4×',
  hold:  'hold',
  dimup: 'dim ↑',
  dimdown: 'dim ↓',
}

function actionLabel(action: string): string {
  return ACTION_LABELS[action] ?? action
}

// ── ButtonRowBadge – compact button summary for the table row ─────────────────

function ButtonRowBadge({ device, flashAt }: { device: Device; flashAt?: number }) {
  const states = getButtonStates(device)
  const keys   = Object.keys(states).sort()
  const isFlashing = flashAt != null && (Date.now() - flashAt) < 2000

  // Most recent action across all buttons
  const lastAction = keys.reduce<string | undefined>((best, k) => {
    const a = states[k]?.action
    if (!a) return best
    return a
  }, undefined)

  return (
    <div className="flex items-center gap-2">
      <span
        className={`flex items-center justify-center h-6 w-6 rounded-md transition-all duration-200 ${
          isFlashing
            ? 'bg-violet-500/30 text-violet-600 dark:text-violet-400 ring-2 ring-violet-500/40'
            : 'bg-violet-500/10 text-violet-600 dark:text-violet-400'
        }`}
      >
        <MousePointerClick className={`h-3.5 w-3.5 ${isFlashing ? 'animate-bounce' : ''}`} />
      </span>
      {lastAction ? (
        <span className="text-xs font-mono text-muted-foreground">{actionLabel(lastAction)}</span>
      ) : (
        <span className="text-muted-foreground/50 text-xs">waiting…</span>
      )}
      {keys.length > 1 && (
        <span className="text-[10px] text-muted-foreground/60">{keys.length} btns</span>
      )}
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
  // dsuid → epoch ms of the most recent button_action event (for flash animation)
  const [buttonFlash, setButtonFlash] = useState<Record<string, number>>({})
  const wsCleanup = useRef<(() => void) | null>(null)

  useEffect(() => {
    wsCleanup.current = connectEvents((e: WsEvent) => {
      if (e.type === 'stateChange') {
        void qc.invalidateQueries({ queryKey: ['devices'] })
        if (selected && e.dsuid === selected.dSUID) {
          void qc.invalidateQueries({ queryKey: ['device', selected.dSUID] })
        }
        const evType = (e.data as Record<string, unknown> | undefined)?.eventType
        if (evType === 'button_action' && e.dsuid) {
          setButtonFlash((prev) => ({ ...prev, [e.dsuid!]: Date.now() }))
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

  // Cluster sibling devices (e.g. Z2M split-button entries with the same IEEE)
  // so they always appear adjacent regardless of the active group-by choice.
  const { ordered, siblingInfo } = useMemo(
    () => clusterSiblings(
      filtered,
      (d) => siblingKeyOf(bridgeByDSUID.get(d.dSUID)),
      (d) => d.dSUID,
    ),
    [filtered, bridgeByDSUID],
  )

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
                {ordered.map((d) => {
                  const isOpen = expanded === d.dSUID
                  const groupInfo = dsGroupInfo(d.primaryGroup)
                  const bridge = bridgeByDSUID.get(d.dSUID)
                  const sib = siblingInfo.get(d.dSUID)

                  return (
                    <Fragment key={d.dSUID}>
                      <tr
                        className={`border-b last:border-0 hover:bg-muted/30 cursor-pointer transition-colors ${
                          isOpen ? 'bg-muted/40' : ''
                        }`}
                        onClick={() => setExpanded(isOpen ? null : d.dSUID)}
                      >
                        <td
                          className="px-2 py-3 text-muted-foreground"
                          style={sib ? { boxShadow: `inset 4px 0 0 ${sib.color}` } : undefined}
                          title={sib ? `Part of ${sib.prefix} (${sib.size} devices on the same physical hardware)` : undefined}
                        >
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
                              <div className="font-medium truncate flex items-center gap-1.5">
                                <span className="truncate">{String(d.name ?? '—')}</span>
                                {sib && (
                                  <button
                                    type="button"
                                    onClick={(ev) => { ev.stopPropagation(); setSearch(sib.prefix) }}
                                    className="inline-flex items-center gap-0.5 rounded px-1 py-0.5 text-[10px] font-mono leading-none hover:bg-muted shrink-0"
                                    style={{ color: sib.color }}
                                    title={`Part of ${sib.prefix} — click to filter to its ${sib.size} siblings`}
                                  >
                                    <Link2 className="h-3 w-3" />
                                    {sib.index + 1}/{sib.size}
                                  </button>
                                )}
                              </div>
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
                          {isButtonDevice(d)
                            ? <ButtonRowBadge device={d} flashAt={buttonFlash[d.dSUID]} />
                            : <ChannelBadges device={d} />}
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
                              flashAt={buttonFlash[d.dSUID]}
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

type DetailTab = 'overview' | 'channels' | 'sensors' | 'buttons' | 'metadata'

function ExpandedRow({
  device,
  bridge,
  flashAt,
  onClose,
  onUnbridge,
  unbridgePending,
  onCopy,
}: {
  device: Device
  bridge: Mapping | undefined
  flashAt?: number
  onClose: () => void
  onUnbridge: () => void
  unbridgePending: boolean
  onCopy: (text: string, label?: string) => void
}) {
  const isButton = isButtonDevice(device)
  const [tab, setTab] = useState<DetailTab>(isButton ? 'buttons' : 'overview')
  const groupInfo = dsGroupInfo(device.primaryGroup)
  const channelCount = Object.keys(getChannelStates(device)).length
  const sensorCount = Object.keys(getSensorStates(device)).length
  const buttonCount = Object.keys(getButtonStates(device)).length

  const tabs: { id: DetailTab; label: string; count?: number }[] = [
    { id: 'overview', label: 'Overview' },
    ...(isButton ? [{ id: 'buttons' as DetailTab, label: 'Buttons', count: buttonCount }] : [
      { id: 'channels' as DetailTab, label: 'Channels', count: channelCount },
      { id: 'sensors'  as DetailTab, label: 'Sensors',  count: sensorCount },
    ]),
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

          {tab === 'buttons' && (
            <ButtonsList
              device={device}
              flashAt={flashAt}
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

// ── ButtonsList – expanded Buttons tab ───────────────────────────────────────

// Subset of dS color groups that make sense to assign to a button input.
// Values mirror p44vdc/vdc_common/dsdefs.h (DsGroup enum).
const BUTTON_GROUP_OPTIONS: { value: number; label: string }[] = [
  { value: 1,  label: '1 · yellow / light' },
  { value: 2,  label: '2 · grey / shadow' },
  { value: 3,  label: '3 · blue / heating' },
  { value: 4,  label: '4 · cyan / audio' },
  { value: 5,  label: '5 · magenta / video' },
  { value: 6,  label: '6 · red / security' },
  { value: 7,  label: '7 · green / access' },
  { value: 8,  label: '8 · black / joker' },
  { value: 9,  label: '9 · cooling' },
  { value: 10, label: '10 · ventilation' },
  { value: 11, label: '11 · windows' },
  { value: 48, label: '48 · room temp. control' },
]

function ButtonsList({ device, flashAt }: { device: Device; flashAt?: number }) {
  const qc = useQueryClient()
  const pushToast = useToasts((s) => s.push)
  const states   = getButtonStates(device)
  const descs    = getButtonDescs(device)
  const settings = getButtonSettings(device)
  const keys     = Object.keys(states).sort()
  const isFlashing = flashAt != null && (Date.now() - flashAt) < 2000

  const groupMut = useMutation({
    mutationFn: ({ idx, group }: { idx: number; group: number }) =>
      api.setButtonGroup(device.dSUID, idx, group),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['devices'] })
      void qc.invalidateQueries({ queryKey: ['device', device.dSUID] })
      pushToast('Group updated. dSS picks it up on next re-announce (Settings → Forget vDSM).', 'success')
    },
    onError: (e: unknown) => pushToast(`Update failed: ${e instanceof Error ? e.message : String(e)}`, 'error'),
  })

  if (keys.length === 0) {
    return (
      <div className="space-y-2">
        <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Buttons</p>
        <p className="text-xs text-muted-foreground">No button state received yet. Press the physical button to see activity.</p>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">Buttons</p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        {keys.map((k) => {
          const st     = states[k]
          const desc   = descs[k]
          const action = st.action
          const flash  = isFlashing && action != null
          const idxNum = Number(k)
          const curGroup = settings[k]?.group ?? 1
          const gInfo = dsGroupInfo(curGroup)

          return (
            <div
              key={k}
              className={`rounded-lg border p-3 flex flex-col gap-2 transition-all duration-300 ${
                flash
                  ? 'border-violet-500/50 bg-violet-500/10'
                  : 'border-border bg-background'
              }`}
            >
              <div className="flex items-center gap-3">
                <span
                  className={`flex items-center justify-center h-8 w-8 rounded-md shrink-0 transition-all duration-300 ${
                    flash
                      ? 'bg-violet-500/30 text-violet-600 dark:text-violet-400'
                      : 'bg-violet-500/10 text-violet-600/60 dark:text-violet-400/60'
                  }`}
                >
                  <MousePointerClick className={`h-4 w-4 ${flash ? 'animate-bounce' : ''}`} />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="text-xs text-muted-foreground">
                      {desc?.name ?? `button ${k}`}
                    </span>
                    {action ? (
                      <span className="text-sm font-semibold font-mono tabular-nums text-violet-700 dark:text-violet-300">
                        {actionLabel(action)}
                      </span>
                    ) : (
                      <span className="text-xs text-muted-foreground/50">—</span>
                    )}
                  </div>
                  <div className="text-[10px] text-muted-foreground/60 mt-0.5">
                    {action ? `last: ${action}` : 'waiting for press'}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2 pt-1 border-t border-border/50">
                <span className="text-[11px] text-muted-foreground shrink-0">Color group</span>
                <span
                  className="inline-block h-2 w-2 rounded-full ring-1 ring-black/10 dark:ring-white/10 shrink-0"
                  style={{ backgroundColor: gInfo.color }}
                  title={gInfo.name}
                />
                <select
                  className="flex-1 min-w-0 h-7 rounded border bg-background px-1 text-xs font-mono"
                  value={curGroup}
                  disabled={groupMut.isPending}
                  onChange={(e) => groupMut.mutate({ idx: idxNum, group: Number(e.target.value) })}
                >
                  {BUTTON_GROUP_OPTIONS.map((o) => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                  {/* Always render the current value if it isn't in the canonical list. */}
                  {!BUTTON_GROUP_OPTIONS.some((o) => o.value === curGroup) && (
                    <option value={curGroup}>group {curGroup}</option>
                  )}
                </select>
              </div>
            </div>
          )
        })}
      </div>
      <p className="text-[11px] text-muted-foreground/70 pt-1">
        Determines which dS scene group the button calls (e.g. lights vs. shades). Takes effect on the next dSS re-announce.
      </p>
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
