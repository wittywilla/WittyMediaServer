// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getToken, serverBase } from '../api'
import { canRoute, openGraph, type SoundGraph } from './sound'
import { drifted, MAX_SECONDS, startPlan, type Stem } from './stemMix'

// Plays a song's separate parts in place of the song. The song's own <audio> keeps
// playing, unheard, and stays in charge: lyrics, seeking, pausing, the lock screen and
// the queue all still follow it. The parts are decoded whole and started together on the
// audio engine's clock, so they are on the same sample as each other, and are started
// again whenever the song pauses, moves, or they have wandered from it.

async function fetchSound(ctx: AudioContext, url: string, signal: AbortSignal): Promise<AudioBuffer> {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers['Authorization'] = 'Bearer ' + tok
  const res = await fetch(serverBase() + url, { headers, credentials: 'same-origin', signal })
  if (!res.ok) throw new Error('stem ' + res.status)
  return await ctx.decodeAudioData(await res.arrayBuffer())
}

export class StemSet {
  private gains = new Map<string, GainNode>()
  private sources: AudioBufferSourceNode[] = []
  private out: GainNode
  private startAt = 0
  private startCtx = 0
  private running = false
  private timer = 0
  private closed = false
  private onEvent = () => this.sync()

  private constructor(private g: SoundGraph, private el: HTMLAudioElement, private buffers: Map<string, AudioBuffer>, private onLost: (why: string) => void) {
    const ctx = g.ctx
    this.out = ctx.createGain()
    this.out.connect(g.stemIn)
    for (const id of buffers.keys()) {
      const gain = ctx.createGain()
      gain.gain.value = 0
      gain.connect(this.out)
      this.gains.set(id, gain)
    }
    for (const ev of ['play', 'playing', 'pause', 'seeking', 'seeked', 'waiting', 'ratechange', 'ended', 'emptied']) el.addEventListener(ev, this.onEvent)
    this.timer = window.setInterval(() => this.watch(), 2000)
    g.hushDecks(true)
    this.sync()
  }

  // Fetches and decodes every part, and takes over from the song. Throws when any part
  // can't be had: the caller keeps the normal track.
  static async open(el: HTMLAudioElement, stems: Stem[], signal: AbortSignal, onLost: (why: string) => void): Promise<StemSet> {
    if (!canRoute(el)) throw new Error('The parts can\'t be played here, as the server is on a different address from this page.')
    if (Number.isFinite(el.duration) && el.duration > MAX_SECONDS) throw new Error('This song is too long to play from its parts here.')
    const g = await openGraph()
    if (!g) throw new Error('This browser wouldn\'t start its audio engine.')
    const list = await Promise.all(stems.map((s) => fetchSound(g.ctx, s.url, signal)))
    if (signal.aborted) throw new DOMException('gone', 'AbortError')
    if (!g.route(el)) throw new Error('This browser wouldn\'t start its audio engine.')
    return new StemSet(g, el, new Map(stems.map((s, i) => [s.id, list[i]])), onLost)
  }

  setGains(gains: Record<string, number>, volume: number) {
    const now = this.g.ctx.currentTime
    for (const [id, node] of this.gains) node.gain.setTargetAtTime(gains[id] ?? 0, now, 0.03)
    this.out.gain.setTargetAtTime(volume, now, 0.03)
  }

  private stopSources() {
    for (const s of this.sources) { try { s.stop() } catch { /* never started */ } s.disconnect() }
    this.sources = []
    this.running = false
  }

  // Starts every part again from where the song is, or stops them if it isn't playing.
  private sync() {
    if (this.closed) return
    this.stopSources()
    const el = this.el
    // Parts can't keep their pitch at another speed the way the song's own player does.
    if (el.playbackRate !== 1) { this.onLost('The song\'s separate parts only play at normal speed, so this is the normal track.'); return }
    if (el.paused || el.ended || el.seeking || el.readyState < 3) return
    const ctx = this.g.ctx
    const plan = startPlan(el.currentTime, ctx.currentTime)
    for (const [id, buf] of this.buffers) {
      if (plan.offset >= buf.duration) continue
      const src = ctx.createBufferSource()
      src.buffer = buf
      src.connect(this.gains.get(id)!)
      src.start(plan.when, plan.offset)
      this.sources.push(src)
    }
    this.startAt = plan.offset
    this.startCtx = plan.when
    this.running = true
  }

  private watch() {
    if (this.closed) return
    const el = this.el
    const playing = !el.paused && !el.ended && !el.seeking && el.readyState >= 3
    if (playing !== this.running || (playing && drifted(this.startAt, this.startCtx, this.g.ctx.currentTime, el.currentTime))) this.sync()
  }

  // Gives the sound back to the song.
  close() {
    if (this.closed) return
    this.closed = true
    window.clearInterval(this.timer)
    for (const ev of ['play', 'playing', 'pause', 'seeking', 'seeked', 'waiting', 'ratechange', 'ended', 'emptied']) this.el.removeEventListener(ev, this.onEvent)
    this.stopSources()
    this.out.disconnect()
    this.g.hushDecks(false)
  }
}
