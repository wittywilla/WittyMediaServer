// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { NavLink } from 'react-router-dom'
import { featureOff, useMyLimits } from '../myLimits'
import { IconHome, IconMenu, IconSearch, IconTv } from '../icons'
import { YouTab } from './Account'
import { useNavPrefs } from './useNav'

// The bar at the bottom on phones, floating over the page in reach of a thumb.
export function TabBar({ onMenu }: { onMenu: () => void }) {
  const { prefs } = useNavPrefs()
  const hidden = prefs.hidden || []
  const noTv = featureOff(useMyLimits(), 'liveTv')
  return (
    <nav className="tabbar" aria-label="Main">
      {!hidden.includes('home') && <NavLink to="/" end aria-label="Home"><IconHome /></NavLink>}
      {!hidden.includes('tv') && !noTv && <NavLink to="/tv" aria-label="TV guide"><IconTv /></NavLink>}
      <NavLink to="/search" aria-label="Search"><IconSearch /></NavLink>
      <button type="button" onClick={onMenu} aria-label="Menu"><IconMenu /></button>
      <YouTab />
    </nav>
  )
}
