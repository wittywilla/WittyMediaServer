// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useRef, useState, type CSSProperties } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { IconDownload, IconLock, IconPlus } from '../icons'
import { api, getToken, mediaUrl, serverBase } from '../api'
import BrandDialog, { uploadLogo } from '../components/BrandDialog'
import { Brand, BrandMark, logoUrl } from '../components/common'
import { toast, useApp } from '../state'
import { applyTheme, AUTO_THEME, currentLook, loadThemeOverrides, currentThemeId, CUSTOM_THEME, deviceTheme, setCustomThemeCss, setDeviceTheme, type ThemeMeta } from '../theme'
import NavCustomize from '../shell/NavCustomize'
import { useMenu } from '../shell/useNav'
import { bump } from '../sdui/client'
import { query } from '../sdui/device'
import { useDevice, useFeatures } from '../sdui/useSdui'
import { announce } from '../a11y'
import { globalView, setGlobalView, type ViewPrefs } from '../player/viewPrefs'
import { Button } from '../ui/Button'
import { Segmented } from '../ui/Choice'
import { DENSITIES, setDensity, useDensity, type Density } from '../ui/density'
import { Field, Input, Switch } from '../ui/Field'
import { ask } from '../ui/Overlay'
import { Notice } from '../ui/Surface'
import { makeTheme, themeColors, themeCss, type MakerInput } from '../ui/themeMaker'
import { tiltBlocked } from '../ui/tilt'
import { setTvMode, useTvMode } from '../tvmode/mode'
import { useUserPref } from '../ui/userPrefs'
import { jumpTo } from './settings/jump'
import { LAYOUT_EVENT } from './ShellEditor'
import { Row, Section } from './settings/ui'
import AutoSkipSection from './settings/AutoSkip'

const TvLookSettings = lazy(() => import('./tv/TvLookSettings'))
const EngineColours = lazy(() => import('./settings/EngineColours'))
const PersonalColours = lazy(() => import('./settings/EngineColours').then((m) => ({ default: m.PersonalColours })))

// Appearance: the app's theme first, then colours of your own, where the menus sit and how
// things move, then the look of the parts of the app that have one of their own, and the
// server's name. Each is a section the page's contents and settings search can jump to.

const SCENE_VARS: Record<string, string> = { bg: '--bg', bg2: '--bg-2', bg3: '--bg-3', bg4: '--bg-4', fg: '--fg', fg2: '--fg-2', fg3: '--fg-3', accent: '--accent', accentHover: '--accent-2', accentFg: '--accent-fg', accentText: '--accent-text' }

// A small piece of the app in a theme's colours, drawn with the real classes, so what
// you see is what you get before choosing anything. big: the large preview, with more in it.
function ThemeScene({ colors, radius, preview, big }: { colors?: Record<string, string>; radius?: number; preview?: ThemeMeta['preview']; big?: boolean }) {
  const vars: Record<string, string> = {}
  for (const [k, v] of Object.entries(SCENE_VARS)) if (colors?.[k]) vars[v] = colors[k]
  if (!colors && preview) {
    const p: Record<string, string | undefined> = { '--bg': preview.bg, '--bg-2': preview.surface, '--bg-3': preview.surface, '--fg': preview.text, '--fg-2': preview.text, '--fg-3': preview.text, '--accent': preview.accent, '--accent-text': preview.accent }
    for (const [k, v] of Object.entries(p)) if (v) vars[k] = v
  }
  if (radius !== undefined) vars['--radius'] = radius + 'px'
  return (
    <span className={'theme-scene' + (big ? ' big' : '')} style={vars as CSSProperties} aria-hidden="true">
      <span className="ts-bar"><i className="ts-dot" /><span className="ts-link on">Home</span><span className="ts-link">Movies</span>{big && <><span className="ts-link">Shows</span><span className="ts-link">Music</span></>}</span>
      <span className="ts-body">
        <span className="ts-posters"><i /><i /><i />{big && <><i /><i /><i /></>}</span>
        <span className="ts-text"><b>Title</b><small>A line of detail</small><span className="btn primary sm">Play</span></span>
      </span>
    </span>
  )
}

const FRESH_MAKER: MakerInput = { name: 'My theme', scheme: 'dark', accent: '#2f6fed', tint: '#2f6fed', corners: 'soft' }

