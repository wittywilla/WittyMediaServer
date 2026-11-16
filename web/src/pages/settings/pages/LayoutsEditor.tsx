// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Settings, Pages & menu, Looks: which experience each kind of device starts with, which experiences each profile may
// choose for itself in Settings, Appearance, and the looks an administrator requires of everyone, of kids or of one
// profile (docs/SDUI.md, "Experiences"). Saved to the draft, and published with the rest of it.

import { useEffect, useState } from 'react'
import { announce } from '../../../a11y'
import { api } from '../../../api'
import { LoadError, Spinner } from '../../../components/common'
import { t, tk, tx } from '../../../i18n'
import { toast } from '../../../state'
import { Button } from '../../../ui'
import { Switch } from '../../../ui/Field'
import type { ThemeMeta } from '../../../theme'
import { PageHead, Row, Section } from '../ui'
import { getLooks, message, saveLooks, type LooksAnswer, type LookPolicy, type Requirement } from './api'
import { DraftBar, useEditor } from './context'

const FORM_FACTORS: [string, string][] = [['phone', tk('Phones')], ['tablet', tk('Tablets')], ['desktop', tk('The desktop app')], ['web', tk('Browsers')], ['tv', tk('Televisions')], ['headset', tk('Headsets')]]
type Person = { id: number; name: string; displayName?: string; kind?: string }

