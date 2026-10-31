// How frames reach the browser. Three ways, tried in this order, all behind one
// interface so the rest of the player never knows which is in use:
//   1. WebRTC: lowest latency, the normal case
//   2. WebSocket with WebCodecs: when UDP is blocked or only HTTP gets through
//   3. WebSocket with JPEG: when nothing else works
// docs/GAMING.md has the reasoning.

export type TransportKind = 'webrtc' | 'ws-h264' | 'ws-jpeg'
export type Mode = 'auto' | TransportKind

export const TRANSPORT_LABELS: Record<TransportKind, string> = {
  webrtc: 'WebRTC (lowest latency)',
  'ws-h264': 'WebSocket with H.264',
  'ws-jpeg': 'WebSocket with JPEG (most compatible)',
}

export type Stats = { rttMs?: number; fps?: number; kbps?: number; jitterMs?: number; lost?: number; codec?: string; decodeMs?: number }

export type PlayEvent = { type: string; port?: number; strength?: number; text?: string; effect?: number; aspect?: number; width?: number; height?: number; turns?: number; index?: number; count?: number }

export interface Transport {
  readonly kind: TransportKind
  readonly element: HTMLVideoElement | HTMLCanvasElement
  connect(): Promise<void> // resolves once the first frame has arrived
  send(msg: Uint8Array): void
  close(): void
  stats(): Promise<Stats>
  // True when the browser refused to start sound without a tap.
  needsUnmute(): boolean
  unmute(): void
  onEvent?: (e: PlayEvent) => void
  onLost?: () => void
}

export type Signal = {
  answer(sdp: string, prefer?: string): Promise<string>
  wsUrl(video: string): string
}

const hasWebRTC = () => typeof RTCPeerConnection !== 'undefined'
const hasWebCodecs = () => typeof (window as any).VideoDecoder !== 'undefined' && typeof (window as any).EncodedVideoChunk !== 'undefined'

// What this browser can try, best first.
// The server says which modes it has (`allowed`); this says which this browser can use.
export function plan(mode: Mode, allowed: string[]): TransportKind[] {
  if (mode !== 'auto') return [mode]
  const out: TransportKind[] = []
  if (hasWebRTC()) out.push('webrtc')
  if (hasWebCodecs()) out.push('ws-h264')
  out.push('ws-jpeg')
  return out.filter((k) => allowed.includes(k))
}

export function make(kind: TransportKind, signal: Signal): Transport {
  if (kind === 'webrtc') return new WebRTCTransport(signal)
  return new WSTransport(signal, kind)
}

const FIRST_FRAME_MS = 12000

class WebRTCTransport implements Transport {
  readonly kind = 'webrtc'
  readonly element: HTMLVideoElement
  onEvent?: (e: PlayEvent) => void
  onLost?: () => void
  private pc: RTCPeerConnection | null = null
  private input: RTCDataChannel | null = null
  private muted = false
  private prev: { t: number; bytes: number; frames: number } | null = null
  private closed = false

  constructor(private signal: Signal) {
    const v = document.createElement('video')
    v.playsInline = true
    v.autoplay = true
    v.setAttribute('playsinline', '')
    v.disablePictureInPicture = true
    this.element = v
  }

  async connect() {
    const pc = new RTCPeerConnection()
    this.pc = pc
    const stream = new MediaStream()
    this.element.srcObject = stream
    pc.ontrack = (e) => {
      stream.addTrack(e.track)
      // Ask the browser to keep its receive buffer as small as it can.
      const r = e.receiver as any
      if ('playoutDelayHint' in r) r.playoutDelayHint = 0
      if ('jitterBufferTarget' in r) r.jitterBufferTarget = 0
    }
    pc.addTransceiver('video', { direction: 'recvonly' })
    pc.addTransceiver('audio', { direction: 'recvonly' })
    this.input = pc.createDataChannel('input', { ordered: false, maxRetransmits: 0 })
    this.input.binaryType = 'arraybuffer'
    const events = pc.createDataChannel('events')
    events.onmessage = (m) => { try { this.onEvent?.(JSON.parse(m.data)) } catch { /* not ours */ } }
    pc.onconnectionstatechange = () => {
      if (!this.closed && (pc.connectionState === 'failed' || pc.connectionState === 'closed')) this.onLost?.()
    }
    const offer = await pc.createOffer()
    await pc.setLocalDescription(offer)
    await gathered(pc, 1500)
    const answer = await this.signal.answer(pc.localDescription!.sdp)
    await pc.setRemoteDescription({ type: 'answer', sdp: answer })
    await this.play()
    await firstFrame(this.element, FIRST_FRAME_MS)
  }

