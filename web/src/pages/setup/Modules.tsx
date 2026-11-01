import { useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../../api'
import { announce } from '../../a11y'
import { IconCheck, IconDown, IconInfo, IconPlug, IconTv, libIcon } from '../../icons'
import { LIB_TYPES } from '../../components/AddLibrary'
import { Button, LoadError, Notice, Progress, Spinner } from '../../ui'
import { CATEGORIES, checkAll, isBusy, mb, PHASE, storeView, type Failure, type Run, type StoreItem, type StoreView } from '../settings/store/storeApi'
import { installAndWait } from './install'
import { ChoiceCards, LaterNote, StepActions, StepHead, TickRow, type StepProps } from './parts'
import { ADVANCED, offerable, PLAIN_WORDS, RECOMMENDED, updateChoiceFrom, updateSettings, type UpdateChoice } from './plan'

// What this server is for. Modules aren't part of the server: choosing one downloads it, signed,
// from the official catalogue, through the store's own install. Nothing's fetched until someone
// presses the button that says so, and nothing depends on it working.

const kindName = (t: string) => LIB_TYPES.find((x) => x.type === t)?.name || t
const icon = (m: StoreItem) => m.libraries?.length ? libIcon(m.libraries[0]) : m.group === 'tv' ? <IconTv /> : <IconPlug />
const ready = (m: StoreItem) => !!m.status?.installed

type Going = { id: string; name: string; state: 'waiting' | 'working' | 'done' | 'failed'; failure?: Failure }

export function Modules({ info, refresh, next, back }: StepProps) {
  const [view, setView] = useState<StoreView | null>(null)
  const [failed, setFailed] = useState(false)
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  const [updates, setUpdates] = useState<UpdateChoice>('ask')
  const [going, setGoing] = useState<Going[] | null>(null)
  const [running, setRunning] = useState(false)
  const [live, setLive] = useState<Run | null>(null)
  const [checkErr, setCheckErr] = useState('')
  const stopped = useRef(false)
  const done = useRef<HTMLButtonElement>(null)
  const load = () => { setFailed(false); return storeView().then((v) => { setView(v); return v }).catch(() => { setFailed(true); return null }) }
  useEffect(() => {
    void load()
    api.get<Record<string, string>>('/settings').then((s) => setUpdates(updateChoiceFrom(s))).catch(() => {})
    return () => { stopped.current = true }
  }, [])

  const offered = useMemo(() => offerable(view?.items || []), [view])
  // A module that can't run on this computer says so in place of being offered. Everything else
  // is only waiting for the list to be read, and that's what the button does.
  const cantRun = (m: StoreItem) => !ready(m) && !m.fit.ok && m.fit.kind !== 'no-version'
  const shelves = useMemo(() => {
    const usual = offered.filter((m) => !ADVANCED.includes(m.id) && !RECOMMENDED.includes(m.id))
    const known = CATEGORIES.map((c) => ({ ...c, modules: usual.filter((m) => m.group === c.id) }))
    const rest = usual.filter((m) => !CATEGORIES.some((c) => c.id === m.group))
    return [{ id: 'first', title: 'Recommended', modules: offered.filter((m) => RECOMMENDED.includes(m.id)) }, ...known, { id: 'other', title: 'Other', modules: rest }].filter((s) => s.modules.length)
  }, [offered])
  const advanced = offered.filter((m) => ADVANCED.includes(m.id))
  // What's recommended starts ticked, once, and can be unticked.
  const ticked = useRef(false)
  useEffect(() => {
    if (!view || ticked.current) return
    ticked.current = true
    setChosen(new Set(offered.filter((m) => RECOMMENDED.includes(m.id) && !ready(m) && !cantRun(m)).map((m) => m.id)))
  }, [view, offered]) // eslint-disable-line react-hooks/exhaustive-deps
  const toggle = (id: string) => setChosen((s) => { const n = new Set(s); if (n.has(id)) n.delete(id); else n.add(id); return n })
  const picks = offered.filter((m) => chosen.has(m.id) && !ready(m))
  const row = (m: StoreItem) => {
    const words = PLAIN_WORDS[m.id] || m.summary
    if (ready(m)) return <TickRow key={m.id} checked disabled icon={icon(m)} title={m.name} text={words} note="Already on this server" />
    if (cantRun(m)) return <TickRow key={m.id} checked={false} disabled icon={icon(m)} title={m.name} text={words} note={m.fit.why} />
    return <TickRow key={m.id} checked={chosen.has(m.id)} onChange={() => toggle(m.id)} icon={icon(m)} title={m.name} text={words}
      note={m.libraries?.length ? `Lets you add: ${m.libraries.map(kindName).join(', ')}` : undefined} />
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
    for (const m of list) {
      if (stopped.current) return
      const it = seen.items.find((i) => i.id === m.id && i.official)
      if (it && !it.fit.ok && !ready(it)) {
        setGoing((g) => g!.map((x) => x.id === m.id ? { ...x, state: 'failed', failure: { kind: 'other', title: `${m.name} can’t be installed right now`, message: it.fit.why || '', try: [] } } : x))
        continue
      }
      setGoing((g) => g!.map((x) => x.id === m.id ? { ...x, state: 'working' } : x))
      announce(`Installing ${m.name}`)
      const r = await installAndWait(m.id, m.name, setLive)
      setGoing((g) => g!.map((x) => x.id === m.id ? { ...x, state: r.ok ? 'done' : 'failed', failure: r.failure } : x))
      announce(r.ok ? `${m.name} is ready` : `${m.name} didn’t install`)
    }
    if (stopped.current) return
    await refresh()
    await load()
    setRunning(false); setLive(null)
    setChosen(new Set())
    requestAnimationFrame(() => done.current?.focus())
  }

  const lead = 'Movies, TV shows and music work straight away. Modules add everything else. Choose what you want and it downloads now. Whatever you don’t choose isn’t downloaded.'
  if (failed) return <section><StepHead title="What do you want on this server?" lead={lead} /><LoadError what="The list of modules" onRetry={load} /><StepActions back={back}><Button variant="primary" size="big" onClick={next}>Continue without modules</Button></StepActions></section>
  if (!view) return <section><StepHead title="What do you want on this server?" lead={lead} /><Spinner /></section>

  if (going) {
    const bad = going.filter((g) => g.state === 'failed')
    const allDone = !running
    return (
      <section>
        <StepHead title={allDone ? (bad.length ? 'Some modules didn’t install' : 'Your modules are ready') : 'Installing your modules'}
          lead={allDone ? (bad.length ? 'Everything else works. You can try again now, or carry on and add these later.' : 'Each one was checked against its signature before it was put to use.') : 'Each one is downloaded and checked before it’s used. You can leave this page open and wait: it takes as long as your connection does.'} />
        <ul className="su-going" aria-label="Modules being installed">
          {going.map((g) => {
            const item = live?.items.find((i) => i.id === g.id)
            const status = view.items.find((i) => i.id === g.id)?.status
            const phase = item?.phase || (status && isBusy(status) ? status.phase : '')
            return (
              <li key={g.id} className={g.state}>
                <span className="su-going-mark" aria-hidden="true">{g.state === 'done' ? <IconCheck size={18} /> : g.state === 'failed' ? <IconInfo size={18} /> : g.state === 'working' ? <span className="spinner su-spin" /> : null}</span>
                <span className="su-going-words">
                  <b>{g.name}</b>
                  <span className="muted">{{ waiting: 'Waiting', working: phase ? `${PHASE[phase] || 'Working'}${item?.total ? `, ${mb(item.done)} of ${mb(item.total)}` : ''}` : 'Starting', done: 'Ready', failed: 'Didn’t install' }[g.state]}</span>
                  {g.state === 'working' && <Progress label={`${g.name}: installing`} value={item?.total ? item.done : undefined} max={item?.total || undefined} />}
                  {g.failure && <span className="su-going-why"><b>{g.failure.title}</b> {g.failure.message} {g.failure.try?.join(' ')}</span>}
                </span>
              </li>
            )
          })}
        </ul>
        {allDone && (
          <StepActions>
            {bad.length > 0 && <Button onClick={() => install(bad)}>Try again</Button>}
            <Button ref={done} variant="primary" size="big" onClick={next}>{bad.length ? 'Continue without them' : 'Continue'}</Button>
          </StepActions>
        )}
      </section>
    )
  }

  return (
    <section>
      <StepHead title="What do you want on this server?" lead={lead} />
      {checkErr && (
        <Notice tone="warn" title="The list of modules couldn’t be read">
          <p className="su-para">{checkErr}</p>
          <p className="su-para">Check this computer’s internet connection and try again. You can also carry on without them: movies, TV shows and music still work, and you can add modules later in Settings, Modules.</p>
        </Notice>
      )}
      <div className="oobe-mod-groups">
        {shelves.map((g) => (
          <section key={g.id} aria-label={g.title} data-nav-group>
            <h2 className="oobe-type-head">{g.title}</h2>
            <div className="su-mods">
              {g.modules.map(row)}
            </div>
          </section>
        ))}
      </div>
      {advanced.length > 0 && (
        <details className="su-more" open={advanced.some((m) => chosen.has(m.id) || ready(m))}>
          <summary><IconDown size={20} /> If you have a gaming PC of your own</summary>
          <div className="su-mods">{advanced.map(row)}</div>
        </details>
      )}
      <h2 className="oobe-sub">When a module has a new version</h2>
      <ChoiceCards label="When a module has a new version" value={updates} onChange={saveUpdates} cards={[
        { value: 'ask', title: 'Tell me, and I’ll choose', text: 'Looks online twice a day and shows what’s new. Nothing changes until you say so.' },
        { value: 'auto', title: 'Install them by themselves', text: 'Keeps your modules up to date without asking. A version that doesn’t work is taken back out by itself.' },
        { value: 'never', title: 'Only when I check', text: 'Doesn’t look online until you press Check now in Settings, Modules.' },
      ]} />
      <p className="muted oobe-hint su-gap">Modules come from the official list on GitHub. Asking for it tells GitHub your internet address, as any download does, and nothing about you or your files. Every module is signed, and this server won’t use one that doesn’t check out.</p>
      <LaterNote info={info} />
      <StepActions back={back}>
        {picks.length > 0
          ? <Button variant="primary" size="big" onClick={() => install(picks.map((m) => ({ id: m.id, name: m.name })))}>Download and install {picks.length === 1 ? '1 module' : `${picks.length} modules`}</Button>
          : <Button variant="primary" size="big" onClick={next}>{offered.some(ready) ? 'Continue' : 'Continue without modules'}</Button>}
      </StepActions>
    </section>
  )
}
