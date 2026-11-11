// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getToken, mediaUrl, serverBase } from '../api'
import type { SoundGraph } from './sound'
import { STEM_NAMES, carryPosition, enginePosition, multitrackPlan, startPlan, tracksEnabled, type Levels, type StemInfo, type StemName, type TrackPlan } from './stems'

// The sound of karaoke's stems. The rules are in stems.ts; this plays what they say.
//
// Several <audio> elements can't be kept together: each has its own clock, and they slide
// apart by tens of milliseconds, which is heard as an echo. So the parts are fetched whole,
// decoded, and played from memory on one clock, every one started at the same instant from
// the same sample. They can't drift, because there is nothing to drift against. The price
// is memory, which stems.ts budgets.

export type StemPlayer = {
  readonly duration: number
  readonly playing: boolean
  readonly canLevel: boolean // false: a part is on or off, nothing in between
  readonly names: StemName[]
  play(): void
  pause(): void
  seek(t: number): void
  position(): number
  setLevels(levels: Levels, gains: Partial<Record<StemName, number>>): void
  setVolume(v: number): void
  onEnded: (() => void) | null
  onState: ((playing: boolean) => void) | null
  destroy(): void
}

// A failure with words for the karaoke screen.
export class StemError extends Error {}

const authHeaders = (): Record<string, string> => { const t = getToken(); return t ? { Authorization: 'Bearer ' + t } : {} }

// Fetches one part, saying how many bytes have come.
async function fetchPart(url: string, signal: AbortSignal, got: (bytes: number) => void): Promise<ArrayBuffer> {
  const res = await fetch(serverBase() + url, { headers: authHeaders(), credentials: serverBase() ? 'omit' : 'same-origin', signal })
  if (!res.ok) throw new StemError(res.status === 404 ? 'One of the stems is no longer on the server.' : `The server wouldn't send a stem (${res.status}).`)
  if (!res.body) { const all = await res.arrayBuffer(); got(all.byteLength); return all }
  const reader = res.body.getReader()
  const chunks: Uint8Array[] = []
  let size = 0
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    chunks.push(value)
    size += value.length
    got(value.length)
  }
  const out = new Uint8Array(size)
  let at = 0
  for (const c of chunks) { out.set(c, at); at += c.length }
  return out.buffer
}

// Whether the context's clock is moving, waiting up to five seconds for it to start.
async function clockRuns(ctx: AudioContext, signal: AbortSignal): Promise<boolean> {
  const from = ctx.currentTime
  for (let i = 0; i < 100 && !signal.aborted; i++) {
    if (ctx.state !== 'running') void ctx.resume().catch(() => {})
    if (ctx.currentTime > from) return true
    await new Promise((r) => setTimeout(r, 50))
  }
  return ctx.currentTime > from
}

// ---------------------------------------------------------------- parts played from memory

export class BufferStemPlayer implements StemPlayer {
  private graph: SoundGraph
  private out: GainNode
  private gains = new Map<StemName, GainNode>()
  private buffers = new Map<StemName, AudioBuffer>()
  private nodes: AudioBufferSourceNode[] = []
  private st = { offset: 0, startedAt: 0, playing: false }
  private run = 0 // which start the playing nodes belong to: an old one's ending means nothing
  duration = 0
  readonly canLevel = true
  onEnded: (() => void) | null = null
  onState: ((playing: boolean) => void) | null = null

  constructor(graph: SoundGraph) {
    this.graph = graph
    this.out = graph.bus()
  }

  get playing() { return this.st.playing }
  get names() { return STEM_NAMES.filter((n) => this.buffers.has(n)) }

