import { useEffect, useRef, useState } from 'react'
import { api } from '../../api'
import { useReminders } from './reminders'
import { dayWord, fmtClock, type GuideHit } from './tvApi'

// Searching what's on: every channel this account can see, the next two weeks.
export default function SearchPanel({ onClose, onGo }: { onClose: () => void; onGo: (hit: GuideHit) => void }) {
  const [q, setQ] = useState('')
  const [hits, setHits] = useState<GuideHit[] | null>(null)
  const [err, setErr] = useState('')
  const [days, setDays] = useState(7)
  const seq = useRef(0)
  const reminders = useReminders()
  const now = Date.now() / 1000
  useEffect(() => {
    const text = q.trim()
    if (text.length < 2) { setHits(null); setErr(''); return }
    const id = ++seq.current
    const t = setTimeout(() => {
      api.get<GuideHit[]>(`/guide/search?q=${encodeURIComponent(text)}&days=${days}`)
        .then((h) => { if (id === seq.current) { setHits(h); setErr('') } })
        .catch((e) => { if (id === seq.current) setErr(e.message) })
    }, 300)
    return () => clearTimeout(t)
  }, [q, days])
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal ctv-search" role="dialog" aria-modal="true" aria-labelledby="ctv-s-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="ctv-s-h">Search the schedule</h2>
        <div className="ctv-search-row">
          <input className="input" type="search" autoFocus value={q} onChange={(e) => setQ(e.target.value)} placeholder="A film, a show, an episode…" aria-label="Search for" />
          <select className="select" value={days} onChange={(e) => setDays(Number(e.target.value))} aria-label="How far ahead">
            <option value={1}>Next 24 hours</option><option value={7}>Next week</option><option value={14}>Next 2 weeks</option>
          </select>
        </div>
        {err && <p role="alert">{err}</p>}
        <div aria-live="polite" className="sr-only">{hits ? `${hits.length} found` : ''}</div>
        {hits && hits.length === 0 && <p className="muted">Nothing like that is coming up.</p>}
        {hits && hits.length > 0 && (
          <ul className="ctv-hits">
            {hits.map((h) => {
              const later = h.start > now
              return (
                <li key={h.channel + h.start}>
                  <button className="ctv-hit" onClick={() => onGo(h)}>
                    <b>{h.title}</b>
                    {h.sub && <small>{h.sub}</small>}
                    <span>{dayWord(h.start, now)} {fmtClock(h.start)} · Channel {h.number}, {h.name}{h.start <= now && h.end > now ? ' · On now' : ''}</span>
                  </button>
                  {later && <button className="btn sm ghost" aria-pressed={reminders.has(h.channel, h.start)} aria-label={`Remind me about ${h.title}`} onClick={() => void reminders.toggle(h.channel, h)}>{reminders.has(h.channel, h.start) ? '✓' : 'Remind'}</button>}
                </li>
              )
            })}
          </ul>
        )}
        <div className="actions"><button className="btn ghost" onClick={onClose}>Close</button></div>
      </div>
    </div>
  )
}
