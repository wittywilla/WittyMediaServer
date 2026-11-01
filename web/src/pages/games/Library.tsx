import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { PromptBar, type Prompt } from '@wittywilla/wittyplay/web/prompts'
import { api, type GameConsole, type GameFacets, type Item, type Library } from '../../api'
import { EmptyState, LoadError, Placeholder, ScanButton } from '../../components/common'
import LegalNote from '../../components/LegalNote'
import { IconFull, IconGamepad, IconPlay, IconSearch, IconSettings, IconStar, IconTune } from '../../icons'
import { toast, useApp } from '../../state'
import { Chip, CardRow, Sheet } from '../../ui'
import { GameMetaNotice } from '../GameMeta'
import { CoverFlow } from './CoverFlow'
import { cachedHome, cardCover, consoleMap, fmtAgo, fmtPlayed, forgetHome, getHome, heroCover, saveThumb, setCouchOn, useCoverTint, useGameLinks, type GameContinue, type GameHome } from './data'
import { VirtualGames, type VirtualHandle } from './VirtualGames'
import { ConsoleMark, useConsoleLogos } from './ConsoleMark'
import './games-ui.css'

// The games home: covers at the top, what's being played, the consoles, shelves, then every
// game. A console or a filter narrows it to one view of the same page. It works with a
// mouse, a finger, a keyboard, a remote or a controller, and the last two see which
// buttons do what along the bottom.

const SORTS = [
  { id: 'popular', label: 'Most popular' },
  { id: 'newest', label: 'Newest first' },
  { id: 'oldest', label: 'Oldest first' },
  { id: 'title', label: 'A to Z' },
] as const
const MORE_SORTS = [{ id: 'rating', label: 'Highest rated' }, { id: 'added', label: 'Recently added' }] as const
const DEFAULT_SORT = 'popular'

// A scan counts archives it can't read by why, and other files by their extension.
const PROBLEMS: Record<string, string> = {
  encrypted: 'password-protected archive', unsafe: 'unsafe archive', toobig: 'oversized archive', toomany: 'archive with too many files',
  damaged: 'damaged archive', method: 'archive with unsupported compression', unavailable: 'unreadable archive',
}
const listProblem = (ext: string, n: number) => `${n.toLocaleString()} ${PROBLEMS[ext] ? (n === 1 ? PROBLEMS[ext] : PROBLEMS[ext].replace('archive', 'archives')) : ext}`
const and = (parts: string[]) => (parts.length < 2 ? parts.join('') : `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`)

const typingIn = (el: EventTarget | null) => !!(el as Element | null)?.matches?.('input:not([type=checkbox]):not([type=radio]):not([type=range]), textarea, select, [contenteditable]')

// Where someone was, for when they come back from a game.
const lastPlace = new Map<string, { y: number; item: number }>()

export function GameCard({ item, console: c, fav, to }: { item: Item; console?: GameConsole; fav?: boolean; to?: string }) {
  const links = useGameLinks()
  return (
    <Link to={to || links.item(item.id)} className="card gm-card" title={item.title} data-gm-item={item.id}>
      <div className="poster">
        {item.poster ? <img src={cardCover(item)} alt="" loading="lazy" decoding="async" onLoad={(e) => e.currentTarget.classList.add('in')} /> : <Placeholder title={item.title} label={c?.short} />}
        {fav && <span className="gm-card-fav" role="img" aria-label="Favourite"><IconStar size={14} /></span>}
        <div className="play"><span><IconPlay /></span></div>
      </div>
      <div className="title">{item.title}</div>
      <div className="sub">
        {item.rating != null && <span className="gm-card-score"><IconStar size={12} />{item.rating.toFixed(1)}</span>}
        {[c?.short, item.year].filter(Boolean).join(' · ')}
      </div>
    </Link>
  )
}

