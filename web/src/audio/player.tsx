import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, fmtTime, getPrefs, getToken, mediaUrl, posterUrl, serverBase, type Item } from '../api'
import { IconAudio, IconBack15, IconClose, IconDown, IconExpand, IconForward30, IconNext, IconPause, IconPlay, IconPrev, IconStop, IconVisualizer } from '../icons'
import { Placeholder } from '../components/common'
import { setSpeed, SPEEDS } from '../player/Controls'
import { toast } from '../state'
import { useCdEngine, type CdActions, type CdNow } from './cdEngine'
import { VisualizerView, visualizerWantedWithPlayer } from './visualizer'

// The audio player: music, audiobooks and podcasts. One <audio> element for
// the whole app, a queue, and a bar along the bottom that keeps playing
// while the listener browses. Audiobooks and podcasts remember where they
// stopped and offer speed and a sleep timer.

// Codecs every current browser plays as they are (keep in step with
// library.DirectAudio on the server); others arrive converted to AAC and are
// seeked by restarting the stream at the new position.
const DIRECT = new Set(['mp3', 'aac', 'flac', 'opus', 'vorbis', 'pcm_s16le', 'pcm_s24le', 'pcm_u8', 'pcm_f32le'])

const RETRY_WAITS = [2000, 5000, 10000] // between tries to reconnect to a station, in milliseconds
const STALL_MS = 15000 // a station that plays nothing for this long has gone

export type AudioMode = 'music' | 'audiobooks' | 'podcasts' | 'radio'
type Repeat = 'off' | 'all' | 'one'

type AudioCtx = {
  queue: Item[]
  index: number
  current: Item | null
  mode: AudioMode
  // The element playing, for what wants to listen to it: the visualizer.
  media: () => HTMLAudioElement | null
  playing: boolean
  nowPlaying: string // what a radio station says it is playing, if it says
  play: (items: Item[], start?: number, mode?: AudioMode) => void
  enqueue: (items: Item[]) => void
  toggle: () => void
  next: () => void
  prev: () => void
  stop: () => void
}

const Ctx = createContext<AudioCtx | null>(null)
export const useAudio = () => useContext(Ctx)!

