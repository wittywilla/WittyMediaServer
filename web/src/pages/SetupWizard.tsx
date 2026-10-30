import { useEffect, useState } from 'react'
import { api, setToken, type Library, type User } from '../api'
import { Brand, VersionTag } from '../components/common'
import AddLibrary, { GroupingChoice, LibrarySummary, type Grouping } from '../components/AddLibrary'
import BrandDialog, { uploadLogo } from '../components/BrandDialog'
import { IconAudio, IconBook, IconCheck, IconDisc, IconPalette, IconPlug, IconTv, IconUsers } from '../icons'
import { setNavStyle, useNavStyle, type NavStyle } from '../navPrefs'
import { useApp } from '../state'
import { applyTheme, type ThemeMeta } from '../theme'
import { AddProfile } from './Profiles'

// The first-run setup: welcome → account → your media → metadata agents →
// discs → look → ready. Every step after the account can be skipped and
// changed later in Settings. It resumes where it left off after a reload (the
// account step is then already done).

type Step = 'welcome' | 'account' | 'libraries' | 'metadata' | 'playback' | 'look' | 'done'
const STEPS: { id: Step; label: string }[] = [
  { id: 'welcome', label: 'Welcome' }, { id: 'account', label: 'Account' }, { id: 'libraries', label: 'Your media' },
  { id: 'metadata', label: 'Information' }, { id: 'playback', label: 'Discs' }, { id: 'look', label: 'Look' }, { id: 'done', label: 'Ready' },
]

const FEATURES = [
  { icon: <IconTv />, title: 'Movies & TV', text: 'Posters, plots and “continue watching” on every screen.' },
  { icon: <IconDisc />, title: 'DVDs & Blu-rays', text: 'Disc images play with their real menus, like a disc player.' },
  { icon: <IconAudio />, title: 'Music & podcasts', text: 'Albums, audiobooks that remember your place, and shows you follow.' },
  { icon: <IconBook />, title: 'Books & comics', text: 'Read EPUB, PDF and manga with highlights, notes and a pen.' },
  { icon: <IconUsers />, title: 'Everyone at home', text: 'Profiles with their own progress, and Kids profiles by age.' },
  { icon: <IconPalette />, title: 'Yours', text: 'Themes, your server’s name, and nothing leaves your home.' },
]

