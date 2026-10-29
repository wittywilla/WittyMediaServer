import { ChooseFolderButton } from '../components/FolderPicker'
import Profiles from './Profiles'
import { useEffect, useState } from 'react'
import { NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { api, getPrefs, type Library } from '../api'
import { IconFolder, IconRefresh, IconTrash } from '../icons'
import { toast, useApp } from '../state'
import TranscodeSettings from './TranscodeSettings'
import AppearanceSettings from './AppearanceSettings'
import LogSettings from './LogSettings'

export default function SettingsPage() {
  const { user } = useApp()
  return (
    <>
      <div className="toolbar"><h1>Settings</h1></div>
      <div className="tabs">
        {user?.isAdmin && <NavLink to="/settings/libraries" className={({ isActive }) => (isActive ? 'active' : '')}>Libraries</NavLink>}
        {user?.isAdmin && <NavLink to="/settings/server" className={({ isActive }) => (isActive ? 'active' : '')}>Server</NavLink>}
        {user?.isAdmin && <NavLink to="/settings/transcoding" className={({ isActive }) => (isActive ? 'active' : '')}>Transcoding</NavLink>}
        <NavLink to="/settings/appearance" className={({ isActive }) => (isActive ? 'active' : '')}>Appearance</NavLink>
        <NavLink to="/settings/users" className={({ isActive }) => (isActive ? 'active' : '')}>Users</NavLink>
        {user?.isAdmin && <NavLink to="/settings/logs" className={({ isActive }) => (isActive ? 'active' : '')}>Logs &amp; privacy</NavLink>}
        <NavLink to="/settings/about" className={({ isActive }) => (isActive ? 'active' : '')}>About &amp; legal</NavLink>
      </div>
      <Routes>
        <Route path="libraries" element={<Libraries />} />
        <Route path="server" element={<ServerSettings />} />
        <Route path="transcoding" element={<TranscodeSettings />} />
        <Route path="appearance" element={<AppearanceSettings />} />
        <Route path="users" element={<Users />} />
        <Route path="logs" element={<LogSettings />} />
        <Route path="about" element={<About />} />
        <Route path="*" element={<Navigate to={user?.isAdmin ? 'libraries' : 'users'} replace />} />
      </Routes>
    </>
  )
}

// ---------------------------------------------------------------- libraries

function Libraries() {
  const { libraries, refreshLibraries } = useApp()
  const [editing, setEditing] = useState<Partial<Library> | null>(null)
  const remove = async (l: Library) => {
    if (!confirm(`Delete library “${l.name}”? Files on disk are not touched.`)) return
    await api.del(`/libraries/${l.id}`)
    await refreshLibraries()
  }
  const scan = async (l: Library) => { await api.post(`/libraries/${l.id}/scan`); toast('Scan started'); setTimeout(refreshLibraries, 1000) }
  return (
    <>
      <div className="panel">
        <h2>Libraries</h2>
        <div className="list">
          {libraries.map((l) => (
            <div className="list-item" key={l.id}>
              <IconFolder />
              <div className="grow">
                <div><b>{l.name}</b> <span className="muted">· {l.type} · {l.itemCount} items{l.scanning ? ' · scanning…' : ''}</span></div>
                <div className="sub">{l.paths.join(' · ')}</div>
              </div>
              <button className="btn sm ghost" onClick={() => scan(l)} title="Scan"><IconRefresh /></button>
              <button className="btn sm ghost" onClick={() => setEditing(l)}>Edit</button>
              <button className="btn sm danger" onClick={() => remove(l)}><IconTrash /></button>
            </div>
          ))}
          {libraries.length === 0 && <div className="muted">No libraries yet.</div>}
        </div>
        <div style={{ marginTop: 14 }}><button className="btn primary" onClick={() => setEditing({ name: '', type: 'movies', paths: [] })}>Add library</button></div>
      </div>
      {editing && <LibraryEditor lib={editing} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await refreshLibraries() }} />}
    </>
  )
}

