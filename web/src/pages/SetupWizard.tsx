import { useState } from 'react'
import { api, setToken, type Library, type User } from '../api'
import { Brand, VersionTag } from '../components/common'
import { ChooseFolderButton } from '../components/FolderPicker'
import { IconAnime, IconAudio, IconBook, IconCheck, IconDisc, IconFilm, IconFolder, IconTv } from '../icons'
import { useApp } from '../state'
import { AddProfile } from './Profiles'

// The first-run setup: welcome → account → libraries → metadata → done.
// Every step after the account can be skipped and changed later in Settings.
// It resumes where it left off after a reload (the account step is then
// already done).

type Step = 'welcome' | 'account' | 'libraries' | 'metadata' | 'done'
const STEPS: { id: Step; label: string }[] = [
  { id: 'welcome', label: 'Welcome' }, { id: 'account', label: 'Your account' }, { id: 'libraries', label: 'Your media' },
  { id: 'metadata', label: 'Posters & info' }, { id: 'done', label: 'Ready' },
]

const LIB_TYPES = [
  { type: 'movies', name: 'Movies', icon: <IconFilm />, hint: 'Films, DVD and Blu-ray images (.iso) and disc folders' },
  { type: 'shows', name: 'TV Shows', icon: <IconTv />, hint: 'One folder per show, with season folders inside' },
  { type: 'music', name: 'Music', icon: <IconAudio />, hint: 'Albums by artist; tags and covers are read from the files' },
  { type: 'audiobooks', name: 'Audiobooks', icon: <IconBook />, hint: 'One folder per book; resumes where you stopped' },
  { type: 'anime', name: 'Anime', icon: <IconAnime />, hint: 'Series with AniList information; also collects anime found in your TV shows' },
  { type: 'videos', name: 'Home videos', icon: <IconDisc />, hint: 'Your own recordings and other videos, no online lookup' },
]

