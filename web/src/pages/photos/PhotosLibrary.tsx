import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, type Library } from '../../api'
import { EmptyState, Spinner } from '../../components/common'
import { canPost, dayKey, fmtSize, monthName, photoThumb, takenDate, uploadFile, type Photo, type PhotoAlbum, type PhotoFolder, type PhotoMonth, type UploadResult } from '../../gallery'
import { IconCheck, IconClose, IconImage, IconPlay, IconPlus, IconRefresh, IconTag } from '../../icons'
import ShareDialog from '../../components/ShareDialog'
import { toast, useApp } from '../../state'
import PhotoMap from './PhotoMap'
import PhotoViewer from './PhotoViewer'
import '../../gallery.css'

// WittyPhotos: the gallery. The timeline loads a month at a time as it comes
// into view, so a library of any size costs what is on screen. All the lists
// and rules are on the server (docs/PHOTOS.md); this draws them.

const TABS: [string, string][] = [['timeline', 'Timeline'], ['albums', 'Albums'], ['favorites', 'Favourites'], ['folders', 'Folders'], ['map', 'Map'], ['people', 'People & places']]

export default function PhotosLibrary({ lib }: { lib: Library }) {
  const [params, setParams] = useSearchParams()
  const { user } = useApp()
  const tab = params.get('tab') || 'timeline'
  const albumId = params.get('album')
  const folder = params.get('folder')
  const [list, setList] = useState<Photo[]>([]) // what the viewer can step through
  const viewId = params.get('view') ? Number(params.get('view')) : null
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [adding, setAdding] = useState(false)
  const [refresh, setRefresh] = useState(0)
  const [search, setSearch] = useState('')
  const set = (p: Record<string, string | null>, replace = false) => {
    const n = new URLSearchParams(params)
    for (const [k, v] of Object.entries(p)) v === null ? n.delete(k) : n.set(k, v)
    setParams(n, { replace })
  }
  const open = useCallback((id: number) => set({ view: String(id) }), [params]) // eslint-disable-line react-hooks/exhaustive-deps
  const closeViewer = () => {
    const last = viewId
    if ((window.history.state?.idx ?? 0) > 0) window.history.back(); else set({ view: null }, true)
    // Back on the grid, at the picture just seen: where a remote user expects to be.
    setTimeout(() => {
      const tile = document.querySelector<HTMLElement>(`.gal-tile[data-id="${last}"]`)
      tile?.focus({ preventScroll: true })
      tile?.scrollIntoView({ block: 'nearest' })
    }, 60)
  }
  const toggle = (id: number) => setSelected((s) => { const n = new Set(s); n.has(id) ? n.delete(id) : n.add(id); return n })
  const stopSelecting = () => { setSelecting(false); setSelected(new Set()) }
  const setTab = (k: string) => { stopSelecting(); setSearch(''); set({ tab: k === 'timeline' ? null : k, album: null, folder: null }) }
  const q = useMemo(() => {
    const u = new URLSearchParams({ library: String(lib.id) })
    if (tab === 'favorites') u.set('favorites', '1')
    if (albumId) u.set('album', albumId)
    if (folder !== null) u.set('folder', folder)
    if (search) u.set('search', search)
    return u.toString()
  }, [lib.id, tab, albumId, folder, search])
  const showGrid = tab === 'timeline' || tab === 'favorites' || albumId !== null || folder !== null
  return (
    <div className="gal">
      <div className="toolbar gal-bar">
        <h1>{lib.name}</h1>
        <div className="seg gal-tabs" role="tablist" aria-label="View">
          {TABS.map(([k, l]) => <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'active' : ''} onClick={() => setTab(k)}>{l}</button>)}
        </div>
        <div style={{ flex: 1 }} />
        {showGrid && !albumId && folder === null && (
          <input className="input gal-search" type="search" placeholder="Search names, folders, cameras" aria-label="Search this library" defaultValue={search}
            onKeyDown={(e) => { if (e.key === 'Enter') setSearch((e.target as HTMLInputElement).value.trim()) }} onBlur={(e) => setSearch(e.target.value.trim())} />
        )}
        {showGrid && canPost(user) && <button className={'btn sm ' + (selecting ? 'primary' : 'ghost')} onClick={() => (selecting ? stopSelecting() : setSelecting(true))}>{selecting ? 'Done' : 'Select'}</button>}
        {canPost(user) && <UploadButton lib={lib} onDone={() => setRefresh((r) => r + 1)} />}
        {user?.isAdmin && <ScanButton lib={lib} onDone={() => setRefresh((r) => r + 1)} />}
      </div>
      {selecting && (
        <div className="gal-selbar" role="toolbar" aria-label="Selected pictures">
          <b>{selected.size} selected</b>
          <button className="btn sm" disabled={!selected.size} onClick={() => setAdding(true)}><IconPlus /> Add to album</button>
          {albumId && <RemoveFromAlbum album={Number(albumId)} ids={[...selected]} onDone={() => { stopSelecting(); setRefresh((r) => r + 1) }} />}
          <button className="btn sm ghost" onClick={stopSelecting}><IconClose /> Clear</button>
        </div>
      )}
      {tab === 'albums' && !albumId && <AlbumsTab lib={lib} refresh={refresh} onOpen={(id) => set({ album: String(id) })} />}
      {albumId && <AlbumHeader id={Number(albumId)} lib={lib} onBack={() => set({ album: null })} onChanged={() => setRefresh((r) => r + 1)} />}
      {tab === 'folders' && folder === null && <FoldersTab lib={lib} onOpen={(f) => set({ folder: f })} />}
      {folder !== null && <div className="gal-sub"><button className="btn sm ghost" onClick={() => set({ folder: null })}>‹ Folders</button><h2>{folder || 'Top folder'}</h2></div>}
      {tab === 'map' && <PhotoMap lib={lib} onOpen={open} onList={setList} />}
      {tab === 'people' && <PeoplePlaces />}
      {showGrid && <Timeline key={q + '|' + refresh} query={q} open={open} selecting={selecting} selected={selected} toggle={toggle} onList={setList}
        empty={tab === 'favorites' ? 'No favourites yet' : albumId ? 'This album is empty' : search ? 'Nothing matched' : 'No pictures yet'} />}
      {viewId !== null && <PhotoViewer key={viewId} id={viewId} list={list} lib={lib} onOpen={(id) => set({ view: String(id) }, true)} onClose={closeViewer} onChanged={() => setRefresh((r) => r + 1)} />}
      {adding && <AddToAlbum lib={lib} ids={[...selected]} onClose={() => setAdding(false)} onDone={() => { setAdding(false); stopSelecting(); setRefresh((r) => r + 1) }} />}
    </div>
  )
}

