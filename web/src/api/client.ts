// Typed REST + WebSocket client for the vdcgo HTTP API.

export interface Device {
  dSUID: string
  name: string
  outputType?: string
  active?: boolean
  zoneID?: number
  primaryGroup?: number
  [key: string]: unknown
}

export interface DSSSession {
  connected: boolean
  vdsmDSUID?: string
  apiVersion?: number
  remoteAddr?: string
  connectedAt?: string
}

export interface DSSInfo {
  vdcDSUID: string
  session: DSSSession
}

export interface HealthInfo {
  ok: boolean
  time: string
  version: string
}

export interface WsEvent {
  type: string
  dsuid?: string
  data?: unknown
}

export interface Plugin {
  id: string
  type: string
  status: string
  enabled: boolean
  // Optional counters surfaced by plugins that implement bridge.StatsProvider
  // (currently tasmota, zigbee2mqtt, wled). HA + MQTT do not provide these.
  stats?: {
    discovered: number
    active: number
  }
}

export interface ConfigFieldSchema {
  key: string
  label: string
  help?: string
  type: 'string' | 'int' | 'bool' | 'password' | 'select' | 'multiselect' | 'object'
  default?: unknown
  required?: boolean
  placeholder?: string
  options?: { value: string; label: string }[]
  /** Dynamic source for select/multiselect options. "plugin" → fetched from
   * GET /api/plugins/{id}/suggest/{key} when an instance id is available. */
  optionsSource?: 'plugin' | ''
  children?: ConfigFieldSchema[]
  min?: number
  max?: number
}

export interface ConfigSchema {
  fields: ConfigFieldSchema[]
}

export interface PluginType {
  type: string
  displayName: string
  description: string
  schema: ConfigSchema
  hasProbe: boolean
}

export interface PluginConfigResponse {
  id: string
  type: string
  config: Record<string, unknown>
  /** Dot-paths of secret fields that are present-but-redacted in `config`. */
  secrets: string[]
}

export interface ProbeResult {
  ok: boolean
  error?: string
}

export interface SuggestOption {
  value: string
  label?: string
  count?: number
}

export interface RemoteEntity {
  id: string
  name: string
  kind: string
  /**
   * Plugin-specific extras. Conventional keys (used by the UI when present):
   *  - `device`        – containing device's friendly name (e.g. HA device)
   *  - `area`          – area / room name
   *  - `manufacturer`  – manufacturer string
   *  - `model`         – model string
   *  - `entity_id`     – plugin's native entity id (often same as `id`)
   *  - `state`         – last raw state (HA: "on", "23.4", …)
   *  - `device_class`  – HA device_class
   * Plugins may add arbitrary extra keys.
   */
  attributes?: Record<string, unknown>
}

export interface DiscoveredEntity extends RemoteEntity {
  mapped: boolean
}

export interface Mapping {
  pluginId: string
  remoteEntityId: string
  dsuid: string
  kind: string
  name: string
}

export interface CreateBridgeRequest {
  pluginId: string
  remoteEntityId: string
  name?: string
  kind?: string
}

export interface SettingsInfo {
  vdcDSUID: string
  description: string
  vendor: string
  model: string
  firmwareVersion: string
  vdcAPIPort: number
  apiProtocol: string
  httpListen: string
  enableDNSSD: boolean
  nonLocal: boolean
  noAuto: boolean
  dataDir: string
  buildVersion: string
  goVersion: string
  os: string
  arch: string
  session: DSSSession
}

export interface ForgetVdsmResponse {
  ok: boolean
  cleared: number
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export interface PluginEvent {
  seq: number
  time: string
  pluginId: string
  level: LogLevel
  code: string
  message: string
  fields?: Record<string, unknown>
}

const BASE = '/api'

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json() as Promise<T>
}

