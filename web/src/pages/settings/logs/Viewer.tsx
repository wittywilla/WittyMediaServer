import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { announce } from '../../../a11y'
import { api, ApiError } from '../../../api'
import { EmptyState, Spinner } from '../../../components/common'
import { IconClose, IconFile, IconPause, IconPlay, IconRefresh } from '../../../icons'
import { toast } from '../../../state'
import { Section } from '../ui'
import { copyText, LEVEL_WORDS, scrollKeys, type Category, type EntriesPage, type Health, type LogModule, type LogRecord } from './types'

// The live log: newest at the top, so new lines don't push the page around and
// "Load older" is where you'd look for it. Log text is shown as text, always.

const MAX_ROWS = 1000
const PAGE = 200
const TAIL_LIMIT = 500
const POLL_MS = 2000
const POLL_RETRY_MS = 6000
const LONG_ATTRS = ['stack', 'stderr', 'output']
const VALID_SESSION = /^[\w.:-]{1,64}$/

type Row = LogRecord & { key: number }
let nextKey = 1
const tag = (r: LogRecord): Row => ({ ...r, key: nextKey++ })
const hasTime = (t: string) => !!t && !t.startsWith('0001-')

const clock = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
const dated = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit' })
function fmtTime(t: string) {
  if (!hasTime(t)) return ''
  const d = new Date(t)
  return (d.toDateString() === new Date().toDateString() ? clock : dated).format(d)
}

function rowText(r: LogRecord) {
  const parts = [hasTime(r.t) ? r.t : '', (LEVEL_WORDS[r.level] || r.level).toUpperCase(), r.cat, r.msg]
  if (r.module) parts.push(`module=${r.module}`)
  if (r.topic) parts.push(`topic=${r.topic}`)
  if (r.session) parts.push(`session=${r.session}`)
  if (r.req) parts.push(`req=${r.req}`)
  for (const a of r.attrs || []) parts.push(`${a.k}=${a.v}`)
  return parts.filter(Boolean).join(' ')
}

function useVisible() {
  const [visible, setVisible] = useState(!document.hidden)
  useEffect(() => {
    const on = () => setVisible(!document.hidden)
    document.addEventListener('visibilitychange', on)
    return () => document.removeEventListener('visibilitychange', on)
  }, [])
  return visible
}

const rowStops = (li: Element) => [li.querySelector('.lg-head'), ...li.querySelectorAll('.lg-detail pre'), li.querySelector('.lg-detail .lg-actions button')].filter(Boolean) as HTMLElement[]

// Up and down go line by line, through what an open line holds. Left to work out
// the nearest control, a remote would jump from a wide line to the settings menu.
function rowKeys(e: React.KeyboardEvent<HTMLLIElement>) {
  if (e.defaultPrevented || (e.key !== 'ArrowDown' && e.key !== 'ArrowUp')) return
  const li = e.currentTarget
  const stops = rowStops(li)
  const on = document.activeElement
  const i = on?.closest('.lg-actions') ? stops.length - 1 : stops.indexOf(on as HTMLElement)
  if (i < 0) return
  const down = e.key === 'ArrowDown'
  const next = stops[i + (down ? 1 : -1)]
    || (down ? li.nextElementSibling && rowStops(li.nextElementSibling)[0] : li.previousElementSibling && rowStops(li.previousElementSibling).pop())
  if (!next) return // the first or last line: the arrows carry on as usual
  e.preventDefault()
  next.focus({ preventScroll: true })
  next.scrollIntoView({ block: 'nearest' })
}

