// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { api, fmtTime } from '../api'
import { IconClose, IconEdit, IconRefresh } from '../icons'
import { toast } from '../state'
import { ownTiming, useLineIndex, type LyricLine, type useLyrics } from './lyrics'
import type { PlayerView } from './types'
import { WebLink } from '../components/WebLink'
import { num, t } from '../i18n'

// Lyrics that follow the music: the line being sung stays in the middle, and pressing any
// line goes to it. Scrolling by hand holds the view still for a few seconds. Words without
// times simply read as text. If the timing is off, nudging it fixes it for you alone, and
// an admin can correct it for everyone: choose another version, paste, or tap along.

const NO_LINES: LyricLine[] = []

// switcher: Now playing's one choice between these words in time and Read (ReadPanel, with the album's booklet), shown
// at the top of both, so there's one control for it (#520).
export function LyricsPanel({ p, ly, onClose, switcher }: { p: PlayerView; ly: ReturnType<typeof useLyrics>; onClose: () => void; switcher?: ReactNode }) {
  const { lyrics, loading, failed, reload, setOffset } = ly
  const lines = lyrics?.synced ? lyrics.lines : NO_LINES
  const offset = lyrics?.offsetMs || 0
  const index = useLineIndex(lines, p.position, offset, p.playing)
  const box = useRef<HTMLOListElement>(null)
  const heldUntil = useRef(0)
  const [fixing, setFixing] = useState(false)
  // Timed words follow along, the sung line in the middle; words without times read as text. Reading at your own pace
  // is Read, beside this (switcher).
  const timed = lines.length > 0
  const reading = !timed
  const readText = lyrics?.plain || lines.map((x) => x.text).join('\n')
  useEffect(() => {
    const list = box.current
    if (!list || index < 0 || performance.now() < heldUntil.current) return
    const el = list.children[index] as HTMLElement | undefined
    if (!el) return
    const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    list.scrollTo({ top: el.offsetTop - list.clientHeight / 2 + el.clientHeight / 2, behavior: reduced ? 'auto' : 'smooth' })
  }, [index])
  const hold = () => { heldUntil.current = performance.now() + 4000 }
  const nudge = (ms: number) => setOffset(Math.max(-120000, Math.min(120000, offset + ms)))

  return (
    <div className="np-side np-lyrics">
      <div className="np-side-head">
        <h3>{t('Lyrics')}</h3>
        {lyrics?.sourceName && <span className="np-side-meta">{lyrics.creditUrl ? <WebLink href={lyrics.creditUrl}>{lyrics.sourceName}</WebLink> : lyrics.sourceName}</span>}
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label={t('Close the lyrics')}><IconClose /></button>
      </div>
      {(switcher || (lyrics && !loading && !lyrics.instrumental && lyrics.plain && !timed)) && (
        <div className="np-modes">
          {switcher}
          {lyrics && !loading && !lyrics.instrumental && lyrics.plain && !timed && <span className="np-hint">{t('These words have no times, so they read as text.')}</span>}
        </div>
      )}
      {loading && <p className="np-hint">{t('Looking for the words…')}</p>}
      {failed && <p className="np-note" role="alert">{failed}</p>}
      {lyrics?.instrumental && <p className="np-empty">{t('This one has no words.')}</p>}
      {lyrics && !loading && !lyrics.instrumental && !lyrics.plain && <NoLyrics p={p} online={lyrics.online} canEdit={lyrics.canEdit} tried={lyrics.tried} failed={lyrics.failed} />}
      {timed && !reading ? (
        <ol className="np-lines" ref={box} onWheel={hold} onTouchMove={hold}>
          {lines.map((l, i) => (
            <li key={i} className={i === index ? 'now' : i < index ? 'past' : ''}>
              <button type="button" aria-current={i === index ? 'true' : undefined} aria-label={`${fmtTime(l.t / 1000)}: ${l.text || t('a break')}`}
                onClick={() => { heldUntil.current = 0; p.seek(Math.max(0, (l.t + offset) / 1000)) }}>
                {l.text || <span aria-hidden="true">♪</span>}
              </button>
            </li>
          ))}
        </ol>
      ) : readText ? <pre className="np-read-text np-plain" tabIndex={0}>{readText}</pre> : null}
      {lyrics && (lyrics.plain || lyrics.canEdit) && (
        <div className="np-lyric-tools">
          {timed && !reading && (
            <span className="np-nudge" role="group" aria-label={t('Timing')}>
              <button type="button" className="np-chip small" onClick={() => nudge(-250)} title={t('The words come sooner')}>{t('Sooner')}</button>
              <span aria-live="polite" className="np-nudge-v">{offset === 0 ? t('On time') : t('{n} s', { n: (offset > 0 ? '+' : '') + num(offset / 1000, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) })}{lyrics?.autoSync === 'set' && offset === ownTiming(lyrics) ? t(', set by listening') : ''}</span>
              <button type="button" className="np-chip small" onClick={() => nudge(250)} title={t('The words come later')}>{t('Later')}</button>
              {offset !== ownTiming(lyrics) && <button type="button" className="np-chip small" onClick={() => setOffset(ownTiming(lyrics))}>{t('Reset')}</button>}
            </span>
          )}
          {lyrics.canEdit && <button type="button" className="np-chip small" onClick={() => setFixing(true)}><IconEdit size={16} /> {t('Fix')}</button>}
        </div>
      )}
      {fixing && lyrics && <FixLyrics p={p} text={lyrics.plain} lines={lines} source={lyrics.source} online={lyrics.online} onClose={() => setFixing(false)} onSaved={() => { setFixing(false); reload() }} />}
    </div>
  )
}

