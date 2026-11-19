// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getToken, mediaUrl, serverBase } from '../api'
import { canRoute, openGraph, type SoundGraph } from './sound'
import { drifted, startPlan, streamedParts, streamStep, type Route, type Stem } from './stemMix'
import { FxChain } from './fxChain'
import { klog } from './klog'
import type { Fx } from './voiceFx'
import { t } from '../i18n'

// What plays a song's parts, whichever way it does it.
// setRoutes: how much of each part goes to the voice's chain and to the music's (stemMix.routesFor); setFx: the voice
// settings those chains apply (voiceFx.ts).
export interface StemPlayer { setRoutes(routes: Record<string, Route>, volume: number): void; setFx(fx: Fx): void; close(): void }

// A part's two sends, silent until the first setRoutes.
type Sends = { voice: GainNode; music: GainNode }
function sends(ctx: AudioContext, fx: FxChain): Sends & { input: GainNode } {
  const input = ctx.createGain()
  const voice = ctx.createGain(), music = ctx.createGain()
  voice.gain.value = 0
  music.gain.value = 0
  input.connect(voice).connect(fx.voice)
  input.connect(music).connect(fx.music)
  return { input, voice, music }
}
function route(ctx: AudioContext, all: Map<string, Sends>, routes: Record<string, Route>, out: GainNode, volume: number) {
  const now = ctx.currentTime
  for (const [id, s] of all) {
    const r = routes[id] || { voice: 0, music: 0 }
    s.voice.gain.setTargetAtTime(r.voice, now, 0.03)
    s.music.gain.setTargetAtTime(r.music, now, 0.03)
  }
  out.gain.setTargetAtTime(volume, now, 0.03)
}

// The parts' way out, through a limiter: the separated parts never add back up to exactly the song, and the voice
// settings and the Mix can each add more, so together they went over full scale and crackled. The limiter only
// touches peaks over -1 dB, quickly, and is otherwise not heard.
function limitedOut(ctx: AudioContext, dest: AudioNode): { out: GainNode; limiter: DynamicsCompressorNode } {
  const out = ctx.createGain()
  const limiter = ctx.createDynamicsCompressor()
  limiter.threshold.value = -1
  limiter.knee.value = 0
  limiter.ratio.value = 20
  limiter.attack.value = 0.002
  limiter.release.value = 0.12
  out.connect(limiter).connect(dest)
  return { out, limiter }
}

// A new chain in place of one that has stopped making sound (FxChain.broken), with the parts' sends moved over to it.
function freshChain(ctx: AudioContext, old: FxChain, all: Iterable<Sends>, out: GainNode, fx: Fx | null): FxChain {
  old.close()
  const next = new FxChain(ctx, out)
  for (const s of all) {
    try { s.voice.disconnect() } catch { /* not connected */ }
    try { s.music.disconnect() } catch { /* not connected */ }
    s.voice.connect(next.voice)
    s.music.connect(next.music)
  }
  if (fx) next.set(fx)
  return next
}

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
  private lastFx: Fx | null = null
  private gains = new Map<string, GainNode>()
  private sends = new Map<string, Sends>()
  private fx: FxChain
  private sources: AudioBufferSourceNode[] = []
  private out: GainNode
  private limiter: DynamicsCompressorNode
  private startAt = 0
  private startCtx = 0
  private running = false
  private timer = 0
  private closed = false
  private onEvent = (e: Event) => this.sync(e.type)

  private constructor(private g: SoundGraph, private el: HTMLAudioElement, private buffers: Map<string, AudioBuffer>) {
    const ctx = g.ctx
    const way = limitedOut(ctx, g.stemIn)
    this.out = way.out
    this.limiter = way.limiter
    this.fx = new FxChain(ctx, this.out)
    for (const id of buffers.keys()) {
      const s = sends(ctx, this.fx)
      this.gains.set(id, s.input)
      this.sends.set(id, s)
    }
    for (const ev of ['play', 'playing', 'pause', 'seeking', 'seeked', 'waiting', 'ratechange', 'ended', 'emptied']) el.addEventListener(ev, this.onEvent)
    this.timer = window.setInterval(() => this.watch(), 2000)
    g.hushDecks(true, this)
    klog('decoded set: open', [...buffers.keys()].join('+'))
    this.sync('open')
  }

  // Takes over from the song with its parts: decoded whole at normal speed, streamed for a
  // long song or another speed (StreamedStems). Throws when any part can't be had: the
  // caller keeps the normal track.
  static async open(el: HTMLAudioElement, stems: Stem[], signal: AbortSignal): Promise<StemPlayer> {
    if (!canRoute(el)) throw new Error(t('The parts can’t be played here, as the server is on a different address from this page.'))
    const g = await openGraph()
    if (!g) throw new Error(t('This browser wouldn’t start its audio engine.'))
    if (streamedParts(el.playbackRate, el.duration)) {
      if (!g.route(el)) throw new Error(t('This browser wouldn’t start its audio engine.'))
      return StreamedStems.start(g, el, stems, signal)
    }
    const list = await Promise.all(stems.map((s) => fetchSound(g.ctx, s.url, signal)))
    if (signal.aborted) throw new DOMException('gone', 'AbortError')
    if (!g.route(el)) throw new Error(t('This browser wouldn’t start its audio engine.'))
    return new StemSet(g, el, new Map(stems.map((s, i) => [s.id, list[i]])))
  }

  setRoutes(routes: Record<string, Route>, volume: number) { route(this.g.ctx, this.sends, routes, this.out, volume) }

  setFx(fx: Fx) { this.lastFx = fx; this.fx.set(fx) }

  private stopSources() {
    for (const s of this.sources) { try { s.stop() } catch { /* never started */ } s.disconnect() }
    this.sources = []
    this.running = false
  }

  // Starts every part again from where the song is, or stops them if it isn't playing.
  private sync(why = 'event') {
    if (this.closed) return
    const was = this.running
    this.stopSources()
    const el = this.el
    // Decoded parts can't keep their key at another speed; the page streams them instead
    // (StemSet.open), and these stay quiet until that has taken over.
    if (el.playbackRate !== 1) { if (was) klog('decoded set: stopped, speed', el.playbackRate); return }
    if (el.paused || el.ended || el.seeking || el.readyState < 3) {
      if (was) klog('decoded set: stopped,', why, el.paused ? 'paused' : el.ended ? 'ended' : el.seeking ? 'seeking' : 'readyState ' + el.readyState, 'at', el.currentTime.toFixed(2))
      return
    }
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
    klog('decoded set: started,', why, 'at', plan.offset.toFixed(2), was ? '(restart)' : '', 'engine', ctx.state)
  }

  private watch() {
    if (this.closed) return
    if (this.fx.broken()) { klog('decoded set: voice chain broken, made again'); this.fx = freshChain(this.g.ctx, this.fx, this.sends.values(), this.out, this.lastFx) }
    const el = this.el
    const playing = !el.paused && !el.ended && !el.seeking && el.readyState >= 3
    if (playing !== this.running) this.sync('watch')
    else if (playing && drifted(this.startAt, this.startCtx, this.g.ctx.currentTime, el.currentTime)) this.sync('drift')
  }

  // Gives the sound back to the song.
  close() {
    if (this.closed) return
    this.closed = true
    window.clearInterval(this.timer)
    for (const ev of ['play', 'playing', 'pause', 'seeking', 'seeked', 'waiting', 'ratechange', 'ended', 'emptied']) this.el.removeEventListener(ev, this.onEvent)
    klog('decoded set: close')
    this.stopSources()
    this.fx.close()
    this.out.disconnect()
    this.limiter.disconnect()
    this.g.hushDecks(false, this)
  }
}