// ---------------------------------------------------------------- timeline

const ROW_HEIGHT = 190

function Timeline({ query, open, selecting, selected, toggle, onList, empty }: {
  query: string; open: (id: number) => void; selecting: boolean; selected: Set<number>; toggle: (id: number) => void; onList: (l: Photo[]) => void; empty: string
}) {
  const [months, setMonths] = useState<PhotoMonth[] | null>(null)
  const [loaded, setLoaded] = useState<Record<number, Photo[]>>({})
  useEffect(() => { api.get<PhotoMonth[]>(`/photos/months?${query}`).then(setMonths).catch(() => setMonths([])) }, [query])
  const load = useCallback(async (ym: number) => {
    const all: Photo[] = []
    let cursor = ''
    // A month is at most a few thousand pictures; it arrives in pages.
    do {
      const r = await api.get<{ photos: Photo[]; nextCursor: string }>(`/photos/list?${query}&ym=${ym}&limit=500${cursor ? `&cursor=${cursor}` : ''}`)
      all.push(...r.photos)
      cursor = r.nextCursor
    } while (cursor)
    setLoaded((l) => ({ ...l, [ym]: all }))
  }, [query])
  useEffect(() => {
    if (!months) return
    onList(months.flatMap((m) => loaded[m.ym] || []))
  }, [months, loaded, onList])
  const rail = useMemo(() => {
    const years = new Map<number, number>()
    for (const m of months || []) { const y = Math.floor(m.ym / 100); if (!years.has(y)) years.set(y, m.ym) }
    return [...years.entries()]
  }, [months])
  if (!months) return <Spinner />
  if (!months.length) return <EmptyState title={empty} text="Pictures appear here from this library's folders, after a scan or an upload." />
  return (
    <div className="gal-timeline">
      <div className="gal-months">
        {months.map((m) => <MonthSection key={m.ym} month={m} photos={loaded[m.ym]} load={load} open={open} selecting={selecting} selected={selected} toggle={toggle} />)}
      </div>
      {rail.length > 1 && (
        <nav className="gal-rail" aria-label="Jump to a year">
          {rail.map(([y, ym]) => <button key={y} onClick={() => document.getElementById(`gm-${ym}`)?.scrollIntoView({ behavior: 'smooth' })}>{y}</button>)}
        </nav>
      )}
    </div>
  )
}

