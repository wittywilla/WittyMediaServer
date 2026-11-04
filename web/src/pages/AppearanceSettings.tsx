import { lazy, Suspense, useEffect, useRef, useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { IconDownload, IconPlus } from '../icons'
import { api, getToken, mediaUrl, serverBase } from '../api'
import BrandDialog, { uploadLogo } from '../components/BrandDialog'
import { Brand, BrandMark, logoUrl } from '../components/common'
import { toast, useApp } from '../state'
import { applyTheme, AUTO_THEME, currentThemeId, CUSTOM_THEME, deviceTheme, setCustomThemeCss, setDeviceTheme, type ThemeMeta } from '../theme'
import { setNavStyle, useNavStyle, type NavStyle } from '../navPrefs'
import NavCustomize from '../shell/NavCustomize'
import { globalView, setGlobalView, type ViewPrefs } from '../player/viewPrefs'
import { Button } from '../ui/Button'
import { Segmented } from '../ui/Choice'
import { Field, Input, Switch } from '../ui/Field'
import { ask } from '../ui/Overlay'
import { Notice } from '../ui/Surface'
import { makeTheme, themeColors, themeCss, type MakerInput } from '../ui/themeMaker'
import { tiltBlocked } from '../ui/tilt'
import { getTenFoot, setTenFoot } from '../ui/tenFoot'
import { useUserPref } from '../ui/userPrefs'
import { jumpTo } from './settings/jump'
import { Row, Section } from './settings/ui'

const TvLookSettings = lazy(() => import('./tv/TvLookSettings'))

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
  const { user, brandName, setBrandName, brandLogo, setBrandLogo, serverTheme, setServerTheme } = useApp()
  const [naming, setNaming] = useState(false)
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [mine, setMine] = useState(deviceTheme())
  // The theme the pointer or the focus is on: the large preview shows it before it's chosen.
  const [peek, setPeek] = useState<string | null>(null)
  const [custom, setCustom] = useUserPref<MakerInput | null>('customTheme', null)
  const fileRef = useRef<HTMLInputElement | null>(null)
  const load = () => api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  useEffect(() => { load() }, [])

  const admin = !!user?.isAdmin
  const fallback = currentThemeId(serverTheme)
  const active = mine || fallback
  const useHere = (id: string) => { setDeviceTheme(id); setMine(id); applyTheme(id || serverTheme) }
  const makeDefault = async (id: string) => {
    try { await api.put('/settings', { theme: id }); setServerTheme(id); toast('Everyone starts with this theme now', { tone: 'ok' }) } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const saveBrand = async (n: string, logo: File | 'remove' | null) => {
    await api.put('/settings', { brandName: n })
    setBrandName(n)
    if (logo === 'remove') { await api.del('/branding/logo'); setBrandLogo('') } else if (logo) setBrandLogo(await uploadLogo(logo))
    toast('Saved', { tone: 'ok' })
  }
  const upload = async (f: File) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/zip' }
    const tok = getToken()
    if (tok) headers.Authorization = 'Bearer ' + tok
    const res = await fetch(`${serverBase()}/api/themes?name=${encodeURIComponent(f.name)}`, { method: 'POST', headers, body: f, credentials: 'same-origin' })
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
      <Section title="Theme" desc={mine
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
          <div className={'theme-card' + (mine === AUTO_THEME ? ' active' : '')}>
            <button type="button" className="theme-pick" role="radio" aria-checked={mine === AUTO_THEME} onClick={() => useHere(AUTO_THEME)} {...hover(AUTO_THEME)}>
              <span className="theme-scene-pair" aria-hidden="true">
                <ThemeScene colors={colorsOf('default')} radius={10} />
                <ThemeScene colors={colorsOf('light')} radius={10} />
              </span>
              <span className="theme-name">Automatic</span>
              <span className="muted">Light or dark, as this device is set.</span>
            </button>
          </div>
          {themes.map((t) => (
            <div key={t.id} className={'theme-card' + (t.id === active ? ' active' : '')}>
              <button type="button" className="theme-pick" role="radio" aria-checked={t.id === active} onClick={() => useHere(t.id)} {...hover(t.id)}>
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
            <div className={'theme-card' + (mine === CUSTOM_THEME ? ' active' : '')}>
              <button type="button" className="theme-pick" role="radio" aria-checked={mine === CUSTOM_THEME} onClick={() => madeForDevice(custom)} {...hover(CUSTOM_THEME)}>
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
          {admin && <>
            <input ref={fileRef} type="file" accept=".zip,application/zip" hidden aria-label="Theme ZIP" onChange={(e) => { const f = e.target.files?.[0]; if (f) upload(f); e.target.value = '' }} />
            <Button variant="ghost" onClick={() => fileRef.current?.click()}>Install a theme ZIP…</Button>
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
      <PlayerSection />
      {admin && (
        <Section title="Name and logo" desc="Shown in the corner, on the sign-in page and in the browser tab. The license notice in Settings, About always remains.">
          <div className="brand-now">
            <div className="brand"><BrandMark /><Brand name={brandName} /></div>
            <Button onClick={() => setNaming(true)}>Rename…</Button>
          </div>
          {naming && <BrandDialog name={brandName} logo={brandLogo ? logoUrl(brandLogo) : ''} onClose={() => setNaming(false)} onSave={saveBrand} />}
        </Section>
      )}
    </>
  )
}

// Where the navigation sits on this device, and what's in it.
function LayoutSection() {
  const style = useNavStyle()
  const opts: [NavStyle, string, string][] = [
    ['side', 'Side menu', 'Libraries in a menu on the left'],
    ['top', 'Top bar', 'Libraries along the top, more room for posters'],
    ['both', 'Both', 'Top bar plus the side menu'],
  ]
  return (
    <Section title="Layout" desc={<>Where the navigation sits on this device. Phones always have the bar at the bottom instead. Rearrange the home page with <b>Customize home</b> at the foot of the home page.</>}>
      <div className="layout-opts" role="radiogroup" aria-label="Navigation">
        {opts.map(([v, label, hint]) => (
          <button key={v} type="button" role="radio" aria-checked={style === v} className={'layout-opt' + (style === v ? ' on' : '')} onClick={() => setNavStyle(v)}>
            <span className={'layout-pic pic-' + v} aria-hidden="true"><i className="s" /><i className="t" /><i className="c" /></span>
            <b>{label}</b><span className="muted">{hint}</span>
          </button>
        ))}
      </div>
      <NavCustomize />
    </Section>
  )
}

// Moving posters, and this screen's distance from the sofa.
function MotionSection() {
  const [tilt, setTilt] = useUserPref('posterTilt', true)
  const [ten, setTen] = useState(getTenFoot)
  const blocked = tiltBlocked()
  return (
    <Section title="Motion and big screens">
      <Row label="Moving posters" desc={<>Movie and show posters lean toward the pointer, or settle in as a remote reaches them. {blocked ? `Off here: ${blocked}` : 'Follows you to your other devices.'}</>}>
        <Switch label="Moving posters" checked={tilt && !blocked} disabled={!!blocked} onChange={(on) => setTilt(on).catch(() => toast('Could not save that', { tone: 'danger' }))} />
      </Row>
      <Row label="Big screen mode" desc="Larger text and a stronger focus outline, for a TV across the room. This device only.">
        <Switch label="Big screen mode" checked={ten} onChange={(on) => { setTenFoot(on); setTen(on) }} />
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
