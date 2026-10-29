import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, backdropUrl, fmtRuntime, isDisc, posterUrl, type Home, type Hub, type Item } from '../api'
import { EmptyState, ItemCard, ShowCard, Spinner } from '../components/common'
import { IconDisc, IconFilm, IconInfo, IconPlay, IconRefresh, IconTv } from '../icons'
import { toast, useApp } from '../state'

// The home page is a column of widgets. Everyone can reorder and hide them
// ("Customize"); the layout is saved to the account, so it follows the
// person to every device. Reordering works by dragging, and with Up/Down
// buttons for touch screens, TV remotes and keyboards.

type Layout = { order: string[]; hidden: string[] }
type Widget = { id: string; title: string; render: () => React.ReactNode | null }

export default function HomePage() {
  const [home, setHome] = useState<Home | null>(null)
  const [err, setErr] = useState('')
  const [layout, setLayout] = useState<Layout>({ order: [], hidden: [] })
  const [editing, setEditing] = useState(false)
  const [dragId, setDragId] = useState<string | null>(null)
  const { libraries } = useApp()
  useEffect(() => { api.get<Home>('/home').then(setHome).catch((e) => setErr(e.message)) }, [libraries])
  useEffect(() => {
    api.get<{ home?: Layout }>('/me/ui').then((u) => { if (u.home) setLayout({ order: u.home.order || [], hidden: u.home.hidden || [] }) }).catch(() => {})
  }, [])

  const widgets = useMemo<Widget[]>(() => {
    if (!home) return []
    const hub = (h: Hub): Widget => ({ id: h.key, title: h.title, render: () => <HubRow hub={h} /> })
    const byKey = (k: string) => home.hubs.find((h) => h.key === k)
    const list: Widget[] = [
      { id: 'hero', title: 'Featured', render: () => (home.featured.length > 0 ? <Hero items={home.featured} /> : null) },
    ]
    for (const k of ['continue', 'nextup']) { const h = byKey(k); if (h) list.push(hub(h)) }
    list.push({ id: 'libraries', title: 'Your libraries', render: () => <LibraryTiles /> })
    for (const h of home.hubs) if (h.key.startsWith('recent-')) list.push(hub(h))
    list.push({ id: 'shuffle', title: 'Shuffle', render: () => <Shuffle /> })
    return list
  }, [home])

  // Saved order first, then any widget the person has not placed yet.
  const ordered = useMemo(() => {
    const pos = (id: string) => { const i = layout.order.indexOf(id); return i < 0 ? 1000 + widgets.findIndex((w) => w.id === id) : i }
    return [...widgets].sort((a, b) => pos(a.id) - pos(b.id))
  }, [widgets, layout.order])

  const save = (next: Layout) => {
    setLayout(next)
    api.put('/me/ui', { home: next }).catch(() => toast('Could not save the layout'))
  }
  const move = (id: string, to: number) => {
    const ids = ordered.map((w) => w.id).filter((x) => x !== id)
    ids.splice(Math.max(0, Math.min(ids.length, to)), 0, id)
    save({ ...layout, order: ids })
  }
  const toggle = (id: string) => save({ ...layout, hidden: layout.hidden.includes(id) ? layout.hidden.filter((x) => x !== id) : [...layout.hidden, id] })

  if (err) return <div className="error">{err}</div>
  if (!home) return <Spinner />
  if (home.libraries.length === 0) {
    return <EmptyState title="Welcome to WittyMediaServer" text="Add a library pointing at a folder of movies, TV shows or DVD ISOs to get started."
      action={<Link className="btn primary" to="/settings/libraries">Add a library</Link>} />
  }
  const shown = editing ? ordered : ordered.filter((w) => !layout.hidden.includes(w.id))
  return (
    <div className={'home' + (editing ? ' editing' : '')}>
      <div className="home-tools">
        {editing ? (
          <>
            <span className="muted">Drag widgets, or use the arrows. Hidden widgets stay here until you show them again.</span>
            <button className="btn sm ghost" onClick={() => save({ order: [], hidden: [] })}>Reset</button>
            <button className="btn sm primary" onClick={() => setEditing(false)}>Done</button>
          </>
        ) : <button className="btn sm ghost" onClick={() => setEditing(true)} title="Rearrange or hide sections of the home page">Customize</button>}
      </div>
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
              <span className="grip" aria-hidden="true">⠿</span>
              <b>{w.title}</b>
              <span className="grow" />
              <button className="btn sm ghost" disabled={i === 0} onClick={() => move(w.id, i - 1)} aria-label={`Move ${w.title} up`}>▲</button>
              <button className="btn sm ghost" disabled={i === shown.length - 1} onClick={() => move(w.id, i + 1)} aria-label={`Move ${w.title} down`}>▼</button>
              <button className="btn sm" onClick={() => toggle(w.id)}>{hidden ? 'Show' : 'Hide'}</button>
            </div>
            {!hidden && <div className="widget-preview">{body || <div className="muted" style={{ padding: 12 }}>Nothing to show here right now.</div>}</div>}
          </div>
        )
      })}
      {!editing && home.hubs.length === 0 && <EmptyState title="Nothing here yet" text="Your libraries are empty or still scanning." />}
    </div>
  )
}

