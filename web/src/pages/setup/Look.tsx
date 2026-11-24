// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { api, getToken, serverBase } from '../../api'
import { getLooks } from '../settings/pages/api'
import { sduiAsk, SDUI_BASE } from '../../sdui/client'
import { toast, useApp } from '../../state'
import { applyTheme, currentLook, loadThemeOverrides, trialLook, type ThemeMeta } from '../../theme'
import { bump } from '../../sdui/client'
import { THEMES as TV_THEMES, themeById, themeTokens, type ThemeId } from '../tv/tvThemes'
import { ThemePicture } from '../tv/LookPanel'
import { appColours } from '../tv/look'
import { Brand } from '../../components/common'
import { contrastRatio } from '../../themeColours'
import { Button, Segmented, Spinner } from '../../ui'
import { TvPart } from '../../tvmode/OnAComputer'
import { LayoutThumb } from '../../ui/LayoutThumb'
import { useDevice } from '../../sdui/useSdui'
import { specFor, type Cards, type Home, type WireMap } from '../../ui/layoutSpecs'
import { ChoiceCards, Customize, Fill, Option, PanelStep, StepActions, StepHead, TickRow, type StepProps } from './parts'
import { t, tx } from '../../i18n'

type Card = { id: string; name: string; tagline?: string; theme?: string; type?: string; density?: string; effects?: Record<string, string>; preview?: { bg: string; surface: string; accent: string }; nav: Record<string, string>; wire?: WireMap; tvTheme?: string }
const FORM_FACTORS = ['phone', 'tablet', 'desktop', 'web', 'tv', 'headset']
// The layout chosen on the first page, carried to the colours page (and back to it): its id.
let chosenLayout = ''

type Device = 'tv' | 'desktop' | 'phone' | 'headset'
const SIDE = new Set(['sidebar', 'rail', 'railDrawer', 'tvSideMenu', 'ornamentTabs'])
const BOTTOM = new Set(['bottomTabs'])

// A small Home drawn in the look being tried, for each kind of device: where its menu is, the server's name, and Home
// arranged as that look arranges it (a spotlight and a row, a poster carousel, big tiles, a list, widgets, a cross menu).
// Made of the look's own colours and gradients, no pictures; the shape comes from the same `wire` the thumbnails draw.
const tint = (i: number) => ({ filter: `hue-rotate(${i * 40}deg)` })
function LookHome({ home, cards }: { home: Home; cards: Cards }) {
  const row = (n: number, kind: string = cards) => <div className={'lp-row c-' + kind}>{Array.from({ length: n }, (_, i) => <i key={i} style={tint(i)} />)}</div>
  const hero = (small?: boolean) => <div className={'lp-hero' + (small ? ' small' : '')}><b>Harbour Lights</b>{!small && <span>{t('A night-shift ferry pilot finds a locked lantern room.')}</span>}<em>{t('Resume')}</em></div>
  switch (home) {
    case 'carousel':
      return <div className="lp-carousel">{[0, 1, 2, 3, 4].map((i) => <i key={i} className={i === 2 ? 'on' : ''} style={tint(i)} />)}<b>Harbour Lights</b></div>
    case 'tiles':
      return <><div className="lp-tiles">{[0, 1, 2].map((i) => <i key={i} className={i === 0 ? 'on' : ''} style={tint(i)}><span>{[t('Films'), t('TV'), t('Music')][i]}</span></i>)}</div></>
    case 'list':
      return <div className="lp-list">{[0, 1, 2, 3, 4, 5].map((i) => <p key={i} className={i === 0 ? 'on' : ''}><i style={tint(i)} /><b /><em /></p>)}</div>
    case 'widgets':
      return <div className="lp-widgets"><i className="wide" style={tint(0)} /><i style={tint(1)} /><i style={tint(2)} /><i style={tint(3)} /><i className="wide" style={tint(4)} /></div>
    case 'rows':
      return <div className="lp-cross"><div className="lp-cross-bar">{[0, 1, 2, 3, 4].map((i) => <i key={i} className={i === 1 ? 'on' : ''} />)}</div><div className="lp-list">{[0, 1, 2, 3].map((i) => <p key={i} className={i === 0 ? 'on' : ''}><i style={tint(i)} /><b /><em /></p>)}</div></div>
    case 'banner':
      return <>{hero(true)}{row(5)}{row(5)}</>
  }
  return <>{hero()}{row(5)}</>
}

