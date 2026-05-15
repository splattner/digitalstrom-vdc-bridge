// Physical-device sibling grouping
//
// Some plugins split a single physical device into multiple vDC / discovered
// entries (notably Zigbee2MQTT, which produces one "button" entity per action
// key on a remote). The convention used by these plugins is to encode the
// physical-device identifier and the per-endpoint suffix into the remote
// entity id separated by a colon, e.g. `0x00158d…:btn_on`.
//
// All entries that share the same `(pluginId, prefix-before-colon)` belong to
// the same physical device and should be visually grouped together regardless
// of any user-controlled grouping/filtering.

export interface SiblingInfo {
  key: string
  size: number
  index: number   // 0-based position within the sibling group
  color: string   // accent for the left rail / chip
  prefix: string  // human-readable label (everything before the ':')
  pluginId: string
}

export interface SiblingSource {
  pluginId: string
  remoteEntityId: string
}

export function siblingKeyOf(
  src: SiblingSource | undefined | null,
): { key: string; prefix: string; pluginId: string } | null {
  if (!src || !src.remoteEntityId) return null
  const colon = src.remoteEntityId.indexOf(':')
  if (colon <= 0) return null
  const prefix = src.remoteEntityId.slice(0, colon)
  return { key: `${src.pluginId}|${prefix}`, prefix, pluginId: src.pluginId }
}

// Stable hue per sibling key — same physical device gets the same colour
// across renders, but adjacent groups look distinct.
const SIBLING_PALETTE = [
  '#6366f1', // indigo-500
  '#06b6d4', // cyan-500
  '#f97316', // orange-500
  '#10b981', // emerald-500
  '#ec4899', // pink-500
  '#a855f7', // purple-500
  '#0ea5e9', // sky-500
  '#84cc16', // lime-500
]

export function siblingColor(key: string): string {
  let h = 0
  for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) | 0
  return SIBLING_PALETTE[Math.abs(h) % SIBLING_PALETTE.length]
}

/**
 * Compute sibling info for a list of items and return a re-ordered copy of the
 * input where members of the same sibling group are always adjacent. The first
 * sibling keeps its original position; subsequent siblings are pulled up right
 * after it. Items with no sibling group (single-endpoint or no `:`) are left
 * exactly where they were.
 */
export function clusterSiblings<T>(
  items: readonly T[],
  getKey: (item: T) => { key: string; prefix: string; pluginId: string } | null,
  getId: (item: T) => string,
): { ordered: T[]; siblingInfo: Map<string, SiblingInfo> } {
  const groups = new Map<string, T[]>()
  const meta = new Map<string, { prefix: string; pluginId: string }>()
  for (const it of items) {
    const k = getKey(it)
    if (!k) continue
    const arr = groups.get(k.key) ?? []
    arr.push(it)
    groups.set(k.key, arr)
    meta.set(k.key, { prefix: k.prefix, pluginId: k.pluginId })
  }
  const info = new Map<string, SiblingInfo>()
  for (const [key, arr] of groups) {
    if (arr.length < 2) continue
    const m = meta.get(key)!
    const color = siblingColor(key)
    arr.forEach((it, i) =>
      info.set(getId(it), { key, size: arr.length, index: i, color, prefix: m.prefix, pluginId: m.pluginId }),
    )
  }
  const ordered: T[] = []
  const seen = new Set<string>()
  for (const it of items) {
    const id = getId(it)
    if (seen.has(id)) continue
    const inf = info.get(id)
    if (inf) {
      for (const s of groups.get(inf.key) ?? [it]) {
        const sid = getId(s)
        if (!seen.has(sid)) {
          ordered.push(s)
          seen.add(sid)
        }
      }
    } else {
      ordered.push(it)
      seen.add(id)
    }
  }
  return { ordered, siblingInfo: info }
}
