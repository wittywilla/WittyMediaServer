// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useId, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../api'
import { IconClose, IconPlus } from '../../icons'
import { anyByHand, markerForm, markerPut, markerSourceWords, type MarkerAnswer, type MarkerForm, type RangeText } from '../../player/markers'
import { toast } from '../../state'
import { IconButton } from '../../ui'
import './markers.css'

// Where a video's intro, recap, credits and advert breaks are, set by hand: the part of the
// edit dialog for PUT /api/media/{id}/markers. Each row shows what is in force now and where
// it came from. What's typed here is saved with this section's own button, at once, as a
// change of match is; a row that was set by hand and is emptied goes back to what the file
// says. dirty: the dialog has changes of its own waiting, which leaving for the player would lose.

type Kind = 'intro' | 'recap' | 'credits'
const ROWS: { kind: Kind; label: string; hint?: string }[] = [
  { kind: 'recap', label: 'Recap' },
  { kind: 'intro', label: 'Intro' },
  { kind: 'credits', label: 'Credits', hint: 'Leave the end empty for credits that run to the end.' },
]

export default function MarkersEditor({ id, dirty, onError }: { id: number; dirty: boolean; onError: (s: string) => void }) {
  const nav = useNavigate()
  const uid = useId()
  const [saved, setSaved] = useState<MarkerAnswer | null>(null)
  const [form, setForm] = useState<MarkerForm | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const took = useCallback((m: MarkerAnswer) => { setSaved(m); setForm(markerForm(m)); setErr('') }, [])
  useEffect(() => { api.get<MarkerAnswer>(`/media/${id}/markers`).then(took).catch((e) => setErr(e.message)) }, [id, took])
  if (!form) return err ? <div className="error" role="alert">{err}</div> : <p className="muted ed-help">Looking up the skip times…</p>

  const was = markerForm(saved)
  const changedRow = (k: Kind) => form[k].start.trim() !== was[k].start || form[k].end.trim() !== was[k].end
  const breaksChanged = JSON.stringify(form.commercials.filter((r) => r.start.trim() || r.end.trim())) !== JSON.stringify(was.commercials)
  const changed = ROWS.some((r) => changedRow(r.kind)) || breaksChanged
  const setRow = (k: Kind, patch: Partial<RangeText>) => setForm({ ...form, [k]: { ...form[k], ...patch } })
  const setBreak = (i: number, patch: Partial<RangeText>) => setForm({ ...form, commercials: form.commercials.map((r, n) => (n === i ? { ...r, ...patch } : r)) })
  const send = async (body: unknown, done: string) => {
    setBusy(true); setErr('')
    try { took(await api.put<MarkerAnswer>(`/media/${id}/markers`, body)); toast(done) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const save = () => {
    const r = markerPut(saved, form)
    if (r.error || !r.body) { setErr(r.error || ''); return }
    send(r.body, 'Skip times saved')
  }
  const pick = () => {
    if (dirty || changed) { onError('Save or undo your changes first: picking a time leaves this dialog.'); return }
    nav(`/play/${id}?pickIntro=markers`)
  }
  const source = (k: Kind | 'commercials', isChanged: boolean) => (isChanged ? 'Not saved yet' : markerSourceWords(saved, k) || 'None')
  return (
    <div className="ed-markers">
      <p className="muted ed-help">Type times as minutes and seconds, such as 1:05, or choose <b>Pick in player</b> to mark them while watching. What you set here wins over the file’s chapters and anything beside the file, and no scan changes it. Empty a time you set to go back to what the file says.</p>
      {ROWS.map(({ kind, label, hint }) => (
        <div key={kind} className="ed-marker" role="group" aria-labelledby={`${uid}${kind}`}>
          <div className="ed-marker-name"><b id={`${uid}${kind}`}>{label}</b><span className="ed-source">{source(kind, changedRow(kind))}</span></div>
          <label>Starts<input className="input" inputMode="numeric" placeholder="0:00" value={form[kind].start} onChange={(e) => setRow(kind, { start: e.target.value })} /></label>
          <label>Ends<input className="input" inputMode="numeric" placeholder={kind === 'credits' ? 'the end' : ''} value={form[kind].end} onChange={(e) => setRow(kind, { end: e.target.value })} aria-describedby={hint ? `${uid}${kind}h` : undefined} /></label>
          <IconButton size="sm" label={`Empty the ${label.toLowerCase()} times`} disabled={!form[kind].start && !form[kind].end} onClick={() => setRow(kind, { start: '', end: '' })}><IconClose size={16} /></IconButton>
          {hint && <div className="ed-hint" id={`${uid}${kind}h`}>{hint}</div>}
        </div>
      ))}
      <div className="ed-marker-breaks" role="group" aria-labelledby={`${uid}ads`}>
        <div className="ed-marker-name"><b id={`${uid}ads`}>Advert breaks</b><span className="ed-source">{source('commercials', breaksChanged)}</span></div>
        {form.commercials.map((r, i) => (
          <div key={i} className="ed-marker" role="group" aria-label={`Advert break ${i + 1}`}>
            <span className="ed-marker-n muted">{i + 1}</span>
            <label>Starts<input className="input" inputMode="numeric" value={r.start} onChange={(e) => setBreak(i, { start: e.target.value })} /></label>
            <label>Ends<input className="input" inputMode="numeric" value={r.end} onChange={(e) => setBreak(i, { end: e.target.value })} /></label>
            <IconButton size="sm" label={`Remove advert break ${i + 1}`} onClick={() => setForm({ ...form, commercials: form.commercials.filter((_, n) => n !== i) })}><IconClose size={16} /></IconButton>
          </div>
        ))}
        <button type="button" className="btn sm ghost" onClick={() => setForm({ ...form, commercials: [...form.commercials, { start: '', end: '' }] })}><IconPlus size={16} /> Add an advert break</button>
      </div>
      {err && <div className="error" role="alert">{err}</div>}
      <div className="ed-marker-btns">
        <button type="button" className="btn" disabled={busy || !changed} onClick={save}>Save skip times</button>
        <button type="button" className="btn" onClick={pick}>Pick in player</button>
        {anyByHand(saved) && <button type="button" className="btn ghost" disabled={busy} onClick={() => send({ reset: true }, 'Back to what the files say')}>Back to what the files say</button>}
      </div>
    </div>
  )
}
