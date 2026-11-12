// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { Link, Navigate, Route, Routes, useParams } from 'react-router-dom'
import { fmtDateTime, api } from '../../api'
import { ChooseFolderButton } from '../../components/FolderPicker'
import { IconEdit, IconPlus, IconRefresh, IconTv } from '../../icons'
import { toast, useApp } from '../../state'
import { ask } from '../../ui'
import { PageHead, Row, Section, Switch } from '../settings/ui'
import TunerLogo from './TunerLogo'
import { LogoConsentHint } from '../../components/edit/LogoChooser'
import { forgetOwnerLook } from './look'
import { THEMES } from './tvThemes'
import { forgetLineup, type Channel, type Driver, type EPGSource, type Found, type ScanState, type Tuner, type TunerChannel } from './tvApi'
import { useFeatures } from '../../sdui/useSdui'
import { BulkBar, ChannelHealth, CheckAll, Duplicates, Merged } from './TunerTools'
import { Field, STATUS } from './tunerFields'
import SpecPage from '../../spec/SpecPage'
import { guideSourcesSpec, tunersSpec } from '../../spec/specs/tuners'
import TunerSetup from './TunerSetup'
import { ModuleMissing } from '../../components/ModuleMissing'
import './tvsettings.css'
import LegalNote from '../../components/LegalNote'

// Settings → Channels & tuners: tuners and IPTV, guide data and WittyTV's options.
// Channels from the library are made and changed on their own page, /tv/channels.

export default function TvSettings() {
  return (
    <Routes>
      <Route index element={<Overview />} />
      <Route path="new" element={<MovedEditor />} />
      <Route path="edit/:id" element={<MovedEditor />} />
      <Route path="tuner/:id" element={<TunerChannels />} />
      <Route path="duplicates" element={<Duplicates />} />
      <Route path="*" element={<Navigate to="/settings/channels" replace />} />
    </Routes>
  )
}

function Overview() {
  return (
    <>
      <PageHead title="Channels & tuners" desc="Live TV from tuners and IPTV, guide data and WittyTV's options. Channels from your library sit beside them in one guide."
        actions={<Link className="btn" to="/tv"><IconTv /> Open the guide</Link>} />
      <LibraryChannels />
      <Tuners />
      <GuideSources />
      <Options />
      <LegalNote kind="tv" className="set-legal" />
      <LegalNote kind="logos" className="set-legal" />
    </>
  )
}

// ------------------------------------------------------------ library channels

// The channels themselves have a page of their own (ChannelsPage), so that this
// one holds only settings.
function LibraryChannels() {
  const [list, setList] = useState<Channel[] | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<Channel[]>('/channels').then(setList).catch((e) => setErr(e.message)) }, [])
  const on = list?.filter((c) => c.enabled).length || 0
  return (
    <Section title="Channels from your library" desc="Always-on channels made from your films and shows. Making and changing them, and the ones suggested from your library, are on their own page.">
      <Row label="Your channels" desc={err ? `The list couldn't be read: ${err}` : list === null ? '' : list.length === 0 ? 'None yet.' : `${list.length} ${list.length === 1 ? 'channel' : 'channels'}, ${on} on the air.`}>
        <Link className="btn sm primary" to="/tv/channels"><IconEdit /> Open the channel list</Link>
      </Row>
    </Section>
  )
}

// Where the editor used to be, for bookmarks and links from before.
function MovedEditor() {
  const { id } = useParams()
  return <Navigate to={id ? `/tv/channels/${id}` : '/tv/channels/new'} replace />
}

// -------------------------------------------------------------------- tuners

