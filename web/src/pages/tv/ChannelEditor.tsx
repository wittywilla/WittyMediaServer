// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useBlocker, useLocation, useNavigate, useParams } from 'react-router-dom'
import { fmtDateTime, api, fmtRuntime, mediaUrl, type UserView } from '../../api'
import { IconPlus, IconTrash } from '../../icons'
import { toast, useApp } from '../../state'
import { ask } from '../../ui'
import { PageHead, Section } from '../settings/ui'
import BrandLogo from '../../components/BrandLogo'
import { ChangeLogoButton, LogoConsentHint } from '../../components/edit/LogoChooser'
import LegalNote from '../../components/LegalNote'
import { BumperSection } from './BumperEditor'
import { LibraryPicks, useShortLibraries } from './VideoPicks'
import SpecPage from '../../spec/SpecPage'
import { audienceSpec, basicsSpec, moreSpec } from '../../spec/specs/channelEditor'
import DiscScenes from './DiscScenes'
import CloneStation from './CloneStation'
import { resetChannel } from './resetChannel'
import './tveditor.css'
import { brandOf, dayNames, emptyDefinition, type ChannelLook, type CommercialPick, fmtClock, forgetLineup, uploadLogo, type Block, type Channel, type Definition, type EditorOptions, type Order, type PoolStats, type Programme, type Rules, type Suggestion, type Visibility } from './tvApi'
import { date, t, tc, tk, tn, tx } from '../../i18n'

// One channel, as much or as little as you want. Simple, Rules and Timetable
// are three views of the same definition: going from one to another changes
// nothing that was set. A draft stays in this browser until it is saved, and
// each save keeps the version it replaced.

const ORDERS: { v: Order['mode']; label: string; hint: string }[] = [
  { v: 'rotate', label: tk('Take turns'), hint: tk('The next episode of each show in turn, like a station running reruns') },
  { v: 'sequential', label: tk('In order'), hint: tk('Every episode of one show, then the next show') },
  { v: 'aired', label: tk('As first aired'), hint: tk('By original air date, across all shows') },
  { v: 'shuffle', label: tk('Shuffle'), hint: tk('Nothing repeats until everything has played') },
]
const ALIGN = [{ v: 0, label: tk('Back to back') }, { v: 5, label: tk('Every 5 minutes') }, { v: 15, label: tk('Quarter hours') }, { v: 30, label: tk('Half hours') }, { v: 60, label: tk('On the hour') }]
const FITS: { v: NonNullable<Block['fit']>; label: string }[] = [
  { v: 'fill', label: tk('Finish on time: fill the gap') }, { v: 'overrun', label: tk('Let the show finish') }, { v: 'cut', label: tk('Cut it at the end of the slot') },
]

type Draft = { name: string; number: string; description: string; definition: Definition; visibility: Visibility; countProgress: boolean; enabled: boolean; categories: string[]; look: ChannelLook }

