// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useId, useMemo, useRef, useState, type ReactNode } from 'react'
import { api, downloadFile, uploadFile } from '../api'
import { IconBack, IconClose, IconDownload, IconFolder, IconRight, IconPause, IconPlay, IconPlus, IconTrash } from '../icons'
import { ask, Field, IconButton, Input, Notice, Select, Tabs, Textarea, toast } from '../ui'
import { skipAt } from '../player/markers'
import Timeline, { type Pick } from './Timeline'
import {
  ACTIONS, FILTER_CATEGORIES, FRAME, SEG_TYPES, SEVERITIES, actOf, changed, check, clockText, defaultAction, draftMarkers, draftOf, manifestOf, markAt,
  moveRange, newSegment, problemWords, readTime, segLabel, type Draft, type Filter, type InfoAnswer, type Problem, type Scene, type Seg, type SegType,
} from './model'
import { sourceWords } from '../player/markers'
import './info.css'

// The .wittyinfo editor (docs/formats/wittyinfo.md): the same one on an item's page, a show's
// page, inside the player and in the wittyinfo program. base is the API path of the file
// (/wittyinfo/item/12, /wittyinfo/show/3?season=1, /wittyinfo/file/0).
//
// player: the video it is for, when there is one to use. In the player it is the player's
// own; in the wittyinfo program the editor plays the video itself. Without one (an item's
// page), Preview opens the player.

export type PlayerLink = {
  now: () => number
  seek: (t: number) => void
  playing: () => boolean
  play: () => void
  pause: () => void
  // fps: frames a second, for the frame step (24 when not known).
  fps?: number
}

type Tab = 'skips' | 'scenes' | 'filters' | 'details' | 'pictures' | 'file'

type Props = {
  base: string
  scope: 'item' | 'season' | 'show'
  player?: PlayerLink | null
  // onPreview: open the player at a moment (an item's page). Unsaved changes are saved first.
  onPreview?: (at: number) => void
  // onDraft: the draft's parts as markers, as they change, so the player can try them.
  onDraft?: (m: ReturnType<typeof draftMarkers> | null) => void
  onSaved?: () => void
  compact?: boolean
  // A tick from the player: redraws the playhead.
  now?: number
}

// under is a path below the file's base, keeping its ?season=N.
const under = (base: string, tail: string, query = '') => {
  const q = base.indexOf('?')
  const path = q < 0 ? base : base.slice(0, q)
  const params = [q < 0 ? '' : base.slice(q + 1), query].filter(Boolean).join('&')
  return path + tail + (params ? '?' + params : '')
}

const PICTURE_ROLES = [
  { role: 'poster', label: 'Poster' }, { role: 'backdrop', label: 'Backdrop' }, { role: 'logo', label: 'Logo' }, { role: 'thumb', label: 'Episode picture' },
]

// TimeInput: a time typed as m:ss, read when it's left or Enter is pressed. blank lets it be
// empty (an end "to the end").
function TimeInput({ value, onChange, label, blank, placeholder }: { value: number | null | undefined; onChange: (v: number | null) => void; label: string; blank?: boolean; placeholder?: string }) {
  const [text, setText] = useState(value == null ? '' : clockText(value))
  const [bad, setBad] = useState(false)
  useEffect(() => { setText(value == null ? '' : clockText(value)); setBad(false) }, [value])
  const commit = () => {
    const v = readTime(text)
    if (v == null) { if (blank) { onChange(null); setBad(false) } else setBad(true); return }
    if (Number.isNaN(v)) { setBad(true); return }
    setBad(false)
    if (v !== value) onChange(v)
  }
  return <Input inputSize="sm" className="wi-time" aria-label={label} aria-invalid={bad || undefined} inputMode="decimal" placeholder={placeholder || '0:00'} value={text}
    onChange={(e) => setText(e.target.value)} onBlur={commit} onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); commit() } }} />
}