function HubRow({ hub }: { hub: Hub }) {
  if (!hub.items.length && !(hub.shows || []).length) return null
  return (
    <section className="hub">
      <div className="hub-head">
        <h2>{hub.title}</h2>
        {hub.libraryId && <Link to={`/library/${hub.libraryId}`}>See all ›</Link>}
      </div>
      <div className="row">
        {hub.items.map((it) => <ItemCard key={it.id} item={it} />)}
        {(hub.shows || []).map((s) => <ShowCard key={'s' + s.id} show={s} />)}
      </div>
    </section>
  )
}

// LibraryTiles: one big tile per library, a quick way in on phones and TVs.
function LibraryTiles() {
  const { libraries } = useApp()
  if (libraries.length < 2) return null
  return (
    <section className="hub">
      <div className="hub-head"><h2>Your libraries</h2></div>
      <div className="lib-tiles">
        {libraries.map((l) => (
          <Link key={l.id} to={`/library/${l.id}`} className="lib-tile">
            {l.type === 'movies' ? <IconFilm /> : l.type === 'shows' ? <IconTv /> : <IconDisc />}
            <b>{l.name}</b><span className="muted">{l.itemCount} {l.itemCount === 1 ? 'item' : 'items'}</span>
          </Link>
        ))}
      </div>
    </section>
  )
}

// Shuffle: a random movie or disc, for when nobody can decide.
function Shuffle() {
  const nav = useNavigate()
  const [it, setIt] = useState<Item | null>(null)
  const roll = () => api.get<{ items: Item[] }>('/items?sort=random&limit=1&noEpisodes=1').then((r) => setIt(r.items[0] || null)).catch(() => {})
  useEffect(() => { roll() }, [])
  if (!it) return null
  return (
    <section className="hub">
      <div className="hub-head"><h2>Can't decide?</h2></div>
      <div className="shuffle">
        <div className="shuffle-art" style={{ backgroundImage: `url(${backdropUrl(it)})` }} />
        <img className="shuffle-poster" src={posterUrl(it)} alt="" loading="lazy" />
        <div className="shuffle-info">
          <h3>{it.title}</h3>
          <div className="muted">{[it.year, fmtRuntime(it.runtimeSec), it.genres.slice(0, 2).join(', ')].filter(Boolean).join(' · ')}</div>
          <div className="actions">
            <button className="btn primary" onClick={() => nav(`/play/${it.id}`)}><IconPlay /> Play</button>
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
  const nav = useNavigate()
  useEffect(() => {
    if (items.length < 2) return
    const t = setInterval(() => setI((x) => (x + 1) % items.length), 9000)
    return () => clearInterval(t)
  }, [items.length])
  const it = items[i]
  if (!it) return null
  return (
    <div className="hero">
      {items.map((x, k) => <div key={x.id} className="bg" style={{ backgroundImage: `url(${backdropUrl(x)})`, opacity: k === i ? 1 : 0 }} />)}
      <div className="shade" />
      <div className="info">
        <h1>{it.title}</h1>
        <div className="meta">
          {it.year && <span>{it.year}</span>}
          {it.runtimeSec > 0 && <span>{fmtRuntime(it.runtimeSec)}</span>}
          {it.contentRating && <span>{it.contentRating}</span>}
          {it.rating && <span>★ {it.rating.toFixed(1)}</span>}
          {isDisc(it) && <span>DVD · interactive menus</span>}
          {it.genres.slice(0, 3).map((g) => <span key={g}>{g}</span>)}
        </div>
        {it.overview && <p>{it.overview}</p>}
        <div className="actions">
          <button className="btn primary" onClick={() => nav(`/play/${it.id}`)}><IconPlay /> {it.progress && !it.progress.watched && it.progress.positionSec > 60 ? 'Resume' : 'Play'}</button>
          {isDisc(it) && it.disc?.hasMenus && <button className="btn" onClick={() => nav(`/play/${it.id}?mode=menu`)}>DVD menu</button>}
          <Link className="btn ghost" to={`/item/${it.id}`}><IconInfo /> Details</Link>
        </div>
      </div>
      {items.length > 1 && <div className="dots">{items.map((_, k) => <button key={k} aria-label={`Show featured title ${k + 1}`} className={k === i ? 'active' : ''} onClick={() => setI(k)} />)}</div>}
    </div>
  )
}
