// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState, type ReactElement } from 'react'
import { Link, NavLink, useLocation } from 'react-router-dom'
import { sduiIcon } from '../sdui/render/icons'
import { Switch } from '../ui/Field'
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
const PIN = 'wms.rail.pinned'
const pinned = () => { try { return localStorage.getItem(PIN) === '1' } catch { return false } }

// withDrawer: the rail has a second column for a library's views (nav style railDrawer).
export function Sidebar({ open, rail, drawer, onClose, withDrawer }: { open: boolean; rail: boolean; drawer: boolean; onClose: () => void; withDrawer?: boolean }) {
  // Keep menu open: the rail shows its full menu all the time, on this device.
  const [keep, setKeep] = useState(pinned)
  const [drawerOf, setDrawerOf] = useState('')
  const { version, brandName, libraries } = useApp()
  const pin = (on: boolean) => { setKeep(on); try { localStorage.setItem(PIN, on ? '1' : '0') } catch { /* this visit only */ } }
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
  // Back closes the drawer first, then the rail.
  useEffect(() => {
    if (!drawerOf) return
    const keys = (e: globalThis.KeyboardEvent) => { if (e.key === 'Escape' || isBackKey(e)) { e.preventDefault(); e.stopImmediatePropagation(); setDrawerOf('') } }
    window.addEventListener('keydown', keys, true)
    return () => window.removeEventListener('keydown', keys, true)
  }, [drawerOf])
  const opened = list.flatMap((e) => ('group' in e ? e.items : [e.item])).find((it) => it.id === drawerOf)?.drawer
  const tip = (node: ReactElement<object>, text: string) => rail ? <Tooltip key={text} text={text} side="right" describes={false}>{node}</Tooltip> : node
  const link = (it: NavItem) => tip(
    <NavLink key={it.id} to={it.to} end={it.end} className={'nav-link' + (it.id === 'home' || it.id === 'tv' || it.id === 'livetv' ? ' nav-dup' : '')} aria-label={rail && !keep ? it.label : undefined}
      aria-haspopup={withDrawer && it.drawer ? 'true' : undefined} aria-expanded={withDrawer && it.drawer ? drawerOf === it.id : undefined}
      onClick={() => setDrawerOf(withDrawer && it.drawer ? it.id : '')}>{navIcon(it)} <span className="nav-text">{it.label}</span></NavLink>, it.label)
  let libsHead = false
  return (
    <>
      {holding && <div className="backdrop-dim" onClick={onClose} />}
      <aside className={'sidebar' + (keep ? ' keep-open' : '')} data-nav-zone="bar" aria-label="Main navigation" aria-hidden={!shown} inert={!shown || undefined}
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
        {rail && <div className="rail-keep"><Switch label="Keep menu open" checked={keep} onChange={pin} /><span className="nav-text">Keep menu open</span></div>}
        <div className="sidebar-footer"><VersionTag version={version} /></div>
      </aside>
      {withDrawer && opened && (
        <nav className="rail-drawer" data-nav-zone="bar" aria-label={opened.title}>
          <header className="rail-drawer-head">
            <span className="rail-drawer-ic" aria-hidden="true">{sduiIcon(opened.icon)}</span>
            <div><h2>{opened.title}</h2>{opened.count ? <p className="muted">{opened.count.toLocaleString()} {opened.count === 1 ? 'title' : 'titles'}</p> : null}</div>
          </header>
          {opened.views.map((v) => <DrawerView key={v.id} view={v} onGo={() => setDrawerOf('')} />)}
        </nav>
      )}
    </>
  )
}

// One view of a library's drawer. Views of one library share a path and differ by their query, so it is the whole
// address that decides which is current (NavLink alone would mark every one).
function DrawerView({ view, onGo }: { view: { id: string; label: string; icon: string; route: string }; onGo: () => void }) {
  const loc = useLocation()
  const [path, query = ''] = view.route.split('?')
  const here = loc.pathname === path && (loc.search.replace(/^\?/, '') === query)
  return <Link to={view.route} className={'nav-link' + (here ? ' active' : '')} aria-current={here ? 'page' : undefined} onClick={onGo}>{sduiIcon(view.icon)} <span>{view.label}</span></Link>
}