// Colours of your own: a few choices, and the rest follows. It shows as you choose, and is
// used once you say so.
function ColourMaker({ start, admin, inUse, onDevice, onServer }: { start: MakerInput; admin: boolean; inUse: boolean; onDevice: (i: MakerInput) => void; onServer: (i: MakerInput) => Promise<void> }) {
  const [i, setI] = useState(start)
  const set = (p: Partial<MakerInput>) => setI((x) => ({ ...x, ...p }))
  const made = makeTheme(i)
  const changed = JSON.stringify(i) !== JSON.stringify(start)
  return (
    <div className="theme-maker">
      <div className="form full">
        <Field label="Name"><Input value={i.name} maxLength={40} onChange={(e) => set({ name: e.target.value })} /></Field>
        <div className="nav-custom-opt"><span>Light or dark</span>
          <Segmented label="Light or dark" value={i.scheme} onChange={(scheme) => set({ scheme })} options={[{ value: 'dark', label: 'Dark' }, { value: 'light', label: 'Light' }]} />
        </div>
        <div className="theme-maker-colours">
          <Field label="Accent" hint="Buttons and highlights"><Input className="theme-colour" type="color" value={i.accent} onChange={(e) => set({ accent: e.target.value })} /></Field>
          <Field label="Background tint" hint="A hint of colour in the page"><Input className="theme-colour" type="color" value={i.tint} onChange={(e) => set({ tint: e.target.value })} /></Field>
        </div>
        <div className="nav-custom-opt"><span>Corners</span>
          <Segmented label="Corners" value={i.corners} onChange={(corners) => set({ corners })} options={[{ value: 'square', label: 'Crisp' }, { value: 'soft', label: 'Soft' }, { value: 'round', label: 'Round' }]} />
        </div>
        <div className="panel-actions">
          <Button variant="primary" disabled={!i.name.trim() || (inUse && !changed)} onClick={() => onDevice(i)}>{inUse && !changed ? 'In use on this device' : 'Use on this device'}</Button>
          {admin && <Button disabled={!i.name.trim()} onClick={() => onServer(i)}>Add for everyone</Button>}
          {changed && <Button variant="ghost" onClick={() => setI(start)}>Undo changes</Button>}
        </div>
      </div>
      <div className="theme-maker-preview">
        <ThemeScene colors={themeColors(made)} radius={made.radius} big />
        {made.adjusted && <Notice>Your accent was adjusted a little so the words on it stay readable.</Notice>}
      </div>
    </div>
  )
}

