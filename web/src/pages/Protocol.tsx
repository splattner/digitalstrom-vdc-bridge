import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, BASE, type Device } from '@/api/client'
import { Button } from '@/components/ui/button'
import { useUIPrefs } from '@/lib/uiPrefs'

// ── types ─────────────────────────────────────────────────────────────────────

interface PbufFrame {
  id: number            // local sequence number
  time: string          // ISO timestamp from server
  direction: 'rx' | 'tx'
  typeNum: number
  typeName: string
  msgId?: number
  hasMsgId?: boolean
  deviceDSUID?: string  // first DSUID extracted from decoded payload
  decoded?: Record<string, unknown>
  rawHex?: string
}

// ── helpers ───────────────────────────────────────────────────────────────────

const DIR_STYLES: Record<string, string> = {
  rx: 'text-blue-500',
  tx: 'text-emerald-500',
}
const DIR_LABEL: Record<string, string> = { rx: '↓ rx', tx: '↑ tx' }

function fmtTime(iso: string) {
  try {
    return new Date(iso).toLocaleTimeString(undefined, { hour12: false, fractionalSecondDigits: 3 })
  } catch {
    return iso
  }
}

// Deterministic hue from a number — golden-angle spread for visual distinctness.
function pairHue(msgId: number): number {
  return (msgId * 137) % 360
}

function pairBorderStyle(msgId: number): React.CSSProperties {
  return { borderLeft: `3px solid hsl(${pairHue(msgId)}, 70%, 55%)` }
}

function shortDSUID(dsuid: string): string {
  if (!dsuid) return ''
  return dsuid.length > 12 ? `${dsuid.slice(0, 6)}\u2026${dsuid.slice(-4)}` : dsuid
}

// Default fallback if a user hasn't customised it via Settings.
const DEFAULT_MAX_FRAMES = 500

// ── FrameRow ──────────────────────────────────────────────────────────────────

function FrameRow({
  frame, selected, pairSelected, pairedId, deviceName, onClick,
}: {
  frame: PbufFrame
  selected: boolean
  pairSelected: boolean
  pairedId?: number
  deviceName?: string
  onClick: () => void
}) {
  const isPaired = pairedId != null
  return (
    <tr
      className={`border-b last:border-0 cursor-pointer text-xs font-mono transition-colors ${
        selected
          ? 'bg-primary/10'
          : pairSelected
          ? 'bg-primary/5'
          : 'hover:bg-muted/40'
      }`}
      style={isPaired ? pairBorderStyle(frame.msgId!) : undefined}
      onClick={onClick}
    >
      <td className="px-2 py-1 tabular-nums text-muted-foreground w-[4ch]">{frame.id}</td>
      <td className="px-2 py-1 tabular-nums text-muted-foreground whitespace-nowrap">{fmtTime(frame.time)}</td>
      <td className={`px-2 py-1 font-semibold w-[5ch] ${DIR_STYLES[frame.direction] ?? ''}`}>
        {DIR_LABEL[frame.direction] ?? frame.direction}
      </td>
      <td className="px-2 py-1 text-[11px]">
        <span className="inline-block rounded bg-muted px-1 py-0.5 leading-tight">
          {frame.typeName}
        </span>
      </td>
      <td className="px-2 py-1 tabular-nums text-muted-foreground">
        {frame.hasMsgId ? (
          <span className="inline-flex items-center gap-0.5">
            <span>#{frame.msgId}</span>
            {isPaired && (
              <span
                className="text-[9px] ml-0.5"
                style={{ color: `hsl(${pairHue(frame.msgId!)}, 70%, 55%)` }}
                title={`Paired with #${pairedId}`}
              >⇄</span>
            )}
          </span>
        ) : (
          <span className="text-muted-foreground/30">—</span>
        )}
      </td>
      <td className="px-2 py-1 max-w-[9rem] truncate">
        {frame.deviceDSUID ? (
          <span className="text-[10.5px] text-muted-foreground" title={frame.deviceDSUID}>
            {deviceName ?? shortDSUID(frame.deviceDSUID)}
          </span>
        ) : (
          <span className="text-muted-foreground/20">—</span>
        )}
      </td>
    </tr>
  )
}

// ── DetailPanel ───────────────────────────────────────────────────────────────

