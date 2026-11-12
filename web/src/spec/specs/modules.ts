// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secP } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// Settings, Modules (canvas page "modupdates"): the update settings and the filter. The store's update-all panel, the
// policy note and the module rows themselves are the page's own, under the sections named below.
// Field ids: autoCheck, autoInstall. Chip ids: check, filter:<id>.
export interface ModulesTop {
  autoCheck: boolean; autoInstall: boolean; loaded: boolean
  checkedAt: string; held: string; checkError: string; checking: boolean
  server: string; api: string
  filters: { id: string; label: string; count: number }[]; filter: string
}
export const UPDATES = 'Updates'

export function modulesTopSpec(v: ModulesTop): SpecPageData {
  const s: SpecSection[] = [
    secN(UPDATES, 'Some modules can update without a new server. Each update is signed, this server installs only what it can verify, and it takes effect straight away, with no restart.'),
    secF('', [
      { id: 'autoCheck', kind: 'switch', label: 'Look for updates automatically', value: v.autoCheck, disabled: !v.loaded, hint: `Looks online twice a day. When this is off, the server only looks when you press Check now.${v.checkedAt ? ` Last checked ${v.checkedAt}.` : ' It hasn’t looked yet.'}` },
      { id: 'autoInstall', kind: 'switch', label: 'Install updates automatically', value: v.autoInstall, disabled: !v.loaded || !v.autoCheck, hint: 'Installs what the check finds, if it works with this server. A version that fails is taken back out by itself. Pinned modules and forks are left alone.' },
    ]),
  ]
  if (v.held) s.push(secN('', `GitHub asked this network to wait, so the server won’t ask again until ${v.held}. Everything keeps working as it is.`))
  if (v.checkError) s.push(secN('', `The last check didn’t work: ${v.checkError} Everything keeps working as it is.`, true))
  s.push(secN('', `This server is version ${v.server} and has module API ${v.api}.`))
  s.push(secP('', [{ id: 'check', title: v.checking ? 'Checking…' : 'Check now', disabled: v.checking }]))
  s.push(secP('Show', v.filters.map((f) => ({ id: 'filter:' + f.id, title: f.id === 'all' ? f.label : `${f.label} · ${f.count}`, on: v.filter === f.id }))))
  return mk('modules', 'Settings · Modules', 'Modules', 'What this server can do, one piece at a time. Add what you want and remove what you don’t.', s)
}
