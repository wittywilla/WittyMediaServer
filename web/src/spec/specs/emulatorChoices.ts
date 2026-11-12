// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secR } from '../model.ts'
import type { SpecPageData, SpecRow } from '../types.ts'

// "Other ways to play this game" on a game's page (canvas key "emustand"): each standalone emulator that can run it,
// with its play link when ready, the reason when not, and its settings. Ids: play.<emulator> (a link),
// settings.<emulator>.
export interface EmulatorChoice { id: string; name: string; ready: boolean; why?: string; experimental?: boolean }

export function emulatorChoicesSpec(list: EmulatorChoice[], playTo: (id: string) => string): SpecPageData {
  const rows: SpecRow[] = list.map((e) => ({
    id: `play.${e.id}`, title: `Play with ${e.name}`,
    sub: !e.ready && e.why ? e.why : undefined, value: e.experimental ? 'Experimental' : undefined,
    ...(e.ready ? { to: playTo(e.id) } : { disabled: true }),
    actions: [{ id: `settings.${e.id}`, title: 'Emulator settings', label: `${e.name} settings` }],
  }))
  return mk('emustand', 'Game · Standalone', 'Other ways to play this game', '', [secR('', rows)])
}
