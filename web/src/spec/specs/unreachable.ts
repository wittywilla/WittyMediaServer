// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, R, mk, secN, secR } from '../model.ts'
import type { SpecAction, SpecPageData } from '../types.ts'

// Can't reach the server (canvas key "unreachable"): shown when a browser's first look at its own server fails,
// with what it is trying and a way forward. Action ids: retry, help.
export interface UnreachableIn { address: string; trying: boolean; seconds: number; tries: number }

export function unreachableSpec(d: UnreachableIn): SpecPageData {
  const lead = d.trying ? 'Trying again now.' : `Trying again in ${d.seconds} ${d.seconds === 1 ? 'second' : 'seconds'}.`
  const actions: SpecAction[] = [{ ...A1('Try again'), id: 'retry', disabled: d.trying }, { ...A2('Help', '/help') }]
  return mk('unreachable', 'Every app', 'Can’t reach the server', lead, [
    secR('This server', [R('Address', d.address), R('Tried', d.tries === 1 ? 'Once' : `${d.tries} times`)]),
    secN('What to check', 'That WittyMediaServer is running, and that this device can reach it: on the same network, or through Remote Access. Nothing you did is lost.'),
  ], actions)
}
