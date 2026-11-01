import { cloneElement, isValidElement, useEffect, useId, useState, type ReactElement, type ReactNode } from 'react'
import { api, getPrefs } from '../../api'
import { toast } from '../../state'

// The building blocks of every settings page: a page header, section cards,
// rows (what a setting is and does on the left, its control on the right),
// switches, and a save bar that appears only when something has changed.

export function PageHead({ title, desc, actions }: { title: string; desc?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="set-head">
      <div><h1>{title}</h1>{desc && <p>{desc}</p>}</div>
      {actions && <div className="set-head-actions">{actions}</div>}
    </header>
  )
}

export function Section({ title, desc, actions, children, id }: { title: string; desc?: ReactNode; actions?: ReactNode; children: ReactNode; id?: string }) {
  return (
    <section className="set-card" id={id} aria-label={title}>
      <header className="set-card-head">
        <div><h2>{title}</h2>{desc && <p>{desc}</p>}</div>
        {actions && <div className="set-card-actions">{actions}</div>}
      </header>
      <div className="set-card-body">{children}</div>
    </section>
  )
}

// stack: the control goes under the text (wide fields, groups of fields).
// The row's title and description name and describe its control for screen
// readers: a lone field is labelled by them directly, anything else is a
// group labelled by them.
export function Row({ label, desc, badge, children, stack, dim }: { label: ReactNode; desc?: ReactNode; badge?: ReactNode; children?: ReactNode; stack?: boolean; dim?: boolean }) {
  const id = useId()
  const labelId = id + 'l', descId = desc ? id + 'd' : undefined
  let ctl = children
  let group = true
  if (isValidElement(children) && typeof children.type === 'string' && ['select', 'input', 'textarea'].includes(children.type)) {
    const el = children as ReactElement<Record<string, unknown>>
    if (!el.props['aria-label'] && !el.props['aria-labelledby']) ctl = cloneElement(el, { 'aria-labelledby': labelId, 'aria-describedby': descId })
    group = false
  }
  return (
    <div className={'set-row' + (stack ? ' stack' : '') + (dim ? ' dim' : '')}>
      <div className="set-row-text">
        <div className="set-row-label" id={labelId}>{label}{badge && <span className="set-badge">{badge}</span>}</div>
        {desc && <div className="set-row-desc" id={descId}>{desc}</div>}
      </div>
      {children !== undefined && (group
        ? <div className="set-row-ctl" role="group" aria-labelledby={labelId} aria-describedby={descId}>{ctl}</div>
        : <div className="set-row-ctl">{ctl}</div>)}
    </div>
  )
}

export function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <button type="button" role="switch" aria-checked={checked} aria-label={label} disabled={disabled}
      className={'set-switch' + (checked ? ' on' : '')} onClick={() => onChange(!checked)}><i /></button>
  )
}

// Settings stored on the server, edited as a draft until saved.
export function useServerSettings() {
  const [saved, setSaved] = useState<Record<string, string> | null>(null)
  const [draft, setDraft] = useState<Record<string, string> | null>(null)
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => { setFailed(false); api.get<Record<string, string>>('/settings').then((s) => { setSaved(s); setDraft(s) }).catch(() => setFailed(true)) }, [attempt])
  const dirty = !!saved && !!draft && Object.keys(draft).some((k) => (draft[k] ?? '') !== (saved[k] ?? ''))
  const set = (k: string, v: string) => setDraft((d) => ({ ...(d || {}), [k]: v }))
  const on = (k: string, def = '1') => (draft?.[k] ?? def) === '1'
  const save = async () => {
    if (!draft) return
    setBusy(true)
    try {
      const n = await api.put<Record<string, string>>('/settings', draft)
      setSaved(n); setDraft(n); getPrefs(true); toast('Settings saved')
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  return { s: draft, set, on, dirty, busy, save, discard: () => setDraft(saved), failed, retry: () => setAttempt((n) => n + 1) }
}

export function SaveBar({ dirty, busy, save, discard }: { dirty: boolean; busy: boolean; save: () => void; discard: () => void }) {
  if (!dirty) return null
  return (
    <div className="set-savebar" role="region" aria-label="Unsaved changes">
      <span><i className="set-dot" />You have unsaved changes</span>
      <button className="btn ghost sm" onClick={discard} disabled={busy}>Discard</button>
      <button className="btn primary sm" onClick={save} disabled={busy}>{busy ? 'Saving…' : 'Save changes'}</button>
    </div>
  )
}
