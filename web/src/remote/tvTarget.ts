// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Live TV as something another device steers (docs/REMOTE-PROTOCOL.md). A channel isn't a
// library item, so the protocol's `play` can't name one, and a state with no item is listed
// by the server as idle: what a remote is told is that this screen is on live TV (`screen`),
// and, when the programme on now is a library item (a channel made from the library), that
// item and how far in it is. It answers pause, resume, stop, volume, sound and subtitle
// tracks, back and forward within what the server has kept, and next and previous as
// channel up and down.
import type { RefObject } from 'react'
import type { RemoteCommand } from './device'
import { tvState, type TvTold } from './playerStates'
import { mediaVolume, useMediaReports, useRemoteTarget } from './target'

export type TvSession = Omit<TvTold, 'behindSec'> & {
  video: RefObject<HTMLVideoElement | null>
  chooseAudio: (id: number) => void
  chooseSubtitle: (id: number) => void // -1 for off
  seekBy: (sec: number) => void
  step: (dir: 1 | -1) => void
  stop: () => void
}

// How far behind the channel this viewer is, in seconds: the TV section's player works it out
// from the stream's own clock as it keeps everyone in step (TvHost.tsx), and leaves it on the video.
const behind = (v: HTMLVideoElement | null) => Math.max(0, Number(v?.dataset.behind) || 0)

export function useTvRemote(t: TvSession) {
  const command = (c: RemoteCommand) => {
    const v = t.video.current
    if (!v) throw new Error('The player isn’t ready yet.')
    switch (c.type) {
      case 'pause': v.pause(); break
      case 'resume': v.play().catch(() => {}); break
      case 'stop': t.stop(); break
      case 'next': t.step(1); break
      case 'previous': t.step(-1); break
      case 'seek':
        if (c.bySec == null) throw new Error('Live TV can only go back or forward from where it is.')
        t.seekBy(c.bySec)
        break
      case 'volume': case 'mute': mediaVolume(v, c); break
      case 'audio': {
        const id = Number(c.audio)
        if (!t.audio.some((a) => a.id === id)) throw new Error('That sound track isn’t on this channel.')
        t.chooseAudio(id)
        break
      }
      case 'subtitle': {
        const id = c.subtitle === 'off' ? -1 : Number(c.subtitle)
        if (id >= 0 && !t.subtitles.some((s) => s.id === id)) throw new Error('Those subtitles aren’t on this channel.')
        t.chooseSubtitle(id)
        break
      }
      default: throw new Error('This device can’t do that.')
    }
  }
  useRemoteTarget({ kind: 'tv', itemId: t.now?.itemId || 0, state: () => tvState({ ...t, behindSec: behind(t.video.current) }, t.video.current, Date.now() / 1000), command },
    [t.now?.itemId, t.now?.start, t.ready, t.audio, t.subtitles])
  useMediaReports(() => t.video.current, [])
}
