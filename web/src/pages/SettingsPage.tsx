import { useEffect, useRef, useState } from 'react'
import { NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { api, type Library } from '../api'
import AddLibrary, { GroupingChoice, type Grouping } from '../components/AddLibrary'
import { UpdateDot, VersionTag } from '../components/common'
import { ChooseFolderButton } from '../components/FolderPicker'
import { IconClose, IconEdit, IconFolder, IconImage, IconInfo, IconLibrary, IconPalette, IconPlus, IconRefresh, IconShield, IconTag, IconTrash, IconTune, IconTv, IconUsers, libIcon } from '../icons'
import { toast, useApp } from '../state'
import AppearanceSettings from './AppearanceSettings'
import LogSettings from './LogSettings'
import Profiles from './Profiles'
import MetadataSettings from './settings/Metadata'
import PlaybackSettings from './settings/Playback'
import SharingSettings from './settings/Sharing'
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
    { to: 'sharing', label: 'Sharing & photos', icon: <IconImage />, hint: 'Uploads, links, the photo map' },
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
                <span className="set-nav-icon">{p.icon}</span><span>{p.label}</span>{p.to === 'about' && <UpdateDot />}
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
          <Route path="sharing" element={<SharingSettings />} />
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
  { v: 'photos', label: 'Photos', hint: 'A timeline, albums and a map of your pictures' },
  { v: 'shorts', label: 'Vertical videos', hint: 'A feed of short videos with a For You page' },
  { v: 'channels', label: 'Creator videos', hint: 'An archive of creators’ videos, by channel' },
  { v: 'musicvideos', label: 'Music videos & concerts', hint: 'Music videos and live recordings by artist' },
  { v: 'radio', label: 'Internet radio', hint: 'Stations by stream address; no folder needed' },
]
const typeLabel = (t: string) => TYPES.find((x) => x.v === t)?.label || t

