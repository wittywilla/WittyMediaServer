// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, fmtTime, type Item } from '../api'
import { toast } from '../state'
import { clockText, markerForm, markerPut, type MarkerAnswer, type MarkerForm, type RangeText } from './markers'
import './pickintro.css'

// Pick in player: play to where a part starts or ends, and press the button. It goes back to
// the edit dialog it came from. mode "show" sets where the intro ends for the whole show;
// "markers" marks any part of this one video (its intro, recap, credits or an advert break).
export default function PickIntroBar({ item, time, mode }: { item: Item; time: number; mode: string }) {
  const nav = useNavigate()
  if (mode === 'markers') return <PickMarkersBar item={item} time={time} />
  const back = mode === 'show' && item.showId != null ? `/show/${item.showId}?edit=playback` : `/item/${item.id}?edit=playback`
  const use = async () => {
    const t = Math.round(time)
    try {
      const target = mode === 'show' && item.showId != null ? `/meta/show/${item.showId}` : `/meta/item/${item.id}`
      await api.put(target, { fields: { introEnd: String(t) } })
      toast(`The intro ends at ${fmtTime(t)}`)
      nav(back, { replace: true })
    } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="pick-intro" role="group" aria-label="Pick where the intro ends" onClick={(e) => e.stopPropagation()}>
      <span>Play to where the intro ends, then press this.</span>
      <button type="button" className="btn primary" onClick={use}>The intro ends at {fmtTime(time)}</button>
      <button type="button" className="btn ghost" onClick={() => nav(back, { replace: true })}>Never mind</button>
    </div>
  )
}

type Part = 'recap' | 'intro' | 'credits' | 'advert'
const PARTS: { id: Part; label: string }[] = [{ id: 'intro', label: 'Intro' }, { id: 'recap', label: 'Recap' }, { id: 'credits', label: 'Credits' }, { id: 'advert', label: 'New advert break' }]
const EMPTY: RangeText = { start: '', end: '' }

// Marking one part after another: choose which, press Starts here and Ends here as it plays,
// and Save. The times already in force are shown, so one end can be moved and the other kept.
function PickMarkersBar({ item, time }: { item: Item; time: number }) {
  const nav = useNavigate()
  const back = () => nav(`/item/${item.id}?edit=playback`, { replace: true })
  const [saved, setSaved] = useState<MarkerAnswer | null>(null)
  const [form, setForm] = useState<MarkerForm | null>(null)
  const [part, setPart] = useState<Part>('intro')
  const [advert, setAdvert] = useState<RangeText>(EMPTY)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const took = (m: MarkerAnswer) => { setSaved(m); setForm(markerForm(m)); setAdvert(EMPTY) }
  useEffect(() => { api.get<MarkerAnswer>(`/media/${item.id}/markers`).then(took).catch((e) => setErr(e.message)) }, [item.id])
  const cur = !form ? EMPTY : part === 'advert' ? advert : form[part]
  const was = part === 'advert' ? EMPTY : markerForm(saved)[part]
  const changed = cur.start !== was.start || cur.end !== was.end
  const set = (patch: Partial<RangeText>) => {
    setErr('')
    if (part === 'advert') setAdvert({ ...advert, ...patch })
    else if (form) setForm({ ...form, [part]: { ...form[part], ...patch } })
  }
  const save = async () => {
    if (!form) return
    const r = markerPut(saved, part === 'advert' ? { ...form, commercials: [...form.commercials, advert] } : form)
    if (r.error || !r.body) { setErr(r.error || ''); return }
    setBusy(true)
    try { took(await api.put<MarkerAnswer>(`/media/${item.id}/markers`, r.body)); toast('Skip times saved') } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const at = clockText(Math.round(time))
  return (
    <div className="pick-intro pick-markers" role="group" aria-label="Mark what can be skipped" onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
      <label className="pick-part">Mark
        <select className="select" value={part} onChange={(e) => { setPart(e.target.value as Part); setErr('') }}>
          {PARTS.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
        </select>
      </label>
      <button type="button" className="btn" disabled={!form} onClick={() => set({ start: at })}>Starts here ({at})</button>
      <button type="button" className="btn" disabled={!form} onClick={() => set({ end: at })}>Ends here ({at})</button>
      <span className="pick-now" role="status">{cur.start || cur.end ? `${cur.start || '0:00'} to ${cur.end || (part === 'credits' ? 'the end' : '…')}` : 'Not marked'}</span>
      <button type="button" className="btn primary" disabled={!form || busy || !changed} onClick={save}>Save</button>
      <button type="button" className="btn ghost" onClick={back}>Done</button>
      {err && <div className="error" role="alert">{err}</div>}
    </div>
  )
}
