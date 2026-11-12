// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyTV More info (docs/CLIENT-API.md, "More info"): one programme in full, over the screen it was asked from, which
// stays where it is. Everything shown comes from GET /tv/{key}/programme: nothing is made up, so a row the server has
// no data for (advisories, say) isn't drawn. Back closes it.
import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '../../api'
import { toast } from '../../state'
import { Button, Dialog, LoadError, Spinner } from '../../ui'
import { ChannelMark } from './ChannelLogo'
import { useLook } from './look'
import { useReminders } from './reminders'
import { fmtClock, fmtDay, fmtSpan } from './tvApi'

type Person = { name: string; role?: string; character?: string; personId?: number }
type Answer = {
  channel: { key: string; kind: 'library' | 'tuner'; number: string; name: string; logo?: string; brand?: { kind: string; name: string }; catchupDays?: number }
  programme: {
    start: number; end: number; kind?: string; title: string; sub?: string; desc?: string; genres?: string[]; rating?: string; year?: number; season?: number; episode?: number
    lengthMin?: number; scores?: { value: number; system: string }[]; studio?: string; language?: string; origLanguage?: string; country?: string; subtitles?: string[]
    signLanguage?: boolean; audio?: string; hd?: boolean; premiere?: boolean; new?: boolean; repeat?: boolean; keywords?: string[]
  }
  actions: { watch?: boolean; record?: boolean; remind?: boolean; reminded?: boolean; fromStart?: boolean; watchLater?: boolean; inList?: boolean; open?: boolean }
  library?: { itemId?: number; showId?: number }
  credits?: { cast?: Person[]; guests?: Person[]; crew?: Person[] }
  trailers?: { itemId: number; title: string; type: string; durationSec?: number }[]
  schedule?: { channel: string; channelName?: string; number?: string; start: number; end: number; this?: boolean }[]
  ids?: { imdb?: string; tmdb?: string }
}

