import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, artSet, sized, WIDE_CARD_SIZES, type Library } from '../../api'
import BrandLogo from '../../components/BrandLogo'
import { EmptyState, ItemCard, LoadError, ScanButton, ShowCard, SkeletonGrid, Spinner, artHue } from '../../components/common'
import { Button, LinkButton, Notice, Shelf as ShelfRow } from '../../ui'
import LegalNote from '../../components/LegalNote'
import LibraryHeader, { type SortOption } from '../../components/LibraryHeader'
import { IconCheck, IconEdit, IconFilm, IconPlus, IconRefresh, IconTune } from '../../icons'
import { artPictureUrl, type BrowseResult, type BrowseRow, type Category, type ListingStatus, type SavedView, type Shelves } from '../../metadata'
import { toast, useApp } from '../../state'
import BulkEdit from './BulkEdit'
import CategoryEditor from './CategoryEditor'
import { ActiveFilters, FILTER_KEYS, FilterPanel, SaveViewDialog } from './Filters'
import './library.css'

// Films and shows: sleek shelves to start with, a grid once something is asked for. The
// address holds the whole state, so a view can be bookmarked or pinned.

const PAGE = 120

const SORTS: SortOption[] = [
  { key: 'shuffle', label: 'Shuffle' },
  { key: 'added', label: 'Recently added', desc: true },
  { key: 'title', label: 'Title A to Z' },
  { key: 'newest', label: 'Newest' },
  { key: 'oldest', label: 'Oldest' },
  { key: 'rating', label: 'Highest rated', desc: true },
  { key: 'quality', label: 'Best quality', desc: true },
]

const browseQuery = (params: URLSearchParams, extra: Record<string, string | number> = {}) => {
  const q = new URLSearchParams()
  for (const k of FILTER_KEYS) { const v = params.get(k); if (v) q.set(k, v) }
  for (const k of ['sort', 'desc', 'seed']) { const v = params.get(k); if (v) q.set(k, v) }
  for (const [k, v] of Object.entries(extra)) q.set(k, String(v))
  return q.toString()
}

export function RowCard({ row }: { row: BrowseRow }) {
  return row.type === 'item' ? <ItemCard item={row.item} /> : <ShowCard show={row.show} />
}

// A row that asks for its titles when it comes near the screen, not before.
function LazyShelf({ title, query, seeAll, count }: { title: string; query: string; seeAll: string; count?: number }) {
  const [rows, setRows] = useState<BrowseRow[] | null>(null)
  const el = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const node = el.current
    if (!node) return
    const io = new IntersectionObserver((es) => {
      if (!es.some((e) => e.isIntersecting)) return
      io.disconnect()
      api.get<BrowseResult>(`/browse?${query}&limit=20`).then((r) => setRows(r.results)).catch(() => setRows([]))
    }, { rootMargin: '600px' })
    io.observe(node)
    return () => io.disconnect()
  }, [query])
  if (rows && rows.length === 0) return null
  return (
    <div ref={el} className="lib-lazy">
      {rows ? <ShelfRow title={title} to={seeAll} toLabel={`See all${count != null && count > 20 ? ` ${count}` : ''}`}>{rows.map((r) => <RowCard key={r.type + (r.type === 'item' ? r.item.id : r.show.id)} row={r} />)}</ShelfRow> : <div className="lib-shelf-wait" />}
    </div>
  )
}

function CategoryCard({ c, to, name, sub }: { c: Pick<Category, 'art'>; to: string; name: string; sub: string }) {
  const [broken, setBroken] = useState(false)
  const art = c.art[0]
  const h = artHue(name)
  return (
    <Link to={to} className="lib-cat" style={{ '--art-hue': h } as React.CSSProperties} aria-label={`${name}, ${sub}`}>
      {art && !broken && <img src={sized(artPictureUrl(art), 480)} srcSet={artSet(artPictureUrl(art), [320, 480, 640, 1280])} sizes={WIDE_CARD_SIZES} alt="" loading="lazy" onError={() => setBroken(true)} />}
      <span className="lib-cat-text"><b>{name}</b><span>{sub}</span></span>
    </Link>
  )
}

