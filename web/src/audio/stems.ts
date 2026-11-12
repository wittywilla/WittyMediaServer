// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getToken, mediaUrl, serverBase } from '../api'
import { canRoute, openGraph, type SoundGraph } from './sound'
import { drifted, startPlan, streamedParts, streamStep, type Stem } from './stemMix'

// What plays a song's parts, whichever way it does it.
export interface StemPlayer { setGains(gains: Record<string, number>, volume: number): void; close(): void }

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

export class StemSet implements StemPlayer {
  private gains = new Map<string, GainNode>()
  private sources: AudioBufferSourceNode[] = []
  private out: GainNode
  private startAt = 0
  private startCtx = 0
  private running = false
  private timer = 0
  private closed = false
  private onEvent = () => this.sync()

  private constructor(private g: SoundGraph, private el: HTMLAudioElement, private buffers: Map<string, AudioBuffer>) {
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

  // Takes over from the song with its parts: decoded whole at normal speed, streamed for a
  // long song or another speed (StreamedStems). Throws when any part can't be had: the
  // caller keeps the normal track.
  static async open(el: HTMLAudioElement, stems: Stem[], signal: AbortSignal): Promise<StemPlayer> {
    if (!canRoute(el)) throw new Error('The parts can\'t be played here, as the server is on a different address from this page.')
    const g = await openGraph()
    if (!g) throw new Error('This browser wouldn\'t start its audio engine.')
    if (streamedParts(el.playbackRate, el.duration)) {
      if (!g.route(el)) throw new Error('This browser wouldn\'t start its audio engine.')
      return StreamedStems.start(g, el, stems, signal)
    }
    const list = await Promise.all(stems.map((s) => fetchSound(g.ctx, s.url, signal)))
    if (signal.aborted) throw new DOMException('gone', 'AbortError')
    if (!g.route(el)) throw new Error('This browser wouldn\'t start its audio engine.')
    return new StemSet(g, el, new Map(stems.map((s, i) => [s.id, list[i]])))
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
    // Decoded parts can't keep their key at another speed; the page streams them instead
    // (StemSet.open), and these stay quiet until that has taken over.
    if (el.playbackRate !== 1) return
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

// StreamedStems: each part played by an <audio> of its own, streamed from the server as the
// song is, so a song of any length and any speed can be sung to; the browser keeps each
// part's key at another speed as it does the song's. The song stays in charge, and each part
// is kept with it by playing a little faster or slower, or by a jump when it's far off.
class StreamedStems implements StemPlayer {
  private parts: { el: HTMLAudioElement; gain: GainNode; source: MediaElementAudioSourceNode }[] = []
  private byId = new Map<string, GainNode>()
  private out: GainNode
  private timer = 0
  private closed = false
  private onEvent = () => this.sync()

  private constructor(private g: SoundGraph, private song: HTMLAudioElement, stems: Stem[]) {
    const ctx = g.ctx
    this.out = ctx.createGain()
    this.out.connect(g.stemIn)
    for (const s of stems) {
      const el = new Audio()
      el.preload = 'auto'
      el.crossOrigin = song.crossOrigin
      el.preservesPitch = true
      el.src = mediaUrl(s.url)
      const source = ctx.createMediaElementSource(el)
      const gain = ctx.createGain()
      gain.gain.value = 0
      source.connect(gain)
      gain.connect(this.out)
      this.parts.push({ el, gain, source })
      this.byId.set(s.id, gain)
    }
    for (const ev of ['play', 'playing', 'pause', 'seeked', 'waiting', 'ratechange', 'ended', 'emptied']) song.addEventListener(ev, this.onEvent)
    this.timer = window.setInterval(() => this.keepUp(), 400)
  }

  // Waits until every part can play, then takes the sound over from the song.
  static async start(g: SoundGraph, song: HTMLAudioElement, stems: Stem[], signal: AbortSignal): Promise<StreamedStems> {
    const set = new StreamedStems(g, song, stems)
    try {
      await Promise.all(set.parts.map(({ el }) => new Promise<void>((ok, fail) => {
        if (el.readyState >= 3) { ok(); return }
        const done = () => { el.removeEventListener('canplay', done); el.removeEventListener('error', bad); ok() }
        const bad = () => { el.removeEventListener('canplay', done); el.removeEventListener('error', bad); fail(new Error('stem ' + (el.error?.code ?? 0))) }
        el.addEventListener('canplay', done)
        el.addEventListener('error', bad)
        signal.addEventListener('abort', () => fail(new DOMException('gone', 'AbortError')), { once: true })
      })))
    } catch (e) { set.close(); throw e }
    g.hushDecks(true)
    set.sync()
    return set
  }

  setGains(gains: Record<string, number>, volume: number) {
    const now = this.g.ctx.currentTime
    for (const [id, node] of this.byId) node.gain.setTargetAtTime(gains[id] ?? 0, now, 0.03)
    this.out.gain.setTargetAtTime(volume, now, 0.03)
  }

  // The song paused, moved or changed speed: every part does the same.
  private sync() {
    if (this.closed) return
    const song = this.song
    const going = !song.paused && !song.ended && song.readyState >= 3
    for (const { el } of this.parts) {
      el.playbackRate = song.playbackRate
      if (Math.abs(el.currentTime - song.currentTime) > 0.05) el.currentTime = song.currentTime
      if (going) { if (el.paused) void el.play().catch(() => {}) } else if (!el.paused) el.pause()
    }
  }

  private keepUp() {
    if (this.closed || this.song.paused || this.song.ended) return
    for (const { el } of this.parts) {
      if (el.paused || el.seeking) continue
      const step = streamStep(this.song.playbackRate, this.song.currentTime - el.currentTime)
      if (step.seek) el.currentTime = this.song.currentTime
      el.playbackRate = step.rate
    }
  }

  close() {
    if (this.closed) return
    this.closed = true
    window.clearInterval(this.timer)
    for (const ev of ['play', 'playing', 'pause', 'seeked', 'waiting', 'ratechange', 'ended', 'emptied']) this.song.removeEventListener(ev, this.onEvent)
    for (const { el, source, gain } of this.parts) { el.pause(); el.removeAttribute('src'); el.load(); source.disconnect(); gain.disconnect() }
    this.out.disconnect()
    this.g.hushDecks(false)
  }
}
