// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Guarded, refreshModuleWeb, useModuleSetupStep, useSetupStepOwner } from '../moduleRegistry'
import { useCallback, useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, setToken, type Library } from '../api'
import type { StyledPart } from '../brandStyle.ts'
import { Brand, BrandMark, Spinner, VersionTag } from '../components/common'
import { HelpLink } from '../components/HelpLink'
import type { LibraryKind, ModuleReport } from '../moduleStore'
import { toast, useApp } from '../state'
import { LocaleFirst, localeAsked, saveLocale } from './setup/Locale'
import { t, tx } from '../i18n'
import { Button, Progress } from '../ui'
import { Account } from './setup/Account'
import { Details } from './setup/Details'
import { Discs } from './setup/Discs'
import { Done } from './setup/Done'
import { Listings } from './setup/Listings'
import { Logos } from './setup/Logos'
import { Look, Theme, WittyTvLook } from './setup/Look'
import { People } from './setup/People'
import { Remote } from './setup/Remote'
import { Media } from './setup/Media'
import { Modules } from './setup/Modules'
import { Karaoke, Music } from './setup/Music'
import { StepHead, type StepProps } from './setup/parts'
import { afterExtras, canSkip, currentStep, DRAWN, drawnId, drawnNumber, missingWhy, neighbours, pathOf, planSteps, stepFromPath, type Facts, type StepId, type StepInfo } from './setup/plan'
import { Name, Network } from './setup/Name'
import { Storage } from './setup/Storage'
import { Welcome } from './setup/Welcome'
import { doneBefore } from './setup/serverSteps'
import './setup/setup.css'

// First-time setup. Which steps there are comes from setup/plan.ts, and each lives in a file of its
// own in setup/. Each step has an address, so Back on a remote, a browser's own Back button and a
// reload all do what they should, and the focus and announcing that any page change gets come
// free. A choice is saved when it's made, and every step reads what's saved when it opens, so
// someone can go back, or come back tomorrow, without losing anything.

