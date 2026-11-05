import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { api, ApiError } from '../../api'
import { IconCheck, IconRefresh } from '../../icons'
import { Button, LinkButton, Notice, Progress, Spinner } from '../../ui'
import { ChoiceCards, type Card } from '../setup/parts'
import TunerField from './TunerField'
import { forgetLineup, type Driver, type EPGSource, type Found, type ScanState, type Tuner, type TunerChannel, type TunerCheck, type TunerProblem, type TunerTest } from './tvApi'
import '../setup/setup.css'
import './tvsettings.css'

// Add live TV: a step at a time, starting from what someone has rather than from driver names.
// Every kind can be added by hand. The server checks the details as it always does, through
// WittyTuner, and an encrypted channel is refused here as it is everywhere else: never decrypted.

type Kind = 'playlist' | 'stream' | 'network' | 'satip' | 'local' | 'backend' | 'other'
type Stage = 'what' | 'find' | 'details' | 'channels' | 'try' | 'guide' | 'done'

const KINDS: { kind: Kind; title: string; text: string; drivers: string[]; search?: boolean }[] = [
  { kind: 'playlist', title: 'A playlist from a provider', text: 'A list of channels in M3U or M3U8, at an address or in a file on the server.', drivers: ['iptv'] },
  { kind: 'stream', title: 'One stream address', text: 'A single channel at an address: HTTP, HLS, RTSP, RTP or UDP.', drivers: ['stream'] },
  { kind: 'network', title: 'A tuner box on my network', text: 'An aerial or cable box that streams to your home network.', drivers: ['network'], search: true },
  { kind: 'satip', title: 'A SAT>IP server', text: 'Satellite, aerial or cable from a SAT>IP box on your network.', drivers: ['satip'], search: true },
  { kind: 'local', title: 'A TV card or capture device', text: 'A tuner card, a USB stick or a capture card in the computer the server runs on.', drivers: ['dvb', 'v4l2', 'dshow', 'avfoundation'], search: true },
  { kind: 'backend', title: 'A TV server I already run', text: 'Another TV program on your network that offers its channels over an open protocol.', drivers: ['backend'] },
  { kind: 'other', title: 'Something else', text: 'Every kind of source WittyTuner knows, by name.', drivers: [] },
]

const STATUS: Record<Driver['status'], string> = {
  tested: '',
  mock: 'This kind was built from the published protocol and tried against stand-in servers, not yet on real hardware.',
  untested: 'This kind was built from the published documentation and hasn’t been tried on real hardware yet, so please report what you find.',
  planned: '',
}

// The fields shown first. The rest wait under More settings.
const FIRST = ['guide', 'source', 'system']

const usable = (d: Driver) => d.available && !d.planned
const kindOf = (driver: string): Kind => KINDS.find((k) => k.drivers.includes(driver))?.kind || 'other'
const plural = (n: number, one: string, many = one + 's') => `${n} ${n === 1 ? one : many}`

type Props = {
  // In first-time setup: what to do when it's finished or skipped, instead of links to elsewhere,
  // and how to go back to the step before.
  onFinish?: () => void
  onLeave?: () => void
}