export const api = {
  health: () => get<HealthInfo>('/health'),
  dss: () => get<DSSInfo>('/dss'),
  devices: () => get<Record<string, Device>>('/devices'),
  device: (dsuid: string) => get<Device>(`/devices/${dsuid}`),
  plugins: () => get<Plugin[]>('/plugins'),
  pluginTypes: () => get<PluginType[]>('/plugin-types'),
  pluginConfig: (id: string) =>
    get<PluginConfigResponse>(`/plugins/${encodeURIComponent(id)}/config`),
  updatePluginConfig: (id: string, config: Record<string, unknown>) =>
    putJSON<PluginConfigResponse>(`/plugins/${encodeURIComponent(id)}/config`, { config }),
  createPlugin: (req: { id: string; type: string; config: Record<string, unknown> }) =>
    postJSON<PluginConfigResponse>('/plugins', req),
  deletePlugin: (id: string) => del(`/plugins/${encodeURIComponent(id)}`),
  restartPlugin: (id: string) =>
    postJSON<{ ok: boolean; error?: string }>(`/plugins/${encodeURIComponent(id)}/restart`, {}),
  enablePlugin: (id: string) =>
    postJSON<{ ok: boolean; enabled: boolean; error?: string }>(`/plugins/${encodeURIComponent(id)}/enable`, {}),
  disablePlugin: (id: string) =>
    postJSON<{ ok: boolean; enabled: boolean; error?: string }>(`/plugins/${encodeURIComponent(id)}/disable`, {}),
  rediscoverPlugin: (id: string) =>
    postJSON<DiscoveredEntity[]>(`/plugins/${encodeURIComponent(id)}/discover`, {}),
  probePlugin: (id: string, config?: Record<string, unknown>) =>
    postJSON<ProbeResult>(`/plugins/${encodeURIComponent(id)}/probe`, { config: config ?? null }),
  probePluginType: (type: string, config: Record<string, unknown>) =>
    postJSON<ProbeResult>(`/plugin-types/${encodeURIComponent(type)}/probe`, { config }),
  discovered: (pluginId: string) =>
    get<DiscoveredEntity[]>(`/plugins/${encodeURIComponent(pluginId)}/discovered`),
  pluginSuggest: (id: string, field: string) =>
    get<SuggestOption[]>(
      `/plugins/${encodeURIComponent(id)}/suggest/${encodeURIComponent(field)}`,
    ),
  bridges: () => get<Mapping[]>('/bridges'),
  createBridge: (req: CreateBridgeRequest) =>
    postJSON<Mapping>('/bridges', req),
  deleteBridge: (dsuid: string) => del(`/bridges/${encodeURIComponent(dsuid)}`),
  setButtonGroup: (dsuid: string, idx: number, group: number) =>
    putJSON<{ ok: boolean; dsuid: string; idx: number; group: number }>(
      `/devices/${encodeURIComponent(dsuid)}/buttons/${idx}/group`,
      { group },
    ),
  settings: () => get<SettingsInfo>('/settings'),
  forgetVdsm: () => postJSON<ForgetVdsmResponse>('/settings/forget-vdsm', {}),
  exportConfigUrl: () => `${BASE}/settings/export`,
  pluginEvents: (id: string, opts?: { since?: number; level?: string; limit?: number }) => {
    const p = new URLSearchParams()
    if (opts?.since) p.set('since', String(opts.since))
    if (opts?.level) p.set('level', opts.level)
    if (opts?.limit) p.set('limit', String(opts.limit))
    const qs = p.toString()
    return get<PluginEvent[]>(`/plugins/${encodeURIComponent(id)}/events${qs ? `?${qs}` : ''}`)
  },
  clearPluginEvents: (id: string) => del(`/plugins/${encodeURIComponent(id)}/events`),
  pluginEventsGlobal: (opts?: { since?: number; level?: string; limit?: number }) => {
    const p = new URLSearchParams()
    if (opts?.since) p.set('since', String(opts.since))
    if (opts?.level) p.set('level', opts.level)
    if (opts?.limit) p.set('limit', String(opts.limit))
    const qs = p.toString()
    return get<PluginEvent[]>(`/plugin-events${qs ? `?${qs}` : ''}`)
  },
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const txt = await res.text()
    throw new Error(`HTTP ${res.status}: ${txt}`)
  }
  return res.json() as Promise<T>
}

async function putJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const txt = await res.text()
    throw new Error(`HTTP ${res.status}: ${txt}`)
  }
  return res.json() as Promise<T>
}

async function del(path: string): Promise<void> {
  const res = await fetch(`${BASE}${path}`, { method: 'DELETE' })
  if (!res.ok && res.status !== 204) {
    const txt = await res.text()
    throw new Error(`HTTP ${res.status}: ${txt}`)
  }
}

export function connectEvents(onEvent: (e: WsEvent) => void): () => void {
  let ws: WebSocket | null = null
  let stopped = false
  let retryTimer: ReturnType<typeof setTimeout> | null = null

  function connect() {
    if (stopped) return
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    ws = new WebSocket(`${proto}://${location.host}/api/events`)
    ws.onmessage = (msg) => {
      try {
        const e = JSON.parse(msg.data as string) as WsEvent
        onEvent(e)
      } catch {
        // ignore malformed frames
      }
    }
    ws.onclose = () => {
      if (!stopped) retryTimer = setTimeout(connect, 2000)
    }
    ws.onerror = () => ws?.close()
  }

  connect()
  return () => {
    stopped = true
    if (retryTimer !== null) clearTimeout(retryTimer)
    ws?.close()
  }
}
