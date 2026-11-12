// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF } from '../model.ts'
import type { SpecField, SpecPageData } from '../types.ts'

// The parts of a profile's limits (Settings, People; canvas "profiles" and "parentreport"): what it may do, an age for
// each kind of media, how long each day, the parents' PIN. Everything here is only the form: the server applies limits.
export const KIND_LABEL: Record<string, string> = { film: 'Films and videos', tv: 'TV shows', game: 'Games', music: 'Music and audio', book: 'Books' }

export const FEATURES: { id: string; label: string; hint: string; adultOnly?: boolean; kidsLabel?: string }[] = [
  { id: 'games', label: 'Games', hint: 'Play games here or on a game PC. Off hides game libraries too.' },
  { id: 'recordings', label: 'Recording Live TV', hint: 'Record programmes and keep recording rules.' },
  { id: 'downloads', label: 'Downloads', hint: 'Save a copy of a photo or video to their device.' },
  { id: 'sharing', label: 'Share links', hint: 'Make links that open a photo, video or album without signing in.', adultOnly: true },
  { id: 'comments', label: 'Comments', hint: 'Read and write comments.', kidsLabel: 'Read comments. Kids and Guest profiles never write them.' },
  { id: 'syncplay', label: 'Watch parties', hint: 'Start a watch party, or join one with its code. They only ever see what their limits allow.' },
  { id: 'remoteControl', label: 'Playing on other devices', hint: 'Play on and steer other devices. Off, their own devices can still be played on by whoever is allowed to.' },
  { id: 'remote', label: 'Use away from home', hint: 'Off keeps this profile to your home network: it is signed out everywhere else.' },
  { id: 'editProfile', label: 'Change their own name and picture', hint: 'Their username and password stay with whoever manages them.' },
  { id: 'editLayout', label: 'Rearrange pages for themselves', hint: 'Reorder, hide and pin the rows of Home and other pages, and choose card sizes. Off shows every page as you set it up.' },
]

// One switch each; field ids are "feature:<id>". Share links are for grown-up accounts whatever is chosen.
export function featuresSpec(off: readonly string[], kind: string): SpecPageData {
  const grownUp = kind === 'adult'
  const items: SpecField[] = FEATURES.map((f) => {
    const never = !!f.adultOnly && !grownUp
    return { id: 'feature:' + f.id, kind: 'switch', label: f.label, hint: never ? 'Only for adult accounts.' : !grownUp && f.kidsLabel ? f.kidsLabel : f.hint, disabled: never, value: !never && !off.includes(f.id) }
  })
  return mk('profiles', 'Settings · People', 'What they may do', '', [secF('', items)])
}

export interface RatingSystemLine { kinds: string[]; ratings: { label: string; age: number }[] }

// The ratings of each system that an age lets through, highest first: "PG-13, 12A, FSK 12".
export function allowedAt(systems: RatingSystemLine[], kind: string, age: number): string {
  const out: string[] = []
  for (const s of systems) {
    if (!s.kinds.includes(kind)) continue
    const ok = s.ratings.filter((r) => r.age <= age)
    if (ok.length) out.push(ok[ok.length - 1].label)
  }
  return [...new Set(out)].join(', ')
}

// The ages worth offering for a kind: each one where some system's next rating begins.
export function agesFor(systems: RatingSystemLine[], kind: string): number[] {
  const ages = new Set<number>()
  for (const s of systems) if (s.kinds.includes(kind)) for (const r of s.ratings) ages.add(r.age)
  return [...ages].sort((a, b) => a - b)
}

// An age for each kind of media; a kind left alone follows the limit above. Ids: "kind:<k>" (a select whose value is
// "same", an age, or "-1") and "unrated:<k>" (a switch, only where an age is set).
export function kindLimitsSpec(kinds: string[], systems: RatingSystemLine[], mine: Record<string, { maxAge: number; allowUnrated: boolean }>): SpecPageData {
  const items: SpecField[] = []
  for (const kind of kinds) {
    const m = mine[kind]
    const ages = agesFor(systems, kind)
    if (m && m.maxAge >= 0 && !ages.includes(m.maxAge)) ages.push(m.maxAge)
    ages.sort((a, b) => a - b)
    items.push({
      id: 'kind:' + kind, kind: 'select', label: KIND_LABEL[kind] || kind, value: m ? String(m.maxAge) : 'same',
      options: [
        { value: 'same', label: 'The same as everything else' },
        ...ages.map((a) => { const w = allowedAt(systems, kind, a); return { value: String(a), label: `${a === 0 ? 'For everyone' : `Up to age ${a}`}${w ? ` (${w})` : ''}` } }),
        { value: '-1', label: 'No age limit' },
      ],
    })
    if (m && m.maxAge >= 0) items.push({ id: 'unrated:' + kind, kind: 'switch', label: `${KIND_LABEL[kind] || kind}: also those with no rating`, value: m.allowUnrated })
  }
  return mk('profiles', 'Settings · People', 'Age for each kind', '', [secF('', items)])
}

