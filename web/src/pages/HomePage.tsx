import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, backdropUrl, fmtRuntime, isDisc, type Home } from '../api'
import { EmptyState, ItemCard, ShowCard, Spinner } from '../components/common'
import { IconInfo, IconPlay } from '../icons'
import { useApp } from '../state'

export default function HomePage() {
  const [home, setHome] = useState<Home | null>(null)
  const [err, setErr] = useState('')
  const { libraries } = useApp()
  useEffect(() => { api.get<Home>('/home').then(setHome).catch((e) => setErr(e.message)) }, [libraries])
  if (err) return <div className="error">{err}</div>
  if (!home) return <Spinner />
  if (home.libraries.length === 0) {
    return <EmptyState title="Welcome to WittyMediaServer" text="Add a library pointing at a folder of movies, TV shows or DVD ISOs to get started."
      action={<Link className="btn primary" to="/settings/libraries">Add a library</Link>} />
  }
  return (
    <>
      {home.featured.length > 0 && <Hero items={home.featured} />}
      {home.hubs.map((h) => (
        <section className="hub" key={h.key}>
          <div className="hub-head">
            <h2>{h.title}</h2>
            {h.libraryId && <Link to={`/library/${h.libraryId}`}>See all ›</Link>}
          </div>
          <div className="row">
            {h.items.map((it) => <ItemCard key={it.id} item={it} />)}
            {(h.shows || []).map((s) => <ShowCard key={'s' + s.id} show={s} />)}
          </div>
        </section>
      ))}
      {home.hubs.length === 0 && <EmptyState title="Nothing here yet" text="Your libraries are empty or still scanning." />}
    </>
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
      {items.length > 1 && <div className="dots">{items.map((_, k) => <button key={k} className={k === i ? 'active' : ''} onClick={() => setI(k)} />)}</div>}
    </div>
  )
}
