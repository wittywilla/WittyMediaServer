// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { IconClose, IconContrast, IconDown, IconEject, IconFf, IconLeft, IconNext, IconPause, IconPlay, IconPrev, IconRew, IconRight, IconStop, IconUp } from '../icons'
import { useMedia } from '../shell/useNav'
import { COLOUR_KEYS, keyLabel, kindLabel, remoteRule, type RemoteForm, type RemoteKey } from './remoteKeys'
import { useTvMode } from '../tvmode/mode'
import { t, tk, tx } from '../i18n'

// The on-screen remote of a disc player. Which keys work for what's playing is in remoteKeys.ts.
export type { RemoteKey }

type Props = {
  onKey: (k: RemoteKey) => void
  enabled: (k: RemoteKey) => boolean
  onClose: () => void
  label?: string
  note?: string // why the menu keys are greyed out, when they are
}

const COLOUR_NAMES: Record<string, string> = { red: tk('Red'), green: tk('Green'), yellow: tk('Yellow'), blue: tk('Blue') }
const ejectTip = (label: string) => (label === 'WittyDVD' ? t('Eject the DVD') : label === 'WittyBluray' ? t('Eject the Blu-ray') : t('Stop and close'))

// The floating remote doesn't fit a phone, or a window as short as one on its side, and a television is far from the
// screen: there the remote is a sheet (large keys, a focus ring) along the bottom.
export const useSheetLayout = () => { const small = useMedia('(max-width: 700px), (max-height: 560px)'); return useTvMode() || small }

// Where the remote sits, how big and how see-through it is; remembered per
// browser. x/y are pixels from the player's top-left corner (null = default
// spot in the bottom-right).
type Geom = { x: number | null; y: number | null; scale: number; alpha: number }
const GEOM_KEY = 'wms.remote.geom'
const ALPHAS = [0.92, 0.75, 0.55, 0.35]

function loadGeom(): Geom {
  try {
    const g = JSON.parse(localStorage.getItem(GEOM_KEY) || '')
    return { x: g.x ?? null, y: g.y ?? null, scale: Math.min(1.6, Math.max(0.6, g.scale || 1)), alpha: g.alpha || ALPHAS[0] }
  } catch { return { x: null, y: null, scale: 1, alpha: ALPHAS[0] } }
}