export default function AppearanceSettings() {
  const { user, brandName, brandColouring, setBrandName, brandLogo, setBrandLogo, serverTheme, setServerTheme } = useApp()
  const [naming, setNaming] = useState(false)
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [mine, setMine] = useState(deviceTheme())
  // The theme the pointer or the focus is on (its card, or a button on it): the large preview
  // shows it before it's chosen.
  const [peek, setPeek] = useState<string | null>(null)
  const [custom, setCustom] = useUserPref<MakerInput | null>('customTheme', null)
  const fileRef = useRef<HTMLInputElement | null>(null)
  const load = () => api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  useEffect(() => { load() }, [])

  const admin = !!user?.isAdmin
  // A profile's own colours, where the server keeps them (feature theme-personal).
  const personalColours = !!useFeatures()?.has('theme-personal')
  const fallback = currentThemeId(serverTheme)
  const active = mine || fallback
  // An administrator can require a theme: it shows locked and nothing here changes it.
  const required = currentLook()?.requiredTheme
  const useHere = (id: string) => { if (required) return; setDeviceTheme(id); setMine(id); applyTheme(id || serverTheme) }
  const makeDefault = async (id: string) => {
    try { await api.put('/settings', { theme: id }); setServerTheme(id); toast('Everyone starts with this theme now', { tone: 'ok' }) } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const saveBrand = async (n: string, logo: File | 'remove' | null, colouring: string) => {
    await api.put('/settings', { brandName: n, brandColouring: colouring })
    setBrandName(n, colouring)
    if (logo === 'remove') { await api.del('/branding/logo'); setBrandLogo('') } else if (logo) setBrandLogo(await uploadLogo(logo))
    toast('Saved', { tone: 'ok' })
  }
  const upload = async (f: File, mineOnly = false) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/zip' }
    const tok = getToken()
    if (tok) headers.Authorization = 'Bearer ' + tok
    const res = await fetch(`${serverBase()}/api/${mineOnly ? 'themes/mine' : 'themes'}?name=${encodeURIComponent(f.name)}`, { method: 'POST', headers, body: f, credentials: 'same-origin' })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) { toast(data.error || 'That theme couldn’t be installed', { tone: 'danger' }); return }
    toast(`“${data.name}” is installed`, { tone: 'ok' })
    load()
  }
  const remove = async (t: ThemeMeta) => {
    if (!await ask({ title: `Remove “${t.name}”?`, body: 'Its ZIP is deleted from the server’s themes folder.', confirm: 'Remove theme', danger: true })) return
    try { await api.del(`/themes/${t.id}`); if (mine === t.id) useHere(''); load() } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const madeForDevice = (i: MakerInput) => {
    setCustomThemeCss(themeCss(i))
    setCustom(i).catch(() => { /* it's still on this device */ })
    useHere(CUSTOM_THEME)
    toast(`“${i.name}” is your theme on this device`, { tone: 'ok' })
  }
  const madeForServer = async (i: MakerInput) => {
    const t = makeTheme(i)
    try {
      const meta = await api.post<ThemeMeta>('/themes/custom', { name: i.name, scheme: i.scheme, colors: t.vars, radius: t.radius })
      await load()
      useHere(meta.id)
      toast(`“${meta.name}” is on the server`, { tone: 'ok' })
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const customMade = custom && makeTheme(custom)
  const colorsOf = (id: string) => themes.find((t) => t.id === id)?.colors

  // What the large preview shows: the theme being looked at, or the one in use.
  const shown = peek ?? (mine === AUTO_THEME || mine === CUSTOM_THEME ? mine : active)
  const shownTheme = themes.find((t) => t.id === shown)
  const shownName = shown === AUTO_THEME ? 'Automatic' : shown === CUSTOM_THEME ? custom?.name || 'Your own' : shownTheme?.name || ''
  const peeking = peek !== null && peek !== (mine || active)
  const hover = (id: string) => ({ onMouseEnter: () => setPeek(id), onFocus: () => setPeek(id) })

  return (
    <>
      <Section title="Theme" desc={required ? <><IconLock /> {lockText(required)}. It can’t be changed here.</> : mine
        ? <>This device has its own theme. <button type="button" className="linkish" onClick={() => useHere('')}>Follow the server’s</button></>
        : 'This device follows the server’s theme. Choosing one here changes this device only.'}>
        <div className="theme-stage">
          {shown === AUTO_THEME
            ? <span className="theme-scene-pair big" aria-hidden="true"><ThemeScene colors={colorsOf('default')} radius={10} big /><ThemeScene colors={colorsOf('light')} radius={10} big /></span>
            : shown === CUSTOM_THEME && customMade
              ? <ThemeScene colors={themeColors(customMade)} radius={customMade.radius} big />
              : <ThemeScene colors={shownTheme?.colors} radius={shownTheme?.radius} preview={shownTheme?.preview} big />}
          {shownName && <p className="theme-stage-caption"><b>{shownName}</b> <span className="muted">{peeking ? 'Preview. Choose it to use it.' : 'In use on this device'}</span></p>}
        </div>
        <div className="theme-grid" role="radiogroup" aria-label="Themes" onMouseLeave={() => setPeek(null)}
          onBlur={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setPeek(null) }}>
          <div className={'theme-card' + (mine === AUTO_THEME ? ' active' : '')} {...hover(AUTO_THEME)}>
            <button type="button" className="theme-pick" role="radio" aria-checked={mine === AUTO_THEME} onClick={() => useHere(AUTO_THEME)}>
              <span className="theme-scene-pair" aria-hidden="true">
                <ThemeScene colors={colorsOf('default')} radius={10} />
                <ThemeScene colors={colorsOf('light')} radius={10} />
              </span>
              <span className="theme-name">Automatic</span>
              <span className="muted">Light or dark, as this device is set.</span>
            </button>
          </div>
          {themes.map((t) => (
            <div key={t.id} className={'theme-card' + (t.id === active ? ' active' : '')} {...hover(t.id)}>
              <button type="button" className="theme-pick" role="radio" aria-checked={t.id === active} onClick={() => useHere(t.id)}>
                <ThemeScene colors={t.colors} radius={t.radius} preview={t.preview} />
                <span className="theme-name">{t.name}{t.id === fallback && <span className="pill">default</span>}</span>
                <span className="muted">{t.description || (t.author ? 'by ' + t.author : '')}</span>
              </button>
              <div className="theme-actions">
                {admin && t.id !== fallback && <Button size="sm" variant="ghost" onClick={() => makeDefault(t.id)}>Make default</Button>}
                {admin && !t.builtin && <Button size="sm" variant="danger" onClick={() => remove(t)}>Remove</Button>}
                <a className="btn sm ghost square theme-dl" href={mediaUrl(`/api/themes/${t.id}/download`)} aria-label={`Download the ${t.name} theme`} title="Download, as a starting point for your own"><IconDownload /></a>
              </div>
            </div>
          ))}
          {custom && customMade && (
            <div className={'theme-card' + (mine === CUSTOM_THEME ? ' active' : '')} {...hover(CUSTOM_THEME)}>
              <button type="button" className="theme-pick" role="radio" aria-checked={mine === CUSTOM_THEME} onClick={() => { setCustomThemeCss(themeCss(custom)); useHere(CUSTOM_THEME) }}>
                <ThemeScene colors={themeColors(customMade)} radius={customMade.radius} />
                <span className="theme-name">{custom.name}</span>
                <span className="muted">Made by you, for this device.</span>
              </button>
              <div className="theme-actions"><Button size="sm" variant="ghost" onClick={() => jumpTo('colours')}>Change its colours</Button></div>
            </div>
          )}
        </div>
        <div className="panel-actions">
          <Button icon={<IconPlus />} onClick={() => jumpTo('colours')}>Make your own</Button>
          {<>
            <input ref={fileRef} type="file" accept=".zip,application/zip" hidden aria-label="Theme ZIP" onChange={(e) => { const f = e.target.files?.[0]; if (f) void upload(f, !admin); e.target.value = '' }} />
            <Button variant="ghost" disabled={!!required} onClick={() => fileRef.current?.click()}>Import a theme…</Button>
          </>}
          <Link className="small-note" to="/help/themes">How themes work</Link>
        </div>
      </Section>
      <Section title="Colours" desc="Make a theme of your own from a few colours, and see it as you go. Every colour is checked as it’s made, so text stays easy to read. WittyTV has colours of its own, further down.">
        <ColourMaker key={custom ? 'mine' : 'fresh'} start={custom || FRESH_MAKER} admin={admin} inUse={mine === CUSTOM_THEME} onDevice={madeForDevice} onServer={madeForServer} />
      </Section>
      <LayoutSection />
      <MotionSection />
      <Suspense fallback={null}><TvLookSettings /></Suspense>
      {admin && <Suspense fallback={null}><EngineColours /></Suspense>}
      {personalColours && <Suspense fallback={null}><PersonalColours /></Suspense>}
      <PlayerSection />
      {/* A profile's own choice. An administrator has it under Playback & discs, a page the others don't get. */}
      {!admin && <AutoSkipSection />}
      {admin && (
        <Section title="Name and logo" desc="Shown in the corner, on the sign-in page and in the browser tab. The license notice in Settings, About always remains.">
          <div className="brand-now">
            <div className="brand"><BrandMark /><Brand name={brandName} /></div>
            <Button onClick={() => setNaming(true)}>Rename…</Button>
          </div>
          <MarkRow />
          {naming && <BrandDialog name={brandName} colouring={brandColouring} logo={brandLogo ? logoUrl(brandLogo) : ''} onClose={() => setNaming(false)} onSave={saveBrand} />}
        </Section>
      )}
    </>
  )
}

// The server's mark: the badge the side rail's header, the phone's tab bar and sign-in draw. A letter by default.
function MarkRow() {
  const [kind, setKind] = useState('letter')
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => { try { const m = JSON.parse(s.brandMark || '{}'); if (m.kind) { setKind(m.kind); setText(m.text || '') } } catch { /* the first letter */ } }).catch(() => {}) }, [])
  const save = async (k: string, t: string) => {
    setBusy(true)
    try {
      await api.put('/settings', { brandMark: k === 'letter' ? '' : JSON.stringify(k === 'text' ? { kind: k, text: t.trim() } : { kind: k }) })
      await loadThemeOverrides()
      toast('Saved', { tone: 'ok' })
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setBusy(false) }
  }
  return (
    <Row label="Mark" desc="The square badge for small spaces: the side rail, the phone’s bar and sign-in. A logo you’ve uploaded is fitted inside it; the letter shows if it can’t load.">
      <Segmented label="Mark" value={kind} onChange={(k) => { setKind(k); if (k !== 'text') void save(k, text) }} options={[{ value: 'letter', label: 'Letter' }, { value: 'initials', label: 'Initials' }, { value: 'text', label: 'Your own' }, { value: 'image', label: 'Logo' }]} />
      {kind === 'text' && <>
        <Input aria-label="Your own one or two characters" maxLength={2} value={text} onChange={(e) => setText(e.target.value)} />
        <Button size="sm" disabled={busy || !text.trim()} onClick={() => void save('text', text)}>Save</Button>
      </>}
    </Row>
  )
}

