import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, backdropUrl, getToken, mediaUrl, posterUrl, serverBase, type GameConsole, type GameDetailsInfo, type GameFacets, type Item, type Library } from '../api'
import ArtworkPicker from '../components/ArtworkPicker'
import { BackLink, EmptyState, Placeholder, ScanButton, SkeletonGrid, Spinner, artHue } from '../components/common'
import { IconEdit, IconGamepad, IconImage, IconPlay, IconSettings, IconSortDown, IconSortUp, IconStar } from '../icons'
import { GameCredits, GameMatchBar, GameMetaNotice, GameRatings, GameShots } from './GameMeta'
import { toast, useApp } from '../state'
import type { Background, ControllerData, ControlsData, PlayHost, PrepareStatus, Profile, ProfileHost, SaveSlot, SessionInfo } from '@wittywilla/wittyplay'
import './games.css'

// Games: the library page, a game's own page, and the page that holds the player.
// The player itself is WittyPlay (modules/wittyplay/web), loaded only when a game
// is opened.

const GamePlayer = lazy(() => import('@wittywilla/wittyplay'))
const Controllers = lazy(() => import('@wittywilla/wittyplay').then((m) => ({ default: m.ControllersPage })))

// Controller profiles and the background, through the same routes a TV app uses.
const profileHost: ProfileHost = {
  list: () => api.get<Profile[]>('/game-profiles'),
  create: (p) => api.post<Profile>('/game-profiles', p),
  update: (id, p) => api.put<Profile>(`/game-profiles/${id}`, p),
  remove: async (id) => { await api.del(`/game-profiles/${id}`) },
  copy: (id) => api.post<Profile>(`/game-profiles/${id}/copy`),
  setActive: (id, active) => api.post<Profile>(`/game-profiles/${id}/active`, { active }),
  exportFile: async (ids) => JSON.stringify(await api.get(`/game-profiles/export?${(ids || []).map((i) => `id=${i}`).join('&')}`), null, 2),
  importFile: (text) => sendBytes('POST', '/game-profiles/import', text),
  setBackground: (b) => api.put<Background>('/game-background', b),
  uploadBackground: (f) => sendBytes('PUT', '/game-background/image', f),
  removeBackground: () => api.del<Background>('/game-background'),
  backgroundUrl: (v) => mediaUrl('/api/game-background/image') + (mediaUrl('/api/game-background/image').includes('?') ? '&' : '?') + `v=${v}`,
}

// The body as it is, for a picture or a file. The server looks inside to see what it is.
async function sendBytes<T>(method: string, path: string, body: BodyInit): Promise<T> {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(`${serverBase()}/api${path}`, { method, headers, body, credentials: 'same-origin' })
  const data = await res.json().catch(() => null)
  if (!res.ok) throw new Error(data?.error || 'That didn’t work')
  return data as T
}

let knownControllers: Promise<ControllerData> | null = null
const getControllers = () => {
  if (!knownControllers) knownControllers = api.get<ControllerData>('/game-controllers')
  knownControllers.catch(() => { knownControllers = null })
  return knownControllers
}

let consolesCache: Promise<GameConsole[]> | null = null
export const getConsoles = (fresh = false): Promise<GameConsole[]> => {
  if (!consolesCache || fresh) consolesCache = api.get<GameConsole[]>('/games/consoles')
  return consolesCache
}

export function useConsoles() {
  const [list, setList] = useState<GameConsole[]>([])
  useEffect(() => { getConsoles().then((l) => { for (const c of l) knownConsoles.set(c.id, c.short); setList(l) }).catch(() => {}) }, [])
  return list
}

// Covers in lists are smaller copies, as big as a card is likely to be on this screen.
const cardWidth = window.innerWidth * (window.devicePixelRatio || 1) > 2000 ? 480 : 320

