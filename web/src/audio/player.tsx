import { createContext, lazy, Suspense, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, getPrefs, getToken, mediaUrl, posterUrl, serverBase, type Item } from '../api'
import { reportMediaError } from '../errorReport'
import { setSpeed, SPEEDS } from '../player/Controls'
import { toast } from '../state'
import { useCdEngine, type CdActions, type CdNow } from './cdEngine'
import MiniPlayer from './MiniPlayer'
import { canRoute, loadSound, openGraph, saveSound, soundGraph, type SoundSettings } from './sound'
import type { AudioMode, PlayerView, Repeat } from './types'

export type { AudioMode } from './types'

// The audio player: music, audiobooks and podcasts. A queue, and a bar along the bottom
// that keeps playing while the listener browses. Two <audio> elements take turns, so one
// song can run into the next with no gap, or fade across it; one element would have to
// stop to change. Audiobooks and podcasts remember where they stopped and offer speed. The
// bar is here; the full-screen player is loaded the first time it's opened.

const NowPlaying = lazy(() => import('./NowPlaying'))

// Codecs every current browser plays as they are (keep in step with
// library.DirectAudio on the server); others arrive converted to AAC and are
// seeked by restarting the stream at the new position.
const DIRECT = new Set(['mp3', 'aac', 'flac', 'opus', 'vorbis', 'pcm_s16le', 'pcm_s24le', 'pcm_u8', 'pcm_f32le'])

const RETRY_WAITS = [2000, 5000, 10000] // between tries to reconnect to a station, in milliseconds
const STALL_MS = 15000 // a station that plays nothing for this long has gone
const COLLAPSE_KEY = 'wms.audio.collapsed'

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
  playNext: (items: Item[]) => void
  toggle: () => void
  next: () => void
  prev: () => void
  stop: () => void
  liked: (id: number) => boolean
  toggleLike: (id: number) => void
  startRadio: (it: Item) => void
}

const Ctx = createContext<AudioCtx | null>(null)
export const useAudio = () => useContext(Ctx)!

// The CD player's view of things. A context of its own, as it changes several
// times a second and only the CD page and the bar care.
export type CdCtxValue = { now: CdNow | null; actions: CdActions; audioEl: HTMLAudioElement | null; playing: boolean; toggle: () => void }
const CdCtx = createContext<CdCtxValue | null>(null)
export const useCd = () => useContext(CdCtx)!

// Screens that have the speakers and the keyboard to themselves.
const takesOver = (path: string) => path.startsWith('/play/') || path.startsWith('/game/') || path.startsWith('/couch-rig/') || /^\/couch\/\d+\/play\//.test(path)

function shuffled<T>(list: T[]) {
  const a = [...list]
  for (let i = a.length - 1; i > 0; i--) { const j = Math.floor(Math.random() * (i + 1)); [a[i], a[j]] = [a[j], a[i]] }
  return a
}

