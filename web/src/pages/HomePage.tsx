import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { albumsOnce, api, ApiError, backdropUrl, discName, fmtRuntime, isDisc, posterUrl, type Home, type Hub, type Show, type Item, inMainMenus } from '../api'
import AddLibrary from '../components/AddLibrary'
import { CardRow, ItemCard, ShowCard, Spinner } from '../components/common'
import { IconDown, IconGrip, IconInfo, IconPause, IconPlay, IconPlus, IconRefresh, IconRight, IconStar, IconUp, libIcon } from '../icons'
import { toast, useApp } from '../state'
import { useOpenItem } from '../openItem'

// The home page is a column of widgets. Everyone can reorder and hide them
// ("Customize"); the layout is saved to the account, so it follows the
// person to every device. Reordering works by dragging, and with Up/Down
// buttons for touch screens, TV remotes and keyboards.

// v: the layout version. Version 2 merged the "Continue" rows into "Jump back
// in"; an older saved order is dropped once (hidden sections are kept).
type Layout = { v?: number; order: string[]; hidden: string[] }
type Widget = { id: string; title: string; render: () => React.ReactNode | null }
const LAYOUT_V = 2

export default function HomePage() {
  const [home, setHome] = useState<Home | null>(null)
  const [err, setErr] = useState<{ offline: boolean; message: string } | null>(null)
  const [layout, setLayout] = useState<Layout>({ v: LAYOUT_V, order: [], hidden: [] })
  const [editing, setEditing] = useState(false)
  const [dragId, setDragId] = useState<string | null>(null)
  const { libraries, user, refreshLibraries } = useApp()
  // The problem card stays up while retrying, so the page doesn't flicker.
  const load = useCallback(() => {
    api.get<Home>('/home').then((h) => { setHome(h); setErr(null) }).catch((e) => setErr({ offline: e instanceof ApiError && e.status === 0, message: e.message }))
  }, [])
  useEffect(load, [libraries, load])
  // Offline: try again every few seconds, so the page comes back by itself
  // when the server does.
  useEffect(() => {
    if (!err?.offline) return
    const t = setTimeout(() => { refreshLibraries(); load() }, 5000)
    return () => clearTimeout(t)
  }, [err, load, refreshLibraries])
  useEffect(() => {
    api.get<{ home?: Layout }>('/me/ui').then((u) => {
      if (!u.home) return
      const hidden = (u.home.hidden || []).map((h) => (h.startsWith('continue') ? 'jump' : h))
      setLayout({ v: LAYOUT_V, order: u.home.v === LAYOUT_V ? u.home.order || [] : [], hidden: [...new Set(hidden)] })
    }).catch(() => {})
  }, [])

  // Recommendations from this profile's own record on this server.
  const [forYou, setForYou] = useState<{ items: Item[]; shows: Show[]; because: string } | null>(null)
  useEffect(() => { api.get<{ items: Item[]; shows: Show[]; because: string }>('/me/recommended').then(setForYou).catch(() => {}) }, [])

  const widgets = useMemo<Widget[]>(() => {
    if (!home) return []
    const hub = (h: Hub): Widget => ({ id: h.key, title: h.title, render: () => <HubRow hub={h} /> })
    const byKey = (k: string) => home.hubs.find((h) => h.key === k)
    // Everything in progress, watching, reading and listening, in one row.
    const going = ['continue', 'continue-reading', 'continue-listening'].map(byKey).filter(Boolean) as Hub[]
    const jump: Hub = { key: 'jump', title: 'Jump back in', items: going.flatMap((h) => h.items), shows: going.flatMap((h) => h.shows || []) }
    const list: Widget[] = [
      { id: 'hero', title: 'Featured', render: () => (home.featured.length > 0 ? <Hero items={home.featured} /> : null) },
      { id: 'libraries', title: 'Your libraries', render: () => <LibraryTiles /> },
      { id: 'jump', title: 'Jump back in', render: () => <HubRow hub={jump} /> },
      { id: 'foryou', title: 'Recommended for you', render: () => (forYou ? <HubRow hub={{ key: 'foryou', title: forYou.because ? `Because you watch ${forYou.because}` : 'Recommended for you', items: forYou.items || [], shows: forYou.shows || [] }} /> : null) },
    ]
    const next = byKey('nextup')
    if (next) list.push(hub(next))
    for (const h of home.hubs) if (h.key.startsWith('recent-')) list.push(hub(h))
    list.push({ id: 'shuffle', title: 'Can’t decide?', render: () => <Shuffle /> })
    return list
  }, [forYou, home])

  // Saved order first, then any widget the person has not placed yet.
  const ordered = useMemo(() => {
    const pos = (id: string) => { const i = layout.order.indexOf(id); return i < 0 ? 1000 + widgets.findIndex((w) => w.id === id) : i }
    return [...widgets].sort((a, b) => pos(a.id) - pos(b.id))
  }, [widgets, layout.order])

  const save = (next: Layout) => {
    const l = { ...next, v: LAYOUT_V }
    setLayout(l)
    api.put('/me/ui', { home: l }).catch(() => toast('Could not save the layout'))
  }
  const move = (id: string, to: number) => {
    const ids = ordered.map((w) => w.id).filter((x) => x !== id)
    ids.splice(Math.max(0, Math.min(ids.length, to)), 0, id)
    save({ ...layout, order: ids })
  }
  const toggle = (id: string) => save({ ...layout, hidden: layout.hidden.includes(id) ? layout.hidden.filter((x) => x !== id) : [...layout.hidden, id] })

  if (err) return <HomeProblem offline={err.offline} message={err.message} retry={() => { refreshLibraries(); load() }} />
  if (!home) return <Spinner />
  if (home.libraries.length === 0) return <Welcome admin={!!user?.isAdmin} name={user?.name || ''} onAdded={refreshLibraries} />
  const shown = editing ? ordered : ordered.filter((w) => !layout.hidden.includes(w.id))
  const scanning = libraries.some((l) => l.scanning)
  const empty = home.featured.length === 0 && home.hubs.every((h) => !h.items.length && !(h.shows || []).length)
  return (
    <div className={'home' + (editing ? ' editing' : '')}>
      {editing && (
        <div className="home-tools">
          <span className="muted">Drag sections, or use the arrows. Hidden sections stay here until you show them again.</span>
          <button className="btn sm ghost" onClick={() => save({ order: [], hidden: [] })}>Reset</button>
          <button className="btn sm primary" onClick={() => setEditing(false)}>Done</button>
        </div>
      )}
      {!editing && (home.featured.length === 0 || layout.hidden.includes('hero')) && <Greeting name={user?.name || ''} />}
      {!editing && empty && (
        <div className="home-card">
          <span className="home-card-ic"><IconRefresh /></span>
          <h2>{scanning ? 'Your libraries are being scanned' : 'Your libraries are empty'}</h2>
          <p className="muted">{scanning
            ? 'Films, shows, music and books appear here as they’re found. The first scan can take a few minutes.'
            : 'Nothing was found in their folders yet. Check the folders in Settings, Libraries, then scan again.'}</p>
          {user?.isAdmin && !scanning && <Link className="btn" to="/settings/libraries">Open library settings</Link>}
        </div>
      )}
      {shown.map((w, i) => {
        const body = w.render()
        if (!editing) return body ? <div key={w.id} className="widget">{body}</div> : null
        const hidden = layout.hidden.includes(w.id)
        return (
          <div key={w.id} className={'widget edit' + (hidden ? ' off' : '') + (dragId === w.id ? ' dragging' : '')} draggable
            onDragStart={(e) => { setDragId(w.id); e.dataTransfer.effectAllowed = 'move' }} onDragEnd={() => setDragId(null)}
            onDragOver={(e) => { if (dragId && dragId !== w.id) e.preventDefault() }}
            onDrop={(e) => { e.preventDefault(); if (dragId) move(dragId, i) }}>
            <div className="widget-bar">
              <span className="grip" aria-hidden="true"><IconGrip size={18} /></span>
              <b>{w.title}</b>
              <span className="grow" />
              <button className="btn sm ghost square" disabled={i === 0} onClick={() => move(w.id, i - 1)} aria-label={`Move ${w.title} up`}><IconUp /></button>
              <button className="btn sm ghost square" disabled={i === shown.length - 1} onClick={() => move(w.id, i + 1)} aria-label={`Move ${w.title} down`}><IconDown /></button>
              <button className="btn sm" onClick={() => toggle(w.id)}>{hidden ? 'Show' : 'Hide'}</button>
            </div>
            {!hidden && <div className="widget-preview">{body || <div className="muted">Nothing to show here right now.</div>}</div>}
          </div>
        )
      })}
      {/* A page action, not a heading: at the foot of the page. */}
      {!editing && (
        <div className="home-foot">
          <button className="btn sm ghost" onClick={() => { setEditing(true); window.scrollTo({ top: 0, behavior: 'smooth' }) }} title="Rearrange or hide sections of the home page">Customize home</button>
        </div>
      )}
    </div>
  )
}

