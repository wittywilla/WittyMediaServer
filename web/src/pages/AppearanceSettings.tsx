import { useEffect, useRef, useState } from 'react'
import { api, getToken, mediaUrl, serverBase } from '../api'
import Tip from '../components/Tip'
import { toast, useApp } from '../state'
import { applyTheme, deviceTheme, setDeviceTheme, type ThemeMeta } from '../theme'
import { setNavStyle, useNavStyle, type NavStyle } from '../navPrefs'

const DOCS = 'https://github.com/wittywilla/WittyMediaServer/blob/master/docs/THEMES.md'

export default function AppearanceSettings() {
  const { user, brandName, setBrandName, serverTheme, setServerTheme } = useApp()
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [mine, setMine] = useState(deviceTheme())
  const [name, setName] = useState(brandName)
  const [filter, setFilter] = useState<'all' | 'dark' | 'light'>('all')
  const fileRef = useRef<HTMLInputElement | null>(null)
  const load = () => api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {})
  useEffect(() => { load() }, [])
  useEffect(() => setName(brandName), [brandName])

  const active = mine || serverTheme
  const useHere = (id: string) => { setDeviceTheme(id); setMine(id); applyTheme(id || serverTheme) }
  const makeDefault = async (id: string) => {
    try { await api.put('/settings', { theme: id }); setServerTheme(id); toast('Server default theme updated') } catch (e: any) { toast(e.message) }
  }
  const saveName = async () => {
    try { await api.put('/settings', { brandName: name.trim() }); setBrandName(name.trim()); toast('Name updated') } catch (e: any) { toast(e.message) }
  }
  const upload = async (f: File) => {
    const headers: Record<string, string> = { 'Content-Type': 'application/zip' }
    const tok = getToken()
    if (tok) headers.Authorization = 'Bearer ' + tok
    const res = await fetch(`${serverBase()}/api/themes?name=${encodeURIComponent(f.name)}`, { method: 'POST', headers, body: f, credentials: 'include' })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) { toast(data.error || 'Upload failed'); return }
    toast(`Installed “${data.name}”`)
    load()
  }
  const remove = async (t: ThemeMeta) => {
    if (!confirm(`Remove the theme “${t.name}”? Its ZIP is deleted from the themes folder.`)) return
    try { await api.del(`/themes/${t.id}`); if (mine === t.id) useHere(''); load() } catch (e: any) { toast(e.message) }
  }
  const shown = themes.filter((t) => filter === 'all' || t.scheme === filter)
  return (
    <>
      <LayoutPanel />
      <div className="panel">
        <h2>Theme</h2>
        <p className="muted" style={{ marginTop: -6 }}>
          Pick a look for this device, {user?.isAdmin ? 'or set the default everyone gets. ' : ''}
          Themes are ZIP files in the server's <code>themes</code> folder — <a href={DOCS} target="_blank" rel="noreferrer">make your own</a>.
        </p>
        <div className="theme-bar">
          <div className="seg">
            {(['all', 'dark', 'light'] as const).map((f) => <button key={f} className={filter === f ? 'active' : ''} onClick={() => setFilter(f)}>{f[0].toUpperCase() + f.slice(1)}</button>)}
          </div>
          <span className="muted" style={{ fontSize: 13 }}>
            {mine ? <>This device uses its own theme. <button className="linkish" onClick={() => useHere('')}>Follow the server default</button></> : 'This device follows the server default.'}
          </span>
        </div>
        <div className="theme-grid" role="radiogroup" aria-label="Themes">
          {shown.map((t) => (
            // The whole card picks the theme; its own buttons stop the click.
            <div key={t.id} className={'theme-card' + (t.id === active ? ' active' : '')} onClick={() => useHere(t.id)}>
              <button type="button" className="swatch" role="radio" aria-checked={t.id === active} aria-label={`${t.name} theme`} title={`Use ${t.name} on this device`}
                onClick={(e) => { e.stopPropagation(); useHere(t.id) }}
                style={{ background: t.preview.bg, color: t.preview.text }}>
                <span className="sw-side" style={{ background: t.preview.surface }} />
                <span className="sw-card" style={{ background: t.preview.surface }} />
                <span className="sw-card" style={{ background: t.preview.surface }} />
                <span className="sw-btn" style={{ background: t.preview.accent }} />
                <span className="sw-text" style={{ background: t.preview.text }} />
              </button>
              <div className="theme-info">
                <div className="t">{t.name} {t.id === serverTheme && <span className="pill">default</span>}</div>
                <div className="muted">{t.description || (t.author ? 'by ' + t.author : '')}</div>
                <div className="theme-actions" onClick={(e) => e.stopPropagation()}>
                  <button className="btn sm" onClick={() => useHere(t.id)} disabled={mine === t.id}>{mine === t.id ? 'In use here' : 'Use here'}</button>
                  {user?.isAdmin && t.id !== serverTheme && <button className="btn sm ghost" onClick={() => makeDefault(t.id)}>Make default</button>}
                  <a className="btn sm ghost" href={mediaUrl(`/api/themes/${t.id}/download`)} title="Download this theme as a starting point for your own">ZIP</a>
                  {user?.isAdmin && !t.builtin && <button className="btn sm danger" onClick={() => remove(t)}>Remove</button>}
                </div>
              </div>
            </div>
          ))}
        </div>
        {user?.isAdmin && (
          <div style={{ marginTop: 16, display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
            <input ref={fileRef} type="file" accept=".zip,application/zip" hidden onChange={(e) => { const f = e.target.files?.[0]; if (f) upload(f); e.target.value = '' }} />
            <button className="btn primary" onClick={() => fileRef.current?.click()}>Install theme ZIP…</button>
            <span className="muted" style={{ fontSize: 13 }}>Or copy the ZIP into the <code>themes</code> folder of the data directory and reload this page.</span>
          </div>
        )}
      </div>
      {user?.isAdmin && (
        <div className="panel">
          <h2>Name</h2>
          <div className="form">
            <label>Name in the top-left corner <Tip>Replaces “WittyMediaServer” in the sidebar, on the sign-in page and in the browser tab. Leave empty for the default. The licence notice in Settings → About always remains.</Tip>
              <div className="field-row">
                <input className="input" value={name} maxLength={40} onChange={(e) => setName(e.target.value)} placeholder="WittyMediaServer" />
                <button className="btn primary" onClick={saveName}>Save</button>
              </div>
            </label>
          </div>
        </div>
      )}
    </>
  )
}

// LayoutPanel: where the navigation sits on this device.
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
      <p className="muted" style={{ marginTop: -6 }}>Where the navigation sits on this device. Phones always also have the bar at the bottom. Rearrange the home page with <b>Customize</b> on the home page.</p>
      <div className="layout-opts" role="radiogroup" aria-label="Navigation">
        {opts.map(([v, label, hint]) => (
          <button key={v} type="button" role="radio" aria-checked={style === v} className={'layout-opt' + (style === v ? ' on' : '')} onClick={() => setNavStyle(v)}>
            <span className={'layout-pic pic-' + v} aria-hidden="true"><i className="s" /><i className="t" /><i className="c" /></span>
            <b>{label}</b><span className="muted">{hint}</span>
          </button>
        ))}
      </div>
    </div>
  )
}
