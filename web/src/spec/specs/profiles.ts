// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF } from '../model.ts'
import type { SpecField, SpecPageData } from '../types.ts'
import { t, tk, tn } from '../../sdui/words.ts'

// The parts of a profile's limits (Settings, People; canvas "profiles" and "parentreport"): what it may do, an age for
// each kind of media, how long each day, the parents' PIN. Everything here is only the form: the server applies limits.
// Marked with tk: show them with t(KIND_LABEL[kind]).
export const KIND_LABEL: Record<string, string> = { film: tk('Films and videos'), tv: tk('TV shows'), game: tk('Games'), music: tk('Music and audio'), book: tk('Books') }

// Labels and hints are marked with tk: shown with t().
export const FEATURES: { id: string; label: string; hint: string; adultOnly?: boolean; kidsLabel?: string }[] = [
  { id: 'games', label: tk('Games'), hint: tk('Play games here or on a game PC. Off hides game libraries too.') },
  { id: 'recordings', label: tk('Recording Live TV'), hint: tk('Record programmes and keep recording rules.') },
  { id: 'downloads', label: tk('Downloads'), hint: tk('Save a copy of a photo or video to their device.') },
  { id: 'sharing', label: tk('Share links'), hint: tk('Make links that open a photo, video or album without signing in.'), adultOnly: true },
  { id: 'comments', label: tk('Comments'), hint: tk('Read and write comments.'), kidsLabel: tk('Read comments. Kids and Guest profiles never write them.') },
  { id: 'syncplay', label: tk('Watch parties'), hint: tk('Start a watch party, or join one with its code. They only ever see what their limits allow.') },
  { id: 'remoteControl', label: tk('Playing on other devices'), hint: tk('Play on and steer other devices. Off, their own devices can still be played on by whoever is allowed to.') },
  { id: 'remote', label: tk('Use away from home'), hint: tk('Off keeps this profile to your home network: it is signed out everywhere else.') },
  { id: 'editProfile', label: tk('Change their own name and picture'), hint: tk('Their username and password stay with whoever manages them.') },
  { id: 'editLayout', label: tk('Rearrange pages for themselves'), hint: tk('Reorder, hide and pin the rows of Home and other pages, and choose card sizes. Off shows every page as you set it up.') },
]

// One switch each; field ids are "feature:<id>". Share links are for grown-up accounts whatever is chosen.
export function featuresSpec(off: readonly string[], kind: string): SpecPageData {
  const grownUp = kind === 'adult'
  const items: SpecField[] = FEATURES.map((f) => {
    const never = !!f.adultOnly && !grownUp
    return { id: 'feature:' + f.id, kind: 'switch', label: t(f.label), hint: never ? t('Only for adult accounts.') : !grownUp && f.kidsLabel ? t(f.kidsLabel) : t(f.hint), disabled: never, value: !never && !off.includes(f.id) }
  })
  return mk('profiles', t('Settings · People'), t('What they may do'), '', [secF('', items)])
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
  const kindName = (kind: string) => (KIND_LABEL[kind] ? t(KIND_LABEL[kind]) : kind)
  for (const kind of kinds) {
    const m = mine[kind]
    const ages = agesFor(systems, kind)
    if (m && m.maxAge >= 0 && !ages.includes(m.maxAge)) ages.push(m.maxAge)
    ages.sort((a, b) => a - b)
    items.push({
      id: 'kind:' + kind, kind: 'select', label: kindName(kind), value: m ? String(m.maxAge) : 'same',
      options: [
        { value: 'same', label: t('The same as everything else') },
        ...ages.map((a) => {
          const w = allowedAt(systems, kind, a)
          const age = a === 0 ? t('For everyone') : t('Up to age {age}', { age: a })
          return { value: String(a), label: w ? t('{age} ({ratings})', { age, ratings: w }) : age }
        }),
        { value: '-1', label: t('No age limit') },
      ],
    })
    if (m && m.maxAge >= 0) items.push({ id: 'unrated:' + kind, kind: 'switch', label: t('{kind}: also those with no rating', { kind: kindName(kind) }), value: m.allowUnrated })
  }
  return mk('profiles', t('Settings · People'), t('Age for each kind'), '', [secF('', items)])
}