export default function AddLiveTv({ onFinish, onLeave }: Props) {
  const found0 = (useLocation().state as { found?: Found } | null)?.found
  const [drivers, setDrivers] = useState<Driver[] | null>(null)
  const [missing, setMissing] = useState(false)
  const [loadErr, setLoadErr] = useState('')
  const [stage, setStage] = useState<Stage>(found0 ? 'details' : 'what')
  const [kind, setKind] = useState<Kind>(found0 ? kindOf(found0.driver) : 'playlist')
  const [driverId, setDriverId] = useState(found0?.driver || '')
  const [name, setName] = useState(found0?.name || '')
  const [opts, setOpts] = useState<Record<string, string>>(found0?.options || {})
  const [tuner, setTuner] = useState<Tuner | null>(null)
  const head = useRef<HTMLHeadingElement>(null)

  const load = () => {
    setLoadErr(''); setMissing(false)
    api.get<Driver[]>('/tuners/drivers').then(setDrivers).catch((e) => { if (e instanceof ApiError && e.status === 424) setMissing(true); else setLoadErr(e.message) })
  }
  useEffect(load, [])
  const first = useRef(true)
  useEffect(() => {
    if (first.current) { first.current = false; return }
    head.current?.focus()
  }, [stage])

  if (missing) return <NeedsTuner onFinish={onFinish} />
  if (loadErr) return <Notice tone="danger" title="The kinds of tuner couldn’t be read" action={<Button onClick={load}>Try again</Button>}>{loadErr}</Notice>
  if (!drivers) return <Spinner />

  const def = KINDS.find((k) => k.kind === kind)!
  const choices = kind === 'other' ? drivers.filter(usable) : drivers.filter((d) => def.drivers.includes(d.id) && usable(d))
  const driver = drivers.find((d) => d.id === driverId) || (choices.length === 1 ? choices[0] : undefined)
  const search = !!def.search
  const steps: { id: Stage; label: string }[] = [
    { id: 'what', label: 'What you have' },
    ...(search ? [{ id: 'find' as Stage, label: 'Find it' }] : []),
    { id: 'details', label: 'Details' },
    { id: 'channels', label: 'Channels' },
    { id: 'try', label: 'Try a channel' },
    { id: 'guide', label: 'Guide data' },
    { id: 'done', label: 'Done' },
  ]
  const at = Math.max(0, steps.findIndex((s) => s.id === stage))
  const restart = () => { setStage('what'); setDriverId(''); setName(''); setOpts({}); setTuner(null) }
  const heading = (text: string) => <h2 ref={head} tabIndex={-1} className="alt-h">{text}</h2>

  return (
    <div className="alt">
      <ol className="alt-steps" aria-label="Steps">
        {steps.map((s, i) => (
          <li key={s.id} className={i < at ? 'done' : i === at ? 'on' : ''} aria-current={i === at ? 'step' : undefined}>
            <span className="alt-step-n" aria-hidden="true">{i < at ? <IconCheck size={14} /> : i + 1}</span>
            <span>{s.label}{i < at && <span className="sr-only"> (done)</span>}</span>
          </li>
        ))}
      </ol>

      {stage === 'what' && (
        <What drivers={drivers} kind={kind} setKind={setKind} heading={heading('What do you have?')}
          onNext={(k) => { setKind(k); setDriverId(''); setOpts({}); setName(''); setStage(KINDS.find((x) => x.kind === k)?.search ? 'find' : 'details') }} onBack={onLeave} onSkip={onFinish} />
      )}
      {stage === 'find' && (
        <Find heading={heading('Look for it')} kind={def} drivers={drivers}
          onPick={(f) => { setDriverId(f.driver); setOpts(f.options); setName(f.name); setStage('details') }}
          onByHand={() => setStage('details')} onBack={() => setStage('what')} />
      )}
      {stage === 'details' && (
        <Details heading={heading(tuner ? `Change ${tuner.name}` : 'Fill in the details')} kind={kind} choices={choices} driver={driver} setDriver={(id) => { setDriverId(id); setOpts({}) }}
          name={name} setName={setName} opts={opts} setOpts={setOpts} tuner={tuner}
          onSaved={(t) => { setTuner(t); forgetLineup(); setStage('channels') }} onBack={() => setStage(tuner ? 'channels' : search ? 'find' : 'what')} />
      )}
      {stage === 'channels' && tuner && driver && (
        <Channels heading={heading('Channels')} tuner={tuner} driver={driver} setTuner={setTuner}
          onNext={() => setStage('try')} onChange={() => setStage('details')} />
      )}
      {stage === 'try' && tuner && (
        <TryOne heading={heading('Try a channel')} tuner={tuner} onNext={() => setStage('guide')} onBack={() => setStage('channels')} />
      )}
      {stage === 'guide' && tuner && driver && (
        <Guide heading={heading('Guide data')} tuner={tuner} driver={driver} onNext={() => setStage('done')} onBack={() => setStage('try')} />
      )}
      {stage === 'done' && tuner && (
        <Done heading={heading(`${tuner.name} is ready`)} tuner={tuner} onAnother={restart} onFinish={onFinish} />
      )}
    </div>
  )
}

