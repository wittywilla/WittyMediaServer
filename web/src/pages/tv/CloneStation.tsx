// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState } from 'react'
import { api, uploadFile } from '../../api'
import { toast } from '../../state'
import { Button, Dialog, Input, Select, Switch } from '../../ui'
import { Section } from '../settings/ui'
import { t, tk } from '../../i18n'
import type { Clone, CloneCandidate, CloneOptions, CloneRule, CloneSlot, CloneSource, Definition, Station } from './tvApi'
import './clone.css'

// Clone a station: a real station's schedule, played from the owner's own files. Pick where the
// guide comes from, the station and the days; every programme is matched to the library, and
// what isn't there is filled in by the closest thing that is, each saying how. One tap swaps a
// slot. Only the guide's words are used: nothing is recorded or decrypted.

type SourceKind = CloneSource['kind']

const COUNTRIES = ['US', 'GB', 'CA', 'AU', 'NZ', 'IE', 'DE', 'FR', 'NL', 'BE', 'SE', 'NO', 'DK', 'FI', 'ES', 'IT', 'JP', 'BR', 'MX', 'IN']

// What each rule means, in a word and a sentence.
export const RULES: Record<CloneRule, { label: string; hint: string; tone: 'exact' | 'stand' | 'missing' }> = {
  exact: { label: tk('Exact'), hint: tk('The same episode or film'), tone: 'exact' },
  title: { label: tk('Exact'), hint: tk('The same episode, found by its title'), tone: 'exact' },
  aired: { label: tk('Exact'), hint: tk('The same episode, found by when it first aired'), tone: 'exact' },
  manual: { label: tk('Your pick'), hint: tk('Chosen by you'), tone: 'exact' },
  near: { label: tk('Closest episode'), hint: tk('The same series, the nearest episode you have'), tone: 'stand' },
  series: { label: tk('Same series'), hint: tk('Another episode of the same series'), tone: 'stand' },
  similar: { label: tk('Similar show'), hint: tk('A show like it: genre, network, era'), tone: 'stand' },
  filler: { label: tk('Filler'), hint: tk('Whatever fits the time best'), tone: 'stand' },
  none: { label: tk('Missing'), hint: tk('Nothing fits: the channel’s own choices play'), tone: 'missing' },
}

