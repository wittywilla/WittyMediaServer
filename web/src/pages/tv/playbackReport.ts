// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The playback report: what the live player saw, in plain text for the owner to paste into an issue or a
// message, so a problem on their connection can be read rather than guessed at. The segment timeline, the
// buffers, how the device reaches the server, the server's version and the encoder it chose. Nothing that
// signs in or finds the server: no addresses, tokens, keys or IDs.

import type { AutoRecord, FragRecord, StallRecord } from './liveStats'

export type ReportData = {
  when: string
  version: string
  channel: string
  via: string
  away: boolean
  choice: string // what the stream was tuned with: the viewer's choice or Auto's step
  autoStep: number | null
  autoStepText: string // the step's cap, as "720p 3000 kbit/s" or "no cap"
  encoder: string
  gpu: string
  clock: number
  buffered: [number, number][]
  paused: boolean
  readyState: number
  frames: { total: number; dropped: number }
  size: string
  browser: string
  frags: FragRecord[]
  stalls: StallRecord[]
  autoChanges: AutoRecord[]
}

const kbit = (bytes: number, ms: number) => (ms > 0 ? Math.round((bytes * 8) / ms) : 0) // kbit/s from bytes and ms

export function formatReport(d: ReportData): string {
  const lines: string[] = []
  lines.push('WittyTV playback report', `When: ${d.when}`, `Server: ${d.version || 'unknown'}`, `Browser: ${d.browser}`)
  lines.push(`Channel: ${d.channel}`, `Connection: ${d.via || 'unknown'}${d.away ? ', away from home' : ', at home'}`)
  lines.push(`Quality: ${d.choice || 'full'}${d.autoStep != null ? ` (Auto step ${d.autoStep}: ${d.autoStepText})` : ''}`)
  lines.push(`Encoder: ${d.encoder || 'unknown'}${d.gpu ? ` (${d.gpu})` : ''}`)
  const ahead = d.buffered.length ? d.buffered[d.buffered.length - 1][1] - d.clock : 0
  lines.push(`Player: at ${d.clock.toFixed(1)} s, ${d.paused ? 'paused' : 'playing'}, ready state ${d.readyState}, ${ahead.toFixed(1)} s buffered ahead, picture ${d.size || '?'}`)
  lines.push(`Buffered: ${d.buffered.map(([a, b]) => `${a.toFixed(1)}-${b.toFixed(1)}`).join(', ') || 'nothing'}`)
  lines.push(`Frames: ${d.frames.total} shown, ${d.frames.dropped} dropped`)
  lines.push(`Stalls: ${d.stalls.length}${d.stalls.length ? ' at ' + d.stalls.map((s) => (s.at / 1000).toFixed(1) + ' s').join(', ') : ''}`)
  if (d.autoChanges.length) lines.push(`Auto changes: ${d.autoChanges.map((c) => `${(c.at / 1000).toFixed(0)} s step ${c.from}→${c.to} (${c.why})`).join('; ')}`)
  if (d.frags.length) {
    const bytes = d.frags.reduce((n, f) => n + f.bytes, 0), sec = d.frags.reduce((n, f) => n + f.durationSec, 0), ms = d.frags.reduce((n, f) => n + f.loadMs, 0)
    lines.push(`Segments: ${d.frags.length}, stream ${sec > 0 ? Math.round((bytes * 8) / sec / 1000) : 0} kbit/s, link while loading ${kbit(bytes, ms)} kbit/s`)
    lines.push('  at(s)  seg  KB  length(s)  first byte(ms)  load(ms)  kbit/s')
    for (const f of d.frags) {
      lines.push(`  ${(f.at / 1000).toFixed(1).padStart(6)} ${String(f.sn).padStart(4)} ${String(Math.round(f.bytes / 1024)).padStart(5)} ${f.durationSec.toFixed(2).padStart(6)} ${String(Math.round(f.firstByteMs)).padStart(8)} ${String(Math.round(f.loadMs)).padStart(8)} ${String(kbit(f.bytes, f.loadMs)).padStart(7)}`)
    }
  } else lines.push('Segments: none loaded yet')
  return lines.join('\n') + '\n'
}
