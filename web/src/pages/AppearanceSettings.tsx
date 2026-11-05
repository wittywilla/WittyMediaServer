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
import { Button } from '../ui/Button'
import { Segmented } from '../ui/Choice'
import { Field, Input, Switch } from '../ui/Field'
import { ask, Dialog } from '../ui/Overlay'
import { Notice } from '../ui/Surface'
import { makeTheme, themeColors, themeCss, type MakerInput } from '../ui/themeMaker'
import { tiltBlocked } from '../ui/tilt'
import { getTenFoot, setTenFoot } from '../ui/tenFoot'
import { useUserPref } from '../ui/userPrefs'


const TvLookSettings = lazy(() => import('./tv/TvLookSettings'))

const SCENE_VARS: Record<string, string> = { bg: '--bg', bg2: '--bg-2', bg3: '--bg-3', bg4: '--bg-4', fg: '--fg', fg2: '--fg-2', fg3: '--fg-3', accent: '--accent', accentHover: '--accent-2', accentFg: '--accent-fg', accentText: '--accent-text' }

// A small piece of the app in a theme's colours, drawn with the real classes, so what
// you see is what you get before choosing anything.
function ThemeScene({ colors, radius, preview }: { colors?: Record<string, string>; radius?: number; preview?: ThemeMeta['preview'] }) {
  const vars: Record<string, string> = {}
  for (const [k, v] of Object.entries(SCENE_VARS)) if (colors?.[k]) vars[v] = colors[k]
  if (!colors && preview) {
    const p: Record<string, string | undefined> = { '--bg': preview.bg, '--bg-2': preview.surface, '--bg-3': preview.surface, '--fg': preview.text, '--fg-2': preview.text, '--fg-3': preview.text, '--accent': preview.accent, '--accent-text': preview.accent }
    for (const [k, v] of Object.entries(p)) if (v) vars[k] = v
  }
  if (radius !== undefined) vars['--radius'] = radius + 'px'
  return (
    <span className="theme-scene" style={vars as CSSProperties} aria-hidden="true">
      <span className="ts-bar"><i className="ts-dot" /><span className="ts-link on">Home</span><span className="ts-link">Movies</span></span>
      <span className="ts-body">
        <span className="ts-posters"><i /><i /><i /></span>
        <span className="ts-text"><b>Title</b><small>A line of detail</small><span className="btn primary sm">Play</span></span>
      </span>
    </span>
  )
}

const FRESH_MAKER: MakerInput = { name: 'My theme', scheme: 'dark', accent: '#2f6fed', tint: '#2f6fed', corners: 'soft' }

function ThemeMaker({ start, admin, onClose, onDevice, onServer }: { start: MakerInput; admin: boolean; onClose: () => void; onDevice: (i: MakerInput) => void; onServer: (i: MakerInput) => Promise<void> }) {
  const [i, setI] = useState(start)
  const set = (p: Partial<MakerInput>) => setI((x) => ({ ...x, ...p }))
  const made = makeTheme(i)
  return (
    <Dialog title="Make a theme" size="wide" onClose={onClose} description="Choose a few things and the rest follows. Every colour is checked as it’s made, so text stays easy to read."
      actions={<>
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        {admin && <Button disabled={!i.name.trim()} onClick={() => onServer(i)}>Add for everyone</Button>}
        <Button variant="primary" disabled={!i.name.trim()} onClick={() => onDevice(i)}>Use on this device</Button>
      </>}>
      <div className="theme-maker">
        <div className="form full">
          <Field label="Name"><Input autoFocus value={i.name} maxLength={40} onChange={(e) => set({ name: e.target.value })} /></Field>
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
        </div>
        <div className="theme-maker-preview">
          <ThemeScene colors={themeColors(made)} radius={made.radius} />
          {made.adjusted && <Notice>Your accent was adjusted a little so the words on it stay readable.</Notice>}
        </div>
      </div>
    </Dialog>
  )
}