function LogoCard({ kind, name, to, count }: { kind: 'studio' | 'network' | 'platform'; name: string; to: string; count: number }) {
  return (
    <Link to={to} className="lib-logo" aria-label={`${name}, ${count} ${count === 1 ? 'title' : 'titles'}`} title={name}>
      <BrandLogo kind={kind} name={name} label={false} />
    </Link>
  )
}

export default function VideoLibrary({ lib }: { lib: Library }) {
  const { id } = useParams()
  const { user } = useApp()
  const [params, setParams] = useSearchParams()
  const sort = params.get('sort') || ''
  const desc = params.get('desc') === '1'
  const asGrid = FILTER_KEYS.some((k) => params.get(k)) || !!sort || params.get('view') === 'all'
  const [shelves, setShelves] = useState<Shelves | null>(null)
  const [listing, setListing] = useState<ListingStatus | null>(null)
  const [views, setViews] = useState<SavedView[]>([])
  const [rows, setRows] = useState<BrowseRow[] | null>(null)
  const [total, setTotal] = useState(0)
  const [failed, setFailed] = useState(false)
  const [retry, setRetry] = useState(0)
  const [scanTick, setScanTick] = useState(0)
  const [scanning, setScanning] = useState<{ done?: number; found?: number } | null>(null)
  const [filters, setFilters] = useState(false)
  const [saving, setSaving] = useState(false)
  const [arranging, setArranging] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Map<string, BrowseRow>>(new Map())
  const [bulk, setBulk] = useState(false)
  const [queueCount, setQueueCount] = useState(0)
  const isShows = lib.type === 'shows' || lib.type === 'anime'

  const query = useMemo(() => browseQuery(params, { library: lib.id, sort: params.get('sort') || 'title' }), [params, lib.id])
  const key = useRef('')

  // The shelves: all a library's page needs in one answer.
  const loadShelves = useCallback(() => {
    api.get<Shelves>(`/browse/shelves?library=${lib.id}`).then(setShelves).catch(() => setShelves({ categories: [], studios: [], networks: [], collections: [], platforms: [] }))
  }, [lib.id])
  useEffect(loadShelves, [loadShelves, scanTick])
  useEffect(() => { api.get<ListingStatus>('/listings/status').then(setListing).catch(() => {}) }, [])
  const loadViews = useCallback(() => { api.get<{ views: SavedView[] }>('/views').then((r) => setViews(r.views.filter((v) => v.libraryId === lib.id))).catch(() => {}) }, [lib.id])
  useEffect(loadViews, [loadViews])
  useEffect(() => {
    if (!user?.isAdmin) return
    api.get<{ counts: Record<string, number> }>(`/match/queue?library=${lib.id}&limit=1`).then((r) => setQueueCount(r.counts.review || 0)).catch(() => {})
  }, [lib.id, user?.isAdmin, scanTick])

  // A library too small or too bare for shelves is a grid from the start.
  const noShelves = !!shelves && shelves.categories.length === 0 && shelves.studios.length === 0 && shelves.networks.length === 0
  const grid = asGrid || noShelves
  const [allCount, setAllCount] = useState<number | null>(null)
  useEffect(() => {
    api.get<BrowseResult>(`/browse?library=${lib.id}&limit=1`).then((r) => setAllCount(r.total)).catch(() => {})
  }, [lib.id, scanTick])

  useEffect(() => {
    if (!grid) return
    if (key.current !== query) { setRows(null); key.current = query }
    setFailed(false)
    const q = query
    api.get<BrowseResult>(`/browse?${q}&limit=${PAGE}`)
      .then((r) => { if (key.current === q) { setRows(r.results); setTotal(r.total) } })
      .catch(() => setFailed(true))
  }, [grid, query, retry, scanTick])

  // The next page, when the end of the list comes near.
  const more = useRef<HTMLDivElement | null>(null)
  const loaded = rows?.length ?? 0
  const hasMore = grid && loaded > 0 && loaded < total
  useEffect(() => {
    const el = more.current
    if (!el || !hasMore) return
    let busy = false
    const q = query
    const io = new IntersectionObserver((es) => {
      if (!es.some((e) => e.isIntersecting) || busy) return
      busy = true
      api.get<BrowseResult>(`/browse?${q}&limit=${PAGE}&offset=${loaded}`)
        .then((r) => { if (key.current === q) { setRows((cur) => [...(cur || []), ...r.results]); setTotal(r.total) } })
        .catch(() => {})
        .finally(() => { busy = false })
    }, { rootMargin: '800px' })
    io.observe(el)
    return () => io.disconnect()
  }, [hasMore, loaded, query])

  useEffect(() => {
    let t: ReturnType<typeof setTimeout>
    let was = false
    const poll = async () => {
      try {
        const r = await api.get<{ scanning: boolean; progress: { done?: number; found?: number } | null }>(`/libraries/${id}/scan`)
        setScanning(r.scanning ? r.progress : null)
        if (was && !r.scanning) setScanTick((n) => n + 1)
        was = r.scanning
        t = setTimeout(poll, r.scanning ? 1500 : 10000)
      } catch { /* the next page asks again */ }
    }
    poll()
    return () => clearTimeout(t)
  }, [id])

  const setParam = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const chooseSort = (s: string, d: boolean) => {
    const p = new URLSearchParams(params)
    p.set('sort', s)
    if (d) p.set('desc', '1'); else p.delete('desc')
    if (s === 'shuffle') p.set('seed', String(Math.floor(Math.random() * 90000) + 1))
    else p.delete('seed')
    setParams(p, { replace: true })
  }
  const clearAll = () => { const p = new URLSearchParams(); setParams(p, { replace: true }) }
  const here = `/library/${lib.id}`
  const toggle = (r: BrowseRow) => {
    const k = r.type + (r.type === 'item' ? r.item.id : r.show.id)
    setSelected((m) => { const n = new Map(m); if (n.has(k)) n.delete(k); else n.set(k, r); return n })
  }
  const activeCount = FILTER_KEYS.filter((k) => params.get(k)).length
  const logosShown = !!shelves && (shelves.studios.length > 0 || shelves.networks.length > 0)
  const noun = isShows ? 'shows' : lib.type === 'discs' ? 'discs' : lib.type === 'videos' ? 'videos' : 'films'
  const allName = isShows ? 'All shows' : lib.type === 'discs' ? 'All discs' : lib.type === 'videos' ? 'All videos' : 'All films'

  return (
    <div className="lib">
      <LibraryHeader
        title={lib.name}
        count={grid && rows ? `${total} ${total === 1 ? noun.replace(/s$/, '') : noun}` : allCount != null ? `${allCount} ${allCount === 1 ? noun.replace(/s$/, '') : noun}` : undefined}
        sorts={SORTS}
        sort={sort || (grid ? 'title' : '')}
        desc={desc}
        onSort={chooseSort}
        onDirection={sort && sort !== 'shuffle' && sort !== 'newest' && sort !== 'oldest' ? (d) => setParam('desc', d ? '1' : '') : undefined}
        end={<Button variant="ghost" icon={<IconTune />} onClick={() => setFilters(true)} aria-haspopup="dialog">Filters{activeCount > 0 ? ` (${activeCount})` : ''}</Button>}
        actions={<>
          {grid && !noShelves && <Button variant="ghost" onClick={clearAll}>Back to shelves</Button>}
          {!grid && <LinkButton variant="ghost" to={`${here}?view=all`}>{allName}</LinkButton>}
          {user?.isAdmin && queueCount > 0 && <LinkButton variant="ghost" to={`/fix-matches?library=${lib.id}`}>Fix matches <span className="lib-badge">{queueCount}</span></LinkButton>}
          {user?.isAdmin && grid && <Button variant="ghost" icon={<IconCheck />} className={selecting ? 'active' : undefined} aria-pressed={selecting} onClick={() => { setSelecting(!selecting); setSelected(new Map()) }}>Select</Button>}
          {user?.isAdmin && !grid && <Button variant="ghost" icon={<IconEdit />} onClick={() => setArranging(true)}>Categories</Button>}
          {scanning
            ? <Button variant="ghost" icon={<IconRefresh />} disabled>Scanning… {scanning.done ?? 0}/{scanning.found ?? '?'}</Button>
            : <ScanButton lib={lib} onStarted={() => setScanning({})} />}
        </>}
      >
        <ActiveFilters params={params} setParam={setParam} onSave={() => setSaving(true)} onClear={clearAll} platforms={shelves?.platforms} />
        {views.length > 0 && !activeCount && (
          <div className="chips lib-views" role="group" aria-label="Saved views">
            {views.map((v) => <Link key={v.id} className="chip" to={`${here}?${v.query}`}>{v.name}</Link>)}
          </div>
        )}
      </LibraryHeader>

      {grid ? (
        <GridBody rows={rows} failed={failed} onRetry={() => setRetry((n) => n + 1)} scanning={!!scanning} filtered={activeCount > 0} lib={lib} isAdmin={!!user?.isAdmin} selecting={selecting} selected={selected} toggle={toggle} />
      ) : shelves ? (
        <Shelf lib={lib} shelves={shelves} listing={listing} isAdmin={!!user?.isAdmin} />
      ) : <SkeletonGrid />}
      {hasMore && <div ref={more} style={{ minHeight: 1 }}><Spinner /></div>}

      {params.get('platform') && listing && <ListingNote s={listing} />}
      {logosShown && <LegalNote kind="logos" />}
      <LegalNote kind="metadata" />

      {selecting && selected.size > 0 && (
        <div className="lib-bulk" role="region" aria-label="Selected titles">
          <span>{selected.size} selected</span>
          <Button variant="primary" onClick={() => setBulk(true)}>Edit together</Button>
          <Button variant="ghost" onClick={() => setSelected(new Map())}>Clear</Button>
        </div>
      )}
      {filters && <FilterPanel library={lib.id} params={params} setParam={setParam} onClear={clearAll} onSave={() => { setFilters(false); setSaving(true) }} onClose={() => setFilters(false)} />}
      {saving && <SaveViewDialog library={lib.id} params={params} onClose={() => setSaving(false)} onSaved={() => { setSaving(false); loadViews(); toast('View saved') }} />}
      {arranging && <CategoryEditor library={lib.id} onClose={() => setArranging(false)} onChanged={loadShelves} />}
      {bulk && <BulkEdit titles={[...selected.values()].map((r) => (r.type === 'item' ? { target: 'item' as const, id: r.item.id, title: r.item.title } : { target: 'show' as const, id: r.show.id, title: r.show.title }))}
        onClose={() => setBulk(false)} onDone={() => { setBulk(false); setSelected(new Map()); setSelecting(false); setRetry((n) => n + 1); loadShelves() }} />}
    </div>
  )
}