function Libraries() {
  const { libraries, refreshLibraries } = useApp()
  const [editing, setEditing] = useState<Partial<Library> | null>(null)
  const [adding, setAdding] = useState(false)
  const [grouping, setGrouping] = useState<Grouping>('combine')
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setGrouping(s.libraryGrouping === 'separate' ? 'separate' : 'combine')).catch(() => {}) }, [])
  const changeGrouping = async (g: Grouping) => {
    setGrouping(g)
    try { await api.put('/settings', { libraryGrouping: g }); toast(g === 'combine' ? 'Folders of one kind now join one library' : 'New folders now become libraries of their own') } catch (e: any) { toast(e.message) }
  }
  const kinds = [...new Set(libraries.map((l) => l.type))].map((k) => `${libraries.filter((l) => l.type === k).length} ${typeLabel(k)}`)
  const remove = async (l: Library) => {
    if (!confirm(`Remove the library “${l.name}”? Your files stay where they are; only the server forgets them.`)) return
    await api.del(`/libraries/${l.id}`)
    await refreshLibraries()
    toast('Library removed')
  }
  const scan = async (l: Library) => { await api.post(`/libraries/${l.id}/scan`); toast(`Scanning ${l.name}`); setTimeout(refreshLibraries, 1000) }
  const add = () => setAdding(true)
  return (
    <>
      <PageHead title="Libraries" desc="Each library is one or more folders of one kind of media. The server watches them and keeps everything up to date."
        actions={libraries.length > 0 && <button className="btn primary" onClick={add}><IconPlus /> Add library</button>} />
      {libraries.length > 0 && (
        <div className="set-lib-summary">
          <b>{libraries.length} {libraries.length === 1 ? 'library' : 'libraries'}</b>
          <span className="muted">{kinds.join(' · ')}</span>
        </div>
      )}
      <Section title="Folders of the same kind" desc="When you add a folder of a kind you already have a library for, it can join that library, or become a new library with its own name (for example “Kids’ movies” beside “Movies”). You can always choose differently when adding.">
        <GroupingChoice value={grouping} onChange={changeGrouping} />
      </Section>
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
      {adding && (
        <div className="modal-backdrop" onClick={() => setAdding(false)}>
          <div className="modal wide add-lib-modal" role="dialog" aria-label="Add a library" onClick={(e) => e.stopPropagation()}>
            <div className="add-lib-modal-head"><h2>Add a library</h2><button className="btn sm ghost square" onClick={() => setAdding(false)} aria-label="Close"><IconClose /></button></div>
            <AddLibrary existing={libraries} grouping={grouping} onAdded={async (l) => { setAdding(false); await refreshLibraries(); toast(`${l.name} saved; scanning`) }} />
          </div>
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
      {user?.isAdmin && <Updates />}
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

// ------------------------------------------------------------------ updates

type UpdateInfo = { version: string; title: string; notes: string; url: string; published?: number; prerelease: boolean; signed: boolean; size: number }
type UpdateStatus = {
  current: string; latest?: UpdateInfo; checkedAt?: number; checkError?: string
  phase: 'idle' | 'downloading' | 'verifying' | 'installing' | 'restarting' | 'failed'
  done?: number; total?: number; error?: string; canApply: boolean; reason?: string; page: string
}
const ACTIVE = ['downloading', 'verifying', 'installing', 'restarting']
const mb = (n = 0) => (n / 1048576).toFixed(n < 10485760 ? 1 : 0)

// Updates: what is installed, what GitHub has, and installing it with one
// click. The server verifies the release's signature and checksum, swaps
// the program's files (never the data) and restarts; this card follows
// along and reloads the page once the new version answers.
function Updates() {
  const { setUpdate } = useApp()
  const [st, setSt] = useState<UpdateStatus | null>(null)
  const [auto, setAuto] = useState(true)
  const [checking, setChecking] = useState(false)
  const [slow, setSlow] = useState(false) // the server has not come back yet
  const from = useRef('')
  const show = (n: UpdateStatus) => { setSt(n); setUpdate(n.latest?.version || '') }
  useEffect(() => {
    api.get<UpdateStatus>('/update').then(show).catch((e) => toast(e.message))
    api.get<Record<string, string>>('/settings').then((s) => setAuto(s.updateCheck !== '0')).catch(() => {})
  }, [])

  // While installing: follow the progress, then wait for the new version.
  const active = !!st && ACTIVE.includes(st.phase)
  useEffect(() => {
    if (!active || !st) return
    if (!from.current) from.current = st.current
    const started = Date.now()
    const t = setInterval(async () => {
      try {
        const n = await api.get<UpdateStatus>('/update')
        if (n.current !== from.current) { window.location.reload(); return } // the new version is running
        setSt(n)
      } catch {
        setSt((s) => s && { ...s, phase: 'restarting' }) // unreachable while it restarts
      }
      setSlow(Date.now() - started > 3 * 60_000)
    }, 1000)
    return () => clearInterval(t)
  }, [active])

  const check = async () => {
    setChecking(true)
    try {
      const n = await api.post<UpdateStatus>('/update/check')
      show(n)
      if (n.checkError) toast(n.checkError)
      else if (!n.latest) toast('You have the newest version')
    } catch (e: any) { toast(e.message) } finally { setChecking(false) }
  }
  const apply = async () => {
    if (!st?.latest || !confirm(`Install WittyMediaServer ${st.latest.version}? The server restarts, which pauses anything playing for a moment. Your library and settings are kept.`)) return
    try { from.current = st.current; setSt(await api.post<UpdateStatus>('/update/apply')) } catch (e: any) { toast(e.message) }
  }
  const setAutoCheck = async (v: boolean) => {
    setAuto(v)
    try { await api.put('/settings', { updateCheck: v ? '1' : '0' }) } catch (e: any) { setAuto(!v); toast(e.message) }
  }

  const l = st?.latest
  return (
    <Section title="Updates" desc="New versions of WittyMediaServer are published on GitHub. Each one is signed, and this server installs only releases whose signature and checksum it can verify."
      actions={<button className="btn ghost sm" onClick={check} disabled={checking || active}><IconRefresh /> {checking ? 'Checking…' : 'Check now'}</button>}>
      <Row label="Installed version" desc={st?.checkedAt ? `Last checked ${new Date(st.checkedAt).toLocaleString()}` : 'Not checked yet'}>
        <span className="mod-ver">{st?.current || '…'}</span>
      </Row>
      {st?.checkError && <p className="upd-msg warn">The last check didn’t work: {st.checkError}</p>}
      {st && !l && !st.checkError && !!st.checkedAt && <p className="upd-msg">You have the newest version.</p>}
      {st && l && (
        <div className="upd-new">
          <div className="upd-head">
            <div>
              <b>{l.title || `Version ${l.version}`}</b>
              <span className="muted">Version {l.version}{l.prerelease && ' · pre-release'}{l.published ? ` · ${new Date(l.published).toLocaleDateString()}` : ''}{l.size ? ` · ${mb(l.size)} MB` : ''}</span>
            </div>
            {st.canApply && !active && <button className="btn primary sm" onClick={apply}>Update now</button>}
          </div>
          {l.notes && <ReleaseNotes text={l.notes} />}
          <a href={l.url || st.page} target="_blank" rel="noreferrer">Release page on GitHub ›</a>
          {!st.canApply && st.reason && <p className="upd-msg">{st.reason}</p>}
        </div>
      )}
      {st && st.phase !== 'idle' && (
        <div className={'upd-progress' + (st.phase === 'failed' ? ' failed' : '')} role="status" aria-live="polite">
          {st.phase === 'downloading' && <>
            <span>Downloading… {mb(st.done)}{st.total ? ` of ${mb(st.total)}` : ''} MB</span>
            <progress max={st.total || undefined} value={st.total ? st.done || 0 : undefined} />
          </>}
          {st.phase === 'verifying' && <span>Checking the signature and checksum…</span>}
          {st.phase === 'installing' && <span>Installing the new files…</span>}
          {st.phase === 'restarting' && <span>{slow
            ? 'The server hasn’t come back yet. If it doesn’t, start WittyMediaServer again (tray icon or Start menu); the new version is installed.'
            : 'Restarting… this page reloads by itself when the new version is running.'}</span>}
          {st.phase === 'failed' && <span>The update didn’t install, and the server keeps running the version it has: {st.error}</span>}
        </div>
      )}
      <Row label="Check for updates automatically" desc="Twice a day the server asks GitHub for its list of releases and shows a dot on Settings when there is a newer one. Nothing about you or your library is sent, and nothing installs without you.">
        <Switch checked={auto} onChange={setAutoCheck} label="Check for updates automatically" />
      </Row>
    </Section>
  )
}

// ReleaseNotes shows the release's Markdown as text with headings and
// bullet points; no HTML from the release is ever rendered.
function ReleaseNotes({ text }: { text: string }) {
  const blocks: React.ReactNode[] = []
  let items: string[] = []
  const flush = () => { if (items.length) { blocks.push(<ul key={blocks.length}>{items.map((t, i) => <li key={i}>{t}</li>)}</ul>); items = [] } }
  for (const raw of text.replace(/\r/g, '').split('\n')) {
    const line = raw.trim().replace(/\*\*|__|`/g, '')
    if (/^[-*+] /.test(line)) { items.push(line.slice(2)); continue }
    flush()
    if (!line) continue
    if (line.startsWith('#')) blocks.push(<b key={blocks.length} className="upd-h">{line.replace(/^#+\s*/, '')}</b>)
    else blocks.push(<p key={blocks.length}>{line}</p>)
  }
  flush()
  return <div className="upd-notes">{blocks}</div>
}
