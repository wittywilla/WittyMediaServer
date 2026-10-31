import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, getToken, mediaUrl, posterUrl, serverBase, uploadFile, type GameConsole, type Item, type Library } from '../api'
import { BackLink, EmptyState, Placeholder, ScanButton, SkeletonGrid, Spinner } from '../components/common'
import { IconGamepad, IconPlay, IconSettings } from '../icons'
import { toast, useApp } from '../state'
import type { PlayHost, SaveSlot, SessionInfo, SettingsHost } from '@wittywilla/wittyplay'
import './games.css'

// Games: the library page, a game's own page, and the page that holds the player.
// The player itself is WittyPlay (modules/wittyplay/web), loaded only when a game
// is opened.

const GamePlayer = lazy(() => import('@wittywilla/wittyplay'))
const SettingsScreen = lazy(() => import('@wittywilla/wittyplay').then((m) => ({ default: m.GameSettingsScreen })))

// How the module's settings screens reach the API.
const useSettingsHost = (isAdmin: boolean) => useMemo<SettingsHost>(() => ({
  isAdmin,
  mediaUrl,
  upload: (path, body) => uploadFile(path, body),
  request: (method, path, body) => (method === 'GET' ? api.get(path) : method === 'DELETE' ? api.del(path) : method === 'PUT' ? api.put(path, body) : api.post(path, body)),
}), [isAdmin])

// Settings for all of someone's games, or a console's, on the settings page.
export function GameSettingsPage() {
  const { user } = useApp()
  const host = useSettingsHost(!!user?.isAdmin)
  const consoles = useConsoles()
  const say = useCallback((m: string) => toast(m), [])
  return <Suspense fallback={<Spinner />}><SettingsScreen host={host} consoles={consoles} onError={say} /></Suspense>
}

let consolesCache: Promise<GameConsole[]> | null = null
export const getConsoles = (fresh = false): Promise<GameConsole[]> => {
  if (!consolesCache || fresh) consolesCache = api.get<GameConsole[]>('/games/consoles')
  return consolesCache
}

export function useConsoles() {
  const [list, setList] = useState<GameConsole[]>([])
  useEffect(() => { getConsoles().then(setList).catch(() => {}) }, [])
  return list
}

function GameCard({ item, console: c }: { item: Item; console?: GameConsole }) {
  return (
    <Link to={`/item/${item.id}`} className="card" title={item.title}>
      <div className="poster">
        {item.poster ? <img src={posterUrl(item)} alt="" loading="lazy" /> : <Placeholder title={item.title} label={c?.short} />}
        <div className="play"><span><IconPlay /></span></div>
      </div>
      <div className="title">{item.title}</div>
      <div className="sub">{[c?.short, item.tagline].filter(Boolean).join(' · ')}</div>
    </Link>
  )
}

// A page at a time: a collection can run to thousands of games, and neither an old
// TV nor a phone should pay for all of them to look at the first few.
const PAGE = 120
const list = (ext: string, n: number) => `${n.toLocaleString()} ${ext}`
const and = (parts: string[]) => (parts.length < 2 ? parts.join('') : `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`)

