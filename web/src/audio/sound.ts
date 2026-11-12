// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What happens to the sound between the player and the speakers: equaliser, loudness,
// vocal reduction and key. It's all optional and nothing here runs until someone turns one
// on: an <audio> element plays straight to the speakers until then, which is both the
// cheapest way and the safest.
//
// Once an element is routed through Web Audio it can't leave, so that's only done with a
// reason, only once the context is running, and never where it would break things: a
// stream on another address would come out silent, and iPhones and iPads stop Web Audio
// when the screen locks (the panel says so before anyone turns these on there).

export const EQ_FREQS = [31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000]

export type EqPreset = { id: string; name: string; gains: number[] }
export const EQ_PRESETS: EqPreset[] = [
  { id: 'flat', name: 'Flat', gains: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0] },
  { id: 'bass', name: 'More bass', gains: [6, 5, 3, 1, 0, 0, 0, 0, 0, 0] },
  { id: 'treble', name: 'More treble', gains: [0, 0, 0, 0, 0, 0, 1, 3, 5, 6] },
  { id: 'vocal', name: 'Clear voices', gains: [-2, -2, -1, 0, 2, 4, 4, 2, 0, -1] },
  { id: 'rock', name: 'Rock', gains: [4, 3, 1, -1, -1, 0, 2, 3, 4, 4] },
  { id: 'pop', name: 'Pop', gains: [-1, 0, 2, 3, 3, 1, 0, 0, 1, 1] },
  { id: 'classical', name: 'Classical', gains: [3, 2, 1, 0, 0, 0, 0, 1, 2, 3] },
  { id: 'jazz', name: 'Jazz', gains: [3, 2, 1, 1, -1, -1, 0, 1, 2, 3] },
  { id: 'night', name: 'Quiet room', gains: [-4, -3, -1, 0, 1, 2, 1, 0, -2, -3] },
  { id: 'speech', name: 'Speech', gains: [-6, -4, -2, 1, 3, 4, 3, 2, 0, -3] },
]

export type SoundSettings = {
  volume: number
  muted: boolean
  eqOn: boolean
  eqPreset: string
  eqGains: number[]
  preamp: number // dB
  replayGain: 'off' | 'track' | 'album'
  gapless: boolean
  crossfade: number // seconds, 0 for none
  crossfadeAlbums: boolean // also between songs of one album, which are often meant to run together
  vocalCut: number // 0 to 1
  keyShift: number // semitones
}

export const DEFAULTS: SoundSettings = { volume: 1, muted: false, eqOn: false, eqPreset: 'flat', eqGains: EQ_PRESETS[0].gains, preamp: 0, replayGain: 'off', gapless: true, crossfade: 0, crossfadeAlbums: false, vocalCut: 0, keyShift: 0 }
const KEY = 'wms.sound'

const num = (v: unknown, lo: number, hi: number, d: number) => (typeof v === 'number' && Number.isFinite(v) ? Math.min(hi, Math.max(lo, v)) : d)

// These belong to a device (speakers differ), so they're kept on it. Vocal reduction and
// key are for one sitting: nobody wants the next day's music in another key by surprise.
export function loadSound(): SoundSettings {
  let s: Partial<SoundSettings> = {}
  try { s = JSON.parse(localStorage.getItem(KEY) || '{}') } catch { /* private window */ }
  return {
    ...DEFAULTS,
    volume: num(s.volume, 0, 1, 1), muted: s.muted === true, eqOn: s.eqOn === true, eqPreset: typeof s.eqPreset === 'string' ? s.eqPreset : 'flat',
    eqGains: Array.isArray(s.eqGains) && s.eqGains.length === EQ_FREQS.length ? s.eqGains.map((g) => num(g, -12, 12, 0)) : DEFAULTS.eqGains,
    preamp: num(s.preamp, -12, 12, 0), replayGain: s.replayGain === 'track' || s.replayGain === 'album' ? s.replayGain : 'off',
    gapless: s.gapless !== false, crossfade: num(s.crossfade, 0, 12, 0), crossfadeAlbums: s.crossfadeAlbums === true,
  }
}

