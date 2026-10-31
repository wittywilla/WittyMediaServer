import { EmptyState } from '../../components/common'
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, mediaUrl } from '../../api'
import { announce } from '../../a11y'
import { IconLeft, IconPlay, IconRight, IconSettings, IconTv } from '../../icons'
import { toast, useApp } from '../../state'
import LookPanel from './LookPanel'
import { lastChannel, useLook, type Layout, type Colours } from './look'
import Preview from './Preview'
import { fmtClock, fmtDay, fmtSpan, getLineup, logoSrc, type GuideChannel, type GuidePage as Page, type Programme } from './tvApi'
import './tv.css'
import LegalNote from '../../components/LegalNote'

// The programme guide. Two layouts over one real ARIA grid: the classic box of
// the early 2000s, and clean streaming rows with a line down every channel at
// the current time. The look is each viewer's own (see look.ts). Only the rows
// on screen are loaded and drawn, so a playlist of thousands of channels stays
// quick.

const SLOT = 1800
const PAGE = 30
const FETCH_SPAN = 3 * 3600
const floorSlot = (u: number) => Math.floor(u / SLOT) * SLOT
const DAY_NAMES = 15

type Filter = { kind: '' | 'library' | 'tuner'; group: string }
type Focus = { row: number; at: number }

// The cells of a row for the window on screen; gaps are cells too, so every
// moment of every channel can be reached with the keyboard.
function cellsFor(ch: GuideChannel | undefined, from: number, to: number): Programme[] {
  if (!ch) return []
  const out: Programme[] = []
  let t = from
  const none = (start: number, end: number): Programme => ({ start, end, kind: 'none', title: ch.kind === 'library' ? 'Off air' : 'No guide data' })
  for (const p of ch.programmes) {
    if (p.end <= from || p.start >= to) continue
    if (p.start > t) out.push(none(t, p.start))
    out.push(p)
    t = Math.max(t, p.end)
  }
  if (t < to) out.push(none(t, to))
  return out
}

const useClock = () => {
  const [now, setNow] = useState(() => Date.now() / 1000)
  useEffect(() => { const t = setInterval(() => setNow(Date.now() / 1000), 5000); return () => clearInterval(t) }, [])
  return now
}

function slotsForWidth(w: number) { return w < 560 ? 3 : w < 1000 ? 4 : w < 1500 ? 5 : 6 }
function baseRow(width: number) { return width < 560 ? 50 : width >= 1800 ? 76 : 56 }