// The tint behind the page, faded from one colour to the next on two layers, so a
// change of colour is one opacity animation.
export function Ambient({ colour }: { colour: string | null }) {
  const layers = useRef<(HTMLDivElement | null)[]>([])
  const front = useRef(0)
  useEffect(() => {
    if (!colour) return
    const next = 1 - front.current
    const a = layers.current[next], b = layers.current[front.current]
    if (!a || !b) return
    a.style.setProperty('--gm-tint', colour)
    a.style.opacity = '1'
    b.style.opacity = '0'
    front.current = next
  }, [colour])
  return (
    <div className="detail-hero gm-ambient" aria-hidden="true">
      <div ref={(el) => { layers.current[0] = el }} className="gm-ambient-layer" />
      <div ref={(el) => { layers.current[1] = el }} className="gm-ambient-layer" />
    </div>
  )
}

// Most interesting in the middle, with the rest spreading out on either side of it.
function centreOut(list: Item[]): Item[] {
  const out: Item[] = []
  list.forEach((it, i) => { if (i % 2) out.push(it); else out.unshift(it) })
  return out
}

// Which cover was at the front, for coming back to the same one.
const heroAt = new Map<string, number>()

function Hero({ items: list, consoles, resumable, title, place }: { items: Item[]; consoles: Map<string, GameConsole>; resumable: Set<number>; title: string; place: string }) {
  const items = useMemo(() => centreOut(list), [list])
  const [at, setAtState] = useState<number | null>(heroAt.get(place) ?? null)
  const setAt = useCallback((i: number) => { heroAt.set(place, i); setAtState(i) }, [place])
  const info = useRef<HTMLDivElement>(null)
  const links = useGameLinks()
  const nav = useNavigate()
  const index = Math.min(at ?? (items.length - 1) >> 1, items.length - 1)
  const it = items[index]
  const tint = useCoverTint(it?.poster ? heroCover(it) : null)
  if (!it) return null
  const c = consoles.get(it.console || '')
  const ready = c?.ready ?? true
  const resume = resumable.has(it.id)
  return (
    <section className="gm-hero" aria-label={title} data-gm-section>
      <Ambient colour={tint} />
      <CoverFlow items={items} index={index} onIndex={setAt} consoles={consoles} onOpen={(g) => nav(links.item(g.id))} label={title}
        onDown={() => { const b = info.current?.querySelector<HTMLElement>('a[href]'); b?.focus({ preventScroll: true }); return !!b }} />
      <div className="gm-hero-info" ref={info}>
        <h2 aria-live="polite">{it.title}</h2>
        <div className="meta">{[c?.name || c?.short, it.year, it.studio].filter(Boolean).map((t) => <span key={String(t)}>{t}</span>)}</div>
        <div className="gm-hero-actions">
          {ready
            ? <Link className="btn primary big gm-play" to={links.play(it.id, resume ? 'resume=1' : '')}><IconPlay /> {resume ? 'Resume' : 'Play'}</Link>
            : <Link className="btn big" to={links.item(it.id)}><IconSettings size={16} /> Needs setting up</Link>}
          <Link className="btn big ghost" to={links.item(it.id)} aria-label={`About ${it.title}`}>About this game</Link>
        </div>
      </div>
    </section>
  )
}

function ContinueCard({ e, console: c }: { e: GameContinue; console?: GameConsole }) {
  const links = useGameLinks()
  const it = e.item
  const ready = c?.ready ?? true
  const played = it.progress?.positionSec || 0
  const thumb = e.save?.hasThumb ? saveThumb(it.id, e.save.slot, e.save.updatedAt) : null
  return (
    <Link to={ready ? links.play(it.id, e.save ? 'resume=1' : '') : links.item(it.id)} className="gm-cont" data-gm-item={it.id}
      aria-label={`${e.save ? 'Resume' : 'Play'} ${it.title}${c ? ', ' + c.short : ''}, last played ${fmtAgo(e.lastPlayed)}`}>
      <span className="gm-cont-shot">
        {thumb ? <img src={thumb} alt="" decoding="async" /> : it.poster ? <img className="soft" src={cardCover(it)} alt="" decoding="async" /> : <Placeholder title={it.title} label={c?.short} />}
        <span className="gm-cont-play"><IconPlay /></span>
      </span>
      <span className="gm-cont-text">
        <b>{it.title}</b>
        <small>{[c?.short, played > 60 ? fmtPlayed(played) : '', fmtAgo(e.lastPlayed)].filter(Boolean).join(' · ')}</small>
      </span>
    </Link>
  )
}