// Without WittyTuner there's nothing to add, and the page says so instead of showing nothing.
export function NeedsTuner({ onFinish }: { onFinish?: () => void }) {
  return (
    <div className="alt"><Notice tone="info" title="Live TV needs the WittyTuner module"
      action={<span className="alt-row">
        <LinkButton variant="primary" to="/settings/modules/store/wittytuner">Get WittyTuner</LinkButton>
        {onFinish && <Button onClick={onFinish}>Skip for now</Button>}
      </span>}>
      <p>WittyTuner is what talks to tuners, playlists and capture cards. It isn’t on this server yet. Installing it takes a moment and needs no restart. Channels made from your own library work without it.</p>
    </Notice></div>
  )
}

function Actions({ back, children }: { back?: () => void; children: ReactNode }) {
  return (
    <div className="alt-actions">
      {back ? <Button variant="ghost" onClick={back}>Back</Button> : <span />}
      <span className="alt-row">{children}</span>
    </div>
  )
}

function ProblemNotice({ p, title }: { p: TunerProblem; title?: string }) {
  const tone = p.kind === 'encrypted' || p.kind === 'busy' || p.kind === 'empty' ? 'warn' : 'danger'
  return (
    <Notice tone={tone} title={title}>
      <p>{p.message}</p>
      {p.kind === 'encrypted'
        ? <p>That isn’t a fault in your setup: encrypted channels are listed, marked as encrypted, and never decrypted. <Link to="/tv/encrypted">What can I do?</Link></p>
        : p.hint && <p>{p.hint}</p>}
    </Notice>
  )
}

// ------------------------------------------------------------------ what you have

function What({ drivers, kind, setKind, heading, onNext, onBack, onSkip }: { drivers: Driver[]; kind: Kind; setKind: (k: Kind) => void; heading: ReactNode; onNext: (k: Kind) => void; onBack?: () => void; onSkip?: () => void }) {
  const have = KINDS.filter((k) => k.kind === 'other' ? drivers.some(usable) : drivers.some((d) => k.drivers.includes(d.id) && usable(d)))
  const away = KINDS.filter((k) => k.kind !== 'other' && !have.includes(k))
  // Why a kind someone may be looking for isn't offered, such as one not built yet.
  const notes = drivers.filter((d) => !usable(d) && d.note)
  const cards: Card<Kind>[] = have.map((k) => ({ value: k.kind, title: k.title, text: k.text }))
  return (
    <section>
      {heading}
      <p className="alt-lead">Choose what you’re adding. You can add as many as you like, one at a time. No channel lists or stream addresses come with the server: bring your own.</p>
      <ChoiceCards label="What you have" value={have.some((k) => k.kind === kind) ? kind : have[0]?.kind} onChange={setKind} cards={cards} />
      {away.length > 0 && <p className="tvs-dim alt-gap">Not on this server: {away.map((k) => k.title.toLowerCase()).join(', ')}. {away.some((k) => k.kind === 'local') && 'Cards and capture devices need drivers for the system the server runs on.'}</p>}
      {notes.length > 0 && <ul className="tvs-dim alt-gap">{notes.map((d) => <li key={d.id}><b>{d.name}:</b> {d.note}</li>)}</ul>}
      <Actions back={onBack}>
        {onSkip && <Button variant="ghost" onClick={onSkip}>Skip for now</Button>}
        <Button variant="primary" onClick={() => onNext(have.some((k) => k.kind === kind) ? kind : have[0].kind)}>Next</Button>
      </Actions>
    </section>
  )
}

// ----------------------------------------------------------------------- find it