export default function Remote({ onKey, enabled, onClose, label = 'WittyDVD', note }: Props) {
  const [pressed, setPressed] = useState<RemoteKey | null>(null)
  const timer = useRef<any>(null)
  useEffect(() => () => clearTimeout(timer.current), [])
  const el = useRef<HTMLDivElement | null>(null)
  const [geom, setGeomState] = useState<Geom>(loadGeom)
  const setGeom = useCallback((g: Geom, persist = true) => {
    setGeomState(g)
    if (persist) { try { localStorage.setItem(GEOM_KEY, JSON.stringify(g)) } catch { /* ignore */ } }
  }, [])
  // Keep the remote inside the player (after drags, resizes and window changes).
  const clamp = useCallback((g: Geom): Geom => {
    const r = el.current, parent = r?.parentElement
    if (!r || !parent) return g
    const w = r.offsetWidth * g.scale, h = r.offsetHeight * g.scale
    const W = parent.clientWidth, H = parent.clientHeight
    const x = g.x == null ? W - w - 18 : g.x
    const y = g.y == null ? H - h - 96 : g.y
    return { ...g, x: Math.max(4, Math.min(W - w - 4, x)), y: Math.max(4, Math.min(H - h - 4, y)) }
  }, [])
  useLayoutEffect(() => {
    const fix = () => setGeomState((g) => clamp(g))
    fix()
    window.addEventListener('resize', fix)
    return () => window.removeEventListener('resize', fix)
  }, [clamp])
  // Drag by the header.
  const startDrag = (e: React.PointerEvent) => {
    if ((e.target as HTMLElement).closest('button')) return
    e.preventDefault()
    const sx = e.clientX, sy = e.clientY, g0 = clamp(geom)
    const at = (ev: PointerEvent) => clamp({ ...g0, x: (g0.x || 0) + ev.clientX - sx, y: (g0.y || 0) + ev.clientY - sy })
    const move = (ev: PointerEvent) => setGeom(at(ev), false)
    const up = (ev: PointerEvent) => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); setGeom(at(ev)) }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
  // Resize from any edge or corner. The remote keeps its shape, so the size
  // follows the larger pointer movement, and the opposite edge stays put.
  const startResize = (dx: -1 | 0 | 1, dy: -1 | 0 | 1) => (e: React.PointerEvent) => {
    e.preventDefault(); e.stopPropagation()
    const sx = e.clientX, sy = e.clientY, g0 = clamp(geom)
    const bw = el.current?.offsetWidth || 196, bh = el.current?.offsetHeight || 640
    const at = (ev: PointerEvent): Geom => {
      const gx = dx ? (dx * (ev.clientX - sx)) / bw : 0
      const gy = dy ? (dy * (ev.clientY - sy)) / bh : 0
      const scale = Math.min(1.6, Math.max(0.6, g0.scale + (Math.abs(gx) > Math.abs(gy) ? gx : gy)))
      const x = dx === -1 ? (g0.x || 0) + (g0.scale - scale) * bw : g0.x
      const y = dy === -1 ? (g0.y || 0) + (g0.scale - scale) * bh : g0.y
      return clamp({ ...g0, scale, x, y })
    }
    const move = (ev: PointerEvent) => setGeom(at(ev), false)
    const up = (ev: PointerEvent) => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); setGeom(at(ev)) }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
  const handles: [string, -1 | 0 | 1, -1 | 0 | 1][] = [['n', 0, -1], ['s', 0, 1], ['e', 1, 0], ['w', -1, 0], ['ne', 1, -1], ['nw', -1, -1], ['se', 1, 1], ['sw', -1, 1]]
  const cycleAlpha = () => setGeom({ ...geom, alpha: ALPHAS[(ALPHAS.indexOf(geom.alpha) + 1) % ALPHAS.length] })
  const style = {
    left: geom.x ?? undefined, top: geom.y ?? undefined, right: geom.x == null ? 18 : 'auto', bottom: geom.y == null ? 96 : 'auto',
    transform: 'scale(' + geom.scale + ')', '--ra': geom.alpha,
  } as React.CSSProperties
  const press = (k: RemoteKey) => { setPressed(k); clearTimeout(timer.current); timer.current = setTimeout(() => setPressed(null), 140); onKey(k) }
  return (
    <KeyCtx.Provider value={{ enabled, press, pressed }}>
    <div className="remote" ref={el} style={style} onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()} role="group" aria-label={t('{name} remote', { name: label })}>
      <div className="remote-head" onPointerDown={startDrag}>
        <B k="eject" cls="round red" tip={ejectTip(label)}><IconEject size={16} /></B>
        <span className="remote-brand">{label}</span>
        <button type="button" className="rk round ghost" data-tip={t('See-through')} aria-label={t('Change how see-through the remote is')} onClick={(e) => { e.stopPropagation(); cycleAlpha() }}><IconContrast size={14} /></button>
        <button type="button" className="rk round ghost" data-tip={t('Close the Remote')} aria-label={t('Close the Remote (r)')} onClick={(e) => { e.stopPropagation(); onClose() }}><IconClose size={14} /></button>
      </div>
      {note ? <div className="remote-note" role="note">{tx(note)}</div> : <div className="remote-note faint" role="note">{tx(remoteRule())}</div>}
      <div className="remote-row2">
        <B k="topmenu" cls="pill" title={t('Top menu (disc title menu)')}>{tx(keyLabel('topmenu', tk('TOP MENU')))}</B>
        <B k="menu" cls="pill" title={t("Menu (the root menu, or a Blu-ray's pop-up menu)")}>{tx(keyLabel('menu', tk('MENU')))}</B>
      </div>
      <div className="remote-dpad">
        <B k="up" cls="d up" title={t('Up')}><IconUp /></B>
        <B k="left" cls="d left" title={t('Left')}><IconLeft /></B>
        <B k="enter" cls="d enter" title={t('Enter')}>{t('ENTER')}</B>
        <B k="right" cls="d right" title={t('Right')}><IconRight /></B>
        <B k="down" cls="d down" title={t('Down')}><IconDown /></B>
      </div>
      <div className="remote-row2">
        <B k="return" cls="pill" title={t('Return (go back)')}>{tx(keyLabel('return', tk('RETURN')))}</B>
        <B k="angle" cls="pill" title={t('Video version (camera angle)')}>{tx(keyLabel('angle', tk('ANGLE')))}</B>
      </div>
      <div className="remote-row2">
        <B k="audio" cls="pill" title={t('Audio & language')}>{tx(keyLabel('audio', tk('AUDIO')))}</B>
        <B k="subtitle" cls="pill" title={t('Subtitles')}>{tx(keyLabel('subtitle', tk('SUBTITLE')))}</B>
      </div>
      <div className="remote-row4">
        <B k="prev" title={t('Previous chapter')}><IconPrev size={16} /></B>
        <B k="rew" title={t('Rewind 10 s')}><IconRew size={16} /></B>
        <B k="ff" title={t('Forward 30 s')}><IconFf size={16} /></B>
        <B k="next" title={t('Next chapter')}><IconNext size={16} /></B>
      </div>
      <div className="remote-row3">
        <B k="stop" title={t('Stop')}><IconStop size={14} /></B>
        <B k="play" cls="accent" title={t('Play')}><IconPlay size={16} /></B>
        <B k="pause" title={t('Pause')}><IconPause size={16} /></B>
      </div>
      <Colours />
      <div className="remote-digits">
        {[1, 2, 3, 4, 5, 6, 7, 8, 9].map((d) => <B key={d} k={`digit${d}` as RemoteKey} title={t('Button / chapter {n}', { n: d })}>{d}</B>)}
        <span />
        <B k="digit0" title="0">0</B>
        <span />
      </div>
      {handles.map(([n, dx, dy]) => (
        <span key={n} className={'remote-rs rs-' + n} onPointerDown={startResize(dx, dy)} onDoubleClick={() => setGeom(clamp({ ...geom, scale: 1 }))} aria-hidden="true" />
      ))}
      <span className="remote-grip" aria-hidden="true" />
    </div>
    </KeyCtx.Provider>
  )
}

