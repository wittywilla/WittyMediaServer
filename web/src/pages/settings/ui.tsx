import { cloneElement, isValidElement, useEffect, useId, useRef, useState, type ReactElement, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, getPrefs } from '../../api'
import { toast } from '../../state'
import { jumpTo } from './jump'
import { sectionId } from './searchIndex'

// The building blocks of every settings page: a page header, section cards,
// rows (what a setting is and does on the left, its control on the right),
// switches, and a save bar that appears only when something has changed.

export function PageHead({ title, desc, actions }: { title: string; desc?: ReactNode; actions?: ReactNode }) {
  return (
    <>
      <header className="set-head">
        <div><h1>{title}</h1>{desc && <p>{desc}</p>}</div>
        {actions && <div className="set-head-actions">{actions}</div>}
      </header>
      <PageToc />
    </>
  )
}

// A long page's contents: a link to each of its sections, in a strip that stays at the top as
// the page scrolls and marks the section you're in. It reads the sections from the page as
// drawn, so it stays right as a page shows more or less, and only appears where there are
// enough to need it.
const TOC_FROM = 4

function PageToc() {
  const ref = useRef<HTMLElement | null>(null)
  const [items, setItems] = useState<{ id: string; title: string }[]>([])
  const [here, setHere] = useState('')
  const { hash } = useLocation()
  const navigate = useNavigate()
  useEffect(() => {
    const page = ref.current?.closest('.set-main') || ref.current?.parentElement
    if (!page) return
    let frame = 0, last = ''
    const read = () => {
      frame = 0
      // Two sections of one title share an id; the contents link only the first.
      const seen = new Set<string>()
      const found = [...page.querySelectorAll<HTMLElement>('section.set-card[id]')]
        .filter((s) => !s.parentElement?.closest('.set-card') && !seen.has(s.id) && !!seen.add(s.id))
        .map((s) => ({ id: s.id, title: s.querySelector(':scope > header h2')?.textContent?.trim() || '' }))
        .filter((s) => s.title)
      const key = found.map((s) => s.id + ' ' + s.title).join('|')
      // A page with contents of its own (Legal) keeps those.
      const own = !!page.querySelector('nav[aria-label="On this page"]:not(.set-toc)')
      if (key !== last) { last = key; setItems(found.length >= TOC_FROM && !own ? found : []) }
    }
    const later = () => { if (!frame) frame = requestAnimationFrame(read) }
    // Only a section coming or going matters, not every line a page adds (a live log).
    const isSection = (n: Node) => n instanceof HTMLElement && (n.matches('section.set-card') || !!n.querySelector('section.set-card'))
    read()
    const watch = new MutationObserver((recs) => { if (recs.some((r) => [...r.addedNodes, ...r.removedNodes].some(isSection))) later() })
    watch.observe(page, { childList: true, subtree: true })
    return () => { watch.disconnect(); cancelAnimationFrame(frame) }
  }, [])
  useEffect(() => {
    if (!items.length) return
    let frame = 0
    const spy = () => {
      frame = 0
      const line = (ref.current?.getBoundingClientRect().bottom || 0) + 40
      let at = items[0].id
      for (const s of items) { const el = document.getElementById(s.id); if (el && el.getBoundingClientRect().top <= line) at = s.id }
      // At the foot of the page the last sections may never reach the line: the last one counts.
      if (innerHeight + scrollY >= document.documentElement.scrollHeight - 2) at = items[items.length - 1].id
      setHere(at)
    }
    const later = () => { if (!frame) frame = requestAnimationFrame(spy) }
    spy()
    addEventListener('scroll', later, { passive: true })
    addEventListener('resize', later, { passive: true })
    return () => { removeEventListener('scroll', later); removeEventListener('resize', later); cancelAnimationFrame(frame) }
  }, [items])
  if (!items.length) return <span ref={ref} hidden />
  return (
    <nav className="set-toc" aria-label="On this page" ref={ref} data-nav-group>
      <span className="set-toc-label" aria-hidden="true">On this page</span>
      <ul>
        {items.map((s) => (
          <li key={s.id}>
            <a href={'#' + s.id} aria-current={here === s.id ? 'location' : undefined} onClick={(e) => {
              e.preventDefault()
              if (hash === '#' + s.id) jumpTo(s.id); else navigate({ hash: s.id }, { replace: true })
            }}>{s.title}</a>
          </li>
        ))}
      </ul>
    </nav>
  )
}

// A section's id is its title in plain words, so a link or a search result can jump to it.
export function Section({ title, desc, actions, children, id }: { title: string; desc?: ReactNode; actions?: ReactNode; children: ReactNode; id?: string }) {
  return (
    <section className="set-card" id={id || sectionId(title)} aria-label={title}>
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

export { Switch } from '../../ui/Field'

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