function MonthSection({ month, photos, load, open, selecting, selected, toggle }: {
  month: PhotoMonth; photos?: Photo[]; load: (ym: number) => void; open: (id: number) => void; selecting: boolean; selected: Set<number>; toggle: (id: number) => void
}) {
  const ref = useRef<HTMLElement | null>(null)
  const asked = useRef(false)
  const ask = useCallback(() => { if (!asked.current) { asked.current = true; load(month.ym) } }, [load, month.ym])
  useEffect(() => {
    const el = ref.current
    if (!el || photos) return
    const io = new IntersectionObserver((e) => { if (e[0].isIntersecting) ask() }, { rootMargin: '800px' })
    io.observe(el)
    return () => io.disconnect()
  }, [photos, ask])
  const days = useMemo(() => {
    const out: { day: number; items: Photo[] }[] = []
    for (const p of photos || []) {
      const d = dayKey(p.takenAt)
      if (out.length && out[out.length - 1].day === d) out[out.length - 1].items.push(p)
      else out.push({ day: d, items: [p] })
    }
    return out
  }, [photos])
  // Until it loads, the month holds about the room it will take, so the scroll
  // bar means something. The heading doubles as a way in with a D-pad.
  const estimate = Math.ceil(month.count / 6) * (ROW_HEIGHT + 4)
  return (
    <section className="gal-month" id={`gm-${month.ym}`} ref={ref} aria-label={monthName(month.ym)}>
      <h2 className="gal-month-h"><button className="gal-month-btn" onClick={ask} onFocus={ask}>{monthName(month.ym)}</button><span className="muted">{month.count.toLocaleString()}</span></h2>
      {!photos ? <div className="gal-month-wait" style={{ height: estimate }} /> : days.map((d) => (
        <div key={d.day} className="gal-day">
          <h3>{takenDate(d.items[0].takenAt)}</h3>
          <div className="gal-grid">{d.items.map((p) => <Tile key={p.id} p={p} open={open} selecting={selecting} on={selected.has(p.id)} toggle={toggle} />)}</div>
        </div>
      ))}
    </section>
  )
}

function Tile({ p, open, selecting, on, toggle }: { p: Photo; open: (id: number) => void; selecting: boolean; on: boolean; toggle: (id: number) => void }) {
  const aspect = p.width && p.height ? Math.min(3, Math.max(0.5, p.width / p.height)) : 1.4
  const [failed, setFailed] = useState(false)
  // Row-filling without measuring: each tile grows in proportion to its shape.
  return (
    <button type="button" data-id={p.id} className={'gal-tile' + (on ? ' on' : '')} style={{ flexGrow: aspect, flexBasis: aspect * ROW_HEIGHT }} aria-pressed={selecting ? on : undefined}
      aria-label={`${p.kind === 'video' ? 'Video' : 'Picture'} ${p.title}, ${takenDate(p.takenAt)}`} onClick={() => (selecting ? toggle(p.id) : open(p.id))}>
      {failed ? <span className="gal-noimg"><IconImage /><small>{p.title}</small></span>
        : <img src={photoThumb(p.id, 480, p.updatedAt)} alt="" loading="lazy" decoding="async" onError={() => setFailed(true)} />}
      {p.kind === 'video' && <span className="gal-badge"><IconPlay size={14} /></span>}
      {p.motion && <span className="gal-badge">LIVE</span>}
      {p.raw && <span className="gal-badge corner">RAW</span>}
      {p.favorite && <span className="gal-fav" aria-hidden="true">♥</span>}
      {selecting && <span className="gal-check" aria-hidden="true">{on && <IconCheck size={16} />}</span>}
    </button>
  )
}

