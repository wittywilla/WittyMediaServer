import { FFT_SIZE } from './analysis.ts'

// Listening to what's playing, without touching it. This is the part that could hurt
// playback, so here's what it guards against:
//
// - Routing the element into Web Audio takes its sound away from it for good, and if the
//   context stops or the stream is from another address, the music goes silent. So where
//   the browser can give a copy of the sound (captureStream), that's what's used: the
//   element keeps playing straight to the speakers, and closing the tap leaves no trace.
// - Where it can't (Firefox's version mutes the element, and Safari has none), the element
//   is routed through a context, only once that context is running, and only when the
//   stream is on this page's own address. A context can't let go of an element, so after
//   that it keeps passing the sound through until the page closes. It costs next to nothing,
//   and the analysis itself stops.
// - A stream on another address has its sound hidden from the page unless the server has
//   agreed to share it, and asking changes how the stream is fetched. So it isn't asked:
//   the picture follows a gentle pattern instead, and says so.
// - The address can carry the sign-in, so it's compared by origin and never written down.

export type TapState = 'live' | 'pattern' | 'needs-gesture' | 'closed'

export interface AudioTap {
  state(): TapState
  why(): string
  rate(): number
  // Fills both with the newest FFT_SIZE samples.
  read(left: Float32Array, right: Float32Array): void
  // Call from a press or a click.
  resume(): Promise<void>
  close(): void
}

type Captureable = HTMLMediaElement & { captureStream?: () => MediaStream }

// What a host can read an address's origin from without keeping the address.
function readable(el: HTMLMediaElement) {
  if (el.crossOrigin !== null) return true // the host has already asked the server to share
  const src = el.currentSrc || el.src
  if (!src) return true
  try { return new URL(src, location.href).origin === location.origin } catch { return false }
}

function mono(left: Float32Array, right: Float32Array) {
  // A mono source fills one side of the splitter.
  let l = 0, r = 0
  for (let i = 0; i < left.length; i += 16) { l += Math.abs(left[i]); r += Math.abs(right[i]) }
  if (r === 0 && l > 0) right.set(left)
  else if (l === 0 && r > 0) left.set(right)
}

class Listener {
  ctx: AudioContext
  splitter: ChannelSplitterNode
  left: AnalyserNode
  right: AnalyserNode
  constructor(ctx: AudioContext) {
    this.ctx = ctx
    this.splitter = ctx.createChannelSplitter(2)
    const an = () => { const a = ctx.createAnalyser(); a.fftSize = FFT_SIZE; a.smoothingTimeConstant = 0; return a }
    this.left = an()
    this.right = an()
    this.splitter.connect(this.left, 0)
    this.splitter.connect(this.right, 1)
  }
  read(left: Float32Array, right: Float32Array) {
    this.left.getFloatTimeDomainData(left as Float32Array<ArrayBuffer>)
    this.right.getFloatTimeDomainData(right as Float32Array<ArrayBuffer>)
    mono(left, right)
  }
}

// Chromium: a copy of the sound. The element isn't changed at all.
class CaptureTap implements AudioTap {
  private ctx = new AudioContext({ latencyHint: 'playback' })
  private stream: MediaStream
  private listener = new Listener(this.ctx)
  private source: MediaStreamAudioSourceNode | null = null
  private trackId = ''
  private reads = 0
  constructor(el: Captureable) {
    this.stream = el.captureStream!()
    this.connect()
  }
  private connect() {
    const t = this.stream.getAudioTracks()[0]
    if (!t || t.id === this.trackId) return
    this.source?.disconnect()
    this.trackId = t.id
    this.source = this.ctx.createMediaStreamSource(new MediaStream([t]))
    this.source.connect(this.listener.splitter)
  }
  state(): TapState { return this.ctx.state === 'closed' ? 'closed' : this.ctx.state === 'running' ? 'live' : 'needs-gesture' }
  why() { return '' }
  rate() { return this.ctx.sampleRate }
  read(left: Float32Array, right: Float32Array) {
    // A new track means a new stream from the element, which happens when it changes song.
    if (this.reads++ % 15 === 0) this.connect()
    this.listener.read(left, right)
  }
  async resume() { await this.ctx.resume().catch(() => {}) }
  close() {
    this.source?.disconnect()
    for (const t of this.stream.getTracks()) t.stop()
    void this.ctx.close().catch(() => {})
  }
}

