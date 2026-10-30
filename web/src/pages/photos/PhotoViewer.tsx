import { useEffect, useRef, useState } from 'react'
import { api, mediaUrl, type Library } from '../../api'
import ShareDialog from '../../components/ShareDialog'
import { canPost, canReact, fmtSize, photoFull, photoThumb, setReaction, takenDate, takenTime, type Photo, type PhotoAlbum, type PhotoDetail } from '../../gallery'
import { IconClose, IconInfo, IconLeft, IconPause, IconPlay, IconPlus, IconRight } from '../../icons'
import InlinePlayer from '../../player/InlinePlayer'
import { toast, useApp } from '../../state'

// One picture filling the screen: step through the list, zoom, slideshow, its
// details and its place on the map. With a remote: Left and Right step, Enter
// starts or stops the slideshow, Down reaches the buttons, Back closes.

const SLIDE_SECONDS = [2, 3, 5, 8, 15]

export default function PhotoViewer({ id, list, lib, onOpen, onClose, onChanged }: {
  id: number; list: Photo[]; lib: Library; onOpen: (id: number) => void; onClose: () => void; onChanged: () => void
}) {
  const { user } = useApp()
  const [d, setD] = useState<PhotoDetail | null>(null)
  const [err, setErr] = useState('')
  const [info, setInfo] = useState(false)
  const [zoom, setZoom] = useState(1)
  const [pan, setPan] = useState({ x: 0, y: 0 })
  const [slide, setSlide] = useState(0) // seconds per picture, 0 = stopped
  const [share, setShare] = useState(false)
  const [album, setAlbum] = useState(false)
  const [motion, setMotion] = useState(false)
  const [loadedFull, setLoadedFull] = useState(false)
  const stage = useRef<HTMLDivElement | null>(null)
  const i = list.findIndex((p) => p.id === id)
  const prev = i > 0 ? list[i - 1] : null
  const next = i >= 0 && i + 1 < list.length ? list[i + 1] : null

  useEffect(() => {
    setD(null); setZoom(1); setPan({ x: 0, y: 0 }); setMotion(false); setLoadedFull(false)
    api.get<PhotoDetail>(`/photos/${id}`).then(setD).catch((e) => setErr(e.message))
    stage.current?.focus({ preventScroll: true })
  }, [id])

  // Having the next picture ready makes stepping feel instant.
  useEffect(() => {
    if (next?.kind === 'photo') new Image().src = photoFull(next.id, next.updatedAt)
    if (prev?.kind === 'photo') new Image().src = photoThumb(prev.id, 1280, prev.updatedAt)
  }, [next?.id, prev?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  // Slideshow: a stopped one stays put; clips play to their end instead.
  useEffect(() => {
    if (!slide || d?.kind === 'video') return
    const t = setTimeout(() => { if (next) onOpen(next.id); else if (list.length) onOpen(list[0].id) }, slide * 1000)
    return () => clearTimeout(t)
  }, [slide, id, next?.id, d?.kind]) // eslint-disable-line react-hooks/exhaustive-deps

  const zoomBy = (f: number) => setZoom((z) => { const n = Math.min(8, Math.max(1, z * f)); if (n === 1) setPan({ x: 0, y: 0 }); return n })
  const onKey = (e: React.KeyboardEvent) => {
    if ((e.target as HTMLElement).closest('.modal, .gal-info, input, select, textarea')) return
    const onStage = e.target === stage.current
    switch (e.key) {
      case 'ArrowLeft': if (onStage || zoom === 1) { e.preventDefault(); if (zoom > 1) setPan((p) => ({ ...p, x: p.x + 80 })); else if (prev) onOpen(prev.id) } break
      case 'ArrowRight': if (onStage || zoom === 1) { e.preventDefault(); if (zoom > 1) setPan((p) => ({ ...p, x: p.x - 80 })); else if (next) onOpen(next.id) } break
      case 'ArrowUp': if (onStage && zoom > 1) { e.preventDefault(); setPan((p) => ({ ...p, y: p.y + 80 })) } break
      case 'ArrowDown':
        if (onStage) {
          e.preventDefault()
          if (zoom > 1) setPan((p) => ({ ...p, y: p.y - 80 }))
          else stage.current?.parentElement?.querySelector<HTMLElement>('.gal-v-bar button')?.focus()
        }
        break
      case '+': case '=': zoomBy(1.5); break
      case '-': zoomBy(1 / 1.5); break
      case 'i': setInfo((o) => !o); break
      case 'f': toggleFav(); break
      case ' ': if (onStage) { e.preventDefault(); setSlide((s) => (s ? 0 : 5)) } break
      case 'Enter': if (onStage) { e.preventDefault(); setSlide((s) => (s ? 0 : 5)) } break
      case 'Escape': case 'GoBack': case 'BrowserBack': e.preventDefault(); e.stopPropagation(); onClose(); break
    }
  }
  const toggleFav = async () => {
    if (!d || !canReact(user)) return
    const on = !d.favorite
    setD({ ...d, favorite: on })
    try { await setReaction(d.id, on ? 1 : 0); onChanged() } catch { setD({ ...d, favorite: !on }); toast("Couldn't save that") }
  }
  // Mouse: drag pans a zoomed picture, the wheel zooms. Touch: swipe to step.
  const drag = useRef<{ x: number; y: number; px: number; py: number } | null>(null)
  const swipe = useRef<number | null>(null)
  return (
    <div className="gal-viewer" role="dialog" aria-modal="true" aria-label={d?.title || 'Picture'} onKeyDown={onKey}>
      <div className="gal-stage" ref={stage} tabIndex={0} aria-label="Picture. Left and right arrows step, Enter starts a slideshow."
        onWheel={(e) => { if (d?.kind === 'photo') zoomBy(e.deltaY < 0 ? 1.25 : 0.8) }}
        onDoubleClick={() => setZoom((z) => { if (z > 1) { setPan({ x: 0, y: 0 }); return 1 } return 2.5 })}
        onPointerDown={(e) => { if (zoom > 1) { drag.current = { x: e.clientX, y: e.clientY, px: pan.x, py: pan.y }; (e.target as Element).setPointerCapture?.(e.pointerId) } }}
        onPointerMove={(e) => { if (drag.current) setPan({ x: drag.current.px + e.clientX - drag.current.x, y: drag.current.py + e.clientY - drag.current.y }) }}
        onPointerUp={() => { drag.current = null }}
        onTouchStart={(e) => { swipe.current = e.touches.length === 1 ? e.touches[0].clientX : null }}
        onTouchEnd={(e) => {
          if (swipe.current === null || zoom > 1) return
          const dx = e.changedTouches[0].clientX - swipe.current
          if (dx > 60 && prev) onOpen(prev.id); else if (dx < -60 && next) onOpen(next.id)
        }}>
        {err && <div className="overlay-msg"><div className="box">{err}</div></div>}
        {d?.kind === 'video' && <InlinePlayer id={d.id} durationSec={d.durationSec} onEnded={() => { if (slide && next) onOpen(next.id) }} className="gal-video" />}
        {d?.kind === 'photo' && motion && <video className="gal-img" src={mediaUrl(`/api/items/${d.id}/motion`)} autoPlay muted loop playsInline />}
        {d?.kind === 'photo' && !motion && (
          <div className="gal-img-wrap" style={{ transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})` }}>
            {/* A sharp preview first, then the full picture for those that have one. */}
            <img className="gal-img under" src={photoThumb(d.id, 1280, d.updatedAt)} alt="" style={{ visibility: loadedFull ? 'hidden' : 'visible' }} />
            <img className="gal-img" src={photoFull(d.id, d.updatedAt)} alt={d.title} draggable={false} onLoad={() => setLoadedFull(true)} />
          </div>
        )}
        {prev && <button className="gal-nav prev" onClick={() => onOpen(prev.id)} aria-label="Previous picture"><IconLeft size={32} /></button>}
        {next && <button className="gal-nav next" onClick={() => onOpen(next.id)} aria-label="Next picture"><IconRight size={32} /></button>}
      </div>
      <div className="gal-v-bar" role="toolbar" aria-label="Picture actions">
        <button className="ibtn" onClick={onClose} aria-label="Close" title="Close (Esc)"><IconClose /></button>
        <div className="gal-v-title"><b>{d?.title}</b>{d && <span>{takenDate(d.takenAt)}, {takenTime(d.takenAt)}</span>}</div>
        {canReact(user) && <button className={'ibtn' + (d?.favorite ? ' on' : '')} onClick={toggleFav} aria-pressed={!!d?.favorite} aria-label="Favourite" title="Favourite (F)">{d?.favorite ? '♥' : '♡'}</button>}
        {d?.motion && <button className={'btn sm' + (motion ? ' primary' : '')} onClick={() => setMotion(!motion)} aria-pressed={motion}>Live</button>}
        {canPost(user) && <button className="ibtn" onClick={() => setAlbum(true)} aria-label="Add to album" title="Add to album"><IconPlus /></button>}
        {canPost(user) && <button className="btn sm" onClick={() => setShare(true)}>Share</button>}
        <select className="select sm gal-slide" value={slide} aria-label="Slideshow" onChange={(e) => setSlide(Number(e.target.value))}>
          <option value={0}>Slideshow</option>
          {SLIDE_SECONDS.map((s) => <option key={s} value={s}>{s} s per picture</option>)}
        </select>
        <button className="ibtn" onClick={() => setSlide((s) => (s ? 0 : 5))} aria-label={slide ? 'Stop the slideshow' : 'Start the slideshow'}>{slide ? <IconPause /> : <IconPlay />}</button>
        <button className="ibtn" onClick={() => zoomBy(1.5)} aria-label="Zoom in" title="Zoom in (+)">＋</button>
        <button className="ibtn" onClick={() => zoomBy(1 / 1.5)} aria-label="Zoom out" title="Zoom out (-)">－</button>
        <button className={'ibtn' + (info ? ' on' : '')} onClick={() => setInfo(!info)} aria-pressed={info} aria-label="Details" title="Details (I)"><IconInfo /></button>
      </div>
      {info && d && <InfoPanel d={d} onClose={() => setInfo(false)} />}
      {share && d && <ShareDialog kind="item" id={d.id} title={d.title} internalPath={`/library/${lib.id}?view=${d.id}`} onClose={() => setShare(false)} />}
      {album && d && <QuickAlbum d={d} lib={lib} onClose={() => setAlbum(false)} onChanged={onChanged} />}
    </div>
  )
}

function InfoPanel({ d, onClose }: { d: PhotoDetail; onClose: () => void }) {
  const camera = [d.make && d.model?.startsWith(d.make) ? '' : d.make, d.model].filter(Boolean).join(' ')
  const exposure = [d.fNumber ? `f/${d.fNumber.toFixed(1)}` : '', d.exposure ? `${d.exposure} s` : '', d.iso ? `ISO ${d.iso}` : '', d.focalMm ? `${Math.round(d.focalMm)} mm` : ''].filter(Boolean).join(' · ')
  return (
    <aside className="gal-info" aria-label="Details">
      <div className="gal-info-head"><h2>Details</h2><button className="ibtn" onClick={onClose} aria-label="Close details"><IconClose /></button></div>
      <dl>
        <dt>Taken</dt><dd>{takenDate(d.takenAt)}, {takenTime(d.takenAt)}</dd>
        <dt>File</dt><dd>{d.fileName}<br /><span className="muted">{[d.width && d.height ? `${d.width} × ${d.height}` : '', fmtSize(d.sizeBytes)].filter(Boolean).join(' · ')}</span></dd>
        {d.folder && <><dt>Folder</dt><dd>{d.folder}</dd></>}
        {camera && <><dt>Camera</dt><dd>{camera}{d.lens && <><br /><span className="muted">{d.lens}</span></>}</dd></>}
        {exposure && <><dt>Exposure</dt><dd>{exposure}</dd></>}
        {d.lat != null && d.lon != null && <><dt>Place</dt><dd>{d.lat.toFixed(5)}, {d.lon.toFixed(5)}{d.alt ? <><br /><span className="muted">{Math.round(d.alt)} m above sea level</span></> : null}</dd></>}
      </dl>
      <a className="btn sm" href={mediaUrl(`/api/items/${d.id}/original`)} download>Download the original</a>
    </aside>
  )
}

function QuickAlbum({ d, lib, onClose, onChanged }: { d: PhotoDetail; lib: Library; onClose: () => void; onChanged: () => void }) {
  const { user } = useApp()
  const [albums, setAlbums] = useState<PhotoAlbum[] | null>(null)
  useEffect(() => { api.get<PhotoAlbum[]>(`/photos/albums?library=${lib.id}`).then(setAlbums).catch(() => setAlbums([])) }, [lib.id])
  const mine = (albums || []).filter((a) => a.ownerId === user?.id || user?.isAdmin)
  const add = async (a: PhotoAlbum) => { await api.post(`/photos/albums/${a.id}/items`, { ids: [d.id] }); toast(`Added to ${a.name}`); onChanged(); onClose() }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="qa-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="qa-h">Add to album</h2>
        {albums && !mine.length && <p className="muted">You have no albums yet. Make one under Albums.</p>}
        <div className="gal-pick">{mine.map((a) => <button key={a.id} className="btn ghost" disabled={d.albums.includes(a.id)} onClick={() => add(a)}>{a.name}{d.albums.includes(a.id) && <span className="muted"> · already in it</span>}</button>)}</div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button></div>
      </div>
    </div>
  )
}