const todayISO = () => { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}` }
const mins = (sec?: number) => Math.round((sec || 0) / 60)
const epLabel = (s: CloneSlot) => (s.season != null && s.episode != null ? `S${s.season}E${s.episode}` : '')

type ImportAnswer = { clone: Clone; commercials: number[]; counts: Record<string, number> }

export default function CloneStation({ def, setDef, name, setName }: { def: Definition; setDef: (p: Partial<Definition>) => void; name: string; setName: (n: string) => void }) {
  const clone = def.clone
  const [restart, setRestart] = useState(false)
  if (!clone || restart) {
    return <ClonePicker onCloned={(r) => {
      setRestart(false)
      const filler = { ...def.filler }
      if (!filler.commercials?.length && r.commercials.length) filler.commercials = r.commercials.map((library) => ({ library }))
      setDef({ mode: 'clone', clone: r.clone, filler, alignMin: 0 })
      if (!name.trim()) setName(r.clone.station)
    }} onCancel={clone ? () => setRestart(false) : undefined} />
  }
  return <ClonePreview clone={clone} set={(c) => setDef({ clone: c })} restart={() => setRestart(true)} />
}

// ---------------------------------------------------------------------------
// Picking a source, the days and a station
// ---------------------------------------------------------------------------

function ClonePicker({ onCloned, onCancel }: { onCloned: (r: ImportAnswer) => void; onCancel?: () => void }) {
  const [kind, setKind] = useState<SourceKind>('tvmaze')
  const [country, setCountry] = useState('US')
  const [url, setUrl] = useState('')
  const [file, setFile] = useState('')
  const [from, setFrom] = useState(todayISO())
  const [days, setDays] = useState(1)
  const [stations, setStations] = useState<Station[] | null>(null)
  const [credit, setCredit] = useState('')
  const [filter, setFilter] = useState('')
  const [busy, setBusy] = useState('')
  const [err, setErr] = useState('')
  const source = (): CloneSource => (kind === 'tvmaze' ? { kind, country } : kind === 'xmltv' ? { kind, url } : { kind })
  useEffect(() => { setStations(null) }, [kind, country, url, from, days])

  const upload = async (f?: File) => {
    if (!f) return
    setBusy('upload'); setErr('')
    try { const r = await uploadFile<{ url: string }>('/channels/clone/upload', f); setUrl(r.url); setFile(f.name) } catch (e: any) { setErr(e.message) } finally { setBusy('') }
  }
  const find = async () => {
    setBusy('find'); setErr('')
    try {
      const r = await api.post<{ stations: Station[]; credit: string }>('/channels/clone/stations', { source: source(), from, days })
      setStations(r.stations.filter((s) => kind !== 'xmltv' || (s.count || 0) > 0 || r.stations.length < 50)); setCredit(r.credit)
      if (!r.stations.length) setErr(t('That guide has no stations for those days.'))
    } catch (e: any) { setErr(e.message) } finally { setBusy('') }
  }
  const pick = async (st: Station) => {
    setBusy('import:' + st.id); setErr('')
    try {
      const r = await api.post<ImportAnswer>('/channels/clone/import', { source: { ...source(), channel: st.id, tz: st.tz }, station: st.name, from, days, options: {} })
      onCloned(r)
    } catch (e: any) { setErr(e.message) } finally { setBusy('') }
  }
  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase()
    return (stations || []).filter((s) => !q || s.name.toLowerCase().includes(q) || s.id.toLowerCase().includes(q)).slice(0, 300)
  }, [stations, filter])
  const sources: { v: SourceKind; label: string; hint: string }[] = [
    { v: 'tvmaze', label: t('Public schedule'), hint: t('TVmaze: free, no key. Series on networks in many countries.') },
    { v: 'xmltv', label: t('Guide file or link'), hint: t('XMLTV, the open guide format: a link or a file you have.') },
    { v: 'guide', label: t('This server’s guide'), hint: t('From your tuners and IPTV playlists in WittyTuner.') },
  ]
  return (
    <Section id="ch-clone" title={tk('Clone a station')} desc={t('Pick a station and a day or a week. Every programme is matched to your own files, and anything missing is filled with the closest thing you have.')}>
      <div className="clone-steps">
        <fieldset className="clone-step">
          <legend>{t('1. Where the guide comes from')}</legend>
          <div className="tvs-levels clone-sources" role="radiogroup" aria-label={t('Where the guide comes from')}>
            {sources.map((s) => (
              <button key={s.v} type="button" role="radio" aria-checked={kind === s.v} className={'tvs-level' + (kind === s.v ? ' on' : '')} onClick={() => setKind(s.v)}>
                <b>{s.label}</b><span>{s.hint}</span>
              </button>
            ))}
          </div>
          {kind === 'tvmaze' && (
            <label className="clone-field">{t('Country')}
              <Select value={country} onChange={(e) => setCountry(e.target.value)} options={COUNTRIES.map((c) => ({ value: c, label: c }))} />
            </label>
          )}
          {kind === 'xmltv' && (
            <div className="clone-xmltv">
              <label className="clone-field grow">{t('Link to an XMLTV guide')}
                <Input type="url" value={url.startsWith('upload:') ? '' : url} placeholder="https://…/guide.xml.gz" onChange={(e) => { setUrl(e.target.value); setFile('') }} />
              </label>
              <span className="clone-or">{t('or')}</span>
              <label className="btn">{busy === 'upload' ? t('Uploading…') : file ? t('Change file') : t('Choose a file')}
                <input type="file" accept=".xml,.gz,.xmltv,application/xml,application/gzip" className="sr-only" onChange={(e) => void upload(e.target.files?.[0])} />
              </label>
              {file && <span className="muted">{file}</span>}
              <p className="tve-sub muted">{t('A link may say {key} where your guide key goes; add the key under Advanced options after cloning.')}</p>
            </div>
          )}
        </fieldset>
        <fieldset className="clone-step">
          <legend>{t('2. Which days')}</legend>
          <div className="clone-row">
            <label className="clone-field">{t('First day')}<Input type="date" value={from} onChange={(e) => setFrom(e.target.value)} /></label>
            <label className="clone-field">{t('How long')}
              <Select value={String(days)} onChange={(e) => setDays(Number(e.target.value))} options={[{ value: '1', label: t('One day, every day') }, { value: '7', label: t('A week, each day on its weekday') }]} />
            </label>
            <Button variant="primary" onClick={() => void find()} disabled={!!busy || (kind === 'xmltv' && !url)}>{busy === 'find' ? t('Reading the guide…') : t('Find stations')}</Button>
            {onCancel && <Button onClick={onCancel}>{t('Keep the station I have')}</Button>}
          </div>
        </fieldset>
        {stations && stations.length > 0 && (
          <fieldset className="clone-step">
            <legend>{t('3. The station')}</legend>
            <Input type="search" className="tve-find" placeholder={t('Find a station')} aria-label={t('Find a station')} value={filter} onChange={(e) => setFilter(e.target.value)} />
            <ul className="clone-stations">
              {shown.map((s) => (
                <li key={s.id}>
                  <button type="button" className="clone-station" disabled={!!busy} onClick={() => void pick(s)}>
                    <b>{s.name}</b>
                    <span>{busy === 'import:' + s.id ? t('Matching to your library…') : s.count ? t('{count} programmes', { count: s.count }) : s.id}</span>
                  </button>
                </li>
              ))}
            </ul>
            {credit && <p className="tve-sub muted clone-credit">{credit}</p>}
          </fieldset>
        )}
      </div>
      {err && <p className="tvs-error" role="alert">{err}</p>}
    </Section>
  )
}

// ---------------------------------------------------------------------------
// The matched schedule
// ---------------------------------------------------------------------------

function ClonePreview({ clone, set, restart }: { clone: Clone; set: (c: Clone) => void; restart: () => void }) {
  const [swap, setSwap] = useState<number | null>(null)
  const [advanced, setAdvanced] = useState(false)
  const [busy, setBusy] = useState(false)
  const [only, setOnly] = useState<'' | 'stand' | 'missing'>('')
  const counts = useMemo(() => {
    const c = { exact: 0, stand: 0, missing: 0 }
    for (const s of clone.slots) c[RULES[s.rule]?.tone || 'missing']++
    return c
  }, [clone.slots])
  const days = useMemo(() => {
    const out: { day: string; rows: { s: CloneSlot; i: number }[] }[] = []
    clone.slots.forEach((s, i) => {
      const day = s.wall.slice(0, 10)
      if (!out.length || out[out.length - 1].day !== day) out.push({ day, rows: [] })
      out[out.length - 1].rows.push({ s, i })
    })
    return out
  }, [clone.slots])

  const rematch = async (options: CloneOptions, again = false) => {
    setBusy(true)
    try {
      const body = { source: clone.source, station: clone.station, from: clone.from, days: clone.days, options, slots: again ? undefined : clone.slots }
      const r = await api.post<ImportAnswer>('/channels/clone/import', body)
      set({ ...r.clone, credit: r.clone.credit || clone.credit })
      toast(again ? t('Read the guide again') : t('Matched again'))
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const choose = (i: number, c: CloneCandidate | null) => {
    const slots = clone.slots.slice()
    const s = { ...slots[i] }
    if (c) Object.assign(s, { item: c.item, itemTitle: c.title, itemSub: c.sub, itemDur: c.dur, rule: 'manual' as CloneRule, confidence: 100, why: t('Chosen by you'), pinned: true })
    else Object.assign(s, { pinned: false, item: 0 })
    slots[i] = s
    set({ ...clone, slots })
    setSwap(null)
    if (!c) void rematch(clone.options).catch(() => {})
  }
  const day = (d: string) => new Date(d + 'T12:00:00').toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' })
  return (
    <Section id="ch-clone" title={tk('Cloned station')} actions={<>
      <Button size="sm" onClick={() => setAdvanced(true)}>{t('Advanced options')}</Button>
      <Button size="sm" onClick={() => void rematch(clone.options, true)} disabled={busy || clone.source.url?.startsWith('upload:')}>{t('Read the guide again')}</Button>
      <Button size="sm" variant="ghost" onClick={restart}>{t('Clone another station')}</Button>
    </>}>
      <p className="clone-summary">
        <b>{clone.station}</b>{' · '}
        {clone.days > 1 ? t('{days} days from {date}', { days: clone.days, date: day(clone.from) }) : day(clone.from)}
        {clone.options.refresh && <>{' · '}{t('Follows the station each week')}</>}
      </p>
      <div className="clone-tally" role="group" aria-label={t('Show')}>
        <button type="button" className={'clone-chip exact' + (only === '' ? ' on' : '')} aria-pressed={only === ''} onClick={() => setOnly('')}>{t('{count} exact', { count: counts.exact })}</button>
        <button type="button" className={'clone-chip stand' + (only === 'stand' ? ' on' : '')} aria-pressed={only === 'stand'} onClick={() => setOnly(only === 'stand' ? '' : 'stand')}>{t('{count} stand-ins', { count: counts.stand })}</button>
        <button type="button" className={'clone-chip missing' + (only === 'missing' ? ' on' : '')} aria-pressed={only === 'missing'} onClick={() => setOnly(only === 'missing' ? '' : 'missing')}>{t('{count} missing', { count: counts.missing })}</button>
        {!clone.options.adsOff && <span className="muted">{t('Commercials fill the time each programme leaves.')}</span>}
      </div>
      {days.map((d) => (
        <div key={d.day} className="clone-day">
          <h3>{day(d.day)}</h3>
          <ol className="clone-slots">
            {d.rows.filter(({ s }) => !only || RULES[s.rule]?.tone === only).map(({ s, i }) => {
              const r = RULES[s.rule] || RULES.none
              return (
                <li key={i} className={'clone-slot ' + r.tone}>
                  <time className="clone-time">{s.wall.slice(11)}<small>{t('{n} min', { n: mins(s.dur) })}</small></time>
                  <div className="clone-guide">
                    <b>{s.gap ? t('Nothing in the guide') : s.title}</b>
                    <span>{[epLabel(s), s.sub].filter(Boolean).join(' · ')}</span>
                  </div>
                  <div className="clone-plays">
                    {s.item ? <><b>{s.itemTitle}</b><span>{[s.itemSub, s.itemDur ? t('{n} min', { n: mins(s.itemDur) }) : ''].filter(Boolean).join(' · ')}</span></>
                      : <span className="muted">{t('Nothing in your library fits')}</span>}
                  </div>
                  <span className={'clone-chip ' + r.tone} title={s.why || t(r.hint)}>{t(r.label)}{s.rule !== 'none' && s.rule !== 'manual' ? ` ${s.confidence}%` : ''}</span>
                  <Button size="sm" onClick={() => setSwap(i)} aria-label={t('Swap what plays at {time}', { time: s.wall.slice(11) })}>{t('Swap')}</Button>
                </li>
              )
            })}
          </ol>
        </div>
      ))}
      {clone.credit && <p className="tve-sub muted clone-credit">{clone.credit}</p>}
      {swap != null && <SwapDialog slot={clone.slots[swap]} options={clone.options} onClose={() => setSwap(null)} onChoose={(c) => choose(swap, c)} />}
      {advanced && <AdvancedDialog options={clone.options} onClose={() => setAdvanced(false)} onApply={(o) => { setAdvanced(false); set({ ...clone, options: o }); void rematch(o) }} />}
    </Section>
  )
}

function SwapDialog({ slot, options, onClose, onChoose }: { slot: CloneSlot; options: CloneOptions; onClose: () => void; onChoose: (c: CloneCandidate | null) => void }) {
  const [list, setList] = useState<CloneCandidate[] | null>(null)
  useEffect(() => {
    api.post<{ candidates: CloneCandidate[] }>('/channels/clone/candidates', { slot, options }).then((r) => setList(r.candidates)).catch((e) => { toast(e.message); setList([]) })
  }, [slot, options])
  return (
    <Dialog size="wide" title={t('What plays at {time}', { time: slot.wall.slice(11) })} description={slot.gap ? t('The guide has nothing here.') : [slot.title, epLabel(slot), slot.sub].filter(Boolean).join(' · ')} onClose={onClose}
      actions={<>{slot.pinned && <Button onClick={() => onChoose(null)}>{t('Back to automatic')}</Button>}<Button onClick={onClose}>{t('Cancel')}</Button></>}>
      {!list ? <div className="center"><div className="spinner" /></div> : !list.length ? <p className="muted">{t('Nothing in your library fits.')}</p> : (
        <ul className="clone-cands">
          {list.map((c) => {
            const r = RULES[c.rule] || RULES.none
            return (
              <li key={c.item}>
                <button type="button" className={'clone-cand ' + r.tone + (c.item === slot.item ? ' on' : '')} onClick={() => onChoose(c)}>
                  <span className="clone-cand-text"><b>{c.title}</b><span>{[c.sub, t('{n} min', { n: mins(c.dur) }), c.why].filter(Boolean).join(' · ')}</span></span>
                  <span className={'clone-chip ' + r.tone}>{t(r.label)} {c.confidence}%</span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </Dialog>
  )
}

const SHIFTS = [-720, -600, -480, -360, -300, -240, -180, -120, -90, -60, -30, 0, 30, 60, 90, 120, 180, 240, 300, 360, 480, 600, 720]

function AdvancedDialog({ options, onClose, onApply }: { options: CloneOptions; onClose: () => void; onApply: (o: CloneOptions) => void }) {
  const [o, setO] = useState<CloneOptions>(options)
  const [key, setKey] = useState('')
  const up = (p: Partial<CloneOptions>) => setO({ ...o, ...p })
  const apply = async () => {
    if (key.trim()) {
      try { await api.put('/settings', { guideKey: key.trim() }); toast(t('Guide key saved')) } catch (e: any) { toast(e.message); return }
    }
    onApply(o)
  }
  const rule = (k: 'noNear' | 'noSeries' | 'noSimilar' | 'noFiller', label: string, hint: string) => (
    <div className="clone-opt"><span><b>{label}</b><small>{hint}</small></span><Switch label={label} checked={!o[k]} onChange={(v) => up({ [k]: !v })} /></div>
  )
  const hours = (m: number) => (m === 0 ? t('No shift') : t('{sign}{h} h {m} min', { sign: m < 0 ? '−' : '+', h: Math.floor(Math.abs(m) / 60), m: Math.abs(m) % 60 }))
  return (
    <Dialog size="wide" title={t('Advanced options')} onClose={onClose} actions={<><Button onClick={onClose}>{t('Cancel')}</Button><Button variant="primary" onClick={() => void apply()}>{t('Apply')}</Button></>}>
      <div className="clone-adv">
        <h3>{t('When something is missing, use')}</h3>
        {rule('noNear', t('The closest episode'), t('Same series, the next or previous episode you have'))}
        {rule('noSeries', t('Any episode of the series'), t('Same series, taking turns through it'))}
        {rule('noSimilar', t('A similar show'), t('Genre, network, era and rating from your metadata'))}
        {rule('noFiller', t('The closest length'), t('Anything that fits the time'))}
        <h3>{t('Commercials')}</h3>
        <div className="clone-opt"><span><b>{t('Commercial breaks')}</b><small>{t('From the channel’s commercials libraries, at natural breaks where your files mark them')}</small></span><Switch label={t('Commercial breaks')} checked={!o.adsOff} onChange={(v) => up({ adsOff: !v })} /></div>
        <label className="clone-field">{t('How many breaks')}
          <Select value={o.density || 'normal'} disabled={o.adsOff} onChange={(e) => up({ density: e.target.value as CloneOptions['density'] })}
            options={[{ value: 'low', label: t('Few: one each half hour') }, { value: 'normal', label: t('Normal: two each half hour') }, { value: 'high', label: t('Many: three each half hour') }]} />
        </label>
        <div className="clone-opt"><span><b>{t('Match the era')}</b><small>{t('Commercials from the programme’s own years first, where they say')}</small></span><Switch label={t('Match the era')} checked={!o.noEra} disabled={o.adsOff} onChange={(v) => up({ noEra: !v })} /></div>
        <h3>{t('Time')}</h3>
        <label className="clone-field">{t('Repeat')}
          <Select value={o.repeat || ''} onChange={(e) => up({ repeat: e.target.value as CloneOptions['repeat'] })}
            options={[{ value: '', label: t('Automatic') }, { value: 'daily', label: t('The first day, every day') }, { value: 'weekly', label: t('Each day on its weekday, every week') }, { value: 'once', label: t('Only on the dates cloned') }]} />
        </label>
        <div className="clone-opt"><span><b>{t('Same moment as the station')}</b><small>{t('Off: the station’s own clock times, here. On: the very moment it airs, in your time zone')}</small></span><Switch label={t('Same moment as the station')} checked={!!o.realTime} onChange={(v) => up({ realTime: v })} /></div>
        <label className="clone-field">{t('Shift everything by')}
          <Select value={String(o.shiftMin || 0)} onChange={(e) => up({ shiftMin: Number(e.target.value) })} options={SHIFTS.map((m) => ({ value: String(m), label: hours(m) }))} />
        </label>
        <div className="clone-opt"><span><b>{t('Follow the station')}</b><small>{t('Read the guide again each week, so the channel keeps up with its new schedule. Your picks stay where the same programme is in the same slot.')}</small></span><Switch label={t('Follow the station')} checked={!!o.refresh} onChange={(v) => up({ refresh: v })} /></div>
        <h3>{t('Guide key')}</h3>
        <label className="clone-field grow">{t('Your own key, for guide links that say {key}')}
          <Input type="password" autoComplete="off" value={key} placeholder={t('Leave empty to keep the saved one')} onChange={(e) => setKey(e.target.value)} />
        </label>
        <p className="tve-sub muted">{t('Kept on this server only, never in the channel. Nothing here comes with a key.')}</p>
      </div>
    </Dialog>
  )
}
