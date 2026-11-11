// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { api, fmtRuntime, mediaUrlQ, type Item } from '../../api'
import { IconPlus, IconSearch, IconTrash } from '../../icons'
import { toast } from '../../state'
import { ask, Button, Dialog, IconButton, Input, Switch } from '../../ui'
import { Section } from '../settings/ui'
import { uploadBumper, type Bumper, type BumperPlan, type BumperRule, type BumperWhen, type Definition, type EditorOptions, type Rules, type Trim, type TrimChoice } from './tvApi'
import './tveditor.css'
import { ACCEPT_VIDEO } from '../../mediaTypes.generated'

// Bumpers, in sentences: a rule reads as "Before each programme, play one of these,
// always". The library is shared between channels; each channel has its own rules.

const newRuleId = () => 'r' + Math.random().toString(36).slice(2, 9)
const WHEN: { v: BumperWhen; label: string }[] = [
  { v: 'before', label: 'Before each programme' }, { v: 'after', label: 'After each programme' },
  { v: 'between', label: 'Between commercials' }, { v: 'every', label: 'In a break, every…' },
]

export function BumperLibrary({ bumpers, reload }: { bumpers: Bumper[] | null; reload: () => void }) {
  const file = useRef<HTMLInputElement | null>(null)
  const [busy, setBusy] = useState(false)
  const [finding, setFinding] = useState(false)
  const upload = async (files: FileList | null) => {
    if (!files?.length) return
    setBusy(true)
    try {
      for (const f of Array.from(files)) {
        const r = await uploadBumper(f)
        toast(`${r.title} added, ${fmtRuntime(r.durationSec)} long`)
      }
    } catch (e) { toast((e as Error).message) } finally {
      setBusy(false)
      if (file.current) file.current.value = ''
      reload()
    }
  }
  const remove = async (b: Bumper) => {
    if (b.uploaded && !await ask({ title: `Delete “${b.title}”?`, body: 'The file you uploaded goes too, and any channel using it stops playing it.', confirm: 'Delete', danger: true })) return
    try { await api.del(`/channels/bumpers/item/${b.itemId}`); reload() } catch (e) { toast((e as Error).message) }
  }
  return (
    <>
      <div className="tvs-toolbar">
        <Button size="sm" variant="primary" icon={<IconPlus />} busy={busy} onClick={() => file.current?.click()}>{busy ? 'Uploading…' : 'Upload videos'}</Button>
        <input ref={file} hidden type="file" accept={"video/*," + ACCEPT_VIDEO} multiple onChange={(e) => void upload(e.target.files)} />
        <Button size="sm" icon={<IconSearch size={16} />} aria-haspopup="dialog" onClick={() => setFinding(true)}>Choose from my media</Button>
      </div>
      {bumpers && bumpers.length === 0 && <p className="muted">Nothing here yet. Upload a short video, or choose one you already have. Anything in a Commercials library also appears here.</p>}
      {bumpers && bumpers.length > 0 && (
        <ul className="tve-bumpers">
          {bumpers.map((b) => (
            <li key={b.itemId} className="tve-bumper">
              <img src={mediaUrlQ(`/api/items/${b.itemId}/poster`, `v=${b.updatedAt}`)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} />
              <div className="tve-bumper-text">
                <b title={b.title}>{b.title}</b>
                <span>{fmtRuntime(b.durationSec)} · {b.uploaded ? 'Uploaded' : b.library}</span>
              </div>
              {(b.uploaded || b.chosen) && <IconButton size="sm" onClick={() => void remove(b)} label={`Remove ${b.title}`}><IconTrash size={16} /></IconButton>}
            </li>
          ))}
        </ul>
      )}
      {finding && <MediaFinder onClose={() => setFinding(false)} onPick={async (it) => {
        try { await api.post('/channels/bumpers', { itemId: it.id }); toast(`${it.title} added`); reload() } catch (e) { toast((e as Error).message) }
      }} />}
    </>
  )
}

