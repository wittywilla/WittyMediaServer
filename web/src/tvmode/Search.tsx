import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Item, type Show } from '../api'
import { EmptyState, ItemCard, ShowCard, SkeletonGrid } from '../components/common'
import LegalNote from '../components/LegalNote'
import { IconSearch } from '../icons'

// Search with an on-screen keyboard a remote or a controller can reach: letters and digits as keys, with
// Space, Delete and Clear. A real keyboard, or a TV's own, types in the field above it. What's typed is
// kept in the address, so coming back from a result finds the same search.

const KEYS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890'.split('')

export default function Search() {
  const [params, setParams] = useSearchParams()
  const [text, setText] = useState(params.get('q') || '')
  const q = (params.get('q') || '').trim()
  const [items, setItems] = useState<Item[] | null>(null)
  const [shows, setShows] = useState<Show[]>([])
  const [failed, setFailed] = useState('')
  // Results come a moment after the typing stops.
  useEffect(() => {
    const t = setTimeout(() => { if (text.trim() !== q) setParams(text.trim() ? { q: text.trim() } : {}, { replace: true }) }, 300)
    return () => clearTimeout(t)
  }, [text]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setItems(null); setShows([]); setFailed('')
    if (!q) return
    let live = true
    api.get<{ items: Item[] }>(`/items?search=${encodeURIComponent(q)}`).then((r) => { if (live) setItems(r.items) }).catch((e) => { if (live) setFailed(e.message) })
    api.get<{ shows: Show[] }>(`/shows?search=${encodeURIComponent(q)}`).then((r) => { if (live) setShows(r.shows) }).catch(() => {})
    return () => { live = false }
  }, [q])

  const type = (k: string) => setText((t) => t + (t && k !== ' ' ? k.toLowerCase() : k))
  return (
    <div className="tvm-page tvm-search">
      <h1 className="tvm-title">Search</h1>
      <div className="tvm-search-body">
        <div className="tvm-search-side">
          <label className="tvm-search-field">
            <IconSearch />
            <input className="input" type="search" aria-label="Search" placeholder="Movies, shows, music, games…" value={text}
              onChange={(e) => setText(e.target.value)} autoComplete="off" spellCheck={false} />
          </label>
          <div className="tvm-keys" role="group" aria-label="Keyboard" data-nav-group>
            {/* Focus starts on the first key. */}
            {KEYS.map((k, i) => <button key={k} data-tv-first={i === 0 || undefined} type="button" className="tvm-key" onClick={() => type(k)}>{k}</button>)}
            <button type="button" className="tvm-key wide" onClick={() => type(' ')}>Space</button>
            <button type="button" className="tvm-key wide" onClick={() => setText((t) => t.slice(0, -1))}>Delete</button>
            <button type="button" className="tvm-key wide" onClick={() => setText('')}>Clear</button>
          </div>
        </div>
        <div className="tvm-search-results">
          {!q ? <EmptyState icon={<IconSearch />} title="Find anything on this server" text="Movies, shows, episodes, music, books and games are all included." />
            : failed ? <EmptyState icon={<IconSearch />} title="The search didn’t work" text={failed} />
            : !items ? <SkeletonGrid />
            : items.length + shows.length === 0 ? <EmptyState icon={<IconSearch />} title="No results" text={`Nothing on this server matches “${q}”. Try a shorter or different word.`} />
            : (
              <>
                {shows.length > 0 && <><h2 className="shelf-head">Shows</h2><div className="grid">{shows.map((s) => <ShowCard key={s.id} show={s} />)}</div></>}
                {items.length > 0 && <>{shows.length > 0 && <h2 className="shelf-head">Everything else</h2>}<div className="grid">{items.map((it) => <ItemCard key={it.id} item={it} />)}</div></>}
                <LegalNote kind="searchResults" />
              </>
            )}
        </div>
      </div>
    </div>
  )
}