const blankDraft = (): Draft => ({ name: '', number: '', description: '', definition: emptyDefinition(), visibility: { mode: '' }, countProgress: false, enabled: true, categories: [], look: {} })
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
  const [categories, setCategories] = useState('')
  const baseline = useRef('')
  const catsLoaded = useRef(false)
  useEffect(() => { if (draft && !catsLoaded.current) { catsLoaded.current = true; setCategories((draft.categories || []).join(', ')) } }, [draft])
  const jump = (target: string) => (e: React.MouseEvent) => {
    e.preventDefault()
    document.getElementById(target)?.scrollIntoView({ block: 'start', behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
  }

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
        finish({ name: c.name, number: c.number, description: c.description, definition: { ...emptyDefinition(), ...c.definition, blocks: c.definition.blocks || [] }, visibility: c.visibility || { mode: '' }, countProgress: c.countProgress, enabled: c.enabled, categories: c.categories || [], look: c.look || {} })
      }).catch((e) => setErr(e.message))
      api.get<typeof history>(`/channels/${id}/history`).then(setHistory).catch(() => {})
    } else {
      const st = loc.state as { suggestion?: Suggestion; clone?: boolean } | null
      const s = st?.suggestion
      const d = blankDraft()
      if (st?.clone) d.definition.mode = 'clone'
      if (s) { d.name = s.name; d.description = s.description; d.definition = { ...emptyDefinition(), ...s.definition, blocks: s.definition.blocks || [] } }
      finish(d)
    }
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  // A closed tab, or a browser that crashes, loses nothing.
  useEffect(() => {
    if (!draft) return
    const timer = setTimeout(() => {
      try {
        if (JSON.stringify(draft) === baseline.current) localStorage.removeItem(draftKey)
        else localStorage.setItem(draftKey, JSON.stringify(draft))
      } catch { /* full or blocked storage */ }
    }, 400)
    return () => clearTimeout(timer)
  }, [draft, draftKey])
  const dirty = !!draft && JSON.stringify(draft) !== baseline.current
  useEffect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])
  // Leaving for another page asks first. What is checked is the draft as it is at
  // that moment, so leaving straight after a save doesn't ask.
  const draftNow = useRef(draft)
  useEffect(() => { draftNow.current = draft }, [draft])
  const blocker = useBlocker(({ currentLocation, nextLocation }) => currentLocation.pathname !== nextLocation.pathname && !!draftNow.current && JSON.stringify(draftNow.current) !== baseline.current)
  const blocked = blocker.state === 'blocked'
  const blockerNow = useRef(blocker)
  useEffect(() => { blockerNow.current = blocker }, [blocker])
  // Once per time it's blocked, whatever else redraws the page meanwhile.
  useEffect(() => {
    if (!blocked) return
    void ask({ title: t('Leave without saving?'), body: t('Your changes to this channel aren’t saved yet. They stay in this browser and come back the next time you open it.'), confirm: t('Leave'), cancel: t('Keep editing') })
      .then((yes) => { const b = blockerNow.current; if (b.state === 'blocked') { if (yes) b.proceed(); else b.reset() } })
  }, [blocked])
  // Back is where this page was opened from: the channel list, the guide or a
  // settings page. Opened from a link or a bookmark, it's the channel list. The
  // router numbers its history entries, and a redirect from an old address
  // replaces the first one rather than adding to it.
  const cameFrom = ((window.history.state as { idx?: number } | null)?.idx ?? 0) > 0
  const back = () => (cameFrom ? nav(-1) : nav('/tv/channels'))

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
      toast(id ? t('Saved. Changes start after what is on now.') : t('{name} is on the air', { name: draft.name }))
      back()
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const discard = () => {
    try { localStorage.removeItem(draftKey) } catch { /* ignore */ }
    setRestored(false)
    if (baseline.current) setDraft(JSON.parse(baseline.current))
  }
  const onLogo = async (f: File | undefined) => {
    if (!f || !id) return
    try { const r = await uploadLogo(Number(id), f); setLogo(r.logo); toast(t('Logo saved')) } catch (e: any) { toast(e.message) }
  }
  const removeLogo = async () => { try { await api.del(`/channels/${id}/logo`); setLogo('') } catch (e: any) { toast(e.message) } }
  const restore = async (h: number) => {
    if (!await ask({ title: t('Go back to this version?'), body: t('The current one is kept in this list too.'), confirm: t('Go back') })) return
    try {
      await api.post(`/channels/${id}/restore`, { history: h })
      try { localStorage.removeItem(draftKey) } catch { /* ignore */ }
      toast(t('Restored'))
      window.location.reload()
    } catch (e: any) { toast(e.message) }
  }
  const reshuffle = async () => {
    try { await api.post(`/channels/${id}/rebuild?reshuffle=1`); toast(t('New schedule from after what is on now')) } catch (e: any) { toast(e.message) }
  }

  if (err && !draft) return <p className="tvs-error" role="alert">{err}</p>
  if (!draft) return <div className="center"><div className="spinner" /></div>
  const def = draft.definition
  const levels: { v: Definition['mode']; label: string; hint: string }[] = [
    { v: 'simple', label: t('Simple'), hint: t('What it plays. That is all it needs.') },
    { v: 'rules', label: t('Rules'), hint: t('Exactly what it may play, day parts and bumpers.') },
    { v: 'timetable', label: t('Full timetable'), hint: t('Your own week, slot by slot.') },
    { v: 'clone', label: t('Clone a station'), hint: t('A real station’s schedule, from your own files.') },
  ]
  return (
    <>
      <PageHead title={id ? (saved?.name ? t('Edit {name}', { name: saved.name }) : t('Edit channel')) : t('New channel')} desc={t('Set as little or as much as you like. Moving between the levels below never loses anything.')}
        actions={<Link className="btn" to="/tv/channels" onClick={(e) => { if (cameFrom) { e.preventDefault(); back() } }}>{t('Back')}</Link>} />
      {restored && <p className="tvs-note" role="status">{t('Your unsaved changes from last time are back.')} <button className="btn sm ghost" onClick={discard}>{t('Discard them')}</button></p>}

      <Section title={tk('The channel')}>
        <SpecPage embedded bare page={basicsSpec({ name: draft.name, number: draft.number, description: draft.description, categories })} handled={new Set()}
          onField={(f, v) => { const x = String(v); if (f === 'name') setDraft({ ...draft, name: x.slice(0, 200) }); else if (f === 'number') setDraft({ ...draft, number: x.slice(0, 12) }); else if (f === 'description') setDraft({ ...draft, description: x.slice(0, 2000) }); else if (f === 'categories') { setCategories(x); setDraft({ ...draft, categories: x.split(',').map((y) => y.trim()).filter(Boolean) }) } }}
          after={{ 0: (
            <div className="tve-accent">
              <label>{t("The channel's own colour")}<input type="color" value={draft.look.accent || '#3478f6'} onChange={(e) => setDraft({ ...draft, look: { ...draft.look, accent: e.target.value } })} /></label>
              {draft.look.accent && <button type="button" className="btn sm ghost" onClick={() => setDraft({ ...draft, look: { ...draft.look, accent: undefined } })}>{t("Use the guide's colours")}</button>}
            </div>) }} />
        {opts && <LogoChoice look={draft.look} def={def} opts={opts} ownLogo={!!logo} set={(l) => setDraft({ ...draft, look: { ...draft.look, ...l } })} />}
      </Section>

      <nav className="tve-jump" aria-label={t('Parts of this channel')}>
        <a href="#ch-plays" onClick={jump('ch-plays')}>{t('What it plays')}</a>
        {def.mode !== 'clone' && <a href="#ch-discs" onClick={jump('ch-discs')}>{t('Discs and scenes')}</a>}
        <a href="#ch-commercials" onClick={jump('ch-commercials')}>{t('Commercials')}</a>
        <a href="#ch-bumpers" onClick={jump('ch-bumpers')}>{t('Bumpers')}</a>
        <a href="#ch-bumpers-trim" onClick={jump('ch-bumpers-trim')}>{t('Timetable')}</a>
        <a href="#ch-schedule" onClick={jump('ch-schedule')}>{tc('programmes', 'Schedule')}</a>
        <a href="#ch-who" onClick={jump('ch-who')}>{t('Who sees it')}</a>
      </nav>

      <div className="tvs-levels" id="ch-plays" role="radiogroup" aria-label={t('How much do you want to set up?')}>
        {levels.map((l) => (
          <button key={l.v} type="button" role="radio" aria-checked={def.mode === l.v} className={'tvs-level' + (def.mode === l.v ? ' on' : '')} onClick={() => setDef({ mode: l.v })}>
            <b>{l.label}</b><span>{l.hint}</span>
          </button>
        ))}
      </div>

      {def.mode === 'simple' && opts && <SimpleLevel def={def} setRules={setRules} setDef={setDef} opts={opts} />}
      {def.mode === 'rules' && opts && <RulesLevel def={def} setRules={setRules} setDef={setDef} opts={opts} />}
      {def.mode === 'timetable' && opts && <TimetableLevel def={def} setDef={setDef} setRules={setRules} opts={opts} />}
      {def.mode === 'clone' && <CloneStation def={def} setDef={setDef} name={draft.name} setName={(n) => setDraft((d) => (d && !d.name.trim() ? { ...d, name: n.slice(0, 200) } : d))} />}

      {def.mode !== 'clone' && <DiscScenes id="ch-discs" rules={def.rules} set={setRules} />}
      <CommercialsSection def={def} setDef={setDef} clone={def.mode === 'clone'} />
      {def.mode !== 'simple' && def.mode !== 'clone' && opts && <FillerSection def={def} setDef={setDef} opts={opts} />}
      {opts && <BumperSection id="ch-bumpers" def={def} setDef={setDef} opts={opts} />}

      <Preview def={def} />

      <Section id="ch-who" title={tk('Who sees it')}>
        <Audience draft={draft} setDraft={setDraft} />
      </Section>

      {id && (
        <Section title={tk('More')}>
          {logo && <img className="tvs-logo" src={mediaUrl(`/api/channels/${id}/logo?v=${logo}`)} alt={t('Current logo')} />}
          <SpecPage embedded bare page={moreSpec({ hasLogo: !!logo, dirty, history: history.map((h) => ({ id: h.id, name: h.name, when: fmtDateTime(h.savedAt * 1000) })) })}
            handled={new Set(['removeLogo', 'reshuffle', 'reset', ...history.map((h) => 'restore:' + h.id)])}
            onField={(f, v) => { if (f === 'logo' && v instanceof File) void onLogo(v) }}
            onAction={(a) => { if (a === 'removeLogo') void removeLogo(); else if (a === 'reshuffle') void reshuffle(); else if (a === 'reset') void resetChannel(Number(id), saved?.name || draft.name); else if (a.startsWith('restore:')) void restore(Number(a.slice(8))) }} />
          <LegalNote kind="logos" className="set-legal" />
        </Section>
      )}

      {err && <p className="tvs-error" role="alert">{err}</p>}
      {(dirty || !id) && (
        <div className="set-savebar" role="region" aria-label={t('Save this channel')}>
          <span>{dirty && <><i className="set-dot" />{t('Unsaved changes, kept in this browser')}</>}</span>
          {dirty && <button className="btn ghost sm" onClick={discard} disabled={busy}>{t('Discard')}</button>}
          <button className="btn primary sm" onClick={() => void save()} disabled={busy || !draft.name.trim()}>{busy ? t('Saving…') : id ? t('Save changes') : t('Create the channel')}</button>
        </div>
      )}
    </>
  )
}

