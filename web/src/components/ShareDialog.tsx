import { useState } from 'react'
import { api, getPrefs } from '../api'
import { copyText, type ShareLink } from '../gallery'
import { IconClose } from '../icons'
import { toast } from '../state'
import { useEffect } from 'react'
import LegalNote from './LegalNote'
import '../gallery.css'

// Sharing something: a link inside this server for other profiles, or (pictures,
// videos and albums) a link for someone with no account, with its own end date.

const DAYS: [number, string][] = [[1, '1 day'], [7, '7 days'], [30, '30 days'], [365, '1 year'], [0, 'No end date']]

export default function ShareDialog({ kind, id, title, onClose, internalPath }: { kind: 'item' | 'album'; id: number; title: string; onClose: () => void; internalPath: string }) {
  const [days, setDays] = useState(30)
  const [download, setDownload] = useState(false)
  const [link, setLink] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [allowed, setAllowed] = useState(true)
  useEffect(() => { getPrefs().then((p) => setAllowed(p.shareLinks !== false)) }, [])
  const internal = location.origin + internalPath
  const make = async () => {
    setBusy(true); setErr('')
    try {
      const l = await api.post<ShareLink>('/shares', { kind, id, days, allowDownload: download })
      setLink(location.origin + '/s/' + l.token)
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const copy = async (t: string) => { toast((await copyText(t)) ? 'Link copied' : "Couldn't copy the link: select it and copy") }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal share-modal" role="dialog" aria-modal="true" aria-labelledby="share-h" onClick={(e) => e.stopPropagation()}>
        <div className="share-head">
          <h2 id="share-h">Share “{title}”</h2>
          <button className="ibtn" onClick={onClose} aria-label="Close"><IconClose /></button>
        </div>
        <LegalNote kind="sharing" />
        <h3>With other profiles here</h3>
        <p className="muted">Anyone signed in to this server who can open this library.</p>
        <div className="share-link"><input className="input" readOnly value={internal} aria-label="Link for other profiles" onFocus={(e) => e.target.select()} /><button className="btn" onClick={() => copy(internal)}>Copy</button></div>
        {allowed && <>
          <h3>With someone without an account</h3>
          {link ? (
            <>
              <p className="muted">This is the only time the link is shown: the server keeps just a fingerprint of it. You can end it any time in Settings, Sharing.</p>
              <div className="share-link"><input className="input" readOnly value={link} aria-label="Link to share" autoFocus onFocus={(e) => e.target.select()} /><button className="btn primary" onClick={() => copy(link)}>Copy</button></div>
            </>
          ) : (
            <>
              <p className="muted">They see only this and your profile name, without signing in. Pictures are re-encoded, so camera details and locations stay here. It works wherever your server can be reached.</p>
              <div className="share-opts">
                <label>Ends after <select className="select" value={days} onChange={(e) => setDays(Number(e.target.value))}>{DAYS.map(([d, l]) => <option key={d} value={d}>{l}</option>)}</select></label>
                <label className="share-check"><input type="checkbox" checked={download} onChange={(e) => setDownload(e.target.checked)} /> Allow downloading the original files</label>
              </div>
              {err && <div className="error" role="alert">{err}</div>}
              <button className="btn primary" disabled={busy} onClick={make}>{busy ? 'Making…' : 'Make a link'}</button>
            </>
          )}
        </>}
      </div>
    </div>
  )
}
