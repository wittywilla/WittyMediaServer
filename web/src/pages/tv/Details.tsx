// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { IconPlay } from '../../icons'
import { toast, useApp } from '../../state'
import { Button, Dialog, LinkButton, Segmented } from '../../ui'
import { type Look } from './look'
import { useReminders } from './reminders'
import { resetChannel } from './resetChannel'
import { canOfferRecord, fmtClock, fmtDay, fmtLeft, fmtSpan, type GuideChannel, type GuidePage, type Programme } from './tvApi'

// More about a programme, or the whole of the channel's day: the same dialog,
// so that one is a key press away from the other.
export default function DetailsDialog({ ch, p, now, startDay, onClose, onWatch, onRecord, onReset }: { ch: GuideChannel; p: Programme; now: number; startDay?: boolean; look: Look; onClose: () => void; onWatch: () => void; onRecord: () => void; onReset?: () => void }) {
  const [desc, setDesc] = useState(p.desc || '')
  const [day, setDay] = useState(!!startDay)
  const reminders = useReminders()
  const { user } = useApp()
  useEffect(() => {
    if (p.desc || !p.itemId) return
    api.get<{ overview: string }>(`/items/${p.itemId}`).then((it) => setDesc(it.overview)).catch(() => {})
  }, [p.desc, p.itemId])
  const addSeries = async () => {
    if (!canOfferRecord(ch, p, now)) return
    try { await api.post('/recording-rules', { channel: ch.key, title: p.title }); toast(`Every airing of ${p.title} will be recorded`); onClose() } catch (e) { toast((e as Error).message) }
  }
  const later = p.start > now
  const reminded = reminders.has(ch.key, p.start)
  const actions = (
    <>
      <Button variant="ghost" onClick={onClose}>Close</Button>
      {user?.isAdmin && ch.kind === 'library' && <LinkButton variant="ghost" to={`/tv/channels/${ch.id}`}>Edit channel</LinkButton>}
      {user?.isAdmin && ch.kind === 'library' && <Button variant="ghost" onClick={() => void resetChannel(ch.id, ch.name).then((done) => { if (done) { onReset?.(); onClose() } })}>Reset channel</Button>}
      {later && !ch.encrypted && p.kind !== 'none' && p.kind !== 'group' && <Button aria-pressed={reminded} onClick={() => void reminders.toggle(ch.key, p)}>{reminded ? '✓ Reminder set' : 'Remind me'}</Button>}
      {canOfferRecord(ch, p, now) && <><Button onClick={() => void addSeries()}>Record every airing</Button><Button onClick={onRecord}>Record this</Button></>}
      {ch.alternative && <LinkButton variant="primary" icon={<IconPlay />} to={`/tv/watch/${ch.alternative.key}`}>Watch {ch.alternative.number} {ch.alternative.name} instead</LinkButton>}
      {!ch.encrypted && <Button variant={ch.alternative ? 'default' : 'primary'} icon={<IconPlay />} onClick={onWatch}>Watch channel now</Button>}
    </>
  )
  return (
    <Dialog className="ctv-details" title={day ? `Channel ${ch.number}, ${ch.name}` : p.title} onClose={onClose} actions={actions}>
      <Segmented<'one' | 'day'> label="Show" value={day ? 'day' : 'one'} onChange={(v) => setDay(v === 'day')} className="ctv-tabs"
        options={[{ value: 'one', label: 'This programme' }, { value: 'day', label: 'The whole day' }]} />
      {day ? <DayView ch={ch} at={p.start} now={now} /> : (
        <>
          {p.sub && <p className="muted">{p.sub}</p>}
          <p className="muted">Channel {ch.number}, {ch.name}{p.kind !== 'none' && <> · {fmtDay(p.start)}, {fmtSpan(p)}</>}{p.rating && <> · {p.rating}</>}</p>
          {p.clip && <p className="muted ctv-note">A scene from this disc{p.clip.label ? `: ${p.clip.label}` : ''}, {fmtLeft(p.clip.end - p.clip.start)} long.</p>}
          {p.group && (
            <ol className="ctv-day">
              {p.group.map((x) => <li key={x.start}><time>{fmtClock(x.start)}</time><span><b>{x.title}</b>{x.sub && <small> {x.sub}</small>}</span></li>)}
            </ol>
          )}
          {desc && <p>{desc}</p>}
          {ch.security?.state === 'encrypted' && <p role="alert"><b>{ch.security.label}.</b> {ch.security.detail} <Link to="/tv/encrypted">What can I do?</Link></p>}
          {ch.security?.state === 'unknown' && <p className="muted ctv-note">{ch.security.detail}</p>}
          {ch.noRecord && <p className="muted ctv-note">This channel can't be recorded: {ch.noRecord}.</p>}
          {ch.notices?.map((n) => <p key={n} className="muted ctv-note">{n}</p>)}
          {ch.kind === 'library' && <p className="muted ctv-note">Library channels can't be recorded: what they play is already in your library.</p>}
        </>
      )}
    </Dialog>
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
            {p.start > now && p.kind !== 'slate' && <Button size="sm" variant="ghost" aria-pressed={reminders.has(ch.key, p.start)} aria-label={`Remind me about ${p.title} at ${fmtClock(p.start)}`} onClick={() => void reminders.toggle(ch.key, p)}>{reminders.has(ch.key, p.start) ? '✓' : 'Remind'}</Button>}
          </li>
        )
      })}
    </ol>
  )
}
