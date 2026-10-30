import { useEffect, useState, type ReactNode } from 'react'
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
export function Row({ label, desc, badge, children, stack, dim }: { label: ReactNode; desc?: ReactNode; badge?: ReactNode; children?: ReactNode; stack?: boolean; dim?: boolean }) {
  return (
    <div className={'set-row' + (stack ? ' stack' : '') + (dim ? ' dim' : '')}>
      <div className="set-row-text">
        <div className="set-row-label">{label}{badge && <span className="set-badge">{badge}</span>}</div>
        {desc && <div className="set-row-desc">{desc}</div>}
      </div>
      {children !== undefined && <div className="set-row-ctl">{children}</div>}
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
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => { setSaved(s); setDraft(s) }).catch((e) => toast(e.message)) }, [])
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
  return { s: draft, set, on, dirty, busy, save, discard: () => setDraft(saved) }
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
