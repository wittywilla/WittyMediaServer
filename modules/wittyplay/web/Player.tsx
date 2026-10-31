import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { SELECT, START, type ControlsData, type SaveControl } from './consoles'
import { DisplayEngine } from './display'
import { HOLD_ACTIONS, Hotkeys, type HotAction } from './hotkeys'
import { deviceId, InputManager, load, onTV, padSupport, save } from './input'
import { computeLayout } from './layout'
import type { ControllerData } from './mapping'
import { Osd } from './Osd'
import { CheatsPanel, CorePanel, JsonEditor, PatchesPanel, SettingsPanel, SlotsPanel, type LevelChoice } from './Panels'
import { downloadBlob, fileName, loadSchema, useSettings, type Level, type Schema, type SettingsHost } from './settings'
import { ControllerStudio, type PortChoice, type ProfileHost } from './Studio'
import { TouchControls, TouchEditor, useOrientation } from './TouchControls'
import { make, plan, TRANSPORT_LABELS, type AudioBuffer_, type Mode, type PlayEvent, type Transport, type TransportKind } from './transport'
import './play.css'

// The player: one game, full window. The server does the running; this shows it
// and passes controllers along. Everything it needs from the app comes through
// PlayHost, so the module works in any app that can provide one.

export type SessionInfo = { id: string; itemId: number; title: string; console: string; core: string; width: number; height: number; aspect: number; fps: number; sampleRate: number; resumed: boolean; encoder?: string; renderer?: string; turns?: number; disc?: number; discs?: number; modes: { id: string }[]; devices?: PortChoice[]; safeMode?: boolean; maxSpeed?: number; notices?: string[] }
export type SaveSlot = { slot: number; updatedAt: number; size: number; hasThumb: boolean; core?: string }
export type PrepareStatus = { state: 'ready' | 'waiting' | 'reading' | 'unpacking'; done: number; total: number }

export interface PlayHost extends SettingsHost {
  itemId: number
  title: string
  consoleId: string
  consoleName: string
  resume: boolean
  safeMode: boolean
  // Gets a game ready, for one that has to be unpacked first. Called until it's ready.
  prepare?(): Promise<PrepareStatus>
  start(resume: boolean, safeMode: boolean): Promise<SessionInfo>
  relaunch(opts: { resume: boolean; safeMode: boolean }): void // start this game again
  stop(sessionId: string): Promise<void>
  answer(sessionId: string, sdp: string, prefer?: string): Promise<string>
  wsUrl(sessionId: string, video: string): string
  action(sessionId: string, type: string, slot?: number, on?: boolean, extra?: { port?: number; device?: number; value?: number; key?: string; text?: string }): Promise<any>
  controls(device: string): Promise<ControlsData>
  saveControl(s: SaveControl): Promise<void>
  controllers(): Promise<ControllerData>
  profiles: ProfileHost
  close(): void // leave the player
}

type SettingsGroup = 'display' | 'osd' | 'emulation' | 'audio' | 'states' | 'hotkeys'
type Panel = null | 'menu' | 'save' | 'load' | 'controllers' | 'connection' | 'touch' | 'discs' | SettingsGroup | 'core' | 'cheats' | 'patches' | 'json'

const MODE_KEY = 'wittyplay.mode'
const TOUCH_KEY = 'wittyplay.touch'
const LEVEL_KEY = 'wittyplay.level'

// The screens that want the game in view, so that a change can be seen as it's made.
const SIDE_PANELS: Panel[] = ['display', 'osd', 'audio', 'core']
const GROUP_TITLES: Record<SettingsGroup, string> = { display: 'Picture', osd: 'On-screen numbers', emulation: 'Speed and rewind', audio: 'Sound', states: 'Saves', hotkeys: 'Shortcuts' }

// prepared waits for a game that has to be unpacked before it can start, saying how
// that's going. A large disc can take minutes on a small server.
async function prepared(host: PlayHost, say: (s: string) => void, cancelled: () => boolean) {
  if (!host.prepare) return
  for (let waited = false; ; waited = true) {
    const p = await host.prepare()
    if (cancelled()) return
    if (p.state === 'ready') {
      if (waited) say('Starting the game…')
      return
    }
    if (p.state === 'waiting') say('Waiting for another game to finish unpacking…')
    else say(p.state === 'unpacking' && p.total > 0 ? `Unpacking the game… ${Math.min(99, Math.floor((p.done / p.total) * 100))}%` : 'Unpacking the game…')
    await new Promise((r) => setTimeout(r, 1000))
  }
}

export default function GamePlayer({ host }: { host: PlayHost }) {
  const [controls, setControls] = useState<ControlsData | null>(null)
  const [known, setKnown] = useState<ControllerData | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    // A list of pads is a help, not a need: the game runs without it.
    Promise.all([host.controls(deviceId()), host.controllers().catch(() => ({ layouts: {}, pads: [] }))])
      .then(([c, k]) => { setKnown(k); setControls(c) }).catch((e) => setError(e?.message || 'The game could not start.'))
  }, [host])
  if (error) {
    return (
      <div className="wp-root"><div className="wp-error" role="alert"><p>{error}</p><div className="wp-row wp-wrap wp-center"><button type="button" className="wp-btn primary" onClick={() => host.close()}>Back</button></div></div></div>
    )
  }
  return controls ? <Playing host={host} initial={controls} known={known} /> : <div className="wp-root"><div className="wp-status" role="status"><span className="wp-spinner" aria-hidden="true" />Starting the game…</div></div>
}