// The name in a look's preview is drawn in that look's colours, not the page's: white or black, whichever reads on
// the look's own bar, and its accent for the coloured word only where that reads too (4.5:1). Without this a light
// theme drew its dark words on a dark look's bar.
function previewInk(pv: Card['preview']): Record<string, string> {
  if (!pv?.surface) return {}
  const white = contrastRatio('#ffffff', pv.surface) ?? 21, black = contrastRatio('#000000', pv.surface) ?? 21
  const ink = white >= black ? 'var(--on-image)' : 'var(--picture-bg)'
  const accent = (contrastRatio(pv.accent, pv.surface) ?? 0) >= 4.5 ? 'var(--lp-accent)' : ink
  return { '--lp-fg': ink, '--lp-brand': accent }
}

function LookPreview({ card, device }: { card: Card; device: Device }) {
  const spec = specFor(card.wire, device)
  const nav = card.nav[device] || card.nav.desktop || 'topTabs'
  const kind = spec.nav === 'none' || spec.nav === 'crossbar' ? 'none' : spec.nav === 'bottom' || BOTTOM.has(nav) ? 'bottom' : ['sidebar', 'rail', 'railDrawer'].includes(spec.nav) || SIDE.has(nav) ? 'side' : 'top'
  const pv = card.preview
  const style = { ['--lp-bg' as string]: pv?.bg, ['--lp-surface' as string]: pv?.surface, ['--lp-accent' as string]: pv?.accent, ...previewInk(pv) }
  const where = kind === 'side' ? t('the menu is at the side') : kind === 'bottom' ? t('the menu is at the bottom') : kind === 'none' ? t('the menu is part of Home') : t('the menu is along the top')
  const on = { tv: t('on a TV'), desktop: t('on a desktop'), phone: t('on a phone'), headset: t('in VR') }[device]
  return (
    <div className={'lp lp-' + device + ' nav-' + kind + ' home-' + spec.home} style={style} role="img" aria-label={t('{look} {device}: {menu}', { look: card.name, device: on, menu: where })}>
      {kind !== 'none' && <div className="lp-nav"><span className="lp-brand"><Brand name="" /></span><i className="on" /><i /><i /><i /></div>}
      <div className="lp-page"><LookHome home={spec.home} cards={spec.cards} /></div>
    </div>
  )
}

// WittyTV's look, chosen on its own: each look brings one (Lumen brings Modern), and picking another here overrides it
// for everyone who hasn't chosen their own in the guide. The large picture is the guide as it will look.
export function TvLookChoice({ suggested, lookName, value, onChange }: { suggested?: string; lookName: string; value: string; onChange: (v: string) => void }) {
  const app = appColours()
  const follow = (themeById(suggested)?.id || 'modern') as ThemeId
  const shown = (value ? value : follow) as ThemeId
  const th = themeById(shown)!
  return (
    <>
      <p className="su-tv-lead"><Fill text={t('The server’s look sets it to begin with: {look} brings {theme}. Pick another to use it for everyone instead. People can still choose their own in the guide.')} parts={{ look: lookName, theme: <b>{tx(themeById(follow)?.name)}</b> }} /></p>
      <div className="su-look su-tv">
        <ChoiceCards label={t('WittyTV look')} value={value || 'follow'} onChange={(v) => onChange(v === 'follow' ? '' : v)} small cards={[
          { value: 'follow', title: t('Follow the look'), text: t('{theme}, from {look}', { theme: tx(themeById(follow)?.name), look: lookName }), pic: <ThemePicture shape={themeById(follow)!.shape} tokens={themeTokens(follow, undefined, app)} /> },
          ...TV_THEMES.filter((x) => x.id !== 'custom').map((x) => ({ value: x.id, title: tx(x.name), text: x.id === follow ? t('The look’s own') : undefined, pic: <ThemePicture shape={x.shape} tokens={themeTokens(x.id, undefined, app)} /> })),
        ]} />
        <div className="su-look-view">
          <div className="su-tv-big"><ThemePicture shape={th.shape} tokens={themeTokens(shown, undefined, app)} /></div>
          <p className="su-tv-note"><Fill text={value ? t('{theme} overrides the look’s own.') : t('{theme} follows {look}.')} parts={{ theme: <b>{tx(th.name)}</b>, look: lookName }} /> {tx(th.blurb)}</p>
        </div>
      </div>
    </>
  )
}