// Other browsers: the element goes through a context, which it can never leave.
type Routed = { ctx: AudioContext; source: MediaElementAudioSourceNode }
const routed = new WeakMap<HTMLMediaElement, Routed>()

class RouteTap implements AudioTap {
  private el: HTMLMediaElement
  private route: Routed | null
  private ctx: AudioContext
  private listener: Listener
  private onChange: () => void
  constructor(el: HTMLMediaElement) {
    this.el = el
    this.route = routed.get(el) ?? null
    this.ctx = this.route?.ctx ?? new AudioContext({ latencyHint: 'playback' })
    this.listener = new Listener(this.ctx)
    // If the system stops the context (a phone call, on a phone), the music would stop too.
    this.onChange = () => { if (this.route && this.ctx.state !== 'running' && this.ctx.state !== 'closed') void this.ctx.resume().catch(() => {}) }
    this.ctx.addEventListener('statechange', this.onChange)
    this.connect()
  }
  private connect() {
    if (this.ctx.state !== 'running') return
    if (!this.route) {
      this.route = { ctx: this.ctx, source: this.ctx.createMediaElementSource(this.el) }
      routed.set(this.el, this.route)
      this.route.source.connect(this.ctx.destination)
    }
    this.route.source.connect(this.listener.splitter)
  }
  state(): TapState { return this.ctx.state === 'running' && this.route ? 'live' : 'needs-gesture' }
  why() { return '' }
  rate() { return this.ctx.sampleRate }
  read(left: Float32Array, right: Float32Array) { this.listener.read(left, right) }
  async resume() {
    await this.ctx.resume().catch(() => {})
    this.connect()
  }
  close() {
    this.ctx.removeEventListener('statechange', this.onChange)
    if (this.route) this.route.source.disconnect(this.listener.splitter)
    else void this.ctx.close().catch(() => {})
  }
}

// When the sound can't be heard: slow swells and a soft pulse, so the picture still moves.
class PatternTap implements AudioTap {
  private at = 0
  constructor(private reason: string) {}
  state(): TapState { return 'pattern' }
  why() { return this.reason }
  rate() { return 44100 }
  read(left: Float32Array, right: Float32Array) {
    const r = 44100, n = left.length
    const slow = this.at / r
    const low = 0.25 + 0.2 * Math.sin(slow * 0.7), middle = 0.12 + 0.1 * Math.sin(slow * 1.1 + 2), high = 0.05 + 0.04 * Math.sin(slow * 1.7 + 4)
    for (let i = 0; i < n; i++) {
      const t = (this.at + i) / r
      const pulse = Math.exp(-((t * 1.2) % 1) * 7)
      const v = Math.sin(t * 2 * Math.PI * 70) * low * pulse + Math.sin(t * 2 * Math.PI * 440) * middle + Math.sin(t * 2 * Math.PI * 3500) * high
      left[i] = v
      right[i] = Math.sin(t * 2 * Math.PI * 70) * low * pulse + Math.sin(t * 2 * Math.PI * 660) * middle + Math.sin(t * 2 * Math.PI * 2500) * high
    }
    this.at += 735
  }
  async resume() { /* nothing to start */ }
  close() { /* nothing to stop */ }
}

export function openTap(el: HTMLMediaElement | null): AudioTap {
  if (!el) return new PatternTap('Nothing is playing yet.')
  if (typeof AudioContext === 'undefined') return new PatternTap('This browser can\'t listen to the music, so the picture is following a pattern.')
  if (!readable(el)) return new PatternTap('This server is on a different address from this page, so its sound can\'t be read here. The picture is following a pattern.')
  try {
    const c = el as Captureable
    if (typeof c.captureStream === 'function') return new CaptureTap(c)
    return new RouteTap(el)
  } catch {
    return new PatternTap('This browser wouldn\'t let the music be read, so the picture is following a pattern.')
  }
}
