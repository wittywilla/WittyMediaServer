import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, posterUrl, type Item, type Show } from '../api'
import { useAudio } from '../audio/player'
import { BackLink, EmptyState, ItemCard, ShowCard, Spinner } from '../components/common'
import { canPost, type Collection, type CollectionRule } from '../gallery'
import { IconClose, IconEdit, IconLibrary, IconPlay, IconPlus, IconTrash } from '../icons'
import { toast, useApp } from '../state'
import '../gallery.css'

// Collections and smart playlists, across every library. A smart playlist is a
// saved rule the server works out each time (docs/COLLECTIONS.md); this page
// lists them and builds the rule.

export default function CollectionsPage() {
  const { user } = useApp()
  const [list, setList] = useState<Collection[] | null>(null)
  const [making, setMaking] = useState(false)
  const load = useCallback(() => { api.get<Collection[]>('/collections').then(setList).catch(() => setList([])) }, [])
  useEffect(load, [load])
  if (!list) return <Spinner />
  return (
    <>
      <div className="toolbar">
        <h1>Collections</h1>
        <div style={{ flex: 1 }} />
        {canPost(user) && <button className="btn sm primary" onClick={() => setMaking(true)}><IconPlus /> New</button>}
      </div>
      {list.length === 0 ? (
        <EmptyState icon={<IconLibrary />} title="No collections yet" text="Gather movies, shows, albums and more from any library, or save a rule, such as unwatched movies from the 90s, as a smart playlist that keeps itself up to date." />
      ) : (
        <div className="gal-album-grid">
          {list.map((c) => (
            <Link key={c.id} to={`/collections/${c.id}`} className="gal-album">
              <span className="gal-album-img">
                {c.coverId ? <img src={`/api/items/${c.coverId}/poster?w=480`} alt="" loading="lazy" onError={(e) => ((e.target as HTMLImageElement).style.visibility = 'hidden')} />
                  : c.coverShowId ? <img src={`/api/shows/${c.coverShowId}/art/poster`} alt="" loading="lazy" /> : <IconPlay />}
              </span>
              <b>{c.name}</b>
              <span className="muted">{[c.kind === 'smart' ? 'Smart' : '', `${c.count} ${c.count === 1 ? 'item' : 'items'}`, c.ownerId !== user?.id ? `from ${c.owner}` : c.shared ? 'shared' : ''].filter(Boolean).join(' · ')}</span>
            </Link>
          ))}
        </div>
      )}
      {making && <CollectionDialog onClose={() => setMaking(false)} onSaved={() => { setMaking(false); load() }} />}
    </>
  )
}