function GameCard({ item, console: c }: { item: Item; console?: GameConsole }) {
  return (
    <Link to={`/item/${item.id}`} className="card gm-card" title={item.title}>
      <div className="poster">
        {item.poster ? <img src={`${posterUrl(item)}&w=${cardWidth}`} alt="" loading="lazy" decoding="async" /> : <Placeholder title={item.title} label={c?.short} />}
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

// A page at a time: a collection can run to thousands of games, and neither an old
// TV nor a phone should pay for all of them to look at the first few.
const PAGE = 120
// A scan counts archives it can't read by why, and other files by their extension.
const PROBLEMS: Record<string, string> = {
  encrypted: 'password-protected archive', unsafe: 'unsafe archive', toobig: 'oversized archive', toomany: 'archive with too many files',
  damaged: 'damaged archive', method: 'archive with unsupported compression', unavailable: 'unreadable archive',
}
const list = (ext: string, n: number) => `${n.toLocaleString()} ${PROBLEMS[ext] ? (n === 1 ? PROBLEMS[ext] : PROBLEMS[ext].replace('archive', 'archives')) : ext}`
const and = (parts: string[]) => (parts.length < 2 ? parts.join('') : `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`)

export function GamesLibrary({ lib }: { lib: Library }) {
  const [consoles, setConsoles] = useState<GameConsole[]>([])
  const [params, setParams] = useSearchParams()
  const [items, setItems] = useState<Item[] | null>(null)
  const [total, setTotal] = useState(0)
  const [playing, setPlaying] = useState<Item[]>([])
  const [skipped, setSkipped] = useState<Record<string, number>>({})
  const filter = params.get('console') || ''
  const search = params.get('q') || ''
  const sort = params.get('sort') || 'title'
  const genre = params.get('genre') || ''
  const decade = params.get('decade') || ''
  const players = params.get('players') || ''
  const score = params.get('score') || ''
  // Something's rating or year is most interesting highest first.
  const desc = params.has('desc') ? params.get('desc') === '1' : sort === 'rating' || sort === 'year' || sort === 'added'
  const [facets, setFacets] = useState<GameFacets | null>(null)
  const [typed, setTyped] = useState(search)
  const url = (limit: number, offset = 0) => `/items?library=${lib.id}&kind=game&sort=${sort}&desc=${desc ? 1 : 0}&console=${encodeURIComponent(filter)}&search=${encodeURIComponent(search)}&genre=${encodeURIComponent(genre)}&decade=${decade}&players=${players}&minRating=${score}${unmatched ? '&unmatched=1' : ''}&limit=${limit}&offset=${offset}`
  const unmatched = params.get('unmatched') === '1'
  const narrowed = !!(search || filter || genre || decade || players || score || unmatched)
  const loadedRef = useRef(0)
  loadedRef.current = items?.length ?? 0
  const load = () => api.get<{ items: Item[]; total: number }>(url(Math.max(PAGE, loadedRef.current)))
    .then((r) => { setItems(r.items); setTotal(r.total) }).catch(() => setItems([]))
  const loadConsoles = () => api.get<GameConsole[]>(`/games/consoles?library=${lib.id}`).then(setConsoles).catch(() => {})
  const [watch, setWatch] = useState(0) // bumped to follow a scan just started
  useEffect(() => { setItems(null); loadedRef.current = 0; load() }, [lib.id, filter, search, sort, desc, genre, decade, players, score, unmatched]) // eslint-disable-line react-hooks/exhaustive-deps
  const loadFacets = () => api.get<GameFacets>(`/games/facets?library=${lib.id}&console=${encodeURIComponent(filter)}`).then(setFacets).catch(() => {})
  useEffect(() => { loadFacets() }, [lib.id, filter]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { api.get<{ items: Item[] }>(`/items?library=${lib.id}&kind=game&played=1&limit=50`).then((r) => setPlaying(r.items)).catch(() => {}) }, [lib.id])
  // A scan may already be running (a library just added), or just started. Follow
  // it, so games appear as they're found and the page ends up complete.
  useEffect(() => {
    let stop = false
    let timer = 0
    const tick = async () => {
      const r = await api.get<{ scanning: boolean; skipped?: Record<string, number> }>(`/libraries/${lib.id}/scan`).catch(() => ({ scanning: false, skipped: undefined }))
      if (stop) return
      setSkipped(r.skipped || {})
      await Promise.all([load(), loadConsoles()])
      if (r.scanning && !stop) timer = window.setTimeout(tick, 2000)
    }
    void tick()
    return () => { stop = true; clearTimeout(timer) }
  }, [lib.id, watch]) // eslint-disable-line react-hooks/exhaustive-deps
  // The next page, when the end of the list comes near.
  const more = useRef<HTMLDivElement | null>(null)
  const loaded = items?.length ?? 0
  const hasMore = loaded > 0 && loaded < total
  useEffect(() => {
    const el = more.current
    if (!el || !hasMore) return
    let busy = false
    const io = new IntersectionObserver((es) => {
      if (!es.some((e) => e.isIntersecting) || busy) return
      busy = true
      api.get<{ items: Item[]; total: number }>(url(PAGE, loaded))
        .then((r) => { setItems((cur) => [...(cur || []), ...r.items]); setTotal(r.total) })
        .catch(() => {}).finally(() => { busy = false })
    }, { rootMargin: '800px' })
    io.observe(el)
    return () => io.disconnect()
  }, [hasMore, loaded, filter, search, sort, desc, genre, decade, players, score, unmatched]) // eslint-disable-line react-hooks/exhaustive-deps
  const setParam = (k: string, v: string) => { const p = new URLSearchParams(params); v ? p.set(k, v) : p.delete(k); setParams(p, { replace: true }) }
  // Search follows typing, once it pauses.
  useEffect(() => {
    if (typed === search) return
    const t = setTimeout(() => setParam('q', typed.trim()), 300)
    return () => clearTimeout(t)
  }, [typed]) // eslint-disable-line react-hooks/exhaustive-deps
  const byId = useMemo(() => new Map(consoles.map((c) => [c.id, c])), [consoles])
  const present = consoles.filter((c) => (c.count || 0) > 0)
  const everything = present.reduce((n, c) => n + (c.count || 0), 0)
  const inProgress = [...playing].sort((a, b) => (b.progress?.updatedAt || 0) - (a.progress?.updatedAt || 0)).slice(0, 8)
  const left = Object.entries(skipped).sort((a, b) => b[1] - a[1])
  const leftCount = left.reduce((n, [, c]) => n + c, 0)
  const locked = left.some(([e]) => e === 'encrypted')
  const { user } = useApp()
  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}</h1>
        {items && <span className="count">{total.toLocaleString()} {total === 1 ? 'game' : 'games'}</span>}
        <div className="grow" />
        <input className="input gm-search" type="search" placeholder="Search these games…" aria-label="Search these games" value={typed} onChange={(e) => setTyped(e.target.value)} />
        <ScanButton lib={lib} onStarted={() => setWatch((n) => n + 1)} />
      </div>
      {user?.isAdmin && <GameMetaNotice unmatched={unmatched} onUnmatched={(on) => setParam('unmatched', on ? '1' : '')} onProgress={() => { load(); loadFacets() }} />}
      {leftCount > 0 && (
        <div className="warn-box gm-skipped" role="status">
          <strong>{leftCount.toLocaleString()} {leftCount === 1 ? 'file' : 'files'} in this library’s folders didn’t become games</strong>
          {and(left.map(([e, c]) => list(e, c)))}.{' '}
          {locked ? 'This server doesn’t open archives with a password: unpack those yourself. ' : ''}
          Anything else is a kind of file, or a console, that isn’t supported yet.
        </div>
      )}
      {present.length > 1 && (
        <div className="chips" role="group" aria-label="Console">
          <button type="button" aria-pressed={!filter} className={'chip' + (!filter ? ' active' : '')} onClick={() => setParam('console', '')}>All · {everything.toLocaleString()}</button>
          {present.map((c) => <button key={c.id} type="button" aria-pressed={filter === c.id} className={'chip' + (filter === c.id ? ' active' : '')} onClick={() => setParam('console', c.id)}>{c.short} · {(c.count || 0).toLocaleString()}</button>)}
        </div>
      )}
      <div className="gm-filters" role="group" aria-label="Sort and filter">
        <select className="select" aria-label="Sort by" value={sort} onChange={(e) => setParams((p) => { const n = new URLSearchParams(p); e.target.value === 'title' ? n.delete('sort') : n.set('sort', e.target.value); n.delete('desc'); return n }, { replace: true })}>
          <option value="title">Title</option><option value="rating">Rating</option><option value="year">Year</option><option value="added">Date added</option>
        </select>
        <button type="button" className="btn ghost square" onClick={() => setParam('desc', desc ? '0' : '1')} aria-label={desc ? 'Sorted high to low: switch to low to high' : 'Sorted low to high: switch to high to low'} title={desc ? 'High to low' : 'Low to high'}>{desc ? <IconSortDown /> : <IconSortUp />}</button>
        {!!facets?.genres.length && (
          <select className="select" aria-label="Genre" value={genre} onChange={(e) => setParam('genre', e.target.value)}>
            <option value="">All genres</option>
            {facets.genres.map((g) => <option key={g.value} value={g.value}>{g.value} ({g.count.toLocaleString()})</option>)}
          </select>
        )}
        {!!facets?.decades.length && (
          <select className="select" aria-label="Decade" value={decade} onChange={(e) => setParam('decade', e.target.value)}>
            <option value="">Any year</option>
            {facets.decades.map((g) => <option key={g.value} value={g.value}>{g.value}s ({g.count.toLocaleString()})</option>)}
          </select>
        )}
        {!!facets?.players.length && (
          <select className="select" aria-label="Players" value={players} onChange={(e) => setParam('players', e.target.value)}>
            <option value="">Any players</option>
            {[...new Set(facets.players.map((g) => Number(g.value)).filter((n) => n > 1))].map((n) => <option key={n} value={n}>{n} or more players</option>)}
          </select>
        )}
        {!!facets?.rated && (
          <select className="select" aria-label="Rating" value={score} onChange={(e) => setParam('score', e.target.value)}>
            <option value="">Any rating</option>
            {[9, 8, 7, 6].map((n) => <option key={n} value={n}>{n} and up</option>)}
          </select>
        )}
      </div>
      {!items ? <SkeletonGrid /> : items.length === 0
        ? (narrowed
          ? <EmptyState icon={<IconGamepad />} title="No games match" text={search ? `Nothing here matches “${search}”.` : 'No games here match those filters.'}
              action={<button className="btn" onClick={() => { setTyped(''); setParams({}, { replace: true }) }}>Show all games</button>} />
          : <EmptyState icon={<IconGamepad />} title="No games here yet" text="Put your own game files in this library’s folder and scan it. Zipped games and multi-disc sets work too." />)
        : (
          <>
            {inProgress.length > 0 && !filter && !search && <>
              <h2 className="shelf-head">Continue playing</h2>
              <div className="grid">{inProgress.map((it) => <GameCard key={it.id} item={it} console={byId.get(it.console || '')} />)}</div>
              <h2 className="shelf-head">All games</h2>
            </>}
            <div className="grid">{items.map((it) => <GameCard key={it.id} item={it} console={byId.get(it.console || '')} />)}</div>
            {hasMore && <div ref={more} className="gm-more"><Spinner /></div>}
          </>
        )}
    </>
  )
}

