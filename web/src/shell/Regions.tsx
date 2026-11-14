// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { Brand } from '../components/common'
import { cornerName } from '../api'
import { IconExpand, IconHelp, IconReturn } from '../icons'
import { sduiIcon } from '../sdui/render/icons'
import { dockCurrent, type DockEntry, type StatusPart } from '../sdui/regions'
import { useApp } from '../state'
import './regions.css'

// The two shell regions a page or an experience can ask for (docs/plans/DESIGN-2-SDUI-GAPS.md, section 6): a status bar (who you
// are, the server's name, the controllers, the time) and a circular system dock. The controllers are the browser's own pads.

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

