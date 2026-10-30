import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, fmtTime, mediaUrl, posterUrl, type Item } from '../api'
import { IconAudio, IconBack15, IconClose, IconDown, IconExpand, IconForward30, IconNext, IconPause, IconPlay, IconPrev, IconStop } from '../icons'
import { setSpeed } from '../player/Controls'

// The audio player: music, audiobooks and podcasts. One <audio> element for
// the whole app, a queue, and a bar along the bottom that keeps playing
// while the listener browses. Audiobooks and podcasts remember where they
// stopped and offer speed and a sleep timer.

// Codecs every current browser plays as they are (keep in step with
// library.DirectAudio on the server); others arrive converted to AAC and are
// seeked by restarting the stream at the new position.
const DIRECT = new Set(['mp3', 'aac', 'flac', 'opus', 'vorbis', 'pcm_s16le', 'pcm_s24le', 'pcm_u8', 'pcm_f32le'])

export type AudioMode = 'music' | 'audiobooks' | 'podcasts'
type Repeat = 'off' | 'all' | 'one'

type AudioCtx = {
  queue: Item[]
  index: number
  current: Item | null
  mode: AudioMode
  playing: boolean
  play: (items: Item[], start?: number, mode?: AudioMode) => void
  enqueue: (items: Item[]) => void
  toggle: () => void
  next: () => void
  prev: () => void
  stop: () => void
}

const Ctx = createContext<AudioCtx | null>(null)
export const useAudio = () => useContext(Ctx)!