export function saveSound(s: SoundSettings) {
  const { vocalCut: _v, keyShift: _k, ...kept } = s
  try { localStorage.setItem(KEY, JSON.stringify(kept)) } catch { /* private window */ }
}

// The sound can only be read from an address that's this page's own.
export function canRoute(el: HTMLMediaElement | null) {
  if (!el || typeof AudioContext === 'undefined') return false
  if (el.crossOrigin !== null) return true
  const src = el.currentSrc || el.src
  if (!src) return true
  try { return new URL(src, location.href).origin === location.origin } catch { return false }
}

export const isAppleMobile = () => /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)

type Deck = { source: MediaElementAudioSourceNode; gain: GainNode }

export class SoundGraph {
  ctx: AudioContext
  analyser: AnalyserNode
  private input: GainNode
  // Everything the player's own elements send goes through here, so karaoke's separate
  // parts can take their place: the song keeps playing, unheard, as their clock.
  private deckBus: GainNode
  private decks = new WeakMap<HTMLMediaElement, Deck>()
  private filters: BiquadFilterNode[]
  private preamp: GainNode
  private loudness: GainNode
  private master: GainNode
  private vocalDry: GainNode
  private vocalWet: GainNode
  private vocalIn: GainNode
  private pitch: AudioWorkletNode | null = null
  private pitchLoaded: Promise<void> | null = null
  private eqOn = false
  private sideLevel: AnalyserNode
  private midLevel: AnalyserNode
  private outputTap: MediaStreamAudioDestinationNode | null = null

  constructor() {
    const ctx = new AudioContext({ latencyHint: 'playback' })
    this.ctx = ctx
    this.input = ctx.createGain()
    this.deckBus = ctx.createGain()
    this.deckBus.connect(this.input)
    this.vocalIn = ctx.createGain()
    this.vocalDry = ctx.createGain()
    this.vocalWet = ctx.createGain()
    this.vocalWet.gain.value = 0
    this.preamp = ctx.createGain()
    this.loudness = ctx.createGain()
    this.master = ctx.createGain()
    this.analyser = ctx.createAnalyser()
    this.analyser.fftSize = 2048
    this.analyser.smoothingTimeConstant = 0.6
    this.filters = EQ_FREQS.map((f, i) => {
      const b = ctx.createBiquadFilter()
      b.type = i === 0 ? 'lowshelf' : i === EQ_FREQS.length - 1 ? 'highshelf' : 'peaking'
      b.frequency.value = f
      b.Q.value = 1.1
      return b
    })
    for (let i = 0; i + 1 < this.filters.length; i++) this.filters[i].connect(this.filters[i + 1])

    // Vocal reduction. A singer is usually mixed dead centre, the same in both speakers, so
    // left minus right removes them, and everything else that's centred: mostly bass and
    // kick, which are put back from below 150 Hz, with some air from above 9 kHz.
    const split = ctx.createChannelSplitter(2)
    const left = ctx.createGain()
    left.gain.value = 0.7 // what's left is often louder than the mix it came from
    const right = ctx.createGain()
    right.gain.value = -0.7
    const side = ctx.createGain()
    const sideHigh = ctx.createBiquadFilter()
    sideHigh.type = 'highpass'
    sideHigh.frequency.value = 150
    const merge = ctx.createChannelMerger(2)
    const bass = ctx.createBiquadFilter()
    bass.type = 'lowpass'
    bass.frequency.value = 150
    const air = ctx.createBiquadFilter()
    air.type = 'highpass'
    air.frequency.value = 9000
    const airLevel = ctx.createGain()
    airLevel.gain.value = 0.6
    this.input.connect(this.vocalIn)
    this.vocalIn.connect(this.vocalDry)
    this.vocalIn.connect(split)
    split.connect(left, 0)
    split.connect(right, 1)
    left.connect(side)
    right.connect(side)
    side.connect(sideHigh)
    sideHigh.connect(merge, 0, 0)
    sideHigh.connect(merge, 0, 1)
    merge.connect(this.vocalWet)
    this.vocalIn.connect(bass)
    bass.connect(this.vocalWet)
    this.vocalIn.connect(air)
    air.connect(airLevel)
    airLevel.connect(this.vocalWet)
    // How much of the sound isn't centred, to say so when there's nothing to separate.
    this.sideLevel = ctx.createAnalyser()
    this.midLevel = ctx.createAnalyser()
    this.sideLevel.fftSize = this.midLevel.fftSize = 1024
    side.connect(this.sideLevel)
    this.vocalIn.connect(this.midLevel)

    this.vocalDry.connect(this.preamp)
    this.vocalWet.connect(this.preamp)
    this.preamp.connect(this.loudness)
    this.loudness.connect(this.analyser)
    this.loudness.connect(this.master)
    this.master.connect(ctx.destination)
    // If the system stops the context (a phone call), the music would stay stopped.
    ctx.addEventListener('statechange', () => { if (ctx.state === 'suspended') void ctx.resume().catch(() => {}) })
  }