// Collections that are always or never allowed: one select each, id "col:<id>", value '', 'allow' or 'block'.
export function collectionRulesSpec(rows: { id: number; name: string; count: number }[], allow: readonly number[], block: readonly number[]): SpecPageData {
  return mk('profiles', 'Settings · People', 'Collections', '', [secF('', rows.map((c) => ({
    id: 'col:' + c.id, kind: 'select' as const, label: c.name, hint: `${c.count} ${c.count === 1 ? 'title' : 'titles'}`,
    value: block.includes(c.id) ? 'block' : allow.includes(c.id) ? 'allow' : '',
    options: [{ value: '', label: 'By their rating' }, { value: 'allow', label: 'Always allow' }, { value: 'block', label: 'Never allow' }],
  })))])
}

const ALLOWANCES = [0, 30, 45, 60, 90, 120, 180, 240, 360]
export const allowanceLabel = (m: number) => m === 0 ? 'No limit' : m < 60 ? `${m} minutes` : m % 60 === 0 ? `${m / 60} ${m === 60 ? 'hour' : 'hours'}` : `${Math.floor(m / 60)} h ${m % 60} min`

// How long each day: ids "schoolDayMinutes" and "weekendMinutes" (the times of day are the page's own time fields).
export function allowanceSpec(schoolDay: number, weekend: number): SpecPageData {
  const sel = (id: string, label: string, hint: string, v: number): SpecField => {
    const opts = ALLOWANCES.includes(v) ? ALLOWANCES : [...ALLOWANCES, v].sort((a, b) => a - b)
    return { id, kind: 'select', label, hint, value: String(v), options: opts.map((m) => ({ value: String(m), label: allowanceLabel(m) })) }
  }
  return mk('profiles', 'Settings · People', 'Time', '', [secF('', [sel('schoolDayMinutes', 'School days', 'Monday to Friday.', schoolDay), sel('weekendMinutes', 'Weekends', 'Saturday and Sunday.', weekend)])])
}

// The parents' PIN (what a parent types on a child's device to give more time). Fields: pin, current, code.
// Actions: save (Set PIN or Change PIN) and remove.
export function parentPinSpec(o: { isSet: boolean; pin: string; current: string; code: string; twoFactor: boolean; error: string; busy: boolean }): SpecPageData {
  const ready = /^\d{4,12}$/.test(o.pin) && !!o.current && (!o.twoFactor || !!o.code)
  const fields: SpecField[] = [
    { id: 'pin', kind: 'pin', label: o.isSet ? 'New PIN' : 'PIN', hint: '4 to 12 digits.', length: 12, value: o.pin },
    { id: 'current', kind: 'password', label: 'Your password', value: o.current, autoComplete: 'current-password', error: o.error || undefined },
  ]
  if (o.twoFactor) fields.push({ id: 'code', kind: 'text', label: 'Code from your authenticator app', value: o.code, autoComplete: 'one-time-code' })
  const actions: SpecPageData['actions'] = [{ id: 'save', title: o.isSet ? 'Change PIN' : 'Set PIN', kind: 'primary', disabled: !ready || o.busy }]
  if (o.isSet) actions.push({ id: 'remove', title: 'Remove it', kind: 'secondary', disabled: o.busy || !o.current })
  return mk('profiles', 'Settings · People', 'Parents’ PIN', `When a profile’s screen time runs out, a parent can type this PIN on that device to give it more time today. It does nothing else, so it can be short. ${o.isSet ? 'A PIN is set.' : 'No PIN is set: a parent’s own username and password work instead.'}`, [secF('', fields)], actions)
}
