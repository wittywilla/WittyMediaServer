// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { ReactNode } from 'react'
import { Brand, BrandMark } from './common'
import { useApp } from '../state'
import './standalone.css'

// A page of its own, with no app around it: signing in, Who's watching, signing in with a phone, can't reach the
// server, choosing a server, From away. One card, centred both ways, with the server's logo and name at the top,
// that fits a 1280×720 screen and a phone without scrolling (docs/DESIGN.md, "Standalone pages"). The tests in
// scripts/test-standalone.mjs hold every page to that, by data-standalone-card.
//
// name: the server's name to show; by default the one its owner gave it in setup (GET /api/status). Pages that
// don't know a server yet (choosing one, can't reach it) pass "" for the app's own name.
// wide: room for two columns (a QR code beside its code, a row of profiles).
export function StandaloneCard({ children, name, wide, label, className, foot }: {
  children: ReactNode; name?: string; wide?: boolean; label?: string; className?: string
  // A small line at the card's foot: the version and links people may need before signing in.
  foot?: ReactNode
}) {
  // Outside the app's state (the last-resort error page draws when the app itself failed): the app's own logo and name.
  const app = useApp() as ReturnType<typeof useApp> | null
  return (
    <main className="sa-page">
      <section className={'sa-card' + (wide ? ' wide' : '') + (className ? ' ' + className : '')} data-standalone-card aria-label={label}>
        <div className="sa-brand">{app ? <><BrandMark /><Brand name={name ?? app.brandName} oneLine /></> : <><img className="brand-mark" src="/icon.svg" alt="" /><span className="brand-name">WittyMediaServer</span></>}</div>
        {children}
        {foot && <div className="sa-foot">{foot}</div>}
      </section>
    </main>
  )
}
