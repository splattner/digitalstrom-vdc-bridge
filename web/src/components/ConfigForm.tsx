import { useCallback } from 'react'
import type { ConfigFieldSchema, ConfigSchema } from '@/api/client'

/**
 * Schema-driven form. Renders fields recursively (string / int / bool /
 * password / select / object).
 *
 * Conventions:
 *   - Password fields show a "(stored — leave blank to keep)" hint when the
 *     parent passes `secrets` containing the field's dot-path. An empty string
 *     in the submitted value tells the server "no change", because the empty
 *     password key is omitted from the JSON body before submission.
 *   - The form is fully controlled — `value` is the source of truth.
 */
export interface ConfigFormProps {
  schema: ConfigSchema
  value: Record<string, unknown>
  secrets?: string[]
  onChange: (next: Record<string, unknown>) => void
  /** Field-keys that are read-only (e.g. id when editing). */
  disabled?: string[]
}

export function ConfigForm({ schema, value, secrets = [], onChange, disabled = [] }: ConfigFormProps) {
  const setField = useCallback(
    (path: string[], v: unknown) => {
      onChange(setIn(value, path, v))
    },
    [value, onChange],
  )

  return (
    <div className="space-y-3">
      {(schema.fields ?? []).map((f) => (
        <FieldView
          key={f.key}
          field={f}
          path={[f.key]}
          value={getIn(value, [f.key])}
          secrets={secrets}
          onChange={setField}
          disabled={disabled.includes(f.key)}
        />
      ))}
    </div>
  )
}

function FieldView({
  field,
  path,
  value,
  secrets,
  onChange,
  disabled,
}: {
  field: ConfigFieldSchema
  path: string[]
  value: unknown
  secrets: string[]
  onChange: (path: string[], v: unknown) => void
  disabled?: boolean
}) {
  const id = `cf-${path.join('.')}`
  const dot = path.join('.')

  if (field.type === 'object') {
    const child = (value as Record<string, unknown> | undefined) ?? {}
    return (
      <fieldset className="border rounded-md p-3 space-y-3">
        <legend className="px-1 text-xs font-medium text-muted-foreground">
          {field.label}
          {field.required ? <span className="text-destructive ml-0.5">*</span> : null}
        </legend>
        {field.help ? <p className="text-xs text-muted-foreground -mt-1">{field.help}</p> : null}
        {(field.children ?? []).map((c) => (
          <FieldView
            key={c.key}
            field={c}
            path={[...path, c.key]}
            value={getIn(child, [c.key])}
            secrets={secrets}
            onChange={onChange}
          />
        ))}
      </fieldset>
    )
  }

  return (
    <div className="space-y-1">
      <label htmlFor={id} className="text-xs font-medium text-foreground">
        {field.label}
        {field.required ? <span className="text-destructive ml-0.5">*</span> : null}
      </label>
      {renderControl(field, id, value, dot, secrets, (v) => onChange(path, v), disabled)}
      {field.help ? <p className="text-xs text-muted-foreground">{field.help}</p> : null}
    </div>
  )
}

function renderControl(
  field: ConfigFieldSchema,
  id: string,
  value: unknown,
  dot: string,
  secrets: string[],
  onChange: (v: unknown) => void,
  disabled?: boolean,
) {
  const baseInput =
    'w-full border rounded px-2 py-1 text-sm bg-background focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50 disabled:cursor-not-allowed'

  switch (field.type) {
    case 'bool':
      return (
        <input
          id={id}
          type="checkbox"
          checked={Boolean(value)}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
          className="h-4 w-4 align-middle"
        />
      )
    case 'int':
      return (
        <input
          id={id}
          type="number"
          value={value == null ? '' : String(value)}
          placeholder={field.placeholder}
          min={field.min}
          max={field.max}
          disabled={disabled}
          onChange={(e) => {
            const t = e.target.value
            if (t === '') {
              onChange(undefined)
              return
            }
            const n = Number(t)
            onChange(Number.isFinite(n) ? n : t)
          }}
          className={baseInput}
        />
      )
    case 'select':
      return (
        <select
          id={id}
          value={value == null ? '' : String(value)}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className={baseInput}
        >
          <option value="" disabled>
            Select…
          </option>
          {(field.options ?? []).map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      )
    case 'password': {
      const stored = secrets.includes(dot)
      return (
        <input
          id={id}
          type="password"
          value={value == null ? '' : String(value)}
          placeholder={stored ? '(stored — leave blank to keep)' : (field.placeholder ?? '')}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          autoComplete="new-password"
          className={baseInput}
        />
      )
    }
    case 'string':
    default:
      return (
        <input
          id={id}
          type="text"
          value={value == null ? '' : String(value)}
          placeholder={field.placeholder}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className={baseInput}
        />
      )
  }
}

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

// ── tiny immutable helpers ────────────────────────────────────────────────────

function getIn(obj: Record<string, unknown>, path: string[]): unknown {
  let cur: unknown = obj
  for (const k of path) {
    if (cur == null || typeof cur !== 'object') return undefined
    cur = (cur as Record<string, unknown>)[k]
  }
  return cur
}

function setIn(obj: Record<string, unknown>, path: string[], v: unknown): Record<string, unknown> {
  if (path.length === 0) return obj
  const [head, ...rest] = path
  const next: Record<string, unknown> = { ...obj }
  if (rest.length === 0) {
    next[head] = v
  } else {
    const child = (obj[head] && typeof obj[head] === 'object' ? obj[head] : {}) as Record<string, unknown>
    next[head] = setIn(child, rest, v)
  }
  return next
}