// Collections that are always or never allowed: one select each, id "col:<id>", value '', 'allow' or 'block'.
export function collectionRulesSpec(rows: { id: number; name: string; count: number }[], allow: readonly number[], block: readonly number[]): SpecPageData {
  return mk('profiles', t('Settings · People'), t('Collections'), '', [secF('', rows.map((c) => ({
    id: 'col:' + c.id, kind: 'select' as const, label: c.name, hint: tn('{count} title', '{count} titles', c.count),
    value: block.includes(c.id) ? 'block' : allow.includes(c.id) ? 'allow' : '',
    options: [{ value: '', label: t('By their rating') }, { value: 'allow', label: t('Always allow') }, { value: 'block', label: t('Never allow') }],
  })))])
}

const ALLOWANCES = [0, 30, 45, 60, 90, 120, 180, 240, 360]
export const allowanceLabel = (m: number) => m === 0 ? t('No limit') : m < 60 ? tn('{count} minute', '{count} minutes', m) : m % 60 === 0 ? tn('{count} hour', '{count} hours', m / 60) : t('{hours} h {minutes} min', { hours: Math.floor(m / 60), minutes: m % 60 })

// How long each day: ids "schoolDayMinutes" and "weekendMinutes" (the times of day are the page's own time fields).
export function allowanceSpec(schoolDay: number, weekend: number): SpecPageData {
  const sel = (id: string, label: string, hint: string, v: number): SpecField => {
    const opts = ALLOWANCES.includes(v) ? ALLOWANCES : [...ALLOWANCES, v].sort((a, b) => a - b)
    return { id, kind: 'select', label, hint, value: String(v), options: opts.map((m) => ({ value: String(m), label: allowanceLabel(m) })) }
  }
  return mk('profiles', t('Settings · People'), t('Time'), '', [secF('', [sel('schoolDayMinutes', t('School days'), t('Monday to Friday.'), schoolDay), sel('weekendMinutes', t('Weekends'), t('Saturday and Sunday.'), weekend)])])
}

// The parents' PIN (what a parent types on a child's device to give more time). Fields: pin, current, code.
// Actions: save (Set PIN or Change PIN) and remove.
export function parentPinSpec(o: { isSet: boolean; pin: string; current: string; code: string; twoFactor: boolean; error: string; busy: boolean }): SpecPageData {
  const ready = /^\d{4,12}$/.test(o.pin) && !!o.current && (!o.twoFactor || !!o.code)
  const fields: SpecField[] = [
    { id: 'pin', kind: 'pin', label: o.isSet ? t('New PIN') : t('PIN'), hint: t('4 to 12 digits.'), length: 12, value: o.pin },
    { id: 'current', kind: 'password', label: t('Your password'), value: o.current, autoComplete: 'current-password', error: o.error || undefined },
  ]
  if (o.twoFactor) fields.push({ id: 'code', kind: 'text', label: t('Code from your authenticator app'), value: o.code, autoComplete: 'one-time-code' })
  const actions: SpecPageData['actions'] = [{ id: 'save', title: o.isSet ? t('Change PIN') : t('Set PIN'), kind: 'primary', disabled: !ready || o.busy }]
  if (o.isSet) actions.push({ id: 'remove', title: t('Remove it'), kind: 'secondary', disabled: o.busy || !o.current })
  const lead = o.isSet
    ? t('A parent types this PIN on a child’s device to give more time, to change that profile’s screen time, or to leave a kids’ profile that stays on its device. Keep it from the children. A PIN is set.')
    : t('A parent types this PIN on a child’s device to give more time, to change that profile’s screen time, or to leave a kids’ profile that stays on its device. No PIN is set: a parent’s own username and password give more time instead, and kids’ profiles can be left freely.')
  return mk('profiles', t('Settings · People'), t('Parents’ PIN'), lead, [secF('', fields)], actions)
}