// The CD player's view of things. A context of its own, as it changes several
// times a second and only the CD page and the bar care.
export type CdCtxValue = { now: CdNow | null; actions: CdActions; audioEl: HTMLAudioElement | null; playing: boolean; toggle: () => void }
const CdCtx = createContext<CdCtxValue | null>(null)
export const useCd = () => useContext(CdCtx)!

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
  const [maxSpeed, setMaxSpeed] = useState(4)
  const [sleepAt, setSleepAt] = useState<number | 'chapter' | null>(null)
  const [full, setFull] = useState(false)
  const [nowPlaying, setNowPlaying] = useState<{ id: number; title: string; artist: string; track: string } | null>(null)
  const hls = useRef<{ destroy: () => void } | null>(null) // an HLS station's player, loaded when one is played
  // A stream that was playing and dropped is tried again a few times, as a radio would.
  const [retrying, setRetrying] = useState(false)
  const wasPlaying = useRef(false)
  const retries = useRef(0)
  const retryTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const progressAt = useRef(0)
  // The visualizer: off until someone turns it on, and off again with each new page.
  const [viz, setViz] = useState<'off' | 'panel' | 'full'>('off')
  const [vizHere, setVizHere] = useState(true) // the owner can take the plugin off the server
  useEffect(() => { getPrefs().then((p) => setVizHere(p.visualizer !== false)).catch(() => {}) }, [])
  const mediaEl = useCallback(() => audio.current, [])
  const vizPlace = useCallback((p: 'panel' | 'mini' | 'full') => setViz(p === 'full' ? 'full' : 'panel'), [])
  const vizOff = useCallback(() => setViz('off'), [])
  const offset = useRef(0) // where a converted stream started
  const current = queue[index] || null
  const currentId = useRef<number | undefined>(undefined)
  currentId.current = current?.id
  const loc = useLocation()
  const nav = useNavigate()
  const books = mode === 'audiobooks' || mode === 'podcasts'
  const live = mode === 'radio' // a station: no seeking, no speed
  const listened = useRef(0) // seconds of this track, for Your year
  // A disc in the player: one stream, and tracks are places in it, so the queue
  // only mirrors it for the bar and the lock screen.
  const cdOn = useRef(false)
  const cd = useCdEngine(audio, {
    onDisc: (items) => {
      cdOn.current = !!items
      setFull(false)
      setMode('music')
      setQueue(items || [])
      setIndex(0)
    },
    onTrack: (i) => { if (i >= 0) setIndex(i) },
    onTime: (t) => setTime(t),
  })
  const lastAt = useRef(0)
  useEffect(() => { getPrefs().then((p) => setMaxSpeed(p.maxSpeed || 4)).catch(() => {}) }, [])
  // Each kind of listening keeps its own speed: 1.5× for audiobooks needn't
  // apply to the next album.
  const speedKey = 'wms.speed.' + mode
  const changeRate = (r: number) => {
    setRate(r)
    if (audio.current) setSpeed(audio.current, r)
    try { localStorage.setItem(speedKey, String(r)) } catch { /* private window */ }
  }
  useEffect(() => {
    let r = 1
    try { r = Number(localStorage.getItem(speedKey)) || 1 } catch { /* private window */ }
    setRate(r)
    if (audio.current) setSpeed(audio.current, r)
  }, [speedKey])

  // On phones the full player is a screen of its own: Back (the button, Esc,
  // or the phone's Back) returns to the page underneath. On wider screens it
  // is a panel beside the library, which stays usable, so browsing doesn't
  // close it.
  const docked = () => window.matchMedia('(min-width: 1000px)').matches
  const openFull = () => {
    if (cdOn.current && cd.now) { nav(`/cd/${cd.now.disc.id}`); return }
    setFull(true)
    if (!docked()) history.pushState({ ...(history.state || {}), wmsAudioFull: true }, '')
    // Opening the player is a press, so a saved choice to show it here can start now.
    if (viz === 'off') visualizerWantedWithPlayer().then((on) => { if (on) setViz('panel') })
  }
  const closeFull = () => { if (history.state?.wmsAudioFull) history.back(); else setFull(false) }
  useEffect(() => {
    if (!full) return
    const pop = () => setFull(false)
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.wv-dialog-back')) closeFull() }
    window.addEventListener('popstate', pop)
    window.addEventListener('keydown', esc)
    return () => { window.removeEventListener('popstate', pop); window.removeEventListener('keydown', esc) }
  }, [full]) // eslint-disable-line react-hooks/exhaustive-deps

  // Start (or switch) the track.
  useEffect(() => {
    const a = audio.current
    if (!a || !current || cdOn.current) return
    const resume = books && current.progress && !current.progress.watched ? current.progress.positionSec : 0
    load(current, resume)
    a.play().catch(() => setPlaying(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id])

  const direct = (it: Item) => DIRECT.has(it.videoCodec)
  const load = (it: Item, at: number) => {
    const a = audio.current!
    hls.current?.destroy()
    hls.current = null
    if (it.kind === 'radio') {
      offset.current = 0
      const src = mediaUrl(`/api/radio/${it.id}/stream`)
      if (!it.radio?.hls || a.canPlayType('application/vnd.apple.mpegurl')) {
        a.src = src
        return
      }
      // Most browsers need help with HLS. It's a large part of the app, so it comes
      // only when such a station is played.
      a.removeAttribute('src')
      const id = it.id
      import('hls.js').then(({ default: Hls }) => {
        if (audio.current !== a || currentId.current !== id) return
        if (!Hls.isSupported()) { toast(`${it.title} can't be played in this browser`); return }
        const h = new Hls({
          xhrSetup: (xhr) => { const t = getToken(); if (t) xhr.setRequestHeader('Authorization', 'Bearer ' + t); xhr.withCredentials = !serverBase() },
          maxBufferLength: 30, backBufferLength: 10, lowLatencyMode: false,
        })
        h.on(Hls.Events.ERROR, (_e, data) => {
          if (!data.fatal) return
          h.destroy()
          if (hls.current === h) hls.current = null
          setPlaying(false)
          toast(`${it.title} can't be reached`)
        })
        h.loadSource(src)
        h.attachMedia(a)
        hls.current = h
        a.play().catch(() => setPlaying(false))
      }).catch(() => toast(`${it.title} can't be played right now`))
      return
    }
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
    if (cdOn.current) { cd.actions.seekInTrack(t); return }
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

  // Music has no place to save, but what was played counts for Your year.
  // Stations are counted by the server, which relays them.
  useEffect(() => {
    listened.current = 0; lastAt.current = 0
    const id = current?.id
    if (!id || books || live) return
    return () => {
      if (listened.current >= 5) api.post(`/items/${id}/view`, { seconds: listened.current }).catch(() => {})
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id, books, live])

  // What a station is playing: the server reads it from the stream it is relaying,
  // so asking costs nothing elsewhere, and isn't asked while the page is hidden.
  useEffect(() => {
    setNowPlaying(null)
    const id = current?.id
    if (!id || !live || !playing) return
    let stopped = false
    const ask = () => {
      if (document.hidden) return
      api.get<{ title: string; artist?: string; track?: string }>(`/radio/${id}/now`)
        .then((n) => { if (!stopped) setNowPlaying(n.title ? { id, title: n.title, artist: n.artist || '', track: n.track || '' } : null) })
        .catch(() => {})
    }
    const first = setTimeout(ask, 2500) // the stream's first title takes a moment
    const t = setInterval(ask, 20000)
    document.addEventListener('visibilitychange', ask)
    return () => { stopped = true; clearTimeout(first); clearInterval(t); document.removeEventListener('visibilitychange', ask) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id, live, playing])

  // A live stream that's paused would keep the server fetching a station for nobody,
  // so hang up; playing again connects again.
  const hangUp = () => {
    const a = audio.current
    clearTimeout(retryTimer.current)
    setRetrying(false)
    if (!a) return
    hls.current?.destroy()
    hls.current = null
    a.removeAttribute('src')
    a.load()
  }
  const reconnect = () => {
    if (!current) return
    const it = current
    hangUp()
    setPlaying(false)
    if (retries.current >= RETRY_WAITS.length) {
      wasPlaying.current = false
      toast(`${it.title} stopped, and can't be reached`)
      return
    }
    setRetrying(true)
    retryTimer.current = setTimeout(() => {
      setRetrying(false)
      load(it, 0)
      audio.current?.play().catch(() => {})
    }, RETRY_WAITS[retries.current++])
  }
  useEffect(() => {
    retries.current = 0
    wasPlaying.current = false
    return () => { clearTimeout(retryTimer.current); setRetrying(false) }
  }, [current?.id])
  // A browser reports a stream that goes silent as nothing at all: no error, no end,
  // just still "playing". So watch for the sound not moving on.
  useEffect(() => {
    if (!live || !playing) return
    progressAt.current = Date.now()
    const t = setInterval(() => { if (wasPlaying.current && Date.now() - progressAt.current > STALL_MS) reconnect() }, 5000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id, live, playing])
  const resume = () => {
    const a = audio.current
    clearTimeout(retryTimer.current)
    setRetrying(false)
    if (!a) return
    if (live && current && !a.getAttribute('src') && !hls.current) {
      load(current, 0)
      if (!current.radio?.hls) a.play().catch(() => {})
      return
    }
    a.play().catch(() => {})
  }

  // A video takes over the speakers: pause the music.
  useEffect(() => { if (loc.pathname.startsWith('/play/')) audio.current?.pause() }, [loc.pathname])

  // Sleep timer.
  useEffect(() => {
    if (typeof sleepAt !== 'number') return
    const t = setTimeout(() => { audio.current?.pause(); setSleepAt(null) }, Math.max(0, sleepAt - Date.now()))
    return () => clearTimeout(t)
  }, [sleepAt])

  const next = useCallback(() => {
    if (cdOn.current) { cd.actions.next(); return }
    if (!queue.length) return
    if (shuffle && queue.length > 1) { let n = index; while (n === index) n = Math.floor(Math.random() * queue.length); setIndex(n); return }
    if (index + 1 < queue.length) setIndex(index + 1)
    else if (repeat === 'all') setIndex(0)
    else { audio.current?.pause() }
  }, [queue.length, index, shuffle, repeat])
  const prev = useCallback(() => {
    if (cdOn.current) { cd.actions.prev(); return }
    if (position() > 4 || index === 0) { seek(0); return }
    setIndex(index - 1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [index, current])
  const onEnded = () => {
    if (cdOn.current) { cd.onEnded(); return }
    if (live) { setPlaying(false); return }
    if (current && books) api.post(`/items/${current.id}/progress`, { positionSec: current.runtimeSec, durationSec: current.runtimeSec }).catch(() => {})
    if (sleepAt === 'chapter') { setSleepAt(null); return }
    if (repeat === 'one') { seek(0); audio.current?.play().catch(() => {}); return }
    next()
  }

  // Lock screen, headphones and keyboard media keys.
  useEffect(() => {
    const ms = navigator.mediaSession
    if (!ms || !current) return
    const song = live && nowPlaying?.id === current.id ? nowPlaying : null
    ms.metadata = new MediaMetadata({ title: song ? song.track || song.title : current.title, artist: song ? song.artist || current.title : current.artist || current.albumArtist || '', album: live ? (song ? current.title : '') : current.album || '',
      artwork: current.poster ? [{ src: new URL(posterUrl(current), location.href).href, sizes: '600x600' }] : [] })
    const h: [MediaSessionAction, MediaSessionActionHandler | null][] = [
      ['play', resume], ['pause', () => audio.current?.pause()],
      ['nexttrack', next], ['previoustrack', prev],
      ['seekto', live ? null : (d) => { if (d.seekTime != null) seek(d.seekTime) }],
      ['seekforward', live ? null : () => seek(position() + 30)], ['seekbackward', live ? null : () => seek(Math.max(0, position() - 15))],
    ]
    for (const [a, f] of h) { try { ms.setActionHandler(a, f) } catch { /* unsupported action */ } }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current, next, prev, nowPlaying, live])

  // V for the visualizer, wherever the music is playing from.
  useEffect(() => {
    if (!current) return
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName
      if (!vizHere || e.key.toLowerCase() !== 'v' || e.ctrlKey || e.metaKey || e.altKey || tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || loc.pathname.startsWith('/play/')) return
      e.preventDefault()
      setViz(viz === 'off' ? 'full' : 'off')
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [current?.id, viz, vizHere, loc.pathname]) // eslint-disable-line react-hooks/exhaustive-deps
  // A video takes over: nothing should be drawing behind it.
  useEffect(() => { if (loc.pathname.startsWith('/play/')) setViz('off') }, [loc.pathname])

  // "<" and ">" step through the speeds, as in the video player.
  useEffect(() => {
    if (!current) return
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.key !== '<' && e.key !== '>') || document.querySelector('.player:not(.mini):not(.inline), .player.inline')) return
      const list = SPEEDS.filter((s) => s <= maxSpeed)
      const i = list.indexOf(rate)
      const next = list[Math.max(0, Math.min(list.length - 1, (i < 0 ? list.indexOf(1) : i) + (e.key === '>' ? 1 : -1)))]
      if (next) changeRate(next)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const ctx = useMemo<AudioCtx>(() => ({
    queue, index, current, mode, playing, media: () => audio.current, nowPlaying: nowPlaying?.id === current?.id ? nowPlaying?.title || '' : '',
    play: (items, start = 0, m = 'music') => { if (cdOn.current) cd.actions.eject(); setMode(m); setQueue(items); setIndex(start); if (items[start]?.id === current?.id) resume() },
    enqueue: (items) => { if (cdOn.current) { cd.actions.eject(); setQueue(items); setIndex(0); return } setQueue((q) => [...q, ...items]) },
    toggle: () => { const a = audio.current; if (a) a.paused ? resume() : a.pause() },
    next, prev,
    stop: () => { if (cdOn.current) cd.actions.eject(); audio.current?.pause(); hangUp(); setQueue([]); if (history.state?.wmsAudioFull) history.back(); setFull(false) },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [queue, index, current, mode, playing, next, prev, nowPlaying])

  const cdValue = useMemo<CdCtxValue>(() => ({ now: cd.now, actions: cd.actions, audioEl: audio.current, playing, toggle: ctx.toggle }), [cd.now, cd.actions, playing, ctx.toggle])
  const cdActive = cdOn.current && !!cd.now

  const dur = current?.runtimeSec || 0
  // The album (or audiobook, or podcast) the track belongs to.
  const albumUrl = live && current ? `/radio/${current.id}` : current?.album ? `/album/${current.libraryId}?artist=${encodeURIComponent(current.albumArtist || current.artist || '')}&album=${encodeURIComponent(current.album)}` : ''
  const goAlbum = () => {
    if (docked()) { nav(albumUrl); return } // the panel stays open beside it
    const wasFull = !!history.state?.wmsAudioFull
    setFull(false)
    nav(albumUrl, { replace: wasFull })
  }
  return (
    <Ctx.Provider value={ctx}>
      <CdCtx.Provider value={cdValue}>{children}</CdCtx.Provider>
      <audio ref={audio} preload="auto" onPlay={() => { setPlaying(true); if (cdOn.current) cd.onPlay() }} onPlaying={() => { if (live) wasPlaying.current = true }}
        onPause={() => {
          setPlaying(false)
          if (cdOn.current) cd.onPause()
          if (!live) return
          // At the end of a stream the browser pauses too: that's a dropped connection.
          if (audio.current?.ended && wasPlaying.current) reconnect()
          else hangUp()
        }} onEnded={onEnded}
        onWaiting={() => { if (cdOn.current) cd.onWaiting() }}
        onEmptied={() => { if (cdOn.current) setPlaying(!audio.current?.paused) }}
        onError={() => {
          if (cdOn.current) { cd.onError(); return }
          if (!live || !current || !audio.current?.getAttribute('src')) return
          if (wasPlaying.current) { reconnect(); return }
          const fail = () => { setPlaying(false); toast(`${current.title} can't be reached`) }
          if (current.radio?.hls) { fail(); return }
          // A station added by its address may have turned out to be HLS just now.
          api.get<Item>(`/radio/${current.id}`).then((r) => {
            if (!r.radio?.hls) { fail(); return }
            const it = { ...current, radio: r.radio }
            setQueue((q) => q.map((x) => (x.id === it.id ? it : x)))
            load(it, 0)
          }).catch(fail)
        }}
        onTimeUpdate={() => {
          if (cdOn.current) {
            const p = position(), d = p - lastAt.current
            if (d > 0 && d < 5) listened.current += d
            lastAt.current = p
            cd.onTimeUpdate()
            return
          }
          const p = position(), d = p - lastAt.current
          if (d > 0 && d < 5) listened.current += d
          if (d > 0) progressAt.current = Date.now()
          if (live && p > 30) retries.current = 0 // it has been steady for a while
          lastAt.current = p
          setTime(p)
        }} />
      {current && !loc.pathname.startsWith('/play/') && !(cdActive && loc.pathname === `/cd/${cd.now!.disc.id}`) && (
        <div className={'audio-bar' + (full ? ' full' : '')} role="region" aria-label="Now playing">
          {full && (
            <div className="ab-top">
              <button className="btn ghost sm ab-back" onClick={closeFull} title="Close (Esc)">{docked() ? <><IconClose /> Close</> : <><IconDown /> Back</>}</button>
              <span className="ab-top-title">Now playing</span>
              {albumUrl ? <button className="btn ghost sm" onClick={goAlbum}>{live ? 'Go to station' : books ? 'Go to book' : 'Go to album'}</button> : <span />}
            </div>
          )}
          {full && (viz === 'panel' || viz === 'full'
            ? <div className="ab-art-big ab-viz"><VisualizerView media={mediaEl} place={viz} onPlace={vizPlace} onClose={vizOff} /></div>
            : <div className="ab-art-big" style={{ backgroundImage: current.poster ? `url(${posterUrl(current)})` : undefined, backgroundSize: live ? 'contain' : undefined, backgroundRepeat: 'no-repeat' }}>{live && !current.poster && <Placeholder title={current.title} />}</div>)}
          <button className={'ab-art' + (cdActive ? ' cd' + (playing ? ' spin' : '') : '')} onClick={full ? closeFull : openFull} aria-label={full ? 'Back' : 'Show the full player'} title="Show the full player">
            {viz === 'panel' && !full
              ? <VisualizerView media={mediaEl} place="mini" onClose={vizOff} />
              : current.poster ? <img src={posterUrl(current)} alt="" style={live ? { objectFit: 'contain' } : undefined} onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} /> : live ? <Placeholder title={current.title} /> : <IconAudio />}
          </button>
          {viz === 'full' && !full && <VisualizerView media={mediaEl} place="full" onPlace={vizPlace} onClose={vizOff} />}
          {full ? (
            <div className="ab-info">
              <div className="ab-title">{current.title}</div>
              <div className="ab-sub">{live ? (retrying ? 'Reconnecting…' : ctx.nowPlaying || 'Live') : [current.artist || current.albumArtist, current.album].filter(Boolean).join(' · ')}</div>
            </div>
          ) : (
            <button className="ab-info" onClick={openFull} title="Show the full player">
              <div className="ab-title">{current.title}</div>
              <div className="ab-sub">{live ? (retrying ? 'Reconnecting…' : ctx.nowPlaying || 'Live') : [current.artist || current.albumArtist, current.album].filter(Boolean).join(' · ')}</div>
            </button>
          )}
          <div className="ab-ctl">
            {books && <button className="ibtn" onClick={() => seek(Math.max(0, position() - 15))} aria-label="Back 15 seconds"><IconBack15 /></button>}
            <button className="ibtn" onClick={prev} aria-label="Previous"><IconPrev /></button>
            <button className="ibtn big" onClick={ctx.toggle} aria-label={playing ? 'Pause' : 'Play'}>{playing ? <IconPause /> : <IconPlay />}</button>
            <button className="ibtn" onClick={next} aria-label="Next"><IconNext /></button>
            {books && <button className="ibtn" onClick={() => seek(position() + 30)} aria-label="Forward 30 seconds"><IconForward30 /></button>}
          </div>
          {!live && <div className="ab-seek">
            <span>{fmtTime(time)}</span>
            <input className="range" style={{ '--pct': (dur > 0 ? Math.min(100, (time / dur) * 100) : 0) + '%' } as React.CSSProperties} type="range" min={0} max={Math.max(1, dur)} step={1} value={Math.min(time, dur)} aria-label="Position" onChange={(e) => seek(Number(e.target.value))} />
            <span>{fmtTime(dur)}</span>
          </div>}
          <div className="ab-extra">
            {cdActive
              ? <button className={'btn sm ' + (cd.now!.shuffle ? 'primary' : 'ghost')} onClick={() => cd.actions.setShuffle(!cd.now!.shuffle)} aria-pressed={cd.now!.shuffle}>Shuffle</button>
              : !books && !live && <button className={'btn sm ' + (shuffle ? 'primary' : 'ghost')} onClick={() => setShuffle(!shuffle)} aria-pressed={shuffle}>Shuffle</button>}
            {cdActive
              ? <button className={'btn sm ' + (cd.now!.repeat !== 'off' ? 'primary' : 'ghost')} onClick={() => cd.actions.setRepeat(cd.now!.repeat === 'off' ? 'all' : cd.now!.repeat === 'all' ? 'one' : 'off')}>Repeat{cd.now!.repeat === 'one' ? ' 1' : ''}</button>
              : !books && !live && <button className={'btn sm ' + (repeat !== 'off' ? 'primary' : 'ghost')} onClick={() => setRepeat(repeat === 'off' ? 'all' : repeat === 'all' ? 'one' : 'off')}>Repeat{repeat === 'one' ? ' 1' : ''}</button>}
            {!live && !cdActive && <select className="select sm" aria-label="Playback speed" title="Playback speed (< and >)" value={rate} onChange={(e) => changeRate(Number(e.target.value))}>
              {SPEEDS.filter((r) => r <= maxSpeed).map((r) => <option key={r} value={r}>{r}×</option>)}
            </select>}
            {(books || live) && (
              <select className="select sm" aria-label="Sleep timer" value={sleepAt === null ? '' : sleepAt === 'chapter' ? 'chapter' : 'set'}
                onChange={(e) => { const v = e.target.value; setSleepAt(v === '' ? null : v === 'chapter' ? 'chapter' : v === 'set' ? sleepAt : Date.now() + Number(v) * 60000) }}>
                <option value="">Sleep: off</option>
                {sleepAt !== null && sleepAt !== 'chapter' && <option value="set">Sleep at {new Date(sleepAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</option>}
                <option value="15">In 15 min</option><option value="30">In 30 min</option><option value="45">In 45 min</option><option value="60">In 1 hour</option>
                {books && <option value="chapter">End of chapter</option>}
              </select>
            )}
            <span className="muted ab-count">{index + 1} / {queue.length}</span>
            {vizHere && <button className={'ibtn ab-viz-btn' + (viz !== 'off' ? ' on' : '')} aria-pressed={viz !== 'off'} aria-label="Visualizer" title="Visualizer (V)" onClick={() => setViz(viz !== 'off' ? 'off' : full ? 'panel' : 'full')}><IconVisualizer /></button>}
            {!full && <button className="ibtn" onClick={openFull} aria-label="Full player" title="Full player"><IconExpand /></button>}
            {full
              ? <button className="btn sm ghost" onClick={ctx.stop} title="Stop playing and close the player"><IconStop /> Stop</button>
              : <button className="ibtn" onClick={ctx.stop} aria-label="Stop playing" title="Stop playing"><IconClose /></button>}
          </div>
          {full && (
            <section className="ab-queue-wrap" aria-label="Queue">
              <div className="ab-queue-head"><span>{books ? 'Chapters' : live ? 'Stations' : 'Up next'}</span><span className="muted">{queue.length} {live ? (queue.length === 1 ? 'station' : 'stations') : queue.length === 1 ? 'track' : 'tracks'}</span></div>
              <ol className="ab-queue">
                {queue.map((q, i) => (
                  <li key={q.id + '-' + i} className={i === index ? 'on' : ''}>
                    <button onClick={() => setIndex(i)} aria-current={i === index ? 'true' : undefined}>
                      <span className="ab-q-n">{i === index && playing ? <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i> : (q.track ?? i + 1)}</span>
                      <span className="ab-q-t"><b>{q.title}</b>{q.artist && q.artist !== current.artist && <small>{q.artist}</small>}</span>
                      {!live && <span className="ab-q-d">{fmtTime(q.runtimeSec)}</span>}
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
