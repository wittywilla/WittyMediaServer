import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { buttonMessage, keyMessage, moveMessage, padMessage, Pads, positionMessage, scrollMessage, TOUCH_BUTTONS, vkFor } from './input'
import './player.css'

// The player: a stream from a PC, in the page. It holds no game logic and knows no server: the
// page around it provides the API through MoonlightHost. Everything here is this project's own,
// and the stream arrives over ordinary WebRTC from the relay.

export type StreamRequest = { width: number; height: number; fps: number; bitrate: number }
export type StreamInfo = { id: string; title: string; width?: number; height?: number; fps?: number }

export interface MoonlightHost {
  title: string
  start(req: StreamRequest): Promise<StreamInfo>
  answer(sid: string, sdp: string): Promise<string>
  end(sid: string, quit: boolean): Promise<void>
  close(): void
}

type Prefs = { height: number; fps: number; bitrate: number }
const PREF_KEY = 'wms.moonlight'
const HEIGHTS = [720, 1080, 1440, 2160]
const defaultBitrate = (height: number, fps: number) => Math.round(({ 720: 10000, 1080: 20000, 1440: 40000, 2160: 80000 } as Record<number, number>)[height] * (fps > 60 ? 1.5 : fps < 60 ? 0.7 : 1))

function loadPrefs(): Prefs {
  try {
    const p = JSON.parse(localStorage.getItem(PREF_KEY) || 'null')
    if (p && p.height && p.fps) return p
  } catch { /* a private window, perhaps */ }
  const tall = Math.round((window.screen?.height || 1080) * (window.devicePixelRatio || 1))
  const height = [...HEIGHTS].reverse().find((h) => h <= tall) || 720
  const capped = Math.min(height, 1080)
  return { height: capped, fps: 60, bitrate: defaultBitrate(capped, 60) }
}

const END_REASONS: Record<number, string> = {
  0: 'The game was closed on the PC.',
  [-100]: 'No video arrived from the PC. A firewall on the PC or its network may be blocking it.',
  [-101]: 'The PC stopped sending video.',
  [-102]: 'The PC ended the stream straight away. Check that its screen is on and its encoder is working.',
  [-103]: 'The PC can\'t stream what\'s on its screen, as it\'s protected.',
}