// Which logo the channel wears. Left alone it's the one network or studio it plays,
// once the logo service has one: an owner's download or upload, never bundled.
function LogoChoice({ look, def, opts, ownLogo, set }: { look: ChannelLook; def: Definition; opts: EditorOptions; ownLogo: boolean; set: (l: ChannelLook) => void }) {
  const value = look.logoKind === 'none' ? 'none' : look.logoName ? `${look.logoKind}|${look.logoName}` : ''
  const brand = brandOf(def, look)
  // A name of someone's own, not one in this library's lists.
  const other = !!look.logoName && !opts.networks.some((n) => n.name === look.logoName) && !opts.studios.includes(look.logoName)
  const [typing, setTyping] = useState('')
  return (
    <div className="tve-logo">
      <label>{t('Logo')}
        <select className="select" value={value} onChange={(e) => {
          const v = e.target.value
          if (v === '') set({ logoKind: undefined, logoName: undefined })
          else if (v === 'none') set({ logoKind: 'none', logoName: undefined })
          else { const [k, ...n] = v.split('|'); set({ logoKind: k as ChannelLook['logoKind'], logoName: n.join('|') }) }
        }}>
          <option value="">{t('Worked out from what it plays')}</option>
          <option value="none">{t('Just its name')}</option>
          {other && <option value={value}>{look.logoName}</option>}
          {opts.networks.length > 0 && <optgroup label={t('Networks')}>{opts.networks.map((n) => <option key={n.name} value={`network|${n.name}`}>{n.name}</option>)}</optgroup>}
          {opts.studios.length > 0 && <optgroup label={t('Studios')}>{opts.studios.map((n) => <option key={n} value={`studio|${n}`}>{n}</option>)}</optgroup>}
        </select>
        <small className="tvs-help">{ownLogo ? t('Your own uploaded logo comes first, while you have one.') + ' ' : ''}{t("Without one the guide uses the channel's name.")}</small>
      </label>
      <form className="tve-logo-name" onSubmit={(e) => { e.preventDefault(); if (typing.trim()) { set({ logoKind: 'network', logoName: typing.trim().slice(0, 120) }); setTyping('') } }}>
        <label>{t('Or another name')}<input className="input" value={typing} maxLength={120} placeholder={t('Any network or studio')} onChange={(e) => setTyping(e.target.value)} /></label>
        <button className="btn sm" disabled={!typing.trim()}>{t('Use it')}</button>
      </form>
      {brand && <span className="tve-logo-show"><BrandLogo kind={brand.kind} name={brand.name} /></span>}
      {brand && <ChangeLogoButton kind={brand.kind} name={brand.name} label={t('Search, upload or choose none')} />}
      <LogoConsentHint compact />
    </div>
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
      <legend>{label}{value.length > 0 && <span className="tvs-count"> · {t('{count} chosen', { count: value.length })}</span>}</legend>
      {options.length > 12 && <input className="input" type="search" aria-label={t('Find in {name}', { name: label })} placeholder={t('Find…')} value={q} onChange={(e) => setQ(e.target.value)} />}
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
      <label>{t('How it plays')}
        <select className="select" value={order.mode || 'rotate'} onChange={(e) => onChange({ ...order, mode: e.target.value as Order['mode'] })}>
          {ORDERS.map((o) => <option key={o.v} value={o.v} title={t(o.hint)}>{t(o.label)}</option>)}
        </select>
        <small className="tvs-help">{tx(ORDERS.find((o) => o.v === (order.mode || 'rotate'))?.hint)}</small>
      </label>
      {order.mode === 'shuffle' && <label className="check-row"><input type="checkbox" checked={!!order.keepEpisodeOrder} onChange={(e) => onChange({ ...order, keepEpisodeOrder: e.target.checked })} /> {t("Keep each show's episodes in order")}</label>}
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
    <Section title={tk('What it plays')}>
      <div className="form" style={{ maxWidth: 'none' }}>
        <label>{t('Play')}
          <select className="select" value={kind} onChange={(e) => change(e.target.value)}>
            <option value="all">{t('Everything')}</option>
            {opts.networks.length > 0 && <option value="network">{t('One TV network')}</option>}
            <option value="genre">{t('A genre')}</option>
            <option value="decade">{t('A decade')}</option>
            <option value="show">{t('One show')}</option>
            <option value="movies">{t('Movies')}</option>
          </select>
        </label>
        {kind === 'network' && (
          <label>{t('Network')}
            <select className="select" value={r.networks?.[0] || ''} onChange={(e) => setRules({ networks: e.target.value ? [e.target.value] : undefined })}>
              <option value="">{t('Choose…')}</option>
              {opts.networks.map((n) => <option key={n.name} value={n.name}>{n.name} ({tn('{count} show', '{count} shows', n.shows)}, {tn('{count} episode', '{count} episodes', n.episodes)})</option>)}
            </select>
          </label>
        )}
        {kind === 'genre' && (
          <label>{t('Genre')}
            <select className="select" value={r.genres?.[0] || ''} onChange={(e) => setRules({ genres: e.target.value ? [e.target.value] : undefined })}>
              <option value="">{t('Choose…')}</option>
              {opts.genres.map((g) => <option key={g} value={g}>{g}</option>)}
            </select>
          </label>
        )}
        {kind === 'decade' && (
          <label>{t('Decade')}
            <select className="select" value={r.decades?.[0] || ''} onChange={(e) => setRules({ decades: e.target.value ? [Number(e.target.value)] : undefined })}>
              <option value="">{t('Choose…')}</option>
              {opts.decades.map((d) => <option key={d} value={d}>{t('{decade}s', { decade: String(d) })}</option>)}
            </select>
          </label>
        )}
        {kind === 'show' && (
          <label>{t('Show')}
            <select className="select" value={r.shows?.[0] || ''} onChange={(e) => setRules({ shows: e.target.value ? [Number(e.target.value)] : undefined })}>
              <option value="">{t('Choose…')}</option>
              {opts.shows.map((s) => <option key={s.id} value={s.id}>{s.title}{s.year ? ` (${s.year})` : ''}</option>)}
            </select>
          </label>
        )}
        <OrderPick order={def.order} onChange={(o) => setDef({ order: o })} />
        <label>{t('Shows start')}
          <select className="select" value={def.alignMin} onChange={(e) => setDef({ alignMin: Number(e.target.value) })}>
            {ALIGN.map((a) => <option key={a.v} value={a.v}>{t(a.label)}</option>)}
          </select>
          <small className="tvs-help">{t("Real TV starts on the half hour. The gaps get a “We'll be right back” card, or your own bumpers (under Rules).")}</small>
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
      {!compact && libraries.length > 1 && <Multi label={t('Libraries')} options={libraries.map((l) => ({ v: String(l.id), label: l.name }))} value={strs(r.libraries)} onChange={(v) => set({ libraries: v.map(Number) })} />}
      <Multi label={t('Kind of title')} options={[{ v: 'episodes', label: t('TV episodes') }, { v: 'movies', label: t('Movies') }, { v: 'discs', label: t('DVDs and Blu-rays') }]} value={r.kinds || []} onChange={(v) => set({ kinds: v })} />
      <Multi label={t("TV networks (or a movie's studio)")} options={opts.networks.map((n) => ({ v: n.name, label: `${n.name} (${n.shows})` }))} value={r.networks || []} onChange={(v) => set({ networks: v })} />
      <Multi label={t('Genres')} options={opts.genres.map((g) => ({ v: g, label: g }))} value={r.genres || []} onChange={(v) => set({ genres: v })} />
      <Multi label={t('Shows')} options={showOpts(opts)} value={strs(r.shows)} onChange={(v) => set({ shows: v.map(Number) })} />
      <Multi label={t('Decades')} options={opts.decades.map((d) => ({ v: String(d), label: t('{decade}s', { decade: String(d) }) }))} value={strs(r.decades)} onChange={(v) => set({ decades: v.map(Number) })} />
      <Multi label={t('Collections')} options={opts.collections.map((c) => ({ v: String(c.id), label: `${c.name} (${c.items})` }))} value={strs(r.collections)} onChange={(v) => set({ collections: v.map(Number) })} />
      {!compact && (
        <>
          <Multi label={t('Leave out these genres')} options={opts.genres.map((g) => ({ v: g, label: g }))} value={r.excludeGenres || []} onChange={(v) => set({ excludeGenres: v })} />
          <Multi label={t('Leave out these shows')} options={showOpts(opts)} value={strs(r.excludeShows)} onChange={(v) => set({ excludeShows: v.map(Number) })} />
          <Multi label={t('Studios')} options={opts.studios.map((g) => ({ v: g, label: g }))} value={r.studios || []} onChange={(v) => set({ studios: v })} />
          <Multi label={t('Certificates')} options={opts.ratings.map((g) => ({ v: g, label: g }))} value={r.ratings || []} onChange={(v) => set({ ratings: v })} />
          <div className="tvs-three">
            <label>{t('From year')}<input className="input" type="number" min={1880} max={2200} value={val(r.yearFrom)} onChange={num('yearFrom')} /></label>
            <label>{t('To year')}<input className="input" type="number" min={1880} max={2200} value={val(r.yearTo)} onChange={num('yearTo')} /></label>
            <label>{t('Lowest score (0–10)')}<input className="input" type="number" step={0.5} min={0} max={10} value={val(r.minScore)} onChange={num('minScore')} /></label>
            <label>{t('From season')}<input className="input" type="number" min={0} value={val(r.seasonFrom)} onChange={num('seasonFrom')} /></label>
            <label>{t('To season')}<input className="input" type="number" min={0} value={val(r.seasonTo)} onChange={num('seasonTo')} /></label>
            <label>{t('Highest age rating')}<input className="input" type="number" min={0} max={21} value={r.maxAge ?? ''} onChange={(e) => set({ maxAge: e.target.value === '' ? undefined : Number(e.target.value) })} /></label>
            <label>{t('At least (minutes)')}<input className="input" type="number" min={0} value={val(r.minMinutes)} onChange={num('minMinutes')} /></label>
            <label>{t('At most (minutes)')}<input className="input" type="number" min={0} value={val(r.maxMinutes)} onChange={num('maxMinutes')} /></label>
            <label>{t('Watched')}
              <select className="select" value={r.watched || ''} onChange={(e) => set({ watched: e.target.value as Rules['watched'] })}>
                <option value="">{t('Either')}</option><option value="unwatched">{t('Not yet watched')}</option><option value="watched">{t('Already watched')}</option>
              </select>
              <small className="tvs-help">{t('Going by your own watch history.')}</small>
            </label>
          </div>
          <label>{t('Title contains')}<input className="input" value={r.search || ''} maxLength={200} onChange={(e) => set({ search: e.target.value })} /></label>
          <label className="check-row"><input type="checkbox" checked={!!r.allowUnrated} onChange={(e) => set({ allowUnrated: e.target.checked })} /> {t('With an age rating set: also play titles that have no rating')}</label>
          <label className="check-row"><input type="checkbox" checked={!!r.includeSpecials} onChange={(e) => set({ includeSpecials: e.target.checked })} /> {t('Include specials')}</label>
        </>
      )}
    </div>
  )
}