export default function SetupWizard({ resume }: { resume: boolean }) {
  const { setUser, setOnboarding, brandName, setBrandName, setBrandLogo, version, refreshLibraries } = useApp()
  const [step, setStep] = useState<Step>(resume ? 'libraries' : 'welcome')
  const idx = STEPS.findIndex((s) => s.id === step)
  const [serverName, setServerName] = useState(brandName)
  // A logo chosen on the welcome page waits until the account exists (only
  // an administrator may store one), then is uploaded.
  const [pendingLogo, setPendingLogo] = useState<File | null>(null)
  const [logoPreview, setLogoPreview] = useState('')
  const [naming, setNaming] = useState(false)
  useEffect(() => {
    if (!pendingLogo) { setLogoPreview(''); return }
    const u = URL.createObjectURL(pendingLogo)
    setLogoPreview(u)
    return () => URL.revokeObjectURL(u)
  }, [pendingLogo])
  const [added, setAdded] = useState<Library[]>([])
  const [addKids, setAddKids] = useState(false)
  const [kidsAdded, setKidsAdded] = useState(false)
  const [keys, setKeys] = useState(0)
  // Addresses other devices at home can open (this one may be "localhost").
  const [lan, setLan] = useState<string[] | null>(null)
  useEffect(() => { if (step === 'done') api.get<{ lanUrls?: string[] }>('/system').then((s) => setLan(s.lanUrls || [])).catch(() => setLan([])) }, [step])
  const here = /^(localhost|127\.|\[::1\])/.test(location.hostname) ? '' : location.origin
  const addrs = here ? [here] : lan || []

  const finish = async () => {
    try { await api.put('/settings', { setupDone: '1', ...(serverName.trim() ? { brandName: serverName.trim() } : {}) }) } catch { /* ignore */ }
    if (serverName.trim()) setBrandName(serverName.trim())
    await refreshLibraries()
    setOnboarding(false)
  }
  const go = (s: Step) => { setStep(s); window.scrollTo({ top: 0 }) }

  return (
    <div className="oobe">
      <div className="oobe-glow" aria-hidden="true" />
      <div className="oobe-shell">
        <header className="oobe-head">
          <div className="brand"><img className="brand-mark" src={logoPreview || '/icon.svg'} alt="" /><Brand name={serverName || brandName} /></div>
          <ol className="oobe-steps" aria-label="Setup progress">
            {STEPS.map((s, i) => (
              <li key={s.id} className={i < idx ? 'done' : i === idx ? 'current' : ''} aria-current={i === idx ? 'step' : undefined}>
                <span className="dot">{i < idx ? <IconCheck size={14} /> : i + 1}</span><span className="lbl">{s.label}</span>
              </li>
            ))}
          </ol>
          <div className="oobe-step-count" aria-hidden="true">Step {idx + 1} of {STEPS.length} · {STEPS[idx].label}</div>
        </header>

        <main className="oobe-card" key={step}>
          {step === 'welcome' && (
            <section className="oobe-welcome">
              <img className="oobe-welcome-logo" src={logoPreview || '/icon.svg'} alt="" />
              <h1>Welcome to your media server</h1>
              <p className="lead">Everything you own — movies, shows, discs, music, books, games — in one place, on every screen in your home. Setup takes about three minutes, and every step can be changed later.</p>
              <ul className="oobe-perks" aria-label="What your server does">
                {FEATURES.map((f) => <li key={f.title}><span className="ic">{f.icon}</span><span><b>{f.title}</b> <span className="muted">{f.text}</span></span></li>)}
              </ul>
              <div className="oobe-name">
                <div className="oobe-name-now">
                  <span className="muted">Your server is called</span>
                  <b>{serverName.trim() || 'WittyMediaServer'}</b>
                </div>
                <button type="button" className="btn" onClick={() => setNaming(true)}>Rename…</button>
              </div>
              {naming && <BrandDialog name={serverName} logo={logoPreview} onClose={() => setNaming(false)}
                onSave={(n, logo) => { setServerName(n); if (logo === 'remove') setPendingLogo(null); else if (logo) setPendingLogo(logo) }} />}
              <button className="btn primary big oobe-start" autoFocus onClick={() => go('account')}>Get started</button>
            </section>
          )}

          {step === 'account' && <AccountStep onBack={() => go('welcome')} onDone={(u, tok) => {
            setToken(tok)
            setOnboarding(true) // keep the wizard open after signing in
            setUser(u)
            if (pendingLogo) uploadLogo(pendingLogo).then(setBrandLogo).catch(() => {})
            go('libraries')
          }} />}

          {step === 'libraries' && <LibrariesStep added={added} onAdded={(l) => setAdded((a) => a.some((x) => x.id === l.id) ? a.map((x) => (x.id === l.id ? l : x)) : [...a, l])} onNext={() => go('metadata')} />}

          {step === 'metadata' && <MetadataStep onBack={() => go('libraries')} onNext={(n) => { setKeys(n); go('playback') }} />}

          {step === 'playback' && <PlaybackStep onBack={() => go('metadata')} onNext={() => go('look')} />}

          {step === 'look' && <LookStep onBack={() => go('playback')} onNext={() => go('done')} />}

          {step === 'done' && (
            <section className="oobe-done">
              <div className="oobe-check" aria-hidden="true"><IconCheck size={44} /></div>
              <h1>You're all set</h1>
              <p className="lead">
                {added.length
                  ? `Your ${added.length === 1 ? 'library is' : `${added.length} libraries are`} being scanned now. Titles and posters appear as each one is found.`
                  : 'Add your first library any time from Settings → Libraries.'}
              </p>
              <p className="muted">When your libraries have been scanned, you can make always-on TV channels out of them with one click, and add tuners or IPTV, under Settings → Channels &amp; tuners. There is nothing to set up now.</p>
              <ul className="oobe-summary">
                <li className={added.length ? 'ok' : ''}><IconCheck size={16} /> {added.length ? `${added.length} ${added.length === 1 ? 'library' : 'libraries'} added` : 'No libraries yet'}</li>
                <li className="ok"><IconCheck size={16} /> TV and movie information with no API key needed{keys ? `, plus ${keys} key${keys === 1 ? '' : 's'}` : ''}</li>
                <li className="ok"><IconCheck size={16} /> Discs, playback and look chosen</li>
              </ul>
              <div className="oobe-next">
                <div className="oobe-next-card">
                  <IconTv size={22} />
                  <b>Watch on every screen at home</b>
                  {addrs.length > 0
                    ? <><span>On the same home network (Wi-Fi or cable), open this address on your TV, phone, tablet or another computer:</span>{addrs.slice(0, 2).map((a) => <code key={a}>{a}</code>)}</>
                    : <span>Right now this server only accepts connections from this computer. To reach it from your TV or phone, start it with a network address, such as <code>-listen :9797</code>.</span>}
                  <small className="oobe-disclaimer">Your server runs in your home, so for now it works only on your home network. To watch away from home, follow the steps in Settings → Remote Access when you’re ready.</small>
                </div>
                <div className="oobe-next-card">
                  <IconUsers size={22} />
                  <b>Add the family</b>
                  <span>{kidsAdded ? 'Kids profile added. Add more in Settings → Profiles.' : 'A Kids profile only shows titles for their age.'}</span>
                  {!kidsAdded && <button type="button" className="btn sm ghost" onClick={() => setAddKids(true)}>Add a Kids profile</button>}
                </div>
                <div className="oobe-next-card">
                  <IconPlug size={22} />
                  <b>Make it smarter</b>
                  <span>Add custom metadata agents, transcoding and more in Settings whenever you like.</span>
                </div>
              </div>
              {addKids && <AddProfile admin initialKind="kids" onClose={() => setAddKids(false)} onDone={() => { setAddKids(false); setKidsAdded(true) }} />}
              <div className="oobe-actions oobe-finish"><button className="btn primary big" autoFocus onClick={finish}>Start watching</button></div>
            </section>
          )}
        </main>
        <footer className="oobe-foot"><VersionTag version={version} /></footer>
      </div>
    </div>
  )
}