function Find({ heading, kind, drivers, onPick, onByHand, onBack }: { heading: ReactNode; kind: typeof KINDS[number]; drivers: Driver[]; onPick: (f: Found) => void; onByHand: () => void; onBack: () => void }) {
  const [found, setFound] = useState<Found[] | null>(null)
  const [err, setErr] = useState('')
  const look = () => {
    setFound(null); setErr('')
    api.get<Found[]>('/tuners/discover').then((l) => setFound(l.filter((f) => kind.drivers.includes(f.driver)))).catch((e) => setErr(e.message))
  }
  useEffect(look, []) // eslint-disable-line react-hooks/exhaustive-deps
  const where = kind.kind === 'local' ? 'in the computer the server runs on' : 'on your network'
  return (
    <section>
      {heading}
      <p className="alt-lead">Looking {where}. This takes a few seconds. If yours isn’t found, you can enter its details by hand.</p>
      {!found && !err && <div className="alt-row" role="status"><Spinner /> <span>Looking…</span></div>}
      {err && <Notice tone="danger" title="The search didn’t work">{err}</Notice>}
      {found && found.length === 0 && <Notice tone="info" title="Nothing new was found">Some devices don’t answer a search, or are on another part of the network, and ones already added aren’t shown again. Enter the details by hand instead: you’ll need its address{kind.kind === 'local' ? ', or the name the system gives the device' : ''}.</Notice>}
      {found && found.length > 0 && (
        <ul className="tvs-list" aria-label="Found">
          {found.map((f, i) => (
            <li key={i} className="tvs-item">
              <div className="tvs-item-text"><b>{f.name}</b><span>{f.detail || drivers.find((d) => d.id === f.driver)?.name}</span></div>
              <Button variant="primary" size="sm" onClick={() => onPick(f)} aria-label={`Use ${f.name}`}>Use this one</Button>
            </li>
          ))}
        </ul>
      )}
      <Actions back={onBack}>
        {found !== null || err ? <Button icon={<IconRefresh />} onClick={look}>Look again</Button> : null}
        <Button variant={found && found.length > 0 ? undefined : 'primary'} onClick={onByHand}>Enter the details by hand</Button>
      </Actions>
    </section>
  )
}

// ----------------------------------------------------------------------- details

function Details({ heading, kind, choices, driver, setDriver, name, setName, opts, setOpts, tuner, onSaved, onBack }: {
  heading: ReactNode; kind: Kind; choices: Driver[]; driver?: Driver; setDriver: (id: string) => void
  name: string; setName: (v: string) => void; opts: Record<string, string>; setOpts: (o: Record<string, string>) => void
  tuner: Tuner | null; onSaved: (t: Tuner) => void; onBack: () => void
}) {
  const [check, setCheck] = useState<TunerCheck | null>(null)
  const [checking, setChecking] = useState(false)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')
  useEffect(() => { setCheck(null); setErr('') }, [driver?.id])
  const set = (k: string, v: string) => { setOpts({ ...opts, [k]: v }); setCheck(null) }
  const body = () => ({ driver: driver!.id, name: name.trim() || driver!.name, options: opts })
  const missing = driver?.fields.filter((f) => f.required && !(opts[f.key] || f.default)) || []
  const runCheck = async () => {
    setChecking(true); setErr(''); setCheck(null)
    try { setCheck(await api.post<TunerCheck>('/tuners/check', { ...body(), id: tuner?.id || 0 })) } catch (e: any) { setErr(e.message) } finally { setChecking(false) }
  }
  const save = async () => {
    setSaving(true); setErr('')
    try {
      const b = body()
      onSaved(tuner ? await api.put<Tuner>(`/tuners/${tuner.id}`, { name: b.name, options: b.options, enabled: tuner.enabled, maxStreams: tuner.maxStreams }) : await api.post<Tuner>('/tuners', b))
    } catch (e: any) { setErr(e.message) } finally { setSaving(false) }
  }
  const first = driver?.fields.filter((f) => f.required || FIRST.includes(f.key)) || []
  const rest = driver?.fields.filter((f) => !first.includes(f)) || []
  return (
    <section>
      {heading}
      {choices.length > 1 && (
        <div className="alt-gap">
          <ChoiceCards small label="Which kind" value={driver?.id || ''} onChange={setDriver} cards={choices.map((d) => ({ value: d.id, title: d.name, text: d.description }))} />
        </div>
      )}
      {!driver && choices.length === 0 && <Notice tone="warn">Nothing of this kind can be added on this server.</Notice>}
      {driver && (
        <form className="form alt-form" onSubmit={(e) => { e.preventDefault(); void save() }}>
          <p className="alt-lead">{driver.description}</p>
          {STATUS[driver.status] && <p className="tvs-note">{STATUS[driver.status]}</p>}
          <label htmlFor="alt-name">Name, as the guide shows it
            <input id="alt-name" className="input" value={name} onChange={(e) => setName(e.target.value)} maxLength={200} placeholder={driver.name} />
          </label>
          {first.map((f) => <TunerField key={f.key} f={f} value={opts[f.key] || ''} onChange={(v) => set(f.key, v)} />)}
          {rest.length > 0 && (
            <details className="tvs-details" open={rest.some((f) => opts[f.key])}>
              <summary>More settings</summary>
              <div className="form alt-form">{rest.map((f) => <TunerField key={f.key} f={f} value={opts[f.key] || ''} onChange={(v) => set(f.key, v)} />)}</div>
            </details>
          )}
          {kind === 'playlist' && <p className="tvs-dim">A file on the server goes by its full path, such as <code>/srv/tv/channels.m3u</code> or <code>D:\TV\channels.m3u</code>. Keep it in a library folder or the server’s data folder.</p>}
          <div aria-live="polite">
            {check?.ok && (
              <Notice tone="ok" title="It answered">
                <p>{check.result.detail ? `${check.result.detail}. ` : ''}{check.result.channels > 0 ? `${plural(check.result.channels, 'channel')} listed` : driver.scans ? 'No channels listed yet: this kind finds them by scanning, which is the next step' : 'No channels listed'}{check.result.encrypted > 0 ? `, ${check.result.encrypted} of them marked as encrypted` : ''}.{check.result.maxStreams ? ` It can play ${plural(check.result.maxStreams, 'channel')} at once.` : ''}</p>
                {check.result.sample && check.result.sample.length > 0 && <p className="tvs-dim">Such as {check.result.sample.join(', ')}.</p>}
              </Notice>
            )}
            {check && !check.ok && <ProblemNotice p={check.problem} title="That didn’t work yet" />}
            {err && <Notice tone="danger" title={tuner ? 'It couldn’t be saved' : 'It couldn’t be added'}>{err}</Notice>}
          </div>
          {missing.length > 0 && <p className="tvs-dim">Still needed: {missing.map((f) => f.label).join(', ')}.</p>}
          <Actions back={onBack}>
            <Button busy={checking} disabled={missing.length > 0 || saving} onClick={() => void runCheck()}>{checking ? 'Checking…' : 'Check it'}</Button>
            <Button type="submit" variant="primary" busy={saving} disabled={missing.length > 0 || checking}>{tuner ? 'Save and go on' : 'Add it'}</Button>
          </Actions>
        </form>
      )}
      {!driver && <Actions back={onBack}><span /></Actions>}
    </section>
  )
}