  private async play() {
    try { await this.element.play() } catch {
      // Autoplay rules: start without sound, and the player offers a tap.
      this.muted = true
      this.element.muted = true
      try { await this.element.play() } catch { /* a tap will start it */ }
    }
  }

  needsUnmute() { return this.muted }
  unmute() { this.muted = false; this.element.muted = false; void this.element.play().catch(() => {}) }

  send(msg: Uint8Array) {
    if (this.input?.readyState === 'open') {
      try { this.input.send(msg as Uint8Array<ArrayBuffer>) } catch { /* the next state corrects it */ }
    }
  }

  close() {
    this.closed = true
    this.pc?.close()
    this.element.srcObject = null
  }

  async stats(): Promise<Stats> {
    const out: Stats = {}
    if (!this.pc) return out
    const report = await this.pc.getStats()
    const codecs = new Map<string, string>()
    report.forEach((s: any) => { if (s.type === 'codec') codecs.set(s.id, s.mimeType) })
    report.forEach((s: any) => {
      if (s.type === 'inbound-rtp' && s.kind === 'video') {
        out.lost = s.packetsLost
        out.jitterMs = s.jitter != null ? Math.round(s.jitter * 1000) : undefined
        out.codec = codecs.get(s.codecId)?.replace('video/', '')
        if (s.framesDecoded && s.totalDecodeTime) out.decodeMs = Math.round((s.totalDecodeTime / s.framesDecoded) * 1000 * 10) / 10
        const now = s.timestamp as number
        if (this.prev && now > this.prev.t) {
          const dt = (now - this.prev.t) / 1000
          out.kbps = Math.round(((s.bytesReceived - this.prev.bytes) * 8) / 1000 / dt)
          out.fps = Math.round((s.framesDecoded - this.prev.frames) / dt)
        }
        this.prev = { t: now, bytes: s.bytesReceived, frames: s.framesDecoded }
      }
      if (s.type === 'candidate-pair' && s.nominated && s.currentRoundTripTime != null) out.rttMs = Math.round(s.currentRoundTripTime * 1000)
    })
    return out
  }
}

function gathered(pc: RTCPeerConnection, maxMs: number) {
  return new Promise<void>((resolve) => {
    if (pc.iceGatheringState === 'complete') return resolve()
    const done = () => { if (pc.iceGatheringState === 'complete') resolve() }
    pc.addEventListener('icegatheringstatechange', done)
    setTimeout(resolve, maxMs) // what was found by now is enough on most networks
  })
}

function firstFrame(v: HTMLVideoElement, ms: number) {
  return new Promise<void>((resolve, reject) => {
    const t = setTimeout(() => { clearInterval(iv); reject(new Error('no picture arrived')) }, ms)
    const iv = setInterval(() => {
      if (v.videoWidth > 0) { clearTimeout(t); clearInterval(iv); resolve() }
    }, 50)
  })
}

// ---- WebSocket ----

const MSG_H264 = 1, MSG_JPEG = 2, MSG_PCM = 3

class WSTransport implements Transport {
  readonly element: HTMLCanvasElement
  onEvent?: (e: PlayEvent) => void
  onLost?: () => void
  private ws: WebSocket | null = null
  private ctx: CanvasRenderingContext2D
  private decoder: any = null
  private audio: AudioContext | null = null
  private audioAt = 0
  private frames = 0
  private bytes = 0
  private statT = performance.now()
  private ts = 0
  private needKey = true
  private first: (() => void) | null = null
  private closed = false
  private drawing = false // JPEG: the newest image wins

  constructor(private signal: Signal, readonly kind: 'ws-h264' | 'ws-jpeg') {
    this.element = document.createElement('canvas')
    this.element.width = 256
    this.element.height = 224
    this.ctx = this.element.getContext('2d', { alpha: false, desynchronized: true } as CanvasRenderingContext2DSettings) as CanvasRenderingContext2D
  }

  async connect() {
    if (this.kind === 'ws-h264') await this.makeDecoder()
    await new Promise<void>((resolve, reject) => {
      const t = setTimeout(() => reject(new Error('no picture arrived')), FIRST_FRAME_MS)
      this.first = () => { clearTimeout(t); resolve() }
      const ws = new WebSocket(this.signal.wsUrl(this.kind === 'ws-h264' ? 'h264' : 'jpeg'))
      ws.binaryType = 'arraybuffer'
      this.ws = ws
      ws.onmessage = (m) => this.message(m.data)
      ws.onerror = () => { clearTimeout(t); reject(new Error('the connection failed')) }
      ws.onclose = () => { if (!this.closed) this.onLost?.() }
    })
  }