function AccountStep({ onBack, onDone }: { onBack: () => void; onDone: (u: User, token: string) => void }) {
  const [name, setName] = useState('')
  const [pw, setPw] = useState('')
  const [pw2, setPw2] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [code, setCode] = useState('')
  const [needCode, setNeedCode] = useState(false)
  useEffect(() => { api.get<{ setupCodeRequired?: boolean }>('/status').then((s) => setNeedCode(!!s.setupCodeRequired)).catch(() => {}) }, [])
  const level = pw.length >= 12 ? 3 : pw.length >= 8 ? 2 : 0
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    if (pw !== pw2) { setErr('The two passwords are different.'); return }
    setBusy(true)
    try {
      const r = await api.post<{ token: string; user: User }>('/setup', { name, password: pw, setupCode: code, device: navigator.userAgent.slice(0, 80) })
      onDone(r.user, r.token)
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <form onSubmit={submit}>
      <h1>Create your account</h1>
      <p className="lead">This is the administrator: the account that manages libraries, people and settings. You'll add family profiles later.</p>
      <div className="oobe-form">
        <label className="oobe-field">Your name<input className="input" autoFocus required value={name} onChange={(e) => setName(e.target.value)} autoComplete="username" /></label>
        <label className="oobe-field">Password
          <input className="input" type="password" required minLength={8} value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" aria-describedby="pw-strength" />
          <span className={'oobe-strength l' + level} id="pw-strength"><i /><i /><i /><span>{['At least 8 characters', '', 'Good', 'Strong'][level]}</span></span>
        </label>
        <label className="oobe-field">Type it again<input className="input" type="password" required value={pw2} onChange={(e) => setPw2(e.target.value)} autoComplete="new-password" /></label>
        {needCode && <label className="oobe-field">Setup code<input className="input mono" required value={code} onChange={(e) => setCode(e.target.value)} autoComplete="off" /><span className="muted">You're setting this server up from another network, so it asks for the code it wrote to its log when it started.</span></label>}
      </div>
      {err && <div className="error" role="alert">{err}</div>}
      <div className="oobe-actions">
        <button type="button" className="btn ghost" onClick={onBack}>Back</button>
        <button className="btn primary big" disabled={busy || !name || pw.length < 8}>{busy ? 'Creating…' : 'Create account'}</button>
      </div>
    </form>
  )
}

function LibrariesStep({ added, onAdded, onNext }: { added: Library[]; onAdded: (l: Library) => void; onNext: () => void }) {
  // While a kind is chosen (folder, then following podcasts) the panel has its
  // own Back and Add buttons; the wizard's Continue would be easy to hit by
  // mistake, so it waits until the panel is closed.
  const [inPanel, setInPanel] = useState(false)
  const [grouping, setGrouping] = useState<Grouping>('combine')
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setGrouping(s.libraryGrouping === 'separate' ? 'separate' : 'combine')).catch(() => {}) }, [])
  const changeGrouping = (g: Grouping) => { setGrouping(g); api.put('/settings', { libraryGrouping: g }).catch(() => {}) }
  return (
    <section>
      <h1>Where is your media?</h1>
      <p className="lead">Pick a kind of media, then the folder it's in. Add as many as you like; each becomes a library you can browse.</p>
      <LibrarySummary libs={added} title="Libraries you've made" />
      {!inPanel && <GroupingChoice value={grouping} onChange={changeGrouping} />}
      <AddLibrary onAdded={onAdded} onPanel={setInPanel} existing={added} grouping={grouping} />
      {!inPanel && (
        <div className="oobe-actions">
          <span className="muted">{added.length ? 'Scanning starts right away in the background. Add another, or continue.' : 'You can also do this later in Settings.'}</span>
          <button type="button" className={'btn big ' + (added.length ? 'primary' : 'ghost')} onClick={onNext}>{added.length ? 'Continue' : 'Skip for now'}</button>
        </div>
      )}
    </section>
  )
}