// ---------------------------------------------------------------------- channels

function Counts({ clear, encrypted, unknown }: { clear: number; encrypted: number; unknown: number }) {
  return (
    <ul className="alt-counts">
      <li><b>{clear}</b> clear</li>
      <li><b>{encrypted}</b> encrypted</li>
      {unknown > 0 && <li><b>{unknown}</b> not checked yet</li>}
    </ul>
  )
}

function Channels({ heading, tuner, driver, setTuner, onNext, onChange }: { heading: ReactNode; tuner: Tuner; driver: Driver; setTuner: (t: Tuner) => void; onNext: () => void; onChange: () => void }) {
  const [scan, setScan] = useState<ScanState | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const reload = async () => {
    const t = (await api.get<Tuner[]>('/tuners')).find((x) => x.id === tuner.id)
    if (t) setTuner(t)
  }
  useEffect(() => {
    if (!scan?.running) return
    const t = setTimeout(() => api.get<ScanState>(`/tuners/${tuner.id}/scan`).then((s) => { setScan(s); if (!s.running) { forgetLineup(); void reload() } }).catch(() => {}), 1500)
    return () => clearTimeout(t)
  }, [scan]) // eslint-disable-line react-hooks/exhaustive-deps
  const startScan = async () => {
    setErr('')
    try { setScan(await api.post<ScanState>(`/tuners/${tuner.id}/scan`, { source: tuner.options.source || '' })) } catch (e: any) { setErr(e.message) }
  }
  const readAgain = async () => {
    setBusy(true); setErr('')
    try { setTuner(await api.post<Tuner>(`/tuners/${tuner.id}/refresh`)); forgetLineup() } catch (e: any) { setErr(e.message); void reload() } finally { setBusy(false) }
  }
  const unknown = Math.max(0, tuner.channels - tuner.clear - tuner.encrypted)
  const scanning = !!scan?.running
  return (
    <section>
      {heading}
      <p className="alt-lead">{tuner.name} is saved. {driver.scans ? 'This kind finds its channels by scanning: it tunes through every frequency, which can take several minutes. You can leave this page while it runs.' : 'Its channel list was read when it was saved.'}</p>
      {scanning && (
        <div className="alt-gap">
          <Progress value={scan!.percent} label="Scan progress" showValue />
          <p role="status" className="tvs-dim">{plural(scan!.found, 'channel')} found so far{scan!.message ? `: ${scan!.message}` : ''}.</p>
        </div>
      )}
      <div aria-live="polite">
        {scan?.error && <Notice tone="danger" title="The scan stopped">{scan.error}</Notice>}
        {err && <Notice tone="danger" title="That didn’t work">{err}</Notice>}
        {!scanning && tuner.lastError && !err && (
          <Notice tone="danger" title="The channel list couldn’t be read">
            <p>{tuner.lastError}</p>
            <p>The source is saved anyway, in case it’s only switched off. Check its details, or read the list again once it’s on.</p>
          </Notice>
        )}
        {!scanning && tuner.channels > 0 && (
          <Notice tone="ok" title={`${plural(tuner.channels, 'channel')}${scan && !scan.error ? ' after the scan' : ''}`}>
            <Counts clear={tuner.clear} encrypted={tuner.encrypted} unknown={unknown} />
            {tuner.encrypted > 0 && <p>Encrypted channels are shown in the guide, marked as encrypted, and never decrypted: they can’t be played or recorded here. That isn’t a fault in your setup. <Link to="/tv/encrypted">What can I do?</Link></p>}
            {unknown > 0 && <p className="tvs-dim">This source doesn’t say which are encrypted, so each is checked the first time it’s tuned.</p>}
          </Notice>
        )}
        {!scanning && tuner.channels === 0 && !tuner.lastError && <Notice tone="info">{driver.scans ? 'No channels yet. Scan to find them.' : 'The list was read, but it has no channels in it.'}</Notice>}
      </div>
      <Actions back={onChange}>
        {driver.scans && <Button variant={tuner.channels === 0 ? 'primary' : undefined} busy={scanning} onClick={() => void startScan()}>{scanning ? 'Scanning…' : tuner.channels > 0 ? 'Scan again' : 'Scan for channels'}</Button>}
        <Button icon={<IconRefresh />} busy={busy} disabled={scanning} onClick={() => void readAgain()}>Read the list again</Button>
        <Button variant={tuner.channels > 0 ? 'primary' : undefined} disabled={scanning} onClick={onNext}>{tuner.channels > 0 ? 'Next' : 'Skip'}</Button>
      </Actions>
    </section>
  )
}

