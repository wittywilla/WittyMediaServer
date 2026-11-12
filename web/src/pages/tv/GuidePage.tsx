// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { EmptyState } from '../../components/common'
import { memo, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../../api'
import { announce } from '../../a11y'
import { IconLeft, IconRight, IconSearch, IconSettings, IconStar, IconTv } from '../../icons'
import { toast, useApp } from '../../state'
import { featureOff, useMyLimits } from '../../myLimits'
import { lookupLogo } from '../../components/BrandLogo'
import { useFeatures } from '../../sdui/useSdui'
import LookPanel from './LookPanel'
import { channelVars, forgetJustWatched, justWatched, lastChannel, previousChannel, setLook, setOwnerLook, surfaceOf, themeRoot, useLook, type Look } from './look'
import { previewAuto, previewLimit, rowHeight, SHAPES } from './tvThemes'
import { ChannelMark } from './ChannelLogo'
import DetailsDialog from './Details'
import MoreInfo from './MoreInfo'
import Hero from './Hero'
import SearchPanel from './SearchPanel'
import SetPicker, { SetButton } from './SetPicker'
import AddToSet from './AddToSet'
import RecordOptions from './RecordOptions'
import { useReminders } from './reminders'
import { Dock, type DockAction } from '../../ui/Dock'
import type { MenuEntry } from '../../ui/Menu'
import { liveNow, noteBackgroundDefault } from './live'
import { useGuidePrefs } from './guidePrefs'
import { ALL_SET, catchupKey, channelOf, chooseSet, dialOutcome, edgeLanding, pageLabel, pageStartOf, PAGE_SIZES, rowsOnPage, stepPage, stepSet, widerLookup, windowStepSlots, type PageSize } from './guideLogic'
import { canOfferRecord, dayWord, fmtClock, fmtDay, fmtSpan, getLineup, getSets, forgetLineup, setFavourite, type GuideChannel, type GuidePage as Page, type Programme, type SetList } from './tvApi'
import './tv.css'
import './tvthemes.css'
import './tvsets.css'
import LegalNote from '../../components/LegalNote'

// The programme guide. A real ARIA grid under any of WittyTV's themes (tvThemes.ts). At the
// top, the highlighted channel: its programme, what's next, what can be done with it and a
// live preview. Below, the rows. The look is each viewer's own (see look.ts). Only the rows
// on screen are loaded and drawn, so a playlist of thousands of channels stays quick.
//
// Which channels it shows is a set (a server with tv-sets: all, favourites, the viewer's own
// and shared ones, each source, each group), chosen and remembered on this device; against an
// older server, the filters it always had. The rows come a page at a time (25 to 100, with
// Previous and Next), or all of them scrolled through, as this device chose.

const SLOT = 1800
// Rows asked for at a time while scrolling through all of them.
const PAGE = 30
const FETCH_SPAN = 3 * 3600
const FETCH_HOURS = 8
const floorSlot = (u: number) => Math.floor(u / SLOT) * SLOT
const DAY_NAMES = 15
// A cell narrower than this on screen can't show anything: several in a row are
// drawn as one, which says how many.
const TINY_PX = 14

type Focus = { row: number; at: number }

// The cells of a row for the window on screen; gaps are cells too, so every
// moment of every channel can be reached with the keyboard.
function cellsFor(ch: GuideChannel | undefined, from: number, to: number, pxPerSec: number): Programme[] {
  if (!ch) return []
  const out: Programme[] = []
  let t = from
  const none = (start: number, end: number): Programme => ({ start, end, kind: 'none', title: ch.kind === 'library' ? 'Off air' : 'No guide data' })
  for (const p of ch.programmes || []) {
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

// "Tomorrow 8 PM" and "Tomorrow morning": times people actually jump to.
function jumps(now: number): { label: string; at: number }[] {
  const at = (days: number, hour: number) => { const d = new Date(now * 1000); d.setDate(d.getDate() + days); d.setHours(hour, 0, 0, 0); return d.getTime() / 1000 }
  const tonight = at(0, 20)
  return [tonight > now + 1800 ? { label: 'Tonight 8 PM', at: tonight } : { label: 'Tomorrow 8 PM', at: at(1, 20) }, { label: 'Tomorrow morning', at: at(1, 8) }]
}


export default function GuidePage() {
  const { user } = useApp()
  const tvOff = featureOff(useMyLimits(), 'liveTv')
  const nav = useNavigate()
  const now = useClock(30000)
  const features = useFeatures()
  const hasCatchup = !!features?.has('tv-catchup')
  const hasRules2 = !!features?.has('tv-dvr-rules-2')
  const [prefs, setPrefs] = useGuidePrefs()
  const pageSize: PageSize = prefs.pageSize
  const [from, setFrom] = useState(() => floorSlot(Date.now() / 1000))
  const [sets, setSets] = useState<SetList | null>(null)
  const [setsFailed, setSetsFailed] = useState(false)
  // Finding a channel: what's typed, and what the rows are asked for, a moment later.
  const [find, setFind] = useState('')
  const [query, setQuery] = useState('')
  // The rows by their place in the whole list. Kept outside React's state, so that a page more
  // of thousands of channels doesn't copy the ones already here; `rowsVer` draws them again.
  const rowsRef = useRef(new Map<number, GuideChannel>())
  const [, setRowsVer] = useState(0)
  const bumpRows = () => setRowsVer((v) => v + 1)
  const [total, setTotal] = useState<number | null>(null)
  const look = useLook()
  const [lookOpen, setLookOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [addTo, setAddTo] = useState<GuideChannel | null>(null)
  const [recordFor, setRecordFor] = useState<{ ch: GuideChannel; p: Programme; series: boolean } | null>(null)
  const [err, setErr] = useState('')
  const [focus, setFocus] = useState<Focus>(() => ({ row: 0, at: Date.now() / 1000 }))
  // Back from a channel, by any way out of it: that channel is highlighted once the rows come.
  const [back, setBack] = useState(justWatched)
  // A row to go to once the rows of what's shown have come (after changing what's shown).
  const [goRow, setGoRow] = useState<{ row: number; at?: number; centre?: boolean } | null>(null)
  const [moreInfo, setMoreInfo] = useState<{ key: string; at: number } | null>(null)
  const [details, setDetails] = useState<{ ch: GuideChannel; p: Programme; day?: boolean } | null>(null)
  const [dial, setDial] = useState('')
  const [width, setWidth] = useState(() => window.innerWidth)
  const [pageSizeBox, setPageSizeBox] = useState({ w: window.innerWidth, h: window.innerHeight })
  const [scrollTop, setScrollTop] = useState(0)
  const [viewH, setViewH] = useState(400)
  const [gutter, setGutter] = useState(0)
  const [trackW, setTrackW] = useState(0)
  const [lineup, setLineup] = useState<GuideChannel[]>([])
  const [tick, setTick] = useState(0)
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
  const rowH = rowHeight(look.shape, width, look.density)
  const to = from + slots * SLOT
  const fetchFrom = Math.floor(from / FETCH_SPAN) * FETCH_SPAN
  const pxPerSec = trackW ? trackW / (to - from) : 0
  const reminders = useReminders()

  // Which set: the one this device chose while the server has it, else all channels.
  const setId = sets ? chooseSet(prefs.set, sets.sets) : ALL_SET
  const curSet = sets?.sets.find((s) => s.id === setId)
  const q = query.trim()
  const viewQuery = `&set=${encodeURIComponent(setId)}` + (q ? `&q=${encodeURIComponent(q)}` : '')
  // Until the server has said what it can do, and which sets there are, nothing is asked for.
  const ready = features !== null && (sets !== null || setsFailed)
  const filterKey = ready ? viewQuery : ''
  const filterKeyRef = useRef(filterKey)
  filterKeyRef.current = filterKey
  const noFilter = !q && setId === ALL_SET

  const loadSets = useCallback(() => {
    getSets().then((s) => { setSets(s); setSetsFailed(false) }).catch((e) => { setSetsFailed(true); setErr((e as Error).message) })
  }, [])
  useEffect(loadSets, [loadSets])
  const pickSet = (id: string) => {
    setPrefs({ set: id })
    const s = sets?.sets.find((x) => x.id === id)
    if (s) announce(`${s.name}, ${s.count} ${s.count === 1 ? 'channel' : 'channels'}`)
  }

  // What's typed in Find a channel is asked for once the typing stops.
  useEffect(() => {
    const t = setTimeout(() => setQuery(find), 300)
    return () => clearTimeout(t)
  }, [find])

  // A changed set, filter or search starts the rows afresh, there and then, so nothing drawn
  // or worked out in between belongs to what was shown before.
  const [viewKey, setViewKey] = useState(filterKey)
  if (viewKey !== filterKey) {
    setViewKey(filterKey)
    setTotal(null)
    setFocus((f) => ({ ...f, row: 0 }))
    rowsRef.current = new Map()
    loaded.current = new Set()
  }
  useLayoutEffect(() => { if (scroller.current) scroller.current.scrollTop = 0; setScrollTop(0) }, [filterKey])
  // A new stretch of hours keeps the rows until the new ones come, so the highlight has somewhere to be meanwhile.
  useEffect(() => { loaded.current.clear() }, [fetchFrom])

  useEffect(() => {
    const onResize = () => setWidth(window.innerWidth)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  // The page's own size, which everything about the preview is held to.
  const pageEl = page.current
  useEffect(() => {
    if (!pageEl) return
    const ro = new ResizeObserver(() => setPageSizeBox({ w: pageEl.clientWidth, h: pageEl.clientHeight }))
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

  const refreshLineup = () => getLineup().then(setLineup).catch(() => {})
  useEffect(() => { void refreshLineup() }, [])
  // The rights notice goes with the logos: it's there only when one is on the screen.
  const [logosShown, setLogosShown] = useState(false)
  useEffect(() => {
    if (!look.logos) { setLogosShown(false); return }
    if (lineup.some((c) => c.logo)) { setLogosShown(true); return }
    const brands = lineup.flatMap((c) => (c.brand && !c.logo ? [c.brand] : []))
    if (!brands.length) { setLogosShown(false); return }
    let on = true
    Promise.all(brands.slice(0, 200).map((b) => lookupLogo(b.kind, b.name))).then((l) => { if (on) setLogosShown(l.some((x) => x.has)) })
    return () => { on = false }
  }, [lineup, look.logos])

  // Which page is shown follows the highlight: moving past its last row is the next page.
  const pageStart = pageStartOf(focus.row, pageSize)
  const pageRows = pageSize ? rowsOnPage(pageStart, pageSize, total ?? 0) : total ?? 0
  const firstRow = pageStart + Math.max(0, Math.floor(scrollTop / rowH) - 3)
  const lastRow = total == null ? pageStart + (pageSize || PAGE) - 1 : Math.min(pageStart + pageRows - 1, pageStart + Math.ceil((scrollTop + viewH) / rowH) + 3)
  const loadRows = useCallback(async (offset: number, limit: number) => {
    const fk = filterKeyRef.current
    const key = `${fetchFrom}|${fk}|${offset}|${limit}`
    if (loaded.current.has(key)) return
    const mark = loaded.current
    mark.add(key)
    try {
      const g = await api.get<Page>(`/guide?from=${fetchFrom}&hours=${FETCH_HOURS}&offset=${offset}&limit=${limit}${fk}`)
      // An answer about what was shown before is no use now.
      if (filterKeyRef.current !== fk || loaded.current !== mark) return
      setTotal(g.total)
      setOwnerLook({ theme: g.theme })
      noteBackgroundDefault(g.backgroundPlay)
      const m = rowsRef.current
      ;(g.channels || []).forEach((c, i) => { m.set(g.offset + i, c) })
      bumpRows()
      setErr('')
    } catch (e) { mark.delete(key); if (filterKeyRef.current === fk) setErr((e as Error).message) }
  }, [fetchFrom])
  useEffect(() => {
    if (!ready) return
    if (pageSize) { void loadRows(pageStart, pageSize); return }
    for (let p = Math.floor(firstRow / PAGE); p <= Math.floor(lastRow / PAGE); p++) void loadRows(p * PAGE, PAGE)
  }, [ready, filterKey, pageSize, pageStart, firstRow, lastRow, loadRows, tick])
  // Keep the guide current while it is left open.
  useEffect(() => {
    const t = setInterval(() => { loaded.current.clear(); setTick((n) => n + 1) }, 5 * 60000)
    return () => clearInterval(t)
  }, [])
  // Asks for what's shown again; `afresh` when rows may have come or gone (a favourite less).
  const refreshView = (afresh = false) => {
    if (afresh) { rowsRef.current = new Map(); bumpRows() }
    loaded.current = new Set()
    setTick((n) => n + 1)
  }

  // Where a channel (its key or number) is among the rows shown: -1 when it isn't there. The server finds it in the set.
  const locate = useCallback(async (what: string, wide = false): Promise<number> => {
    const scope = wide ? `&set=${ALL_SET}` : filterKeyRef.current
    const g = await api.get<Page>(`/guide?list=1&lite=1&limit=1${scope}&locate=${encodeURIComponent(what)}`)
    return g.located ?? -1
  }, [])
  // Everything shown again, at that row of all channels.
  const showAllAt = (row: number, at?: number) => {
    setPrefs({ set: ALL_SET })
    setFind(''); setQuery('')
    setGoRow({ row, at, centre: true })
  }

  // keepFocus: the keyboard stays where it is (on Previous or Next), rather than going to the row.
  const moveRow = useCallback((row: number, keepFocus = false) => {
    setTotal((tot) => {
      const r = Math.max(0, Math.min((tot ?? 1) - 1, row))
      wantFocus.current = !keepFocus
      setFocus((f) => ({ ...f, row: r }))
      const el = scroller.current
      if (el) {
        const top = (r - pageStartOf(r, pageSize)) * rowH
        if (top < el.scrollTop) el.scrollTop = top
        else if (top + rowH > el.scrollTop + el.clientHeight) el.scrollTop = top + rowH - el.clientHeight
      }
      return tot
    })
  }, [rowH, pageSize])
  // A new page: the highlighted row on screen, once the page is drawn at its own height.
  useLayoutEffect(() => {
    const el = scroller.current
    if (!el || !pageSize) return
    const top = (focus.row - pageStart) * rowH
    if (top < el.scrollTop) el.scrollTop = top
    else if (top + rowH > el.scrollTop + el.clientHeight) el.scrollTop = top + rowH - el.clientHeight
    setScrollTop(el.scrollTop)
  }, [pageStart, pageSize]) // eslint-disable-line react-hooks/exhaustive-deps

  // A row asked for before the rows came: there, in the middle, once they have.
  useEffect(() => {
    if (!goRow || total == null) return
    setGoRow(null)
    const r = Math.max(0, Math.min(total - 1, goRow.row))
    const el = scroller.current
    if (el && goRow.centre) el.scrollTop = Math.max(0, (r - pageStartOf(r, pageSize)) * rowH - (el.clientHeight - rowH) / 2)
    wantFocus.current = true
    if (goRow.at != null) setFrom(floorSlot(goRow.at))
    setFocus((f) => ({ row: r, at: goRow.at ?? f.at }))
    if (!goRow.centre) moveRow(r)
  }, [goRow, total, rowH, pageSize, moveRow])

  // The channel just left, in the middle of the rows, at now, with the keyboard on it.
  // It's found in what is shown now (a set, a search), and only an answer about that is used:
  // clearing `back` itself mustn't throw the answer away.
  useEffect(() => {
    if (!back || total == null) return
    setBack('')
    forgetJustWatched()
    const fk = filterKeyRef.current
    locate(back).then((i) => { if (filterKeyRef.current === fk && i >= 0) setGoRow({ row: i, at: Date.now() / 1000, centre: true }) }).catch(() => {})
  }, [back, total, locate])

  const cur = rowsRef.current.get(focus.row)
  const cells = useMemo(() => cellsFor(cur, from, to, pxPerSec), [cur, from, to, pxPerSec])
  const curCell = cells.find((c) => c.start <= focus.at && c.end > focus.at) || cells[0]
  // What the top of the page is about: the highlighted programme, and what follows it.
  const panelCell = details ? (details.p.kind === 'group' ? curCell : details.p) : curCell
  // A run of very short programmes is one cell, but the top of the page says which one.
  const panel = panelCell?.group ? panelCell.group.find((c) => c.start <= focus.at && c.end > focus.at) || panelCell.group[0] : panelCell
  const nextShow = useMemo(() => {
    if (!cur || !panel) return undefined
    return (cur.programmes || []).find((p) => p.start >= panel.end && p.kind !== 'slate' && p.kind !== 'none')
  }, [cur, panel])

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
  const goPage = (dir: 1 | -1, keepFocus = false) => { if (total) moveRow(stepPage(focus.row, pageSize, total, dir), keepFocus) }

  const watch = (key: string) => nav(`/tv/watch/${key}`, { state: { fromGuide: true } })
  const watchFromStart = (ch: GuideChannel, p: Programme) => nav(`/tv/watch/${ch.key}?from=${Math.floor(p.start)}&to=${Math.floor(p.end)}`, { state: { fromGuide: true, catchup: catchupKey(ch.key, p.start, p.end) } })
  const activate = (ch: GuideChannel, p: Programme) => {
    if (ch.encrypted || p.kind === 'group') { setDetails({ ch, p }); return }
    const live = p.kind === 'none' ? p.start <= now + 60 : p.start <= now && p.end > now
    if (live) watch(ch.key)
    else setDetails({ ch, p })
  }

  // Dialling: the number among the rows shown, or among all channels with the offer to show them.
  const dialTo = async (n: string) => {
    try {
      const here = await locate(n)
      const wider = widerLookup(setId, q, here)
      const all = wider ? await locate(n, true) : null
      const out = dialOutcome(here, all)
      if (out.kind === 'here') { moveRow(out.row); announce(`Channel ${n}`) }
      else if (out.kind === 'elsewhere') {
        const where = curSet?.name || 'this set'
        announce(`Channel ${n} isn't in ${where}`)
        toast(`Channel ${n} isn't in ${where}.`, { action: { label: 'Show all channels', run: () => showAllAt(out.row) } })
      } else { announce(`No channel ${n}`); toast(`No channel ${n}`) }
    } catch (e) { toast((e as Error).message) }
  }
  const dialTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const pressDigit = (d: string) => {
    const next = (dial + d).slice(0, 8)
    setDial(next)
    clearTimeout(dialTimer.current)
    dialTimer.current = setTimeout(() => { setDial(''); void dialTo(next) }, 1200)
  }
  useEffect(() => () => clearTimeout(dialTimer.current), [])

  const record = async (ch: GuideChannel, p: Programme) => {
    if (!canOfferRecord(ch, p, now)) return
    try {
      await api.post('/recordings', { channel: ch.key, title: p.title, sub: p.sub, start: p.start, stop: p.end })
      toast(`Recording ${p.title}`)
      announce(`Recording ${p.title}`)
    } catch (e) { toast((e as Error).message) }
  }
  const toggleFav = async (ch: GuideChannel) => {
    const on = !ch.favourite
    const m = rowsRef.current
    for (const [i, c] of m) if (c.key === ch.key) m.set(i, { ...c, favourite: on })
    bumpRows()
    announce(on ? `${ch.name} added to favourites` : `${ch.name} removed from favourites`)
    try {
      await setFavourite(ch.key, on)
      forgetLineup(); void refreshLineup()
      loadSets(); if (setId === 'fav') refreshView(true)
    } catch (e) { toast((e as Error).message) }
  }
  const jumpToLast = async () => {
    const key = cur && lastChannel() === cur.key ? previousChannel() : lastChannel()
    const c = lineup.find((x) => x.key === key)
    if (!c) { toast('No other channel watched yet'); return }
    const i = await locate(key).catch(() => -1)
    if (i >= 0) { moveRow(i); announce(`Channel ${c.number}, ${c.name}`) } else watch(key)
  }
  const nextSet = (dir: 1 | -1) => { if (sets) pickSet(stepSet(sets.sets, setId, dir)) }
  const cycle = () => nextSet(1)
  const openPicker = () => { if (sets) setPickerOpen(true) }
  const offerRecord = (ch: GuideChannel, p: Programme, series: boolean) => {
    if (hasRules2) setRecordFor({ ch, p, series })
    else if (!series) void record(ch, p)
  }

  const onKey = (e: React.KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const row = focus.row
    const idx = cells.findIndex((c) => c === curCell)
    const rowsPerScreen = Math.max(1, Math.floor(viewH / rowH) - 1)
    let handled = true
    switch (e.key) {
      case 'ArrowUp':
        // The top row hands the arrows back, so a remote can reach the buttons above.
        if (row === 0) return
        moveRow(row - 1); break
      case 'ArrowDown':
        // The same at the bottom, for what is under the guide.
        if (total != null && row >= total - 1) return
        // The last row of a page hands Down on, to the Dock under the grid (the page itself turns with Page Up and
        // Down, the shoulder triggers, Channel Up and Down, or the Dock's Channels).
        if (pageSize && row === pageStart + pageRows - 1) return
        moveRow(row + 1); break
      case 'PageUp': case 'ChannelUp': moveRow(pageSize && total ? stepPage(row, pageSize, total, -1) : row - rowsPerScreen); break
      case 'PageDown': case 'ChannelDown': moveRow(pageSize && total ? stepPage(row, pageSize, total, 1) : row + rowsPerScreen); break
      case 'Home': goNow(); break
      case 'ArrowRight': {
        const next = cells[idx + 1]
        if (next && next.start < to) { wantFocus.current = true; setFocus({ row, at: next.start }) }
        else shiftTime(windowStepSlots(slots) * SLOT, edgeLanding(1, from, to, curCell?.end))
        break
      }
      case 'ArrowLeft': {
        const prev = cells[idx - 1]
        if (prev) { wantFocus.current = true; setFocus({ row, at: Math.max(prev.start, from) }) }
        else if (from <= floorSlot(now) - 2 * 3600) return
        else shiftTime(-windowStepSlots(slots) * SLOT, edgeLanding(-1, from, to))
        break
      }
      case 'Enter': if (cur && curCell) activate(cur, curCell); break
      case 'i': case 'I': if (cur && curCell) setMoreInfo({ key: cur.key, at: Math.max(curCell.start, curCell.end > now && curCell.start <= now ? now : curCell.start) }); break
      case 'r': case 'R': if (cur && curCell && canOfferRecord(cur, curCell, now)) void record(cur, curCell); break
      case 'f': case 'F': if (cur) void toggleFav(cur); break
      case 'd': case 'D': if (cur && curCell) setDetails({ ch: cur, p: curCell, day: true }); break
      case 'm': case 'M': if (cur && curCell && curCell.start > now) void reminders.toggle(cur.key, curCell); break
      case 'l': case 'L': void jumpToLast(); break
      case 'v': case 'V': setLookOpen(true); break
      case 's': case 'S': case '/': openSearch(); break
      case 'n': case 'N': goNow(); break
      case '+': case '=': setLook({ hours: Math.max(1.5, (look.hours || slots / 2) - 0.5) as Look['hours'] }); break
      case '-': setLook({ hours: Math.min(4, (look.hours || slots / 2) + 0.5) as Look['hours'] }); break
      case 'a': case 'A': case 'ColorF0Red': shiftTime(-86400); break
      case 'b': case 'B': case 'ColorF1Green': shiftTime(86400); break
      case 'c': case 'C': case 'ColorF2Yellow': cycle(); break
      case 'ColorF3Blue': openPicker(); break
      case '[': nextSet(-1); break
      case ']': nextSet(1); break
      case 't': case 'T': if (cur) setAddTo(cur); else handled = false; break
      default:
        if (/^[0-9.]$/.test(e.key)) pressDigit(e.key)
        else handled = false
    }
    if (handled) { e.preventDefault(); e.stopPropagation() }
  }

  // Dialling and Channel Up and Down work wherever the keyboard is on the guide's page, not only in
  // the grid: on a button above it, or on nothing at all just after coming back from a channel.
  // Typing in a field, and a dialog that's open, keep their keys; the grid has handled its own.
  useEffect(() => {
    const onWindowKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey) return
      const t = e.target as HTMLElement | null
      if (t?.closest?.('input, textarea, select, [contenteditable="true"], .modal, .ctv-grid')) return
      if (document.querySelector('.modal-backdrop')) return
      if (t && t !== document.body && !page.current?.contains(t)) return
      const row = focus.row
      if (/^[0-9]$/.test(e.key)) pressDigit(e.key)
      else if (e.key === 'ChannelUp') moveRow(pageSize && total ? stepPage(row, pageSize, total, -1) : row - 1)
      else if (e.key === 'ChannelDown') moveRow(pageSize && total ? stepPage(row, pageSize, total, 1) : row + 1)
      else return
      e.preventDefault()
    }
    window.addEventListener('keydown', onWindowKey)
    return () => window.removeEventListener('keydown', onWindowKey)
  })

  // New data can replace the cell that has the keyboard. The browser then drops it on
  // the page, where the next arrow would start from nowhere: it goes back to the cell.
  useLayoutEffect(() => {
    if (gridHadFocus.current && document.activeElement === document.body) wantFocus.current = true
  })

  // Roving focus: the highlighted cell holds the keyboard after each move, before the
  // next key can arrive.
  useLayoutEffect(() => {
    if (!wantFocus.current) return
    // TV mode's tabs keep the focus when they open the guide: the arrows come down into it from there.
    if (document.activeElement?.closest('[data-keep-focus]')) { wantFocus.current = false; return }
    const el = grid.current?.querySelector<HTMLElement>('[data-focused="1"]')
    if (el) { wantFocus.current = false; el.focus({ preventScroll: true }) }
  })

  // The rows near what's on screen, and always the highlighted one: it's where the
  // arrows come back to, however far away the rows have been scrolled.
  const rowCount = total ?? 0
  const drawnRows: number[] = []
  for (let r = firstRow; r <= Math.min(lastRow, rowCount - 1); r++) drawnRows.push(r)
  if (focus.row < rowCount && !drawnRows.includes(focus.row)) drawnRows.push(focus.row)
  const timeHeads = useMemo(() => Array.from({ length: slots }, (_, i) => from + i * SLOT), [from, slots])
  const pct = (u: number) => ((u - from) / (to - from)) * 100
  const totalRows = total ?? 0
  // What the little TV shows: settled on by the preview itself. Until the rows come, it keeps
  // what was already playing, or the channel just left, so coming back doesn't stop it.
  const previewKey = look.preview === 'off' ? '' : look.preview === 'last' ? (lastChannel() || (cur && !cur.encrypted ? cur.key : '')) : back || (cur ? (cur.encrypted ? '' : cur.key) : channelOf(liveNow().key))
  const previewName = useMemo(() => {
    if (!previewKey) return ''
    const c = cur?.key === previewKey ? cur : lineup.find((x) => x.key === previewKey)
    return c ? `${c.number} ${c.name}` : ''
  }, [previewKey, cur, lineup])
  // The most the preview may take, and what it starts at until someone chooses a size.
  const narrow = pageSizeBox.w < 760
  const previewMax = previewLimit(pageSizeBox.w, pageSizeBox.h, narrow)
  const previewStart = previewAuto(look.shape, pageSizeBox.w, pageSizeBox.h, narrow)

  const shell = (children: React.ReactNode) => (
    <div ref={page} className={'ctv ctv-page' + (narrow ? ' narrow' : '')} {...themeRoot(look)}>{children}</div>
  )
  if (total === 0 && noFilter) {
    // Nothing to show yet, so the guide's own colours have nothing to dress up.
    return (
      <>
        <h1 className="sr-only">TV guide</h1>
        <EmptyState icon={<IconTv />} title={tvOff ? 'Live TV is off for this profile' : 'No channels yet'}
          text={user?.isAdmin ? 'Make channels out of your library in one click, or add a tuner or an IPTV playlist.' : tvOff ? 'Whoever looks after this profile can turn it on.' : 'Ask whoever runs this server to set some up.'}
          action={user?.isAdmin ? <Link className="btn primary" to="/tv/channels">Set up channels</Link> : undefined} />
      </>
    )
  }

  const dayStart = (u: number) => { const d = new Date(u * 1000); d.setHours(0, 0, 0, 0); return d.getTime() / 1000 }
  const today = dayStart(now)
  const dayOffset = Math.round((dayStart(from) - today) / 86400)
  const goDay = (n: number) => shiftTime(n === 0 ? floorSlot(now) - from : (n - dayOffset) * 86400)
  const emptyWords = q ? 'No channel has that number or name here.'
    : setId === 'fav' ? 'No favourites yet: press the star on a channel.' : 'There are no channels in this set that you can watch.'
  const lastPage = total != null && pageSize ? pageStart + pageSize >= total : true
  // The Dock under the grid: six buttons that never move, whatever the look (docs/plans/DESIGN-2.md section 4).
  const hasFav = !!sets?.sets.some((x) => x.id === 'fav')
  const filterItems: MenuEntry[] = [
    { key: 'find', label: 'Find a channel', onSelect: () => document.querySelector<HTMLInputElement>('.ctv-find input')?.focus() },
    { key: 'all', label: 'All channels', current: setId === ALL_SET, onSelect: () => pickSet(ALL_SET) },
    ...(hasFav ? [{ key: 'fav', label: 'Favourites', current: setId === 'fav', onSelect: () => pickSet('fav') }] : []),
    { kind: 'separator', key: 'sep' },
    { key: 'togglefav', label: cur?.favourite ? 'Remove this channel from favourites' : 'Add this channel to favourites', disabled: !cur, onSelect: () => { if (cur) void toggleFav(cur) } },
    { key: 'addset', label: 'Add this channel to a set…', disabled: !cur, onSelect: () => { if (cur) setAddTo(cur) } },
  ]
  const channelItems: MenuEntry[] = [
    { key: 'sets', label: 'Choose channels…', hint: 'C', onSelect: openPicker },
    { kind: 'separator', key: 'sep' },
    { kind: 'heading', key: 'head', label: 'Channels per page' },
    ...PAGE_SIZES.map((n) => ({ key: 'n' + n, label: n ? `${n} per page` : 'All, scrolling', checked: pageSize === n, onSelect: () => setPrefs({ pageSize: n }) })),
    ...(pageSize ? [
      { kind: 'separator', key: 'sep2' } as MenuEntry,
      { key: 'prev', label: 'Previous page', hint: 'Page Up', disabled: pageStart === 0, onSelect: () => goPage(-1) },
      { key: 'next', label: 'Next page', hint: 'Page Down', disabled: lastPage, onSelect: () => goPage(1) },
    ] : []),
  ]
  const dayItems: MenuEntry[] = [
    { key: 'now', label: 'Now', hint: 'N', current: floorSlot(now) === from, onSelect: goNow },
    ...jumps(now).map((j) => ({ key: j.label, label: j.label, onSelect: () => goAt(j.at) })),
    { kind: 'separator', key: 'sep' },
    ...Array.from({ length: DAY_NAMES }, (_, i) => ({ key: 'd' + i, label: i === 0 ? 'Today' : i === 1 ? 'Tomorrow' : fmtDay(today + i * 86400 + 3600), current: dayOffset === i, onSelect: () => goDay(i) })),
  ]
  const canRecord = !!cur && !!panel && canOfferRecord(cur, panel, now)
  const dockActions: DockAction[] = [
    { id: 'look', label: 'Look', hint: 'V', onClick: () => setLookOpen(true) },
    { id: 'day', label: 'Day', hint: 'A B', items: dayItems },
    { id: 'channels', label: 'Channels', hint: 'C', items: channelItems, detail: curSet?.name },
    { id: 'filter', label: 'Filter', items: filterItems },
    { id: 'info', label: 'More info', hint: 'I', disabled: !cur || !curCell, onClick: () => { if (cur && curCell) setMoreInfo({ key: cur.key, at: curCell.start }) } },
    { id: 'recordings', label: 'Recordings', to: '/tv/recordings' },
    hasRules2
      ? { id: 'record', label: 'Record', hint: 'R', disabled: !canRecord, items: [
        { key: 'one', label: 'Record this programme', onSelect: () => { if (cur && panel) offerRecord(cur, panel, false) } },
        { key: 'series', label: 'Record the series', onSelect: () => { if (cur && panel) offerRecord(cur, panel, true) } },
      ] }
      : { id: 'record', label: 'Record', hint: 'R', disabled: !canRecord, onClick: () => { if (cur && panel) void record(cur, panel) } },
  ]
  return shell(
    <>
      <h1 className="sr-only">TV guide</h1>
      <Hero ch={cur} p={panel} next={nextShow} look={look} now={now} previewKey={previewKey} previewName={previewName} previewMax={previewMax} previewStart={previewStart}
        onWatch={watch} onDetails={() => { if (cur && curCell) setDetails({ ch: cur, p: curCell }) }} onDay={() => { if (cur && curCell) setDetails({ ch: cur, p: curCell, day: true }) }}
        onFav={() => { if (cur) void toggleFav(cur) }} onUp={() => moveRow(focus.row + 1)} onDown={() => moveRow(focus.row - 1)} onRecord={() => { if (cur && panel) void record(cur, panel) }}
        onFromStart={hasCatchup ? (ch, p) => watchFromStart(ch, p) : undefined} />

      <div className="ctv-bar">
        <div className="ctv-bar-group">
          <h2 className="ctv-bar-title">Guide</h2>
          {sets && <SetButton set={curSet} onOpen={openPicker} />}
          <label className="ctv-field ctv-find"><span className="sr-only">Find a channel</span>
              <input type="search" value={find} placeholder="Find a channel" aria-label="Find a channel, by number or name" onChange={(e) => setFind(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') { e.preventDefault(); setQuery(find); wantFocus.current = true; setFocus((f) => ({ ...f, row: 0 })) }
                  if (e.key === 'Escape' && find) { e.preventDefault(); e.stopPropagation(); setFind(''); setQuery('') }
                }} />
          </label>
        </div>
        <div className="ctv-bar-group ctv-bar-time" role="group" aria-label="Time" data-nav-group>
          <button className="ctv-btn sm icon" onClick={() => shiftTime(-SLOT)} aria-label="Half an hour earlier"><IconLeft size={16} /></button>
          <button className={'ctv-btn sm' + (floorSlot(now) === from ? ' on' : '')} onClick={goNow}>Now</button>
          <button className="ctv-btn sm icon" onClick={() => shiftTime(SLOT)} aria-label="Half an hour later"><IconRight size={16} /></button>
          <button className="ctv-btn sm icon" onClick={openSearch} aria-haspopup="dialog" aria-label="Search the schedule" title="Search the schedule (S)"><IconSearch size={16} /></button>
          {user?.isAdmin && <Link className="ctv-btn sm" to="/tv/channels" aria-label="Edit channels"><IconSettings size={16} /> <span className="hide-narrow">Edit channels</span></Link>}
        </div>
      </div>

      {err && <div className="ctv-err" role="alert">{err}</div>}
      {dial && <div className="ctv-dial" role="status">CH {dial}<span className="ctv-blink">_</span></div>}

      <div className="ctv-grid" ref={grid} role="grid" aria-label={`TV guide, ${fmtDay(from)}${curSet ? `, ${curSet.name}` : ''}`} aria-rowcount={totalRows + 1} aria-colcount={slots + 1} onKeyDown={onKey}
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
          <div style={{ height: pageRows * rowH, position: 'relative' }} role="presentation">
            {drawnRows.map((r) => (
              <GuideRow key={r} r={r} ch={rowsRef.current.get(r)} top={(r - pageStart) * rowH} rowH={rowH} from={from} to={to} pxPerSec={pxPerSec} now={now} focusAt={r === focus.row ? focus.at : null} look={look}
                onFocus={(row, at) => setFocus({ row, at })} onActivate={activate} wantFocus={wantFocus} />
            ))}
          </div>
        </div>
      </div>
      {total === 0 && <div className="ctv-err">{emptyWords}</div>}
      {pageSize > 0 && total != null && total > 0 && (
        <div className="ctv-pager" role="group" aria-label="Pages of channels" data-nav-group>
          <button className="ctv-btn sm" data-nav-skip disabled={pageStart === 0} onClick={() => goPage(-1, true)} title="Previous page (Page Up)"><IconLeft size={16} /> Previous</button>
          <span className="ctv-pager-at" role="status">{pageLabel(pageStart, pageSize, total)}</span>
          <button className="ctv-btn sm" data-nav-skip disabled={lastPage} onClick={() => goPage(1, true)} title="Next page (Page Down)">Next <IconRight size={16} /></button>
        </div>
      )}

      <Dock label="Guide controls" className="ctv-dock" btn="ctv-btn sm" scope="list" actions={dockActions} />
      <div className="ctv-legals">
        <LegalNote kind="tv" className="ctv-legal" />
        {logosShown && <LegalNote kind="logos" className="ctv-legal" />}
      </div>

      {moreInfo && <MoreInfo channelKey={moreInfo.key} at={moreInfo.at} onClose={() => { setMoreInfo(null); wantFocus.current = true }} />}
      {details && <DetailsDialog ch={details.ch} p={details.p} now={now} startDay={details.day} look={look} onClose={() => setDetails(null)} onWatch={() => watch(details.ch.key)}
        onRecord={() => { void record(details.ch, details.p); setDetails(null) }}
        onRecordOptions={hasRules2 ? (series) => { const d = details; setDetails(null); offerRecord(d.ch, d.p, series) } : undefined}
        onFromStart={hasCatchup ? () => watchFromStart(details.ch, details.p) : undefined}
        onAddToSet={() => { const ch = details.ch; setDetails(null); setAddTo(ch) }}
        onReset={() => { void refreshLineup(); refreshView() }} />}
      {lookOpen && <LookPanel look={look} onClose={() => setLookOpen(false)} />}
      {pickerOpen && sets && <SetPicker sets={sets.sets} current={setId} onPick={pickSet} onClose={() => { setPickerOpen(false); wantFocus.current = true }} />}
      {addTo && sets && <AddToSet ch={addTo} sets={sets.sets} onClose={() => { setAddTo(null); wantFocus.current = true }} onChanged={(id) => { loadSets(); if (id === setId) refreshView(true) }} />}
      {recordFor && <RecordOptions ch={recordFor.ch} p={recordFor.p} series={recordFor.series} onClose={() => { setRecordFor(null); wantFocus.current = true }} />}
      {searchOpen && <SearchPanel onClose={closeSearch} onGo={(hit) => {
        setSearchOpen(false)
        void (async () => {
          const i = await locate(hit.channel).catch(() => -1)
          if (i >= 0) { goAt(hit.start, i); return }
          const k = await locate(hit.channel, true).catch(() => -1)
          showAllAt(Math.max(0, k), hit.start)
        })()
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
  // Whether what's being pressed was already highlighted when the press began.
  const pressedHighlighted = useRef(false)
  const focusedCell = focusAt == null ? null : cells.find((c) => c.start <= focusAt && c.end > focusAt) || cells[0]
  const accent = ch?.accent ? channelVars(ch.accent, look.bg) : undefined
  // With a highlighted tile the logo needs the version that shows up on it.
  const on = surfaceOf(look, focusAt != null)
  return (
    <div className={'ctv-row' + (focusAt != null ? ' current' : '')} role="row" aria-rowindex={r + 2} style={{ top, height: rowH, ...accent }}>
      {ch ? (
        // Named in words: the number and the logo or name beside it would run together as "101MOVIE CHANNEL".
        <div className="ctv-ch" role="rowheader" aria-colindex={1} title={ch.name} aria-label={[`${ch.number} ${ch.name}`, ch.favourite && 'favourite', ch.encrypted && 'encrypted'].filter(Boolean).join(', ')}>
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
              onPointerDown={() => { pressedHighlighted.current = focused }}
              onFocus={() => { if (!focused) onFocus(r, Math.max(p.start, from)) }}
              onClick={(e) => { if (focused && (e.detail === 0 || pressedHighlighted.current)) onActivate(ch, p); else { onFocus(r, Math.max(p.start, from)); wantFocus.current = true } }}
              onDoubleClick={() => onActivate(ch, p)}>
              {look.progress && live && p.kind !== 'none' && <i className="ctv-fill" aria-hidden="true" style={{ width: ((now - start) / Math.max(1, end - start)) * 100 + '%' }} />}
              {!tiny && p.start < from && <span className="ctv-more" aria-hidden="true">◄</span>}
              {tiny ? (p.kind === 'group' ? <span className="ctv-count" aria-hidden="true">{p.group?.length}</span> : null) : (
                <span className="ctv-cell-text">
                  <b>{p.title}</b>
                  {look.subtitles && SHAPES[look.shape].details && details && <small>{details}</small>}
                  {look.subtitles && !SHAPES[look.shape].details && p.sub && <small>{p.sub}</small>}
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
