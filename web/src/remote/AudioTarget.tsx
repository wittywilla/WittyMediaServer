// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The audio player as something another device steers (docs/REMOTE-PROTOCOL.md): music,
// audiobooks and podcasts, radio stations, and a CD in the player. It has its own queue,
// which is the one reported and the one next and previous move along. Mounted by the
// audio player beside its bar; it draws nothing.
import { useCd } from '../audio/player'
import type { PlayerView } from '../audio/types'
import type { RemoteCommand } from './device'
import { audioPlayerKind, audioState } from './playerStates'
import { seekTarget } from './position'
import { useMediaReports, useRemoteTarget } from './target'

export default function AudioTarget({ p }: { p: PlayerView }) {
  const cd = useCd()
  const kind = audioPlayerKind(p)
  // A disc in the player: the item is the disc, and the place is the place in its track.
  const disc = p.cdActive && cd.now ? { id: cd.now.disc.id, trackLen: cd.now.trackLen } : null
  const place = () => (disc ? Math.max(0, cd.now?.trackSec || 0) : p.position())
  const command = (c: RemoteCommand) => {
    if (!p.current) throw new Error('Nothing is playing there.')
    switch (c.type) {
      case 'pause': if (p.playing) p.toggle(); break
      case 'resume': if (!p.playing) p.toggle(); break
      case 'stop': p.stop(); break
      case 'next': p.next(); break
      case 'previous': p.prev(); break
      case 'seek': p.seek(seekTarget(c.positionSec != null ? c.positionSec : place(), c.positionSec != null ? 0 : c.bySec || 0, disc ? disc.trackLen : p.duration)); break
      case 'volume': p.setSound({ volume: Math.max(0, Math.min(1, c.level ?? p.sound.volume)), muted: false }); break
      case 'mute': p.setSound({ muted: !!c.muted }); break
      case 'rate': p.changeRate(Math.min(c.rate || 1, p.maxSpeed)); break
      default: throw new Error('This device can’t do that.')
    }
  }
  useRemoteTarget({ kind, itemId: disc ? disc.id : p.current?.id || 0, on: !!p.current, background: true, state: () => audioState(p, place(), disc), command },
    [p.current?.id, p.playing, p.retrying, p.queue, p.index, p.rate, p.sound.volume, p.sound.muted, p.duration])
  // A seek moves the place without anything above changing.
  useMediaReports(p.media, [p.current?.id, kind])
  return null
}