export function AudioProvider({ children }: { children: React.ReactNode }) {
  const audio = useRef<HTMLAudioElement | null>(null)
  const [queue, setQueue] = useState<Item[]>([])
  const [index, setIndex] = useState(0)
  const [mode, setMode] = useState<AudioMode>('music')
  const [playing, setPlaying] = useState(false)
  const [shuffle, setShuffle] = useState(false)
  const [repeat, setRepeat] = useState<Repeat>('off')
  const [time, setTime] = useState(0)
  const [rate, setRate] = useState(1)
  const [sleepAt, setSleepAt] = useState<number | 'chapter' | null>(null)
  const [full, setFull] = useState(false)
  const offset = useRef(0) // where a converted stream started
  const current = queue[index] || null
  const loc = useLocation()
  const nav = useNavigate()
  const books = mode !== 'music'

  // On phones the full player is a screen of its own: Back (the button, Esc,
  // or the phone's Back) returns to the page underneath. On wider screens it
  // is a panel beside the library, which stays usable, so browsing doesn't
  // close it.
  const docked = () => window.matchMedia('(min-width: 1000px)').matches
  const openFull = () => { setFull(true); if (!docked()) history.pushState({ ...(history.state || {}), wmsAudioFull: true }, '') }
  const closeFull = () => { if (history.state?.wmsAudioFull) history.back(); else setFull(false) }
  useEffect(() => {
    if (!full) return
    const pop = () => setFull(false)
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') closeFull() }
    window.addEventListener('popstate', pop)
    window.addEventListener('keydown', esc)
    return () => { window.removeEventListener('popstate', pop); window.removeEventListener('keydown', esc) }
  }, [full]) // eslint-disable-line react-hooks/exhaustive-deps

  // Start (or switch) the track.
  useEffect(() => {
    const a = audio.current
    if (!a || !current) return
    const resume = books && current.progress && !current.progress.watched ? current.progress.positionSec : 0
    load(current, resume)
    a.play().catch(() => setPlaying(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id])

  const direct = (it: Item) => DIRECT.has(it.videoCodec)
  const load = (it: Item, at: number) => {
    const a = audio.current!
    if (direct(it)) {
      offset.current = 0
      a.src = mediaUrl(`/api/items/${it.id}/stream`)
      if (at > 0) a.addEventListener('loadedmetadata', () => { a.currentTime = at }, { once: true })
    } else {
      offset.current = at
      a.src = mediaUrl(`/api/items/${it.id}/stream${at > 0 ? `?start=${at.toFixed(1)}` : ''}`)
    }
    setSpeed(a, rate)
  }
  const position = () => (audio.current ? audio.current.currentTime + offset.current : 0)
  const seek = (t: number) => {
    const a = audio.current
    if (!a || !current) return
    if (direct(current)) a.currentTime = t
    else { load(current, t); a.play().catch(() => {}) }
  }

  // Save the place in audiobooks and podcasts.
  useEffect(() => {
    if (!books || !current) return
    const save = () => {
      const p = position()
      if (p > 5) api.post(`/items/${current.id}/progress`, { positionSec: p, durationSec: current.runtimeSec }).catch(() => {})
    }
    const t = setInterval(save, 15000)
    return () => { clearInterval(t); save() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id, books])

  // A video takes over the speakers: pause the music.
  useEffect(() => { if (loc.pathname.startsWith('/play/')) audio.current?.pause() }, [loc.pathname])

  // Sleep timer.
  useEffect(() => {
    if (typeof sleepAt !== 'number') return
    const t = setTimeout(() => { audio.current?.pause(); setSleepAt(null) }, Math.max(0, sleepAt - Date.now()))
    return () => clearTimeout(t)
  }, [sleepAt])

  const next = useCallback(() => {
    if (!queue.length) return
    if (shuffle && queue.length > 1) { let n = index; while (n === index) n = Math.floor(Math.random() * queue.length); setIndex(n); return }
    if (index + 1 < queue.length) setIndex(index + 1)
    else if (repeat === 'all') setIndex(0)
    else { audio.current?.pause() }
  }, [queue.length, index, shuffle, repeat])
  const prev = useCallback(() => {
    if (position() > 4 || index === 0) { seek(0); return }
    setIndex(index - 1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [index, current])
  const onEnded = () => {
    if (current && books) api.post(`/items/${current.id}/progress`, { positionSec: current.runtimeSec, durationSec: current.runtimeSec }).catch(() => {})
    if (sleepAt === 'chapter') { setSleepAt(null); return }
    if (repeat === 'one') { seek(0); audio.current?.play().catch(() => {}); return }
    next()
  }

  // Lock screen, headphones and keyboard media keys.
  useEffect(() => {
    const ms = navigator.mediaSession
    if (!ms || !current) return
    ms.metadata = new MediaMetadata({ title: current.title, artist: current.artist || current.albumArtist || '', album: current.album || '',
      artwork: current.poster ? [{ src: new URL(posterUrl(current), location.href).href, sizes: '600x600' }] : [] })
    const h: [MediaSessionAction, MediaSessionActionHandler][] = [
      ['play', () => audio.current?.play()], ['pause', () => audio.current?.pause()],
      ['nexttrack', next], ['previoustrack', prev],
      ['seekto', (d) => { if (d.seekTime != null) seek(d.seekTime) }],
      ['seekforward', () => seek(position() + 30)], ['seekbackward', () => seek(Math.max(0, position() - 15))],
    ]
    for (const [a, f] of h) { try { ms.setActionHandler(a, f) } catch { /* unsupported action */ } }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current, next, prev])

  const ctx = useMemo<AudioCtx>(() => ({
    queue, index, current, mode, playing,
    play: (items, start = 0, m = 'music') => { setMode(m); setQueue(items); setIndex(start); if (items[start]?.id === current?.id) audio.current?.play() },
    enqueue: (items) => setQueue((q) => [...q, ...items]),
    toggle: () => { const a = audio.current; if (a) a.paused ? a.play().catch(() => {}) : a.pause() },
    next, prev,
    stop: () => { audio.current?.pause(); setQueue([]); if (history.state?.wmsAudioFull) history.back(); setFull(false) },
  }), [queue, index, current, mode, playing, next, prev])

  const dur = current?.runtimeSec || 0
  // The album (or audiobook, or podcast) the track belongs to.
  const albumUrl = current?.album ? `/album/${current.libraryId}?artist=${encodeURIComponent(current.albumArtist || current.artist || '')}&album=${encodeURIComponent(current.album)}` : ''
  const goAlbum = () => {
    if (docked()) { nav(albumUrl); return } // the panel stays open beside it
    const wasFull = !!history.state?.wmsAudioFull
    setFull(false)
    nav(albumUrl, { replace: wasFull })
  }
  return (
    <Ctx.Provider value={ctx}>
      {children}
      <audio ref={audio} preload="auto" onPlay={() => setPlaying(true)} onPause={() => setPlaying(false)} onEnded={onEnded}
        onTimeUpdate={() => setTime(position())} />
      {current && !loc.pathname.startsWith('/play/') && (
        <div className={'audio-bar' + (full ? ' full' : '')} role="region" aria-label="Now playing">
          {full && (
            <div className="ab-top">
              <button className="btn ghost sm ab-back" onClick={closeFull} title="Close (Esc)">{docked() ? <><IconClose /> Close</> : <><IconDown /> Back</>}</button>
              <span className="ab-top-title">Now playing</span>
              {albumUrl ? <button className="btn ghost sm" onClick={goAlbum}>{books ? 'Go to book' : 'Go to album'}</button> : <span />}
            </div>
          )}
          {full && <div className="ab-art-big" style={{ backgroundImage: current.poster ? `url(${posterUrl(current)})` : undefined }} />}
          <button className="ab-art" onClick={full ? closeFull : openFull} aria-label={full ? 'Back' : 'Show the full player'} title="Show the full player">
            {current.poster ? <img src={posterUrl(current)} alt="" /> : <IconAudio />}
          </button>
          {full ? (
            <div className="ab-info">
              <div className="ab-title">{current.title}</div>
              <div className="ab-sub">{[current.artist || current.albumArtist, current.album].filter(Boolean).join(' · ')}</div>
            </div>
          ) : (
            <button className="ab-info" onClick={openFull} title="Show the full player">
              <div className="ab-title">{current.title}</div>
              <div className="ab-sub">{[current.artist || current.albumArtist, current.album].filter(Boolean).join(' · ')}</div>
            </button>
          )}
          <div className="ab-ctl">
            {books && <button className="ibtn" onClick={() => seek(Math.max(0, position() - 15))} aria-label="Back 15 seconds"><IconBack15 /></button>}
            <button className="ibtn" onClick={prev} aria-label="Previous"><IconPrev /></button>
            <button className="ibtn big" onClick={ctx.toggle} aria-label={playing ? 'Pause' : 'Play'}>{playing ? <IconPause /> : <IconPlay />}</button>
            <button className="ibtn" onClick={next} aria-label="Next"><IconNext /></button>
            {books && <button className="ibtn" onClick={() => seek(position() + 30)} aria-label="Forward 30 seconds"><IconForward30 /></button>}
          </div>
          <div className="ab-seek">
            <span>{fmtTime(time)}</span>
            <input className="range" style={{ '--pct': (dur > 0 ? Math.min(100, (time / dur) * 100) : 0) + '%' } as React.CSSProperties} type="range" min={0} max={Math.max(1, dur)} step={1} value={Math.min(time, dur)} aria-label="Position" onChange={(e) => seek(Number(e.target.value))} />
            <span>{fmtTime(dur)}</span>
          </div>
          <div className="ab-extra">
            {!books && <button className={'btn sm ' + (shuffle ? 'primary' : 'ghost')} onClick={() => setShuffle(!shuffle)} aria-pressed={shuffle}>Shuffle</button>}
            {!books && <button className={'btn sm ' + (repeat !== 'off' ? 'primary' : 'ghost')} onClick={() => setRepeat(repeat === 'off' ? 'all' : repeat === 'all' ? 'one' : 'off')}>Repeat{repeat === 'one' ? ' 1' : ''}</button>}
            {books && (
              <select className="select sm" aria-label="Speed" value={rate} onChange={(e) => { const r = Number(e.target.value); setRate(r); if (audio.current) setSpeed(audio.current, r) }}>
                {[0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3].map((r) => <option key={r} value={r}>{r}×</option>)}
              </select>
            )}
            {books && (
              <select className="select sm" aria-label="Sleep timer" value={sleepAt === null ? '' : sleepAt === 'chapter' ? 'chapter' : 'set'}
                onChange={(e) => { const v = e.target.value; setSleepAt(v === '' ? null : v === 'chapter' ? 'chapter' : v === 'set' ? sleepAt : Date.now() + Number(v) * 60000) }}>
                <option value="">Sleep: off</option>
                {sleepAt !== null && sleepAt !== 'chapter' && <option value="set">Sleep at {new Date(sleepAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</option>}
                <option value="15">In 15 min</option><option value="30">In 30 min</option><option value="45">In 45 min</option><option value="60">In 1 hour</option>
                <option value="chapter">End of chapter</option>
              </select>
            )}
            <span className="muted ab-count">{index + 1} / {queue.length}</span>
            {!full && <button className="ibtn" onClick={openFull} aria-label="Full player" title="Full player"><IconExpand /></button>}
            {full
              ? <button className="btn sm ghost" onClick={ctx.stop} title="Stop playing and close the player"><IconStop /> Stop</button>
              : <button className="ibtn" onClick={ctx.stop} aria-label="Stop playing" title="Stop playing"><IconClose /></button>}
          </div>
          {full && (
            <section className="ab-queue-wrap" aria-label="Queue">
              <div className="ab-queue-head"><span>{books ? 'Chapters' : 'Up next'}</span><span className="muted">{queue.length} {queue.length === 1 ? 'track' : 'tracks'}</span></div>
              <ol className="ab-queue">
                {queue.map((q, i) => (
                  <li key={q.id + '-' + i} className={i === index ? 'on' : ''}>
                    <button onClick={() => setIndex(i)} aria-current={i === index ? 'true' : undefined}>
                      <span className="ab-q-n">{i === index && playing ? <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i> : (q.track ?? i + 1)}</span>
                      <span className="ab-q-t"><b>{q.title}</b>{q.artist && q.artist !== current.artist && <small>{q.artist}</small>}</span>
                      <span className="ab-q-d">{fmtTime(q.runtimeSec)}</span>
                    </button>
                  </li>
                ))}
              </ol>
            </section>
          )}
        </div>
      )}
    </Ctx.Provider>
  )
}
