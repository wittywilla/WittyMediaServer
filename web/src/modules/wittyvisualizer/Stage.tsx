import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { CYCLE_CHOICES, SCALE_CHOICES } from './engine.ts'
import { MAX_USER_BYTES } from './format.ts'
import type { VisualizerHost } from './host.ts'
import { Session, type Place } from './session.ts'
import './visualizer.css'

// What people see: the picture, its controls, and the notice before the first use. The
// canvas belongs to the session, so it can move between the player's panel, its thumbnail
// and the whole screen without starting over.

let session: Session | null = null
let sessionOwner: StageProps['owner']
let mounted = 0
const getSession = (host: VisualizerHost, owner: StageProps['owner']) => {
  if (session && owner !== sessionOwner) {
    session.discard()
    session = null
  }
  sessionOwner = owner
  return (session ??= new Session(host))
}
export const currentSession = () => session

const Svg = ({ d }: { d: string }) => <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true"><path d={d} /></svg>
const ICON = {
  prev: 'M6 6h2v12H6zm3.5 6l8.5 6V6z',
  next: 'M6 18l8.5-6L6 6zm10-12v12h2V6z',
  starOn: 'M12 2.6l2.9 6.3 6.9.8-5.1 4.7 1.4 6.8L12 17.8l-6.1 3.4 1.4-6.8L2.2 9.7l6.9-.8z',
  starOff: 'M12 2.6l2.9 6.3 6.9.8-5.1 4.7 1.4 6.8L12 17.8l-6.1 3.4 1.4-6.8L2.2 9.7l6.9-.8zm0 3.7l-1.6 3.5-3.8.5 2.8 2.6-.8 3.8 3.4-1.9 3.4 1.9-.8-3.8 2.8-2.6-3.8-.5z',
  full: 'M7 14H5v5h5v-2H7zm-2-4h2V7h3V5H5zm12 7h-3v2h5v-5h-2zm-3-12v2h3v3h2V5z',
  shrink: 'M5 16h3v3h2v-5H5zm3-8H5v2h5V5H8zm6 11h2v-3h3v-2h-5zm2-11V5h-2v5h5V8z',
  gear: 'M19.4 13a7.9 7.9 0 000-2l2.1-1.6-2-3.5-2.5 1a8 8 0 00-1.7-1L15 3H9l-.3 2.9a8 8 0 00-1.7 1l-2.5-1-2 3.5L4.6 11a7.9 7.9 0 000 2l-2.1 1.6 2 3.5 2.5-1a8 8 0 001.7 1L9 21h6l.3-2.9a8 8 0 001.7-1l2.5 1 2-3.5zM12 15.5a3.5 3.5 0 110-7 3.5 3.5 0 010 7z',
  close: 'M19 6.4L17.6 5 12 10.6 6.4 5 5 6.4l5.6 5.6L5 17.6 6.4 19l5.6-5.6 5.6 5.6 1.4-1.4-5.6-5.6z',
}

function IconButton({ icon, label, onClick, pressed }: { icon: string; label: string; onClick: () => void; pressed?: boolean }) {
  return <button type="button" className="wv-icon" aria-label={label} title={label} aria-pressed={pressed} onClick={onClick}><Svg d={icon} /></button>
}

export type StageProps = {
  host: VisualizerHost
  // Who this is for, on an app with several people. Settings, and having read the flashing
  // notice, are each person's own, and the page can outlive a change of person.
  owner?: string | number
  // Where the sound comes from: the media element that's playing.
  media: () => HTMLMediaElement | null
  place: Place
  onPlace?: (p: Place) => void
  onClose: () => void
}

