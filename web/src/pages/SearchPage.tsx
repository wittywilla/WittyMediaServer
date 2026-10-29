import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Item, type Show } from '../api'
import { EmptyState, ItemCard, ShowCard, Spinner } from '../components/common'

export default function SearchPage() {
  const [params] = useSearchParams()
  const q = params.get('q') || ''
  const [items, setItems] = useState<Item[] | null>(null)
  const [shows, setShows] = useState<Show[]>([])
  useEffect(() => {
    setItems(null)
    api.get<{ items: Item[] }>(`/items?search=${encodeURIComponent(q)}`).then((r) => setItems(r.items))
    api.get<{ shows: Show[] }>(`/shows?search=${encodeURIComponent(q)}`).then((r) => setShows(r.shows)).catch(() => setShows([]))
  }, [q])
  return (
    <>
      <div className="toolbar"><h1>Search</h1><span className="count">“{q}”</span></div>
      {!items ? <Spinner /> : items.length + shows.length === 0 ? <EmptyState title="No results" /> : (
        <>
          {shows.length > 0 && <><h3 className="section-title">Shows</h3><div className="grid">{shows.map((s) => <ShowCard key={s.id} show={s} />)}</div></>}
          {items.length > 0 && <>{shows.length > 0 && <h3 className="section-title">Movies &amp; episodes</h3>}<div className="grid">{items.map((it) => <ItemCard key={it.id} item={it} />)}</div></>}
        </>
      )}
    </>
  )
}