export function CollectionPage() {
  const { id } = useParams()
  const { user } = useApp()
  const nav = useNavigate()
  const audio = useAudio()
  const [c, setC] = useState<Collection | null>(null)
  const [items, setItems] = useState<Item[] | null>(null)
  const [shows, setShows] = useState<Show[]>([])
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState(false)
  const load = useCallback(() => {
    api.get<{ collection: Collection; items: Item[]; shows: Show[] }>(`/collections/${id}`).then((r) => { setC(r.collection); setItems(r.items); setShows(r.shows || []) }).catch((e) => setErr(e.message))
  }, [id])
  useEffect(load, [load])
  if (err) return <EmptyState title="Collection not found" text={err} />
  if (!c || !items) return <Spinner />
  const own = c.ownerId === user?.id || user?.isAdmin
  const playAll = () => {
    const songs = items.filter((x) => x.kind === 'audio')
    if (songs.length && songs.length === items.length) { audio.play(songs); return }
    const first = items.find((x) => x.kind === 'video' || x.kind.startsWith('dvd') || x.kind.startsWith('bluray'))
    if (first) nav(`/play/${first.id}?collection=${c.id}`)
  }
  const remove = async () => {
    if (!confirm(`Delete “${c.name}”? What's in it stays in its library.`)) return
    await api.del(`/collections/${c.id}`)
    nav('/collections', { replace: true })
  }
  const take = async (body: { items?: number[]; shows?: number[] }) => {
    try {
      const res = await fetch(`/api/collections/${c.id}/items`, { method: 'DELETE', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...(localStorage.getItem('wms.token') ? { Authorization: 'Bearer ' + localStorage.getItem('wms.token') } : {}) }, body: JSON.stringify(body) })
      if (!res.ok) throw new Error()
      load()
    } catch { toast("Couldn't change this collection") }
  }
  return (
    <>
      <BackLink to="/collections" label="Collections" />
      <div className="toolbar">
        <h1>{c.name}</h1>
        <span className="count">{c.count} {c.count === 1 ? 'item' : 'items'}{c.kind === 'smart' ? ' · smart' : ''}</span>
        <div style={{ flex: 1 }} />
        {items.length > 0 && <button className="btn sm primary" onClick={playAll}><IconPlay /> Play all</button>}
        {own && <button className="btn sm ghost" onClick={() => setEdit(true)}><IconEdit /> Edit</button>}
        {own && !c.system && <button className="btn sm ghost" onClick={remove}><IconTrash /> Delete</button>}
      </div>
      {c.description && <p className="muted" style={{ marginTop: -6 }}>{c.description}</p>}
      {items.length + shows.length === 0 ? <EmptyState icon={<IconLibrary />} title="Nothing here yet" text={c.kind === 'smart' ? 'Nothing matches this rule at the moment.' : 'Use Save on a movie, show or video to add it.'} /> : (
        <div className="grid">
          {shows.map((s) => <div key={'s' + s.id} className="col-cell"><ShowCard show={s} />{own && c.kind !== 'smart' && <button className="btn sm ghost col-x" aria-label={`Remove ${s.title}`} onClick={() => take({ shows: [s.id] })}><IconClose /></button>}</div>)}
          {items.map((it) => <div key={it.id} className="col-cell"><ItemCard item={it} />{own && c.kind !== 'smart' && <button className="btn sm ghost col-x" aria-label={`Remove ${it.title}`} onClick={() => take({ items: [it.id] })}><IconClose /></button>}</div>)}
        </div>
      )}
      {edit && <CollectionDialog collection={c} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load() }} />}
    </>
  )
}

const TYPES: [string, string][] = [['movies', 'Movies'], ['shows', 'TV shows'], ['anime', 'Anime'], ['discs', 'Discs'], ['videos', 'Home videos'], ['music', 'Music'], ['musicvideos', 'Music videos'], ['audiobooks', 'Audiobooks'], ['podcasts', 'Podcasts'], ['books', 'Books'], ['channels', 'Creator videos'], ['shorts', 'Vertical videos'], ['photos', 'Photos']]