export default function MoreInfo({ channelKey, at, onClose }: { channelKey: string; at: number; onClose: () => void }) {
  const nav = useNavigate()
  const look = useLook()
  const reminders = useReminders()
  const [a, setA] = useState<Answer | null>(null)
  const [err, setErr] = useState('')
  const [gone, setGone] = useState(false)
  const load = () => { setErr(''); api.get<Answer>(`/tv/${encodeURIComponent(channelKey)}/programme?start=${Math.floor(at)}`).then(setA).catch((e) => { if ((e as { status?: number }).status === 404) setGone(true); else setErr((e as Error).message) }) }
  useEffect(load, [channelKey, at]) // eslint-disable-line react-hooks/exhaustive-deps
  // The guide keeps about a month of the past: an older programme is no longer there to ask about.
  if (gone) return <Dialog title="More info" onClose={onClose}><p>Details are no longer available.</p></Dialog>
  if (err) return <Dialog title="More info" onClose={onClose}><LoadError what="This programme" onRetry={load} /></Dialog>
  if (!a) return <Dialog title="More info" onClose={onClose}><Spinner /></Dialog>
  const p = a.programme, ch = a.channel
  const now = Date.now() / 1000
  const live = p.start <= now && p.end > now
  const facts = [p.year, p.rating, p.lengthMin ? `${p.lengthMin} min` : '', p.season ? `Season ${p.season}${p.episode ? `, episode ${p.episode}` : ''}` : '', p.hd ? 'HD' : '', p.new ? 'New' : p.repeat ? 'Repeat' : '', p.premiere ? 'Premiere' : ''].filter(Boolean)
  const details: [string, string][] = ([
    ['Genres', p.genres?.join(', ') || ''], ['Studio', p.studio || ''], ['Country', p.country || ''], ['Language', [p.language, p.origLanguage && p.origLanguage !== p.language ? `original ${p.origLanguage}` : ''].filter(Boolean).join(', ')],
    ['Audio', p.audio || ''], ['Subtitles', p.subtitles?.join(', ') || ''], ['Sign language', p.signLanguage ? 'Yes' : ''], ['Keywords', p.keywords?.join(', ') || ''],
  ] as [string, string][]).filter(([, v]) => v)
  const run = async (what: () => Promise<unknown>, said: string) => { try { await what(); toast(said, { tone: 'ok' }) } catch (e) { toast((e as Error).message, { tone: 'danger' }) } }
  const programme = { start: p.start, end: p.end, kind: 'show' as const, title: p.title, sub: p.sub, desc: p.desc }
  return (
    <Dialog title={p.title} size="wide" onClose={onClose} className="ctv-more"
      actions={<>
        {a.actions.watch && <Button variant="primary" onClick={() => { onClose(); nav(`/tv/watch/${ch.key}`, { state: { fromGuide: true } }) }}>{live ? 'Watch now' : 'Watch'}</Button>}
        {a.actions.fromStart && <Button onClick={() => { onClose(); nav(`/tv/watch/${ch.key}?from=${Math.floor(p.start)}&to=${Math.floor(p.end)}`, { state: { fromGuide: true } }) }}>Watch from the start</Button>}
        {a.actions.record && <Button onClick={() => void run(() => api.post('/recordings', { channel: ch.key, title: p.title, sub: p.sub, start: p.start, stop: p.end }), `Recording ${p.title}`)}>Record</Button>}
        {a.actions.remind && <Button aria-pressed={reminders.has(ch.key, p.start)} onClick={() => void reminders.toggle(ch.key, programme)}>{reminders.has(ch.key, p.start) ? '✓ Reminder set' : 'Remind me'}</Button>}
        {a.actions.watchLater && a.library && <Button disabled={a.actions.inList} onClick={() => void run(() => api.post('/collections/watch-later/items', a.library!.showId ? { shows: [a.library!.showId] } : { items: [a.library!.itemId] }), 'Added to your Watchlist')}>{a.actions.inList ? '✓ In your Watchlist' : 'Watchlist'}</Button>}
        {a.actions.open && a.library && <Link className="btn" to={a.library.showId ? `/show/${a.library.showId}` : `/item/${a.library.itemId}`} onClick={onClose}>Open item page</Link>}
        <Button variant="ghost" onClick={onClose}>Close</Button>
      </>}>
      <header className="ctv-more-head">
        <ChannelMark ch={{ logo: ch.logo, name: ch.name, brand: ch.brand as never }} look={look} on="dark" />
        <div><b>{ch.number} {ch.name}</b><br /><span className="muted">{fmtDay(p.start)}, {fmtSpan({ start: p.start, end: p.end } as never)}{live ? ' · on now' : ''}</span></div>
      </header>
      {p.sub && <h3 className="ctv-more-sub">{p.sub}</h3>}
      {facts.length > 0 && <p className="ctv-more-facts">{facts.join(' · ')}{p.scores?.length ? ' · ' + p.scores.map((s) => `${s.value} ${s.system}`).join(' · ') : ''}</p>}
      {p.desc && <p>{p.desc}</p>}
      {details.length > 0 && <dl className="ctv-info-list">{details.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>}
      {(a.credits?.cast?.length || a.credits?.guests?.length || a.credits?.crew?.length) ? (
        <section aria-label="Cast and crew">
          <h3>Cast and crew</h3>
          <ul className="ctv-more-people">
            {[...(a.credits?.cast || []), ...(a.credits?.guests || []), ...(a.credits?.crew || [])].slice(0, 24).map((c, i) => <li key={i}><b>{c.name}</b>{c.character || c.role ? <span className="muted"> {c.character || c.role}</span> : null}</li>)}
          </ul>
        </section>
      ) : null}
      {a.trailers && a.trailers.length > 0 && (
        <section aria-label="Trailers and clips"><h3>Trailers and clips</h3>
          <ul className="ctv-more-people">{a.trailers.map((t) => <li key={t.itemId}><Link to={`/play/${t.itemId}`} onClick={onClose}>{t.title}</Link> <span className="muted">{t.type}{t.durationSec ? `, ${Math.round(t.durationSec / 60)} min` : ''}</span></li>)}</ul>
        </section>
      )}
      {a.schedule && a.schedule.length > 1 && (
        <section aria-label="Also on"><h3>Also on</h3>
          <ul className="ctv-more-people">{a.schedule.map((s, i) => <li key={i} aria-current={s.this || undefined}><b>{s.number} {s.channelName || s.channel}</b> <span className="muted">{fmtDay(s.start)}, {fmtClock(s.start)}{s.this ? ' (this one)' : ''}</span></li>)}</ul>
        </section>
      )}
    </Dialog>
  )
}