export default function SetupWizard() {
  const { user, setUser, setOnboarding, brandName, version, refreshLibraries } = useApp()
  const loc = useLocation()
  const navigate = useNavigate()
  const signedIn = !!user
  const [facts, setFacts] = useState<Facts | null>(signedIn ? null : { signedIn: false, store: true, modules: [], kinds: null })
  const [added, setAdded] = useState<Library[]>([])
  // The modules that draw a step of their own (Live TV, Games), if any is in use.
  const owners: Partial<Record<StepId, string>> = { tv: useSetupStepOwner('tv'), games: useSetupStepOwner('games') }

  // What's on this server decides which steps there are. What its modules add to the web app is asked
  // for again with it: the first ask, before there was an account, was refused, and the Modules step
  // adds some, so a module's own step (Live TV, Games) would otherwise have nothing to draw.
  const refresh = useCallback(async () => {
    const [report, kinds, libs, , place] = await Promise.all([
      api.get<ModuleReport>('/modules').catch(() => null),
      api.get<{ libraries: LibraryKind[] }>('/modules/libraries').then((r) => r.libraries || []).catch(() => [] as LibraryKind[]),
      api.get<Library[]>('/libraries').catch(() => [] as Library[]),
      refreshModuleWeb(),
      api.get<{ ask: boolean }>('/data-location').catch(() => null),
    ])
    setFacts({ signedIn: true, store: !!report, modules: (report?.modules || []).filter((m) => m.installed).map((m) => m.id), kinds, askData: !!place?.ask })
    setAdded(libs)
  }, [])
  useEffect(() => { if (signedIn) void refresh() }, [signedIn, refresh])

  // The name being tried on the Name step, drawn in the corner as it is typed.
  const [tryName, setTryName] = useState<{ name: string; parts: StyledPart[] } | null>(null)

  const steps = facts ? planSteps(facts) : []
  const wanted = stepFromPath(loc.pathname)
  const found = facts ? currentStep(steps, wanted, signedIn) : null
  // The language and time zone are asked before anything else, once, in this browser.
  const step = found?.id === 'welcome' && !localeAsked() ? steps.find((s) => s.id === 'language') || found : found
  // Show the address of the step that's showing, without a stop in the history for the old one.
  useEffect(() => { if (step && wanted !== step.id) navigate(pathOf(step.id) + loc.search, { replace: true }) }, [step, wanted, navigate, loc.search])
  useEffect(() => { window.scrollTo({ top: 0 }) }, [step?.id])
  // The server keeps where first run was left, so a reload or another browser carries on from there.
  const stepId = step?.id
  useEffect(() => { if (signedIn && stepId) api.put('/setup/state', { step: drawnId(stepId), done: doneBefore(stepId) }).catch(() => { /* it is only a bookmark */ }) }, [signedIn, stepId])

  if (!facts || !step) return <div className="oobe"><div className="center"><Spinner /></div></div>

  const { back, next } = neighbours(steps, step.id)
  const number = drawnNumber(steps, step.id)
  // Each numbered step, with why it's passed over when it is ("Nothing to set up").
  const stepper = DRAWN.map((x) => ({ id: x.id, label: tx(x.label), first: steps.find((s) => s.drawn === x.id)?.id, why: missingWhy(x.id, steps, step.id, { t }) }))
  const nowAt = stepper.findIndex((x) => x.id === drawnId(step.id))
  const go = (id: StepId) => navigate(pathOf(id))
  const props: StepProps = { info: step, facts, refresh, next: () => { if (next) go(next.id) }, back: back ? () => go(back.id) : undefined }
  // A module's own step is remembered on the server when someone moves on from it, done or put off,
  // so "Finish setting up" isn't left on Home for a step that was gone through here. The module's
  // page may say so too; twice is still once.
  // WittyTV's look is the server's own question about WittyTuner (wittytv-look), asked again after a fresh install.
  const passed = (id: StepId, done: boolean) => {
    const [module, name] = id === 'wittytv' ? [facts.modules.includes('wittytuner') ? 'wittytuner' : undefined, 'wittytv-look'] : [owners[id], id]
    if (module) api.post('/setup/module-steps/skip', { module, step: name, done }).catch(() => { /* Home offers it again, which is all */ })
  }
  // A module that draws its own step has its own Skip for now; the one here is for the app's own pages.
  const drawsOwnSkip = (step.id === 'tv' || step.id === 'games') && !!owners[step.id]
  const firstExtra = steps.find((s) => s.drawn === 'extras')?.id === step.id
  const moduleProps = (id: StepId) => ({ ...props, next: () => { passed(id, true); props.next() } })
  const finish = async (to = '/') => {
    // First run is over: the server answers the apps from here on.
    try { await api.post('/setup/complete', {}) } catch { /* an administrator only; the settings below still say so */ }
    try { await api.put('/settings', { setupDone: '1' }) } catch { /* it asks again next time */ }
    await refreshLibraries()
    navigate(to, { replace: true })
    setOnboarding(false)
  }

  return (
    <div className="oobe">
      <div className="oobe-glow" aria-hidden="true" />
      <div className="oobe-shell">
        <header className="oobe-head">
          <div className="brand"><BrandMark /><Brand name={tryName?.name ?? brandName} parts={tryName?.parts} /></div>
          <div className="su-progress">
            <div className="su-progress-line">
              <p className="su-progress-words" id="su-where">{t('Step {at} of {of}', { at: number.at, of: number.of })}<span className="su-where-name"> · {tx(DRAWN.find((x) => x.id === drawnId(step.id))?.label || step.label)}</span>{number.page && <span className="su-where-page"> · {t('part {at} of {of}: {name}', { at: number.page.at, of: number.page.of, name: tx(step.label) })}</span>}</p>
              <HelpLink to="getting-started#what-first-time-setup-asks" newTab>{t('Help with setup')}</HelpLink>
            </div>
            <Progress label={t('How far through setup')} value={number.at} max={number.of} />
          </div>
        </header>

        <div className="oobe-body">
          <nav className="oobe-steps" aria-label={t('Setup steps')}>
            <ol>
              {stepper.map((x, i) => {
                const state = i < nowAt ? 'done' : i === nowAt ? 'now' : 'todo'
                const goBack = signedIn && i < nowAt && !x.why && x.first && x.id !== 'language' && x.id !== 'welcome' && x.id !== 'account'
                return (
                  <li key={x.id} className={'oobe-step ' + state + (x.why ? ' missing' : '')} aria-current={state === 'now' ? 'step' : undefined}>
                    {goBack
                      ? <button type="button" onClick={() => go(x.first as StepId)}><i aria-hidden="true">{state === 'done' ? '✓' : i + 1}</i>{x.label}<span className="sr-only"> {t('(done)')}</span></button>
                      : <span><i aria-hidden="true">{state === 'done' ? '✓' : i + 1}</i><span className="oobe-step-label">{x.label}{x.why && <small><span className="sr-only">, </span>{x.why}</small>}</span>{state === 'done' && <span className="sr-only"> {t('(done)')}</span>}</span>}
                  </li>
                )
              })}
            </ol>
          </nav>

          <div className="oobe-col">
            <main id="main" tabIndex={-1} className="oobe-card su-card-main" key={step.id} aria-describedby="su-where">
              {step.id === 'language' && <LocaleFirst onDone={() => go('welcome')} />}
              {step.id === 'welcome' && <Welcome onStart={() => go('account')} onLanguage={() => go('language')} />}
              {step.id === 'account' && <Account back={() => go('welcome')} onDone={(u, tok) => {
                setToken(tok)
                setOnboarding(true) // keep setup open after signing in
                setFacts(null)
                setUser(u)
                // From here it counts as set up only when the last page says so, even once there are libraries.
                api.put('/settings', { setupDone: '0' }).catch(() => {})
                // The language and time zone asked before Welcome are saved now there's an account, before the
                // Name step reads them back.
                void saveLocale().catch(() => toast(t('The language and time zone couldn’t be saved. Choose them again on this page.'), { tone: 'danger' }))
                  // The first step after the account: where data is kept, on a desktop that asks, else the name (the address
                  // falls back to the first step this server has).
                  .finally(() => navigate(pathOf('storage'), { replace: true }))
              }} />}
              {step.id === 'storage' && <Storage {...props} />}
              {step.id === 'name' && <Name {...props} onTry={setTryName} />}
              {step.id === 'network' && <Network {...props} />}
              {step.id === 'modules' && <Modules {...props} />}
              {step.id === 'media' && <Media {...props} added={added} onAdded={(l) => setAdded((a) => a.some((x) => x.id === l.id) ? a.map((x) => (x.id === l.id ? l : x)) : [...a, l])} />}
              {step.id === 'games' && <ModuleStep id="games" module={owners.games} props={{ ...moduleProps('games'), added, goTo: go }} own={<NotLoaded info={step} skip={next ? () => { passed('games', false); props.next() } : undefined} />} />}
              {step.id === 'discs' && <Discs {...props} />}
              {step.id === 'details' && <Details {...props} />}
              {step.id === 'logos' && <Logos {...props} />}
              {step.id === 'music' && <Music {...props} />}
              {step.id === 'karaoke' && <Karaoke {...props} />}
              {step.id === 'listings' && <Listings {...props} />}
              {step.id === 'tv' && <ModuleStep id="tv" module={owners.tv} props={moduleProps('tv')} own={<NotLoaded info={step} skip={next ? () => { passed('tv', false); props.next() } : undefined} />} />}
              {step.id === 'look' && <Look {...props} />}
              {step.id === 'theme' && <Theme {...props} />}
              {step.id === 'wittytv' && <WittyTvLook {...moduleProps('wittytv')} />}
              {step.id === 'people' && <People {...props} />}
              {step.id === 'remote' && <Remote {...props} />}
              {step.id === 'done' && <Done steps={steps} facts={facts} onFinish={finish} />}
            </main>
            {/* The page itself says where to change it later, once. */}
            {canSkip(step) && next && !drawsOwnSkip && (
              <p className="oobe-skip">
                <Button variant="ghost" onClick={() => { passed(step.id, false); props.next() }}>{t('Skip for now')}</Button>
                {firstExtra && afterExtras(steps) && <Button variant="ghost" onClick={() => go('done')}>{t('Skip the extras')}</Button>}
              </p>
            )}
          </div>
        </div>
        <footer className="oobe-foot"><VersionTag version={version} /></footer>
      </div>
    </div>
  )
}

// A first-run step a module of API 5 draws itself (moduleRegistry.tsx), given the same props the
// app's own step has; the app's own while there is none, or if the module's step crashes, so
// setup always goes on.
function ModuleStep({ id, module, props, own }: { id: string; module?: string; props: Record<string, unknown>; own: React.ReactNode }) {
  const C = useModuleSetupStep(id)
  if (C === undefined) return <div className="center"><div className="spinner" /></div>
  return C && module ? <Guarded module={module} instead={own}><C {...props} /></Guarded> : <>{own}</>
}

// A module's step whose page didn't load says so, never an empty card, with its own way on.
function NotLoaded({ info, skip }: { info: StepInfo; skip?: () => void }) {
  return (
    <>
      <StepHead title={tx(info.label)} lead={t('This step comes from a module, and its page didn’t load. Skip it for now and set it up later in {where}.', { where: info.later?.where ? tx(info.later.where) : t('Settings') })} />
      {skip && <div className="oobe-actions su-actions"><span /><Button variant="primary" size="big" onClick={skip}>{t('Skip for now')}</Button></div>}
    </>
  )
}
