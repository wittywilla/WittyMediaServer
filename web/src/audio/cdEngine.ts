import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { api, mediaUrl, type Item } from '../api'
import { toast } from '../state'

// Playing a CD: one <audio> element, one continuous stream. The server sends the
// whole disc (or a chosen order of tracks) as a single run of samples, so there
// is no join between tracks to get wrong. Tracks and indexes are places in that
// run, worked out here from the disc's table, the same way the server lays the
// stream out (docs/CLIENT-API.md, "Audio CDs").

export const CD_RATE = 44100
export const SECTOR_SAMPLES = 588
const SECTORS_PER_SEC = 75

export type CdTrack = {
  number: number; itemId: number; title: string; artist?: string; audio: boolean
  pregap: number; start: number; end: number // sectors from the start of the disc
  indexes: { number: number; lba: number }[]; preEmphasis: boolean; isrc?: string
}
export type CdWarning = { code: string; message: string }
export type CdDisc = {
  id: number; libraryId: number; title: string; artist: string; year: number | null; poster: string
  format: string; discId: string; cddbId?: string; toc: string; sizeBytes: number; sessions: number
  samples: number; durationSec: number; positionSec: number; tracks: CdTrack[]; hiddenTrack: boolean; hasGraphics: boolean
  problem?: string; warnings: CdWarning[]; formats: string[]; suggested: string
  lookup?: string; matches?: number; source?: string; releaseId?: string; attribution?: string
  insertEffect: boolean; insertSound: boolean
}
export type CdRepeat = 'off' | 'all' | 'one'
export type CdFormat = 'flac' | 'wav' | 'aac'
export type CdQuality = 'auto' | 'lossless' | 'compressed'

type Seg = { start: number; samples: number; lba: number; track: CdTrack }

// layout is the stream's map. With no program it is the disc as it is, pregaps and
// all; a program is each track from its index 1 to where the next one's pregap
// starts. The server's Stream does the same.
function layout(disc: CdDisc, program: number[] | null): Seg[] {
  const segs: Seg[] = []
  let at = 0
  const push = (lba: number, sectors: number, track: CdTrack) => {
    const samples = sectors * SECTOR_SAMPLES
    segs.push({ start: at, samples, lba, track })
    at += samples
  }
  if (!program) {
    for (const t of disc.tracks) if (t.audio) push(t.pregap, t.end - t.pregap, t)
    return segs
  }
  for (const n of program) {
    const t = disc.tracks.find((x) => x.number === n && x.audio)
    if (t) push(t.start, t.end - t.start, t)
  }
  return segs
}

const lengthOf = (segs: Seg[]) => (segs.length ? segs[segs.length - 1].start + segs[segs.length - 1].samples : 0)

export type CdNow = {
  disc: CdDisc
  track: CdTrack | null
  // trackSec counts up from index 1, and is negative in the pregap before it.
  trackSec: number; trackLen: number; index: number
  discSec: number; discLen: number
  repeat: CdRepeat; shuffle: boolean; program: number[] | null; order: number[] | null
  format: CdFormat; quality: CdQuality; buffering: boolean; stopped: boolean
}

export type CdActions = {
  insert: (disc: CdDisc, opts?: { play?: boolean; track?: number }) => void
  eject: () => void
  stop: () => void
  next: () => void
  prev: () => void
  nextIndex: (dir: 1 | -1) => void
  seekInTrack: (sec: number) => void
  nudge: (sec: number) => void
  goTrack: (n: number, play?: boolean) => void
  goHidden: () => void
  setRepeat: (r: CdRepeat) => void
  setShuffle: (on: boolean) => void
  setProgram: (p: number[] | null) => void
  setQuality: (q: CdQuality) => void
  refresh: (disc: CdDisc) => void
}