function NoLyrics({ p, online, canEdit, tried, failed }: { p: PlayerView; online: boolean; canEdit: boolean; tried: boolean; failed?: string }) {
  if (failed) return <p className="np-empty">{t('The lyrics source couldn’t be reached just now. It’ll be tried again next time this song is opened.')}</p>
  if (tried) return <p className="np-empty">{t('No words found for this song online.')} {canEdit ? t('Fix lets you search under another name, or add them yourself.') : ''}</p>
  if (online) return <p className="np-empty">{t('No words found for this song.')}</p>
  return (
    <div className="np-empty">
      <p>{t('No words here. A .lrc file with the same name as the song, beside it, is used first.')}</p>
      {canEdit
        ? <p>{t('Or find them online for free, if you agree:')} <button type="button" className="np-link" onClick={() => p.goTowards('/settings/metadata#lyrics')}>{t('turn on online lyrics')}</button> {t('in Settings. Your server sends only the artist, title, album and length.')}</p>
        : <p>{t('Whoever runs this server can also turn on finding them online.')}</p>}
      <p><button type="button" className="np-link" onClick={() => p.goTowards('/help/lyrics-karaoke#if-lyrics-are-missing-or-wrong')}>{t('How lyrics work')}</button></p>
    </div>
  )
}

type Found = { provider: string; id: string; title: string; artist: string; album?: string; durationSec?: number; hasSynced: boolean; instrumental?: boolean }

