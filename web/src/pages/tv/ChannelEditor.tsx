import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import { api, fmtRuntime, mediaUrl, type Item, type UserView } from '../../api'
import { IconPlus, IconTrash } from '../../icons'
import { toast, useApp } from '../../state'
import { PageHead, Section } from '../settings/ui'
import { DAYS, emptyDefinition, type CommercialPick, fmtClock, forgetLineup, fullDay, uploadLogo, type Block, type Channel, type Definition, type EditorOptions, type Order, type PoolStats, type Programme, type Rules, type Suggestion, type Visibility } from './tvApi'

// One channel, as much or as little as you want. Simple, Rules and Timetable
// are three views of the same definition: going from one to another changes
// nothing that was set. A draft stays in this browser until it is saved, and
// each save keeps the version it replaced.

const ORDERS: { v: Order['mode']; label: string; hint: string }[] = [
  { v: 'rotate', label: 'Take turns', hint: 'The next episode of each show in turn, like a station running reruns' },
  { v: 'sequential', label: 'In order', hint: 'Every episode of one show, then the next show' },
  { v: 'aired', label: 'As first aired', hint: 'By original air date, across all shows' },
  { v: 'shuffle', label: 'Shuffle', hint: 'Nothing repeats until everything has played' },
]
const ALIGN = [{ v: 0, label: 'Back to back' }, { v: 5, label: 'Every 5 minutes' }, { v: 15, label: 'Quarter hours' }, { v: 30, label: 'Half hours' }, { v: 60, label: 'On the hour' }]
const FITS: { v: NonNullable<Block['fit']>; label: string }[] = [
  { v: 'fill', label: 'Finish on time: fill the gap' }, { v: 'overrun', label: 'Let the programme finish' }, { v: 'cut', label: 'Cut it at the end of the slot' },
]

type Draft = { name: string; number: string; description: string; definition: Definition; visibility: Visibility; countProgress: boolean; enabled: boolean }

const blankDraft = (): Draft => ({ name: '', number: '', description: '', definition: emptyDefinition(), visibility: { mode: '' }, countProgress: false, enabled: true })
const newBlockId = () => 'b' + Math.random().toString(36).slice(2, 9)
const clean = <T extends object>(o: T): T => JSON.parse(JSON.stringify(o, (_, v) => (v === '' || (Array.isArray(v) && v.length === 0) ? undefined : v)))