function Tuners() {
  const [list, setList] = useState<Tuner[] | null>(null)
  const [drivers, setDrivers] = useState<Driver[] | null>(null)
  const [noModule, setNoModule] = useState(false)
  const [driversErr, setDriversErr] = useState('')
  const [found, setFound] = useState<Found[] | null>(null)
  const [searching, setSearching] = useState(false)
  // The setup, open: from the start, at a device that was found, or at one kind by hand.
  const [setup, setSetup] = useState<{ start?: Found } | null>(null)
  const dupes = !!useFeatures()?.has('tv-duplicates')
  const load = () => api.get<Tuner[]>('/tuners').then(setList).catch((e) => { if (e.status !== 424) toast(e.message) })
  const loadDrivers = () => { setDriversErr(''); return api.get<Driver[]>('/tuners/drivers').then((d) => { setDrivers(d); setNoModule(false) }).catch((e) => { if (e.status === 424) setNoModule(true); else setDriversErr(e.message) }) }
  useEffect(() => { void load(); void loadDrivers() }, [])
  const search = async () => {
    setSearching(true)
    // A search that doesn't finish found nothing: Add live TV takes it by hand.
    try { setFound(await api.get<Found[]>('/tuners/discover')) } catch { setFound([]) } finally { setSearching(false) }
  }
  const remove = async (t: Tuner) => {
    if (!await ask({ title: `Remove “${t.name}” from this server?`, body: t.channels === 1 ? 'Its channel goes with it.' : `Its ${t.channels} channels go with it.`, confirm: 'Remove tuner', danger: true })) return
    try { await api.del(`/tuners/${t.id}`); forgetLineup(); void load() } catch (e: any) { toast(e.message) }
  }
  const desc = <><Link to="/help/channels#live-tv-playlists">How tuners and playlists work.</Link> Add as many as you like, including streams a licensed device offers to home media servers. Encrypted channels are shown as encrypted and never decrypted (<Link to="/tv/encrypted">why, and what you can do</Link>). No channel lists or stream addresses come with the server: bring your own.</>
  if (noModule) {
    return (
      <Section title="Tuners and IPTV" desc={desc}>
        <p className="muted">Live TV from a tuner, a provider's playlist or a capture card needs the WittyTuner module. Channels from your library don't.</p>
        <ModuleMissing ids={['wittytuner']} what="live TV" onReady={() => { void loadDrivers(); void load() }} />
      </Section>
    )
  }
  return (
    <Section title="Tuners and IPTV" desc={desc}
      actions={<>
        {dupes && list && list.length > 0 && <Link className="btn sm" to="duplicates">Duplicates</Link>}
        <button className="btn sm" disabled={searching || !drivers} onClick={() => void search()}><IconRefresh /> {searching ? 'Searching…' : 'Search for tuners'}</button>
        <button className="btn sm primary" disabled={!drivers} onClick={() => setSetup({})}><IconPlus /> Add live TV</button>
      </>}>
      {!drivers && !driversErr && <p className="muted" role="status">Getting WittyTuner ready…</p>}
      {driversErr && <p className="tvs-error" role="alert">WittyTuner didn't answer: {driversErr} <button className="btn sm" onClick={() => void loadDrivers()}><IconRefresh /> Try again</button></p>}
      {drivers && (list || found) && <TunersList list={list} found={found} drivers={drivers} onSetup={(f) => setSetup({ start: f })} onRemove={(t) => void remove(t)} />}
      {drivers && drivers.length > 0 && (
        <details className="tvs-details">
          <summary>What WittyTuner can use, and how well each has been tried</summary>
          <div className="tvs-drivers">
            {drivers.map((d) => (
              <div key={d.id} className={'tvs-driver' + (d.available ? '' : ' off')}>
                <div className="tvs-driver-head"><b>{d.name}</b><span className={'tvs-status ' + d.status} title={STATUS[d.status]?.desc}>{STATUS[d.status]?.label}</span></div>
                <p>{d.description}</p>
                {d.note && <p className="tvs-dim">{d.note}</p>}
                {d.available && !d.planned && <button className="btn sm" onClick={() => setSetup({ start: { driver: d.id, name: '', options: {} } })} aria-label={`Add ${d.name} by hand`}><IconPlus /> Add by hand</button>}
              </div>
            ))}
          </div>
        </details>
      )}
      {setup && drivers && <TunerSetup drivers={drivers} start={setup.start} onClose={() => setSetup(null)} onAdded={() => { setFound(null); void load() }} />}
    </Section>
  )
}

// The tuners found and the tuners added, as rows.
function TunersList({ list, found, drivers, onSetup, onRemove }: { list: Tuner[] | null; found: Found[] | null; drivers: Driver[]; onSetup: (f: Found) => void; onRemove: (t: Tuner) => void }) {
  const page = tunersSpec(list && list.map((t) => ({ id: t.id, name: t.name, driverName: drivers.find((d) => d.id === t.driver)?.name || t.driver, channels: t.channels, clear: t.clear, encrypted: t.encrypted, lastError: t.lastError })),
    found && found.map((f) => ({ name: f.name, detail: f.detail || drivers.find((x) => x.id === f.driver)?.name || '', canSetUp: drivers.some((x) => x.id === f.driver) })))
  const handled = new Set<string>()
  for (const s of page.sections) if (s.kind === 'rows') for (const r of s.items) for (const a of r.actions || []) if (a.id) handled.add(a.id)
  const onAction = (id: string) => {
    if (id.startsWith('set:')) { const f = found?.[Number(id.slice(4))]; if (f) onSetup(f) } else if (id.startsWith('remove:')) { const t = list?.find((x) => x.id === Number(id.slice(7))); if (t) onRemove(t) }
  }
  return <SpecPage page={page} embedded bare handled={handled} onAction={onAction} />
}

function TunerChannels() {
  const id = location.pathname.split('/').pop() || ''
  const [t, setT] = useState<Tuner | null>(null)
  const [list, setList] = useState<TunerChannel[] | null>(null)
  const [driver, setDriver] = useState<Driver | null>(null)
  const [scan, setScan] = useState<ScanState | null>(null)
  const [q, setQ] = useState('')
  const [opts, setOpts] = useState<Record<string, string> | null>(null)
  const [limit, setLimit] = useState('0')
  const features = useFeatures()
  const health = !!features?.has('tv-channel-health')
  const dupes = !!features?.has('tv-duplicates')
  // Bulk changes arrived with health checks and merging; either says the server has them.
  const bulk = health || dupes
  const [picked, setPicked] = useState<Set<number>>(() => new Set())
  const load = () => {
    api.get<Tuner[]>('/tuners').then((l) => {
      const me = l.find((x) => String(x.id) === id) || null
      setT(me)
      if (me) { setOpts(me.options); setLimit(String(me.maxStreams)); api.get<Driver[]>('/tuners/drivers').then((d) => setDriver(d.find((x) => x.id === me.driver) || null)).catch(() => {}) }
    }).catch((e) => toast(e.message))
    api.get<TunerChannel[]>(`/tuners/${id}/channels`).then(setList).catch((e) => toast(e.message))
  }
  useEffect(load, [id])
  useEffect(() => {
    if (!scan?.running) return
    const t = setTimeout(() => api.get<ScanState>(`/tuners/${id}/scan`).then((s) => { setScan(s); if (!s.running) { load(); forgetLineup(); toast(s.error || `Scan finished: ${s.clear} clear, ${s.encrypted} encrypted${s.unknown ? `, ${s.unknown} not checked yet` : ''}`) } }).catch(() => {}), 1500)
    return () => clearTimeout(t)
  }, [scan]) // eslint-disable-line react-hooks/exhaustive-deps
  const edit = async (c: TunerChannel, patch: Record<string, unknown>) => {
    try {
      const n = await api.put<TunerChannel>(`/tuner-channels/${c.id}`, patch)
      setList((l) => l?.map((x) => (x.id === c.id ? n : x)) || null)
      forgetLineup()
    } catch (e: any) { toast(e.message) }
  }
  const startScan = async (source: string) => {
    try { setScan(await api.post<ScanState>(`/tuners/${id}/scan`, { source })) } catch (e: any) { toast(e.message) }
  }
  const all = async (on: boolean) => {
    try { await api.post(`/tuners/${id}/channels/enable`, { enabled: on }); forgetLineup(); load() } catch (e: any) { toast(e.message) }
  }
  const saveSettings = async () => {
    try { await api.put(`/tuners/${id}`, { name: t?.name, options: opts, enabled: t?.enabled, maxStreams: Number(limit) || 0 }); toast('Saved'); load() } catch (e: any) { toast(e.message) }
  }
  const refresh = async () => {
    try { await api.post(`/tuners/${id}/refresh`); toast('Channel list read'); forgetLineup(); load() } catch (e: any) { toast(e.message) }
  }
  const shown = (list || []).filter((c) => !q || c.name.toLowerCase().includes(q.toLowerCase()) || c.number.includes(q) || (c.group || '').toLowerCase().includes(q.toLowerCase())).slice(0, 500)
  const pick = (c: TunerChannel, on: boolean) => setPicked((p) => { const n = new Set(p); if (on) n.add(c.id); else n.delete(c.id); return n })
  const allShownPicked = shown.length > 0 && shown.every((c) => picked.has(c.id))
  // What is picked, in the order shown: renumbering and merging go by it.
  const pickedInOrder = shown.filter((c) => picked.has(c.id)).concat((list || []).filter((c) => picked.has(c.id) && !shown.includes(c)))
  return (
    <>
      <PageHead title={t?.name || 'Tuner'} desc={t?.detail || driver?.name} actions={<Link className="btn" to="..">Back</Link>} />
      {t?.lastError && <p className="tvs-error" role="alert">{t.lastError}</p>}
      <Section title="Channels" desc={list ? `${list.length} channels, ${list.filter((c) => c.enabled).length} shown in the guide. What you change here stays when the list is read again.` : ''}
        actions={<>
          <button className="btn sm" onClick={() => void refresh()}><IconRefresh /> Read the list</button>
          {driver?.scans && <button className="btn sm primary" disabled={scan?.running} onClick={() => void startScan(opts?.source || '')}>{scan?.running ? `Scanning… ${scan.percent}%` : 'Scan for channels'}</button>}
        </>}>
        {health && <CheckAll tunerId={id} onDone={load} />}
        {scan?.running && <div className="tvs-progress" role="progressbar" aria-valuenow={scan.percent} aria-valuemin={0} aria-valuemax={100} aria-label="Scan progress"><i style={{ width: scan.percent + '%' }} /><span>{scan.found} found{scan.message ? ` · ${scan.message}` : ''}</span></div>}
        {scan?.error && <p className="tvs-error" role="alert">{scan.error}</p>}
        {scan && !scan.running && !scan.error && <p className="tvs-note" role="status">The scan found {scan.clear} clear and {scan.encrypted} encrypted {scan.clear + scan.encrypted === 1 ? 'channel' : 'channels'}{scan.unknown ? `, and ${scan.unknown} not checked yet` : ''}.{scan.encrypted > 0 && <> Encrypted channels can't be played here, which isn't a fault in your setup. <Link to="/tv/encrypted">What can I do?</Link></>}</p>}
        <div className="tvs-toolbar">
          <input className="input" type="search" aria-label="Find a channel" placeholder="Find a channel…" value={q} onChange={(e) => setQ(e.target.value)} />
          <button className="btn sm ghost" onClick={() => void all(true)}>Show all</button>
          <button className="btn sm ghost" onClick={() => void all(false)}>Hide all</button>
        </div>
        {bulk && picked.size > 0 && <BulkBar picked={pickedInOrder} all={list || []} health={health} merging={dupes} onDone={() => { setPicked(new Set()); forgetLineup(); load() }} onClear={() => setPicked(new Set())} />}
        <div className="tvs-table-wrap">
          <table className="tvs-table">
            <thead><tr>
              {bulk && <th scope="col"><input type="checkbox" aria-label="Select every channel shown" checked={allShownPicked} onChange={(e) => setPicked(e.target.checked ? new Set([...picked, ...shown.map((c) => c.id)]) : new Set([...picked].filter((x) => !shown.some((c) => c.id === x))))} /></th>}
              <th scope="col">In guide</th><th scope="col">Number</th><th scope="col">Name</th><th scope="col">Logo</th><th scope="col">Status</th>{health && <th scope="col">Health</th>}<th scope="col">Can record</th><th scope="col">Guide id</th>
            </tr></thead>
            <tbody>
              {shown.map((c) => (
                <tr key={c.id} className={(c.encrypted ? 'locked' : '') + (c.mergedInto ? ' tvs-backup' : '')}>
                  {bulk && <td><input type="checkbox" aria-label={`Select ${c.number} ${c.name}`} checked={picked.has(c.id)} onChange={(e) => pick(c, e.target.checked)} /></td>}
                  <td><input type="checkbox" aria-label={`${c.name} shows in the guide`} checked={c.enabled} onChange={(e) => void edit(c, { enabled: e.target.checked })} /></td>
                  <td><input className="input tvs-num-in" aria-label={`Number of ${c.name}`} defaultValue={c.number} onBlur={(e) => { if (e.target.value !== c.number) void edit(c, { number: e.target.value }) }} /></td>
                  <td>
                    <input className="input" aria-label={`Name of ${c.name}`} defaultValue={c.name} onBlur={(e) => { if (e.target.value !== c.name) void edit(c, { name: e.target.value }) }} />
                    {c.group && <small className="tvs-dim">{c.group}</small>}
                    <Merged c={c} all={list || []} />
                    {!!c.catchupDays && <small className="tvs-dim">Catch-up: {c.catchupDays === 1 ? '1 day' : `${c.catchupDays} days`}</small>}
                  </td>
                  <td><TunerLogo channel={c} onChange={(n) => setList((l) => l?.map((x) => (x.id === n.id ? n : x)) || null)} /></td>
                  <td>
                    {c.security === 'encrypted' && <small className="tvs-lock">Encrypted: can't be played here. {c.encrypted}. <Link to="/tv/encrypted">What can I do?</Link> <button className="btn sm ghost" onClick={() => void edit(c, { checkAgain: true })} title="If the station has stopped encrypting: it is checked again next time it is tuned">Check again</button></small>}
                    {c.security === 'clear' && <small className="tvs-clear">Clear</small>}
                    {c.security === 'unknown' && <small className="tvs-dim" title="This source doesn't say. It is checked when you tune in.">Not checked yet</small>}
                    {c.atsc3 && <small className="tvs-dim">ATSC 3.0{c.audioCodec ? ` · ${c.videoCodec || ''} ${c.audioCodec}` : ''}</small>}
                  </td>
                  {health && <td><ChannelHealth c={c} /></td>}
                  <td><input type="checkbox" aria-label={`${c.name} can be recorded`} disabled={!!c.encrypted} checked={!c.noRecord && !c.encrypted} title={c.noRecord || 'Untick if the device says this may not be recorded'} onChange={(e) => void edit(c, { noRecord: !e.target.checked })} /></td>
                  <td><input className="input mono" aria-label={`Guide id of ${c.name}`} defaultValue={c.epgSet || c.epgId || ''} onBlur={(e) => { if (e.target.value !== (c.epgSet || c.epgId || '')) void edit(c, { epg: e.target.value }) }} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {list && shown.length < list.length && shown.length === 500 && <p className="muted">Showing the first 500. Use the search box to find others.</p>}
        <LogoConsentHint compact />
        <LegalNote kind="logos" className="set-legal" />
      </Section>
      {driver && opts && (
        <Section title="Settings" actions={<button className="btn sm primary" onClick={() => void saveSettings()}>Save</button>}>
          <div className="form" style={{ maxWidth: 'none' }}>
            {driver.fields.map((f) => <Field key={f.key} f={f} value={opts[f.key] || ''} onChange={(v) => setOpts({ ...opts, [f.key]: v })} />)}
            <label htmlFor="tf-limit">Streams at once (0 = as the device says)
              <input className="input" id="tf-limit" type="number" min={0} value={limit} onChange={(e) => setLimit(e.target.value)} />
            </label>
          </div>
        </Section>
      )}
    </>
  )
}

// --------------------------------------------------------------- guide sources

function GuideSources() {
  const [list, setList] = useState<EPGSource[] | null>(null)
  const [url, setUrl] = useState('')
  const load = () => api.get<EPGSource[]>('/epg/sources').then(setList).catch((e) => toast(e.message))
  useEffect(() => { void load() }, [])
  const add = async () => {
    try { await api.post('/epg/sources', { url }); setUrl(''); toast('Reading the guide…'); setTimeout(() => void load(), 2500) } catch (e: any) { toast(e.message) }
  }
  const remove = async (s: EPGSource) => { try { await api.del(`/epg/sources/${s.id}`); void load() } catch (e: any) { toast(e.message) } }
  const refresh = async (s: EPGSource) => { try { await api.post(`/epg/sources/${s.id}/refresh`); toast('Reading the guide…'); setTimeout(() => void load(), 2500) } catch (e: any) { toast(e.message) } }
  return (
    <Section title="Guide data" desc="Listings for tuner and IPTV channels, in XMLTV format, refreshed by themselves. Broadcasts also carry their own, read while a channel is on. Library channels make their own schedule. Matched by guide id, then by name.">
      <form onSubmit={(e) => { e.preventDefault(); if (url.trim()) void add() }}>
      <SpecPage embedded bare page={guideSourcesSpec(list && list.map((s) => ({ id: s.id, name: s.name, programmes: s.programmes, read: s.lastFetch ? fmtDateTime(s.lastFetch * 1000) : '', lastError: s.lastError })), url)}
        handled={new Set([...(list || []).flatMap((s) => ['again:' + s.id, 'drop:' + s.id]), 'add'])}
        onField={(id, v) => { if (id === 'url') setUrl(String(v)) }}
        onAction={(id) => {
          if (id === 'add') void add()
          else if (id.startsWith('again:')) { const s = list?.find((x) => x.id === Number(id.slice(6))); if (s) void refresh(s) }
          else if (id.startsWith('drop:')) { const s = list?.find((x) => x.id === Number(id.slice(5))); if (s) void remove(s) }
        }} />
      <button type="submit" hidden />
      </form>
    </Section>
  )
}

// --------------------------------------------------------------------- options

// What a new viewer starts with. Someone's own choice comes first, then what their app theme
// suggests, and then this, so that's what it says.
function OwnerTheme({ s, set }: { s: Record<string, string>; set: (k: string, v: string) => Promise<void> }) {
  const current = s.tvTheme || ''
  return (
    <Row label="WittyTV theme for new viewers" desc={<>What someone starts with until they choose. Their own choice in <Link to="/settings/appearance#wittytv">Settings, Appearance</Link> or the guide's Look panel comes first, then what their app theme suggests, and then this. Custom is each viewer's own, so it isn't offered here.</>}>
      <select className="select" value={current} onChange={(e) => { void set('tvTheme', e.target.value).then(forgetOwnerLook) }}>
        {!current && <option value="">Classic</option>}
        {THEMES.filter((t) => t.id !== 'custom').map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
      </select>
    </Row>
  )
}

function Options() {
  const { libraries } = useApp()
  const [s, setS] = useState<Record<string, string> | null>(null)
  const saved = useRef<Record<string, string>>({})
  useEffect(() => { api.get<Record<string, string>>('/tv/settings').then((v) => { setS(v); saved.current = v }).catch((e) => toast(e.message)) }, [])
  const set = async (k: string, v: string) => {
    setS((x) => ({ ...(x || {}), [k]: v }))
    if (saved.current[k] === v) return
    try { saved.current = await api.put<Record<string, string>>('/tv/settings', { [k]: v }) } catch (e: any) { toast(e.message); setS(saved.current) }
  }
  if (!s) return null
  return (
    <Section title="Options">
      <OwnerTheme s={s} set={set} />
      <Row label="Pass H.264 through" desc="Video the browser can already play is sent as it is. Much lighter on the machine; turn it off only if channels stutter when shows change.">
        <Switch checked={s.tvCopyVideo === '1'} label="Pass H.264 through" onChange={(v) => void set('tvCopyVideo', v ? '1' : '0')} />
      </Row>
      <Row label="Keep playing in the background, on new devices" desc="Whether a channel carries on when a viewer switches to another tab or app. Each device can choose for itself in WittyTV's look and in the player; this is what one starts with.">
        <Switch checked={s.tvBackgroundPlay !== '0'} label="Keep playing in the background, on new devices" onChange={(v) => void set('tvBackgroundPlay', v ? '1' : '0')} />
      </Row>
      <Row label="Pause buffer, library channels (minutes)" desc="How far back viewers can pause and rewind. Kept on disk only while someone is watching.">
        <input className="input tvs-short" type="number" min={1} max={240} defaultValue={s.tvLibraryBufferMin} onBlur={(e) => void set('tvLibraryBufferMin', e.target.value)} />
      </Row>
      <Row label="Pause buffer, tuner channels (minutes)" desc="An hour of HD can take a few gigabytes of disk.">
        <input className="input tvs-short" type="number" min={1} max={240} defaultValue={s.tvTunerBufferMin} onBlur={(e) => void set('tvTunerBufferMin', e.target.value)} />
      </Row>
      <Row label="Largest picture on channels" desc="Height in lines; 0 uses the server's general limit. Lower is lighter on small machines.">
        <input className="input tvs-short" type="number" min={0} max={4320} defaultValue={s.tvMaxHeight} onBlur={(e) => void set('tvMaxHeight', e.target.value)} />
      </Row>
      <Row label="Recordings folder" desc={s.dvrFolder || 'In the server’s data folder.'} stack>
        <div className="tvs-toolbar">
          <ChooseFolderButton className="btn sm" onPick={(p) => void set('dvrFolder', p)} label="Choose folder…" />
          {s.dvrFolder && <button className="btn sm ghost" onClick={() => void set('dvrFolder', '')}>Use the default</button>}
        </div>
      </Row>
      <Row label="Library to update after a recording" desc="To see recordings in a library, choose a recordings folder inside it, and that library here.">
        <select className="select" value={s.dvrLibrary} onChange={(e) => void set('dvrLibrary', e.target.value)}>
          <option value="">None</option>
          {libraries.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
        </select>
      </Row>
    </Section>
  )
}
