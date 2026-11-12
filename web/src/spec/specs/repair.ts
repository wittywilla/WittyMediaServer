// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { anchored, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'

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
    ? secN('The repaired database is in use', `It took over on ${l.at}. The old file is kept in ${l.kept}.`, true)
    : secN('The repaired database couldn’t take over', `${l.error} The old database is still in use. Run the check again.`, true))
  s.push(anchored(secR('Check this server', [
    { title: 'Check a disc or a drive’s speed and health', value: 'WittyDiagnoser', to: '/settings/diagnoser', sub: 'Your media files are never changed or removed.' },
    ...(v.checked && !v.running ? [{ title: 'Last checked', value: v.checked }] : []),
  ]), 'check-this-server'))
  if (v.running) s.push(secN('', v.phase || 'Working…'))
  if (v.error) s.push(secN('', v.error, true))
  s.push(secP('', [
    { id: 'check', title: v.running === 'check' ? 'Checking…' : v.hasReport ? 'Check again' : 'Check this server', on: v.safeCount === 0, disabled: !!v.running },
    ...(v.safeCount > 0 && !v.running ? [{ id: 'allsafe', title: `Fix everything that’s safe (${v.safeCount})`, on: true }] : []),
  ]))
  if (v.restart) {
    s.push(secN('A restart finishes the repair', `The new database takes over when the server next starts, and anyone watching is interrupted. ${v.canRestart ? '' : 'This server can’t restart itself: stop it and start it again.'}`.trim(), true))
    s.push(secP('', [{ id: 'restart', title: 'Restart now' }]))
  }
  return mk('repair', 'Settings · Repair', 'Repair', 'Finds and fixes problems with your metadata, the database, modules and this install, and works from the command line too if the server won’t start. It only looks until you press a fix, and a safety copy is made first.', s)
}

export interface RepairResult { key: string; title: string; ok: boolean; done?: string; error?: string }
export function repairResultsSpec(results: RepairResult[], copy?: string): SpecPageData {
  const s: SpecSection[] = [secR('What was done', results.map((r) => ({ title: `${r.ok ? 'Fixed' : 'Not fixed'}: ${r.title}`, sub: (r.ok ? r.done : r.error) || undefined, on: !r.ok })))]
  if (copy) s.push(secN('', `Safety copy: ${copy}`))
  return mk('repair.done', 'Settings · Repair', 'What was done', '', s)
}

export interface RemapView {
  from: string; to: string; busy: boolean; twoFactor: boolean; password: string; code: string; error: string
  plan: { why?: string; applied?: boolean; copy?: string; roots: number; rows: number; checked: number; found: number; sample: string[]; notFound: string[]; canApply: boolean } | null
}
export function remapSpec(v: RemapView): SpecPageData {
  const s: SpecSection[] = [
    anchored(secN('Moved a drive?', 'If a library’s folder is now somewhere else, this changes where the server looks, and everything it knows about those titles stays: watch progress, matches and your edits. Your files aren’t touched.'), 'moved-a-drive'),
    secF('', [
      { id: 'from', kind: 'text', label: 'Where it was', value: v.from, placeholder: 'D:\\Movies', autoComplete: 'off' },
      { id: 'to', kind: 'text', label: 'Where it is now', value: v.to, placeholder: 'E:\\Movies', autoComplete: 'off' },
    ]),
  ]
  if (v.error && !v.plan) s.push(secN('', v.error, true))
  s.push(secP('', [{ id: 'look', title: 'See what would change', disabled: v.busy || !v.from.trim() || !v.to.trim() }]))
  const p = v.plan
  if (p) {
    if (p.why) s.push(secN('', p.why, true))
    if (p.applied) s.push(secN('', `Done. Safety copy: ${p.copy}`))
    if (!p.applied && p.roots > 0) s.push(secN('', `${p.roots === 1 ? 'One library folder changes' : `${p.roots} library folders change`}, and about ${p.rows.toLocaleString()} values in all.${p.checked > 0 ? ` It looked for ${p.checked.toLocaleString()} files, and ${p.found.toLocaleString()} are in the new place.` : ''}`))
    if (!p.applied && (p.sample.length || p.notFound.length)) s.push(secR('Some of the files', [...p.sample.map((x) => ({ title: 'Found', value: x })), ...p.notFound.map((x) => ({ title: 'Not found', value: x }))]))
    if (p.canApply && !p.applied) {
      const f: SpecField[] = [{ id: 'password', kind: 'password', label: 'Your password', value: v.password, autoComplete: 'current-password' }]
      if (v.twoFactor) f.push({ id: 'code', kind: 'text', label: 'Code from your authenticator app', value: v.code, autoComplete: 'one-time-code' })
      s.push(secF('', f))
      if (v.error) s.push(secN('', v.error, true))
      s.push(secP('', [{ id: 'move', title: 'Move it', on: true, disabled: v.busy || !v.password || (v.twoFactor && !v.code) }]))
    }
  }
  return mk('repair.remap', 'Settings · Repair', 'Moved a drive?', '', s)
}

export function copiesSpec(copies: string[]): SpecPageData {
  return mk('repair.copies', 'Settings · Repair', 'Safety copies', '', [secN('Safety copies', 'Made before a repair. The newest five are kept, in the backups folder inside the data folder. What a repair removed is in each one’s removed folder.'), secR('', copies.map((c) => ({ title: c })))])
}
