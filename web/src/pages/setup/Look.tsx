// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { api, getToken, serverBase } from '../../api'
import { getLooks } from '../settings/pages/api'
import { sduiAsk, SDUI_BASE } from '../../sdui/client'
import { toast, useApp } from '../../state'
import { applyTheme, trialLook, type ThemeMeta } from '../../theme'
import { Button } from '../../ui'
import { setTvMode, useTvMode } from '../../tvmode/mode'
import { ChoiceCards, LaterNote, StepActions, StepHead, TickRow, type StepProps } from './parts'

type Card = { id: string; name: string; tagline?: string; theme?: string; type?: string; density?: string; effects?: Record<string, string>; preview?: { bg: string; surface: string; accent: string }; nav: Record<string, string> }
const FORM_FACTORS = ['phone', 'tablet', 'desktop', 'web', 'tv', 'headset']
const swatch = (c: Card) => (
  <span className="oobe-theme-pic" style={{ background: c.preview?.bg }} aria-hidden="true">
    <i className="bar" style={{ background: c.preview?.surface }} />
    <i className="card" style={{ background: c.preview?.surface }} /><i className="card" style={{ background: c.preview?.surface }} /><i className="card" style={{ background: c.preview?.surface }} />
    <i className="btn" style={{ background: c.preview?.accent }} />
  </span>
)

// Step: Look. All fourteen experiences, each tried on this page as it is chosen (its finish, type and spacing, and the
// theme it wears). The choice becomes the server's starting look for every kind of device; people can pick their own later.
export function Look({ info, next, back }: StepProps) {
  const { serverTheme } = useApp()
  const [cards, setCards] = useState<Card[]>([])
  const [pick, setPick] = useState('lumen')
  const tv = useTvMode()
  useEffect(() => { api.get<{ experiences: Card[]; default: string }>('/sdui/experiences').then((r) => { setCards(r.experiences); setPick(r.default) }).catch(() => {}) }, [])
  const choose = (id: string) => {
    setPick(id)
    const c = cards.find((x) => x.id === id)
    if (!c) return
    trialLook({ experience: c.id, theme: c.theme || serverTheme, type: c.type, density: c.density, effects: c.effects })
    // Wears its own theme for now; the next step chooses one for everyone.
    if (c.theme) applyTheme(c.theme)
  }
  const save = async () => {
    try {
      const got = await getLooks(false)
      const policy = { ...(got.experiences || {}), default: Object.fromEntries(FORM_FACTORS.map((f) => [f, pick])) }
      await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: policy, headers: { 'If-Match': got.etag } })
      const c = cards.find((x) => x.id === pick)
      if (c?.theme) { try { await api.put('/settings', { theme: c.theme }) } catch { /* the next step can set it */ } }
    } catch { toast('The look couldn’t be saved. You can choose it later in Settings, Appearance.', { tone: 'danger' }) }
    next()
  }
  return (
    <section>
      <StepHead title="Choose a look" lead="Fourteen looks, each a layout, finish and type of its own. Pick one for everyone: it is tried here as you choose. Colours are the next step, and people can pick their own look later." />
      {cards.length > 0 && (
        <ChoiceCards label="Look" value={pick} onChange={choose} small cards={cards.map((c) => ({ value: c.id, title: c.name, text: c.tagline, pic: swatch(c) }))} />
      )}
      <div className="su-gap">
        <TickRow kind="switch" checked={tv} onChange={setTvMode} title="TV mode" text="Laid out and driven like the Apple TV and Android TV apps, with a remote or a controller, for a screen across the room. Only on this device." />
      </div>
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={() => void save()}>Continue</Button></StepActions>
    </section>
  )
}

// Step: Theme. Colour is separate from the look: any theme goes with any look, an imported one included.
export function Theme({ info, next, back }: StepProps) {
  const { serverTheme, setServerTheme } = useApp()
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [pick, setPick] = useState(serverTheme)
  const file = useRef<HTMLInputElement | null>(null)
  const load = () => api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  useEffect(() => { void load() }, [])
  const choose = (id: string) => { setPick(id); applyTheme(id) }
  const importZip = async (f: File) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/zip' }
    const tok = getToken()
    if (tok) headers.Authorization = 'Bearer ' + tok
    const res = await fetch(`${serverBase()}/api/themes?name=${encodeURIComponent(f.name)}`, { method: 'POST', headers, body: f, credentials: 'same-origin' })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) { toast(data.error || 'That theme couldn’t be installed', { tone: 'danger' }); return }
    toast(`“${data.name}” is installed`, { tone: 'ok' })
    await load()
    if (data.id) choose(data.id)
  }
  const save = async () => {
    try { await api.put('/settings', { theme: pick }); setServerTheme(pick) } catch { /* can be changed later */ }
    next()
  }
  return (
    <section>
      <StepHead title="Choose a theme" lead="The colours. Any theme goes with any look, and it changes right away. You can import one, or make your own later in Settings, Appearance." />
      {themes.length > 0 && (
        <ChoiceCards label="Theme" value={pick} onChange={choose} small cards={themes.map((t) => ({
          value: t.id, title: t.name.replace(/^WittyMediaServer /, ''),
          pic: (
            <span className="oobe-theme-pic" style={{ background: t.preview.bg }} aria-hidden="true">
              <i className="bar" style={{ background: t.preview.surface }} />
              <i className="card" style={{ background: t.preview.surface }} /><i className="card" style={{ background: t.preview.surface }} /><i className="card" style={{ background: t.preview.surface }} />
              <i className="btn" style={{ background: t.preview.accent }} />
            </span>
          ),
        }))} />
      )}
      <div className="su-gap">
        <input ref={file} type="file" accept=".zip,application/zip" hidden aria-label="Theme ZIP" onChange={(e) => { const f = e.target.files?.[0]; if (f) void importZip(f); e.target.value = '' }} />
        <Button variant="ghost" onClick={() => file.current?.click()}>Import a theme…</Button>
      </div>
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={() => void save()}>Continue</Button></StepActions>
    </section>
  )
}