// StreamedStems: each part played by an <audio> of its own, streamed from the server as the
// song is, so a song of any length and any speed can be sung to; the browser keeps each
// part's key at another speed as it does the song's. The song stays in charge, and each part
// is kept with it by playing a little faster or slower, or by a jump when it's far off.
class StreamedStems implements StemPlayer {
  private lastFx: Fx | null = null
  private checked = 0
  private parts: { el: HTMLAudioElement; gain: GainNode; source: MediaElementAudioSourceNode }[] = []
  private byId = new Map<string, Sends>()
  private out: GainNode
  private limiter: DynamicsCompressorNode
  private fx: FxChain
  private timer = 0
  private closed = false
  private onEvent = () => this.sync()

  private constructor(private g: SoundGraph, private song: HTMLAudioElement, stems: Stem[]) {
    const ctx = g.ctx
    const way = limitedOut(ctx, g.stemIn)
    this.out = way.out
    this.limiter = way.limiter
    this.fx = new FxChain(ctx, this.out)
    for (const s of stems) {
      const el = new Audio()
      el.preload = 'auto'
      el.crossOrigin = song.crossOrigin
      el.preservesPitch = true
      el.src = mediaUrl(s.url)
      const source = ctx.createMediaElementSource(el)
      const sent = sends(ctx, this.fx)
      const gain = sent.input
      source.connect(gain)
      this.parts.push({ el, gain, source })
      this.byId.set(s.id, sent)
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
    g.hushDecks(true, set)
    klog('streamed set: open', stems.map((s) => s.id).join('+'))
    set.sync()
    return set
  }

  setRoutes(routes: Record<string, Route>, volume: number) { route(this.g.ctx, this.byId, routes, this.out, volume) }

  setFx(fx: Fx) { this.lastFx = fx; this.fx.set(fx) }

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
    if (this.closed) return
    if (++this.checked % 5 === 0 && this.fx.broken()) { klog('streamed set: voice chain broken, made again'); this.fx = freshChain(this.g.ctx, this.fx, this.byId.values(), this.out, this.lastFx) }
    if (this.song.paused || this.song.ended) return
    for (const { el } of this.parts) {
      if (el.paused || el.seeking) continue
      const step = streamStep(this.song.playbackRate, this.song.currentTime - el.currentTime)
      if (step.seek) { klog('streamed set: part jumped', (this.song.currentTime - el.currentTime).toFixed(2), 's'); el.currentTime = this.song.currentTime }
      el.playbackRate = step.rate
    }
  }

  close() {
    if (this.closed) return
    this.closed = true
    window.clearInterval(this.timer)
    for (const ev of ['play', 'playing', 'pause', 'seeked', 'waiting', 'ratechange', 'ended', 'emptied']) this.song.removeEventListener(ev, this.onEvent)
    klog('streamed set: close')
    for (const { el, source, gain } of this.parts) { el.pause(); el.removeAttribute('src'); el.load(); source.disconnect(); gain.disconnect() }
    this.fx.close()
    this.out.disconnect()
    this.limiter.disconnect()
    this.g.hushDecks(false, this)
  }
}