export function GamesLibrary({ lib }: { lib: Library }) {
  const [consoles, setConsoles] = useState<GameConsole[]>([])
  const [params, setParams] = useSearchParams()
  const [items, setItems] = useState<Item[] | null>(null)
  const [total, setTotal] = useState(0)
  const [playing, setPlaying] = useState<Item[]>([])
  const [favourites, setFavourites] = useState<Item[]>([])
  const [skipped, setSkipped] = useState<Record<string, number>>({})
  const filter = params.get('console') || ''
  const search = params.get('q') || ''
  const [typed, setTyped] = useState(search)
  const url = (limit: number, offset = 0) => `/items?library=${lib.id}&kind=game&sort=title&console=${encodeURIComponent(filter)}&search=${encodeURIComponent(search)}&limit=${limit}&offset=${offset}`
  const loadedRef = useRef(0)
  loadedRef.current = items?.length ?? 0
  const load = () => api.get<{ items: Item[]; total: number }>(url(Math.max(PAGE, loadedRef.current)))
    .then((r) => { setItems(r.items); setTotal(r.total) }).catch(() => setItems([]))
  const loadConsoles = () => api.get<GameConsole[]>(`/games/consoles?library=${lib.id}`).then(setConsoles).catch(() => {})
  const [watch, setWatch] = useState(0) // bumped to follow a scan just started
  useEffect(() => { setItems(null); loadedRef.current = 0; load() }, [lib.id, filter, search]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { api.get<{ items: Item[] }>(`/items?library=${lib.id}&kind=game&played=1&limit=50`).then((r) => setPlaying(r.items)).catch(() => {}) }, [lib.id])
  useEffect(() => { api.get<{ item: Item }[]>(`/games/recent?favourites=1&limit=60`).then((r) => setFavourites(r.map((x) => x.item).filter((it) => it.libraryId === lib.id))).catch(() => {}) }, [lib.id])
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
  }, [hasMore, loaded, filter, search]) // eslint-disable-line react-hooks/exhaustive-deps
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
  const archives = left.some(([e]) => e === '.7z' || e === '.rar')
  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}</h1>
        {items && <span className="count">{total.toLocaleString()} {total === 1 ? 'game' : 'games'}</span>}
        <div className="grow" />
        <input className="input gm-search" type="search" placeholder="Search these games…" aria-label="Search these games" value={typed} onChange={(e) => setTyped(e.target.value)} />
        <ScanButton lib={lib} onStarted={() => setWatch((n) => n + 1)} />
      </div>
      {leftCount > 0 && (
        <div className="warn-box gm-skipped" role="status">
          <strong>{leftCount.toLocaleString()} {leftCount === 1 ? 'file' : 'files'} in this library’s folders didn’t become games</strong>
          {and(left.map(([e, c]) => list(e, c)))}.{' '}
          {archives ? '.7z and .rar archives can’t be read yet: unpack them, or zip them instead. ' : ''}
          Anything else is a kind of file, or a console, that isn’t supported yet.
        </div>
      )}
      {present.length > 1 && (
        <div className="chips" role="group" aria-label="Console">
          <button type="button" aria-pressed={!filter} className={'chip' + (!filter ? ' active' : '')} onClick={() => setParam('console', '')}>All · {everything.toLocaleString()}</button>
          {present.map((c) => <button key={c.id} type="button" aria-pressed={filter === c.id} className={'chip' + (filter === c.id ? ' active' : '')} onClick={() => setParam('console', c.id)}>{c.short} · {(c.count || 0).toLocaleString()}</button>)}
        </div>
      )}
      {!items ? <SkeletonGrid /> : items.length === 0
        ? (search || filter
          ? <EmptyState icon={<IconGamepad />} title="No games match" text={search ? `Nothing here matches “${search}”.` : 'There are none for this console.'}
              action={<button className="btn" onClick={() => { setTyped(''); setParams({}, { replace: true }) }}>Show all games</button>} />
          : <EmptyState icon={<IconGamepad />} title="No games here yet" text="Put your own game files in this library’s folder and scan it. Zipped games and multi-disc sets work too." />)
        : (
          <>
            {(inProgress.length > 0 || favourites.length > 0) && !filter && !search && <>
              {inProgress.length > 0 && <>
                <h2 className="shelf-head">Continue playing</h2>
                <div className="grid">{inProgress.map((it) => <GameCard key={it.id} item={it} console={byId.get(it.console || '')} />)}</div>
              </>}
              {favourites.length > 0 && <>
                <h2 className="shelf-head">Favourites</h2>
                <div className="grid">{favourites.map((it) => <GameCard key={it.id} item={it} console={byId.get(it.console || '')} />)}</div>
              </>}
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
export function GameDetails({ it }: { it: Item }) {
  const nav = useNavigate()
  const { user, libraries } = useApp()
  const consoles = useConsoles()
  const c = consoles.find((x) => x.id === it.console)
  const [saves, setSaves] = useState<SaveSlot[]>([])
  const load = () => api.get<SaveSlot[]>(`/games/${it.id}/saves`).then(setSaves).catch(() => {})
  useEffect(() => { load() }, [it.id])
  const auto = saves.find((s) => s.slot === 0)
  const ready = c?.ready ?? false
  const remove = async (slot: number) => {
    try { await api.del(`/games/${it.id}/saves/${slot}`); load() } catch (e: any) { toast(e.message) }
  }
  const played = it.progress?.positionSec || 0
  const [meta, setMeta] = useState<GameMeta | null>(null)
  const [resumeMode, setResumeMode] = useState('ask')
  const [showSettings, setShowSettings] = useState(false)
  const [note, setNote] = useState('')
  useEffect(() => {
    api.get<GameMeta>(`/games/${it.id}/meta`).then((m) => { setMeta(m); setNote(m.note) }).catch(() => {})
    api.get<{ resolved: { values: Record<string, unknown> } }>(`/games/settings?item=${it.id}`).then((v) => setResumeMode(String(v.resolved.values['states.resume'] || 'ask'))).catch(() => {})
  }, [it.id])
  const putMeta = async (patch: Partial<GameMeta>) => {
    try { setMeta(await api.put<GameMeta>(`/games/${it.id}/meta`, patch)) } catch (e: any) { toast(e.message) }
  }
  const settingsHost = useSettingsHost(!!user?.isAdmin)
  const say = useCallback((m: string) => toast(m), [])
  const consoleList = useMemo(() => consoles.map((x) => ({ id: x.id, name: x.name, short: x.short })), [consoles])
  // What Play does, when someone said they always or never want to pick up where they left off.
  const startsResumed = !!auto && resumeMode === 'always'
  const offerResume = !!auto && resumeMode === 'ask'
  const battery = async () => {
    const f = await pickSaveFile('.srm,.sav,.ram')
    if (!f) return
    try { await uploadFile(`/games/${it.id}/battery`, f, 'PUT'); toast('Battery save replaced. The old one is kept beside it on the server.') } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="game-page">
      <BackLink to={`/library/${it.libraryId}`} label={libraries.find((l) => l.id === it.libraryId)?.name || 'Library'} />
      <div className="game-head">
        <div className="poster game-poster">
          {it.poster ? <img src={posterUrl(it)} alt="" /> : <Placeholder title={it.title} label={c?.short} />}
        </div>
        <div className="game-info">
          <h1>{it.title}</h1>
          <p className="muted">{[c?.name, it.tagline, it.year, played > 60 ? `Played for ${fmtPlay(played)}` : '', meta && meta.plays > 0 ? `${meta.plays} ${meta.plays === 1 ? 'play' : 'plays'}` : '', meta && meta.lastPlayed > 0 ? `Last played ${new Date(meta.lastPlayed * 1000).toLocaleDateString()}` : ''].filter(Boolean).join(' · ')}</p>
          {it.overview && <p>{it.overview}</p>}
          {offerResume && ready && auto && (
            <button className="game-resume" onClick={() => nav(`/game/${it.id}?resume=1`)}>
              {auto.hasThumb ? <img src={thumbUrl(it.id, 0, auto.updatedAt)} alt="" /> : <span className="game-save-empty" />}
              <span><b>Pick up where you left off</b><small className="muted">{new Date(auto.updatedAt * 1000).toLocaleString()}</small></span>
            </button>
          )}
          <div className="game-actions">
            {offerResume && ready && <button className="btn primary" onClick={() => nav(`/game/${it.id}?resume=1`)}><IconPlay /> Resume</button>}
            {ready && <button className={'btn' + (offerResume ? '' : ' primary')} onClick={() => nav(`/game/${it.id}${startsResumed ? '?resume=1' : ''}`)}><IconPlay /> {offerResume ? 'Start over' : startsResumed ? 'Resume' : 'Play'}</button>}
            {meta && <button className="btn" aria-pressed={meta.favourite} onClick={() => putMeta({ favourite: !meta.favourite })}>{meta.favourite ? '★ Favourite' : '☆ Add to favourites'}</button>}
            <button className="btn" aria-expanded={showSettings} onClick={() => setShowSettings((s) => !s)}><IconSettings size={15} /> Settings</button>
            {ready && <button className="btn ghost" onClick={() => nav(`/game/${it.id}?safe=1${auto ? '&resume=1' : ''}`)}>Start with default settings</button>}
          </div>
          {meta && (
            <label className="game-notes">Your notes
              <textarea className="input" rows={2} maxLength={8000} value={note} onChange={(e) => setNote(e.target.value)} onBlur={() => { if (note !== meta.note) void putMeta({ note }) }} placeholder="Passwords, where you got to, anything. Only you can see these." />
            </label>
          )}
          {c && !ready && (
            <div className="game-note" role="status">
              {c.unavailable || <>This server has no core for the {c.name} yet.{' '}
                {user?.isAdmin ? <Link to="/settings/gaming"><IconSettings size={15} /> Install one in Settings</Link> : 'Ask whoever runs this server to install one.'}</>}
            </div>
          )}
        </div>
      </div>
      {showSettings && (
        <section aria-label="Settings for this game">
          <h2 className="shelf-head">Settings for this game</h2>
          <Suspense fallback={<Spinner />}><SettingsScreen host={settingsHost} consoles={consoleList} itemId={it.id} itemTitle={it.title} consoleId={it.console} onError={say} /></Suspense>
        </section>
      )}
      <section aria-label="Saves">
        <div className="game-save-tools">
          <a className="btn sm ghost" href={mediaUrl(`/api/games/${it.id}/battery`)} download>Download battery save</a>
          <button className="btn sm ghost" onClick={() => void battery()}>Replace battery save…</button>
        </div>
      </section>
      {saves.length > 0 && (
        <section aria-label="Saved states">
          <h2 className="shelf-head">Saved states</h2>
          <ul className="game-saves">
            {saves.map((s) => {
              const name = s.slot === 0 ? 'Automatic' : s.slot === 100 ? 'Before the last load' : s.slot === 101 ? 'Before starting over' : `Slot ${s.slot}`
              return (
                <li key={s.slot}>
                  {s.hasThumb ? <img src={thumbUrl(it.id, s.slot, s.updatedAt)} alt="" /> : <span className="game-save-empty" />}
                  <div><b>{name}</b><small className="muted">{new Date(s.updatedAt * 1000).toLocaleString()}</small></div>
                  <a className="btn sm ghost" href={mediaUrl(`/api/games/${it.id}/saves/${s.slot}/file`)} download aria-label={`Download ${name}`}>Download</a>
                  <button className="btn sm ghost" onClick={() => remove(s.slot)} aria-label={`Delete ${name}`}>Delete</button>
                </li>
              )
            })}
          </ul>
        </section>
      )}
    </div>
  )
}

type GameMeta = { favourite: boolean; note: string; plays: number; lastPlayed: number; cheats: unknown[] }

const thumbUrl = (id: number, slot: number, v: number) => {
  const u = mediaUrl(`/api/games/${id}/saves/${slot}/thumb`)
  return u + (u.includes('?') ? '&' : '?') + `v=${v}`
}

const pickSaveFile = (accept: string) => new Promise<File | null>((resolve) => {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = accept
  input.onchange = () => resolve(input.files?.[0] || null)
  input.oncancel = () => resolve(null)
  input.click()
})

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
  const [consoles, setConsoles] = useState<GameConsole[]>([])
  const { user } = useApp()
  const settingsHost = useSettingsHost(!!user?.isAdmin)
  useEffect(() => { api.get<Item>(`/items/${id}`).then(setIt).catch((e) => setErr(e.message)) }, [id])
  useEffect(() => { getConsoles().then(setConsoles).catch(() => {}) }, [])
  const short = consoles.find((c) => c.id === it?.console)?.short || ''
  const host = useMemo<PlayHost | null>(() => {
    if (!it) return null
    const wsBase = () => {
      const base = serverBase() || window.location.origin
      return base.replace(/^http/, 'ws')
    }
    return {
      ...settingsHost,
      itemId: it.id, title: it.title, consoleId: it.console || '', consoleName: short, resume: params.get('resume') === '1', safeMode: params.get('safe') === '1',
      start: (resume, safeMode) => api.post<SessionInfo>(`/games/${it.id}/play`, { resume, safeMode, client: 'browser' }),
      relaunch: ({ resume, safeMode }) => {
        const q = [resume ? 'resume=1' : '', safeMode ? 'safe=1' : ''].filter(Boolean).join('&')
        window.location.assign(window.location.pathname + (q ? '?' + q : ''))
      },
      controls: (device) => api.get<any>(`/consoles/${it.console}/controls?device=${encodeURIComponent(device)}`),
      saveControl: async (s) => { await api.put(`/consoles/${it.console}/controls`, s) },
      stop: async (sid) => { await api.del(`/play/${sid}`) },
      answer: async (sid, sdp, prefer) => (await api.post<{ sdp: string }>(`/play/${sid}/webrtc`, { sdp, prefer })).sdp,
      wsUrl: (sid, video) => {
        const tok = getToken()
        return `${wsBase()}/api/play/${sid}/ws?video=${video}` + (tok ? `&token=${encodeURIComponent(tok)}` : '')
      },
      action: (sid, type, slot, on, extra) => api.post(`/play/${sid}/action`, { type, slot: slot ?? 0, on: !!on, ...extra }),
      close: () => nav(`/item/${it.id}`, { replace: true }),
    }
  }, [it, short, settingsHost])
  if (err) return <div className="error" role="alert">{err}</div>
  if (!host) return <Spinner />
  return <Suspense fallback={<Spinner />}><GamePlayer host={host} /></Suspense>
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
