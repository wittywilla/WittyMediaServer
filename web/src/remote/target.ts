// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// How a player makes itself the thing another device's remote steers (device.ts): one hook,
// given what the player is, what it reports and how it obeys. The mapping from the
// protocol's commands onto each player is with the player, or beside it in this folder.
import { useEffect, useRef } from 'react'
import { registerPlayer, reportSoon, type RemoteCommand, type RemoteState } from './device'
import type { PlayerKind } from './capabilities'

export type RemoteTarget = {
  kind: PlayerKind
  itemId: number
  // False while there is nothing to steer (the music bar with an empty queue).
  on?: boolean
  background?: boolean
  state: () => Omit<RemoteState, 'v'>
  command: (cmd: RemoteCommand) => void | Promise<void>
}

// useRemoteTarget registers the player for as long as it is mounted and on. `changes` are the
// things a remote would show: when one changes, the new state is reported shortly.
export function useRemoteTarget(target: RemoteTarget, changes: readonly unknown[]) {
  const now = useRef(target)
  now.current = target
  const on = target.on !== false
  useEffect(() => {
    if (!on) return
    return registerPlayer({
      kind: target.kind, itemId: target.itemId, background: target.background,
      state: () => ({ ...now.current.state() }),
      command: (cmd) => now.current.command(cmd),
    })
  }, [on, target.kind, target.itemId, target.background])
  useEffect(() => { reportSoon() }, changes) // eslint-disable-line react-hooks/exhaustive-deps
}

// A media element's own events are changes too: it plays, pauses and seeks without React hearing.
const MEDIA_EVENTS = ['play', 'pause', 'seeked', 'ratechange', 'volumechange', 'ended', 'waiting', 'playing']
export function useMediaReports(el: () => HTMLMediaElement | null, deps: readonly unknown[]) {
  useEffect(() => {
    const m = el()
    if (!m) return
    MEDIA_EVENTS.forEach((e) => m.addEventListener(e, reportSoon))
    return () => MEDIA_EVENTS.forEach((e) => m.removeEventListener(e, reportSoon))
  }, deps) // eslint-disable-line react-hooks/exhaustive-deps
}

// The volume and mute commands, for a player whose sound is a media element's.
export function mediaVolume(m: HTMLMediaElement, cmd: RemoteCommand) {
  if (cmd.type === 'volume') { m.volume = Math.max(0, Math.min(1, cmd.level ?? m.volume)); if (m.volume > 0) m.muted = false } else m.muted = !!cmd.muted
}
