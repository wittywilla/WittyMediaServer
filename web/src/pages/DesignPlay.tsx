// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyPlay Online's page drawn from the conformance fixture (pkg/sdui/conformance/fixtures/wittyplay-online-tv.json), with the new
// pieces switched on (Shelf focusGrow and detail, item actions with keys): what the renderer does with them before a server sends
// them. For administrators, like /design.
import { useMemo } from 'react'
import { describe, type DeviceFacts } from '../sdui/device'
import { PageView } from '../sdui/SduiPage'
import type { Document } from '../sdui/types'
import fixture from './design/playFixture.json'

const facts = (tv: boolean): DeviceFacts => ({ width: tv ? 1920 : window.innerWidth, height: 1080, tvMode: tv, desktopShell: false, touch: false, pointer: true, reducedMotion: false, hdr: false, locale: 'en-GB', uhd: false })

export default function DesignPlay() {
  const device = useMemo(() => describe(facts(true)), [])
  return (
    <main id="main" tabIndex={-1} className="design-play" style={{ padding: 'var(--sp-5)' }}>
      <h1 className="sr-only">WittyPlay Online</h1>
      <PageView doc={fixture as unknown as Document} device={device} handlers={{ act: () => {}, more: () => {}, busy: () => false, failed: () => false, setQuery: () => {}, menu: () => {} }} />
    </main>
  )
}
