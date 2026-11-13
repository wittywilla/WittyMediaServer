// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Brand } from '../components/common'
import { cornerName } from '../api'
import { IconExpand, IconHelp, IconReturn } from '../icons'
import { useUserPref } from '../ui/userPrefs'
import { sduiIcon } from '../sdui/render/icons'
import { dockCurrent, usePageRegions, type DockEntry, type StatusPart } from '../sdui/regions'
import { setPlayMode } from '../theme'
import { isBackKey } from '../ui/spatial'
import { useApp } from '../state'
import { dockFor, exitWords, isPlayPath, isPlayTop, PLAY_CONSOLES, PLAY_CONTROLLERS, PLAY_EXIT, PLAY_PADS_TITLE, PLAY_STATUS, PLAY_TIP, PLAY_TIP_KEY, tipDue, type PlayLayout, type PlayPads } from './playPaths'
import { usePlayTabs, LayoutChooser, LayoutSwitch, PadsChooser, PadsSetting, PlayGlyph, usePlayLayout, usePlayPads } from './playPads'
import { GameArtContext, GlyphContext, RowLayoutContext, TabsContext, type GlyphKey } from '../sdui/render/glyphs'
import { useGameArtApi } from './GamePictureArt'
import './playChrome.css'

export { isPlayPath }

// The two shell regions WittyPlay Online asks for (docs/plans/DESIGN-2-SDUI-GAPS.md, section 6): a status bar (who you are, the server's
// name, the controllers, the time) and a circular system dock (where the console's home goes). An experience or module scope turns them
// on with placements; the dock's buttons are shell.systemDock. The controllers are the browser's own pads: a web page is not told their battery.