function CollectionDialog({ collection, onClose, onSaved }: { collection?: Collection; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(collection?.name || '')
  const [desc, setDesc] = useState(collection?.description || '')
  const [shared, setShared] = useState(collection?.shared || false)
  const [kind, setKind] = useState<'manual' | 'smart'>(collection?.kind === 'smart' ? 'smart' : 'manual')
  const [rule, setRule] = useState<CollectionRule>(collection?.rule || { sort: 'added', desc: true, limit: 100 })
  const [preview, setPreview] = useState<{ count: number; items: Item[] } | null>(null)
  const [err, setErr] = useState('')
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const up = (p: Partial<CollectionRule>) => setRule((r) => ({ ...r, ...p }))
  useEffect(() => {
    if (kind !== 'smart') return
    clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      api.post<{ count: number; items: Item[] }>('/collections/preview', { rule }).then(setPreview).catch(() => setPreview(null))
    }, 300)
    return () => clearTimeout(timer.current)
  }, [rule, kind])
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setErr('')
    const body = { name, description: desc, kind, shared, rule: kind === 'smart' ? rule : undefined }
    try {
      if (collection) await api.put(`/collections/${collection.id}`, body)
      else await api.post('/collections', body)
      onSaved()
    } catch (x: any) { setErr(x.message) }
  }
  const num = (v: string) => (v === '' ? undefined : Number(v))
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal wide col-dialog" role="dialog" aria-modal="true" aria-labelledby="col-h" onClick={(e) => e.stopPropagation()} onSubmit={save}>
        <h2 id="col-h">{collection ? 'Edit collection' : 'New collection'}</h2>
        <label className="oobe-field">Name<input className="input" autoFocus required maxLength={200} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className="oobe-field">Description<input className="input" maxLength={4000} value={desc} onChange={(e) => setDesc(e.target.value)} /></label>
        {!collection && (
          <div className="seg" role="radiogroup" aria-label="Kind">
            <button type="button" role="radio" aria-checked={kind === 'manual'} className={kind === 'manual' ? 'active' : ''} onClick={() => setKind('manual')}>A list you fill</button>
            <button type="button" role="radio" aria-checked={kind === 'smart'} className={kind === 'smart' ? 'active' : ''} onClick={() => setKind('smart')}>Smart playlist</button>
          </div>
        )}
        {kind === 'smart' && (
          <fieldset className="col-rule">
            <legend>Everything that matches</legend>
            <div className="col-types" role="group" aria-label="From these kinds of library (none means all)">
              {TYPES.map(([k, l]) => {
                const on = rule.libraryTypes?.includes(k)
                return <button key={k} type="button" className={'ch-tag' + (on ? ' on' : '')} aria-pressed={!!on} onClick={() => up({ libraryTypes: on ? rule.libraryTypes?.filter((x) => x !== k) : [...(rule.libraryTypes || []), k] })}>{l}</button>
              })}
            </div>
            <div className="col-fields">
              <label>Genres (any)<input className="input" placeholder="Comedy, Drama" defaultValue={rule.genres?.join(', ')} onChange={(e) => up({ genres: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })} /></label>
              <label>Words in the title<input className="input" value={rule.search || ''} onChange={(e) => up({ search: e.target.value })} /></label>
              <label>Artist or creator<input className="input" value={rule.artist || ''} onChange={(e) => up({ artist: e.target.value })} /></label>
              <label>From year<input className="input" type="number" min={1800} max={3000} value={rule.yearFrom ?? ''} onChange={(e) => up({ yearFrom: num(e.target.value) })} /></label>
              <label>To year<input className="input" type="number" min={1800} max={3000} value={rule.yearTo ?? ''} onChange={(e) => up({ yearTo: num(e.target.value) })} /></label>
              <label>Rating at least<input className="input" type="number" min={0} max={10} step={0.5} value={rule.ratingMin ?? ''} onChange={(e) => up({ ratingMin: num(e.target.value) })} /></label>
              <label>Added in the last (days)<input className="input" type="number" min={0} value={rule.addedWithinDays ?? ''} onChange={(e) => up({ addedWithinDays: num(e.target.value) })} /></label>
              <label>Watched<select className="select" value={rule.watched || ''} onChange={(e) => up({ watched: e.target.value as CollectionRule['watched'] })}><option value="">Either</option><option value="no">Not watched</option><option value="started">Started</option><option value="yes">Watched</option></select></label>
              <label>Sort by<select className="select" value={rule.sort || 'added'} onChange={(e) => up({ sort: e.target.value as CollectionRule['sort'] })}><option value="added">Date added</option><option value="title">Title</option><option value="year">Year</option><option value="rating">Rating</option><option value="runtime">Length</option><option value="played">Most played</option><option value="random">Shuffle</option></select></label>
              <label>Order<select className="select" value={rule.desc ? 'd' : 'a'} onChange={(e) => up({ desc: e.target.value === 'd' })}><option value="d">Highest or newest first</option><option value="a">Lowest or oldest first</option></select></label>
              <label>At most<input className="input" type="number" min={1} max={500} value={rule.limit ?? 100} onChange={(e) => up({ limit: num(e.target.value) })} /></label>
            </div>
            <div className="col-checks">
              <label className="share-check"><input type="checkbox" checked={!!rule.liked} onChange={(e) => up({ liked: e.target.checked })} /> Only what I liked</label>
              <label className="share-check"><input type="checkbox" checked={!!rule.episodes} onChange={(e) => up({ episodes: e.target.checked })} /> Include TV episodes</label>
            </div>
            <div className="col-preview" aria-live="polite">
              {preview && <b>{preview.count} {preview.count === 1 ? 'match' : 'matches'}</b>}
              <div className="col-preview-row">{preview?.items.filter((it) => it.poster).slice(0, 8).map((it) => <img key={it.id} src={posterUrl(it)} alt={it.title} title={it.title} loading="lazy" onError={(e) => ((e.target as HTMLImageElement).style.visibility = 'hidden')} />)}</div>
            </div>
          </fieldset>
        )}
        {!collection?.system && <label className="share-check"><input type="checkbox" checked={shared} onChange={(e) => setShared(e.target.checked)} /> Share with the other profiles; each sees what their own limits allow</label>}
        {err && <div className="error" role="alert">{err}</div>}
        <div className="actions"><button type="button" className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary">{collection ? 'Save' : 'Make it'}</button></div>
      </form>
    </div>
  )
}
