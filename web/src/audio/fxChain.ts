// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { fxNodes, isOff, type Fx } from './voiceFx'

// The voice settings as audio nodes. A song in separate parts sends its voice into `voice` and
// everything else into `music`; both come out of `out`. Nothing here runs unless there are
// parts, and the reverb's room is only made when someone turns reverb up, so a small machine
// that never touches these does no more work than before.
//
//   voice → low cut → warmth → presence → air → compressor → pan ─┬─────────────→ out
//                                                                 ├→ doubling ──→ out
//                                                                 ├→ echo ⟲ ───→ out
//                                                                 └→ reverb ────→ out
//   music → bass → treble → room for the voice ───────────────────────────────→ out

const TC = 0.04 // how quickly a change is heard, so sliders don't click

export class FxChain {
  readonly voice: GainNode
  readonly music: GainNode
  private lowCut: BiquadFilterNode
  private bass: BiquadFilterNode
  private presence: BiquadFilterNode
  private air: BiquadFilterNode
  private comp: DynamicsCompressorNode
  private makeUp: GainNode
  private pan: StereoPannerNode
  private doubleDelay: DelayNode
  private doubleLevel: GainNode
  private echoDelay: DelayNode
  private echoBack: GainNode
  private echoLevel: GainNode
  private reverb: ConvolverNode | null = null
  private reverbLevel: GainNode
  private roomMade = 0
  private mBass: BiquadFilterNode
  private mTreble: BiquadFilterNode
  private pocket: BiquadFilterNode

  constructor(private ctx: AudioContext, out: AudioNode) {
    const f = (type: BiquadFilterType, freq: number, q = 0.8) => { const b = ctx.createBiquadFilter(); b.type = type; b.frequency.value = freq; b.Q.value = q; return b }
    this.voice = ctx.createGain()
    this.lowCut = f('highpass', 20, 0.7)
    this.bass = f('lowshelf', 200)
    this.presence = f('peaking', 3000, 0.9)
    this.air = f('highshelf', 10000)
    this.comp = ctx.createDynamicsCompressor()
    this.comp.attack.value = 0.005
    this.comp.release.value = 0.15
    this.comp.knee.value = 8
    this.makeUp = ctx.createGain()
    this.pan = ctx.createStereoPanner()
    this.voice.connect(this.lowCut).connect(this.bass).connect(this.presence).connect(this.air).connect(this.comp).connect(this.makeUp).connect(this.pan).connect(out)

    // Doubling: the same voice 28 ms later, a touch to the other side.
    this.doubleDelay = ctx.createDelay(0.1)
    this.doubleDelay.delayTime.value = 0.028
    this.doubleLevel = ctx.createGain()
    this.doubleLevel.gain.value = 0
    this.pan.connect(this.doubleDelay).connect(this.doubleLevel).connect(out)

    // Echo: a delay that feeds back into itself, a little darker each time round.
    this.echoDelay = ctx.createDelay(1.5)
    this.echoBack = ctx.createGain()
    const dark = f('lowpass', 4500, 0.5)
    this.echoLevel = ctx.createGain()
    this.echoLevel.gain.value = 0
    this.pan.connect(this.echoDelay)
    this.echoDelay.connect(dark).connect(this.echoBack).connect(this.echoDelay)
    this.echoDelay.connect(this.echoLevel).connect(out)

    this.reverbLevel = ctx.createGain()
    this.reverbLevel.gain.value = 0
    this.reverbLevel.connect(out)

    this.music = ctx.createGain()
    this.mBass = f('lowshelf', 120)
    this.mTreble = f('highshelf', 6000)
    this.pocket = f('peaking', 1800, 0.7)
    this.music.connect(this.mBass).connect(this.mTreble).connect(this.pocket).connect(out)
  }

  set(fx: Fx) {
    const n = fxNodes(fx)
    const now = this.ctx.currentTime
    const to = (p: AudioParam, v: number) => p.setTargetAtTime(v, now, TC)
    to(this.lowCut.frequency, n.lowCutHz)
    to(this.bass.gain, n.bassDb)
    to(this.presence.gain, n.presenceDb)
    to(this.air.gain, n.airDb)
    to(this.comp.threshold, n.threshold)
    to(this.comp.ratio, n.ratio)
    to(this.makeUp.gain, n.makeUp)
    to(this.pan.pan, n.pan)
    to(this.doubleLevel.gain, n.double)
    to(this.echoLevel.gain, n.echo)
    to(this.echoDelay.delayTime, n.echoTime)
    to(this.echoBack.gain, n.echo > 0 ? n.echoFeedback : 0)
    to(this.mBass.gain, n.musicBassDb)
    to(this.mTreble.gain, n.musicTrebleDb)
    to(this.pocket.gain, n.pocketDb)
    if (n.reverb > 0 && (!this.reverb || Math.abs(this.roomMade - n.room) > 0.05)) this.makeRoom(n.room)
    to(this.reverbLevel.gain, n.reverb)
    if (isOff(fx) && this.reverb) { this.reverb.disconnect(); this.pan.disconnect(this.reverb); this.reverb = null; this.roomMade = 0 }
  }

  // A room: noise that dies away over its length, different in each ear, made once per size.
  private makeRoom(seconds: number) {
    const rate = this.ctx.sampleRate
    const len = Math.max(1, Math.floor(rate * seconds))
    const buf = this.ctx.createBuffer(2, len, rate)
    for (let ch = 0; ch < 2; ch++) {
      const d = buf.getChannelData(ch)
      for (let i = 0; i < len; i++) d[i] = (Math.random() * 2 - 1) * Math.pow(1 - i / len, 3)
    }
    const next = this.ctx.createConvolver()
    next.buffer = buf
    if (this.reverb) { this.pan.disconnect(this.reverb); this.reverb.disconnect() }
    this.pan.connect(next)
    next.connect(this.reverbLevel)
    this.reverb = next
    this.roomMade = seconds
  }

  close() {
    for (const n of [this.voice, this.music, this.pan, this.echoDelay, this.echoLevel, this.doubleLevel, this.reverbLevel, this.pocket, this.makeUp]) { try { n.disconnect() } catch { /* already */ } }
    this.reverb?.disconnect()
  }
}