function MediaFinder({ onClose, onPick }: { onClose: () => void; onPick: (it: Item) => void }) {
  const [q, setQ] = useState('')
  const [items, setItems] = useState<Item[] | null>(null)
  useEffect(() => {
    if (q.trim().length < 2) { setItems(null); return }
    const t = setTimeout(() => {
      api.get<{ items: Item[] }>(`/items?search=${encodeURIComponent(q.trim())}&limit=40&kind=video`).then((r) => setItems(r.items)).catch((e) => toast(e.message))
    }, 300)
    return () => clearTimeout(t)
  }, [q])
  return (
    <Dialog className="tve-finder" title="Choose from my media" onClose={onClose} actions={<Button variant="primary" onClick={onClose}>Done</Button>}>
      <Input type="search" autoFocus aria-label="Search your videos" value={q} placeholder="Search your videos…" onChange={(e) => setQ(e.target.value)} />
      {items && items.length === 0 && <p className="muted">Nothing found.</p>}
      {items && items.length > 0 && (
        <ul className="tve-found">
          {items.map((it) => <li key={it.id}><span><b>{it.showTitle ? `${it.showTitle}: ` : ''}{it.title}</b> <small>{fmtRuntime(it.runtimeSec)}</small></span><Button size="sm" onClick={() => onPick(it)}>Add</Button></li>)}
        </ul>
      )}
    </Dialog>
  )
}

// Chips that choose several, with a filter when there are many.
function Chips({ label, options, value, onChange }: { label: string; options: { v: string; label: string }[]; value: string[]; onChange: (v: string[]) => void }) {
  const [q, setQ] = useState('')
  if (options.length === 0) return null
  const shown = options.filter((o) => !q || o.label.toLowerCase().includes(q.toLowerCase())).slice(0, 200)
  return (
    <fieldset className="tvs-multi">
      <legend>{label}{value.length > 0 && <span className="tvs-count"> · {value.length} chosen</span>}</legend>
      {options.length > 12 && <input className="input" type="search" aria-label={`Find in ${label}`} placeholder="Find…" value={q} onChange={(e) => setQ(e.target.value)} />}
      <div className="tvs-chips">
        {shown.map((o) => (
          <label key={o.v} className={'tvs-chip' + (value.includes(o.v) ? ' on' : '')}>
            <input type="checkbox" checked={value.includes(o.v)} onChange={(e) => onChange(e.target.checked ? [...value, o.v] : value.filter((x) => x !== o.v))} /> {o.label}
          </label>
        ))}
      </div>
    </fieldset>
  )
}

type OnlyKind = '' | 'films' | 'shows' | 'genres' | 'collections'
const onlyKindOf = (r?: Rules): OnlyKind => (!r ? '' : r.items?.length ? 'films' : r.shows?.length ? 'shows' : r.genres?.length ? 'genres' : r.collections?.length ? 'collections' : 'films')

