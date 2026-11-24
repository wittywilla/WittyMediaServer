// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secP } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'
import { t, tk } from '../../sdui/words.ts'

// Settings, Module store (canvas page "modupdates"): the update settings and the filter. The store's update-all panel, the
// policy note and the module rows themselves are the page's own, under the sections named below.
// Field ids: autoCheck, autoInstall. Chip ids: check, filter:<id>. With no filters (the store's Updates tab) there's no Show row.
export interface ModulesTop {
  autoCheck: boolean; autoInstall: boolean; loaded: boolean
  checkedAt: string; held: string; checkError: string; checking: boolean
  server: string; api: string
  filters: { id: string; label: string; count: number }[]; filter: string
}
// The first section's heading (marked with tk: it is shown as t(UPDATES)).
export const UPDATES = tk('Updates')

export function modulesTopSpec(v: ModulesTop): SpecPageData {
  const s: SpecSection[] = [
    secN(t(UPDATES), t('Some modules can update without a new server. Each update is signed, this server installs only what it can verify, and it takes effect straight away, with no restart.')),
    secF('', [
      { id: 'autoCheck', kind: 'switch', label: t('Look for updates automatically'), value: v.autoCheck, disabled: !v.loaded, hint: v.checkedAt ? t('Looks online twice a day. When this is off, the server only looks when you press Check now. Last checked {time}.', { time: v.checkedAt }) : t('Looks online twice a day. When this is off, the server only looks when you press Check now. It hasn’t looked yet.') },
      { id: 'autoInstall', kind: 'switch', label: t('Install updates automatically'), value: v.autoInstall, disabled: !v.loaded || !v.autoCheck, hint: t('Installs what the check finds, if it works with this server. A version that fails is taken back out by itself. Pinned modules and forks are left alone.') },
    ]),
  ]
  if (v.held) s.push(secN('', t('GitHub asked this network to wait, so the server won’t ask again until {time}. Everything keeps working as it is.', { time: v.held })))
  if (v.checkError) s.push(secN('', t('The last check didn’t work: {error} Everything keeps working as it is.', { error: v.checkError }), true))
  s.push(secN('', t('This server is version {version} and has module API {api}.', { version: v.server, api: v.api })))
  s.push(secP('', [{ id: 'check', title: v.checking ? t('Checking…') : t('Check now'), disabled: v.checking }]))
  if (v.filters.length) s.push(secP(t('Show'), v.filters.map((f) => ({ id: 'filter:' + f.id, title: f.id === 'all' ? f.label : t('{filter} · {count}', { filter: f.label, count: f.count }), on: v.filter === f.id }))))
  return mk('modules', t('Settings · Modules'), t('Modules'), t('What this server can do, one piece at a time. Add what you want and remove what you don’t.'), s)
}