// A game's page.
export function GameDetails({ it, reload }: { it: Item; reload?: () => void }) {
  const nav = useNavigate()
  const { user, libraries } = useApp()
  const consoles = useConsoles()
  const c = consoles.find((x) => x.id === it.console)
  const [saves, setSaves] = useState<SaveSlot[]>([])
  const [meta, setMeta] = useState<GameDetailsInfo | null>(null)
  const [edit, setEdit] = useState(false)
  const [art, setArt] = useState(false)
  const load = () => api.get<SaveSlot[]>(`/games/${it.id}/saves`).then(setSaves).catch(() => {})
  useEffect(() => { load() }, [it.id])
  const loadMeta = () => api.get<GameDetailsInfo>(`/games/${it.id}/metadata`).then(setMeta).catch(() => {})
  useEffect(() => { loadMeta() }, [it.id, it.updatedAt]) // eslint-disable-line react-hooks/exhaustive-deps
  // Still being looked up: show what's found as it arrives.
  useEffect(() => {
    if (meta?.status !== 'pending') return
    const t = setTimeout(() => { loadMeta(); reload?.() }, 6000)
    return () => clearTimeout(t)
  }, [meta]) // eslint-disable-line react-hooks/exhaustive-deps
  const auto = saves.find((s) => s.slot === 0)
  const ready = c?.ready ?? false
  const remove = async (slot: number) => {
    try { await api.del(`/games/${it.id}/saves/${slot}`); load() } catch (e: any) { toast(e.message) }
  }
  const played = it.progress?.positionSec || 0
  const lib = libraries.find((l) => l.id === it.libraryId)
  const back = backdropUrl(it)
  const h = artHue(it.title)
  const sizeText = it.sizeBytes ? (it.sizeBytes >= 1e9 ? `${(it.sizeBytes / 1e9).toFixed(1)} GB` : `${Math.max(1, Math.round(it.sizeBytes / 1e6))} MB`) : ''
  return (
    <>
      <div className="detail-hero">
        <div className="bg" style={back ? { backgroundImage: `url(${back})` } : { background: `linear-gradient(155deg, hsl(${h} 32% 32%), hsl(${(h + 40) % 360} 38% 15%))` }} />
        <div className="shade" />
      </div>
      <div className="detail game-page">
        <div>
          <div className="poster game-poster">
            {it.poster ? <img src={posterUrl(it)} alt="" /> : <Placeholder title={it.title} label={c?.short} />}
          </div>
        </div>
        <div className="game-info">
          {lib && <div className="crumbs"><Link to={`/library/${it.libraryId}`}>{lib.name}</Link></div>}
          <h1>{it.title}</h1>
          {it.tagline && <div className="tagline">{it.tagline}</div>}
          <div className="meta">
            {c && <span>{c.name}</span>}
            {it.year && <span>{it.year}</span>}
            {meta && meta.players > 0 && <span>{meta.players === 1 ? '1 player' : `Up to ${meta.players} players`}</span>}
            {played > 60 && <span>Played for {fmtPlay(played)}</span>}
            {it.contentRating && <span className="pill">{it.contentRating}</span>}
            {it.rating != null && <span className="star"><IconStar /> {it.rating.toFixed(1)}</span>}
            {(it.genres || []).length > 0 && <span title={it.genres.join(', ')}>{it.genres.slice(0, 3).join(', ')}{it.genres.length > 3 ? '…' : ''}</span>}
          </div>
          <div className="actions game-actions">
            {auto && ready && <button className="btn primary" onClick={() => nav(`/game/${it.id}?resume=1`)}><IconPlay /> Resume</button>}
            {ready && <button className={'btn' + (auto ? '' : ' primary')} onClick={() => nav(`/game/${it.id}`)}><IconPlay /> {auto ? 'Start over' : 'Play'}</button>}
            {user?.isAdmin && <button className="btn ghost" onClick={() => setArt(true)}><IconImage /> Artwork</button>}
            {user?.isAdmin && <button className="btn ghost" onClick={() => setEdit(true)}><IconEdit /> Edit</button>}
            <Link className="btn" to={`/controllers?console=${encodeURIComponent(it.console || '')}`}><IconGamepad /> Controllers</Link>
          </div>
          {c && !ready && (
            <div className="game-note" role="status">
              {c.unavailable || <>This server has no core for the {c.name} yet.{' '}
                {user?.isAdmin ? <Link to="/settings/gaming"><IconSettings size={15} /> Install one in Settings</Link> : 'Ask whoever runs this server to install one.'}</>}
            </div>
          )}
          {it.overview && <p className="overview">{it.overview}</p>}
          {meta && <GameRatings d={meta} />}
          <div className="facts">
            {meta?.released && <div className="fact"><div className="k">Released</div><div className="v">{fmtReleased(meta.released)}</div></div>}
            {it.studio && <div className="fact"><div className="k">Developer</div><div className="v">{it.studio}</div></div>}
            {meta?.publisher && <div className="fact"><div className="k">Publisher</div><div className="v">{meta.publisher}</div></div>}
            {meta?.franchise && <div className="fact"><div className="k">Series</div><div className="v">{meta.franchise}</div></div>}
            {(it.genres || []).length > 3 && <div className="fact"><div className="k">Genres</div><div className="v">{it.genres.join(', ')}</div></div>}
            <div className="fact"><div className="k">File</div><div className="v">{[it.container?.toUpperCase(), sizeText].filter(Boolean).join(' · ')}</div></div>
            {meta?.tags && meta.tags !== it.tagline && <div className="fact"><div className="k">File name says</div><div className="v">{meta.tags}</div></div>}
            {user?.isAdmin && <div className="fact wide"><div className="k">Path</div><div className="v">{it.path}</div></div>}
          </div>
          {saves.length > 0 && (
            <section aria-label="Saved states">
              <h2 className="shelf-head">Saved states</h2>
              <ul className="game-saves">
                {saves.map((s) => {
                  const name = s.slot === 0 ? 'Automatic' : `Slot ${s.slot}`
                  return (
                    <li key={s.slot}>
                      {s.hasThumb ? <img src={mediaUrl(`/api/games/${it.id}/saves/${s.slot}/thumb`) + `&v=${s.updatedAt}`} alt="" /> : <span className="game-save-empty" />}
                      <div><b>{name}</b><small className="muted">{new Date(s.updatedAt * 1000).toLocaleString()}</small></div>
                      <button className="btn sm ghost" onClick={() => remove(s.slot)} aria-label={`Delete ${name}`}>Delete</button>
                    </li>
                  )
                })}
              </ul>
            </section>
          )}
          {meta && <GameShots d={meta} title={it.title} />}
          {user?.isAdmin && meta && <GameMatchBar it={it} d={meta} onChanged={() => { loadMeta(); reload?.() }} />}
          {meta && <GameCredits d={meta} />}
          <div className="page-back"><BackLink to={`/library/${it.libraryId}`} label="Back to library" /></div>
        </div>
      </div>
      {edit && <EditGame item={it} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); reload?.() }} />}
      {art && <ArtworkPicker base={`/items/${it.id}`} onClose={() => setArt(false)}
        onPick={async (kind, url) => { await api.put<Item>(`/items/${it.id}`, { [kind]: url }); reload?.(); toast(kind === 'poster' ? 'Cover updated' : 'Backdrop updated') }} />}
    </>
  )
}