export default function ChannelEditor() {
  const { id } = useParams()
  const loc = useLocation()
  const nav = useNavigate()
  const draftKey = 'wms.channelDraft.' + (id || 'new')
  const [draft, setDraft] = useState<Draft | null>(null)
  const [saved, setSaved] = useState<Channel | null>(null)
  const [opts, setOpts] = useState<EditorOptions | null>(null)
  const [restored, setRestored] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [logo, setLogo] = useState('')
  const [history, setHistory] = useState<{ id: number; savedAt: number; name: string }[]>([])
  const baseline = useRef('')

  useEffect(() => { api.get<EditorOptions>('/channels/options').then(setOpts).catch((e) => toast(e.message)) }, [])
  useEffect(() => {
    const finish = (d: Draft) => {
      baseline.current = JSON.stringify(d)
      try {
        const raw = localStorage.getItem(draftKey)
        if (raw && raw !== baseline.current) { setDraft(JSON.parse(raw)); setRestored(true); return }
      } catch { /* a draft that can't be read is no draft */ }
      setDraft(d)
    }
    if (id) {
      api.get<Channel>(`/channels/${id}`).then((c) => {
        setSaved(c); setLogo(c.logo)
        finish({ name: c.name, number: c.number, description: c.description, definition: { ...emptyDefinition(), ...c.definition, blocks: c.definition.blocks || [] }, visibility: c.visibility || { mode: '' }, countProgress: c.countProgress, enabled: c.enabled })
      }).catch((e) => setErr(e.message))
      api.get<typeof history>(`/channels/${id}/history`).then(setHistory).catch(() => {})
    } else {
      const s = (loc.state as { suggestion?: Suggestion } | null)?.suggestion
      const d = blankDraft()
      if (s) { d.name = s.name; d.description = s.description; d.definition = { ...emptyDefinition(), ...s.definition, blocks: s.definition.blocks || [] } }
      finish(d)
    }
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  // A closed tab, or a browser that crashes, loses nothing.
  useEffect(() => {
    if (!draft) return
    const t = setTimeout(() => {
      try {
        if (JSON.stringify(draft) === baseline.current) localStorage.removeItem(draftKey)
        else localStorage.setItem(draftKey, JSON.stringify(draft))
      } catch { /* full or blocked storage */ }
    }, 400)
    return () => clearTimeout(t)
  }, [draft, draftKey])
  const dirty = !!draft && JSON.stringify(draft) !== baseline.current
  useEffect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])

  const setDef = (patch: Partial<Definition>) => setDraft((d) => (d ? { ...d, definition: { ...d.definition, ...patch } } : d))
  const setRules = (patch: Partial<Rules>) => setDraft((d) => (d ? { ...d, definition: { ...d.definition, rules: { ...d.definition.rules, ...patch } } } : d))

  const save = async () => {
    if (!draft) return
    setBusy(true); setErr('')
    const body = { ...draft, definition: { ...draft.definition, rules: clean(draft.definition.rules), blocks: draft.definition.blocks || [] } }
    try {
      if (id) await api.put(`/channels/${id}`, body)
      else await api.post('/channels', body)
      try { localStorage.removeItem(draftKey) } catch { /* ignore */ }
      baseline.current = JSON.stringify(draft)
      forgetLineup()
      toast(id ? 'Saved. Changes start after what is on now.' : `${draft.name} is on the air`)
      nav('/settings/channels')
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const discard = () => {
    try { localStorage.removeItem(draftKey) } catch { /* ignore */ }
    setRestored(false)
    if (baseline.current) setDraft(JSON.parse(baseline.current))
  }
  const onLogo = async (f: File | undefined) => {
    if (!f || !id) return
    try { const r = await uploadLogo(Number(id), f); setLogo(r.logo); toast('Logo saved') } catch (e: any) { toast(e.message) }
  }
  const removeLogo = async () => { try { await api.del(`/channels/${id}/logo`); setLogo('') } catch (e: any) { toast(e.message) } }
  const restore = async (h: number) => {
    if (!confirm('Go back to this version? The current one is kept in this list too.')) return
    try {
      await api.post(`/channels/${id}/restore`, { history: h })
      try { localStorage.removeItem(draftKey) } catch { /* ignore */ }
      toast('Restored')
      window.location.reload()
    } catch (e: any) { toast(e.message) }
  }
  const reshuffle = async () => {
    try { await api.post(`/channels/${id}/rebuild?reshuffle=1`); toast('New schedule from after what is on now') } catch (e: any) { toast(e.message) }
  }

  if (err && !draft) return <p className="tvs-error" role="alert">{err}</p>
  if (!draft) return <div className="center"><div className="spinner" /></div>
  const def = draft.definition
  const levels: { v: Definition['mode']; label: string; hint: string }[] = [
    { v: 'simple', label: 'Simple', hint: 'What it plays. That is all it needs.' },
    { v: 'rules', label: 'Rules', hint: 'Exactly what it may play, day parts and bumpers.' },
    { v: 'timetable', label: 'Full timetable', hint: 'Your own week, slot by slot.' },
  ]
  return (
    <>
      <PageHead title={id ? `Edit ${saved?.name || 'channel'}` : 'New channel'} desc="Set as little or as much as you like. Moving between the levels below never loses anything."
        actions={<Link className="btn" to="/settings/channels">Back</Link>} />
      {restored && <p className="tvs-note" role="status">Your unsaved changes from last time are back. <button className="btn sm ghost" onClick={discard}>Discard them</button></p>}

      <Section title="The channel">
        <div className="form" style={{ maxWidth: 'none' }}>
          <div className="tvs-two">
            <label>Name<input className="input" value={draft.name} maxLength={200} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="Saturday Morning" /></label>
            <label>Number<input className="input tvs-short" value={draft.number} maxLength={12} onChange={(e) => setDraft({ ...draft, number: e.target.value })} placeholder="Next free" /></label>
          </div>
          <label>Description<input className="input" value={draft.description} maxLength={2000} onChange={(e) => setDraft({ ...draft, description: e.target.value })} /></label>
        </div>
      </Section>

      <div className="tvs-levels" role="radiogroup" aria-label="How much do you want to set up?">
        {levels.map((l) => (
          <button key={l.v} type="button" role="radio" aria-checked={def.mode === l.v} className={'tvs-level' + (def.mode === l.v ? ' on' : '')} onClick={() => setDef({ mode: l.v })}>
            <b>{l.label}</b><span>{l.hint}</span>
          </button>
        ))}
      </div>

      {def.mode === 'simple' && opts && <SimpleLevel def={def} setRules={setRules} setDef={setDef} opts={opts} />}
      {def.mode === 'rules' && opts && <RulesLevel def={def} setRules={setRules} setDef={setDef} opts={opts} />}
      {def.mode === 'timetable' && opts && <TimetableLevel def={def} setDef={setDef} setRules={setRules} opts={opts} />}

      <CommercialsSection def={def} setDef={setDef} />
      {def.mode !== 'simple' && opts && <FillerSection def={def} setDef={setDef} opts={opts} />}

      <Preview def={def} />

      <Section title="Who sees it">
        <Audience draft={draft} setDraft={setDraft} />
      </Section>

      {id && (
        <Section title="More">
          <div className="tvs-toolbar">
            {logo && <img className="tvs-logo" src={mediaUrl(`/api/channels/${id}/logo?v=${logo}`)} alt="Current logo" />}
            <label className="btn sm">Upload your own logo<input className="sr-only" type="file" accept="image/png,image/jpeg" onChange={(e) => void onLogo(e.target.files?.[0])} /></label>
            {logo && <button className="btn sm ghost" onClick={() => void removeLogo()}>Remove the logo</button>}
            <button className="btn sm ghost" onClick={() => void reshuffle()}>Make a new schedule</button>
          </div>
          <p className="muted" style={{ fontSize: 13 }}>No logos come with the server. Use only pictures you have the right to use.</p>
          {history.length > 0 && (
            <details className="tvs-details">
              <summary>Earlier versions ({history.length})</summary>
              <ul className="tvs-list">
                {history.map((h) => (
                  <li key={h.id} className="tvs-item">
                    <div className="tvs-item-text"><b>{h.name}</b><span>{new Date(h.savedAt * 1000).toLocaleString()}</span></div>
                    <button className="btn sm ghost" onClick={() => void restore(h.id)}>Go back to this</button>
                  </li>
                ))}
              </ul>
            </details>
          )}
        </Section>
      )}

      {err && <p className="tvs-error" role="alert">{err}</p>}
      {(dirty || !id) && (
        <div className="set-savebar" role="region" aria-label="Save this channel">
          <span>{dirty && <><i className="set-dot" />Unsaved changes, kept in this browser</>}</span>
          {dirty && <button className="btn ghost sm" onClick={discard} disabled={busy}>Discard</button>}
          <button className="btn primary sm" onClick={() => void save()} disabled={busy || !draft.name.trim()}>{busy ? 'Saving…' : id ? 'Save changes' : 'Create the channel'}</button>
        </div>
      )}
    </>
  )
}

// A long list to choose several from, with a filter when it is worth having.
function Multi({ label, options, value, onChange }: { label: string; options: { v: string; label: string }[]; value: string[]; onChange: (v: string[]) => void }) {
  const [q, setQ] = useState('')
  if (options.length === 0) return null
  const shown = options.filter((o) => !q || o.label.toLowerCase().includes(q.toLowerCase())).slice(0, 300)
  const toggle = (v: string, on: boolean) => onChange(on ? [...value, v] : value.filter((x) => x !== v))
  return (
    <fieldset className="tvs-multi">
      <legend>{label}{value.length > 0 && <span className="tvs-count"> · {value.length} chosen</span>}</legend>
      {options.length > 12 && <input className="input" type="search" aria-label={`Find in ${label}`} placeholder="Find…" value={q} onChange={(e) => setQ(e.target.value)} />}
      <div className="tvs-chips">
        {shown.map((o) => (
          <label key={o.v} className={'tvs-chip' + (value.includes(o.v) ? ' on' : '')}>
            <input type="checkbox" checked={value.includes(o.v)} onChange={(e) => toggle(o.v, e.target.checked)} /> {o.label}
          </label>
        ))}
      </div>
    </fieldset>
  )
}

const strs = (l?: (string | number)[]) => (l || []).map(String)
const showOpts = (o: EditorOptions) => o.shows.map((s) => ({ v: String(s.id), label: s.title + (s.year ? ` (${s.year})` : '') }))

function OrderPick({ order, onChange }: { order: Order; onChange: (o: Order) => void }) {
  return (
    <div className="tvs-two">
      <label>How it plays
        <select className="select" value={order.mode || 'rotate'} onChange={(e) => onChange({ ...order, mode: e.target.value as Order['mode'] })}>
          {ORDERS.map((o) => <option key={o.v} value={o.v} title={o.hint}>{o.label}</option>)}
        </select>
        <small className="tvs-help">{ORDERS.find((o) => o.v === (order.mode || 'rotate'))?.hint}</small>
      </label>
      {order.mode === 'shuffle' && <label className="check-row"><input type="checkbox" checked={!!order.keepEpisodeOrder} onChange={(e) => onChange({ ...order, keepEpisodeOrder: e.target.checked })} /> Keep each show's episodes in order</label>}
    </div>
  )
}

function SimpleLevel({ def, setRules, setDef, opts }: { def: Definition; setRules: (p: Partial<Rules>) => void; setDef: (p: Partial<Definition>) => void; opts: EditorOptions }) {
  const r = def.rules
  const pick = r.networks?.length ? 'network' : r.genres?.length ? 'genre' : r.decades?.length ? 'decade' : r.shows?.length ? 'show' : r.kinds?.includes('movies') && r.kinds.length === 1 ? 'movies' : 'all'
  const [kind, setKind] = useState(pick)
  const change = (k: string) => {
    setKind(k)
    setRules({ networks: undefined, genres: undefined, decades: undefined, shows: undefined, kinds: k === 'movies' ? ['movies'] : undefined })
  }
  return (
    <Section title="What it plays">
      <div className="form" style={{ maxWidth: 'none' }}>
        <label>Play
          <select className="select" value={kind} onChange={(e) => change(e.target.value)}>
            <option value="all">Everything</option>
            {opts.networks.length > 0 && <option value="network">One TV network</option>}
            <option value="genre">A genre</option>
            <option value="decade">A decade</option>
            <option value="show">One show</option>
            <option value="movies">Films</option>
          </select>
        </label>
        {kind === 'network' && (
          <label>Network
            <select className="select" value={r.networks?.[0] || ''} onChange={(e) => setRules({ networks: e.target.value ? [e.target.value] : undefined })}>
              <option value="">Choose…</option>
              {opts.networks.map((n) => <option key={n.name} value={n.name}>{n.name} ({n.shows} {n.shows === 1 ? 'show' : 'shows'}, {n.episodes} episodes)</option>)}
            </select>
          </label>
        )}
        {kind === 'genre' && (
          <label>Genre
            <select className="select" value={r.genres?.[0] || ''} onChange={(e) => setRules({ genres: e.target.value ? [e.target.value] : undefined })}>
              <option value="">Choose…</option>
              {opts.genres.map((g) => <option key={g} value={g}>{g}</option>)}
            </select>
          </label>
        )}
        {kind === 'decade' && (
          <label>Decade
            <select className="select" value={r.decades?.[0] || ''} onChange={(e) => setRules({ decades: e.target.value ? [Number(e.target.value)] : undefined })}>
              <option value="">Choose…</option>
              {opts.decades.map((d) => <option key={d} value={d}>{d}s</option>)}
            </select>
          </label>
        )}
        {kind === 'show' && (
          <label>Show
            <select className="select" value={r.shows?.[0] || ''} onChange={(e) => setRules({ shows: e.target.value ? [Number(e.target.value)] : undefined })}>
              <option value="">Choose…</option>
              {opts.shows.map((s) => <option key={s.id} value={s.id}>{s.title}{s.year ? ` (${s.year})` : ''}</option>)}
            </select>
          </label>
        )}
        <OrderPick order={def.order} onChange={(o) => setDef({ order: o })} />
        <label>Programmes start
          <select className="select" value={def.alignMin} onChange={(e) => setDef({ alignMin: Number(e.target.value) })}>
            {ALIGN.map((a) => <option key={a.v} value={a.v}>{a.label}</option>)}
          </select>
          <small className="tvs-help">Real TV starts on the half hour. The gaps get a “We'll be right back” card, or your own bumpers (under Rules).</small>
        </label>
      </div>
    </Section>
  )
}

function RuleFields({ r, set, opts, compact }: { r: Rules; set: (p: Partial<Rules>) => void; opts: EditorOptions; compact?: boolean }) {
  const { libraries } = useApp()
  const num = (k: keyof Rules) => (e: React.ChangeEvent<HTMLInputElement>) => set({ [k]: e.target.value === '' ? undefined : Number(e.target.value) } as Partial<Rules>)
  const val = (v?: number) => (v === undefined || v === 0 ? '' : v)
  return (
    <div className="form" style={{ maxWidth: 'none' }}>
      {!compact && libraries.length > 1 && <Multi label="Libraries" options={libraries.map((l) => ({ v: String(l.id), label: l.name }))} value={strs(r.libraries)} onChange={(v) => set({ libraries: v.map(Number) })} />}
      <Multi label="Kind of title" options={[{ v: 'episodes', label: 'TV episodes' }, { v: 'movies', label: 'Films' }, { v: 'discs', label: 'DVDs and Blu-rays' }]} value={r.kinds || []} onChange={(v) => set({ kinds: v })} />
      <Multi label="TV networks (or a film's studio)" options={opts.networks.map((n) => ({ v: n.name, label: `${n.name} (${n.shows})` }))} value={r.networks || []} onChange={(v) => set({ networks: v })} />
      <Multi label="Genres" options={opts.genres.map((g) => ({ v: g, label: g }))} value={r.genres || []} onChange={(v) => set({ genres: v })} />
      <Multi label="Shows" options={showOpts(opts)} value={strs(r.shows)} onChange={(v) => set({ shows: v.map(Number) })} />
      <Multi label="Decades" options={opts.decades.map((d) => ({ v: String(d), label: d + 's' }))} value={strs(r.decades)} onChange={(v) => set({ decades: v.map(Number) })} />
      {!compact && (
        <>
          <Multi label="Leave out these genres" options={opts.genres.map((g) => ({ v: g, label: g }))} value={r.excludeGenres || []} onChange={(v) => set({ excludeGenres: v })} />
          <Multi label="Leave out these shows" options={showOpts(opts)} value={strs(r.excludeShows)} onChange={(v) => set({ excludeShows: v.map(Number) })} />
          <Multi label="Studios" options={opts.studios.map((g) => ({ v: g, label: g }))} value={r.studios || []} onChange={(v) => set({ studios: v })} />
          <Multi label="Certificates" options={opts.ratings.map((g) => ({ v: g, label: g }))} value={r.ratings || []} onChange={(v) => set({ ratings: v })} />
          <div className="tvs-three">
            <label>From year<input className="input" type="number" min={1880} max={2200} value={val(r.yearFrom)} onChange={num('yearFrom')} /></label>
            <label>To year<input className="input" type="number" min={1880} max={2200} value={val(r.yearTo)} onChange={num('yearTo')} /></label>
            <label>Lowest score (0–10)<input className="input" type="number" step={0.5} min={0} max={10} value={val(r.minScore)} onChange={num('minScore')} /></label>
            <label>From season<input className="input" type="number" min={0} value={val(r.seasonFrom)} onChange={num('seasonFrom')} /></label>
            <label>To season<input className="input" type="number" min={0} value={val(r.seasonTo)} onChange={num('seasonTo')} /></label>
            <label>Highest age rating<input className="input" type="number" min={0} max={21} value={r.maxAge ?? ''} onChange={(e) => set({ maxAge: e.target.value === '' ? undefined : Number(e.target.value) })} /></label>
            <label>At least (minutes)<input className="input" type="number" min={0} value={val(r.minMinutes)} onChange={num('minMinutes')} /></label>
            <label>At most (minutes)<input className="input" type="number" min={0} value={val(r.maxMinutes)} onChange={num('maxMinutes')} /></label>
            <label>Watched
              <select className="select" value={r.watched || ''} onChange={(e) => set({ watched: e.target.value as Rules['watched'] })}>
                <option value="">Either</option><option value="unwatched">Not yet watched</option><option value="watched">Already watched</option>
              </select>
              <small className="tvs-help">Going by your own watch history.</small>
            </label>
          </div>
          <label>Title contains<input className="input" value={r.search || ''} maxLength={200} onChange={(e) => set({ search: e.target.value })} /></label>
          <label className="check-row"><input type="checkbox" checked={!!r.allowUnrated} onChange={(e) => set({ allowUnrated: e.target.checked })} /> With an age rating set: also play titles that have no rating</label>
          <label className="check-row"><input type="checkbox" checked={!!r.includeSpecials} onChange={(e) => set({ includeSpecials: e.target.checked })} /> Include specials</label>
        </>
      )}
    </div>
  )
}

function RulesLevel({ def, setRules, setDef, opts }: { def: Definition; setRules: (p: Partial<Rules>) => void; setDef: (p: Partial<Definition>) => void; opts: EditorOptions }) {
  return (
    <>
      <Section title="What it may play" desc="Leave anything empty and it does not limit the channel.">
        <RuleFields r={def.rules} set={setRules} opts={opts} />
      </Section>
      <Section title="How it plays">
        <div className="form" style={{ maxWidth: 'none' }}>
          <OrderPick order={def.order} onChange={(o) => setDef({ order: o })} />
          <label>Programmes start
            <select className="select" value={def.alignMin} onChange={(e) => setDef({ alignMin: Number(e.target.value) })}>
              {ALIGN.map((a) => <option key={a.v} value={a.v}>{a.label}</option>)}
            </select>
          </label>
        </div>
      </Section>
      <DayParts def={def} setDef={setDef} opts={opts} />
    </>
  )
}

const CLASSIC_PARTS: { name: string; start: string; end: string; genres: string[] }[] = [
  { name: 'Morning', start: '06:00', end: '12:00', genres: ['Children', 'Kids', 'Family', 'Animation'] },
  { name: 'Afternoon', start: '12:00', end: '17:00', genres: ['Soap', 'Game Show', 'Reality', 'Drama'] },
  { name: 'Early evening', start: '17:00', end: '20:00', genres: ['Comedy', 'Family', 'Animation'] },
  { name: 'Prime time', start: '20:00', end: '23:00', genres: ['Drama', 'Crime', 'Action', 'Thriller', 'Comedy'] },
  { name: 'Late night', start: '23:00', end: '02:00', genres: ['Horror', 'Comedy', 'Thriller'] },
]

function DayParts({ def, setDef, opts }: { def: Definition; setDef: (p: Partial<Definition>) => void; opts: EditorOptions }) {
  const blocks = def.blocks || []
  const addClassic = () => {
    const known = (g: string[]) => g.filter((x) => opts.genres.some((o) => o.toLowerCase() === x.toLowerCase()))
    const parts = CLASSIC_PARTS.filter((p) => known(p.genres).length > 0).map((p): Block => ({ id: newBlockId(), name: p.name, start: p.start, end: p.end, rules: { ...def.rules, genres: known(p.genres) } }))
    if (parts.length === 0) { toast('Your library has none of the genres the classic day parts use'); return }
    setDef({ blocks: [...blocks, ...parts] })
  }
  return (
    <Section title="Day parts" desc="Different things at different times: Saturday morning cartoons, prime time drama. The rest of the day plays the rules above."
      actions={<button className="btn sm" onClick={addClassic}>Add the classic day parts</button>}>
      <BlockList def={def} setDef={setDef} opts={opts} />
    </Section>
  )
}

function TimetableLevel({ def, setDef, setRules, opts }: { def: Definition; setDef: (p: Partial<Definition>) => void; setRules: (p: Partial<Rules>) => void; opts: EditorOptions }) {
  const [busy, setBusy] = useState(false)
  const net = def.rules.networks?.[0] || opts.networks[0]?.name || ''
  const asAired = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ definition: Definition }>('/channels/from-network', { network: net, style: 'asaired' })
      if (!r.definition.blocks?.length) { toast('The metadata does not say when these shows aired. Try “Fill in missing facts” in the channel list.'); return }
      setDef({ blocks: [...(def.blocks || []), ...r.definition.blocks] })
      setRules({ networks: [net] })
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  return (
    <>
      <Section title="The week" desc="Slots repeat every week. Time no slot covers plays the channel's rules, if it has any, or a “We'll be right back” card."
        actions={net ? <button className="btn sm" disabled={busy} onClick={() => void asAired()}>Fill in from when {net} shows aired</button> : undefined}>
        <WeekView blocks={def.blocks || []} />
        <BlockList def={def} setDef={setDef} opts={opts} />
      </Section>
    </>
  )
}

// A picture of the week. The list below is the same information for anyone not
// looking at it, so this is hidden from screen readers.
function WeekView({ blocks }: { blocks: Block[] }) {
  const toPct = (s: string) => ((Number(s.slice(0, 2)) * 60 + Number(s.slice(3))) / 1440) * 100
  return (
    <div className="tvs-week" aria-hidden="true">
      {DAYS.map((d, i) => (
        <div key={d} className="tvs-week-day">
          <div className="tvs-week-name">{d}</div>
          <div className="tvs-week-track">
            {blocks.filter((b) => !b.days?.length || b.days.includes(i)).map((b) => {
              const s = toPct(b.start)
              const e = toPct(b.end)
              return <i key={b.id} style={{ top: s + '%', height: Math.max(1.5, (e > s ? e : 100) - s) + '%' }} title={`${b.name || 'Slot'} ${b.start}–${b.end}`} />
            })}
          </div>
        </div>
      ))}
    </div>
  )
}

type Pick = { kind: '' | 'show' | 'genre' | 'network' | 'decade' | 'custom'; value: string }
const pickOf = (r?: Rules): Pick => {
  if (!r) return { kind: '', value: '' }
  const keys = Object.entries(r).filter(([, v]) => v !== undefined && v !== '' && !(Array.isArray(v) && v.length === 0)).map(([k]) => k)
  if (keys.length === 1) {
    if (r.shows?.length === 1) return { kind: 'show', value: String(r.shows[0]) }
    if (r.genres?.length === 1) return { kind: 'genre', value: r.genres[0] }
    if (r.networks?.length === 1) return { kind: 'network', value: r.networks[0] }
    if (r.decades?.length === 1) return { kind: 'decade', value: String(r.decades[0]) }
  }
  return { kind: 'custom', value: '' }
}

function BlockList({ def, setDef, opts }: { def: Definition; setDef: (p: Partial<Definition>) => void; opts: EditorOptions }) {
  const blocks = def.blocks || []
  const up = (id: string, patch: Partial<Block>) => setDef({ blocks: blocks.map((b) => (b.id === id ? { ...b, ...patch } : b)) })
  const remove = (id: string) => setDef({ blocks: blocks.filter((b) => b.id !== id) })
  const copy = (b: Block) => setDef({ blocks: [...blocks, { ...b, id: newBlockId() }] })
  const add = () => setDef({ blocks: [...blocks, { id: newBlockId(), start: '18:00', end: '18:30', days: [1, 2, 3, 4, 5] }] })
  const sorted = [...blocks].sort((a, b) => a.start.localeCompare(b.start))
  return (
    <>
      {sorted.length === 0 && <p className="muted">Nothing here yet.</p>}
      <ul className="tvs-blocks">
        {sorted.map((b, i) => <BlockEditor key={b.id} n={i + 1} b={b} opts={opts} up={(p) => up(b.id, p)} remove={() => remove(b.id)} copy={() => copy(b)} />)}
      </ul>
      <button className="btn sm" onClick={add}><IconPlus /> Add a slot</button>
    </>
  )
}

function BlockEditor({ n, b, opts, up, remove, copy }: { n: number; b: Block; opts: EditorOptions; up: (p: Partial<Block>) => void; remove: () => void; copy: () => void }) {
  const pick = pickOf(b.rules)
  const [open, setOpen] = useState(pick.kind === 'custom')
  const days = b.days || []
  const toggleDay = (d: number) => up({ days: days.includes(d) ? days.filter((x) => x !== d) : [...days, d].sort() })
  const setPick = (kind: Pick['kind'], value = '') => {
    switch (kind) {
      case '': up({ rules: undefined }); break
      case 'show': up({ rules: value ? { shows: [Number(value)] } : {} }); break
      case 'genre': up({ rules: value ? { genres: [value] } : {} }); break
      case 'network': up({ rules: value ? { networks: [value] } : {} }); break
      case 'decade': up({ rules: value ? { decades: [Number(value)] } : {} }); break
      case 'custom': setOpen(true); up({ rules: b.rules || {} }); break
    }
  }
  const label = b.name || `Slot ${n}`
  return (
    <li className="tvs-block">
      <div className="tvs-block-top">
        <label className="tvs-block-name">Name<input className="input" value={b.name || ''} maxLength={200} placeholder={`Slot ${n}`} onChange={(e) => up({ name: e.target.value })} /></label>
        <label>Starts<input className="input" type="time" value={b.start} onChange={(e) => up({ start: e.target.value })} /></label>
        <label>Ends<input className="input" type="time" value={b.end} onChange={(e) => up({ end: e.target.value })} /></label>
        <button className="btn sm ghost" onClick={copy} aria-label={`Copy ${label}`}>Copy</button>
        <button className="btn sm ghost" onClick={remove} aria-label={`Remove ${label}`}><IconTrash /></button>
      </div>
      <div className="tvs-days" role="group" aria-label={`Days of ${label}`}>
        {DAYS.map((d, i) => <button key={d} type="button" aria-pressed={days.includes(i)} aria-label={fullDay[i]} className={'tvs-day' + (days.includes(i) ? ' on' : '')} onClick={() => toggleDay(i)}>{d}</button>)}
        <span className="tvs-dim">{days.length === 0 ? 'Every day' : ''}</span>
      </div>
      <div className="tvs-three">
        <label>Plays
          <select className="select" value={open && pick.kind === 'custom' ? 'custom' : pick.kind} onChange={(e) => setPick(e.target.value as Pick['kind'])}>
            <option value="">The channel's rules</option>
            <option value="show">One show</option>
            <option value="genre">A genre</option>
            {opts.networks.length > 0 && <option value="network">A network</option>}
            <option value="decade">A decade</option>
            <option value="custom">Its own rules…</option>
          </select>
        </label>
        {pick.kind === 'show' && <label>Show<select className="select" value={pick.value} onChange={(e) => setPick('show', e.target.value)}><option value="">Choose…</option>{opts.shows.map((s) => <option key={s.id} value={s.id}>{s.title}</option>)}</select></label>}
        {pick.kind === 'genre' && <label>Genre<select className="select" value={pick.value} onChange={(e) => setPick('genre', e.target.value)}><option value="">Choose…</option>{opts.genres.map((g) => <option key={g} value={g}>{g}</option>)}</select></label>}
        {pick.kind === 'network' && <label>Network<select className="select" value={pick.value} onChange={(e) => setPick('network', e.target.value)}><option value="">Choose…</option>{opts.networks.map((x) => <option key={x.name} value={x.name}>{x.name}</option>)}</select></label>}
        {pick.kind === 'decade' && <label>Decade<select className="select" value={pick.value} onChange={(e) => setPick('decade', e.target.value)}><option value="">Choose…</option>{opts.decades.map((d) => <option key={d} value={d}>{d}s</option>)}</select></label>}
        <label>Order
          <select className="select" value={b.order?.mode || ''} onChange={(e) => up({ order: e.target.value ? { mode: e.target.value as Order['mode'], keepEpisodeOrder: b.order?.keepEpisodeOrder } : undefined })}>
            <option value="">As the channel</option>
            {ORDERS.map((o) => <option key={o.v} value={o.v}>{o.label}</option>)}
          </select>
        </label>
        <label>If it doesn't fit
          <select className="select" value={b.fit || 'fill'} onChange={(e) => up({ fit: e.target.value as Block['fit'] })}>
            {FITS.map((f) => <option key={f.v} value={f.v}>{f.label}</option>)}
          </select>
        </label>
      </div>
      {pick.kind === 'custom' && b.rules && (
        <div className="tvs-block-rules"><RuleFields compact r={b.rules} set={(p) => up({ rules: { ...b.rules, ...p } })} opts={opts} /></div>
      )}
    </li>
  )
}

// Commercials: any number of libraries of them, each whole or only the videos
// chosen here, per channel.
function CommercialsSection({ def, setDef }: { def: Definition; setDef: (p: Partial<Definition>) => void }) {
  const { libraries } = useApp()
  const libs = libraries.filter((l) => l.type === 'commercials')
  const f = def.filler || {}
  const picks = f.commercials || []
  const setPicks = (next: CommercialPick[], between = f.between) => setDef({ filler: { ...f, commercials: next, between } })
  const pickOf = (id: number) => picks.find((p) => p.library === id)
  const toggle = (id: number, on: boolean) => {
    if (on) setPicks([...picks, { library: id }], f.between || 2)
    else setPicks(picks.filter((p) => p.library !== id))
  }
  const change = (id: number, patch: Partial<CommercialPick>) => setPicks(picks.map((p) => (p.library === id ? { ...p, ...patch } : p)))
  if (libs.length === 0) {
    return (
      <Section title="Commercials" desc="Your own adverts between programmes.">
        <p className="muted">You have no commercials library yet. <Link to="/settings/libraries">Add one under Libraries</Link>, choosing the kind called Commercials. You can have as many as you like: one for the 1990s, one for toys, one for each channel.</p>
      </Section>
    )
  }
  return (
    <Section title="Commercials" desc="Play your own adverts between programmes and in the gaps. Use any of your commercials libraries, whole or only the videos you choose.">
      <ul className="tvs-blocks">
        {libs.map((l) => {
          const p = pickOf(l.id)
          return (
            <li key={l.id} className="tvs-block">
              <label className="check-row"><input type="checkbox" checked={!!p} onChange={(e) => toggle(l.id, e.target.checked)} /> <b>{l.name}</b> <span className="tvs-dim">{l.itemCount} {l.itemCount === 1 ? 'video' : 'videos'}</span></label>
              {p && (
                <>
                  <div className="tvs-days" role="radiogroup" aria-label={`What to use from ${l.name}`}>
                    <button type="button" role="radio" aria-checked={!p.only} className={'tvs-day' + (!p.only ? ' on' : '')} onClick={() => change(l.id, { only: false })}>All of them</button>
                    <button type="button" role="radio" aria-checked={!!p.only} className={'tvs-day' + (p.only ? ' on' : '')} onClick={() => change(l.id, { only: true })}>Only these…</button>
                  </div>
                  {p.only && <CommercialPicker library={l.id} name={l.name} value={p.items || []} onChange={(items) => change(l.id, { items })} />}
                </>
              )}
            </li>
          )
        })}
      </ul>
      {picks.length > 0 && (
        <div className="tvs-three">
          <label>After each programme, play
            <select className="select" value={f.between || 0} onChange={(e) => setDef({ filler: { ...f, between: Number(e.target.value) } })}>
              {[0, 1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{n === 0 ? 'Only in gaps' : `${n} ${n === 1 ? 'commercial' : 'commercials'}`}</option>)}
            </select>
          </label>
          <label>No longer than (seconds)<input className="input" type="number" min={0} value={f.maxSeconds || ''} onChange={(e) => setDef({ filler: { ...f, maxSeconds: Number(e.target.value) || 0 } })} /></label>
        </div>
      )}
      <p className="muted" style={{ fontSize: 13 }}>Commercials count as part of the channel for parental limits. A Kids profile won't see this channel unless these videos have a suitable rating (edit a video to give it one).</p>
    </Section>
  )
}

function CommercialPicker({ library, name, value, onChange }: { library: number; name: string; value: number[]; onChange: (v: number[]) => void }) {
  const [items, setItems] = useState<Item[] | null>(null)
  useEffect(() => { api.get<{ items: Item[] }>(`/items?library=${library}&limit=2000&sort=title`).then((r) => setItems(r.items)).catch((e) => toast(e.message)) }, [library])
  if (!items) return <p className="muted">Loading…</p>
  if (items.length === 0) return <p className="muted">Nothing is in this library yet. It may still be scanning.</p>
  const options = items.map((i) => ({ v: String(i.id), label: `${i.title} (${Math.round(i.runtimeSec)} s)` }))
  return (
    <>
      <Multi label={`Videos from ${name}`} options={options} value={value.map(String)} onChange={(v) => onChange(v.map(Number))} />
      {value.length === 0 && <p className="tvs-note" role="status">None chosen yet, so nothing from this library will play.</p>}
    </>
  )
}

function FillerSection({ def, setDef, opts }: { def: Definition; setDef: (p: Partial<Definition>) => void; opts: EditorOptions }) {
  const { libraries } = useApp()
  const f = def.filler || {}
  const rules = f.rules
  const setF = (p: Partial<Definition['filler']>) => setDef({ filler: { ...f, ...p } })
  return (
    <Section title="Other filler" desc="Anything else from your files to play between programmes: station idents or trailers from any library, chosen by the rules below. They never count as watched.">
      <div className="form" style={{ maxWidth: 'none' }}>
        <label className="check-row"><input type="checkbox" checked={!!rules} onChange={(e) => setF({ rules: e.target.checked ? {} : undefined })} /> Use filler from my files</label>
        {rules && (
          <>
            {libraries.length > 0 && <Multi label="From these libraries" options={libraries.map((l) => ({ v: String(l.id), label: l.name }))} value={strs(rules.libraries)} onChange={(v) => setF({ rules: { ...rules, libraries: v.map(Number) } })} />}
            <Multi label="From these shows" options={showOpts(opts)} value={strs(rules.shows)} onChange={(v) => setF({ rules: { ...rules, shows: v.map(Number) } })} />
            <div className="tvs-three">
              <label>After each programme, play
                <select className="select" value={f.between || 0} onChange={(e) => setF({ between: Number(e.target.value) })}>
                  {[0, 1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{n === 0 ? 'Only in gaps' : `${n} ${n === 1 ? 'bumper' : 'bumpers'}`}</option>)}
                </select>
              </label>
              <label>No longer than (seconds)<input className="input" type="number" min={0} value={f.maxSeconds || ''} onChange={(e) => setF({ maxSeconds: Number(e.target.value) || 0 })} /></label>
            </div>
          </>
        )}
      </div>
    </Section>
  )
}

function Audience({ draft, setDraft }: { draft: Draft; setDraft: (d: Draft) => void }) {
  const [users, setUsers] = useState<UserView[]>([])
  useEffect(() => { api.get<UserView[]>('/users').then(setUsers).catch(() => {}) }, [])
  const v = draft.visibility
  return (
    <div className="form" style={{ maxWidth: 'none' }}>
      <label>Shown to
        <select className="select" value={v.mode} onChange={(e) => setDraft({ ...draft, visibility: { ...v, mode: e.target.value as Visibility['mode'] } })}>
          <option value="">Everyone</option><option value="adults">Adults only</option><option value="only">Chosen profiles</option>
        </select>
        <small className="tvs-help">A profile's own limits always apply. Everyone on a channel shares one stream, so a channel with anything a Kids profile may not see is hidden from that profile, not filtered.</small>
      </label>
      {v.mode === 'only' && <Multi label="Profiles" options={users.map((u) => ({ v: String(u.id), label: u.name }))} value={strs(v.users)} onChange={(x) => setDraft({ ...draft, visibility: { ...v, users: x.map(Number) } })} />}
      <label className="check-row"><input type="checkbox" checked={draft.countProgress} onChange={(e) => setDraft({ ...draft, countProgress: e.target.checked })} /> Count towards “watched” and Continue Watching</label>
    </div>
  )
}

function Preview({ def }: { def: Definition }) {
  const [day, setDay] = useState(0)
  const [data, setData] = useState<{ stats: PoolStats; programmes: Programme[] } | null>(null)
  const [err, setErr] = useState('')
  const key = useMemo(() => JSON.stringify(def), [def])
  useEffect(() => {
    const t = setTimeout(() => {
      api.post<{ stats: PoolStats; programmes: Programme[] }>('/channels/preview', { definition: def, day }).then((d) => { setData(d); setErr('') }).catch((e) => setErr(e.message))
    }, 450)
    return () => clearTimeout(t)
  }, [key, day]) // eslint-disable-line react-hooks/exhaustive-deps
  const when = new Date(Date.now() + day * 86400000)
  const programmes = (data?.programmes || []).filter((p) => p.kind !== 'filler' && !(p.kind === 'slate' && p.end - p.start < 900))
  const empty = data && data.stats.titles === 0 && def.mode !== 'timetable'
  return (
    <Section title="What it will look like" desc={data ? `The rules choose ${data.stats.titles} ${data.stats.titles === 1 ? 'title' : 'titles'} (${fmtRuntime(data.stats.seconds) || 'no time'}).` : undefined}
      actions={<label className="tvs-inline">Day<select className="select sm" value={day} onChange={(e) => setDay(Number(e.target.value))}>{Array.from({ length: 8 }, (_, i) => <option key={i} value={i}>{i === 0 ? 'Today' : new Date(Date.now() + i * 86400000).toLocaleDateString([], { weekday: 'long' })}</option>)}</select></label>}>
      {err && <p className="tvs-error" role="alert">{err}</p>}
      {empty && <p className="tvs-note" role="status">Nothing in your library matches these rules, so the channel would show a “We'll be right back” card.</p>}
      <ol className="tvs-preview" aria-label={`Schedule for ${when.toLocaleDateString()}`}>
        {programmes.slice(0, 60).map((p) => (
          <li key={p.start} className={p.kind}>
            <time>{fmtClock(p.start)}</time>
            <span><b>{p.kind === 'slate' ? 'We\'ll be right back' : p.title}</b>{p.sub && <small> {p.sub}</small>}</span>
          </li>
        ))}
      </ol>
      {programmes.length > 60 && <p className="muted">…and {programmes.length - 60} more.</p>}
    </Section>
  )
}