// The experiences a server offers (docs/SDUI.md, "Experiences"): one for this profile on this device, kept by the
// server; "As the server sets it" follows the administrator's choice. An administrator can require one, and it then
// shows locked with who set it.
type ExperienceCard = { id: string; name: string; tagline?: string; theme?: string; preview?: { bg: string; surface: string; accent: string }; nav: Record<string, string> }
const NAV_PIC: Record<string, string> = { sidebar: 'side', rail: 'rail', railDrawer: 'rail', bottomTabs: 'top', topTabs: 'top', floatingGlassBar: 'top', cableBar: 'top', bevelBar: 'top', teleKeys: 'top', crossbar: 'stream', ornamentTabs: 'top', tvTopMenu: 'top', tvSideMenu: 'rail' }
const lockText = (r: { by?: string; scope: string }) => `Required by ${r.by || 'an administrator'}${r.scope === 'everyone' ? ' for everyone' : r.scope === 'kids' ? ' for kids profiles' : ''}`

function ServerLookRows({ choice }: { choice: NonNullable<ReturnType<typeof useMenu>['experiences']> }) {
  const [busy, setBusy] = useState('')
  const [cards, setCards] = useState<ExperienceCard[]>([])
  const device = useDevice()
  useEffect(() => { api.get<{ experiences: ExperienceCard[] }>('/sdui/experiences').then((r) => setCards(r.experiences)).catch(() => {}) }, [])
  const choose = async (value: string) => {
    setBusy(value)
    // For this device: its name and what it is, as the menu was asked for.
    const path = choice.url.replace(/^\/api(\/v1)?/, '') + (choice.url.includes('?') ? '&' : '?') + query(device)
    try {
      if (value) await api.put(path, { experience: value })
      else await api.del(path)
      bump()
      void loadThemeOverrides()
      announce(value ? `Look: ${choice.options.find((o) => o.value === value)?.label || value}` : 'Look: as the server sets it')
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setBusy('') }
  }
  const locked = !!choice.required
  const current = choice.chosen || ''
  // A required look is the only one shown as on, and nothing can be pressed.
  const options = locked ? [{ value: choice.required!.value, label: cards.find((c) => c.id === choice.required!.value)?.name || choice.required!.value }] : choice.options
  return (
    <>
      {locked && <p className="small-note" role="status"><IconLock /> {lockText(choice.required!)}.</p>}
      <div className="layout-opts look-opts" role="radiogroup" aria-label="Look on this device">
        {options.map((o) => {
          const c = cards.find((x) => x.id === o.value)
          const on = choice.current === o.value
          return (
            <button key={o.value} type="button" role="radio" aria-checked={on} disabled={!!busy || locked} className={'layout-opt' + (on ? ' on' : '')} onClick={() => void choose(o.value)}>
              <span className={'layout-pic pic-' + (NAV_PIC[c?.nav.desktop || ''] || 'side')} aria-hidden="true" style={c?.preview ? { background: c.preview.bg, ['--pic-surface' as string]: c.preview.surface, ['--pic-accent' as string]: c.preview.accent } : undefined}><i className="s" /><i className="t" /><i className="c" /></span>
              <b>{o.label}</b><span className="muted">{c?.tagline || ''}</span>
            </button>
          )
        })}
      </div>
      {!locked && (
        <Row label="As the server sets it" desc={current ? 'This device has a look of its own. Go back to the one the server gives everyone.' : 'This device follows the server’s look.'}>
          <Button size="sm" variant="ghost" disabled={!current || !!busy} onClick={() => void choose('')}>Use the server’s</Button>
        </Row>
      )}
    </>
  )
}

