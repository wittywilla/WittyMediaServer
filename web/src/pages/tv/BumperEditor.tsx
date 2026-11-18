// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmtRuntime, mediaUrlQ, type Item } from '../../api'
import { IconPlus, IconSearch, IconTrash } from '../../icons'
import { toast } from '../../state'
import { ask, Button, Dialog, IconButton, Input, Switch } from '../../ui'
import { Section } from '../settings/ui'
import { channelVideos, uploadBumper, type Bumper, type VideoPage, type BumperPlan, type BumperRule, type BumperWhen, type Definition, type EditorOptions, type Rules, type Trim, type TrimChoice } from './tvApi'
import './tveditor.css'
import { ACCEPT_VIDEO } from '../../mediaTypes.generated'
import { LibraryPicks, useShortLibraries, VideoPicker } from './VideoPicks'
import { num, t, tk, tn } from '../../i18n'

// Bumpers, in sentences: a rule reads as "Before each programme, play one of these,
// always". Each channel chooses its bumper libraries and its own rules; uploads
// and picks from other libraries are shared between channels.

const OWN_PAGE = 12

// emptyRules: the rules choose nothing in particular, as the server sees it.
const emptyRules = (r: Rules) => Object.values(r).every((v) => v == null || v === false || v === '' || v === 0 || (Array.isArray(v) && v.length === 0))

const newRuleId = () => 'r' + Math.random().toString(36).slice(2, 9)
const WHEN: { v: BumperWhen; label: string }[] = [
  { v: 'before', label: tk('Before each programme') }, { v: 'after', label: tk('After each programme') },
  { v: 'between', label: tk('Between commercials') }, { v: 'every', label: tk('In a break, every…') },
]

// BumperLibrary is the shared list of uploads and videos chosen from other
// libraries: a count on the page, and the list a page at a time in a dialog.
export function BumperLibrary() {
  const file = useRef<HTMLInputElement | null>(null)
  const [busy, setBusy] = useState(false)
  const [view, setView] = useState<'' | 'list' | 'find'>('')
  const [offset, setOffset] = useState(0)
  const [page, setPage] = useState<VideoPage | null>(null)
  const [tick, setTick] = useState(0)
  const reload = () => setTick((n) => n + 1)
  useEffect(() => {
    let live = true
    channelVideos({ own: true, offset, limit: OWN_PAGE }).then((r) => { if (live) setPage(r) }).catch((e) => toast((e as Error).message))
    return () => { live = false }
  }, [offset, tick])
  const upload = async (files: FileList | null) => {
    if (!files?.length) return
    setBusy(true)
    try {
      for (const f of Array.from(files)) {
        const r = await uploadBumper(f)
        toast(t('{title} added, {length} long', { title: r.title, length: fmtRuntime(r.durationSec) }))
      }
    } catch (e) { toast((e as Error).message) } finally {
      setBusy(false)
      if (file.current) file.current.value = ''
      reload()
    }
  }
  const remove = async (b: Bumper) => {
    if (b.uploaded && !await ask({ title: t('Delete “{title}”?', { title: b.title }), body: t('The file you uploaded goes too, and any channel using it stops playing it.'), confirm: t('Delete'), danger: true })) return
    try { await api.del(`/channels/bumpers/item/${b.itemId}`); reload() } catch (e) { toast((e as Error).message) }
  }
  const total = page?.total || 0
  return (
    <div className="tve-vpick">
      <span><b>{t('Your uploads and picks')}</b><span className="tvs-count"> · {page ? num(total) : '…'}</span></span>
      <Button size="sm" aria-haspopup="dialog" onClick={() => setView('list')}>{t('Manage…')}</Button>
      <input ref={file} hidden type="file" accept={"video/*," + ACCEPT_VIDEO} multiple onChange={(e) => void upload(e.target.files)} />
      {view === 'list' && <Dialog className="tve-vpick-dialog" title={t('Your uploads and picks')} onClose={() => setView('')} actions={<Button variant="primary" onClick={() => setView('')}>{t('Done')}</Button>}>
        <p className="muted tve-sub">{t('Shared by every channel. Uploads go into the commercials library called WittyTV bumpers: tick it under Bumper libraries to use them here.')}</p>
        <div className="tvs-toolbar">
          <Button size="sm" variant="primary" icon={<IconPlus />} busy={busy} onClick={() => file.current?.click()}>{busy ? t('Uploading…') : t('Upload videos')}</Button>
          <Button size="sm" icon={<IconSearch size={16} />} aria-haspopup="dialog" onClick={() => setView('find')}>{t('Choose from my media')}</Button>
        </div>
        {page && total === 0 && <p className="muted">{t('Nothing here yet. Upload a short video, or choose one you already have.')}</p>}
        {page && page.items.length > 0 && (
          <ul className="tve-bumpers">
            {page.items.map((b) => (
              <li key={b.itemId} className="tve-bumper">
                <img src={mediaUrlQ(`/api/items/${b.itemId}/poster`, `v=${b.updatedAt}`)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} />
                <div className="tve-bumper-text">
                  <b title={b.title}>{b.title}</b>
                  <span>{fmtRuntime(b.durationSec)} · {b.uploaded ? t('Uploaded') : b.library}</span>
                </div>
                <IconButton size="sm" onClick={() => void remove(b)} label={t('Remove {name}', { name: b.title })}><IconTrash size={16} /></IconButton>
              </li>
            ))}
          </ul>
        )}
        {total > OWN_PAGE && (
          <div className="tve-vpick-pages">
            <Button size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - OWN_PAGE))}>{t('Previous')}</Button>
            <span className="tvs-dim">{t('{from}–{to} of {total}', { from: num(offset + 1), to: num(Math.min(offset + OWN_PAGE, total)), total: num(total) })}</span>
            <Button size="sm" disabled={offset + OWN_PAGE >= total} onClick={() => setOffset(offset + OWN_PAGE)}>{t('Next')}</Button>
          </div>
        )}
      </Dialog>}
      {view === 'find' && <MediaFinder onClose={() => setView('list')} onPick={async (it) => {
        try { await api.post('/channels/bumpers', { itemId: it.id }); toast(t('{title} added', { title: it.title })); reload() } catch (e) { toast((e as Error).message) }
      }} />}
    </div>
  )
}