// Buttons are module-level components (not defined inside Remote), so a
// re-render keeps the same DOM buttons: focus, hover and keyboard/controller
// navigation survive the player's frequent updates.
const KeyCtx = createContext<{ enabled: (k: RemoteKey) => boolean; press: (k: RemoteKey) => void; pressed: RemoteKey | null }>({ enabled: () => true, press: () => {}, pressed: null })

// tip: a see-through tooltip instead of the browser's (used in the header).
function B({ k, children, cls = '', title, tip }: { k: RemoteKey; children: React.ReactNode; cls?: string; title?: string; tip?: string }) {
  const { enabled, press, pressed } = useContext(KeyCtx)
  return (
    <button type="button" className={'rk ' + cls + (pressed === k ? ' down' : '')} disabled={!enabled(k)} title={tip ? undefined : title} data-tip={tip} aria-label={tip || title}
      onClick={(e) => { e.stopPropagation(); press(k) }}>
      {children}
    </button>
  )
}

// The colour keys, which Blu-ray Java menus use.
function Colours() {
  return (
    <div className="remote-colours">
      {COLOUR_KEYS.map((c) => <B key={c} k={c} cls={'colour key-' + c} title={t('{colour} (F{n})', { colour: tx(COLOUR_NAMES[c]), n: COLOUR_KEYS.indexOf(c) + 1 })}><span aria-hidden="true" /></B>)}
    </div>
  )
}

