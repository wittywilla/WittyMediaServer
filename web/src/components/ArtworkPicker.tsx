import { useEffect, useState } from 'react'
import { api, artPreviewUrl, type ArtCandidate } from '../api'
import { Spinner } from './common'

// Lets an admin pick a poster or backdrop from the candidates the server
// finds: images beside the media, TMDB (movies) and TheTVDB (shows).
export default function ArtworkPicker({ base, onPick, onClose }: { base: string; onPick: (kind: 'poster' | 'backdrop', url: string) => Promise<void>; onClose: () => void }) {
  const [kind, setKind] = useState<'poster' | 'backdrop'>('poster')
  const [list, setList] = useState<ArtCandidate[] | null>(null)
  const [warn, setWarn] = useState('')
  const [url, setUrl] = useState('')
  const [saving, setSaving] = useState(false)
  useEffect(() => {
    setList(null)
    api.get<{ candidates: ArtCandidate[]; warning: string }>(`${base}/artwork?kind=${kind}`)
      .then((r) => { setList(r.candidates); setWarn(r.warning) })
      .catch((e) => { setList([]); setWarn(e.message) })
  }, [base, kind])
  const pick = async (u: string) => { setSaving(true); try { await onPick(kind, u) } finally { setSaving(false) } }
  const label = { local: 'In folder', tmdb: 'TMDB', tvdb: 'TheTVDB', tvmaze: 'TVmaze', current: 'Current' }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <h2>Choose artwork</h2>
        <div className="tabs" style={{ marginBottom: 12 }}>
          <a className={kind === 'poster' ? 'active' : ''} onClick={() => setKind('poster')}>Poster</a>
          <a className={kind === 'backdrop' ? 'active' : ''} onClick={() => setKind('backdrop')}>Backdrop</a>
        </div>
        {warn && <div className="muted" style={{ marginBottom: 10, fontSize: 13 }}>{warn}</div>}
        {!list ? <Spinner /> : (
          <div className={'art-grid ' + kind}>
            {list.map((c) => (
              <button key={c.url} className={'art-choice' + (c.source === 'current' ? ' current' : '')} disabled={saving} onClick={() => pick(c.url)} title={c.url}>
                <img src={artPreviewUrl(c.url)} alt="" loading="lazy" />
                <span className="src">{label[c.source]}</span>
              </button>
            ))}
            {list.length === 0 && <div className="muted">No artwork found.</div>}
          </div>
        )}
        <div className="field-row" style={{ marginTop: 14 }}>
          <input className="input" placeholder="…or paste an image URL / file path" value={url} onChange={(e) => setUrl(e.target.value)} />
          <button className="btn" disabled={!url || saving} onClick={() => pick(url)}>Use</button>
        </div>
        <p className="muted" style={{ fontSize: 12, marginTop: 10 }}>The chosen image is saved next to the media (poster.jpg / fanart.jpg or &lt;name&gt;-poster.jpg) so other media servers and Kodi pick it up too.</p>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Close</button></div>
      </div>
    </div>
  )
}
