import { useEffect, useState } from 'react'
import { NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { api, type Library } from '../api'
import { VersionTag } from '../components/common'
import { ChooseFolderButton } from '../components/FolderPicker'
import { IconClose, IconEdit, IconFolder, IconInfo, IconLibrary, IconPalette, IconPlus, IconRefresh, IconShield, IconTag, IconTrash, IconTune, IconTv, IconUsers, libIcon } from '../icons'
import { toast, useApp } from '../state'
import AppearanceSettings from './AppearanceSettings'
import LogSettings from './LogSettings'
import Profiles from './Profiles'
import MetadataSettings from './settings/Metadata'
import PlaybackSettings from './settings/Playback'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './settings/ui'
import TranscodeSettings from './TranscodeSettings'
import './settings/settings.css'

// Settings: a menu of pages grouped by what they are about (beside the page
// on wide screens, a row of buttons on phones), each page a column of
// section cards in the middle of the screen.

type Page = { to: string; label: string; icon: React.ReactNode; admin?: boolean; hint: string }
const GROUPS: { title: string; pages: Page[] }[] = [
  { title: 'Server', pages: [
    { to: 'libraries', label: 'Libraries', icon: <IconLibrary />, admin: true, hint: 'Folders of movies, shows, music and books' },
    { to: 'metadata', label: 'Metadata agents', icon: <IconTag />, admin: true, hint: 'Where posters and plots come from' },
    { to: 'playback', label: 'Playback & discs', icon: <IconTv />, admin: true, hint: 'Player, DVDs and Blu-rays' },
    { to: 'transcoding', label: 'Transcoding', icon: <IconTune />, admin: true, hint: 'Quality, hardware and formats' },
  ] },
  { title: 'You', pages: [
    { to: 'users', label: 'Profiles', icon: <IconUsers />, hint: 'People, passwords and limits' },
    { to: 'appearance', label: 'Appearance', icon: <IconPalette />, hint: 'Themes, layout and name' },
  ] },
  { title: 'System', pages: [
    { to: 'logs', label: 'Logs & privacy', icon: <IconShield />, admin: true, hint: 'What is recorded, and for how long' },
    { to: 'about', label: 'About & legal', icon: <IconInfo />, hint: 'Version, modules, licence and credits' },
  ] },
]

export default function SettingsPage() {
  const { user, version } = useApp()
  const loc = useLocation()
  const groups = GROUPS.map((g) => ({ ...g, pages: g.pages.filter((p) => !p.admin || user?.isAdmin) })).filter((g) => g.pages.length)
  useEffect(() => { window.scrollTo({ top: 0 }) }, [loc.pathname])
  return (
    <div className="set-shell">
      <nav className="set-nav" aria-label="Settings">
        <h1 className="set-nav-title">Settings</h1>
        {groups.map((g) => (
          <div key={g.title} className="set-nav-group">
            <div className="set-nav-head">{g.title}</div>
            {g.pages.map((p) => (
              <NavLink key={p.to} to={`/settings/${p.to}`} className="set-nav-link" title={p.hint}>
                <span className="set-nav-icon">{p.icon}</span><span>{p.label}</span>
              </NavLink>
            ))}
          </div>
        ))}
        <NavLink to="/settings/about#modules" className="set-nav-version" title="Version and modules">
          <VersionTag version={version} /><span>Version and modules ›</span>
        </NavLink>
      </nav>
      <main className="set-main">
        <Routes>
          <Route path="libraries" element={<Libraries />} />
          <Route path="metadata" element={<MetadataSettings />} />
          <Route path="playback" element={<PlaybackSettings />} />
          <Route path="server" element={<Navigate to="/settings/metadata" replace />} />
          <Route path="transcoding" element={<><PageHead title="Transcoding" desc="How video is converted for devices that can't play a file as it is: quality, hardware acceleration and formats." /><TranscodeSettings /></>} />
          <Route path="appearance" element={<><PageHead title="Appearance" desc="How this server looks for you: theme, layout and its name." /><AppearanceSettings /></>} />
          <Route path="users" element={<Users />} />
          <Route path="logs" element={<><PageHead title="Logs & privacy" desc="What the server records to help fix problems, and for how long. Nothing is ever sent anywhere." /><LogSettings /></>} />
          <Route path="about" element={<About />} />
          <Route path="*" element={<Navigate to={user?.isAdmin ? '/settings/libraries' : '/settings/users'} replace />} />
        </Routes>
      </main>
    </div>
  )
}

// ---------------------------------------------------------------- libraries

const TYPES: { v: string; label: string; hint: string }[] = [
  { v: 'movies', label: 'Movies', hint: 'Film files; disc images of films can go here too' },
  { v: 'shows', label: 'TV shows', hint: 'Series in season folders; disc images of a series too' },
  { v: 'discs', label: 'DVDs & Blu-rays', hint: 'Disc images of films and series, with their menus' },
  { v: 'anime', label: 'Anime', hint: 'Series with AniList information' },
  { v: 'music', label: 'Music', hint: 'Albums and songs' },
  { v: 'audiobooks', label: 'Audiobooks', hint: 'Books to listen to, by chapter' },
  { v: 'podcasts', label: 'Podcasts', hint: 'RSS feeds you follow, plus episode files; the folder holds downloads' },
  { v: 'books', label: 'Books & comics', hint: 'EPUB, PDF, comics and manga' },
  { v: 'videos', label: 'Home videos', hint: 'Your own videos; nothing is looked up online' },
]
const typeLabel = (t: string) => TYPES.find((x) => x.v === t)?.label || t

function Libraries() {
  const { libraries, refreshLibraries } = useApp()
  const [editing, setEditing] = useState<Partial<Library> | null>(null)
  const remove = async (l: Library) => {
    if (!confirm(`Remove the library “${l.name}”? Your files stay where they are; only the server forgets them.`)) return
    await api.del(`/libraries/${l.id}`)
    await refreshLibraries()
    toast('Library removed')
  }
  const scan = async (l: Library) => { await api.post(`/libraries/${l.id}/scan`); toast(`Scanning ${l.name}`); setTimeout(refreshLibraries, 1000) }
  const add = () => setEditing({ name: '', type: 'movies', paths: [] })
  return (
    <>
      <PageHead title="Libraries" desc="Each library is one or more folders of one kind of media. The server watches them and keeps everything up to date."
        actions={libraries.length > 0 && <button className="btn primary" onClick={add}><IconPlus /> Add library</button>} />
      {libraries.length === 0 ? (
        <div className="set-card set-empty big">
          <IconLibrary size={34} />
          <b>No libraries yet</b>
          <p>Add a folder of movies, shows, music or books to get started.</p>
          <button className="btn primary" onClick={add}><IconPlus /> Add your first library</button>
        </div>
      ) : (
        <div className="set-libs">
          {libraries.map((l) => (
            <article key={l.id} className="set-lib">
              <div className="set-lib-top">
                <span className={'set-lib-icon t-' + l.type}>{libIcon(l.type)}</span>
                <div className="set-lib-title">
                  <b>{l.name}</b>
                  <span>{typeLabel(l.type)} · {l.itemCount} {l.itemCount === 1 ? 'item' : 'items'}</span>
                </div>
                {l.scanning && <span className="set-pill live">Scanning…</span>}
              </div>
              <ul className="set-lib-paths">{l.paths.map((p) => <li key={p} title={p}><IconFolder size={14} /><span>{p}</span></li>)}</ul>
              <div className="set-lib-actions">
                <button className="btn sm" onClick={() => scan(l)} disabled={l.scanning}><IconRefresh /> Scan now</button>
                <button className="btn sm ghost" onClick={() => setEditing(l)}><IconEdit /> Edit</button>
                <span className="grow" />
                <button className="btn sm ghost square danger-hover" onClick={() => remove(l)} aria-label={`Remove ${l.name}`} title="Remove library"><IconTrash /></button>
              </div>
            </article>
          ))}
        </div>
      )}
      {editing && <LibraryEditor lib={editing} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await refreshLibraries(); toast('Library saved; scanning') }} />}
    </>
  )
}