  // Fetches and decodes the named parts, two at a time: a phone has neither the
  // connections nor the memory for five at once. progress goes from 0 to 1.
  async load(stems: StemInfo[], names: StemName[], signal: AbortSignal, progress: (f: number) => void) {
    const want = stems.filter((s) => names.includes(s.name as StemName) && !this.buffers.has(s.name as StemName))
    const total = want.reduce((n, s) => n + Math.max(1, s.size), 0) || 1
    let got = 0, decoded = 0
    // Fetching is most of the wait on a slow connection, decoding on a slow device: each
    // gets a share of the bar.
    const tell = () => progress(Math.min(1, 0.7 * Math.min(1, got / total) + 0.3 * (decoded / Math.max(1, want.length))))
    const queue = [...want]
    const work = async () => {
      for (;;) {
        const s = queue.shift()
        if (!s) return
        const data = await fetchPart(s.url, signal, (b) => { got += b; tell() })
        let buf: AudioBuffer
        try { buf = await this.graph.ctx.decodeAudioData(data) } catch { throw new StemError(`This browser can't play this song's stems (${s.mime.replace('audio/', '').toUpperCase()}).`) }
        if (signal.aborted) return
        this.buffers.set(s.name as StemName, buf)
        const g = this.graph.ctx.createGain()
        g.gain.value = 0
        g.connect(this.out)
        this.gains.set(s.name as StemName, g)
        decoded++
        tell()
      }
    }
    await Promise.all([work(), work()])
    if (signal.aborted) throw new DOMException('given up', 'AbortError')
    // A sound engine that has only just been started can say it is running a moment before
    // its clock moves (the sound card is still being opened). The song keeps playing as it
    // is until the clock does move: taking over before then would be a silence.
    if (!(await clockRuns(this.graph.ctx, signal))) throw new StemError('This browser\'s audio engine didn\'t start, so the stems can\'t be played together here.')
    // The song is as long as its longest part: a voice part often stops before the music does.
    this.duration = Math.max(0, ...[...this.buffers.values()].map((b) => b.duration))
  }

  private stopNodes() {
    this.run++
    for (const n of this.nodes) { n.onended = null; try { n.stop() } catch { /* never started */ } n.disconnect() }
    this.nodes = []
  }

  play() {
    if (this.st.playing || !this.buffers.size) return
    const ctx = this.graph.ctx
    if (ctx.state !== 'running') void ctx.resume().catch(() => {})
    if (this.st.offset >= this.duration - 0.06) this.st.offset = 0 // at the end: once more from the top
    const plan = startPlan(ctx.currentTime, this.st.offset, this.duration)
    const run = ++this.run
    let left = 0
    for (const [name, buf] of this.buffers) {
      if (plan.offset >= buf.duration) continue // this part has already finished by here
      const n = ctx.createBufferSource()
      n.buffer = buf
      n.connect(this.gains.get(name)!)
      left++
      n.onended = () => {
        if (run !== this.run || --left > 0) return
        // The last part to end is the end of the song.
        this.nodes = []
        this.st = { offset: this.duration, startedAt: 0, playing: false }
        this.onState?.(false)
        this.onEnded?.()
      }
      // The same instant and the same place for every part.
      n.start(plan.when, plan.offset)
      this.nodes.push(n)
    }
    this.st = { offset: plan.offset, startedAt: plan.when, playing: true }
    this.onState?.(true)
  }

  pause() {
    if (!this.st.playing) return
    const at = this.position()
    this.stopNodes()
    this.st = { offset: at, startedAt: 0, playing: false }
    this.onState?.(false)
  }

  seek(t: number) {
    const was = this.st.playing
    if (was) this.stopNodes()
    this.st = { offset: carryPosition(t, this.duration), startedAt: 0, playing: false }
    if (was) this.play()
  }

  position() {
    const ctx = this.graph.ctx
    return enginePosition(this.st, ctx.currentTime, ctx.outputLatency ?? ctx.baseLatency ?? 0, this.duration)
  }

  setLevels(_levels: Levels, gains: Partial<Record<StemName, number>>) {
    const now = this.graph.ctx.currentTime
    for (const [name, g] of this.gains) g.gain.setTargetAtTime(gains[name] ?? 0, now, 0.015)
  }

