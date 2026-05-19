import { NavLink, Outlet } from 'react-router-dom'
import { useEffect, useState } from 'react'
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
  Menu,
  X,
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
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
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
      {/* ── Desktop Sidebar ─────────────────────────────────────────────── */}
      <aside className="hidden md:flex w-56 shrink-0 flex-col bg-slate-950 text-slate-300">
        {/* Brand */}
        <div className="flex items-center gap-2.5 px-4 py-4">
          <img
            src={`${import.meta.env.BASE_URL}logo.png`}
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
            <div className="text-[10px] text-slate-500 mt-0.5">API is reachable</div>
          </div>
          {version && (
            <div className="text-[10px] text-slate-500 px-3">
              {version.startsWith('v') ? version : `v${version}`}
            </div>
          )}
        </div>
      </aside>

      {/* ── Mobile slide-in menu ─────────────────────────────────────────── */}
      {mobileMenuOpen && (
        <>
          {/* Backdrop */}
          <div
            className="fixed inset-0 z-40 bg-black/60 md:hidden"
            onClick={() => setMobileMenuOpen(false)}
          />
          {/* Drawer */}
          <aside className="fixed inset-y-0 left-0 z-50 flex w-64 flex-col bg-slate-950 text-slate-300 md:hidden">
            <div className="flex items-center justify-between px-4 py-4">
              <div className="flex items-center gap-2.5">
                <img
                  src={`${import.meta.env.BASE_URL}logo.png`}
                  alt="Digitalstrom vDC Bridge"
                  className="h-9 w-9 rounded-md object-contain bg-white/5 p-0.5"
                />
                <div className="leading-tight">
                  <div className="font-semibold text-white text-[13px]">Digitalstrom</div>
                  <div className="text-[11px] text-slate-400">vDC Bridge</div>
                </div>
              </div>
              <button
                onClick={() => setMobileMenuOpen(false)}
                className="rounded-md p-1.5 text-slate-400 hover:bg-white/10 hover:text-slate-100 transition-colors"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <nav className="flex-1 px-2 py-2 space-y-0.5">
              {navItems.map(({ to, label, icon: Icon }) => (
                <NavLink
                  key={to}
                  to={to}
                  onClick={() => setMobileMenuOpen(false)}
                  className={({ isActive }) =>
                    `flex items-center gap-3 px-3 py-2.5 text-sm rounded-md transition-colors ${
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
              </div>
              {version && (
                <div className="text-[10px] text-slate-500 px-3">
                  {version.startsWith('v') ? version : `v${version}`}
                </div>
              )}
            </div>
          </aside>
        </>
      )}

      {/* ── Main area ──────────────────────────────────────────────────── */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* ── Mobile top bar ─────────────────────────────────────────────── */}
        <header className="flex md:hidden items-center gap-3 px-4 py-3 border-b bg-slate-950 text-slate-200 shrink-0">
          <button
            onClick={() => setMobileMenuOpen(true)}
            className="rounded-md p-1.5 text-slate-400 hover:bg-white/10 hover:text-slate-100 transition-colors"
          >
            <Menu className="h-5 w-5" />
          </button>
          <img
            src={`${import.meta.env.BASE_URL}logo.png`}
            alt=""
            className="h-7 w-7 rounded object-contain bg-white/5 p-0.5"
          />
          <span className="font-semibold text-sm">vDC Bridge</span>
          <div className="flex-1" />
          {/* vDSM status pill */}
          <div className="flex items-center gap-1.5 rounded-full border border-white/10 bg-white/5 px-2.5 py-1">
            <span
              className={`h-1.5 w-1.5 rounded-full ${
                vdsmConnected ? 'bg-emerald-400' : 'bg-slate-500'
              }`}
            />
            <span className="text-[11px] text-slate-300">
              {vdsmConnected ? 'dSS connected' : 'no dSS'}
            </span>
          </div>
          <button
            onClick={() => updatePrefs({ theme: isDark ? 'light' : 'dark' })}
            className="rounded-md p-1.5 text-slate-400 hover:bg-white/10 transition-colors"
            title={`Switch to ${isDark ? 'light' : 'dark'} theme`}
          >
            {isDark ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
          </button>
        </header>

        {/* ── Desktop top bar ─────────────────────────────────────────────── */}
        <header className="hidden md:flex border-b bg-card/50 backdrop-blur-sm shrink-0">
          <div className="flex items-center gap-6 px-6 py-3 w-full">
            {/* Identity card – this VDC */}
            <div className="flex items-center gap-3 min-w-0">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-500/10 text-blue-500 ring-1 ring-blue-500/20">
                <Box className="h-5 w-5" />
              </div>
              <div className="leading-tight min-w-0">
                <div className="text-[10px] uppercase tracking-wider text-muted-foreground">this vDC</div>
                <div className="font-mono text-sm font-semibold" title={dss?.vdcDSUID}>
                  {dss?.vdcDSUID ?? '—'}
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
          </div>
        </header>

        <main className="flex-1 overflow-auto px-4 py-4 md:px-6 md:py-6 bg-muted/30 pb-20 md:pb-6">
          <Outlet />
        </main>

        {/* ── Mobile bottom tab bar ───────────────────────────────────────── */}
        <nav className="fixed bottom-0 left-0 right-0 z-30 flex md:hidden border-t bg-slate-950/95 backdrop-blur-sm">
          {navItems.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                `flex flex-1 flex-col items-center gap-1 px-1 py-2.5 text-[10px] font-medium transition-colors ${
                  isActive
                    ? 'text-emerald-400'
                    : 'text-slate-500 hover:text-slate-300'
                }`
              }
            >
              <Icon className="h-5 w-5" />
              <span>{label}</span>
            </NavLink>
          ))}
        </nav>
      </div>
      <Toaster />
    </div>
  )
}