function MediaFinder({ onClose, onPick }: { onClose: () => void; onPick: (it: Item) => void }) {
  const [q, setQ] = useState('')
  const [items, setItems] = useState<Item[] | null>(null)
  useEffect(() => {
    if (q.trim().length < 2) { setItems(null); return }
    const timer = setTimeout(() => {
      api.get<{ items: Item[] }>(`/items?search=${encodeURIComponent(q.trim())}&limit=40&kind=video`).then((r) => setItems(r.items)).catch((e) => toast(e.message))
    }, 300)
    return () => clearTimeout(timer)
  }, [q])
  return (
    <Dialog className="tve-finder" title={t('Choose from my media')} onClose={onClose} actions={<Button variant="primary" onClick={onClose}>{t('Done')}</Button>}>
      <Input type="search" autoFocus aria-label={t('Search your videos')} value={q} placeholder={t('Search your videos…')} onChange={(e) => setQ(e.target.value)} />
      {items && items.length === 0 && <p className="muted">{t('Nothing found.')}</p>}
      {items && items.length > 0 && (
        <ul className="tve-found">
          {items.map((it) => <li key={it.id}><span><b>{it.showTitle ? `${it.showTitle}: ` : ''}{it.title}</b> <small>{fmtRuntime(it.runtimeSec)}</small></span><Button size="sm" onClick={() => onPick(it)}>{t('Add')}</Button></li>)}
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
      <legend>{label}{value.length > 0 && <span className="tvs-count"> · {t('{count} chosen', { count: value.length })}</span>}</legend>
      {options.length > 12 && <input className="input" type="search" aria-label={t('Find in {name}', { name: label })} placeholder={t('Find…')} value={q} onChange={(e) => setQ(e.target.value)} />}
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

function RuleCard({ rule, n, sources, opts, films, up, remove }: { rule: BumperRule; n: number; sources: number[]; opts: EditorOptions; films: { id: number; title: string }[]; up: (p: Partial<BumperRule>) => void; remove: () => void }) {
  const [onlyKind, setOnlyKind] = useState<OnlyKind>(onlyKindOf(rule.only))
  const label = rule.name || t('Rule {n}', { n })
  const chosen = rule.source.items || []
  // A rule that names no videos of its own plays the channel's bumper libraries.
  const [own, setOwn] = useState(!emptyRules(rule.source))
  const otherSource = !!(rule.source.libraries?.length || rule.source.clips?.length)
  const setOnly = (k: OnlyKind) => { setOnlyKind(k); up({ only: k === '' ? undefined : {} }) }
  return (
    <li className={'tve-rule' + (rule.off ? ' off' : '')}>
      <div className="tve-rule-head">
        <input className="input tve-rule-name" value={rule.name || ''} placeholder={t('Rule {n}', { n })} maxLength={200} aria-label={t('Name of {name}', { name: label })} onChange={(e) => up({ name: e.target.value })} />
        <label className="tve-mandatory" title={t('Never cut or left out to keep to the timetable')}><Switch checked={!!rule.mandatory} label={t('{name} is mandatory', { name: label })} onChange={(v) => up({ mandatory: v })} /> {t('Mandatory')}</label>
        <label className="tve-mandatory"><Switch checked={!rule.off} label={t('{name} is in use', { name: label })} onChange={(v) => up({ off: !v })} /> {t('In use')}</label>
        <IconButton size="sm" onClick={remove} label={t('Remove {name}', { name: label })}><IconTrash size={16} /></IconButton>
      </div>
      <div className="tve-sentence">
        <label>{t('When')}
          <select className="select" value={rule.when} onChange={(e) => up({ when: e.target.value as BumperWhen, everyMin: e.target.value === 'every' ? (rule.everyMin || 15) : undefined })}>
            {WHEN.map((w) => <option key={w.v} value={w.v}>{t(w.label)}</option>)}
          </select>
        </label>
        {rule.when === 'every' && <label>{t('Minutes')}<input className="input tvs-short" type="number" min={5} max={240} value={rule.everyMin || 15} onChange={(e) => up({ everyMin: Number(e.target.value) })} /></label>}
        <label>{t('Play')}
          <select className="select" value={rule.count || 1} onChange={(e) => up({ count: Number(e.target.value) })}>
            {[1, 2, 3, 4, 5].map((c) => <option key={c} value={c}>{tn('one of these', '{count} of these', c)}</option>)}
          </select>
        </label>
        <label>{t('For')}
          <select className="select" value={onlyKind} onChange={(e) => setOnly(e.target.value as OnlyKind)}>
            <option value="">{t('every programme')}</option>
            <option value="films">{t('particular films')}</option>
            <option value="shows">{t('particular shows')}</option>
            <option value="genres">{t('particular genres')}</option>
            {opts.collections.length > 0 && <option value="collections">{t('particular collections')}</option>}
          </select>
        </label>
      </div>
      <div className="tvs-days" role="radiogroup" aria-label={t('Videos for {name}', { name: label })}>
        <button type="button" role="radio" aria-checked={!own} className={'tvs-day' + (!own ? ' on' : '')} onClick={() => { setOwn(false); up({ source: {} }) }}>{t('Any from the bumper libraries')}</button>
        <button type="button" role="radio" aria-checked={own} className={'tvs-day' + (own ? ' on' : '')} onClick={() => setOwn(true)}>{t('Only these…')}</button>
      </div>
      {!own && sources.length === 0 && <p className="tvs-note" role="status">{t('This channel has no bumper libraries yet, so this rule plays nothing. Choose some above.')}</p>}
      {own && otherSource && <p className="muted tve-sub">{t('This rule also uses videos chosen another way, which it keeps.')}</p>}
      {own && <VideoPicker label={t('Videos (they take turns)')} libraries={sources} own value={chosen} onChange={(v) => up({ source: { ...rule.source, items: v.length ? v : undefined } })} />}
      {own && chosen.length === 0 && !otherSource && <p className="tvs-note" role="status">{t('None chosen yet, so this rule plays any from the bumper libraries.')}</p>}
      {onlyKind === 'films' && <Chips label={t('Only with these films')} options={films.map((f) => ({ v: String(f.id), label: f.title }))} value={(rule.only?.items || []).map(String)} onChange={(v) => up({ only: { items: v.map(Number) } })} />}
      {onlyKind === 'shows' && <Chips label={t('Only with these shows')} options={opts.shows.map((s) => ({ v: String(s.id), label: s.title }))} value={(rule.only?.shows || []).map(String)} onChange={(v) => up({ only: { shows: v.map(Number) } })} />}
      {onlyKind === 'genres' && <Chips label={t('Only with these genres')} options={opts.genres.map((g) => ({ v: g, label: g }))} value={rule.only?.genres || []} onChange={(v) => up({ only: { genres: v } })} />}
      {onlyKind === 'collections' && <Chips label={t('Only with these collections')} options={opts.collections.map((c) => ({ v: String(c.id), label: c.name }))} value={(rule.only?.collections || []).map(String)} onChange={(v) => up({ only: { collections: v.map(Number) } })} />}
    </li>
  )
}

const TRIM_ADS: { v: TrimChoice; label: string; hint: string }[] = [
  { v: 'cut', label: tk('Cut them short'), hint: tk('Commercials stop early, the last one first. Any with under a few seconds left are left out.') },
  { v: 'drop', label: tk('Leave out whole ones'), hint: tk('Commercials always play in full, but as many as need to are left out. A gap may be filled by a card.') },
  { v: 'keep', label: tk('Always play them'), hint: tk('Commercials never give way, so a programme after them can start late.') },
]
const TRIM_BUMPERS: { v: TrimChoice; label: string }[] = [
  { v: 'drop', label: tk('Leave them out') }, { v: 'cut', label: tk('Cut them short') }, { v: 'keep', label: tk('Always play them') },
]
const TAILS = [{ v: 0, label: tk('Never') }, { v: 30, label: tk('Up to 30 seconds') }, { v: 60, label: tk('Up to 1 minute') }, { v: 120, label: tk('Up to 2 minutes') }, { v: 300, label: tk('Up to 5 minutes') }]

export function BumperSection({ def, setDef, opts, id }: { def: Definition; setDef: (p: Partial<Definition>) => void; opts: EditorOptions; id?: string }) {
  const [films, setFilms] = useState<{ id: number; title: string }[]>([])
  const shortLibs = useShortLibraries()
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
  const libs = plan.libraries || []
  const adLibs = def.filler?.commercials || []
  // What a rule's "Only these" chooses from: the bumper libraries, and the
  // commercials too when they may be bumpers. The shared list is always there.
  const sources = [...new Set([...libs, ...(plan.fromCommercials ? adLibs : [])].map((p) => p.library))]
  const count = libs.reduce((n, p) => n + (p.only ? (p.items || []).length : shortLibs.find((l) => l.id === p.library)?.itemCount || 0), 0)
  return (
    <>
      <Section id={id} title={tk('Bumpers')} desc={t('Short videos around your programmes: a station ident before each one, a sting before one particular film, a break every 15 minutes.')}>
        <h3 className="tvs-h3">{t('Bumper libraries')}</h3>
        <p className="muted tve-sub">{libs.length === 0 ? t('None chosen yet. Bumpers come from these, and commercials from the commercials libraries.') : t('{libraries}, {videos}.', { libraries: tn('{count} library', '{count} libraries', libs.length), videos: tn('{count} video', '{count} videos', count) })}</p>
        {shortLibs.length === 0
          ? <p className="muted">{t('You have no commercials library yet: bumpers live in one too.')} <Link to="/settings/libraries">{t('Add one under Libraries')}</Link>. {t('Or upload a video below.')}</p>
          : <LibraryPicks label={t('Bumper libraries')} picks={libs} onChange={(next) => setPlan({ libraries: next.length ? next : undefined })} note={(lid) => (adLibs.some((p) => p.library === lid) ? t('a commercials library here') : '')} />}
        <label className="check-row">
          <input type="checkbox" checked={!!plan.fromCommercials} onChange={(e) => setPlan({ fromCommercials: e.target.checked || undefined })} /> {t('Bumpers may also come from the commercials libraries')}
        </label>
        <BumperLibrary />
        <h3 className="tvs-h3">{t('Rules for this channel')}</h3>
        {rules.length === 0 && <p className="muted">{t('No rules yet, so no bumpers play on this channel.')}</p>}
        <ul className="tve-rules">
          {rules.map((r, i) => <RuleCard key={r.id} rule={r} n={i + 1} sources={sources} opts={opts} films={films} up={(p) => setRule(r.id, p)} remove={() => setPlan({ rules: rules.filter((x) => x.id !== r.id) })} />)}
        </ul>
        <div className="tvs-toolbar">
          <Button size="sm" icon={<IconPlus />} onClick={() => addRule({ when: 'before', name: t('Ident before every programme'), mandatory: true })}>{t('Before every programme')}</Button>
          <Button size="sm" icon={<IconPlus />} onClick={() => addRule({ when: 'before', name: t('Only before a certain title'), only: {} })}>{t('Only for a certain film or show')}</Button>
          <Button size="sm" icon={<IconPlus />} onClick={() => addRule({ when: 'every', everyMin: 15, name: t('A break every 15 minutes') })}>{t('A break every so often')}</Button>
          <Button size="sm" variant="ghost" icon={<IconPlus />} onClick={() => addRule({})}>{t('Blank rule')}</Button>
        </div>
      </Section>
      <Section id={id ? id + '-trim' : undefined} title={tk('Keeping to the timetable')} desc={t('When a programme runs long, something has to give, or everything after it starts late.')}>
        <div className="tve-trim-switch">
          <Switch checked={!!trim.on} label={t('Cut commercials short to keep to the timetable')} onChange={(v) => setTrim({ on: v })} />
          <div><b>{t('Cut commercials short to keep to the timetable')}</b><span>{t('The next programme starts on time, and the guide says when that really is.')}</span></div>
        </div>
        {trim.on && noGrid && (
          <p className="tvs-note" role="status">{t("This channel plays back to back, so there's no timetable to keep to.")} <Button size="sm" onClick={() => setDef({ alignMin: 30 })}>{t('Start programmes on the half hour')}</Button></p>
        )}
        {trim.on && (
          <div className="tve-trim">
            <fieldset className="tve-radios">
              <legend>{t('Commercials')}</legend>
              {TRIM_ADS.map((o) => (
                <label key={o.v} className={'tve-radio' + ((trim.ads || 'cut') === o.v ? ' on' : '')}>
                  <input type="radio" name="trim-ads" checked={(trim.ads || 'cut') === o.v} onChange={() => setTrim({ ads: o.v })} />
                  <span><b>{t(o.label)}</b><small>{t(o.hint)}</small></span>
                </label>
              ))}
            </fieldset>
            <fieldset className="tve-radios row">
              <legend>{t("Bumpers that aren't mandatory")}</legend>
              {TRIM_BUMPERS.map((o) => (
                <label key={o.v} className={'tve-radio' + ((trim.bumpers || 'drop') === o.v ? ' on' : '')}>
                  <input type="radio" name="trim-bumpers" checked={(trim.bumpers || 'drop') === o.v} onChange={() => setTrim({ bumpers: o.v })} />
                  <span><b>{t(o.label)}</b></span>
                </label>
              ))}
            </fieldset>
            <div className="tvs-three">
              <label>{t('A programme can lose from its end')}
                <select className="select" value={trim.tailSec || 0} onChange={(e) => setTrim({ tailSec: Number(e.target.value) })}>
                  {TAILS.map((x) => <option key={x.v} value={x.v}>{t(x.label)}</option>)}
                </select>
                <small className="tvs-help">{t('Usually the credits. Only used when a programme is just over its slot.')}</small>
              </label>
              <label>{t('Leave out what has less than this left (seconds)')}
                <input className="input" type="number" min={1} max={300} value={trim.minKeepSec || 5} onChange={(e) => setTrim({ minKeepSec: Number(e.target.value) })} />
              </label>
            </div>
            <p className="muted tve-sub">{t('Mandatory bumpers and programmes are never touched, apart from the end of a programme if you allow it above.')}</p>
          </div>
        )}
      </Section>
    </>
  )
}
