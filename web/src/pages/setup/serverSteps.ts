// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The server keeps where first run is (GET and PUT /setup/state) in its own fourteen steps: welcome, account, name,
// look, theme, modules, libraries, details, livetv, games, discs, people, remote, ready. This page's steps are named
// for what they hold; this is which of the server's each one is, so the server can say where setup was left.
import type { StepId } from './plan'

export const SERVER_STEP: Record<StepId, string> = {
  welcome: 'welcome', account: 'account', look: 'look', theme: 'theme', modules: 'modules', media: 'libraries',
  details: 'details', logos: 'details', music: 'details', listings: 'details', tv: 'livetv', games: 'games', discs: 'discs', done: 'ready',
}

// The server steps that are behind this one, in the server's order, for `done`.
const ORDER = ['welcome', 'account', 'name', 'look', 'theme', 'modules', 'libraries', 'details', 'livetv', 'games', 'discs', 'people', 'remote', 'ready']
export const doneBefore = (id: StepId): string[] => { const i = ORDER.indexOf(SERVER_STEP[id]); return i > 0 ? ORDER.slice(0, i) : [] }
