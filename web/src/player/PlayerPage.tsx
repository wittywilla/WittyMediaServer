import { useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, dvdStartMode, getPrefs, isDisc, type Item, type Prefs } from '../api'
import { IconClose } from '../icons'
import { PlayerChromeContext, usePlayerChrome, type PlayerChrome } from './chrome'
import DvdPlayer from './DvdPlayer'
import VideoPlayer from './VideoPlayer'

type Playing = { id: string; search: string }

// PlayerDock hosts the one player of the app. On /play/:id it fills the
// screen; navigating elsewhere with "back" keeps the same player (and its
// stream and disc session) running as a mini player in the corner. It is
// rendered at a fixed spot in the app tree so React never remounts it when
// the page around it changes.
export function PlayerDock() {
  const loc = useLocation()
  const nav = useNavigate()
  const match = loc.pathname.match(/^\/play\/([^/]+)/)
  const [session, setSession] = useState<Playing | null>(null)
  const fromUrl = match ? { id: match[1], search: loc.search } : null
  useEffect(() => {
    if (fromUrl) setSession((s) => (s && s.id === fromUrl.id && s.search === fromUrl.search ? s : fromUrl))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fromUrl?.id, fromUrl?.search])
  const cur = fromUrl || session
  const full = !!fromUrl
  const chrome = useMemo<PlayerChrome | null>(() => {
    if (!cur) return null
    const leave = () => {
      // Back to where the viewer came from when that is inside the app.
      if ((window.history.state?.idx ?? 0) > 0) nav(-1)
      else nav(`/item/${cur.id}`, { replace: true })
    }
    return {
      mini: !full,
      minimize: () => { if (full) leave() },
      expand: () => { if (!full) nav(`/play/${cur.id}${cur.search}`) },
      close: () => { setSession(null); if (full) leave() },
    }
  }, [cur?.id, cur?.search, full, nav]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!cur || !chrome) return null
  return (
    <PlayerChromeContext.Provider value={chrome}>
      <PlayerPage key={cur.id + cur.search} id={cur.id} search={cur.search} />
    </PlayerChromeContext.Provider>
  )
}

function PlayerPage({ id, search }: { id: string; search: string }) {
  const params = useMemo(() => new URLSearchParams(search), [search])
  const [item, setItem] = useState<Item | null>(null)
  const [err, setErr] = useState('')
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  // Playing a list through: the server says what comes next in it.
  const listId = params.get('collection')
  useEffect(() => { api.get<Item>(`/items/${id}${listId ? `?collection=${encodeURIComponent(listId)}` : ''}`).then(setItem).catch((e) => setErr(e.message)) }, [id, listId])
  useEffect(() => { getPrefs().then(setPrefs) }, [])
  if (err) return <div className="player"><div className="overlay-msg"><div className="box">{err}<br /><BackButton /></div></div></div>
  if (!item || !prefs) return <div className="player"><div className="center-spinner"><div className="spinner" /></div></div>
  const startParam = params.get('start')
  const startAt = startParam != null ? Number(startParam) : null
  if (isDisc(item)) {
    // No explicit mode: the item's start setting (menu by default).
    const m = params.get('mode')
    const mode = m === 'menu' || m === 'title' ? m : params.get('ttn') ? 'title' : dvdStartMode(item, prefs)
    const ttn = params.get('ttn') ? Number(params.get('ttn')) : undefined
    const chapter = params.get('chapter') ? Number(params.get('chapter')) : undefined
    const menu = params.get('menu') || undefined
    const menuVts = params.get('menuVts') ? Number(params.get('menuVts')) : undefined
    return <DvdPlayer item={item} mode={mode} ttn={ttn} chapter={chapter} startAt={startAt} insert={params.get('insert') === '1'} menu={menu} menuVts={menuVts} />
  }
  return <VideoPlayer item={item} startAt={startAt} />
}

function BackButton() {
  const { close } = usePlayerChrome()
  return <button className="btn" style={{ marginTop: 10 }} onClick={close}><IconClose /> Close</button>
}