const PROVIDERS = [
  {
    id: 'tmdb', name: 'The Movie Database (TMDB)', for: 'Movies, discs and TV', key: 'tmdbKey',
    steps: [
      <>Create a free account at <a href="https://www.themoviedb.org/signup" target="_blank" rel="noreferrer">themoviedb.org</a>.</>,
      <>Open <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">Settings → API</a> and ask for a key (choose “Personal use”).</>,
      <>Copy the <b>API Key</b> and paste it below.</>,
    ],
    note: 'This product uses the TMDB API but is not endorsed or certified by TMDB.',
  },
  {
    id: 'tvdb', name: 'TheTVDB', for: 'TV shows', key: 'tvdbKey', pinKey: 'tvdbPin',
    steps: [
      <>Create an account at <a href="https://thetvdb.com/auth/register" target="_blank" rel="noreferrer">thetvdb.com</a>.</>,
      <>Open the <a href="https://thetvdb.com/api-information" target="_blank" rel="noreferrer">API information</a> page and create a key.</>,
      <>Paste the key below. If TheTVDB gave you a <b>subscriber PIN</b>, add it too.</>,
    ],
    note: 'TV information and images are provided by TheTVDB.',
  },
] as const

function MetadataStep({ onBack, onNext }: { onBack: () => void; onNext: (keys: number) => void }) {
  const [keys, setKeys] = useState<Record<string, string>>({})
  const [state, setState] = useState<Record<string, { ok?: boolean; msg?: string; busy?: boolean }>>({})
  const [open, setOpen] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const test = async (p: (typeof PROVIDERS)[number]) => {
    setState((s) => ({ ...s, [p.id]: { busy: true } }))
    try {
      const r = await api.post<{ ok: boolean; error?: string }>('/metadata/test', { provider: p.id, key: keys[p.key] || '', pin: 'pinKey' in p ? keys[p.pinKey] || '' : '' })
      setState((s) => ({ ...s, [p.id]: { ok: r.ok, msg: r.ok ? 'Connected' : r.error } }))
    } catch (e: any) { setState((s) => ({ ...s, [p.id]: { ok: false, msg: e.message } })) }
  }
  const save = async () => {
    setSaving(true)
    const body: Record<string, string> = {}
    for (const [k, v] of Object.entries(keys)) if (v.trim()) body[k] = v.trim()
    try { if (Object.keys(body).length) await api.put('/settings', body) } catch { /* keys can be added later */ }
    setSaving(false)
    onNext(PROVIDERS.filter((p) => keys[p.key]?.trim()).length)
  }
  const any = Object.values(keys).some((v) => v.trim())
  return (
    <section>
      <h1>Media Indexers and Metadata Agents</h1>
      <p className="lead">Agents find titles, posters, plots and ratings for your media. These work already, with no API key needed. Your files never leave this computer: only their names are looked up.</p>
      <div className="oobe-agents">
        <div className="oobe-agent ok"><IconCheck size={16} /><b>TVmaze</b><span>TV shows</span><em>No API key needed</em></div>
        <div className="oobe-agent ok"><IconCheck size={16} /><b>Wikidata &amp; Wikipedia</b><span>Movies</span><em>No API key needed</em></div>
        <div className="oobe-agent ok"><IconCheck size={16} /><b>AniList</b><span>Anime</span><em>No API key needed</em></div>
      </div>
      <h2 className="oobe-sub">Optional: posters and the best matches</h2>
      <p className="muted oobe-hint">Add a free API key from these services. Each takes about two minutes, and you can do it later in Settings.</p>
      <div className="oobe-providers">
        {PROVIDERS.map((p) => {
          const st = state[p.id] || {}
          const shown = open === p.id || !!keys[p.key]
          return (
            <div key={p.id} className={'oobe-provider' + (st.ok ? ' ok' : '')}>
              <button type="button" className="op-head" onClick={() => setOpen(open === p.id ? null : p.id)} aria-expanded={shown}>
                <b>{p.name}</b><span className="pill">{p.for}</span>
                {st.ok ? <span className="op-ok"><IconCheck size={14} /> Connected</span> : <span className="op-add">{shown ? 'Hide' : 'Add a key'}</span>}
              </button>
              {shown && (
                <>
                  <ol className="op-steps">{p.steps.map((s, i) => <li key={i}>{s}</li>)}</ol>
                  <div className="field-row">
                    <input className="input" aria-label={`${p.name} key`} placeholder="Paste your key" value={keys[p.key] || ''} spellCheck={false} autoComplete="off"
                      onChange={(e) => { setKeys((k) => ({ ...k, [p.key]: e.target.value })); setState((s) => ({ ...s, [p.id]: {} })) }} />
                    {'pinKey' in p && <input className="input pin" aria-label="Subscriber PIN" placeholder="PIN (optional)" value={keys[p.pinKey] || ''} onChange={(e) => setKeys((k) => ({ ...k, [p.pinKey]: e.target.value }))} />}
                    <button type="button" className="btn" disabled={!keys[p.key]?.trim() || st.busy} onClick={() => test(p)}>{st.busy ? 'Checking…' : 'Test'}</button>
                  </div>
                  {st.msg && !st.ok && <div className="error" role="alert">{st.msg}</div>}
                  <div className="muted op-note">{p.note}</div>
                </>
              )}
            </div>
          )
        })}
      </div>
      <p className="muted oobe-hint">Have your own catalogue? Add <b>custom agents</b> later in Settings → Metadata agents.</p>
      <div className="oobe-actions">
        <button type="button" className="btn ghost" onClick={onBack}>Back</button>
        <button type="button" className="btn primary big" disabled={saving} onClick={save}>{any ? 'Save and continue' : 'Continue'}</button>
      </div>
    </section>
  )
}