const greeting = () => { const h = new Date().getHours(); return h >= 5 && h < 12 ? 'Good morning' : h >= 12 && h < 18 ? 'Good afternoon' : 'Good evening' }

function Greeting({ name }: { name: string }) {
  return <div className="home-greet"><h1>{greeting()}{name ? `, ${name}` : ''}</h1><p className="muted">Here’s what’s on your server.</p></div>
}

// Welcome: the first visit, before any library exists.
function Welcome({ admin, name, onAdded }: { admin: boolean; name: string; onAdded: () => void }) {
  return (
    <div className="home-welcome">
      <div className="home-welcome-head">
        <span className="home-card-ic big">{libIcon('movies')}</span>
        <h1>Welcome{name ? `, ${name}` : ''}! Let’s fill your server.</h1>
        <p className="muted">{admin
          ? 'Choose what kind of media you have, then point to the folder it’s in. Your files stay where they are; nothing is moved or changed. You can add more libraries any time.'
          : 'There’s nothing to watch, read or listen to yet. Whoever runs this server can add libraries in Settings, and they’ll appear here.'}</p>
      </div>
      {admin && <AddLibrary onAdded={onAdded} />}
    </div>
  )
}

// HomeProblem: the server can't be reached, or the home page failed to load.
function HomeProblem({ offline, message, retry }: { offline: boolean; message: string; retry: () => void }) {
  return (
    <div className="home-card problem" role="alert">
      <span className="home-card-ic"><IconInfo /></span>
      <h2>{offline ? 'Can’t reach your server' : 'Home didn’t load'}</h2>
      <p className="muted">{offline
        ? 'Check that WittyMediaServer is running on its computer and that this device is on the same network. This page tries again by itself every few seconds.'
        : message}</p>
      <button className="btn primary" onClick={retry}><IconRefresh /> Try again</button>
    </div>
  )
}

