import type { ConfigSchema } from '@/api/client'

/**
 * Strip empty password fields from a config so the server treats them as
 * "no change". Walks the schema recursively for nested object fields.
 */
export function stripEmptyPasswords(
  cfg: Record<string, unknown>,
  schema: ConfigSchema,
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(cfg)) {
    out[k] = v
  }
  for (const f of schema.fields ?? []) {
    if (f.type === 'password') {
      if (out[f.key] === '' || out[f.key] == null) delete out[f.key]
    } else if (f.type === 'object' && out[f.key] && typeof out[f.key] === 'object') {
      out[f.key] = stripEmptyPasswords(
        out[f.key] as Record<string, unknown>,
        { fields: f.children ?? [] },
      )
    }
  }
  return out
}

/** Build the initial form value from a schema's default values. */
export function defaultsForSchema(schema: ConfigSchema): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of schema.fields ?? []) {
    if (f.type === 'object') {
      out[f.key] = defaultsForSchema({ fields: f.children ?? [] })
    } else if (f.default !== undefined) {
      out[f.key] = f.default
    }
  }
  return out
}
