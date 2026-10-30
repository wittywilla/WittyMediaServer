import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Item, type Show } from '../api'
import { EmptyState, ItemCard, ShowCard, SkeletonGrid } from '../components/common'
import { IconSearch } from '../icons'

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
      {!items ? <SkeletonGrid /> : items.length + shows.length === 0 ? <EmptyState icon={<IconSearch />} title="No results" text={q ? `Nothing on this server matches “${q}”. Try a shorter or different word.` : 'Type what you’re looking for in the search box.'} /> : (
        <>
          {shows.length > 0 && <><h2 className="shelf-head">Shows</h2><div className="grid">{shows.map((s) => <ShowCard key={s.id} show={s} />)}</div></>}
          {items.length > 0 && <>{shows.length > 0 && <h2 className="shelf-head">Movies &amp; episodes</h2>}<div className="grid">{items.map((it) => <ItemCard key={it.id} item={it} />)}</div></>}
        </>
      )}
    </>
  )
}
