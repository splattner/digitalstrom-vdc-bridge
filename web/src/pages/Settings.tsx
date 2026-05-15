import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  Copy,
  Download,
  Eraser,
  Monitor,
  Moon,
  RefreshCw,
  Save,
  Sun,
} from 'lucide-react'
import { api, type SettingsInfo } from '@/api/client'
import { Button } from '@/components/ui/button'
import { useToasts } from '@/lib/toasts'
import { useUIPrefs, type Theme, type TimeFormat } from '@/lib/uiPrefs'

export default function SettingsPage() {
  const { data, isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ['settings'],
    queryFn: api.settings,
    refetchInterval: 5000,
  })

  return (
    <div className="space-y-4 max-w-4xl">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Settings</h1>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => refetch()}
          disabled={isFetching}
          title="Reload"
        >
          <RefreshCw className={`size-3.5 ${isFetching ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {isLoading && <p className="text-muted-foreground text-sm">Loading…</p>}
      {error && (
        <p className="text-destructive text-sm">
          Failed to load settings: {error instanceof Error ? error.message : String(error)}
        </p>
      )}

      {data && (
        <>
          <IdentityCard info={data} />
          <RuntimeCard info={data} />
          <SessionCard info={data} />
          <UIPrefsCard />
          <DangerCard info={data} />
        </>
      )}
    </div>
  )
}

// --- shared bits -----------------------------------------------------------

function Card({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: React.ReactNode
}) {
  return (
    <section className="rounded-lg border bg-card">
      <header className="px-4 py-3 border-b">
        <h2 className="text-sm font-semibold">{title}</h2>
        {description && (
          <p className="text-xs text-muted-foreground mt-0.5">{description}</p>
        )}
      </header>
      <div className="px-4 py-3 space-y-2.5">{children}</div>
    </section>
  )
}

function Row({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="grid grid-cols-[140px_1fr] items-center gap-3 text-sm">
      <span className="text-xs text-muted-foreground">{label}</span>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <Button
      size="sm"
      variant="ghost"
      className="h-7 px-2"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value)
          setCopied(true)
          setTimeout(() => setCopied(false), 1500)
        } catch {
          /* ignore */
        }
      }}
      title="Copy to clipboard"
    >
      {copied ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
    </Button>
  )
}

function Mono({ children, title }: { children: React.ReactNode; title?: string }) {
  return (
    <span className="font-mono text-xs truncate" title={title}>
      {children}
    </span>
  )
}

function BoolBadge({ value }: { value: boolean }) {
  return (
    <span
      className={`inline-block rounded-md border px-1.5 py-0.5 text-[11px] font-mono ${
        value
          ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border-emerald-500/30'
          : 'bg-muted text-muted-foreground border-border'
      }`}
    >
      {value ? 'enabled' : 'disabled'}
    </span>
  )
}

// --- cards -----------------------------------------------------------------

function IdentityCard({ info }: { info: SettingsInfo }) {
  const queryClient = useQueryClient()
  const toast = useToasts((s) => s.push)
  const [desc, setDesc] = useState(info.description)
  const [vendor, setVendor] = useState(info.vendor)
  const [model, setModel] = useState(info.model)

  const dirty =
    desc !== info.description || vendor !== info.vendor || model !== info.model

  const save = useMutation({
    mutationFn: () => api.patchIdentity({ description: desc, vendor, model }),
    onSuccess: () => {
      toast('Identity saved.', 'success')
      queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: (err) =>
      toast(`Save failed: ${err instanceof Error ? err.message : String(err)}`, 'error'),
  })

  return (
    <Card title="vDC identity" description="How this vDC presents itself to the digitalSTROM server.">
      <Row label="vDC dSUID">
        <div className="flex items-center gap-1">
          <Mono title={info.vdcDSUID}>{info.vdcDSUID}</Mono>
          <CopyButton value={info.vdcDSUID} />
        </div>
      </Row>
      <Row label="Description">
        <input
          className="w-full rounded-md border border-input bg-background px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          value={desc}
          onChange={(e) => setDesc(e.target.value)}
          placeholder="vdcgo external"
        />
      </Row>
      <Row label="Vendor">
        <input
          className="w-full rounded-md border border-input bg-background px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          value={vendor}
          onChange={(e) => setVendor(e.target.value)}
          placeholder="github.com/splattner"
        />
      </Row>
      <Row label="Model">
        <input
          className="w-full rounded-md border border-input bg-background px-2 py-1 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          value={model}
          onChange={(e) => setModel(e.target.value)}
          placeholder="vdcgo"
        />
      </Row>
      <Row label="Firmware">
        <Mono>{info.firmwareVersion}</Mono>
      </Row>
      <Row label="Build">
        <Mono>
          {info.buildVersion} · {info.goVersion} · {info.os}/{info.arch}
        </Mono>
      </Row>
      {dirty && (
        <div className="flex justify-end pt-1">
          <Button
            size="sm"
            variant="default"
            disabled={save.isPending}
            onClick={() => save.mutate()}
          >
            <Save className="size-3.5" />
            Save
          </Button>
        </div>
      )}
    </Card>
  )
}

function RuntimeCard({ info }: { info: SettingsInfo }) {
  return (
    <Card title="Runtime" description="Read-only snapshot of the daemon's startup configuration.">
      <Row label="vDC API">
        <span className="text-sm">
          <Mono>:{info.vdcAPIPort}</Mono>{' '}
          <span className="text-muted-foreground text-xs">({info.apiProtocol})</span>
        </span>
      </Row>
      <Row label="HTTP listen">
        <Mono>{info.httpListen || '—'}</Mono>
      </Row>
      <Row label="mDNS / DNS-SD">
        <BoolBadge value={info.enableDNSSD} />
      </Row>
      <Row label="Non-local">
        <BoolBadge value={info.nonLocal} />
      </Row>
      <Row label="No-auto">
        <BoolBadge value={info.noAuto} />
      </Row>
      <Row label="Data directory">
        <Mono title={info.dataDir}>{info.dataDir || '— (ephemeral)'}</Mono>
      </Row>
    </Card>
  )
}

function SessionCard({ info }: { info: SettingsInfo }) {
  const s = info.session
  if (!s.connected) {
    return (
      <Card title="vDSM session">
        <p className="text-sm text-muted-foreground">No vDSM connected.</p>
      </Card>
    )
  }
  return (
    <Card title="vDSM session" description="Current digitalSTROM server connection.">
      <Row label="Status">
        <span className="inline-flex items-center gap-1.5 text-sm">
          <span className="h-2 w-2 rounded-full bg-emerald-500" />
          connected
        </span>
      </Row>
      {s.vdsmDSUID && (
        <Row label="vDSM dSUID">
          <div className="flex items-center gap-1">
            <Mono title={s.vdsmDSUID}>{s.vdsmDSUID}</Mono>
            <CopyButton value={s.vdsmDSUID} />
          </div>
        </Row>
      )}
      {s.remoteAddr && (
        <Row label="Peer">
          <Mono>{s.remoteAddr}</Mono>
        </Row>
      )}
      {s.apiVersion != null && (
        <Row label="API version">
          <Mono>v{s.apiVersion}</Mono>
        </Row>
      )}
      {s.connectedAt && (
        <Row label="Connected at">
          <span className="text-sm">{new Date(s.connectedAt).toLocaleString()}</span>
        </Row>
      )}
    </Card>
  )
}

function UIPrefsCard() {
  const [prefs, update] = useUIPrefs()
  return (
    <Card title="UI preferences" description="These settings only affect this browser.">
      <Row label="Theme">
        <ThemePicker value={prefs.theme} onChange={(theme) => update({ theme })} />
      </Row>
      <Row label="Time format">
        <SegmentedPicker<TimeFormat>
          value={prefs.timeFormat}
          options={[
            { value: '24h', label: '24h' },
            { value: '12h', label: '12h' },
          ]}
          onChange={(timeFormat) => update({ timeFormat })}
        />
      </Row>
      <Row label="Protocol page">
        <label className="inline-flex items-center gap-2 text-sm cursor-pointer select-none">
          <input
            type="checkbox"
            className="accent-primary"
            checked={prefs.protocolAutoScroll}
            onChange={(e) => update({ protocolAutoScroll: e.target.checked })}
          />
          Auto-scroll to newest frame
        </label>
      </Row>
      <Row label="Frame buffer">
        <div className="flex items-center gap-2">
          <input
            type="number"
            min={50}
            max={5000}
            step={50}
            value={prefs.protocolFrameBufferCap}
            onChange={(e) => {
              const n = Number(e.target.value)
              if (Number.isFinite(n) && n >= 50 && n <= 5000) {
                update({ protocolFrameBufferCap: n })
              }
            }}
            className="w-24 rounded-md border bg-background px-2 py-1 text-sm font-mono outline-none focus:border-ring focus:ring-3 focus:ring-ring/30"
          />
          <span className="text-xs text-muted-foreground">frames retained on Protocol page</span>
        </div>
      </Row>
    </Card>
  )
}

function ThemePicker({ value, onChange }: { value: Theme; onChange: (t: Theme) => void }) {
  const opts: { value: Theme; label: string; icon: React.ReactNode }[] = [
    { value: 'light', label: 'Light', icon: <Sun className="size-3.5" /> },
    { value: 'dark', label: 'Dark', icon: <Moon className="size-3.5" /> },
    { value: 'system', label: 'System', icon: <Monitor className="size-3.5" /> },
  ]
  return (
    <div className="inline-flex rounded-md border overflow-hidden text-xs">
      {opts.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={`inline-flex items-center gap-1.5 px-2.5 py-1 transition-colors ${
            value === o.value
              ? 'bg-primary text-primary-foreground'
              : 'bg-background text-muted-foreground hover:bg-muted'
          }`}
        >
          {o.icon}
          {o.label}
        </button>
      ))}
    </div>
  )
}

function SegmentedPicker<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T
  options: { value: T; label: string }[]
  onChange: (v: T) => void
}) {
  return (
    <div className="inline-flex rounded-md border overflow-hidden text-xs">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={`px-2.5 py-1 transition-colors ${
            value === o.value
              ? 'bg-primary text-primary-foreground'
              : 'bg-background text-muted-foreground hover:bg-muted'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

function DangerCard({ info }: { info: SettingsInfo }) {
  const queryClient = useQueryClient()
  const toast = useToasts((s) => s.push)
  const [confirming, setConfirming] = useState(false)

  const forget = useMutation({
    mutationFn: api.forgetVdsm,
    onSuccess: (res) => {
      toast(
        `Forgot ${res.cleared} announced device${res.cleared === 1 ? '' : 's'} — re-announce on next vDSM connect.`,
        'success',
      )
      setConfirming(false)
      queryClient.invalidateQueries({ queryKey: ['settings'] })
      queryClient.invalidateQueries({ queryKey: ['dss'] })
    },
    onError: (err) =>
      toast(
        `Forget failed: ${err instanceof Error ? err.message : String(err)}`,
        'error',
      ),
  })

  const exportDisabled = !info.dataDir
  const exportTitle = useMemo(
    () =>
      exportDisabled
        ? 'No data directory configured (start with --datadir)'
        : 'Download tar.gz of config.json, scenes.json, bridges.json, plugins.json, dsuid',
    [exportDisabled],
  )

  return (
    <Card
      title="Maintenance"
      description="Operations that can affect the next vDSM session or persisted state."
    >
      <Row label="Forget vDSM">
        {confirming ? (
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">
              Re-announce all devices on next vDSM hello?
            </span>
            <Button
              size="sm"
              variant="destructive"
              disabled={forget.isPending}
              onClick={() => forget.mutate()}
            >
              <Eraser className="size-3.5" />
              Yes, forget
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>
              Cancel
            </Button>
          </div>
        ) : (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setConfirming(true)}
            title="Clear the set of DSUIDs already announced to the vDSM"
          >
            <Eraser className="size-3.5" />
            Forget vDSM session
          </Button>
        )}
      </Row>
      <Row label="Export config">
        <a
          href={api.exportConfigUrl()}
          download
          className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 h-7 text-[0.8rem] font-medium transition-all ${
            exportDisabled
              ? 'pointer-events-none opacity-50 border-border'
              : 'border-border bg-background hover:bg-muted'
          }`}
          aria-disabled={exportDisabled}
          title={exportTitle}
        >
          <Download className="size-3.5" />
          Download tar.gz
        </a>
      </Row>
    </Card>
  )
}