// One line. Its open or closed state is its own, so opening one doesn't draw the
// other thousand again.
const LogRow = memo(function LogRow({ r, catLabel, modLabel, onSession }: { r: Row; catLabel: string; modLabel?: string; onSession: (id: string) => void }) {
  const [open, setOpen] = useState(false)
  const attrs = r.attrs || []
  const long = attrs.filter((a) => LONG_ATTRS.includes(a.k) || a.v.includes('\n'))
  const short = attrs.filter((a) => !long.includes(a) && a.v !== '')
  const times = attrs.find((a) => a.k === 'count' || a.k === 'repeated')?.v
  const failed = attrs.find((a) => a.k === 'request')?.v
  const copy = async () => toast((await copyText(rowText(r))) ? 'Copied' : 'This browser wouldn’t copy it')
  return (
    <li className={'lg-row lv-' + r.level + (open ? ' open' : '')} onKeyDown={rowKeys}>
      <button type="button" className="lg-head" aria-expanded={open} onClick={() => setOpen(!open)}>
        <time className="lg-time" dateTime={hasTime(r.t) ? r.t : undefined}>{fmtTime(r.t)}</time>
        <span className="lg-level">{LEVEL_WORDS[r.level] || r.level}</span>
        <span className="lg-cat">{catLabel}</span>
        <span className="lg-msg">
          {modLabel && <span className="lg-tag">{modLabel}</span>}
          {r.msg}{times && <span className="lg-times"> ×{times}</span>}
          {r.explain && <span className="lg-hint">{r.explain.title}</span>}
        </span>
      </button>
      {open && (
        <div className="lg-detail">
          {r.explain && (
            <div className="lg-explain">
              <b>{r.explain.title}</b>
              <p>{r.explain.what}</p>
              <p><b>What to do:</b> {r.explain.fix}</p>
            </div>
          )}
          {(short.length > 0 || r.session || r.req || r.module) && (
            <dl className="lg-attrs">
              {r.module && <div><dt>module</dt><dd>{r.module}</dd></div>}
              {r.topic && <div><dt>topic</dt><dd>{r.topic}</dd></div>}
              {r.session && <div><dt>session</dt><dd>{r.session}</dd></div>}
              {r.req && <div><dt>request</dt><dd>{r.req}</dd></div>}
              {short.map((a, i) => <div key={i}><dt>{a.k}</dt><dd>{a.v}</dd></div>)}
            </dl>
          )}
          {long.map((a, i) => (
            <div key={i} className="lg-long">
              <div className="lg-long-name">{a.k}</div>
              <pre className="lg-pre" tabIndex={0} aria-label={a.k} onKeyDown={scrollKeys}>{a.v}</pre>
            </div>
          ))}
          <div className="lg-actions">
            {(r.session || r.req) && <button type="button" className="btn sm" onClick={() => onSession((r.session || r.req)!)}>Show everything from this session</button>}
            {r.session && r.req && <button type="button" className="btn sm ghost" onClick={() => onSession(r.req!)}>Only this request</button>}
            {failed && <button type="button" className="btn sm ghost" onClick={() => onSession(failed)}>Show the request that failed</button>}
            <button type="button" className="btn sm ghost" onClick={copy}>Copy</button>
          </div>
        </div>
      )}
    </li>
  )
})

