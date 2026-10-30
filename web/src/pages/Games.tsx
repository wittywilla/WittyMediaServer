import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, getToken, mediaUrl, posterUrl, serverBase, type GameConsole, type Item, type Library } from '../api'
import { BackLink, EmptyState, Placeholder, Spinner } from '../components/common'
import { IconPlay, IconRefresh, IconSettings } from '../icons'
import { toast, useApp } from '../state'
import type { PlayHost, SaveSlot, SessionInfo } from '@wittywilla/wittyplay'
import './games.css'

// Games: the library page, a game's own page, and the page that holds the player.
// The player itself is WittyPlay (modules/wittyplay/web), loaded only when a game
// is opened.

const GamePlayer = lazy(() => import('@wittywilla/wittyplay'))

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
        {item.poster ? <img src={posterUrl(item)} alt="" loading="lazy" /> : <Placeholder title={item.title} sub={c?.short} />}
        <div className="play"><span><IconPlay /></span></div>
      </div>
      <div className="title">{item.title}</div>
      <div className="sub">{[c?.short, item.tagline].filter(Boolean).join(' · ')}</div>
    </Link>
  )
}

export function GamesLibrary({ lib }: { lib: Library }) {
  const { user } = useApp()
  const consoles = useConsoles()
  const [params, setParams] = useSearchParams()
  const [items, setItems] = useState<Item[] | null>(null)
  const filter = params.get('console') || ''
  const load = () => api.get<{ items: Item[] }>(`/items?library=${lib.id}&kind=game&sort=title&limit=2000`).then((r) => setItems(r.items)).catch(() => setItems([]))
  const [watch, setWatch] = useState(0) // bumped to follow a scan just started
  useEffect(() => { setItems(null); load() }, [lib.id])
  // A scan may already be running (a library just added), or just started. Follow
  // it, so games appear as they're found and the page ends up complete.
  useEffect(() => {
    let stop = false
    let timer = 0
    const tick = async () => {
      const r = await api.get<{ scanning: boolean }>(`/libraries/${lib.id}/scan`).catch(() => ({ scanning: false }))
      if (stop) return
      await load()
      if (r.scanning && !stop) timer = window.setTimeout(tick, 2000)
    }
    void tick()
    return () => { stop = true; clearTimeout(timer) }
  }, [lib.id, watch])
  const byId = useMemo(() => new Map(consoles.map((c) => [c.id, c])), [consoles])
  const present = useMemo(() => [...new Set((items || []).map((i) => i.console || ''))].filter(Boolean), [items])
  const shown = (items || []).filter((i) => !filter || i.console === filter)
  const playing = (items || []).filter((i) => i.progress && i.progress.positionSec > 60).sort((a, b) => (b.progress!.updatedAt) - (a.progress!.updatedAt)).slice(0, 8)
  const scan = async () => {
    try { await api.post(`/libraries/${lib.id}/scan`); toast(`Scanning ${lib.name}`); setWatch((n) => n + 1) } catch (e: any) { toast(e.message) }
  }
  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}{items && <span className="muted"> · {shown.length}</span>}</h1>
        {present.length > 1 && (
          <div className="seg" role="radiogroup" aria-label="Console">
            <button type="button" role="radio" aria-checked={!filter} className={!filter ? 'active' : ''} onClick={() => setParams({})}>All</button>
            {present.map((c) => <button key={c} type="button" role="radio" aria-checked={filter === c} className={filter === c ? 'active' : ''} onClick={() => setParams({ console: c })}>{byId.get(c)?.short || c}</button>)}
          </div>
        )}
        <span className="grow" />
        {user?.isAdmin && <button className="btn ghost" onClick={scan}><IconRefresh /> Scan</button>}
      </div>
      {!items ? <Spinner /> : items.length === 0
        ? <EmptyState title="No games here yet" text="Put your own game files in this library’s folder and scan it. Zipped games and multi-disc sets work too." />
        : (
          <>
            {playing.length > 0 && !filter && <>
              <h2 className="shelf-head">Continue playing</h2>
              <div className="grid">{playing.map((it) => <GameCard key={it.id} item={it} console={byId.get(it.console || '')} />)}</div>
              <h2 className="shelf-head">All games</h2>
            </>}
            <div className="grid">{shown.map((it) => <GameCard key={it.id} item={it} console={byId.get(it.console || '')} />)}</div>
          </>
        )}
    </>
  )
}

// A game's page.
export function GameDetails({ it }: { it: Item }) {
  const nav = useNavigate()
  const { user } = useApp()
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
  return (
    <div className="game-page">
      <BackLink to={`/library/${it.libraryId}`} label="Back to the library" />
      <div className="game-head">
        <div className="poster game-poster">
          {it.poster ? <img src={posterUrl(it)} alt="" /> : <Placeholder title={it.title} sub={c?.short} />}
        </div>
        <div className="game-info">
          <h1>{it.title}</h1>
          <p className="muted">{[c?.name, it.tagline, it.year, played > 60 ? `Played for ${fmtPlay(played)}` : ''].filter(Boolean).join(' · ')}</p>
          {it.overview && <p>{it.overview}</p>}
          <div className="game-actions">
            {auto && ready && <button className="btn primary" onClick={() => nav(`/game/${it.id}?resume=1`)}><IconPlay /> Resume</button>}
            {ready && <button className={'btn' + (auto ? '' : ' primary')} onClick={() => nav(`/game/${it.id}`)}><IconPlay /> {auto ? 'Start over' : 'Play'}</button>}
          </div>
          {c && !ready && (
            <div className="game-note" role="status">
              {c.unavailable || <>This server has no core for the {c.name} yet.{' '}
                {user?.isAdmin ? <Link to="/settings/gaming"><IconSettings size={15} /> Install one in Settings</Link> : 'Ask whoever runs this server to install one.'}</>}
            </div>
          )}
        </div>
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
    </div>
  )
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
  useEffect(() => { api.get<Item>(`/items/${id}`).then(setIt).catch((e) => setErr(e.message)) }, [id])
  const host = useMemo<PlayHost | null>(() => {
    if (!it) return null
    const wsBase = () => {
      const base = serverBase() || window.location.origin
      return base.replace(/^http/, 'ws')
    }
    return {
      itemId: it.id, title: it.title, consoleId: it.console || '', resume: params.get('resume') === '1',
      start: (resume) => api.post<SessionInfo>(`/games/${it.id}/play`, { resume, client: 'browser' }),
      controls: (device) => api.get<any>(`/consoles/${it.console}/controls?device=${encodeURIComponent(device)}`),
      saveControl: async (s) => { await api.put(`/consoles/${it.console}/controls`, s) },
      stop: async (sid) => { await api.del(`/play/${sid}`) },
      answer: async (sid, sdp, prefer) => (await api.post<{ sdp: string }>(`/play/${sid}/webrtc`, { sdp, prefer })).sdp,
      wsUrl: (sid, video) => {
        const tok = getToken()
        return `${wsBase()}/api/play/${sid}/ws?video=${video}` + (tok ? `&token=${encodeURIComponent(tok)}` : '')
      },
      action: async (sid, type, slot, on) => { await api.post(`/play/${sid}/action`, { type, slot: slot ?? 0, on: !!on }) },
      saves: () => api.get<SaveSlot[]>(`/games/${it.id}/saves`),
      thumbUrl: (slot, v) => mediaUrl(`/api/games/${it.id}/saves/${slot}/thumb`) + `&v=${v}`,
      close: () => nav(`/item/${it.id}`, { replace: true }),
    }
  }, [it])
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