function PlaybackStep({ onBack, onNext }: { onBack: () => void; onNext: () => void }) {
  const [start, setStart] = useState('menu')
  const [fx, setFx] = useState(true)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => { setStart(s.dvdStart || 'menu'); setFx(s.insertFx !== '0') }).catch(() => {}) }, [])
  const save = async () => {
    setBusy(true)
    try { await api.put('/settings', { dvdStart: start, insertFx: fx ? '1' : '0' }) } catch { /* can be changed later */ }
    setBusy(false)
    onNext()
  }
  return (
    <section>
      <h1>DVDs and Blu-rays</h1>
      <p className="lead">Disc images play like they would in a disc player. When someone presses play on a disc, what should happen?</p>
      <div className="oobe-choices" role="radiogroup" aria-label="When a disc starts">
        <button type="button" role="radio" aria-checked={start === 'menu'} className={'oobe-choice' + (start === 'menu' ? ' on' : '')} onClick={() => setStart('menu')}>
          <span className="oobe-choice-pic menu" aria-hidden="true"><i /><i /><i /></span>
          <b>Show the disc menu</b><span>Studio intros, then the menu with its buttons, chapters and extras.</span>
        </button>
        <button type="button" role="radio" aria-checked={start === 'title'} className={'oobe-choice' + (start === 'title' ? ' on' : '')} onClick={() => setStart('title')}>
          <span className="oobe-choice-pic film" aria-hidden="true"><i /></span>
          <b>Play the movie straight away</b><span>Skip intros and menus. The menu is still one button away.</span>
        </button>
      </div>
      <label className="oobe-toggle">
        <input type="checkbox" checked={fx} onChange={(e) => setFx(e.target.checked)} />
        <span><b>Disc insert animation</b><small>A tray slides in when a disc starts, with drive sounds.</small></span>
      </label>
      <div className="oobe-actions">
        <button type="button" className="btn ghost" onClick={onBack}>Back</button>
        <button type="button" className="btn primary big" disabled={busy} onClick={save}>Continue</button>
      </div>
    </section>
  )
}