// Pictures and creator videos are wide; a poster's shape would crop them.
const WIDE_LIBRARIES = ['photos', 'channels', 'musicvideos']

function HubRow({ hub }: { hub: Hub }) {
  const { libraries } = useApp()
  if (!hub.items.length && !(hub.shows || []).length) return null
  const wideRow = WIDE_LIBRARIES.includes(libraries.find((l) => l.id === hub.libraryId)?.type || '')
  return (
    <section className="hub">
      <div className="hub-head">
        <h2>{hub.title}</h2>
        {hub.libraryId && <Link to={`/library/${hub.libraryId}`}>See all <IconRight /></Link>}
      </div>
      <CardRow>
        {albumsOnce(hub.items).map((it) => (wideRow ? <ItemCard key={it.id} item={it} wide /> : <ItemCard key={it.id} item={it} />))}
        {(hub.shows || []).map((s) => <ShowCard key={'s' + s.id} show={s} />)}
      </CardRow>
    </section>
  )
}

// LibraryTiles: a way into every library, with an Add tile for admins.
const NOUNS: Record<string, [string, string]> = {
  movies: ['title', 'titles'], discs: ['disc', 'discs'], videos: ['video', 'videos'], commercials: ['commercial', 'commercials'], shows: ['episode', 'episodes'], anime: ['episode', 'episodes'],
  music: ['track', 'tracks'], audiobooks: ['file', 'files'], podcasts: ['episode', 'episodes'], books: ['book', 'books'],
  photos: ['picture', 'pictures'], shorts: ['video', 'videos'], channels: ['video', 'videos'], musicvideos: ['video', 'videos'], radio: ['station', 'stations'],
}
function LibraryTiles() {
  const { libraries: allLibraries, user } = useApp()
  const libraries = allLibraries.filter(inMainMenus)
  if (!libraries.length) return null
  return (
    <section className="hub">
      <div className="hub-head"><h2>Your libraries</h2>{user?.isAdmin && <Link to="/settings/libraries">Manage <IconRight /></Link>}</div>
      <div className="lib-tiles">
        {libraries.map((l) => {
          const [one, many] = NOUNS[l.type] || ['item', 'items']
          return (
            <Link key={l.id} to={`/library/${l.id}`} className="lib-tile">
              <span className="lib-tile-ic">{libIcon(l.type)}</span>
              <span className="lib-tile-txt"><b>{l.name}</b><span className="muted">{l.scanning ? 'Scanning…' : `${l.itemCount.toLocaleString()} ${l.itemCount === 1 ? one : many}`}</span></span>
            </Link>
          )
        })}
        {user?.isAdmin && (
          <Link to="/settings/libraries" className="lib-tile add">
            <span className="lib-tile-ic" aria-hidden="true"><IconPlus /></span>
            <span className="lib-tile-txt"><b>Add a library</b><span className="muted">Another folder of media</span></span>
          </Link>
        )}
      </div>
    </section>
  )
}

