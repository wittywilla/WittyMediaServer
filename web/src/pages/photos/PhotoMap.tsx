import { useEffect, useMemo, useRef, useState } from 'react'
import { api, serverBase, type Library } from '../../api'
import { photoThumb, type Photo, type PhotoCluster } from '../../gallery'
import { IconLeft, IconRight, IconUp, IconDown } from '../../icons'

// The photo map. By default it needs nothing from the internet: a small outline
// of the world's land ships with the app, and the groups of pictures come from
// this server. An administrator can add an online map's address (Settings,
// Sharing & photos); only then does this page ask anyone else for anything.

// Web Mercator, as a share of the world's width: online maps use the same.
const mercX = (lon: number) => (lon + 180) / 360
const mercY = (lat: number) => {
  const r = Math.min(85.05, Math.max(-85.05, lat)) * Math.PI / 180
  return 0.5 - Math.log(Math.tan(Math.PI / 4 + r / 2)) / (2 * Math.PI)
}
const unMercLat = (y: number) => (Math.atan(Math.sinh((0.5 - y) * 2 * Math.PI)) * 180) / Math.PI

type View = { x: number; y: number; z: number } // centre and zoom (2^z worlds across 256 px)
const clamp = (n: number, a: number, b: number) => Math.min(b, Math.max(a, n))
const MAX_ZOOM = 18

