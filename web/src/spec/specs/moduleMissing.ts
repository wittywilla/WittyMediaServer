// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, Cc, mk, secN, secP } from '../model.ts'
import type { SpecAction, SpecPageData, SpecSection } from '../types.ts'

// "Module missing" (canvas page "modmissing"): what a page shows when the module behind it isn't on this server.
export interface MissingModule { id: string; name: string; summary?: string; unavailable?: string; phase?: string; done?: number; total?: number }
export interface MissingInput { ids: string[]; what?: string; admin: boolean; mods: MissingModule[] | null; adding: boolean; error: string }

export const INSTALL = 'install:'

export function moduleMissingSpec(i: MissingInput): SpecPageData {
  const name = i.mods?.[0]?.name || i.ids[0]
  const busy = i.mods?.find((m) => m.phase && m.phase !== 'idle' && m.phase !== 'failed')
  const noBuild = i.mods?.find((m) => m.unavailable)
  const sections: SpecSection[] = []
  for (const m of i.mods || []) if (m.summary) sections.push(secN('', `${m.name}: ${m.summary}`))
  if (!i.admin) sections.push(secN('', 'An administrator of this server can install it in Settings, Modules.', true))
  if (noBuild) sections.push(secN('', noBuild.unavailable || '', true))
  if (busy) sections.push(secN('', `Installing ${busy.name}${busy.total ? `: ${Math.round((busy.done || 0) * 100 / busy.total)}%` : '…'} There’s no need to restart anything.`, true))
  if (i.error) sections.push(secN('', i.error, true))
  const links = [{ ...Cc('What modules are'), id: 'help', to: '/help/module-store' }]
  if (i.admin) links.unshift({ ...Cc('Open the store'), id: 'store', to: '/settings/modules' })
  sections.push(secP('', links))
  const actions: SpecAction[] = []
  if (i.admin && !busy && !noBuild) {
    (i.mods || i.ids.map((id) => ({ id, name: id }))).forEach((m, n) => {
      const a = n === 0 ? A1(`Install ${m.name}`) : A2(`Install ${m.name}`)
      actions.push({ ...a, id: INSTALL + m.id, disabled: i.adding })
    })
  }
  return mk('modmissing', 'Module missing', i.what || `This needs ${name}`, `${name} isn’t on this server, so this can’t open yet. Everything else works as usual.`, sections, actions)
}