// Shuffle: a random movie or disc, for when nobody can decide.
function Shuffle() {
  const opener = useOpenItem()
  const [it, setIt] = useState<Item | null>(null)
  const roll = () => api.get<{ items: Item[] }>('/items?sort=random&limit=1&noEpisodes=1&watchable=1').then((r) => setIt(r.items[0] || null)).catch(() => {})
  useEffect(() => { roll() }, [])
  if (!it) return null
  return (
    <section className="hub">
      <div className="hub-head"><h2>Can’t decide?</h2></div>
      <div className="shuffle">
        <div className="shuffle-art" style={backdropUrl(it) ? { backgroundImage: `url(${backdropUrl(it)})` } : undefined} />
        <img className="shuffle-poster" src={posterUrl(it)} alt="" loading="lazy" />
        <div className="shuffle-info">
          <h3>{it.title}</h3>
          <div className="muted">{[it.year, fmtRuntime(it.runtimeSec), it.genres.slice(0, 2).join(', ')].filter(Boolean).join(' · ')}</div>
          <div className="actions">
            <button className="btn primary" onClick={() => opener.open(it)}>{opener.icon(it)} {opener.label(it)}</button>
            <Link className="btn" to={`/item/${it.id}`}><IconInfo /> Details</Link>
            <button className="btn ghost" onClick={roll}><IconRefresh /> Another</button>
          </div>
        </div>
      </div>
    </section>
  )
}

function Hero({ items }: { items: Home['featured'] }) {
  const [i, setI] = useState(0)
  // It only turns by itself if nobody asked for less motion, and stops while
  // the pointer or keyboard is on it, or when paused (WCAG 2.2.2).
  const [paused, setPaused] = useState(() => matchMedia('(prefers-reduced-motion: reduce)').matches)
  const [hold, setHold] = useState(false)
  const nav = useNavigate()
  const opener = useOpenItem()
  useEffect(() => {
    if (items.length < 2 || paused || hold) return
    const t = setInterval(() => setI((x) => (x + 1) % items.length), 9000)
    return () => clearInterval(t)
  }, [items.length, paused, hold])
  const it = items[i]
  if (!it) return null
  return (
    <div className="hero" onMouseEnter={() => setHold(true)} onMouseLeave={() => setHold(false)} onFocus={() => setHold(true)} onBlur={() => setHold(false)}>
      {items.map((x, k) => <div key={x.id} className="bg" style={{ backgroundImage: backdropUrl(x) ? `url(${backdropUrl(x)})` : undefined, opacity: k === i ? 1 : 0 }} />)}
      <div className="shade" />
      <div className="info">
        <h1>{it.title}</h1>
        <div className="meta">
          {it.year && <span>{it.year}</span>}
          {it.runtimeSec > 0 && <span>{fmtRuntime(it.runtimeSec)}</span>}
          {it.contentRating && <span className="pill">{it.contentRating}</span>}
          {it.rating && <span className="star"><IconStar /> {it.rating.toFixed(1)}</span>}
          {isDisc(it) && <span>{discName(it)}{it.disc?.hasMenus !== false ? ' · interactive menus' : ''}</span>}
          {it.genres.slice(0, 3).map((g) => <span key={g}>{g}</span>)}
        </div>
        {it.overview && <p>{it.overview}</p>}
        <div className="actions">
          <button className="btn primary" onClick={() => opener.open(it)}>{opener.icon(it)} {opener.label(it)}</button>
          {isDisc(it) && it.disc?.hasMenus && <button className="btn" onClick={() => nav(`/play/${it.id}?mode=menu`)}>Disc menu</button>}
          <Link className="btn ghost" to={`/item/${it.id}`}><IconInfo /> Details</Link>
        </div>
      </div>
      {items.length > 1 && (
        <div className="dots">
          <button type="button" className="hero-pause" aria-label={paused ? 'Start turning the featured titles' : 'Stop turning the featured titles'} title={paused ? 'Play' : 'Pause'} onClick={() => setPaused((p) => !p)}>{paused ? <IconPlay /> : <IconPause />}</button>
          {items.map((_, k) => <button key={k} type="button" aria-label={`Show featured title ${k + 1}`} aria-current={k === i || undefined} className={k === i ? 'active' : ''} onClick={() => setI(k)} />)}
        </div>
      )}
    </div>
  )
}