// ---------------------------------------------------------------- try a channel

function TryOne({ heading, tuner, onNext, onBack }: { heading: ReactNode; tuner: Tuner; onNext: () => void; onBack: () => void }) {
  const [list, setList] = useState<TunerChannel[] | null>(null)
  const [pick, setPick] = useState('')
  const [busy, setBusy] = useState(false)
  const [res, setRes] = useState<TunerTest | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<TunerChannel[]>(`/tuners/${tuner.id}/channels`).then(setList).catch((e) => setErr(e.message)) }, [tuner.id])
  const open = (list || []).filter((c) => c.security !== 'encrypted' && c.enabled).slice(0, 500)
  const run = async () => {
    setBusy(true); setErr(''); setRes(null)
    try { setRes(await api.post<TunerTest>(`/tuners/${tuner.id}/test`, { channel: Number(pick) || 0 })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const name = res?.channel ? `${res.channel.number ? res.channel.number + ' ' : ''}${res.channel.name}` : ''
  return (
    <section>
      {heading}
      <p className="alt-lead">This tunes one channel for a few seconds, as watching it would, to show that it plays. Nothing is recorded, and it uses one of the device’s tuners while it runs.</p>
      {list && open.length > 0 && (
        <label htmlFor="alt-pick" className="alt-field">Channel
          <select id="alt-pick" className="select" value={pick} onChange={(e) => { setPick(e.target.value); setRes(null) }}>
            <option value="">Let it choose</option>
            {open.map((c) => <option key={c.id} value={c.id}>{c.number ? `${c.number} ` : ''}{c.name}</option>)}
          </select>
        </label>
      )}
      <div aria-live="polite">
        {busy && <div className="alt-row" role="status"><Spinner /> <span>Tuning in. This can take up to a minute…</span></div>}
        {res?.ok && (
          <Notice tone="ok" title={`${name} played`}>
            <p>{[res.video && `Picture: ${res.video}`, res.audio && `Sound: ${res.audio}`].filter(Boolean).join('. ')}{res.video || res.audio ? '. ' : ''}It started in {res.seconds.toFixed(1)} seconds.</p>
            {res.clear && <p>The server watched the stream and it isn’t encrypted.</p>}
          </Notice>
        )}
        {res && !res.ok && res.problem && <ProblemNotice p={res.problem} title={name ? `${name} didn’t play` : 'Nothing could be tried'} />}
        {err && <Notice tone="danger" title="The test couldn’t run">{err}</Notice>}
      </div>
      <Actions back={onBack}>
        <Button variant={res?.ok ? undefined : 'primary'} busy={busy} disabled={list?.length === 0} onClick={() => void run()}>{res ? 'Try again' : 'Tune in for a few seconds'}</Button>
        <Button variant={res?.ok ? 'primary' : 'ghost'} disabled={busy} onClick={onNext}>{res?.ok ? 'Next' : 'Skip'}</Button>
      </Actions>
    </section>
  )
}

// -------------------------------------------------------------------- guide data

const OVER_THE_AIR = ['network', 'satip', 'dvb']

function Guide({ heading, tuner, driver, onNext, onBack }: { heading: ReactNode; tuner: Tuner; driver: Driver; onNext: () => void; onBack: () => void }) {
  const [list, setList] = useState<EPGSource[] | null>(null)
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [added, setAdded] = useState<EPGSource | null>(null)
  useEffect(() => { api.get<EPGSource[]>('/epg/sources').then(setList).catch((e) => setErr(e.message)) }, [])
  const own = list?.filter((s) => s.tunerId === tuner.id) || []
  const add = async () => {
    setBusy(true); setErr('')
    try { setAdded(await api.post<EPGSource>('/epg/sources', { url })); setUrl('') } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <section>
      {heading}
      {!list && !err && <Spinner />}
      {own.length > 0 && <Notice tone="ok" title="This source brings its own guide">It’s read by itself, and kept up to date: {own.map((s) => s.name).join(', ')}.</Notice>}
      {list && own.length === 0 && (
        <>
          <p className="alt-lead">The guide shows what’s on from listings in XMLTV format. {driver.id === 'iptv' ? 'This playlist names none of its own. Many providers give a guide address beside the playlist’s.' : 'This source has none of its own.'}{OVER_THE_AIR.includes(driver.id) ? ' Broadcasts also carry a few hours of listings, which are read while a channel is on.' : ''}</p>
          <form className="tvs-toolbar" onSubmit={(e) => { e.preventDefault(); void add() }}>
            <input className="input" aria-label="XMLTV address, or a file on the server" placeholder="XMLTV address, or a file on the server" value={url} onChange={(e) => setUrl(e.target.value)} />
            <Button type="submit" busy={busy} disabled={!url.trim()}>Add guide data</Button>
          </form>
        </>
      )}
      <div aria-live="polite">
        {added && <Notice tone="ok" title="Guide data added">It’s being read now. Channels are matched to it by their guide id, then by name.</Notice>}
        {err && <Notice tone="danger" title="That didn’t work">{err}</Notice>}
      </div>
      <Actions back={onBack}>
        <Button variant="primary" onClick={onNext}>{own.length > 0 || added ? 'Next' : 'Skip'}</Button>
      </Actions>
    </section>
  )
}

// -------------------------------------------------------------------------- done

function Done({ heading, tuner, onAnother, onFinish }: { heading: ReactNode; tuner: Tuner; onAnother: () => void; onFinish?: () => void }) {
  return (
    <section>
      {heading}
      <p className="alt-lead">Its channels are in the guide beside any you made from your library. You can hide channels, renumber them and choose their logos on its channel list.</p>
      <Actions>
        <Button onClick={onAnother}>Add another</Button>
        {onFinish
          ? <Button variant="primary" onClick={onFinish}>Continue</Button>
          : <>
            <LinkButton to={`/settings/channels/tuner/${tuner.id}`}>Its channel list</LinkButton>
            <LinkButton variant="primary" to="/tv">Open the guide</LinkButton>
          </>}
      </Actions>
    </section>
  )
}
