// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, mk, secF, secR } from '../model.ts'
import type { SpecPageData, SpecRow } from '../types.ts'

// WittyDiagnoser (canvas page "diag"), as the three parts of Settings, Diagnoser that are controls and lists.
// Field and row ids are what the page answers in onField and onAction.

export interface DiscChoiceState { path: string; knownDiscs: { title: string; path: string }[]; crawl: boolean; damage: boolean; busy: boolean; error: string; browsing: boolean }

export function discSpec(s: DiscChoiceState): SpecPageData {
  const known = s.knownDiscs
  return mk('diag-disc', 'Settings · Diagnoser', 'Check a disc', 'A disc image (.iso, or a CD’s .cue), or a folder holding VIDEO_TS or BDMV, from your libraries or the data folder.', [
    secR('Which disc', [
      { id: 'browse', title: s.browsing ? 'Close the browser' : 'Browse for a disc', value: s.path ? baseName(s.path) : 'Nothing chosen yet', sub: s.path || undefined, on: !!s.path },
    ]),
    ...(known.length ? [secF('', [{ id: 'known', kind: 'select' as const, label: 'Discs in your libraries', value: known.some((d) => d.path === s.path) ? s.path : '', options: [{ value: '', label: 'Choose one…' }, ...known.map((d) => ({ value: d.path, label: d.title }))] }])] : []),
    secF('Options', [
      { id: 'crawl', kind: 'switch', label: 'Press every menu button', value: s.crawl, hint: 'Walks through every menu the disc has and reports the buttons that lead nowhere. Usually a minute or two, and it stops by itself after 10 minutes.' },
      { id: 'damage', kind: 'switch', label: 'Check for damaged or unreadable sectors', value: s.damage, hint: 'Reads everything on the disc from start to end: a few minutes for a DVD, and 20 minutes or more for a Blu-ray. It gives way to anyone watching something, and you can leave this page while it runs.' },
    ]),
    ...(s.error ? [{ kind: 'note' as const, h: '', text: s.error, tone: true }] : []),
  ], [{ ...A1(s.busy ? 'Starting…' : 'Start the check'), id: 'start', disabled: !s.path || s.busy }])
}

export interface StorageState { folders: { id: string; name: string; path: string; group: 'library' | 'data' }[]; chosen: ReadonlySet<string>; walk: boolean; readTest: boolean; writeTest: boolean; busy: boolean; error: string }

export function storageSpec(s: StorageState): SpecPageData {
  const row = (f: StorageState['folders'][number]): SpecRow => ({ id: 'dir:' + f.id, title: f.name, sub: f.path, value: s.chosen.has(f.id) ? 'Chosen' : '', on: s.chosen.has(f.id) })
  const lib = s.folders.filter((f) => f.group === 'library'), data = s.folders.filter((f) => f.group === 'data')
  return mk('diag-storage', 'Settings · Diagnoser', 'Check storage', 'Free space, what kind of drive it is, whether the server can read each folder, how fast it reads, and the drive’s health where the system tells. Nothing is changed or repaired.', [
    ...(lib.length ? [secR('Library folders', lib.map(row))] : []),
    secR('The server’s own folders', data.map(row)),
    secF('Options', [
      { id: 'walk', kind: 'switch', label: 'Go through every file', value: s.walk, hint: 'Looks for files that can’t be opened, empty files, and names and paths that cause trouble. Up to 2 minutes for each folder.' },
      { id: 'read', kind: 'switch', label: 'Measure read speed', value: s.readTest, hint: 'Reads for up to 15 seconds from the largest files in each folder, and says how many streams it could keep up with.' },
      { id: 'write', kind: 'switch', label: 'Try a small write', value: s.writeTest, hint: 'Writes a 32 MB temporary file, reads it back and removes it. Only ever in the server’s own folders, and never in or around a library.' },
    ]),
    ...(s.error ? [{ kind: 'note' as const, h: '', text: s.error, tone: true }] : []),
  ], [
    { ...A1(s.busy ? 'Starting…' : 'Start the check'), id: 'start', disabled: s.chosen.size === 0 || s.busy },
    { title: s.chosen.size === s.folders.length ? 'Clear all' : 'Choose all', kind: 'secondary', id: 'all' },
  ])
}

export interface PastJob { id: string; kind: 'disc' | 'storage'; subject: string; running: boolean; state: string; worst?: string; finished?: string }

export function pastSpec(jobs: PastJob[], current: string, worstWords: Record<string, string>): SpecPageData {
  const rows: SpecRow[] = jobs.map((j) => ({
    id: 'job:' + j.id, title: j.subject, on: j.id === current,
    value: j.running ? 'Running' : j.state === 'cancelled' ? 'Stopped' : j.worst ? worstWords[j.worst] || j.worst : 'Finished',
    sub: j.finished,
  }))
  return mk('diag-past', 'Settings · Diagnoser', 'Past reports', 'The last 20 are kept on the server, and the newest few go in the log download with names hidden.',
    rows.length ? [secR('Report', rows)] : [{ kind: 'note', h: '', text: 'No reports yet. When a check finishes, its report stays here.' }])
}

const baseName = (p: string) => p.split(/[\\/]/).filter(Boolean).pop() || p
