// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import menu from '../../../../../pkg/sdui/data/settingsmenu.json'
import type { MenuTable } from './model'

// The server's settings menu (pkg/sdui/data/settingsmenu.json), built into the app: the same file the catalogue
// carries as settingsMenu, so the page and the apps draw one list.
export const SETTINGS_MENU = menu as MenuTable
