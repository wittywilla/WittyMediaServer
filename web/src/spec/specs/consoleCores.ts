// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { R, mk, secN, secR } from '../model.ts'
import type { SpecPageData } from '../types.ts'

// "Where <console> games run" on a console's page (canvas key "consoles"): one row per core, with whether it is
// installed, what this server can do with it and where it runs.
export interface CoreLine { id: string; name: string; installed: boolean; here: string; runs: string }

export function coresSpec(short: string, cores: CoreLine[]): SpecPageData {
  return mk('consoles', 'Games · Consoles', `Where ${short} games run`, '', [
    secR('', cores.map((c) => ({ ...R(c.name, c.installed ? 'Installed' : '', c.installed, `On this server: ${c.here} ${c.runs}`), id: `core.${c.id}` }))),
    secN('', 'Every device plays them: the server runs the game and streams it.'),
  ])
}
