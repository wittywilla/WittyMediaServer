// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { ReactNode } from 'react'
import { useLocation } from 'react-router-dom'
import { tvScreenFor } from './levelTable'
import { OnAComputer } from './OnAComputer'

// Every page TV mode shows passes through here (TvFrame.tsx). A screen the table says is desktopOnly is
// the card that says to use a computer; one that says view is the card with the screen beneath it,
// shown and not changeable; every other screen is itself. The table is the server's
// (pkg/sdui/data/tvlevels.json), so the apps' TV screens make the same choices (docs/TV-MODE.md).
export function TvLevelGate({ children }: { children: ReactNode }) {
  const loc = useLocation()
  const screen = tvScreenFor(loc.pathname)
  if (!screen || screen.level === 'full') return <>{children}</>
  return (
    <OnAComputer key={screen.id} level={screen.level} title={screen.title} path={loc.pathname + loc.search} summary={screen.summary}>
      {children}
    </OnAComputer>
  )
}
