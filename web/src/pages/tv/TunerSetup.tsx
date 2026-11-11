// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { announce } from '../../a11y'
import { IconRefresh } from '../../icons'
import { toast } from '../../state'
import { Button, Dialog, LinkButton } from '../../ui'
import { Field, STATUS } from './tunerFields'
import { forgetLineup, type Driver, type Found, type ScanState, type Tuner, type TunerChannel, type TunerCheck, type TunerProblem, type TunerTest } from './tvApi'

// Adding live TV, a step at a time: what someone has, in their words; its details,
// typed in or found on the network, and checked before saving if they like; then its
// channels read or scanned, one of them tried for a few seconds, and guide data where
// it has none. The drivers and what each asks for come from WittyTuner.

type Kind = { id: string; label: string; hint: string; drivers: string[] }

// Driver ids by what people call the thing they have. A driver this list doesn't
// know yet goes under Something else, so a new one in WittyTuner still shows.
const KINDS: Kind[] = [
  { id: 'playlist', label: 'A playlist from a TV provider', hint: 'An M3U or M3U8 list of channels: an address, or a file on this server.', drivers: ['iptv'] },
  { id: 'stream', label: 'One stream address', hint: 'A single channel at an HTTP, HLS, RTSP, RTP or UDP address.', drivers: ['stream'] },
  { id: 'network', label: 'A tuner box on your network', hint: 'An aerial or cable tuner that you reach by its address.', drivers: ['network'] },
  { id: 'satip', label: 'A SAT>IP box', hint: 'Satellite, aerial or cable through a SAT>IP server on your network.', drivers: ['satip'] },
  { id: 'card', label: 'A TV card or capture device in this computer', hint: 'A tuner card, a USB stick or a capture device plugged into the server.', drivers: ['dvb', 'bda', 'v4l2', 'dshow', 'avfoundation'] },
  { id: 'other', label: 'Something else', hint: 'Another TV program on this machine or network, or a command that prints a stream.', drivers: [] },
]
const known = new Set(KINDS.flatMap((k) => k.drivers))
const driversOf = (k: Kind, all: Driver[]) => all.filter((d) => (k.id === 'other' ? !known.has(d.id) : k.drivers.includes(d.id)))

type Step = 'kind' | 'details' | 'channels' | 'try' | 'guide' | 'done'
const STEPS: Step[] = ['kind', 'details', 'channels', 'try', 'guide']
const STEP_NAMES: Record<Step, string> = { kind: 'What you have', details: 'Its details', channels: 'Its channels', try: 'Try a channel', guide: 'Guide data', done: 'Done' }

const plural = (n: number, one: string) => `${n} ${n === 1 ? one : one + 's'}`

// Why a check or a try didn't work, as the server words it. An encrypted channel isn't a fault.
function Problem({ p, title }: { p: TunerProblem; title: string }) {
  return (
    <div className={p.kind === 'encrypted' || p.kind === 'busy' ? 'tvs-note' : 'tvw-problem'} role="alert">
      <b>{title}</b>
      <p>{p.message}</p>
      {p.kind === 'encrypted' ? <p><Link to="/tv/encrypted">What can I do about encrypted channels?</Link></p> : p.hint && <p>{p.hint}</p>}
    </div>
  )
}

