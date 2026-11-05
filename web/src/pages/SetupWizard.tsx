import { useCallback, useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, setToken, type Library } from '../api'
import { Brand, Spinner, VersionTag } from '../components/common'
import { HelpLink } from '../components/HelpLink'
import { uploadLogo } from '../components/BrandDialog'
import type { LibraryKind, ModuleReport } from '../moduleStore'
import { useApp } from '../state'
import { Progress } from '../ui'
import { Account } from './setup/Account'
import { Details } from './setup/Details'
import { Discs } from './setup/Discs'
import { Done } from './setup/Done'
import { Games } from './setup/Games'
import { Listings } from './setup/Listings'
import { Logos } from './setup/Logos'
import { Look } from './setup/Look'
import { Media } from './setup/Media'
import { Modules } from './setup/Modules'
import { Music } from './setup/Music'
import type { StepProps } from './setup/parts'
import { currentStep, neighbours, pathOf, planSteps, stepFromPath, stepNumber, type Facts, type StepId } from './setup/plan'
import { Welcome } from './setup/Welcome'
import './setup/setup.css'

// First-time setup. Which steps there are comes from setup/plan.ts, and each lives in a file of its
// own in setup/. Each step has an address, so Back on a remote, a browser's own Back button and a
// reload all do what they should, and the focus and announcing that any page change gets come
// free. A choice is saved when it's made, and every step reads what's saved when it opens, so
// someone can go back, or come back tomorrow, without losing anything.

export default function SetupWizard() {
  const { user, setUser, setOnboarding, brandName, brandColouring, setBrandName, setBrandLogo, version, refreshLibraries } = useApp()
  const loc = useLocation()
  const navigate = useNavigate()
  const signedIn = !!user
  const [facts, setFacts] = useState<Facts | null>(signedIn ? null : { signedIn: false, store: true, modules: [], kinds: null })
  const [added, setAdded] = useState<Library[]>([])

  // What's on this server decides which steps there are.
  const refresh = useCallback(async () => {
    const [report, kinds, libs] = await Promise.all([
      api.get<ModuleReport>('/modules').catch(() => null),
      api.get<{ libraries: LibraryKind[] }>('/modules/libraries').then((r) => r.libraries || []).catch(() => [] as LibraryKind[]),
      api.get<Library[]>('/libraries').catch(() => [] as Library[]),
    ])
    setFacts({ signedIn: true, store: !!report, modules: (report?.modules || []).filter((m) => m.installed).map((m) => m.id), kinds })
    setAdded(libs)
  }, [])
  useEffect(() => { if (signedIn) void refresh() }, [signedIn, refresh])

  const [serverName, setServerName] = useState(brandName)
  const [colouring, setColouring] = useState(brandColouring)
  // A logo chosen on the welcome page waits until the account exists, as only an administrator
  // may store one, and is uploaded then.
  const [pendingLogo, setPendingLogo] = useState<File | null>(null)
  const [logoPreview, setLogoPreview] = useState('')
  useEffect(() => {
    if (!pendingLogo) { setLogoPreview(''); return }
    const u = URL.createObjectURL(pendingLogo)
    setLogoPreview(u)
    return () => URL.revokeObjectURL(u)
  }, [pendingLogo])

  const steps = facts ? planSteps(facts) : []
  const wanted = stepFromPath(loc.pathname)
  const step = facts ? currentStep(steps, wanted, signedIn) : null
  // Show the address of the step that's showing, without a stop in the history for the old one.
  useEffect(() => { if (step && wanted !== step.id) navigate(pathOf(step.id) + loc.search, { replace: true }) }, [step, wanted, navigate, loc.search])
  useEffect(() => { window.scrollTo({ top: 0 }) }, [step?.id])

  if (!facts || !step) return <div className="oobe"><div className="center"><Spinner /></div></div>

  const { back, next, index } = neighbours(steps, step.id)
  const number = stepNumber(index, steps.length, signedIn)
  const go = (id: StepId) => navigate(pathOf(id))
  const props: StepProps = { info: step, facts, refresh, next: () => { if (next) go(next.id) }, back: back ? () => go(back.id) : undefined }
  const finish = async (to = '/') => {
    try { await api.put('/settings', { setupDone: '1', brandColouring: colouring, ...(serverName.trim() ? { brandName: serverName.trim() } : {}) }) } catch { /* it asks again next time */ }
    setBrandName(serverName.trim() || brandName, colouring)
    await refreshLibraries()
    navigate(to, { replace: true })
    setOnboarding(false)
  }

  return (
    <div className="oobe">
      <div className="oobe-glow" aria-hidden="true" />
      <div className="oobe-shell">
        <header className="oobe-head">
          <div className="brand"><img className="brand-mark" src={logoPreview || '/icon.svg'} alt="" /><Brand name={serverName || brandName} colouring={colouring} /></div>
          <div className="su-progress">
            <div className="su-progress-line">
              <p className="su-progress-words">Step {number.at} of {number.of}: {step.label}</p>
              <HelpLink to="getting-started#what-first-time-setup-asks" newTab>Help with setup</HelpLink>
            </div>
            <Progress label="How far through setup" value={number.at} max={number.of} />
          </div>
        </header>

        <main id="main" tabIndex={-1} className="oobe-card su-card-main" key={step.id}>
          {step.id === 'welcome' && <Welcome serverName={serverName} colouring={colouring} logo={logoPreview} onStart={() => go('account')}
            onName={(n, logo, c) => { setServerName(n); setColouring(c); if (logo === 'remove') setPendingLogo(null); else if (logo) setPendingLogo(logo) }} />}
          {step.id === 'account' && <Account back={() => go('welcome')} onDone={(u, tok) => {
            setToken(tok)
            setOnboarding(true) // keep setup open after signing in
            setFacts(null)
            setUser(u)
            // From here it counts as set up only when the last page says so, even once there are libraries.
            api.put('/settings', { setupDone: '0' }).catch(() => {})
            if (pendingLogo) uploadLogo(pendingLogo).then(setBrandLogo).catch(() => {})
            navigate(pathOf('modules'), { replace: true })
          }} />}
          {step.id === 'modules' && <Modules {...props} />}
          {step.id === 'media' && <Media {...props} added={added} onAdded={(l) => setAdded((a) => a.some((x) => x.id === l.id) ? a.map((x) => (x.id === l.id ? l : x)) : [...a, l])} />}
          {step.id === 'games' && <Games {...props} added={added} goTo={go} />}
          {step.id === 'discs' && <Discs {...props} />}
          {step.id === 'details' && <Details {...props} />}
          {step.id === 'logos' && <Logos {...props} />}
          {step.id === 'music' && <Music {...props} />}
          {step.id === 'listings' && <Listings {...props} />}
          {step.id === 'look' && <Look {...props} />}
          {step.id === 'done' && <Done steps={steps} onFinish={finish} />}
        </main>
        <footer className="oobe-foot"><VersionTag version={version} /></footer>
      </div>
    </div>
  )
}