  private async makeDecoder() {
    const VD = (window as any).VideoDecoder
    const config = { codec: 'avc1.42E01F', optimizeForLatency: true }
    const support = await VD.isConfigSupported(config).catch(() => null)
    if (!support?.supported) throw new Error('this browser cannot decode H.264 here')
    this.decoder = new VD({
      output: (f: any) => {
        if (this.element.width !== f.displayWidth || this.element.height !== f.displayHeight) {
          this.element.width = f.displayWidth
          this.element.height = f.displayHeight
        }
        this.ctx.drawImage(f, 0, 0)
        f.close()
        this.frames++
        this.first?.()
        this.first = null
      },
      error: () => { this.needKey = true; if (!this.closed) void this.remake() },
    })
    this.decoder.configure(config)
  }

  private async remake() {
    try { this.decoder?.close() } catch { /* already closed */ }
    await this.makeDecoder().catch(() => this.onLost?.())
  }

  private message(data: ArrayBuffer | string) {
    if (typeof data === 'string') {
      try { const e = JSON.parse(data); if (e.type !== 'hello') this.onEvent?.(e) } catch { /* not ours */ }
      return
    }
    const b = new Uint8Array(data)
    this.bytes += b.length
    switch (b[0]) {
      case MSG_H264: this.h264(b); break
      case MSG_JPEG: this.jpeg(b); break
      case MSG_PCM: this.pcm(data); break
    }
  }

  private h264(b: Uint8Array) {
    const key = b[1] === 1
    if (this.needKey && !key) return
    if (!this.decoder || this.decoder.state !== 'configured') return
    // A decoder that has fallen behind is skipped ahead, because a late frame is
    // worth nothing in a game.
    if (this.decoder.decodeQueueSize > 3 && !key) { this.needKey = true; return }
    this.needKey = false
    const Chunk = (window as any).EncodedVideoChunk
    this.ts += 16667
    this.decoder.decode(new Chunk({ type: key ? 'key' : 'delta', timestamp: this.ts, data: b.subarray(2) }))
  }

  private jpeg(b: Uint8Array) {
    if (this.drawing) return
    this.drawing = true
    const blob = new Blob([b.subarray(1) as Uint8Array<ArrayBuffer>], { type: 'image/jpeg' })
    createImageBitmap(blob).then((img) => {
      if (this.element.width !== img.width || this.element.height !== img.height) {
        this.element.width = img.width
        this.element.height = img.height
      }
      this.ctx.drawImage(img, 0, 0)
      img.close()
      this.frames++
      this.first?.()
      this.first = null
    }).catch(() => {}).finally(() => { this.drawing = false })
  }

  // Sound is scheduled in small pieces. Falling too far behind means skipping
  // ahead instead of letting delay grow.
  private pcm(data: ArrayBuffer) {
    if (!this.audio) {
      try { this.audio = new AudioContext({ latencyHint: 'interactive' }) } catch { return }
    }
    if (this.audio.state === 'suspended') return // waiting for a tap
    const rate = new DataView(data).getUint32(1, true)
    const n = Math.floor((data.byteLength - 5) / 4)
    if (n === 0 || rate < 3000 || rate > 384000) return
    const buf = this.audio.createBuffer(2, n, rate)
    const l = buf.getChannelData(0), r = buf.getChannelData(1)
    const v = new DataView(data, 5)
    for (let i = 0; i < n; i++) {
      l[i] = v.getInt16(i * 4, true) / 32768
      r[i] = v.getInt16(i * 4 + 2, true) / 32768
    }
    const now = this.audio.currentTime
    if (this.audioAt < now) this.audioAt = now + 0.03
    if (this.audioAt > now + 0.25) this.audioAt = now + 0.03
    const src = this.audio.createBufferSource()
    src.buffer = buf
    src.connect(this.audio.destination)
    src.start(this.audioAt)
    this.audioAt += n / rate
  }

  needsUnmute() { return !this.audio || this.audio.state === 'suspended' }
  unmute() {
    if (!this.audio) { try { this.audio = new AudioContext({ latencyHint: 'interactive' }) } catch { return } }
    void this.audio.resume().catch(() => {})
  }

  send(msg: Uint8Array) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(msg as Uint8Array<ArrayBuffer>)
  }

  close() {
    this.closed = true
    this.ws?.close()
    try { this.decoder?.close() } catch { /* already closed */ }
    void this.audio?.close().catch(() => {})
  }

  async stats(): Promise<Stats> {
    const now = performance.now()
    const dt = (now - this.statT) / 1000
    const out: Stats = dt > 0 ? { fps: Math.round(this.frames / dt), kbps: Math.round((this.bytes * 8) / 1000 / dt), codec: this.kind === 'ws-h264' ? 'H264' : 'JPEG' } : {}
    this.frames = 0
    this.bytes = 0
    this.statT = now
    return out
  }
}