function RuleCard({ rule, n, bumpers, opts, films, up, remove }: { rule: BumperRule; n: number; bumpers: Bumper[]; opts: EditorOptions; films: { id: number; title: string }[]; up: (p: Partial<BumperRule>) => void; remove: () => void }) {
  const [onlyKind, setOnlyKind] = useState<OnlyKind>(onlyKindOf(rule.only))
  const label = rule.name || `Rule ${n}`
  const chosen = (rule.source.items || []).map(String)
  const empty = chosen.length === 0 && !rule.source.libraries?.length && !rule.source.clips?.length
  const setOnly = (k: OnlyKind) => { setOnlyKind(k); up({ only: k === '' ? undefined : {} }) }
  return (
    <li className={'tve-rule' + (rule.off ? ' off' : '')}>
      <div className="tve-rule-head">
        <input className="input tve-rule-name" value={rule.name || ''} placeholder={`Rule ${n}`} maxLength={200} aria-label={`Name of ${label}`} onChange={(e) => up({ name: e.target.value })} />
        <label className="tve-mandatory" title="Never cut or left out to keep to the timetable"><Switch checked={!!rule.mandatory} label={`${label} is mandatory`} onChange={(v) => up({ mandatory: v })} /> Mandatory</label>
        <label className="tve-mandatory"><Switch checked={!rule.off} label={`${label} is in use`} onChange={(v) => up({ off: !v })} /> In use</label>
        <IconButton size="sm" onClick={remove} label={`Remove ${label}`}><IconTrash size={16} /></IconButton>
      </div>
      <div className="tve-sentence">
        <label>When
          <select className="select" value={rule.when} onChange={(e) => up({ when: e.target.value as BumperWhen, everyMin: e.target.value === 'every' ? (rule.everyMin || 15) : undefined })}>
            {WHEN.map((w) => <option key={w.v} value={w.v}>{w.label}</option>)}
          </select>
        </label>
        {rule.when === 'every' && <label>Minutes<input className="input tvs-short" type="number" min={5} max={240} value={rule.everyMin || 15} onChange={(e) => up({ everyMin: Number(e.target.value) })} /></label>}
        <label>Play
          <select className="select" value={rule.count || 1} onChange={(e) => up({ count: Number(e.target.value) })}>
            {[1, 2, 3, 4, 5].map((c) => <option key={c} value={c}>{c === 1 ? 'one of these' : `${c} of these`}</option>)}
          </select>
        </label>
        <label>For
          <select className="select" value={onlyKind} onChange={(e) => setOnly(e.target.value as OnlyKind)}>
            <option value="">every programme</option>
            <option value="films">particular films</option>
            <option value="shows">particular shows</option>
            <option value="genres">particular genres</option>
            {opts.collections.length > 0 && <option value="collections">particular collections</option>}
          </select>
        </label>
      </div>
      {bumpers.length > 0
        ? <Chips label="Videos (they take turns)" options={bumpers.map((b) => ({ v: String(b.itemId), label: `${b.title} (${fmtRuntime(b.durationSec)})` }))} value={chosen} onChange={(v) => up({ source: { ...rule.source, items: v.map(Number) } })} />
        : <p className="muted">Add a video to the bumper library first.</p>}
      {empty && bumpers.length > 0 && <p className="tvs-note" role="status">Choose at least one video, or this rule plays nothing.</p>}
      {onlyKind === 'films' && <Chips label="Only with these films" options={films.map((f) => ({ v: String(f.id), label: f.title }))} value={(rule.only?.items || []).map(String)} onChange={(v) => up({ only: { items: v.map(Number) } })} />}
      {onlyKind === 'shows' && <Chips label="Only with these shows" options={opts.shows.map((s) => ({ v: String(s.id), label: s.title }))} value={(rule.only?.shows || []).map(String)} onChange={(v) => up({ only: { shows: v.map(Number) } })} />}
      {onlyKind === 'genres' && <Chips label="Only with these genres" options={opts.genres.map((g) => ({ v: g, label: g }))} value={rule.only?.genres || []} onChange={(v) => up({ only: { genres: v } })} />}
      {onlyKind === 'collections' && <Chips label="Only with these collections" options={opts.collections.map((c) => ({ v: String(c.id), label: c.name }))} value={(rule.only?.collections || []).map(String)} onChange={(v) => up({ only: { collections: v.map(Number) } })} />}
    </li>
  )
}

const TRIM_ADS: { v: TrimChoice; label: string; hint: string }[] = [
  { v: 'cut', label: 'Cut them short', hint: 'Commercials stop early, the last one first. Any with under a few seconds left are left out.' },
  { v: 'drop', label: 'Leave out whole ones', hint: 'Commercials always play in full, but as many as need to are left out. A gap may be filled by a card.' },
  { v: 'keep', label: 'Always play them', hint: 'Commercials never give way, so a programme after them can start late.' },
]
const TRIM_BUMPERS: { v: TrimChoice; label: string }[] = [
  { v: 'drop', label: 'Leave them out' }, { v: 'cut', label: 'Cut them short' }, { v: 'keep', label: 'Always play them' },
]
const TAILS = [{ v: 0, label: 'Never' }, { v: 30, label: 'Up to 30 seconds' }, { v: 60, label: 'Up to 1 minute' }, { v: 120, label: 'Up to 2 minutes' }, { v: 300, label: 'Up to 5 minutes' }]