export default function AppearanceSettings() {
  const { user, brandName, brandColouring, setBrandName, brandLogo, setBrandLogo, serverTheme, setServerTheme } = useApp()
  const [naming, setNaming] = useState(false)
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [mine, setMine] = useState(deviceTheme())
  const [making, setMaking] = useState(false)
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
  const saveBrand = async (n: string, logo: File | 'remove' | null, colouring: string) => {
    await api.put('/settings', { brandName: n, brandColouring: colouring })
    setBrandName(n, colouring)
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
    setMaking(false)
  }
  const madeForServer = async (i: MakerInput) => {
    const t = makeTheme(i)
    try {
      const meta = await api.post<ThemeMeta>('/themes/custom', { name: i.name, scheme: i.scheme, colors: t.vars, radius: t.radius })
      setMaking(false)
      await load()
      useHere(meta.id)
      toast(`“${meta.name}” is on the server`, { tone: 'ok' })
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const customMade = custom && makeTheme(custom)
  return (
    <>
      <div className="panel">
        <h2>Theme</h2>
        <p className="panel-note">
          {mine ? <>This device has its own theme. <button type="button" className="linkish" onClick={() => useHere('')}>Follow the server’s</button></> : 'This device follows the server’s theme. Choosing one here changes this device only.'}
        </p>
        <div className="theme-grid" role="radiogroup" aria-label="Themes">
          <div className={'theme-card' + (mine === AUTO_THEME ? ' active' : '')}>
            <button type="button" className="theme-pick" role="radio" aria-checked={mine === AUTO_THEME} onClick={() => useHere(AUTO_THEME)}>
              <span className="theme-scene-pair" aria-hidden="true">
                <ThemeScene colors={themes.find((t) => t.id === 'default')?.colors} radius={10} />
                <ThemeScene colors={themes.find((t) => t.id === 'light')?.colors} radius={10} />
              </span>
              <span className="theme-name">Automatic</span>
              <span className="muted">Light or dark, as this device is set.</span>
            </button>
          </div>
          {themes.map((t) => (
            <div key={t.id} className={'theme-card' + (t.id === active ? ' active' : '')}>
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
            <div className={'theme-card' + (mine === CUSTOM_THEME ? ' active' : '')}>
              <button type="button" className="theme-pick" role="radio" aria-checked={mine === CUSTOM_THEME} onClick={() => madeForDevice(custom)}>
                <ThemeScene colors={themeColors(customMade)} radius={customMade.radius} />
                <span className="theme-name">{custom.name}</span>
                <span className="muted">Made by you, for this device.</span>
              </button>
              <div className="theme-actions"><Button size="sm" variant="ghost" onClick={() => setMaking(true)}>Edit</Button></div>
            </div>
          )}
        </div>
        <div className="panel-actions">
          <Button icon={<IconPlus />} onClick={() => setMaking(true)}>Make a theme</Button>
          {admin && <>
            <input ref={fileRef} type="file" accept=".zip,application/zip" hidden aria-label="Theme ZIP" onChange={(e) => { const f = e.target.files?.[0]; if (f) upload(f); e.target.value = '' }} />
            <Button variant="ghost" onClick={() => fileRef.current?.click()}>Install a theme ZIP…</Button>
          </>}
          <Link className="small-note" to="/help/themes">How themes work</Link>
        </div>
        {making && <ThemeMaker start={custom || FRESH_MAKER} admin={admin} onClose={() => setMaking(false)} onDevice={madeForDevice} onServer={madeForServer} />}
      </div>
      <LayoutPanel />
      <MotionPanel />
      <Suspense fallback={null}><TvLookSettings /></Suspense>
      {admin && (
        <div className="panel">
          <h2>Name and logo</h2>
          <div className="brand-now">
            <div className="brand"><BrandMark /><Brand name={brandName} /></div>
            <Button onClick={() => setNaming(true)}>Rename…</Button>
          </div>
          <p className="small-note">Shown in the corner, on the sign-in page and in the browser tab. The license notice in Settings, About always remains.</p>
          {naming && <BrandDialog name={brandName} colouring={brandColouring} logo={brandLogo ? logoUrl(brandLogo) : ''} onClose={() => setNaming(false)} onSave={saveBrand} />}
        </div>
      )}
    </>
  )
}

// LayoutPanel: where the navigation sits on this device, and what's in it.
function LayoutPanel() {
  const style = useNavStyle()
  const opts: [NavStyle, string, string][] = [
    ['side', 'Side menu', 'Libraries in a menu on the left'],
    ['top', 'Top bar', 'Libraries along the top, more room for posters'],
    ['both', 'Both', 'Top bar plus the side menu'],
  ]
  return (
    <div className="panel">
      <h2>Layout</h2>
      <p className="panel-note">Where the navigation sits on this device. Phones always have the bar at the bottom instead. Rearrange the home page with <b>Customize home</b> at the foot of the home page.</p>
      <div className="layout-opts" role="radiogroup" aria-label="Navigation">
        {opts.map(([v, label, hint]) => (
          <button key={v} type="button" role="radio" aria-checked={style === v} className={'layout-opt' + (style === v ? ' on' : '')} onClick={() => setNavStyle(v)}>
            <span className={'layout-pic pic-' + v} aria-hidden="true"><i className="s" /><i className="t" /><i className="c" /></span>
            <b>{label}</b><span className="muted">{hint}</span>
          </button>
        ))}
      </div>
      <NavCustomize />
    </div>
  )
}

// Moving posters, and this screen's distance from the sofa.
function MotionPanel() {
  const [tilt, setTilt] = useUserPref('posterTilt', true)
  const [ten, setTen] = useState(getTenFoot)
  const blocked = tiltBlocked()
  return (
    <div className="panel">
      <h2>Motion and big screens</h2>
      <div className="set-row-inline">
        <span id="pref-tilt"><b>Moving posters</b><small className="muted block">Movie and show posters lean toward the pointer, or settle in as a remote reaches them. {blocked ? `Off here: ${blocked}` : 'Follows you to your other devices.'}</small></span>
        <Switch labelledBy="pref-tilt" checked={tilt && !blocked} disabled={!!blocked} onChange={(on) => setTilt(on).catch(() => toast('Could not save that', { tone: 'danger' }))} />
      </div>
      <div className="set-row-inline">
        <span id="pref-ten"><b>Big screen mode</b><small className="muted block">Larger text and a stronger focus outline, for a TV across the room. This device only.</small></span>
        <Switch labelledBy="pref-ten" checked={ten} onChange={(on) => { setTenFoot(on); setTen(on) }} />
      </div>
    </div>
  )
}
