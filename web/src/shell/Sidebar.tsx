// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, type ReactElement } from 'react'
import { Link, NavLink } from 'react-router-dom'
import { Brand, BrandMark, UpdateDot, VersionTag } from '../components/common'
import { IconSettings } from '../icons'
import { useApp } from '../state'
import { isBackKey } from '../ui/spatial'
import { Tooltip } from '../ui/Tooltip'
import { arrange, entries, isLibrary, type NavItem } from './navModel'
import { navIcon, useNavItems, useNavPrefs } from './useNav'

// The side menu. Three forms: beside the page, folded to a rail of icons, and on
// narrower screens a drawer that slides over the page and holds the keyboard until
// it's closed.
export function Sidebar({ open, rail, drawer, onClose }: { open: boolean; rail: boolean; drawer: boolean; onClose: () => void }) {
  const { version, brandName, libraries } = useApp()
  const items = useNavItems()
  const { prefs } = useNavPrefs()
  const list = useMemo(() => entries(arrange(items, prefs), prefs), [items, prefs])
  const shown = open || rail
  const holding = drawer && open
  useEffect(() => {
    if (!holding) return
    const keys = (e: globalThis.KeyboardEvent) => { if (e.key === 'Escape' || isBackKey(e)) { e.preventDefault(); onClose() } }
    window.addEventListener('keydown', keys)
    return () => window.removeEventListener('keydown', keys)
  }, [holding, onClose])
  const tip = (node: ReactElement<object>, text: string) => rail ? <Tooltip key={text} text={text} side="right" describes={false}>{node}</Tooltip> : node
  const link = (it: NavItem) => tip(
    <NavLink key={it.id} to={it.to} end={it.end} className={'nav-link' + (it.id === 'home' || it.id === 'tv' || it.id === 'livetv' ? ' nav-dup' : '')} aria-label={rail ? it.label : undefined}>{navIcon(it)} <span className="nav-text">{it.label}</span></NavLink>, it.label)
  let libsHead = false
  return (
    <>
      {holding && <div className="backdrop-dim" onClick={onClose} />}
      <aside className="sidebar" aria-label="Main navigation" aria-hidden={!shown} inert={!shown || undefined}
        role={holding ? 'dialog' : undefined} aria-modal={holding || undefined}>
        <Link to="/" className="brand" aria-label={rail ? brandName || 'Home' : undefined}><BrandMark /><span className="nav-text"><Brand name={brandName} /></span></Link>
        {list.map((e) => {
          if ('group' in e) return (
            <div key={e.key} className="nav-group" role="group" aria-label={e.group}>
              <div className="nav-section" aria-hidden="true">{e.group}</div>
              {e.items.map(link)}
            </div>
          )
          const first = !libsHead && !prefs.grouped && isLibrary(e.item.id)
          if (first) libsHead = true
          return first ? <div key={e.key} className="nav-group"><div className="nav-section">Libraries</div>{link(e.item)}</div> : link(e.item)
        })}
        {libraries.length === 0 && <div className="nav-empty">No libraries yet</div>}
        <div className="nav-section nav-dup">Manage</div>
        {tip(<NavLink to="/settings" className="nav-link nav-dup" aria-label={rail ? 'Settings' : undefined}><IconSettings /> <span className="nav-text">Settings</span><UpdateDot /></NavLink>, 'Settings')}
        <div className="sidebar-footer"><VersionTag version={version} /></div>
      </aside>
    </>
  )
}