export default function MoonlightPlayer({ host }: { host: MoonlightHost }) {
  const video = useRef<HTMLVideoElement | null>(null)
  const pcRef = useRef<RTCPeerConnection | null>(null)
  const inputRef = useRef<RTCDataChannel | null>(null)
  const controlRef = useRef<RTCDataChannel | null>(null)
  const sidRef = useRef('')
  const padsRef = useRef<Pads | null>(null)
  const [phase, setPhase] = useState<'choose' | 'starting' | 'playing' | 'ended'>('choose')
  const [message, setMessage] = useState('')
  const [menu, setMenu] = useState(false)
  const [muted, setMuted] = useState(false)
  const [captured, setCaptured] = useState(false)
  const [padCount, setPadCount] = useState(0)
  const [poor, setPoor] = useState(false)
  const [touchPad, setTouchPad] = useState(() => matchMedia('(pointer: coarse)').matches)
  const [prefs, setPrefs] = useState<Prefs>(loadPrefs)
  const touchState = useRef({ buttons: 0, lx: 0, ly: 0 })

  const send = useCallback((m: Uint8Array, reliable: boolean) => {
    const dc = reliable ? controlRef.current : inputRef.current
    if (dc?.readyState === 'open') dc.send(new Uint8Array(m))
  }, [])

  const hangUp = useCallback(() => {
    padsRef.current?.stop()
    padsRef.current = null
    pcRef.current?.close()
    pcRef.current = null
    if (document.pointerLockElement) document.exitPointerLock()
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {})
  }, [])

  const begin = async () => {
    setPhase('starting')
    setMessage('Starting the game on the PC. This can take a little while…')
    try {
      localStorage.setItem(PREF_KEY, JSON.stringify(prefs))
    } catch { /* it just won't be remembered */ }
    try {
      // Width follows this screen's shape, on an even number, as encoders need.
      const aspect = Math.max(1.25, Math.min(3.6, (window.screen?.width || 16) / (window.screen?.height || 9)))
      const width = Math.round((prefs.height * aspect) / 2) * 2
      const info = await host.start({ width, height: prefs.height, fps: prefs.fps, bitrate: prefs.bitrate })
      sidRef.current = info.id
      await connect(info.id)
    } catch (e: any) {
      setMessage(e?.message || 'The stream couldn\'t start.')
      setPhase('ended')
    }
  }

  const connect = async (sid: string) => {
    const pc = new RTCPeerConnection()
    pcRef.current = pc
    pc.addTransceiver('video', { direction: 'recvonly' })
    pc.addTransceiver('audio', { direction: 'recvonly' })
    controlRef.current = pc.createDataChannel('control')
    inputRef.current = pc.createDataChannel('input', { ordered: false, maxRetransmits: 0 })
    controlRef.current.onmessage = (e) => {
      if (typeof e.data !== 'string') return
      let m: any
      try { m = JSON.parse(e.data) } catch { return }
      if (m.type === 'rumble') padsRef.current?.rumble(m.controller, m.low, m.high)
      else if (m.type === 'status') setPoor(!!m.poor)
      else if (m.type === 'terminated') {
        setMessage(END_REASONS[m.code] || `The stream ended (${m.code}).`)
        hangUp()
        setPhase('ended')
      }
    }
    pc.ontrack = (e) => {
      const v = video.current
      if (!v) return
      // Late is worse than rough, in a game: ask the browser to hold nothing back.
      const r = e.receiver as RTCRtpReceiver & { playoutDelayHint?: number; jitterBufferTarget?: number }
      try { r.playoutDelayHint = 0 } catch { /* not every browser has it */ }
      try { r.jitterBufferTarget = 0 } catch { /* not every browser has it */ }
      if (!v.srcObject) v.srcObject = new MediaStream()
      ;(v.srcObject as MediaStream).addTrack(e.track)
      v.play().catch(() => {
        // Browsers can refuse sound until someone touches the page.
        v.muted = true
        setMuted(true)
        v.play().catch(() => {})
      })
    }
    pc.onconnectionstatechange = () => {
      if (pc.connectionState === 'connected') {
        setPhase('playing')
        setMessage('')
      } else if (pc.connectionState === 'failed') {
        setMessage('The stream couldn\'t reach this device. If you\'re away from home, the server\'s UDP port may need opening: see the guide.')
        hangUp()
        setPhase('ended')
      }
    }
    const offer = await pc.createOffer()
    await pc.setLocalDescription(offer)
    await new Promise<void>((done) => {
      if (pc.iceGatheringState === 'complete') return done()
      const t = window.setTimeout(done, 2000)
      pc.onicegatheringstatechange = () => { if (pc.iceGatheringState === 'complete') { clearTimeout(t); done() } }
    })
    const answer = await host.answer(sid, pc.localDescription!.sdp)
    await pc.setRemoteDescription({ type: 'answer', sdp: answer })
    const pads = new Pads(send, () => setMenu((m) => !m), setPadCount)
    padsRef.current = pads
    pads.start()
  }

  useEffect(() => () => { hangUp() }, [hangUp])

  const leave = async (quit: boolean) => {
    const sid = sidRef.current
    hangUp()
    if (sid) await host.end(sid, quit).catch(() => {})
    host.close()
  }

  // Keyboard and mouse, once the picture has been clicked. Everything goes to the PC until
  // the way out below: browsers won't let a page hold on to Esc without the keyboard lock.
  useEffect(() => {
    if (phase !== 'playing') return
    const held = new Set<string>()
    const key = (down: boolean) => (e: KeyboardEvent) => {
      if (e.ctrlKey && e.altKey && e.shiftKey && e.code === 'KeyM') {
        e.preventDefault()
        if (down) setMenu((m) => !m)
        return
      }
      if (menu || !captured) return
      if (vkFor(e.code) === undefined) return
      e.preventDefault()
      if (down && e.repeat) return
      if (down) held.add(e.code)
      else held.delete(e.code)
      const m = keyMessage(e.code, down, e)
      if (m) send(m, true)
    }
    const onDown = key(true), onUp = key(false)
    const lock = () => {
      const locked = document.pointerLockElement === video.current || !!document.fullscreenElement
      if (!locked && captured) {
        // Anything held down when focus went stays down on the PC unless it's let go here.
        held.forEach((code) => { const m = keyMessage(code, false, { shiftKey: false, ctrlKey: false, altKey: false, metaKey: false }); if (m) send(m, true) })
        held.clear()
        setCaptured(false)
      }
    }
    window.addEventListener('keydown', onDown)
    window.addEventListener('keyup', onUp)
    document.addEventListener('pointerlockchange', lock)
    document.addEventListener('fullscreenchange', lock)
    return () => {
      document.removeEventListener('fullscreenchange', lock)
      window.removeEventListener('keydown', onDown)
      window.removeEventListener('keyup', onUp)
      document.removeEventListener('pointerlockchange', lock)
    }
  }, [phase, menu, captured, send])

  const capture = async () => {
    const v = video.current
    if (!v || menu) return
    if (v.muted && muted) { v.muted = false; setMuted(false) }
    try { await (v.parentElement as HTMLElement).requestFullscreen?.() } catch { /* a phone or a frame may refuse */ }
    try {
      await (navigator as Navigator & { keyboard?: { lock?(): Promise<void> } }).keyboard?.lock?.()
    } catch { /* only some browsers, and only when fullscreen */ }
    // Some browsers and most phones won't lock the pointer. The keyboard still comes along, and
    // the mouse then moves by where it is rather than how far it went.
    try { await (v.requestPointerLock() as unknown as Promise<void> | undefined) } catch { /* see above */ }
    setCaptured(true)
  }

  const mousePosition = (e: ReactPointerEvent<HTMLVideoElement>) => {
    const v = e.currentTarget
    const r = v.getBoundingClientRect()
    // The picture fills the element's shorter side, with bars on the other.
    const scale = Math.min(r.width / (v.videoWidth || r.width), r.height / (v.videoHeight || r.height))
    const w = (v.videoWidth || r.width) * scale, h = (v.videoHeight || r.height) * scale
    const x = e.clientX - r.left - (r.width - w) / 2, y = e.clientY - r.top - (r.height - h) / 2
    return positionMessage(Math.max(0, Math.min(w, x)), Math.max(0, Math.min(h, y)), w, h)
  }
  const onMove = (e: ReactPointerEvent<HTMLVideoElement>) => {
    if (document.pointerLockElement === e.currentTarget) send(moveMessage(e.movementX, e.movementY), false)
    else if (e.pointerType === 'touch' || captured) send(mousePosition(e), false)
  }
  const onPress = (e: ReactPointerEvent<HTMLVideoElement>, down: boolean) => {
    if (menu) return
    if (e.pointerType === 'mouse' && !captured) {
      if (down) void capture()
      return
    }
    if (e.pointerType === 'touch' && down) send(mousePosition(e), true)
    send(buttonMessage(e.button, down), true)
    e.preventDefault()
  }

  // The on-screen controller.
  const sendTouch = useCallback(() => {
    const t = touchState.current
    send(padMessage(0, 1, { buttons: t.buttons, lt: 0, rt: 0, lx: t.lx, ly: t.ly, rx: 0, ry: 0 }), true)
  }, [send])
  const press = (flag: number, down: boolean) => {
    touchState.current.buttons = down ? touchState.current.buttons | flag : touchState.current.buttons & ~flag
    sendTouch()
    if (down) navigator.vibrate?.(8)
  }
  const stickRef = useRef<HTMLDivElement | null>(null)
  const onStick = (e: ReactPointerEvent<HTMLDivElement>, end = false) => {
    const el = stickRef.current
    if (!el) return
    if (end) {
      touchState.current.lx = touchState.current.ly = 0
    } else {
      const r = el.getBoundingClientRect()
      const clamp = (v: number) => Math.max(-1, Math.min(1, v))
      touchState.current.lx = Math.round(clamp((e.clientX - r.left - r.width / 2) / (r.width / 2)) * 32767)
      touchState.current.ly = Math.round(clamp(-(e.clientY - r.top - r.height / 2) / (r.height / 2)) * 32767)
    }
    sendTouch()
  }
  const touchButton = (label: string, flag: number, cls: string) => (
    <button key={label} type="button" className={'ml-t ' + cls} aria-label={label}
      onPointerDown={(e) => { e.preventDefault(); press(flag, true) }} onPointerUp={() => press(flag, false)} onPointerCancel={() => press(flag, false)} onPointerLeave={() => press(flag, false)}>{label}</button>
  )
  const T = TOUCH_BUTTONS

  const setHeight = (height: number) => setPrefs((p) => ({ ...p, height, bitrate: defaultBitrate(height, p.fps) }))
  const setFps = (fps: number) => setPrefs((p) => ({ ...p, fps, bitrate: defaultBitrate(p.height, fps) }))

  return (
    <div className="ml-root">
      <video ref={video} className="ml-video" autoPlay playsInline disablePictureInPicture
        onPointerMove={onMove} onPointerDown={(e) => onPress(e, true)} onPointerUp={(e) => onPress(e, false)} onContextMenu={(e) => e.preventDefault()}
        onWheel={(e) => { if (captured) send(scrollMessage(-e.deltaY, false), true); if (captured && e.deltaX) send(scrollMessage(e.deltaX, true), true) }} />

      {phase === 'choose' && (
        <div className="ml-panel" role="dialog" aria-label={`Play ${host.title}`}>
          <h1>{host.title}</h1>
          <label>Picture
            <select value={prefs.height} onChange={(e) => setHeight(+e.target.value)}>
              {HEIGHTS.map((h) => <option key={h} value={h}>{h === 2160 ? '4K' : `${h}p`}</option>)}
            </select>
          </label>
          <label>Frames a second
            <select value={prefs.fps} onChange={(e) => setFps(+e.target.value)}>
              {[30, 60, 90, 120].map((f) => <option key={f} value={f}>{f}</option>)}
            </select>
          </label>
          <label>Quality: {(prefs.bitrate / 1000).toFixed(0)} Mbps
            <input type="range" min={2000} max={150000} step={1000} value={prefs.bitrate} onChange={(e) => setPrefs({ ...prefs, bitrate: +e.target.value })} />
            <small>Lower it if the picture stutters on Wi-Fi or away from home.</small>
          </label>
          <div className="ml-row">
            <button className="ml-btn primary" autoFocus onClick={begin}>Play</button>
            <button className="ml-btn" onClick={() => host.close()}>Back</button>
          </div>
        </div>
      )}
      {phase === 'starting' && <div className="ml-panel" role="status"><p>{message}</p></div>}
      {phase === 'ended' && (
        <div className="ml-panel" role="alert">
          <p>{message}</p>
          <div className="ml-row">
            <button className="ml-btn primary" autoFocus onClick={() => { setMessage(''); setPhase('choose') }}>Try again</button>
            <button className="ml-btn" onClick={() => host.close()}>Back</button>
          </div>
        </div>
      )}

      {phase === 'playing' && !captured && !menu && !touchPad && (
        <div className="ml-hint">Click the picture for keyboard and mouse. <kbd>Esc</kbd> gives them back. {padCount > 0 ? `${padCount} controller${padCount > 1 ? 's' : ''} connected.` : 'A controller works as soon as you press a button on it.'}</div>
      )}
      {poor && phase === 'playing' && <div className="ml-poor" role="status">The connection is struggling. Lower the quality if this keeps up.</div>}
      {phase === 'playing' && !captured && <button className="ml-menu-btn" onClick={() => setMenu(!menu)} aria-expanded={menu}>Menu</button>}

      {phase === 'playing' && touchPad && !menu && (
        <div className="ml-touch">
          <div className="ml-t-left">
            {touchButton('LB', T.lb, 'sh')}
            <div className="ml-stick" ref={stickRef} onPointerDown={(e) => { e.currentTarget.setPointerCapture(e.pointerId); onStick(e) }} onPointerMove={(e) => { if (e.buttons || e.pointerType === 'touch') onStick(e) }} onPointerUp={(e) => onStick(e, true)} onPointerCancel={(e) => onStick(e, true)} role="presentation" />
            <div className="ml-dpad">{touchButton('↑', T.up, 'u')}{touchButton('←', T.left, 'l')}{touchButton('→', T.right, 'r')}{touchButton('↓', T.down, 'd')}</div>
          </div>
          <div className="ml-t-mid">{touchButton('Back', T.back, 'sm')}{touchButton('Start', T.start, 'sm')}</div>
          <div className="ml-t-right">
            {touchButton('RB', T.rb, 'sh')}
            <div className="ml-abxy">{touchButton('Y', T.y, 'u')}{touchButton('X', T.x, 'l')}{touchButton('B', T.b, 'r')}{touchButton('A', T.a, 'd')}</div>
          </div>
        </div>
      )}

      {menu && phase === 'playing' && (
        <div className="ml-panel" role="dialog" aria-label="Game menu">
          <h1>{host.title}</h1>
          <p className="ml-note">Press <kbd>Ctrl</kbd>+<kbd>Alt</kbd>+<kbd>Shift</kbd>+<kbd>M</kbd>, or hold Start, Back and both shoulders, to open this.</p>
          <button className="ml-btn primary" autoFocus onClick={() => setMenu(false)}>Back to the game</button>
          <button className="ml-btn" onClick={() => { setTouchPad(!touchPad); setMenu(false) }}>{touchPad ? 'Hide' : 'Show'} the on-screen controller</button>
          <button className="ml-btn" onClick={() => leave(false)}>Leave it running on the PC</button>
          <button className="ml-btn danger" onClick={() => leave(true)}>Close the game</button>
        </div>
      )}
    </div>
  )
}