export default function InfoEditor({ base, scope, player, onPreview, onDraft, onSaved, compact, now }: Props) {
  const uid = useId()
  const [ans, setAns] = useState<InfoAnswer | null>(null)
  const [saved, setSaved] = useState<Draft | null>(null)
  const [d, setD] = useState<Draft | null>(null)
  const [err, setErr] = useState('')
  const [problems, setProblems] = useState<Problem[]>([])
  const [busy, setBusy] = useState(false)
  const [tab, setTab] = useState<Tab>('skips')
  const [pick, setPick] = useState<Pick>(null)
  const [addType, setAddType] = useState<SegType>('intro')
  const importer = useRef<HTMLInputElement | null>(null)
  const [, tick] = useState(0)

  const took = useCallback((a: InfoAnswer, keepDraft = false) => {
    setAns(a)
    const fresh = draftOf(a.info, a.info ? null : a.markers)
    setSaved(draftOf(a.info))
    setD((cur) => (keepDraft && cur ? { ...cur, artwork: fresh.artwork } : fresh))
    setProblems(a.problems || [])
  }, [])
  const load = useCallback(() => api.get<InfoAnswer>(base).then((a) => { took(a); setErr('') }).catch((e) => setErr((e as Error).message)), [base, took])
  useEffect(() => { load() }, [load])

  const duration = ans?.durationSec || 0
  useEffect(() => { if (d && onDraft) onDraft(draftMarkers(d, duration)) }, [d, duration]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => onDraft?.(null), []) // eslint-disable-line react-hooks/exhaustive-deps
  // The playhead follows the video while this is open.
  useEffect(() => {
    if (!player || now != null) return
    const t = setInterval(() => tick((n) => n + 1), 250)
    return () => clearInterval(t)
  }, [player, now])

  const local = useMemo(() => (d ? check(d, scope) : []), [d, scope])
  if (err && !ans) return <div className="error" role="alert">{err}</div>
  if (!ans || !d || !saved) return <p className="muted wi-help">Reading the file…</p>

  const dirty = changed(d, saved)
  const at = player ? player.now() : 0
  const frame = 1 / (player?.fps || 1 / FRAME)
  const item = scope === 'item'
  const set = (patch: Partial<Draft>) => setD({ ...d, ...patch })
  const setSeg = (i: number, s: Seg) => set({ segments: d.segments.map((x, n) => (n === i ? s : x)) })
  const setScene = (i: number, s: Scene) => set({ scenes: d.scenes.map((x, n) => (n === i ? s : x)) })
  const setFilter = (i: number, f: Filter) => set({ filters: d.filters.map((x, n) => (n === i ? f : x)) })

  const save = async (): Promise<boolean> => {
    if (local.length) { setProblems(local); toast('Some times need fixing first', { tone: 'danger' }); return false }
    setBusy(true); setErr('')
    try {
      const a = await api.put<InfoAnswer>(base, { info: manifestOf(d) })
      took(a)
      toast(a.origin === 'file' ? 'Saved beside the video' : 'Saved on the server')
      onSaved?.()
      return true
    } catch (e) {
      const data = (e as { data?: { problems?: Problem[] } }).data
      if (data?.problems) setProblems(data.problems)
      setErr((e as Error).message)
      return false
    } finally { setBusy(false) }
  }
  const run = async (fn: () => Promise<InfoAnswer>, done: string) => {
    setBusy(true); setErr('')
    try { took(await fn(), true); toast(done); onSaved?.() } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }

  // ------------------------------------------------------------ marking at the video's time
  const sel = pick && pick.list === 'segments' ? d.segments[pick.index] : null
  const selScene = pick && pick.list === 'scenes' ? d.scenes[pick.index] : null
  const selFilter = pick && pick.list === 'filters' ? d.filters[pick.index] : null
  const mark = (edge: 'start' | 'end') => {
    if (!player) return
    const t = player.now()
    if (pick?.list === 'segments' && sel) setSeg(pick.index, markAt(sel, edge, t))
    else if (pick?.list === 'filters' && selFilter) setFilter(pick.index, markAt(selFilter, edge, t) as Filter)
    else if (pick?.list === 'scenes' && selScene && edge === 'start') setScene(pick.index, { ...selScene, start: Math.round(t * 1000) / 1000 })
    else {
      // Nothing picked: start a new part here.
      const s = newSegment(addType, t, duration)
      set({ segments: [...d.segments, edge === 'start' ? s : { ...s, start: Math.max(0, t - 30), end: t }] })
      setPick({ list: 'segments', index: d.segments.length })
    }
  }
  const nudge = (delta: number) => {
    if (pick?.list === 'segments' && sel) setSeg(pick.index, moveRange(sel, 'both', delta, duration))
    else if (player) player.seek(Math.max(0, player.now() + delta))
  }
  const preview = async () => {
    const target = sel || selFilter || selScene
    const from = Math.max(0, (target?.start ?? at) - 5)
    if (player) { player.seek(from); player.play(); return }
    if (onPreview) {
      if (dirty && !(await save())) return
      onPreview(from)
    }
  }

  const tabs: { value: Tab; label: string }[] = [
    { value: 'skips', label: `Skips${d.segments.length ? ` (${d.segments.length})` : ''}` },
    ...(item ? [{ value: 'scenes' as Tab, label: `Scenes${d.scenes.length ? ` (${d.scenes.length})` : ''}` }] : []),
    ...(item ? [{ value: 'filters' as Tab, label: 'Content' }] : []),
    ...(item && !compact ? [{ value: 'details' as Tab, label: 'Details' }, { value: 'pictures' as Tab, label: 'Pictures' }] : []),
    { value: 'file', label: 'File' },
  ]
  const allProblems = problems.length ? problems : local
  const where = ans.origin === 'file' ? 'Beside the video' : ans.origin === 'stored' ? 'Kept on the server' : 'Not saved yet'

  return (
    <div className={'wi-editor' + (compact ? ' compact' : '')}>
      <div className="wi-status" role="status">
        <span className={'wi-badge ' + (ans.origin || 'new')}>{where}</span>
        {ans.origin === 'stored' && ans.canWrite === false && <span className="muted">Writing .wittyinfo files beside the media is off. Export saves a copy.</span>}
        {ans.origin === 'stored' && ans.canWrite && <span className="muted">Saving now writes it beside the video.</span>}
        {ans.info?.authorship?.author && <span className="muted">By {ans.info.authorship.author}{ans.info.authorship.updated ? `, ${new Date(ans.info.authorship.updated).toLocaleString()}` : ''}</span>}
        {dirty && <span className="wi-badge dirty">Changed</span>}
      </div>
      {ans.match && !ans.match.usable && (
        <Notice tone="warn" title="Made for another cut">
          {ans.match.message} <button type="button" className="btn sm" disabled={busy} onClick={() => run(() => api.post<InfoAnswer>(under(base, '/use-anyway'), { use: true }), 'Used anyway')}>Use it anyway</button>
        </Notice>
      )}
      {ans.match?.result === 'forced' && <Notice tone="info">Made for another cut, and used anyway. <button type="button" className="btn sm ghost" onClick={() => run(() => api.post<InfoAnswer>(under(base, '/use-anyway'), { use: false }), 'Not used any more')}>Stop using it</button></Notice>}
      {ans.fileError && <Notice tone="danger" title="The file beside the video can’t be used">{ans.fileError}. Saving here replaces it; the old one is kept as .wittyinfo.bak.</Notice>}

      {(tab === 'skips' || tab === 'scenes' || tab === 'filters') && (
        <Timeline duration={duration} segments={d.segments} scenes={d.scenes} filters={tab === 'filters' || d.filters.length ? d.filters : []} pick={pick} onPick={setPick}
          onSegment={setSeg} onScene={setScene} onFilter={setFilter} now={player ? (now ?? player.now()) : undefined} onSeek={player ? player.seek : undefined}
          ghosts={ans.markers?.segments || undefined} />
      )}

      {player && (tab === 'skips' || tab === 'scenes' || tab === 'filters') && (
        <div className="wi-markbar" role="toolbar" aria-label="Mark at the video’s time">
          <span className="wi-clock" aria-live="off">{clockText(at)}</span>
          <IconButton size="sm" label={player.playing() ? 'Pause' : 'Play'} onClick={() => (player.playing() ? player.pause() : player.play())}>{player.playing() ? <IconPause /> : <IconPlay />}</IconButton>
          <button type="button" className="btn sm" onClick={() => nudge(-1)} title={sel ? 'Move the part a second earlier' : 'Back a second'}>−1 s</button>
          <button type="button" className="btn sm" onClick={() => { player.pause(); player.seek(Math.max(0, player.now() - frame)) }} title="Back a frame"><IconBack size={16} /> Frame</button>
          <button type="button" className="btn sm" onClick={() => { player.pause(); player.seek(player.now() + frame) }} title="On a frame">Frame <IconRight size={16} /></button>
          <button type="button" className="btn sm" onClick={() => nudge(1)} title={sel ? 'Move the part a second later' : 'On a second'}>+1 s</button>
          <span className="wi-sep" aria-hidden="true" />
          <button type="button" className="btn sm primary" onClick={() => mark('start')}>Mark start</button>
          <button type="button" className="btn sm primary" onClick={() => mark('end')}>Mark end</button>
          {!pick && <Select inputSize="sm" aria-label="Kind of part to mark" value={addType} onChange={(e) => setAddType(e.target.value as SegType)} options={SEG_TYPES.map((t) => ({ value: t.value, label: t.label }))} />}
          {pick && <button type="button" className="btn sm ghost" onClick={() => setPick(null)}>New part</button>}
        </div>
      )}

      <Tabs label="Parts of the file" value={tab} onChange={setTab} items={tabs} panelId={uid + 'panel'} />
      <div id={uid + 'panel'} role="tabpanel" className="wi-panel">
        {tab === 'skips' && (
          <>
            <p className="muted wi-help">{scope === 'item'
              ? 'Parts of this video to skip or know about. What is here wins over the file’s chapters and anything found by itself; the faint bands on the timeline are those.'
              : `Defaults for every episode of this ${scope}. An episode’s own .wittyinfo, a .skip.json or .edl beside it wins; these win over chapters and anything found by itself.`}</p>
            {d.segments.length === 0 && <p className="muted">No parts yet.</p>}
            <ul className="wi-list" aria-label="Parts">
              {d.segments.map((s, i) => {
                const hint = SEG_TYPES.find((t) => t.value === s.type)?.hint
                return (
                  <li key={s.id || i} className={'wi-item' + (pick?.list === 'segments' && pick.index === i ? ' wi-picked' : '')} onFocusCapture={() => setPick({ list: 'segments', index: i })}>
                    <span className={'wi-swatch wi-t-' + s.type} aria-hidden="true" />
                    <Select inputSize="sm" aria-label={`Part ${i + 1}: kind`} title={hint} value={s.type} onChange={(e) => { const type = e.target.value as SegType; setSeg(i, { ...s, type, action: s.action && s.action !== defaultAction(s.type) ? s.action : undefined }) }} options={SEG_TYPES.map((t) => ({ value: t.value, label: t.label }))} />
                    <TimeInput label={`Part ${i + 1}: start`} value={s.start} onChange={(v) => setSeg(i, { ...s, start: v ?? 0 })} />
                    <TimeInput label={`Part ${i + 1}: end`} value={s.end} blank placeholder="the end" onChange={(v) => setSeg(i, { ...s, end: v })} />
                    <Select inputSize="sm" aria-label={`Part ${i + 1}: what the player does`} value={actOf(s)} onChange={(e) => setSeg(i, { ...s, action: e.target.value as Seg['action'] })} options={ACTIONS.map((a) => ({ value: a.value, label: a.label }))} />
                    <Input inputSize="sm" className="wi-label" aria-label={`Part ${i + 1}: name`} placeholder={s.type === 'custom' ? 'Name it' : 'Name (optional)'} maxLength={200} value={s.label || ''} onChange={(e) => setSeg(i, { ...s, label: e.target.value })} />
                    <span className="wi-row-btns">
                      {(player || onPreview) && <button type="button" className="btn sm ghost" onClick={() => { setPick({ list: 'segments', index: i }); if (player) { player.seek(Math.max(0, s.start - 5)); player.play() } else onPreview?.(Math.max(0, s.start - 5)) }}>Preview</button>}
                      <IconButton size="sm" label={`Remove part ${i + 1}`} onClick={() => { set({ segments: d.segments.filter((_, n) => n !== i) }); setPick(null) }}><IconTrash size={16} /></IconButton>
                    </span>
                  </li>
                )
              })}
            </ul>
            <div className="wi-add">
              <Select inputSize="sm" aria-label="Kind of part to add" value={addType} onChange={(e) => setAddType(e.target.value as SegType)} options={SEG_TYPES.map((t) => ({ value: t.value, label: t.label }))} />
              <button type="button" className="btn sm" onClick={() => { set({ segments: [...d.segments, newSegment(addType, player ? player.now() : 0, duration)] }); setPick({ list: 'segments', index: d.segments.length }) }}><IconPlus size={16} /> Add {segLabel(addType).toLowerCase()}{player ? ' here' : ''}</button>
            </div>
            <fieldset className="wi-absent">
              <legend>This {scope === 'item' ? 'video' : scope} has no…</legend>
              <p className="muted wi-help">Ticked kinds aren’t taken from anywhere else: for an episode with a cold open and no intro, say.</p>
              {SEG_TYPES.filter((t) => t.value !== 'postcredits').map((t) => (
                <label key={t.value} className="wi-check"><input type="checkbox" checked={d.absent.includes(t.value)} onChange={(e) => set({ absent: e.target.checked ? [...d.absent, t.value] : d.absent.filter((x) => x !== t.value) })} /> {t.label.toLowerCase()}</label>
              ))}
            </fieldset>
            {!compact && ans.markers && (ans.markers.segments || []).length > 0 && (
              <details className="wi-inforce">
                <summary>What plays now, and where it comes from</summary>
                <ul>
                  {(ans.markers.segments || []).map((s, i) => (
                    <li key={i}><span className={'wi-swatch wi-t-' + s.kind} aria-hidden="true" /> {segLabel(s.kind)} {clockText(s.startSec)}–{clockText(s.endSec)} · <span className="muted">{sourceWords(s.source) || s.source}</span></li>
                  ))}
                </ul>
                {ans.defaults && Object.entries(ans.defaults).map(([k, v]) => v.origin && <p key={k} className="muted">The {k}’s .wittyinfo gives {v.segments} part{v.segments === 1 ? '' : 's'}.</p>)}
              </details>
            )}
          </>
        )}

        {tab === 'scenes' && (
          <>
            <p className="muted wi-help">Scenes take the place of the video’s own chapters in the player’s Scenes.{(ans.chapters || []).length ? ' The video has chapters of its own: copy them in to start from those.' : ''}</p>
            <ul className="wi-list" aria-label="Scenes">
              {d.scenes.map((s, i) => (
                <li key={s.id || i} className={'wi-item wi-scene-item' + (pick?.list === 'scenes' && pick.index === i ? ' wi-picked' : '')} onFocusCapture={() => setPick({ list: 'scenes', index: i })}>
                  <span className="wi-n muted">{i + 1}</span>
                  <Input inputSize="sm" className="wi-label" aria-label={`Scene ${i + 1}: title`} placeholder="Title" maxLength={200} value={s.title} onChange={(e) => setScene(i, { ...s, title: e.target.value })} />
                  <TimeInput label={`Scene ${i + 1}: start`} value={s.start} onChange={(v) => setScene(i, { ...s, start: v ?? 0 })} />
                  <TimeInput label={`Scene ${i + 1}: picture from`} value={s.thumbnailAt} blank placeholder="its start" onChange={(v) => setScene(i, { ...s, thumbnailAt: v })} />
                  <span className="wi-row-btns">
                    {player && <button type="button" className="btn sm ghost" onClick={() => setScene(i, { ...s, thumbnailAt: Math.round(player.now() * 10) / 10 })}>Picture here</button>}
                    <IconButton size="sm" label={`Remove scene ${i + 1}`} onClick={() => { set({ scenes: d.scenes.filter((_, n) => n !== i) }); setPick(null) }}><IconTrash size={16} /></IconButton>
                  </span>
                </li>
              ))}
            </ul>
            <div className="wi-add">
              <button type="button" className="btn sm" onClick={() => { set({ scenes: [...d.scenes, { title: `Scene ${d.scenes.length + 1}`, start: Math.round(at * 10) / 10 }] }); setPick({ list: 'scenes', index: d.scenes.length }) }}><IconPlus size={16} /> Add a scene{player ? ' here' : ''}</button>
              {(ans.chapters || []).length > 0 && <button type="button" className="btn sm ghost" onClick={() => set({ scenes: [...d.scenes, ...(ans.chapters || []).map((c, n) => ({ title: c.title || `Chapter ${n + 1}`, start: c.start }))] })}>Copy the video’s {ans.chapters!.length} chapters</button>}
            </div>
          </>
        )}

        {tab === 'filters' && (
          <>
            <p className="muted wi-help">Mark what some people may not want to see or hear. Each person chooses in their own settings (Playback → Content) whether to skip, mute or be warned, so one file serves the whole household.</p>
            <ul className="wi-list" aria-label="Content filters">
              {d.filters.map((f, i) => (
                <li key={f.id || i} className={'wi-item wi-filter-item' + (pick?.list === 'filters' && pick.index === i ? ' wi-picked' : '')} onFocusCapture={() => setPick({ list: 'filters', index: i })}>
                  <Select inputSize="sm" aria-label={`Filter ${i + 1}: what it is`} value={f.category} onChange={(e) => setFilter(i, { ...f, category: e.target.value as Filter['category'] })} options={FILTER_CATEGORIES} />
                  <Select inputSize="sm" aria-label={`Filter ${i + 1}: how strong`} value={String(f.severity || 0)} onChange={(e) => setFilter(i, { ...f, severity: Number(e.target.value) })} options={SEVERITIES.map((s) => ({ value: String(s.value), label: s.label }))} />
                  <TimeInput label={`Filter ${i + 1}: start`} value={f.start} onChange={(v) => setFilter(i, { ...f, start: v ?? 0 })} />
                  <TimeInput label={`Filter ${i + 1}: end`} value={f.end} onChange={(v) => setFilter(i, { ...f, end: v ?? f.start + 1 })} />
                  <Select inputSize="sm" aria-label={`Filter ${i + 1}: suggestion`} value={f.action || ''} onChange={(e) => setFilter(i, { ...f, action: (e.target.value || undefined) as Filter['action'] })} options={[{ value: '', label: 'No suggestion' }, { value: 'skip', label: 'Suggest skip' }, { value: 'mute', label: 'Suggest mute' }, { value: 'warn', label: 'Suggest a warning' }]} />
                  <span className="wi-row-btns"><IconButton size="sm" label={`Remove filter ${i + 1}`} onClick={() => { set({ filters: d.filters.filter((_, n) => n !== i) }); setPick(null) }}><IconTrash size={16} /></IconButton></span>
                </li>
              ))}
            </ul>
            <div className="wi-add">
              <button type="button" className="btn sm" onClick={() => { const s = Math.round(at * 10) / 10; set({ filters: [...d.filters, { category: 'language', start: s, end: Math.round((s + 2) * 10) / 10 }] }); setPick({ list: 'filters', index: d.filters.length }) }}><IconPlus size={16} /> Add a filter{player ? ' here' : ''}</button>
            </div>
          </>
        )}

        {tab === 'details' && (
          <div className="wi-form">
            <p className="muted wi-help">What is filled in here replaces what was found about this title, on the next scan and straight away when saved. Leave a box empty to keep what was found.</p>
            <Field label="Title"><Input value={d.metadata.title || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, title: e.target.value } })} /></Field>
            <Field label="Sort as"><Input value={d.metadata.sortTitle || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, sortTitle: e.target.value } })} /></Field>
            <Field label="Edition" hint="Director’s Cut, Extended…"><Input value={d.metadata.edition || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, edition: e.target.value } })} /></Field>
            <Field label="Year"><Input inputMode="numeric" value={d.metadata.year ?? ''} onChange={(e) => set({ metadata: { ...d.metadata, year: e.target.value ? Number(e.target.value) || undefined : undefined } })} /></Field>
            <Field label="Tagline"><Input value={d.metadata.tagline || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, tagline: e.target.value } })} /></Field>
            <Field label="Overview"><Textarea rows={4} value={d.metadata.overview || ''} maxLength={20000} onChange={(e) => set({ metadata: { ...d.metadata, overview: e.target.value } })} /></Field>
            <Field label="Genres" hint="Separated by commas."><Input value={(d.metadata.genres || []).join(', ')} onChange={(e) => set({ metadata: { ...d.metadata, genres: e.target.value.split(',').map((g) => g.trim()).filter(Boolean).slice(0, 50) } })} /></Field>
            <Field label="Content rating"><Input value={d.metadata.contentRating || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, contentRating: e.target.value } })} /></Field>
            <Field label="Kind of episode" hint="Mostly for anime: whether it is part of the story.">
              <Select value={d.metadata.episodeKind || ''} onChange={(e) => set({ metadata: { ...d.metadata, episodeKind: e.target.value || undefined } })} options={[{ value: '', label: 'Not said' }, { value: 'canon', label: 'Part of the story' }, { value: 'filler', label: 'Filler' }, { value: 'mixed', label: 'Partly filler' }, { value: 'recap', label: 'Recap episode' }]} />
            </Field>
            <h3 className="wi-h">Playing</h3>
            <Field label="Start playing at" hint="The first time: past a studio logo, say."><TimeInput label="Start playing at" value={d.playback.startAt} blank placeholder="the start" onChange={(v) => set({ playback: { ...d.playback, startAt: v ?? undefined } })} /></Field>
            <Field label="Sound language" hint="A language code such as jpn or eng."><Input maxLength={35} value={d.playback.audioLanguage || ''} onChange={(e) => set({ playback: { ...d.playback, audioLanguage: e.target.value } })} /></Field>
            <Field label="Subtitle language"><Input maxLength={35} value={d.playback.subtitleLanguage || ''} onChange={(e) => set({ playback: { ...d.playback, subtitleLanguage: e.target.value } })} /></Field>
            <Field label="Subtitles"><Select value={d.playback.subtitleMode || ''} onChange={(e) => set({ playback: { ...d.playback, subtitleMode: e.target.value || undefined } })} options={[{ value: '', label: 'As the viewer chooses' }, { value: 'off', label: 'Off' }, { value: 'forced', label: 'Only forced subtitles' }, { value: 'always', label: 'Always' }]} /></Field>
            <h3 className="wi-h">Notes</h3>
            <Field label="Notes" hint="Anything worth saying about this file."><Textarea rows={3} value={d.notes} maxLength={20000} onChange={(e) => set({ notes: e.target.value })} /></Field>
          </div>
        )}

        {tab === 'pictures' && (
          <div className="wi-pictures">
            <p className="muted wi-help">Pictures kept in the .wittyinfo itself, so they go wherever it goes. They take the place of what was found. JPEG, PNG or WebP, 16 MB at most.</p>
            {PICTURE_ROLES.map(({ role, label }) => {
              const name = d.artwork[role]
              const file = ans.files.find((f) => f.name === name)
              return (
                <div key={role} className="wi-picture">
                  <div className={'wi-pic-frame ' + role}>{file?.url ? <img src={file.url} alt={label} /> : <span className="muted">None</span>}</div>
                  <b>{label}</b>
                  <label className="btn sm">
                    {name ? 'Replace' : 'Add'}
                    <input type="file" accept="image/jpeg,image/png,image/webp" className="wi-file" disabled={busy}
                      onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) run(() => uploadFile<InfoAnswer>(under(base, `/images/${role}`), f, 'PUT'), `${label} saved`) }} />
                  </label>
                  {name && <button type="button" className="btn sm ghost" disabled={busy} onClick={() => run(() => api.del<InfoAnswer>(under(base, `/images/${role}`)), `${label} taken out`)}>Remove</button>}
                </div>
              )
            })}
          </div>
        )}

        {tab === 'file' && (
          <div className="wi-filetab">
            <dl className="wi-facts">
              <dt>Where</dt><dd>{where}{ans.path ? <> · <code>{ans.path}</code></> : null}</dd>
              {ans.info?.identity && <><dt>Made for</dt><dd>{String(ans.info.identity.fileName || 'a video')}, {clockText(Number(ans.info.identity.durationSec))} long{ans.match ? ` · ${ans.match.result === 'exact' ? 'this very file' : ans.match.result === 'same-cut' ? 'the same cut' : ans.match.result === 'other-cut' ? 'another cut' : ans.match.result === 'forced' ? 'another cut, used anyway' : 'not checked'}` : ''}</dd></>}
              {ans.info?.authorship?.tool && <><dt>Written with</dt><dd>{ans.info.authorship.tool.name} {ans.info.authorship.tool.version}</dd></>}
              <dt>Saving</dt><dd>{ans.canWrite ? 'Writes the .wittyinfo beside the video.' : 'Kept on the server: writing .wittyinfo files beside the media is off (Settings → Metadata), or there is no place for one.'}</dd>
            </dl>
            <div className="wi-add">
              <button type="button" className="btn sm" disabled={!ans.info} onClick={() => downloadFile(under(base, '/export'), 'info.wittyinfo').catch((e) => toast((e as Error).message, { tone: 'danger' }))}><IconDownload size={16} /> Export</button>
              <button type="button" className="btn sm" disabled={busy} onClick={() => importer.current?.click()}><IconFolder size={16} /> Import…</button>
              <input ref={importer} type="file" accept=".wittyinfo,.json,application/zip,application/json" className="wi-file" onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) run(() => uploadFile<InfoAnswer>(under(base, '/import'), f, 'POST'), 'Imported') }} />
              {ans.origin && <button type="button" className="btn sm ghost danger" disabled={busy} onClick={async () => {
                const beside = ans.fileFound && ans.canWrite && (ans.origin === 'file' || !!ans.path)
                const ok = await ask({ title: 'Remove the custom information?', body: beside ? 'The server’s copy is forgotten and the file beside the video is renamed to .wittyinfo.bak. The times found by themselves come back.' : 'The server’s copy is forgotten. A file beside the video, if there is one, is used again.', confirm: 'Remove', danger: true })
                if (ok) run(() => api.del<InfoAnswer>(under(base, '', beside ? 'file=1' : '')), 'Removed')
              }}><IconTrash size={16} /> Remove</button>}
            </div>
            <p className="muted wi-help">A .wittyinfo is a small archive any WittyMediaServer reads, and the wittyinfo program edits without a server. The format is in the documentation (formats/wittyinfo.md).</p>
          </div>
        )}
      </div>

      {(allProblems.length > 0 || err) && (
        <div className="error wi-problems" role="alert">
          {err && <p>{err}</p>}
          {allProblems.length > 0 && <ul>{allProblems.slice(0, 8).map((p, i) => <li key={i}>{problemWords(p, d)}</li>)}</ul>}
        </div>
      )}
      <div className="wi-foot">
        {(player || onPreview) && (sel || selScene || selFilter) && <button type="button" className="btn" onClick={preview}><IconPlay size={16} /> Preview from just before</button>}
        <span className="grow" />
        {dirty && <button type="button" className="btn ghost" disabled={busy} onClick={() => { setD(saved); setProblems([]) }}><IconClose size={16} /> Undo changes</button>}
        <button type="button" className="btn primary" disabled={busy || !dirty} onClick={save}>Save</button>
      </div>
    </div>
  )
}

// LivePreview plays the video in the wittyinfo program, with the skip button the player
// would show for the parts as they are now, unsaved or not.
export function LivePreview({ src, markers, duration, onLink }: { src: string; markers: ReturnType<typeof draftMarkers> | null; duration: number; onLink: (l: PlayerLink, now: number) => void }): ReactNode {
  const v = useRef<HTMLVideoElement | null>(null)
  const [now, setNow] = useState(0)
  const skip = skipAt(markers, now, duration)
  useEffect(() => {
    const el = v.current
    if (!el) return
    onLink({ now: () => el.currentTime, seek: (t) => { el.currentTime = t }, playing: () => !el.paused, play: () => { void el.play().catch(() => {}) }, pause: () => el.pause() }, now)
  }, [now]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="wi-preview">
      <video ref={v} src={src} controls preload="metadata" onTimeUpdate={(e) => setNow(e.currentTarget.currentTime)} onSeeked={(e) => setNow(e.currentTarget.currentTime)} />
      {skip && <button type="button" className="btn skip-intro wi-skip" onClick={() => { if (v.current) v.current.currentTime = skip.to }}>{skip.label} ›</button>}
    </div>
  )
}