export default function Viewer({ categories, modules, onHealth }: { categories: Category[]; modules: LogModule[]; onHealth: (h: Health) => void }) {
  const [params, setParams] = useSearchParams()
  const session = VALID_SESSION.test(params.get('session') || '') ? params.get('session')! : ''
  const moduleId = VALID_SESSION.test(params.get('module') || '') ? params.get('module')!.toLowerCase() : ''
  const [level, setLevel] = useState('')
  const [cats, setCats] = useState<string[]>([])
  const [text, setText] = useState('')
  const [q, setQ] = useState('')
  const [sessionText, setSessionText] = useState(session)
  const [rows, setRows] = useState<Row[] | null>(null)
  const [error, setError] = useState('')
  const [pollFailed, setPollFailed] = useState(false)
  const [live, setLive] = useState(true)
  const [older, setOlder] = useState<'more' | 'loading' | 'done'>('more')
  const [partial, setPartial] = useState(false)
  const [skipped, setSkipped] = useState(false)
  const visible = useVisible()
  const last = useRef(0)
  const generation = useRef(0) // answers to an older question are thrown away
  const missingNewest = useRef(false) // "Load older" pushed the newest lines out
  const back = useRef<HTMLButtonElement | null>(null)
  const jumped = useRef(false)

  useEffect(() => { const t = setTimeout(() => setQ(text.trim()), 400); return () => clearTimeout(t) }, [text])
  useEffect(() => { setSessionText(session) }, [session])

  const query = useMemo(() => {
    const p = new URLSearchParams()
    if (level) p.set('level', level)
    if (cats.length) p.set('cat', cats.join(','))
    if (session) p.set('session', session)
    if (moduleId) p.set('module', moduleId)
    if (q) p.set('q', q)
    const s = p.toString()
    return s ? '&' + s : ''
  }, [level, cats, session, moduleId, q])

  const load = useCallback(async () => {
    const g = ++generation.current
    setRows(null); setError(''); setSkipped(false); setPartial(false); setOlder('more'); setPollFailed(false)
    missingNewest.current = false
    try {
      const p = await api.get<EntriesPage>(`/logs/entries?limit=${PAGE}${query}`)
      if (g !== generation.current) return
      last.current = p.last
      if (p.health) onHealth(p.health)
      setPartial(p.partial)
      setRows(p.entries.map(tag).reverse())
    } catch (e: any) {
      if (g === generation.current) { setRows([]); setError(e.message) }
    }
  }, [query, onHealth])
  useEffect(() => { void load() }, [load])

  // The live tail. The next request is only asked for when the last has come
  // back, so a slow server is never queued up on.
  const ready = rows !== null && !error
  useEffect(() => {
    if (!live || !visible || !ready) return
    let stop = false
    let t = 0
    const tick = async () => {
      const g = generation.current
      try {
        const p = await api.get<EntriesPage>(`/logs/entries?limit=${TAIL_LIMIT}&after=${last.current}${query}`)
        if (stop || g !== generation.current) return
        setPollFailed(false)
        if (p.health) onHealth(p.health)
        if (p.last < last.current) { void load(); return } // the server restarted, and counts from the start again
        last.current = p.last
        if (p.more) setSkipped(true)
        if (p.entries.length) {
          const added = p.entries.map(tag).reverse()
          setRows((old) => {
            const all = [...added, ...(old || [])]
            return all.length > MAX_ROWS ? all.slice(0, MAX_ROWS) : all
          })
          setOlder((o) => (o === 'done' ? 'more' : o))
          const bad = p.entries.filter((e) => e.level === 'error' || e.level === 'crash').length
          if (bad) announce(bad === 1 ? 'A new error in the log' : `${bad} new errors in the log`)
        }
        t = window.setTimeout(tick, POLL_MS)
      } catch {
        if (!stop) { setPollFailed(true); t = window.setTimeout(tick, POLL_RETRY_MS) }
      }
    }
    t = window.setTimeout(tick, POLL_MS)
    return () => { stop = true; clearTimeout(t) }
  }, [live, visible, ready, query, load, onHealth])

  const loadOlder = async () => {
    const oldest = [...(rows || [])].reverse().find((r) => hasTime(r.t))
    const g = generation.current
    setOlder('loading')
    try {
      const before = oldest ? `&before=${encodeURIComponent(oldest.t)}` : ''
      const p = await api.get<EntriesPage>(`/logs/entries?limit=${PAGE}&history=1${before}${query}`)
      if (g !== generation.current) return
      setPartial(p.partial)
      setOlder(!p.more && p.entries.length < PAGE ? 'done' : 'more')
      const added = p.entries.map(tag).reverse()
      const all = [...(rows || []), ...added]
      if (all.length > MAX_ROWS) {
        // To stay within the limit the newest lines go, so the tail waits.
        missingNewest.current = true
        setLive(false)
        setRows(all.slice(all.length - MAX_ROWS))
      } else setRows(all)
    } catch (e: any) {
      if (g !== generation.current) return
      setOlder('more')
      toast(e instanceof ApiError && e.status === 429 ? 'Another search of the log files is running. Try again in a moment.' : e.message)
    }
  }

  const resume = () => {
    setLive(true)
    if (missingNewest.current) void load()
  }

  const goSession = useCallback((id: string) => {
    jumped.current = true
    setLive(true)
    setLevel(''); setCats([]); setText(''); setQ('') // everything means everything
    setParams((p) => { const n = new URLSearchParams(p); n.delete('tab'); n.delete('module'); n.set('session', id); return n })
    announce('Showing everything from this session')
  }, [setParams])
  const leaveSession = () => setParams((p) => { const n = new URLSearchParams(p); n.delete('session'); return n })
  // Focus was on a line that has gone: bring it to the way back.
  useEffect(() => {
    if (session && jumped.current) { jumped.current = false; back.current?.focus() }
  }, [session])

  const commitSession = () => {
    const v = sessionText.trim()
    if (v === session) return
    if (v && !VALID_SESSION.test(v)) { toast('That doesn’t look like a session ID'); return }
    setParams((p) => { const n = new URLSearchParams(p); if (v) n.set('session', v); else n.delete('session'); return n }, { replace: true })
  }

  const toggleCat = (id: string) => setCats((c) => (c.includes(id) ? c.filter((x) => x !== id) : [...c, id]))
  const setModule = (id: string) => setParams((p) => { const n = new URLSearchParams(p); if (id) n.set('module', id); else n.delete('module'); return n }, { replace: true })
  const clearFilters = () => { setLevel(''); setCats([]); setText(''); setQ(''); setModule('') }
  const filtered = !!(level || cats.length || text.trim() || moduleId)
  const labels = useMemo(() => new Map(categories.map((c) => [c.id, c.label])), [categories])
  const modNames = useMemo(() => new Map(modules.map((m) => [m.id, m.name])), [modules])

  return (
    <Section title="Live log" desc="Newest first. Open a line for its details, what it means and what to do."
      actions={(
        <div className="lg-live">
          <span className={'set-pill' + (live && visible ? ' live' : '')}>{live ? (visible ? 'Live' : 'Waiting') : 'Paused'}</span>
          {live
            ? <button type="button" className="btn sm" onClick={() => setLive(false)}><IconPause /> Pause</button>
            : <button type="button" className="btn sm" onClick={resume}><IconPlay /> Resume</button>}
        </div>
      )}>
      <div className="lg-filters">
        <label className="lg-field">Show
          <select className="select" value={level} onChange={(e) => setLevel(e.target.value)}>
            <option value="">Everything</option>
            <option value="info">Normal and above</option>
            <option value="warn">Warnings and errors</option>
            <option value="error">Errors only</option>
          </select>
        </label>
        <label className="lg-field">Module
          <select className="select" value={moduleId} onChange={(e) => setModule(e.target.value)}>
            <option value="">All</option>
            {modules.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
            {moduleId && !modNames.has(moduleId) && <option value={moduleId}>{moduleId}</option>}
          </select>
        </label>
        <label className="lg-field grow">Search
          <input className="input" type="search" value={text} placeholder="Words anywhere in a line" onChange={(e) => setText(e.target.value)} />
        </label>
        <label className="lg-field">Session
          <input className="input" type="search" value={sessionText} placeholder="A session or request ID" spellCheck={false} autoCapitalize="off"
            onChange={(e) => setSessionText(e.target.value)} onBlur={commitSession} onKeyDown={(e) => { if (e.key === 'Enter') commitSession() }} />
        </label>
      </div>
      <details className="lg-cats">
        <summary>Areas: {cats.length ? cats.map((c) => labels.get(c) || c).join(', ') : 'all'}</summary>
        <div className="chips">
          {categories.map((c) => (
            <button key={c.id} type="button" className={'chip' + (cats.includes(c.id) ? ' active' : '')} aria-pressed={cats.includes(c.id)} onClick={() => toggleCat(c.id)}>{c.label}</button>
          ))}
        </div>
      </details>
      {filtered && <button type="button" className="btn sm ghost lg-clear" onClick={clearFilters}><IconClose /> Clear filters</button>}

      {session && (
        <div className="lg-session">
          <span>Everything from session <code>{session}</code>, including older log files.</span>
          <button ref={back} type="button" className="btn sm" onClick={leaveSession}>Back to all lines</button>
        </div>
      )}
      {pollFailed && <div className="warn-box lg-note" role="status">The server isn’t answering. Still trying.</div>}
      {skipped && <div className="warn-box lg-note">Lines arrived faster than they could be shown, so some in between are missing. Use <b>Load older</b> or the log files to see them.</div>}
      {partial && <div className="warn-box lg-note">The log files are very large, so only part of them was searched. Narrow the search to find more.</div>}

      {rows === null ? <Spinner />
        : error ? <div className="error lg-note" role="alert">{error}<div className="lg-note-actions"><button type="button" className="btn sm" onClick={() => void load()}><IconRefresh /> Try again</button></div></div>
        : rows.length === 0 ? (
          <EmptyState icon={<IconFile />} title={filtered || session ? 'No lines match' : 'Nothing logged yet'}
            text={filtered || session ? 'Nothing recorded so far fits these filters. New lines will appear here as they arrive.' : 'New lines will appear here as they arrive.'} />
        ) : (
          <ul className="lg-list">
            {rows.map((r) => <LogRow key={r.key} r={r} catLabel={labels.get(r.cat) || r.cat} modLabel={r.module ? modNames.get(r.module) || r.module : undefined} onSession={goSession} />)}
          </ul>
        )}
      {rows !== null && !error && (
        <div className="lg-foot">
          {older === 'done'
            ? <span className="muted">That’s everything the log files hold.</span>
            : <button type="button" className="btn sm" disabled={older === 'loading'} onClick={() => void loadOlder()}>{older === 'loading' ? 'Looking…' : rows.length ? 'Load older' : 'Search older log files'}</button>}
          {rows.length >= MAX_ROWS && <span className="muted">Showing the {MAX_ROWS.toLocaleString()} lines that fit here.</span>}
        </div>
      )}
    </Section>
  )
}
