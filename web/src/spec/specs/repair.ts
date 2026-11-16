// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { anchored, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'
import { t, tn } from '../../sdui/words.ts'

// Settings, Repair (canvas page "repair"). Four pieces, each its own SpecPage, with the page's finding cards between
// them: the check and what it found (repairTopSpec), what was done (repairResultsSpec), Moved a drive? (remapSpec)
// and the safety copies (copiesSpec). Chip ids: check, allsafe, restart, look, move. Field ids: from, to, password, code.
export interface RepairTop {
  lastRepair?: { ok: boolean; kept?: string; error?: string; at: string }
  running: string; phase: string; checked: string; error: string
  safeCount: number; hasReport: boolean
  restart: boolean; canRestart: boolean
}

export function repairTopSpec(v: RepairTop): SpecPageData {
  const s: SpecSection[] = []
  const l = v.lastRepair
  if (l) s.push(l.ok
    ? secN(t('The repaired database is in use'), t('It took over on {time}. The old file is kept in {path}.', { time: l.at, path: l.kept }), true)
    : secN(t('The repaired database couldn’t take over'), t('{error} The old database is still in use. Run the check again.', { error: l.error }), true))
  s.push(anchored(secR(t('Check this server'), [
    { title: t('Check a disc or a drive’s speed and health'), value: 'WittyDiagnoser', to: '/settings/diagnoser', sub: t('Your media files are never changed or removed.') },
    ...(v.checked && !v.running ? [{ title: t('Last checked'), value: v.checked }] : []),
  ]), 'check-this-server'))
  if (v.running) s.push(secN('', v.phase || t('Working…')))
  if (v.error) s.push(secN('', v.error, true))
  s.push(secP('', [
    { id: 'check', title: v.running === 'check' ? t('Checking…') : v.hasReport ? t('Check again') : t('Check this server'), on: v.safeCount === 0, disabled: !!v.running },
    ...(v.safeCount > 0 && !v.running ? [{ id: 'allsafe', title: t('Fix everything that’s safe ({count})', { count: v.safeCount }), on: true }] : []),
  ]))
  if (v.restart) {
    s.push(secN(t('A restart finishes the repair'), v.canRestart
      ? t('The new database takes over when the server next starts, and anyone watching is interrupted.')
      : t('The new database takes over when the server next starts, and anyone watching is interrupted. This server can’t restart itself: stop it and start it again.'), true))
    s.push(secP('', [{ id: 'restart', title: t('Restart now') }]))
  }
  return mk('repair', t('Settings · Repair'), t('Repair'), t('Finds and fixes problems with your metadata, the database, modules and this install, and works from the command line too if the server won’t start. It only looks until you press a fix, and a safety copy is made first.'), s)
}

export interface RepairResult { key: string; title: string; ok: boolean; done?: string; error?: string }
export function repairResultsSpec(results: RepairResult[], copy?: string): SpecPageData {
  const s: SpecSection[] = [secR(t('What was done'), results.map((r) => ({ title: r.ok ? t('Fixed: {title}', { title: r.title }) : t('Not fixed: {title}', { title: r.title }), sub: (r.ok ? r.done : r.error) || undefined, on: !r.ok })))]
  if (copy) s.push(secN('', t('Safety copy: {path}', { path: copy })))
  return mk('repair.done', t('Settings · Repair'), t('What was done'), '', s)
}

export interface RemapView {
  from: string; to: string; busy: boolean; twoFactor: boolean; password: string; code: string; error: string
  plan: { why?: string; applied?: boolean; copy?: string; roots: number; rows: number; checked: number; found: number; sample: string[]; notFound: string[]; canApply: boolean } | null
}
export function remapSpec(v: RemapView): SpecPageData {
  const s: SpecSection[] = [
    anchored(secN(t('Moved a drive?'), t('If a library’s folder is now somewhere else, this changes where the server looks, and everything it knows about those titles stays: watch progress, matches and your edits. Your files aren’t touched.')), 'moved-a-drive'),
    secF('', [
      { id: 'from', kind: 'text', label: t('Where it was'), value: v.from, placeholder: 'D:\\Movies', autoComplete: 'off' },
      { id: 'to', kind: 'text', label: t('Where it is now'), value: v.to, placeholder: 'E:\\Movies', autoComplete: 'off' },
    ]),
  ]
  if (v.error && !v.plan) s.push(secN('', v.error, true))
  s.push(secP('', [{ id: 'look', title: t('See what would change'), disabled: v.busy || !v.from.trim() || !v.to.trim() }]))
  const p = v.plan
  if (p) {
    if (p.why) s.push(secN('', p.why, true))
    if (p.applied) s.push(secN('', t('Done. Safety copy: {path}', { path: p.copy })))
    if (!p.applied && p.roots > 0) {
      const folders = tn('One library folder changes', '{count} library folders change', p.roots)
      const what = t('{folders}, and about {count} values in all.', { folders, count: p.rows })
      s.push(secN('', p.checked > 0 ? what + ' ' + t('It looked for {checked} files, and {found} are in the new place.', { checked: p.checked, found: p.found }) : what))
    }
    if (!p.applied && (p.sample.length || p.notFound.length)) s.push(secR(t('Some of the files'), [...p.sample.map((x) => ({ title: t('Found'), value: x })), ...p.notFound.map((x) => ({ title: t('Not found'), value: x }))]))
    if (p.canApply && !p.applied) {
      const f: SpecField[] = [{ id: 'password', kind: 'password', label: t('Your password'), value: v.password, autoComplete: 'current-password' }]
      if (v.twoFactor) f.push({ id: 'code', kind: 'text', label: t('Code from your authenticator app'), value: v.code, autoComplete: 'one-time-code' })
      s.push(secF('', f))
      if (v.error) s.push(secN('', v.error, true))
      s.push(secP('', [{ id: 'move', title: t('Move it'), on: true, disabled: v.busy || !v.password || (v.twoFactor && !v.code) }]))
    }
  }
  return mk('repair.remap', t('Settings · Repair'), t('Moved a drive?'), '', s)
}

export function copiesSpec(copies: string[]): SpecPageData {
  return mk('repair.copies', t('Settings · Repair'), t('Safety copies'), '', [secN(t('Safety copies'), t('Made before a repair. The newest five are kept, in the backups folder inside the data folder. What a repair removed is in each one’s removed folder.')), secR('', copies.map((c) => ({ title: c })))])
}
