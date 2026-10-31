import { useState, type ReactNode } from 'react'
import { downloadFile } from '../api'
import { IconDownload } from '../icons'
import { toast } from '../state'

// A download that signs in the way the app's other requests do, with no token in
// the address, for what an admin takes away: logs, crash reports, diagnoser reports.
export default function DownloadButton({ path, name, className = 'btn sm', label, children }: { path: string; name: string; className?: string; label?: string; children: ReactNode }) {
  const [busy, setBusy] = useState(false)
  const start = async () => {
    setBusy(true)
    try { await downloadFile(path, name) } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  return <button type="button" className={className} disabled={busy} aria-label={label} onClick={start}><IconDownload /> {busy ? 'Preparing…' : children}</button>
}