// Step 4: Look. All fourteen layouts, each tried on this page as it is chosen (its finish, type and spacing, and the theme
// it wears). The choice becomes the server's starting look for every kind of device; people can pick their own later.
// Colours, and WittyTV's guide where WittyTuner is, are under Customize: until someone picks, each layout brings its own.
export function Look({ facts, next, back }: StepProps) {
  // The layouts are drawn as this device shows them: an upright phone, a tablet, a computer or a TV.
  const here = useDevice().formFactor as keyof WireMap
  const { serverTheme, setServerTheme } = useApp()
  const [cards, setCards] = useState<Card[]>([])
  const [pick, setPick] = useState(chosenLayout || 'lumen')
  // The preview starts as this device: a phone shows the phone, a TV the TV.
  const [device, setDevice] = useState<Device>(here === 'phone' ? 'phone' : here === 'tv' ? 'tv' : here === 'headset' ? 'headset' : 'desktop')
  // A theme picked under Customize, kept for everyone whichever look they use; '' follows the layout's own.
  const [theme, setTheme] = useState('')
  const tuner = facts.modules.includes('wittytuner')
  const [tvLook, setTvLook] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<{ experiences: Card[]; default: string }>('/sdui/experiences').then((r) => { setCards(r.experiences); setPick(chosenLayout || r.default) }).catch(() => {})
    if (tuner) api.get<Record<string, string>>('/tv/settings').then((v) => setTvLook(v.tvTheme || '')).catch(() => {})
  }, [tuner])
  const card = cards.find((c) => c.id === pick)
  const choose = (id: string) => {
    setPick(id)
    const c = cards.find((x) => x.id === id)
    if (!c) return
    trialLook({ experience: c.id, theme: theme || c.theme || serverTheme, type: c.type, density: c.density, effects: c.effects })
    if (!theme && c.theme) applyTheme(c.theme)
  }
  const pickTheme = (id: string) => { setTheme(id); applyTheme(id || card?.theme || serverTheme) }
  const save = async () => {
    setBusy(true)
    chosenLayout = pick
    try {
      const got = await getLooks(false)
      const policy = { ...(got.experiences || {}), default: Object.fromEntries(FORM_FACTORS.map((f) => [f, pick])) }
      await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: policy, headers: { 'If-Match': got.etag } })
      // Only a theme picked by someone is pinned; otherwise the server follows each look's own.
      if (theme) { await api.put('/settings', { theme }); setServerTheme(theme) }
      if (tuner && tvLook) await api.put('/tv/settings', { tvTheme: tvLook })
    } catch { toast(t('The look couldn’t be saved. You can choose it later in Settings, Appearance.'), { tone: 'danger' }) }
    bump(); void loadThemeOverrides()
    setBusy(false)
    next()
  }
  return (
    <section>
      <StepHead title={t('Choose a look')} lead={t('This is how everyone’s Home starts out. Each person can pick their own later.')} />
      {cards.length === 0 && <Spinner />}
      {cards.length > 0 && (
        <div className="su-look su-look-first-view">
          <ChoiceCards label={t('Layout')} value={pick} onChange={choose} small cards={cards.map((c) => ({ value: c.id, title: c.name, text: c.tagline ? tx(c.tagline) : undefined, pic: <LayoutThumb wire={c.wire} formFactor={here} colours={c.preview} /> }))} />
          <div className="su-look-view">
            <Segmented<Device> label={t('Preview on')} value={device} onChange={setDevice} options={[{ value: 'tv', label: t('TV') }, { value: 'desktop', label: t('Desktop') }, { value: 'phone', label: t('Phone') }, { value: 'headset', label: t('VR') }]} />
            {card && <LookPreview card={card} device={device} />}
          </div>
        </div>
      )}
      <StepActions back={back}>
        {!facts.advanced && <Customize title={t('Look options')}>
          <Option title={t('Colours')} text={theme ? t('The same colours with every look') : t('Each look brings its own')}>
            <ThemeChoice layout={card} here={here} theme={theme} onPick={pickTheme} />
          </Option>
          {tuner && card && (
            <Option title={t('TV guide')} text={tvLook ? t('Your own choice') : t('Follows the look')}>
              <TvLookChoice suggested={card.tvTheme} lookName={card.name} value={tvLook} onChange={setTvLook} />
            </Option>
          )}
        </Customize>}
        <Button variant="primary" size="big" busy={busy} onClick={() => void save()}>{t('Continue')}</Button>
      </StepActions>
    </section>
  )
}

