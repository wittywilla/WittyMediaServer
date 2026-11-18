// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { announce } from '../a11y'
import { useUserPref } from '../ui/userPrefs'
import { FILTER_PREFS_KEY, filterAt, filterLabel, filterPrefs, type Filter } from '../wittyinfo/model'
import './autoskip.css'

// Content filters from a video's .wittyinfo (docs/formats/wittyinfo.md), done as the profile
// chose in its own settings (Playback, Content): skipped, muted or warned about. The file
// only says what is where; nothing happens for a profile that chose nothing.
//
// A skip jumps to the end of the filter every time playing runs into it: it is there so
// something isn't seen. Muting turns the sound off for as long as it lasts, and back on only
// if this turned it off.

export default function ContentFilters({ video, filters, now, live, seek }: {
  video: React.RefObject<HTMLVideoElement | null>; filters: Filter[] | null | undefined; now: number; live: boolean; seek: (t: number) => void
}) {
  const [raw] = useUserPref<unknown>(FILTER_PREFS_KEY, null)
  const prefs = filterPrefs(raw)
  const mutedByUs = useRef(false)
  const [note, setNote] = useState('')
  const hit = filters && filters.length ? filterAt(filters, prefs, now) : null

  useEffect(() => {
    const v = video.current
    if (!v) return
    if (hit?.act === 'mute') {
      if (!v.muted) { v.muted = true; mutedByUs.current = true; setNote(`Muted: ${filterLabel(hit.filter.category).toLowerCase()}`) }
    } else if (mutedByUs.current) {
      v.muted = false
      mutedByUs.current = false
      setNote('')
    }
    if (hit?.act === 'skip' && live && !v.seeking) {
      seek(hit.filter.end)
      const words = `Skipped: ${filterLabel(hit.filter.category).toLowerCase()}`
      setNote(words)
      announce(words + '.')
    }
    if (hit?.act === 'warn') setNote(`Coming up: ${filterLabel(hit.filter.category).toLowerCase()}${hit.filter.label ? ` (${hit.filter.label})` : ''}`)
  }, [hit?.act, hit?.filter.start, live]) // eslint-disable-line react-hooks/exhaustive-deps
  // Leaving the player gives the sound back.
  useEffect(() => () => { if (mutedByUs.current && video.current) video.current.muted = false }, []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!note || hit?.act === 'mute' || hit?.act === 'warn') return
    const t = setTimeout(() => setNote(''), 4000)
    return () => clearTimeout(t)
  }, [note, hit?.act])
  useEffect(() => { if (!hit && note.startsWith('Coming up')) setNote('') }, [hit, note])
  if (!note) return null
  return <div className="auto-skip content-note" role="status">{note}</div>
}
