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
import { Button, Segmented } from '../../ui'
import { setTvMode, useTvMode } from '../../tvmode/mode'
import { LayoutThumb } from '../../ui/LayoutThumb'
import { useDevice } from '../../sdui/useSdui'
import { specFor, type Cards, type Home, type WireMap } from '../../ui/layoutSpecs'
import { ChoiceCards, Fill, LaterNote, StepActions, StepHead, TickRow, type StepProps } from './parts'
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

function LookPreview({ card, device }: { card: Card; device: Device }) {
  const spec = specFor(card.wire, device)
  const nav = card.nav[device] || card.nav.desktop || 'topTabs'
  const kind = spec.nav === 'none' || spec.nav === 'crossbar' ? 'none' : spec.nav === 'bottom' || BOTTOM.has(nav) ? 'bottom' : ['sidebar', 'rail', 'railDrawer'].includes(spec.nav) || SIDE.has(nav) ? 'side' : 'top'
  const pv = card.preview
  const style = { ['--lp-bg' as string]: pv?.bg, ['--lp-surface' as string]: pv?.surface, ['--lp-accent' as string]: pv?.accent }
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
function TvLookChoice({ suggested, lookName, value, onChange }: { suggested?: string; lookName: string; value: string; onChange: (v: string) => void }) {
  const app = appColours()
  const follow = (themeById(suggested)?.id || 'modern') as ThemeId
  const shown = (value ? value : follow) as ThemeId
  const th = themeById(shown)!
  return (
    <>
      <h2 className="oobe-sub">{t('WittyTV look')}</h2>
      <p className="su-tv-lead"><Fill text={t('The live TV guide has a look of its own. The look you chose above sets it: {look} brings {theme}. Pick another to override it for everyone. People can still choose their own in the guide.')} parts={{ look: lookName, theme: <b>{tx(themeById(follow)?.name)}</b> }} /></p>
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

// Step: Look. All fourteen experiences, each tried on this page as it is chosen (its finish, type and spacing, and the
// theme it wears). The choice becomes the server's starting look for every kind of device; people can pick their own later.
export function Look({ info, next, back }: StepProps) {
  // The layouts are drawn as this device shows them: an upright phone, a tablet, a computer or a TV.
  const here = useDevice().formFactor as keyof WireMap
  const { serverTheme } = useApp()
  const [cards, setCards] = useState<Card[]>([])
  const [pick, setPick] = useState(chosenLayout || 'lumen')
  const tv = useTvMode()
  const [device, setDevice] = useState<Device>('desktop')
  // WittyTV's own look: '' follows what the chosen look suggests; anything else overrides it for everyone.
  const [tvLook, setTvLook] = useState('')
  useEffect(() => { api.get<{ experiences: Card[]; default: string }>('/sdui/experiences').then((r) => { setCards(r.experiences); setPick(chosenLayout || r.default) }).catch(() => {}) }, [])
  useEffect(() => { api.get<Record<string, string>>('/tv/settings').then((v) => setTvLook(v.tvTheme || '')).catch(() => {}) }, [])
  const choose = (id: string) => {
    setPick(id)
    const c = cards.find((x) => x.id === id)
    if (!c) return
    trialLook({ experience: c.id, theme: c.theme || serverTheme, type: c.type, density: c.density, effects: c.effects })
    // Wears its own theme for now; the next step chooses one for everyone.
    if (c.theme) applyTheme(c.theme)
  }
  const save = async () => {
    chosenLayout = pick
    try {
      const got = await getLooks(false)
      const policy = { ...(got.experiences || {}), default: Object.fromEntries(FORM_FACTORS.map((f) => [f, pick])) }
      await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: policy, headers: { 'If-Match': got.etag } })
      // The theme is not written here: the server follows the look's own theme until someone chooses another (Theme step).
    } catch { toast(t('The look couldn’t be saved. You can choose it later in Settings, Appearance.'), { tone: 'danger' }) }
    try { await api.put('/tv/settings', { tvTheme: tvLook }) } catch { /* can be changed later, in WittyTV's Look */ }
    bump(); void loadThemeOverrides()
    next()
  }
  return (
    <section>
      <StepHead title={t('Choose a layout')} lead={t('Each look is a layout, finish and type of its own, drawn here as a phone shows it. Pick one for everyone: it is tried here as you choose. Colours are the next page, and people can pick their own later.')} />
      {cards.length > 0 && (
        <div className="su-look">
          <ChoiceCards label={t('Layout')} value={pick} onChange={choose} small cards={cards.map((c) => ({ value: c.id, title: c.name, text: c.tagline ? tx(c.tagline) : undefined, pic: <LayoutThumb wire={c.wire} formFactor={here} colours={c.preview} /> }))} />
          <div className="su-look-view">
            <Segmented<Device> label={t('Preview on')} value={device} onChange={setDevice} options={[{ value: 'tv', label: t('TV') }, { value: 'desktop', label: t('Desktop') }, { value: 'phone', label: t('Phone') }, { value: 'headset', label: t('VR') }]} />
            {cards.find((c) => c.id === pick) && <LookPreview card={cards.find((c) => c.id === pick)!} device={device} />}
            {cards.find((c) => c.id === pick) && <p className="su-tv-note"><b>{cards.find((c) => c.id === pick)!.name}</b>{cards.find((c) => c.id === pick)!.tagline ? ` · ${tx(cards.find((c) => c.id === pick)!.tagline)}.` : '.'} {t('You can change it, or rearrange any page, later in Settings, Appearance.')}</p>}
          </div>
        </div>
      )}
      {cards.length > 0 && <TvLookChoice suggested={cards.find((c) => c.id === pick)?.tvTheme} lookName={cards.find((c) => c.id === pick)?.name || ''} value={tvLook} onChange={setTvLook} />}
      <div className="su-gap">
        <TickRow kind="switch" checked={tv} onChange={setTvMode} title={t('TV mode')} text={t('Laid out and driven like the Apple TV and Android TV apps, with a remote or a controller, for a screen across the room. Only on this device.')} />
      </div>
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={() => void save()}>{t('Next')}</Button></StepActions>
    </section>
  )
}

// Step: Theme. Colour is separate from the look: any theme goes with any look, an imported one included.
export function Theme({ info, next, back }: StepProps) {
  // The layouts are drawn as this device shows them: an upright phone, a tablet, a computer or a TV.
  const here = useDevice().formFactor as keyof WireMap
  const { serverTheme, setServerTheme } = useApp()
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  // Nothing is chosen until someone does: then the theme stays the look's own, whichever look is picked later.
  const [chosen, setChosen] = useState(false)
  const chosenNow = useRef(false)
  // Until someone chooses, the theme ticked is the one the look on the page before wears.
  const [pick, setPick] = useState(currentLook()?.theme || serverTheme)
  const [own, setOwn] = useState(false)
  useEffect(() => { getLooks(false).then((l) => setOwn(!!l.experiences?.profileThemes)).catch(() => {}) }, [])
  const file = useRef<HTMLInputElement | null>(null)
  // The layout chosen on the page before, drawn in each theme's colours.
  const [layout, setLayout] = useState<Card | null>(null)
  useEffect(() => {
    api.get<{ experiences: Card[]; default: string }>('/sdui/experiences').then((r) => {
      const id = chosenLayout || currentLook()?.experience || r.default
      const c = r.experiences.find((x) => x.id === id) || r.experiences[0] || null
      setLayout(c)
      // The look's own theme, unless one was picked while this was on its way.
      if (c?.theme && !chosenNow.current) { setPick(c.theme); applyTheme(c.theme) }
    }).catch(() => {})
  }, [])
  const load = () => api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  useEffect(() => { void load() }, [])
  const mark = (v: boolean) => { chosenNow.current = v; setChosen(v) }
  const choose = (id: string) => { mark(true); setPick(id); applyTheme(id) }
  const follow = () => { mark(false); const t = layout?.theme || currentLook()?.theme || serverTheme; setPick(t); applyTheme(t) }
  const importZip = async (f: File) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/zip' }
    const tok = getToken()
    if (tok) headers.Authorization = 'Bearer ' + tok
    const res = await fetch(`${serverBase()}/api/themes?name=${encodeURIComponent(f.name)}`, { method: 'POST', headers, body: f, credentials: 'same-origin' })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) { toast(data.error || t('That theme couldn’t be installed'), { tone: 'danger' }); return }
    toast(t('“{name}” is installed', { name: data.name }), { tone: 'ok' })
    await load()
    if (data.id) choose(data.id)
  }
  const save = async () => {
    // Only an explicit choice pins a theme; carrying on with what the look wears stores nothing.
    try { if (chosen) { await api.put('/settings', { theme: pick }); setServerTheme(pick) } } catch { /* can be changed later */ }
    try {
      const got = await getLooks(false)
      if (!!got.experiences?.profileThemes !== own) await sduiAsk('PUT', `${SDUI_BASE}/config/experience`, { body: { ...(got.experiences || {}), profileThemes: own || undefined }, headers: { 'If-Match': got.etag } })
    } catch { /* can be changed later */ }
    bump(); void loadThemeOverrides()
    next()
  }
  return (
    <section>
      <StepHead title={t('Choose colours')} lead={layout ? t('The colours for {look}. Any theme goes with any layout, and it changes right away. You can import one, or make your own later in Settings, Appearance.', { look: layout.name }) : t('The colours for the layout you chose. Any theme goes with any layout, and it changes right away. You can import one, or make your own later in Settings, Appearance.')} />
      {themes.length > 0 && (
        <div className="su-colours">
          <figure className="su-colours-big">
            <LayoutThumb wire={layout?.wire} formFactor={here} colours={themes.find((x) => x.id === pick)?.preview} />
            <figcaption><b>{themes.find((x) => x.id === pick)?.name.replace(/^WittyMediaServer /, '') || ''}</b>{layout ? ` ${t('with {look}', { look: layout.name })}` : ''}</figcaption>
          </figure>
          <ChoiceCards label={t('Theme')} value={pick} onChange={choose} small cards={themes.map((x) => ({
            value: x.id, title: x.name.replace(/^WittyMediaServer /, ''),
            pic: <LayoutThumb wire={layout?.wire} formFactor={here} colours={x.preview} />,
          }))} />
        </div>
      )}
      <h2 className="oobe-sub">{t('How people use it')}</h2>
      <div className="su-gap su-how">
        <TickRow kind="switch" checked={!chosen} onChange={(v) => { if (v) follow(); else mark(true) }} title={t('Start with the look’s own theme')} text={t('Each look brings the theme it was made for. Choosing a theme above keeps that one for everyone, whichever look they use.')} />
        <TickRow kind="switch" checked={own} onChange={setOwn} title={t('Let people import their own themes')} text={t('Anyone can add a theme of their own from a ZIP. Requiring a look or a theme of someone is on the People step.')} />
      </div>
      <div className="su-gap">
        <input ref={file} type="file" accept=".zip,application/zip" hidden aria-label={t('Theme ZIP')} onChange={(e) => { const f = e.target.files?.[0]; if (f) void importZip(f); e.target.value = '' }} />
        <Button variant="ghost" onClick={() => file.current?.click()}>{t('Import a theme…')}</Button>
      </div>
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={() => void save()}>{t('Use this')}</Button></StepActions>
    </section>
  )
}
