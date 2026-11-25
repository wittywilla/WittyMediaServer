// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, R, mk, secN, secR } from '../model.ts'
import type { SpecAction, SpecPageData } from '../types.ts'
import { t, tn } from '../../sdui/words.ts'

// Can't reach the server (canvas key "unreachable"): shown when a browser's first look at its own server fails,
// with what it is trying and a way forward. Action ids: retry, help.
export interface UnreachableIn { address: string; trying: boolean; seconds: number; tries: number }

export function unreachableSpec(d: UnreachableIn): SpecPageData {
  const lead = d.trying ? t('Trying again now.') : tn('Trying again in {count} second.', 'Trying again in {count} seconds.', d.seconds)
  const actions: SpecAction[] = [{ ...A1(t('Try again')), id: 'retry', disabled: d.trying }, { ...A2(t('Help'), '/help') }]
  return mk('unreachable', t('Every app'), t('Can’t reach the server'), lead, [
    secR(t('This server'), [R(t('Address'), d.address), R(t('Tried'), tn('Once', '{count} times', d.tries))]),
    secN(t('What to check'), t('That WittyMediaServer is running, and that this device can reach it: on the same network, or through Remote access. Nothing you did is lost.')),
  ], actions)
}