function GridBody({ rows, failed, onRetry, scanning, filtered, lib, isAdmin, selecting, selected, toggle }: {
  rows: BrowseRow[] | null; failed: boolean; onRetry: () => void; scanning: boolean; filtered: boolean; lib: Library; isAdmin: boolean; selecting: boolean; selected: Map<string, BrowseRow>; toggle: (r: BrowseRow) => void
}) {
  if (!rows) return failed ? <LoadError what="This library" onRetry={onRetry} /> : <SkeletonGrid />
  if (rows.length === 0) {
    return <EmptyState icon={<IconFilm />} title={scanning ? 'Scanning…' : filtered ? 'Nothing matches' : 'Nothing here yet'}
      text={scanning ? 'Titles appear as they are found.' : filtered ? 'Try taking away a filter.' : lib.type === 'shows' || lib.type === 'anime' ? 'Organize episodes as Show/Season 1/Show - S01E01 - Title.mkv and scan again.' : 'No videos were found in this library’s folders.'}
      action={!scanning && !filtered && isAdmin ? <Link to="/settings/libraries" className="btn">Check its folders</Link> : undefined} />
  }
  if (!selecting) return <div className="grid">{rows.map((r) => <RowCard key={r.type + (r.type === 'item' ? r.item.id : r.show.id)} row={r} />)}</div>
  return (
    <div className="grid lib-selecting" role="group" aria-label="Choose titles">
      {rows.map((r) => {
        const k = r.type + (r.type === 'item' ? r.item.id : r.show.id)
        const name = r.type === 'item' ? r.item.title : r.show.title
        const on = selected.has(k)
        return (
          <label key={k} className={'lib-pick' + (on ? ' on' : '')}>
            <input type="checkbox" checked={on} onChange={() => toggle(r)} aria-label={`Choose ${name}`} />
            <span className="lib-pick-card" inert><RowCard row={r} /></span>
          </label>
        )
      })}
    </div>
  )
}

