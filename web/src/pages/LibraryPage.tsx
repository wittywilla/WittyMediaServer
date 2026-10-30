import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, type Item, type Show } from '../api'
import { EmptyState, ItemCard, ShowCard, SkeletonGrid, Spinner } from '../components/common'
import { useApp } from '../state'
import { IconRefresh, IconSortDown, IconSortUp, IconTv } from '../icons'
import { toast } from '../state'
import AudioLibrary from './AudioLibrary'
import BooksLibrary from './Books'
import { GamesLibrary } from './Games'

// Loaded when opened: most servers have only some of these libraries.
const PhotosLibrary = lazy(() => import('./photos/PhotosLibrary'))
const ShortsLibrary = lazy(() => import('./creators/ShortsLibrary'))
const ChannelsLibrary = lazy(() => import('./creators/ChannelsLibrary'))
const RadioLibrary = lazy(() => import('./RadioLibrary'))

const AUDIO = ['music', 'audiobooks', 'podcasts']
// Items load a page at a time as the viewer scrolls: a library of thousands
// must not cost its whole list (and that many cards) on an old TV or phone.
const PAGE = 120

export default function LibraryPage() {
  const { id } = useParams()
  const { libraries } = useApp()
  const lib = libraries.find((l) => String(l.id) === id)
  if (lib && AUDIO.includes(lib.type)) return <AudioLibrary key={lib.id} lib={lib} />
  if (lib?.type === 'books') return <BooksLibrary key={lib.id} lib={lib} />
  if (lib?.type === 'photos') return <Suspense fallback={<Spinner />}><PhotosLibrary key={lib.id} lib={lib} /></Suspense>
  if (lib?.type === 'shorts') return <Suspense fallback={<Spinner />}><ShortsLibrary key={lib.id} lib={lib} /></Suspense>
  if (lib?.type === 'channels' || lib?.type === 'musicvideos') return <Suspense fallback={<Spinner />}><ChannelsLibrary key={lib.id} lib={lib} /></Suspense>
  if (lib?.type === 'radio') return <Suspense fallback={<Spinner />}><RadioLibrary key={lib.id} lib={lib} /></Suspense>
  if (lib?.type === 'games') return <GamesLibrary key={lib.id} lib={lib} />
  return <VideoLibrary />
}

