import { EmptyState } from '../../components/common'
import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../../api'
import { announce } from '../../a11y'
import { IconLeft, IconRight, IconSearch, IconSettings, IconStar, IconTv } from '../../icons'
import { toast, useApp } from '../../state'
import { lookupLogo } from '../../components/BrandLogo'
import LookPanel from './LookPanel'
import { channelVars, customVars, lastChannel, previousChannel, setLook, surfaceOf, useLook, type Colours, type Layout, type Look } from './look'
import { ChannelMark } from './ChannelLogo'
import DetailsDialog from './Details'
import Hero from './Hero'
import SearchPanel from './SearchPanel'
import { useReminders } from './reminders'
import { dayWord, fmtClock, fmtDay, fmtSpan, getLineup, forgetLineup, setFavourite, type GuideChannel, type GuidePage as Page, type Programme } from './tvApi'
import './tv.css'
import LegalNote from '../../components/LegalNote'

// The programme guide. Two looks over one real ARIA grid: the classic box of the
// early 2000s, and a clean modern one. At the top, the highlighted channel: its
// programme, what's next, what can be done with it and a live preview. Below,
// the rows. The look is each viewer's own (see look.ts). Only the rows on screen
// are loaded and drawn, so a playlist of thousands of channels stays quick.

const SLOT = 1800
const PAGE = 30
const FETCH_SPAN = 3 * 3600
const FETCH_HOURS = 8
const floorSlot = (u: number) => Math.floor(u / SLOT) * SLOT
const DAY_NAMES = 15
// A cell narrower than this on screen can't show anything: several in a row are
// drawn as one, which says how many.
const TINY_PX = 14

type Filter = { kind: '' | 'library' | 'tuner'; group: string; favourites: boolean }
type Focus = { row: number; at: number }

// The cells of a row for the window on screen; gaps are cells too, so every
// moment of every channel can be reached with the keyboard.
function cellsFor(ch: GuideChannel | undefined, from: number, to: number, pxPerSec: number): Programme[] {
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
  return groupTiny(out, pxPerSec, from, to)
}

function groupTiny(cells: Programme[], pxPerSec: number, from: number, to: number): Programme[] {
  if (!pxPerSec) return cells
  const width = (p: Programme) => (Math.min(p.end, to) - Math.max(p.start, from)) * pxPerSec
  const out: Programme[] = []
  for (let i = 0; i < cells.length; i++) {
    let j = i
    while (j + 1 < cells.length && width(cells[j]) < TINY_PX && width(cells[j + 1]) < TINY_PX) j++
    if (j > i) {
      const group = cells.slice(i, j + 1)
      out.push({ start: group[0].start, end: group[group.length - 1].end, kind: 'group', title: `${group.length} short programmes`, group })
      i = j
    } else out.push(cells[i])
  }
  return out
}

// The clock, a little at a time: this redraws the page, so not every second.
const useClock = (ms: number) => {
  const [now, setNow] = useState(() => Date.now() / 1000)
  useEffect(() => { const t = setInterval(() => setNow(Date.now() / 1000), ms); return () => clearInterval(t) }, [ms])
  return now
}

function slotsForWidth(w: number) { return w < 560 ? 3 : w < 1000 ? 4 : w < 1500 ? 5 : 6 }
function baseRow(width: number, layout: Layout) { return width < 560 ? 56 : width >= 1800 ? 84 : layout === 'streaming' ? 68 : 56 }

// "Tomorrow 8 PM" and "Tomorrow morning": times people actually jump to.
function jumps(now: number): { label: string; at: number }[] {
  const at = (days: number, hour: number) => { const d = new Date(now * 1000); d.setDate(d.getDate() + days); d.setHours(hour, 0, 0, 0); return d.getTime() / 1000 }
  const tonight = at(0, 20)
  return [tonight > now + 1800 ? { label: 'Tonight 8 PM', at: tonight } : { label: 'Tomorrow 8 PM', at: at(1, 20) }, { label: 'Tomorrow morning', at: at(1, 8) }]
}

const matches = (c: GuideChannel, f: Filter) => (!f.kind || c.kind === f.kind) && (!f.group || c.group === f.group || !!c.categories?.includes(f.group)) && (!f.favourites || !!c.favourite)