export function VisualizerStage({ host, owner, media, place, onPlace, onClose }: StageProps) {
  const s = getSession(host, owner)
  const [, force] = useState(0)
  const rerender = useCallback(() => force((n) => n + 1), [])
  useSyncExternalStore(s.subscribe, () => s.status)
  useEffect(() => s.subscribe(rerender), [s, rerender])
  useEffect(() => { void s.load() }, [s])

  // Someone may move between places (one stage goes, the next arrives in the same moment),
  // so a stop only happens when none has come back by the next task.
  useEffect(() => {
    mounted++
    return () => {
      mounted--
      setTimeout(() => { if (mounted === 0) s.stop() }, 0)
    }
  }, [s])

  const accepted = s.loaded && s.settings.notice
  useEffect(() => { if (accepted) s.start(media) }, [s, accepted, media])

  if (!s.loaded) return null
  if (!accepted) {
    if (place === 'mini') return null
    // On the page itself: where it was asked for may be a moving or clipped part of the player.
    return createPortal(<FlashNotice session={s} onDecline={onClose} />, document.body)
  }
  return place === 'mini'
    ? <Mini session={s} />
    : <Full session={s} place={place} onPlace={onPlace} onClose={onClose} />
}

function FlashNotice({ session: s, onDecline }: { session: Session; onDecline: () => void }) {
  const [calm, setCalm] = useState(s.calm)
  return (
    <div className="wv-dialog-back">
      <div className="wv-dialog" role="alertdialog" aria-modal="true" aria-labelledby="wv-notice-title" aria-describedby="wv-notice-text"
        onKeyDown={(e) => { if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack') { e.preventDefault(); e.stopPropagation(); onDecline() } }}>
        <h2 id="wv-notice-title">Before you turn this on</h2>
        <div id="wv-notice-text">
          <p>The visualizer fills the screen with moving, pulsing light that follows the music.</p>
          <p><strong>Flashing and fast-moving patterns can trigger seizures in people with photosensitive epilepsy</strong>, and can make other people feel unwell. It limits flashes by design, but that can't make it safe for everyone.</p>
          <p>If you, or anyone who might be watching, is sensitive to this, leave it off. If you feel unwell, look away and turn it off.</p>
        </div>
        <label className="wv-check">
          <input type="checkbox" checked={calm} onChange={(e) => setCalm(e.target.checked)} />
          <span>Use calm mode: slower, softer, and only the gentle effects</span>
        </label>
        <div className="wv-dialog-actions">
          <button type="button" className="wv-btn" autoFocus onClick={onDecline}>Not now</button>
          <button type="button" className="wv-btn primary" onClick={() => { s.update({ notice: true, on: true, calm: calm ? true : s.settings.calm === null && !s.calm ? null : false }); void s.unlock() }}>Turn on</button>
        </div>
      </div>
    </div>
  )
}

function Mini({ session: s }: { session: Session }) {
  const box = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => { if (box.current) s.attach(box.current, 'mini') }, [s])
  return <div className="wv-mini" ref={box} aria-hidden="true" />
}

function Full({ session: s, place, onPlace, onClose }: { session: Session; place: Place; onPlace?: (p: Place) => void; onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null)
  const root = useRef<HTMLDivElement>(null)
  const bar = useRef<HTMLDivElement>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [idle, setIdle] = useState(false)
  const [said, setSaid] = useState('')
  const idleTimer = useRef(0)
  const whole = place === 'full'
  const set = s.settings, st = s.status
  const effect = s.effects.find((e) => e.id === st.effect)

  useLayoutEffect(() => { if (box.current) s.attach(box.current, place) }, [s, place])

  // The controls step aside after a few seconds on the whole screen, and come back for
  // any movement, key or focus.
  const wake = useCallback(() => {
    setIdle(false)
    clearTimeout(idleTimer.current)
    idleTimer.current = window.setTimeout(() => {
      if (root.current?.querySelector('.wv-bar:focus-within, .wv-dialog-back')) return
      setIdle(true)
    }, 3500)
  }, [])
  useEffect(() => { wake(); return () => clearTimeout(idleTimer.current) }, [wake, whole])

  // The browser's own full screen, with the fixed overlay as the fallback where it has none.
  useEffect(() => {
    const el = root.current
    if (!whole || !el) return
    el.focus({ preventScroll: true })
    let asked = false
    if (el.requestFullscreen) el.requestFullscreen({ navigationUI: 'hide' }).then(() => { asked = true }).catch(() => {})
    const left = () => { if (asked && !document.fullscreenElement) onPlace?.('panel') }
    document.addEventListener('fullscreenchange', left)
    return () => {
      document.removeEventListener('fullscreenchange', left)
      if (document.fullscreenElement === el) void document.exitFullscreen().catch(() => {})
    }
  }, [whole, onPlace])

  const announce = (id: string) => setTimeout(() => setSaid(s.effects.find((e) => e.id === id)?.name ?? ''), 0)
  const goNext = () => { s.next(); announce(s.status.effect) }
  const goPrev = () => { s.previous(); announce(s.status.effect) }

  // Keys: on the whole screen they're always here. In the panel, when the focus is here.
  // Arrows on a control belong to the app's spatial navigation, so a remote can reach
  // every control; on the picture itself left and right change the effect.
  const onKey = useCallback((e: KeyboardEvent) => {
    const r = root.current
    if (!r) return
    const t = e.target as HTMLElement | null
    const onControl = !!t && t !== r && r.contains(t) && t.tagName !== 'CANVAS'
    const typing = t?.tagName === 'INPUT' && (t as HTMLInputElement).type === 'range'
    if (!whole && !(t && r.contains(t))) return
    if (t && t.closest('select') && e.key.startsWith('Arrow')) return
    if (e.ctrlKey || e.metaKey || e.altKey) return
    const handled = () => { e.preventDefault(); e.stopImmediatePropagation(); wake() }
    switch (e.key) {
      case 'Escape': case 'GoBack': case 'BrowserBack': case 'Backspace':
        if (settingsOpen) { handled(); setSettingsOpen(false); return }
        if (whole) { handled(); if (document.fullscreenElement) void document.exitFullscreen().catch(() => {}); onPlace?.('panel'); return }
        return
      case 'ArrowLeft': if (!onControl) { handled(); goPrev() } return
      case 'ArrowRight': if (!onControl) { handled(); goNext() } return
      case 'ArrowUp': case 'ArrowDown': case 'Enter':
        if (!onControl) { handled(); bar.current?.querySelector<HTMLElement>('button')?.focus() }
        return
    }
    if (typing || settingsOpen) return
    switch (e.key.toLowerCase()) {
      case 'n': handled(); goNext(); return
      case 'p': handled(); goPrev(); return
      case 'r': handled(); s.update({ random: !s.settings.random }); return
      case 'f': handled(); onPlace?.(whole ? 'panel' : 'full'); return
      case 's': handled(); if (s.status.effect) s.toggleFavourite(s.status.effect); return
      case 'v': handled(); onClose(); return
      case '+': case '=': handled(); s.update({ intensity: s.settings.intensity + 0.1 }); return
      case '-': handled(); s.update({ intensity: s.settings.intensity - 0.1 }); return
    }
  }, [whole, settingsOpen, onPlace, onClose, s, wake]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onKey])

  const favourite = !!st.effect && set.favourites.includes(st.effect)
  const stage = (
    <div ref={root} className={'wv-stage ' + (whole ? 'whole' : 'panel') + (idle && whole ? ' idle' : '')} tabIndex={-1} role="region" aria-label="Visualizer"
      data-effect={st.effect} data-fps={st.fps} data-width={st.width} data-height={st.height} data-scale={st.scale} data-renderer={st.renderer} data-gpu={st.rendererDetail} data-tap={st.tap} data-frame-ms={st.frameMs} data-beats={st.beats}
      onPointerMove={wake} onFocusCapture={wake}>
      <div className="wv-picture" ref={box} onClick={() => { if (whole) setIdle(!idle); else onPlace?.('full') }} />
      <div className="wv-sr" aria-live="polite">{said && `Now showing ${said}`}</div>

      {st.tap === 'needs-gesture' && (
        <button type="button" className="wv-start" onClick={() => void s.unlock()}>Start listening</button>
      )}
      {(st.tapWhy || st.problem) && <div className="wv-note" role="status">{st.problem || st.tapWhy}</div>}
      {set.stats && <div className="wv-stats">{st.fps} fps · {st.width}×{st.height} · {Math.round(st.scale * 100)}% · {st.frameMs} ms · {st.renderer === 'canvas' ? 'Plain canvas' : st.rendererDetail}</div>}

      <div className="wv-bar" ref={bar} role="toolbar" aria-label="Visualizer controls">
        <IconButton icon={ICON.prev} label="Previous effect" onClick={goPrev} />
        <select className="wv-select" aria-label="Effect" value={st.effect} onChange={(e) => { s.select(e.target.value); announce(e.target.value) }}>
          {s.effects.map((e) => <option key={e.id} value={e.id}>{e.name}{e.builtin ? '' : ' (yours)'}</option>)}
        </select>
        <IconButton icon={ICON.next} label="Next effect" onClick={goNext} />
        <IconButton icon={favourite ? ICON.starOn : ICON.starOff} label={effect ? `Favourite: ${effect.name}` : 'Favourite'} pressed={favourite} onClick={() => st.effect && s.toggleFavourite(st.effect)} />
        <span className="wv-grow" />
        <IconButton icon={ICON.gear} label="Visualizer settings" onClick={() => setSettingsOpen(true)} />
        <IconButton icon={whole ? ICON.shrink : ICON.full} label={whole ? 'Leave full screen' : 'Full screen'} onClick={() => onPlace?.(whole ? 'panel' : 'full')} />
        <IconButton icon={ICON.close} label="Turn the visualizer off" onClick={onClose} />
      </div>

      {settingsOpen && whole && <SettingsDialog session={s} onClose={() => setSettingsOpen(false)} />}
      {settingsOpen && !whole && createPortal(<SettingsDialog session={s} onClose={() => setSettingsOpen(false)} />, document.body)}
    </div>
  )
  return whole ? createPortal(stage, document.body) : stage
}

