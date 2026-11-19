// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../api'
import { useApp } from '../state'
import { decodeLevels, defaultLevel, takeOver, encodeLevels, levelOf, routesFor, levelsKey, MIX, playable, pollDelay, qualityName, statusLine, type KaraokeAnswer, type Levels } from './stemMix'
import { StemSet, type StemPlayer } from './stems'
import { decodeFx, encodeFx, FX_OFF, fxKey, type Fx } from './voiceFx'
import type { PlayerView } from './types'
import { klog } from './klog'
import { t, tx } from '../i18n'

// Karaoke from a song's separate parts. Opening karaoke asks the server once; it makes a
// quick version if the song has none, and a properly separated one behind it if it can.
// Whatever it has is played in place of the song, with a slider for the voice, and swapped
// for the better one when that's ready. Nobody has to set anything up or press anything.

// How loud the guide voice is: the singer, quietly, to follow.
const GUIDE_LEVEL = 0.2
// Which of the player's two decks, for the log (klog.ts).
const deckIds = new WeakMap<object, number>()
let decksSeen = 0
const deckId = (el: object) => { if (!deckIds.has(el)) deckIds.set(el, ++decksSeen); return deckIds.get(el) }

const readLevels = (key: string): Levels => { try { return decodeLevels(localStorage.getItem(key)) } catch { return {} } }
const readFx = (key: string): Fx => { try { return decodeFx(localStorage.getItem(key)) } catch { return { ...FX_OFF } } }

export function useKaraokeStems(p: PlayerView) {
  const { user } = useApp()
  const key = levelsKey(user?.id)
  const id = p.mode === 'music' && !p.cdActive && !p.live ? p.current?.id : undefined
  const [answer, setAnswer] = useState<KaraokeAnswer | null>(null)
  const [levels, setLevels] = useState<Levels>(() => readLevels(key))
  const fxStore = fxKey(user?.id)
  const [fx, setFxState] = useState<Fx>(() => readFx(fxStore))
  // Karaoke starts with the singer taken out, every time, in the quick version and the separated one, whatever the
  // Voice slider was left at: the guide voice, or the slider, put them back.
  const [sing, setSing] = useState(true)
  const [active, setActive] = useState(false)
  const [lost, setLost] = useState('')
  const set = useRef<StemPlayer | null>(null)
  // The answer the playing set was made from. The levels are worked out from its parts: a newer answer (the separated
  // version, while it's still being loaded) names other parts, and set on the playing quick version it silenced every
  // one of them until the new set took over (#517).
  const opened = useRef<KaraokeAnswer | null>(null)
  const drop = useCallback(() => { set.current?.close(); set.current = null; opened.current = null; setActive(false) }, [])

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
  // The song's own player, which the parts follow. The player has two and moves between them (gapless, crossfade):
  // parts left following the one that has finished would go quiet with it, so they follow whichever is playing.
  const deck = p.media()
  // What a set of parts is told the moment it takes over (below): the effects further down tell it only when something
  // changes, and a set that replaces another (the quick version making way for the separated one) changes nothing they
  // watch, so it was left silent and with no voice settings at all (#517).
  const now = useRef({ levels, sing, fx, volume: p.sound.muted ? 0 : p.sound.volume })
  now.current = { levels, sing, fx, volume: p.sound.muted ? 0 : p.sound.volume }
  useEffect(() => {
    const el = deck
    klog('parts effect: version', version || '-', 'song', id, 'normal speed', normalSpeed, 'deck', el ? deckId(el) : '-')
    if (!version || !playable(answer) || !el) { drop(); return }
    setLost('')
    const stop = new AbortController()
    // The parts playing now carry on until the new ones are ready.
    StemSet.open(el, answer.stems, stop.signal).then((s) => {
      if (stop.signal.aborted) { s.close(); return }
      const was = set.current
      takeOver(s, answer, now.current)
      set.current = s
      opened.current = answer
      was?.close()
      setActive(true)
    }).catch((e: unknown) => {
      if (stop.signal.aborted) return
      drop()
      setLost(e instanceof Error && !/^stem \d+$/.test(e.message) && e.name === 'Error' ? e.message : t('The song’s separate parts couldn’t be played here, so this is the normal track.'))
    })
    return () => stop.abort()
  }, [version, id, normalSpeed, deck]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => { set.current?.close(); set.current = null }, [])

  useEffect(() => {
    const a = opened.current
    if (active && a) set.current?.setRoutes(routesFor(a, levels, sing), p.sound.muted ? 0 : p.sound.volume)
  }, [active, answer, levels, sing, p.sound.volume, p.sound.muted])
  useEffect(() => { if (active) set.current?.setFx(fx) }, [active, fx])
  // The same function every time, so Voice settings doesn't draw itself again as the words move along.
  const setFx = useCallback((next: Fx) => {
    setFxState(next)
    try { localStorage.setItem(fxStore, encodeFx(next)) } catch { /* a private window */ }
  }, [fxStore])

  const setLevel = (stem: string, v: number) => {
    if (stem === 'vocals') setSing(false)
    setLevels((old) => {
      const next = { ...old, [stem]: Math.min(1, Math.max(0, v)) }
      try { localStorage.setItem(key, encodeLevels(next)) } catch { /* a private window */ }
      return next
    })
  }
  // Back to how karaoke starts: the voice out, the music and the track full.
  const reset = () => {
    setSing(true)
    setLevels({})
    try { localStorage.removeItem(key) } catch { /* a private window */ }
  }
  // Until the parts play, the Mix holds its place, waiting: asked and not answered yet, or being made.
  const waiting = !!id && !active && !lost && answer?.state !== 'unavailable'
  // No separator on the server, but one could be installed: the quick version says a cleaner one is an optional install.
  const installable = answer?.separator === 'installable'
  const admin = !!user?.isAdmin
  const toSettings = () => p.goTowards('/settings/metadata#lyrics')
  // The guide voice, one press from the first screen: the singer quietly back, or out again.
  const guide = !sing && !!answer && levelOf(answer, levels, 'vocals') > 0
  const toggleGuide = () => { if (guide) setSing(true); else setLevel('vocals', GUIDE_LEVEL) }
  return { answer, active, waiting, levels, sing, setSing, setLevel, reset, fx, setFx, installable, admin, toSettings, guide, toggleGuide, note: statusLine(answer, lost, t) }
}