export default function GuidePage() {
  const { user } = useApp()
  const nav = useNavigate()
  const now = useClock(30000)
  const [from, setFrom] = useState(() => floorSlot(Date.now() / 1000))
  const [filter, setFilter] = useState<Filter>({ kind: '', group: '', favourites: false })
  const [rows, setRows] = useState<Record<number, GuideChannel>>({})
  const [total, setTotal] = useState<number | null>(null)
  const [groups, setGroups] = useState<string[]>([])
  const [serverLook, setServerLook] = useState<{ layout?: Layout; colours?: Colours }>({})
  const look = useLook(serverLook.layout, serverLook.colours)
  const [lookOpen, setLookOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [err, setErr] = useState('')
  const [focus, setFocus] = useState<Focus>(() => ({ row: 0, at: Date.now() / 1000 }))
  const [details, setDetails] = useState<{ ch: GuideChannel; p: Programme; day?: boolean } | null>(null)
  const [dial, setDial] = useState('')
  const [width, setWidth] = useState(() => window.innerWidth)
  const [pageSize, setPageSize] = useState({ w: window.innerWidth, h: window.innerHeight })
  const [scrollTop, setScrollTop] = useState(0)
  const [viewH, setViewH] = useState(400)
  const [gutter, setGutter] = useState(0)
  const [trackW, setTrackW] = useState(0)
  const [lineup, setLineup] = useState<GuideChannel[]>([])
  const page = useRef<HTMLDivElement | null>(null)
  const scroller = useRef<HTMLDivElement | null>(null)
  const grid = useRef<HTMLDivElement | null>(null)
  const wantFocus = useRef(true)
  const gridHadFocus = useRef(false)
  // Search takes the keyboard as it opens, so it can't be left to the dialog code
  // to work out where the keyboard came from.
  const searchBack = useRef<HTMLElement | null>(null)
  const openSearch = () => { searchBack.current = document.activeElement as HTMLElement | null; setSearchOpen(true) }
  const closeSearch = () => {
    setSearchOpen(false)
    const back = searchBack.current
    requestAnimationFrame(() => { if (back?.isConnected) back.focus({ preventScroll: true }); else wantFocus.current = true })
  }
  const loaded = useRef(new Set<string>())
  const slots = look.hours ? look.hours * 2 : slotsForWidth(width)
  const rowH = Math.round(baseRow(width, look.layout) * ({ compact: 0.8, normal: 1, large: 1.25 }[look.density]) * (look.layout === 'classic' ? 1.14 : 1))
  const to = from + slots * SLOT
  const fetchFrom = Math.floor(from / FETCH_SPAN) * FETCH_SPAN
  const filterKey = filter.kind + '|' + filter.group + '|' + filter.favourites
  const pxPerSec = trackW ? trackW / (to - from) : 0
  const reminders = useReminders()

  useEffect(() => {
    const onResize = () => setWidth(window.innerWidth)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  // The page's own size, which everything about the preview is held to.
  const pageEl = page.current
  useEffect(() => {
    if (!pageEl) return
    const ro = new ResizeObserver(() => setPageSize({ w: pageEl.clientWidth, h: pageEl.clientHeight }))
    ro.observe(pageEl)
    return () => ro.disconnect()
  }, [pageEl])
  // The scrollbar takes width from the rows: the times over them must line up.
  useLayoutEffect(() => {
    const el = scroller.current
    if (!el) return
    setViewH(el.clientHeight)
    setGutter(el.offsetWidth - el.clientWidth)
    const track = el.querySelector<HTMLElement>('.ctv-track')
    if (track) setTrackW(track.clientWidth)
  })

  // A changed filter starts the rows afresh. A new stretch of hours keeps them until
  // the new ones come, so the highlight has somewhere to be meanwhile.
  useEffect(() => { loaded.current.clear(); setRows({}) }, [filterKey])
  useEffect(() => { loaded.current.clear() }, [fetchFrom])
  useEffect(() => { setTotal(null); setFocus((f) => ({ ...f, row: 0 })); if (scroller.current) scroller.current.scrollTop = 0 }, [filterKey])
  useEffect(() => { getLineup().then(setLineup).catch(() => {}) }, [])
  // The rights notice goes with the logos: it's there only when one is on the screen.
  const [logosShown, setLogosShown] = useState(false)
  useEffect(() => {
    const brands = lineup.flatMap((c) => (c.brand ? [c.brand] : []))
    if (!look.logos || !brands.length) { setLogosShown(false); return }
    let on = true
    Promise.all(brands.slice(0, 200).map((b) => lookupLogo(b.kind, b.name))).then((l) => { if (on) setLogosShown(l.some((x) => x.has)) })
    return () => { on = false }
  }, [lineup, look.logos])

  const firstRow = Math.max(0, Math.floor(scrollTop / rowH) - 3)
  const lastRow = Math.ceil((scrollTop + viewH) / rowH) + 3
  const loadPage = useCallback(async (p: number) => {
    const key = `${fetchFrom}|${filterKey}|${p}`
    if (loaded.current.has(key)) return
    loaded.current.add(key)
    try {
      const g = await api.get<Page>(`/guide?from=${fetchFrom}&hours=${FETCH_HOURS}&offset=${p * PAGE}&limit=${PAGE}&kind=${filter.kind}&group=${encodeURIComponent(filter.group)}${filter.favourites ? '&favourites=1' : ''}`)
      setTotal(g.total)
      setGroups((was) => (filter.group || filter.favourites || filter.kind ? [...new Set([...was, ...g.groups])] : g.groups))
      setServerLook({ layout: g.layout, colours: g.skin })
      setRows((r) => { const n = { ...r }; (g.channels || []).forEach((c, i) => { n[g.offset + i] = c }); return n })
      setErr('')
    } catch (e) { loaded.current.delete(key); setErr((e as Error).message) }
  }, [fetchFrom, filterKey, filter.kind, filter.group, filter.favourites])
  useEffect(() => {
    for (let p = Math.floor(firstRow / PAGE); p <= Math.floor(lastRow / PAGE); p++) void loadPage(p)
  }, [firstRow, lastRow, loadPage])
  // Keep the guide current while it is left open.
  useEffect(() => {
    const t = setInterval(() => { loaded.current.clear(); setScrollTop((s) => s + 0.001) }, 5 * 60000)
    return () => clearInterval(t)
  }, [])

  const cur = rows[focus.row]
  const cells = useMemo(() => cellsFor(cur, from, to, pxPerSec), [cur, from, to, pxPerSec])
  const curCell = cells.find((c) => c.start <= focus.at && c.end > focus.at) || cells[0]
  // What the top of the page is about: the highlighted programme, and what follows it.
  const panelCell = details ? (details.p.kind === 'group' ? curCell : details.p) : curCell
  // A run of very short programmes is one cell, but the top of the page says which one.
  const panel = panelCell?.group ? panelCell.group.find((c) => c.start <= focus.at && c.end > focus.at) || panelCell.group[0] : panelCell
  const nextShow = useMemo(() => {
    if (!cur || !panel) return undefined
    return cur.programmes.find((p) => p.start >= panel.end && p.kind !== 'slate' && p.kind !== 'none')
  }, [cur, panel])

  const moveRow = useCallback((row: number) => {
    setTotal((tot) => {
      const r = Math.max(0, Math.min((tot ?? 1) - 1, row))
      wantFocus.current = true
      setFocus((f) => ({ ...f, row: r }))
      const el = scroller.current
      if (el) {
        const top = r * rowH
        if (top < el.scrollTop) el.scrollTop = top
        else if (top + rowH > el.scrollTop + el.clientHeight) el.scrollTop = top + rowH - el.clientHeight
      }
      return tot
    })
  }, [rowH])
  const shiftTime = (dt: number, at?: number) => {
    const minFrom = floorSlot(now) - 2 * 3600
    const maxFrom = floorSlot(now) + 14 * 86400
    const next = Math.max(minFrom, Math.min(maxFrom, from + dt))
    setFrom(next)
    wantFocus.current = true
    setFocus((f) => ({ ...f, at: at ?? Math.max(next, Math.min(next + slots * SLOT - 1, f.at + dt)) }))
  }
  const goAt = (at: number, row?: number) => {
    setFrom(floorSlot(at))
    setFocus((f) => ({ row: row ?? f.row, at }))
    wantFocus.current = true
    if (row != null) moveRow(row)
  }
  const goNow = () => goAt(Date.now() / 1000)

  const watch = (key: string) => nav(`/tv/watch/${key}`)
  const activate = (ch: GuideChannel, p: Programme) => {
    if (ch.encrypted || p.kind === 'group') { setDetails({ ch, p }); return }
    const live = p.kind === 'none' ? p.start <= now + 60 : p.start <= now && p.end > now
    if (live) watch(ch.key)
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
      const shown = list.filter((c) => matches(c, filter))
      let i = shown.findIndex((c) => c.number === next)
      // On another filter: leave it, to get there.
      if (i < 0 && list.some((c) => c.number === next)) {
        setFilter({ kind: '', group: '', favourites: false })
        i = list.findIndex((c) => c.number === next)
        setTimeout(() => moveRow(i), 0)
        announce(`Channel ${list[i].number}, ${list[i].name}`)
        return
      }
      if (i >= 0) { moveRow(i); announce(`Channel ${shown[i].number}, ${shown[i].name}`) } else { announce(`No channel ${next}`); toast(`No channel ${next}`) }
    }, 1200)
  }
  useEffect(() => () => clearTimeout(dialTimer.current), [])

  const record = async (ch: GuideChannel, p: Programme) => {
    try {
      await api.post('/recordings', { channel: ch.key, title: p.title, sub: p.sub, start: p.start, stop: p.end })
      toast(`Recording ${p.title}`)
      announce(`Recording ${p.title}`)
    } catch (e) { toast((e as Error).message) }
  }
  const toggleFav = async (ch: GuideChannel) => {
    const on = !ch.favourite
    setRows((r) => { const n = { ...r }; for (const k of Object.keys(n)) if (n[Number(k)]?.key === ch.key) n[Number(k)] = { ...n[Number(k)], favourite: on }; return n })
    announce(on ? `${ch.name} added to favourites` : `${ch.name} removed from favourites`)
    try { await setFavourite(ch.key, on); forgetLineup(); getLineup().then(setLineup).catch(() => {}) } catch (e) { toast((e as Error).message) }
  }
  const jumpToLast = () => {
    const key = cur && lastChannel() === cur.key ? previousChannel() : lastChannel()
    const c = lineup.find((x) => x.key === key)
    if (!c) { toast('No other channel watched yet'); return }
    const i = lineup.filter((x) => matches(x, filter)).findIndex((x) => x.key === key)
    if (i >= 0) { moveRow(i); announce(`Channel ${c.number}, ${c.name}`) } else watch(key)
  }
  const cycleKind = () => setFilter((f) => ({ ...f, kind: f.kind === '' ? 'library' : f.kind === 'library' ? 'tuner' : '' }))

  const onKey = (e: React.KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const row = focus.row
    const idx = cells.findIndex((c) => c === curCell)
    let handled = true
    switch (e.key) {
      case 'ArrowUp':
        // The top row hands the arrows back, so a remote can reach the buttons above.
        if (row === 0) return
        moveRow(row - 1); break
      case 'ArrowDown':
        // The same at the bottom, for what is under the guide.
        if (total != null && row >= total - 1) return
        moveRow(row + 1); break
      case 'PageUp': case 'ChannelUp': moveRow(row - Math.max(1, Math.floor(viewH / rowH) - 1)); break
      case 'PageDown': case 'ChannelDown': moveRow(row + Math.max(1, Math.floor(viewH / rowH) - 1)); break
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
        else if (from <= floorSlot(now) - 2 * 3600) return
        else shiftTime(-SLOT, from - SLOT)
        break
      }
      case 'Enter': if (cur && curCell) activate(cur, curCell); break
      case 'i': case 'I': if (cur && curCell) setDetails({ ch: cur, p: curCell }); break
      case 'r': case 'R': if (cur?.canRecord && curCell?.kind === 'broadcast') void record(cur, curCell); break
      case 'f': case 'F': if (cur) void toggleFav(cur); break
      case 'd': case 'D': if (cur && curCell) setDetails({ ch: cur, p: curCell, day: true }); break
      case 'm': case 'M': if (cur && curCell && curCell.start > now) void reminders.toggle(cur.key, curCell); break
      case 'l': case 'L': jumpToLast(); break
      case 's': case 'S': case '/': openSearch(); break
      case 'n': case 'N': goNow(); break
      case '+': case '=': setLook({ hours: Math.max(1.5, (look.hours || slots / 2) - 0.5) as Look['hours'] }); break
      case '-': setLook({ hours: Math.min(4, (look.hours || slots / 2) + 0.5) as Look['hours'] }); break
      case 'a': case 'A': shiftTime(-86400); break
      case 'b': case 'B': shiftTime(86400); break
      case 'c': case 'C': cycleKind(); break
      default:
        if (/^[0-9.]$/.test(e.key)) pressDigit(e.key)
        else handled = false
    }
    if (handled) { e.preventDefault(); e.stopPropagation() }
  }

  // New data can replace the cell that has the keyboard. The browser then drops it on
  // the page, where the next arrow would start from nowhere: it goes back to the cell.
  useLayoutEffect(() => {
    if (gridHadFocus.current && document.activeElement === document.body) wantFocus.current = true
  }, [rows, from])

  // Roving focus: the highlighted cell holds the keyboard after each move, before the
  // next key can arrive.
  useLayoutEffect(() => {
    if (!wantFocus.current) return
    const el = grid.current?.querySelector<HTMLElement>('[data-focused="1"]')
    if (el) { wantFocus.current = false; el.focus({ preventScroll: true }) }
  })

  // The rows near what's on screen, and always the highlighted one: it's where the
  // arrows come back to, however far away the rows have been scrolled.
  const rowCount = total ?? 0
  const drawnRows = Array.from({ length: Math.max(0, Math.min(lastRow, rowCount - 1) - firstRow + 1) }, (_, i) => firstRow + i)
  if (focus.row < rowCount && !drawnRows.includes(focus.row)) drawnRows.push(focus.row)
  const timeHeads = useMemo(() => Array.from({ length: slots }, (_, i) => from + i * SLOT), [from, slots])
  const pct = (u: number) => ((u - from) / (to - from)) * 100
  const totalRows = total ?? 0
  // What the little TV shows: settled on by the preview itself.
  const previewKey = look.preview === 'off' ? '' : look.preview === 'last' ? (lastChannel() || (cur && !cur.encrypted ? cur.key : '')) : (cur && !cur.encrypted ? cur.key : '')
  const previewName = useMemo(() => {
    if (!previewKey) return ''
    const c = cur?.key === previewKey ? cur : lineup.find((x) => x.key === previewKey)
    return c ? `${c.number} ${c.name}` : ''
  }, [previewKey, cur, lineup])

  // The most the preview may take: never so much that the words beside it or the
  // guide below have no room. A phone gives it the whole width, on its own line.
  const narrow = pageSize.w < 760
  const previewMax = Math.round(Math.min(narrow ? pageSize.w - 32 : pageSize.w * 0.55, (pageSize.h * (narrow ? 0.3 : 0.5) - 40) * (16 / 9)))

  const vars = { ...customVars(look) }
  const shell = (children: React.ReactNode) => (
    <div ref={page} className={'ctv ctv-page' + (narrow ? ' narrow' : '')} data-skin={look.colours} data-layout={look.layout} style={vars}>{children}</div>
  )
  if (total === 0 && !filter.kind && !filter.group && !filter.favourites) {
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

  const dayStart = (u: number) => { const d = new Date(u * 1000); d.setHours(0, 0, 0, 0); return d.getTime() / 1000 }
  const today = dayStart(now)
  const dayOffset = Math.round((dayStart(from) - today) / 86400)
  const goDay = (n: number) => shiftTime(n === 0 ? floorSlot(now) - from : (n - dayOffset) * 86400)
  const hasBoth = lineup.some((c) => c.kind === 'tuner') && lineup.some((c) => c.kind === 'library')
  const anyFav = lineup.some((c) => c.favourite)
  return shell(
    <>
      <h1 className="sr-only">TV guide</h1>
      <Hero ch={cur} p={panel} next={nextShow} look={look} now={now} previewKey={previewKey} previewName={previewName} previewMax={previewMax}
        onWatch={watch} onDetails={() => { if (cur && curCell) setDetails({ ch: cur, p: curCell }) }} onDay={() => { if (cur && curCell) setDetails({ ch: cur, p: curCell, day: true }) }}
        onFav={() => { if (cur) void toggleFav(cur) }} onUp={() => moveRow(focus.row + 1)} onDown={() => moveRow(focus.row - 1)} onRecord={() => { if (cur && panel) void record(cur, panel) }} />

      <div className="ctv-bar">
        <div className="ctv-bar-group">
          <h2 className="ctv-bar-title">Guide</h2>
          <div className="ctv-chips" role="group" aria-label="Which channels" data-nav-group>
            <button className={'ctv-chip' + (!filter.group && !filter.favourites && !filter.kind ? ' on' : '')} aria-pressed={!filter.group && !filter.favourites && !filter.kind} onClick={() => setFilter({ kind: '', group: '', favourites: false })}>All</button>
            {(anyFav || filter.favourites) && <button className={'ctv-chip' + (filter.favourites ? ' on' : '')} aria-pressed={filter.favourites} onClick={() => setFilter({ kind: '', group: '', favourites: !filter.favourites })}><IconStar size={14} /> Favourites</button>}
            {groups.map((g) => <button key={g} className={'ctv-chip' + (filter.group === g ? ' on' : '')} aria-pressed={filter.group === g} onClick={() => setFilter({ kind: '', favourites: false, group: filter.group === g ? '' : g })}>{g}</button>)}
            {hasBoth && <button className={'ctv-chip' + (filter.kind === 'library' ? ' on' : '')} aria-pressed={filter.kind === 'library'} onClick={() => setFilter({ group: '', favourites: false, kind: filter.kind === 'library' ? '' : 'library' })}>My library</button>}
            {hasBoth && <button className={'ctv-chip' + (filter.kind === 'tuner' ? ' on' : '')} aria-pressed={filter.kind === 'tuner'} onClick={() => setFilter({ group: '', favourites: false, kind: filter.kind === 'tuner' ? '' : 'tuner' })}>Live TV</button>}
          </div>
        </div>
        <div className="ctv-bar-group ctv-bar-time" role="group" aria-label="Time" data-nav-group>
          <button className="ctv-btn sm icon" onClick={() => shiftTime(-SLOT)} aria-label="Half an hour earlier"><IconLeft size={16} /></button>
          <button className={'ctv-btn sm' + (floorSlot(now) === from ? ' on' : '')} onClick={goNow}>Now</button>
          <button className="ctv-btn sm icon" onClick={() => shiftTime(SLOT)} aria-label="Half an hour later"><IconRight size={16} /></button>
          {jumps(now).map((j) => <button key={j.label} className="ctv-link-btn" onClick={() => goAt(j.at)}>{j.label}</button>)}
          <label className="ctv-field"><span className="sr-only">Day</span>
            <select value={dayOffset} onChange={(e) => goDay(Number(e.target.value))}>
              {dayOffset < 0 && <option value={dayOffset}>Yesterday</option>}
              {Array.from({ length: DAY_NAMES }, (_, i) => <option key={i} value={i}>{i === 0 ? 'Today' : i === 1 ? 'Tomorrow' : fmtDay(today + i * 86400 + 3600)}</option>)}
            </select>
          </label>
          <button className="ctv-btn sm icon" onClick={openSearch} aria-haspopup="dialog" aria-label="Search the schedule" title="Search the schedule (S)"><IconSearch size={16} /></button>
          <button className="ctv-btn sm" onClick={() => setLookOpen(true)} aria-haspopup="dialog">Look</button>
          <Link className="ctv-btn sm hide-narrow" to="/tv/recordings">Recordings</Link>
          {user?.isAdmin && <Link className="ctv-btn sm" to="/settings/channels"><IconSettings size={16} /> <span className="hide-narrow">Edit channels</span><span className="sr-only">Edit channels</span></Link>}
        </div>
      </div>

      {err && <div className="ctv-err" role="alert">{err}</div>}
      {dial && <div className="ctv-dial" role="status">CH {dial}<span className="ctv-blink">_</span></div>}

      <div className="ctv-grid" ref={grid} role="grid" aria-label={`TV guide, ${fmtDay(from)}`} aria-rowcount={totalRows + 1} aria-colcount={slots + 1} onKeyDown={onKey}
        onFocus={() => { gridHadFocus.current = true }} onBlur={(e) => { if (e.relatedTarget) gridHadFocus.current = false }}
        style={{ '--ctv-row': rowH + 'px', '--ctv-gutter': gutter + 'px' } as React.CSSProperties}>
        <div className="ctv-times" role="row" aria-rowindex={1}>
          <div className="ctv-corner" role="columnheader" aria-colindex={1}><b>{dayWord(from, now)}</b> <span className="hide-narrow">{fmtClock(from)}</span></div>
          <div className="ctv-time-track">
            {timeHeads.map((t, i) => <div key={t} role="columnheader" aria-colindex={i + 2} className="ctv-time" style={{ left: pct(t) + '%', width: 100 / slots + '%' }}>{fmtClock(t)}</div>)}
          </div>
        </div>
        {look.nowLine && <NowLine from={from} to={to} />}
        <div className="ctv-rows" ref={scroller} onScroll={(e) => setScrollTop((e.target as HTMLElement).scrollTop)} role="presentation">
          <div style={{ height: totalRows * rowH, position: 'relative' }} role="presentation">
            {drawnRows.map((r) => (
              <GuideRow key={r} r={r} ch={rows[r]} top={r * rowH} rowH={rowH} from={from} to={to} pxPerSec={pxPerSec} now={now} focusAt={r === focus.row ? focus.at : null} look={look}
                onFocus={(row, at) => setFocus({ row, at })} onActivate={activate} wantFocus={wantFocus} />
            ))}
          </div>
        </div>
      </div>
      {total === 0 && <div className="ctv-err">{filter.favourites ? 'No favourites yet: press the star on a channel.' : 'No channels match that.'}</div>}

      <footer className="ctv-help" aria-label="Keys" data-nav-group>
        <button className="ctv-key a" onClick={() => shiftTime(-86400)}><b>A</b> Day −</button>
        <button className="ctv-key b" onClick={() => shiftTime(86400)}><b>B</b> Day +</button>
        <button className="ctv-key c" onClick={cycleKind}><b>C</b> {filter.kind === '' ? 'All' : filter.kind === 'library' ? 'Library' : 'Live TV'}</button>
        <span className="ctv-hint hide-narrow">▲▼ Channel · ◄► Time · OK Watch · 0–9 Dial · F Favourite · M Remind · D Day · S Search · L Last · I Info</span>
      </footer>
      <LegalNote kind="tv" className="ctv-legal" />
      {logosShown && <LegalNote kind="logos" className="ctv-legal" />}

      {details && <DetailsDialog ch={details.ch} p={details.p} now={now} startDay={details.day} look={look} onClose={() => setDetails(null)} onWatch={() => watch(details.ch.key)} onRecord={() => { void record(details.ch, details.p); setDetails(null) }} />}
      {lookOpen && <LookPanel look={look} onClose={() => setLookOpen(false)} />}
      {searchOpen && <SearchPanel onClose={closeSearch} onGo={(hit) => {
        setSearchOpen(false)
        const i = lineup.filter((x) => matches(x, filter)).findIndex((x) => x.key === hit.channel)
        if (i >= 0) goAt(hit.start, i)
        else { setFilter({ kind: '', group: '', favourites: false }); const k = lineup.findIndex((x) => x.key === hit.channel); setTimeout(() => goAt(hit.start, Math.max(0, k)), 0) }
      }} />}
    </>
  )
}

// The line at the current time. It has a clock of its own, so that moving it
// doesn't redraw anything else, and it moves only every few seconds.
function NowLine({ from, to }: { from: number; to: number }) {
  const now = useClock(5000)
  if (now < from || now >= to) return null
  const pct = ((now - from) / (to - from)) * 100
  return (
    <div className="ctv-nowline" aria-hidden="true" style={{ left: `calc(var(--ctv-lead) + var(--ctv-chw) + (100% - var(--ctv-lead) - var(--ctv-chw) - var(--ctv-trail) - var(--ctv-gutter)) * ${pct / 100})` }}>
      <span>{fmtClock(now)}</span>
    </div>
  )
}

const programmeLabel = (ch: GuideChannel, p: Programme, now: number) => {
  const when = p.kind === 'none' ? '' : fmtSpan(p)
  const on = p.start <= now && p.end > now ? ', on now' : ''
  const lock = ch.security?.state === 'encrypted' ? `, ${ch.security.label}` : ''
  return `Channel ${ch.number}, ${ch.name}: ${[p.title, p.sub, when].filter(Boolean).join(', ')}${on}${lock}`
}

type RowProps = {
  r: number; ch?: GuideChannel; top: number; rowH: number; from: number; to: number; pxPerSec: number; now: number; focusAt: number | null; look: Look
  onFocus: (row: number, at: number) => void; onActivate: (ch: GuideChannel, p: Programme) => void; wantFocus: React.RefObject<boolean>
}

// One channel's row. Only rows that changed draw again, which is what keeps
// moving around the guide quick on a cheap box.
const GuideRow = memo(function GuideRow({ r, ch, top, rowH, from, to, pxPerSec, now, focusAt, look, onFocus, onActivate, wantFocus }: RowProps) {
  const span = to - from
  const pct = (u: number) => ((u - from) / span) * 100
  const cells = cellsFor(ch, from, to, pxPerSec)
  const focusedCell = focusAt == null ? null : cells.find((c) => c.start <= focusAt && c.end > focusAt) || cells[0]
  const accent = ch?.accent ? channelVars(ch.accent) : undefined
  // With a highlighted tile the logo needs the version that shows up on it.
  const on = surfaceOf(look, focusAt != null)
  return (
    <div className={'ctv-row' + (focusAt != null ? ' current' : '')} role="row" aria-rowindex={r + 2} style={{ top, height: rowH, ...accent }}>
      {ch ? (
        <div className="ctv-ch" role="rowheader" aria-colindex={1} title={ch.name}>
          <span className="ctv-num">{ch.number}</span>
          <ChannelMark ch={ch} look={look} on={on} />
          {ch.favourite && <span className="ctv-fav" role="img" aria-label="Favourite"><IconStar size={13} /></span>}
          {ch.encrypted && <span className="ctv-lock" aria-hidden="true">🔒</span>}
        </div>
      ) : <div className="ctv-ch" role="rowheader" aria-colindex={1}><span className="ctv-name">…</span></div>}
      <div className="ctv-track">
        {ch && cells.map((p) => {
          const start = Math.max(p.start, from), end = Math.min(p.end, to)
          const focused = p === focusedCell
          const colStart = Math.floor((start - from) / SLOT)
          const colSpan = Math.max(1, Math.ceil((end - from) / SLOT) - colStart)
          const live = p.start <= now && p.end > now
          const wide = pxPerSec ? (end - start) * pxPerSec : 200
          const tiny = wide < 34
          const small = wide < 96
          const details = [p.kind === 'none' || small ? '' : fmtClock(p.start), p.year ? String(p.year) : '', p.genre || ''].filter(Boolean).join(' · ')
          return (
            <button key={p.start + '-' + p.end} type="button" role="gridcell" aria-colindex={colStart + 2} aria-colspan={colSpan}
              aria-label={programmeLabel(ch, p, now)} aria-selected={focused} tabIndex={focused ? 0 : -1} data-focused={focused ? '1' : undefined} data-nav-skip={focused ? undefined : ''}
              className={'ctv-cell' + (focused ? ' focus' : '') + (live ? ' live' : '') + (p.kind === 'none' || p.kind === 'slate' ? ' dim' : '') + (ch.encrypted ? ' locked' : '') + (p.end <= now ? ' past' : '') + (tiny ? ' tiny' : '') + (p.kind === 'group' ? ' group' : '')}
              style={{ left: pct(start) + '%', width: pct(end) - pct(start) + '%' }}
              onFocus={() => { if (!focused) onFocus(r, Math.max(p.start, from)) }}
              onClick={() => { if (focused) onActivate(ch, p); else { onFocus(r, Math.max(p.start, from)); wantFocus.current = true } }}
              onDoubleClick={() => onActivate(ch, p)}>
              {look.progress && live && p.kind !== 'none' && <i className="ctv-fill" aria-hidden="true" style={{ width: ((now - start) / Math.max(1, end - start)) * 100 + '%' }} />}
              {!tiny && p.start < from && <span className="ctv-more" aria-hidden="true">◄</span>}
              {tiny ? (p.kind === 'group' ? <span className="ctv-count" aria-hidden="true">{p.group?.length}</span> : null) : (
                <span className="ctv-cell-text">
                  <b>{p.title}</b>
                  {look.subtitles && look.layout === 'streaming' && details && <small>{details}</small>}
                  {look.subtitles && look.layout !== 'streaming' && p.sub && <small>{p.sub}</small>}
                </span>
              )}
              {!tiny && p.end > to && <span className="ctv-more" aria-hidden="true">►</span>}
            </button>
          )
        })}
      </div>
    </div>
  )
})