// ---------------------------------------------------------------- albums

function AlbumCover({ id, name, sub, onClick }: { id: number; name: string; sub: string; onClick: () => void }) {
  return (
    <button type="button" className="gal-album" onClick={onClick}>
      <span className="gal-album-img">{id ? <img src={photoThumb(id, 480)} alt="" loading="lazy" /> : <IconImage />}</span>
      <b>{name}</b><span className="muted">{sub}</span>
    </button>
  )
}

function AlbumsTab({ lib, refresh, onOpen }: { lib: Library; refresh: number; onOpen: (id: number) => void }) {
  const { user } = useApp()
  const [albums, setAlbums] = useState<PhotoAlbum[] | null>(null)
  const [making, setMaking] = useState(false)
  const load = useCallback(() => { api.get<PhotoAlbum[]>(`/photos/albums?library=${lib.id}`).then(setAlbums).catch(() => setAlbums([])) }, [lib.id])
  useEffect(load, [load, refresh])
  if (!albums) return <Spinner />
  return (
    <>
      <div className="gal-album-grid">
        {canPost(user) && (
          <button type="button" className="gal-album new" onClick={() => setMaking(true)}><span className="gal-album-img"><IconPlus /></span><b>New album</b><span className="muted">Just for you, or shared</span></button>
        )}
        {albums.map((a) => <AlbumCover key={a.id} id={a.coverId} name={a.name} sub={`${a.count} ${a.count === 1 ? 'item' : 'items'}${a.ownerId !== user?.id ? ` · from ${a.owner}` : a.shared ? ' · shared' : ''}`} onClick={() => onOpen(a.id)} />)}
      </div>
      {!albums.length && !canPost(user) && <EmptyState title="No albums yet" />}
      {making && <AlbumDialog lib={lib} onClose={() => setMaking(false)} onSaved={() => { setMaking(false); load() }} />}
    </>
  )
}

function AlbumDialog({ lib, album, onClose, onSaved }: { lib: Library; album?: PhotoAlbum; onClose: () => void; onSaved: (a?: PhotoAlbum) => void }) {
  const [name, setName] = useState(album?.name || '')
  const [desc, setDesc] = useState(album?.description || '')
  const [shared, setShared] = useState(album?.shared || false)
  const [err, setErr] = useState('')
  const save = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      if (album) { await api.put(`/photos/albums/${album.id}`, { name, description: desc, shared }); onSaved() }
      else onSaved(await api.post<PhotoAlbum>('/photos/albums', { libraryId: lib.id, name, description: desc, shared }))
    } catch (x: any) { setErr(x.message) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal" role="dialog" aria-modal="true" aria-labelledby="al-h" onClick={(e) => e.stopPropagation()} onSubmit={save}>
        <h2 id="al-h">{album ? 'Album details' : 'New album'}</h2>
        <label className="oobe-field">Name<input className="input" autoFocus required maxLength={200} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className="oobe-field">Description<textarea className="input" maxLength={4000} rows={3} value={desc} onChange={(e) => setDesc(e.target.value)} /></label>
        <label className="share-check"><input type="checkbox" checked={shared} onChange={(e) => setShared(e.target.checked)} /> Share with other profiles who can open this library</label>
        {err && <div className="error" role="alert">{err}</div>}
        <div className="actions"><button type="button" className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary">{album ? 'Save' : 'Make album'}</button></div>
      </form>
    </div>
  )
}

