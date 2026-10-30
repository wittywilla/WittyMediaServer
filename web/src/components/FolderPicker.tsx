import { useState } from 'react'
import FileBrowser, { type Listing } from '@wittywilla/wittyintegratedfilebrowser'
import { api } from '../api'
import { IconDrive, IconFolder } from '../icons'
import { useApp } from '../state'

// The server's folders for WittyIntegratedFileBrowser.
const listServer = (path: string, hidden: boolean) => api.get<Listing>(`/fs${path ? `?path=${encodeURIComponent(path)}${hidden ? '&hidden=1' : ''}` : ''}`)

// Choosing folders on the server. "Choose folder…" opens the operating
// system's own folder window when the browser runs on the server's machine
// (Explorer, Finder, the desktop's dialog). "Browse folders" opens the
// built-in browser, which lists the server's drives and folders in the page
// and so works from any device: a phone, a TV, another computer.
export function ChooseFolderButton({ onPick, label = 'Choose folder…', className = 'btn' }: { onPick: (path: string) => void; label?: string; className?: string }) {
  const { pickerLocal } = useApp()
  const [browse, setBrowse] = useState(false)
  const [busy, setBusy] = useState(false)
  const choose = async () => {
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
      {pickerLocal && (
        <button type="button" className={className} onClick={choose} disabled={busy} title="Opens this computer's own folder window">
          <IconFolder /> {busy ? 'Waiting for the folder window…' : label}
        </button>
      )}
      <button type="button" className={pickerLocal ? 'btn' : className} onClick={() => setBrowse(true)}
        title="Browse the server's drives and folders from this device">
        <IconDrive /> {pickerLocal ? 'Browse folders' : label.replace('Choose', 'Browse')}
      </button>
      {browse && <FileBrowser list={listServer} onClose={() => setBrowse(false)} onPick={(ps) => { ps.forEach(onPick); setBrowse(false) }} />}
    </>
  )
}