function Playing({ host, initial, known }: { host: PlayHost; initial: ControlsData; known: ControllerData | null }) {
  const [controls, setControls] = useState(initial)
  const [info, setInfo] = useState<SessionInfo | null>(null)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('Starting the game…')
  const [transport, setTransport] = useState<Transport | null>(null)
  const [panel, setPanel] = useState<Panel>(null)
  const [needsTap, setNeedsTap] = useState(false)
  const [toast, setToast] = useState('')
  const [schema, setSchema] = useState<Schema | null>(null)
  const [level, setLevelState] = useState<Level>(() => load<Level>(LEVEL_KEY) || 'console')
  const [fastForward, setFastForward] = useState(false)
  const [slow, setSlow] = useState(false)
  const [rewinding, setRewinding] = useState(false)
  const [frozen, setFrozen] = useState(false)
  const [tabHidden, setTabHidden] = useState(false)
  const [heldSpeed, setHeldSpeed] = useState(1)
  const [recording, setRecording] = useState(false)
  const [area, setArea] = useState({ w: 0, h: 0 })
  const [mode, setModeState] = useState<Mode>(() => load<Mode>(MODE_KEY) || 'auto')
  const [touchOn, setTouchOn] = useState<boolean>(() => load<boolean>(TOUCH_KEY) ?? (window.matchMedia?.('(pointer: coarse)').matches ?? false))
  const [padName, setPadName] = useState('')
  const orientation = useOrientation()
  const input = useMemo(() => new InputManager(initial, known, host.itemId), [initial, known, host.itemId])
  const tv = useMemo(onTV, [])
  // Layouts live on the server, so they follow the account to its other devices.
  const saveControl = useCallback(async (s: SaveControl) => {
    await host.saveControl(s)
    const fresh = await host.controls(deviceId())
    setControls(fresh)
    input.setControls(fresh)
  }, [host, input])
  const refreshControls = useCallback(async () => {
    const fresh = await host.controls(deviceId())
    setControls(fresh)
    input.setControls(fresh)
  }, [host, input])
  const pic = useRef<HTMLDivElement>(null)
  const areaRef = useRef<HTMLDivElement>(null)
  const engine = useRef<DisplayEngine | null>(null)
  const turnsRef = useRef(0)
  const root = useRef<HTMLDivElement>(null)
  const sessionRef = useRef<SessionInfo | null>(null)
  const transportRef = useRef<Transport | null>(null)
  const closing = useRef(false)
  const attempts = useRef(0)

  const say = useCallback((t: string) => setToast(t), [])
  useEffect(() => {
    if (!toast) return
    const t = setTimeout(() => setToast(''), 3500)
    return () => clearTimeout(t)
  }, [toast])

  // A person's settings, or defaults in safe mode: that's its point.
  const settings = useSettings(host, useMemo(() => ({ item: host.itemId }), [host.itemId]), say)
  useEffect(() => { loadSchema(host).then(setSchema).catch(() => {}) }, [host])
  const safe = !!info?.safeMode || host.safeMode
  const values = useMemo<Record<string, unknown>>(() => {
    if (safe) return Object.fromEntries((schema?.settings || []).map((d) => [d.key, d.default]))
    return settings.view?.resolved.values || {}
  }, [safe, schema, settings.view])
  const valuesRef = useRef(values)
  valuesRef.current = values
  const setLevel = (l: Level) => { setLevelState(l); save(LEVEL_KEY, l) }

  const onEvent = useCallback((e: PlayEvent) => {
    if (e.type === 'rumble') input.rumble(e.port || 0, e.strength || 0, e.effect || 0)
    else if (e.type === 'message' && e.text) say(e.text)
    else if (e.type === 'shutdown') { say('The game has ended'); leave() }
    else if (e.type === 'ended') {
      // The server said why, so a closing connection is no reason to try another.
      closing.current = true
      setStatus('')
      setError(e.text || 'The game has stopped.')
    } else if (e.type === 'geometry' || e.type === 'avinfo') {
      if (e.width && e.height) setInfo((i) => i && ({ ...i, width: e.width!, height: e.height!, aspect: e.aspect || 0 }))
    } else if (e.type === 'frame') {
      if (e.width && e.height) setInfo((i) => i && ({ ...i, width: e.width!, height: e.height! }))
    } else if (e.type === 'rotation') setInfo((i) => i && ({ ...i, turns: (e.turns || 0) & 3 }))
    else if (e.type === 'disc') setInfo((i) => i && ({ ...i, disc: e.index, discs: e.count }))
  }, [input])

  // Tries each way of connecting in turn until one shows a picture. A later failure
  // starts the list again, skipping what has already failed this session.
  const connect = useCallback(async (s: SessionInfo, m: Mode, skip: Set<TransportKind> = new Set()) => {
    // Only the newest attempt counts: changing the mode twice quickly abandons the first.
    const attempt = ++attempts.current
    const stale = () => closing.current || attempt !== attempts.current
    transportRef.current?.close()
    transportRef.current = null
    setTransport(null)
    const signal = {
      answer: (sdp: string, prefer?: string) => host.answer(s.id, sdp, prefer),
      wsUrl: (video: string) => host.wsUrl(s.id, video),
    }
    let last = ''
    for (const kind of plan(m, (s.modes || []).map((x) => x.id))) {
      if (skip.has(kind)) continue
      if (stale()) return
      setStatus(kind === 'webrtc' ? 'Connecting…' : 'Trying another way to connect…')
      const t = make(kind, signal)
      t.onEvent = onEvent
      try {
        await t.connect()
        if (stale()) { t.close(); return }
        t.onLost = () => {
          if (closing.current || transportRef.current !== t) return
          skip.add(kind)
          void connect(s, m, skip)
        }
        transportRef.current = t
        input.setSender((msg) => t.send(msg))
        setTransport(t)
        setStatus('')
        setNeedsTap(t.needsUnmute())
        // A picture has arrived, so the real size of the game's pixels is known now.
        host.request<SessionInfo>('GET', `/play/${s.id}?client=browser`).then((now) => setInfo((i) => i && ({ ...i, width: now.width, height: now.height }))).catch(() => {})
        return
      } catch (e: any) {
        t.close()
        if (stale()) return
        last = e?.message || String(e)
        skip.add(kind)
      }
    }
    setError('The game is running, but this browser couldn’t connect to it' + (last ? `: ${last}` : '.'))
  }, [host, input, onEvent])

  useEffect(() => {
    let cancelled = false
    closing.current = false
    prepared(host, setStatus, () => cancelled).then(() => (cancelled ? Promise.reject(new Error('stopped')) : host.start(host.resume, host.safeMode))).then((s) => {
      if (cancelled) { void host.stop(s.id).catch(() => {}); return }
      sessionRef.current = s
      setInfo(s)
      if (s.notices?.length) say(s.notices.join(' '))
      else if (s.safeMode) say('Safe mode: your settings, cheats and patches aren’t being used')
      else if (s.resumed) say('Back where you left off')
      void connect(s, mode)
    }).catch((e) => { if (!cancelled) setError(e?.message || 'The game could not start.') })
    input.start()
    input.onChange = () => {
      const pads = input.listPads()
      setPadName(pads.length ? pads.map((p) => `${p.name}: Player ${p.port + 1}`).join(', ') : '')
    }
    input.onChange()
    return () => {
      cancelled = true
      closing.current = true
      input.stop()
      transportRef.current?.close()
      const s = sessionRef.current
      if (s) void host.stop(s.id).catch(() => {})
    }
  }, [])

  // Tell people what a controller did when it was connected.
  const lastPads = useRef('')
  useEffect(() => {
    if (padName && padName !== lastPads.current) say(padName)
    else if (!padName && lastPads.current) say('Controller disconnected')
    lastPads.current = padName
  }, [padName, say])

  const leaveThen = useCallback((done: () => void) => {
    closing.current = true
    input.stop()
    transportRef.current?.close()
    const s = sessionRef.current
    sessionRef.current = null
    setStatus('Saving…')
    if (s) host.stop(s.id).then(done, done)
    else done()
  }, [host, input])
  const leave = useCallback(() => leaveThen(() => host.close()), [host, leaveThen])

  // Menus stop the game and take the keyboard. So does stepping away, when that's the
  // setting. Pausing by hand leaves the controls alone, so that a frame can be stepped
  // with buttons held.
  const paused = panel !== null
  const pausedNow = paused || frozen || tabHidden
  useEffect(() => { input.setEnabled(!paused && !tabHidden) }, [paused, tabHidden, input])
  useEffect(() => {
    const s = sessionRef.current
    if (s && transport) void host.action(s.id, 'pause', undefined, pausedNow).catch(() => {})
  }, [pausedNow, host, transport])
  useEffect(() => {
    const check = () => setTabHidden(document.hidden && valuesRef.current['emulation.pauseHidden'] === true)
    document.addEventListener('visibilitychange', check)
    return () => document.removeEventListener('visibilitychange', check)
  }, [])

  // The picture: its size and shape, and whatever's between the game and the screen.
  useEffect(() => {
    const el = areaRef.current
    if (!el) return
    const ro = new ResizeObserver(() => setArea({ w: el.clientWidth, h: el.clientHeight }))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  const coreTurns = info?.turns || 0
  const personTurns = Number(values['display.rotate'] || 0) / 90
  const turns = (personTurns - coreTurns + 8) % 4 // a core's are anticlockwise; a person's are clockwise
  turnsRef.current = turns
  const bezel = String(values['display.bezel'] || 'none')
  const layout = useMemo(() => computeLayout({
    areaW: area.w, areaH: area.h, srcW: info?.width || 256, srcH: info?.height || 224, aspect: info?.aspect || 0,
    mode: String(values['display.aspect'] || 'original'), custom: Number(values['display.aspectCustom'] || 0), integer: values['display.integer'] === true,
    cropX: Number(values['display.cropX'] || 0), cropY: Number(values['display.cropY'] || 0), turns, inset: bezel === 'none' ? 0 : Number(values['display.bezelInset'] || 0),
  }), [area, info?.width, info?.height, info?.aspect, values, turns, bezel])
  useEffect(() => {
    if (!pic.current) return
    const e = new DisplayEngine(pic.current)
    engine.current = e
    return () => { e.destroy(); engine.current = null }
  }, [])
  useEffect(() => {
    engine.current?.update(values, { width: info?.width || 256, height: info?.height || 224 }, { picW: layout.picW, picH: layout.picH })
  }, [values, info?.width, info?.height, layout.picW, layout.picH, transport])

  // The video or canvas belongs to the transport; this puts it on the page.
  useEffect(() => {
    const el = pic.current
    if (!el || !transport) return
    const media = transport.element
    media.className = 'wp-media'
    media.setAttribute('aria-label', `${host.title}, playing`)
    el.prepend(media)
    engine.current?.attach(media)
    return () => { media.remove() }
  }, [transport, host.title])
  useEffect(() => {
    const m = transport?.element
    if (!m) return
    // The crop is the picture being larger than the box it's seen through.
    m.style.width = `${layout.scaleX * 100}%`
    m.style.height = `${layout.scaleY * 100}%`
    m.style.left = `${(100 - layout.scaleX * 100) / 2}%`
    m.style.top = `${(100 - layout.scaleY * 100) / 2}%`
  }, [transport, layout.scaleX, layout.scaleY])

  // Sound.
  useEffect(() => {
    if (!transport) return
    transport.setVolume(typeof values['audio.volume'] === 'number' ? values['audio.volume'] as number : 1, values['audio.mute'] === true)
    transport.setAudioBuffer((values['audio.latency'] as AudioBuffer_) || 'lowest')
  }, [transport, values])

  // How fast the game goes. A speed is always sent by a reliable request: a lost "let go
  // of fast-forward" would be a stuck game.
  const wanted = fastForward ? Number(values['emulation.ffSpeed'] || 4) : slow ? Number(values['emulation.slowSpeed'] || 0.5) : 1
  useEffect(() => {
    const s = sessionRef.current
    if (!s || !transport) return
    host.action(s.id, 'speed', undefined, undefined, { value: wanted }).then((r) => setHeldSpeed(r?.speed ?? wanted)).catch(() => {})
  }, [wanted, host, transport])

  // Things a person does while playing, from any kind of input.
  const quickSlot = () => Number(valuesRef.current['states.quickSlot'] || 1)
  const session = () => sessionRef.current
  const act = useCallback(async (type: string, slot?: number, on?: boolean, extra?: { value?: number }) => {
    const s = session()
    if (!s) return null
    return host.action(s.id, type, slot, on, extra)
  }, [host])
  const screenshot = useCallback(async () => {
    const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')
    const name = `${fileName(host.title)} ${stamp}.png`
    const b = await engine.current?.capture()
    if (b) { downloadBlob(b, name); say('Screenshot saved') } else {
      const s = session()
      if (!s) return
      const a = document.createElement('a')
      a.href = host.mediaUrl(`/api/play/${s.id}/screenshot`)
      a.download = name
      a.click()
    }
  }, [host, say])
  const clip = useRef<{ rec: MediaRecorder; timer: number } | null>(null)
  const canClip = typeof MediaRecorder !== 'undefined'
  const toggleClip = useCallback(() => {
    if (clip.current) { clip.current.rec.stop(); return }
    const stream = transportRef.current?.captureStream()
    if (!stream) { say('This way of connecting can’t record'); return }
    const type = ['video/webm;codecs=vp9,opus', 'video/webm;codecs=vp8,opus', 'video/webm', 'video/mp4'].find((t) => MediaRecorder.isTypeSupported(t))
    try {
      const rec = new MediaRecorder(stream, type ? { mimeType: type } : undefined)
      const chunks: Blob[] = []
      rec.ondataavailable = (e) => { if (e.data.size) chunks.push(e.data) }
      rec.onstop = () => {
        const c = clip.current
        clip.current = null
        if (c) window.clearTimeout(c.timer)
        setRecording(false)
        const ext = (type || '').includes('mp4') ? 'mp4' : 'webm'
        if (chunks.length) { downloadBlob(new Blob(chunks, { type: type || 'video/webm' }), `${fileName(host.title)} clip.${ext}`); say('Clip saved') }
      }
      rec.start(1000)
      // Clips stay short: they're kept in this device's memory until they're saved.
      clip.current = { rec, timer: window.setTimeout(() => { try { rec.stop() } catch { /* already stopped */ } }, 60000) }
      setRecording(true)
      say('Recording, for up to a minute')
    } catch { say('This browser can’t record this') }
  }, [host.title, say])

  const fire = useCallback((a: HotAction, down: boolean) => {
    const v = valuesRef.current
    switch (a) {
      case 'fastForward':
        if (v['emulation.ffMode'] === 'toggle') { if (down) setFastForward((x) => !x) } else setFastForward(down)
        break
      case 'slow': setSlow(down); break
      case 'rewind':
        setRewinding(down)
        act('rewind', undefined, down).catch((e) => { setRewinding(false); if (down) say(e?.message || 'Rewind isn’t available') })
        break
      case 'quickSave': act('save', quickSlot()).then(() => say(`Saved to slot ${quickSlot()}`), (e) => say(e?.message || 'That didn’t work')); break
      case 'quickLoad': act('load', quickSlot()).then(() => say(`Loaded slot ${quickSlot()}`), (e) => say(e?.message || 'That didn’t work')); break
      case 'pause': setFrozen((x) => !x); break
      case 'advance':
        if (frozenRef.current) void act('advance', undefined, undefined, { value: 1 }).catch(() => {})
        else setFrozen(true)
        break
      case 'screenshot': void screenshot(); break
      case 'menu': setPanel((p) => (p ? null : 'menu')); break
      case 'mute': settings.set('person', 'audio.mute', v['audio.mute'] !== true); break
      case 'osd': settings.set('person', 'osd.on', v['osd.on'] !== true); break
    }
  }, [act, say, screenshot, settings.set])
  const frozenRef = useRef(frozen)
  frozenRef.current = frozen
  const hot = useMemo(() => new Hotkeys((a, d) => fireRef.current(a, d), (code) => input.isBound(code)), [input])
  const fireRef = useRef(fire)
  fireRef.current = fire
  useEffect(() => { hot.configure(values) }, [hot, values])
  useEffect(() => { hot.enabled = panel === null }, [hot, panel])
  useEffect(() => {
    hot.start()
    input.filter = (port, mask) => (port === 0 ? hot.filter(mask) : mask)
    return () => { hot.stop(); input.filter = undefined }
  }, [hot, input])
  // Nobody can let go of a button in a menu they've gone into.
  useEffect(() => { if (panel) { setFastForward(false); setSlow(false) } }, [panel])

  // Escape, or Start and Select together on a pad, opens the menu.
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      // Escape on a keyboard, and the many names of Back on a TV remote.
      const typing = (e.target as Element | null)?.matches?.('input:not([type=range]):not([type=checkbox]):not([type=color]), textarea')
      if (!['Escape', 'F1', 'GoBack', 'BrowserBack', 'Back', 'XF86Back'].includes(e.key) && !(tv && e.key === 'Backspace' && !typing)) return
      e.preventDefault()
      setPanel((p) => (p === 'touch' ? p : p ? (p === 'menu' ? null : 'menu') : 'menu'))
    }
    window.addEventListener('keydown', key)
    input.onMenu = () => setPanel((x) => (x ? null : 'menu'))
    return () => { window.removeEventListener('keydown', key); input.onMenu = undefined }
  }, [tv, input])

  // A menu's contents change without it closing, so focus has to be put back in by
  // hand, or a remote would have nothing to move from.
  const menuRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (menuRef.current) menuRef.current.scrollTop = 0
    const first = menuRef.current?.querySelector<HTMLElement>('button:not([disabled]), input:not([disabled]), a[href]')
    first?.focus({ preventScroll: true })
  }, [panel])

  const press = (id: number) => { setPanel(null); setTimeout(() => input.pulse(id), 150) }

  const setMode = (m: Mode) => {
    setModeState(m)
    save(MODE_KEY, m === 'auto' ? null : m)
    const s = sessionRef.current
    if (s) void connect(s, m)
  }
  const toggleTouch = () => setTouchOn((v) => { save(TOUCH_KEY, !v); return !v })
  const levels: LevelChoice[] = [
    { level: 'game', label: 'This game' },
    { level: 'console', label: host.consoleName ? `All ${host.consoleName} games` : 'This console' },
    { level: 'person', label: 'All my games' },
  ]
  const fullscreen = () => {
    const el = root.current as any
    if (document.fullscreenElement) void document.exitFullscreen()
    else void (el?.requestFullscreen?.() || el?.webkitRequestFullscreen?.())
  }
  const tap = () => { transport?.unmute(); setNeedsTap(false) }
  const onMask = useCallback((m: number) => input.setTouch(m), [input])
  const onStick = useCallback((x: number, y: number) => input.setTouchStick(x, y), [input])

  // A console's touch screen: a press on the picture is the stylus, wherever it
  // came from. Positions are fractions of the picture, which is all the server wants.
  const hasPointer = !!controls.pad?.pointer
  const onPoint = useCallback((clientX: number, clientY: number, phase: 'down' | 'move' | 'up') => {
    const r = pic.current?.getBoundingClientRect()
    if (!r || r.width === 0 || r.height === 0) return
    // The rectangle already holds the turned shape; undo the turn to reach the game's own axes.
    let x = (clientX - r.left) / r.width, y = (clientY - r.top) / r.height
    const quarter = ((turnsRef.current % 4) + 4) % 4
    if (quarter === 1) [x, y] = [y, 1 - x]
    else if (quarter === 2) [x, y] = [1 - x, 1 - y]
    else if (quarter === 3) [x, y] = [1 - y, x]
    input.setPointer(x, y, phase !== 'up')
  }, [input])
  const stylusDown = useRef(false)
  const stageDown = (e: React.PointerEvent) => {
    if (!hasPointer || paused) return
    e.preventDefault()
    try { e.currentTarget.setPointerCapture(e.pointerId) } catch { /* fine */ }
    stylusDown.current = true
    onPoint(e.clientX, e.clientY, 'down')
  }
  const stageMove = (e: React.PointerEvent) => { if (stylusDown.current) onPoint(e.clientX, e.clientY, 'move') }
  const stageUp = (e: React.PointerEvent) => { if (stylusDown.current) { stylusDown.current = false; onPoint(e.clientX, e.clientY, 'up') } }

  return (
    <div ref={root} className={`wp-root wp-${orientation}${touchOn ? ' wp-with-touch' : ''}${paused ? '' : ' wp-playing'}`}>
      <div ref={areaRef} className="wp-video-area" style={bezel === 'color' ? { background: String(values['display.bezelColor'] || '#000') } : undefined}>
        {bezel === 'image' && !!values['display.bezelImage'] && <img className="wp-bezel-img" src={host.mediaUrl(`/api/game-bezels/${values['display.bezelImage']}`)} alt="" />}
        <div className={'wp-stage' + (hasPointer ? ' wp-has-pointer' : '')} style={{ width: layout.boxW, height: layout.boxH }}
          onPointerDown={stageDown} onPointerMove={stageMove} onPointerUp={stageUp} onPointerCancel={stageUp}>
          <div ref={pic} className="wp-pic" style={{ width: layout.picW, height: layout.picH, transform: `translate(-50%, -50%) rotate(${turns * 90}deg)` }} />
        </div>
        {status && !error && <div className="wp-status" role="status"><span className="wp-spinner" aria-hidden="true" />{status}</div>}
        {error && (
          <div className="wp-error" role="alert">
            <p>{error}</p>
            <div className="wp-row wp-wrap wp-center">
              <button type="button" className="wp-btn primary" onClick={leave}>Back</button>
              {!safe && <button type="button" className="wp-btn" onClick={() => host.relaunch({ resume: false, safeMode: true })}>Start with default settings</button>}
            </div>
          </div>
        )}
      </div>

      {touchOn && transport && !paused && <TouchControls data={controls} orientation={orientation} onMask={onMask} onStick={onStick} onPoint={hasPointer ? onPoint : undefined} />}

      {needsTap && !paused && <button type="button" className="wp-tap" onClick={tap}>Tap for sound</button>}

      <button type="button" className="wp-menu-btn" onClick={() => setPanel('menu')} aria-label="Game menu"><svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"><path fill="currentColor" d="M4 6h16v2H4zm0 5h16v2H4zm0 5h16v2H4z" /></svg></button>

      {values['hotkeys.touchBar'] === true && !paused && (
        <div className="wp-touchbar" role="group" aria-label="Shortcuts">
          {([['rewind', 'Rewind'], ['fastForward', 'Fast-forward'], ['quickSave', 'Quick save'], ['quickLoad', 'Quick load']] as [HotAction, string][]).map(([a, label]) => (
            <button key={a} type="button" className="wp-btn" aria-label={label}
              onPointerDown={(e) => { e.preventDefault(); fire(a, true) }}
              onPointerUp={() => { if (HOLD_ACTIONS.includes(a)) fire(a, false) }}
              onPointerCancel={() => { if (HOLD_ACTIONS.includes(a)) fire(a, false) }}
              onPointerLeave={() => { if (HOLD_ACTIONS.includes(a)) fire(a, false) }}>{label}</button>
          ))}
        </div>
      )}

      {values['osd.on'] === true && <Osd host={host} session={info} transport={transport} values={values} />}

      {!paused && (frozen || rewinding || heldSpeed !== 1 || recording) && (
        <div className="wp-state" role="status">
          {recording && <span>● Recording</span>}
          {rewinding && <span>Rewinding</span>}
          {heldSpeed > 1 && <span>{heldSpeed}× speed</span>}
          {heldSpeed < 1 && <span>{heldSpeed}× speed</span>}
          {frozen && (
            <>
              <span>Paused</span>
              <button type="button" className="wp-btn" onClick={() => fire('advance', true)}>Next frame</button>
              <button type="button" className="wp-btn" onClick={() => setFrozen(false)}>Resume</button>
            </>
          )}
        </div>
      )}

      <div className="wp-toast" role="status" aria-live="polite">{toast}</div>

      {panel === 'touch' && <TouchEditor data={controls} orientation={orientation} onClose={() => setPanel('menu')}
        onSave={(l) => saveControl({ kind: 'touch', key: deviceId(), orientation, data: l })} />}

      {panel === 'controllers' && <ControllerStudio host={host.profiles} data={controls} known={known} input={input} consoleName={host.consoleName} gameId={host.itemId} gameTitle={host.title}
        devices={info?.devices} refresh={refreshControls} onTouch={tv ? undefined : () => setPanel('touch')} onClose={() => setPanel('menu')}
        setDevice={async (port, device) => {
          const s = sessionRef.current
          if (!s) return
          await host.action(s.id, 'device', 0, false, { port, device })
          setInfo((i) => i && { ...i, devices: i.devices?.map((d) => (d.port === port ? { ...d, current: device } : d)) })
        }} />}

      {panel && panel !== 'touch' && panel !== 'controllers' && (
        <div className={'wp-menu-back' + (SIDE_PANELS.includes(panel) ? ' wp-side' : '')} onClick={() => setPanel(null)}>
          <div ref={menuRef} className="wp-menu" role="dialog" aria-modal="true" aria-label={panelTitle(panel, host.title)} onClick={(e) => e.stopPropagation()}>
            {panel === 'menu' && (
              <>
                <h2>{host.title}</h2>
                <nav className="wp-menu-list" aria-label="Game menu">
                  <button type="button" autoFocus className="wp-btn primary" onClick={() => setPanel(null)}>Resume</button>
                  <div className="wp-menu-row">
                    <button type="button" className="wp-btn" onClick={() => { fire('quickSave', true); setPanel(null) }}>Quick save</button>
                    <button type="button" className="wp-btn" onClick={() => { fire('quickLoad', true); setPanel(null) }}>Quick load</button>
                    <button type="button" className="wp-btn" onClick={() => act('undo').then(() => { say('Undone'); setPanel(null) }, (e) => say(e?.message || 'There’s nothing to undo'))}>Undo load</button>
                  </div>
                  <button type="button" className="wp-btn" onClick={() => setPanel('save')}>Save state…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('load')}>Load state…</button>
                  {(info?.discs || 0) > 1 && <button type="button" className="wp-btn" onClick={() => setPanel('discs')}>Change disc…</button>}
                  <div className="wp-menu-row">
                    <button type="button" className="wp-btn" aria-pressed={fastForward} onClick={() => { setPanel(null); setTimeout(() => setFastForward((x) => !x), 50) }}>{fastForward ? 'Normal speed' : 'Fast-forward'}</button>
                    <button type="button" className="wp-btn" aria-pressed={slow} onClick={() => { setPanel(null); setTimeout(() => setSlow((x) => !x), 50) }}>{slow ? 'Normal speed' : 'Slow motion'}</button>
                    <button type="button" className="wp-btn" aria-pressed={frozen} onClick={() => { setFrozen((x) => !x); setPanel(null) }}>{frozen ? 'Unpause' : 'Pause'}</button>
                  </div>
                  <div className="wp-menu-row">
                    <button type="button" className="wp-btn" onClick={() => { setPanel(null); setTimeout(() => void screenshot(), 120) }}>Screenshot</button>
                    {canClip && <button type="button" className="wp-btn" onClick={() => { toggleClip(); setPanel(null) }}>{recording ? 'Stop the clip' : 'Record a clip'}</button>}
                  </div>
                  <h3>Settings</h3>
                  <button type="button" className="wp-btn" onClick={() => setPanel('display')}>Picture…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('osd')}>On-screen numbers…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('emulation')}>Speed and rewind…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('audio')}>Sound…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('states')}>Saves…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('core')}>Core options…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('cheats')}>Cheats…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('patches')}>Patches…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('hotkeys')}>Shortcuts…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('json')}>Custom settings file…</button>
                  <h3>This game</h3>
                  <button type="button" className="wp-btn" onClick={() => press(START)}>Press Start</button>
                  <button type="button" className="wp-btn" onClick={() => press(SELECT)}>Press Select</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('controllers')}>Controllers…</button>
                  {!tv && <button type="button" className="wp-btn" onClick={() => setPanel('touch')}>Edit touch controls…</button>}
                  {!tv && <label className="wp-check"><input type="checkbox" checked={touchOn} onChange={toggleTouch} /> Show touch controls</label>}
                  <button type="button" className="wp-btn" onClick={() => setPanel('connection')}>Connection…</button>
                  <button type="button" className="wp-btn" onClick={fullscreen}>Full screen</button>
                  <button type="button" className="wp-btn" onClick={() => { const s = sessionRef.current; if (s) void host.action(s.id, 'reset').then(() => { say('Reset'); setPanel(null) }) }}>Reset the game</button>
                  {!safe && <button type="button" className="wp-btn" onClick={() => leaveThen(() => host.relaunch({ resume: true, safeMode: true }))}>Restart with default settings</button>}
                  {safe && <button type="button" className="wp-btn" onClick={() => leaveThen(() => host.relaunch({ resume: true, safeMode: false }))}>Restart with my settings</button>}
                  <button type="button" className="wp-btn danger" onClick={leave}>Save and quit</button>
                </nav>
                {!padName && padSupport() === 'insecure' && <p className="wp-muted wp-small">Using a gamepad? Most browsers only show them to secure pages. Open this server over https, or at localhost on this computer.</p>}
                {info && <p className="wp-muted wp-small">{info.core}</p>}
              </>
            )}
            {(panel === 'save' || panel === 'load') && (
              <>
                <h2>{panel === 'save' ? 'Save state' : 'Load state'}</h2>
                <SlotsPanel host={host} schema={schema} itemId={host.itemId} kind={panel} confirm={values['states.confirm'] !== false} core={info?.core} say={say}
                  act={async (n) => { const s = sessionRef.current; if (s) await host.action(s.id, panel, n) }} onDone={(closed) => setPanel(closed ? null : 'menu')} />
              </>
            )}
            {panel && panel in GROUP_TITLES && schema && settings.view && (
              <>
                <h2>{GROUP_TITLES[panel as SettingsGroup]}</h2>
                {safe && <p className="wp-caution" role="note">Safe mode: changes here are kept for later, but this game is running with defaults.</p>}
                <SettingsPanel schema={schema} api={settings} view={settings.view} groups={[panel as string]} level={level} levels={levels} onLevel={setLevel} consoleName={host.consoleName} host={host} say={say}
                  presets={panel === 'display' ? [...schema.presets, ...settings.view.resolved.presets] : undefined} />
                <Back onClick={() => setPanel('menu')} />
              </>
            )}
            {panel === 'core' && settings.view && schema && (
              <>
                <h2>Core options</h2>
                <CorePanel host={host} itemId={host.itemId} api={settings} view={settings.view} level={level} levels={levels} onLevel={setLevel} consoleName={host.consoleName} say={say} onJson={() => setPanel('json')} />
                <Back onClick={() => setPanel('menu')} />
              </>
            )}
            {panel === 'json' && settings.view && (
              <>
                <h2>Custom settings file</h2>
                <JsonEditor host={host} api={settings} view={settings.view} scope={{ item: host.itemId }} level={level} levels={levels} onLevel={setLevel} consoleName={host.consoleName} title={host.title} say={say} itemId={host.itemId} />
                <Back onClick={() => setPanel('menu')} />
              </>
            )}
            {panel === 'cheats' && (
              <>
                <h2>Cheats</h2>
                <CheatsPanel host={host} itemId={host.itemId} title={host.title} say={say} />
                <Back onClick={() => setPanel('menu')} />
              </>
            )}
            {panel === 'patches' && (
              <>
                <h2>Patches</h2>
                <PatchesPanel host={host} itemId={host.itemId} say={say} onRestart={() => leaveThen(() => host.relaunch({ resume: true, safeMode: false }))} />
                <Back onClick={() => setPanel('menu')} />
              </>
            )}
            {panel === 'discs' && (
              <>
                <h2>Change disc</h2>
                <nav className="wp-menu-list" aria-label="Discs">
                  {Array.from({ length: info?.discs || 0 }, (_, n) => (
                    <button key={n} type="button" className={`wp-btn${n === (info?.disc || 0) ? ' primary' : ''}`} onClick={() => {
                      const s = sessionRef.current
                      if (s) void host.action(s.id, 'disc', n).then(() => { say(`Disc ${n + 1}`); setPanel(null) }, (e: any) => say(e?.message || 'That didn’t work'))
                    }}>Disc {n + 1}{n === (info?.disc || 0) ? ' (in the drive)' : ''}</button>
                  ))}
                </nav>
                <div className="wp-row"><span className="wp-grow" /><button type="button" className="wp-btn" onClick={() => setPanel('menu')}>Back</button></div>
              </>
            )}
            {panel === 'connection' && (
              <>
                <h2>Connection</h2>
                <fieldset className="wp-radios">
                  <legend>How the game reaches this browser</legend>
                  {(['auto', 'webrtc', 'ws-h264', 'ws-jpeg'] as Mode[]).map((m) => (
                    <label key={m}><input type="radio" name="wp-mode" checked={mode === m} onChange={() => setMode(m)} /> {m === 'auto' ? 'Automatic (recommended)' : TRANSPORT_LABELS[m as TransportKind]}</label>
                  ))}
                </fieldset>
                <p className="wp-muted wp-small">Now using: {transport ? TRANSPORT_LABELS[transport.kind] : 'nothing yet'}</p>
                <label className="wp-check"><input type="checkbox" checked={values['osd.on'] === true} onChange={(e) => settings.set('person', 'osd.on', e.target.checked)} /> Show connection numbers while playing</label>
                <div className="wp-row"><span className="wp-grow" /><button type="button" className="wp-btn" onClick={() => setPanel('menu')}>Back</button></div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

// The same screen away from a game, for setting things up in peace. It reads controllers
// but sends them nowhere.
export function ControllersPage({ profiles, controls, controllers, consoles, consoleId, onConsole, saveControl, onClose }: {
  profiles: ProfileHost; controls: (consoleId: string, device: string) => Promise<ControlsData>; controllers: () => Promise<ControllerData>; consoles: { id: string; short: string; name: string }[]
  consoleId: string; onConsole: (id: string) => void; saveControl: (consoleId: string, s: SaveControl) => Promise<void>; onClose: () => void
}) {
  const [data, setData] = useState<ControlsData | null>(null)
  const [known, setKnown] = useState<ControllerData | null>(null)
  const [error, setError] = useState('')
  const [touch, setTouch] = useState(false)
  const orientation = useOrientation()
  const fetchData = useCallback(() => controls(consoleId, deviceId()), [controls, consoleId])
  useEffect(() => {
    let stale = false
    Promise.all([fetchData(), controllers().catch(() => ({ layouts: {}, pads: [] }))]).then(([c, k]) => { if (!stale) { setKnown(k); setData(c) } }).catch((e) => setError(e?.message || 'This couldn’t be loaded'))
    return () => { stale = true }
  }, [fetchData, controllers])
  const input = useMemo(() => (data && known ? new InputManager(data, known) : null), [data?.console, known]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!input) return
    input.setEnabled(false) // no keys taken, and nothing sent: this is only for reading pads
    input.start()
    return () => input.stop()
  }, [input])
  const refresh = useCallback(async () => {
    const fresh = await fetchData()
    setData(fresh)
    input?.setControls(fresh)
  }, [fetchData, input])
  // Escape, and the many names of Back on a TV remote.
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (touch || !['Escape', 'GoBack', 'BrowserBack', 'Back', 'XF86Back'].includes(e.key)) return
      e.preventDefault()
      onClose()
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [touch, onClose])
  if (error) return <div className="wp-page"><div className="wp-error" role="alert"><p>{error}</p><button type="button" className="wp-btn primary" onClick={onClose}>Back</button></div></div>
  if (!data || !input) return <div className="wp-page"><div className="wp-status" role="status"><span className="wp-spinner" aria-hidden="true" />Loading…</div></div>
  const current = consoles.find((c) => c.id === consoleId)
  return (
    <div className="wp-page">
      {touch
        ? <TouchEditor data={data} orientation={orientation} onClose={() => setTouch(false)} onSave={async (l) => { await saveControl(consoleId, { kind: 'touch', key: deviceId(), orientation, data: l }); await refresh() }} />
        : <ControllerStudio key={data.console} host={profiles} data={data} known={known} input={input} consoleName={current?.short || consoleId} refresh={refresh} onClose={onClose} onTouch={onTV() ? undefined : () => setTouch(true)}
            consolePicker={(
              <div className="wps-consoles" role="group" aria-label="Which console">
                {consoles.map((c) => <button key={c.id} type="button" aria-pressed={c.id === consoleId} className={'wp-btn wps-small' + (c.id === consoleId ? ' primary' : '')} onClick={() => onConsole(c.id)}>{c.short}</button>)}
              </div>
            )} />}
    </div>
  )
}

const panelTitle = (p: Panel, title: string) => ({ menu: title, save: 'Save state', load: 'Load state', controllers: 'Controllers', connection: 'Connection', touch: 'Touch controls', discs: 'Change disc', core: 'Core options', cheats: 'Cheats', patches: 'Patches', json: 'Custom settings file', ...GROUP_TITLES } as Record<string, string>)[p as string]

const Back = ({ onClick }: { onClick: () => void }) => <div className="wp-row"><span className="wp-grow" /><button type="button" className="wp-btn" onClick={onClick}>Back</button></div>
