import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { SELECT, START, type ControlsData, type SaveControl } from './consoles'
import { ControllersPanel } from './Controllers'
import { deviceId, InputManager, load, onTV, save } from './input'
import { TouchControls, TouchEditor, useOrientation } from './TouchControls'
import { make, plan, TRANSPORT_LABELS, type Mode, type PlayEvent, type Stats, type Transport, type TransportKind } from './transport'
import './play.css'

// The player: one game, full window. The server does the running; this shows it
// and passes controllers along. Everything it needs from the app comes through
// PlayHost, so the module works in any app that can provide one.

export type SessionInfo = { id: string; itemId: number; title: string; console: string; core: string; width: number; height: number; aspect: number; fps: number; sampleRate: number; resumed: boolean; encoder?: string; renderer?: string; modes: { id: string }[] }
export type SaveSlot = { slot: number; updatedAt: number; size: number; hasThumb: boolean }

export interface PlayHost {
  itemId: number
  title: string
  consoleId: string
  resume: boolean
  start(resume: boolean): Promise<SessionInfo>
  stop(sessionId: string): Promise<void>
  answer(sessionId: string, sdp: string, prefer?: string): Promise<string>
  wsUrl(sessionId: string, video: string): string
  action(sessionId: string, type: string, slot?: number, on?: boolean): Promise<void>
  saves(): Promise<SaveSlot[]>
  thumbUrl(slot: number, version: number): string
  controls(device: string): Promise<ControlsData>
  saveControl(s: SaveControl): Promise<void>
  close(): void // leave the player
}

type Panel = null | 'menu' | 'save' | 'load' | 'controllers' | 'connection' | 'touch'

const MODE_KEY = 'wittyplay.mode'
const TOUCH_KEY = 'wittyplay.touch'

export default function GamePlayer({ host }: { host: PlayHost }) {
  const [controls, setControls] = useState<ControlsData | null>(null)
  const [error, setError] = useState('')
  useEffect(() => { host.controls(deviceId()).then(setControls).catch((e) => setError(e?.message || 'The game could not start.')) }, [host])
  if (error) {
    return (
      <div className="wp-root"><div className="wp-error" role="alert"><p>{error}</p><button type="button" className="wp-btn primary" onClick={() => host.close()}>Back</button></div></div>
    )
  }
  return controls ? <Playing host={host} initial={controls} /> : <div className="wp-root"><div className="wp-status" role="status"><span className="wp-spinner" aria-hidden="true" />Starting the game…</div></div>
}