// Colour is separate from the look: any theme goes with any look, an imported one included. '' is the look's own.
export function ThemeChoice({ layout, here, theme, onPick }: { layout?: Card; here: keyof WireMap; theme: string; onPick: (id: string) => void }) {
  const { serverTheme } = useApp()
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const file = useRef<HTMLInputElement | null>(null)
  const load = () => api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  useEffect(() => { void load() }, [])
  const shown = theme || layout?.theme || currentLook()?.theme || serverTheme
  const importZip = async (f: File) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/zip' }
    const tok = getToken()
    if (tok) headers.Authorization = 'Bearer ' + tok
    const res = await fetch(`${serverBase()}/api/themes?name=${encodeURIComponent(f.name)}`, { method: 'POST', headers, body: f, credentials: 'same-origin' })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) { toast(data.error || t('That theme couldn’t be installed'), { tone: 'danger' }); return }
    toast(t('“{name}” is installed', { name: data.name }), { tone: 'ok' })
    await load()
    if (data.id) onPick(data.id)
  }
  return (
    <>
      <TickRow kind="switch" checked={!theme} onChange={(v) => onPick(v ? '' : shown)} title={t('Start with the look’s own theme')} />
      {themes.length > 0 && (
        <div className="su-gap">
          <ChoiceCards label={t('Theme')} value={shown} onChange={onPick} small cards={themes.map((x) => ({
            value: x.id, title: x.name.replace(/^WittyMediaServer /, ''),
            pic: <LayoutThumb wire={layout?.wire} formFactor={here} colours={x.preview} />,
          }))} />
        </div>
      )}
      <TvPart id="appearance.import">
        <div className="su-gap">
          <input ref={file} type="file" accept=".zip,application/zip" hidden aria-label={t('Theme ZIP')} onChange={(e) => { const f = e.target.files?.[0]; if (f) void importZip(f); e.target.value = '' }} />
          <Button variant="ghost" onClick={() => file.current?.click()}>{t('Import a theme…')}</Button>
        </div>
      </TvPart>
    </>
  )
}

// The layout chosen on the Look page, for the pages after it.
function useChosenLayout() {
  const [card, setCard] = useState<Card | undefined>()
  useEffect(() => {
    api.get<{ experiences: Card[]; default: string }>('/sdui/experiences').then((r) => {
      const id = chosenLayout || currentLook()?.experience || r.default
      setCard(r.experiences.find((x) => x.id === id) || r.experiences[0])
    }).catch(() => {})
  }, [])
  return card
}

// Advanced setup: Colours. Any theme goes with any layout; until one is picked, each layout brings its own.
export function Colours({ next, back }: StepProps) {
  const here = useDevice().formFactor as keyof WireMap
  const { serverTheme, setServerTheme } = useApp()
  const layout = useChosenLayout()
  const [theme, setTheme] = useState('')
  const [busy, setBusy] = useState(false)
  const pick = (id: string) => { setTheme(id); applyTheme(id || layout?.theme || serverTheme) }
  const save = async () => {
    setBusy(true)
    try { if (theme) { await api.put('/settings', { theme }); setServerTheme(theme) } } catch { toast(t('The colours couldn’t be saved. You can choose them later in Settings, Appearance.'), { tone: 'danger' }) }
    bump(); void loadThemeOverrides()
    setBusy(false)
    next()
  }
  return (
    <PanelStep wide title={t('Colours')} lead={t('Keep the colours each look comes with, or pick one theme for all of them.')} back={back} next={() => void save()} busy={busy}>
      <ThemeChoice layout={layout} here={here} theme={theme} onPick={pick} />
    </PanelStep>
  )
}

// Advanced setup: the live TV guide's own look, where WittyTuner is, starting from what the server's look brings.
export function TvGuide({ next, back }: StepProps) {
  const layout = useChosenLayout()
  const [tvLook, setTvLook] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/tv/settings').then((v) => setTvLook(v.tvTheme || '')).catch(() => {}) }, [])
  const save = async () => {
    setBusy(true)
    try { await api.put('/tv/settings', { tvTheme: tvLook }) } catch { toast(t('The TV guide’s look couldn’t be saved. You can choose it later in Settings, Appearance, under WittyTV.'), { tone: 'danger' }) }
    setBusy(false)
    next()
  }
  return (
    <PanelStep wide title={t('TV guide')} lead={t('How the live TV guide lays out its channels and programmes.')} back={back} next={() => void save()} busy={busy}>
      {layout && <TvLookChoice suggested={layout.tvTheme} lookName={layout.name} value={tvLook} onChange={setTvLook} />}
    </PanelStep>
  )
}
