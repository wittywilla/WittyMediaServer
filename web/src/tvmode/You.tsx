// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { SettingsHomePage } from '../pages/settings/menu/SettingsMenu'

// You, in TV mode: the settings menu (settingsmenu.json) as Android TV and Apple TV draw it, the person on the left
// third and the groups on the right two-thirds. A row a TV can't change says what is set and opens the panel with
// that setting's address and its QR code.
export default function You() {
  return <SettingsHomePage wide tv />
}
