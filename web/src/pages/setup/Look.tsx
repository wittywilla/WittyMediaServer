// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { api, getToken, serverBase } from '../../api'
import { getLooks } from '../settings/pages/api'
import { sduiAsk, SDUI_BASE } from '../../sdui/client'
import { toast, useApp } from '../../state'
import { applyTheme, trialLook, type ThemeMeta } from '../../theme'
import { Brand } from '../../components/common'
import { Button, Segmented } from '../../ui'
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

type Device = 'tv' | 'desktop' | 'phone' | 'headset'
const SIDE = new Set(['sidebar', 'rail', 'railDrawer', 'tvSideMenu', 'ornamentTabs'])
const BOTTOM = new Set(['bottomTabs'])

// A small Home drawn in the look being tried, for each kind of device: where its menu is, the server's name, a spotlight and a row.
// Made of the look's own colours and gradients, no pictures.
function LookPreview({ card, device }: { card: Card; device: Device }) {
  const nav = card.nav[device] || card.nav.desktop || 'topTabs'
  const kind = SIDE.has(nav) ? 'side' : BOTTOM.has(nav) ? 'bottom' : 'top'
  const pv = card.preview
  const style = { ['--lp-bg' as string]: pv?.bg, ['--lp-surface' as string]: pv?.surface, ['--lp-accent' as string]: pv?.accent }
  return (
    <div className={'lp lp-' + device + ' nav-' + kind} style={style} role="img" aria-label={`${card.name} on a ${device}: the menu is ${kind === 'side' ? 'at the side' : kind === 'bottom' ? 'at the bottom' : 'along the top'}`}>
      <div className="lp-nav"><span className="lp-brand"><Brand name="" /></span><i className="on" /><i /><i /><i /></div>
      <div className="lp-page">
        <div className="lp-hero"><b>Harbour Lights</b><span>A night-shift ferry pilot finds a locked lantern room.</span><em>Resume</em></div>
        <div className="lp-row">{[0, 1, 2, 3, 4].map((i) => <i key={i} style={{ filter: `hue-rotate(${i * 40}deg)` }} />)}</div>
      </div>
    </div>
  )
}

// Step: Look. All fourteen experiences, each tried on this page as it is chosen (its finish, type and spacing, and the
// theme it wears). The choice becomes the server's starting look for every kind of device; people can pick their own later.
export function Look({ info, next, back }: StepProps) {
  const { serverTheme } = useApp()
  const [cards, setCards] = useState<Card[]>([])
  const [pick, setPick] = useState('lumen')
  const tv = useTvMode()
  const [device, setDevice] = useState<Device>('desktop')
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
      // The theme is not written here: the server follows the look's own theme until someone chooses another (Theme step).
    } catch { toast('The look couldn’t be saved. You can choose it later in Settings, Appearance.', { tone: 'danger' }) }
    next()
  }
  return (
    <section>
      <StepHead title="Choose a look" lead="Fourteen looks, each a layout, finish and type of its own. Pick one for everyone: it is tried here as you choose. Colours are the next step, and people can pick their own look later." />
      {cards.length > 0 && (
        <div className="su-look">
          <ChoiceCards label="Look" value={pick} onChange={choose} small cards={cards.map((c) => ({ value: c.id, title: c.name, text: c.tagline, pic: swatch(c) }))} />
          <div className="su-look-view">
            <Segmented<Device> label="Preview on" value={device} onChange={setDevice} options={[{ value: 'tv', label: 'TV' }, { value: 'desktop', label: 'Desktop' }, { value: 'phone', label: 'Phone' }, { value: 'headset', label: 'Headset' }]} />
            {cards.find((c) => c.id === pick) && <LookPreview card={cards.find((c) => c.id === pick)!} device={device} />}
            <div className="su-look-notes">
              <p><b>WittyTV look</b> Suggested with the look. Change it any time under Look in the guide.</p>
              <p><b>Server name</b> Drawn in this look’s colours on every screen. Edit it under Name and logo.</p>
              <p><b>Edit layout</b> Move anything later, for everyone or just for you.</p>
            </div>
          </div>
        </div>
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
  // Only an explicit choice pins a theme; carrying on with what the look wears stores nothing.
  const [chosen, setChosen] = useState(false)
  const file = useRef<HTMLInputElement | null>(null)
  const load = () => api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  useEffect(() => { void load() }, [])
  const choose = (id: string) => { setPick(id); setChosen(true); applyTheme(id) }
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
    if (chosen) { try { await api.put('/settings', { theme: pick }); setServerTheme(pick) } catch { /* can be changed later */ } }
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
