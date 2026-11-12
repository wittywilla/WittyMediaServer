// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { announce } from '../../a11y'
import { toast } from '../../state'
import { Button, Dialog, LinkButton } from '../../ui'
import { STATUS } from './tunerFields'
import SpecPage from '../../spec/SpecPage'
import { handledIds } from '../../spec/model'
import { channelsSpec, detailsSpec, guideSpec, kindSpec, trySpec } from '../../spec/specs/tunerSetup'
import { forgetLineup, type Driver, type Found, type ScanState, type Tuner, type TunerChannel, type TunerCheck, type TunerTest } from './tvApi'

// Adding live TV, a step at a time: what someone has, in their words; its details,
// typed in or found on the network, and checked before saving if they like; then its
// channels read or scanned, one of them tried for a few seconds, and guide data where
// it has none. The drivers and what each asks for come from WittyTuner.

type Kind = { id: string; label: string; hint: string; drivers: string[] }

// Driver ids by what people call the thing they have. A driver this list doesn't
// know yet goes under Something else, so a new one in WittyTuner still shows.
const KINDS: Kind[] = [
  { id: 'playlist', label: 'A playlist from a TV provider', hint: 'An M3U or M3U8 list of channels: an address, or a file on this server.', drivers: ['iptv'] },
  { id: 'xtream', label: 'An Xtream Codes-style provider', hint: 'A provider that gave you a server address, a username and a password. Its channels, groups, logos, guide and catch-up come with it.', drivers: ['xtream'] },
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
  // The spec's answers: rows and chips by id, fields by id.
  const lineOf = (d: Driver) => ({ id: d.id, name: d.name, available: d.available, planned: d.planned, note: d.note, description: d.description, statusNote: d.status !== 'tested' ? STATUS[d.status]?.desc : undefined })
  const onAction = (id: string) => {
    if (id.startsWith('kind:')) { const k = KINDS.find((x) => 'kind:' + x.id === id); if (k) pickKind(k) } else if (id === 'search') void search()
    else if (id.startsWith('use:')) { const f = found?.[Number(id.slice(4))]; if (f) pickFound(f) } else if (id === 'change') void changeDetails()
    else if (id === 'scan') void startScan(); else if (id === 'again') void readAgain()
  }
  const onField = (id: string, v: string | number | boolean | File) => {
    const val = String(v)
    if (id === 'name') setName(val); else if (id === 'guide') setGuideURL(val); else if (id === 'pick') { setPick(val); setTried(null) }
    else if (id === 'driver') { setDriver(choices.find((d) => d.id === val) || null); setOpts({}) }
    else if (id.startsWith('f:')) setOpts((o) => ({ ...o, [id.slice(2)]: typeof v === 'boolean' ? (v ? '1' : '0') : val }))
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

      {step === 'kind' && <Spec page={kindSpec(KINDS.map((k) => { const ds = driversOf(k, drivers); const usable = ds.filter((d) => d.available && !d.planned); return { id: k.id, label: k.label, hint: k.hint, usable: usable.length > 0, dim: ds.find((d) => d.note)?.note, n: ds.length } }).filter((k) => k.n > 0), kind?.id || null)} onAction={onAction} onField={onField} />}

      {step === 'details' && (
        <form onSubmit={(e) => { e.preventDefault(); if (!missing.length) void save() }}>
          <Spec page={detailsSpec({
            choices: choices.map(lineOf), driver: driver ? lineOf(driver) : null, fields: driver?.fields || [], name, opts,
            canSearch: ['network', 'satip', 'card'].includes(kind?.id || ''), searching, found: found ? found.map((f) => ({ name: f.name, detail: f.detail || drivers.find((d) => d.id === f.driver)?.name || '' })) : null,
            missing, err,
            check: check ? (check.ok ? { ok: true, text: `${check.result.detail ? `${check.result.detail}. ` : ''}${check.result.channels > 0 ? `${plural(check.result.channels, 'channel')} listed` : driver?.scans ? 'No channels listed yet: this kind finds them with a scan after it is added' : 'No channels listed yet'}.`, sample: check.result.sample?.join(', ') } : { ok: false, problem: check.problem }) : null,
          })} onAction={onAction} onField={onField} />
          <button type="submit" hidden />
        </form>
      )}

      {step === 'channels' && tuner && (
        <Spec page={channelsSpec({
          error: tuner.lastError, hasError: !!tuner.lastError, busy, zero: tuner.channels === 0, scanning: !!scan?.running, percent: scan?.percent || 0, canScan: !!driver?.scans,
          found: `${tuner.channels} ${tuner.channels === 1 ? 'channel' : 'channels'} found${tuner.clear ? `, ${tuner.clear} clear` : ''}${tuner.encrypted ? `, ${tuner.encrypted} encrypted` : ''}. They’re in the guide now.`,
          encrypted: tuner.encrypted > 0,
          scanNote: driver?.scans ? `${tuner.channels === 0 ? 'A tuner usually needs a scan to find its channels.' : 'A scan looks for channels the device doesn’t list by itself.'} It can take a few minutes.` : '',
          scanText: scan?.error ? scan.error : scan?.running ? `Found ${scan.found} so far` : scan ? `The scan found ${scan.clear} clear and ${scan.encrypted} encrypted ${scan.clear + scan.encrypted === 1 ? 'channel' : 'channels'}${scan.unknown ? `, and ${scan.unknown} it could not tell` : ''}.` : '',
        })} onAction={onAction} onField={onField} after={<>
          {tuner.encrypted > 0 && <p className="tvs-note"><Link to="/tv/encrypted">What can I do about encrypted channels?</Link></p>}
          {scan?.running && <div className="tvs-progress" role="progressbar" aria-valuenow={scan.percent} aria-valuemin={0} aria-valuemax={100} aria-label="Scan progress"><i style={{ width: scan.percent + '%' }} /></div>}
        </>} />
      )}

      {step === 'try' && tuner && (
        <Spec page={trySpec({
          options: channels ? tryable.map((c) => ({ value: String(c.id), label: `${c.number ? `${c.number} ` : ''}${c.name}` })) : null, pick, trying,
          answer: tried?.ok ? { ok: true, title: `${triedName} played`, text: `${[tried.video && `Picture: ${tried.video}`, tried.audio && `Sound: ${tried.audio}`].filter(Boolean).join('. ')}${tried.video || tried.audio ? '. ' : ''}It started in ${tried.seconds.toFixed(1)} seconds.${tried.clear ? ' The server watched the stream, and it isn’t encrypted.' : ''}` }
            : tried && tried.problem ? { ok: false, title: triedName ? `${triedName} didn’t play` : 'Nothing could be tried', problem: tried.problem } : null,
        })} onAction={onAction} onField={onField} after={tried?.problem?.kind === 'encrypted' ? <p className="tvs-note"><Link to="/tv/encrypted">What can I do about encrypted channels?</Link></p> : undefined} />
      )}

      {step === 'guide' && (
        <form onSubmit={(e) => { e.preventDefault(); if (guideURL.trim()) void addGuide() }}>
          <Spec page={guideSpec(guideURL, err)} onAction={onAction} onField={onField} />
          <button type="submit" hidden />
        </form>
      )}

      {step === 'done' && (
        <p>{tuner && !tuner.lastError ? `${tuner.channels} ${tuner.channels === 1 ? 'channel is' : 'channels are'} in the guide.` : 'You can read its channels again from its page.'} Choose which show in the guide, their numbers and logos on its page.</p>
      )}
    </Dialog>
  )
}

// The setup's body for one step: a page of its own inside the dialog. Every item with no address is answered here.
function Spec({ page, onAction, onField, after }: { page: import('../../spec/types').SpecPageData; onAction: (id: string) => void; onField: (id: string, v: string | number | boolean | File) => void; after?: React.ReactNode }) {
  const handled = new Set(handledIds(page))
  for (const sec of page.sections) if (sec.kind === 'rows') for (const r of sec.items) for (const a of r.actions || []) if (a.id && !a.disabled) handled.add(a.id)
  return <SpecPage page={page} embedded handled={handled} onAction={onAction} onField={onField} after={after ? { [page.sections.length - 1]: after } : undefined} />
}
