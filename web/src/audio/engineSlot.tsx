// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from 'react'
import type { Item } from '../api'
import { playerOwner, registrationOf, useModuleWeb } from '../moduleRegistry'
import type { Repeat } from './types'

// The audio-engine slot (web host API 5): something a module plays through the app's own audio
// element, such as an audio CD (WittyCD), with the bar along the bottom, the lock screen and the
// remote following it. The module that claims the player for a media kind (integration.json
// players) hands over its engine as registration.audioEngines[kind]: a React hook taking the
// audio element and these hooks, returning what the audio player needs. The app's player calls
// useAudioEngineSlot, whose hooks never change in number; the module's hook runs in a component
// of its own (EngineRunner), mounted once the bundle has loaded, and its answer comes back here.

/** What the audio player tells an engine. */
export type EngineHooks = {
  /** A disc went in (its tracks as items, for the bar and the lock screen) or came out (null). */
  onDisc: (items: Item[] | null) => void
  onTrack: (index: number) => void
  onTime: (trackSec: number) => void
}

/** What the app reads of what an engine is playing; the engine's own screens read the rest. */
export type EngineNow = { disc: { id: number }; shuffle: boolean; repeat: Repeat; trackSec: number; trackLen: number; [k: string]: unknown }

/** What the app asks of an engine; its own screens ask the rest. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type EngineActions = { eject: () => void; next: () => void; prev: () => void; seekInTrack: (sec: number) => void; setShuffle: (on: boolean) => void; setRepeat: (r: Repeat) => void; [k: string]: (...a: any[]) => any }

/** What an engine's hook returns. */
export type AudioEngine = {
  now: EngineNow | null
  actions: EngineActions
  onTimeUpdate: () => void
  onPlay: () => void
  onPause: () => void
  onEnded: () => void
  onWaiting: () => void
  onError: () => boolean
}

type EngineHook = (audio: RefObject<HTMLAudioElement | null>, hooks: EngineHooks) => AudioEngine

/** The engine for a media kind, if a module of API 5 in use claims its player, and the element
 * that runs it, to be drawn anywhere inside the audio player's tree. Until it is loaded, nothing
 * plays: now is null and every action does nothing. */
export function useAudioEngineSlot(kind: string, audio: RefObject<HTMLAudioElement | null>, hooks: EngineHooks): { engine: AudioEngine; runner: ReactNode } {
  const owner = playerOwner(useModuleWeb(), kind)
  const [hook, setHook] = useState<EngineHook | null>(null)
  const [now, setNow] = useState<EngineNow | null>(null)
  const live = useRef<AudioEngine | null>(null)
  const hooksRef = useRef(hooks)
  hooksRef.current = hooks
  useEffect(() => {
    setHook(null)
    if (!owner) return
    let on = true
    // Once the page is idle: a disc's page loads the bundle anyway, and a disc playing on is
    // put in by it.
    const go = () => registrationOf(owner).then((r) => {
      const h = r?.audioEngines?.[kind]
      if (on && typeof h === 'function') setHook(() => h as EngineHook)
    })
    const idle = (window as unknown as { requestIdleCallback?: (f: () => void) => number }).requestIdleCallback
    if (idle) idle(go); else setTimeout(go, 1)
    return () => { on = false }
  }, [owner, kind])
  // One object for the app's whole life: what it calls goes to the engine there is now.
  const engine = useMemo<AudioEngine>(() => {
    const actions = new Proxy({} as EngineActions, {
      get: (_, k: string) => (...a: unknown[]) => live.current?.actions[k]?.(...a),
    })
    return {
      now: null,
      actions,
      onTimeUpdate: () => live.current?.onTimeUpdate(),
      onPlay: () => live.current?.onPlay(),
      onPause: () => live.current?.onPause(),
      onEnded: () => live.current?.onEnded(),
      onWaiting: () => live.current?.onWaiting(),
      onError: () => live.current?.onError() ?? false,
    }
  }, [])
  const stable = hooksRef
  const forward = useMemo<EngineHooks>(() => ({
    onDisc: (items) => stable.current.onDisc(items),
    onTrack: (i) => stable.current.onTrack(i),
    onTime: (t) => stable.current.onTime(t),
  }), [stable])
  const runner = hook ? <EngineRunner key={owner} useEngine={hook} audio={audio} hooks={forward} live={live} onNow={setNow} /> : null
  return { engine: useMemo(() => ({ ...engine, now }), [engine, now]), runner }
}

// EngineRunner calls the module's hook, always the same one, and hands its answer back.
function EngineRunner({ useEngine, audio, hooks, live, onNow }: {
  useEngine: EngineHook; audio: RefObject<HTMLAudioElement | null>; hooks: EngineHooks
  live: { current: AudioEngine | null }; onNow: (n: EngineNow | null) => void
}) {
  const e = useEngine(audio, hooks)
  live.current = e
  useEffect(() => { onNow(e.now) }, [e.now, onNow])
  useEffect(() => () => { live.current = null; onNow(null) }, [live, onNow])
  return null
}
