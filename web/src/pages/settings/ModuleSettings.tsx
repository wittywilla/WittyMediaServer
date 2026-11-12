// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Settings → Module settings: what each module in use lets people set (docs/SDUI-MODULES.md,
// "Settings"). A module describes its settings and never draws them: they are drawn here with
// Settings' own sections, rows and controls, in Settings' own look. A server row is changed by
// an administrator for everyone and shown to others as a value; a profile row is each
// profile's own. Each change is saved as it is made; a refusal stays beside its row.

import { useCallback, useEffect, useId, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { announce } from '../../a11y'
import { api, ApiError } from '../../api'
import { EmptyState, LoadError, Spinner } from '../../components/common'
import { IconPlug, IconRight } from '../../icons'
import { sduiIcon } from '../../sdui/render/icons'
import { Switch } from '../../ui/Field'
import { PageHead, Row, Section } from './ui'

type Option = { value: string; label: string }
export type SettingRow = {
  key?: string; kind: 'bool' | 'enum' | 'int' | 'string' | 'secret' | 'info'; label: string; help?: string
  scope?: 'server' | 'profile'; default?: unknown; options?: Option[]; min?: number; max?: number; unit?: string; maxLength?: number
}
type Group = { title: string; footer?: string; rows: SettingRow[] }
type SettingsPageDesc = { id: string; title: string; icon?: string; admin?: boolean; groups: Group[] }
export type ModuleSettingsView = { module: string; label: string; pages: SettingsPageDesc[]; values: Record<string, unknown>; secretsSet: string[]; editable: boolean }
type Listed = { module: string; label: string; icon?: string; url: string }

const BASE = '/sdui/modules'

// The modules that have settings this profile may see.
export function ModuleSettingsList() {
  const [list, setList] = useState<Listed[] | null>(null)
  const [failed, setFailed] = useState('')
  const load = useCallback(() => { setFailed(''); api.get<{ modules: Listed[] }>(`${BASE}/settings`).then((r) => setList(r.modules || [])).catch((e) => setFailed((e as Error).message)) }, [])
  useEffect(load, [load])
  return (
    <>
      <PageHead title="Module settings" desc="What the modules on this server let you set. Some are for the whole server and only an administrator changes them; others are your own." />
      {failed ? <LoadError what="Module settings" onRetry={load} />
        : !list ? <Spinner />
          : list.length === 0 ? <EmptyState icon={<IconPlug />} title="No module settings" text="None of the modules in use has settings for you." />
            : (
              <Section title="Modules">
                {list.map((m) => (
                  <Row key={m.module} label={<Link className="ms-link" to={`/settings/module-settings/${encodeURIComponent(m.module)}`}>{m.icon ? sduiIcon(m.icon) : <IconPlug />} <span>{m.label}</span></Link>}>
                    <IconRight aria-hidden="true" />
                  </Row>
                ))}
              </Section>
            )}
    </>
  )
}

// One module's settings, every page of it one after another.
export function ModuleSettingsPage() {
  const { module = '' } = useParams()
  const [v, setV] = useState<ModuleSettingsView | null>(null)
  const [failed, setFailed] = useState('')
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState<string>('')
  const load = useCallback(() => {
    setFailed('')
    api.get<ModuleSettingsView>(`${BASE}/${encodeURIComponent(module)}/settings`).then(setV).catch((e) => setFailed(e instanceof ApiError && e.status === 404 ? 'This module isn’t in use, or this profile can’t use it.' : (e as Error).message))
  }, [module])
  useEffect(load, [load])

  // Saves one value at once, as Settings' other switches do; the answer is the new state.
  const save = async (row: SettingRow, value: unknown) => {
    if (!row.key || !v) return
    const personal = row.scope === 'profile'
    setBusy(row.key)
    try {
      const next = await api.put<ModuleSettingsView>(`${BASE}/${encodeURIComponent(module)}/settings${personal ? '/personal' : ''}`, { values: { [row.key]: value } })
      setV(next)
      setErrors((e) => { const n = { ...e }; delete n[row.key!]; return n })
    } catch (e) {
      const why = (e as Error).message
      setErrors((x) => ({ ...x, [row.key!]: why }))
      announce(why)
    } finally { setBusy('') }
  }

  if (failed) return <><PageHead title="Module settings" /><LoadError what={failed} onRetry={load} /></>
  if (!v) return <Spinner />
  return (
    <>
      <PageHead title={v.label} desc={<Link to="/settings/module-settings">All module settings</Link>} />
      {v.pages.map((p) => (
        <div key={p.id} className="ms-page">
          {v.pages.length > 1 && <h2 className="ms-page-title">{p.title}</h2>}
          {p.groups.map((g, gi) => (
            <Section key={gi} title={g.title} desc={g.footer}>
              {g.rows.map((r, ri) => <SettingRowView key={r.key || `info-${ri}`} row={r} view={v} busy={busy === r.key} error={r.key ? errors[r.key] : ''} onSave={(value) => void save(r, value)} />)}
            </Section>
          ))}
        </div>
      ))}
    </>
  )
}

// valueOf: what a row shows now: the saved value, else its default.
export function valueOf(row: SettingRow, values: Record<string, unknown>): unknown {
  if (!row.key) return undefined
  return row.key in values ? values[row.key] : row.default
}

// canChange: a server row is an administrator's to change; a profile row its own profile's.
export const canChange = (row: SettingRow, editable: boolean) => row.kind !== 'info' && (row.scope === 'profile' || editable)

function SettingRowView({ row, view, busy, error, onSave }: { row: SettingRow; view: ModuleSettingsView; busy: boolean; error?: string; onSave: (v: unknown) => void }) {
  const id = useId()
  const value = valueOf(row, view.values)
  const editable = canChange(row, view.editable)
  const desc = (
    <>
      {row.help}
      {!editable && row.kind !== 'info' && <span className="muted">{row.help ? ' ' : ''}Set by an administrator for the whole server.</span>}
      {error && <span className="error" role="alert" id={id + 'e'}> {error}</span>}
    </>
  )
  if (row.kind === 'info') return <Row label={row.label} desc={row.help} />
  if (!editable) {
    const shown = row.kind === 'bool' ? (value ? 'On' : 'Off') : row.kind === 'enum' ? row.options?.find((o) => o.value === value)?.label ?? String(value ?? '') : row.kind === 'secret' ? (view.secretsSet.includes(row.key!) ? 'Set' : 'Not set') : `${value ?? ''}${row.unit ? ' ' + row.unit : ''}`
    return <Row label={row.label} desc={desc}><span className="ms-value" aria-label={`${row.label}: ${shown}, read only`} tabIndex={0}>{shown}</span></Row>
  }
  switch (row.kind) {
    case 'bool':
      return <Row label={row.label} desc={desc}><Switch label={row.label} checked={value === true} disabled={busy} onChange={(on) => onSave(on)} /></Row>
    case 'enum':
      return (
        <Row label={row.label} desc={desc}>
          <select className="select" aria-label={row.label} value={String(value ?? '')} disabled={busy} aria-describedby={error ? id + 'e' : undefined} onChange={(e) => onSave(e.target.value)}>
            {(row.options || []).map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </Row>
      )
    case 'int':
      return <Row label={row.label} desc={desc}><NumberField row={row} value={typeof value === 'number' ? value : undefined} busy={busy} errorId={error ? id + 'e' : undefined} onSave={onSave} /></Row>
    case 'string':
      return <Row label={row.label} desc={desc} stack><TextField row={row} value={typeof value === 'string' ? value : ''} busy={busy} errorId={error ? id + 'e' : undefined} onSave={onSave} /></Row>
    case 'secret':
      return <Row label={row.label} desc={desc} stack><SecretField row={row} set={view.secretsSet.includes(row.key!)} busy={busy} onSave={onSave} /></Row>
  }
  return null
}

// A number is saved when the field is left or Enter is pressed, as Settings' other fields are.
function NumberField({ row, value, busy, errorId, onSave }: { row: SettingRow; value?: number; busy: boolean; errorId?: string; onSave: (v: unknown) => void }) {
  const [text, setText] = useState(value === undefined ? '' : String(value))
  useEffect(() => setText(value === undefined ? '' : String(value)), [value])
  const commit = () => {
    if (text.trim() === '') { if (value !== undefined) onSave(null); return }
    const n = Number(text)
    if (Number.isInteger(n) && n !== value) onSave(n)
  }
  return (
    <span className="ms-num">
      <input className="input" type="number" inputMode="numeric" aria-label={row.label} aria-describedby={errorId} min={row.min} max={row.max} step={1} value={text} disabled={busy}
        onChange={(e) => setText(e.target.value)} onBlur={commit} onKeyDown={(e) => { if (e.key === 'Enter') commit() }} />
      {row.unit && <span className="muted">{row.unit}</span>}
    </span>
  )
}

function TextField({ row, value, busy, errorId, onSave }: { row: SettingRow; value: string; busy: boolean; errorId?: string; onSave: (v: unknown) => void }) {
  const [text, setText] = useState(value)
  useEffect(() => setText(value), [value])
  const commit = () => { if (text !== value) onSave(text === '' ? null : text) }
  return <input className="input" aria-label={row.label} aria-describedby={errorId} value={text} maxLength={row.maxLength || 1000} disabled={busy} spellCheck={false}
    onChange={(e) => setText(e.target.value)} onBlur={commit} onKeyDown={(e) => { if (e.key === 'Enter') commit() }} />
}

// A secret is never shown or filled in: whether one is set, a field for a new one, and Clear.
function SecretField({ row, set, busy, onSave }: { row: SettingRow; set: boolean; busy: boolean; onSave: (v: unknown) => void }) {
  const [text, setText] = useState('')
  return (
    <span className="ms-secret">
      <span className="muted" aria-live="polite">{set ? 'Set' : 'Not set'}</span>
      <input className="input" type="password" autoComplete="off" aria-label={`New ${row.label}`} value={text} maxLength={row.maxLength || 1000} disabled={busy} onChange={(e) => setText(e.target.value)} />
      <button type="button" className="btn sm" disabled={busy || !text} onClick={() => { onSave(text); setText('') }}>Save</button>
      {set && <button type="button" className="btn sm ghost" disabled={busy} onClick={() => onSave(null)}>Clear</button>}
    </span>
  )
}