function RulesLevel({ def, setRules, setDef, opts }: { def: Definition; setRules: (p: Partial<Rules>) => void; setDef: (p: Partial<Definition>) => void; opts: EditorOptions }) {
  return (
    <>
      <Section title={tk('What it may play')} desc={t('Leave anything empty and it does not limit the channel.')}>
        <RuleFields r={def.rules} set={setRules} opts={opts} />
      </Section>
      <Section title={tk('How it plays')}>
        <div className="form" style={{ maxWidth: 'none' }}>
          <OrderPick order={def.order} onChange={(o) => setDef({ order: o })} />
          <label>{t('Shows start')}
            <select className="select" value={def.alignMin} onChange={(e) => setDef({ alignMin: Number(e.target.value) })}>
              {ALIGN.map((a) => <option key={a.v} value={a.v}>{t(a.label)}</option>)}
            </select>
          </label>
        </div>
      </Section>
      <DayParts def={def} setDef={setDef} opts={opts} />
    </>
  )
}

const CLASSIC_PARTS: { name: string; start: string; end: string; genres: string[] }[] = [
  { name: tk('Morning'), start: '06:00', end: '12:00', genres: ['Children', 'Kids', 'Family', 'Animation'] },
  { name: tk('Afternoon'), start: '12:00', end: '17:00', genres: ['Soap', 'Game Show', 'Reality', 'Drama'] },
  { name: tk('Early evening'), start: '17:00', end: '20:00', genres: ['Comedy', 'Family', 'Animation'] },
  { name: tk('Prime time'), start: '20:00', end: '23:00', genres: ['Drama', 'Crime', 'Action', 'Thriller', 'Comedy'] },
  { name: tk('Late night'), start: '23:00', end: '02:00', genres: ['Horror', 'Comedy', 'Thriller'] },
]

