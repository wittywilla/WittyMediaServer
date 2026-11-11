// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { LookEditor } from './LookPanel'
import { useLook } from './look'
import { Section } from '../settings/ui'
import './tv.css'

// WittyTV's look in Settings, Appearance: the same choices as the guide's Look panel. Its id
// keeps older links (/settings/appearance#wittytv) working.
export default function TvLookSettings() {
  const look = useLook()
  return (
    <Section title="WittyTV" id="wittytv" desc="How the TV guide, its player, recordings and channel pages look for you, apart from the app’s theme. It follows you to your other devices.">
      <LookEditor look={look} inSettings />
    </Section>
  )
}
