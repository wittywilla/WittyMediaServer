// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../../api'
import { announce } from '../../a11y'
import { IconCheck, IconDown, IconInfo, IconPlug, IconTv, libIcon } from '../../icons'
import { LIB_TYPES } from '../../components/AddLibrary'
import { Button, LoadError, Notice, Progress, Spinner } from '../../ui'
import { CATEGORIES, checkAll, isBusy, mb, PHASE, storeView, type Failure, type Run, type StoreItem, type StoreView } from '../settings/store/storeApi'
import { installAndWait } from './install'
import { notInstalledWords, olderFromStore, series } from '../settings/store/olderServer'
import { useOlderAsk } from '../settings/store/OlderServerDialog'
import { ChoiceCards, TickRow } from './parts'
import { ADVANCED, offerable, PLAIN_WORDS, RECOMMENDED, updateChoiceFrom, updateSettings, type UpdateChoice } from './plan'
import { list, t, tn, tx } from '../../i18n'

// Add-ons, in the Customize dialog of Your media: what the server does beyond movies, shows and music. Modules aren't part
// of the server: choosing one downloads it, signed, from the official catalogue, through the store's own install. Nothing's
// fetched until someone presses the button that says so, and nothing depends on it working. A kind of library that needs
// one offers it right on the step, so most people never open this.

const kindName = (k: string) => tx(LIB_TYPES.find((x) => x.type === k)?.name) || k
const glyph = (m: StoreItem) => m.libraries?.length ? libIcon(m.libraries[0]) : m.group === 'tv' ? <IconTv /> : <IconPlug />
// Each module's own icon, the same picture as the catalogue's, from the server itself (web/public/module-icons, drawn
// by scripts/genicon): nothing is fetched from the internet. A module without one keeps a glyph.
function ModulePicture({ m }: { m: StoreItem }) {
  const [broken, setBroken] = useState(false)
  if (broken || !/^[a-z0-9-]+$/.test(m.id)) return glyph(m)
  return <img className="su-module-icon" src={`/module-icons/${m.id}.png`} alt="" width={40} height={40} decoding="async" onError={() => setBroken(true)} />
}
const icon = (m: StoreItem) => <ModulePicture m={m} />
const ready = (m: StoreItem) => !!m.status?.installed

// skipped: its only version was made for an older server, and Install anyway wasn't chosen.
type Going = { id: string; name: string; state: 'waiting' | 'working' | 'done' | 'failed' | 'skipped'; failure?: Failure }

