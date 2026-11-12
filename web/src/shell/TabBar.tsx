// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { NavLink } from 'react-router-dom'
import { featureOff, useMyLimits } from '../myLimits'
import { IconHome, IconMenu, IconSearch, IconTv } from '../icons'
import { YouTab } from './Account'
import { navIcon, useMenu } from './useNav'

// The bar at the bottom on phones, floating over the page in reach of a thumb. A server's
// menu says which entries are in the bar itself (its primary ones); the rest are behind
// Menu, which opens the side menu. Against an older server it is Home, TV, Search and You.
export function TabBar({ onMenu }: { onMenu: () => void }) {
  const menu = useMenu()
  const noTv = featureOff(useMyLimits(), 'liveTv')
  if (!menu.server) {
    const shown = new Set(menu.all.map((it) => it.id))
    return (
      <nav className="tabbar" data-nav-zone="bar" aria-label="Main">
        {shown.has('home') && <NavLink to="/" end aria-label="Home"><IconHome /></NavLink>}
        {shown.has('tv') && !noTv && <NavLink to="/tv" aria-label="TV guide"><IconTv /></NavLink>}
        <NavLink to="/search" aria-label="Search"><IconSearch /></NavLink>
        <button type="button" onClick={onMenu} aria-label="Menu"><IconMenu /></button>
        <YouTab />
      </nav>
    )
  }
  // You keeps its place at the end, with the profile's picture; Settings is in the menu.
  const bar = menu.all.filter((it) => (it.placement === 'primary' || it.placement === 'end') && it.screen !== 'profiles' && it.screen !== 'settings').slice(0, 4)
  const you = menu.all.some((it) => it.screen === 'profiles')
  return (
    <nav className="tabbar" data-nav-zone="bar" aria-label="Main">
      {bar.map((it) => <NavLink key={it.id} to={it.to} end={it.end} aria-label={it.label}>{navIcon(it)}</NavLink>)}
      <button type="button" onClick={onMenu} aria-label="Menu"><IconMenu /></button>
      {you && <YouTab />}
    </nav>
  )
}
