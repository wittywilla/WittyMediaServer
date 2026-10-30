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
  const [failed, setFailed] = useState('')
  useEffect(() => {
    setItems(null); setFailed('')
    if (!q) return
    api.get<{ items: Item[] }>(`/items?search=${encodeURIComponent(q)}`).then((r) => setItems(r.items)).catch((e) => setFailed(e.message))
    api.get<{ shows: Show[] }>(`/shows?search=${encodeURIComponent(q)}`).then((r) => setShows(r.shows)).catch(() => setShows([]))
  }, [q])
  // Arriving with nothing typed (the phone's Search tab): the box is the next step.
  useEffect(() => { if (!q) document.querySelector<HTMLInputElement>('.search input')?.focus() }, [q])
  if (!q) {
    return (
      <>
        <div className="toolbar"><h1>Search</h1></div>
        <EmptyState icon={<IconSearch />} title="Find anything on this server" text="Type a title in the search box: movies, shows, episodes, music, books and games are all included." />
      </>
    )
  }
  if (failed) {
    return (
      <>
        <div className="toolbar"><h1>Search</h1><span className="count">“{q}”</span></div>
        <EmptyState icon={<IconSearch />} title="The search didn’t work" text={failed} />
      </>
    )
  }
  return (
    <>
      <div className="toolbar"><h1>Search</h1><span className="count">“{q}”</span></div>
      {!items ? <SkeletonGrid /> : items.length + shows.length === 0 ? <EmptyState icon={<IconSearch />} title="No results" text={q ? `Nothing on this server matches “${q}”. Try a shorter or different word.` : 'Type what you’re looking for in the search box.'} /> : (
        <>
          {shows.length > 0 && <><h2 className="shelf-head">Shows</h2><div className="grid">{shows.map((s) => <ShowCard key={s.id} show={s} />)}</div></>}
          {items.length > 0 && <>{shows.length > 0 && <h2 className="shelf-head">Everything else</h2>}<div className="grid">{items.map((it) => <ItemCard key={it.id} item={it} />)}</div></>}
        </>
      )}
    </>
  )
}