// Someone's edits are theirs: sources leave what was typed here alone.
function EditGame({ item, onClose, onSaved }: { item: Item; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ title: item.title, year: item.year ? String(item.year) : '', overview: item.overview, genres: (item.genres || []).join(', '), contentRating: item.contentRating || '' })
  const [err, setErr] = useState('')
  const save = async () => {
    const body: Record<string, unknown> = {}
    if (f.title !== item.title) body.title = f.title
    if (f.year !== (item.year ? String(item.year) : '')) body.year = f.year ? Number(f.year) : null
    if (f.overview !== item.overview) body.overview = f.overview
    if (f.genres !== (item.genres || []).join(', ')) body.genres = f.genres.split(',').map((s) => s.trim()).filter(Boolean)
    if (f.contentRating !== (item.contentRating || '')) body.contentRating = f.contentRating
    try { await api.put<Item>(`/items/${item.id}`, body); onSaved() } catch (e: any) { setErr(e.message) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>Edit details</h2>
        <div className="form full">
          <div className="row2">
            <label>Title<input className="input" value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} /></label>
            <label>Year<input className="input" inputMode="numeric" value={f.year} onChange={(e) => setF({ ...f, year: e.target.value.replace(/\D/g, '').slice(0, 4) })} /></label>
          </div>
          <label>Summary<textarea className="input" rows={5} value={f.overview} onChange={(e) => setF({ ...f, overview: e.target.value })} /></label>
          <label>Genres (comma separated)<input className="input" value={f.genres} onChange={(e) => setF({ ...f, genres: e.target.value })} /></label>
          <label>Age rating <span className="muted">(decides what Kids and Guest profiles can see)</span>
            <input className="input" placeholder="e.g. ESRB T, PEGI 12, CERO B" value={f.contentRating} onChange={(e) => setF({ ...f, contentRating: e.target.value })} />
          </label>
          {err && <div className="error" role="alert">{err}</div>}
        </div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save}>Save</button></div>
      </div>
    </div>
  )
}

