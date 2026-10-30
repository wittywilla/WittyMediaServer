import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { toast, useApp } from '../../state'
import { useLook } from './look'
import { fmtClock, fmtDay, type Recording, type RecordingRule } from './tvApi'
import './tv.css'

const STATUS: Record<Recording['status'], string> = { scheduled: 'Scheduled', recording: 'Recording now', done: 'Done', failed: 'Failed', cancelled: 'Cancelled' }

function fmtSize(b?: number) {
  if (!b) return ''
  return b > 1e9 ? (b / 1e9).toFixed(1) + ' GB' : Math.round(b / 1e6) + ' MB'
}

export default function RecordingsPage() {
  const { user } = useApp()
  const look = useLook()
  const [list, setList] = useState<Recording[] | null>(null)
  const [rules, setRules] = useState<RecordingRule[]>([])
  const [err, setErr] = useState('')
  const load = () => {
    api.get<Recording[]>('/recordings').then((l) => { setList(l); setErr('') }).catch((e) => setErr(e.message))
    api.get<RecordingRule[]>('/recording-rules').then(setRules).catch(() => {})
  }
  useEffect(() => { load(); const t = setInterval(load, 15000); return () => clearInterval(t) }, [])
  const remove = async (r: Recording, deleteFile: boolean) => {
    const what = r.status === 'done' || r.status === 'failed' ? `Remove “${r.title}” from this list?` : `Cancel the recording of “${r.title}”?`
    if (!confirm(deleteFile ? `Delete the recording of “${r.title}” and its file? This cannot be undone.` : what)) return
    try { await api.del(`/recordings/${r.id}${deleteFile ? '?deleteFile=1' : ''}`); load() } catch (e: any) { toast(e.message) }
  }
  const removeRule = async (r: RecordingRule) => {
    try { await api.del(`/recording-rules/${r.id}`); load() } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="ctv ctv-page ctv-list-page" data-skin={look.colours} data-layout={look.layout}>
      <header className="ctv-list-head">
        <h1>Recordings</h1>
        <Link className="ctv-btn sm" to="/tv">Back to the guide</Link>
      </header>
      {err && <div className="ctv-err" role="alert">{err}</div>}
      {list && list.length === 0 && <p className="ctv-list-empty">Nothing recorded yet. Highlight a programme on a Live TV channel in the guide and press R.</p>}
      {list && list.length > 0 && (
        <ul className="ctv-list">
          {list.map((r) => (
            <li key={r.id} className={'ctv-rec ' + r.status}>
              <div className="ctv-rec-main">
                <b>{r.title}</b>{r.sub && <small>{r.sub}</small>}
                <span>{fmtDay(r.start)}, {fmtClock(r.start)} to {fmtClock(r.stop)} · Channel {r.number} {r.channel}{r.sizeBytes ? ' · ' + fmtSize(r.sizeBytes) : ''}</span>
                {r.error && <span className="ctv-rec-err" role="alert">{r.error}</span>}
              </div>
              <span className="ctv-rec-status">{STATUS[r.status] || r.status}</span>
              <button className="ctv-btn sm" onClick={() => void remove(r, false)}>{r.status === 'done' || r.status === 'failed' ? 'Remove' : 'Cancel'}</button>
              {user?.isAdmin && r.path && r.status !== 'recording' && r.status !== 'scheduled' && <button className="ctv-btn sm" onClick={() => void remove(r, true)}>Delete file</button>}
            </li>
          ))}
        </ul>
      )}
      {rules.length > 0 && (
        <section>
          <h2 className="ctv-list-h2">Series</h2>
          <ul className="ctv-list">
            {rules.map((r) => (
              <li key={r.id} className="ctv-rec">
                <div className="ctv-rec-main"><b>{r.title}</b><span>Every airing on {r.channel}</span></div>
                <button className="ctv-btn sm" onClick={() => void removeRule(r)}>Stop</button>
              </li>
            ))}
          </ul>
        </section>
      )}
      <p className="ctv-list-note">Recordings are of Live TV channels only. Library channels play what is already in your library. Protected broadcasts are never recorded.</p>
    </div>
  )
}
