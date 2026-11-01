import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { IconPlay } from '../../icons'
import { toast } from '../../state'
import { type Look } from './look'
import { useReminders } from './reminders'
import { fmtClock, fmtDay, fmtLeft, fmtSpan, type GuideChannel, type GuidePage, type Programme } from './tvApi'

// More about a programme, or the whole of the channel's day: the same dialog,
// so that one is a key press away from the other.
export default function DetailsDialog({ ch, p, now, startDay, onClose, onWatch, onRecord }: { ch: GuideChannel; p: Programme; now: number; startDay?: boolean; look: Look; onClose: () => void; onWatch: () => void; onRecord: () => void }) {
  const [desc, setDesc] = useState(p.desc || '')
  const [day, setDay] = useState(!!startDay)
  const reminders = useReminders()
  useEffect(() => {
    if (p.desc || !p.itemId) return
    api.get<{ overview: string }>(`/items/${p.itemId}`).then((it) => setDesc(it.overview)).catch(() => {})
  }, [p.desc, p.itemId])
  const addSeries = async () => {
    try { await api.post('/recording-rules', { channel: ch.key, title: p.title }); toast(`Every airing of ${p.title} will be recorded`); onClose() } catch (e: any) { toast(e.message) }
  }
  const later = p.start > now
  const reminded = reminders.has(ch.key, p.start)
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal ctv-details" role="dialog" aria-modal="true" aria-labelledby="ctv-d-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="ctv-d-h">{day ? `Channel ${ch.number}, ${ch.name}` : p.title}</h2>
        <div className="ctv-tabs" role="group" aria-label="Show">
          <button className={'btn sm' + (!day ? ' primary' : ' ghost')} aria-pressed={!day} onClick={() => setDay(false)}>This programme</button>
          <button className={'btn sm' + (day ? ' primary' : ' ghost')} aria-pressed={day} onClick={() => setDay(true)}>The whole day</button>
        </div>
        {day ? <DayView ch={ch} at={p.start} now={now} /> : (
          <>
            {p.sub && <p className="muted">{p.sub}</p>}
            <p className="muted">Channel {ch.number}, {ch.name}{p.kind !== 'none' && <> · {fmtDay(p.start)}, {fmtSpan(p)}</>}{p.rating && <> · {p.rating}</>}</p>
            {p.clip && <p className="muted" style={{ fontSize: 13 }}>A scene from this disc{p.clip.label ? `: ${p.clip.label}` : ''}, {fmtLeft(p.clip.end - p.clip.start)} long.</p>}
            {p.group && (
              <ol className="ctv-day">
                {p.group.map((x) => <li key={x.start}><time>{fmtClock(x.start)}</time><span><b>{x.title}</b>{x.sub && <small> {x.sub}</small>}</span></li>)}
              </ol>
            )}
            {desc && <p>{desc}</p>}
            {ch.security?.state === 'encrypted' && <p role="alert"><b>{ch.security.label}.</b> {ch.security.detail} <Link to="/tv/encrypted">What can I do?</Link></p>}
            {ch.security?.state === 'unknown' && <p className="muted" style={{ fontSize: 13 }}>{ch.security.detail}</p>}
            {ch.noRecord && <p className="muted" style={{ fontSize: 13 }}>This channel can't be recorded: {ch.noRecord}.</p>}
            {ch.notices?.map((n) => <p key={n} className="muted" style={{ fontSize: 13 }}>{n}</p>)}
            {ch.kind === 'library' && <p className="muted" style={{ fontSize: 13 }}>Library channels can't be recorded: what they play is already in your library.</p>}
          </>
        )}
        <div className="actions">
          <button className="btn ghost" onClick={onClose}>Close</button>
          {later && !ch.encrypted && p.kind !== 'none' && p.kind !== 'group' && <button className="btn" aria-pressed={reminded} onClick={() => void reminders.toggle(ch.key, p)}>{reminded ? '✓ Reminder set' : 'Remind me'}</button>}
          {ch.canRecord && p.kind === 'broadcast' && p.end > now && <><button className="btn" onClick={() => void addSeries()}>Record every airing</button><button className="btn" onClick={onRecord}>Record this</button></>}
          {ch.alternative && <Link className="btn primary" to={`/tv/watch/${ch.alternative.key}`}><IconPlay /> Watch {ch.alternative.number} {ch.alternative.name} instead</Link>}
          {!ch.encrypted && <button className={'btn' + (ch.alternative ? '' : ' primary')} onClick={onWatch}><IconPlay /> Watch channel now</button>}
        </div>
      </div>
    </div>
  )
}

function DayView({ ch, at, now }: { ch: GuideChannel; at: number; now: number }) {
  const start = (() => { const d = new Date(at * 1000); d.setHours(0, 0, 0, 0); return d.getTime() / 1000 })()
  const [list, setList] = useState<Programme[] | null>(null)
  const [err, setErr] = useState('')
  const reminders = useReminders()
  const box = useRef<HTMLOListElement | null>(null)
  // It opens at what's on now, not at midnight.
  useEffect(() => { box.current?.querySelector('li.on')?.scrollIntoView({ block: 'center' }) }, [list])
  useEffect(() => {
    api.get<GuidePage>(`/guide?from=${Math.max(start, Math.floor(now) - 86000)}&hours=24&limit=1&channel=${encodeURIComponent(ch.key)}`)
      .then((g) => setList(g.channels[0]?.programmes || []))
      .catch((e) => setErr(e.message))
  }, [ch.key, start, now])
  if (err) return <p role="alert">{err}</p>
  if (!list) return <div className="center"><div className="spinner" /></div>
  if (!list.length) return <p className="muted">Nothing is listed for this day.</p>
  return (
    <ol className="ctv-day" ref={box} aria-label={`Schedule for ${fmtDay(start + 3600)}`}>
      {list.map((p) => {
        const on = p.start <= now && p.end > now, past = p.end <= now
        return (
          <li key={p.start} className={(on ? 'on' : '') + (past ? ' past' : '')}>
            <time>{fmtClock(p.start)}</time>
            <span><b>{p.title}</b>{p.sub && <small> {p.sub}</small>}{on && <em> On now</em>}</span>
            {p.start > now && p.kind !== 'slate' && <button className="btn sm ghost" aria-pressed={reminders.has(ch.key, p.start)} aria-label={`Remind me about ${p.title} at ${fmtClock(p.start)}`} onClick={() => void reminders.toggle(ch.key, p)}>{reminders.has(ch.key, p.start) ? '✓' : 'Remind'}</button>}
          </li>
        )
      })}
    </ol>
  )
}