function LookStep({ onBack, onNext }: { onBack: () => void; onNext: () => void }) {
  const { serverTheme, setServerTheme } = useApp()
  const [themes, setThemes] = useState<ThemeMeta[]>([])
  const [pick, setPick] = useState(serverTheme)
  const nav = useNavStyle()
  useEffect(() => { api.get<{ themes: ThemeMeta[] }>('/themes').then((r) => setThemes(r.themes)).catch(() => {}) }, [])
  const choose = (id: string) => { setPick(id); applyTheme(id) }
  const save = async () => {
    try { await api.put('/settings', { theme: pick }); setServerTheme(pick) } catch { /* can be changed later */ }
    onNext()
  }
  // A handful of looks to start from; every theme is in Settings → Appearance.
  const featured = ['default', 'dark', 'oled', 'light', 'dark-blue', 'dark-green', 'light-pink', 'dark-gold']
  const shown = themes.filter((t) => featured.includes(t.id)).sort((a, b) => featured.indexOf(a.id) - featured.indexOf(b.id))
  const navOpts: [NavStyle, string, string][] = [['top', 'Top bar', 'Libraries along the top'], ['side', 'Side menu', 'Libraries in a menu on the left']]
  return (
    <section>
      <h1>Make it yours</h1>
      <p className="lead">Pick a look for everyone. It changes right away; people can pick their own later in Settings → Appearance, where there are more themes.</p>
      <div className="oobe-themes" role="radiogroup" aria-label="Theme">
        {shown.map((t) => (
          <button key={t.id} type="button" role="radio" aria-checked={pick === t.id} className={'oobe-theme' + (pick === t.id ? ' on' : '')} onClick={() => choose(t.id)}>
            <span className="oobe-theme-pic" style={{ background: t.preview.bg }}>
              <i className="bar" style={{ background: t.preview.surface }} />
              <i className="card" style={{ background: t.preview.surface }} /><i className="card" style={{ background: t.preview.surface }} /><i className="card" style={{ background: t.preview.surface }} />
              <i className="btn" style={{ background: t.preview.accent }} />
            </span>
            <b>{t.name.replace(/^WittyMediaServer /, '')}</b>
          </button>
        ))}
      </div>
      <h2 className="oobe-sub">Navigation on this device</h2>
      <div className="oobe-choices two" role="radiogroup" aria-label="Navigation">
        {navOpts.map(([v, l, d]) => (
          <button key={v} type="button" role="radio" aria-checked={nav === v} className={'oobe-choice small' + (nav === v ? ' on' : '')} onClick={() => setNavStyle(v)}>
            <span className={'layout-pic pic-' + v} aria-hidden="true"><i className="s" /><i className="t" /><i className="c" /></span>
            <b>{l}</b><span>{d}</span>
          </button>
        ))}
      </div>
      <div className="oobe-actions">
        <button type="button" className="btn ghost" onClick={onBack}>Back</button>
        <button type="button" className="btn primary big" onClick={save}>Continue</button>
      </div>
    </section>
  )
}
