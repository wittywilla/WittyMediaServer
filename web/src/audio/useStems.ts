// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { announce } from '../a11y'
import { api } from '../api'
import { useWatched } from '../extras/jobs'
import { toast } from '../ui'
import { openGraph, soundGraph } from './sound'
import { BufferStemPlayer, StemError, TrackStemPlayer, canSwitchTracks, type StemPlayer } from './stemEngine'
import {
  backingOf, carryChoice, carryPosition, defaultLevels, sourcesOf, stemGains, stemsToLoad, voiceChoices,
  type ChoiceId, type Levels, type Sources, type StemName, type StemsAnswer, type VoiceChoice,
} from './stems'
import type { PlayerView } from './types'

// Karaoke's stems, between the screen and the sound. The ordinary player keeps playing
// the song until the parts are fetched and ready; then it is paused where it is and the
// parts carry on from that second, and from then on they are the clock the words follow.
// Going back, or leaving karaoke, the ordinary player picks the song up where the parts
// are. The rules are in stems.ts.

export type StemLoad = { state: 'idle' | 'loading' | 'ready' | 'failed'; progress: number; error: string; partial: boolean }
const IDLE: StemLoad = { state: 'idle', progress: 0, error: '', partial: false }

export type StemKaraoke = {
  answer: StemsAnswer | null
  sources: Sources
  choices: VoiceChoice[]
  choice: VoiceChoice
  choose: (id: ChoiceId) => void
  levels: Levels
  setLevel: (name: StemName, level: number) => void
  resetLevels: () => void
  load: StemLoad
  active: boolean // the parts are playing the song, not the ordinary player
  canLevel: boolean
  loaded: StemName[]
  multitrack: 'none' | 'used' | 'unsupported' | 'unused'
  reload: () => void
  // Having the server make what the song lacks. making is the request on its way; the
  // listing names the task from then on.
  makeApproximate: () => void
  separate: () => void
  starting: '' | 'approximate' | 'external'
  // The player as karaoke should see it: the parts' clock and controls while they play.
  position: () => number
  seek: (t: number) => void
  toggle: () => void
  playing: boolean
}

const EMPTY: StemsAnswer['stems'] = []