function AlbumHeader({ id, lib, onBack, onChanged }: { id: number; lib: Library; onBack: () => void; onChanged: () => void }) {
  const { user } = useApp()
  const [a, setA] = useState<PhotoAlbum | null>(null)
  const [edit, setEdit] = useState(false)
  const [share, setShare] = useState(false)
  const load = useCallback(() => { api.get<PhotoAlbum[]>(`/photos/albums?library=${lib.id}`).then((l) => setA(l.find((x) => x.id === id) || null)).catch(() => {}) }, [id, lib.id])
  useEffect(load, [load])
  const own = a && (a.ownerId === user?.id || user?.isAdmin)
  const remove = async () => {
    if (!a || !confirm(`Delete the album “${a.name}”? The pictures stay in the library.`)) return
    await api.del(`/photos/albums/${a.id}`)
    onBack(); onChanged()
  }
  return (
    <div className="gal-sub">
      <button className="btn sm ghost" onClick={onBack}>‹ Albums</button>
      <div className="grow"><h2>{a?.name || ''}</h2>{a?.description && <p className="muted">{a.description}</p>}</div>
      {a && canPost(user) && <button className="btn sm" onClick={() => setShare(true)}>Share</button>}
      {own && <button className="btn sm ghost" onClick={() => setEdit(true)}>Details</button>}
      {own && <button className="btn sm ghost" onClick={remove}>Delete album</button>}
      {edit && a && <AlbumDialog lib={lib} album={a} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); load(); onChanged() }} />}
      {share && a && <ShareDialog kind="album" id={a.id} title={a.name} internalPath={`/library/${lib.id}?tab=albums&album=${a.id}`} onClose={() => setShare(false)} />}
    </div>
  )
}

function AddToAlbum({ lib, ids, onClose, onDone }: { lib: Library; ids: number[]; onClose: () => void; onDone: () => void }) {
  const { user } = useApp()
  const [albums, setAlbums] = useState<PhotoAlbum[] | null>(null)
  const [making, setMaking] = useState(false)
  useEffect(() => { api.get<PhotoAlbum[]>(`/photos/albums?library=${lib.id}`).then(setAlbums).catch(() => setAlbums([])) }, [lib.id])
  const add = async (a: PhotoAlbum) => {
    const r = await api.post<{ added: number }>(`/photos/albums/${a.id}/items`, { ids })
    toast(`${r.added} added to ${a.name}`)
    onDone()
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="ata-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="ata-h">Add to album</h2>
        {!albums ? <Spinner /> : (
          <div className="gal-pick">
            <button className="btn" onClick={() => setMaking(true)}><IconPlus /> New album</button>
            {albums.filter((a) => a.ownerId === user?.id || user?.isAdmin).map((a) => <button key={a.id} className="btn ghost" onClick={() => add(a)}>{a.name} <span className="muted">{a.count}</span></button>)}
          </div>
        )}
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button></div>
        {making && <AlbumDialog lib={lib} onClose={() => setMaking(false)} onSaved={(a) => { setMaking(false); if (a) add(a) }} />}
      </div>
    </div>
  )
}

function RemoveFromAlbum({ album, ids, onDone }: { album: number; ids: number[]; onDone: () => void }) {
  const remove = async () => {
    try {
      const res = await fetch(`/api/photos/albums/${album}/items`, { method: 'DELETE', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...(localStorage.getItem('wms.token') ? { Authorization: 'Bearer ' + localStorage.getItem('wms.token') } : {}) }, body: JSON.stringify({ ids }) })
      if (!res.ok) throw new Error()
      onDone()
    } catch { toast("Couldn't change the album") }
  }
  return <button className="btn sm" disabled={!ids.length} onClick={remove}>Remove from album</button>
}

// ---------------------------------------------------------------- folders

function FoldersTab({ lib, onOpen }: { lib: Library; onOpen: (f: string) => void }) {
  const [list, setList] = useState<PhotoFolder[] | null>(null)
  useEffect(() => { api.get<PhotoFolder[]>(`/photos/folders?library=${lib.id}`).then(setList).catch(() => setList([])) }, [lib.id])
  if (!list) return <Spinner />
  if (!list.length) return <EmptyState title="No folders yet" />
  return <div className="gal-album-grid">{list.map((f) => <AlbumCover key={f.folder} id={f.coverId} name={f.folder || 'Top folder'} sub={`${f.count} ${f.count === 1 ? 'item' : 'items'}`} onClick={() => onOpen(f.folder)} />)}</div>
}

// ---------------------------------------------------------------- local AI placeholder

