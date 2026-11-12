// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Settings, Pages & menu, Looks: which experience each kind of device starts with, which experiences each profile may
// choose for itself in Settings, Appearance, and the looks an administrator requires of everyone, of kids or of one
// profile (docs/SDUI.md, "Experiences"). Saved to the draft, and published with the rest of it.

import { useEffect, useState } from 'react'
import { announce } from '../../../a11y'
import { api } from '../../../api'
import { LoadError, Spinner } from '../../../components/common'
import { toast } from '../../../state'
import { Button } from '../../../ui'
import { Switch } from '../../../ui/Field'
import type { ThemeMeta } from '../../../theme'
import { PageHead, Row, Section } from '../ui'
import { getLooks, message, saveLooks, type LooksAnswer, type LookPolicy, type Requirement } from './api'
import { DraftBar, useEditor } from './context'

const FORM_FACTORS: [string, string][] = [['phone', 'Phones'], ['tablet', 'Tablets'], ['desktop', 'The desktop app'], ['web', 'Browsers'], ['tv', 'Televisions'], ['headset', 'Headsets']]
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
  if (failed) return <LoadError what="Looks" onRetry={load} />
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
      announce('Saved to the draft'); toast('Saved to the draft', { tone: 'ok' })
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
        <select className="select" aria-label={`Look required for ${label}`} value={r?.experience || ''} onChange={(e) => setRequired(who, { experience: e.target.value })}>
          <option value="">Free to choose</option>
          {options.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
        </select>
        <select className="select" aria-label={`Theme required for ${label}`} value={r?.theme || ''} onChange={(e) => setRequired(who, { theme: e.target.value })}>
          <option value="">Free to choose</option>
          {themes.map((t) => <option key={t.id} value={t.id}>{t.name.replace(/^WittyMediaServer /, '')}</option>)}
        </select>
      </Row>
    )
  }
  return (
    <>
      <PageHead title="Looks" desc="How the apps look and are laid out: where the menu is and how pages are drawn (the experience), and the colours it wears (the theme). Choose what each kind of device starts with, which looks people may choose, and which you require." />
      <DraftBar />
      <Section title="What each device starts with" desc="A profile that hasn’t chosen, or may not rearrange pages, gets this.">
        {FORM_FACTORS.map(([ff, label]) => (
          <Row key={ff} label={label}>
            <select className="select" aria-label={`Look for ${label.toLowerCase()}`} value={startOf(ff)} onChange={(e) => setStart(ff, e.target.value)}>
              {options.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
            </select>
          </Row>
        ))}
      </Section>
      <Section title="What people may choose" desc="Each profile picks one for each of its devices. A look a device starts with is always allowed.">
        {options.map((o) => {
          const fixed = FORM_FACTORS.some(([ff]) => startOf(ff) === o.id)
          return (
            <Row key={o.id} label={o.name} desc={o.tagline || o.description} dim={fixed}>
              <Switch label={`Allow ${o.name}`} checked={allowed.includes(o.id)} disabled={fixed} onChange={(on) => setAllowed(o.id, on)} />
            </Row>
          )
        })}
        <Row label="Profiles may import themes" desc="A profile can add up to five themes of its own, offered to that profile only. Administrators always may.">
          <Switch label="Profiles may import themes" checked={!!policy.profileThemes} onChange={(on) => setPolicy((p) => ({ ...p, profileThemes: on || undefined }))} />
        </Row>
      </Section>
      <Section title="Required looks" desc="A required look is shown locked, with your name, and the profile can’t change it. The most specific wins: a profile, then kids, then everyone.">
        {lookRow('Everyone', 'Every profile on every device.', 'everyone')}
        {lookRow('Kids profiles', 'Every kids profile.', 'kids')}
        {people.map((u) => lookRow(u.displayName || u.name, u.kind === 'kids' ? 'A kids profile.' : 'This profile only.', { profile: String(u.id) }))}
      </Section>
      <div className="pe-actions">
        <Button variant="primary" busy={busy} disabled={!dirty} onClick={() => void save()}>Save to draft</Button>
        <Button variant="ghost" disabled={!dirty || busy} onClick={() => setPolicy(got.experiences || {})}>Undo changes</Button>
      </div>
    </>
  )
}
