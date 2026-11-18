// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useId, useMemo, useRef, useState, type ReactNode } from 'react'
import { api, downloadFile, mediaUrl, uploadFile } from '../api'
import { IconBack, IconClose, IconDownload, IconFolder, IconPause, IconPlay, IconPlus, IconRight, IconTrash } from '../icons'
import { ask, Field, IconButton, Input, Notice, Select, Tabs, Textarea, toast } from '../ui'
import { t, tn, tx, useLanguage } from '../i18n'
import { skipAt, sourceWords } from '../player/markers'
import Timeline, { type Pick } from './Timeline'
import {
  ACTIONS, COMMON_TAGS, FILTER_CATEGORIES, FRAME, LANGUAGE_CODES, SEG_TYPES, SEVERITIES, actOf, canonicalLanguage, changed, check, clockText, defaultAction,
  draftMarkers, draftOf, ensureIds, languageLabel, manifestOf, markAt, moveRange, newId, newSegment, problemWords, readTime, segLabel, setText, validLanguage,
  type Draft, type Filter, type InfoAnswer, type Lyrics, type Problem, type Scene, type Seg, type SegType,
} from './model'
import './info.css'

// The .wittyinfo and .wittyaudio editor (docs/formats/wittyinfo.md, wittyaudio.md): the same
// one on a title's page, a show's, an album's, inside the player and in the wittyinfo program.
// base is the API path of the file (/wittyinfo/item/12, /wittyinfo/show/3?season=1,
// /wittyinfo/album/40, /wittyinfo/file/0). Desktop only: TV mode never opens it
// (desktopOnly.ts).
//
// player: the video or song it is for, when there is one to use. In the player it is the
// player's own; for a song, and in the wittyinfo program, the editor plays it itself. Without
// one (a title's page), Preview opens the player.

export type PlayerLink = {
  now: () => number
  seek: (t: number) => void
  playing: () => boolean
  play: () => void
  pause: () => void
  // fps: frames a second, for the frame step (24 when not known).
  fps?: number
}

export type EditorScope = 'item' | 'season' | 'show' | 'song' | 'album' | 'artist'
type Tab = 'skips' | 'scenes' | 'filters' | 'lyrics' | 'music' | 'details' | 'pictures' | 'subtitles' | 'languages' | 'file'

type Props = {
  base: string
  scope: EditorScope
  player?: PlayerLink | null
  onPreview?: (at: number) => void
  onDraft?: (m: ReturnType<typeof draftMarkers> | null) => void
  onSaved?: () => void
  compact?: boolean
  now?: number
  // songSrc: the song to play beside the editor (a song's own editor).
  songSrc?: string
}

// under is a path below the file's base, keeping its ?season=N.
const under = (base: string, tail: string, query = '') => {
  const q = base.indexOf('?')
  const path = q < 0 ? base : base.slice(0, q)
  const params = [q < 0 ? '' : base.slice(q + 1), query].filter(Boolean).join('&')
  return path + tail + (params ? '?' + params : '')
}

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