function FixLyrics({ p, text, lines, source, online, onClose, onSaved }: { p: PlayerView; text: string; lines: LyricLine[]; source: string; online: boolean; onClose: () => void; onSaved: () => void }) {
  const it = p.current!
  const [tab, setTab] = useState<'search' | 'edit' | 'tap'>(online ? 'search' : 'edit')
  const [q, setQ] = useState(`${it.artist || ''} ${it.title}`.trim())
  const [found, setFound] = useState<Found[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [draft, setDraft] = useState(lines.length ? lines.map((l) => `[${stamp(l.t)}]${l.text}`).join('\n') : text)
  const [error, setError] = useState('')
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => { ref.current?.querySelector<HTMLElement>('input, textarea, button:not(.np-tab)')?.focus() }, [tab])
  useEffect(() => {
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.preventDefault(); e.stopImmediatePropagation(); onClose() } }
    window.addEventListener('keydown', esc, true)
    return () => window.removeEventListener('keydown', esc, true)
  }, [onClose])
  const url = `/music/lyrics/${it.id}`
  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true)
    setError('')
    try { await fn() } catch (e) { setError((e as Error).message) }
    setBusy(false)
  }
  return (
    <div className="np-fix" role="dialog" aria-modal="true" aria-label={t('Fix the lyrics of {title}', { title: it.title })} ref={ref}>
      <div className="np-side-head">
        <h3>{t('Fix the lyrics')}</h3>
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label={t('Close')}><IconClose /></button>
      </div>
      <div className="np-tabs" role="tablist">
        {online && <button type="button" role="tab" className={'np-tab' + (tab === 'search' ? ' on' : '')} aria-selected={tab === 'search'} onClick={() => setTab('search')}>{t('Other versions')}</button>}
        <button type="button" role="tab" className={'np-tab' + (tab === 'edit' ? ' on' : '')} aria-selected={tab === 'edit'} onClick={() => setTab('edit')}>{t('Edit')}</button>
        <button type="button" role="tab" className={'np-tab' + (tab === 'tap' ? ' on' : '')} aria-selected={tab === 'tap'} onClick={() => setTab('tap')}>{t('Tap along')}</button>
      </div>
      {error && <p className="np-note" role="alert">{error}</p>}
      {tab === 'search' && (
        <div className="np-fix-body">
          <form className="np-fix-search" onSubmit={(e) => { e.preventDefault(); void run(async () => setFound(await api.post<Found[]>(`${url}/search`, { q }))) }}>
            <input className="input" aria-label={t('Search for lyrics')} value={q} maxLength={300} onChange={(e) => setQ(e.target.value)} />
            <button className="btn sm primary" disabled={busy || !q.trim()}>{t('Search')}</button>
          </form>
          {found?.length === 0 && <p className="np-hint">{t('Nothing found. Try fewer words.')}</p>}
          <ul className="np-found">
            {found?.map((f) => {
              const off = f.durationSec && it.runtimeSec ? Math.abs(f.durationSec - it.runtimeSec) : null
              return (
                <li key={f.provider + f.id}>
                  <button type="button" disabled={busy} onClick={() => void run(async () => { await api.put(url, { provider: f.provider, id: f.id }); toast(t('Lyrics changed')); onSaved() })}>
                    <b>{f.title}</b><small>{[f.artist, f.album].filter(Boolean).join(' · ')}</small>
                    <small>{f.instrumental ? t('No words') : f.hasSynced ? t('Timed') : t('Words only')}{off !== null ? ` · ${off < 3 ? t('same length') : t('{n} s different', { n: Math.round(off) })}` : ''}</small>
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      )}
      {tab === 'edit' && (
        <div className="np-fix-body">
          <textarea className="input np-fix-text" aria-label={t('The lyrics')} value={draft} spellCheck={false} onChange={(e) => setDraft(e.target.value)} placeholder={t('[00:12.50]A line, with the time it starts\nor plain words with no times')} />
          <p className="np-hint">{t('Kept on this server and used in place of everything else. Nothing is written beside your music.')}</p>
          <div className="np-fix-row">
            <button type="button" className="btn sm primary" disabled={busy || !draft.trim()} onClick={() => void run(async () => { await api.put(url, { text: draft }); toast(t('Lyrics saved')); onSaved() })}>{t('Save')}</button>
            <button type="button" className="btn sm" disabled={busy} onClick={() => void run(async () => { await api.put(url, { instrumental: true }); toast(t('Marked as having no words')); onSaved() })}>{t('This song has no words')}</button>
            {(source === 'override' || source === 'lrclib' || source === 'mirror' || source === 'musixmatch') && <button type="button" className="btn sm ghost" disabled={busy} onClick={() => void run(async () => { await api.del(url); toast(t('Back to what was found')); onSaved() })}><IconRefresh size={16} /> {t('Start over')}</button>}
          </div>
        </div>
      )}
      {tab === 'tap' && <TapAlong p={p} text={lines.length ? lines.map((l) => l.text).join('\n') : text} busy={busy}
        onSave={(lrc) => void run(async () => { await api.put(url, { text: lrc }); toast(t('Timing saved')); onSaved() })} />}
    </div>
  )
}

const stamp = (ms: number) => {
  const m = Math.floor(ms / 60000), s = Math.floor(ms / 1000) % 60, c = Math.floor((ms % 1000) / 10)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}.${String(c).padStart(2, '0')}`
}

// The simple way to fix timing: it plays from the start, and you press as each line begins.
function TapAlong({ p, text, busy, onSave }: { p: PlayerView; text: string; busy: boolean; onSave: (lrc: string) => void }) {
  const lines = text.split('\n').map((l) => l.trim()).filter(Boolean)
  const [times, setTimes] = useState<number[]>([])
  const [started, setStarted] = useState(false)
  const button = useRef<HTMLButtonElement>(null)
  const at = times.length
  const done = at >= lines.length && lines.length > 0
  const tap = () => { if (!done) { setTimes((t) => [...t, Math.round(p.position() * 1000)]); button.current?.focus() } }
  const start = () => {
    setTimes([])
    setStarted(true)
    p.seek(0)
    if (!p.playing) p.toggle()
    requestAnimationFrame(() => button.current?.focus())
  }
  if (!lines.length) return <div className="np-fix-body"><p className="np-hint">{t('Add the words under Edit first, then come back to time them.')}</p></div>
  return (
    <div className="np-fix-body np-tap">
      {!started ? (
        <>
          <p>{t('The song starts from the beginning. Press the big button, or Space, as each line starts.')}</p>
          <button type="button" className="btn primary" onClick={start}>{t('Start')}</button>
        </>
      ) : (
        <>
          <p className="np-tap-prev">{lines[at - 1] || ' '}</p>
          <p className="np-tap-now">{done ? t('That’s all of them.') : lines[at]}</p>
          <p className="np-tap-next">{lines[at + 1] || ' '}</p>
          <button type="button" ref={button} className="btn primary np-tap-btn" disabled={done} onClick={tap}>{t('This line starts now ({n} of {total})', { n: at + 1, total: lines.length })}</button>
          <div className="np-fix-row">
            <button type="button" className="btn sm" disabled={!at} onClick={() => { const t = times[at - 2] ?? 0; setTimes(times.slice(0, -1)); p.seek(Math.max(0, t / 1000)); button.current?.focus() }}>{t('Undo the last one')}</button>
            <button type="button" className="btn sm ghost" onClick={start}>{t('Start again')}</button>
            <button type="button" className="btn sm primary" disabled={!done || busy} onClick={() => onSave(times.map((t, i) => `[${stamp(t)}]${lines[i]}`).join('\n') + '\n')}>{t('Save')}</button>
          </div>
        </>
      )}
    </div>
  )
}
