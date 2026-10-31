import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { serverBase } from '../api'
import { Spinner } from '../components/common'
import { IconClose, IconLeft, IconRight } from '../icons'
import { useSource } from '../player/hooks'
import '../gallery.css'
import LegalNote from '../components/LegalNote'

// What someone sees when they open a link they were sent: only this, and no way
// into the server. No account, no navigation, nothing but what was shared.

type SharedItem = { id: number; kind: 'photo' | 'video'; title: string; takenAt?: number; width: number; height: number; durationSec?: number }
type Shared = { kind: 'item' | 'album'; title: string; owner: string; allowDownload: boolean; expiresAt: number | null; items: SharedItem[] }

export default function SharedPage() {
  const { token } = useParams()
  const [data, setData] = useState<Shared | null>(null)
  const [err, setErr] = useState('')
  const [open, setOpen] = useState<number | null>(null)
  const base = `${serverBase()}/api/share/${token}`
  useEffect(() => {
    fetch(base).then(async (r) => { if (!r.ok) throw new Error(); return r.json() }).then((d: Shared) => {
      setData(d)
      document.title = d.title
      if (d.items.length === 1) setOpen(0)
    }).catch(() => setErr('This link has ended, or it was never valid.'))
  }, [base])
  if (err) return <div className="gal-public"><div className="gal-planned"><h1>{err}</h1><p className="muted">Ask the person who sent it for a new one.</p></div></div>
  if (!data) return <Spinner />
  const it = open !== null ? data.items[open] : null
  return (
    <div className="gal-public">
      <header className="gal-public-head">
        <h1>{data.title}</h1>
        <p className="muted">Shared by {data.owner}{data.expiresAt ? ` · until ${new Date(data.expiresAt * 1000).toLocaleDateString()}` : ''}</p>
        <LegalNote kind="sharedView" newTab />
      </header>
      <div className="gal-grid">
        {data.items.map((p, i) => {
          const a = p.width && p.height ? p.width / p.height : 1.4
          return (
            <button key={p.id} type="button" className="gal-tile" style={{ flexGrow: a, flexBasis: a * 190 }} aria-label={p.title} onClick={() => setOpen(i)}>
              <img src={`${base}/image/${p.id}?w=480`} alt="" loading="lazy" decoding="async" />
              {p.kind === 'video' && <span className="gal-badge">▶</span>}
            </button>
          )
        })}
      </div>
      {it && open !== null && (
        <div className="gal-viewer" role="dialog" aria-modal="true" aria-label={it.title} tabIndex={-1}
          onKeyDown={(e) => {
            if (e.key === 'ArrowLeft' && open > 0) setOpen(open - 1)
            if (e.key === 'ArrowRight' && open + 1 < data.items.length) setOpen(open + 1)
            if (e.key === 'Escape') setOpen(null)
          }}>
          <div className="gal-stage">
            {it.kind === 'video' ? <SharedVideo key={it.id} url={`${base}/play/${it.id}`} /> : <img className="gal-img" src={`${base}/image/${it.id}?w=1920`} alt={it.title} />}
            {open > 0 && <button className="gal-nav prev" onClick={() => setOpen(open - 1)} aria-label="Previous"><IconLeft size={32} /></button>}
            {open + 1 < data.items.length && <button className="gal-nav next" onClick={() => setOpen(open + 1)} aria-label="Next"><IconRight size={32} /></button>}
          </div>
          <div className="gal-v-bar">
            <button className="ibtn" autoFocus onClick={() => setOpen(null)} aria-label="Close"><IconClose /></button>
            <div className="gal-v-title"><b>{it.title}</b></div>
            {data.allowDownload && <a className="btn sm" href={`${base}/original/${it.id}`} download>Download</a>}
            <LegalNote kind="sharedView" newTab className="gal-v-legal" />
          </div>
        </div>
      )}
    </div>
  )
}

function SharedVideo({ url }: { url: string }) {
  const video = useRef<HTMLVideoElement | null>(null)
  const [src, setSrc] = useState<string | null>(null)
  useEffect(() => { fetch(url).then((r) => r.json()).then((p) => setSrc(p.url)).catch(() => {}) }, [url])
  useSource(video, src, { onReady: () => { video.current?.play().catch(() => {}) } })
  return <video ref={video} className="gal-img" controls playsInline />
}
