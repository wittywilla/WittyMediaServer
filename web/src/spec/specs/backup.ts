// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { anchored, mk, secF, secN, secP } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'

// Settings, Backup (canvas page "backup"): making one now (makeBackupSpec) and the schedule (scheduleSpec). The job's
// progress, the files list and restoring are the page's own. Field ids: part:<id>, protect, password, again,
// freq (chips freq:<off|daily|weekly>), weekday, hour, keep, current, code. Chip ids: make, save, changepw, clearpw.
export interface BackupPart { id: string; label: string; desc: string; needs?: string[] }
export const sizeWords = (n = 0) => (n >= 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(n < 10 << 20 ? 1 : 0)} MB` : n >= 1024 ? `${Math.round(n / 1024)} KB` : `${n} bytes`)
export const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

export function partFields(parts: BackupPart[], chosen: ReadonlySet<string>, sizes?: Record<string, number> | null): SpecField[] {
  return parts.map((p) => {
    const needs = (p.needs || []).map((id) => parts.find((x) => x.id === id)?.label).filter(Boolean)
    const known = sizes?.[p.id]
    const size = known === undefined ? '' : known > 0 ? ` About ${sizeWords(known)}.` : ' Empty.'
    return { id: 'part:' + p.id, kind: 'switch' as const, label: p.label, value: chosen.has(p.id), hint: `${p.desc}${needs.length ? ` Needs ${needs.join(' and ')} too.` : ''}${size}` }
  })
}

// Turning a part on brings what it needs; turning it off takes with it whatever needs it.
export function flipPart(parts: BackupPart[], chosen: ReadonlySet<string>, id: string, on: boolean): Set<string> {
  const n = new Set(chosen)
  const p = parts.find((x) => x.id === id)
  if (!p) return n
  if (on) { n.add(id); (p.needs || []).forEach((x) => n.add(x)) } else { n.delete(id); parts.filter((o) => (o.needs || []).includes(id)).forEach((o) => n.delete(o.id)) }
  return n
}

const pwFields = (password: string, again: string, min = 8): SpecField[] => [
  { id: 'password', kind: 'password', label: 'Password', value: password, autoComplete: 'new-password', hint: `At least ${min} characters. If you lose it, nobody can open the backup.`, error: password !== '' && password.length < min ? 'That’s too short.' : undefined },
  { id: 'again', kind: 'password', label: 'The same again', value: again, autoComplete: 'new-password', error: again !== '' && again !== password ? 'These aren’t the same.' : undefined },
]

export interface MakeView { parts: BackupPart[]; chosen: ReadonlySet<string>; sizes: Record<string, number> | null; protect: boolean; password: string; again: string; ready: boolean; busy: boolean }
export const MAKE = 'Make the backup'
export function makeBackupSpec(v: MakeView): SpecPageData {
  const total = v.sizes ? [...v.chosen].reduce((n, id) => n + (v.sizes![id] || 0), 0) : null
  const s: SpecSection[] = [
    secN('What to include', 'Your media files are never in a backup, however large or small it is. Neither are modules, game cores, BIOS files, logs or sign-ins.'),
    secF('', partFields(v.parts, v.chosen, v.sizes)),
    secN('', total === null ? 'Working out how large this is…' : `About ${sizeWords(total)} before compression.`),
    secN('Protect it with a password', 'Real encryption: AES-256, with the key made from your password. It also proves nobody has changed the backup.'),
    secF('', [{ id: 'protect', kind: 'switch', label: 'Protect with a password', value: v.protect, hint: v.protect ? 'Everything comes back: API keys, two-factor sign-in, share links and this server’s identity.' : undefined }, ...(v.protect ? pwFields(v.password, v.again) : [])]),
  ]
  if (!v.protect) s.push(secN('Without a password, secrets are left out', 'API keys, two-factor secrets, share links and logins in addresses aren’t in the file, so you add them again after a restore. Account passwords are in it as hashes, and titles and file names can be read: keep it private. Anything can be changed in a file with no password, so a restore also leaves out settings holding web addresses.', true))
  s.push(secP(MAKE, [{ id: 'make', title: 'Make a backup now', on: true, disabled: !v.ready }]))
  return mk('backup.make', 'Settings · Backup', 'Backup', '', s)
}

export interface ScheduleView {
  parts: BackupPart[]; chosen: ReadonlySet<string>; frequency: 'off' | 'daily' | 'weekly'; weekday: number; hour: number; keep: number; fewer: boolean; twoFactor: boolean
  current: string; code: string; protectedNow: boolean; changePw: boolean; password: string; again: string
  lastMade: string; lastError: string; busy: boolean; badPw: boolean
}
export function scheduleSpec(v: ScheduleView): SpecPageData {
  const on = v.frequency !== 'off'
  const s: SpecSection[] = [
    anchored(secP('Back up by itself', (['off', 'daily', 'weekly'] as const).map((f) => ({ id: 'freq:' + f, title: f === 'off' ? 'Off' : f === 'daily' ? 'Every day' : 'Every week', on: v.frequency === f }))), 'back-up-by-itself'),
    secN('', 'The backup is made in the background at the time you choose. If the server was off, it’s made soon after it next starts.'),
  ]
  if (on) {
    const f: SpecField[] = []
    if (v.frequency === 'weekly') f.push({ id: 'weekday', kind: 'select', label: 'On', value: String(v.weekday), options: WEEKDAYS.map((d, i) => ({ value: String(i), label: d })) })
    f.push({ id: 'hour', kind: 'select', label: 'At', value: String(v.hour), options: Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` })) })
    f.push({ id: 'keep', kind: 'number', label: 'Keep the newest', value: v.keep, min: 1, max: 365, hint: 'Older scheduled backups are deleted. Ones you make yourself never are.' })
    if (v.fewer) {
      f.push({ id: 'current', kind: 'password', label: 'Your password', value: v.current, autoComplete: 'current-password', hint: 'Keeping fewer deletes backups at the next run, so this asks for your password.' })
      if (v.twoFactor) f.push({ id: 'code', kind: 'text', label: 'Code from your authenticator app', value: v.code, autoComplete: 'one-time-code' })
    }
    s.push(secF('', f))
  }
  if (v.lastMade) s.push(secN('', `Last made ${v.lastMade}.`))
  if (v.lastError) s.push(secN('The last scheduled backup failed', v.lastError, true))
  if (on) {
    s.push(secN('What to include', 'What each scheduled backup includes.'), secF('', partFields(v.parts, v.chosen)))
    s.push(secN('Password', 'This server keeps a key made from the password, not the password itself. It opens these backups, as anyone who has this server’s database could anyway.'))
    if (v.protectedNow && !v.changePw) s.push(secN('', 'Scheduled backups are protected.'), secP('', [{ id: 'changepw', title: 'Change the password' }, { id: 'clearpw', title: 'Remove it' }]))
    else {
      s.push(secF('', pwFields(v.password, v.again)))
      if (!v.protectedNow && !v.changePw && v.password === '') s.push(secN('', 'With no password, secrets are left out of these backups.'))
    }
  }
  s.push(secP('', [{ id: 'save', title: 'Save', on: true, disabled: v.busy || v.badPw || (on && v.chosen.size === 0) }]))
  return mk('backup.schedule', 'Settings · Backup', 'Back up by itself', '', s)
}