// Where the navigation sits on this device, and what's in it.
function LayoutSection() {
  const menu = useMenu()
  return (
    <Section title="Look" desc={<>How the app looks and is laid out on this device: where the menu is, how pages are drawn, its finish and type. Colours are the theme, above. Kept for you on this device; your other devices keep their own. Rearrange a page with <b>Customise this page</b> at its foot.</>}>
      {menu.experiences && <ServerLookRows choice={menu.experiences} />}
      <Row label="Edit layout" desc="Move the menu, the server’s name, search, the Dock and the spotlight, or hide what you don’t use: for you on this device, or for everyone."><Button onClick={() => window.dispatchEvent(new Event(LAYOUT_EVENT))}>Edit layout…</Button></Row>
      <NavCustomize />
      <DensityRow />
    </Section>
  )
}

// How closely things are spaced on this device.
function DensityRow() {
  const d = useDensity()
  const names: Record<Density, string> = { comfortable: 'Comfortable', compact: 'Compact', spacious: 'Spacious' }
  return (
    <Row label="Spacing" desc="How closely things sit on this device. Compact fits more on the screen; spacious gives everything room. Touch screens keep their full-size buttons.">
      <Segmented label="Spacing" value={d} onChange={(v) => setDensity(v as Density)} options={DENSITIES.map((v) => ({ value: v, label: names[v] }))} />
    </Row>
  )
}