function LibraryEditor({ lib, onClose, onSaved }: { lib: Partial<Library>; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(lib.name || '')
  const [type, setType] = useState(lib.type || 'movies')
  const [paths, setPaths] = useState<string[]>(lib.paths || [])
  const [manual, setManual] = useState('')
  const [err, setErr] = useState('')
  const add = (p: string) => { if (p.trim()) setPaths([...new Set([...paths, p.trim()])]); setManual('') }
  const save = async () => {
    try {
      const body = { name: name.trim(), type, paths }
      if (lib.id) await api.put(`/libraries/${lib.id}`, body); else await api.post('/libraries', body)
      onSaved()
    } catch (e: any) { setErr(e.message) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal set-modal wide" onClick={(e) => e.stopPropagation()} role="dialog" aria-label={lib.id ? 'Edit library' : 'Add library'}>
        <h2>{lib.id ? `Edit ${lib.name}` : 'Add a library'}</h2>
        <div className="set-form">
          <fieldset className="set-types">
            <legend>What's in it?</legend>
            {TYPES.map((t) => (
              <label key={t.v} className={'set-type' + (type === t.v ? ' on' : '')}>
                <input type="radio" name="libtype" checked={type === t.v} onChange={() => { setType(t.v); if (!name || TYPES.some((x) => x.label === name)) setName(t.label) }} />
                <span className="set-type-icon">{libIcon(t.v)}</span>
                <b>{t.label}</b><small>{t.hint}</small>
              </label>
            ))}
          </fieldset>
          <label>Name<input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Movies" /></label>
          <div className="set-field">
            <span className="set-field-label">Folders</span>
            {paths.length > 0 && (
              <ul className="set-paths">
                {paths.map((p) => <li key={p}><IconFolder size={16} /><span title={p}>{p}</span><button className="btn sm ghost square" onClick={() => setPaths(paths.filter((x) => x !== p))} aria-label={`Remove ${p}`}><IconClose /></button></li>)}
              </ul>
            )}
            <div className="set-addpath">
              <ChooseFolderButton className="btn" onPick={add} />
              <input className="input mono" placeholder="…or type a folder path and press Enter" aria-label="Folder path" value={manual} onChange={(e) => setManual(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') add(manual) }} />
              {manual && <button className="btn sm" onClick={() => add(manual)}>Add</button>}
            </div>
            {type === 'podcasts' && <small className="muted">A folder is only needed for episodes you download.</small>}
          </div>
          {err && <div className="error">{err}</div>}
        </div>
        <div className="actions">
          <button className="btn ghost" onClick={onClose}>Cancel</button>
          <button className="btn primary" onClick={save} disabled={!name.trim() || (paths.length === 0 && type !== 'podcasts')}>{lib.id ? 'Save and scan' : 'Add library'}</button>
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- profiles

function Users() {
  const { user } = useApp()
  return (
    <>
      <PageHead title="Profiles" desc="Everyone who uses this server, with their passwords and what they may watch." />
      <Profiles />
      {user?.isAdmin && <SignInScreen />}
    </>
  )
}

// SignInScreen chooses between the "Who's watching?" profile screen and the
// classic name + password form.
function SignInScreen() {
  const st = useServerSettings()
  const { s, set, on } = st
  if (!s) return null
  const profiles = (s.loginStyle || 'profiles') === 'profiles'
  return (
    <Section title="Signing in">
      <Row label="Sign-in screen" desc="Profiles show everyone's picture to pick from, like a streaming service. The classic form asks for a name and password.">
        <div className="seg" role="radiogroup" aria-label="Sign-in screen">
          <button role="radio" aria-checked={profiles} className={profiles ? 'active' : ''} onClick={() => set('loginStyle', 'profiles')}>Profiles</button>
          <button role="radio" aria-checked={!profiles} className={!profiles ? 'active' : ''} onClick={() => set('loginStyle', 'classic')}>Classic</button>
        </div>
      </Row>
      <Row label="Show profiles outside your home" desc="Off by default: anyone who can reach the server would see everyone's names. Passwords are always required." dim={!profiles}>
        <Switch label="Show profiles outside your home" disabled={!profiles} checked={on('profilesRemote', '0')} onChange={(v) => set('profilesRemote', v ? '1' : '0')} />
      </Row>
      <Row label="Who may manage Kids profiles">
        <select className="select" value={s.kidsManagedBy || 'adults'} onChange={(e) => set('kidsManagedBy', e.target.value)}>
          <option value="adults">Every adult</option>
          <option value="admin">Only the administrator</option>
        </select>
      </Row>
      <SaveBar {...st} />
    </Section>
  )
}

// ---------------------------------------------------------------- about

// The readers built into the web app (their versions are set at build time).
const READERS: Record<string, [string, string]> = {
  wittyebook: ['WittyEbook', 'EPUB reader'],
  wittypdf: ['WittyPDF', 'PDF reader'],
  wittycomics: ['WittyComics', 'Comic and manga reader'],
  wittyink: ['WittyInk', 'Highlights, notes and pen, shared by the readers'],
}
type Engine = { name: string; module: string; version: string; local?: boolean; role: string }

function About() {
  const [sys, setSys] = useState<any>(null)
  const [about, setAbout] = useState<{ version: string; modules: Engine[] } | null>(null)
  const { user, version } = useApp()
  const loc = useLocation()
  useEffect(() => { if (user?.isAdmin) api.get('/system').then(setSys).catch(() => {}) }, [user])
  useEffect(() => { api.get<{ version: string; modules: Engine[] }>('/about').then(setAbout).catch(() => {}) }, [])
  useEffect(() => { if (loc.hash === '#modules' && about) document.getElementById('modules')?.scrollIntoView({ block: 'start' }) }, [loc.hash, about])
  const readers = __WEB_MODULES__.filter((m) => READERS[m.id]).sort((a, b) => Object.keys(READERS).indexOf(a.id) - Object.keys(READERS).indexOf(b.id))
  const hw = (sys?.hardware || []).filter((h: any) => h.working).map((h: any) => h.label)
  return (
    <>
      <PageHead title="About & legal" desc="This server's version, the modules inside it, its system, the licence, and the services it relies on." />
      <Section title="Version and modules" desc="WittyMediaServer is built from separate modules, each also published on its own. These are the versions in this server.">
        <div id="modules" className="mod-list">
          <div className="mod-row mod-self">
            <span className="mod-name"><b>WittyMediaServer</b><span className="muted">The server and this app</span></span>
            <span className="mod-ver">{about?.version || version || 'dev'}</span>
          </div>
          <div className="mod-group">Disc engines</div>
          {(about?.modules || []).map((m) => (
            <a key={m.name} className="mod-row" href={`https://github.com/wittywilla/${m.name}`} target="_blank" rel="noreferrer">
              <span className="mod-name"><b>{m.name}</b><span className="muted">{m.role}</span></span>
              <span className="mod-ver">{m.version}{m.local && <span className="mod-local" title="Built from this server's own copy of the module">built in</span>}</span>
            </a>
          ))}
          <div className="mod-group">Readers</div>
          {readers.map((m) => (
            <a key={m.id} className="mod-row" href={`https://github.com/wittywilla/${READERS[m.id][0]}`} target="_blank" rel="noreferrer">
              <span className="mod-name"><b>{READERS[m.id][0]}</b><span className="muted">{READERS[m.id][1]}</span></span>
              <span className="mod-ver">{m.version}</span>
            </a>
          ))}
        </div>
      </Section>
      {sys && (
        <Section title="This server">
          <dl className="set-facts">
            <div className="wide"><dt>Open it on other devices at home</dt><dd className="mono">{(sys.lanUrls || []).length ? sys.lanUrls.join('  ·  ') : 'This server only accepts connections from this computer'}</dd></div>
            <div><dt>System</dt><dd>{sys.os} / {sys.arch}</dd></div>
            <div><dt>Computer</dt><dd>{sys.hostname}</dd></div>
            <div><dt>Hardware video</dt><dd>{hw.length ? hw.join(', ') : 'None found; software encoding'}</dd></div>
            <div><dt>Open discs</dt><dd>{sys.dvdSessions} DVD · {sys.blurays ?? 0} Blu-ray</dd></div>
            <div><dt>Built with</dt><dd>{sys.go}</dd></div>
            <div className="wide"><dt>Data folder</dt><dd className="mono">{sys.dataDir}</dd></div>
            <div className="wide"><dt>FFmpeg</dt><dd className="mono">{sys.ffmpegInfo}</dd></div>
          </dl>
        </Section>
      )}
      <Section title="Licence">
        <div className="set-prose">
          <p><b>WittyMediaServer</b> is open-source software under the <b>Apache License 2.0</b>. You may use, change and share it, keeping the licence and the <code>NOTICE</code> attribution (“This product includes WittyMediaServer, developed by WittyWilla”). Source: <a href="https://github.com/wittywilla/WittyMediaServer" target="_blank" rel="noreferrer">github.com/wittywilla/WittyMediaServer</a>.</p>
        </div>
      </Section>
      <Section title="Your media, your responsibility">
        <div className="set-prose">
          <p>WittyMediaServer organises and streams media <b>you already own and have the right to use</b>. It does not supply, index or search for any content. Server operators and users are responsible for the files they add and for following the laws that apply where they and their viewers live. The authors accept no responsibility for media stored on, played from or shared through a server.</p>
          <p><b>No circumvention.</b> Copy protection is never decrypted or bypassed: CSS-scrambled DVDs and AACS-encrypted Blu-rays are refused, and only unprotected content plays.</p>
        </div>
      </Section>
      <Section title="Third-party components and services">
        <div className="set-prose">
          <p>Video is converted by <b>FFmpeg</b>, run as a separate program; the bundled builds are GPL-licensed, with sources linked in <code>THIRD_PARTY_LICENSES.md</code>. With your own key, information comes from <b>The Movie Database (TMDB)</b>: this product uses the TMDB API but is not endorsed or certified by TMDB. Without keys, TV information comes from <b>TVmaze</b> (CC BY-SA 4.0), movie facts from <b>Wikidata</b> (CC0) and plot summaries from <b>Wikipedia</b> (CC BY-SA 4.0). Disc menus are run by WittyMediaServer's own WittyDVD and WittyBluray engines.</p>
          <p><b>No warranty.</b> The software is provided “as is”, without warranty of any kind. WittyMediaServer is not affiliated with any other media software project, studio or rights holder; all trademarks belong to their owners.</p>
        </div>
      </Section>
    </>
  )
}
