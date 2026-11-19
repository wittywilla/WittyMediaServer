// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { api } from '../../api'
import { Button, Dialog, Input, Select } from '../../ui'
import { useReminders } from './reminders'
import { dayWord, fmtClock, type GuideHit } from './tvApi'
import { t } from '../../i18n'

// Searching what's on: every channel this account can see, the next two weeks.
export default function SearchPanel({ onClose, onGo }: { onClose: () => void; onGo: (hit: GuideHit) => void }) {
  const [q, setQ] = useState('')
  const [hits, setHits] = useState<GuideHit[] | null>(null)
  const [err, setErr] = useState('')
  const [days, setDays] = useState(7)
  const [busy, setBusy] = useState(false)
  const seq = useRef(0)
  const reminders = useReminders()
  const now = Date.now() / 1000
  useEffect(() => {
    const text = q.trim()
    const id = ++seq.current
    if (text.length < 2) { setHits(null); setErr(''); setBusy(false); return }
    setBusy(true)
    const timer = setTimeout(() => {
      api.get<GuideHit[]>(`/guide/search?q=${encodeURIComponent(text)}&days=${days}`)
        .then((h) => { if (id === seq.current) { setHits(h); setErr(''); setBusy(false) } })
        .catch((e) => { if (id === seq.current) { setErr(e.message); setBusy(false) } })
    }, 300)
    return () => clearTimeout(timer)
  }, [q, days])
  return (
    <Dialog className="ctv-search" title={t('Search the schedule')} onClose={onClose} actions={<Button variant="ghost" onClick={onClose}>{t('Close')}</Button>}>
      <div className="ctv-search-row">
        <Input type="search" autoFocus aria-label={t('Search for')} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('A film, a show, an episode…')} />
        <Select aria-label={t('How far ahead')} value={days} onChange={(e) => setDays(Number(e.target.value))}>
          <option value={1}>{t('Next 24 hours')}</option><option value={7}>{t('Next week')}</option><option value={14}>{t('Next 2 weeks')}</option>
        </Select>
      </div>
      {err && <p role="alert">{err}</p>}
      {busy && <p className="muted" role="status">{t('Searching…')}</p>}
      <div aria-live="polite" className="sr-only">{hits && !busy ? t('{count} found', { count: hits.length }) : ''}</div>
      {hits && !busy && hits.length === 0 && <p className="muted">{t('Nothing like that is coming up.')}</p>}
      {hits && hits.length > 0 && (
        <ul className="ctv-hits">
          {hits.map((h) => {
            const later = h.start > now
            return (
              <li key={h.channel + h.start}>
                <button type="button" className="ctv-hit" onClick={() => onGo(h)}>
                  <b>{h.title}</b>
                  {h.sub && <small>{h.sub}</small>}
                  <span>{dayWord(h.start, now)} {fmtClock(h.start)} · {t('Channel {number}, {name}', { number: h.number, name: h.name })}{h.start <= now && h.end > now ? ` · ${t('On now')}` : ''}</span>
                </button>
                {later && <Button size="sm" variant="ghost" aria-pressed={reminders.has(h.channel, h.start)} aria-label={t('Remind me about {title}', { title: h.title })} onClick={() => void reminders.toggle(h.channel, h)}>{reminders.has(h.channel, h.start) ? '✓' : t('Remind')}</Button>}
              </li>
            )
          })}
        </ul>
      )}
    </Dialog>
  )
}
