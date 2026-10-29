import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Item } from '../api'
import { EmptyState, ItemCard, Spinner } from '../components/common'

export default function SearchPage() {
  const [params] = useSearchParams()
  const q = params.get('q') || ''
  const [items, setItems] = useState<Item[] | null>(null)
  useEffect(() => {
    setItems(null)
    api.get<{ items: Item[] }>(`/items?search=${encodeURIComponent(q)}`).then((r) => setItems(r.items))
  }, [q])
  return (
    <>
      <div className="toolbar"><h1>Search</h1><span className="count">“{q}”</span></div>
      {!items ? <Spinner /> : items.length === 0 ? <EmptyState title="No results" /> : <div className="grid">{items.map((it) => <ItemCard key={it.id} item={it} />)}</div>}
    </>
  )
}