function usePads(): string[] {
  const read = () => Array.from(navigator.getGamepads?.() || []).filter((p): p is Gamepad => !!p && p.connected).map((p) => p.id.replace(/\s*\(.*$/, '').trim() || 'Controller')
  const [pads, setPads] = useState<string[]>(read)
  useEffect(() => {
    const on = () => setPads(read())
    window.addEventListener('gamepadconnected', on); window.addEventListener('gamepaddisconnected', on)
    return () => { window.removeEventListener('gamepadconnected', on); window.removeEventListener('gamepaddisconnected', on) }
  }, [])
  return pads
}

function useClock(): string {
  const fmt = () => new Date().toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  const [t, setT] = useState(fmt)
  useEffect(() => { const id = window.setInterval(() => setT(fmt()), 15000); return () => window.clearInterval(id) }, [])
  return t
}

// Real full screen, when the browser allows it: offered on a button, never forced.
function useFullscreen(): [boolean, (() => void) | null] {
  const can = typeof document !== 'undefined' && !!document.documentElement.requestFullscreen && document.fullscreenEnabled !== false
  const [on, setOn] = useState(() => typeof document !== 'undefined' && !!document.fullscreenElement)
  useEffect(() => {
    const ch = () => setOn(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', ch)
    return () => document.removeEventListener('fullscreenchange', ch)
  }, [])
  if (!can) return [false, null]
  return [on, () => { if (document.fullscreenElement) void document.exitFullscreen().catch(() => {}); else void document.documentElement.requestFullscreen().catch(() => {}) }]
}

export function StatusBar({ parts, onExit, onHelp }: { parts: StatusPart[]; onExit?: () => void; onHelp?: () => void }) {
  const { user, brandName } = useApp()
  const pads = usePads()
  const time = useClock()
  const [full, toggleFull] = useFullscreen()
  if (!user) return null
  const name = cornerName(user)
  return (
    <header className="play-status" aria-label="Status">
      <div className="play-who">
        <span className="play-avatar" aria-hidden="true">{name.slice(0, 1).toUpperCase()}</span>
        <span className="play-who-text"><b>{name}</b>{parts.filter((p) => p.kind === 'text' || (p.kind === 'profile' && p.text)).map((p) => <span key={p.id}>{p.text}</span>)}</span>
      </div>
      <div className="play-right">
        {parts.some((p) => p.kind === 'server') && <span className="play-brand"><Brand name={brandName} /></span>}
        {pads.length > 0 && <ul className="play-pads" aria-label="Controllers">{pads.slice(0, 4).map((p, i) => <li key={i} title={p}><i aria-hidden="true" />P{i + 1}<span className="sr-only"> {p}</span></li>)}</ul>}
        <time className="play-clock">{time}</time>
        {onHelp && <button type="button" className="play-sys" onClick={onHelp} aria-label="How to get back to the app" title="How to get back to the app"><IconHelp /></button>}
        {toggleFull && <button type="button" className="play-sys" onClick={toggleFull} aria-pressed={full} aria-label="Full screen" title={full ? 'Leave full screen' : 'Full screen'}><IconExpand /></button>}
        {onExit && <button type="button" className="play-sys play-exit" onClick={onExit} aria-label="Return To Server" title="Return to the server's Home (Esc)"><IconReturn /><span className="play-exit-label" aria-hidden="true">Return To Server</span></button>}
      </div>
    </header>
  )
}

export function SystemDock({ entries }: { entries: DockEntry[] }) {
  const loc = useLocation()
  if (!entries.length) return null
  const current = dockCurrent(entries, loc.pathname, loc.search)
  return (
    <nav className="play-dock" aria-label="System">
      {entries.map((e, i) => (
        <Link key={e.id} to={e.route} className={i === current ? 'on' : ''} aria-current={i === current ? 'page' : undefined} aria-label={e.label}>
          <span className="play-dock-ic" aria-hidden="true">{sduiIcon(e.icon)}</span>
          <span className="play-dock-label" aria-hidden="true">{e.label}</span>
        </Link>
      ))}
    </nav>
  )
}

// The first-time tip, a dialog (focus moves in and stays; Esc, Back or B closes it): on the first launch "Which buttons should we
// show?" and then, optionally, the layout; always how to get back to the app, in those buttons, with a ring round the Home button.
function ExitTip({ onClose, choose, pads, setPads, layout, setLayout }: { onClose: () => void; choose: boolean; pads: PlayPads; setPads: (p: PlayPads) => void; layout: PlayLayout; setLayout: (l: PlayLayout) => void }) {
  const id = useId()
  const [step, setStep] = useState<'pads' | 'layout' | 'exit'>(choose ? 'pads' : 'exit')
  const box = useRef<HTMLDivElement | null>(null)
  const [ring, setRing] = useState<DOMRect | null>(null)
  useEffect(() => {
    const before = document.activeElement as HTMLElement | null
    return () => { if (before?.isConnected) before.focus({ preventScroll: true }) }
  }, [])
  useEffect(() => {
    box.current?.querySelector<HTMLElement>(step === 'exit' ? '.wplay-tip-ok' : '[aria-checked=true]')?.focus()
    if (step !== 'exit') { setRing(null); return }
    const place = () => setRing(document.querySelector('.wplay .play-exit')?.getBoundingClientRect() || null)
    place()
    window.addEventListener('resize', place)
    return () => window.removeEventListener('resize', place)
  }, [step])
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape' || isBackKey(e.nativeEvent) || e.key === 'Backspace') {
      e.preventDefault(); e.stopPropagation()
      if (step === 'pads') setPads(pads)
      onClose()
    }
  }
  const drawn = (g: 'a' | 'b') => <i className="sdui-glyph drawn" aria-hidden="true"><PlayGlyph pads={pads} g={g} /></i>
  const ways = PLAY_TIP.ways.map((w) => (w.key === 'B' ? { ...w, text: exitWords(pads), glyph: true } : { ...w, glyph: false }))
    .filter((w) => !(pads === 'keyboard' && w.glyph))
  return (
    <div className="wplay-tip-scrim" onClick={(e) => { if (e.target === e.currentTarget) { if (step === 'pads') setPads(pads); onClose() } }} onKeyDown={keys}>
      {ring && <span className="wplay-tip-ring" aria-hidden="true" style={{ top: ring.top - 8, left: ring.left - 8, width: ring.width + 16, height: ring.height + 16 }} />}
      <div ref={box} className={'wplay-tip step-' + step} role="dialog" aria-modal="true" aria-labelledby={id + 't'} aria-describedby={id + 'd'}>
        {step === 'pads' ? (
          <>
            <h2 id={id + 't'}>{PLAY_PADS_TITLE}</h2>
            <p id={id + 'd'}>WittyPlay shows these buttons on its screens. You can change it later under Controllers.</p>
            <PadsChooser value={pads} onChange={setPads} labelledBy={id + 't'} />
            <button type="button" className="wplay-tip-ok" onClick={() => { setPads(pads); setStep('layout') }}>{drawn('a')} Continue</button>
          </>
        ) : step === 'layout' ? (
          <>
            <h2 id={id + 't'}>How should your games be laid out?</h2>
            <p id={id + 'd'}>You can switch at any time with the buttons at the end of the tabs, or in Settings.</p>
            <LayoutChooser value={layout} onChange={setLayout} labelledBy={id + 't'} />
            <div className="wplay-tip-row">
              <button type="button" className="wplay-tip-ok" onClick={() => { setLayout(layout); setStep('exit') }}>{drawn('a')} Continue</button>
              <button type="button" className="wplay-tip-skip" onClick={() => setStep('exit')}>Skip</button>
            </div>
          </>
        ) : (
          <>
            <h2 id={id + 't'}>{PLAY_TIP.title}</h2>
            <p id={id + 'd'}>{PLAY_TIP.intro}</p>
            <ul>{ways.map((w) => <li key={w.key}>{w.glyph ? <span className="wplay-tip-key">{drawn('b')}</span> : w.key === 'server' ? <span className="wplay-tip-key" aria-hidden="true"><IconReturn size={22} /></span> : <kbd>{w.key}</kbd>}<span>{w.text}</span></li>)}</ul>
            <button type="button" className="wplay-tip-ok" onClick={onClose}>{drawn('a')} {PLAY_TIP.done}</button>
          </>
        )}
      </div>
    </div>
  )
}