// inSetup: opened from first-time setup, which carries on afterwards, so the last step doesn't lead away from it.
export default function TunerSetup({ drivers, start, onClose, onAdded, inSetup }: { drivers: Driver[]; start?: Found; onClose: () => void; onAdded: () => void; inSetup?: boolean }) {
  const first = start ? drivers.find((d) => d.id === start.driver) : undefined
  const [step, setStep] = useState<Step>(first ? 'details' : 'kind')
  const [kind, setKind] = useState<Kind | null>(first ? KINDS.find((k) => driversOf(k, drivers).includes(first)) || null : null)
  const [driver, setDriver] = useState<Driver | null>(first || null)
  const [name, setName] = useState(start?.name || '')
  const [opts, setOpts] = useState<Record<string, string>>(start?.options || {})
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [tuner, setTuner] = useState<Tuner | null>(null)
  const [scan, setScan] = useState<ScanState | null>(null)
  const [guideURL, setGuideURL] = useState('')
  const [found, setFound] = useState<Found[] | null>(null)
  const [searching, setSearching] = useState(false)
  const [check, setCheck] = useState<TunerCheck | null>(null)
  const [checking, setChecking] = useState(false)
  const [channels, setChannels] = useState<TunerChannel[] | null>(null)
  const [pick, setPick] = useState('')
  const [trying, setTrying] = useState(false)
  const [tried, setTried] = useState<TunerTest | null>(null)
  const heading = useRef<HTMLHeadingElement | null>(null)

  // Each step says where it is, and the next one starts at its heading.
  useEffect(() => { heading.current?.focus() }, [step])

  const choices = driversOf(kind || KINDS[0], drivers)
  const pickKind = (k: Kind) => {
    setKind(k)
    const ds = driversOf(k, drivers).filter((d) => d.available && !d.planned)
    setDriver(ds.length === 1 ? ds[0] : null)
    setOpts({}); setErr('')
  }

  // Devices of this kind on the network or this machine, for the details step.
  const search = async () => {
    setSearching(true)
    try {
      const all = await api.get<Found[]>('/tuners/discover')
      const mine = all.filter((f) => choices.some((d) => d.id === f.driver))
      setFound(mine)
      announce(mine.length === 0 ? 'Nothing found. Type its details instead.' : `${mine.length} found`)
    } catch {
      // A search that doesn't finish is the same as one that finds nothing: type it in.
      setFound([])
      announce("The search didn't finish. Type its details instead.")
    } finally { setSearching(false) }
  }
  const pickFound = (f: Found) => {
    const d = drivers.find((x) => x.id === f.driver)
    if (d) setDriver(d)
    setName(f.name); setOpts(f.options); setFound(null)
  }

  const missing = driver ? driver.fields.filter((f) => f.required && !(opts[f.key] || f.default || '').trim()).map((f) => f.label) : []
  // What a field shows by default is what's sent, as if it had been chosen.
  const body = (d: Driver) => ({ driver: d.id, name: name.trim() || d.name, options: { ...Object.fromEntries(d.fields.filter((f) => f.default).map((f) => [f.key, f.default!])), ...opts } })
  // What the details would give, asked of the source without saving anything.
  const runCheck = async () => {
    if (!driver) return
    setChecking(true); setErr(''); setCheck(null)
    try {
      const c = await api.post<TunerCheck>('/tuners/check', body(driver))
      setCheck(c)
      announce(c.ok ? `It answered: ${plural(c.result.channels, 'channel')}` : `That didn't work yet: ${c.problem.message}`)
    } catch (e: any) { setErr(e.message) } finally { setChecking(false) }
  }
  useEffect(() => { setCheck(null) }, [driver, opts])
  const save = async () => {
    if (!driver) return
    setBusy(true); setErr('')
    try {
      const t = await api.post<Tuner>('/tuners', body(driver))
      forgetLineup()
      setTuner(t)
      setStep('channels')
      onAdded()
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const readAgain = async () => {
    if (!tuner) return
    setBusy(true)
    try { setTuner(await api.post<Tuner>(`/tuners/${tuner.id}/refresh`)); forgetLineup() } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  // A typo in an address is fixed where it was typed: the tuner that was saved goes.
  const changeDetails = async () => {
    if (!tuner) return
    setBusy(true)
    try { await api.del(`/tuners/${tuner.id}`); forgetLineup(); setTuner(null); setScan(null); setStep('details'); onAdded() } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const startScan = async () => {
    if (!tuner) return
    try { setScan(await api.post<ScanState>(`/tuners/${tuner.id}/scan`, { source: opts.source || '' })) } catch (e: any) { toast(e.message) }
  }
  useEffect(() => {
    if (!scan?.running || !tuner) return
    const t = setTimeout(() => api.get<ScanState>(`/tuners/${tuner.id}/scan`).then((s) => {
      setScan(s)
      if (!s.running) {
        forgetLineup()
        announce(s.error || `Scan finished: ${s.clear} clear, ${s.encrypted} encrypted`)
        api.get<Tuner[]>('/tuners').then((l) => setTuner(l.find((x) => x.id === tuner.id) || tuner)).catch(() => {})
      }
    }).catch(() => {}), 1500)
    return () => clearTimeout(t)
  }, [scan, tuner])
  // One channel, tuned for a few seconds as watching it would. The server picks one if none is chosen.
  useEffect(() => {
    if (step !== 'try' || !tuner) return
    setTried(null)
    api.get<TunerChannel[]>(`/tuners/${tuner.id}/channels`).then(setChannels).catch(() => setChannels([]))
  }, [step, tuner?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const tryOne = async () => {
    if (!tuner) return
    setTrying(true); setTried(null)
    try {
      const r = await api.post<TunerTest>(`/tuners/${tuner.id}/test`, { channel: Number(pick) || 0 })
      setTried(r)
      announce(r.ok ? `${r.channel?.name} played` : r.problem?.message || "It didn't play")
    } catch (e: any) { toast(e.message) } finally { setTrying(false) }
  }
  const tryable = (channels || []).filter((c) => c.security !== 'encrypted' && c.enabled).slice(0, 500)
  const triedName = tried?.channel ? `${tried.channel.number ? tried.channel.number + ' ' : ''}${tried.channel.name}` : ''
  const addGuide = async () => {
    setBusy(true)
    try { await api.post('/epg/sources', { url: guideURL.trim() }); toast('Reading the guide…'); setStep('done') } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }

  // A playlist that names its own guide, or anything that isn't a playlist or stream, needs nothing more.
  // Only playlists and streams can be matched to XMLTV listings by channel.
  const ownGuide = !!(opts.guide || '').trim() || !['iptv', 'stream'].includes(driver?.id || '')
  // Arrow keys move between the kinds and choose, as in any group of radio buttons.
  const radioKeys = (e: React.KeyboardEvent<HTMLDivElement>) => {
    const by = ({ ArrowDown: 1, ArrowRight: 1, ArrowUp: -1, ArrowLeft: -1 } as Record<string, number>)[e.key]
    if (!by) return
    const all = [...e.currentTarget.querySelectorAll<HTMLButtonElement>('[role=radio]:not([disabled])')]
    const i = all.indexOf(document.activeElement as HTMLButtonElement)
    const next = all[(i + by + all.length) % all.length]
    if (!next) return
    e.preventDefault(); e.stopPropagation()
    next.focus(); next.click()
  }
  const at = STEPS.indexOf(step)
  const title = step === 'done' ? `${tuner?.name || 'Live TV'} is ready` : 'Add live TV'
  const back = () => setStep(STEPS[Math.max(0, at - 1)])

  const actions = (() => {
    switch (step) {
      case 'kind': return <><Button variant="ghost" onClick={onClose}>Cancel</Button><Button variant="primary" disabled={!kind} onClick={() => setStep('details')}>Next</Button></>
      case 'details': return <><Button variant="ghost" onClick={back}>Back</Button><Button busy={checking} disabled={!driver || missing.length > 0 || checking || busy} onClick={() => void runCheck()}>{checking ? 'Checking…' : 'Check it'}</Button><Button variant="primary" busy={busy} disabled={!driver || missing.length > 0 || busy || checking} onClick={() => void save()}>{busy ? 'Adding…' : 'Add and read its channels'}</Button></>
      case 'channels': return <Button variant="primary" disabled={!!scan?.running} onClick={() => setStep('try')}>Next</Button>
      case 'try': return <><Button variant="ghost" disabled={trying} onClick={() => setStep(ownGuide ? 'done' : 'guide')}>{tried?.ok ? 'Next' : 'Skip'}</Button><Button variant="primary" busy={trying} disabled={trying || channels === null} onClick={() => void tryOne()}>{trying ? 'Tuning in…' : tried ? 'Try again' : 'Tune in for a few seconds'}</Button></>
      case 'guide': return <><Button variant="ghost" onClick={() => setStep('done')}>Skip</Button><Button variant="primary" busy={busy} disabled={!guideURL.trim() || busy} onClick={() => void addGuide()}>Add guide data</Button></>
      case 'done': return inSetup
        ? <Button variant="primary" onClick={onClose}>Done</Button>
        : <><Button variant="ghost" onClick={onClose}>Close</Button>{tuner && <LinkButton to={`/settings/channels/tuner/${tuner.id}`} onClick={onClose}>See its channels</LinkButton>}<LinkButton variant="primary" to="/tv">Open the guide</LinkButton></>
    }
  })()

  return (
    <Dialog size="wide" className="tvw" title={title} onClose={onClose} actions={actions}>
      {step !== 'done' && (
        <ol className="tvw-steps" aria-label="Steps">
          {STEPS.map((s, i) => <li key={s} className={i === at ? 'on' : i < at ? 'past' : ''} aria-current={i === at ? 'step' : undefined}>{STEP_NAMES[s]}</li>)}
        </ol>
      )}
      <h3 className="tvw-h" ref={heading} tabIndex={-1}>{step === 'done' ? 'All set' : `Step ${at + 1} of ${STEPS.length}: ${STEP_NAMES[step]}`}</h3>

      {step === 'kind' && (
        <div className="tvw-kinds" role="radiogroup" aria-label="What do you have?" onKeyDown={radioKeys}>
          {KINDS.map((k) => {
            const ds = driversOf(k, drivers)
            const usable = ds.filter((d) => d.available && !d.planned)
            if (ds.length === 0) return null
            const on = kind?.id === k.id
            return (
              <button key={k.id} type="button" role="radio" aria-checked={on} disabled={usable.length === 0} className={'tvs-level tvw-kind' + (on ? ' on' : '')} onClick={() => pickKind(k)}>
                <b>{k.label}</b><span>{k.hint}</span>
                {usable.length === 0 && <span className="tvs-dim">{ds.find((d) => d.note)?.note || 'Not on this kind of computer'}</span>}
              </button>
            )
          })}
        </div>
      )}

      {step === 'details' && (
        <form className="form" style={{ maxWidth: 'none' }} onSubmit={(e) => { e.preventDefault(); if (!missing.length) void save() }}>
          {choices.length > 1 && (
            <label>Which one
              <select className="select" value={driver?.id || ''} onChange={(e) => { setDriver(choices.find((d) => d.id === e.target.value) || null); setOpts({}) }}>
                <option value="">Choose…</option>
                {choices.map((d) => <option key={d.id} value={d.id} disabled={!d.available || d.planned}>{d.name}{d.planned ? ' (not built yet)' : !d.available ? ' (not on this computer)' : ''}</option>)}
              </select>
            </label>
          )}
          {choices.some((d) => (!d.available || d.planned) && d.note) && (
            <ul className="tvs-dim tvw-notes">{choices.filter((d) => (!d.available || d.planned) && d.note).map((d) => <li key={d.id}><b>{d.name}:</b> {d.note}</li>)}</ul>
          )}
          {driver && (
            <>
              <p className="muted">{driver.description}</p>
              {driver.status !== 'tested' && <p className="tvs-note">{STATUS[driver.status]?.desc}</p>}
              {driver.note && <p className="tvs-dim">{driver.note}</p>}
              {['network', 'satip', 'card'].includes(kind?.id || '') && (
                <div className="tvw-find">
                  <Button size="sm" icon={<IconRefresh />} busy={searching} disabled={searching} onClick={() => void search()}>{searching ? 'Searching…' : 'Search for it'}</Button>
                  <span className="tvs-dim">Or type its details below: anything the search doesn't find can be added by hand.</span>
                </div>
              )}
              {found && found.length > 0 && (
                <ul className="tvs-list" aria-label="Found">
                  {found.map((f, i) => (
                    <li key={i} className="tvs-item">
                      <div className="tvs-item-text"><b>{f.name}</b><span>{f.detail || drivers.find((d) => d.id === f.driver)?.name}</span></div>
                      <Button size="sm" onClick={() => pickFound(f)}>Use this one</Button>
                    </li>
                  ))}
                </ul>
              )}
              {found && found.length === 0 && <p className="tvs-note" role="status">Nothing found on your network or this computer, or the search didn't finish. Type its details below.</p>}
              <label>Name<input className="input" value={name} onChange={(e) => setName(e.target.value)} maxLength={200} placeholder={driver.name} autoFocus /></label>
              {driver.fields.map((f) => <Field key={f.key} f={f} value={opts[f.key] || ''} onChange={(v) => setOpts({ ...opts, [f.key]: v })} />)}
              {missing.length > 0 && <p className="tvs-dim">Still needed: {missing.join(', ')}.</p>}
              <p className="tvs-dim">No channel lists or stream addresses come with the server: bring your own.</p>
            </>
          )}
          {!driver && choices.length > 1 && <p className="muted">Choose which one you have.</p>}
          {driver && <p className="tvs-dim">Check it asks for its channels without saving anything, to see that the details are right.</p>}
          <div aria-live="polite">
            {check?.ok && (
              <div className="tvw-answer" role="status">
                <b>It answered</b>
                <p>{check.result.detail ? `${check.result.detail}. ` : ''}{check.result.channels > 0 ? `${plural(check.result.channels, 'channel')} listed` : driver?.scans ? 'No channels listed yet: this kind finds them with a scan, after it’s added' : 'No channels listed'}{check.result.encrypted > 0 ? `, ${check.result.encrypted} of them marked as encrypted` : ''}.{check.result.maxStreams ? ` It can play ${plural(check.result.maxStreams, 'channel')} at once.` : ''}</p>
                {check.result.sample && check.result.sample.length > 0 && <p className="tvs-dim">Such as {check.result.sample.join(', ')}.</p>}
              </div>
            )}
            {check && !check.ok && <Problem p={check.problem} title="That didn’t work yet" />}
          </div>
          {err && <p className="tvs-error" role="alert">{err}</p>}
          <button type="submit" hidden />
        </form>
      )}

      {step === 'channels' && tuner && (
        <div className="tvw-result" aria-live="polite">
          {tuner.lastError
            ? <p className="tvs-error" role="alert">It was added, but its channels couldn't be read: {tuner.lastError}</p>
            : <p><b>{tuner.channels} {tuner.channels === 1 ? 'channel' : 'channels'}</b> found{tuner.clear ? `, ${tuner.clear} clear` : ''}{tuner.encrypted ? `, ${tuner.encrypted} encrypted` : ''}. They're in the guide now.</p>}
          {tuner.encrypted > 0 && <p className="tvs-note">Encrypted channels can't be played here, which isn't a fault in your setup. <Link to="/tv/encrypted">What can I do?</Link></p>}
          {driver?.scans && <p className="muted">{tuner.channels === 0 ? 'A tuner usually needs a scan to find its channels.' : 'A scan looks for channels the device doesn’t list by itself.'} It can take a few minutes.</p>}
          <div className="tvs-toolbar">
            {tuner.lastError && <Button disabled={busy || !!scan?.running} onClick={() => void changeDetails()}>Remove it and change the details</Button>}
            {driver?.scans && <Button variant={tuner.channels === 0 ? 'primary' : 'default'} disabled={!!scan?.running} onClick={() => void startScan()}>{scan?.running ? `Scanning… ${scan.percent}%` : 'Scan for channels'}</Button>}
            <Button disabled={busy || !!scan?.running} icon={<IconRefresh />} onClick={() => void readAgain()}>Read the list again</Button>
          </div>
          {scan?.running && <div className="tvs-progress" role="progressbar" aria-valuenow={scan.percent} aria-valuemin={0} aria-valuemax={100} aria-label="Scan progress"><i style={{ width: scan.percent + '%' }} /><span>{scan.found} found{scan.message ? ` · ${scan.message}` : ''}</span></div>}
          {scan?.error && <p className="tvs-error" role="alert">{scan.error}</p>}
          {scan && !scan.running && !scan.error && <p className="tvs-note" role="status">The scan found {scan.clear} clear and {scan.encrypted} encrypted {scan.clear + scan.encrypted === 1 ? 'channel' : 'channels'}{scan.unknown ? `, and ${scan.unknown} not checked yet` : ''}.</p>}
        </div>
      )}

      {step === 'try' && tuner && (
        <div className="tvw-result">
          <p className="muted">This tunes one channel for a few seconds, as watching it would, to show that it plays. Nothing is recorded. It uses one of the device’s tuners while it runs, and can take up to a minute.</p>
          {channels && tryable.length > 0 && (
            <label className="tvw-pick">Channel
              <select className="select" value={pick} onChange={(e) => { setPick(e.target.value); setTried(null) }}>
                <option value="">Let it choose</option>
                {tryable.map((c) => <option key={c.id} value={c.id}>{c.number ? `${c.number} ` : ''}{c.name}</option>)}
              </select>
            </label>
          )}
          <div aria-live="polite">
            {trying && <p className="muted" role="status">Tuning in…</p>}
            {tried?.ok && (
              <div className="tvw-answer" role="status">
                <b>{triedName} played</b>
                <p>{[tried.video && `Picture: ${tried.video}`, tried.audio && `Sound: ${tried.audio}`].filter(Boolean).join('. ')}{tried.video || tried.audio ? '. ' : ''}It started in {tried.seconds.toFixed(1)} seconds.</p>
                {tried.clear && <p>The server watched the stream, and it isn’t encrypted.</p>}
              </div>
            )}
            {tried && !tried.ok && tried.problem && <Problem p={tried.problem} title={triedName ? `${triedName} didn’t play` : 'Nothing could be tried'} />}
          </div>
        </div>
      )}

      {step === 'guide' && (
        <form className="form" style={{ maxWidth: 'none' }} onSubmit={(e) => { e.preventDefault(); if (guideURL.trim()) void addGuide() }}>
          <p className="muted">Listings for these channels come from an XMLTV guide, which many providers offer beside their playlist. Without one, the guide shows the channels with no programmes. You can add one later under Guide data.</p>
          <label>XMLTV address, or a file on the server<input className="input" value={guideURL} onChange={(e) => setGuideURL(e.target.value)} autoFocus /></label>
          {err && <p className="tvs-error" role="alert">{err}</p>}
          <button type="submit" hidden />
        </form>
      )}

      {step === 'done' && (
        <p>{tuner && !tuner.lastError ? `${tuner.channels} ${tuner.channels === 1 ? 'channel is' : 'channels are'} in the guide.` : 'You can read its channels again from its page.'} Choose which show in the guide, their numbers and logos on its page.</p>
      )}
    </Dialog>
  )
}
