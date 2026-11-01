import { useEffect, useRef, useState } from 'react'
import { api, fmtTime } from '../api'
import { IconClose, IconEdit, IconRefresh } from '../icons'
import { toast } from '../state'
import { useLineIndex, type LyricLine, type useLyrics } from './lyrics'
import type { PlayerView } from './types'
import { WebLink } from '../components/WebLink'

// Lyrics that follow the music: the line being sung stays in the middle, and pressing any
// line goes to it. Scrolling by hand holds the view still for a few seconds. Words without
// times simply read as text. If the timing is off, nudging it fixes it for you alone, and
// an admin can correct it for everyone: choose another version, paste, or tap along.

const NO_LINES: LyricLine[] = []

export function LyricsPanel({ p, ly, onClose }: { p: PlayerView; ly: ReturnType<typeof useLyrics>; onClose: () => void }) {
  const { lyrics, loading, failed, reload, setOffset } = ly
  const lines = lyrics?.synced ? lyrics.lines : NO_LINES
  const offset = lyrics?.offsetMs || 0
  const index = useLineIndex(lines, p.position, offset, p.playing)
  const box = useRef<HTMLOListElement>(null)
  const heldUntil = useRef(0)
  const [fixing, setFixing] = useState(false)
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
        <h3>Lyrics</h3>
        {lyrics?.sourceName && <span className="np-side-meta">{lyrics.creditUrl ? <WebLink href={lyrics.creditUrl}>{lyrics.sourceName}</WebLink> : lyrics.sourceName}</span>}
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label="Close the lyrics"><IconClose /></button>
      </div>
      {loading && <p className="np-hint">Looking for the words…</p>}
      {failed && <p className="np-note" role="alert">{failed}</p>}
      {lyrics?.instrumental && <p className="np-empty">This one has no words.</p>}
      {lyrics && !loading && !lyrics.instrumental && !lyrics.plain && <NoLyrics p={p} online={lyrics.online} canEdit={lyrics.canEdit} tried={lyrics.tried} failed={lyrics.failed} />}
      {lines.length > 0 ? (
        <ol className="np-lines" ref={box} onWheel={hold} onTouchMove={hold}>
          {lines.map((l, i) => (
            <li key={i} className={i === index ? 'now' : i < index ? 'past' : ''}>
              <button type="button" aria-current={i === index ? 'true' : undefined} aria-label={`${fmtTime(l.t / 1000)}: ${l.text || 'a break'}`}
                onClick={() => { heldUntil.current = 0; p.seek(Math.max(0, (l.t + offset) / 1000)) }}>
                {l.text || <span aria-hidden="true">♪</span>}
              </button>
            </li>
          ))}
        </ol>
      ) : lyrics?.plain ? <pre className="np-read-text np-plain" tabIndex={0}>{lyrics.plain}</pre> : null}
      {lyrics && (lyrics.plain || lyrics.canEdit) && (
        <div className="np-lyric-tools">
          {lines.length > 0 && (
            <span className="np-nudge" role="group" aria-label="Timing">
              <button type="button" className="np-chip small" onClick={() => nudge(-250)} title="The words come sooner">Sooner</button>
              <span aria-live="polite" className="np-nudge-v">{offset === 0 ? 'On time' : `${offset > 0 ? '+' : ''}${(offset / 1000).toFixed(2)} s`}</span>
              <button type="button" className="np-chip small" onClick={() => nudge(250)} title="The words come later">Later</button>
              {offset !== 0 && <button type="button" className="np-chip small" onClick={() => setOffset(0)}>Reset</button>}
            </span>
          )}
          {lyrics.canEdit && <button type="button" className="np-chip small" onClick={() => setFixing(true)}><IconEdit size={16} /> Fix</button>}
        </div>
      )}
      {fixing && lyrics && <FixLyrics p={p} text={lyrics.plain} lines={lines} source={lyrics.source} online={lyrics.online} onClose={() => setFixing(false)} onSaved={() => { setFixing(false); reload() }} />}
    </div>
  )
}

