// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secP, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// A module's page in the store (canvas "modstore"): three of its sections. Notes, versions with their notes, the
// screenshots and the dialogs stay the page's own.

// What it asks for: one row per permission, or "No special permissions."
export function permissionsSpec(perms: { name: string; words: string }[]): SpecPageData {
  const s: SpecSection[] = perms.length ? [secR('', perms.map((p) => ({ title: p.name, sub: p.words })))] : [secR('', [{ title: t('No special permissions.') }])]
  return mk('modperms', t('Module'), t('What it asks for'), '', s)
}

// Details: the facts as rows, the modules it is built on as links.
export interface Detail { title: string; value: string }
export function detailsSpec(facts: Detail[], builtOn: { id: string; name: string; to: string }[]): SpecPageData {
  const s: SpecSection[] = [secR('', facts.map((f) => ({ title: f.title, value: f.value })))]
  if (builtOn.length) s.push(secP(t('Built on'), builtOn.map((b) => ({ id: 'on:' + b.id, title: b.name, to: b.to }))))
  return mk('moddetails', t('Module'), t('Details'), '', s)
}

// Manage: what can be done to it now. Ids: rollback, off, on, remove, official, log.
export interface Manage { installed: boolean; canRollback: boolean; previous?: string; core: boolean; enabled: boolean; builtin: boolean; usingFork: boolean; working: boolean }
export function manageSpec(m: Manage): SpecPageData {
  const c = []
  if (m.installed && m.canRollback) c.push({ id: 'rollback', title: m.previous ? t('Go back to version {version}', { version: m.previous }) : t('Stop using this version'), disabled: m.working })
  if (m.installed && !m.core && m.enabled) c.push({ id: 'off', title: t('Turn off'), disabled: m.working })
  if (m.installed && !m.enabled) c.push({ id: 'on', title: t('Turn on'), disabled: m.working })
  if (m.installed && !m.core && !m.builtin) c.push({ id: 'remove', title: t('Remove'), disabled: m.working })
  if (m.usingFork) c.push({ id: 'official', title: t('Go back to the official version'), disabled: m.working })
  c.push({ id: 'log', title: t('Show log') })
  return mk('modmanage', t('Module'), t('Manage'), '', [secP('', c)])
}