export default function PhotoMap({ lib, onOpen, onList }: { lib: Library; onOpen: (id: number) => void; onList: (l: Photo[]) => void }) {
  const box = useRef<HTMLDivElement | null>(null)
  const [size, setSize] = useState({ w: 800, h: 500 })
  const [view, setView] = useState<View>({ x: 0.5, y: 0.45, z: 1 })
  const [clusters, setClusters] = useState<PhotoCluster[] | null>(null)
  const [tiles, setTiles] = useState('')
  const [land, setLand] = useState('')
  const [area, setArea] = useState<Photo[] | null>(null)
  const [fitted, setFitted] = useState(false)
  useEffect(() => {
    const el = box.current
    if (!el) return
    const ro = new ResizeObserver(() => setSize({ w: el.clientWidth, h: el.clientHeight }))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  useEffect(() => { fetch(serverBase() + '/world-land.txt').then((r) => r.text()).then(setLand).catch(() => {}) }, [])
  const z = Math.round(view.z)
  useEffect(() => {
    const t = setTimeout(() => {
      api.get<{ clusters: PhotoCluster[]; tiles: string }>(`/photos/map?library=${lib.id}&zoom=${z}`).then((r) => {
        setClusters(r.clusters); setTiles(r.tiles)
        if (!fitted && r.clusters.length) { setFitted(true); fitBounds(r.clusters.map((c) => [c.south, c.west, c.north, c.east] as const)) }
      }).catch(() => setClusters([]))
    }, 120)
    return () => clearTimeout(t)
  }, [lib.id, z]) // eslint-disable-line react-hooks/exhaustive-deps

  const scale = 256 * 2 ** view.z // pixels across the world
  const toPx = (x: number, y: number) => ({ left: (x - view.x) * scale + size.w / 2, top: (y - view.y) * scale + size.h / 2 })
  const fitBounds = (b: readonly (readonly [number, number, number, number])[]) => {
    const s = Math.min(...b.map((v) => v[0])), w = Math.min(...b.map((v) => v[1])), n = Math.max(...b.map((v) => v[2])), e = Math.max(...b.map((v) => v[3]))
    const x1 = mercX(w), x2 = mercX(e), y1 = mercY(n), y2 = mercY(s)
    const el = box.current
    const W = el?.clientWidth || size.w, H = el?.clientHeight || size.h
    const zx = Math.log2((W * 0.8) / (256 * Math.max(x2 - x1, 1e-6))), zy = Math.log2((H * 0.8) / (256 * Math.max(y2 - y1, 1e-6)))
    setView({ x: (x1 + x2) / 2, y: (y1 + y2) / 2, z: clamp(Math.min(zx, zy), 0.5, 14) })
  }
  const zoom = (d: number) => setView((v) => ({ ...v, z: clamp(v.z + d, 0.5, MAX_ZOOM) }))
  const pan = (dx: number, dy: number) => setView((v) => ({ ...v, x: clamp(v.x + (dx * 150) / (256 * 2 ** v.z), 0, 1), y: clamp(v.y + (dy * 150) / (256 * 2 ** v.z), 0, 1) }))

  // A small group opens its pictures; a large one zooms in first.
  const choose = (c: PhotoCluster) => {
    if (c.count > 30 && z < MAX_ZOOM - 2 && (c.north - c.south > 1e-4 || c.east - c.west > 1e-4)) { fitBounds([[c.south, c.west, c.north, c.east]]); return }
    const pad = 1e-5
    api.get<{ photos: Photo[] }>(`/photos/list?library=${lib.id}&limit=500&bbox=${c.south - pad},${c.west - pad},${c.north + pad},${c.east + pad}`).then((r) => { setArea(r.photos); onList(r.photos) }).catch(() => {})
  }

  const tileImgs = useMemo(() => {
    if (!tiles) return []
    const tz = clamp(Math.round(view.z), 0, 19)
    const n = 2 ** tz
    const tileSize = scale / n
    const out: { key: string; src: string; left: number; top: number; size: number }[] = []
    const cols = Math.ceil(size.w / tileSize) + 2, rows = Math.ceil(size.h / tileSize) + 2
    const cx = Math.floor(view.x * n - cols / 2), cy = Math.floor(view.y * n - rows / 2)
    for (let i = 0; i < cols; i++) for (let j = 0; j < rows; j++) {
      const tx = cx + i, ty = cy + j
      if (ty < 0 || ty >= n) continue
      const wrapped = ((tx % n) + n) % n
      const p = toPx(tx / n, ty / n)
      out.push({ key: `${tz}/${tx}/${ty}`, src: tiles.replace('{z}', String(tz)).replace('{x}', String(wrapped)).replace('{y}', String(ty)), left: p.left, top: p.top, size: tileSize })
    }
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tiles, view, size])

  const drag = useRef<{ x: number; y: number; vx: number; vy: number; moved: boolean } | null>(null)
  const landPx = toPx(0, 0)
  const placed = clusters?.length ?? 0
  return (
    <div className="gal-map-wrap">
      <div className="gal-map" ref={box}
        onWheel={(e) => zoom(e.deltaY < 0 ? 0.5 : -0.5)}
        onPointerDown={(e) => { if ((e.target as HTMLElement).closest('button')) return; drag.current = { x: e.clientX, y: e.clientY, vx: view.x, vy: view.y, moved: false }; (e.currentTarget as Element).setPointerCapture?.(e.pointerId) }}
        onPointerMove={(e) => { const d = drag.current; if (d) setView((v) => ({ ...v, x: clamp(d.vx - (e.clientX - d.x) / scale, 0, 1), y: clamp(d.vy - (e.clientY - d.y) / scale, 0, 1) })) }}
        onPointerUp={() => { drag.current = null }}>
        {tileImgs.map((t) => <img key={t.key} className="gal-tile-img" src={t.src} alt="" draggable={false} referrerPolicy="no-referrer" style={{ left: t.left, top: t.top, width: t.size, height: t.size }} />)}
        {!tiles && land && (
          <svg className="gal-land" width={size.w} height={size.h} aria-hidden="true">
            <path d={land} transform={`translate(${landPx.left} ${landPx.top}) scale(${scale / 1000})`} fillRule="evenodd" vectorEffect="non-scaling-stroke" />
          </svg>
        )}
        {clusters?.map((c) => {
          const p = toPx(mercX(c.lon), mercY(c.lat))
          if (p.left < -40 || p.top < -40 || p.left > size.w + 40 || p.top > size.h + 40) return null
          return (
            <button key={`${c.lat},${c.lon}`} className="gal-pin" style={{ left: p.left, top: p.top }} onClick={() => choose(c)}
              aria-label={`${c.count} ${c.count === 1 ? 'picture' : 'pictures'} near ${c.lat.toFixed(2)}, ${c.lon.toFixed(2)}`}>
              <img src={photoThumb(c.coverId, 160)} alt="" loading="lazy" />
              {c.count > 1 && <b>{c.count > 999 ? Math.round(c.count / 1000) + 'K' : c.count}</b>}
            </button>
          )
        })}
        <div className="gal-map-ctl" role="group" aria-label="Move the map">
          <button className="btn icon" onClick={() => zoom(1)} aria-label="Zoom in">＋</button>
          <button className="btn icon" onClick={() => zoom(-1)} aria-label="Zoom out">－</button>
          <button className="btn icon" onClick={() => pan(0, -1)} aria-label="Move up"><IconUp /></button>
          <button className="btn icon" onClick={() => pan(0, 1)} aria-label="Move down"><IconDown /></button>
          <button className="btn icon" onClick={() => pan(-1, 0)} aria-label="Move left"><IconLeft /></button>
          <button className="btn icon" onClick={() => pan(1, 0)} aria-label="Move right"><IconRight /></button>
        </div>
        {tiles ? <span className="gal-map-credit">Map from the provider set in Settings</span> : <span className="gal-map-credit">Outline: Natural Earth · Latitude {unMercLat(view.y).toFixed(1)}°</span>}
        {clusters !== null && placed === 0 && <div className="gal-map-empty"><b>No pictures with a location yet</b><p>Phones and some cameras save where a picture was taken. Those pictures appear here.</p></div>}
      </div>
      {area && (
        <section className="gal-map-area" aria-label="Pictures here">
          <div className="gal-sub"><h2>{area.length} {area.length === 1 ? 'picture' : 'pictures'} here</h2><button className="btn sm ghost" onClick={() => setArea(null)}>Close</button></div>
          <div className="gal-grid small">
            {area.map((p) => (
              <button key={p.id} type="button" className="gal-tile" style={{ flexGrow: p.width && p.height ? p.width / p.height : 1.4, flexBasis: ((p.width && p.height ? p.width / p.height : 1.4) * 130) }} aria-label={p.title} onClick={() => onOpen(p.id)}>
                <img src={photoThumb(p.id, 320, p.updatedAt)} alt="" loading="lazy" />
              </button>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