const fmtReleased = (d: string) => {
  const [y, m, day] = d.split('-').map(Number)
  if (!m) return String(y)
  return new Date(Date.UTC(y, m - 1, day || 1)).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: day ? 'numeric' : undefined, timeZone: 'UTC' })
}

const fmtPlay = (sec: number) => {
  const h = Math.floor(sec / 3600), m = Math.round((sec % 3600) / 60)
  return h > 0 ? `${h} h ${m} min` : `${m} min`
}

// The full-window page around the player, and the player's way to the API.
export function GamePage() {
  const { id } = useParams()
  const nav = useNavigate()
  const [params] = useSearchParams()
  const [it, setIt] = useState<Item | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { Promise.all([api.get<Item>(`/items/${id}`), getConsoles().then((l) => { for (const c of l) knownConsoles.set(c.id, c.short) }).catch(() => {})]).then(([i]) => setIt(i)).catch((e) => setErr(e.message)) }, [id])
  const host = useMemo<PlayHost | null>(() => {
    if (!it) return null
    const wsBase = () => {
      const base = serverBase() || window.location.origin
      return base.replace(/^http/, 'ws')
    }
    return {
      itemId: it.id, title: it.title, consoleId: it.console || '', resume: params.get('resume') === '1',
      prepare: () => api.post<PrepareStatus>(`/games/${it.id}/prepare`),
      start: (resume) => api.post<SessionInfo>(`/games/${it.id}/play`, { resume, client: 'browser' }),
      controls: (device) => api.get<any>(`/consoles/${it.console}/controls?device=${encodeURIComponent(device)}&game=${it.id}`),
      controllers: getControllers, profiles: profileHost, consoleName: consoleName(it.console || ''),
      saveControl: async (s) => { await api.put(`/consoles/${it.console}/controls`, s) },
      stop: async (sid) => { await api.del(`/play/${sid}`) },
      answer: async (sid, sdp, prefer) => (await api.post<{ sdp: string }>(`/play/${sid}/webrtc`, { sdp, prefer })).sdp,
      wsUrl: (sid, video) => {
        const tok = getToken()
        return `${wsBase()}/api/play/${sid}/ws?video=${video}` + (tok ? `&token=${encodeURIComponent(tok)}` : '')
      },
      action: async (sid, type, slot, on, extra) => { await api.post(`/play/${sid}/action`, { type, slot: slot ?? 0, on: !!on, ...extra }) },
      saves: () => api.get<SaveSlot[]>(`/games/${it.id}/saves`),
      thumbUrl: (slot, v) => mediaUrl(`/api/games/${it.id}/saves/${slot}/thumb`) + `&v=${v}`,
      close: () => nav(`/item/${it.id}`, { replace: true }),
    }
  }, [it])
  if (err) return <div className="error" role="alert">{err}</div>
  if (!host) return <Spinner />
  return <Suspense fallback={<Spinner />}><GamePlayer host={host} /></Suspense>
}