export default function LayoutsEditor() {
  const { refresh, onConflict } = useEditor()
  const [got, setGot] = useState<LooksAnswer | null>(null)
  const [failed, setFailed] = useState(false)
  const [policy, setPolicy] = useState<LookPolicy>({})
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [people, setPeople] = useState<Person[]>([])
  const [busy, setBusy] = useState(false)
  const load = () => { setFailed(false); getLooks().then((a) => { setGot(a); setPolicy(a.experiences || {}) }).catch(() => setFailed(true)) }
  useEffect(load, [])
  useEffect(() => {
    api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
    api.get<Person[]>('/users').then(setPeople).catch(() => {})
  }, [])
  if (failed) return <LoadError what={t('Looks')} onRetry={load} />
  if (!got) return <Spinner />
  const options = got.options
  const allowed = policy.allowed && policy.allowed.length ? policy.allowed : options.map((o) => o.id)
  const startOf = (ff: string) => policy.default?.[ff] || got.builtin[ff] || 'lumen'
  const dirty = JSON.stringify(policy) !== JSON.stringify(got.experiences || {})
  const save = async () => {
    setBusy(true)
    try {
      await saveLooks(policy, onConflict)
      await refresh()
      load()
      announce(t('Saved to the draft')); toast(t('Saved to the draft'), { tone: 'ok' })
    } catch (e) { toast(message(e), { tone: 'danger' }) } finally { setBusy(false) }
  }
  const setStart = (ff: string, id: string) => setPolicy((p) => ({ ...p, default: { ...(p.default || {}), [ff]: id } }))
  const setAllowed = (id: string, on: boolean) => setPolicy((p) => {
    const now = new Set(p.allowed && p.allowed.length ? p.allowed : options.map((o) => o.id))
    if (on) now.add(id); else now.delete(id)
    // A device's starting experience is always one profiles may have.
    for (const [ff] of FORM_FACTORS) now.add(p.default?.[ff] || got.builtin[ff] || 'lumen')
    return { ...p, allowed: options.map((o) => o.id).filter((x) => now.has(x)) }
  })
  // A requirement: an experience, a theme or both. Empty on both takes it away.
  const setRequired = (who: 'everyone' | 'kids' | { profile: string }, part: Partial<Requirement>) => setPolicy((p) => {
    const req = { ...(p.required || {}) }
    const current = who === 'everyone' ? req.everyone : who === 'kids' ? req.kids : req.profiles?.[who.profile]
    const next: Requirement = { ...(current || {}), ...part }
    if (!next.experience) delete next.experience
    if (!next.theme) delete next.theme
    const none = !next.experience && !next.theme
    if (who === 'everyone' || who === 'kids') { if (none) delete req[who]; else req[who] = next }
    else {
      const profiles = { ...(req.profiles || {}) }
      if (none) delete profiles[who.profile]; else profiles[who.profile] = next
      if (Object.keys(profiles).length) req.profiles = profiles; else delete req.profiles
    }
    return { ...p, required: Object.keys(req).length ? req : undefined }
  })
  const lookRow = (label: string, desc: string, who: 'everyone' | 'kids' | { profile: string }) => {
    const r = who === 'everyone' ? policy.required?.everyone : who === 'kids' ? policy.required?.kids : policy.required?.profiles?.[who.profile]
    return (
      <Row key={typeof who === 'string' ? who : who.profile} label={label} desc={desc}>
        <select className="select" aria-label={t('Look required for {who}', { who: label })} value={r?.experience || ''} onChange={(e) => setRequired(who, { experience: e.target.value })}>
          <option value="">{t('Free to choose')}</option>
          {options.map((o) => <option key={o.id} value={o.id}>{tx(o.name)}</option>)}
        </select>
        <select className="select" aria-label={t('Theme required for {who}', { who: label })} value={r?.theme || ''} onChange={(e) => setRequired(who, { theme: e.target.value })}>
          <option value="">{t('Free to choose')}</option>
          {themes.map((th) => <option key={th.id} value={th.id}>{tx(th.name.replace(/^WittyMediaServer /, ''))}</option>)}
        </select>
      </Row>
    )
  }
  return (
    <>
      <PageHead title={t('Looks')} desc={t('How the apps look and are laid out: where the menu is and how pages are drawn (the experience), and the colours it wears (the theme). Choose what each kind of device starts with, which looks people may choose, and which you require.')} />
      <DraftBar />
      <Section title={t('What each device starts with')} desc={t('A profile that hasn’t chosen, or may not rearrange pages, gets this.')}>
        {FORM_FACTORS.map(([ff, label]) => (
          <Row key={ff} label={t(label)}>
            <select className="select" aria-label={t('Look for {devices}', { devices: t(label).toLowerCase() })} value={startOf(ff)} onChange={(e) => setStart(ff, e.target.value)}>
              {options.map((o) => <option key={o.id} value={o.id}>{tx(o.name)}</option>)}
            </select>
          </Row>
        ))}
      </Section>
      <Section title={t('What people may choose')} desc={t('Each profile picks one for each of its devices. A look a device starts with is always allowed.')}>
        {options.map((o) => {
          const fixed = FORM_FACTORS.some(([ff]) => startOf(ff) === o.id)
          return (
            <Row key={o.id} label={tx(o.name)} desc={tx(o.tagline || o.description)} dim={fixed}>
              <Switch label={t('Allow {name}', { name: tx(o.name) })} checked={allowed.includes(o.id)} disabled={fixed} onChange={(on) => setAllowed(o.id, on)} />
            </Row>
          )
        })}
        <Row label={t('Profiles may import themes')} desc={t('A profile can add up to five themes of its own, offered to that profile only. Administrators always may.')}>
          <Switch label={t('Profiles may import themes')} checked={!!policy.profileThemes} onChange={(on) => setPolicy((p) => ({ ...p, profileThemes: on || undefined }))} />
        </Row>
      </Section>
      <Section title={t('Required looks')} desc={t('A required look is shown locked, with your name, and the profile can’t change it. The most specific wins: a profile, then kids, then everyone.')}>
        {lookRow(t('Everyone'), t('Every profile on every device.'), 'everyone')}
        {lookRow(t('Kids profiles'), t('Every kids profile.'), 'kids')}
        {people.map((u) => lookRow(u.displayName || u.name, u.kind === 'kids' ? t('A kids profile.') : t('This profile only.'), { profile: String(u.id) }))}
      </Section>
      <div className="pe-actions">
        <Button variant="primary" busy={busy} disabled={!dirty} onClick={() => void save()}>{t('Save to draft')}</Button>
        <Button variant="ghost" disabled={!dirty || busy} onClick={() => setPolicy(got.experiences || {})}>{t('Undo changes')}</Button>
      </div>
    </>
  )
}