function DayParts({ def, setDef, opts }: { def: Definition; setDef: (p: Partial<Definition>) => void; opts: EditorOptions }) {
  const blocks = def.blocks || []
  const addClassic = () => {
    const known = (g: string[]) => g.filter((x) => opts.genres.some((o) => o.toLowerCase() === x.toLowerCase()))
    const parts = CLASSIC_PARTS.filter((p) => known(p.genres).length > 0).map((p): Block => ({ id: newBlockId(), name: t(p.name), start: p.start, end: p.end, rules: { ...def.rules, genres: known(p.genres) } }))
    if (parts.length === 0) { toast(t('Your library has none of the genres the classic day parts use')); return }
    setDef({ blocks: [...blocks, ...parts] })
  }
  return (
    <Section title={tk('Day parts')} desc={t('Different things at different times: Saturday morning cartoons, prime time drama, a Christmas season, a weekend marathon. The rest of the day plays the rules above.')}
      actions={<>
        <button className="btn sm" onClick={addClassic}>{t('Add the classic day parts')}</button>
        <button className="btn sm" onClick={() => setDef({ blocks: [...blocks, { id: newBlockId(), name: t('Holiday season'), start: '00:00', end: '23:59', fromDate: '12-01', toDate: '12-26', rules: { genres: [] } }] })}>{t('Add a holiday block')}</button>
        <button className="btn sm" onClick={() => setDef({ blocks: [...blocks, { id: newBlockId(), name: t('Marathon'), start: '12:00', end: '20:00', days: [6], marathon: true, order: { mode: 'sequential' }, rules: { shows: [] } }] })}>{t('Add a marathon')}</button>
      </>}>
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
      if (!r.definition.blocks?.length) { toast(t('The metadata does not say when these shows aired. Try “Fill in missing facts” in the channel list.')); return }
      setDef({ blocks: [...(def.blocks || []), ...r.definition.blocks] })
      setRules({ networks: [net] })
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  return (
    <>
      <Section title={tk('The week')} desc={t("Slots repeat every week. Time no slot covers plays the channel's rules, if it has any, or a “We'll be right back” card.")}
        actions={net ? <button className="btn sm" disabled={busy} onClick={() => void asAired()}>{t('Fill in from when {network} shows aired', { network: net })}</button> : undefined}>
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
      {dayNames('short').map((d, i) => (
        <div key={i} className="tvs-week-day">
          <div className="tvs-week-name">{d}</div>
          <div className="tvs-week-track">
            {blocks.filter((b) => !b.days?.length || b.days.includes(i)).map((b) => {
              const s = toPct(b.start)
              const e = toPct(b.end)
              return <i key={b.id} style={{ top: s + '%', height: Math.max(1.5, (e > s ? e : 100) - s) + '%' }} title={`${b.name || t('Slot')} ${b.start}–${b.end}`} />
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
      {sorted.length === 0 && <p className="muted">{t('Nothing here yet.')}</p>}
      <ul className="tvs-blocks">
        {sorted.map((b, i) => <BlockEditor key={b.id} n={i + 1} b={b} opts={opts} up={(p) => up(b.id, p)} remove={() => remove(b.id)} copy={() => copy(b)} />)}
      </ul>
      <button className="btn sm" onClick={add}><IconPlus /> {t('Add a slot')}</button>
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
  const label = b.name || t('Slot {n}', { n })
  const short = dayNames('short'), long = dayNames('long')
  return (
    <li className="tvs-block">
      <div className="tvs-block-top">
        <label className="tvs-block-name">{t('Name')}<input className="input" value={b.name || ''} maxLength={200} placeholder={t('Slot {n}', { n })} onChange={(e) => up({ name: e.target.value })} /></label>
        <label>{t('Starts')}<input className="input" type="time" value={b.start} onChange={(e) => up({ start: e.target.value })} /></label>
        <label>{t('Ends')}<input className="input" type="time" value={b.end} onChange={(e) => up({ end: e.target.value })} /></label>
        <button className="btn sm ghost" onClick={copy} aria-label={t('Copy {name}', { name: label })}>{t('Copy')}</button>
        <button className="btn sm ghost" onClick={remove} aria-label={t('Remove {name}', { name: label })}><IconTrash /></button>
      </div>
      <div className="tvs-days" role="group" aria-label={t('Days of {name}', { name: label })}>
        {short.map((d, i) => <button key={i} type="button" aria-pressed={days.includes(i)} aria-label={long[i]} className={'tvs-day' + (days.includes(i) ? ' on' : '')} onClick={() => toggleDay(i)}>{d}</button>)}
        <span className="tvs-dim">{days.length === 0 ? t('Every day') : ''}</span>
      </div>
      <div className="tvs-three">
        <label>{t('Plays')}
          <select className="select" value={open && pick.kind === 'custom' ? 'custom' : pick.kind} onChange={(e) => setPick(e.target.value as Pick['kind'])}>
            <option value="">{t("The channel's rules")}</option>
            <option value="show">{t('One show')}</option>
            <option value="genre">{t('A genre')}</option>
            {opts.networks.length > 0 && <option value="network">{t('A network')}</option>}
            <option value="decade">{t('A decade')}</option>
            <option value="custom">{t('Its own rules…')}</option>
          </select>
        </label>
        {pick.kind === 'show' && <label>{t('Show')}<select className="select" value={pick.value} onChange={(e) => setPick('show', e.target.value)}><option value="">{t('Choose…')}</option>{opts.shows.map((s) => <option key={s.id} value={s.id}>{s.title}</option>)}</select></label>}
        {pick.kind === 'genre' && <label>{t('Genre')}<select className="select" value={pick.value} onChange={(e) => setPick('genre', e.target.value)}><option value="">{t('Choose…')}</option>{opts.genres.map((g) => <option key={g} value={g}>{g}</option>)}</select></label>}
        {pick.kind === 'network' && <label>{t('Network')}<select className="select" value={pick.value} onChange={(e) => setPick('network', e.target.value)}><option value="">{t('Choose…')}</option>{opts.networks.map((x) => <option key={x.name} value={x.name}>{x.name}</option>)}</select></label>}
        {pick.kind === 'decade' && <label>{t('Decade')}<select className="select" value={pick.value} onChange={(e) => setPick('decade', e.target.value)}><option value="">{t('Choose…')}</option>{opts.decades.map((d) => <option key={d} value={d}>{t('{decade}s', { decade: String(d) })}</option>)}</select></label>}
        <label>{t('Order')}
          <select className="select" value={b.order?.mode || ''} onChange={(e) => up({ order: e.target.value ? { mode: e.target.value as Order['mode'], keepEpisodeOrder: b.order?.keepEpisodeOrder } : undefined })}>
            <option value="">{t('As the channel')}</option>
            {ORDERS.map((o) => <option key={o.v} value={o.v}>{t(o.label)}</option>)}
          </select>
        </label>
        <label>{t("If it doesn't fit")}
          <select className="select" value={b.fit || 'fill'} onChange={(e) => up({ fit: e.target.value as Block['fit'] })}>
            {FITS.map((f) => <option key={f.v} value={f.v}>{t(f.label)}</option>)}
          </select>
        </label>
      </div>
      <div className="tve-season">
        <label>{t('Only from (MM-DD)')}<input className="input" value={b.fromDate || ''} placeholder={t('All year')} maxLength={5} aria-label={t('{name}: first date of the season', { name: label })} onChange={(e) => up({ fromDate: e.target.value || undefined })} /></label>
        <label>{t('To (MM-DD)')}<input className="input" value={b.toDate || ''} placeholder={t('All year')} maxLength={5} aria-label={t('{name}: last date of the season', { name: label })} onChange={(e) => up({ toDate: e.target.value || undefined })} /></label>
        <label className="check-row"><input type="checkbox" checked={!!b.marathon} onChange={(e) => up({ marathon: e.target.checked })} /> {t('Marathon: straight on, no commercials')}</label>
      </div>
      {pick.kind === 'custom' && b.rules && (
        <div className="tvs-block-rules"><RuleFields compact r={b.rules} set={(p) => up({ rules: { ...b.rules, ...p } })} opts={opts} /></div>
      )}
    </li>
  )
}

// Commercials: any number of libraries of them, each whole or only the videos
// chosen here, per channel.
function CommercialsSection({ def, setDef, clone }: { def: Definition; setDef: (p: Partial<Definition>) => void; clone?: boolean }) {
  const libs = useShortLibraries()
  const f = def.filler || {}
  const picks = f.commercials || []
  const bumperLibs = def.bumpers?.libraries || []
  const setPicks = (next: CommercialPick[]) => setDef({ filler: { ...f, commercials: next, between: picks.length === 0 && next.length > 0 ? f.between || 2 : f.between } })
  if (libs.length === 0) {
    return (
      <Section id="ch-commercials" title={tk('Commercials')} desc={t('Your own ads between shows.')}>
        <p className="muted">{t('You have no commercials library yet.')} <Link to="/settings/libraries">{t('Add one under Libraries')}</Link>. {t('Choose the kind called Commercials. You can have as many as you like: one for the 1990s, one for toys, one for each channel.')}</p>
      </Section>
    )
  }
  const inUse = picks.length > 0 || (!!f.fromBumpers && bumperLibs.length > 0)
  return (
    <Section id="ch-commercials" title={tk('Commercials')} desc={t('Play your own ads between shows and in the gaps. Tick as many commercials libraries as you like, each whole or only the videos you choose.')}>
      <LibraryPicks label={t('Commercials libraries')} picks={picks} onChange={setPicks} note={(id) => (bumperLibs.some((p) => p.library === id) ? t('a bumper library here') : '')} />
      <label className="check-row">
        <input type="checkbox" checked={!!f.fromBumpers} onChange={(e) => setDef({ filler: { ...f, fromBumpers: e.target.checked || undefined } })} /> {t('Commercials may also come from the bumper libraries')}
      </label>
      {f.fromBumpers && bumperLibs.length === 0 && <p className="tvs-note" role="status">{t('This channel has no bumper libraries yet: choose them under Bumpers.')}</p>}
      {clone && <p className="tvs-note">{t('On a cloned station, commercials fill the time each programme leaves: some in breaks through it, the rest after it. Choose how many breaks under Advanced options.')}</p>}
      {inUse && !clone && (
        <div className="tvs-three">
          <label>{t('After each show, play')}
            <select className="select" value={f.between || 0} onChange={(e) => setDef({ filler: { ...f, between: Number(e.target.value) } })}>
              {[0, 1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{n === 0 ? t('Only in gaps') : tn('{count} commercial', '{count} commercials', n)}</option>)}
            </select>
          </label>
          <label>{t('No longer than (seconds)')}<input className="input" type="number" min={0} value={f.maxSeconds || ''} onChange={(e) => setDef({ filler: { ...f, maxSeconds: Number(e.target.value) || 0 } })} /></label>
          <label>{t('During a programme, a break every')}
            <select className="select" value={f.breakEveryMin || 0} onChange={(e) => { const m = Number(e.target.value); setDef({ filler: { ...f, breakEveryMin: m, breakAds: m ? f.breakAds || 2 : 0 } }) }}>
              {[0, 10, 15, 20, 30, 45, 60].map((m) => <option key={m} value={m}>{m === 0 ? t('No breaks') : tn('{count} minute', '{count} minutes', m)}</option>)}
            </select>
          </label>
          {!!f.breakEveryMin && (
            <label>{t('With')}
              <select className="select" value={f.breakAds || 2} onChange={(e) => setDef({ filler: { ...f, breakAds: Number(e.target.value) } })}>
                {[1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{tn('{count} commercial', '{count} commercials', n)}</option>)}
              </select>
            </label>
          )}
        </div>
      )}
      {!!f.breakEveryMin && !clone && <p className="muted" style={{ fontSize: 13 }}>{t("No break comes so near the end that you'd just wait for the programme to finish. With “Cut commercials short” on, these give way too, once those after the programme have.")}</p>}
      <p className="muted" style={{ fontSize: 13 }}>{t("Commercials count as part of the channel for parental limits. A Kids profile won't see this channel unless these videos have a suitable rating (edit a video to give it one).")}</p>
    </Section>
  )
}

function FillerSection({ def, setDef, opts }: { def: Definition; setDef: (p: Partial<Definition>) => void; opts: EditorOptions }) {
  const { libraries } = useApp()
  const f = def.filler || {}
  const rules = f.rules
  const setF = (p: Partial<Definition['filler']>) => setDef({ filler: { ...f, ...p } })
  return (
    <Section title={tk('Other filler')} desc={t('Anything else from your files to play between shows: station idents or trailers from any library, chosen by the rules below. They never count as watched.')}>
      <div className="form" style={{ maxWidth: 'none' }}>
        <label className="check-row"><input type="checkbox" checked={!!rules} onChange={(e) => setF({ rules: e.target.checked ? {} : undefined })} /> {t('Use filler from my files')}</label>
        {rules && (
          <>
            {libraries.length > 0 && <Multi label={t('From these libraries')} options={libraries.map((l) => ({ v: String(l.id), label: l.name }))} value={strs(rules.libraries)} onChange={(v) => setF({ rules: { ...rules, libraries: v.map(Number) } })} />}
            <Multi label={t('From these shows')} options={showOpts(opts)} value={strs(rules.shows)} onChange={(v) => setF({ rules: { ...rules, shows: v.map(Number) } })} />
            <div className="tvs-three">
              <label>{t('After each show, play')}
                <select className="select" value={f.between || 0} onChange={(e) => setF({ between: Number(e.target.value) })}>
                  {[0, 1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{n === 0 ? t('Only in gaps') : tn('{count} bumper', '{count} bumpers', n)}</option>)}
                </select>
              </label>
              <label>{t('No longer than (seconds)')}<input className="input" type="number" min={0} value={f.maxSeconds || ''} onChange={(e) => setF({ maxSeconds: Number(e.target.value) || 0 })} /></label>
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
    <SpecPage embedded bare page={audienceSpec({ mode: v.mode, countProgress: draft.countProgress })}
      onField={(f, x) => { if (f === 'mode') setDraft({ ...draft, visibility: { ...v, mode: x as Visibility['mode'] } }); else if (f === 'count') setDraft({ ...draft, countProgress: !!x }) }}
      after={v.mode === 'only' ? { 0: <Multi label={t('Profiles')} options={users.map((u) => ({ v: String(u.id), label: u.name }))} value={strs(v.users)} onChange={(x) => setDraft({ ...draft, visibility: { ...v, users: x.map(Number) } })} /> } : undefined} />
  )
}

// What the channel will really do: every programme, bumper, commercial and gap,
// with what gave way to keep to the timetable. This is the schedule the server
// would make from this draft, without saving it.
function Preview({ def }: { def: Definition }) {
  const [day, setDay] = useState(0)
  const [data, setData] = useState<{ stats: PoolStats; programmes: Programme[] } | null>(null)
  const [err, setErr] = useState('')
  const [all, setAll] = useState(true)
  const key = useMemo(() => JSON.stringify(def), [def])
  useEffect(() => {
    const timer = setTimeout(() => {
      api.post<{ stats: PoolStats; programmes: Programme[] }>('/channels/preview', { definition: def, day }).then((d) => { setData(d); setErr('') }).catch((e) => setErr(e.message))
    }, 450)
    return () => clearTimeout(timer)
  }, [key, day]) // eslint-disable-line react-hooks/exhaustive-deps
  const when = new Date(Date.now() + day * 86400000)
  const list = data?.programmes || []
  const programmes = list.filter((p) => all || (p.kind !== 'filler' && !(p.kind === 'slate' && p.end - p.start < 900)))
  const empty = data && data.stats.titles === 0 && def.mode !== 'timetable' && !(def.rules.clips?.length)
  // The first two hours as a strip, to see the rhythm of it at a glance.
  const span = 2 * 3600
  const stripStart = list[0]?.start || 0
  const strip = list.filter((p) => p.start < stripStart + span)
  const secs = (f: (p: Programme) => boolean) => list.filter(f).reduce((n, p) => n + p.end - p.start, 0)
  const cuts = list.filter((p) => p.trimmed && p.kind === 'filler')
  const lost = list.filter((p) => p.trimmed && p.kind === 'show')
  const grid = def.alignMin * 60
  const shows = list.filter((p, i) => p.kind === 'show' && !p.offset && list[i - 1]?.role !== 'pre')
  const leads = list.filter((p) => p.role === 'pre')
  const offGrid = grid ? [...shows, ...leads].filter((p) => p.start % grid !== 0).length : 0
  const tag = (p: Programme) => ({ pre: t('Before'), post: t('After'), break: t('Break'), ad: t('Commercial'), between: t('Between'), pad: t('Fills gap') } as Record<string, string>)[p.role || ''] || t('Bumper')
  const fmtCut = (n: number) => (n >= 60 ? t('{m}m {s}s', { m: Math.floor(n / 60), s: n % 60 }) : t('{s}s', { s: n }))
  return (
    <Section id="ch-schedule" title={tk('What it will look like')} desc={data ? tn('The rules choose {count} title ({length}).', 'The rules choose {count} titles ({length}).', data.stats.titles, { length: fmtRuntime(data.stats.seconds) || t('no time') }) : undefined}
      actions={<>
        <label className="check-row tvs-inline"><input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} /> {t('Bumpers and commercials')}</label>
        <label className="tvs-inline">{t('Day')}<select className="select sm" value={day} onChange={(e) => setDay(Number(e.target.value))}>{Array.from({ length: 8 }, (_, i) => <option key={i} value={i}>{i === 0 ? t('Today') : date(Date.now() + i * 86400000, { weekday: 'long' })}</option>)}</select></label>
      </>}>
      {err && <p className="tvs-error" role="alert">{err}</p>}
      {empty && <p className="tvs-note" role="status">{t("Nothing in your library matches these rules, so the channel would show a “We'll be right back” card.")}</p>}
      {list.length > 0 && (
        <div className="tve-summary" role="group" aria-label={t('This day in numbers')}>
          <div className="tve-stat">{tn('{count} programme', '{count} programmes', shows.length + leads.length)}</div>
          <div className="tve-stat">{t('{length} of bumpers and commercials', { length: fmtRuntime(secs((p) => p.kind === 'filler')) || '0' })}</div>
          <div className="tve-stat">{t('{count} cut short, {length} in all', { count: cuts.length, length: fmtCut(cuts.reduce((n, p) => n + (p.trimmed || 0), 0)) })}</div>
          {lost.length > 0 && <div className="tve-stat">{tn('{count} programme lost {length} from its end', '{count} programmes lost {length} from their ends', lost.length, { length: fmtCut(lost.reduce((n, p) => n + (p.trimmed || 0), 0)) })}</div>}
          {grid > 0 && <div className="tve-stat">{t('{count} started off the timetable', { count: offGrid })}</div>}
        </div>
      )}
      {strip.length > 0 && (
        <>
          <div className="tve-strip" aria-hidden="true">
            {strip.map((p) => <i key={p.start} className={p.kind + (p.mandatory ? ' must' : '')} style={{ flex: `${Math.min(p.end, stripStart + span) - p.start} 0 0` }} title={p.title} />)}
          </div>
          <div className="tve-strip-key" aria-hidden="true"><span><i style={{ background: 'var(--accent)' }} />{t('Programme')}</span><span><i style={{ background: 'var(--fg)' }} />{t('Mandatory bumper')}</span><span><i style={{ background: 'var(--fg-2)' }} />{t('Other bumper or commercial')}</span><span><i style={{ background: 'var(--bg-4)' }} />{tc('title card', 'Card')}</span><span>{t('The first two hours')}</span></div>
        </>
      )}
      <ol className="tve-sched" aria-label={t('Schedule for {day}', { day: date(when) })}>
        {programmes.slice(0, 200).map((p) => (
          <li key={p.start} className={p.kind}>
            <time>{fmtClock(p.start)}</time>
            {p.kind === 'filler' && <span className={'tag' + (p.mandatory ? ' must' : '')}>{p.mandatory ? t('Mandatory') : tag(p)}</span>}
            <span className="main"><span>{p.kind === 'slate' ? t("We'll be right back") : p.title}</span>{p.sub && <small> {p.sub}</small>}{p.offset ? <small> {t('(continues)')}</small> : null}{p.rule && <small> · {p.rule}</small>}</span>
            {p.trimmed ? <span className="cut">{t('{length} cut', { length: fmtCut(p.trimmed) })}</span> : null}
            <small>{fmtCut(p.end - p.start)}</small>
          </li>
        ))}
      </ol>
      {programmes.length > 200 && <p className="muted">{t('…and {count} more.', { count: programmes.length - 200 })}</p>}
    </Section>
  )
}