  async resume() { await this.ctx.resume().catch(() => {}) }

  routed(el: HTMLMediaElement | null) { return !!el && this.decks.has(el) }

  // Takes an element's sound. Only called once the context is running.
  route(el: HTMLMediaElement) {
    if (this.decks.has(el) || this.ctx.state !== 'running' || !canRoute(el)) return this.decks.has(el)
    const source = this.ctx.createMediaElementSource(el)
    const gain = this.ctx.createGain()
    source.connect(gain)
    gain.connect(this.deckBus)
    this.decks.set(el, { source, gain })
    return true
  }

  // Where karaoke's parts come in: before the key change and the equaliser, as a song does.
  get stemIn(): AudioNode { return this.input }
  // Silences, or gives back, the song's own sound while its parts play instead.
  hushDecks(on: boolean) { this.deckBus.gain.setTargetAtTime(on ? 0 : 1, this.ctx.currentTime, 0.02) }

  setDeckLevel(el: HTMLMediaElement, level: number) {
    const d = this.decks.get(el)
    if (d) d.gain.gain.setTargetAtTime(level, this.ctx.currentTime, 0.015)
  }

  setEq(on: boolean, gains: number[], preampDb: number) {
    if (on !== this.eqOn) {
      this.eqOn = on
      this.vocalDry.disconnect()
      this.vocalWet.disconnect()
      const into = on ? this.filters[0] : this.preamp
      this.vocalDry.connect(into)
      this.vocalWet.connect(into)
      const last = this.filters[this.filters.length - 1]
      try { last.disconnect() } catch { /* wasn't connected */ }
      if (on) last.connect(this.preamp)
    }
    gains.forEach((g, i) => this.filters[i].gain.setTargetAtTime(g, this.ctx.currentTime, 0.02))
    this.preamp.gain.setTargetAtTime(on ? Math.pow(10, preampDb / 20) : 1, this.ctx.currentTime, 0.02)
  }

  setLoudness(db: number) {
    this.loudness.gain.setTargetAtTime(Math.pow(10, db / 20), this.ctx.currentTime, 0.05)
  }

  setVocalCut(amount: number) {
    this.vocalWet.gain.setTargetAtTime(amount, this.ctx.currentTime, 0.05)
    this.vocalDry.gain.setTargetAtTime(1 - amount, this.ctx.currentTime, 0.05)
  }

  // Whether this recording has anything off-centre to work with. Close to nothing means
  // it's mono, and cutting the centre would leave almost silence.
  nearlyMono(): boolean | null {
    const a = new Float32Array(this.sideLevel.fftSize), b = new Float32Array(this.midLevel.fftSize)
    this.sideLevel.getFloatTimeDomainData(a)
    this.midLevel.getFloatTimeDomainData(b)
    let s = 0, m = 0
    for (let i = 0; i < a.length; i++) { s += a[i] * a[i]; m += b[i] * b[i] }
    if (m < 1e-5) return null // silence: can't tell
    return s / m < 0.01
  }

  // A copy of what's going to the speakers, before the volume, for the visualizer on
  // browsers where it can't take its own from the element.
  outputCopy(): MediaStream {
    this.outputTap ??= this.ctx.createMediaStreamDestination()
    this.loudness.connect(this.outputTap)
    return this.outputTap.stream.clone()
  }