export function Modules({ refresh }: { refresh: () => Promise<void> }) {
  const [view, setView] = useState<StoreView | null>(null)
  const [failed, setFailed] = useState(false)
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  const [updates, setUpdates] = useState<UpdateChoice>('ask')
  const [going, setGoing] = useState<Going[] | null>(null)
  const [running, setRunning] = useState(false)
  const [live, setLive] = useState<Run | null>(null)
  const [checkErr, setCheckErr] = useState('')
  const stopped = useRef(false)
  const older = useOlderAsk()
  const load = () => { setFailed(false); return storeView().then((v) => { setView(v); return v }).catch(() => { setFailed(true); return null }) }
  useEffect(() => {
    void load()
    api.get<Record<string, string>>('/settings').then((s) => setUpdates(updateChoiceFrom(s))).catch(() => {})
    return () => { stopped.current = true }
  }, [])

  const offered = useMemo(() => offerable(view?.items || []), [view])
  // A module that can't run on this computer says so in place of being offered. Everything else
  // is only waiting for the list to be read, and that's what the button does.
  // One whose only version was made for an older server can be chosen, and asks before it installs.
  const cantRun = (m: StoreItem) => !ready(m) && !m.fit.ok && m.fit.kind !== 'no-version' && m.fit.kind !== 'older-server'
  const onlyOlder = (m: StoreItem) => !ready(m) && m.fit.kind === 'older-server'
  const shelves = useMemo(() => {
    const usual = offered.filter((m) => !ADVANCED.includes(m.id) && !RECOMMENDED.includes(m.id))
    const known = CATEGORIES.map((c) => ({ ...c, modules: usual.filter((m) => m.group === c.id) }))
    const rest = usual.filter((m) => !CATEGORIES.some((c) => c.id === m.group))
    return [{ id: 'first', title: t('Recommended'), modules: offered.filter((m) => RECOMMENDED.includes(m.id)) }, ...known.map((k) => ({ ...k, title: tx(k.title) })), { id: 'other', title: t('Other'), modules: rest }].filter((s) => s.modules.length)
  }, [offered])
  const advanced = offered.filter((m) => ADVANCED.includes(m.id))
  // What's recommended starts ticked, once, and can be unticked.
  const ticked = useRef(false)
  useEffect(() => {
    if (!view || ticked.current) return
    ticked.current = true
    setChosen(new Set(offered.filter((m) => RECOMMENDED.includes(m.id) && !ready(m) && !cantRun(m) && !onlyOlder(m)).map((m) => m.id)))
  }, [view, offered]) // eslint-disable-line react-hooks/exhaustive-deps
  const toggle = (id: string) => setChosen((s) => { const n = new Set(s); if (n.has(id)) n.delete(id); else n.add(id); return n })
  const picks = offered.filter((m) => chosen.has(m.id) && !ready(m))
  const row = (m: StoreItem) => {
    const words = tx(PLAIN_WORDS[m.id]) || m.summary
    if (ready(m)) return <TickRow key={m.id} checked disabled icon={icon(m)} title={m.name} text={words} note={t('Already on this server')} />
    if (cantRun(m)) return <TickRow key={m.id} checked={false} disabled icon={icon(m)} title={m.name} text={words} note={tx(m.fit.why)} />
    if (onlyOlder(m)) return <TickRow key={m.id} checked={chosen.has(m.id)} onChange={() => toggle(m.id)} icon={icon(m)} title={m.name} text={words}
      note={t('No version for this server is published yet. {name} {version} was made for WittyMediaServer {series}, and you’ll be asked before it’s installed.', { name: m.name, version: m.version, series: series(m.madeFor || '') })} />
    return <TickRow key={m.id} checked={chosen.has(m.id)} onChange={() => toggle(m.id)} icon={icon(m)} title={m.name} text={words}
      note={m.libraries?.length ? t('Lets you add: {kinds}', { kinds: list(m.libraries.map(kindName)) }) : undefined} />
  }

  const saveUpdates = async (c: UpdateChoice) => {
    setUpdates(c)
    await api.put('/settings', updateSettings(c)).catch(() => {})
  }

  const install = async (list: { id: string; name: string }[]) => {
    setRunning(true); setCheckErr(''); setLive(null)
    setGoing(list.map((m) => ({ id: m.id, name: m.name, state: 'waiting' })))
    // The list of what can be installed comes from the catalogue, and this is the moment to read it.
    let seen: StoreView
    try { seen = await checkAll() } catch (e: any) {
      setCheckErr(e.message); setRunning(false); setGoing(null)
      return
    }
    setView(seen)
    if (seen.checkError) { setCheckErr(seen.checkError); setRunning(false); setGoing(null); return }
    // Versions made for an older server are never installed without someone's say: one pop-up for
    // all of them, a choice for each, and Don't install unless they choose otherwise.
    const olderOnes = olderFromStore(seen.items, list.map((m) => m.id), seen.server)
    const yes = olderOnes.length ? await older.ask(olderOnes) : []
    for (const m of list) {
      if (stopped.current) return
      const it = seen.items.find((i) => i.id === m.id && i.official)
      const o = olderOnes.find((x) => x.module === m.id)
      const anyway = !!o && yes.includes(m.id)
      if (o && !anyway) {
        setGoing((g) => g!.map((x) => x.id === m.id ? { ...x, state: 'skipped', failure: { kind: 'older-server', title: t('Not installed'), message: notInstalledWords(o), try: [t('Add it later from the module store, once a version made for this server is published.')] } } : x))
        announce(t('{name} wasn’t installed', { name: m.name }))
        continue
      }
      if (it && !it.fit.ok && !ready(it) && !anyway) {
        setGoing((g) => g!.map((x) => x.id === m.id ? { ...x, state: 'failed', failure: { kind: 'other', title: t('{name} can’t be installed right now', { name: m.name }), message: tx(it.fit.why), try: [] } } : x))
        continue
      }
      setGoing((g) => g!.map((x) => x.id === m.id ? { ...x, state: 'working' } : x))
      announce(t('Installing {name}', { name: m.name }))
      const r = await installAndWait(m.id, m.name, setLive, { confirmOlder: anyway, askOlder: older.ask })
      setGoing((g) => g!.map((x) => x.id === m.id ? { ...x, state: r.ok ? 'done' : r.declined ? 'skipped' : 'failed', failure: r.failure } : x))
      announce(r.ok ? t('{name} is ready', { name: m.name }) : t('{name} didn’t install', { name: m.name }))
    }
    if (stopped.current) return
    await refresh()
    await load()
    setRunning(false); setLive(null)
    setChosen(new Set())
  }

  if (failed) return <LoadError what={t('The list of add-ons')} onRetry={load} />
  if (!view) return <Spinner />

  if (going) {
    const bad = going.filter((g) => g.state === 'failed' || g.state === 'skipped')
    const allDone = !running
    return (
      <section>
        <p className="oobe-sub" role="status">{allDone ? (bad.length ? t('Some add-ons didn’t install. Everything else works.') : t('Your add-ons are ready.')) : t('Installing. It takes as long as your connection does.')}</p>
        <ul className="su-going" aria-label={t('Modules being installed')}>
          {going.map((g) => {
            const item = live?.items.find((i) => i.id === g.id)
            const status = view.items.find((i) => i.id === g.id)?.status
            const phase = item?.phase || (status && isBusy(status) ? status.phase : '')
            return (
              <li key={g.id} className={g.state}>
                <span className="su-going-mark" aria-hidden="true">{g.state === 'done' ? <IconCheck size={18} /> : g.state === 'failed' || g.state === 'skipped' ? <IconInfo size={18} /> : g.state === 'working' ? <span className="spinner su-spin" /> : null}</span>
                <span className="su-going-words">
                  <b>{g.name}</b>
                  <span className="muted">{{ waiting: t('Waiting'), working: phase ? (item?.total ? t('{phase}, {done} of {total}', { phase: tx(PHASE[phase]) || t('Working'), done: mb(item.done), total: mb(item.total) }) : tx(PHASE[phase]) || t('Working')) : t('Starting'), done: t('Ready'), failed: t('Didn’t install'), skipped: t('Not installed') }[g.state]}</span>
                  {g.state === 'working' && <Progress label={t('{name}: installing', { name: g.name })} value={item?.total ? item.done : undefined} max={item?.total || undefined} />}
                  {g.failure && <span className="su-going-why"><b>{g.failure.title}</b> {g.failure.message} {g.failure.try?.join(' ')}</span>}
                </span>
              </li>
            )
          })}
        </ul>
        {allDone && (
          <div className="su-gap su-row">
            {bad.length > 0 && <Button onClick={() => install(bad)}>{t('Try again')}</Button>}
            <Button onClick={() => setGoing(null)}>{t('Add more')}</Button>
          </div>
        )}
        {older.dialog}
      </section>
    )
  }

  return (
    <section>
      {checkErr && (
        <Notice tone="warn" title={t('The list of modules couldn’t be read')}>
          <p className="su-para">{checkErr}</p>
          <p className="su-para">{t('Check this computer’s internet connection and try again. You can also carry on without them: movies, TV shows and music still work, and you can add modules later in Settings, Modules.')}</p>
        </Notice>
      )}
      <div className="oobe-mod-groups">
        {shelves.map((g) => (
          <section key={g.id} aria-label={g.title} data-nav-group>
            <h3 className="oobe-type-head">{g.title}</h3>
            <div className="su-mods">
              {g.modules.map(row)}
            </div>
          </section>
        ))}
      </div>
      {advanced.length > 0 && (
        <details className="su-more" open={advanced.some((m) => chosen.has(m.id) || ready(m))}>
          <summary><IconDown size={20} /> {t('If you have a gaming PC of your own')}</summary>
          <div className="su-mods">{advanced.map(row)}</div>
        </details>
      )}
      <h3 className="oobe-sub">{t('When an add-on has a new version')}</h3>
      <ChoiceCards small label={t('When an add-on has a new version')} value={updates} onChange={saveUpdates} cards={[
        { value: 'ask', title: t('Tell me, and I’ll choose'), text: t('Looks online twice a day and shows what’s new. Nothing changes until you say so.') },
        { value: 'auto', title: t('Install them by themselves'), text: t('Keeps your modules up to date without asking. A version that doesn’t work is taken back out by itself.') },
        { value: 'never', title: t('Only when I check'), text: t('Doesn’t look online until you press Check now in Settings, Modules.') },
      ]} />
      <p className="muted oobe-hint su-gap">{t('Add-ons come signed from the official list on GitHub, which sees only your internet address.')}</p>
      {picks.length > 0 && <div className="su-gap"><Button variant="primary" onClick={() => install(picks.map((m) => ({ id: m.id, name: m.name })))}>{tn('Install {count} add-on', 'Install {count} add-ons', picks.length)}</Button></div>}
    </section>
  )
}