function NoLyrics({ p, online, canEdit, tried, failed }: { p: PlayerView; online: boolean; canEdit: boolean; tried: boolean; failed?: string }) {
  if (failed) return <p className="np-empty">The lyrics source couldn't be reached just now. It'll be tried again next time.</p>
  if (tried) return <p className="np-empty">No words found for this song online. {canEdit ? 'Fix lets you search under another name, or add them yourself.' : ''}</p>
  if (online) return <p className="np-empty">No words found for this song.</p>
  return (
    <div className="np-empty">
      <p>No words here. A <code>.lrc</code> file with the same name as the song, beside it, is used first.</p>
      {canEdit
        ? <p>Or find them online for free, if you agree: <button type="button" className="np-link" onClick={() => p.goTowards('/settings/metadata#lyrics')}>turn on online lyrics</button> in Settings. Your server sends only the artist, title, album and length.</p>
        : <p>Whoever runs this server can also turn on finding them online.</p>}
      <p><button type="button" className="np-link" onClick={() => p.goTowards('/help/lyrics-karaoke#if-lyrics-are-missing-or-wrong')}>How lyrics work</button></p>
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
    <div className="np-fix" role="dialog" aria-modal="true" aria-label={`Fix the lyrics of ${it.title}`} ref={ref}>
      <div className="np-side-head">
        <h3>Fix the lyrics</h3>
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label="Close"><IconClose /></button>
      </div>
      <div className="np-tabs" role="tablist">
        {online && <button type="button" role="tab" className={'np-tab' + (tab === 'search' ? ' on' : '')} aria-selected={tab === 'search'} onClick={() => setTab('search')}>Other versions</button>}
        <button type="button" role="tab" className={'np-tab' + (tab === 'edit' ? ' on' : '')} aria-selected={tab === 'edit'} onClick={() => setTab('edit')}>Edit</button>
        <button type="button" role="tab" className={'np-tab' + (tab === 'tap' ? ' on' : '')} aria-selected={tab === 'tap'} onClick={() => setTab('tap')}>Tap along</button>
      </div>
      {error && <p className="np-note" role="alert">{error}</p>}
      {tab === 'search' && (
        <div className="np-fix-body">
          <form className="np-fix-search" onSubmit={(e) => { e.preventDefault(); void run(async () => setFound(await api.post<Found[]>(`${url}/search`, { q }))) }}>
            <input className="input" aria-label="Search for lyrics" value={q} maxLength={300} onChange={(e) => setQ(e.target.value)} />
            <button className="btn sm primary" disabled={busy || !q.trim()}>Search</button>
          </form>
          {found?.length === 0 && <p className="np-hint">Nothing found. Try fewer words.</p>}
          <ul className="np-found">
            {found?.map((f) => {
              const off = f.durationSec && it.runtimeSec ? Math.abs(f.durationSec - it.runtimeSec) : null
              return (
                <li key={f.provider + f.id}>
                  <button type="button" disabled={busy} onClick={() => void run(async () => { await api.put(url, { provider: f.provider, id: f.id }); toast('Lyrics changed'); onSaved() })}>
                    <b>{f.title}</b><small>{[f.artist, f.album].filter(Boolean).join(' · ')}</small>
                    <small>{f.instrumental ? 'No words' : f.hasSynced ? 'Timed' : 'Words only'}{off !== null ? ` · ${off < 3 ? 'same length' : `${Math.round(off)} s different`}` : ''}</small>
                  </button>
                </li>
              )
            })}
          </ul>
        </div>
      )}
      {tab === 'edit' && (
        <div className="np-fix-body">
          <textarea className="input np-fix-text" aria-label="The lyrics" value={draft} spellCheck={false} onChange={(e) => setDraft(e.target.value)} placeholder={'[00:12.50]A line, with the time it starts\nor plain words with no times'} />
          <p className="np-hint">Kept on this server and used in place of everything else. Nothing is written beside your music.</p>
          <div className="np-fix-row">
            <button type="button" className="btn sm primary" disabled={busy || !draft.trim()} onClick={() => void run(async () => { await api.put(url, { text: draft }); toast('Lyrics saved'); onSaved() })}>Save</button>
            <button type="button" className="btn sm" disabled={busy} onClick={() => void run(async () => { await api.put(url, { instrumental: true }); toast('Marked as having no words'); onSaved() })}>This song has no words</button>
            {(source === 'override' || source === 'lrclib' || source === 'mirror' || source === 'musixmatch') && <button type="button" className="btn sm ghost" disabled={busy} onClick={() => void run(async () => { await api.del(url); toast('Back to what was found'); onSaved() })}><IconRefresh size={16} /> Start over</button>}
          </div>
        </div>
      )}
      {tab === 'tap' && <TapAlong p={p} text={lines.length ? lines.map((l) => l.text).join('\n') : text} busy={busy}
        onSave={(lrc) => void run(async () => { await api.put(url, { text: lrc }); toast('Timing saved'); onSaved() })} />}
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
  if (!lines.length) return <div className="np-fix-body"><p className="np-hint">Add the words under Edit first, then come back to time them.</p></div>
  return (
    <div className="np-fix-body np-tap">
      {!started ? (
        <>
          <p>The song starts from the beginning. Press the big button, or Space, as each line starts.</p>
          <button type="button" className="btn primary" onClick={start}>Start</button>
        </>
      ) : (
        <>
          <p className="np-tap-prev">{lines[at - 1] || ' '}</p>
          <p className="np-tap-now">{done ? 'That\'s all of them.' : lines[at]}</p>
          <p className="np-tap-next">{lines[at + 1] || ' '}</p>
          <button type="button" ref={button} className="btn primary np-tap-btn" disabled={done} onClick={tap}>This line starts now ({at + 1} of {lines.length})</button>
          <div className="np-fix-row">
            <button type="button" className="btn sm" disabled={!at} onClick={() => { const t = times[at - 2] ?? 0; setTimes(times.slice(0, -1)); p.seek(Math.max(0, t / 1000)); button.current?.focus() }}>Undo the last one</button>
            <button type="button" className="btn sm ghost" onClick={start}>Start again</button>
            <button type="button" className="btn sm primary" disabled={!done || busy} onClick={() => onSave(times.map((t, i) => `[${stamp(t)}]${lines[i]}`).join('\n') + '\n')}>Save</button>
          </div>
        </>
      )}
    </div>
  )
}