function ConsoleTile({ c, lib }: { c: GameConsole; lib: number }) {
  const links = useGameLinks()
  const { user } = useApp()
  const n = c.count || 0
  return (
    <Link to={links.library(lib, `console=${encodeURIComponent(c.id)}`)} className="gm-console" aria-label={`${c.name}: ${n.toLocaleString()} ${n === 1 ? 'game' : 'games'}${c.ready ? '' : ', not set up yet'}`}>
      <ConsoleMark console={c} />
      <small>{n.toLocaleString()} {n === 1 ? 'game' : 'games'}{!c.ready && user?.isAdmin ? ' · set up' : ''}</small>
    </Link>
  )
}

function ShelfOf({ title, items, consoles, favs, to, wide }: { title: string; items: Item[]; consoles: Map<string, GameConsole>; favs: Set<number>; to?: string; wide?: boolean }) {
  if (items.length === 0) return null
  return (
    <section className={'hub gm-section' + (wide ? ' wide' : '')} data-gm-section aria-label={title}>
      <div className="hub-head"><h2>{title}</h2>{to && <Link to={to} aria-label={`See all ${title}`}>See all</Link>}</div>
      <CardRow>{items.map((it) => <GameCard key={it.id} item={it} console={consoles.get(it.console || '')} fav={favs.has(it.id)} />)}</CardRow>
    </section>
  )
}

function FiltersSheet({ facets, params, set, onClose }: { facets: GameFacets | null; params: URLSearchParams; set: (k: string, v: string) => void; onClose: () => void }) {
  const Group = ({ title, name, options }: { title: string; name: string; options: { value: string; label: string }[] }) => {
    if (options.length === 0) return null
    const now = params.get(name) || ''
    return (
      <fieldset className="gm-filter-group">
        <legend>{title}</legend>
        <div className="chips">{options.map((o) => <Chip key={o.value} selected={now === o.value} onClick={() => set(name, now === o.value ? '' : o.value)}>{o.label}</Chip>)}</div>
      </fieldset>
    )
  }
  const sort = params.get('sort') || DEFAULT_SORT
  return (
    <Sheet title="Filter and sort" onClose={onClose} actions={<button type="button" className="btn primary" onClick={onClose}>Done</button>}>
      <fieldset className="gm-filter-group">
        <legend>Sort</legend>
        <div className="chips">{[...SORTS, ...MORE_SORTS].map((s) => <Chip key={s.id} selected={sort === s.id} onClick={() => set('sort', s.id === DEFAULT_SORT ? '' : s.id)}>{s.label}</Chip>)}</div>
      </fieldset>
      <Group title="Genre" name="genre" options={(facets?.genres || []).slice(0, 40).map((g) => ({ value: g.value, label: `${g.value} (${g.count.toLocaleString()})` }))} />
      <Group title="Series" name="series" options={(facets?.series || []).slice(0, 40).map((g) => ({ value: g.value, label: `${g.value} (${g.count.toLocaleString()})` }))} />
      <Group title="Decade" name="decade" options={(facets?.decades || []).map((g) => ({ value: g.value, label: `${g.value}s (${g.count.toLocaleString()})` }))} />
      <Group title="Players" name="players" options={[...new Set((facets?.players || []).map((g) => Number(g.value)).filter((n) => n > 1))].map((n) => ({ value: String(n), label: `${n} or more` }))} />
      {!!facets?.rated && <Group title="Rating" name="score" options={[9, 8, 7, 6].map((n) => ({ value: String(n), label: `${n} and up` }))} />}
    </Sheet>
  )
}