export function ListingNote({ s }: { s: ListingStatus }) {
  return (
    <div className="lib-listing-note">
      <Notice tone="warn" title="Nothing streams from these services.">
        This only sorts the media already on your server by which titles they currently list, for region {s.region}{s.newest ? `, as of ${new Date(s.newest * 1000).toLocaleDateString()}` : ''}. There's no connection with any of them, and you may have no account with them at all. <span className="muted">{s.attribution}</span>
      </Notice>
    </div>
  )
}

function Shelf({ lib, shelves, listing, isAdmin }: { lib: Library; shelves: Shelves; listing: ListingStatus | null; isAdmin: boolean }) {
  const here = `/library/${lib.id}`
  const q = (extra: string) => `library=${lib.id}&${extra}`
  const brandTo = (kind: string, name: string) => `/brand/${kind}/${encodeURIComponent(name)}?library=${lib.id}`
  return (
    <>
      {shelves.categories.length > 0 && (
        <ShelfRow title="Categories">
          {shelves.categories.map((c) => <CategoryCard key={c.key} c={c} to={`${here}?genre=${encodeURIComponent(c.name)}`} name={c.name} sub={`${c.count} ${c.count === 1 ? 'title' : 'titles'}`} />)}
        </ShelfRow>
      )}
      {shelves.collections.length > 0 && (
        <ShelfRow title="Collections" to={shelves.collections.some((c) => !c.key) ? '/collections' : undefined}>
          {shelves.collections.map((c) => <CategoryCard key={c.key || c.id} c={c} to={c.key ? `${here}?series=${encodeURIComponent(c.name)}` : `/collections/${c.id}`} name={c.name} sub={`${c.count} ${c.count === 1 ? 'title' : 'titles'}`} />)}
        </ShelfRow>
      )}
      {shelves.studios.length > 0 && (
        <ShelfRow title="Studios">{shelves.studios.slice(0, 40).map((b) => <LogoCard key={b.key} kind="studio" name={b.name} to={brandTo('studio', b.name)} count={b.count} />)}</ShelfRow>
      )}
      {shelves.networks.length > 0 && (
        <ShelfRow title="Networks">{shelves.networks.slice(0, 40).map((b) => <LogoCard key={b.key} kind="network" name={b.name} to={brandTo('network', b.name)} count={b.count} />)}</ShelfRow>
      )}
      {listing?.enabled && shelves.platforms.length > 0 && (
        <>
          <ShelfRow title="Also listed on">{shelves.platforms.map((p) => <LogoCard key={p.key} kind="platform" name={p.name} to={`${here}?platform=${encodeURIComponent(p.key)}`} count={p.count} />)}</ShelfRow>
          <ListingNote s={listing} />
        </>
      )}
      <LazyShelf title="Recently added" query={q('sort=added&desc=1')} seeAll={`${here}?sort=added&desc=1`} />
      <LazyShelf title="Highest rated" query={q('sort=rating&desc=1&minRating=6')} seeAll={`${here}?sort=rating&desc=1`} />
      {shelves.categories.slice(0, 12).map((c) => (
        <LazyShelf key={c.key} title={c.name} query={q(`genre=${encodeURIComponent(c.name)}&sort=rating&desc=1`)} seeAll={`${here}?genre=${encodeURIComponent(c.name)}`} count={c.count} />
      ))}
      {isAdmin && shelves.categories.length === 0 && (
        <p className="muted lib-empty-note"><IconPlus size={16} /> <span>Categories come from genres, which come from matching titles. <Link to={`/fix-matches?library=${lib.id}`}>Fix matches</Link> to fill them in.</span></p>
      )}
    </>
  )
}
