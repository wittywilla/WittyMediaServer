// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../api'
import { useApp } from '../state'
import { decodeLevels, defaultLevel, encodeLevels, gainsFor, levelOf, levelsKey, playable, pollDelay, qualityName, sliders, statusLine, type KaraokeAnswer, type Levels } from './stemMix'
import { StemSet, type StemPlayer } from './stems'
import type { PlayerView } from './types'

// Karaoke from a song's separate parts. Opening karaoke asks the server once; it makes a
// quick version if the song has none, and a properly separated one behind it if it can.
// Whatever it has is played in place of the song, with a slider for the voice, and swapped
// for the better one when that's ready. Nobody has to set anything up or press anything.

const readLevels = (key: string): Levels => { try { return decodeLevels(localStorage.getItem(key)) } catch { return {} } }

export function useKaraokeStems(p: PlayerView) {
  const { user } = useApp()
  const key = levelsKey(user?.id)
  const id = p.mode === 'music' && !p.cdActive && !p.live ? p.current?.id : undefined
  const [answer, setAnswer] = useState<KaraokeAnswer | null>(null)
  const [supported, setSupported] = useState(true)
  const [levels, setLevels] = useState<Levels>(() => readLevels(key))
  const [sing, setSing] = useState(false)
  const [active, setActive] = useState(false)
  const [lost, setLost] = useState('')
  const set = useRef<StemPlayer | null>(null)
  const drop = useCallback(() => { set.current?.close(); set.current = null; setActive(false) }, [])

  // Ask once for this song, then as often as the server says while something is being made.
  useEffect(() => {
    setAnswer(null)
    setLost('')
    if (!id) return
    let gone = false
    let timer = 0
    let failures = 0
    const ask = (first: boolean) => {
      (first ? api.post<KaraokeAnswer>(`/media/${id}/karaoke/prepare`) : api.get<KaraokeAnswer>(`/media/${id}/karaoke`)).then((a) => {
        if (gone) return
        failures = 0
        setAnswer(a)
        const wait = pollDelay(a, 0)
        if (wait) timer = window.setTimeout(() => ask(false), wait)
      }).catch((e: unknown) => {
        if (gone) return
        // A server from before this: karaoke stays as it was.
        if (e instanceof ApiError && (e.status === 404 || e.status === 405) && first) { setSupported(false); return }
        if (e instanceof ApiError && e.status >= 400 && e.status < 500 && e.status !== 408 && e.status !== 429) { setAnswer(null); return }
        failures++
        timer = window.setTimeout(() => ask(first), pollDelay(null, failures))
      })
    }
    ask(true)
    return () => { gone = true; window.clearTimeout(timer) }
  }, [id])

  // Load the parts whenever there's a new set of them, and take over from the song.
  const version = playable(answer) ? answer.version : ''
  const normalSpeed = p.rate === 1
  useEffect(() => {
    const el = p.media()
    if (!version || !playable(answer) || !el) { drop(); return }
    setLost('')
    const stop = new AbortController()
    // The parts playing now carry on until the new ones are ready.
    StemSet.open(el, answer.stems, stop.signal).then((s) => {
      if (stop.signal.aborted) { s.close(); return }
      set.current?.close()
      set.current = s
      setActive(true)
      p.setSound({ vocalCut: 0 })
    }).catch((e: unknown) => {
      if (stop.signal.aborted) return
      drop()
      setLost(e instanceof Error && !/^stem \d+$/.test(e.message) && e.name === 'Error' ? e.message : 'The song\'s separate parts couldn\'t be played here, so this is the normal track.')
    })
    return () => stop.abort()
  }, [version, id, normalSpeed]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => { set.current?.close(); set.current = null }, [])

  useEffect(() => {
    if (active && playable(answer)) set.current?.setGains(gainsFor(answer, levels, sing), p.sound.muted ? 0 : p.sound.volume)
  }, [active, answer, levels, sing, p.sound.volume, p.sound.muted])

  const setLevel = (stem: string, v: number) => {
    if (stem === 'vocals') setSing(false)
    setLevels((old) => {
      const next = { ...old, [stem]: Math.min(1, Math.max(0, v)) }
      try { localStorage.setItem(key, encodeLevels(next)) } catch { /* a private window */ }
      return next
    })
  }
  const reset = () => {
    setSing(false)
    setLevels({})
    try { localStorage.removeItem(key) } catch { /* a private window */ }
  }
  return { supported: supported && !!id, answer, active, levels, sing, setSing, setLevel, reset, note: statusLine(answer, lost) }
}

export type KaraokeStemsState = ReturnType<typeof useKaraokeStems>

function Level({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  const pct = Math.round(value * 100)
  return (
    <label className="kar-level">
      <span>{label}</span>
      <input type="range" min={0} max={100} step={5} value={pct} onChange={(e) => onChange(Number(e.target.value) / 100)} aria-valuetext={`${pct}%`} />
      <output className="np-pill">{pct}%</output>
    </label>
  )
}

// The Mix: a row for each part the song has been separated into (Voice, then whatever else there is: Music, Drums,
// Bass, Other), each with its level as a pill, and the three shortcuts under them (board "stems"). Only drawn while
// the parts are what's playing.
export function KaraokeStemsControl({ k }: { k: KaraokeStemsState }) {
  const a = k.answer
  if (!k.active || !playable(a)) return null
  const rows = sliders(a)
  const voice = rows.find((r) => r.id === 'vocals')
  const pristine = !k.sing && rows.every((r) => levelOf(a, k.levels, r.id) === defaultLevel(a, r.id))
  return (
    <div className="kar-stems kar-mix" role="group" aria-label="Mix">
      <div className="kar-mix-rows">
        {rows.map((r) => <Level key={r.id} label={r.id === 'vocals' ? 'Voice' : r.label} value={r.id === 'vocals' && k.sing ? 0 : levelOf(a, k.levels, r.id)} onChange={(n) => k.setLevel(r.id, n)} />)}
      </div>
      <div className="kar-mix-pills">
        {voice && <button type="button" className={'kar-chip' + (k.sing ? ' on' : '')} aria-pressed={k.sing} onClick={() => k.setSing(!k.sing)} title="Take the singer out, or put them back (V)">Voice off</button>}
        {voice && <button type="button" className="kar-chip" onClick={() => k.setLevel('vocals', 0.2)} title="Keep the singer quietly, as a guide">Guide voice</button>}
        <button type="button" className="kar-chip" onClick={k.reset} disabled={pristine}>Reset</button>
      </div>
      <p className="kar-mix-note">{qualityName(a)}. The parts are separated on your server, so the levels are approximate.</p>
    </div>
  )
}