function DetailPanel({ frame, pair, deviceName }: {
  frame: PbufFrame
  pair?: PbufFrame
  deviceName?: string
}) {
  const [tab, setTab] = useState<'decoded' | 'raw'>('decoded')
  const [side, setSide] = useState<'main' | 'pair'>('main')

  // reset to main + decoded when selected frame changes
  useEffect(() => { setSide('main') }, [frame.id])
  useEffect(() => { setTab('decoded') }, [frame.id, side])

  const active = side === 'main' || !pair ? frame : pair

  return (
    <div className="flex flex-col h-full">
      {/* header */}
      <div className="px-4 py-3 border-b">
        <div className="flex items-center gap-2 flex-wrap">
          <span className={`font-semibold text-sm ${DIR_STYLES[active.direction] ?? ''}`}>
            {DIR_LABEL[active.direction] ?? active.direction}
          </span>
          <span className="font-mono text-sm">{active.typeName}</span>
          {active.hasMsgId && (
            <span className="text-xs text-muted-foreground font-mono">msg#{active.msgId}</span>
          )}
          {deviceName && (
            <span
              className="text-xs rounded bg-muted px-1.5 py-0.5 text-muted-foreground truncate max-w-[12rem]"
              title={active.deviceDSUID}
            >
              {deviceName}
            </span>
          )}
        </div>
        <div className="text-xs text-muted-foreground mt-0.5">{fmtTime(active.time)}</div>
      </div>

      {/* req/resp switcher (only when paired) */}
      {pair && (
        <div className="flex border-b text-xs">
          <button
            onClick={() => setSide('main')}
            className={`flex-1 py-1 transition-colors ${
              side === 'main'
                ? 'border-b-2 border-primary text-foreground font-medium'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <span className={DIR_STYLES[frame.direction]}>{DIR_LABEL[frame.direction]}</span>
            {' '}{frame.typeName}
          </button>
          <button
            onClick={() => setSide('pair')}
            className={`flex-1 py-1 transition-colors ${
              side === 'pair'
                ? 'border-b-2 border-primary text-foreground font-medium'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <span className={DIR_STYLES[pair.direction]}>{DIR_LABEL[pair.direction]}</span>
            {' '}{pair.typeName}
          </button>
        </div>
      )}

      {/* decoded / raw tab bar */}
      <div className="flex border-b">
        {(['decoded', 'raw'] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-4 py-1.5 text-xs font-medium transition-colors ${
              tab === t
                ? 'border-b-2 border-primary text-foreground'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {/* content */}
      <div className="flex-1 overflow-auto p-3 text-xs font-mono">
        {tab === 'decoded' ? (
          active.decoded ? (
            <pre className="whitespace-pre-wrap break-all text-left">
              {JSON.stringify(active.decoded, null, 2)}
            </pre>
          ) : (
            <p className="text-muted-foreground italic">No decoded payload available.</p>
          )
        ) : (
          active.rawHex ? (
            <HexDump hex={active.rawHex} />
          ) : (
            <p className="text-muted-foreground italic">No raw bytes available.</p>
          )
        )}
      </div>
    </div>
  )
}

// ── HexDump ───────────────────────────────────────────────────────────────────

function HexDump({ hex }: { hex: string }) {
  const bytes = hex.match(/.{1,2}/g) ?? []
  const rows: string[][] = []
  for (let i = 0; i < bytes.length; i += 16) {
    rows.push(bytes.slice(i, i + 16))
  }
  return (
    <div className="space-y-0.5">
      {rows.map((row, i) => {
        const ascii = row
          .map((b) => {
            const c = parseInt(b, 16)
            return c >= 0x20 && c < 0x7f ? String.fromCharCode(c) : '.'
          })
          .join('')
        return (
          <div key={i} className="flex gap-4">
            <span className="text-muted-foreground w-[7ch] shrink-0">
              {(i * 16).toString(16).padStart(4, '0')}
            </span>
            <span className="flex gap-1 flex-wrap">
              {row.map((b, j) => (
                <span key={j} className="w-[2.4ch]">{b}</span>
              ))}
            </span>
            <span className="text-muted-foreground ml-auto">{ascii}</span>
          </div>
        )
      })}
    </div>
  )
}

// ── ProtocolPage ──────────────────────────────────────────────────────────────

export default function ProtocolPage() {
  const [frames, setFrames] = useState<PbufFrame[]>([])
  const [selected, setSelected] = useState<PbufFrame | null>(null)
  const [paused, setPaused] = useState(false)
  const [typeFilter, setTypeFilter] = useState('')
  const [deviceFilter, setDeviceFilter] = useState<string>('')  // DSUID or ''
  const [connected, setConnected] = useState(false)
  const seqRef = useRef(0)
  const pausedRef = useRef(false)
  const wsRef = useRef<WebSocket | null>(null)
  const tableBottomRef = useRef<HTMLTableRowElement | null>(null)
  const [{ protocolAutoScroll, protocolFrameBufferCap }] = useUIPrefs()
  const autoScrollRef = useRef(protocolAutoScroll)
  const maxFramesRef = useRef(protocolFrameBufferCap || DEFAULT_MAX_FRAMES)

  pausedRef.current = paused
  autoScrollRef.current = protocolAutoScroll
  maxFramesRef.current = protocolFrameBufferCap || DEFAULT_MAX_FRAMES

  // ── Device name lookup ─────────────────────────────────────────────────
  const { data: devices } = useQuery({ queryKey: ['devices'], queryFn: api.devices })
  const deviceByDSUID = useMemo(() => {
    const m = new Map<string, Device>()
    for (const [dsuid, d] of Object.entries(devices ?? {})) m.set(dsuid, d)
    return m
  }, [devices])

  function deviceName(dsuid?: string): string | undefined {
    if (!dsuid) return undefined
    const d = deviceByDSUID.get(dsuid)
    return d ? String(d.name) : undefined
  }

  // ── Request/response pair computation ─────────────────────────────────
  const pairMap = useMemo(() => {
    const reqByMsgId = new Map<number, number>()  // msgId → req frame.id
    const pairs = new Map<number, number>()        // frame.id ↔ frame.id
    for (const f of frames) {
      if (!f.hasMsgId || !f.msgId) continue
      if (f.direction === 'rx') {
        reqByMsgId.set(f.msgId, f.id)
      } else {
        const reqId = reqByMsgId.get(f.msgId)
        if (reqId != null) {
          pairs.set(f.id, reqId)
          pairs.set(reqId, f.id)
          reqByMsgId.delete(f.msgId)
        }
      }
    }
    return pairs
  }, [frames])

  const frameById = useMemo(() => {
    const m = new Map<number, PbufFrame>()
    for (const f of frames) m.set(f.id, f)
    return m
  }, [frames])

  // Unique DSUIDs seen across all frames (for filter chips).
  const seenDSUIDs = useMemo(() => {
    const s = new Set<string>()
    for (const f of frames) { if (f.deviceDSUID) s.add(f.deviceDSUID) }
    return [...s].sort()
  }, [frames])

  const connect = useCallback(() => {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}${BASE}/debug/pbuf`)
    wsRef.current = ws

    ws.onopen = () => setConnected(true)
    ws.onclose = () => {
      setConnected(false)
      // reconnect after 2 s
      setTimeout(connect, 2000)
    }
    ws.onerror = () => ws.close()
    ws.onmessage = (msg) => {
      if (pausedRef.current) return
      try {
        const ev = JSON.parse(msg.data as string) as { type: string; data?: Record<string, unknown> }
        if (ev.type !== 'pbuf' || !ev.data) return
        const d = ev.data
        const frame: PbufFrame = {
          id: ++seqRef.current,
          time: (d.time as string) ?? new Date().toISOString(),
          direction: (d.direction as 'rx' | 'tx') ?? 'rx',
          typeNum: (d.typeNum as number) ?? 0,
          typeName: (d.typeName as string) ?? `type_${d.typeNum}`,
          msgId: d.msgId as number | undefined,
          hasMsgId: d.hasMsgId as boolean | undefined,
          deviceDSUID: (d.deviceDSUID as string | undefined) || undefined,
          decoded: d.decoded as Record<string, unknown> | undefined,
          rawHex: d.rawHex as string | undefined,
        }
        setFrames((prev) => {
          const next = [...prev, frame]
          const cap = maxFramesRef.current
          return next.length > cap ? next.slice(next.length - cap) : next
        })
      } catch {
        // ignore malformed frames
      }
    }
  }, [])

  useEffect(() => {
    connect()
    return () => {
      wsRef.current?.close()
      wsRef.current = null
    }
  }, [connect])

  // auto-scroll to bottom when not paused
  useEffect(() => {
    if (!paused && autoScrollRef.current) {
      tableBottomRef.current?.scrollIntoView({ block: 'nearest' })
    }
  }, [frames, paused])

  const filtered = useMemo(() => {
    return frames.filter((f) => {
      if (typeFilter && !f.typeName.toLowerCase().includes(typeFilter.toLowerCase())) return false
      if (deviceFilter && f.deviceDSUID !== deviceFilter) return false
      return true
    })
  }, [frames, typeFilter, deviceFilter])

  return (
    <div className="flex flex-col h-full gap-3">
      {/* toolbar */}
      <div className="flex items-center gap-2 flex-wrap">
        <h1 className="text-lg font-semibold mr-2">Protocol Debug</h1>
        <div className={`w-2 h-2 rounded-full ${connected ? 'bg-emerald-500' : 'bg-muted-foreground'}`} title={connected ? 'connected' : 'disconnected'} />
        <span className="text-xs text-muted-foreground">{connected ? 'live' : 'disconnected'}</span>
        <div className="flex-1" />
        <input
          className="border rounded px-2 py-1 text-xs w-36 bg-background"
          placeholder="filter type…"
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
        />
        <Button
          size="sm"
          variant={paused ? 'default' : 'outline'}
          onClick={() => setPaused((p) => !p)}
        >
          {paused ? '▶ Resume' : '⏸ Pause'}
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => { setFrames([]); setSelected(null) }}
        >
          Clear
        </Button>
      </div>

      {/* device filter chips */}
      {seenDSUIDs.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-[10px] uppercase tracking-wider text-muted-foreground mr-1">Device</span>
          <button
            type="button"
            onClick={() => setDeviceFilter('')}
            className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs transition-colors ${
              deviceFilter === ''
                ? 'border-primary bg-primary text-primary-foreground'
                : 'border-border bg-background hover:bg-muted text-muted-foreground'
            }`}
          >
            all
          </button>
          {seenDSUIDs.map((dsuid) => (
            <button
              key={dsuid}
              type="button"
              onClick={() => setDeviceFilter(dsuid === deviceFilter ? '' : dsuid)}
              className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-mono transition-colors ${
                deviceFilter === dsuid
                  ? 'border-primary bg-primary text-primary-foreground'
                  : 'border-border bg-background hover:bg-muted text-muted-foreground'
              }`}
              title={dsuid}
            >
              {deviceName(dsuid) ?? shortDSUID(dsuid)}
            </button>
          ))}
        </div>
      )}

      {/* main area */}
      <div className="flex flex-1 min-h-0 gap-3">
        {/* frame table */}
        <div className="flex-1 min-w-0 border rounded-lg overflow-auto">
          {filtered.length === 0 ? (
            <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
              {connected ? 'Waiting for messages…' : 'Not connected to daemon.'}
            </div>
          ) : (
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10 bg-muted/80 backdrop-blur-sm">
                <tr className="border-b">
                  <th className="text-left px-2 py-1.5 text-xs font-medium text-muted-foreground w-[4ch]">#</th>
                  <th className="text-left px-2 py-1.5 text-xs font-medium text-muted-foreground">Time</th>
                  <th className="text-left px-2 py-1.5 text-xs font-medium text-muted-foreground w-[5ch]">Dir</th>
                  <th className="text-left px-2 py-1.5 text-xs font-medium text-muted-foreground">Type</th>
                  <th className="text-left px-2 py-1.5 text-xs font-medium text-muted-foreground w-[7ch]">MsgID</th>
                  <th className="text-left px-2 py-1.5 text-xs font-medium text-muted-foreground">Device</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((f) => {
                  const pairedId = pairMap.get(f.id)
                  return (
                    <FrameRow
                      key={f.id}
                      frame={f}
                      selected={selected?.id === f.id}
                      pairSelected={selected != null && pairedId === selected.id}
                      pairedId={pairedId}
                      deviceName={deviceName(f.deviceDSUID)}
                      onClick={() => setSelected((prev) => prev?.id === f.id ? null : f)}
                    />
                  )
                })}
                <tr ref={tableBottomRef} />
              </tbody>
            </table>
          )}
        </div>

        {/* detail panel */}
        {selected && (
          <aside className="w-80 shrink-0 border rounded-lg overflow-hidden flex flex-col">
            <DetailPanel
              frame={selected}
              pair={(() => {
                const pairedId = pairMap.get(selected.id)
                return pairedId != null ? frameById.get(pairedId) : undefined
              })()}
              deviceName={deviceName(selected.deviceDSUID)}
            />
          </aside>
        )}
      </div>
    </div>
  )
}