function Playing({ host, initial }: { host: PlayHost; initial: ControlsData }) {
  const [controls, setControls] = useState(initial)
  const [info, setInfo] = useState<SessionInfo | null>(null)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('Starting the game…')
  const [transport, setTransport] = useState<Transport | null>(null)
  const [panel, setPanel] = useState<Panel>(null)
  const [needsTap, setNeedsTap] = useState(false)
  const [stats, setStats] = useState<Stats>({})
  const [showStats, setShowStats] = useState(false)
  const [toast, setToast] = useState('')
  const [mode, setModeState] = useState<Mode>(() => load<Mode>(MODE_KEY) || 'auto')
  const [touchOn, setTouchOn] = useState<boolean>(() => load<boolean>(TOUCH_KEY) ?? (window.matchMedia?.('(pointer: coarse)').matches ?? false))
  const [padName, setPadName] = useState('')
  const orientation = useOrientation()
  const input = useMemo(() => new InputManager(initial), [initial])
  const tv = useMemo(onTV, [])
  // Layouts live on the server, so they follow the account to its other devices.
  const saveControl = useCallback(async (s: SaveControl) => {
    await host.saveControl(s)
    const fresh = await host.controls(deviceId())
    setControls(fresh)
    input.setControls(fresh)
  }, [host, input])
  const stage = useRef<HTMLDivElement>(null)
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

  const onEvent = useCallback((e: PlayEvent) => {
    if (e.type === 'rumble') input.rumble(e.port || 0, e.strength || 0)
    else if (e.type === 'message' && e.text) say(e.text)
    else if (e.type === 'shutdown') { say('The game has ended'); leave() }
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
    host.start(host.resume).then((s) => {
      if (cancelled) { void host.stop(s.id).catch(() => {}); return }
      sessionRef.current = s
      setInfo(s)
      if (s.resumed) say('Back where you left off')
      void connect(s, mode)
    }).catch((e) => setError(e?.message || 'The game could not start.'))
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
    lastPads.current = padName
  }, [padName, say])

  const leave = useCallback(() => {
    closing.current = true
    input.stop()
    transportRef.current?.close()
    const s = sessionRef.current
    sessionRef.current = null
    setStatus('Saving…')
    const done = () => host.close()
    if (s) host.stop(s.id).then(done, done)
    else done()
  }, [host, input])

  // Menus stop the game and take the keyboard.
  const paused = panel !== null
  useEffect(() => {
    input.setEnabled(!paused)
    const s = sessionRef.current
    if (s && transport) void host.action(s.id, 'pause', undefined, paused).catch(() => {})
  }, [paused, input, host, transport])

  // The video or canvas belongs to the transport; this puts it on the page.
  useEffect(() => {
    const el = stage.current
    if (!el || !transport) return
    const media = transport.element
    media.className = 'wp-media'
    media.setAttribute('aria-label', `${host.title}, playing`)
    el.appendChild(media)
    return () => { media.remove() }
  }, [transport, host.title])

  useEffect(() => {
    if (!showStats || !transport) return
    const iv = setInterval(() => { transport.stats().then(setStats).catch(() => {}) }, 1000)
    return () => clearInterval(iv)
  }, [showStats, transport])

  // Escape, or Start and Select together on a pad, opens the menu.
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      // Escape on a keyboard, and the many names of Back on a TV remote.
      if (!['Escape', 'F1', 'GoBack', 'BrowserBack', 'Back', 'XF86Back'].includes(e.key) && !(tv && e.key === 'Backspace')) return
      e.preventDefault()
      setPanel((p) => (p === 'touch' ? p : p ? (p === 'menu' ? null : 'menu') : 'menu'))
    }
    window.addEventListener('keydown', key)
    let held = false
    const iv = setInterval(() => {
      let both = false
      try {
        for (const p of navigator.getGamepads?.() || []) {
          if (p && ((p.buttons[8]?.pressed && p.buttons[9]?.pressed) || p.buttons[16]?.pressed)) both = true
        }
      } catch { /* blocked by policy */ }
      if (both && !held) setPanel((x) => (x ? null : 'menu'))
      held = both
    }, 100)
    return () => { window.removeEventListener('keydown', key); clearInterval(iv) }
  }, [tv])

  // A menu's contents change without it closing, so focus has to be put back in by
  // hand, or a remote would have nothing to move from.
  const menuRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
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
  const fullscreen = () => {
    const el = root.current as any
    if (document.fullscreenElement) void document.exitFullscreen()
    else void (el?.requestFullscreen?.() || el?.webkitRequestFullscreen?.())
  }
  const tap = () => { transport?.unmute(); setNeedsTap(false) }
  const aspect = info?.aspect && info.aspect > 0 ? info.aspect : info ? info.width / info.height : 4 / 3
  const onMask = useCallback((m: number) => input.setTouch(m), [input])

  // A console's touch screen: a press on the picture is the stylus, wherever it
  // came from. Positions are fractions of the picture, which is all the server wants.
  const hasPointer = !!controls.pad?.pointer
  const onPoint = useCallback((clientX: number, clientY: number, phase: 'down' | 'move' | 'up') => {
    const r = stage.current?.getBoundingClientRect()
    if (!r || r.width === 0 || r.height === 0) return
    input.setPointer((clientX - r.left) / r.width, (clientY - r.top) / r.height, phase !== 'up')
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
      <div className="wp-video-area">
        <div ref={stage} className={'wp-stage' + (hasPointer ? ' wp-has-pointer' : '')} style={{ aspectRatio: String(aspect) }}
          onPointerDown={stageDown} onPointerMove={stageMove} onPointerUp={stageUp} onPointerCancel={stageUp} />
        {status && !error && <div className="wp-status" role="status"><span className="wp-spinner" aria-hidden="true" />{status}</div>}
        {error && (
          <div className="wp-error" role="alert">
            <p>{error}</p>
            <button type="button" className="wp-btn primary" onClick={leave}>Back</button>
          </div>
        )}
      </div>

      {touchOn && transport && !paused && <TouchControls data={controls} orientation={orientation} onMask={onMask} onPoint={hasPointer ? onPoint : undefined} />}

      {needsTap && !paused && <button type="button" className="wp-tap" onClick={tap}>Tap for sound</button>}

      <button type="button" className="wp-menu-btn" onClick={() => setPanel('menu')} aria-label="Game menu"><svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"><path fill="currentColor" d="M4 6h16v2H4zm0 5h16v2H4zm0 5h16v2H4z" /></svg></button>

      {showStats && (
        <div className="wp-stats" aria-hidden="true">
          {transport && <b>{transport.kind}</b>} {info?.encoder && <span>{info.encoder}</span>} {info?.renderer && <span>{info.renderer}</span>}
          {stats.codec && <span>{stats.codec}</span>}
          {stats.fps != null && <span>{stats.fps} fps</span>}
          {stats.kbps != null && <span>{stats.kbps} kbps</span>}
          {stats.rttMs != null && <span>{stats.rttMs} ms round trip</span>}
          {stats.decodeMs != null && <span>{stats.decodeMs} ms decode</span>}
          {stats.jitterMs != null && <span>{stats.jitterMs} ms jitter</span>}
          {stats.lost != null && stats.lost > 0 && <span>{stats.lost} lost</span>}
        </div>
      )}

      <div className="wp-toast" role="status" aria-live="polite">{toast}</div>

      {panel === 'touch' && <TouchEditor data={controls} orientation={orientation} onClose={() => setPanel('menu')}
        onSave={(l) => saveControl({ kind: 'touch', key: deviceId(), orientation, data: l })} />}

      {panel && panel !== 'touch' && (
        <div className="wp-menu-back" onClick={() => setPanel(null)}>
          <div ref={menuRef} className="wp-menu" role="dialog" aria-modal="true" aria-label={panelTitle(panel, host.title)} onClick={(e) => e.stopPropagation()}>
            {panel === 'menu' && (
              <>
                <h2>{host.title}</h2>
                <nav className="wp-menu-list" aria-label="Game menu">
                  <button type="button" autoFocus className="wp-btn primary" onClick={() => setPanel(null)}>Resume</button>
                  <button type="button" className="wp-btn" onClick={() => press(START)}>Press Start</button>
                  <button type="button" className="wp-btn" onClick={() => press(SELECT)}>Press Select</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('save')}>Save state…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('load')}>Load state…</button>
                  <button type="button" className="wp-btn" onClick={() => setPanel('controllers')}>Controllers…</button>
                  {!tv && <button type="button" className="wp-btn" onClick={() => setPanel('touch')}>Edit touch controls…</button>}
                  {!tv && <label className="wp-check"><input type="checkbox" checked={touchOn} onChange={toggleTouch} /> Show touch controls</label>}
                  <button type="button" className="wp-btn" onClick={() => setPanel('connection')}>Connection…</button>
                  <button type="button" className="wp-btn" onClick={fullscreen}>Full screen</button>
                  <button type="button" className="wp-btn" onClick={() => { const s = sessionRef.current; if (s) void host.action(s.id, 'reset').then(() => { say('Reset'); setPanel(null) }) }}>Reset the game</button>
                  <button type="button" className="wp-btn danger" onClick={leave}>Save and quit</button>
                </nav>
                {info && <p className="wp-muted wp-small">{info.core}</p>}
              </>
            )}
            {(panel === 'save' || panel === 'load') && <SlotsPanel host={host} kind={panel} session={sessionRef.current} say={say} onDone={(closed) => setPanel(closed ? null : 'menu')} />}
            {panel === 'controllers' && (
              <>
                <h2>Controllers</h2>
                <ControllersPanel input={input} controls={controls} onSave={saveControl} say={say} />
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
                <label className="wp-check"><input type="checkbox" checked={showStats} onChange={(e) => setShowStats(e.target.checked)} /> Show connection numbers while playing</label>
                <div className="wp-row"><span className="wp-grow" /><button type="button" className="wp-btn" onClick={() => setPanel('menu')}>Back</button></div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

const panelTitle = (p: Panel, title: string) => ({ menu: title, save: 'Save state', load: 'Load state', controllers: 'Controllers', connection: 'Connection', touch: 'Touch controls' } as Record<string, string>)[p as string]

function SlotsPanel({ host, kind, session, say, onDone }: { host: PlayHost; kind: 'save' | 'load'; session: SessionInfo | null; say: (t: string) => void; onDone: (closed: boolean) => void }) {
  const [slots, setSlots] = useState<SaveSlot[] | null>(null)
  const [busy, setBusy] = useState(false)
  useEffect(() => { host.saves().then(setSlots).catch(() => setSlots([])) }, [host])
  const act = async (n: number) => {
    if (!session) return
    setBusy(true)
    try {
      await host.action(session.id, kind, n)
      say(kind === 'save' ? `Saved to slot ${n}` : n === 0 ? 'Loaded the automatic save' : `Loaded slot ${n}`)
      onDone(true)
    } catch (e: any) { say(e?.message || 'That didn’t work') } finally { setBusy(false) }
  }
  const nums = kind === 'load' ? [0, 1, 2, 3, 4, 5, 6, 7, 8, 9] : [1, 2, 3, 4, 5, 6, 7, 8, 9]
  return (
    <>
      <h2>{kind === 'save' ? 'Save state' : 'Load state'}</h2>
      <div className="wp-slots">
        {nums.map((n) => {
          const s = slots?.find((x) => x.slot === n)
          if (kind === 'load' && !s) return null
          const name = n === 0 ? 'Automatic' : `Slot ${n}`
          const when = s ? new Date(s.updatedAt * 1000).toLocaleString() : 'Empty'
          return (
            <button key={n} type="button" className="wp-slot" disabled={busy} onClick={() => act(n)} aria-label={`${name}: ${when}`}>
              {s?.hasThumb ? <img src={host.thumbUrl(n, s.updatedAt)} alt="" /> : <span className="wp-slot-empty" />}
              <b>{name}</b><small className="wp-muted">{when}</small>
            </button>
          )
        })}
        {kind === 'load' && slots && slots.length === 0 && <p className="wp-muted">No saved states yet.</p>}
      </div>
      <div className="wp-row"><span className="wp-grow" /><button type="button" className="wp-btn" onClick={() => onDone(false)}>Back</button></div>
    </>
  )
}
