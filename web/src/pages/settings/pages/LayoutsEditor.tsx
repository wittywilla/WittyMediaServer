// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Settings, Pages & menu, Layouts: which layout each kind of device starts with, and which
// layouts each profile may choose for itself in Settings, Appearance (docs/SDUI.md, "Layout
// options"). Saved to the draft, and published with the rest of it.

import { useEffect, useState } from 'react'
import { announce } from '../../../a11y'
import { LoadError, Spinner } from '../../../components/common'
import { toast } from '../../../state'
import { Button } from '../../../ui'
import { Switch } from '../../../ui/Field'
import { PageHead, Row, Section } from '../ui'
import { getLayouts, message, saveLayouts, type LayoutsAnswer, type LayoutPolicy } from './api'
import { DraftBar, useEditor } from './context'

const FORM_FACTORS: [string, string][] = [['phone', 'Phones'], ['tablet', 'Tablets'], ['desktop', 'The desktop app'], ['web', 'Browsers'], ['tv', 'Televisions']]

export default function LayoutsEditor() {
  const { refresh, onConflict } = useEditor()
  const [got, setGot] = useState<LayoutsAnswer | null>(null)
  const [failed, setFailed] = useState(false)
  const [policy, setPolicy] = useState<LayoutPolicy>({})
  const [busy, setBusy] = useState(false)
  const load = () => { setFailed(false); getLayouts().then((a) => { setGot(a); setPolicy(a.layouts || {}) }).catch(() => setFailed(true)) }
  useEffect(load, [])
  if (failed) return <LoadError what="Layouts" onRetry={load} />
  if (!got) return <Spinner />
  const options = got.options
  const allowed = policy.allowed && policy.allowed.length ? policy.allowed : options.map((o) => o.id)
  const startOf = (ff: string) => policy.default?.[ff] || got.builtin[ff] || 'sidebar'
  const dirty = JSON.stringify(policy) !== JSON.stringify(got.layouts || {})
  const save = async () => {
    setBusy(true)
    try {
      await saveLayouts(policy, onConflict)
      await refresh()
      load()
      announce('Saved to the draft'); toast('Saved to the draft', { tone: 'ok' })
    } catch (e) { toast(message(e), { tone: 'danger' }) } finally { setBusy(false) }
  }
  const setStart = (ff: string, id: string) => setPolicy((p) => ({ ...p, default: { ...(p.default || {}), [ff]: id } }))
  const setAllowed = (id: string, on: boolean) => setPolicy((p) => {
    const now = new Set(p.allowed && p.allowed.length ? p.allowed : options.map((o) => o.id))
    if (on) now.add(id); else now.delete(id)
    // A device's starting layout is always one profiles may have.
    for (const [ff] of FORM_FACTORS) now.add(p.default?.[ff] || got.builtin[ff] || 'sidebar')
    return { ...p, allowed: options.map((o) => o.id).filter((x) => now.has(x)) }
  })
  return (
    <>
      <PageHead title="Layouts" desc="How the apps are laid out: where the menu is and how pages look. Choose what each kind of device starts with, and which layouts people may choose for themselves in Settings, Appearance." />
      <DraftBar />
      <Section title="What each device starts with" desc="A profile that hasn’t chosen, or may not rearrange pages, gets this.">
        {FORM_FACTORS.map(([ff, label]) => (
          <Row key={ff} label={label}>
            <select className="select" aria-label={`Layout for ${label.toLowerCase()}`} value={startOf(ff)} onChange={(e) => setStart(ff, e.target.value)}>
              {options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}
            </select>
          </Row>
        ))}
      </Section>
      <Section title="What people may choose" desc="Each profile picks one for each of its devices. A layout a device starts with is always allowed.">
        {options.map((o) => {
          const fixed = FORM_FACTORS.some(([ff]) => startOf(ff) === o.id)
          return (
            <Row key={o.id} label={o.label} desc={o.description} dim={fixed}>
              <Switch label={`Allow ${o.label}`} checked={allowed.includes(o.id)} disabled={fixed} onChange={(on) => setAllowed(o.id, on)} />
            </Row>
          )
        })}
      </Section>
      <div className="pe-actions">
        <Button variant="primary" busy={busy} disabled={!dirty} onClick={() => void save()}>Save to draft</Button>
        <Button variant="ghost" disabled={!dirty || busy} onClick={() => setPolicy(got.layouts || {})}>Undo changes</Button>
      </div>
    </>
  )
}
