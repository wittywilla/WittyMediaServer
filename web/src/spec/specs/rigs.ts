// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secN, secP, secR } from '../model.ts'
import type { SpecChip, SpecPageData, SpecSection } from '../types.ts'

// A gaming PC's status line on its library page and a game's page (canvas page "gamepcs"): the PC with its state,
// then what can be done to it: wake it, end what's running, pair it. Real data from GET /rigs. Ids: pc, wake, quit, pair.
export interface RigState { name: string; state: string; message: string; currentGame?: string; paired: boolean; canWake: boolean; canQuit: boolean; hasMac?: boolean }

const WORDS: Record<string, string> = { online: 'Online', busy: 'Online', asleep: 'Asleep', offline: 'Not reachable', unpaired: 'Not paired' }
const word = (state: string) => WORDS[state] || (state ? state.charAt(0).toUpperCase() + state.slice(1) : 'Unknown')

export function rigSpec(rig: RigState, o: { isAdmin: boolean; busy?: boolean; wakeLabel?: string }): SpecPageData {
  const running = rig.state === 'busy' && rig.currentGame ? `Running: ${rig.currentGame}` : ''
  const chips: SpecChip[] = []
  if (rig.canWake) chips.push({ ...Cc(o.wakeLabel || 'Wake it up'), id: 'wake', disabled: o.busy })
  if (rig.canQuit) chips.push({ ...Cc('End what’s running'), id: 'quit', disabled: o.busy })
  if (o.isAdmin && !rig.paired) chips.push({ ...Cc('Pair it'), id: 'pair', to: '/settings/couch' })
  const sections: SpecSection[] = [secR('', [{ ...R(rig.name, [word(rig.state), running].filter(Boolean).join(' · '), rig.state === 'online' || rig.state === 'busy', running ? '' : rig.message), id: 'pc' }])]
  if (chips.length) sections.push(secP('', chips))
  if (rig.state === 'asleep' && !rig.canWake && o.isAdmin && !rig.hasMac) sections.push(secN('', 'Waking it from here needs it to have been paired and on once.'))
  return mk('gamepcs', 'Game PCs', 'Your game PCs', 'Stream from a PC you own.', sections)
}
