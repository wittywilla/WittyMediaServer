import { useEffect, useState } from 'react'
import { api } from '../api'
import { IconFolder } from '../icons'
import { useApp } from '../state'

// ChooseFolderButton opens the operating system's own folder picker when the
// browser runs on the server's machine (Explorer on Windows, Finder on
// macOS, the desktop's dialog on Linux). Everywhere else, or if that is not
// possible, it falls back to browsing the server's folders in the page.
export function ChooseFolderButton({ onPick, label = 'Choose folder…', className = 'btn' }: { onPick: (path: string) => void; label?: string; className?: string }) {
  const { pickerLocal } = useApp()
  const [browse, setBrowse] = useState(false)
  const [busy, setBusy] = useState(false)
  const choose = async () => {
    if (!pickerLocal) { setBrowse(true); return }
    setBusy(true)
    try {
      const r = await api.post<{ path?: string; cancelled?: boolean }>('/fs/pick')
      if (r.path) onPick(r.path)
    } catch {
      setBrowse(true) // no desktop (service, container): browse in the page instead
    } finally { setBusy(false) }
  }
  return (
    <>
      <button type="button" className={className} onClick={choose} disabled={busy}>
        <IconFolder /> {busy ? 'Waiting for the folder window…' : label}
      </button>
      {browse && (
        <div className="modal-backdrop" onClick={() => setBrowse(false)}>
          <div className="modal" role="dialog" aria-label="Choose a folder" onClick={(e) => e.stopPropagation()}>
            <h2>Choose a folder</h2>
            <FolderBrowser onPick={(p) => { onPick(p); setBrowse(false) }} />
            <div className="actions"><button className="btn ghost" onClick={() => setBrowse(false)}>Cancel</button></div>
          </div>
        </div>
      )}
    </>
  )
}

type Listing = { path: string; parent: string | null; entries: { name: string; path: string }[] }

// FolderBrowser lists the server's folders; it works from any device.
export function FolderBrowser({ onPick }: { onPick: (p: string) => void }) {
  const [cur, setCur] = useState('')
  const [data, setData] = useState<Listing | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    setErr('')
    api.get<Listing>(`/fs?path=${encodeURIComponent(cur)}`).then(setData).catch((e) => setErr(e.message))
  }, [cur])
  return (
    <div className="folder-browser">
      <div className="fb-bar">
        <span className="fb-path" aria-live="polite">{data?.path || 'Drives and common places'}</span>
        {data?.path && <button type="button" className="btn sm ghost" onClick={() => setCur(data.parent || '')}>Up</button>}
        {data?.path && <button type="button" className="btn sm primary" onClick={() => onPick(data.path)}>Use this folder</button>}
      </div>
      {err && <div className="error">{err}</div>}
      <div className="fs-browser" role="list">
        {data?.entries.map((e) => <button type="button" role="listitem" key={e.path} onClick={() => setCur(e.path)}><IconFolder /> {e.name}</button>)}
        {data && data.entries.length === 0 && <div className="muted" style={{ padding: 10 }}>No folders inside. Press “Use this folder” to pick this one.</div>}
      </div>
    </div>
  )
}
