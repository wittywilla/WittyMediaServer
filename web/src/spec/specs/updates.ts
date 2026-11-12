// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, anchored, mk, secF, secN, secR } from '../model.ts'
import type { SpecAction, SpecPageData, SpecSection } from '../types.ts'

// Settings, About, Updates (canvas page "modupdates" in spirit): what is installed, what is newest, the one way this
// copy updates, and the automatic check. Ids: check, apply, installer (actions), auto (field). The release notes, the
// installer steps and the progress bar are the page's own and sit under the "Newest version" section.
export interface UpdatesView {
  current: string
  checkedAt: string // already formatted, or ''
  withdrawn: string
  checkError: string
  upToDate: boolean
  latest: { title: string; version: string; meta: string } | null
  method: string // apply | installer | docker | ...
  hasInstallerPhase: boolean
  active: boolean
  checking: boolean
  auto: boolean
}

export const NEWEST = 'Newest version'

export function updatesSpec(v: UpdatesView): SpecPageData {
  const sections: SpecSection[] = [
    anchored(secR('Updates', [{ title: 'Installed version', value: v.current || '…', sub: v.checkedAt ? `Last checked ${v.checkedAt}` : 'Not checked yet', on: true }]), 'updates'),
  ]
  if (v.withdrawn) sections.push(secN('', `This version was withdrawn by the project: ${v.withdrawn}. Update as soon as a newer one is offered.`, true))
  if (v.checkError) sections.push(secN('', `The last check didn’t work: ${v.checkError}`, true))
  if (v.upToDate) sections.push(secN('', 'You have the newest version.'))
  if (v.latest) sections.push(secR(NEWEST, [{ title: v.latest.title || `Version ${v.latest.version}`, value: v.latest.version, sub: v.latest.meta, on: true }]))
  sections.push(secF('Automatic checks', [{ id: 'auto', kind: 'switch', label: 'Check for updates automatically', value: v.auto, hint: 'Twice a day the server asks GitHub for its list of releases and shows a dot on Settings when there is a newer one. Nothing about you or your library is sent, and nothing installs without you.' }]))
  const actions: SpecAction[] = []
  if (v.latest && v.method === 'apply' && !v.active) actions.push({ ...A1('Update now'), id: 'apply' })
  if (v.latest && v.method === 'installer' && !v.hasInstallerPhase) actions.push({ ...A1('Download the installer'), id: 'installer' })
  actions.push({ ...A2(v.checking ? 'Checking…' : 'Check now'), id: 'check', disabled: v.checking || v.active })
  return mk('updates', 'Settings · About', 'Updates', 'New versions of WittyMediaServer are published on GitHub. Each one is signed, and this server installs only releases whose signature and checksum it can verify.', sections, actions)
}