// WittyPlay Online takes over the window: its own status bar, page and dock over everything of the app (menu, rail, tab bar, audio
// bar, footer), in its own palette and none of the app's look or theme (playChrome.css, theme.ts setPlayMode). Exit (the Home button,
// or Back, Esc, Backspace or a pad's B on its top page) puts it away and goes to the app's Home page.
export function PlayFrame({ children }: { children: ReactNode }) {
  const regions = usePageRegions()
  const nav = useNavigate()
  const loc = useLocation()
  const exit = () => {
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
    nav(PLAY_EXIT)
  }
  // Before the first paint, so the app's look never shows for a frame.
  useLayoutEffect(() => {
    const html = document.documentElement
    html.dataset.wplay = ''
    setPlayMode(true)
    return () => { delete html.dataset.wplay; setPlayMode(false) }
  }, [])
  // The tip shows by itself the first time this profile opens WittyPlay (kept in the profile's /me/ui, so once on every device),
  // a moment after the screen has drawn and the profile's own choices have come; the help button shows it again.
  const [seen, setSeen] = useUserPref<boolean>(PLAY_TIP_KEY, false)
  const [tip, setTip] = useState(false)
  const [ready, setReady] = useState(false)
  useEffect(() => { const t = window.setTimeout(() => setReady(true), 900); return () => window.clearTimeout(t) }, [])
  useEffect(() => { if (ready && tipDue(seen)) setTip(true) }, [ready]) // eslint-disable-line react-hooks/exhaustive-deps
  const closeTip = () => { setTip(false); if (tipDue(seen)) void setSeen(true).catch(() => {}) }
  const [pads, padsChosen, setPads] = usePlayPads()
  const glyph = useCallback((g: GlyphKey) => <PlayGlyph pads={pads} g={g} />, [pads])
  const [layout, setLayout] = usePlayLayout()
  // How each game's picture is drawn: fitted, filled, or in its case (Settings › Game pictures, and a game's own Picture style).
  const gameArt = useGameArtApi()
  // The Consoles tab beside the server's tabs, and the layout switch at the end of the row.
  const [tabPref] = usePlayTabs()
  const tabs = useMemo(() => ({ page: 'wittyplay-online', tabs: [{ id: 'consoles', label: 'Consoles', to: PLAY_CONSOLES, after: 'recent' }], end: <LayoutSwitch />, order: tabPref.order, hidden: tabPref.hidden }), [tabPref.order.join(), tabPref.hidden.join()]) // eslint-disable-line react-hooks/exhaustive-deps
  const top = isPlayTop(loc.pathname)
  useEffect(() => {
    if (!top) return
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.defaultPrevented) return
      const t = e.target as HTMLElement | null
      const typing = !!t?.matches?.('input, textarea, select, [contenteditable=true]')
      const back = e.key === 'Escape' || isBackKey(e) || (e.key === 'Backspace' && !typing)
      if (!back || (typing && e.key !== 'Escape')) return
      // A dialog, menu or list open over the page closes first.
      if (document.querySelector('[role=dialog], [role=alertdialog], [role=menu], [role=listbox], .modal-backdrop')) return
      e.preventDefault()
      e.stopImmediatePropagation()
      exit()
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [top]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <GlyphContext.Provider value={glyph}><RowLayoutContext.Provider value={layout}><TabsContext.Provider value={tabs}><GameArtContext.Provider value={gameArt}>
      <div className="wplay" data-scope="wittyplay" data-pads={pads} data-layout={layout}>
        <StatusBar parts={regions?.statusBar || PLAY_STATUS} onExit={exit} onHelp={() => setTip(true)} />
        <main className="wplay-stage" id="main" data-nav-zone="stage" tabIndex={-1}>
          {loc.pathname === PLAY_CONTROLLERS && <PadsSetting />}
          {children}
        </main>
        <SystemDock entries={dockFor(regions?.systemDock)} />
        {tip && <ExitTip onClose={closeTip} choose={!padsChosen} pads={pads} setPads={setPads} layout={layout} setLayout={setLayout} />}
      </div>
    </GameArtContext.Provider></TabsContext.Provider></RowLayoutContext.Provider></GlyphContext.Provider>
  )
}
