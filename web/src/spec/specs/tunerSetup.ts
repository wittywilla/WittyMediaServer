// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { choice, mk, secF, secN, secR } from '../model.ts'
import type { SpecField, SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Settings, Channels and tuners: adding live TV (canvas "tuner"), step by step inside its dialog. Each step is a page
// body: the page's own Dialog keeps the title and the Back/Next buttons. Field ids are f:<driver field key>, "name",
// "guide", "pick"; row ids kind:<id> and found:<index>; action ids as the dialog handles them.
export interface KindLine { id: string; label: string; hint: string; usable: boolean; dim?: string }
export interface DriverLine { id: string; name: string; available: boolean; planned?: boolean; note?: string; description: string; statusNote?: string }
export interface DriverFieldLine { key: string; label: string; kind: 'text' | 'url' | 'path' | 'password' | 'select' | 'number' | 'textarea' | 'toggle'; help?: string; default?: string; required?: boolean; options?: string[] }
export interface FoundLine { name: string; detail: string }
export interface ProblemLine { message: string; hint?: string }

const plural = (n: number, one: string) => `${n} ${n === 1 ? one : one + 's'}`

// Step 1: what the person has.
export function kindSpec(kinds: KindLine[], chosen: string | null): SpecPageData {
  const rows: SpecRow[] = kinds.map((k) => ({ id: 'kind:' + k.id, title: k.label, sub: k.usable ? k.hint : `${k.hint} ${k.dim || 'Not on this kind of computer'}`, on: chosen === k.id, disabled: !k.usable }))
  return mk('tuner', 'Settings · Channels and tuners', 'What you have', '', [choice({ ...secR('What do you have?', rows) })])
}

export function toField(f: DriverFieldLine, value: string): SpecField {
  const base = { id: 'f:' + f.key, label: f.label + (f.required ? ' (needed)' : ''), hint: f.help }
  if (f.kind === 'toggle') return { ...base, kind: 'switch', value: (value || f.default) === '1' }
  if (f.kind === 'select') return { ...base, kind: 'select', value: value || f.default || '', options: (f.options || []).map((o) => ({ value: o, label: o || 'None' })) }
  if (f.kind === 'textarea') return { ...base, kind: 'textarea', value }
  if (f.kind === 'password') return { ...base, kind: 'password', value, autoComplete: 'new-password' }
  if (f.kind === 'number') return { ...base, kind: 'number', value }
  return { ...base, kind: 'text', value, autoComplete: 'off' }
}

export interface DetailsState {
  choices: DriverLine[]; driver: DriverLine | null; fields: DriverFieldLine[]; name: string; opts: Record<string, string>
  canSearch: boolean; searching: boolean; found: FoundLine[] | null; missing: string[]
  check: { ok: true; text: string; sample?: string } | { ok: false; problem: ProblemLine } | null; err: string
}

// Step 2: its details, found on the network or typed.
export function detailsSpec(s: DetailsState): SpecPageData {
  const sections: SpecSection[] = []
  if (s.choices.length > 1) {
    sections.push(secF('', [{ id: 'driver', kind: 'select', label: 'Which one', value: s.driver?.id || '', options: [{ value: '', label: 'Choose…' }, ...s.choices.map((d) => ({ value: d.id, label: d.name + (d.planned ? ' (not built yet)' : !d.available ? ' (not on this computer)' : '') }))] }]))
  }
  const unavailable = s.choices.filter((d) => (!d.available || d.planned) && d.note)
  if (unavailable.length) sections.push(secN('', unavailable.map((d) => `${d.name}: ${d.note}`).join(' ')))
  if (s.driver) {
    sections.push(secN('', [s.driver.description, s.driver.statusNote, s.driver.note].filter(Boolean).join(' ')))
    if (s.canSearch) {
      sections.push(secR('Search', [{ id: 'search', title: s.searching ? 'Searching…' : 'Search for it', sub: 'Or type its details below: anything the search doesn’t find can be added by hand.', disabled: s.searching }]))
    }
    if (s.found && s.found.length) sections.push(secR('Found', s.found.map((f, i) => ({ id: 'found:' + i, title: f.name, sub: f.detail, actions: [{ id: 'use:' + i, title: 'Use this one', label: 'Use ' + f.name }] }))))
    if (s.found && !s.found.length) sections.push(secN('', 'Nothing found on your network or this computer, or the search didn’t finish. Type its details below.'))
    sections.push(secF('Details', [{ id: 'name', kind: 'text', label: 'Name', value: s.name, placeholder: s.driver.name, autoFocus: true }, ...s.fields.map((f) => toField(f, s.opts[f.key] || ''))]))
    if (s.missing.length) sections.push(secN('', `Still needed: ${s.missing.join(', ')}.`))
    sections.push(secN('', 'No channel lists or stream addresses come with the server: bring your own. Check asks for its channels without saving anything, to see that the details are right.'))
  } else if (s.choices.length > 1) sections.push(secN('', 'Choose which one you have.'))
  if (s.check?.ok) sections.push(secN('It answered', s.check.text + (s.check.sample ? ` Such as ${s.check.sample}.` : ''), true))
  if (s.check && !s.check.ok) sections.push(secN('That didn’t work yet', [s.check.problem.message, s.check.problem.hint].filter(Boolean).join(' '), true))
  if (s.err) sections.push(secN('', s.err, true))
  return mk('tuner', 'Settings · Channels and tuners', 'Its details', '', sections)
}

export interface ChannelsState { error: string; found: string; encrypted: boolean; scanNote: string; canScan: boolean; scanning: boolean; percent: number; scanText: string; hasError: boolean; busy: boolean; zero: boolean }

// Step 3: its channels, read or scanned.
export function channelsSpec(s: ChannelsState): SpecPageData {
  const sections: SpecSection[] = []
  sections.push(secN('', s.error ? `It was added, but its channels couldn’t be read: ${s.error}` : s.found, !!s.error))
  if (s.encrypted) sections.push(secN('', 'Encrypted channels can’t be played here, which isn’t a fault in your setup.'))
  if (s.scanNote) sections.push(secN('', s.scanNote))
  const rows: SpecRow[] = []
  if (s.hasError) rows.push({ id: 'change', title: 'Remove it and change the details', disabled: s.busy || s.scanning })
  if (s.canScan) rows.push({ id: 'scan', title: s.scanning ? `Scanning… ${s.percent}%` : 'Scan for channels', on: s.zero, disabled: s.scanning })
  rows.push({ id: 'again', title: 'Read the list again', disabled: s.busy || s.scanning })
  sections.push(secR('Channels', rows))
  if (s.scanText) sections.push(secN('', s.scanText))
  return mk('tuner', 'Settings · Channels and tuners', 'Its channels', '', sections)
}

export interface TryState { options: { value: string; label: string }[] | null; pick: string; trying: boolean; answer: { ok: true; title: string; text: string } | { ok: false; title: string; problem: ProblemLine } | null }

// Step 4: one channel tried for a few seconds.
export function trySpec(s: TryState): SpecPageData {
  const sections: SpecSection[] = [secN('', 'This tunes one channel for a few seconds, as watching it would, to show that it plays. Nothing is recorded. It uses one of the device’s tuners while it runs, and can take up to a minute.')]
  if (s.options && s.options.length) sections.push(secF('', [{ id: 'pick', kind: 'select', label: 'Channel', value: s.pick, options: [{ value: '', label: 'Let it choose' }, ...s.options] }]))
  if (s.trying) sections.push(secN('', 'Tuning in…'))
  if (s.answer?.ok) sections.push(secN(s.answer.title, s.answer.text, true))
  if (s.answer && !s.answer.ok) sections.push(secN(s.answer.title, [s.answer.problem.message, s.answer.problem.hint].filter(Boolean).join(' '), true))
  return mk('tuner', 'Settings · Channels and tuners', 'Try a channel', '', sections)
}

// Step 5: guide data.
export function guideSpec(url: string, err: string): SpecPageData {
  const sections: SpecSection[] = [
    secN('', 'Listings for these channels come from an XMLTV guide, which many providers offer beside their playlist. Without one, the guide shows the channels with no programmes. You can add one later under Guide data.'),
    secF('', [{ id: 'guide', kind: 'text', label: 'XMLTV address, or a file on the server', value: url, autoFocus: true }]),
  ]
  if (err) sections.push(secN('', err, true))
  return mk('tuner', 'Settings · Channels and tuners', 'Guide data', '', sections)
}

export const doneText = (tuner: { channels: number; lastError: string } | null) =>
  `${tuner && !tuner.lastError ? `${plural(tuner.channels, 'channel')} ${tuner.channels === 1 ? 'is' : 'are'} in the guide.` : 'You can read its channels again from its page.'}`
