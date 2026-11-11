// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useNavigate } from 'react-router-dom'
import { api, fmtTime, type Item } from '../api'
import { toast } from '../state'
import './pickintro.css'

// Pick in player: play to where the intro ends, and press the button. It goes back to the edit
// dialog it came from. mode "show" sets it for the whole show.
export default function PickIntroBar({ item, time, mode }: { item: Item; time: number; mode: string }) {
  const nav = useNavigate()
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
