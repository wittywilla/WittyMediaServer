import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { api, fmtTime, mediaUrl, posterUrl, type Item } from '../api'
import { IconClose, IconExpand, IconNext, IconPause, IconPlay, IconPrev } from '../icons'
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
  const books = mode !== 'music'

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
    stop: () => { audio.current?.pause(); setQueue([]); setFull(false) },
  }), [queue, index, current, mode, playing, next, prev])

  const dur = current?.runtimeSec || 0
  return (
    <Ctx.Provider value={ctx}>
      {children}
      <audio ref={audio} preload="auto" onPlay={() => setPlaying(true)} onPause={() => setPlaying(false)} onEnded={onEnded}
        onTimeUpdate={() => setTime(position())} />
      {current && !loc.pathname.startsWith('/play/') && (
        <div className={'audio-bar' + (full ? ' full' : '')} role="region" aria-label="Now playing">
          {full && <div className="ab-art-big" style={{ backgroundImage: current.poster ? `url(${posterUrl(current)})` : undefined }} />}
          <button className="ab-art" onClick={() => setFull(!full)} aria-label={full ? 'Smaller player' : 'Show the full player'}>
            {current.poster ? <img src={posterUrl(current)} alt="" /> : <span>♪</span>}
          </button>
          <div className="ab-info">
            <div className="ab-title">{current.title}</div>
            <div className="ab-sub">{[current.artist || current.albumArtist, current.album].filter(Boolean).join(' · ')}</div>
          </div>
          <div className="ab-ctl">
            {books && <button className="ibtn" onClick={() => seek(Math.max(0, position() - 15))} aria-label="Back 15 seconds">↺15</button>}
            <button className="ibtn" onClick={prev} aria-label="Previous"><IconPrev /></button>
            <button className="ibtn big" onClick={ctx.toggle} aria-label={playing ? 'Pause' : 'Play'}>{playing ? <IconPause /> : <IconPlay />}</button>
            <button className="ibtn" onClick={next} aria-label="Next"><IconNext /></button>
            {books && <button className="ibtn" onClick={() => seek(position() + 30)} aria-label="Forward 30 seconds">30↻</button>}
          </div>
          <div className="ab-seek">
            <span>{fmtTime(time)}</span>
            <input type="range" min={0} max={Math.max(1, dur)} step={1} value={Math.min(time, dur)} aria-label="Position" onChange={(e) => seek(Number(e.target.value))} />
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
            <button className="ibtn" onClick={() => setFull(!full)} aria-label={full ? 'Smaller player' : 'Full player'}><IconExpand /></button>
            <button className="ibtn" onClick={ctx.stop} aria-label="Stop and close"><IconClose /></button>
          </div>
          {full && (
            <ol className="ab-queue" aria-label="Queue">
              {queue.map((q, i) => (
                <li key={q.id + '-' + i} className={i === index ? 'on' : ''}>
                  <button onClick={() => setIndex(i)}><span className="muted">{q.track ?? i + 1}</span> {q.title}<span className="muted">{fmtTime(q.runtimeSec)}</span></button>
                </li>
              ))}
            </ol>
          )}
        </div>
      )}
    </Ctx.Provider>
  )
}
