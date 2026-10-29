import { useEffect, useState } from 'react'
import { api, mediaUrl } from '../api'
import { ChooseFolderButton } from '../components/FolderPicker'
import { IconFolder } from '../icons'
import { toast } from '../state'

type Category = { id: string; label: string; description: string }
type LogFile = { name: string; size: number; modified: string }
type LogsView = { dir: string; level: string; categories: Category[]; on: string[]; files: LogFile[]; canOpen: boolean }

// Settings → Logs & privacy: what the server writes to its log files, where
// they are, and getting a file to attach to a bug report.
export default function LogSettings() {
  const [v, setV] = useState<LogsView | null>(null)
  const [dirInput, setDirInput] = useState('')
  const load = () => api.get<LogsView>('/logs').then((x) => { setV(x); setDirInput(x.dir) }).catch((e) => toast(e.message))
  useEffect(() => { load() }, [])
  if (!v) return null
  const put = async (body: Partial<{ on: string[]; level: string; dir: string }>) => {
    try { const x = await api.put<LogsView>('/logs', body); setV(x); setDirInput(x.dir); toast('Log settings saved') } catch (e: any) { toast(e.message) }
  }
  const toggle = (id: string) => put({ on: v.on.includes(id) ? v.on.filter((x) => x !== id) : [...v.on, id] })
  const open = async () => { try { await api.post('/logs/open') } catch (e: any) { toast(e.message) } }
  const size = (n: number) => (n > 1 << 20 ? (n / (1 << 20)).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB')
  return (
    <>
      <div className="panel privacy-note" role="note">
        <h2>Your logs stay with you</h2>
        <p>
          Logs are written only to this server's own disk. <b>They are never uploaded or sent anywhere, and nobody, including the people who make
          this software, ever sees them unless you choose to send a file yourself</b> (for example with a bug report).
        </p>
        <p className="muted">Warnings and errors are always logged so problems can be traced. Everything below is off until you turn it on, and you can see exactly what each switch adds.</p>
      </div>

      <div className="panel">
        <h2>What to log</h2>
        <div className="log-cats">
          {v.categories.map((c) => {
            const on = v.on.includes(c.id)
            return (
              <div key={c.id} className="log-cat">
                <div className="grow">
                  <div className="lc-title">{c.label}</div>
                  <div className="muted lc-desc">{c.description}</div>
                </div>
                <button type="button" role="switch" aria-checked={on} aria-label={c.label} className={'switch' + (on ? ' on' : '')} onClick={() => toggle(c.id)}><i /></button>
              </div>
            )
          })}
        </div>
        <label className="log-level">Detail of everything else
          <select className="select" value={v.level} onChange={(e) => put({ level: e.target.value })}>
            <option value="error">Errors only</option>
            <option value="warn">Warnings and errors</option>
            <option value="info">Normal (recommended)</option>
            <option value="debug">Everything (for troubleshooting)</option>
          </select>
        </label>
      </div>

      <div className="panel">
        <h2>Where the logs are</h2>
        <div className="log-dir">
          <IconFolder />
          <code className="grow" title={v.dir}>{v.dir}</code>
          {v.canOpen && <button className="btn primary" onClick={open}>Open folder</button>}
        </div>
        <div className="field-row" style={{ marginTop: 10 }}>
          <ChooseFolderButton className="btn" label="Choose another folder…" onPick={(p) => put({ dir: p })} />
          <input className="input" aria-label="Log folder" value={dirInput} onChange={(e) => setDirInput(e.target.value)} />
          <button className="btn" disabled={!dirInput.trim() || dirInput === v.dir} onClick={() => put({ dir: dirInput })}>Use this folder</button>
          <button className="btn ghost" onClick={() => put({ dir: '' })} title="Back to the folder inside the server's data folder">Default</button>
        </div>
        <p className="muted" style={{ fontSize: 13 }}>New messages go to the new folder; existing files stay where they are. Files older than 14 days are deleted automatically.</p>

        <h3 className="section-title" style={{ marginTop: 18 }}>Log files</h3>
        {v.files.length === 0 && <div className="muted">No log files yet.</div>}
        <div className="list">
          {v.files.map((f) => (
            <div key={f.name} className="list-item">
              <div className="grow"><b>{f.name}</b> <span className="muted">· {size(f.size)} · {new Date(f.modified).toLocaleString()}</span></div>
              <a className="btn sm" href={mediaUrl(`/api/logs/files/${encodeURIComponent(f.name)}`)} download={f.name}>Download</a>
            </div>
          ))}
        </div>
        <button className="btn ghost sm" style={{ marginTop: 10 }} onClick={load}>Refresh</button>
      </div>
    </>
  )
}