function LetterSheet({ letters, onJump, onClose }: { letters: { label: string; offset: number; count: number }[]; onJump: (offset: number) => void; onClose: () => void }) {
  return (
    <Sheet title="Jump to a letter" onClose={onClose}>
      <div className="gm-letters">
        {letters.map((l) => <button key={l.label} type="button" className="btn gm-letter" aria-label={`${l.label}: ${l.count.toLocaleString()} games`} onClick={() => { onJump(l.offset); onClose() }}>{l.label}</button>)}
      </div>
    </Sheet>
  )
}

const FILTER_NAMES: Record<string, string> = { genre: 'Genre', series: 'Series', decade: 'Decade', players: 'Players', score: 'Rating', unmatched: 'No match' }

export default function GamesLibrary({ lib }: { lib: Library }) {
  const links = useGameLinks()
  const nav = useNavigate()
  const { user } = useApp()
  const [params, setParams] = useSearchParams()
  const [consoles, setConsoles] = useState<GameConsole[]>([])
  const [home, setHome] = useState<GameHome | null>(() => cachedHome(lib.id))
  const [homeFailed, setHomeFailed] = useState(false)
  const [facets, setFacets] = useState<GameFacets | null>(null)
  const [total, setTotal] = useState<number | null>(null)
  const [skipped, setSkipped] = useState<Record<string, number>>({})
  const [version, setVersion] = useState(0)
  const [watch, setWatch] = useState(0)
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [lettersOpen, setLettersOpen] = useState(false)
  const [favs, setFavs] = useState<Set<number>>(() => new Set(cachedHome(lib.id)?.favouriteIds || []))
  const grid = useRef<VirtualHandle>(null)
  const search = useRef<HTMLInputElement>(null)

  const consoleId = params.get('console') || ''
  const q = params.get('q') || ''
  const sort = params.get('sort') || DEFAULT_SORT
  const genre = params.get('genre') || '', decade = params.get('decade') || '', players = params.get('players') || '', score = params.get('score') || '', series = params.get('series') || ''
  const unmatched = params.get('unmatched') === '1'
  const narrowed = !!(q || genre || decade || players || score || series || unmatched)
  const isHome = !narrowed && !consoleId
  const [typed, setTyped] = useState(q)

  const set = useCallback((k: string, v: string) => {
    setParams((p) => { const n = new URLSearchParams(p); if (v) n.set(k, v); else n.delete(k); return n }, { replace: true })
  }, [setParams])

  const descending = sort === 'rating' || sort === 'added'
  const query = `library=${lib.id}&kind=game&sort=${sort}&desc=${descending ? 1 : 0}&console=${encodeURIComponent(consoleId)}&search=${encodeURIComponent(q)}&genre=${encodeURIComponent(genre)}&series=${encodeURIComponent(series)}&decade=${decade}&players=${players}&minRating=${score}${unmatched ? '&unmatched=1' : ''}`

  const loadConsoles = useCallback(() => api.get<GameConsole[]>(`/games/consoles?library=${lib.id}`).then(setConsoles).catch(() => {}), [lib.id])
  const loadHome = useCallback(() => getHome(lib.id).then((h) => { setHome(h); setFavs(new Set(h.favouriteIds)); setHomeFailed(false) }).catch(() => setHomeFailed(true)), [lib.id])
  useEffect(() => { void loadHome() }, [loadHome])
  useEffect(() => { api.get<GameFacets>(`/games/facets?library=${lib.id}&console=${encodeURIComponent(consoleId)}`).then(setFacets).catch(() => {}) }, [lib.id, consoleId, version])
  useEffect(() => { const t = typed.trim(); if (t === q) return; const id = setTimeout(() => set('q', t), 300); return () => clearTimeout(id) }, [typed]) // eslint-disable-line react-hooks/exhaustive-deps

  // A scan may be running, or just started. Follow it, so games appear as they're found.
  useEffect(() => {
    let stop = false, timer = 0
    const tick = async () => {
      const r = await api.get<{ scanning: boolean; skipped?: Record<string, number> }>(`/libraries/${lib.id}/scan`).catch(() => ({ scanning: false, skipped: undefined }))
      if (stop) return
      setSkipped(r.skipped || {})
      await loadConsoles()
      if (r.scanning) { setVersion((n) => n + 1); forgetHome(lib.id); void loadHome(); timer = window.setTimeout(tick, 3000) }
    }
    void tick()
    return () => { stop = true; clearTimeout(timer) }
  }, [lib.id, watch, loadConsoles, loadHome])

  // A console page has covers of its own at the top.
  const [consoleHero, setConsoleHero] = useState<Item[]>([])
  useEffect(() => {
    if (!consoleId) { setConsoleHero([]); return }
    api.get<{ items: Item[] }>(`/items?library=${lib.id}&kind=game&console=${encodeURIComponent(consoleId)}&sort=popular&limit=40`).then((r) => setConsoleHero(r.items.filter((it) => it.poster).slice(0, 15))).catch(() => setConsoleHero([]))
  }, [lib.id, consoleId])

  const byId = useMemo(() => consoleMap(consoles), [consoles])
  const present = useMemo(() => consoles.filter((c) => (c.count || 0) > 0).sort((a, b) => (b.count || 0) - (a.count || 0)), [consoles])
  const everything = present.reduce((n, c) => n + (c.count || 0), 0)
  const logosShown = useConsoleLogos(isHome ? present : [])
  const current = consoleId ? byId.get(consoleId) : undefined
  const resumable = useMemo(() => new Set((home?.continue || []).filter((e) => e.save).map((e) => e.item.id)), [home])
  const left = Object.entries(skipped).sort((a, b) => b[1] - a[1])
  const leftCount = left.reduce((n, [, c]) => n + c, 0)

  // Jumping along titles: only when the list is every game of this view, by title.
  const letters = useMemo(() => {
    if (sort !== 'title' || q || genre || decade || players || score || series || unmatched) return []
    let offset = 0
    const out: { label: string; offset: number; count: number }[] = []
    for (const f of facets?.letters || []) {
      const label = /^[A-Z]$/.test(f.value) ? f.value : '#'
      const had = out.find((x) => x.label === label)
      if (had) had.count += f.count
      else out.push({ label, offset, count: f.count })
      offset += f.count
    }
    return out
  }, [facets, sort, q, genre, decade, players, score, series, unmatched])

  const toggleFav = useCallback(async (id: number) => {
    const on = !favs.has(id)
    setFavs((s) => { const n = new Set(s); if (on) n.add(id); else n.delete(id); return n })
    try {
      await api.put(`/games/${id}/meta`, { favourite: on })
      toast(on ? 'Added to favourites' : 'Taken out of favourites')
      forgetHome(lib.id)
      void loadHome()
    } catch (e: any) { toast(e.message) }
  }, [favs, lib.id, loadHome])

  // Controller and keyboard shortcuts. tv.ts turns a controller's buttons into keys, which
  // aren't "trusted" like real ones, so that letters typed on a keyboard mean nothing here.
  const cycleSort = useCallback(() => {
    const i = SORTS.findIndex((s) => s.id === sort)
    const next = SORTS[(i + 1) % SORTS.length]
    set('sort', next.id === DEFAULT_SORT ? '' : next.id)
    toast(next.label)
  }, [sort, set])
  const sectionJump = useCallback((by: number) => {
    const all = Array.from(document.querySelectorAll<HTMLElement>('[data-gm-section]'))
    if (!all.length) return
    // From the section in use, or else the one on screen.
    const own = (document.activeElement as HTMLElement | null)?.closest?.('[data-gm-section]') as HTMLElement | null
    let at = own ? all.indexOf(own) : -1
    if (at < 0) {
      const y = window.innerHeight * 0.28
      at = all.findIndex((s) => s.getBoundingClientRect().bottom > y)
      if (at < 0) at = all.length - 1
    }
    const to = all[Math.max(0, Math.min(all.length - 1, at + by))]
    to.scrollIntoView({ block: 'start', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
    const first = to.querySelector<HTMLElement>('[data-front="1"], .card:not(.gm-card-wait), .gm-cont, .gm-console') || to.querySelector<HTMLElement>('a[href], button:not([disabled])')
    first?.focus({ preventScroll: true })
  }, [])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.ctrlKey || e.altKey || e.metaKey || typingIn(e.target) || document.querySelector('.modal')) return
      const pad = !e.isTrusted
      const itemOf = () => Number((document.activeElement as HTMLElement | null)?.closest?.('[data-gm-item]')?.getAttribute('data-gm-item') || (document.activeElement as HTMLElement | null)?.getAttribute?.('data-gm-flow') || 0)
      if (e.key === 'PageUp' || e.key === 'PageDown') { e.preventDefault(); sectionJump(e.key === 'PageUp' ? -1 : 1) }
      else if (pad && e.key === ' ') { e.preventDefault(); search.current?.focus(); if (links.couch) nav('/big/search') }
      else if ((pad && e.key === 'm') || (!pad && e.key === 'f')) { const id = itemOf(); if (id) { e.preventDefault(); void toggleFav(id) } }
      else if (pad && e.key === 't') { e.preventDefault(); cycleSort() }
      else if (pad && e.key === 'ContextMenu' && !links.couch) { e.preventDefault(); setCouchOn(true); nav('/big') }
      else if (!pad && e.key === '/') { e.preventDefault(); (window.matchMedia('(max-width: 600px)').matches ? document.querySelector<HTMLElement>('.at-list input') : search.current)?.focus() }
      else if (pad && (e.key === 'MediaRewind' || e.key === 'MediaFastForward')) {
        if (!letters.length) { sectionJump(e.key === 'MediaRewind' ? -1 : 1); return }
        const focusedAt = Number((document.activeElement as HTMLElement | null)?.closest?.('[data-gm-at]')?.getAttribute('data-gm-at') ?? -1)
        const i = letters.reduce((best, l, n) => (l.offset <= focusedAt ? n : best), 0)
        const to = letters[Math.max(0, Math.min(letters.length - 1, i + (e.key === 'MediaRewind' ? -1 : 1)))]
        grid.current?.jumpTo(to.offset)
        toast(to.label)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [links.couch, nav, toggleFav, cycleSort, sectionJump, letters])

  // Back from a game: the same place, the same card under focus.
  const placeKey = `${lib.id}?${params.toString()}`
  const placeRef = useRef(placeKey)
  placeRef.current = placeKey
  useLayoutEffect(() => {
    const spot = lastPlace.get(placeKey)
    if (!spot) return
    const frame = requestAnimationFrame(() => {
      window.scrollTo(0, spot.y)
      document.querySelector<HTMLElement>(`[data-gm-item="${spot.item}"]`)?.focus({ preventScroll: true })
    })
    return () => cancelAnimationFrame(frame)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const remember = (e: React.MouseEvent) => {
    const id = (e.target as Element).closest?.('[data-gm-item]')?.getAttribute('data-gm-item')
    lastPlace.set(placeRef.current, { y: window.scrollY, item: Number(id || 0) })
  }

  const prompts: Prompt[] = [
    { button: 'dpad', label: 'Move' },
    { button: 'confirm', label: 'Open' },
    { button: 'back', label: 'Back' },
    { button: 'north', label: 'Favourite', key: 'F' },
    { button: 'west', label: 'Search', key: '/' },
    { button: 'select', label: 'Sort' },
    { button: 'lb', label: 'Sections' },
    ...(letters.length ? [{ button: 'rt', label: 'Next letter' } as Prompt] : []),
    ...(links.couch ? [] : [{ button: 'start', label: 'Couch mode' } as Prompt]),
  ]

  const clear = () => { setTyped(''); setParams(consoleId ? { console: consoleId } : {}, { replace: true }) }
  const activeFilters = Object.keys(FILTER_NAMES).filter((k) => params.get(k))
  const title = current ? current.name : lib.name
  const shownTotal = total ?? (consoleId ? current?.count : narrowed ? undefined : everything)
  const showHeroHome = isHome && home && home.hero.length > 0
  const shelves = home?.shelves || []
  const genreShelves = shelves.filter((s) => s.kind === 'genre')
  const seriesShelves = shelves.filter((s) => s.kind === 'series')

  return (
    <div className={'gm-page' + (showHeroHome || consoleHero.length > 0 ? ' has-hero' : '')} onClickCapture={remember}>
      <div className="toolbar gm-toolbar">
        {consoleId && <Link className="btn ghost sm" to={links.library(lib.id)}>All consoles</Link>}
        <h1>{current ? <ConsoleMark console={current} heading /> : title}</h1>
        {shownTotal != null && <span className="count">{shownTotal.toLocaleString()} {shownTotal === 1 ? 'game' : 'games'}</span>}
        <div className="grow" />
        {!links.couch && (
          <>
            <label className="gm-search-box at-top">
              <IconSearch size={18} />
              <input ref={search} className="input gm-search" type="search" placeholder="Search these games" aria-label="Search these games" value={typed} onChange={(e) => setTyped(e.target.value)} />
            </label>
            <ScanButton lib={lib} onStarted={() => setWatch((n) => n + 1)} />
          </>
        )}
        {!links.couch && <Link className="btn ghost" to="/big" onClick={() => { setCouchOn(true); void document.documentElement.requestFullscreen?.().catch(() => {}) }} aria-label="Couch mode: full screen, made for controllers"><IconFull size={18} /> <span className="gm-hide-narrow">Couch mode</span></Link>}
      </div>
      {current && !current.ready && (
        <div className="game-note" role="status">
          {current.unavailable || <>This server has no core for the {current.name} yet, so these games can’t start.{' '}
            {user?.isAdmin ? <Link to="/settings/gaming"><IconSettings size={15} /> Install one in Settings</Link> : 'Ask whoever runs this server to install one.'}</>}
        </div>
      )}

      {showHeroHome && <Hero items={home!.hero} consoles={byId} resumable={resumable} title="Featured games" place={`${lib.id}`} />}
      {!isHome && consoleId && !narrowed && consoleHero.length > 0 && <Hero items={consoleHero} consoles={byId} resumable={resumable} title={`${current?.short || 'These'} games`} place={`${lib.id}/${consoleId}`} />}
      {isHome && homeFailed && !home && <LoadError what="This library" onRetry={() => { void loadHome() }} />}

      {isHome && home && home.continue.length > 0 && (
        <section className="hub gm-section" data-gm-section aria-label="Continue playing">
          <div className="hub-head"><h2>Continue playing</h2></div>
          <CardRow>{home.continue.map((e) => <ContinueCard key={e.item.id} e={e} console={byId.get(e.item.console || '')} />)}</CardRow>
        </section>
      )}
      {isHome && present.length > 0 && (
        <section className="hub gm-section" data-gm-section aria-label="Consoles">
          <div className="hub-head"><h2>Consoles</h2><a href="#gm-all" onClick={(e) => { e.preventDefault(); document.getElementById('gm-all')?.scrollIntoView(); document.querySelector<HTMLElement>('#gm-all .chip')?.focus({ preventScroll: true }) }}>All {everything.toLocaleString()} games</a></div>
          <CardRow>{present.map((c) => <ConsoleTile key={c.id} c={c} lib={lib.id} />)}</CardRow>
          {logosShown && <LegalNote kind="logos" />}
        </section>
      )}
      {isHome && home && (
        <>
          <ShelfOf title="Recently added" items={home.recent} consoles={byId} favs={favs} to={links.library(lib.id, 'sort=added')} />
          <ShelfOf title="Favourites" items={home.favourites} consoles={byId} favs={favs} />
          <ShelfOf title="Most played" items={home.mostPlayed} consoles={byId} favs={favs} />
          {seriesShelves.map((s) => <ShelfOf key={'s' + s.name} title={s.name} items={s.items} consoles={byId} favs={favs} to={links.library(lib.id, 'series=' + encodeURIComponent(s.name))} />)}
          {genreShelves.map((s) => <ShelfOf key={'g' + s.name} title={s.name} items={s.items} consoles={byId} favs={favs} to={links.library(lib.id, 'genre=' + encodeURIComponent(s.name))} />)}
        </>
      )}

      {!links.couch && user?.isAdmin && (isHome || unmatched) && <GameMetaNotice unmatched={unmatched} onUnmatched={(on) => set('unmatched', on ? '1' : '')} onProgress={() => { setVersion((n) => n + 1) }} />}
      {!links.couch && leftCount > 0 && (
        <div className="warn-box gm-skipped" role="status">
          <strong>{leftCount.toLocaleString()} {leftCount === 1 ? 'file' : 'files'} in this library’s folders didn’t become games</strong>{' '}
          {and(left.map(([e, c]) => listProblem(e, c)))}.{' '}
          {left.some(([e]) => e === 'encrypted') ? 'This server doesn’t open archives with a password: unpack those yourself. ' : ''}
          Anything else is a kind of file, or a console, that isn’t supported yet.
        </div>
      )}
      <section className="gm-section gm-all" id="gm-all" data-gm-section aria-label="Games">
        <div className="gm-all-head">
          <h2>Games {total != null && <small>{total.toLocaleString()} {total === 1 ? 'game' : 'games'}</small>}</h2>
          {/* On a phone the search is here, beside the list it searches, so the covers come first. */}
          {!links.couch && (
            <label className="gm-search-box at-list">
              <IconSearch size={18} />
              <input className="input gm-search" type="search" placeholder="Search these games" aria-label="Search these games" value={typed} onChange={(e) => setTyped(e.target.value)} />
            </label>
          )}
          <div className="chips gm-sorts" role="group" aria-label="Sort">
            {SORTS.map((s) => <Chip key={s.id} selected={sort === s.id} onClick={() => set('sort', s.id === DEFAULT_SORT ? '' : s.id)}>{s.label}</Chip>)}
            {MORE_SORTS.some((s) => s.id === sort) && <Chip selected onClick={() => setFiltersOpen(true)}>{MORE_SORTS.find((s) => s.id === sort)!.label}</Chip>}
            <button type="button" className="chip gm-filter-btn" onClick={() => setFiltersOpen(true)}><IconTune size={16} /> Filter{activeFilters.length ? ` (${activeFilters.length})` : ''}</button>
            {letters.length > 1 && <button type="button" className="chip" onClick={() => setLettersOpen(true)}>Jump to a letter</button>}
          </div>
          {activeFilters.length > 0 && (
            <div className="chips gm-active" role="group" aria-label="Filters in use">
              {activeFilters.map((k) => <Chip key={k} onRemove={() => set(k, '')} removeLabel={`Remove ${FILTER_NAMES[k]} filter`}>{FILTER_NAMES[k]}: {params.get(k)}</Chip>)}
            </div>
          )}
        </div>
        <VirtualGames handle={grid} query={query} version={version} onTotal={setTotal} label={`${title}: games`} restore={false}
          card={(it) => <GameCard item={it} console={byId.get(it.console || '')} fav={favs.has(it.id)} />}
          failed={(retry) => <LoadError what="This library" onRetry={retry} />}
          empty={narrowed
            ? <EmptyState icon={<IconGamepad />} title="No games match" text={q ? `Nothing here matches “${q}”.` : 'No games here match those filters.'} action={<button type="button" className="btn" onClick={clear}>Show all games</button>} />
            : <EmptyState icon={<IconGamepad />} title="No games here yet" text="Put your own game files in this library’s folder and scan it. Zipped games and multi-disc sets work too." />} />
      </section>
      <LegalNote kind="games" />
      {filtersOpen && <FiltersSheet facets={facets} params={params} set={set} onClose={() => setFiltersOpen(false)} />}
      {lettersOpen && <LetterSheet letters={letters} onJump={(o) => grid.current?.jumpTo(o)} onClose={() => setLettersOpen(false)} />}
      <PromptBar className="gm-prompts" always={links.couch} items={prompts} />
    </div>
  )
}

