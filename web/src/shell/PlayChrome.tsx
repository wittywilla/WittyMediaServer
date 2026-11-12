// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { Brand } from '../components/common'
import { cornerName } from '../api'
import { sduiIcon } from '../sdui/render/icons'
import { dockCurrent, type DockEntry, type StatusPart } from '../sdui/regions'
import { useApp } from '../state'
import './playChrome.css'

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

export function StatusBar({ parts }: { parts: StatusPart[] }) {
  const { user, brandName } = useApp()
  const pads = usePads()
  const time = useClock()
  if (!user) return null
  return (
    <header className="play-status" aria-label="Status">
      <div className="play-who">
        <span className="play-avatar" aria-hidden="true">{cornerName(user).slice(0, 1).toUpperCase()}</span>
        <span className="play-who-text"><b>{cornerName(user)}</b>{parts.filter((p) => p.kind === 'text' || (p.kind === 'profile' && p.text)).map((p) => <span key={p.id}>{p.text}</span>)}</span>
      </div>
      <div className="play-right">
        {parts.some((p) => p.kind === 'server') && <span className="play-brand"><Brand name={brandName} /></span>}
        <ul className="play-pads" aria-label="Controllers">{pads.slice(0, 4).map((p, i) => <li key={i} title={p}><i aria-hidden="true" />P{i + 1}<span className="sr-only"> {p}</span></li>)}</ul>
        <time className="play-clock">{time}</time>
      </div>
    </header>
  )
}

export function SystemDock({ entries }: { entries: DockEntry[] }) {
  const loc = useLocation()
  if (!entries.length) return null
  const current = dockCurrent(entries, loc.pathname)
  return (
    <nav className="play-dock" aria-label="System">
      {entries.map((e, i) => (
        <Link key={e.id} to={e.route} className={i === current ? 'on' : ''} aria-current={i === current ? 'page' : undefined} aria-label={e.label}>
          <span className="play-dock-ic" aria-hidden="true">{sduiIcon(e.icon)}</span>
          {i === current && <span className="play-dock-label">{e.label}</span>}
        </Link>
      ))}
    </nav>
  )
}
