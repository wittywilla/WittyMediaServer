// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { anchored, mk, secF, secN, secP } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'
import { t, tk, tn, tx } from '../../sdui/words.ts'

// Settings, Backup (canvas page "backup"): making one now (makeBackupSpec) and the schedule (scheduleSpec). The job's
// progress, the files list and restoring are the page's own. Field ids: part:<id>, protect, password, again,
// freq (chips freq:<off|daily|weekly>), weekday, hour, keep, current, code. Chip ids: make, save, changepw, clearpw.
export interface BackupPart { id: string; label: string; desc: string; needs?: string[] }
export const sizeWords = (n = 0) => (n >= 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(n < 10 << 20 ? 1 : 0)} MB` : n >= 1024 ? `${Math.round(n / 1024)} KB` : tn('{count} byte', '{count} bytes', n))
export const WEEKDAYS = [tk('Sunday'), tk('Monday'), tk('Tuesday'), tk('Wednesday'), tk('Thursday'), tk('Friday'), tk('Saturday')]

export function partFields(parts: BackupPart[], chosen: ReadonlySet<string>, sizes?: Record<string, number> | null): SpecField[] {
  return parts.map((p) => {
    const needs = (p.needs || []).map((id) => parts.find((x) => x.id === id)?.label).filter(Boolean).map((l) => tx(l))
    const known = sizes?.[p.id]
    const size = known === undefined ? '' : known > 0 ? ' ' + t('About {size}.', { size: sizeWords(known) }) : ' ' + t('Empty.')
    return { id: 'part:' + p.id, kind: 'switch' as const, label: tx(p.label), value: chosen.has(p.id), hint: tx(p.desc) + (needs.length ? ' ' + t('Needs {parts} too.', { parts: needs.join(t(' and ')) }) : '') + size }
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
  { id: 'password', kind: 'password', label: t('Password'), value: password, autoComplete: 'new-password', hint: t('At least {min} characters. If you lose it, nobody can open the backup.', { min }), error: password !== '' && password.length < min ? t('That’s too short.') : undefined },
  { id: 'again', kind: 'password', label: t('The same again'), value: again, autoComplete: 'new-password', error: again !== '' && again !== password ? t('These aren’t the same.') : undefined },
]

export interface MakeView { parts: BackupPart[]; chosen: ReadonlySet<string>; sizes: Record<string, number> | null; protect: boolean; password: string; again: string; ready: boolean; busy: boolean }
export const MAKE = tk('Make the backup')
export function makeBackupSpec(v: MakeView): SpecPageData {
  const total = v.sizes ? [...v.chosen].reduce((n, id) => n + (v.sizes![id] || 0), 0) : null
  const s: SpecSection[] = [
    secN(t('What to include'), t('Your media files are never in a backup, however large or small it is. Neither are modules, game cores, BIOS files, logs or sign-ins.')),
    secF('', partFields(v.parts, v.chosen, v.sizes)),
    secN('', total === null ? t('Working out how large this is…') : t('About {size} before compression.', { size: sizeWords(total) })),
    secN(t('Protect it with a password'), t('Real encryption: AES-256, with the key made from your password. It also proves nobody has changed the backup.')),
    secF('', [{ id: 'protect', kind: 'switch', label: t('Protect with a password'), value: v.protect, hint: v.protect ? t('Everything comes back: API keys, two-factor sign-in, share links and this server’s identity.') : undefined }, ...(v.protect ? pwFields(v.password, v.again) : [])]),
  ]
  if (!v.protect) s.push(secN(t('Without a password, secrets are left out'), t('API keys, two-factor secrets, share links and logins in addresses aren’t in the file, so you add them again after a restore. Account passwords are in it as hashes, and titles and file names can be read: keep it private. Anything can be changed in a file with no password, so a restore also leaves out settings holding web addresses.'), true))
  s.push(secP(t(MAKE), [{ id: 'make', title: t('Make a backup now'), on: true, disabled: !v.ready }]))
  return mk('backup.make', t('Settings · Backup'), t('Backup'), '', s)
}

export interface ScheduleView {
  parts: BackupPart[]; chosen: ReadonlySet<string>; frequency: 'off' | 'daily' | 'weekly'; weekday: number; hour: number; keep: number; fewer: boolean; twoFactor: boolean
  current: string; code: string; protectedNow: boolean; changePw: boolean; password: string; again: string
  lastMade: string; lastError: string; busy: boolean; badPw: boolean
}
export function scheduleSpec(v: ScheduleView): SpecPageData {
  const on = v.frequency !== 'off'
  const s: SpecSection[] = [
    anchored(secP(t('Back up by itself'), (['off', 'daily', 'weekly'] as const).map((f) => ({ id: 'freq:' + f, title: f === 'off' ? t('Off') : f === 'daily' ? t('Every day') : t('Every week'), on: v.frequency === f }))), 'back-up-by-itself'),
    secN('', t('The backup is made in the background at the time you choose. If the server was off, it’s made soon after it next starts.')),
  ]
  if (on) {
    const f: SpecField[] = []
    if (v.frequency === 'weekly') f.push({ id: 'weekday', kind: 'select', label: t('On'), value: String(v.weekday), options: WEEKDAYS.map((d, i) => ({ value: String(i), label: t(d) })) })
    f.push({ id: 'hour', kind: 'select', label: t('At'), value: String(v.hour), options: Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` })) })
    f.push({ id: 'keep', kind: 'number', label: t('Keep the newest'), value: v.keep, min: 1, max: 365, hint: t('Older scheduled backups are deleted. Ones you make yourself never are.') })
    if (v.fewer) {
      f.push({ id: 'current', kind: 'password', label: t('Your password'), value: v.current, autoComplete: 'current-password', hint: t('Keeping fewer deletes backups at the next run, so this asks for your password.') })
      if (v.twoFactor) f.push({ id: 'code', kind: 'text', label: t('Code from your authenticator app'), value: v.code, autoComplete: 'one-time-code' })
    }
    s.push(secF('', f))
  }
  if (v.lastMade) s.push(secN('', t('Last made {when}.', { when: v.lastMade })))
  if (v.lastError) s.push(secN(t('The last scheduled backup failed'), v.lastError, true))
  if (on) {
    s.push(secN(t('What to include'), t('What each scheduled backup includes.')), secF('', partFields(v.parts, v.chosen)))
    s.push(secN(t('Password'), t('This server keeps a key made from the password, not the password itself. It opens these backups, as anyone who has this server’s database could anyway.')))
    if (v.protectedNow && !v.changePw) s.push(secN('', t('Scheduled backups are protected.')), secP('', [{ id: 'changepw', title: t('Change the password') }, { id: 'clearpw', title: t('Remove it') }]))
    else {
      s.push(secF('', pwFields(v.password, v.again)))
      if (!v.protectedNow && !v.changePw && v.password === '') s.push(secN('', t('With no password, secrets are left out of these backups.')))
    }
  }
  s.push(secP('', [{ id: 'save', title: t('Save'), on: true, disabled: v.busy || v.badPw || (on && v.chosen.size === 0) }]))
  return mk('backup.schedule', t('Settings · Backup'), t('Back up by itself'), '', s)
}
