import { useEffect, useRef, useState } from 'react'
import { Link, Navigate, Route, Routes } from 'react-router-dom'
import { api, fmtRuntime, mediaUrl } from '../../api'
import { ChooseFolderButton } from '../../components/FolderPicker'
import { IconEdit, IconPlus, IconRefresh, IconTrash, IconTv } from '../../icons'
import { toast, useApp } from '../../state'
import { PageHead, Row, Section, Switch } from '../settings/ui'
import ChannelEditor from './ChannelEditor'
import { forgetLineup, type Channel, type Driver, type DriverField, type EPGSource, type Found, type ScanState, type Suggestion, type Tuner, type TunerChannel } from './tvApi'
import './tvsettings.css'
import LegalNote from '../../components/LegalNote'

// Settings → Channels & tuners. Everything starts from the simplest thing:
// one click makes a channel; more detail is there for whoever wants it.

export default function TvSettings() {
  return (
    <Routes>
      <Route index element={<Overview />} />
      <Route path="new" element={<ChannelEditor />} />
      <Route path="edit/:id" element={<ChannelEditor />} />
      <Route path="tuner/:id" element={<TunerChannels />} />
      <Route path="*" element={<Navigate to="/settings/channels" replace />} />
    </Routes>
  )
}

function Overview() {
  return (
    <>
      <PageHead title="Channels & tuners" desc="Always-on TV channels made from your library, and live TV from tuners and IPTV, side by side in one guide."
        actions={<Link className="btn" to="/tv"><IconTv /> Open the guide</Link>} />
      <LegalNote kind="tv" className="set-legal" />
      <LibraryChannels />
      <Tuners />
      <GuideSources />
      <Options />
    </>
  )
}

// ------------------------------------------------------------ library channels

const GROUP_NAMES: Record<Suggestion['group'], string> = { network: 'TV networks', era: 'Eras', genre: 'Genres', show: 'Marathons', classic: 'Classics' }

