import { useEffect, useState } from 'react'
import { PageHelp } from '../components/HelpLink'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, artSet, CARD_SIZES, mediaUrl, posterUrl, sized, type Album, type Item, type Library } from '../api'
import { BackLink, EmptyState, ScanButton, SkeletonGrid } from '../components/common'
import { IconBook } from '../icons'
import { useApp } from '../state'
import LegalNote from '../components/LegalNote'
import LibraryHeader from '../components/LibraryHeader'
import { BOOK_FORMATS } from '../mediaTypes.generated'

// Books, comics and manga: the library page and a book's details.

const FORMAT = BOOK_FORMATS
export const readable = (it: Item) => !!it.reader

function progressOf(it: Item) {
  if (!it.progress || !it.runtimeSec) return 0
  return it.progress.watched ? 100 : Math.min(100, (it.progress.positionSec / it.runtimeSec) * 100)
}

export function BookCard({ it }: { it: Item }) {
  const pct = progressOf(it)
  return (
    <Link to={`/item/${it.id}`} className="album-card book-card">
      <div className="album-cover book">
        {it.poster ? <img src={sized(posterUrl(it), 320)} srcSet={artSet(posterUrl(it))} sizes={CARD_SIZES} alt="" loading="lazy" /> : <span><IconBook /></span>}
        {it.reading === 'rtl' && <span className="book-badge">Manga</span>}
        {pct > 0 && <span className="book-prog" style={{ width: pct + '%' }} />}
      </div>
      <b>{it.title}</b>
      <span className="muted">{it.artist || it.album || FORMAT[it.container] || ''}</span>
    </Link>
  )
}

export default function BooksLibrary({ lib }: { lib: Library }) {
  const [params, setParams] = useSearchParams()
  const tab = params.get('tab') || 'books'
  const series = params.get('series')
  const sort = params.get('sort') || 'title'
  const desc = params.get('desc') === '1'
  const [items, setItems] = useState<Item[] | null>(null)
  const [groups, setGroups] = useState<Album[] | null>(null)
  useEffect(() => {
    if (tab === 'books') {
      setItems(null)
      const q = series !== null ? `&sort=track` : `&sort=${sort}&desc=${desc ? 1 : 0}`
      api.get<{ items: Item[] }>(`/items?library=${lib.id}${q}&kind=book`)
        .then((r) => setItems(series !== null ? r.items.filter((x) => (x.album || '') === series) : r.items)).catch(() => setItems([]))
    } else {
      setGroups(null)
      api.get<Album[]>(`/music/albums?library=${lib.id}&sort=title`).then(setGroups).catch(() => setGroups([]))
    }
  }, [lib.id, tab, series, sort, desc])
  const set = (p: Record<string, string | null>) => { const n = new URLSearchParams(params); for (const [k, v] of Object.entries(p)) v === null ? n.delete(k) : n.set(k, v); setParams(n) }
  return (
    <>
      <LibraryHeader
        title={lib.name}
        sub={series !== null ? series || 'No series' : undefined}
        count={tab === 'books' && items ? `${items.length} ${items.length === 1 ? 'book' : 'books'}` : undefined}
        tabs={<div className="seg" role="tablist">
          {[['books', 'Books'], ['series', 'Series']].map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'active' : ''} onClick={() => set({ tab: k, series: null })}>{l}</button>)}
        </div>}
        sorts={tab === 'books' && series === null ? [{ key: 'title', label: 'Title' }, { key: 'added', label: 'Recently added', desc: true }, { key: 'year', label: 'Year', desc: true }] : undefined}
        sort={sort}
        desc={desc}
        onSort={(k, d) => set({ sort: k, desc: d ? '1' : null })}
        onDirection={(d) => set({ desc: d ? '1' : null })}
        actions={<><PageHelp to="books" /><ScanButton lib={lib} /></>}
      />
      {tab === 'books' && (!items ? <SkeletonGrid /> : items.length === 0 ? <EmptyState icon={<IconBook />} title="No books yet" text="Put EPUB, PDF or CBZ files in this library's folders and scan it." /> : (
        <div className="album-grid">{items.map((it) => <BookCard key={it.id} it={it} />)}</div>
      ))}
      {tab === 'series' && (!groups ? <SkeletonGrid /> : (
        <div className="album-grid">
          {groups.filter((g) => g.title).map((g) => (
            <button key={g.artist + '|' + g.title} className="album-card" onClick={() => set({ tab: 'books', series: g.title })}>
              <div className="album-cover book">{g.coverItem ? <img src={mediaUrl(`/api/items/${g.coverItem}/poster`)} alt="" loading="lazy" /> : <span><IconBook /></span>}</div>
              <b>{g.title}</b><span className="muted">{g.tracks} {g.tracks === 1 ? 'book' : 'books'}</span>
            </button>
          ))}
        </div>
      ))}
      <LegalNote kind="reader" />
    </>
  )
}

// BookDetails is the item page of a book.
export function BookDetails({ it }: { it: Item }) {
  const nav = useNavigate()
  const pct = progressOf(it)
  const started = pct > 0 && pct < 100
  const libName = useApp().libraries.find((l) => l.id === it.libraryId)?.name || 'Library'
  return (
    <div className="album-page">
      <BackLink to={`/library/${it.libraryId}`} label={libName} />
      <div className="album-head">
        <div className="album-cover big book">{it.poster ? <img src={posterUrl(it)} alt="" /> : <span><IconBook /></span>}</div>
        <div className="album-meta">
          <span className="muted">{it.reading === 'rtl' ? 'Manga' : it.reader === 'cbz' ? 'Comic' : 'Book'} · {FORMAT[it.container] || it.container}</span>
          <h1>{it.title}</h1>
          {it.artist && <span className="album-artist">{it.artist}</span>}
          <div className="muted">{[it.album && it.track ? `${it.album} #${it.track}` : it.album, it.year, it.runtimeSec ? `${it.runtimeSec} ${it.reader === 'epub' ? 'chapters' : 'pages'}` : ''].filter(Boolean).join(' · ')}</div>
          {pct > 0 && <div className="book-progress"><span style={{ width: pct + '%' }} /></div>}
          <div className="actions">
            {readable(it)
              ? <button data-tv-first className="btn primary" onClick={() => nav(`/read/${it.id}`)}><IconBook /> {started ? 'Continue reading' : pct >= 100 ? 'Read again' : 'Read'}</button>
              : <span className="muted">This format can't be read in the browser. Convert it to EPUB or CBZ, or download it.</span>}
            <a className="btn ghost" href={mediaUrl(`/api/items/${it.id}/file`)} download>Download</a>
          </div>
          {it.overview && <p className="album-desc">{it.overview}</p>}
          <LegalNote kind="reader" />
        </div>
      </div>
    </div>
  )
}