export function AudioProvider({ children }: { children: React.ReactNode }) {
  const deckA = useRef<HTMLAudioElement | null>(null)
  const deckB = useRef<HTMLAudioElement | null>(null)
  const audio = useRef<HTMLAudioElement | null>(null) // whichever deck is playing
  const standby = () => (audio.current === deckA.current ? deckB.current : deckA.current)
  const [queue, setQueue] = useState<Item[]>([])
  const [index, setIndex] = useState(0)
  const [mode, setMode] = useState<AudioMode>('music')
  const [playing, setPlaying] = useState(false)
  const [shuffle, setShuffleOn] = useState(false)
  const [repeat, setRepeat] = useState<Repeat>('off')
  const unshuffled = useRef<Item[] | null>(null) // the order to go back to
  const [time, setTime] = useState(0)
  const [rate, setRate] = useState(1)
  const [maxSpeed, setMaxSpeed] = useState(4)
  const [sleepAt, setSleepAt] = useState<number | 'chapter' | null>(null)
  const [full, setFull] = useState(false)
  const [wantFull, setWantFull] = useState(false) // once opened it stays loaded, and so is quick the next time
  const [fullOpts, setFullOpts] = useState<{ visualizer?: boolean; panel?: 'lyrics' | 'queue' }>({})
  const [collapsed, setCollapsedState] = useState(() => { try { return localStorage.getItem(COLLAPSE_KEY) === '1' } catch { return false } })
  const [nowPlaying, setNowPlaying] = useState<{ id: number; title: string; artist: string; track: string } | null>(null)
  const [likes, setLikes] = useState<Set<number>>(() => new Set())
  const hls = useRef<{ destroy: () => void } | null>(null) // an HLS station's player, loaded when one is played
  // A stream that was playing and dropped is tried again a few times, as a radio would.
  const [retrying, setRetrying] = useState(false)
  const wasPlaying = useRef(false)
  const retries = useRef(0)
  const retryTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const progressAt = useRef(0)
  const [vizHere, setVizHere] = useState(true) // the owner can take the plugin off the server
  useEffect(() => { getPrefs().then((p) => setVizHere(p.visualizer !== false)).catch(() => {}) }, [])
  const mediaEl = useCallback(() => audio.current, [])
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

  // ------------------------------------------------------------ the sound

  const [sound, setSoundState] = useState<SoundSettings>(loadSound)
  const [soundNote, setSoundNote] = useState('')
  const soundRef = useRef(sound)
  soundRef.current = sound
  const setSound = useCallback((patch: Partial<SoundSettings>) => {
    setSoundState((s) => {
      const n = { ...s, ...patch }
      saveSound(n)
      return n
    })
  }, [])
  const fade = useRef(new WeakMap<HTMLAudioElement, number>())
  // One place sets how loud a deck is: through the graph if it's in there, on the element if not.
  const level = useCallback((el: HTMLAudioElement | null) => {
    if (!el) return
    const s = soundRef.current
    const v = (s.muted ? 0 : s.volume) * (fade.current.get(el) ?? 1)
    const g = soundGraph()
    if (g?.routed(el)) { el.volume = 1; g.setDeckLevel(el, v) } else el.volume = v
  }, [])
  useEffect(() => { level(deckA.current); level(deckB.current) }, [sound.volume, sound.muted, level])

  const usesGraph = sound.eqOn || sound.replayGain !== 'off' || sound.vocalCut > 0 || sound.keyShift !== 0
  // Whenever something needs the graph: this runs from a press (turning it on), or once
  // something plays, which a press also started.
  useEffect(() => {
    // With the graph already there, turning the last thing off has to reach it too.
    if ((!usesGraph && !soundGraph()) || !playing || cdOn.current) return
    let gone = false
    void (async () => {
      const a = audio.current
      if (!canRoute(a)) { setSoundNote('This can\'t change the sound here, as the server is on a different address from this page.'); return }
      const g = await openGraph()
      if (!g || gone) { if (!g && !gone) setSoundNote('This browser wouldn\'t start its audio engine.'); return }
      for (const el of [deckA.current, deckB.current]) if (el && canRoute(el)) { g.route(el); level(el) }
      const s = soundRef.current
      g.setEq(s.eqOn, s.eqGains, s.preamp)
      g.setVocalCut(s.vocalCut)
      const keyOk = await g.setKey(s.keyShift)
      if (gone) return
      setSoundNote(keyOk || s.keyShift === 0 ? '' : 'This browser can\'t change the key.')
    })()
    return () => { gone = true }
  }, [usesGraph, playing, sound.eqOn, sound.eqGains, sound.preamp, sound.vocalCut, sound.keyShift, current?.id, level])

  // A file tagged with how loud it is, brought to one level.
  const loudKnown = useRef(new Map<number, { trackDb: number | null; albumDb: number | null; trackPeak: number | null; albumPeak: number | null }>())
  useEffect(() => {
    const g = soundGraph()
    if (sound.replayGain === 'off' || !current || mode !== 'music' || cdOn.current) { g?.setLoudness(0); return }
    let gone = false
    const id = current.id
    const apply = (l: { trackDb: number | null; albumDb: number | null; trackPeak: number | null; albumPeak: number | null }) => {
      if (gone) return
      const album = sound.replayGain === 'album'
      const db = (album ? l.albumDb ?? l.trackDb : l.trackDb ?? l.albumDb) ?? 0
      const peak = (album ? l.albumPeak ?? l.trackPeak : l.trackPeak ?? l.albumPeak) ?? 0
      // Never so loud that the file's own peak would clip.
      const room = peak > 0 ? -20 * Math.log10(peak) : 12
      soundGraph()?.setLoudness(Math.min(db, room))
    }
    const have = loudKnown.current.get(id)
    if (have) apply(have)
    else api.get<{ trackDb: number | null; albumDb: number | null; trackPeak: number | null; albumPeak: number | null }>(`/music/loudness/${id}`).then((l) => { loudKnown.current.set(id, l); apply(l) }).catch(() => {})
    return () => { gone = true }
  }, [sound.replayGain, current?.id, mode, playing]) // eslint-disable-line react-hooks/exhaustive-deps

  // ------------------------------------------------------------ likes

  const likesFor = useRef(0)
  useEffect(() => {
    if (mode !== 'music' || !current || likesFor.current === current.libraryId) return
    likesFor.current = current.libraryId
    api.get<number[]>(`/music/likes?library=${current.libraryId}`).then((ids) => setLikes((old) => new Set([...old, ...ids]))).catch(() => {})
  }, [current?.libraryId, mode]) // eslint-disable-line react-hooks/exhaustive-deps
  const toggleLike = useCallback((id: number) => {
    setLikes((old) => {
      const n = new Set(old)
      const on = !n.has(id)
      if (on) n.add(id); else n.delete(id)
      api.put(`/items/${id}/reaction`, { value: on ? 1 : 0 }).catch((e: Error) => {
        toast(e.message)
        setLikes((back) => { const b = new Set(back); if (on) b.delete(id); else b.add(id); return b })
      })
      return n
    })
  }, [])

  // ------------------------------------------------------------ the full player

  // A screen of its own: Back (the button, Esc, or the phone's or remote's Back) returns to
  // the page underneath, which stays exactly as it was.
  const openFull = (opts: { visualizer?: boolean; panel?: 'lyrics' | 'queue' } = {}) => {
    if (cdOn.current && cd.now) { nav(`/cd/${cd.now.disc.id}`); return }
    setFullOpts(opts)
    setWantFull(true)
    if (!full) {
      setFull(true)
      history.pushState({ ...(history.state || {}), wmsAudioFull: true }, '')
    }
  }
  const closeFull = () => { if (history.state?.wmsAudioFull) history.back(); else setFull(false) }
  useEffect(() => {
    if (!full) return
    const pop = () => setFull(false)
    window.addEventListener('popstate', pop)
    // What's behind can't be reached with Tab or read out while this covers it.
    const app = document.querySelector('.app')
    app?.setAttribute('inert', '')
    document.documentElement.classList.add('np-open')
    return () => {
      window.removeEventListener('popstate', pop)
      app?.removeAttribute('inert')
      document.documentElement.classList.remove('np-open')
      // Back where the person came from: the bar, which has just returned.
      requestAnimationFrame(() => {
        for (const el of document.querySelectorAll<HTMLElement>('.ab-expand, .audio-bar .phone-only')) {
          el.focus({ preventScroll: true })
          if (document.activeElement === el) break
        }
      })
    }
  }, [full])
  const setCollapsed = (v: boolean) => {
    setCollapsedState(v)
    try { localStorage.setItem(COLLAPSE_KEY, v ? '1' : '0') } catch { /* private window */ }
  }

  // ------------------------------------------------------------ the two decks

  const prepared = useRef<{ id: number; el: HTMLAudioElement } | null>(null)
  const handingTo = useRef<number | null>(null) // the track the decks have already moved to
  const handoffTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const fading = useRef<ReturnType<typeof setInterval> | undefined>(undefined)

  const direct = (it: Item) => DIRECT.has(it.videoCodec)
  const dropPrepared = useCallback(() => {
    clearTimeout(handoffTimer.current)
    handoffTimer.current = undefined
    const p = prepared.current
    prepared.current = null
    if (p && p.el !== audio.current) { p.el.pause(); p.el.removeAttribute('src'); p.el.load() }
  }, [])
  const silenceOther = useCallback(() => {
    clearInterval(fading.current)
    const other = standby()
    if (other && other.getAttribute('src') && !prepared.current) { other.pause(); other.removeAttribute('src'); other.load() }
    if (audio.current) { fade.current.set(audio.current, 1); level(audio.current) }
  }, [level]) // eslint-disable-line react-hooks/exhaustive-deps

  // Start (or switch) the track.
  useEffect(() => {
    const a = audio.current
    if (!a || !current || cdOn.current) return
    if (handingTo.current === current.id) { handingTo.current = null; return } // it's already playing
    dropPrepared()
    silenceOther()
    const resume = books && current.progress && !current.progress.watched ? current.progress.positionSec : 0
    load(current, resume)
    a.play().catch(() => setPlaying(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id])
  // After that, as load used the speed from before a change of kind of listening. A disc
  // plays at its own speed, whatever the music was at.
  useEffect(() => { if (audio.current) setSpeed(audio.current, cdOn.current ? 1 : rate) }, [rate, current?.id, cd.now?.disc.id])

  const hlsLoad = useRef(0) // the newest station to ask for hls.js: it can arrive after they've gone
  useEffect(() => () => { hlsLoad.current++; hls.current?.destroy(); hls.current = null }, [])
  const load = (it: Item, at: number) => {
    const a = audio.current!
    hls.current?.destroy()
    hls.current = null
    const asked = ++hlsLoad.current
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
        if (audio.current !== a || currentId.current !== id || hlsLoad.current !== asked) return
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
  const position = useCallback(() => (audio.current ? audio.current.currentTime + offset.current : 0), [])
  const seek = (t: number) => {
    const a = audio.current
    if (!a || !current) return
    if (cdOn.current) { cd.actions.seekInTrack(t); return }
    if (direct(current)) a.currentTime = t
    else { load(current, t); a.play().catch(() => {}) }
    dropPrepared()
    setTime(t)
  }

  // What comes after this one, if anything does.
  const upNext = () => {
    if (index + 1 < queue.length) return { item: queue[index + 1], i: index + 1 }
    if (repeat === 'all' && queue.length > 1) return { item: queue[0], i: 0 }
    return null
  }
  // Songs of one album are often meant to run together, so they're only crossed if asked.
  const fadeFor = (n: Item) => {
    const s = soundRef.current
    if (!s.crossfade) return 0
    if (!s.crossfadeAlbums && current && n.album && n.album === current.album) return 0
    return s.crossfade
  }
  const handOff = (n: { item: Item; i: number }, seconds: number) => {
    const old = audio.current, fresh = prepared.current?.el
    if (!old || !fresh || prepared.current?.id !== n.item.id || fresh.readyState < 2) return
    prepared.current = null
    fresh.currentTime = 0
    setSpeed(fresh, rate)
    fade.current.set(fresh, seconds > 0 ? 0 : 1)
    level(fresh)
    audio.current = fresh
    offset.current = 0
    handingTo.current = n.item.id
    fresh.play().catch(() => setPlaying(false))
    setIndex(n.i)
    clearInterval(fading.current)
    if (seconds <= 0) {
      setTimeout(() => { if (audio.current !== old) { old.pause(); old.removeAttribute('src'); old.load() } }, 400)
      return
    }
    // Equal power, so the middle of the fade isn't a dip.
    const began = performance.now()
    fading.current = setInterval(() => {
      const p = Math.min(1, (performance.now() - began) / (seconds * 1000))
      fade.current.set(fresh, Math.sin((p * Math.PI) / 2))
      fade.current.set(old, Math.cos((p * Math.PI) / 2))
      level(fresh)
      level(old)
      if (p >= 1) {
        clearInterval(fading.current)
        if (audio.current !== old) { old.pause(); old.removeAttribute('src'); old.load() }
        fade.current.set(old, 1)
      }
    }, 60)
  }
  // Called as a song plays: gets the next one ready, then starts it at the right moment.
  const watchEnd = () => {
    const a = audio.current, s = soundRef.current
    if (!a || mode !== 'music' || cdOn.current || repeat === 'one' || sleepAt === 'chapter' || !(s.gapless || s.crossfade)) return
    const d = a.duration
    if (!Number.isFinite(d) || d <= 0) return
    const n = upNext()
    if (!n || !direct(n.item)) return
    const left = (d - a.currentTime) / (a.playbackRate || 1)
    const crossing = fadeFor(n.item)
    if (left > crossing + 20) return
    if (prepared.current?.id !== n.item.id) {
      dropPrepared()
      const el = standby()
      if (!el) return
      el.src = mediaUrl(`/api/items/${n.item.id}/stream`)
      el.preload = 'auto'
      el.load()
      prepared.current = { id: n.item.id, el }
      if (soundGraph()) { soundGraph()!.route(el); fade.current.set(el, 0); level(el) }
      return
    }
    if (!handoffTimer.current && left <= crossing + 1.5 && !a.paused) {
      handoffTimer.current = setTimeout(() => { handoffTimer.current = undefined; if (!audio.current?.paused) handOff(n, crossing) }, Math.max(0, (left - crossing) * 1000 - 25))
    }
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
      if (a.getAttribute('src')) a.play().catch(() => {}) // otherwise hls.js is on its way, and plays when it's ready
      return
    }
    a.play().catch(() => {})
  }

  // A video or a game takes over the speakers: pause the music.
  const takenOver = takesOver(loc.pathname)
  useEffect(() => {
    if (!takenOver) return
    audio.current?.pause()
    if (full) closeFull()
  }, [takenOver]) // eslint-disable-line react-hooks/exhaustive-deps

  // Sleep timer.
  useEffect(() => {
    if (typeof sleepAt !== 'number') return
    const t = setTimeout(() => { audio.current?.pause(); setSleepAt(null) }, Math.max(0, sleepAt - Date.now()))
    return () => clearTimeout(t)
  }, [sleepAt])

  // The next one, or the same one again when it's the only one there is.
  const goTo = (i: number) => {
    if (queue[i]?.id === current?.id) { seek(0); audio.current?.play().catch(() => {}); return }
    setIndex(i)
  }
  const next = useCallback(() => {
    if (cdOn.current) { cd.actions.next(); return }
    if (!queue.length) return
    if (index + 1 < queue.length) goTo(index + 1)
    else if (repeat === 'all') goTo(0)
    else { audio.current?.pause() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queue, index, repeat])
  const prev = useCallback(() => {
    if (cdOn.current) { cd.actions.prev(); return }
    if (position() > 4 || index === 0) { seek(0); return }
    goTo(index - 1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [index, current, queue])
  const onEnded = () => {
    if (cdOn.current) { cd.onEnded(); return }
    if (live) { setPlaying(false); return }
    if (current && books) api.post(`/items/${current.id}/progress`, { positionSec: current.runtimeSec, durationSec: current.runtimeSec }).catch(() => {})
    if (sleepAt === 'chapter') { setSleepAt(null); return }
    if (repeat === 'one') { seek(0); audio.current?.play().catch(() => {}); return }
    next()
  }

  // Shuffle really reorders what's to come, so the queue shows what will actually play, and
  // Previous goes back to what actually did.
  const setShuffle = (on: boolean) => {
    if (cdOn.current) return
    setShuffleOn(on)
    if (on) {
      unshuffled.current = queue
      if (current) { setQueue([current, ...shuffled(queue.filter((_, i) => i !== index))]); setIndex(0) }
    } else if (unshuffled.current) {
      const back = unshuffled.current.filter((q) => queue.some((x) => x.id === q.id))
      const extra = queue.filter((q) => !back.some((x) => x.id === q.id))
      const all = [...back, ...extra]
      setQueue(all)
      setIndex(Math.max(0, all.findIndex((q) => q.id === current?.id)))
      unshuffled.current = null
    }
  }

  // Editing the queue. The song playing keeps playing, wherever it ends up.
  const editQueue = (fn: (q: Item[]) => Item[]) => {
    const keep = index
    setQueue((q) => {
      const cur = q[keep]
      const n = fn(q)
      const at = n.indexOf(cur)
      setIndex(at >= 0 ? at : Math.min(keep, Math.max(0, n.length - 1)))
      return n
    })
    dropPrepared()
  }
  const moveInQueue = (from: number, to: number) => editQueue((q) => {
    if (to < 0 || to >= q.length) return q
    const n = [...q]
    const [it] = n.splice(from, 1)
    n.splice(to, 0, it)
    return n
  })
  const removeFromQueue = (i: number) => {
    if (i === index) { if (queue.length === 1) { ctx.stop(); return } }
    editQueue((q) => q.filter((_, x) => x !== i))
  }
  const clearUpNext = () => editQueue((q) => q.slice(0, index + 1))

  // Lock screen, headphones and keyboard media keys.
  useEffect(() => {
    const ms = navigator.mediaSession
    if (!ms || !current) return
    const song = live && nowPlaying?.id === current.id ? nowPlaying : null
    const art = (w: number) => ({ src: new URL(posterUrl(current) + `&w=${w}`, location.href).href, sizes: `${w}x${w}` })
    ms.metadata = new MediaMetadata({ title: song ? song.track || song.title : current.title, artist: song ? song.artist || current.title : current.artist || current.albumArtist || '', album: live ? (song ? current.title : '') : current.album || '',
      artwork: current.poster ? [art(256), art(512), art(1024)] : [] })
    const h: [MediaSessionAction, MediaSessionActionHandler | null][] = [
      ['play', resume], ['pause', () => audio.current?.pause()], ['stop', () => ctx.stop()],
      ['nexttrack', next], ['previoustrack', prev],
      ['seekto', live ? null : (d) => { if (d.seekTime != null) seek(d.seekTime) }],
      ['seekforward', live ? null : () => seek(position() + (books ? 30 : 10))], ['seekbackward', live ? null : () => seek(Math.max(0, position() - (books ? 15 : 10)))],
    ]
    for (const [a, f] of h) { try { ms.setActionHandler(a, f) } catch { /* unsupported action */ } }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current, next, prev, nowPlaying, live])
  useEffect(() => {
    const ms = navigator.mediaSession
    if (!ms) return
    ms.playbackState = playing ? 'playing' : 'paused'
  }, [playing])
  // The lock screen's own progress bar: told now and then, and it counts by itself.
  const durNow = current?.runtimeSec || 0
  const secondsAt = Math.floor(time / 5)
  useEffect(() => {
    const ms = navigator.mediaSession
    if (!ms?.setPositionState || live || !durNow) return
    try { ms.setPositionState({ duration: durNow, playbackRate: cdOn.current ? 1 : rate, position: Math.min(durNow, position()) }) } catch { /* it refuses numbers it doesn't like */ }
  }, [secondsAt, durNow, rate, live, position])

  // V for the visualizer, wherever the music is playing from. In the full player it's the
  // switch for the background, which that screen handles itself.
  useEffect(() => {
    if (!current || full) return
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName
      // A game's V is the game's, and the CD player has a visualizer of its own.
      if (!vizHere || e.key.toLowerCase() !== 'v' || e.ctrlKey || e.metaKey || e.altKey || tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || takenOver || loc.pathname.startsWith('/cd/')) return
      e.preventDefault()
      openFull({ visualizer: true })
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [current?.id, full, vizHere, takenOver, loc.pathname]) // eslint-disable-line react-hooks/exhaustive-deps

  // "<" and ">" step through the speeds, as in the video player.
  useEffect(() => {
    if (!current) return
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName
      if (cdOn.current || tag === 'INPUT' || tag === 'TEXTAREA' || (e.key !== '<' && e.key !== '>') || document.querySelector('.player:not(.mini):not(.inline), .player.inline')) return
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
    play: (items, start = 0, m = 'music') => {
      if (cdOn.current) cd.actions.eject()
      setMode(m)
      setShuffleOn(false)
      unshuffled.current = null
      setQueue(items)
      setIndex(start)
      if (items[start]?.id === current?.id) resume()
    },
    enqueue: (items) => {
      if (cdOn.current) { cd.actions.eject(); setQueue(items); setIndex(0); return }
      setQueue((q) => [...q, ...items])
      if (unshuffled.current) unshuffled.current = [...unshuffled.current, ...items]
    },
    playNext: (items) => {
      if (cdOn.current || !current) { ctx.enqueue(items); return }
      setQueue((q) => [...q.slice(0, index + 1), ...items, ...q.slice(index + 1)])
      if (unshuffled.current) unshuffled.current = [...unshuffled.current, ...items]
    },
    toggle: () => { const a = audio.current; if (a) a.paused ? resume() : a.pause() },
    next, prev,
    stop: () => {
      if (cdOn.current) cd.actions.eject()
      audio.current?.pause()
      hangUp()
      dropPrepared()
      silenceOther()
      setQueue([])
      setShuffleOn(false)
      unshuffled.current = null
      if (history.state?.wmsAudioFull) history.back()
      setFull(false)
    },
    liked: (id) => likes.has(id),
    toggleLike,
    startRadio: (it) => {
      api.get<Item[]>(`/music/radio/${it.id}`).then((list) => {
        if (list.length < 2) { toast('There isn\'t enough here yet to make a station from this song'); return }
        ctx.play(list, 0, 'music')
      }).catch((e: Error) => toast(e.message))
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [queue, index, current, mode, playing, next, prev, nowPlaying, likes, toggleLike])

  const cdValue = useMemo<CdCtxValue>(() => ({ now: cd.now, actions: cd.actions, audioEl: audio.current, playing, toggle: ctx.toggle }), [cd.now, cd.actions, playing, ctx.toggle])
  const cdActive = cdOn.current && !!cd.now

  const dur = current?.runtimeSec || 0
  // The album (or audiobook, or podcast) the track belongs to.
  const albumUrl = live && current ? `/radio/${current.id}` : current?.album ? `/album/${current.libraryId}?artist=${encodeURIComponent(current.albumArtist || current.artist || '')}&album=${encodeURIComponent(current.album)}` : ''
  const artistUrl = !live && !books && current && (current.albumArtist || current.artist) ? `/artist/${current.libraryId}?name=${encodeURIComponent(current.albumArtist || current.artist || '')}` : ''
  const goTowards = (url: string) => {
    if (!url) return
    const wasFull = !!history.state?.wmsAudioFull
    setFull(false)
    nav(url, { replace: wasFull })
  }

  const view: PlayerView = {
    queue, index, current, mode, playing, time, duration: dur, shuffle: cdActive ? !!cd.now?.shuffle : shuffle, repeat: cdActive ? cd.now!.repeat : repeat, rate, maxSpeed, sleepAt, retrying,
    stationNow: ctx.nowPlaying, books, live, cdActive, vizHere, full, fullOpts, collapsed, albumUrl, artistUrl, sound, soundNote,
    media: mediaEl, position, seek, toggle: ctx.toggle, next, prev, stop: ctx.stop, jump: goTo, moveInQueue, removeFromQueue, clearUpNext,
    setShuffle: (on) => { if (cdActive) cd.actions.setShuffle(on); else setShuffle(on) },
    cycleRepeat: () => {
      const r = cdActive ? cd.now!.repeat : repeat
      const n = r === 'off' ? 'all' : r === 'all' ? 'one' : 'off'
      if (cdActive) cd.actions.setRepeat(n); else setRepeat(n)
    },
    changeRate, setSleepAt, setSound, liked: ctx.liked(current?.id ?? -1), toggleLike, openFull, closeFull, setCollapsed, goTowards, startRadio: ctx.startRadio,
  }

  const onDeck = (el: EventTarget) => el === audio.current
  const deckEvents = {
    onPlay: (e: React.SyntheticEvent) => { if (!onDeck(e.currentTarget)) return; setPlaying(true); if (cdOn.current) cd.onPlay() },
    onPlaying: (e: React.SyntheticEvent) => { if (onDeck(e.currentTarget) && live) wasPlaying.current = true },
    onPause: (e: React.SyntheticEvent) => {
      if (!onDeck(e.currentTarget)) return
      setPlaying(false)
      if (cdOn.current) cd.onPause()
      if (!live) return
      // At the end of a stream the browser pauses too: that's a dropped connection.
      if (audio.current?.ended && wasPlaying.current) reconnect()
      else hangUp()
    },
    onEnded: (e: React.SyntheticEvent) => { if (onDeck(e.currentTarget)) onEnded() },
    onWaiting: (e: React.SyntheticEvent) => { if (onDeck(e.currentTarget) && cdOn.current) cd.onWaiting() },
    onEmptied: (e: React.SyntheticEvent) => { if (onDeck(e.currentTarget) && cdOn.current) setPlaying(!audio.current?.paused) },
    onError: (e: React.SyntheticEvent) => {
      if (!onDeck(e.currentTarget)) { if (prepared.current?.el === e.currentTarget) prepared.current = null; return }
      if (cdOn.current) { cd.onError(); return }
      if (!live || !current || !audio.current?.getAttribute('src')) return
      if (wasPlaying.current) { reconnect(); return }
      const fail = () => { setPlaying(false); toast(`${current.title} can't be reached`); if (audio.current) reportMediaError(audio.current, `Audio of item ${current.id}`) }
      if (current.radio?.hls) { fail(); return }
      // A station added by its address may have turned out to be HLS just now.
      api.get<Item>(`/radio/${current.id}`).then((r) => {
        if (!r.radio?.hls) { fail(); return }
        const it = { ...current, radio: r.radio }
        setQueue((q) => q.map((x) => (x.id === it.id ? it : x)))
        load(it, 0)
      }).catch(fail)
    },
    onTimeUpdate: (e: React.SyntheticEvent) => {
      if (!onDeck(e.currentTarget)) return
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
      watchEnd()
    },
  }

  return (
    <Ctx.Provider value={ctx}>
      <CdCtx.Provider value={cdValue}>{children}</CdCtx.Provider>
      <audio ref={(el) => { deckA.current = el; if (el && !audio.current) audio.current = el }} preload="auto" {...deckEvents} />
      <audio ref={deckB} preload="none" {...deckEvents} />
      {current && !takenOver && !(cdActive && loc.pathname === `/cd/${cd.now!.disc.id}`) && (
        <>
          {!full && <MiniPlayer p={view} cdSpin={cdActive} />}
          {wantFull && <Suspense fallback={null}>{full && <NowPlaying p={view} />}</Suspense>}
        </>
      )}
    </Ctx.Provider>
  )
}