// Moving posters, and this screen's distance from the sofa.
function MotionSection() {
  const [tilt, setTilt] = useUserPref('posterTilt', true)
  const [canvases, setCanvases] = useUserPref('canvases', true)
  const tv = useTvMode()
  const go = useNavigate()
  const blocked = tiltBlocked()
  return (
    <Section title="Motion and big screens">
      <Row label="Moving posters" desc={<>Movie and show posters lean toward the pointer, or settle in as a remote reaches them. {blocked ? `Off here: ${blocked}` : 'Follows you to your other devices.'}</>}>
        <Switch label="Moving posters" checked={tilt && !blocked} disabled={!!blocked} onChange={(on) => setTilt(on).catch(() => toast('Could not save that', { tone: 'danger' }))} />
      </Row>
      <Row label="Moving backgrounds" desc="A song, album or title that has a canvas plays it behind the page: a short, silent, looping video. Off shows the still artwork. It's also left out when this device is set to reduce motion or save data, and away from home when streams are limited. Follows you to your other devices.">
        <Switch label="Moving backgrounds" checked={canvases} onChange={(on) => setCanvases(on).catch(() => toast('Could not save that', { tone: 'danger' }))} />
      </Row>
      <Row label="TV mode" desc="Laid out and driven like the Apple TV and Android TV apps, with a remote or a controller, for a screen across the room. Only on this device.">
        <Switch label="TV mode" checked={tv} onChange={(on) => { setTvMode(on); if (on) go('/') }} />
      </Row>
    </Section>
  )
}

// How videos look while they play: the player's own gear menu has the same, and can also keep
// choices for one disc or video, which then win over these.
function PlayerSection() {
  const [v, setV] = useState(globalView)
  const set = (p: Partial<ViewPrefs>) => { setGlobalView(p); setV((x) => ({ ...x, ...p })) }
  return (
    <Section title="Player" desc="How videos look while they play, on this device. A disc or video you’ve set differently from the player’s own menu keeps its own.">
      <Row label="Seek bar" desc="The timeline under the picture."><Switch label="Seek bar" checked={v.seekBar} onChange={(seekBar) => set({ seekBar })} /></Row>
      <Row label="Chapter marks" desc="Where chapters start, on the timeline."><Switch label="Chapter marks" checked={v.chapterMarks} onChange={(chapterMarks) => set({ chapterMarks })} /></Row>
      <Row label="Hide the controls after">
        <Segmented label="Hide the controls after" value={String(v.hideAfter)} onChange={(s) => set({ hideAfter: Number(s) })}
          options={[{ value: '2', label: '2 s' }, { value: '3.5', label: '3.5 s' }, { value: '6', label: '6 s' }, { value: '0', label: 'Never' }]} />
      </Row>
      <Row label="Picture" desc="Fit shows all of it, with black bars where the shapes differ. Fill zooms in to fill the screen.">
        <Segmented label="Picture" value={v.picture} onChange={(picture) => set({ picture })} options={[{ value: 'fit', label: 'Fit' }, { value: 'fill', label: 'Fill screen' }]} />
      </Row>
      <Row label="Disc subtitle size" desc="Subtitles drawn by DVDs and Blu-rays.">
        <Segmented label="Disc subtitle size" value={String(v.subtitleSize)} onChange={(s) => set({ subtitleSize: Number(s) })}
          options={[{ value: '0.8', label: 'S' }, { value: '1', label: 'M' }, { value: '1.25', label: 'L' }, { value: '1.5', label: 'XL' }]} />
      </Row>
    </Section>
  )
}
