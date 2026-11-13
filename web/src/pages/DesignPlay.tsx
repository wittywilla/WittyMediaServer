// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyPlay Online's page drawn from the conformance fixture (pkg/sdui/conformance/fixtures/wittyplay-online-tv.json), with the new
// pieces switched on (Shelf focusGrow and detail, item actions with keys): what the renderer does with them before a server sends
// them. For administrators, like /design. It shows in WittyPlay's own frame (shell/PlayChrome.tsx), which brings the page's main.
import { useMemo } from 'react'
import { describe, type DeviceFacts } from '../sdui/device'
import { PageView } from '../sdui/SduiPage'
import type { Document } from '../sdui/types'
import { useSearchParams } from 'react-router-dom'
import fixture from './design/playFixture.json'
import privacy from './design/playPrivacy.json'

// ?screen=privacy draws the games privacy form (a phone's), the default the home row on a television.
const facts = (tv: boolean): DeviceFacts => ({ width: tv ? 1920 : window.innerWidth, height: 1080, tvMode: tv, desktopShell: false, touch: false, pointer: true, reducedMotion: false, hdr: false, locale: 'en-GB', uhd: false })

export default function DesignPlay() {
  const [q] = useSearchParams()
  const form = q.get('screen') === 'privacy'
  const device = useMemo(() => describe(facts(!form)), [form])
  return (
    <div className="design-play">
      <h1 className="sr-only">WittyPlay Online</h1>
      <PageView doc={(form ? privacy : fixture) as unknown as Document} device={device} handlers={{ act: () => {}, more: () => {}, busy: () => false, failed: () => false, setQuery: () => {}, menu: () => {} }} />
    </div>
  )
}
