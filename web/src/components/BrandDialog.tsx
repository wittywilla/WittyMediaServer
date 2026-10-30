import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { getToken, serverBase } from '../api'
import { Brand } from './common'

// The server's name and logo: a window with a live preview of the corner of
// the screen, a name box and a logo upload. Used by setup and by Settings,
// Appearance.

const MAX_LOGO = 2 << 20

// logo: what is shown now (an address), or '' for the app icon.
// onSave gets the name and the logo change: a new File, 'remove', or null
// for no change.
export default function BrandDialog({ name, logo, onSave, onClose }: {
  name: string; logo: string
  onSave: (name: string, logo: File | 'remove' | null) => Promise<void> | void
  onClose: () => void
}) {
  const [n, setN] = useState(name)
  const [file, setFile] = useState<File | 'remove' | null>(null)
  const [preview, setPreview] = useState(logo)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement | null>(null)
  useEffect(() => {
    if (!(file instanceof File)) return
    const u = URL.createObjectURL(file)
    setPreview(u)
    return () => URL.revokeObjectURL(u)
  }, [file])
  useEffect(() => {
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', esc)
    return () => window.removeEventListener('keydown', esc)
  }, [onClose])
  const pick = (f: File | undefined) => {
    setErr('')
    if (!f) return
    if (!/^image\/(png|jpeg)$/.test(f.type)) { setErr('Use a PNG or JPEG image.'); return }
    if (f.size > MAX_LOGO) { setErr('The logo must be 2 MB or smaller.'); return }
    setFile(f)
  }
  const save = async () => {
    setBusy(true); setErr('')
    try { await onSave(n.trim(), file); onClose() } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const mark = file === 'remove' || !preview ? '/icon.svg' : preview
  // A portal, so the window never takes on the styles of the page it opens from.
  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal brand-dialog" role="dialog" aria-modal="true" aria-labelledby="brand-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="brand-h">Name your server</h2>
        <p className="muted">Shown in the top-left corner, on the sign-in page and in the browser tab.</p>

        <div className="brand-preview" aria-label="Preview">
          <span className="brand-preview-label">Preview</span>
          <div className="brand-preview-bar">
            <div className="brand"><img className="brand-mark" src={mark} alt="" /><Brand name={n.trim()} /></div>
            <span className="brand-preview-search" />
            <span className="brand-preview-avatar" />
          </div>
          <div className="brand-preview-card">
            <div className="brand"><img className="brand-mark" src={mark} alt="" /><Brand name={n.trim()} /></div>
            <span className="muted">Who's watching?</span>
          </div>
        </div>

        <label className="brand-field">Server name
          <input className="input" autoFocus value={n} maxLength={40} onChange={(e) => setN(e.target.value)} placeholder="WittyMediaServer"
            onKeyDown={(e) => { if (e.key === 'Enter') save() }} />
          <span className="muted">Leave empty to use “WittyMediaServer”.</span>
        </label>

        <div className="brand-field">Logo
          <div className="brand-logo-row">
            <img className="brand-logo-big" src={mark} alt="" />
            <div className="brand-logo-actions">
              <button type="button" className="btn" onClick={() => input.current?.click()}>Upload logo…</button>
              {mark !== '/icon.svg' && <button type="button" className="btn ghost" onClick={() => { setFile('remove'); setPreview('') }}>Use the standard icon</button>}
              <span className="muted">PNG or JPEG, up to 2 MB. Square images look best.</span>
            </div>
          </div>
          <input ref={input} type="file" accept="image/png,image/jpeg" hidden onChange={(e) => { pick(e.target.files?.[0]); e.target.value = '' }} />
        </div>

        {err && <div className="error" role="alert">{err}</div>}
        <div className="actions">
          <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
          <button type="button" className="btn primary" disabled={busy} onClick={save}>{busy ? 'Saving…' : 'Save'}</button>
        </div>
      </div>
    </div>,
    document.body,
  )
}

// uploadLogo stores a new logo on the server and returns its version.
export async function uploadLogo(f: File): Promise<string> {
  const headers: Record<string, string> = { 'Content-Type': f.type }
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(`${serverBase()}/api/branding/logo`, { method: 'PUT', headers, body: f, credentials: 'same-origin' })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || 'The logo could not be saved')
  return data.brandLogo as string
}
