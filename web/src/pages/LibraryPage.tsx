import { useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, type Item, type Show } from '../api'
import { EmptyState, ItemCard, ShowCard, Spinner } from '../components/common'
import { useApp } from '../state'
import { IconRefresh } from '../icons'
import { toast } from '../state'

export default function LibraryPage() {
  const { id } = useParams()
  const { libraries, user } = useApp()
  const lib = libraries.find((l) => String(l.id) === id)
  const [params, setParams] = useSearchParams()
  const sort = params.get('sort') || 'title'
  const desc = params.get('desc') === '1'
  const genre = params.get('genre') || ''
  const network = params.get('network') || ''
  const [networks, setNetworks] = useState<{ name: string; shows: number }[]>([])
  const [items, setItems] = useState<Item[] | null>(null)
  const [shows, setShows] = useState<Show[] | null>(null)
  const isShows = lib?.type === 'shows'
  const [total, setTotal] = useState(0)
  const [genres, setGenres] = useState<string[]>([])
  const [scan, setScan] = useState<any>(null)

  useEffect(() => {
    setItems(null); setShows(null)
    if (isShows) {
      api.get<{ shows: Show[]; total: number }>(`/shows?library=${id}&sort=${sort}&desc=${desc ? 1 : 0}&genre=${encodeURIComponent(genre)}&network=${encodeURIComponent(network)}`)
        .then((r) => { setShows(r.shows); setItems([]); setTotal(r.total) })
      api.get<string[]>(`/shows/genres?library=${id}`).then(setGenres).catch(() => {})
      api.get<{ name: string; shows: number }[]>(`/shows/networks?library=${id}`).then(setNetworks).catch(() => {})
      return
    }
    api.get<{ items: Item[]; total: number }>(`/items?library=${id}&sort=${sort}&desc=${desc ? 1 : 0}&genre=${encodeURIComponent(genre)}`)
      .then((r) => { setItems(r.items); setTotal(r.total) })
    api.get<string[]>(`/genres?library=${id}`).then(setGenres).catch(() => {})
  }, [id, sort, desc, genre, network, scan?.done, isShows])

  useEffect(() => {
    let t: any
    const poll = async () => {
      try {
        const r = await api.get<{ scanning: boolean; progress: any }>(`/libraries/${id}/scan`)
        setScan(r.scanning ? r.progress : null)
        t = setTimeout(poll, r.scanning ? 1500 : 10000)
      } catch { /* ignore */ }
    }
    poll()
    return () => clearTimeout(t)
  }, [id])

  const set = (k: string, v: string) => { const p = new URLSearchParams(params); v ? p.set(k, v) : p.delete(k); setParams(p, { replace: true }) }
  const rescan = async () => { await api.post(`/libraries/${id}/scan`); toast('Scan started'); setScan({ phase: 'starting' }) }

  if (!lib) return <EmptyState title="Library not found" action={<Link to="/settings/libraries" className="btn">Manage libraries</Link>} />
  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}</h1>
        <span className="count">{total} {isShows ? (total === 1 ? 'show' : 'shows') : total === 1 ? 'item' : 'items'}</span>
        <div style={{ flex: 1 }} />
        <select className="select" value={sort} onChange={(e) => set('sort', e.target.value)}>
          <option value="title">Title</option><option value="added">Date added</option><option value="year">Year</option>
          <option value="rating">Rating</option>{!isShows && <option value="runtime">Runtime</option>}
        </select>
        {isShows && networks.length > 1 && (
          <select className="select" value={network} onChange={(e) => set('network', e.target.value)} title="Network">
            <option value="">All networks</option>
            {networks.map((n) => <option key={n.name} value={n.name === 'Unknown network' ? '' : n.name}>{n.name} ({n.shows})</option>)}
          </select>
        )}
        <button className="btn sm ghost" onClick={() => set('desc', desc ? '' : '1')}>{desc ? '↓ Desc' : '↑ Asc'}</button>
        {user?.isAdmin && <button className="btn sm ghost" onClick={rescan} disabled={!!scan}><IconRefresh /> {scan ? `Scanning… ${scan.done ?? 0}/${scan.found ?? '?'}` : 'Scan'}</button>}
      </div>
      {genres.length > 0 && (
        <div className="chips" style={{ marginBottom: 18 }}>
          <button className={'chip' + (!genre ? ' active' : '')} onClick={() => set('genre', '')}>All</button>
          {genres.map((g) => <button key={g} className={'chip' + (genre === g ? ' active' : '')} onClick={() => set('genre', g)}>{g}</button>)}
        </div>
      )}
      {!items ? <Spinner /> : isShows
        ? (shows && shows.length > 0
          ? <div className="grid">{shows.map((s) => <ShowCard key={s.id} show={s} />)}</div>
          : <EmptyState title={scan ? 'Scanning…' : 'No shows'} text={scan ? 'Shows appear as they are found.' : 'Organise episodes as Show/Season 1/Show - S01E01 - Title.mkv and rescan.'} />)
        : items.length === 0
        ? <EmptyState title={scan ? 'Scanning…' : 'No items'} text={scan ? 'Items appear as they are found.' : 'Nothing matched. Check the library folders in Settings.'} />
        : <div className="grid">{items.map((it) => <ItemCard key={it.id} item={it} />)}</div>}
    </>
  )
}