// The tracks of a disc as ordinary items, for the bar, the lock screen and the
// listening history. A disc on a shelf has no track items, so they share its.
export function cdTrackItems(d: CdDisc): Item[] {
  return d.tracks.filter((t) => t.audio).map((t) => ({
    id: t.itemId || d.id, libraryId: d.libraryId, kind: 'audio', path: '', title: t.title, sortTitle: t.title, year: d.year, overview: '', tagline: '', genres: [],
    rating: null, contentRating: '', studio: '', runtimeSec: (t.end - t.start) / SECTORS_PER_SEC, poster: d.poster, backdrop: '', container: 'cd', videoCodec: 'pcm_s16le',
    width: 0, height: 0, audioLangs: [], subLangs: [], provider: '', providerId: '', sizeBytes: 0, addedAt: 0, updatedAt: 0, playMode: '', insertFx: '',
    artist: t.artist || d.artist, album: d.title, albumArtist: d.artist, track: t.number,
  }))
}

function loadQuality(): CdQuality {
  try {
    const q = localStorage.getItem('wms.cd.quality')
    if (q === 'lossless' || q === 'compressed') return q
  } catch { /* private window */ }
  return 'auto'
}

function playableFormat(disc: CdDisc, quality: CdQuality, a: HTMLAudioElement | null): CdFormat {
  const has = (f: string) => disc.formats.includes(f)
  const can = (type: string) => !a || a.canPlayType(type) !== ''
  if (quality === 'compressed' && has('aac')) return 'aac'
  if (quality === 'auto' && disc.suggested === 'aac') return 'aac'
  if (has('flac') && can('audio/flac')) return 'flac'
  if (has('wav') && can('audio/wav')) return 'wav'
  return has('aac') ? 'aac' : 'wav'
}

function shuffled(list: number[], first?: number): number[] {
  const out = list.filter((n) => n !== first)
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[out[i], out[j]] = [out[j], out[i]]
  }
  return first != null ? [first, ...out] : out
}

type Hooks = {
  // The bar and lock screen follow the track playing.
  onDisc: (items: Item[] | null) => void
  onTrack: (index: number) => void
  onTime: (trackSec: number) => void
}