// LanguagePicker: every language, named in the app's language, and any tag typed by hand.
export function LanguagePicker({ value, onChange, label, allowNone, exclude }: { value: string; onChange: (v: string) => void; label: string; allowNone?: boolean; exclude?: string[] }) {
  const ui = useLanguage()
  const [custom, setCustom] = useState('')
  const options = useMemo(() => {
    const all = [...LANGUAGE_CODES, ...COMMON_TAGS].filter((c) => !exclude?.some((x) => canonicalLanguage(x) === canonicalLanguage(c)))
    return all.map((c) => ({ value: c, label: languageLabel(c, ui) })).sort((a, b) => a.label.localeCompare(b.label, ui))
  }, [ui, exclude])
  const known = !value || options.some((o) => o.value === value)
  return (
    <span className="wi-langpick">
      <Select inputSize="sm" aria-label={label} value={known ? value : '__other'} onChange={(e) => { if (e.target.value !== '__other') onChange(e.target.value) }}
        options={[...(allowNone ? [{ value: '', label: t('Not said') }] : []), ...options, { value: '__other', label: known ? t('Another, by its code…') : languageLabel(value, ui) }]} />
      {!known || custom ? null : null}
      <Input inputSize="sm" className="wi-langcode" aria-label={t('{label}: a code typed by hand', { label })} placeholder={t('or a code: de-AT, yue')} value={custom}
        onChange={(e) => setCustom(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter' && validLanguage(custom)) { e.preventDefault(); onChange(canonicalLanguage(custom)); setCustom('') } }}
        onBlur={() => { if (validLanguage(custom)) { onChange(canonicalLanguage(custom)); setCustom('') } }} />
    </span>
  )
}

export default function InfoEditor({ base, scope, player: givenPlayer, onPreview, onDraft, onSaved, compact, now, songSrc }: Props) {
  const uid = useId()
  const ui = useLanguage()
  const [ans, setAns] = useState<InfoAnswer | null>(null)
  const [saved, setSaved] = useState<Draft | null>(null)
  const [d, setD] = useState<Draft | null>(null)
  const [err, setErr] = useState('')
  const [problems, setProblems] = useState<Problem[]>([])
  const [busy, setBusy] = useState(false)
  const audio = scope === 'song' || scope === 'album' || scope === 'artist'
  const [tab, setTab] = useState<Tab>(scope === 'song' ? 'lyrics' : audio ? 'music' : 'skips')
  const [pick, setPick] = useState<Pick>(null)
  const [addType, setAddType] = useState<SegType>(audio ? 'custom' : 'intro')
  const [lang, setLang] = useState('')
  const [subLang, setSubLang] = useState('')
  // The next lyric line "Line starts now" times.
  const [cursor, setCursor] = useState(0)
  const importer = useRef<HTMLInputElement | null>(null)
  const songEl = useRef<HTMLAudioElement | null>(null)
  const [, tick] = useState(0)

  // A song's own player, when nothing else plays it.
  const songPlayer: PlayerLink | null = songSrc ? {
    now: () => songEl.current?.currentTime || 0, seek: (v) => { if (songEl.current) songEl.current.currentTime = v },
    playing: () => !!songEl.current && !songEl.current.paused, play: () => { void songEl.current?.play().catch(() => {}) }, pause: () => songEl.current?.pause(), fps: 100,
  } : null
  const player = givenPlayer || songPlayer

  const took = useCallback((a: InfoAnswer, keepDraft = false) => {
    setAns(a)
    const fresh = draftOf(a.info, a.info ? null : a.markers)
    setSaved(draftOf(a.info))
    setD((cur) => (keepDraft && cur ? { ...cur, artwork: fresh.artwork, subtitles: fresh.subtitles } : fresh))
    setProblems(a.problems || [])
  }, [])
  const load = useCallback(() => api.get<InfoAnswer>(base).then((a) => { took(a); setErr('') }).catch((e) => setErr((e as Error).message)), [base, took])
  useEffect(() => { load() }, [load])

  const duration = ans?.durationSec || 0
  useEffect(() => { if (d && onDraft) onDraft(draftMarkers(d, duration)) }, [d, duration]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => onDraft?.(null), []) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!player || now != null) return
    const tm = setInterval(() => tick((n) => n + 1), 250)
    return () => clearInterval(tm)
  }, [!!player, now]) // eslint-disable-line react-hooks/exhaustive-deps

  const local = useMemo(() => (d ? check(d, scope === 'song' ? 'item' : scope === 'album' || scope === 'artist' ? 'show' : scope) : []), [d, scope])
  if (err && !ans) return <div className="error" role="alert">{err}</div>
  if (!ans || !d || !saved) return <p className="muted wi-help">{t('Reading the file…')}</p>

  const dirty = changed(d, saved)
  const at = player ? player.now() : 0
  const frame = 1 / (player?.fps || 1 / FRAME)
  const single = scope === 'item' || scope === 'song'
  const set = (patch: Partial<Draft>) => setD({ ...d, ...patch })
  const setSeg = (i: number, s: Seg) => set({ segments: d.segments.map((x, n) => (n === i ? s : x)) })
  const setScene = (i: number, s: Scene) => set({ scenes: d.scenes.map((x, n) => (n === i ? s : x)) })
  const setFilter = (i: number, f: Filter) => set({ filters: d.filters.map((x, n) => (n === i ? f : x)) })
  const music = (d.rest.audio || {}) as Record<string, unknown> & { lyrics?: Lyrics }
  const setMusic = (patch: Record<string, unknown>) => setD({ ...d, rest: { ...d.rest, audio: { ...music, ...patch } } })
  const lyricLines = music.lyrics?.lines || []
  const setLyrics = (l: Lyrics) => setMusic({ lyrics: l })

  const save = async (): Promise<boolean> => {
    if (local.length) { setProblems(local); toast(t('Some times need fixing first'), { tone: 'danger' }); return false }
    setBusy(true); setErr('')
    try {
      const a = await api.put<InfoAnswer>(base, { info: manifestOf(d) })
      took(a)
      toast(a.origin === 'file' ? t('Saved beside the file') : t('Saved on the server'))
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

  // ------------------------------------------------------------ marking at the moment playing
  const sel = pick && pick.list === 'segments' ? d.segments[pick.index] : null
  const selScene = pick && pick.list === 'scenes' ? d.scenes[pick.index] : null
  const selFilter = pick && pick.list === 'filters' ? d.filters[pick.index] : null
  const mark = (edge: 'start' | 'end') => {
    if (!player) return
    const tm = player.now()
    if (pick?.list === 'segments' && sel) setSeg(pick.index, markAt(sel, edge, tm))
    else if (pick?.list === 'filters' && selFilter) setFilter(pick.index, markAt(selFilter, edge, tm) as Filter)
    else if (pick?.list === 'scenes' && selScene && edge === 'start') setScene(pick.index, { ...selScene, start: Math.round(tm * 1000) / 1000 })
    else {
      const s = { ...newSegment(addType, tm, duration), id: newId('seg') }
      set({ segments: [...d.segments, edge === 'start' ? s : { ...s, start: Math.max(0, tm - 30), end: tm }] })
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

  const translationLangs = Object.keys(d.translations)
  const tabs: { value: Tab; label: string }[] = [
    ...(scope === 'song' ? [{ value: 'lyrics' as Tab, label: t('Lyrics') }] : []),
    ...(audio ? [{ value: 'music' as Tab, label: t('Music') }] : []),
    { value: 'skips', label: d.segments.length ? t('Skips ({count})', { count: d.segments.length }) : t('Skips') },
    ...(single ? [{ value: 'scenes' as Tab, label: audio ? (d.scenes.length ? t('Chapters ({count})', { count: d.scenes.length }) : t('Chapters')) : (d.scenes.length ? t('Scenes ({count})', { count: d.scenes.length }) : t('Scenes')) }] : []),
    ...(scope === 'item' ? [{ value: 'filters' as Tab, label: t('Content') }] : []),
    ...(!compact ? [{ value: 'details' as Tab, label: t('Details') }, { value: 'pictures' as Tab, label: t('Pictures') }] : []),
    ...(scope === 'item' && !compact ? [{ value: 'subtitles' as Tab, label: d.subtitles.length ? t('Subtitles ({count})', { count: d.subtitles.length }) : t('Subtitles') }] : []),
    { value: 'languages', label: translationLangs.length ? t('Languages ({count})', { count: translationLangs.length + 1 }) : t('Languages') },
    { value: 'file', label: t('File') },
  ]
  const allProblems = problems.length ? problems : local
  const where = ans.origin === 'file' ? t('Beside the file') : ans.origin === 'stored' ? t('Kept on the server') : t('Not saved yet')
  const showTimeline = tab === 'skips' || tab === 'scenes' || tab === 'filters' || tab === 'lyrics'
  const activeLang = lang && d.translations[lang] ? lang : translationLangs[0] || ''
  const tr = activeLang ? d.translations[activeLang] || {} : {}
  const langName = (c: string) => languageLabel(c, ui)

  return (
    <div className={'wi-editor' + (compact ? ' compact' : '')}>
      {songSrc && <audio ref={songEl} className="wi-song" src={mediaUrl(songSrc)} controls preload="metadata" />}
      <div className="wi-status" role="status">
        <span className={'wi-badge ' + (ans.origin || 'new')}>{where}</span>
        {ans.origin === 'stored' && ans.canWrite === false && <span className="muted">{t('Writing these files beside the media is off. Export saves a copy.')}</span>}
        {ans.origin === 'stored' && ans.canWrite && <span className="muted">{t('Saving now writes it beside the file.')}</span>}
        {ans.info?.authorship?.author && <span className="muted">{t('By {author}', { author: ans.info.authorship.author })}{ans.info.authorship.updated ? `, ${new Date(ans.info.authorship.updated).toLocaleString(ui)}` : ''}</span>}
        {d.language && <span className="muted">{t('Written in {language}', { language: langName(d.language) })}</span>}
        {dirty && <span className="wi-badge dirty">{t('Changed')}</span>}
      </div>
      {ans.match && !ans.match.usable && (
        <Notice tone="warn" title={t('Made for another cut')}>
          {tx(ans.match.message)} <button type="button" className="btn sm" disabled={busy} onClick={() => run(() => api.post<InfoAnswer>(under(base, '/use-anyway'), { use: true }), t('Used anyway'))}>{t('Use it anyway')}</button>
        </Notice>
      )}
      {ans.match?.result === 'forced' && <Notice tone="info">{t('Made for another cut, and used anyway.')} <button type="button" className="btn sm ghost" onClick={() => run(() => api.post<InfoAnswer>(under(base, '/use-anyway'), { use: false }), t('Not used any more'))}>{t('Stop using it')}</button></Notice>}
      {ans.fileError && <Notice tone="danger" title={t('The file beside it can’t be used')}>{tx(ans.fileError)}. {t('Saving here replaces it; the old one is kept with .bak on the end.')}</Notice>}

      {showTimeline && tab !== 'lyrics' && (
        <Timeline duration={duration} segments={d.segments} scenes={d.scenes} filters={tab === 'filters' || d.filters.length ? d.filters : []} pick={pick} onPick={setPick}
          onSegment={setSeg} onScene={setScene} onFilter={setFilter} now={player ? (now ?? player.now()) : undefined} onSeek={player ? player.seek : undefined}
          ghosts={ans.markers?.segments || undefined} />
      )}

      {player && showTimeline && (
        <div className="wi-markbar" role="toolbar" aria-label={t('Mark at the time playing')}>
          <span className="wi-clock" aria-live="off">{clockText(at)}</span>
          <IconButton size="sm" label={player.playing() ? t('Pause') : t('Play')} onClick={() => (player.playing() ? player.pause() : player.play())}>{player.playing() ? <IconPause /> : <IconPlay />}</IconButton>
          <button type="button" className="btn sm" onClick={() => nudge(-1)} title={sel ? t('Move the part a second earlier') : t('Back a second')}>{t('−1 s')}</button>
          <button type="button" className="btn sm" onClick={() => { player.pause(); player.seek(Math.max(0, player.now() - frame)) }} title={t('Back a frame')}><IconBack size={16} /> {t('Frame')}</button>
          <button type="button" className="btn sm" onClick={() => { player.pause(); player.seek(player.now() + frame) }} title={t('On a frame')}>{t('Frame')} <IconRight size={16} /></button>
          <button type="button" className="btn sm" onClick={() => nudge(1)} title={sel ? t('Move the part a second later') : t('On a second')}>{t('+1 s')}</button>
          <span className="wi-sep" aria-hidden="true" />
          {tab === 'lyrics' ? (
            <button type="button" className="btn sm primary" onClick={() => {
              // The next line to time starts now; past the last, a new line does.
              const tm = Math.round(player.now() * 100) / 100
              const lines = [...lyricLines]
              const i = Math.min(cursor, lines.length)
              const start = Math.max(tm, i > 0 ? lines[i - 1].start : 0)
              if (i < lines.length) lines[i] = { ...lines[i], start }
              else lines.push({ start, text: '' })
              setLyrics({ ...(music.lyrics || {}), lines })
              setCursor(i + 1)
            }}>{t('Line starts now')}</button>
          ) : (
            <>
              <button type="button" className="btn sm primary" onClick={() => mark('start')}>{t('Mark start')}</button>
              <button type="button" className="btn sm primary" onClick={() => mark('end')}>{t('Mark end')}</button>
              {!pick && <Select inputSize="sm" aria-label={t('Kind of part to mark')} value={addType} onChange={(e) => setAddType(e.target.value as SegType)} options={SEG_TYPES.map((x) => ({ value: x.value, label: tx(x.label) }))} />}
              {pick && <button type="button" className="btn sm ghost" onClick={() => setPick(null)}>{t('New part')}</button>}
            </>
          )}
        </div>
      )}

      <Tabs label={t('Parts of the file')} value={tab} onChange={setTab} items={tabs} panelId={uid + 'panel'} />
      <div id={uid + 'panel'} role="tabpanel" className="wi-panel">
        {tab === 'skips' && (
          <>
            <p className="muted wi-help">{single
              ? t('Parts to skip or know about. What is here wins over the file’s chapters and anything found by itself; the faint bands on the timeline are those.')
              : t('Defaults for everything in this {what}. A file of its own beside one of them wins; these win over chapters and anything found by itself.', { what: tx(scope) })}</p>
            {d.segments.length === 0 && <p className="muted">{t('No parts yet.')}</p>}
            <ul className="wi-list" aria-label={t('Parts')}>
              {d.segments.map((s, i) => {
                const hint = SEG_TYPES.find((x) => x.value === s.type)?.hint
                const n = i + 1
                return (
                  <li key={s.id || i} className={'wi-item' + (pick?.list === 'segments' && pick.index === i ? ' wi-picked' : '')} onFocusCapture={() => setPick({ list: 'segments', index: i })}>
                    <span className={'wi-swatch wi-t-' + s.type} aria-hidden="true" />
                    <Select inputSize="sm" aria-label={t('Part {n}: kind', { n })} title={hint ? tx(hint) : undefined} value={s.type} onChange={(e) => { const type = e.target.value as SegType; setSeg(i, { ...s, type, action: s.action && s.action !== defaultAction(s.type) ? s.action : undefined }) }} options={SEG_TYPES.map((x) => ({ value: x.value, label: tx(x.label) }))} />
                    <TimeInput label={t('Part {n}: start', { n })} value={s.start} onChange={(v) => setSeg(i, { ...s, start: v ?? 0 })} />
                    <TimeInput label={t('Part {n}: end', { n })} value={s.end} blank placeholder={t('the end')} onChange={(v) => setSeg(i, { ...s, end: v })} />
                    <Select inputSize="sm" aria-label={t('Part {n}: what the player does', { n })} value={actOf(s)} onChange={(e) => setSeg(i, { ...s, action: e.target.value as Seg['action'] })} options={ACTIONS.map((a) => ({ value: a.value, label: tx(a.label) }))} />
                    <Input inputSize="sm" className="wi-label" aria-label={t('Part {n}: name', { n })} placeholder={s.type === 'custom' ? t('Name it') : t('Name (optional)')} maxLength={200} value={s.label || ''} onChange={(e) => setSeg(i, { ...s, label: e.target.value })} />
                    <span className="wi-row-btns">
                      {(player || onPreview) && <button type="button" className="btn sm ghost" onClick={() => { setPick({ list: 'segments', index: i }); if (player) { player.seek(Math.max(0, s.start - 5)); player.play() } else onPreview?.(Math.max(0, s.start - 5)) }}>{t('Preview')}</button>}
                      <IconButton size="sm" label={t('Remove part {n}', { n })} onClick={() => { set({ segments: d.segments.filter((_, k) => k !== i) }); setPick(null) }}><IconTrash size={16} /></IconButton>
                    </span>
                  </li>
                )
              })}
            </ul>
            <div className="wi-add">
              <Select inputSize="sm" aria-label={t('Kind of part to add')} value={addType} onChange={(e) => setAddType(e.target.value as SegType)} options={SEG_TYPES.map((x) => ({ value: x.value, label: tx(x.label) }))} />
              <button type="button" className="btn sm" onClick={() => { set({ segments: [...d.segments, { ...newSegment(addType, player ? player.now() : 0, duration), id: newId('seg') }] }); setPick({ list: 'segments', index: d.segments.length }) }}><IconPlus size={16} /> {player ? t('Add here') : t('Add')}</button>
            </div>
            <fieldset className="wi-absent">
              <legend>{t('This has none of these:')}</legend>
              <p className="muted wi-help">{t('Ticked kinds aren’t taken from anywhere else: for an episode with a cold open and no intro, say.')}</p>
              {SEG_TYPES.filter((x) => x.value !== 'postcredits').map((x) => (
                <label key={x.value} className="wi-check"><input type="checkbox" checked={d.absent.includes(x.value)} onChange={(e) => set({ absent: e.target.checked ? [...d.absent, x.value] : d.absent.filter((y) => y !== x.value) })} /> {tx(x.label)}</label>
              ))}
            </fieldset>
            {!compact && ans.markers && (ans.markers.segments || []).length > 0 && (
              <details className="wi-inforce">
                <summary>{t('What plays now, and where it comes from')}</summary>
                <ul>
                  {(ans.markers.segments || []).map((s, i) => (
                    <li key={i}><span className={'wi-swatch wi-t-' + s.kind} aria-hidden="true" /> {tx(segLabel(s.kind))} {clockText(s.startSec)}–{clockText(s.endSec)} · <span className="muted">{tx(sourceWords(s.source)) || s.source}</span></li>
                  ))}
                </ul>
                {ans.defaults && Object.entries(ans.defaults).map(([k, v]) => v.origin && <p key={k} className="muted">{tn('The {what}’s file gives {count} part.', 'The {what}’s file gives {count} parts.', v.segments, { what: tx(k) })}</p>)}
              </details>
            )}
          </>
        )}

        {tab === 'scenes' && (
          <>
            <p className="muted wi-help">{audio ? t('Chapters of an audiobook or a long recording, shown where its parts are listed.') : t('Scenes take the place of the video’s own chapters in the player’s Scenes.')}{(ans.chapters || []).length ? ' ' + t('It has chapters of its own: copy them in to start from those.') : ''}</p>
            <ul className="wi-list" aria-label={audio ? t('Chapters') : t('Scenes')}>
              {d.scenes.map((s, i) => {
                const n = i + 1
                return (
                  <li key={s.id || i} className={'wi-item wi-scene-item' + (pick?.list === 'scenes' && pick.index === i ? ' wi-picked' : '')} onFocusCapture={() => setPick({ list: 'scenes', index: i })}>
                    <span className="wi-n muted">{n}</span>
                    <Input inputSize="sm" className="wi-label" aria-label={t('Scene {n}: title', { n })} placeholder={t('Title')} maxLength={200} value={s.title} onChange={(e) => setScene(i, { ...s, title: e.target.value })} />
                    <TimeInput label={t('Scene {n}: start', { n })} value={s.start} onChange={(v) => setScene(i, { ...s, start: v ?? 0 })} />
                    {!audio && <TimeInput label={t('Scene {n}: picture from', { n })} value={s.thumbnailAt} blank placeholder={t('its start')} onChange={(v) => setScene(i, { ...s, thumbnailAt: v })} />}
                    <span className="wi-row-btns">
                      {player && !audio && <button type="button" className="btn sm ghost" onClick={() => setScene(i, { ...s, thumbnailAt: Math.round(player.now() * 10) / 10 })}>{t('Picture here')}</button>}
                      <IconButton size="sm" label={t('Remove scene {n}', { n })} onClick={() => { set({ scenes: d.scenes.filter((_, k) => k !== i) }); setPick(null) }}><IconTrash size={16} /></IconButton>
                    </span>
                  </li>
                )
              })}
            </ul>
            <div className="wi-add">
              <button type="button" className="btn sm" onClick={() => { set({ scenes: [...d.scenes, { id: newId('scene'), title: t('Scene {n}', { n: d.scenes.length + 1 }), start: Math.round(at * 10) / 10 }] }); setPick({ list: 'scenes', index: d.scenes.length }) }}><IconPlus size={16} /> {player ? t('Add one here') : t('Add one')}</button>
              {(ans.chapters || []).length > 0 && <button type="button" className="btn sm ghost" onClick={() => set({ scenes: [...d.scenes, ...(ans.chapters || []).map((c, k) => ({ id: newId('scene'), title: c.title || t('Chapter {n}', { n: k + 1 }), start: c.start }))] })}>{tn('Copy its {count} chapter', 'Copy its {count} chapters', ans.chapters!.length)}</button>}
            </div>
          </>
        )}

        {tab === 'filters' && (
          <>
            <p className="muted wi-help">{t('Mark what some people may not want to see or hear. Each person chooses in their own settings (Playback, Content) whether to skip, mute or be warned, so one file serves the whole household.')}</p>
            <ul className="wi-list" aria-label={t('Content filters')}>
              {d.filters.map((f, i) => {
                const n = i + 1
                return (
                  <li key={f.id || i} className={'wi-item wi-filter-item' + (pick?.list === 'filters' && pick.index === i ? ' wi-picked' : '')} onFocusCapture={() => setPick({ list: 'filters', index: i })}>
                    <Select inputSize="sm" aria-label={t('Filter {n}: what it is', { n })} value={f.category} onChange={(e) => setFilter(i, { ...f, category: e.target.value as Filter['category'] })} options={FILTER_CATEGORIES.map((c) => ({ value: c.value, label: tx(c.label) }))} />
                    <Select inputSize="sm" aria-label={t('Filter {n}: how strong', { n })} value={String(f.severity || 0)} onChange={(e) => setFilter(i, { ...f, severity: Number(e.target.value) })} options={SEVERITIES.map((s) => ({ value: String(s.value), label: tx(s.label) }))} />
                    <TimeInput label={t('Filter {n}: start', { n })} value={f.start} onChange={(v) => setFilter(i, { ...f, start: v ?? 0 })} />
                    <TimeInput label={t('Filter {n}: end', { n })} value={f.end} onChange={(v) => setFilter(i, { ...f, end: v ?? f.start + 1 })} />
                    <Select inputSize="sm" aria-label={t('Filter {n}: suggestion', { n })} value={f.action || ''} onChange={(e) => setFilter(i, { ...f, action: (e.target.value || undefined) as Filter['action'] })} options={[{ value: '', label: t('No suggestion') }, { value: 'skip', label: t('Suggest skipping') }, { value: 'mute', label: t('Suggest muting') }, { value: 'warn', label: t('Suggest a warning') }]} />
                    <span className="wi-row-btns"><IconButton size="sm" label={t('Remove filter {n}', { n })} onClick={() => { set({ filters: d.filters.filter((_, k) => k !== i) }); setPick(null) }}><IconTrash size={16} /></IconButton></span>
                  </li>
                )
              })}
            </ul>
            <div className="wi-add">
              <button type="button" className="btn sm" onClick={() => { const s = Math.round(at * 10) / 10; set({ filters: [...d.filters, { id: newId('filter'), category: 'language', start: s, end: Math.round((s + 2) * 10) / 10 }] }); setPick({ list: 'filters', index: d.filters.length }) }}><IconPlus size={16} /> {player ? t('Add one here') : t('Add one')}</button>
            </div>
          </>
        )}

        {tab === 'lyrics' && (
          <div className="wi-lyrics">
            <p className="muted wi-help">{t('The words, line by line in time. Play the song above and press “Line starts now” as each line begins, or type the times. A line with no time shows as plain words.')}</p>
            <div className="wi-add">
              <Field label={t('Language of the words')}><LanguagePicker label={t('Language of the words')} allowNone value={music.lyrics?.language || ''} onChange={(v) => setLyrics({ ...(music.lyrics || {}), language: v })} /></Field>
              <label className="wi-check"><input type="checkbox" checked={!!music.lyrics?.instrumental} onChange={(e) => setLyrics({ ...(music.lyrics || {}), instrumental: e.target.checked })} /> {t('Instrumental: no words')}</label>
            </div>
            <ol className="wi-list wi-lyric-lines" aria-label={t('Lines')}>
              {lyricLines.map((l, i) => (
                <li key={i} className={'wi-item wi-lyric' + (i === cursor ? ' wi-picked' : '')} aria-current={i === cursor ? 'step' : undefined} onFocusCapture={() => setCursor(i)}>
                  <TimeInput label={t('Line {n}: starts', { n: i + 1 })} value={l.start} onChange={(v) => setLyrics({ ...(music.lyrics || {}), lines: lyricLines.map((x, k) => (k === i ? { ...x, start: v ?? 0 } : x)) })} />
                  <Input inputSize="sm" className="wi-label" aria-label={t('Line {n}: words', { n: i + 1 })} value={l.text} maxLength={1000} onChange={(e) => setLyrics({ ...(music.lyrics || {}), lines: lyricLines.map((x, k) => (k === i ? { ...x, text: e.target.value } : x)) })} />
                  <Input inputSize="sm" className="wi-singer" aria-label={t('Line {n}: who sings it', { n: i + 1 })} placeholder={t('Singer')} value={l.singer || ''} maxLength={200} onChange={(e) => setLyrics({ ...(music.lyrics || {}), lines: lyricLines.map((x, k) => (k === i ? { ...x, singer: e.target.value || undefined } : x)) })} />
                  <span className="wi-row-btns">
                    {player && <button type="button" className="btn sm ghost" onClick={() => { player.seek(Math.max(0, l.start - 2)); player.play() }}>{t('Play')}</button>}
                    <IconButton size="sm" label={t('Remove line {n}', { n: i + 1 })} onClick={() => setLyrics({ ...(music.lyrics || {}), lines: lyricLines.filter((_, k) => k !== i) })}><IconTrash size={16} /></IconButton>
                  </span>
                </li>
              ))}
            </ol>
            <div className="wi-add">
              <button type="button" className="btn sm" onClick={() => setLyrics({ ...(music.lyrics || {}), lines: [...lyricLines, { start: lyricLines.length ? lyricLines[lyricLines.length - 1].start + 3 : 0, text: '' }] })}><IconPlus size={16} /> {t('Add a line')}</button>
            </div>
            <Field label={t('Paste words to time them')} hint={t('One line each. They replace the lines above, with no times yet: then play the song and press “Line starts now” for each.')}>
              <Textarea rows={3} onBlur={(e) => { const text = e.target.value.trim(); if (!text) return; setLyrics({ ...(music.lyrics || {}), lines: text.split('\n').map((x) => ({ start: 0, text: x.trim() })).filter((x) => x.text) }); setCursor(0); e.target.value = '' }} />
            </Field>
          </div>
        )}

        {tab === 'music' && (
          <div className="wi-form">
            <p className="muted wi-help">{scope === 'album' ? t('Details for every song of this album. A song’s own file wins over these.') : t('What is filled in here replaces what the song’s tags say. The song file itself is never changed.')}</p>
            <Field label={t('Artists')} hint={t('Separated by commas.')}><Input value={((music.artists as string[]) || []).join(', ')} onChange={(e) => setMusic({ artists: e.target.value.split(',').map((x) => x.trim()).filter(Boolean).slice(0, 50) })} /></Field>
            <Field label={t('Album')}><Input maxLength={200} value={(music.album as string) || ''} onChange={(e) => setMusic({ album: e.target.value || undefined })} /></Field>
            <Field label={t('Album artist')}><Input maxLength={200} value={(music.albumArtist as string) || ''} onChange={(e) => setMusic({ albumArtist: e.target.value || undefined })} /></Field>
            {scope === 'song' && <Field label={t('Track')}><Input inputMode="numeric" value={(music.track as number) ?? ''} onChange={(e) => setMusic({ track: e.target.value ? Number(e.target.value) || undefined : undefined })} /></Field>}
            {scope === 'song' && <Field label={t('Disc')}><Input inputMode="numeric" value={(music.disc as number) ?? ''} onChange={(e) => setMusic({ disc: e.target.value ? Number(e.target.value) || undefined : undefined })} /></Field>}
            <Field label={t('Composers')} hint={t('Separated by commas.')}><Input value={((music.composers as string[]) || []).join(', ')} onChange={(e) => setMusic({ composers: e.target.value.split(',').map((x) => x.trim()).filter(Boolean).slice(0, 50) })} /></Field>
            <Field label={t('Label')}><Input maxLength={200} value={(music.label as string) || ''} onChange={(e) => setMusic({ label: e.target.value || undefined })} /></Field>
            {scope === 'song' && <Field label="ISRC" hint={t('Such as GB-AAA-26-00001.')}><Input maxLength={20} value={(music.isrc as string) || ''} onChange={(e) => setMusic({ isrc: e.target.value.toUpperCase() || undefined })} /></Field>}
            {scope === 'song' && <Field label={t('Beats a minute')}><Input inputMode="decimal" value={(music.bpm as number) ?? ''} onChange={(e) => setMusic({ bpm: e.target.value ? Number(e.target.value) || undefined : undefined })} /></Field>}
            {scope === 'song' && <Field label={t('Key')} hint={t('Such as A minor or F#m.')}><Input maxLength={16} value={(music.key as string) || ''} onChange={(e) => setMusic({ key: e.target.value || undefined })} /></Field>}
            {scope === 'song' && <label className="wi-check"><input type="checkbox" checked={!!music.explicit} onChange={(e) => setMusic({ explicit: e.target.checked || undefined })} /> {t('Explicit')}</label>}
            {scope === 'artist' && <Field label={t('Biography')}><Textarea rows={4} maxLength={20000} value={(music.biography as string) || ''} onChange={(e) => setMusic({ biography: e.target.value || undefined })} /></Field>}
            <h3 className="wi-h">{t('Loudness and trimming')}</h3>
            <Field label={t('Track gain (dB)')} hint={t('How much to change it so every song plays as loud.')}><Input inputMode="decimal" value={((music.loudness as Record<string, number>) || {}).trackGain ?? ''} onChange={(e) => setMusic({ loudness: { ...((music.loudness as object) || {}), trackGain: e.target.value === '' ? undefined : Number(e.target.value) } })} /></Field>
            <Field label={t('Album gain (dB)')}><Input inputMode="decimal" value={((music.loudness as Record<string, number>) || {}).albumGain ?? ''} onChange={(e) => setMusic({ loudness: { ...((music.loudness as object) || {}), albumGain: e.target.value === '' ? undefined : Number(e.target.value) } })} /></Field>
            {scope === 'song' && <Field label={t('Start playing at')}><TimeInput label={t('Start playing at')} value={music.trimStart as number | undefined} blank placeholder={t('the start')} onChange={(v) => setMusic({ trimStart: v ?? undefined })} /></Field>}
            {scope === 'song' && <Field label={t('Stop playing at')} hint={t('Before a long silence or a hidden track.')}><TimeInput label={t('Stop playing at')} value={music.trimEnd as number | undefined} blank placeholder={t('the end')} onChange={(v) => setMusic({ trimEnd: v ?? undefined })} /></Field>}
            {scope === 'song' && <Field label={t('Fade out (seconds)')}><Input inputMode="decimal" value={(music.fadeOut as number) ?? ''} onChange={(e) => setMusic({ fadeOut: e.target.value === '' ? undefined : Number(e.target.value) })} /></Field>}
          </div>
        )}

        {tab === 'details' && (
          <div className="wi-form">
            <p className="muted wi-help">{t('What is filled in here replaces what was found, on the next scan and straight away when saved. Leave a box empty to keep what was found.')}</p>
            <Field label={audio && scope === 'album' ? t('Album title') : t('Title')}><Input value={d.metadata.title || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, title: e.target.value } })} /></Field>
            <Field label={t('Sort as')}><Input value={d.metadata.sortTitle || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, sortTitle: e.target.value } })} /></Field>
            {!audio && <Field label={t('Edition')} hint={t('Director’s Cut, Extended…')}><Input value={d.metadata.edition || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, edition: e.target.value } })} /></Field>}
            <Field label={t('Year')}><Input inputMode="numeric" value={d.metadata.year ?? ''} onChange={(e) => set({ metadata: { ...d.metadata, year: e.target.value ? Number(e.target.value) || undefined : undefined } })} /></Field>
            {!audio && <Field label={t('Tagline')}><Input value={d.metadata.tagline || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, tagline: e.target.value } })} /></Field>}
            <Field label={t('Overview')}><Textarea rows={4} value={d.metadata.overview || ''} maxLength={20000} onChange={(e) => set({ metadata: { ...d.metadata, overview: e.target.value } })} /></Field>
            <Field label={t('Genres')} hint={t('Separated by commas.')}><Input value={(d.metadata.genres || []).join(', ')} onChange={(e) => set({ metadata: { ...d.metadata, genres: e.target.value.split(',').map((g) => g.trim()).filter(Boolean).slice(0, 50) } })} /></Field>
            {!audio && <Field label={t('Content rating')}><Input value={d.metadata.contentRating || ''} maxLength={200} onChange={(e) => set({ metadata: { ...d.metadata, contentRating: e.target.value } })} /></Field>}
            {scope === 'item' && <Field label={t('Kind of episode')} hint={t('Mostly for anime: whether it is part of the story.')}>
              <Select value={d.metadata.episodeKind || ''} onChange={(e) => set({ metadata: { ...d.metadata, episodeKind: e.target.value || undefined } })} options={[{ value: '', label: t('Not said') }, { value: 'canon', label: t('Part of the story') }, { value: 'filler', label: t('Filler') }, { value: 'mixed', label: t('Partly filler') }, { value: 'recap', label: t('Recap episode') }]} />
            </Field>}
            {scope === 'item' && (
              <>
                <h3 className="wi-h">{t('Playing')}</h3>
                <Field label={t('Start playing at')} hint={t('The first time: past a studio logo, say.')}><TimeInput label={t('Start playing at')} value={d.playback.startAt} blank placeholder={t('the start')} onChange={(v) => set({ playback: { ...d.playback, startAt: v ?? undefined } })} /></Field>
                <Field label={t('Sound in')}><LanguagePicker label={t('Sound in')} allowNone value={d.playback.audioLanguage || ''} onChange={(v) => set({ playback: { ...d.playback, audioLanguage: v || undefined } })} /></Field>
                <Field label={t('Subtitles in')}><LanguagePicker label={t('Subtitles in')} allowNone value={d.playback.subtitleLanguage || ''} onChange={(v) => set({ playback: { ...d.playback, subtitleLanguage: v || undefined } })} /></Field>
                <Field label={t('Subtitles')}><Select value={d.playback.subtitleMode || ''} onChange={(e) => set({ playback: { ...d.playback, subtitleMode: e.target.value || undefined } })} options={[{ value: '', label: t('As each person chooses') }, { value: 'off', label: t('Off') }, { value: 'forced', label: t('Only forced subtitles') }, { value: 'always', label: t('Always') }]} /></Field>
                <Field label={t('Louder or quieter (dB)')} hint={t('For a file that is much quieter or louder than the rest: −30 to 30.')}><Input inputMode="decimal" value={d.playback.volumeDb ?? ''} onChange={(e) => set({ playback: { ...d.playback, volumeDb: e.target.value === '' ? undefined : Math.max(-30, Math.min(30, Number(e.target.value) || 0)) } })} /></Field>
                <Field label={t('Picture shape')} hint={t('For a file that says the wrong shape.')}><Select value={d.playback.aspectRatio || ''} onChange={(e) => set({ playback: { ...d.playback, aspectRatio: e.target.value || undefined } })} options={[{ value: '', label: t('As the file says') }, ...['4:3', '16:9', '1.85:1', '2:1', '2.35:1', '2.39:1', '21:9'].map((v) => ({ value: v, label: v }))]} /></Field>
              </>
            )}
            <h3 className="wi-h">{t('Notes')}</h3>
            <Field label={t('Notes')} hint={t('Anything worth saying about this file.')}><Textarea rows={3} value={d.notes} maxLength={20000} onChange={(e) => set({ notes: e.target.value })} /></Field>
          </div>
        )}

        {tab === 'pictures' && (
          <div className="wi-pictures">
            <p className="muted wi-help">{t('Pictures kept in the file itself, so they go wherever it goes. They take the place of what was found. JPEG, PNG or WebP, 16 MB at most.')}</p>
            {(audio ? [{ role: 'poster', label: t('Cover') }, { role: 'back', label: t('Back of the sleeve') }, { role: 'artist', label: t('Artist') }, { role: 'disc', label: t('Disc') }]
              : [{ role: 'poster', label: t('Poster') }, { role: 'backdrop', label: t('Backdrop') }, { role: 'logo', label: t('Logo') }, { role: 'thumb', label: t('Episode picture') }, { role: 'banner', label: t('Banner') }, { role: 'clearart', label: t('Clear art') }, { role: 'disc', label: t('Disc') }]
            ).map(({ role, label }) => {
              const name = d.artwork[role]
              const file = ans.files.find((f) => f.name === name)
              return (
                <div key={role} className="wi-picture">
                  <div className={'wi-pic-frame ' + role}>{file?.url ? <img src={mediaUrl(file.url.replace(/^\/api/, '/api'))} alt={label} /> : <span className="muted">{t('None')}</span>}</div>
                  <b>{label}</b>
                  <label className="btn sm">
                    {name ? t('Replace') : t('Add')}
                    <input type="file" accept="image/jpeg,image/png,image/webp" className="wi-file" disabled={busy}
                      onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) run(() => uploadFile<InfoAnswer>(under(base, `/images/${role}`), f, 'PUT'), t('{what} saved', { what: label })) }} />
                  </label>
                  {name && <button type="button" className="btn sm ghost" disabled={busy} onClick={() => run(() => api.del<InfoAnswer>(under(base, `/images/${role}`)), t('{what} taken out', { what: label }))}>{t('Remove')}</button>}
                </div>
              )
            })}
          </div>
        )}

        {tab === 'subtitles' && (
          <div className="wi-subs">
            <p className="muted wi-help">{t('Subtitle files kept in the file itself (SRT, WebVTT, ASS or SSA, 2 MB at most). Each is a track the player offers, after the video’s own.')}</p>
            <ul className="wi-list" aria-label={t('Subtitles')}>
              {d.subtitles.map((s) => (
                <li key={s.file} className="wi-item wi-sub-item">
                  <b>{s.title || (s.language ? langName(s.language) : s.file.replace(/^subtitles\//, ''))}</b>
                  <span className="muted">{[s.language && langName(s.language), s.forced && t('forced'), s.sdh && t('for the deaf and hard of hearing')].filter(Boolean).join(' · ')}</span>
                  <span className="wi-row-btns"><IconButton size="sm" label={t('Remove {what}', { what: s.file })} onClick={() => run(() => api.del<InfoAnswer>(under(base, `/subtitles/${encodeURIComponent(s.file.replace(/^subtitles\//, ''))}`)), t('Taken out'))}><IconTrash size={16} /></IconButton></span>
                </li>
              ))}
            </ul>
            <div className="wi-add">
              <LanguagePicker label={t('Language of the subtitles')} allowNone value={subLang} onChange={setSubLang} />
              <label className="wi-check"><input type="checkbox" id={uid + 'forced'} /> {t('Forced')}</label>
              <label className="wi-check"><input type="checkbox" id={uid + 'sdh'} /> {t('For the deaf and hard of hearing')}</label>
              <label className="btn sm">
                <IconPlus size={16} /> {t('Add a subtitle file…')}
                <input type="file" accept=".srt,.vtt,.ass,.ssa" className="wi-file" disabled={busy} onChange={(e) => {
                  const f = e.target.files?.[0]; e.target.value = ''
                  if (!f) return
                  const ext = (f.name.match(/\.(srt|vtt|ass|ssa)$/i)?.[1] || 'srt').toLowerCase()
                  const forced = (document.getElementById(uid + 'forced') as HTMLInputElement | null)?.checked
                  const sdh = (document.getElementById(uid + 'sdh') as HTMLInputElement | null)?.checked
                  const name = [subLang || 'und', forced ? 'forced' : '', sdh ? 'sdh' : '', String(d.subtitles.length + 1)].filter(Boolean).join('.') + '.' + ext
                  const q = new URLSearchParams({ ...(subLang ? { language: subLang } : {}), ...(forced ? { forced: '1' } : {}), ...(sdh ? { sdh: '1' } : {}) }).toString()
                  run(() => uploadFile<InfoAnswer>(under(base, `/subtitles/${name}`, q), f, 'PUT'), t('Subtitles added'))
                }} />
              </label>
            </div>
          </div>
        )}

        {tab === 'languages' && (
          <div className="wi-languages">
            <p className="muted wi-help">{t('The file’s own text is in one language; add others and translate each name, title and the details. Everyone sees the language closest to their own, and the file’s own text where nothing is translated.')}</p>
            <Field label={t('This file is written in')}><LanguagePicker label={t('This file is written in')} allowNone value={d.language} onChange={(v) => set({ language: v })} /></Field>
            <div className="wi-add">
              <LanguagePicker label={t('Add a language')} value="" exclude={[d.language, ...translationLangs]} onChange={(v) => { if (!v || d.translations[v]) return; setD(ensureIds({ ...d, translations: { ...d.translations, [v]: {} } })); setLang(v) }} />
            </div>
            {translationLangs.length > 0 && (
              <>
                <Tabs label={t('Translations')} value={activeLang} onChange={setLang} items={translationLangs.map((c) => ({ value: c, label: langName(c) }))} />
                <div className="wi-translation" lang={activeLang}>
                  <div className="wi-add">
                    <button type="button" className="btn sm ghost danger" onClick={async () => {
                      if (!(await ask({ title: t('Remove the {language} translation?', { language: langName(activeLang) }), confirm: t('Remove'), danger: true }))) return
                      const rest = { ...d.translations }; delete rest[activeLang]; set({ translations: rest }); setLang('')
                    }}><IconTrash size={16} /> {t('Remove this language')}</button>
                  </div>
                  {d.segments.some((s) => s.label || s.type === 'custom') && <h3 className="wi-h">{t('Names of parts')}</h3>}
                  {d.segments.filter((s) => s.label || s.type === 'custom').map((s) => (
                    <Field key={s.id} label={s.label || tx(segLabel(s.type))}><Input maxLength={200} value={tr.segments?.[s.id!]?.label || ''} placeholder={s.label} onChange={(e) => setD(setText(ensureIds(d), activeLang, 'segments', s.id!, 'label', e.target.value))} /></Field>
                  ))}
                  {d.scenes.length > 0 && <h3 className="wi-h">{audio ? t('Chapters') : t('Scenes')}</h3>}
                  {d.scenes.map((s) => (
                    <Field key={s.id} label={s.title}><Input maxLength={200} value={tr.scenes?.[s.id!]?.title || ''} placeholder={s.title} onChange={(e) => setD(setText(ensureIds(d), activeLang, 'scenes', s.id!, 'title', e.target.value))} /></Field>
                  ))}
                  {d.filters.filter((f) => f.label).map((f) => (
                    <Field key={f.id} label={f.label!}><Input maxLength={200} value={tr.filters?.[f.id!]?.label || ''} placeholder={f.label} onChange={(e) => setD(setText(ensureIds(d), activeLang, 'filters', f.id!, 'label', e.target.value))} /></Field>
                  ))}
                  <h3 className="wi-h">{t('Details')}</h3>
                  <Field label={t('Title')}><Input maxLength={200} value={tr.metadata?.title || ''} placeholder={d.metadata.title} onChange={(e) => setD(setText(d, activeLang, 'metadata', '', 'title', e.target.value))} /></Field>
                  {!audio && <Field label={t('Tagline')}><Input maxLength={200} value={tr.metadata?.tagline || ''} placeholder={d.metadata.tagline} onChange={(e) => setD(setText(d, activeLang, 'metadata', '', 'tagline', e.target.value))} /></Field>}
                  <Field label={t('Overview')}><Textarea rows={3} maxLength={20000} value={tr.metadata?.overview || ''} placeholder={d.metadata.overview} onChange={(e) => setD(setText(d, activeLang, 'metadata', '', 'overview', e.target.value))} /></Field>
                  <Field label={t('Notes')}><Textarea rows={2} maxLength={20000} value={tr.notes || ''} placeholder={d.notes} onChange={(e) => setD(setText(d, activeLang, 'notes', '', '', e.target.value))} /></Field>
                  {scope === 'song' && lyricLines.length > 0 && (
                    <>
                      <h3 className="wi-h">{t('Lyrics')}</h3>
                      <p className="muted wi-help">{t('Each line keeps the song’s own time.')}</p>
                      {lyricLines.map((l, i) => {
                        const lines = (tr.lyrics as Lyrics | undefined)?.lines || []
                        return (
                          <Field key={i} label={`${clockText(l.start)}  ${l.text}`}>
                            <Input maxLength={1000} value={lines[i]?.text || ''} placeholder={l.text} onChange={(e) => {
                              const next = lyricLines.map((x, k) => ({ start: x.start, text: k === i ? e.target.value : lines[k]?.text || '' }))
                              setD({ ...d, translations: { ...d.translations, [activeLang]: { ...tr, lyrics: { lines: next } } } })
                            }} />
                          </Field>
                        )
                      })}
                    </>
                  )}
                </div>
              </>
            )}
          </div>
        )}

        {tab === 'file' && (
          <div className="wi-filetab">
            <dl className="wi-facts">
              <dt>{t('Where')}</dt><dd>{where}{ans.path ? <> · <code>{ans.path}</code></> : null}</dd>
              {ans.info?.identity && <><dt>{t('Made for')}</dt><dd>{t('{name}, {length} long', { name: String(ans.info.identity.fileName || t('a file')), length: clockText(Number(ans.info.identity.durationSec)) })}{ans.match ? ` · ${ans.match.result === 'exact' ? t('this very file') : ans.match.result === 'same-cut' ? t('the same cut') : ans.match.result === 'other-cut' ? t('another cut') : ans.match.result === 'forced' ? t('another cut, used anyway') : t('not checked')}` : ''}</dd></>}
              {ans.info?.authorship?.tool && <><dt>{t('Written with')}</dt><dd>{ans.info.authorship.tool.name} {ans.info.authorship.tool.version}</dd></>}
              <dt>{t('Format')}</dt><dd>{audio ? '.wittyaudio' : '.wittyinfo'}</dd>
              <dt>{t('Saving')}</dt><dd>{ans.canWrite ? t('Writes the file beside the media.') : t('Kept on the server: writing these files beside the media is off (Settings, Metadata), or there is no place for one.')}</dd>
            </dl>
            <div className="wi-add">
              <button type="button" className="btn sm" disabled={!ans.info} onClick={() => downloadFile(under(base, '/export'), audio ? 'info.wittyaudio' : 'info.wittyinfo').catch((e) => toast((e as Error).message, { tone: 'danger' }))}><IconDownload size={16} /> {t('Export')}</button>
              <button type="button" className="btn sm" disabled={busy} onClick={() => importer.current?.click()}><IconFolder size={16} /> {t('Import…')}</button>
              <input ref={importer} type="file" accept=".wittyinfo,.wittyaudio,.json,application/zip,application/json" className="wi-file" onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) run(() => uploadFile<InfoAnswer>(under(base, '/import'), f, 'POST'), t('Imported')) }} />
              {ans.origin && <button type="button" className="btn sm ghost danger" disabled={busy} onClick={async () => {
                const beside = ans.fileFound && ans.canWrite && (ans.origin === 'file' || !!ans.path)
                const ok = await ask({ title: t('Remove the custom information?'), body: beside ? t('The server’s copy is forgotten and the file beside the media is renamed with .bak on the end. What was found by itself comes back.') : t('The server’s copy is forgotten. A file beside the media, if there is one, is used again.'), confirm: t('Remove'), danger: true })
                if (ok) run(() => api.del<InfoAnswer>(under(base, '', beside ? 'file=1' : '')), t('Removed'))
              }}><IconTrash size={16} /> {t('Remove')}</button>}
            </div>
            <p className="muted wi-help">{t('These files are small archives any WittyMediaServer reads, and the wittyinfo program edits without a server.')}</p>
          </div>
        )}
      </div>

      {(allProblems.length > 0 || err) && (
        <div className="error wi-problems" role="alert">
          {err && <p>{tx(err)}</p>}
          {allProblems.length > 0 && <ul>{allProblems.slice(0, 8).map((p, i) => <li key={i}>{tx(problemWords(p, d))}</li>)}</ul>}
        </div>
      )}
      <div className="wi-foot">
        {(player || onPreview) && (sel || selScene || selFilter) && <button type="button" className="btn" onClick={preview}><IconPlay size={16} /> {t('Preview from just before')}</button>}
        <span className="grow" />
        {dirty && <button type="button" className="btn ghost" disabled={busy} onClick={() => { setD(saved); setProblems([]) }}><IconClose size={16} /> {t('Undo changes')}</button>}
        <button type="button" className="btn primary" disabled={busy || !dirty} onClick={save}>{t('Save')}</button>
      </div>
    </div>
  )
}

// LivePreview plays the video in the wittyinfo program, with the skip button the player would
// show for the parts as they are now, unsaved or not.
export function LivePreview({ src, markers, duration, onLink }: { src: string; markers: ReturnType<typeof draftMarkers> | null; duration: number; onLink: (l: PlayerLink, now: number) => void }): ReactNode {
  const v = useRef<HTMLVideoElement | null>(null)
  const [now, setNow] = useState(0)
  const skip = skipAt(markers, now, duration)
  useEffect(() => {
    const el = v.current
    if (!el) return
    onLink({ now: () => el.currentTime, seek: (tm) => { el.currentTime = tm }, playing: () => !el.paused, play: () => { void el.play().catch(() => {}) }, pause: () => el.pause() }, now)
  }, [now]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="wi-preview">
      <video ref={v} src={src} controls preload="metadata" onTimeUpdate={(e) => setNow(e.currentTarget.currentTime)} onSeeked={(e) => setNow(e.currentTarget.currentTime)} />
      {skip && <button type="button" className="btn skip-intro wi-skip" onClick={() => { if (v.current) v.current.currentTime = skip.to }}>{tx(skip.label)} ›</button>}
    </div>
  )
}