function VideoLibrary() {
  const { id } = useParams()
  const { libraries, user } = useApp()
  const lib = libraries.find((l) => String(l.id) === id)
  const [params, setParams] = useSearchParams()
  const sort = params.get('sort') || 'title'
  const desc = params.get('desc') === '1'
  const genre = params.get('genre') || ''
  const network = params.get('network') || ''
  // shelf: which discs to show, "" (everything), discs, movie or tv
  const shelf = params.get('shelf') || ''
  const [discCount, setDiscCount] = useState({ all: 0, tv: 0 })
  const [showDiscs, setShowDiscs] = useState<Item[]>([])
  const [networks, setNetworks] = useState<{ name: string; shows: number }[]>([])
  const [items, setItems] = useState<Item[] | null>(null)
  const [shows, setShows] = useState<Show[] | null>(null)
  const isShows = lib?.type === 'shows' || lib?.type === 'anime'
  const [total, setTotal] = useState(0)
  const [genres, setGenres] = useState<string[]>([])
  const [scan, setScan] = useState<any>(null)
  const listKey = `${id}|${sort}|${desc}|${genre}|${network}|${shelf}|${isShows}`
  const shownKey = useRef('')
  const listUrl = () => {
    const filter = shelf === 'discs' ? '&kind=disc' : shelf ? `&kind=disc&discContent=${shelf}` : ''
    return `/items?library=${id}&sort=${sort}&desc=${desc ? 1 : 0}&genre=${encodeURIComponent(genre)}${filter}&limit=${PAGE}`
  }

  useEffect(() => {
    // A rescan refreshes the list in place; the spinner is for a different list.
    if (shownKey.current !== listKey) { setItems(null); setShows(null); shownKey.current = listKey }
    if (isShows) {
      api.get<{ items: Item[] }>(`/items?library=${id}&kind=disc&sort=title`).then((r) => setShowDiscs(r.items)).catch(() => setShowDiscs([]))
      api.get<{ shows: Show[]; total: number }>(`/shows?library=${id}&sort=${sort}&desc=${desc ? 1 : 0}&genre=${encodeURIComponent(genre)}&network=${encodeURIComponent(network)}`)
        .then((r) => { setShows(r.shows); setItems([]); setTotal(r.total) })
      api.get<string[]>(`/shows/genres?library=${id}`).then(setGenres).catch(() => {})
      api.get<{ name: string; shows: number }[]>(`/shows/networks?library=${id}`).then(setNetworks).catch(() => {})
      return
    }
    api.get<{ items: Item[]; total: number }>(listUrl())
      .then((r) => { if (shownKey.current === listKey) { setItems(r.items); setTotal(r.total) } })
    Promise.all([
      api.get<{ total: number }>(`/items?library=${id}&kind=disc&limit=1`),
      api.get<{ total: number }>(`/items?library=${id}&kind=disc&discContent=tv&limit=1`),
    ]).then(([a, t]) => setDiscCount({ all: a.total, tv: t.total })).catch(() => {})
    api.get<string[]>(`/genres?library=${id}`).then(setGenres).catch(() => {})
  }, [id, sort, desc, genre, network, shelf, scan?.done, isShows])

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

  // The next page, when the end of the list comes near.
  const more = useRef<HTMLDivElement | null>(null)
  const loaded = items?.length ?? 0
  const hasMore = !isShows && loaded > 0 && loaded < total
  useEffect(() => {
    const el = more.current
    if (!el || !hasMore) return
    let busy = false
    const key = listKey
    const io = new IntersectionObserver((es) => {
      if (!es.some((e) => e.isIntersecting) || busy) return
      busy = true
      api.get<{ items: Item[]; total: number }>(`${listUrl()}&offset=${loaded}`)
        .then((r) => { if (shownKey.current === key) { setItems((cur) => [...(cur || []), ...r.items]); setTotal(r.total) } })
        .catch(() => {})
        .finally(() => { busy = false })
    }, { rootMargin: '800px' })
    io.observe(el)
    return () => io.disconnect()
  }, [hasMore, loaded, listKey]) // eslint-disable-line react-hooks/exhaustive-deps

  const set = (k: string, v: string) => { const p = new URLSearchParams(params); v ? p.set(k, v) : p.delete(k); setParams(p, { replace: true }) }
  const rescan = async () => { await api.post(`/libraries/${id}/scan`); toast('Scan started'); setScan({ phase: 'starting' }) }

  if (!lib) return <EmptyState title="Library not found" action={<Link to="/settings/libraries" className="btn">Manage libraries</Link>} />
  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}</h1>
        <span className="count">{total} {isShows ? (total === 1 ? 'show' : 'shows') : total === 1 ? 'item' : 'items'}</span>
        <div className="grow" />
        <select className="select" value={sort} aria-label="Sort by" onChange={(e) => set('sort', e.target.value)}>
          <option value="title">Title</option><option value="added">Date added</option><option value="year">Year</option>
          <option value="rating">Rating</option>{!isShows && <option value="runtime">Runtime</option>}
        </select>
        {isShows && networks.length > 1 && (
          <select className="select" value={network} onChange={(e) => set('network', e.target.value)} title="Network">
            <option value="">All networks</option>
            {networks.map((n) => <option key={n.name} value={n.name === 'Unknown network' ? '' : n.name}>{n.name} ({n.shows})</option>)}
          </select>
        )}
        <button className="btn ghost square" onClick={() => set('desc', desc ? '' : '1')} aria-label={desc ? 'Sorted high to low: switch to low to high' : 'Sorted low to high: switch to high to low'} title={desc ? 'High to low' : 'Low to high'}>{desc ? <IconSortDown /> : <IconSortUp />}</button>
        {user?.isAdmin && <button className="btn ghost" onClick={rescan} disabled={!!scan}><IconRefresh /> {scan ? `Scanning… ${scan.done ?? 0}/${scan.found ?? '?'}` : 'Scan'}</button>}
      </div>
      {!isShows && discCount.all > 0 && (lib.type === 'discs' ? discCount.tv > 0 && discCount.tv < discCount.all : true) && (
        <div className="seg shelf-seg" role="tablist" aria-label="Show">
          {(lib.type === 'discs'
            ? [['', 'All discs'], ['movie', 'Films'], ['tv', 'TV series']]
            : [['', 'Everything'], ['discs', 'DVDs & Blu-rays'], ...(discCount.tv ? [['tv', 'TV series on disc']] : [])]
          ).map(([k, l]) => <button key={k} role="tab" aria-selected={shelf === k} className={shelf === k ? 'active' : ''} onClick={() => set('shelf', k)}>{l}</button>)}
        </div>
      )}
      {genres.length > 0 && (
        <div className="chips">
          <button className={'chip' + (!genre ? ' active' : '')} onClick={() => set('genre', '')}>All</button>
          {genres.map((g) => <button key={g} className={'chip' + (genre === g ? ' active' : '')} onClick={() => set('genre', g)}>{g}</button>)}
        </div>
      )}
      {!items ? <SkeletonGrid /> : isShows
        ? (shows && shows.length > 0
          ? <>
              <div className="grid">{shows.map((s) => <ShowCard key={s.id} show={s} />)}</div>
              {showDiscs.length > 0 && <><h2 className="shelf-head">DVDs & Blu-rays</h2><div className="grid">{showDiscs.map((it) => <ItemCard key={it.id} item={it} />)}</div></>}
            </>
          : showDiscs.length > 0
          ? <><h2 className="shelf-head">DVDs & Blu-rays</h2><div className="grid">{showDiscs.map((it) => <ItemCard key={it.id} item={it} />)}</div></>
          : <EmptyState icon={<IconTv />} title={scan ? 'Scanning…' : 'No shows'} text={scan ? 'Shows appear as they are found.' : 'Organise episodes as Show/Season 1/Show - S01E01 - Title.mkv and rescan.'} />)
        : items.length === 0
        ? <EmptyState title={scan ? 'Scanning…' : 'No items'} text={scan ? 'Items appear as they are found.' : 'Nothing matched. Check the library folders in Settings.'} />
        : <><div className="grid">{items.map((it) => <ItemCard key={it.id} item={it} />)}</div>{hasMore && <div ref={more} style={{ minHeight: 1 }}><Spinner /></div>}</>}
    </>
  )
}
