// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, anchored, mk, secF, secN, secR } from '../model.ts'
import type { SpecAction, SpecPageData, SpecSection } from '../types.ts'
import { t, tk } from '../../sdui/words.ts'

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

// The section's heading (marked with tk: shown as t(NEWEST)).
export const NEWEST = tk('Newest version')

export function updatesSpec(v: UpdatesView): SpecPageData {
  const sections: SpecSection[] = [
    anchored(secR(t('Updates'), [{ title: t('Installed version'), value: v.current || '…', sub: v.checkedAt ? t('Last checked {time}', { time: v.checkedAt }) : t('Not checked yet'), on: true }]), 'updates'),
  ]
  if (v.withdrawn) sections.push(secN('', t('This version was withdrawn by the project: {reason}. Update as soon as a newer one is offered.', { reason: v.withdrawn }), true))
  if (v.checkError) sections.push(secN('', t('The last check didn’t work: {error}', { error: v.checkError }), true))
  if (v.upToDate) sections.push(secN('', t('You have the newest version.')))
  if (v.latest) sections.push(secR(t(NEWEST), [{ title: v.latest.title || t('Version {version}', { version: v.latest.version }), value: v.latest.version, sub: v.latest.meta, on: true }]))
  sections.push(secF(t('Automatic checks'), [{ id: 'auto', kind: 'switch', label: t('Check for updates automatically'), value: v.auto, hint: t('Twice a day the server asks GitHub for its list of releases and shows a dot on Settings when there is a newer one. Nothing about you or your library is sent, and nothing installs without you.') }]))
  const actions: SpecAction[] = []
  if (v.latest && v.method === 'apply' && !v.active) actions.push({ ...A1(t('Update now')), id: 'apply' })
  if (v.latest && v.method === 'installer' && !v.hasInstallerPhase) actions.push({ ...A1(t('Download the installer')), id: 'installer' })
  actions.push({ ...A2(v.checking ? t('Checking…') : t('Check now')), id: 'check', disabled: v.checking || v.active })
  return mk('updates', t('Settings · About'), t('Updates'), t('New versions of WittyMediaServer are published on GitHub. Each one is signed, and this server installs only releases whose signature and checksum it can verify.'), sections, actions)
}