export function useStemKaraoke(p: PlayerView): StemKaraoke {
  const it = p.current!
  const id = it.id
  const listing = useWatched<StemsAnswer>(
    (signal) => api.get<StemsAnswer>(`/media/${id}/stems`, signal),
    (a) => !!(a.separation?.approximate?.task || a.separation?.external?.task),
    String(id),
  )
  const answer = listing.value
  const stems = answer?.stems || EMPTY
  const [unusable, setUnusable] = useState(false) // this browser couldn't load or decode them
  // The listing is read again while something is being made; the parts are the same parts
  // unless their names, sizes or makers change.
  const stemsKey = stems.map((s) => s.name + s.size + s.source).join('|')
  const named = useMemo(() => sourcesOf(stems), [stemsKey]) // eslint-disable-line react-hooks/exhaustive-deps
  // A multitrack file stands in only when there are no separate parts to use.
  const viaTracks = named.multitrack && backingOf(named) === 'none' && canSwitchTracks()
  const sources = useMemo<Sources>(() => (viaTracks ? { ...named, vocals: true, drums: true, bass: true, other: true } : named), [named, viaTracks])
  const multitrack: StemKaraoke['multitrack'] = !named.multitrack ? 'none' : viaTracks ? 'used' : backingOf(named) !== 'none' ? 'unused' : 'unsupported'
  // Tracks of one file are on or off, so there is no "lower" to offer with them.
  const choices = useMemo(() => voiceChoices(sources, !unusable).filter((c) => !(viaTracks && c.engine === 'stems' && c.id === 'lower')), [sources, unusable, viaTracks])

  const [wanted, setWanted] = useState<ChoiceId>('on')
  const choiceId = carryChoice(wanted, choices)
  const choice = choices.find((c) => c.id === choiceId) || choices[0]
  const [levels, setLevels] = useState<Levels>(() => defaultLevels(sources, 'on'))
  const levelsRef = useRef(levels)
  levelsRef.current = levels
  const [load, setLoad] = useState<StemLoad>(IDLE)
  const [active, setActive] = useState(false)
  const [stemPlaying, setStemPlaying] = useState(false)
  const engine = useRef<StemPlayer | null>(null)
  const fetching = useRef<AbortController | null>(null) // the fetch of this song's parts, to give up
  const activeRef = useRef(false)
  const pRef = useRef(p)
  pRef.current = p
  const sourcesRef = useRef(sources)
  sourcesRef.current = sources

  // The ordinary player takes the song back, from where the parts are.
  const handBack = useCallback(() => {
    const e = engine.current
    if (!e || !activeRef.current) return
    const at = e.position(), was = e.playing
    e.pause()
    activeRef.current = false
    setActive(false)
    setStemPlaying(false)
    const pl = pRef.current
    pl.seek(carryPosition(at, pl.duration || e.duration))
    const deck = pl.media()
    if (was) { if (deck?.paused) pl.toggle() } else deck?.pause() // a converted stream starts itself on a seek
  }, [])

  // The parts take the song over, from where the ordinary player is.
  const takeOver = useCallback(() => {
    const e = engine.current
    if (!e || activeRef.current) return
    const pl = pRef.current
    const at = pl.position(), was = pl.playing
    activeRef.current = true
    setActive(true)
    pl.media()?.pause()
    e.setLevels(levelsRef.current, stemGains(sourcesRef.current, levelsRef.current))
    e.setVolume(pl.sound.muted ? 0 : pl.sound.volume)
    e.seek(at)
    if (was) e.play()
    // Key and equaliser are set by the ordinary player only while it plays; here they're set directly.
    const g = soundGraph()
    if (g && e.canLevel) { g.setVocalCut(0); g.setEq(pl.sound.eqOn, pl.sound.eqGains, pl.sound.preamp); void g.setKey(pl.sound.keyShift) }
  }, [])

  const dropEngine = useCallback(() => {
    fetching.current?.abort()
    fetching.current = null
    engine.current?.destroy()
    engine.current = null
    activeRef.current = false
    setActive(false)
    setStemPlaying(false)
    setLoad(IDLE)
  }, [])

  // Leaving karaoke while the parts play: the song carries on in the ordinary player.
  // Declared before what throws the parts away, so on the way out it runs first.
  useEffect(() => () => { handBack() }, [handBack])

  // Another song: its parts are its own. The song itself is already playing in the ordinary
  // player, so there is nothing to hand back.
  useEffect(() => { setUnusable(false); return dropEngine }, [id, dropEngine])

  // New parts on the server (just made, or replaced): what was fetched is stale.
  const knownKey = useRef(stemsKey)
  useEffect(() => {
    if (knownKey.current === stemsKey) return
    knownKey.current = stemsKey
    if (activeRef.current) handBack()
    dropEngine()
    setUnusable(false)
  }, [stemsKey, handBack, dropEngine])

  // Fetch and decode when a choice needs the parts, and take over once they're ready.
  const wantsStems = choice.engine === 'stems'
  const hasAnswer = !!answer
  useEffect(() => {
    if (!wantsStems || !hasAnswer) return
    if (load.state === 'ready') { if (engine.current) takeOver(); return }
    if (load.state !== 'idle') return
    const stop = new AbortController()
    fetching.current = stop
    let mine: StemPlayer | null = null
    setLoad({ state: 'loading', progress: 0, error: '', partial: false })
    void (async () => {
      try {
        let partial = false
        if (viaTracks) {
          const t = new TrackStemPlayer()
          mine = t
          await t.load(stems.find((s) => s.multitrack || s.name === 'multitrack')!, stop.signal)
        } else {
          const plan = stemsToLoad(named, levelsRef.current, pRef.current.duration || it.runtimeSec || 0)
          if (!plan.ok) throw new StemError('This song is too long for its stems to be played together in a browser.')
          partial = !plan.all
          const g = await openGraph()
          if (!g) throw new StemError('This browser wouldn\'t start its audio engine, which is what plays stems together.')
          const b = new BufferStemPlayer(g)
          mine = b
          await b.load(stems, plan.names, stop.signal, (f) => setLoad((l) => (l.state === 'loading' ? { ...l, progress: f } : l)))
        }
        if (stop.signal.aborted) { mine.destroy(); return }
        const e = mine
        e.onState = (on) => { if (engine.current === e) setStemPlaying(on) }
        e.onEnded = () => {
          if (engine.current !== e || !activeRef.current) return
          // The end of the song: the queue moves on as it would have. With nothing after
          // it, the parts stay at the end, and Play starts them from the top.
          const pl = pRef.current
          if (pl.repeat === 'one') { e.seek(0); e.play(); return }
          if (pl.sleepAt === 'chapter') { pl.setSleepAt(null); return }
          if (pl.index + 1 < pl.queue.length || (pl.repeat === 'all' && pl.queue.length > 0)) pl.next()
        }
        engine.current = e
        setLoad({ state: 'ready', progress: 1, error: '', partial })
      } catch (err) {
        mine?.destroy()
        if (stop.signal.aborted || (err as Error).name === 'AbortError') return
        const words = err instanceof StemError ? err.message : 'The stems couldn\'t be fetched. Check the connection and choose it again.'
        setLoad({ state: 'failed', progress: 0, error: words, partial: false })
        // What can't be decoded here never will be; a dropped connection may well come back.
        if (err instanceof StemError) setUnusable(true)
        announce(words)
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wantsStems, hasAnswer, load.state, stemsKey])

  // Going back to the song as it is.
  useEffect(() => { if (!wantsStems && activeRef.current) handBack() }, [wantsStems, handBack])

  // The sliders start where the choice puts them, for this song's parts.
  useEffect(() => { setLevels(defaultLevels(sources, choiceId)) }, [sources, choiceId])
  useEffect(() => { engine.current?.setLevels(levels, stemGains(sources, levels)) }, [levels, sources, active])
  useEffect(() => { engine.current?.setVolume(p.sound.muted ? 0 : p.sound.volume) }, [p.sound.volume, p.sound.muted, active])
  // The key, while the parts play (the ordinary player sets it only while it is playing).
  useEffect(() => { if (active && engine.current?.canLevel) void soundGraph()?.setKey(p.sound.keyShift) }, [p.sound.keyShift, active])

  // Something else pressed play on the ordinary player (a headset's button, the lock
  // screen) while the parts have the song: that means "play", so the parts do.
  useEffect(() => {
    if (!active) return
    const deck = p.media()
    if (!deck) return
    const onPlay = () => { if (!activeRef.current) return; deck.pause(); engine.current?.play() }
    deck.addEventListener('play', onPlay)
    return () => deck.removeEventListener('play', onPlay)
  }, [active, p.media])

  const choose = useCallback((next: ChoiceId) => {
    const c = choices.find((x) => x.id === next)
    if (!c) return
    setWanted(next)
    // A failed fetch is tried again by choosing it again.
    if (c.engine === 'stems') setLoad((l) => (l.state === 'failed' ? IDLE : l))
    pRef.current.setSound({ vocalCut: c.engine === 'mix' ? c.vocalCut : 0 })
    // From this press, where a browser wants one before sound can be made.
    if (c.engine === 'stems') void openGraph()
  }, [choices])

  // A song without the wanted choice plays the nearest one: the ordinary player is told too.
  useEffect(() => {
    const cut = choice.engine === 'mix' ? choice.vocalCut : 0
    if (pRef.current.sound.vocalCut !== cut) pRef.current.setSound({ vocalCut: cut })
  }, [choice])

  // Asking the server to make what the song lacks. The listing then names the task, and is
  // read again until it is gone.
  const [starting, setStarting] = useState<StemKaraoke['starting']>('')
  const start = (way: 'approximate' | 'external') => {
    if (starting) return
    setStarting(way)
    api.post(`/media/${id}/stems/${way === 'approximate' ? 'approximate' : 'separate'}`)
      .then(() => listing.reload())
      .catch((e: Error) => toast(e.message, { tone: 'danger' }))
      .finally(() => setStarting(''))
  }
  // Said once when a task ends, wherever the focus is: the panel may be closed by then.
  const tasks = useRef({ id, approximate: '', external: '' })
  useEffect(() => {
    if (!answer) return
    const was = tasks.current.id === id ? tasks.current : { id, approximate: '', external: '' }
    const now = { id, approximate: answer.separation.approximate.task || '', external: answer.separation.external.task || '' }
    tasks.current = now
    if (was.approximate && !now.approximate) announce(answer.separation.approximate.error || (named.instrumental ? 'The approximate instrumental is ready.' : 'The approximate instrumental wasn\'t made.'))
    if (was.external && !now.external) announce(answer.separation.external.error ? 'The separator failed.' : named.vocals || named.instrumental ? 'The stems are ready.' : 'The song wasn\'t separated.')
  }, [answer, id, named])

  const position = useCallback(() => (activeRef.current && engine.current ? engine.current.position() : pRef.current.position()), [])
  const seek = useCallback((t: number) => { if (activeRef.current && engine.current) engine.current.seek(t); else pRef.current.seek(t) }, [])
  const toggle = useCallback(() => {
    const e = engine.current
    if (activeRef.current && e) { if (e.playing) e.pause(); else e.play() } else pRef.current.toggle()
  }, [])

  return {
    answer, sources, choices, choice, choose, levels,
    setLevel: (name, level) => setLevels((l) => ({ ...l, [name]: Math.min(1, Math.max(0, level)) })),
    resetLevels: () => setLevels(defaultLevels(sources, choiceId)),
    load, active, canLevel: engine.current?.canLevel ?? !viaTracks, loaded: engine.current?.names || [],
    multitrack, reload: listing.reload, makeApproximate: () => start('approximate'), separate: () => start('external'), starting, position, seek, toggle, playing: active ? stemPlaying : p.playing,
  }
}