function LibraryChannels() {
  const [list, setList] = useState<Channel[] | null>(null)
  const [sug, setSug] = useState<Suggestion[] | null>(null)
  const [busy, setBusy] = useState('')
  const [facts, setFacts] = useState<{ running: boolean; total: number; done: number; found: number } | null>(null)
  const load = () => {
    api.get<Channel[]>('/channels').then(setList).catch((e) => toast(e.message))
    api.get<Suggestion[]>('/channels/suggestions').then(setSug).catch(() => setSug([]))
  }
  useEffect(load, [])
  useEffect(() => {
    if (!facts?.running) return
    const t = setTimeout(() => api.get<typeof facts>('/channels/networks/refresh').then((f) => { setFacts(f); if (f && !f.running) { toast(`Network facts filled in for ${f.found} ${f.found === 1 ? 'show' : 'shows'}`); load() } }).catch(() => {}), 2000)
    return () => clearTimeout(t)
  }, [facts])
  const add = async (s: Suggestion) => {
    setBusy(s.key)
    try {
      await api.post('/channels', { name: s.name, description: s.description, definition: s.definition })
      forgetLineup()
      toast(`${s.name} is on the air`)
      load()
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const remove = async (c: Channel) => {
    if (!confirm(`Delete the channel “${c.name}”? Your files are not touched.`)) return
    try { await api.del(`/channels/${c.id}`); forgetLineup(); load() } catch (e: any) { toast(e.message) }
  }
  const setOn = async (c: Channel, enabled: boolean) => {
    try { await api.put(`/channels/${c.id}`, { enabled }); forgetLineup(); load() } catch (e: any) { toast(e.message) }
  }
  const groups = (['network', 'classic', 'era', 'genre', 'show'] as const).map((g) => ({ g, items: (sug || []).filter((s) => s.group === g && !s.exists) })).filter((x) => x.items.length)
  return (
    <Section title="Channels from your library" desc="They run all the time, like real TV: tune in and you join the show part-way through. FFmpeg only runs while someone is watching."
      actions={<Link className="btn sm primary" to="new"><IconPlus /> New channel</Link>}>
      {list && list.length > 0 && (
        <ul className="tvs-list">
          {list.map((c) => (
            <li key={c.id} className="tvs-item">
              <span className="tvs-num">{c.number}</span>
              {c.logo ? <img className="tvs-logo" src={mediaUrl(`/api/channels/${c.id}/logo?v=${c.logo}`)} alt="" /> : null}
              <div className="tvs-item-text"><b>{c.name}</b><span>{c.description || ({ simple: 'Simple', rules: 'Rules', timetable: 'Timetable' } as const)[c.definition.mode]}</span></div>
              <Switch checked={c.enabled} label={`${c.name} is on the air`} onChange={(v) => void setOn(c, v)} />
              <Link className="btn sm ghost" to={`edit/${c.id}`}><IconEdit /> Edit</Link>
              <button className="btn sm ghost" onClick={() => void remove(c)} aria-label={`Delete ${c.name}`}><IconTrash /></button>
            </li>
          ))}
        </ul>
      )}
      {sug && groups.length > 0 && <h3 className="tvs-h3">Add with one click</h3>}
      {groups.map(({ g, items }) => (
        <div key={g} className="tvs-sug-group">
          <h4 className="tvs-h4">{GROUP_NAMES[g]}</h4>
          <div className="tvs-sugs">
            {items.map((s) => (
              <div key={s.key} className="tvs-sug">
                <b>{s.name}</b>
                <span>{s.description}</span>
                <span className="tvs-dim">{s.stats.titles} titles · {fmtRuntime(s.stats.seconds)}</span>
                <div className="tvs-sug-actions">
                  <button className="btn sm primary" disabled={busy === s.key} onClick={() => void add(s)}><IconPlus /> Add</button>
                  <Link className="btn sm ghost" to="new" state={{ suggestion: s }}>Change first</Link>
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}
      {sug && sug.length === 0 && <p className="muted">Nothing to suggest yet. Once a TV or film library has been scanned, channels you can add with one click appear here.</p>}
      <Row label="Network facts" desc="Fills in the network and the original day and time of shows that don't have them yet, by asking your metadata services. Files are not touched. Network names are plain facts from metadata: nothing here is affiliated with, or endorsed by, any network.">
        <button className="btn sm" disabled={facts?.running} onClick={() => api.post<typeof facts>('/channels/networks/refresh').then(setFacts).catch((e) => toast(e.message))}>
          <IconRefresh /> {facts?.running ? `Asking… ${facts.done} of ${facts.total}` : 'Fill in missing facts'}
        </button>
      </Row>
    </Section>
  )
}

// -------------------------------------------------------------------- tuners

const STATUS: Record<Driver['status'], { label: string; desc: string }> = {
  tested: { label: 'Tested', desc: 'Tested end to end.' },
  mock: { label: 'Tried on stand-ins', desc: 'Built from the published protocol and tried against stand-in servers, not yet on real hardware.' },
  untested: { label: 'Not yet tried', desc: 'Built from the published documentation. It has not been tried on real hardware yet, so please report what you find.' },
  planned: { label: 'Not built yet', desc: '' },
}

function Tuners() {
  const [list, setList] = useState<Tuner[] | null>(null)
  const [drivers, setDrivers] = useState<Driver[]>([])
  const [found, setFound] = useState<Found[] | null>(null)
  const [searching, setSearching] = useState(false)
  const [adding, setAdding] = useState<{ driver: Driver; options: Record<string, string>; name: string } | null>(null)
  const load = () => api.get<Tuner[]>('/tuners').then(setList).catch((e) => toast(e.message))
  useEffect(() => { void load(); api.get<Driver[]>('/tuners/drivers').then(setDrivers).catch(() => {}) }, [])
  const search = async () => {
    setSearching(true)
    try { setFound(await api.get<Found[]>('/tuners/discover')) } catch (e: any) { toast(e.message) } finally { setSearching(false) }
  }
  const remove = async (t: Tuner) => {
    if (!confirm(`Remove “${t.name}” and its ${t.channels} channels from this server?`)) return
    try { await api.del(`/tuners/${t.id}`); forgetLineup(); void load() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title="Tuners and IPTV" desc={<>Add as many as you like, including streams a licensed device offers to home media servers. Encrypted channels are shown as encrypted and never decrypted (<Link to="/tv/encrypted">why, and what you can do</Link>). No channel lists or stream addresses come with the server: bring your own.</>}
      actions={<button className="btn sm" disabled={searching} onClick={() => void search()}><IconRefresh /> {searching ? 'Searching…' : 'Search for tuners'}</button>}>
      {found && found.length === 0 && <p className="muted">No new tuners found on your network or this machine. You can still add one below.</p>}
      {found && found.length > 0 && (
        <ul className="tvs-list">
          {found.map((f, i) => {
            const d = drivers.find((x) => x.id === f.driver)
            return (
              <li key={i} className="tvs-item">
                <div className="tvs-item-text"><b>{f.name}</b><span>{f.detail || d?.name}</span></div>
                {d && <button className="btn sm primary" onClick={() => setAdding({ driver: d, options: f.options, name: f.name })}><IconPlus /> Add</button>}
              </li>
            )
          })}
        </ul>
      )}
      {list && list.length > 0 && (
        <ul className="tvs-list">
          {list.map((t) => (
            <li key={t.id} className="tvs-item">
              <div className="tvs-item-text">
                <b>{t.name}</b>
                <span>{drivers.find((d) => d.id === t.driver)?.name || t.driver} · {t.channels} {t.channels === 1 ? 'channel' : 'channels'}{t.clear > 0 ? ` · ${t.clear} clear` : ''}{t.encrypted > 0 ? ` · ${t.encrypted} encrypted` : ''}{t.inUse > 0 ? ` · ${t.inUse} in use` : ''}</span>
                {t.lastError && <span className="tvs-error" role="alert">{t.lastError}</span>}
              </div>
              <Link className="btn sm ghost" to={`tuner/${t.id}`}><IconEdit /> Channels</Link>
              <button className="btn sm ghost" onClick={() => void remove(t)} aria-label={`Remove ${t.name}`}><IconTrash /></button>
            </li>
          ))}
        </ul>
      )}
      <h3 className="tvs-h3">Add a tuner</h3>
      <div className="tvs-drivers">
        {drivers.map((d) => (
          <div key={d.id} className={'tvs-driver' + (d.available ? '' : ' off')}>
            <div className="tvs-driver-head"><b>{d.name}</b><span className={'tvs-status ' + d.status} title={STATUS[d.status]?.desc}>{STATUS[d.status]?.label}</span></div>
            <p>{d.description}</p>
            {d.note && <p className="tvs-dim">{d.note}</p>}
            {d.available && <button className="btn sm" onClick={() => setAdding({ driver: d, options: {}, name: '' })}><IconPlus /> Add</button>}
          </div>
        ))}
      </div>
      {adding && <TunerForm driver={adding.driver} initial={adding.options} initialName={adding.name} onClose={() => setAdding(null)} onDone={() => { setAdding(null); setFound(null); forgetLineup(); void load() }} />}
    </Section>
  )
}

function Field({ f, value, onChange }: { f: DriverField; value: string; onChange: (v: string) => void }) {
  const id = 'tf-' + f.key
  const help = f.help ? <small id={id + '-h'} className="tvs-help">{f.help}</small> : null
  const aria = { id, 'aria-describedby': f.help ? id + '-h' : undefined, required: f.required }
  if (f.kind === 'toggle') {
    return <label className="check-row"><input type="checkbox" id={id} checked={(value || f.default) === '1'} onChange={(e) => onChange(e.target.checked ? '1' : '0')} /> {f.label}{help}</label>
  }
  return (
    <label htmlFor={id}>{f.label}{f.required ? ' (needed)' : ''}
      {f.kind === 'select' ? (
        <select className="select" {...aria} value={value || f.default || ''} onChange={(e) => onChange(e.target.value)}>
          {(f.options || []).map((o) => <option key={o} value={o}>{o || 'None'}</option>)}
        </select>
      ) : f.kind === 'textarea' ? (
        <textarea className="input mono" rows={4} {...aria} value={value} onChange={(e) => onChange(e.target.value)} spellCheck={false} />
      ) : (
        <input className="input" type={f.kind === 'password' ? 'password' : f.kind === 'number' ? 'number' : 'text'} autoComplete={f.kind === 'password' ? 'new-password' : 'off'} {...aria} value={value} placeholder={f.default} onChange={(e) => onChange(e.target.value)} />
      )}
      {help}
    </label>
  )
}

function TunerForm({ driver, initial, initialName, onClose, onDone }: { driver: Driver; initial: Record<string, string>; initialName: string; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState(initialName)
  const [opts, setOpts] = useState<Record<string, string>>(initial)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const save = async () => {
    setBusy(true); setErr('')
    try {
      await api.post('/tuners', { driver: driver.id, name: name.trim() || driver.name, options: opts })
      toast(`${name.trim() || driver.name} added`)
      onDone()
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal wide" role="dialog" aria-modal="true" aria-labelledby="tf-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="tf-h">Add {driver.name}</h2>
        <p className="muted">{driver.description}</p>
        {driver.status !== 'tested' && <p className="tvs-note">{STATUS[driver.status]?.desc}</p>}
        <form className="form" style={{ maxWidth: 'none' }} onSubmit={(e) => { e.preventDefault(); void save() }}>
          <label>Name<input className="input" value={name} onChange={(e) => setName(e.target.value)} maxLength={200} placeholder={driver.name} autoFocus /></label>
          {driver.fields.map((f) => <Field key={f.key} f={f} value={opts[f.key] || ''} onChange={(v) => setOpts({ ...opts, [f.key]: v })} />)}
          {err && <p className="tvs-error" role="alert">{err}</p>}
          <div className="actions">
            <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
            <button className="btn primary" disabled={busy}>{busy ? 'Adding…' : 'Add'}</button>
          </div>
        </form>
      </div>
    </div>
  )
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
  return (
    <>
      <PageHead title={t?.name || 'Tuner'} desc={t?.detail || driver?.name} actions={<Link className="btn" to="..">Back</Link>} />
      {t?.lastError && <p className="tvs-error" role="alert">{t.lastError}</p>}
      <Section title="Channels" desc={list ? `${list.length} channels, ${list.filter((c) => c.enabled).length} shown in the guide. What you change here stays when the list is read again.` : ''}
        actions={<>
          <button className="btn sm" onClick={() => void refresh()}><IconRefresh /> Read the list</button>
          {driver?.scans && <button className="btn sm primary" disabled={scan?.running} onClick={() => void startScan(opts?.source || '')}>{scan?.running ? `Scanning… ${scan.percent}%` : 'Scan for channels'}</button>}
        </>}>
        {scan?.running && <div className="tvs-progress" role="progressbar" aria-valuenow={scan.percent} aria-valuemin={0} aria-valuemax={100} aria-label="Scan progress"><i style={{ width: scan.percent + '%' }} /><span>{scan.found} found{scan.message ? ` · ${scan.message}` : ''}</span></div>}
        {scan?.error && <p className="tvs-error" role="alert">{scan.error}</p>}
        {scan && !scan.running && !scan.error && <p className="tvs-note" role="status">The scan found {scan.clear} clear and {scan.encrypted} encrypted {scan.clear + scan.encrypted === 1 ? 'channel' : 'channels'}{scan.unknown ? `, and ${scan.unknown} not checked yet` : ''}.{scan.encrypted > 0 && <> Encrypted channels can't be played here, which isn't a fault in your setup. <Link to="/tv/encrypted">What can I do?</Link></>}</p>}
        <div className="tvs-toolbar">
          <input className="input" type="search" aria-label="Find a channel" placeholder="Find a channel…" value={q} onChange={(e) => setQ(e.target.value)} />
          <button className="btn sm ghost" onClick={() => void all(true)}>Show all</button>
          <button className="btn sm ghost" onClick={() => void all(false)}>Hide all</button>
        </div>
        <div className="tvs-table-wrap">
          <table className="tvs-table">
            <thead><tr><th scope="col">In guide</th><th scope="col">Number</th><th scope="col">Name</th><th scope="col">Status</th><th scope="col">Can record</th><th scope="col">Guide id</th></tr></thead>
            <tbody>
              {shown.map((c) => (
                <tr key={c.id} className={c.encrypted ? 'locked' : ''}>
                  <td><input type="checkbox" aria-label={`${c.name} shows in the guide`} checked={c.enabled} onChange={(e) => void edit(c, { enabled: e.target.checked })} /></td>
                  <td><input className="input tvs-num-in" aria-label={`Number of ${c.name}`} defaultValue={c.number} onBlur={(e) => { if (e.target.value !== c.number) void edit(c, { number: e.target.value }) }} /></td>
                  <td>
                    <input className="input" aria-label={`Name of ${c.name}`} defaultValue={c.name} onBlur={(e) => { if (e.target.value !== c.name) void edit(c, { name: e.target.value }) }} />
                    {c.group && <small className="tvs-dim">{c.group}</small>}
                  </td>
                  <td>
                    {c.security === 'encrypted' && <small className="tvs-lock">Encrypted: can't be played here. {c.encrypted}. <Link to="/tv/encrypted">What can I do?</Link> <button className="btn sm ghost" onClick={() => void edit(c, { checkAgain: true })} title="If the station has stopped encrypting: it is checked again next time it is tuned">Check again</button></small>}
                    {c.security === 'clear' && <small className="tvs-clear">Clear</small>}
                    {c.security === 'unknown' && <small className="tvs-dim" title="This source doesn't say. It is checked when you tune in.">Not checked yet</small>}
                    {c.atsc3 && <small className="tvs-dim">ATSC 3.0{c.audioCodec ? ` · ${c.videoCodec || ''} ${c.audioCodec}` : ''}</small>}
                  </td>
                  <td><input type="checkbox" aria-label={`${c.name} can be recorded`} disabled={!!c.encrypted} checked={!c.noRecord && !c.encrypted} title={c.noRecord || 'Untick if the device says this may not be recorded'} onChange={(e) => void edit(c, { noRecord: !e.target.checked })} /></td>
                  <td><input className="input mono" aria-label={`Guide id of ${c.name}`} defaultValue={c.epgSet || c.epgId || ''} onBlur={(e) => { if (e.target.value !== (c.epgSet || c.epgId || '')) void edit(c, { epg: e.target.value }) }} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {list && shown.length < list.length && shown.length === 500 && <p className="muted">Showing the first 500. Use the search box to find others.</p>}
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
      {list && list.length > 0 && (
        <ul className="tvs-list">
          {list.map((s) => (
            <li key={s.id} className="tvs-item">
              <div className="tvs-item-text">
                <b>{s.name}</b>
                <span>{s.lastFetch ? `${s.programmes} listings · read ${new Date(s.lastFetch * 1000).toLocaleString()}` : 'Not read yet'}</span>
                {s.lastError && <span className="tvs-error" role="alert">{s.lastError}</span>}
              </div>
              <button className="btn sm ghost" onClick={() => void refresh(s)} aria-label={`Read ${s.name} again`}><IconRefresh /></button>
              <button className="btn sm ghost" onClick={() => void remove(s)} aria-label={`Remove ${s.name}`}><IconTrash /></button>
            </li>
          ))}
        </ul>
      )}
      <form className="tvs-toolbar" onSubmit={(e) => { e.preventDefault(); void add() }}>
        <input className="input" aria-label="XMLTV address or file on the server" placeholder="XMLTV address, or a file on the server" value={url} onChange={(e) => setUrl(e.target.value)} />
        <button className="btn sm" disabled={!url.trim()}><IconPlus /> Add</button>
      </form>
    </Section>
  )
}

// --------------------------------------------------------------------- options

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
      <Row label="Guide layout" desc="What people start with. Everyone can change the layout, colours and preview for their own device with the Look button on the guide.">
        <select className="select" value={s.tvLayout} onChange={(e) => void set('tvLayout', e.target.value)}>
          <option value="classic">Classic box</option><option value="streaming">Streaming</option>
        </select>
      </Row>
      <Row label="Guide colours">
        <select className="select" value={s.tvSkin} onChange={(e) => void set('tvSkin', e.target.value)}>
          <option value="cable">Cable blue</option><option value="satellite">Satellite black and amber</option><option value="theme">The server's theme</option>
        </select>
      </Row>
      <Row label="Pass H.264 through" desc="Video the browser can already play is sent as it is. Much lighter on the machine; turn it off only if channels stutter when shows change.">
        <Switch checked={s.tvCopyVideo === '1'} label="Pass H.264 through" onChange={(v) => void set('tvCopyVideo', v ? '1' : '0')} />
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
