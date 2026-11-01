import { lazy, Suspense, useEffect, useState } from 'react'
import { api } from '../../api'
import BrandDialog from '../../components/BrandDialog'
import { IconAudio, IconBook, IconGamepad, IconPalette, IconTv, IconUsers } from '../../icons'
import { Button, Field, Input } from '../../ui'
import { WhereIsTheCode } from './parts'

const RestoreFlow = lazy(() => import('../settings/Backup').then((m) => ({ default: m.RestoreFlow })))

const FEATURES = [
  { icon: <IconTv />, title: 'Movies, TV and discs', text: 'Posters, plots and “continue watching” on every screen. Copies of your discs play with their real menus.' },
  { icon: <IconAudio />, title: 'Music and podcasts', text: 'Albums, audiobooks that remember your place, and shows you follow.' },
  { icon: <IconBook />, title: 'Books and comics', text: 'Read them here, with highlights and notes.' },
  { icon: <IconGamepad />, title: 'Games', text: 'Your own games, played right in the browser.' },
  { icon: <IconUsers />, title: 'Everyone at home', text: 'A profile for each person, and Kids profiles by age.' },
  { icon: <IconPalette />, title: 'Yours', text: 'It runs on your own computer. No account with anyone, and nothing is reported back.' },
]

export function Welcome({ serverName, logo, onName, onStart }: { serverName: string; logo: string; onName: (name: string, logo: File | 'remove' | null) => void; onStart: () => void }) {
  const [naming, setNaming] = useState(false)
  const [restoring, setRestoring] = useState(false)
  // While a backup is coming back, starting something else would only get in its way.
  const [busy, setBusy] = useState(false)
  return (
    <section className="oobe-welcome">
      <img className="oobe-welcome-logo" src={logo || '/icon.svg'} alt="" />
      <h1>Welcome to your media server</h1>
      <p className="lead">Everything you own, in one place, on every screen in your home. Setup takes a few minutes. You can skip anything you’re not sure about, and change it all later.</p>
      <div className="su-starts" data-nav-group>
        {!busy && <Button variant="primary" size="big" autoFocus onClick={onStart}>Set up a new server</Button>}
        {!busy && <Button size="big" aria-expanded={restoring} onClick={() => setRestoring((o) => !o)}>{restoring ? 'Close restore' : 'Restore from a backup'}</Button>}
      </div>
      <p className="muted su-start-note">Coming back from a reset, or moving to this computer? Restoring brings back your accounts, libraries, what you’ve watched and your settings. Modules download again afterwards, as a backup doesn’t hold them.</p>
      {restoring && <RestorePanel onBusy={setBusy} />}
      <ul className="oobe-perks" aria-label="What your server does">
        {FEATURES.map((f) => <li key={f.title}><span className="ic">{f.icon}</span><span><b>{f.title}</b> <span className="muted">{f.text}</span></span></li>)}
      </ul>
      <div className="oobe-name">
        <div className="oobe-name-now">
          <span className="muted">Your server is called</span>
          <b>{serverName.trim() || 'WittyMediaServer'}</b>
        </div>
        <Button onClick={() => setNaming(true)}>Rename…</Button>
      </div>
      {naming && <BrandDialog name={serverName} logo={logo} onClose={() => setNaming(false)} onSave={onName} />}
    </section>
  )
}

// Restoring needs the setup code from another device, the same as making an account does.
function RestorePanel({ onBusy }: { onBusy: (b: boolean) => void }) {
  const [needCode, setNeedCode] = useState(false)
  const [code, setCode] = useState('')
  useEffect(() => { api.get<{ setupCodeRequired?: boolean }>('/status').then((s) => setNeedCode(!!s.setupCodeRequired)).catch(() => {}) }, [])
  return (
    <div className="su-restore">
      {needCode && (
        <>
          <Field label="Setup code"><Input autoComplete="off" autoCapitalize="characters" spellCheck={false} value={code} onChange={(e) => setCode(e.target.value)} /></Field>
          <WhereIsTheCode />
        </>
      )}
      {(!needCode || code.trim().length > 0) && <Suspense fallback={null}><RestoreFlow firstRun setupCode={needCode ? code.trim() : undefined} onBusy={onBusy} /></Suspense>}
    </div>
  )
}