export function BumperSection({ def, setDef, opts, id }: { def: Definition; setDef: (p: Partial<Definition>) => void; opts: EditorOptions; id?: string }) {
  const [bumpers, setBumpers] = useState<Bumper[] | null>(null)
  const [films, setFilms] = useState<{ id: number; title: string }[]>([])
  const load = () => { api.get<Bumper[]>('/channels/bumpers').then(setBumpers).catch((e) => toast(e.message)) }
  useEffect(load, [])
  useEffect(() => {
    api.get<{ items: Item[] }>('/items?noEpisodes=1&limit=1000&sort=title').then((r) => setFilms(r.items.map((i) => ({ id: i.id, title: i.title })))).catch(() => {})
  }, [])
  const plan: BumperPlan = def.bumpers || {}
  const rules = plan.rules || []
  const setPlan = (p: Partial<BumperPlan>) => setDef({ bumpers: { ...plan, ...p } })
  const setRule = (id: string, p: Partial<BumperRule>) => setPlan({ rules: rules.map((r) => (r.id === id ? { ...r, ...p } : r)) })
  const addRule = (r: Partial<BumperRule>) => setPlan({ rules: [...rules, { id: newRuleId(), when: 'before', source: {}, ...r }] })
  const trim: Trim = plan.trim || {}
  const setTrim = (p: Partial<Trim>) => setPlan({ trim: { ...trim, ...p } })
  const noGrid = !def.alignMin && !(def.blocks || []).length
  return (
    <>
      <Section id={id} title="Bumpers" desc="Short videos around your programmes: a station ident before each one, a sting before one particular film, a break every 15 minutes.">
        <h3 className="tvs-h3">The bumper library</h3>
        <p className="muted tve-sub">Shared by every channel.</p>
        <BumperLibrary bumpers={bumpers} reload={load} />
        <h3 className="tvs-h3">Rules for this channel</h3>
        {rules.length === 0 && <p className="muted">No rules yet, so no bumpers play on this channel.</p>}
        <ul className="tve-rules">
          {rules.map((r, i) => <RuleCard key={r.id} rule={r} n={i + 1} bumpers={bumpers || []} opts={opts} films={films} up={(p) => setRule(r.id, p)} remove={() => setPlan({ rules: rules.filter((x) => x.id !== r.id) })} />)}
        </ul>
        <div className="tvs-toolbar">
          <Button size="sm" icon={<IconPlus />} onClick={() => addRule({ when: 'before', name: 'Ident before every programme', mandatory: true })}>Before every programme</Button>
          <Button size="sm" icon={<IconPlus />} onClick={() => addRule({ when: 'before', name: 'Only before a certain title', only: {} })}>Only for a certain film or show</Button>
          <Button size="sm" icon={<IconPlus />} onClick={() => addRule({ when: 'every', everyMin: 15, name: 'A break every 15 minutes' })}>A break every so often</Button>
          <Button size="sm" variant="ghost" icon={<IconPlus />} onClick={() => addRule({})}>Blank rule</Button>
        </div>
      </Section>
      <Section id={id ? id + '-trim' : undefined} title="Keeping to the timetable" desc="When a programme runs long, something has to give, or everything after it starts late.">
        <div className="tve-trim-switch">
          <Switch checked={!!trim.on} label="Cut commercials short to keep to the timetable" onChange={(v) => setTrim({ on: v })} />
          <div><b>Cut commercials short to keep to the timetable</b><span>The next programme starts on time, and the guide says when that really is.</span></div>
        </div>
        {trim.on && noGrid && (
          <p className="tvs-note" role="status">This channel plays back to back, so there's no timetable to keep to. <Button size="sm" onClick={() => setDef({ alignMin: 30 })}>Start programmes on the half hour</Button></p>
        )}
        {trim.on && (
          <div className="tve-trim">
            <fieldset className="tve-radios">
              <legend>Commercials</legend>
              {TRIM_ADS.map((o) => (
                <label key={o.v} className={'tve-radio' + ((trim.ads || 'cut') === o.v ? ' on' : '')}>
                  <input type="radio" name="trim-ads" checked={(trim.ads || 'cut') === o.v} onChange={() => setTrim({ ads: o.v })} />
                  <span><b>{o.label}</b><small>{o.hint}</small></span>
                </label>
              ))}
            </fieldset>
            <fieldset className="tve-radios row">
              <legend>Bumpers that aren't mandatory</legend>
              {TRIM_BUMPERS.map((o) => (
                <label key={o.v} className={'tve-radio' + ((trim.bumpers || 'drop') === o.v ? ' on' : '')}>
                  <input type="radio" name="trim-bumpers" checked={(trim.bumpers || 'drop') === o.v} onChange={() => setTrim({ bumpers: o.v })} />
                  <span><b>{o.label}</b></span>
                </label>
              ))}
            </fieldset>
            <div className="tvs-three">
              <label>A programme can lose from its end
                <select className="select" value={trim.tailSec || 0} onChange={(e) => setTrim({ tailSec: Number(e.target.value) })}>
                  {TAILS.map((t) => <option key={t.v} value={t.v}>{t.label}</option>)}
                </select>
                <small className="tvs-help">Usually the credits. Only used when a programme is just over its slot.</small>
              </label>
              <label>Leave out what has less than this left (seconds)
                <input className="input" type="number" min={1} max={300} value={trim.minKeepSec || 5} onChange={(e) => setTrim({ minKeepSec: Number(e.target.value) })} />
              </label>
            </div>
            <p className="muted tve-sub">Mandatory bumpers and programmes are never touched, apart from the end of a programme if you allow it above.</p>
          </div>
        )}
      </Section>
    </>
  )
}