export type KaraokeStemsState = ReturnType<typeof useKaraokeStems>

function Level({ label, value, onChange, disabled }: { label: string; value: number; onChange: (v: number) => void; disabled?: boolean }) {
  const pct = Math.round(value * 100)
  return (
    <label className="kar-level">
      <span>{label}</span>
      <input type="range" min={0} max={100} step={5} value={pct} disabled={disabled} onChange={(e) => onChange(Number(e.target.value) / 100)} aria-valuetext={`${pct}%`} />
      <output className="np-pill">{disabled ? '' : `${pct}%`}</output>
    </label>
  )
}

// The Mix: Voice, Background Music and Track, each with its level as a pill, less any the person has hidden, and
// the shortcuts under them (Voice off and Guide voice go with Voice). Only drawn while the parts are what's playing; until then, the same Mix, still, saying how far along the karaoke version is, so
// nothing on the bar changes shape when it arrives.
const noop = () => {}
export function KaraokeStemsControl({ k, hide = [] }: { k: KaraokeStemsState; hide?: readonly string[] }) {
  const a = k.answer
  const rows = MIX.filter((r) => !hide.includes(r.id))
  const voice = rows.some((r) => r.id === 'vocals')
  if (!k.active || !playable(a)) {
    if (!k.waiting) return null
    return (
      <div className="kar-stems kar-mix kar-mix-wait" role="group" aria-label={t('Mix')} aria-busy="true">
        {rows.length > 0 && <div className="kar-mix-rows">
          {rows.map((r) => <Level key={r.id} label={tx(r.label)} value={1} onChange={noop} disabled />)}
        </div>}
        <div className="kar-mix-pills" aria-hidden="true">
          {voice && <button type="button" className="kar-chip" disabled tabIndex={-1}>{t('Voice off')}</button>}
          {voice && <button type="button" className="kar-chip" disabled tabIndex={-1}>{t('Guide voice')}</button>}
          <button type="button" className="kar-chip" disabled tabIndex={-1}>{t('Reset')}</button>
        </div>
        <p className="kar-mix-note" role="status">{k.note || t('Getting the karaoke version…')}</p>
      </div>
    )
  }
  const pristine = k.sing && MIX.every((r) => levelOf(a, k.levels, r.id) === defaultLevel(a, r.id))
  return (
    <div className="kar-stems kar-mix" role="group" aria-label={t('Mix')}>
      {rows.length > 0 && <div className="kar-mix-rows">
        {rows.map((r) => <Level key={r.id} label={tx(r.label)} value={r.id === 'vocals' && k.sing ? 0 : levelOf(a, k.levels, r.id)} onChange={(n) => k.setLevel(r.id, n)} />)}
      </div>}
      <div className="kar-mix-pills">
        {voice && <button type="button" className={'kar-chip' + (k.sing ? ' on' : '')} aria-pressed={k.sing} onClick={() => k.setSing(!k.sing)} title={t('Take the singer out, or put them back (V)')}>{t('Voice off')}</button>}
        {voice && <button type="button" className="kar-chip" onClick={() => k.setLevel('vocals', GUIDE_LEVEL)} title={t('Keep the singer quietly, as a guide')}>{t('Guide voice')}</button>}
        <button type="button" className="kar-chip" onClick={k.reset} disabled={pristine}>{t('Reset')}</button>
      </div>
      <p className="kar-mix-note">
        {t('{quality}. The parts are separated on your server, so the levels are approximate.', { quality: qualityName(a, t) })}
        {k.installable && a.quality !== 'stems' && (k.admin
          ? <> {t('A cleaner, properly separated version is an optional install:')} <button type="button" className="np-link" onClick={k.toSettings}>{t('Demucs in Settings')}</button>.</>
          : <> {t('A cleaner, properly separated version is an optional install an administrator can add.')}</>)}
      </p>
    </div>
  )
}