export default function GuidePage() {
  const { user } = useApp()
  const nav = useNavigate()
  const now = useClock()
  const [from, setFrom] = useState(() => floorSlot(Date.now() / 1000))
  const [filter, setFilter] = useState<Filter>({ kind: '', group: '' })
  const [rows, setRows] = useState<Record<number, GuideChannel>>({})
  const [total, setTotal] = useState<number | null>(null)
  const [groups, setGroups] = useState<string[]>([])
  const [serverLook, setServerLook] = useState<{ layout?: Layout; colours?: Colours }>({})
  const look = useLook(serverLook.layout, serverLook.colours)
  const [lookOpen, setLookOpen] = useState(false)
  const [err, setErr] = useState('')
  const [focus, setFocus] = useState<Focus>(() => ({ row: 0, at: Date.now() / 1000 }))
  const [details, setDetails] = useState<{ ch: GuideChannel; p: Programme } | null>(null)
  const [dial, setDial] = useState('')
  const [width, setWidth] = useState(() => window.innerWidth)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewH, setViewH] = useState(400)
  const [gutter, setGutter] = useState(0)
  const [lineup, setLineup] = useState<GuideChannel[]>([])
  const scroller = useRef<HTMLDivElement | null>(null)
  const grid = useRef<HTMLDivElement | null>(null)
  const wantFocus = useRef(false)
  const loaded = useRef(new Set<string>())
  const slots = look.hours ? look.hours * 2 : slotsForWidth(width)
  const rowH = Math.round(baseRow(width) * ({ compact: 0.8, normal: 1, large: 1.25 }[look.density]) * (look.layout === 'classic' ? 1.14 : 1))
  const to = from + slots * SLOT
  const fetchFrom = Math.floor(from / FETCH_SPAN) * FETCH_SPAN
  const filterKey = filter.kind + '|' + filter.group

  useEffect(() => {
    const onResize = () => setWidth(window.innerWidth)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  // The scrollbar takes width from the rows: the times over them must line up.
  useLayoutEffect(() => {
    const el = scroller.current
    if (!el) return
    setViewH(el.clientHeight)
    setGutter(el.offsetWidth - el.clientWidth)
  })

  // A changed filter or a new stretch of hours starts the rows afresh.
  useEffect(() => { loaded.current.clear(); setRows({}) }, [filterKey, fetchFrom])
  useEffect(() => { setTotal(null); setFocus((f) => ({ ...f, row: 0 })); if (scroller.current) scroller.current.scrollTop = 0 }, [filterKey])
  useEffect(() => { getLineup().then(setLineup).catch(() => {}) }, [])

  const firstRow = Math.max(0, Math.floor(scrollTop / rowH) - 3)
  const lastRow = Math.ceil((scrollTop + viewH) / rowH) + 3
  const loadPage = useCallback(async (p: number) => {
    const key = `${fetchFrom}|${filterKey}|${p}`
    if (loaded.current.has(key)) return
    loaded.current.add(key)
    try {
      const g = await api.get<Page>(`/guide?from=${fetchFrom}&hours=6&offset=${p * PAGE}&limit=${PAGE}&kind=${filter.kind}&group=${encodeURIComponent(filter.group)}`)
      setTotal(g.total)
      setGroups(g.groups)
      setServerLook({ layout: g.layout, colours: g.skin })
      setRows((r) => { const n = { ...r }; (g.channels || []).forEach((c, i) => { n[g.offset + i] = c }); return n })
      setErr('')
    } catch (e: any) { loaded.current.delete(key); setErr(e.message) }
  }, [fetchFrom, filterKey, filter.kind, filter.group])
  useEffect(() => {
    for (let p = Math.floor(firstRow / PAGE); p <= Math.floor(lastRow / PAGE); p++) void loadPage(p)
  }, [firstRow, lastRow, loadPage])
  // Keep the guide current while it is left open.
  useEffect(() => {
    const t = setInterval(() => { loaded.current.clear(); setScrollTop((s) => s + 0.001) }, 5 * 60000)
    return () => clearInterval(t)
  }, [])

  const cur = rows[focus.row]
  const cells = useMemo(() => cellsFor(cur, from, to), [cur, from, to])
  const curCell = cells.find((c) => c.start <= focus.at && c.end > focus.at) || cells[0]

  const moveRow = (row: number) => {
    const max = (total ?? 1) - 1
    const r = Math.max(0, Math.min(max, row))
    wantFocus.current = true
    setFocus((f) => ({ ...f, row: r }))
    const el = scroller.current
    if (el) {
      const top = r * rowH
      if (top < el.scrollTop) el.scrollTop = top
      else if (top + rowH > el.scrollTop + el.clientHeight) el.scrollTop = top + rowH - el.clientHeight
    }
  }
  const shiftTime = (dt: number, at?: number) => {
    const minFrom = floorSlot(now) - 2 * 3600
    const maxFrom = floorSlot(now) + 14 * 86400
    const next = Math.max(minFrom, Math.min(maxFrom, from + dt))
    setFrom(next)
    wantFocus.current = true
    setFocus((f) => ({ ...f, at: at ?? Math.max(next, Math.min(next + slots * SLOT - 1, f.at + dt)) }))
  }
  const goNow = () => { const f = floorSlot(now); setFrom(f); setFocus((x) => ({ ...x, at: now })); wantFocus.current = true }

  const activate = (ch: GuideChannel, p: Programme) => {
    if (ch.encrypted) { setDetails({ ch, p }); return }
    const live = p.kind === 'none' ? p.start <= now + 60 : p.start <= now && p.end > now
    if (live) nav(`/tv/watch/${ch.key}`)
    else setDetails({ ch, p })
  }

  const dialTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const pressDigit = (d: string) => {
    const next = (dial + d).slice(0, 8)
    setDial(next)
    clearTimeout(dialTimer.current)
    dialTimer.current = setTimeout(async () => {
      setDial('')
      const list = await getLineup().catch(() => [] as GuideChannel[])
      const shown = list.filter((c) => (!filter.kind || c.kind === filter.kind) && (!filter.group || c.group === filter.group))
      const i = shown.findIndex((c) => c.number === next)
      if (i >= 0) { moveRow(i); announce(`Channel ${shown[i].number}, ${shown[i].name}`) } else { announce(`No channel ${next}`); toast(`No channel ${next}`) }
    }, 1200)
  }
  useEffect(() => () => clearTimeout(dialTimer.current), [])

  const record = async (ch: GuideChannel, p: Programme) => {
    try {
      await api.post('/recordings', { channel: ch.key, title: p.title, sub: p.sub, start: p.start, stop: p.end })
      toast(`Recording ${p.title}`)
      announce(`Recording ${p.title}`)
    } catch (e: any) { toast(e.message) }
  }

  const onKey = (e: React.KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const row = focus.row
    const idx = cells.findIndex((c) => c === curCell)
    let handled = true
    switch (e.key) {
      case 'ArrowUp': moveRow(row - 1); break
      case 'ArrowDown': moveRow(row + 1); break
      case 'PageUp': moveRow(row - Math.max(1, Math.floor(viewH / rowH) - 1)); break
      case 'PageDown': moveRow(row + Math.max(1, Math.floor(viewH / rowH) - 1)); break
      case 'Home': goNow(); break
      case 'ArrowRight': {
        const next = cells[idx + 1]
        if (next && next.start < to) { wantFocus.current = true; setFocus({ row, at: next.start }) }
        else shiftTime(SLOT, curCell ? Math.max(curCell.end, to) : undefined)
        break
      }
      case 'ArrowLeft': {
        const prev = cells[idx - 1]
        if (prev) { wantFocus.current = true; setFocus({ row, at: Math.max(prev.start, from) }) }
        else shiftTime(-SLOT, from - SLOT)
        break
      }
      case 'Enter': if (cur && curCell) activate(cur, curCell); break
      case 'i': case 'I': if (cur && curCell) setDetails({ ch: cur, p: curCell }); break
      case 'r': case 'R': if (cur?.canRecord && curCell?.kind === 'broadcast') void record(cur, curCell); break
      case 'a': case 'A': shiftTime(-86400); break
      case 'b': case 'B': shiftTime(86400); break
      case 'c': case 'C': setFilter((f) => ({ ...f, kind: f.kind === '' ? 'library' : f.kind === 'library' ? 'tuner' : '' })); break
      default:
        if (/^[0-9.]$/.test(e.key)) pressDigit(e.key)
        else handled = false
    }
    if (handled) { e.preventDefault(); e.stopPropagation() }
  }

  // Roving focus: the highlighted cell holds the keyboard after each move.
  useEffect(() => {
    if (!wantFocus.current) return
    const el = grid.current?.querySelector<HTMLElement>('[data-focused="1"]')
    if (el) { wantFocus.current = false; el.focus({ preventScroll: true }) }
  })

  const timeHeads = useMemo(() => Array.from({ length: slots }, (_, i) => from + i * SLOT), [from, slots])
  const pct = (u: number) => ((u - from) / (to - from)) * 100
  const nowPct = now >= from && now < to ? pct(now) : null
  const totalRows = total ?? 0
  const label = (ch: GuideChannel, p: Programme) => {
    const when = p.kind === 'none' ? '' : fmtSpan(p)
    const on = p.start <= now && p.end > now ? ', on now' : ''
    const lock = ch.security?.state === 'encrypted' ? `, ${ch.security.label}` : ''
    return `Channel ${ch.number}, ${ch.name}: ${[p.title, p.sub, when].filter(Boolean).join(', ')}${on}${lock}`
  }
  // What the little TV shows: settled on by the preview itself.
  const previewKey = look.preview === 'off' ? '' : look.preview === 'last' ? (lastChannel() || (cur && !cur.encrypted ? cur.key : '')) : (cur && !cur.encrypted ? cur.key : '')
  const previewName = useMemo(() => {
    if (!previewKey) return ''
    const c = cur?.key === previewKey ? cur : lineup.find((x) => x.key === previewKey)
    return c ? `${c.number} ${c.name}` : ''
  }, [previewKey, cur, lineup])

  const shell = (children: React.ReactNode) => (
    <div className={'ctv ctv-page pv-' + (look.preview === 'off' ? 'none' : look.previewPos) + ' pvs-' + look.previewSize} data-skin={look.colours} data-layout={look.layout}>{children}</div>
  )
  if (total === 0 && !filter.kind && !filter.group) {
    // Nothing to show yet, so the guide's own colours have nothing to dress up.
    return (
      <>
        <h1 className="sr-only">TV guide</h1>
        <EmptyState icon={<IconTv />} title="No channels yet"
          text={user?.isAdmin ? 'Make channels out of your library in one click, or add a tuner or an IPTV playlist.' : 'Ask whoever runs this server to set some up.'}
          action={user?.isAdmin ? <Link className="btn primary" to="/settings/channels">Set up channels</Link> : undefined} />
      </>
    )
  }

  const panel = details ? null : curCell
  const dayStart = (u: number) => { const d = new Date(u * 1000); d.setHours(0, 0, 0, 0); return d.getTime() / 1000 }
  const today = dayStart(now)
  const dayOffset = Math.round((dayStart(from) - today) / 86400)
  const goDay = (n: number) => shiftTime(n === 0 ? floorSlot(now) - from : (n - dayOffset) * 86400)
  return shell(
    <>
      <h1 className="sr-only">TV guide</h1>
      <section className="ctv-info" aria-label="Highlighted show">
        {look.preview === 'off' && (
          <div className="ctv-art" aria-hidden="true">
            {panel?.itemId ? <img src={mediaUrl(`/api/items/${panel.itemId}/backdrop`)} alt="" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} /> : null}
            <span className="ctv-art-ch">{cur ? <><b>{cur.number}</b><i>{cur.name}</i></> : <IconTv size={32} />}</span>
          </div>
        )}
        <div className="ctv-info-text">
          <div className="ctv-info-chan">{cur ? `${cur.number} ${cur.name}` : ' '}</div>
          <div className="ctv-info-title">{panel?.title || cur?.name || ' '}</div>
          <div className="ctv-info-sub">{panel?.sub || ' '}</div>
          <div className="ctv-info-meta">
            {panel && panel.kind !== 'none' && <span>{fmtSpan(panel)}</span>}
            {panel?.rating && <span className="ctv-rate">{panel.rating}</span>}
            {panel?.genre && <span>{panel.genre}</span>}
            {panel?.year ? <span>{panel.year}</span> : null}
            {cur?.hd && <span className="ctv-rate">HD</span>}
            {cur?.atsc3 && <span className="ctv-rate">ATSC 3.0</span>}
            {cur?.security?.state === 'clear' && <span className="ctv-rate">Clear</span>}
            {cur?.security?.state === 'unknown' && <span className="ctv-rate" title={cur.security.detail}>{cur.security.label}</span>}
          </div>
          {panel?.desc && <p className="ctv-info-desc">{panel.desc}</p>}
          {cur?.security?.state === 'encrypted' && (
            <p className="ctv-info-desc"><b>{cur.security.label}.</b> {cur.security.detail} <Link className="ctv-link" to="/tv/encrypted">What can I do?</Link></p>
          )}
          {cur?.notices?.map((n) => <p key={n} className="ctv-info-desc">{n}</p>)}
          {cur?.alternative && (
            <div className="ctv-info-actions">
              <button className="ctv-btn on" onClick={() => nav(`/tv/watch/${cur.alternative!.key}`)}><IconPlay size={15} /> Watch {cur.alternative.number} {cur.alternative.name} instead</button>
            </div>
          )}
          {cur && panel && !cur.encrypted && (
            <div className="ctv-info-actions">
              <button className="ctv-btn on" onClick={() => activate(cur, panel)}><IconPlay size={15} /> {panel.kind === 'none' || (panel.start <= now && panel.end > now) ? 'Watch' : 'Details'}</button>
              {cur.canRecord && panel.kind === 'broadcast' && panel.end > now && <button className="ctv-btn" onClick={() => void record(cur, panel)}>● Record</button>}
            </div>
          )}
        </div>
        <div className="ctv-clock" aria-hidden="true">
          <b>{fmtClock(now)}</b>
          <span>{fmtDay(now)}</span>
        </div>
      </section>
      {look.preview !== 'off' && <Preview channel={previewKey} label={previewName} look={look} />}

      <div className="ctv-bar">
        <div className="ctv-bar-group" role="group" aria-label="Time">
          <button className="ctv-btn sm" onClick={() => shiftTime(-SLOT)} aria-label="Half an hour earlier"><IconLeft size={16} /></button>
          <button className="ctv-btn sm" onClick={goNow}>Now</button>
          <button className="ctv-btn sm" onClick={() => shiftTime(SLOT)} aria-label="Half an hour later"><IconRight size={16} /></button>
          <label className="ctv-field"><span className="sr-only">Day</span>
            <select value={dayOffset} onChange={(e) => goDay(Number(e.target.value))}>
              {dayOffset < 0 && <option value={dayOffset}>Yesterday</option>}
              {Array.from({ length: DAY_NAMES }, (_, i) => <option key={i} value={i}>{i === 0 ? 'Today' : i === 1 ? 'Tomorrow' : fmtDay(today + i * 86400 + 3600)}</option>)}
            </select>
          </label>
        </div>
        <div className="ctv-bar-group">
          <label className="ctv-field">Show
            <select value={filter.kind} onChange={(e) => setFilter({ kind: e.target.value as Filter['kind'], group: '' })}>
              <option value="">All channels</option>
              <option value="library">My library</option>
              <option value="tuner">Live TV</option>
            </select>
          </label>
          {groups.length > 0 && (
            <label className="ctv-field">Group
              <select value={filter.group} onChange={(e) => setFilter((f) => ({ ...f, group: e.target.value }))}>
                <option value="">Any</option>
                {groups.map((g) => <option key={g} value={g}>{g}</option>)}
              </select>
            </label>
          )}
          <button className="ctv-btn sm" onClick={() => setLookOpen(true)} aria-haspopup="dialog">Look</button>
          <Link className="ctv-btn sm" to="/tv/recordings">Recordings</Link>
          {user?.isAdmin && <Link className="ctv-btn sm" to="/settings/channels" aria-label="Set up channels and tuners" title="Set up channels and tuners"><IconSettings size={16} /></Link>}
        </div>
      </div>

      {err && <div className="ctv-err" role="alert">{err}</div>}
      {dial && <div className="ctv-dial" role="status">CH {dial}<span className="ctv-blink">_</span></div>}

      <div className="ctv-grid" ref={grid} role="grid" aria-label={`TV guide, ${fmtDay(from)}`} aria-rowcount={totalRows + 1} aria-colcount={slots + 1} onKeyDown={onKey}
        style={{ '--ctv-row': rowH + 'px', '--ctv-gutter': gutter + 'px' } as React.CSSProperties}>
        <div className="ctv-times" role="row" aria-rowindex={1}>
          <div className="ctv-corner" role="columnheader" aria-colindex={1}>Channel</div>
          <div className="ctv-time-track">
            {timeHeads.map((t, i) => <div key={t} role="columnheader" aria-colindex={i + 2} className="ctv-time" style={{ left: pct(t) + '%', width: 100 / slots + '%' }}>{fmtClock(t)}</div>)}
          </div>
        </div>
        {look.nowLine && nowPct != null && (
          <div className="ctv-nowline" aria-hidden="true" style={{ left: `calc(var(--ctv-chw) + (100% - var(--ctv-chw) - var(--ctv-gutter)) * ${nowPct / 100})` }}>
            <span>{fmtClock(now)}</span>
          </div>
        )}
        <div className="ctv-rows" ref={scroller} onScroll={(e) => setScrollTop((e.target as HTMLElement).scrollTop)} role="presentation">
          <div style={{ height: totalRows * rowH, position: 'relative' }} role="presentation">
            {Array.from({ length: Math.max(0, Math.min(lastRow, totalRows - 1) - firstRow + 1) }, (_, i) => firstRow + i).map((r) => {
              const ch = rows[r]
              const rowCells = cellsFor(ch, from, to)
              return (
                <div key={r} className={'ctv-row' + (r === focus.row ? ' current' : '')} role="row" aria-rowindex={r + 2} style={{ top: r * rowH, height: rowH }}>
                  {ch ? (
                    <div className="ctv-ch" role="rowheader" aria-colindex={1} title={ch.name}>
                      <span className="ctv-num">{ch.number}</span>
                      {look.logos && ch.logo ? <img className="ctv-logo" src={logoSrc(ch.logo)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} /> : null}
                      <span className="ctv-name">{ch.name}</span>
                      {ch.encrypted && <span className="ctv-lock" aria-hidden="true">🔒</span>}
                    </div>
                  ) : <div className="ctv-ch" role="rowheader" aria-colindex={1}><span className="ctv-name">…</span></div>}
                  <div className="ctv-track">
                    {ch && rowCells.map((p) => {
                      const start = Math.max(p.start, from), end = Math.min(p.end, to)
                      const focused = r === focus.row && p === (rowCells.find((c) => c.start <= focus.at && c.end > focus.at) || rowCells[0])
                      const colStart = Math.floor((start - from) / SLOT)
                      const colSpan = Math.max(1, Math.ceil((end - from) / SLOT) - colStart)
                      const live = p.start <= now && p.end > now
                      return (
                        <button key={p.start + '-' + p.end} type="button" role="gridcell" aria-colindex={colStart + 2} aria-colspan={colSpan}
                          aria-label={label(ch, p)} aria-selected={focused} tabIndex={focused ? 0 : -1} data-focused={focused ? '1' : undefined}
                          className={'ctv-cell' + (focused ? ' focus' : '') + (live ? ' live' : '') + (p.kind === 'none' || p.kind === 'slate' ? ' dim' : '') + (ch.encrypted ? ' locked' : '') + (p.end <= now ? ' past' : '')}
                          style={{ left: pct(start) + '%', width: pct(end) - pct(start) + '%' }}
                          onClick={() => { if (focused) activate(ch, p); else { setFocus({ row: r, at: Math.max(p.start, from) }); wantFocus.current = true } }}
                          onDoubleClick={() => activate(ch, p)}>
                          {look.progress && live && p.kind !== 'none' && <i className="ctv-fill" aria-hidden="true" style={{ width: ((now - start) / Math.max(1, end - start)) * 100 + '%' }} />}
                          {p.start < from && <span className="ctv-more" aria-hidden="true">◄</span>}
                          <span className="ctv-cell-text">
                            <b>{p.title}</b>
                            {look.subtitles && p.sub && <small>{p.sub}</small>}
                            {look.layout === 'streaming' && live && p.kind !== 'none' && <small className="ctv-left">{Math.max(1, Math.round((p.end - now) / 60))} min left</small>}
                          </span>
                          {p.end > to && <span className="ctv-more" aria-hidden="true">►</span>}
                        </button>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      </div>
      {total === 0 && <div className="ctv-err">No channels match that.</div>}

      <footer className="ctv-help" aria-label="Keys">
        <button className="ctv-key a" onClick={() => shiftTime(-86400)}><b>A</b> Day −</button>
        <button className="ctv-key b" onClick={() => shiftTime(86400)}><b>B</b> Day +</button>
        <button className="ctv-key c" onClick={() => setFilter((f) => ({ ...f, kind: f.kind === '' ? 'library' : f.kind === 'library' ? 'tuner' : '' }))}><b>C</b> {filter.kind === '' ? 'All' : filter.kind === 'library' ? 'Library' : 'Live TV'}</button>
        <span className="ctv-hint hide-narrow">▲▼ Channel · ◄► Time · OK Watch · 0–9 Dial · I Info · R Record</span>
      </footer>
      <LegalNote kind="tv" className="ctv-legal" />

      {details && <Details ch={details.ch} p={details.p} now={now} onClose={() => setDetails(null)} onWatch={() => nav(`/tv/watch/${details.ch.key}`)} onRecord={() => { void record(details.ch, details.p); setDetails(null) }} />}
      {lookOpen && <LookPanel look={look} onClose={() => setLookOpen(false)} />}
    </>
  )
}

function Details({ ch, p, now, onClose, onWatch, onRecord }: { ch: GuideChannel; p: Programme; now: number; onClose: () => void; onWatch: () => void; onRecord: () => void }) {
  const [desc, setDesc] = useState(p.desc || '')
  useEffect(() => {
    if (p.desc || !p.itemId) return
    api.get<{ overview: string }>(`/items/${p.itemId}`).then((it) => setDesc(it.overview)).catch(() => {})
  }, [p.desc, p.itemId])
  const addSeries = async () => {
    try { await api.post('/recording-rules', { channel: ch.key, title: p.title }); toast(`Every airing of ${p.title} will be recorded`); onClose() } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal ctv-details" role="dialog" aria-modal="true" aria-labelledby="ctv-d-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="ctv-d-h">{p.title}</h2>
        {p.sub && <p className="muted">{p.sub}</p>}
        <p className="muted">Channel {ch.number}, {ch.name}{p.kind !== 'none' && <> · {fmtDay(p.start)}, {fmtSpan(p)}</>}{p.rating && <> · {p.rating}</>}</p>
        {desc && <p>{desc}</p>}
        {ch.security?.state === 'encrypted' && <p role="alert"><b>{ch.security.label}.</b> {ch.security.detail} <Link to="/tv/encrypted">What can I do?</Link></p>}
        {ch.security?.state === 'unknown' && <p className="muted" style={{ fontSize: 13 }}>{ch.security.detail}</p>}
        {ch.noRecord && <p className="muted" style={{ fontSize: 13 }}>This channel can't be recorded: {ch.noRecord}.</p>}
        {ch.notices?.map((n) => <p key={n} className="muted" style={{ fontSize: 13 }}>{n}</p>)}
        {ch.kind === 'library' && <p className="muted" style={{ fontSize: 13 }}>Library channels can't be recorded: what they play is already in your library.</p>}
        <div className="actions">
          <button className="btn ghost" onClick={onClose}>Close</button>
          {ch.canRecord && p.kind === 'broadcast' && p.end > now && <><button className="btn" onClick={() => void addSeries()}>Record every airing</button><button className="btn" onClick={onRecord}>Record this</button></>}
          {ch.alternative && <Link className="btn primary" to={`/tv/watch/${ch.alternative.key}`}><IconPlay /> Watch {ch.alternative.number} {ch.alternative.name} instead</Link>}
          {!ch.encrypted && <button className={'btn' + (ch.alternative ? '' : ' primary')} onClick={onWatch}><IconPlay /> Watch channel now</button>}
        </div>
      </div>
    </div>
  )
}