function LibraryEditor({ lib, onClose, onSaved }: { lib: Partial<Library>; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(lib.name || '')
  const [type, setType] = useState(lib.type || 'movies')
  const [paths, setPaths] = useState<string[]>(lib.paths || [])
  const [manual, setManual] = useState('')
  const [err, setErr] = useState('')
  const save = async () => {
    try {
      const body = { name, type, paths }
      if (lib.id) await api.put(`/libraries/${lib.id}`, body); else await api.post('/libraries', body)
      onSaved()
    } catch (e: any) { setErr(e.message) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>{lib.id ? 'Edit library' : 'Add library'}</h2>
        <div className="form" style={{ maxWidth: 'none' }}>
          <div className="row2">
            <label>Name<input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Movies" /></label>
            <label>Type<select className="select" value={type} onChange={(e) => setType(e.target.value)}>
              <option value="movies">Movies and discs</option><option value="shows">TV shows</option><option value="anime">Anime (AniList information)</option><option value="videos">Home and other videos (no online lookup)</option>
            </select></label>
          </div>
          <label>Folders
            <div className="list">
              {paths.map((p) => <div className="list-item" key={p}><IconFolder /><div className="grow sub" style={{ color: 'var(--fg)' }}>{p}</div><button className="btn sm ghost" onClick={() => setPaths(paths.filter((x) => x !== p))}>Remove</button></div>)}
            </div>
          </label>
          <div className="field-row">
            <ChooseFolderButton className="btn primary" onPick={(p) => setPaths([...new Set([...paths, p])])} />
            <input className="input" placeholder="…or type a folder path" aria-label="Folder path" value={manual} onChange={(e) => setManual(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter' && manual) { setPaths([...new Set([...paths, manual])]); setManual('') } }} />
            <button className="btn" onClick={() => { if (manual) { setPaths([...new Set([...paths, manual])]); setManual('') } }}>Add</button>
          </div>
          {err && <div className="error">{err}</div>}
        </div>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" onClick={save} disabled={!name || paths.length === 0}>Save &amp; scan</button></div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- server

function ServerSettings() {
  const [s, setS] = useState<Record<string, string> | null>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then(setS) }, [])
  if (!s) return null
  const up = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setS({ ...s, [k]: e.target.type === 'checkbox' ? ((e.target as HTMLInputElement).checked ? '1' : '0') : e.target.value })
  const save = async () => { setS(await api.put('/settings', s)); getPrefs(true); setSaved(true); setTimeout(() => setSaved(false), 2000) }
  const check = (k: string, label: React.ReactNode) => (
    <label style={{ flexDirection: 'row', alignItems: 'center', display: 'flex', gap: 10 }}>
      <input type="checkbox" checked={s[k] === '1'} onChange={up(k)} /> {label}
    </label>
  )
  return (
    <div className="panel">
      <h2>Server</h2>
      <div className="form">
        <label>TMDB API key <span className="muted">(optional — enables posters, overviews and ratings; this product uses the TMDB API but is not endorsed by TMDB)</span>
          <input className="input" value={s.tmdbKey || ''} onChange={up('tmdbKey')} placeholder="Paste your API key (v3)" />
        </label>
        <div className="row2">
          <label>TheTVDB API key <span className="muted">(TV libraries — free at thetvdb.com/api-information; metadata provided by TheTVDB)</span>
            <input className="input" value={s.tvdbKey || ''} onChange={up('tvdbKey')} placeholder="Paste your v4 API key" />
          </label>
          <label>TheTVDB subscriber PIN <span className="muted">(only for user-supported keys)</span>
            <input className="input" value={s.tvdbPin || ''} onChange={up('tvdbPin')} placeholder="optional" />
          </label>
        </div>
        {check('tvmaze', <>Use <b>TVmaze</b> for TV shows when there is no TheTVDB key <span className="muted">(free, no key; data shared under CC BY-SA 4.0)</span></>)}
        {check('wikidata', <>Use <b>Wikidata</b> and Wikipedia for movie facts and ratings when there is no TMDB key <span className="muted">(free, no key; no posters)</span></>)}
        {check('saveMetadata', <>Save metadata and artwork next to media files <span className="muted">(standard NFO + poster.jpg/fanart.jpg that other media servers read too; existing NFOs are backed up as .nfo.bak)</span></>)}
        <div className="muted" style={{ fontSize: 13 }}>Encoders, direct play, resolution and quality are under <NavLink to="/settings/transcoding">Transcoding</NavLink>.</div>
        <h3 className="section-title" style={{ margin: '8px 0 0' }}>Anime</h3>
        <label>Anime kept in TV libraries appears in <span className="muted">(shows with the genre “Anime”; needs an Anime library)</span>
          <select className="select" value={s.animeIn || 'both'} onChange={up('animeIn')}>
            <option value="both">Both the Anime library and the TV library</option>
            <option value="anime">Only the Anime library</option>
          </select>
        </label>
        <label>Anime titles
          <select className="select" value={s.animeTitles || 'english'} onChange={up('animeTitles')}>
            <option value="english">English (e.g. Attack on Titan)</option>
            <option value="romaji">Rōmaji (e.g. Shingeki no Kyojin)</option>
            <option value="native">Japanese (e.g. 進撃の巨人)</option>
          </select>
        </label>
        <label>TV episode layout <span className="muted">(default for every show; each show can override it with the toggle on its page)</span>
          <select className="select" value={s.episodeView || 'cards'} onChange={up('episodeView')}>
            <option value="cards">Cards with thumbnails and overviews</option><option value="list">Compact playlist</option>
          </select>
        </label>
        <label>Fastest playback speed <span className="muted">(viewers pick a speed with the gauge button in the player; the pitch of voices is kept at every speed)</span>
          <select className="select" value={s.maxSpeed || '4'} onChange={up('maxSpeed')}>
            <option value="2">2×</option><option value="4">4× (recommended)</option><option value="6">6×</option><option value="8">8×</option><option value="16">16× (browser limit)</option>
          </select>
        </label>
        {Number(s.maxSpeed || 4) > 4 && (
          <div className="warn-box" role="note">
            <strong>Above 4× needs a strong server and the right files.</strong> At 8× the server has to decode and re-encode eight
            seconds of video every second for anything that is transcoded (DVDs, Blu-rays, and files the browser cannot play directly).
            Plan on a recent 6-core CPU or a hardware encoder (NVENC, Quick Sync, AMF, VideoToolbox) for 1080p, and a fast disk.
            Files in <b>H.264 or HEVC with AAC audio</b> in MP4/MKV direct-play and cost the server almost nothing at any speed.
            Most browsers also mute sound above 4×.
          </div>
        )}
        <h3 className="section-title" style={{ margin: '8px 0 0' }}>WittyDVD</h3>
        <label>When a DVD is inserted
          <select className="select" value={s.dvdStart || 'menu'} onChange={up('dvdStart')}>
            <option value="menu">Start with the disc menu (like a DVD player)</option><option value="title">Skip menus and play the main feature</option>
          </select>
        </label>
        {check('insertFx', <>Insert DVD effect <span className="muted">(tray animation when a disc starts; each disc can override this in its Disc settings)</span></>)}
        <div style={{ marginLeft: 28, opacity: s.insertFx === '0' ? 0.5 : 1 }}>{check('insertSound', <>Play the drive sounds with it <span className="muted">(tray motor and disc spin-up)</span></>)}</div>
        <div className="row2">
          <label>Preferred DVD menu language<input className="input" value={s.menuLanguage || ''} onChange={up('menuLanguage')} placeholder="en" /></label>
          <label>Preferred audio language<input className="input" value={s.audioLanguage || ''} onChange={up('audioLanguage')} placeholder="en" /></label>
        </div>
        <label>Preferred subtitle language<input className="input" value={s.subtitleLanguage || ''} onChange={up('subtitleLanguage')} placeholder="leave empty for none" /></label>
        <div><button className="btn primary" onClick={save}>Save</button> {saved && <span className="ok" style={{ marginLeft: 10 }}>Saved</span>}</div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- users

function Users() {
  const { user } = useApp()
  return (
    <>
      <Profiles />
      {user?.isAdmin && <SignInScreen />}
    </>
  )
}

// SignInScreen chooses between the "Who's watching?" profile screen and the
// classic name + password form.
function SignInScreen() {
  const [s, setS] = useState<Record<string, string> | null>(null)
  useEffect(() => { api.get<Record<string, string>>('/settings').then(setS).catch(() => {}) }, [])
  if (!s) return null
  const save = async (patch: Record<string, string>) => {
    try { setS(await api.put<Record<string, string>>('/settings', patch)); toast('Sign-in screen updated') } catch (e: any) { toast(e.message) }
  }
  const profiles = (s.loginStyle || 'profiles') === 'profiles'
  return (
    <div className="panel">
      <h2>Sign-in screen</h2>
      <div className="form">
        <div className="seg big" role="radiogroup" aria-label="Sign-in screen">
          <button role="radio" aria-checked={profiles} className={profiles ? 'active' : ''} onClick={() => save({ loginStyle: 'profiles' })}>Who's watching? (profiles)</button>
          <button role="radio" aria-checked={!profiles} className={!profiles ? 'active' : ''} onClick={() => save({ loginStyle: 'classic' })}>Classic name and password</button>
        </div>
        <label>Who may create and change Kids profiles
          <select className="select" value={s.kidsManagedBy || 'adults'} onChange={(e) => save({ kidsManagedBy: e.target.value })}>
            <option value="adults">Every adult profile</option>
            <option value="admin">Only the administrator</option>
          </select>
        </label>
        {profiles && (
          <label style={{ flexDirection: 'row', display: 'flex', gap: 10, alignItems: 'flex-start' }}>
            <input type="checkbox" checked={s.profilesRemote === '1'} onChange={(e) => save({ profilesRemote: e.target.checked ? '1' : '0' })} />
            <span>Also show profiles to devices outside your home network <span className="muted">(off by default: anyone who can reach the server would see everyone's names; passwords are always required)</span></span>
          </label>
        )}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- about

function About() {
  const [sys, setSys] = useState<any>(null)
  useEffect(() => { api.get('/system').then(setSys).catch(() => {}) }, [])
  return (
    <>
      {sys && (
        <div className="panel">
          <h2>About</h2>
          <div className="facts" style={{ marginTop: 0 }}>
            <div className="fact"><div className="k">Version</div><div className="v">{sys.version}</div></div>
            <div className="fact"><div className="k">Platform</div><div className="v">{sys.os}/{sys.arch} · {sys.go}</div></div>
            <div className="fact"><div className="k">Host</div><div className="v">{sys.hostname}</div></div>
            <div className="fact"><div className="k">Data directory</div><div className="v" style={{ wordBreak: 'break-all', fontWeight: 400 }}>{sys.dataDir}</div></div>
            <div className="fact"><div className="k">FFmpeg</div><div className="v" style={{ wordBreak: 'break-all', fontWeight: 400 }}>{sys.ffmpegInfo}<br /><span className="muted">{sys.ffmpeg}</span></div></div>
            <div className="fact"><div className="k">H.264 encoders</div><div className="v">{(sys.encoders || []).join(', ') || 'none'}{(sys.hardware || []).some((h: any) => h.working) && <><br /><span className="muted">Working hardware: {(sys.hardware || []).filter((h: any) => h.working).map((h: any) => h.label).join(', ')}</span></>}</div></div>
            <div className="fact"><div className="k">Open DVD sessions</div><div className="v">{sys.dvdSessions}</div></div>
          </div>
        </div>
      )}
      <div className="panel legal">
        <h2>Licence &amp; legal</h2>
        <p>
          <b>WittyMediaServer</b> is open-source software released under the <b>Apache License, Version 2.0</b>. You may use, modify and
          redistribute it, provided you keep the licence and the accompanying <code>NOTICE</code> attribution (“This product includes
          WittyMediaServer, developed by WittyWilla”). Source: <a href="https://github.com/wittywilla/WittyMediaServer" target="_blank" rel="noreferrer">github.com/wittywilla/WittyMediaServer</a>.
        </p>
        <h3>Your media, your responsibility</h3>
        <p>
          WittyMediaServer is a tool for organising and streaming media <b>you already own and have the right to use</b>. It does not
          supply, index, or search for any content. The operators and users of a server are solely responsible for the files they add to
          it and for complying with the copyright, licensing and other laws that apply where they live and where their viewers are.
          The authors and contributors of WittyMediaServer accept no responsibility or liability for any media stored on, played from,
          or shared through a server, nor for how a server is used.
        </p>
        <h3>No circumvention</h3>
        <p>
          WittyMediaServer does <b>not</b> decrypt or bypass copy protection. CSS-scrambled DVD images are refused. Only unprotected
          content can be played. Where backing up a disc you own requires removing protection, it is up to you to determine whether that
          is lawful in your jurisdiction; WittyMediaServer provides no such capability.
        </p>
        <h3>Third-party components and services</h3>
        <p>
          Transcoding is performed by <b>FFmpeg</b>, run as a separate process; the bundled builds are licensed under the GPL and their
          source is available from the links in <code>THIRD_PARTY_LICENSES.md</code>. Optional metadata and artwork come from
          <b> The Movie Database (TMDB)</b> when you supply your own API key — this product uses the TMDB API but is not endorsed or
          certified by TMDB, and TMDB's terms apply to that data. Without keys, TV information comes from <b>TVmaze</b>
          (CC BY-SA 4.0), movie facts from <b>Wikidata</b> (CC0) and plot summaries from <b>Wikipedia</b> (CC BY-SA 4.0); each title
          links to its source. DVD menus are rendered by WittyMediaServer's own DVD-Video navigator.
        </p>
        <h3>No warranty</h3>
        <p>
          The software is provided “as is”, without warranty of any kind, express or implied, including fitness for a particular purpose.
          Use it at your own risk. WittyMediaServer is not affiliated with any other media software project, DreamWorks, or any studio or
          rights holder; all trademarks belong to their owners.
        </p>
      </div>
    </>
  )
}
