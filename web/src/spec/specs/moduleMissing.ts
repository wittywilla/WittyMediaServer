// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, Cc, mk, secN, secP } from '../model.ts'
import type { SpecAction, SpecPageData, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// "Module missing" (canvas page "modmissing"): what a page shows when the module behind it isn't on this server.
export interface MissingModule { id: string; name: string; summary?: string; unavailable?: string; phase?: string; done?: number; total?: number }
export interface MissingInput { ids: string[]; what?: string; admin: boolean; mods: MissingModule[] | null; adding: boolean; error: string }

export const INSTALL = 'install:'

// What the page is still waiting for. A page that needs a reader and the kit it is built on names
// only what isn't in use: with the reader installed and the kit absent it is about the kit, and
// offers nothing for the reader. ready: everything it asked for is in use.
export function stillMissing<T extends { id: string; enabled?: boolean }>(ids: string[], modules: T[]): { gone: T[]; ready: boolean } {
  const list = ids.map((id) => modules.find((m) => m.id === id)).filter(Boolean) as T[]
  const gone = list.filter((m) => !m.enabled)
  return { gone, ready: !gone.length && list.length === ids.length }
}

export function moduleMissingSpec(i: MissingInput): SpecPageData {
  const name = i.mods?.[0]?.name || i.ids[0]
  const busy = i.mods?.find((m) => m.phase && m.phase !== 'idle' && m.phase !== 'failed')
  const noBuild = i.mods?.find((m) => m.unavailable)
  const sections: SpecSection[] = []
  for (const m of i.mods || []) if (m.summary) sections.push(secN('', `${m.name}: ${m.summary}`))
  if (!i.admin) sections.push(secN('', t('An administrator of this server can install it in Settings, Modules.'), true))
  if (noBuild) sections.push(secN('', noBuild.unavailable || '', true))
  if (busy) sections.push(secN('', busy.total ? t('Installing {name}: {percent}% There’s no need to restart anything.', { name: busy.name, percent: Math.round((busy.done || 0) * 100 / busy.total) }) : t('Installing {name}… There’s no need to restart anything.', { name: busy.name }), true))
  if (i.error) sections.push(secN('', i.error, true))
  const links = [{ ...Cc(t('What modules are')), id: 'help', to: '/help/module-store' }]
  if (i.admin) links.unshift({ ...Cc(t('Open the store')), id: 'store', to: '/settings/modules' })
  sections.push(secP('', links))
  const actions: SpecAction[] = []
  if (i.admin && !busy && !noBuild) {
    (i.mods || i.ids.map((id) => ({ id, name: id }))).forEach((m, n) => {
      const a = n === 0 ? A1(t('Install {name}', { name: m.name })) : A2(t('Install {name}', { name: m.name }))
      actions.push({ ...a, id: INSTALL + m.id, disabled: i.adding })
    })
  }
  return mk('modmissing', t('Module missing'), i.what || t('This needs {name}', { name }), t('{name} isn’t on this server, so this can’t open yet. Everything else works as usual.', { name }), sections, actions)
}
