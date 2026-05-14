import { NavLink, Outlet } from 'react-router-dom'
import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  LayoutGrid,
  Compass,
  Network,
  Puzzle,
  Settings as SettingsIcon,
  Box,
  Activity,
  Sun,
  Moon,
} from 'lucide-react'
import { api, connectEvents, type WsEvent } from '@/api/client'
import { Toaster } from '@/components/Toaster'
import { useToasts } from '@/lib/toasts'
import { useUIPrefs } from '@/lib/uiPrefs'

const navItems = [
  { to: '/devices',    label: 'Devices',    icon: LayoutGrid },
  { to: '/discovered', label: 'Discovered', icon: Compass },
  { to: '/protocol',   label: 'Protocol',   icon: Network },
  { to: '/plugins',    label: 'Plugins',    icon: Puzzle },
  { to: '/settings',   label: 'Settings',   icon: SettingsIcon },
]

function shortDSUID(dsuid?: string): string {
  if (!dsuid) return '—'
  if (dsuid.length <= 12) return dsuid
  return `${dsuid.slice(0, 6)}…${dsuid.slice(-4)}`
}

export default function Layout() {
  const qc = useQueryClient()
  const pushToast = useToasts((s) => s.push)
  const [{ theme }, updatePrefs] = useUIPrefs()
  const { data: health } = useQuery({ queryKey: ['health'], queryFn: api.health, refetchInterval: 10_000 })
  const { data: dss } = useQuery({ queryKey: ['dss'], queryFn: api.dss, refetchInterval: 5_000 })

  // Single global WS listener for cross-page concerns: bridge lifecycle toasts
  // and cache invalidation. Devices.tsx still has its own listener for state
  // changes; both subscribers run independently.
  useEffect(() => {
    const stop = connectEvents((e: WsEvent) => {
      if (e.type === 'bridgeAdded') {
        const data = (e.data ?? {}) as { name?: string; pluginId?: string }
        pushToast(`Bridged: ${data.name ?? e.dsuid ?? 'device'} (${data.pluginId ?? '?'})`, 'success')
        void qc.invalidateQueries({ queryKey: ['devices'] })
        void qc.invalidateQueries({ queryKey: ['bridges'] })
        void qc.invalidateQueries({ queryKey: ['discovered'] })
      } else if (e.type === 'bridgeRemoved') {
        const data = (e.data ?? {}) as { name?: string; pluginId?: string }
        pushToast(`Un-bridged: ${data.name ?? e.dsuid ?? 'device'}`, 'info')
        void qc.invalidateQueries({ queryKey: ['devices'] })
        void qc.invalidateQueries({ queryKey: ['bridges'] })
        void qc.invalidateQueries({ queryKey: ['discovered'] })
      }
    })
    return stop
  }, [qc, pushToast])

  const online = health?.ok ?? false
  const session = dss?.session
  const vdsmConnected = session?.connected ?? false
  const isDark =
    theme === 'dark' ||
    (theme === 'system' &&
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-color-scheme: dark)').matches)
  const version = health?.version ?? ''

  return (
    <div className="flex h-screen bg-background text-foreground">
      {/* ── Sidebar ────────────────────────────────────────────────────── */}
      <aside className="w-56 shrink-0 flex flex-col bg-slate-950 text-slate-300">
        {/* Brand */}
        <div className="flex items-center gap-2.5 px-4 py-4">
          <img
            src="/logo.png"
            alt="Digitalstrom vDC Bridge"
            className="h-9 w-9 rounded-md object-contain bg-white/5 p-0.5"
          />
          <div className="leading-tight min-w-0">
            <div className="font-semibold text-white text-[13px] truncate">Digitalstrom</div>
            <div className="text-[11px] text-slate-400 truncate">vDC Bridge</div>
          </div>
        </div>

        {/* Nav */}
        <nav className="flex-1 px-2 py-2 space-y-0.5">
          {navItems.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 text-sm rounded-md transition-colors ${
                  isActive
                    ? 'bg-emerald-500/15 text-emerald-300 ring-1 ring-emerald-500/25'
                    : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'
                }`
              }
            >
              <Icon className="h-4 w-4 shrink-0" />
              <span>{label}</span>
            </NavLink>
          ))}
        </nav>

        {/* Footer status */}
        <div className="px-3 py-3 border-t border-white/5 space-y-1">
          <div className="rounded-md bg-white/[0.03] px-3 py-2">
            <div className="flex items-center gap-2">
              <span
                className={`h-2 w-2 rounded-full ${
                  online
                    ? 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.6)]'
                    : 'bg-red-500'
                }`}
              />
              <span className="text-xs font-medium text-slate-200">
                {online ? 'API online' : 'API offline'}
              </span>
            </div>
            <div className="text-[10px] text-slate-500 mt-0.5">vdcgo API is reachable</div>
          </div>
          {version && (
            <div className="text-[10px] text-slate-500 px-3">
              {version.startsWith('v') ? version : `v${version}`}
            </div>
          )}
        </div>
      </aside>

      {/* ── Main area ──────────────────────────────────────────────────── */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Top bar */}
        <header className="border-b bg-card/50 backdrop-blur-sm">
          <div className="flex items-center gap-6 px-6 py-3">
            {/* Identity card – this VDC */}
            <div className="flex items-center gap-3 min-w-0">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-500/10 text-blue-500 ring-1 ring-blue-500/20">
                <Box className="h-5 w-5" />
              </div>
              <div className="leading-tight min-w-0">
                <div className="text-[10px] uppercase tracking-wider text-muted-foreground">this vDC</div>
                <div className="font-mono text-sm font-semibold truncate" title={dss?.vdcDSUID}>
                  {shortDSUID(dss?.vdcDSUID)}
                </div>
                <div className="text-[11px] text-muted-foreground truncate">Digitalstrom VDC</div>
              </div>
            </div>

            {/* Identity card – vDSM session */}
            <div className="flex items-center gap-3 min-w-0">
              <div
                className={`flex h-10 w-10 items-center justify-center rounded-lg ring-1 ${
                  vdsmConnected
                    ? 'bg-emerald-500/10 text-emerald-500 ring-emerald-500/20'
                    : 'bg-muted text-muted-foreground ring-border'
                }`}
              >
                <Activity className={`h-5 w-5 ${vdsmConnected ? 'animate-pulse' : ''}`} />
              </div>
              <div className="leading-tight min-w-0">
                <div className="text-[10px] uppercase tracking-wider text-muted-foreground">vDSM session</div>
                {vdsmConnected ? (
                  <>
                    <div className="text-sm font-semibold truncate" title={session?.remoteAddr}>
                      {session?.remoteAddr ?? 'connected'}
                    </div>
                    <div className="text-[11px] text-muted-foreground truncate">
                      {session?.vdsmDSUID ? shortDSUID(session.vdsmDSUID) : ''}
                      {session?.apiVersion ? ` · API v${session.apiVersion}` : ''}
                    </div>
                  </>
                ) : (
                  <>
                    <div className="text-sm font-semibold text-muted-foreground">no vDSM connected</div>
                    <div className="text-[11px] text-muted-foreground truncate">waiting for handshake</div>
                  </>
                )}
              </div>
            </div>

            <div className="flex-1" />

            {/* Theme toggle */}
            <button
              onClick={() => updatePrefs({ theme: isDark ? 'light' : 'dark' })}
              className="inline-flex items-center gap-1.5 rounded-full border bg-background px-3 py-1.5 text-xs font-medium hover:bg-muted transition-colors"
              title={`Switch to ${isDark ? 'light' : 'dark'} theme`}
            >
              {isDark ? <Moon className="h-3.5 w-3.5" /> : <Sun className="h-3.5 w-3.5" />}
              {isDark ? 'Dark' : 'Light'}
            </button>

            {/* API status pill */}
            <span
              className={`inline-flex items-center gap-2 rounded-full px-3 py-1.5 text-xs font-medium ring-1 ${
                online
                  ? 'bg-emerald-500/10 text-emerald-700 ring-emerald-500/30 dark:text-emerald-300'
                  : 'bg-red-500/10 text-red-700 ring-red-500/30 dark:text-red-300'
              }`}
            >
              <span className={`h-2 w-2 rounded-full ${online ? 'bg-emerald-500' : 'bg-red-500'}`} />
              {online ? 'API online' : 'API offline'}
            </span>
          </div>
        </header>

        {/* Page content */}
        <main className="flex-1 overflow-auto px-6 py-6 bg-muted/30">
          <Outlet />
        </main>
      </div>
      <Toaster />
    </div>
  )
}