export default function SetupWizard({ resume }: { resume: boolean }) {
  const { setUser, setOnboarding, brandName, setBrandName, version, refreshLibraries } = useApp()
  const [step, setStep] = useState<Step>(resume ? 'libraries' : 'welcome')
  const idx = STEPS.findIndex((s) => s.id === step)
  const [serverName, setServerName] = useState(brandName)
  const [added, setAdded] = useState<Library[]>([])
  const [addKids, setAddKids] = useState(false)
  const [kidsAdded, setKidsAdded] = useState(false)

  const finish = async () => {
    try { await api.put('/settings', { setupDone: '1', ...(serverName.trim() ? { brandName: serverName.trim() } : {}) }) } catch { /* ignore */ }
    if (serverName.trim()) setBrandName(serverName.trim())
    await refreshLibraries()
    setOnboarding(false)
  }

  return (
    <div className="oobe">
      <div className="oobe-glow" aria-hidden="true" />
      <div className="oobe-shell">
        <header className="oobe-head">
          <div className="brand"><img src="/icon.svg" alt="" /><Brand name={serverName || brandName} /></div>
          <ol className="oobe-steps" aria-label="Setup progress">
            {STEPS.map((s, i) => (
              <li key={s.id} className={i < idx ? 'done' : i === idx ? 'current' : ''} aria-current={i === idx ? 'step' : undefined}>
                <span className="dot">{i < idx ? <IconCheck size={14} /> : i + 1}</span><span className="lbl">{s.label}</span>
              </li>
            ))}
          </ol>
        </header>

        <main className="oobe-card" key={step}>
          {step === 'welcome' && (
            <section>
              <h1>Welcome to your media server</h1>
              <p className="lead">Let's get your movies, shows and discs ready to watch on every screen in your home. It takes about three minutes.</p>
              <ul className="oobe-points">
                <li><b>1.</b> Create your account</li>
                <li><b>2.</b> Show us where your media is</li>
                <li><b>3.</b> Optionally connect free services for posters and descriptions</li>
              </ul>
              <label className="oobe-field">Give your server a name <span className="muted">(optional, shown in the corner)</span>
                <input className="input" value={serverName} onChange={(e) => setServerName(e.target.value)} placeholder="e.g. The Living Room Cinema" maxLength={40} />
              </label>
              <div className="oobe-actions"><span /><button className="btn primary big" autoFocus onClick={() => setStep('account')}>Get started</button></div>
            </section>
          )}

          {step === 'account' && <AccountStep onBack={() => setStep('welcome')} onDone={(u, tok) => {
            setToken(tok)
            setOnboarding(true) // keep the wizard open after signing in
            setUser(u)
            setStep('libraries')
          }} />}

          {step === 'libraries' && (
            <LibrariesStep added={added} onAdded={(l) => setAdded((a) => [...a, l])}
              onNext={() => setStep('metadata')} />
          )}

          {step === 'metadata' && <MetadataStep onBack={() => setStep('libraries')} onNext={() => setStep('done')} />}

          {step === 'done' && (
            <section className="oobe-done">
              <div className="oobe-check" aria-hidden="true"><IconCheck size={44} /></div>
              <h1>You're all set</h1>
              <p className="lead">
                {added.length
                  ? `Your ${added.length === 1 ? 'library is' : `${added.length} libraries are`} being scanned now. Posters and titles appear as each item is found.`
                  : 'You can add libraries any time in Settings → Libraries.'}
              </p>
              <p className="muted">Open this server from your TV, phone or another computer at the same address you're using now.</p>
              <div className="oobe-kids">
                {kidsAdded
                  ? <span><IconCheck size={16} /> Kids profile added</span>
                  : <><span>Do children watch too? A Kids profile only shows titles for their age.</span><button type="button" className="btn" onClick={() => setAddKids(true)}>Add a Kids profile</button></>}
              </div>
              {addKids && <AddProfile admin initialKind="kids" onClose={() => setAddKids(false)} onDone={() => { setAddKids(false); setKidsAdded(true) }} />}
              <div className="oobe-actions"><span /><button className="btn primary big" autoFocus onClick={finish}>Start watching</button></div>
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
  const strength = pw.length >= 12 ? 'Strong' : pw.length >= 8 ? 'Good' : pw.length >= 4 ? 'Short (fine for a PIN at home)' : ''
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    if (pw !== pw2) { setErr('The two passwords are different.'); return }
    setBusy(true)
    try {
      const r = await api.post<{ token: string; user: User }>('/setup', { name, password: pw, device: navigator.userAgent.slice(0, 80) })
      onDone(r.user, r.token)
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <form onSubmit={submit}>
      <h1>Create your account</h1>
      <p className="lead">This is the administrator: the account that manages libraries, users and settings. You can add family profiles later.</p>
      <div className="oobe-form">
        <label className="oobe-field">Your name<input className="input" autoFocus required value={name} onChange={(e) => setName(e.target.value)} autoComplete="username" /></label>
        <label className="oobe-field">Password
          <input className="input" type="password" required minLength={4} value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" aria-describedby="pw-strength" />
          <span id="pw-strength" className="muted" style={{ fontSize: 12 }}>{strength || 'At least 4 characters'}</span>
        </label>
        <label className="oobe-field">Type it again<input className="input" type="password" required value={pw2} onChange={(e) => setPw2(e.target.value)} autoComplete="new-password" /></label>
      </div>
      {err && <div className="error" role="alert">{err}</div>}
      <div className="oobe-actions">
        <button type="button" className="btn ghost" onClick={onBack}>Back</button>
        <button className="btn primary big" disabled={busy || !name || pw.length < 4}>{busy ? 'Creating…' : 'Create account'}</button>
      </div>
    </form>
  )
}

function LibrariesStep({ added, onAdded, onNext }: { added: Library[]; onAdded: (l: Library) => void; onNext: () => void }) {
  const [type, setType] = useState<string | null>(null)
  const [paths, setPaths] = useState<string[]>([])
  const [name, setName] = useState('')
  const [manual, setManual] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const t = LIB_TYPES.find((x) => x.type === type)
  const add = async () => {
    setBusy(true); setErr('')
    try {
      const lib = await api.post<Library>('/libraries', { name: name.trim() || t!.name, type, paths })
      onAdded(lib)
      setType(null); setPaths([]); setName('')
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <section>
      <h1>Where is your media?</h1>
      <p className="lead">Pick a kind of media, then the folder it's in. Add as many as you like; each becomes a library.</p>
      {added.length > 0 && (
        <ul className="oobe-added" aria-label="Libraries added">
          {added.map((l) => <li key={l.id}><IconCheck size={16} /> <b>{l.name}</b> <span className="muted">{l.paths.join(', ')}</span></li>)}
        </ul>
      )}
      {!type ? (
        <div className="oobe-types">
          {LIB_TYPES.map((x) => (
            <button key={x.type} type="button" className="oobe-type" onClick={() => setType(x.type)}>
              <span className="ic">{x.icon}</span><b>{x.name}</b><span className="muted">{x.hint}</span>
            </button>
          ))}
        </div>
      ) : (
        <div className="oobe-lib">
          <div className="oobe-lib-head"><span className="ic">{t!.icon}</span><b>{t!.name}</b><button type="button" className="linkish" onClick={() => { setType(null); setPaths([]) }}>Change</button></div>
          <label className="oobe-field">Library name<input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder={t!.name} /></label>
          <div className="oobe-paths">
            {paths.map((p) => <div key={p} className="oobe-path"><IconFolder /> <span>{p}</span><button type="button" className="btn sm ghost" onClick={() => setPaths(paths.filter((x) => x !== p))} aria-label={`Remove ${p}`}>Remove</button></div>)}
          </div>
          <div className="field-row">
            <ChooseFolderButton className={'btn ' + (paths.length ? '' : 'primary')} label={paths.length ? 'Add another folder…' : 'Choose folder…'} onPick={(p) => setPaths((x) => [...new Set([...x, p])])} />
            <input className="input" placeholder="…or type a path, e.g. D:\Movies" aria-label="Folder path" value={manual} onChange={(e) => setManual(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' && manual.trim()) { e.preventDefault(); setPaths((x) => [...new Set([...x, manual.trim()])]); setManual('') } }} />
          </div>
          {err && <div className="error" role="alert">{err}</div>}
          <div className="oobe-actions"><span /><button type="button" className="btn primary" disabled={!paths.length || busy} onClick={add}>{busy ? 'Adding…' : `Add ${name.trim() || t!.name}`}</button></div>
        </div>
      )}
      <div className="oobe-actions">
        <span className="muted" style={{ fontSize: 13 }}>{added.length ? 'Scanning starts right away in the background.' : 'You can also do this later.'}</span>
        <button type="button" className={'btn big ' + (added.length ? 'primary' : 'ghost')} onClick={onNext}>{added.length ? 'Continue' : 'Skip for now'}</button>
      </div>
    </section>
  )
}

const PROVIDERS = [
  {
    id: 'tmdb', name: 'The Movie Database (TMDB)', for: 'Movies and discs', key: 'tmdbKey',
    steps: [
      <>Create a free account at <a href="https://www.themoviedb.org/signup" target="_blank" rel="noreferrer">themoviedb.org</a>.</>,
      <>Open <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noreferrer">Settings → API</a> and ask for a key (choose “Personal use”).</>,
      <>Copy the <b>API Key</b> (or the longer <b>API Read Access Token</b>) and paste it below.</>,
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

function MetadataStep({ onBack, onNext }: { onBack: () => void; onNext: () => void }) {
  const [keys, setKeys] = useState<Record<string, string>>({})
  const [state, setState] = useState<Record<string, { ok?: boolean; msg?: string; busy?: boolean }>>({})
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
    onNext()
  }
  const any = Object.values(keys).some((v) => v.trim())
  return (
    <section>
      <h1>Posters, plots and cast</h1>
      <p className="lead">Your server already looks up <b>TV shows on TVmaze</b> and <b>movie facts on Wikidata</b>, no setup needed. For movie posters and the best matches, add a free key from the services below; each takes about two minutes. Your files never leave this computer, only their names are looked up.</p>
      <div className="oobe-builtin" role="note">
        <span><IconCheck size={16} /> TV: TVmaze, built in</span>
        <span><IconCheck size={16} /> Movies: Wikidata facts and ratings, built in</span>
      </div>
      <div className="oobe-providers">
        {PROVIDERS.map((p) => {
          const st = state[p.id] || {}
          return (
            <div key={p.id} className={'oobe-provider' + (st.ok ? ' ok' : '')}>
              <div className="op-head"><b>{p.name}</b><span className="pill">{p.for}</span>{st.ok && <span className="op-ok"><IconCheck size={14} /> Connected</span>}</div>
              <ol className="op-steps">{p.steps.map((s, i) => <li key={i}>{s}</li>)}</ol>
              <div className="field-row">
                <input className="input" aria-label={`${p.name} key`} placeholder="Paste your key" value={keys[p.key] || ''} spellCheck={false} autoComplete="off"
                  onChange={(e) => { setKeys((k) => ({ ...k, [p.key]: e.target.value })); setState((s) => ({ ...s, [p.id]: {} })) }} />
                {'pinKey' in p && <input className="input pin" aria-label="Subscriber PIN" placeholder="PIN (optional)" value={keys[p.pinKey] || ''} onChange={(e) => setKeys((k) => ({ ...k, [p.pinKey]: e.target.value }))} />}
                <button type="button" className="btn" disabled={!keys[p.key]?.trim() || st.busy} onClick={() => test(p)}>{st.busy ? 'Checking…' : 'Test'}</button>
              </div>
              {st.msg && !st.ok && <div className="error" role="alert">{st.msg}</div>}
              <div className="muted op-note">{p.note}</div>
            </div>
          )
        })}
      </div>
      <div className="oobe-actions">
        <button type="button" className="btn ghost" onClick={onBack}>Back</button>
        <button type="button" className={'btn big ' + (any ? 'primary' : 'ghost')} disabled={saving} onClick={save}>{any ? 'Save and continue' : 'Skip for now'}</button>
      </div>
    </section>
  )
}
