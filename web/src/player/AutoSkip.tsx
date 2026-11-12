// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { announce } from '../a11y'
import { useUserPref } from '../ui/userPrefs'
import { AUTO_SKIP_KEY, anyAutoSkip, autoSkipPrefs, autoSkipStart, autoSkipStep, type AutoSkipAct, type Markers } from './markers'
import './autoskip.css'

// Skipping without a press, for a profile that asked for it (Settings, Playback & discs,
// Skipping). When to skip is decided in markers.ts; this does it, and says so: a short note
// with Undo, read out politely. Credits with an episode after them count down to that
// episode instead, and the countdown can be stopped.
//
// live: the picture is really playing (loaded, not paused, not waiting), so its clock can be
// believed. seek is the player's own, which in a watch party seeks for everyone.

const NOTE_MS = 8000
const NEXT_IN = 5

export default function AutoSkip({ itemId, video, markers, now, duration, live, mini, party, seek, nextHref }: {
  itemId: number; video: React.RefObject<HTMLVideoElement | null>; markers: Markers | null; now: number; duration: number; live: boolean; mini: boolean
  party: 'none' | 'control' | 'follow'; seek: (t: number) => void; nextHref: string | null
}) {
  const nav = useNavigate()
  const [raw] = useUserPref<unknown>(AUTO_SKIP_KEY, null)
  const prefs = autoSkipPrefs(raw)
  const on = anyAutoSkip(prefs)
  const state = useRef(autoSkipStart())
  const [note, setNote] = useState<AutoSkipAct | null>(null)
  const [left, setLeft] = useState(NEXT_IN)
  useEffect(() => { state.current = autoSkipStart(); setNote(null) }, [itemId])

  useEffect(() => {
    const v = video.current
    if (!on || !live || !v || v.seeking) return
    // Fast playback moves further between two looks at the clock.
    const r = autoSkipStep(state.current, markers, prefs, now, { duration, hasNext: !!nextHref && !mini, party, maxStep: Math.max(3, (v.playbackRate || 1) * 1.5) })
    state.current = r.state
    const act = r.act
    if (!act) return
    setNote(act)
    if (act.next) { setLeft(NEXT_IN); announce(`Credits. The next episode starts in ${NEXT_IN} seconds.`); return }
    seek(act.to)
    announce(`${act.label}.`)
  }, [now, live, on, markers, raw]) // eslint-disable-line react-hooks/exhaustive-deps

  // The note goes by itself. While it counts down to the next episode it stays, and a seek
  // away from the credits calls that off.
  useEffect(() => {
    if (!note) return
    if (!note.next) { const t = setTimeout(() => setNote(null), NOTE_MS); return () => clearTimeout(t) }
    if (left <= 0) { if (nextHref) nav(nextHref, { replace: true }); setNote(null); return }
    if (!live) return
    const t = setTimeout(() => setLeft((n) => n - 1), 1000)
    return () => clearTimeout(t)
  }, [note, left, live]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (note?.next && (now < note.from - 1 || now >= note.to)) setNote(null) }, [note, now])

  if (!note || mini) return null
  const undo = () => { seek(note.from); setNote(null); announce('Back where it was.') }
  return (
    <div className="auto-skip" role="group" aria-label={note.next ? 'Next episode' : note.label} onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
      <span>{note.next ? `Credits · next episode in ${left} s` : note.label}</span>
      {note.next
        ? <button type="button" className="btn sm" onClick={() => { setNote(null); announce('Staying for the credits.') }}>Watch the credits</button>
        : <button type="button" className="btn sm" onClick={undo}>Undo</button>}
    </div>
  )
}