// RemoteBar is the same remote laid out as one strip above the seek bar, for
// viewers who prefer it out of the picture.
export function RemoteBar({ onKey, enabled, disc = true, note, below = false, form }: { onKey: (k: RemoteKey) => void; enabled: (k: RemoteKey) => boolean; disc?: boolean; note?: string; below?: boolean; form?: RemoteForm }) {
  const head = form ? kindLabel(form) : ''
  return (
    <KeyCtx.Provider value={{ enabled, press: onKey, pressed: null }}>
    <div className={'remote-bar' + (below ? ' below' : '')} role="toolbar" aria-label={t('Disc remote')} onClick={(e) => e.stopPropagation()}>
      {head && <span className="rb-title">{t('{name} remote', { name: tx(head) })}</span>}
      {note && <span className="rb-note" role="note">{tx(note)}</span>}
      <BarKey k="topmenu" label={t('Top menu')}>{tx(keyLabel('topmenu', tk('TOP MENU')))}</BarKey>
      <BarKey k="menu" label={t('Menu')}>{tx(keyLabel('menu', tk('MENU')))}</BarKey>
      <BarKey k="return" label={t('Return')}>{tx(keyLabel('return', tk('RETURN')))}</BarKey>
      <span className="rb-sep" />
      <BarKey k="left" label={t('Left')}><IconLeft size={16} /></BarKey>
      <BarKey k="up" label={t('Up')}><IconUp size={16} /></BarKey>
      <BarKey k="enter" label={t('Enter')} cls="accent">OK</BarKey>
      <BarKey k="down" label={t('Down')}><IconDown size={16} /></BarKey>
      <BarKey k="right" label={t('Right')}><IconRight size={16} /></BarKey>
      <span className="rb-sep" />
      <BarKey k="audio" label={t('Audio & language')}>{tx(keyLabel('audio', tk('AUDIO')))}</BarKey>
      <BarKey k="subtitle" label={t('Subtitles')}>{tx(keyLabel('subtitle', tk('SUBTITLE')))}</BarKey>
      <BarKey k="angle" label={t('Video version (camera angle)')}>{tx(keyLabel('angle', tk('ANGLE')))}</BarKey>
      <span className="rb-sep hide-narrow" />
      <span className="rb-digits hide-narrow">
        {[1, 2, 3, 4, 5, 6, 7, 8, 9, 0].map((d) => <BarKey key={d} k={`digit${d}` as RemoteKey} label={t('Button or chapter {n}', { n: d })}>{d}</BarKey>)}
      </span>
      <span className="rb-sep hide-narrow" />
      <span className="rb-colours hide-narrow">
        {COLOUR_KEYS.map((c, i) => <BarKey key={c} k={c} label={t('{colour} (F{n})', { colour: tx(COLOUR_NAMES[c]), n: i + 1 })} cls={'colour key-' + c}><span aria-hidden="true" /></BarKey>)}
      </span>
      <BarKey k="eject" label={disc ? t('Eject the disc') : t('Stop and close')} cls="red"><IconEject size={14} /></BarKey>
    </div>
    </KeyCtx.Provider>
  )
}

function BarKey({ k, children, label, cls = '' }: { k: RemoteKey; children: React.ReactNode; label: string; cls?: string }) {
  const { enabled, press } = useContext(KeyCtx)
  return <button type="button" className={'rb ' + cls} disabled={!enabled(k)} title={label} aria-label={label} onClick={(e) => { e.stopPropagation(); press(k) }}>{children}</button>
}