  // Key: a worklet, loaded the first time. False where the browser has none, or it won't load.
  async setKey(semitones: number): Promise<boolean> {
    if (semitones === 0 && !this.pitch) return true
    if (typeof AudioWorkletNode === 'undefined' || !this.ctx.audioWorklet) return false
    try {
      // A file of its own beside the page, as a small one bundled would become a data: address,
      // which the page's security policy rightly refuses.
      this.pitchLoaded ??= this.ctx.audioWorklet.addModule(new URL('/pitch-worklet.js', location.href).href)
      await this.pitchLoaded
    } catch { this.pitchLoaded = null; return false }
    if (!this.pitch) {
      this.pitch = new AudioWorkletNode(this.ctx, 'wms-pitch', { outputChannelCount: [2] })
      this.input.disconnect(this.vocalIn)
      this.input.connect(this.pitch)
      this.pitch.connect(this.vocalIn)
    }
    this.pitch.parameters.get('ratio')?.setValueAtTime(Math.pow(2, semitones / 12), this.ctx.currentTime)
    return true
  }
}

let graph: SoundGraph | null = null
export const soundGraph = () => graph

// Makes the graph, from a press where the browser wants one for sound to start.
export async function openGraph(): Promise<SoundGraph | null> {
  if (typeof AudioContext === 'undefined') return null
  graph ??= new SoundGraph()
  await graph.resume()
  return graph.ctx.state === 'running' ? graph : null
}

// ---------------------------------------------------------------- listening

// The sound to draw from, for the player's own styles. The element isn't touched where
// the browser can give a copy of the sound (Chrome, Edge, Android): it stays out of Web
// Audio, which is what keeps it playing on a locked phone. If the graph is already in use
// it's read from there. Otherwise, as with the visualizer plugin, it goes through the graph
// on browsers with no copy to take, but not on iPhones, which get a pattern instead.

type Captureable = HTMLMediaElement & { captureStream?: () => MediaStream }

export class SoundListener {
  private el: () => HTMLMediaElement | null
  private ctx: AudioContext | null = null
  private analyser: AnalyserNode | null = null
  private source: MediaStreamAudioSourceNode | null = null
  private stream: MediaStream | null = null
  private watched: HTMLMediaElement | null = null
  private trackId = ''
  private frames = 0
  live = false

  constructor(el: () => HTMLMediaElement | null) { this.el = el }

  private connect() {
    const el = this.el() as Captureable | null
    const g = soundGraph()
    if (el && g?.routed(el)) { this.analyser = g.analyser; this.live = true; this.dropCopy(); return }
    if (!el || !canRoute(el)) { this.live = false; return }
    if (typeof el.captureStream === 'function') {
      if (el !== this.watched) { this.dropCopy(); this.watched = el; this.stream = el.captureStream() }
      const t = this.stream?.getAudioTracks()[0]
      if (t && t.id !== this.trackId) {
        this.trackId = t.id
        this.source?.disconnect()
        this.ctx ??= new AudioContext({ latencyHint: 'playback' })
        if (!this.analyser || this.analyser.context !== this.ctx) {
          this.analyser = this.ctx.createAnalyser()
          this.analyser.fftSize = 2048
          this.analyser.smoothingTimeConstant = 0.6
        }
        this.source = this.ctx.createMediaStreamSource(new MediaStream([t]))
        this.source.connect(this.analyser)
      }
      if (this.ctx?.state === 'suspended') void this.ctx.resume().catch(() => {})
      this.live = !!t && this.ctx?.state === 'running'
      return
    }
    if (isAppleMobile()) { this.live = false; return }
    void openGraph().then((gr) => { const e = this.el(); if (gr && e) gr.route(e) })
    this.live = false
  }

  private dropCopy() {
    this.source?.disconnect()
    this.source = null
    this.stream?.getTracks().forEach((t) => t.stop())
    this.stream = null
    this.watched = null
    this.trackId = ''
  }

  // Fills freq with this moment's levels (0 to 255). False means nothing to read.
  read(freq: Uint8Array): boolean {
    if (this.frames++ % 15 === 0) this.connect()
    if (!this.analyser || !this.live) return false
    this.analyser.getByteFrequencyData(freq as Uint8Array<ArrayBuffer>)
    return true
  }

  close() {
    this.dropCopy()
    void this.ctx?.close().catch(() => {})
    this.ctx = null
    this.analyser = null
    this.live = false
  }
}