const consoleName = (id: string) => knownConsoles.get(id) || id.toUpperCase()
const knownConsoles = new Map<string, string>()

// The controller screen on a page of its own: the game menu has the same one.
export function ControllersScreen() {
  const nav = useNavigate()
  const [params, setParams] = useSearchParams()
  const consoles = useConsoles()
  const consoleId = params.get('console') || consoles.find((c) => c.ready)?.id || consoles[0]?.id || ''
  const controls = useMemo(() => (id: string, device: string) => api.get<ControlsData>(`/consoles/${encodeURIComponent(id)}/controls?device=${encodeURIComponent(device)}`), [])
  if (!consoleId) return <Spinner />
  return (
    <Suspense fallback={<Spinner />}>
      <Controllers profiles={profileHost} controls={controls} controllers={getControllers} consoles={consoles} consoleId={consoleId}
        onConsole={(id) => setParams({ console: id }, { replace: true })} saveControl={async (id, s) => { await api.put(`/consoles/${id}/controls`, s) }} onClose={() => nav(-1)} />
    </Suspense>
  )
}

// The console picker in the add-library flow and the library editor.
export function ConsolePicker({ value, onChange }: { value: string; onChange: (id: string, name: string) => void }) {
  const consoles = useConsoles()
  return (
    <label className="oobe-field">Console
      <select className="select" value={value} onChange={(e) => {
        const id = e.target.value
        onChange(id, id === 'auto' ? 'Games' : consoles.find((c) => c.id === id)?.short || id)
      }}>
        <option value="auto">Many consoles in one folder (sorted by folder and file type)</option>
        {consoles.map((c) => <option key={c.id} value={c.id}>{c.name}{c.unavailable ? ' (listed only, not playable yet)' : ''}</option>)}
      </select>
    </label>
  )
}

// Where BIOS files go, shown wherever someone is choosing a games folder. Nothing
// downloads one, so this is the only way people find out.
export function BiosHint({ console: id }: { console: string }) {
  const consoles = useConsoles()
  const c = consoles.find((x) => x.id === id)
  const required = c?.bios?.find((b) => b.required)
  return (
    <p className="muted game-bios-hint">
      {required
        ? <><b>{c!.name} games need a BIOS</b>: a file such as <code>{required.file}</code>, from a console you own.{' '}</>
        : <>Some consoles need a BIOS file from a console you own.{' '}</>}
      Put BIOS files in a folder named <code>BIOS</code> inside {c ? <>your {c.short} games folder</> : <>each console’s games folder</>}, such as <code>{c ? c.short : 'PS2'}/BIOS</code>.
      Folders inside it are fine. It’s never listed as a game, and the server picks up what it needs when you press Play.
    </p>
  )
}
