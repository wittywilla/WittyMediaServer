import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../../../api'
import DownloadButton from '../../../components/DownloadButton'
import { IconCheck, IconTrash } from '../../../icons'
import { toast } from '../../../state'
import { Section } from '../ui'
import { fmtDate, fmtSize, scrollKeys, type Crash } from './types'
import { Dialog } from '../../../ui/Overlay'

const KIND_WORDS: Record<string, string> = {
  fatal: 'Stopped by an internal error',
  panic: 'Caught; the server carried on',
  unclean: 'Stopped without shutting down',
  hang: 'Stopped answering',
  saved: 'Saved from Settings',
}

// What can be kept beside a report, for whoever looks into it.
const PART_WORDS: Record<string, string> = {
  'flight.log': 'The last 10 minutes',
  'goroutines.txt': 'What it was waiting for',
  trace: 'Execution trace',
}

const crashDownload = (id: string) => `/logs/crashes/${encodeURIComponent(id)}?download=1`
const partDownload = (id: string, part: string) => `/logs/crashes/${encodeURIComponent(id)}/${encodeURIComponent(part)}`

const bannerTitle = (c: Crash) => c.kind === 'panic' ? `Part of the server crashed on ${fmtDate(c.time)}`
  : c.kind === 'hang' ? `The server stopped answering on ${fmtDate(c.time)}`
  : `The server stopped unexpectedly on ${fmtDate(c.time)}`

// What the top of the page says about reports not yet looked at.
export function CrashBanner({ crashes, onSeen }: { crashes: Crash[]; onSeen: (id: string) => void }) {
  const unseen = crashes.filter((c) => !c.seen && c.kind !== 'saved')
  if (!unseen.length) return null
  const c = unseen[0]
  return (
    <div className="lg-banner bad" role="alert">
      <div className="lg-banner-text">
        <b>{bannerTitle(c)}</b>
        {c.summary && <p>{c.summary}</p>}
        {unseen.length > 1 && <p>There {unseen.length === 2 ? 'is 1 more report' : `are ${unseen.length - 1} more reports`} you haven’t looked at.</p>}
      </div>
      <div className="lg-banner-actions">
        <Link className="btn sm" to={`?tab=crashes&crash=${encodeURIComponent(c.id)}`}>View</Link>
        <DownloadButton path={crashDownload(c.id)} name={c.id + '.txt'}>Download</DownloadButton>
        <button type="button" className="btn sm ghost" onClick={() => onSeen(c.id)}><IconCheck /> Mark as seen</button>
        <Link className="btn sm ghost" to="?tab=bundle">Download all logs</Link>
      </div>
    </div>
  )
}

function Report({ crash, onSeen }: { crash: Crash; onSeen: (id: string) => void }) {
  const [text, setText] = useState<string | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    let gone = false
    api.text(`/logs/crashes/${encodeURIComponent(crash.id)}`).then((t) => { if (!gone) setText(t) }).catch((e) => { if (!gone) setErr(e.message) })
    if (!crash.seen) onSeen(crash.id) // it's being looked at now
    return () => { gone = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [crash.id])
  if (err) return <div className="error lg-note" role="alert">{err}</div>
  if (text === null) return <p className="muted lg-note" role="status">Reading the report…</p>
  return <pre className="lg-pre tall" tabIndex={0} aria-label="Crash report" onKeyDown={scrollKeys}>{text}</pre>
}

export default function Crashes({ crashes, reload, onSeen }: { crashes: Crash[]; reload: () => void; onSeen: (id: string) => void }) {
  const [params, setParams] = useSearchParams()
  const open = params.get('crash') || ''
  const [deleting, setDeleting] = useState<Crash | null>(null)
  const show = (id: string) => setParams((p) => {
    const n = new URLSearchParams(p)
    if (id) n.set('crash', id); else n.delete('crash')
    return n
  }, { replace: true })
  const remove = async () => {
    if (!deleting) return
    const id = deleting.id
    setDeleting(null)
    try {
      await api.del(`/logs/crashes/${encodeURIComponent(id)}`)
      if (open === id) show('')
      reload()
      toast('Crash report deleted')
    } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title="Crash reports" desc="Written when the server crashes, stops without shutting down or stops answering, and when you save what just happened. They stay on this server for 90 days; the newest five keep the detail beside them. Passwords and tokens are left out.">
      {crashes.length === 0
        ? <div className="set-empty"><IconCheck size={28} /><b>No crashes</b><p>Nothing has gone wrong that needed a report.</p></div>
        : (
          <ul className="lg-crashes">
            {crashes.map((c) => (
              <li key={c.id}>
                <div className="lg-crash-top">
                  <div className="lg-file-text">
                    <b>{KIND_WORDS[c.kind] || c.kind}{!c.seen && <span className="set-badge lg-new">New</span>}</b>
                    <span>{fmtDate(c.time)}{c.version ? ` · version ${c.version}` : ''} · {fmtSize(c.size)}</span>
                  </div>
                </div>
                {c.summary && <p className="lg-crash-summary">{c.summary}</p>}
                <div className="lg-actions">
                  <button type="button" className="btn sm" aria-expanded={open === c.id} onClick={() => show(open === c.id ? '' : c.id)}>{open === c.id ? 'Hide' : 'View'}</button>
                  <DownloadButton path={crashDownload(c.id)} name={c.id + '.txt'}>Download</DownloadButton>
                  {(c.parts || []).map((p) => <DownloadButton key={p} path={partDownload(c.id, p)} name={`${c.id}.${p}`}>{PART_WORDS[p] || p}</DownloadButton>)}
                  <button type="button" className="btn sm ghost danger-hover" onClick={() => setDeleting(c)}><IconTrash /> Delete</button>
                </div>
                {open === c.id && <Report crash={c} onSeen={onSeen} />}
              </li>
            ))}
          </ul>
        )}
      {deleting && (
        <Dialog title="Delete this crash report?" onClose={() => setDeleting(null)} className="set-modal" actions={<><button type="button" className="btn ghost" onClick={() => setDeleting(null)}>Keep it</button>
              <button type="button" className="btn danger" onClick={() => void remove()}>Delete</button></>}>
            <p className="muted">The report from {fmtDate(deleting.time)} will be gone for good. Download it first if someone may need it.</p>
        </Dialog>
      )}
    </Section>
  )
}