// Marked plainly as not there yet: nothing here looks at anyone's pictures.
function PeoplePlaces() {
  return (
    <div className="gal-planned" role="note">
      <span className="gal-planned-tag"><IconTag /> Planned</span>
      <h2>People &amp; places need the local AI</h2>
      <p>Finding the people and places in your photos will come with the local AI: it will run on this machine, and nothing will be sent anywhere. It doesn't exist yet, so nothing here looks at your pictures today.</p>
      <p className="muted">Pictures with a location already appear on the Map.</p>
    </div>
  )
}

// ---------------------------------------------------------------- uploads and scans

function UploadButton({ lib, onDone }: { lib: Library; onDone: () => void }) {
  const input = useRef<HTMLInputElement | null>(null)
  const [state, setState] = useState<{ name: string; n: number; of: number; pct: number } | null>(null)
  const [errors, setErrors] = useState<UploadResult[]>([])
  const run = async (files: File[]) => {
    const failed: UploadResult[] = []
    let total = 0
    for (let i = 0; i < files.length; i++) {
      total += files[i].size
      setState({ name: files[i].name, n: i + 1, of: files.length, pct: 0 })
      const r = await uploadFile(lib, files[i], (d) => setState({ name: files[i].name, n: i + 1, of: files.length, pct: d }))
      if (r.error) failed.push(r)
    }
    setState(null); setErrors(failed)
    if (!failed.length) toast(`${files.length} ${files.length === 1 ? 'file' : 'files'} added (${fmtSize(total)})`)
    onDone()
  }
  // Dropping files anywhere on the gallery works too.
  useEffect(() => {
    const over = (e: DragEvent) => { if (e.dataTransfer?.types.includes('Files')) e.preventDefault() }
    const drop = (e: DragEvent) => { if (e.dataTransfer?.files.length) { e.preventDefault(); run([...e.dataTransfer.files]) } }
    window.addEventListener('dragover', over)
    window.addEventListener('drop', drop)
    return () => { window.removeEventListener('dragover', over); window.removeEventListener('drop', drop) }
  })
  if (!lib.paths.length) return null
  return (
    <>
      <input ref={input} type="file" multiple hidden accept="image/*,video/*,.heic,.heif,.avif,.dng,.cr2,.cr3,.nef,.arw,.orf,.rw2,.raf"
        onChange={(e) => { const f = [...(e.target.files || [])]; e.target.value = ''; if (f.length) run(f) }} />
      <button className="btn sm" disabled={!!state} onClick={() => input.current?.click()}><IconPlus /> {state ? `Uploading ${state.n}/${state.of} (${Math.round(state.pct * 100)}%)` : 'Upload'}</button>
      {state && <span className="sr-only" role="status">Uploading {state.name}</span>}
      {errors.length > 0 && (
        <div className="modal-backdrop" onClick={() => setErrors([])}>
          <div className="modal" role="dialog" aria-modal="true" aria-labelledby="ue-h" onClick={(e) => e.stopPropagation()}>
            <h2 id="ue-h">{errors.length} {errors.length === 1 ? 'file' : 'files'} not added</h2>
            <ul className="gal-errs">{errors.map((x, i) => <li key={i}><b>{x.name}</b>: {x.error}</li>)}</ul>
            <div className="actions"><button className="btn primary" onClick={() => setErrors([])}>Close</button></div>
          </div>
        </div>
      )}
    </>
  )
}

function ScanButton({ lib, onDone }: { lib: Library; onDone: () => void }) {
  const [scan, setScan] = useState<{ done?: number; found?: number } | null>(null)
  const was = useRef(false)
  useEffect(() => {
    let t: ReturnType<typeof setTimeout>
    const poll = async () => {
      try {
        const r = await api.get<{ scanning: boolean; progress: any }>(`/libraries/${lib.id}/scan`)
        setScan(r.scanning ? r.progress : null)
        if (was.current && !r.scanning) onDone()
        was.current = r.scanning
        t = setTimeout(poll, r.scanning ? 1500 : 15000)
      } catch { /* ignore */ }
    }
    poll()
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lib.id])
  return <button className="btn sm ghost" disabled={!!scan} onClick={async () => { await api.post(`/libraries/${lib.id}/scan`); setScan({}); was.current = true }}><IconRefresh /> {scan ? `Scanning… ${scan.done ?? 0}/${scan.found ?? '?'}` : 'Scan'}</button>
}
