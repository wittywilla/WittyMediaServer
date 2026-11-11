// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { toast } from '../../state'
import { loadThemeOverrides } from '../../theme'
import { Button } from '../../ui/Button'
import { Field, Input, Select } from '../../ui/Field'
import { Notice } from '../../ui/Surface'
import { Section } from './ui'

type Warning = { scope: string; variant?: string; text: string; surface: string; ratio: number; need: number }
type Resolved = { tokens: Record<string, string>; modules: Record<string, { overrides: Record<string, string> }>; warnings: Warning[] }
type Installed = { id: string; name?: string }

// The parts of the app that belong to a module and have pages of their own here.
const KNOWN: Installed[] = [{ id: 'wittytv', name: 'WittyTV' }, { id: 'wittyplay', name: 'WittyPlay' }]

// Colours for one module's pages, for everyone (the theme engine's module overrides): its
// pages take this accent in place of the theme's, and the rest of the app keeps the theme's.
export default function ModuleColours() {
  const [theme, setTheme] = useState<Resolved | null>(null)
  const [mods, setMods] = useState<Installed[]>(KNOWN)
  const [mod, setMod] = useState(KNOWN[0].id)
  const [accent, setAccent] = useState('')
  const [busy, setBusy] = useState(false)
  const load = () => api.get<Resolved>('/theme').then((t) => {
    setTheme(t)
    setMods((list) => [...list, ...Object.keys(t.modules || {}).filter((id) => !list.some((m) => m.id === id)).map((id) => ({ id }))])
  }).catch(() => {})
  useEffect(() => { load() }, [])
  const saved = theme?.modules?.[mod]?.overrides?.accent || ''
  const shown = (accent || saved || theme?.tokens?.accent || '#7955ff').slice(0, 7).toLowerCase()
  const done = async (work: Promise<unknown>, said: string) => {
    setBusy(true)
    try { await work; setAccent(''); await load(); await loadThemeOverrides(); toast(said) } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const warnings = (theme?.warnings || []).filter((w) => w.scope === 'module:' + mod)
  return (
    <Section title="Module colours" desc="Give one module’s pages an accent of their own, for everyone. The rest of the app keeps the theme’s colours.">
      <div className="form full">
        <Field label="Module"><Select value={mod} onChange={(e) => { setMod(e.target.value); setAccent('') }} options={mods.map((m) => ({ value: m.id, label: m.name || m.id }))} /></Field>
        <Field label="Accent" hint={saved ? 'This module has its own accent.' : 'This module uses the theme’s accent.'}>
          <Input className="theme-colour" type="color" value={shown} onChange={(e) => setAccent(e.target.value)} />
        </Field>
        {warnings.map((w, i) => (
          <Notice key={i}>With this colour, {w.text} on {w.surface}{w.variant ? ` in the ${w.variant} look` : ''} has a contrast of {w.ratio}:1, and text needs {w.need}:1 to be easy to read.</Notice>
        ))}
        <div className="panel-actions">
          <Button variant="primary" disabled={busy || !accent || accent.toLowerCase() === saved.toLowerCase()} onClick={() => done(api.put(`/theme/modules/${mod}`, { tokens: { ...(theme?.modules?.[mod]?.overrides || {}), accent, accentText: accent, focus: accent } }), 'Saved for everyone.')}>Save for everyone</Button>
          {saved && <Button variant="ghost" disabled={busy} onClick={() => done(api.del(`/theme/modules/${mod}`), 'This module uses the theme’s colours again.')}>Use the theme’s colours</Button>}
        </div>
      </div>
    </Section>
  )
}