  setVolume(v: number) { this.out.gain.setTargetAtTime(v, this.graph.ctx.currentTime, 0.015) }

  destroy() {
    this.onEnded = this.onState = null
    this.stopNodes()
    this.st.playing = false
    for (const g of this.gains.values()) g.disconnect()
    this.out.disconnect()
    this.gains.clear()
    this.buffers.clear()
  }
}

// ---------------------------------------------------------------- one file, many tracks

// A Native Instruments .stem.mp4. Browsers decode only a file's first sound track for
// memory playback, so its parts can't be mixed the way separate files are. Where the
// browser lets a page switch a file's sound tracks on and off (Safari does; Chrome and
// Firefox don't, outside an experiment), one element plays the file and the tracks are
// switched: they share the element's clock, so they stay together. A part is then on or
// off, with nothing in between, and the key can't be moved.

type AudioTrackish = { enabled: boolean; label: string }
type WithTracks = HTMLAudioElement & { audioTracks?: { length: number; [i: number]: AudioTrackish } }

export const canSwitchTracks = () => typeof HTMLMediaElement !== 'undefined' && 'audioTracks' in HTMLMediaElement.prototype

export class TrackStemPlayer implements StemPlayer {
  private el: WithTracks
  private plan: NonNullable<TrackPlan> = { mix: 0, parts: {} }
  readonly canLevel = false
  onEnded: (() => void) | null = null
  onState: ((playing: boolean) => void) | null = null

  constructor() {
    this.el = new Audio() as WithTracks
    this.el.preload = 'auto'
    this.el.addEventListener('play', () => this.onState?.(true))
    this.el.addEventListener('pause', () => this.onState?.(false))
    this.el.addEventListener('ended', () => { this.onState?.(false); this.onEnded?.() })
  }

  get duration() { return Number.isFinite(this.el.duration) ? this.el.duration : 0 }
  get playing() { return !this.el.paused && !this.el.ended }
  get names() { return STEM_NAMES.filter((n) => this.plan.parts[n] !== undefined) }

  load(stem: StemInfo, signal: AbortSignal): Promise<void> {
    return new Promise((resolve, reject) => {
      const el = this.el
      const fail = (words: string) => { cleanup(); reject(new StemError(words)) }
      const ready = () => {
        const tracks = el.audioTracks
        const labels: string[] = []
        for (let i = 0; tracks && i < tracks.length; i++) labels.push(tracks[i].label || '')
        const plan = multitrackPlan(labels)
        if (!plan) { fail('This browser sees only one sound track in the stem file, so its parts can\'t be played separately here.'); return }
        this.plan = plan
        cleanup()
        resolve()
      }
      const onError = () => fail('This browser can\'t play the stem file.')
      const onAbort = () => { cleanup(); el.removeAttribute('src'); el.load(); reject(new DOMException('given up', 'AbortError')) }
      const cleanup = () => { el.removeEventListener('loadedmetadata', ready); el.removeEventListener('error', onError); signal.removeEventListener('abort', onAbort) }
      el.addEventListener('loadedmetadata', ready)
      el.addEventListener('error', onError)
      signal.addEventListener('abort', onAbort)
      el.src = mediaUrl(stem.url)
    })
  }

  play() { void this.el.play().catch(() => this.onState?.(false)) }
  pause() { this.el.pause() }
  seek(t: number) { this.el.currentTime = carryPosition(t, this.duration) }
  position() { return this.el.currentTime }

  setLevels(levels: Levels) {
    const tracks = this.el.audioTracks
    if (!tracks) return
    const on = tracksEnabled(this.plan, levels, tracks.length)
    for (let i = 0; i < tracks.length; i++) if (tracks[i].enabled !== on[i]) tracks[i].enabled = on[i]
  }

  setVolume(v: number) { this.el.volume = Math.min(1, Math.max(0, v)) }

  destroy() {
    this.onEnded = this.onState = null
    this.el.pause()
    this.el.removeAttribute('src')
    this.el.load()
  }
}
