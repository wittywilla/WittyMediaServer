// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The DVD and Blu-ray player as something another device steers (docs/REMOTE-PROTOCOL.md):
// the protocol's commands, mapped onto what a disc session can really do. A disc isn't a
// file: its menus have no place to seek to, a title may forbid skipping, and "next" is the
// next chapter, as on a disc player's remote. The D-pad keys aren't here: they arrive as key
// presses (device.ts), which the player already takes for the disc's menu.
import { useEffect, useRef, type RefObject } from 'react'
import { dvdTrackLabel, type Item, type Presentation } from '../api'
import type { RemoteKey } from '../player/remoteKeys'
import { setSpeed } from '../player/speed'
import type { RemoteCommand } from './device'
import { discState } from './playerStates'
import { mediaVolume, useMediaReports, useRemoteTarget } from './target'

export type DiscSession = {
  item: Item
  video: RefObject<HTMLVideoElement | null>
  pres: Presentation | null
  waiting: boolean
  stopped: boolean
  // What the disc forbids here: 'seek', 'chapter', 'skip', 'pause'.
  locked: (what: string) => boolean
  key: (k: RemoteKey) => void
  seek: (sec: number) => void
  audio: (stream: number) => void
  subtitle: (stream: number) => void // -1 for off
  close: () => void
}

export function useDiscRemote(d: DiscSession) {
  const title = d.pres?.kind === 'title' ? d.pres.title : undefined
  const command = (c: RemoteCommand) => {
    const v = d.video.current
    if (!v) throw new Error('The player isn’t ready yet.')
    switch (c.type) {
      case 'pause': if (d.locked('pause')) throw new Error('The disc doesn’t allow pausing here.'); d.key('pause'); break
      case 'resume': d.key('play'); break
      case 'stop': d.close(); break
      case 'next': case 'previous':
        if (title && d.locked('skip')) throw new Error('The disc doesn’t allow skipping here.')
        d.key(c.type === 'next' ? 'next' : 'prev')
        break
      case 'seek':
        if (!title) throw new Error('A disc’s menu has nowhere to skip to.')
        if (d.locked('seek') || d.locked('chapter')) throw new Error('The disc doesn’t allow skipping here.')
        d.seek(Math.min(c.positionSec != null ? c.positionSec : v.currentTime + (c.bySec || 0), Math.max(0, title.durationSec - 1)))
        break
      case 'rate':
        if (!title) throw new Error('A disc’s menu plays at its own speed.')
        setSpeed(v, c.rate || 1)
        break
      case 'volume': case 'mute': mediaVolume(v, c); break
      case 'audio': {
        const n = Number(c.audio)
        if (!title?.audio.some((a) => a.n === n)) throw new Error('That sound track isn’t on this disc’s title.')
        d.audio(n)
        break
      }
      case 'subtitle': {
        const n = c.subtitle === 'off' ? -1 : Number(c.subtitle)
        if (!title || (n >= 0 && !title.subtitles.some((s) => s.n === n))) throw new Error('Those subtitles aren’t on this disc’s title.')
        d.subtitle(n)
        break
      }
      default: throw new Error('This device can’t do that.')
    }
  }
  useRemoteTarget({ kind: 'disc', itemId: d.item.id, state: () => discState({ itemId: d.item.id, stopped: d.stopped, waiting: d.waiting, pres: d.pres }, d.video.current, dvdTrackLabel), command },
    [d.pres?.serial, d.pres?.kind, title?.audioStream, title?.subtitleStream, d.stopped, d.waiting])
  useMediaReports(() => d.video.current, [d.item.id])

  // Started from another device with a sound track or subtitles named: chosen once the
  // title is up, if the title has them. Ids are the disc's own stream numbers.
  const asked = useRef<{ audio: string | null; subtitle: string | null } | null>(null)
  if (!asked.current) { const q = new URLSearchParams(location.search); asked.current = q.get('remote') ? { audio: q.get('audio'), subtitle: q.get('subtitle') } : { audio: null, subtitle: null } }
  useEffect(() => {
    const want = asked.current!
    if (!title || (want.audio == null && want.subtitle == null)) return
    asked.current = { audio: null, subtitle: null }
    const a = Number(want.audio), s = want.subtitle === 'off' ? -1 : Number(want.subtitle)
    if (want.audio != null && a !== title.audioStream && title.audio.some((x) => x.n === a)) d.audio(a)
    if (want.subtitle != null && s !== title.subtitleStream && (s < 0 || title.subtitles.some((x) => x.n === s))) d.subtitle(s)
  }, [title?.ttn]) // eslint-disable-line react-hooks/exhaustive-deps
}
