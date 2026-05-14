// User-interface preferences persisted in localStorage.
//
// These settings are local to the browser and do not affect the vDC daemon.
import { useCallback, useEffect, useSyncExternalStore } from 'react'

export type Theme = 'light' | 'dark' | 'system'
export type TimeFormat = '24h' | '12h'

export interface UIPrefs {
  theme: Theme
  timeFormat: TimeFormat
  protocolAutoScroll: boolean
  protocolFrameBufferCap: number
  defaultPage: string
}

const DEFAULTS: UIPrefs = {
  theme: 'system',
  timeFormat: '24h',
  protocolAutoScroll: true,
  protocolFrameBufferCap: 500,
  defaultPage: '/devices',
}

const STORAGE_KEY = 'vdcgo.uiPrefs.v1'

function load(): UIPrefs {
  if (typeof window === 'undefined') return { ...DEFAULTS }
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULTS }
    const parsed = JSON.parse(raw) as Partial<UIPrefs>
    return { ...DEFAULTS, ...parsed }
  } catch {
    return { ...DEFAULTS }
  }
}

let current: UIPrefs = load()
const listeners = new Set<() => void>()

function emit() {
  for (const l of listeners) l()
}

function persist() {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(current))
  } catch {
    /* quota / privacy mode — ignore */
  }
}

export function getUIPrefs(): UIPrefs {
  return current
}

export function setUIPrefs(patch: Partial<UIPrefs>) {
  current = { ...current, ...patch }
  persist()
  if (patch.theme !== undefined) applyTheme(current.theme)
  emit()
}

export function resetUIPrefs() {
  current = { ...DEFAULTS }
  persist()
  applyTheme(current.theme)
  emit()
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

export function useUIPrefs(): [UIPrefs, (patch: Partial<UIPrefs>) => void] {
  const snap = useSyncExternalStore(subscribe, () => current, () => current)
  const update = useCallback((patch: Partial<UIPrefs>) => setUIPrefs(patch), [])
  return [snap, update]
}

// --- Theme application -----------------------------------------------------

function resolveTheme(theme: Theme): 'light' | 'dark' {
  if (theme === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light'
  }
  return theme
}

export function applyTheme(theme: Theme) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  const resolved = resolveTheme(theme)
  root.classList.toggle('dark', resolved === 'dark')
  root.style.colorScheme = resolved
}

/**
 * Initialise theme on app boot and keep it in sync when "system" is selected
 * and the OS-level color-scheme changes.
 */
export function useThemeSync() {
  const [{ theme }] = useUIPrefs()
  useEffect(() => {
    applyTheme(theme)
    if (theme !== 'system') return
    const mql = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => applyTheme('system')
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [theme])
}

// Apply once at module load so first paint matches the saved preference.
applyTheme(current.theme)