function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  const id = useId()
  return (
    <div className="wv-row">
      <label htmlFor={id}>{label}{hint && <small>{hint}</small>}</label>
      <span data-for={id}>{children}</span>
    </div>
  )
}

function Slider({ label, value, min, max, onChange }: { label: string; value: number; min: number; max: number; onChange: (v: number) => void }) {
  const pct = Math.round(value * 100)
  return (
    <label className="wv-slider">
      <span>{label}<b>{pct}%</b></span>
      <input className="range" type="range" min={min} max={max} step={5} value={pct} aria-label={label} aria-valuetext={`${pct}%`}
        style={{ '--pct': ((pct - min) / (max - min)) * 100 + '%' } as React.CSSProperties} onChange={(e) => onChange(Number(e.target.value) / 100)} />
    </label>
  )
}

const everyLabel = (sec: number) => (sec === 0 ? 'Stay on one effect' : sec < 60 ? `Every ${sec} seconds` : sec === 60 ? 'Every minute' : `Every ${sec / 60} minutes`)
const scaleLabel = (v: number) => (v === 0 ? 'Automatic (recommended)' : `${Math.round(v * 100)}%`)

function SettingsDialog({ session: s, onClose }: { session: Session; onClose: () => void }) {
  const set = s.settings
  const [adding, setAdding] = useState(false)
  const [addMsg, setAddMsg] = useState('')
  const custom = s.effects.filter((e) => !e.builtin)
  const onFile = async (file: File | undefined) => {
    if (!file) return
    if (file.size > MAX_USER_BYTES) { setAddMsg(`That file is too large. An effect can be up to ${MAX_USER_BYTES / 1024} KB.`); return }
    setAdding(true)
    setAddMsg('')
    const why = await s.addCustom(await file.text()).catch((e) => String(e))
    setAdding(false)
    setAddMsg(why || 'Added. You\'ll find it in the list of effects.')
  }
  return (
    <div className="wv-dialog-back" onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
      onKeyDown={(e) => { if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack') { e.preventDefault(); e.stopPropagation(); onClose() } }}>
      <div className="wv-dialog wide" role="dialog" aria-modal="true" aria-labelledby="wv-settings-title">
        <div className="wv-dialog-head">
          <h2 id="wv-settings-title">Visualizer</h2>
          <IconButton icon={ICON.close} label="Close settings" onClick={onClose} />
        </div>

        <Row label="Change effect">
          <select className="wv-select" value={set.cycleSec} aria-label="Change effect" onChange={(e) => s.update({ cycleSec: Number(e.target.value) })}>
            {CYCLE_CHOICES.map((c) => <option key={c} value={c}>{everyLabel(c)}</option>)}
          </select>
        </Row>
        <label className="wv-check"><input type="checkbox" checked={set.random} onChange={(e) => s.update({ random: e.target.checked })} /><span>Random order</span></label>
        <label className="wv-check"><input type="checkbox" checked={set.favouritesOnly} onChange={(e) => s.update({ favouritesOnly: e.target.checked })} /><span>Only my favourites{set.favourites.length ? ` (${set.favourites.length})` : ' (mark some with the star)'}</span></label>

        <Slider label="How strongly it reacts" value={set.intensity} min={30} max={150} onChange={(v) => s.update({ intensity: v })} />
        <Slider label="How easily it hears the music" value={set.sensitivity} min={25} max={200} onChange={(v) => s.update({ sensitivity: v })} />

        <Row label="Calm mode" hint="Slower, softer, and only the gentle effects.">
          <select className="wv-select" aria-label="Calm mode" value={set.calm === null ? 'auto' : set.calm ? 'on' : 'off'} onChange={(e) => s.update({ calm: e.target.value === 'auto' ? null : e.target.value === 'on' })}>
            <option value="auto">Follow this device's reduced motion setting</option>
            <option value="on">On</option>
            <option value="off">Off</option>
          </select>
        </Row>
        <Row label="Resolution" hint="Lower it if the picture stutters on an older device.">
          <select className="wv-select" aria-label="Resolution" value={set.scale} onChange={(e) => s.update({ scale: Number(e.target.value) })}>
            {SCALE_CHOICES.map((c) => <option key={c} value={c}>{scaleLabel(c)}</option>)}
          </select>
        </Row>
        <label className="wv-check"><input type="checkbox" checked={set.on} onChange={(e) => s.update({ on: e.target.checked })} /><span>Show it when I open the full player</span></label>
        <label className="wv-check"><input type="checkbox" checked={set.stats} onChange={(e) => s.update({ stats: e.target.checked })} /><span>Show frame rate and resolution</span></label>

        <details className="wv-details">
          <summary>Add your own effect</summary>
          <p>An effect is a small text file with a shader in it. See “Writing effects” in WittyVisualizer's guide. It's checked and tried out before it's used, it stays on this device, and it can't get around the flash limiter.</p>
          <label className="wv-btn file">
            {adding ? 'Checking…' : 'Choose a file'}
            <input type="file" accept=".glsl,.txt,text/plain" disabled={adding} onChange={(e) => { void onFile(e.target.files?.[0]); e.target.value = '' }} />
          </label>
          {addMsg && <p role="status" className="wv-msg">{addMsg}</p>}
          {custom.length > 0 && <ul className="wv-custom">
            {custom.map((e) => <li key={e.id}><span>{e.name}{e.author ? ` by ${e.author}` : ''}{e.license ? ` (${e.license})` : ''}</span><button type="button" className="wv-btn" onClick={() => s.removeCustom(e.id)}>Remove<span className="wv-sr"> {e.name}</span></button></li>)}
          </ul>}
        </details>

        <p className="wv-keys">Keys: <kbd>←</kbd> <kbd>→</kbd> or <kbd>P</kbd> <kbd>N</kbd> change effect, <kbd>R</kbd> random, <kbd>S</kbd> favourite, <kbd>F</kbd> full screen, <kbd>+</kbd> <kbd>−</kbd> strength, <kbd>V</kbd> off.</p>
      </div>
    </div>
  )
}