export function useCdEngine(audio: RefObject<HTMLAudioElement | null>, hooks: Hooks) {
  const [now, setNow] = useState<CdNow | null>(null)
  // What a stream is for. Kept in a ref: timeupdate reads it several times a second.
  const e = useRef({
    disc: null as CdDisc | null,
    segs: [] as Seg[],
    program: null as number[] | null, // a chosen order
    order: null as number[] | null, // what is playing: the program, or a shuffle
    shuffle: false,
    repeat: 'off' as CdRepeat,
    loops: 1,
    format: 'flac' as CdFormat,
    quality: 'auto' as CdQuality,
    offset: 0, // the sample a reloaded stream started at
    current: null as CdTrack | null,
    index: 1,
    stopped: true,
    stalls: [] as number[],
    loadedAt: 0,
    savedAt: 0,
    // A slider being dragged: where it is, and the seek that will follow it.
    pendingSec: null as number | null,
    pendingTimer: 0,
    reloads: 0,
  })
  const hooksRef = useRef(hooks)
  hooksRef.current = hooks

  const active = () => !!e.current.disc
  const sample = () => {
    const a = audio.current
    return a ? Math.round(a.currentTime * CD_RATE) + e.current.offset : 0
  }

  // streamTracks: what is asked for. Repeating one track is that track on a loop.
  const streamTracks = (): number[] | null => {
    const s = e.current
    if (s.repeat === 'one' && s.current) return [s.current.number]
    return s.order
  }

  // Seconds into the current track, below zero in its pregap.
  const trackPosition = () => {
    const s = e.current
    const len = lengthOf(s.segs) || 1
    const into = sample() % len
    const seg = s.segs.find((x) => into < x.start + x.samples)
    return seg ? (seg.lba * SECTOR_SAMPLES + (into - seg.start) - seg.track.start * SECTOR_SAMPLES) / CD_RATE : 0
  }

  const urlFor = (start: number) => {
    const s = e.current
    const q = new URLSearchParams({ format: s.format })
    const tracks = streamTracks()
    if (tracks) q.set('tracks', tracks.join(','))
    // Several times round in one stream: repeating without a join. WAV has a size
    // limit, and an album is 800 MB.
    s.loops = s.repeat === 'off' ? 1 : s.repeat === 'one' ? (s.format === 'wav' ? 8 : 32) : (s.format === 'wav' ? 4 : 8)
    if (s.loops > 1) q.set('loops', String(s.loops))
    if (s.format !== 'wav' && start > 0) q.set('start', String(start))
    return mediaUrl(`/api/cd/${s.disc!.id}/audio?${q}`)
  }

  const publish = useCallback(() => {
    const s = e.current
    const d = s.disc
    if (!d) { setNow(null); return }
    const len = lengthOf(s.segs)
    const at = sample()
    const into = len ? ((at % len) + len) % len : 0
    const seg = s.segs.find((x) => into < x.start + x.samples) || s.segs[0]
    let trackSec = 0, discSec = 0, index = 1
    if (seg) {
      const t = seg.track
      const lbaSample = seg.lba * SECTOR_SAMPLES + (into - seg.start)
      trackSec = (lbaSample - t.start * SECTOR_SAMPLES) / CD_RATE
      const lba = Math.floor(lbaSample / SECTOR_SAMPLES)
      index = lba < t.start ? 0 : 1
      for (const x of t.indexes) if (lba >= x.lba) index = x.number
      discSec = lbaSample / CD_RATE
      if (s.current !== t) {
        s.current = t
        const audioTracks = d.tracks.filter((x) => x.audio)
        hooksRef.current.onTrack(audioTracks.indexOf(t))
      }
      s.index = index
      if (s.pendingSec != null) trackSec = s.pendingSec
      hooksRef.current.onTime(Math.max(0, trackSec))
    }
    const a = audio.current
    setNow({
      disc: d, track: s.current, trackSec, trackLen: s.current ? (s.current.end - s.current.start) / SECTORS_PER_SEC : 0, index,
      discSec, discLen: len / CD_RATE, repeat: s.repeat, shuffle: s.shuffle, program: s.program, order: s.order, format: s.format, quality: s.quality,
      buffering: !!a && !a.paused && a.readyState < 3, stopped: s.stopped,
    })
  }, [audio])

  // loadAt points the element at a place in the stream. WAV carries its own seek
  // table in its size, so it seeks in the element; the others start a new request.
  const loadAt = (start: number, play: boolean) => {
    const a = audio.current
    const s = e.current
    if (!a || !s.disc) return
    const url = urlFor(start)
    s.loadedAt = performance.now()
    if (s.format === 'wav') {
      s.offset = 0
      if (a.src !== url) {
        a.src = url
        if (start > 0) a.addEventListener('loadedmetadata', () => { a.currentTime = start / CD_RATE }, { once: true })
      } else {
        a.currentTime = start / CD_RATE
      }
    } else {
      s.offset = start
      a.src = url
    }
    s.stopped = false
    if (play) a.play().catch(() => publish())
    publish()
  }

  const saveProgress = (force = false) => {
    const s = e.current
    if (!s.disc || s.stopped || (!force && performance.now() - s.savedAt < 14000)) return
    s.savedAt = performance.now()
    const a = audio.current
    if (!a || !s.current) return
    const len = lengthOf(s.segs)
    const into = len ? sample() % len : 0
    const seg = s.segs.find((x) => into < x.start + x.samples)
    if (!seg) return
    const pos = (seg.lba * SECTOR_SAMPLES + (into - seg.start)) / CD_RATE
    if (pos > 3) api.post(`/items/${s.disc.id}/progress`, { positionSec: pos, durationSec: s.disc.durationSec }).catch(() => {})
  }

  // Where in this stream a track's index 1 is, from the loop now playing.
  const startOf = (n: number): number | null => {
    const s = e.current
    const seg = s.segs.find((x) => x.track.number === n)
    if (!seg) return null
    const len = lengthOf(s.segs) || 1
    const loop = Math.floor(sample() / len) * len
    return loop + seg.start + (seg.track.start - seg.lba) * SECTOR_SAMPLES
  }

  const seekStream = (at: number) => {
    const a = audio.current
    const s = e.current
    if (!a || !s.disc) return
    const was = !a.paused || s.stopped
    if (s.format === 'wav' && a.src) {
      a.currentTime = at / CD_RATE
      s.stopped = false
      if (was) a.play().catch(() => {})
      publish()
    } else {
      loadAt(at, was)
    }
  }

  const rebuild = (play: boolean) => {
    // A new stream, at the same place on the same track when it has one.
    const s = e.current
    if (!s.disc) return
    const cur = s.current
    const len = lengthOf(s.segs)
    const into = len ? sample() % len : 0
    const seg = s.segs.find((x) => into < x.start + x.samples)
    const lbaSample = seg ? seg.lba * SECTOR_SAMPLES + (into - seg.start) : 0
    s.segs = layout(s.disc, streamTracks())
    let target = 0
    if (cur) {
      const ns = s.segs.find((x) => x.track.number === cur.number)
      if (ns) target = Math.max(0, ns.start + lbaSample - ns.lba * SECTOR_SAMPLES)
    }
    if (s.format === 'wav' && audio.current) audio.current.removeAttribute('src')
    loadAt(target, play)
  }

  const listOrder = () => {
    const s = e.current
    return s.order || s.disc!.tracks.filter((t) => t.audio).map((t) => t.number)
  }

  const actions = useMemo<CdActions>(() => ({
    insert(disc, opts = {}) {
      const s = e.current
      const a = audio.current
      s.disc = disc
      s.program = null; s.order = null; s.shuffle = false; s.repeat = 'off'
      s.quality = loadQuality()
      s.format = playableFormat(disc, s.quality, a)
      s.segs = layout(disc, null)
      s.current = null
      s.stalls = []
      hooksRef.current.onDisc(cdTrackItems(disc))
      const first = disc.tracks.find((t) => t.audio)
      let at = first ? startOfDisc(first) : 0
      if (opts.track != null) {
        const t = disc.tracks.find((x) => x.number === opts.track && x.audio)
        if (t) at = sampleOfLba(s.segs, t.start)
      } else if (disc.positionSec > 3) {
        at = sampleOfLba(s.segs, Math.floor(disc.positionSec * SECTORS_PER_SEC))
      }
      if (a) { a.removeAttribute('src'); a.load() }
      loadAt(at, !!opts.play)
      if (!opts.play) { s.stopped = !(opts.track != null || disc.positionSec > 3); publish() }
    },
    eject() {
      saveProgress(true)
      const s = e.current
      const a = audio.current
      if (a) { a.pause(); a.removeAttribute('src'); a.load() }
      s.disc = null; s.current = null; s.segs = []; s.offset = 0; s.stopped = true
      hooksRef.current.onDisc(null)
      setNow(null)
    },
    stop() {
      // As on a real player: back to the start, and the place is forgotten.
      const s = e.current
      const a = audio.current
      if (!s.disc || !a) return
      a.pause()
      s.current = null
      s.order = s.program || (s.shuffle ? listOrder() : null)
      s.segs = layout(s.disc, streamTracks())
      const first = s.disc.tracks.find((t) => t.audio)
      const at = s.order ? 0 : first ? startOfDisc(first) : 0
      if (s.format === 'wav') a.removeAttribute('src')
      loadAt(at, false)
      s.stopped = true
      api.del(`/items/${s.disc.id}/progress`).catch(() => {})
      publish()
    },
    next() {
      const s = e.current
      if (!s.disc || !s.current) return
      const list = listOrder()
      const i = list.indexOf(s.current.number)
      const n = list[i + 1] ?? (s.repeat === 'all' || s.repeat === 'one' ? list[0] : null)
      if (n != null) actions.goTrack(n)
    },
    prev() {
      const s = e.current
      if (!s.disc || !s.current) return
      const list = listOrder()
      const i = list.indexOf(s.current.number)
      if (trackPosition() > 3 || (i === 0 && !s.disc.hiddenTrack)) { actions.goTrack(s.current.number); return }
      if (i === 0) { actions.goHidden(); return } // rewinding off the start finds what is hiding there
      actions.goTrack(list[i - 1])
    },
    nextIndex(dir) {
      const s = e.current
      const t = s.current
      if (!t) return
      const marks = [t.start, ...t.indexes.map((x) => x.lba)]
      const lba = t.start + Math.floor(Math.max(0, trackPosition()) * SECTORS_PER_SEC)
      let pick = dir > 0 ? marks.find((m) => m > lba) : [...marks].reverse().find((m) => m < lba - SECTORS_PER_SEC)
      if (dir < 0 && pick == null) pick = t.start
      if (pick == null) return
      const seg = s.segs.find((x) => x.track === t)
      if (seg) seekStream(Math.floor(sample() / (lengthOf(s.segs) || 1)) * lengthOf(s.segs) + seg.start + (pick - seg.lba) * SECTOR_SAMPLES)
    },
    seekInTrack(sec) {
      const s = e.current
      const t = s.current
      if (!t) return
      // A slider sends many of these while it moves, and all but the last would
      // each be a new request.
      s.pendingSec = sec
      window.clearTimeout(s.pendingTimer)
      s.pendingTimer = window.setTimeout(() => {
        s.pendingSec = null
        const base = s.current ? startOf(s.current.number) : null
        if (base != null) seekStream(Math.round(base + sec * CD_RATE))
      }, 160)
      publish()
    },
    nudge(sec) {
      const s = e.current
      if (!s.disc) return
      const len = lengthOf(s.segs)
      seekStream(Math.max(0, Math.min(len - 1, sample() + Math.round(sec * CD_RATE))))
    },
    goTrack(n, play) {
      const s = e.current
      const a = audio.current
      if (!s.disc || !a) return
      const t = s.disc.tracks.find((x) => x.number === n && x.audio)
      if (!t) return
      const wasPlaying = play ?? (!a.paused || s.stopped)
      if (s.repeat === 'one') {
        s.current = t
        s.segs = layout(s.disc, [n])
        if (s.format === 'wav') a.removeAttribute('src')
        hooksRef.current.onTrack(s.disc.tracks.filter((x) => x.audio).indexOf(t))
        loadAt(0, wasPlaying)
        return
      }
      const at = startOf(n)
      if (at == null) {
        // Not in this program: it plays from here in disc order.
        s.program = null; s.order = null; s.shuffle = false
        s.segs = layout(s.disc, null)
        s.current = t
        if (s.format === 'wav') a.removeAttribute('src')
        loadAt(sampleOfLba(s.segs, t.start), wasPlaying)
        return
      }
      if (s.format === 'wav' && a.src) {
        a.currentTime = at / CD_RATE
        s.stopped = false
        if (wasPlaying) a.play().catch(() => {})
        publish()
      } else {
        loadAt(at, wasPlaying)
      }
    },
    goHidden() {
      const s = e.current
      if (!s.disc?.hiddenTrack) return
      if (s.order) actions.setProgram(null)
      const first = s.disc.tracks.find((t) => t.audio)
      if (first) seekStream(sampleOfLba(s.segs, first.pregap))
    },
    setRepeat(r) {
      const s = e.current
      if (s.repeat === r) return
      const a = audio.current
      s.repeat = r
      rebuild(!!a && !a.paused)
    },
    setShuffle(on) {
      const s = e.current
      if (!s.disc) return
      const a = audio.current
      s.shuffle = on
      const all = s.disc.tracks.filter((t) => t.audio).map((t) => t.number)
      s.order = on ? shuffled(s.program || all, s.current?.number) : s.program
      rebuild(!!a && !a.paused)
    },
    setProgram(p) {
      const s = e.current
      if (!s.disc) return
      const a = audio.current
      s.program = p && p.length ? p : null
      s.shuffle = false
      s.order = s.program
      rebuild(!!a && !a.paused)
    },
    setQuality(q) {
      const s = e.current
      if (!s.disc) return
      const a = audio.current
      s.quality = q
      try { localStorage.setItem('wms.cd.quality', q) } catch { /* private window */ }
      const f = playableFormat(s.disc, q, a)
      if (f === s.format) { publish(); return }
      s.format = f
      rebuild(!!a && !a.paused)
    },
    refresh(disc) {
      const s = e.current
      if (s.disc?.id !== disc.id) return
      s.disc = disc
      s.current = null
      s.segs = layout(disc, streamTracks())
      hooksRef.current.onDisc(cdTrackItems(disc))
      publish()
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [audio, publish])

  // Events from the shared <audio>, which the provider passes on.
  const onTimeUpdate = () => {
    publish()
    saveProgress()
  }
  const onPlay = () => { e.current.stopped = false; publish() }
  const onPause = () => { saveProgress(true); publish() }
  const onEnded = () => {
    const s = e.current
    // A stream the server let go of (paused for a long time, or a network that went
    // away) reads as ended too. If there is more disc, carry on from here.
    const total = lengthOf(s.segs) * s.loops
    if (s.disc && sample() < total - CD_RATE && s.reloads < 5) {
      s.reloads++
      loadAt(sample(), true)
      return
    }
    s.reloads = 0
    if (s.repeat !== 'off') {
      if (s.format === 'wav' && audio.current) audio.current.removeAttribute('src')
      s.segs = layout(s.disc!, streamTracks())
      loadAt(0, true)
      return
    }
    actions.stop()
  }
  // Keeps stalling: a connection too slow for lossless. Fall back rather than stutter.
  const onWaiting = () => {
    const s = e.current
    if (!s.disc || s.quality !== 'auto' || s.format === 'aac' || !s.disc.formats.includes('aac')) { publish(); return }
    const t = performance.now()
    if (t - s.loadedAt < 2500) return
    s.stalls = s.stalls.filter((x) => t - x < 30000)
    s.stalls.push(t)
    if (s.stalls.length >= 3) {
      s.stalls = []
      s.format = 'aac'
      toast('This connection is slow, so the disc has switched to compressed audio')
      rebuild(true)
    }
    publish()
  }
  // A format the browser turns out not to play after all: the next one down.
  const onError = () => {
    const s = e.current
    const a = audio.current
    if (!s.disc || !a?.src) return false
    const order: CdFormat[] = ['flac', 'wav', 'aac']
    if (a.error?.code === MediaError.MEDIA_ERR_NETWORK && s.reloads < 5) {
      s.reloads++
      loadAt(sample(), !a.paused)
      return true
    }
    const nextFormat = order.slice(order.indexOf(s.format) + 1).find((f) => s.disc!.formats.includes(f))
    if (nextFormat && a.error && a.error.code === MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED) {
      s.format = nextFormat
      rebuild(true)
      return true
    }
    toast('This disc can’t be played just now')
    publish()
    return true
  }

  useEffect(() => {
    const save = () => saveProgress(true)
    window.addEventListener('pagehide', save)
    return () => window.removeEventListener('pagehide', save)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return { now, actions, active, onTimeUpdate, onPlay, onPause, onEnded, onWaiting, onError, publish }
}

// Where the disc starts playing: index 1 of its first track, past any pregap.
function startOfDisc(first: CdTrack): number {
  return (first.start - first.pregap) * SECTOR_SAMPLES
}

function sampleOfLba(segs: Seg[], lba: number): number {
  for (const s of segs) {
    const end = s.lba + s.samples / SECTOR_SAMPLES
    if (lba >= s.lba && lba < end) return s.start + (lba - s.lba) * SECTOR_SAMPLES
  }
  return 0
}