// RemoteSheet is the remote as a sheet along the bottom: on a phone, upright or on its side,
// and over the page while the mini player plays (docked). Every key is on it: the first page
// has the menus, arrows, tracks and playback, the second the digits and colour keys.
export function RemoteSheet({ onKey, enabled, onClose, label = 'WittyDVD', note, docked = false }: Props & { docked?: boolean }) {
  const [page, setPage] = useState<'keys' | 'numbers'>('keys')
  const [pressed, setPressed] = useState<RemoteKey | null>(null)
  const timer = useRef<any>(null)
  useEffect(() => () => clearTimeout(timer.current), [])
  const press = (k: RemoteKey) => { setPressed(k); clearTimeout(timer.current); timer.current = setTimeout(() => setPressed(null), 140); onKey(k) }
  // Esc closes it, before anything else hears the key.
  useEffect(() => {
    const onEsc = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.stopImmediatePropagation(); e.preventDefault(); onClose() } }
    window.addEventListener('keydown', onEsc, true)
    return () => window.removeEventListener('keydown', onEsc, true)
  }, [onClose])
  // Open over a full player, the sheet says how tall it is (--remote-space on the player), so in portrait the picture
  // moves up to the middle of the room left above it rather than sit half under it (player/bar.css).
  const own = useRef<HTMLDivElement | null>(null)
  useLayoutEffect(() => {
    const el = own.current
    const player = el?.closest<HTMLElement>('.player')
    if (!el || !player || docked) return
    const set = () => player.style.setProperty('--remote-space', `${Math.round(el.getBoundingClientRect().height)}px`)
    set()
    player.classList.add('remote-up')
    const ro = new ResizeObserver(set)
    ro.observe(el)
    return () => { ro.disconnect(); player.classList.remove('remote-up'); player.style.removeProperty('--remote-space') }
  }, [docked])
  const sheet = (
    <KeyCtx.Provider value={{ enabled, press, pressed }}>
    <div ref={own} className={'remote remote-sheet' + (docked ? ' docked' : '')} role="dialog" aria-label={t('{name} remote', { name: label })}
      onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
      <div className="sheet-head">
        <B k="eject" cls="round red" title={ejectTip(label)}><IconEject size={16} /></B>
        <span className="remote-brand">{label}</span>
        <div className="sheet-pages" role="group" aria-label={t('Remote page')}>
          <button type="button" className={'sheet-page' + (page === 'keys' ? ' on' : '')} aria-pressed={page === 'keys'} onClick={() => setPage('keys')}>{t('Keys')}</button>
          <button type="button" className={'sheet-page' + (page === 'numbers' ? ' on' : '')} aria-pressed={page === 'numbers'} onClick={() => setPage('numbers')}>1 2 3</button>
        </div>
        <button type="button" className="rk round ghost" aria-label={t('Close the remote')} title={t('Close the remote')} onClick={onClose}><IconClose size={14} /></button>
      </div>
      {note && <div className="remote-note" role="note">{tx(note)}</div>}
      {page === 'keys' ? (
        <div className="sheet-keys">
          <div className="sheet-menus">
            <B k="topmenu" cls="pill" title={t('Top menu (disc title menu)')}>{tx(keyLabel('topmenu', tk('TOP MENU')))}</B>
            <B k="menu" cls="pill" title={t("Menu (the root menu, or a Blu-ray's pop-up menu)")}>{tx(keyLabel('menu', tk('MENU')))}</B>
            <B k="return" cls="pill" title={t('Return (go back)')}>{tx(keyLabel('return', tk('RETURN')))}</B>
          </div>
          <div className="remote-dpad">
            <B k="up" cls="d up" title={t('Up')}><IconUp /></B>
            <B k="left" cls="d left" title={t('Left')}><IconLeft /></B>
            <B k="enter" cls="d enter" title={t('Enter')}>OK</B>
            <B k="right" cls="d right" title={t('Right')}><IconRight /></B>
            <B k="down" cls="d down" title={t('Down')}><IconDown /></B>
          </div>
          <div className="sheet-tracks">
            <B k="audio" cls="pill" title={t('Audio & language')}>{tx(keyLabel('audio', tk('AUDIO')))}</B>
            <B k="subtitle" cls="pill" title={t('Subtitles')}>{tx(keyLabel('subtitle', tk('SUBTITLE')))}</B>
            <B k="angle" cls="pill" title={t('Video version (camera angle)')}>{tx(keyLabel('angle', tk('ANGLE')))}</B>
          </div>
          <div className="sheet-transport">
            <B k="prev" title={t('Previous chapter')}><IconPrev size={16} /></B>
            <B k="rew" title={t('Rewind 10 s')}><IconRew size={16} /></B>
            <B k="play" cls="accent" title={t('Play')}><IconPlay size={16} /></B>
            <B k="pause" title={t('Pause')}><IconPause size={16} /></B>
            <B k="ff" title={t('Forward 30 s')}><IconFf size={16} /></B>
            <B k="next" title={t('Next chapter')}><IconNext size={16} /></B>
            <B k="stop" title={t('Stop')}><IconStop size={14} /></B>
          </div>
        </div>
      ) : (
        <div className="sheet-numbers">
          <div className="sheet-digits">
            {[1, 2, 3, 4, 5, 6, 7, 8, 9, 0].map((d) => <B key={d} k={`digit${d}` as RemoteKey} title={t('Button / chapter {n}', { n: d })}>{d}</B>)}
          </div>
          <Colours />
        </div>
      )}
    </div>
    </KeyCtx.Provider>
  )
  // Docked, it sits over the page: the mini player is too small to hold it.
  return docked ? createPortal(sheet, document.body) : sheet
}
