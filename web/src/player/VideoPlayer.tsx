// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, ApiError, episodeLabel, fitWidth, langName, mediaUrl, posterUrl, sized, type Item, type PlaybackAnswer, type Show } from '../api'
import { IconBack, IconClose, IconDevices, IconExpand, IconInfo, IconMini, IconNext, IconPause, IconPlay, IconRemote, IconUsers } from '../icons'
import { browserCapabilities } from './capabilities'
import PlayInfoPanel from './PlayInfoPanel'
import { toast } from '../state'
import { usePlayerChrome } from './chrome'
import PickIntroBar from './PickIntroBar'
import { skipAt, type Markers } from './markers'
import { Controls, useVideoState } from './Controls'
import { haveHls, toggleFullscreen, useIdle, useSource, type NetNote } from './hooks'
import { deviceStreaming, mayBeRobust, measureSpeed, rememberSpeed, setDeviceStreaming, type DeviceStreaming } from './streaming'
import Hls from 'hls.js'
import Remote, { RemoteBar, RemoteSheet, useSheetLayout, type RemoteKey } from './Remote'
import { keyWorks, remoteLabel, remoteNote, videoForm } from './remoteKeys'
import TrackPicker from './TrackPicker'
import ViewSettings from './ViewSettings'
import { useViewPrefs } from './viewPrefs'
import { getTvMode } from '../tvmode/mode'
import { registerPlayer, reportSoon, type RemoteCommand, type RemoteState } from '../remote/device'
import { usePartySync } from '../remote/party'
import { useRemoteAllowed } from '../remote/allowed'
import { setSpeed } from './speed'

// Opened from the player's top bar, and not before.
const PlayOn = lazy(() => import('../remote/PlayOn'))
const WatchParty = lazy(() => import('../remote/WatchParty'))

haveHls(Hls)

const MODE_WORDS: Record<string, string> = { direct: 'Direct play', remux: 'Rewrapped', transcode: 'Transcoding' }

const legacyRemote = () => { try { return localStorage.getItem('wms.remote.video') === '1' } catch { return false } }

export default function VideoPlayer({ item, startAt }: { item: Item; startAt: number | null }) {
  const nav = useNavigate()
  const chrome = usePlayerChrome()
  // A list being played through carries on with its next item.
  const search = new URLSearchParams(useLocation().search)
  const listId = search.get('collection')
  // Choosing where the intro ends, from the edit dialog.
  const pickIntro = search.get('pickIntro')
  const nextHref = `/play/${item.nextId}${listId ? `?collection=${encodeURIComponent(listId)}` : ''}`
  const mini = chrome.mini
  const view = useViewPrefs(item.id, legacyRemote())
  const prefs = view.prefs
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [picker, setPicker] = useState<'audio' | 'subtitle' | null>(null)
  const video = useRef<HTMLVideoElement | null>(null)
  const root = useIdle(prefs.hideAfter * 1000, [mini])
  const [info, setInfo] = useState<PlaybackAnswer | null>(null)
  const [infoOpen, setInfoOpen] = useState(false)
  // What has been asked of the server. Each change is a new request. A subtitle the
  // player draws itself changes with none: that's sub, which starts as the server's
  // choice. restartAt: a rewrapped stream only exists from where it started, so going
  // past what's been made asks for a new one, starting there.
  // reopen counts fresh requests for the same thing at the same place: a robust stream
  // that couldn't go on (the connection was gone, or the server forgot the stream).
  // Another device that started this here may have named a sound track and subtitles.
  const [want, setWant] = useState<{ audio?: number; burn?: { index: number; styled: boolean } | null; video?: number; flat3d?: boolean; restartAt?: number; convert?: boolean; reopen?: number }>(() => {
    const a = search.get('audio')
    return a != null && Number.isInteger(Number(a)) ? { audio: Number(a) } : {}
  })
  // What a robust stream's player did: shown for a moment, and listed in the information panel.
  const [note, setNote] = useState<NetNote | null>(null)
  const [netLog, setNetLog] = useState<string[]>([])
  const [streamPref, setStreamPref] = useState<DeviceStreaming>(deviceStreaming)
  const [sub, setSub] = useState<number | null>(() => {
    const t = search.get('subtitle')
    return t == null ? null : t === 'off' ? -1 : Number.isInteger(Number(t)) ? Number(t) : null
  })
  const [sendTo, setSendTo] = useState<'device' | 'party' | null>(null)
  const may = useRemoteAllowed() // either can be turned off for a profile
  const audio = want.audio ?? info?.audioIndex ?? 0
  const burn = want.burn
  const flat3d = !!want.flat3d
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const pendingSeek = useRef<number | null>(startAt ?? item.progress?.resumeSec ?? 0)
  const st = useVideoState(video)
  const offset = info?.timeOffsetSec || 0
  const now = st.time + offset
  // A copy of a disc's film gets that disc's remote. On a phone, and in the mini player, the
  // remote is a sheet, opened each time it's wanted rather than remembered.
  const form = videoForm(item)
  const copy = form !== 'video'
  const sheetLayout = useSheetLayout()
  const [sheetOpen, setSheetOpen] = useState(false)
  useEffect(() => setSheetOpen(false), [mini])
  const remoteOn = sheetLayout || mini ? sheetOpen : prefs.remote
  const setRemoteOn = (on: boolean) => { if (sheetLayout || mini) setSheetOpen(on); else view.update({ remote: on }, view.scope) }
  const [upNext, setUpNext] = useState<number | null>(null)
  const [next, setNext] = useState<{ item: Item; show?: Show | null; recommended: boolean } | null>(null)
  const [nextDismissed, setNextDismissed] = useState(false)
  const [ended, setEnded] = useState(false)
  const inParty = useRef(false)

  // "Watch next": the next episode of this show, or (end of a show / a
  // movie) a recommendation, loaded once the credits are near.
  const nearEnd = (info?.durationSec || item.runtimeSec || 0) > 0 && now >= (info?.durationSec || item.runtimeSec) - 30
  useEffect(() => {
    if ((!nearEnd && !ended) || next) return
    if (item.nextId) api.get<Item>(`/items/${item.nextId}`).then((it) => setNext({ item: it, recommended: false })).catch(() => {})
    else api.get<{ item: Item | null; show: Show | null }>(`/items/${item.id}/recommend`).then((r) => { if (r.item) setNext({ item: r.item, show: r.show, recommended: true }) }).catch(() => {})
  }, [nearEnd, ended, next, item.id, item.nextId])

  // Episodes: count down into the next one when this one ends.
  useEffect(() => {
    const v = video.current
    if (!v) return
    // In a watch party the room decides what comes next, for everyone at once.
    const onEnded = () => { setEnded(true); if (item.nextId && !inParty.current) setUpNext(8) }
    v.addEventListener('ended', onEnded)
    return () => v.removeEventListener('ended', onEnded)
  }, [item.nextId])
  useEffect(() => {
    if (upNext == null || nextDismissed || mini) return
    if (upNext <= 0) { nav(nextHref, { replace: true }); return }
    const t = setTimeout(() => setUpNext(upNext - 1), 1000)
    return () => clearTimeout(t)
  }, [upNext, item.nextId, nav, nextDismissed, mini])

  // What can be skipped, from every source the server knows (set by hand, a file beside the
  // video, chapter names, found by comparing episodes, the show's own intro). Until that
  // answers, and if it doesn't, the item's own intro.
  const [markers, setMarkers] = useState<Markers | null>(null)
  useEffect(() => {
    setMarkers(null)
    const stop = new AbortController()
    api.get<Markers>(`/media/${item.id}/markers`, stop.signal).then(setMarkers).catch(() => {})
    return () => stop.abort()
  }, [item.id])
  const ownIntro: Markers | null = item.introEnd != null ? { intro: { startSec: item.introStart || 0, endSec: item.introEnd } } : null
  const skip = skipAt(markers || ownIntro, now, item.runtimeSec || 0)

  useEffect(() => {
    setInfo(null)
    let current = true
    let retry = 0
    const start = want.restartAt ?? pendingSeek.current
    const body: Record<string, unknown> = { profile: 'web', capabilities: browserCapabilities(), startSec: start ?? 0, flatten3d: flat3d }
    if (want.audio != null) body.audio = want.audio
    if (burn) { body.subtitle = burn.index; body.burnSubtitle = burn.styled } else if (sub != null) body.subtitle = sub
    if (want.video != null) body.video = want.video
    if (want.convert) body.forceTranscode = true
    const device = deviceStreaming()
    if (device !== 'auto') body.streaming = device
    // A robust stream starts at a rate chosen from what this connection carries.
    const ask = async (wait: number): Promise<void> => {
      if (await mayBeRobust()) body.measuredKbps = await measureSpeed()
      try {
        const a = await api.post<PlaybackAnswer>(`/items/${item.id}/playback`, body)
        if (!current) return
        setInfo(a)
        if (want.reopen) setNote((n) => (n?.kind === 'lost' ? { kind: 'back', text: 'Reconnected, at the same place.' } : n))
        setSub((s) => (s == null ? a.subtitleIndex : s))
        // A stream starts where it was asked to; a file or a full stream seeks once open.
        pendingSeek.current = a.seek === 'restart' ? null : (a.startAtSec || null)
      } catch (e) {
        if (!current) return
        // Picking up after a dropped connection: keep asking, waiting longer each time, rather
        // than stop with an error the moment the connection is still away.
        if (want.reopen && !(e instanceof ApiError && e.status >= 400 && e.status < 500)) {
          setNote({ kind: 'lost', text: 'Connection lost. Trying again…' })
          retry = window.setTimeout(() => { ask(Math.min(wait * 2, 8000)) }, wait)
          return
        }
        setErr((e as Error).message)
      }
    }
    ask(1000)
    return () => { current = false; clearTimeout(retry) }
  }, [item.id, want]) // eslint-disable-line react-hooks/exhaustive-deps

  // Subtitles the player draws: the chosen track, with its cues in the player's time.
  const burned = info?.subtitleDelivery === 'burn'
  const shownSub = info && sub != null && sub >= 0 && !burned ? info.subtitles.find((s) => s.index === sub) : undefined
  const subUrl = shownSub?.url || ''
  useEffect(() => {
    const v = video.current
    if (!v || !subUrl) return
    // Fetched and handed over as a local file: a <track> can't send a sign-in, and an app
    // whose page isn't on the server's own address couldn't load one directly at all.
    let stopped = false
    let track: HTMLTrackElement | null = null
    let local = ''
    fetch(mediaUrl(subUrl)).then((r) => { if (!r.ok) throw new Error(String(r.status)); return r.text() }).then((vtt) => {
      if (stopped) return
      local = URL.createObjectURL(new Blob([vtt], { type: 'text/vtt' }))
      track = document.createElement('track')
      track.kind = 'subtitles'
      track.src = local
      track.srclang = shownSub?.language?.slice(0, 2) || 'und'
      track.label = shownSub?.title || shownSub?.languageName || 'Subtitles'
      track.default = true
      v.appendChild(track)
      track.track.mode = 'showing'
      // Clear of the controls, which sit over the bottom of the picture.
      const t = track
      t.addEventListener('load', () => { for (const c of Array.from(t.track.cues || [])) { const cue = c as VTTCue; if (cue.line === 'auto') cue.line = -3 } })
    }).catch(() => toast('These subtitles couldn’t be loaded.'))
    return () => {
      stopped = true
      if (track) { track.track.mode = 'disabled'; track.remove() }
      if (local) URL.revokeObjectURL(local)
    }
  }, [subUrl]) // eslint-disable-line react-hooks/exhaustive-deps

  // Done with a stream (left the player, another audio or subtitle track):
  // the server can stop converting it now rather than after a while.
  const streamId = info?.streamId || ''
  useEffect(() => {
    if (!streamId) return
    return () => { api.del(`/hls/${streamId}`).catch(() => {}) }
  }, [streamId])

  // A file, or a full stream with its whole length listed ahead, seeks anywhere. A
  // rewrapped stream seeks within what has been made so far, and otherwise starts anew.
  const rawSeek = useCallback((t: number) => {
    const v = video.current
    if (!v) return
    const to = Math.max(0, Math.min(t, (info?.durationSec || v.duration || t)))
    if (info?.seek !== 'restart') { v.currentTime = to; return }
    const local = to - offset
    let made = 0
    try { if (v.seekable.length) made = v.seekable.end(v.seekable.length - 1) } catch { /* not open yet */ }
    if (local >= 0 && local <= made - 1) { v.currentTime = local; return }
    setWant((w) => ({ ...w, restartAt: to }))
  }, [info, offset])

  // A watch party playing this: the room says where the picture is, and a seek here is a
  // seek for everyone (docs/SYNCPLAY.md). Otherwise seeking is this player's own business.
  const party = usePartySync(video, item.id, { seek: rawSeek, offset: () => offset, loaded: () => !!info, blocked: () => setNeedClick(true), cheapSeek: () => info?.seek !== 'restart' })
  inParty.current = party.active
  const seek = useCallback((t: number) => { if (party.active) party.seek(t); else rawSeek(t) }, [party.active, rawSeek]) // eslint-disable-line react-hooks/exhaustive-deps

  // Robust streams: the player says what it did, and a stream that can't go on is asked
  // for again at the same place. A file that stalled is asked for as a steadier stream.
  const robust = info?.streaming?.mode === 'robust' ? info.streaming.player : null
  const lastNote = useRef<NetNote['kind'] | null>(null)
  const onNote = useCallback((n: NetNote) => {
    lastNote.current = n.kind
    setNote(n)
    if (n.kind !== 'lost') setNetLog((l) => [...l.slice(-7), n.text])
  }, [])
  useEffect(() => {
    if (!note || note.kind === 'lost') return
    const t = setTimeout(() => setNote(null), 6000)
    return () => clearTimeout(t)
  }, [note])
  const hlsRef = useSource(video, info?.url || null, {
    startAt: () => pendingSeek.current,
    growing: info?.seek === 'restart',
    robust, measuredKbps: info?.streaming?.measuredKbps, startHeight: info?.streaming?.ladder?.[0]?.height, onNote,
    onReopen: (at) => {
      const est = hlsRef.current?.bandwidthEstimate
      if (est) rememberSpeed(est / 1000) // what the connection was doing, for the new stream's start
      pendingSeek.current = at + offset
      const steadier = lastNote.current === 'stall' && info?.decision !== 'transcode'
      setWant((w) => ({ ...w, restartAt: undefined, reopen: (w.reopen || 0) + 1, convert: w.convert || steadier }))
    },
    onReady: () => {
      const v = video.current
      if (!v) return
      if (pendingSeek.current && pendingSeek.current > 0 && Math.abs(v.currentTime - pendingSeek.current) > 1) { v.currentTime = pendingSeek.current }
      pendingSeek.current = null
      // In a watch party the room says when to start; see usePartySync.
      if (!inParty.current) v.play().catch(() => setNeedClick(true))
    },
    onError: (m) => { if (!convertInstead()) setErr(m) },
  })

  // Browsers sometimes say they can play what they then can't, or find a file's codecs
  // aren't what its first bytes suggested. Rather than an error, it's converted, once.
  const convertInstead = () => {
    if (want.convert || !info || info.decision === 'transcode') return false
    const v = video.current
    pendingSeek.current = v ? v.currentTime + offset : 0
    setErr('')
    setWant((w) => ({ ...w, restartAt: undefined, convert: true }))
    return true
  }
  useEffect(() => {
    const v = video.current
    if (!v) return
    const onFail = () => {
      if (robust && v.error?.code === MediaError.MEDIA_ERR_NETWORK) return // asked for again at the same place (useSource)
      if (v.error && !convertInstead()) setErr(v.error.message || 'This file can’t be played here.')
    }
    v.addEventListener('error', onFail)
    return () => v.removeEventListener('error', onFail)
  })

  // Progress reporting.
  useEffect(() => {
    const send = () => {
      const v = video.current
      if (!v || !info || v.currentTime + (info.timeOffsetSec || 0) < 5) return
      api.post(`/items/${item.id}/progress`, { positionSec: v.currentTime + (info.timeOffsetSec || 0), durationSec: info.durationSec || v.duration }).catch(() => {})
    }
    const t = setInterval(send, 10000)
    window.addEventListener('beforeunload', send)
    return () => { clearInterval(t); window.removeEventListener('beforeunload', send); send() }
  }, [item.id, info])

  // The picture has the arrow keys until the viewer goes to the controls.
  useEffect(() => { if (!mini) root.current?.focus({ preventScroll: true }) }, [mini, root])

  // Keyboard and TV remotes (not while minimized: the app owns the keys then).
  useEffect(() => {
    if (mini) return
    const onKey = (e: KeyboardEvent) => {
      const v = video.current
      if (!v || picker || settingsOpen || infoOpen) return
      if (document.querySelector('.modal-backdrop, .scene-panel')) return // a window over the player has the keys
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
      if (e.key.startsWith('Arrow') && (e.target as HTMLElement)?.closest?.('.bottom, .top, .remote, .watch-next')) return
      switch (e.key) {
        case ' ': case 'k': case 'MediaPlayPause': e.preventDefault(); v.paused ? v.play() : v.pause(); break
        case 'ArrowRight': seek(v.currentTime + offset + 10); break
        case 'ArrowLeft': seek(v.currentTime + offset - 10); break
        case 'ArrowUp': v.volume = Math.min(1, v.volume + 0.1); break
        case 'ArrowDown': v.volume = Math.max(0, v.volume - 0.1); break
        case 'f': toggleFullscreen(root.current); break
        case 'm': v.muted = !v.muted; break
        case 'r': setRemoteOn(!remoteOn); break
        case 'i': setInfoOpen((o) => !o); break
        case 'n': case 'MediaTrackNext': if (copy) onRemote('next'); break
        case 'p': case 'MediaTrackPrevious': if (copy) onRemote('prev'); break
        // Back leaves the player. In TV mode that stops it, as a TV app does; otherwise it carries on in the corner.
        case 'Escape': case 'GoBack': case 'BrowserBack': if (!document.fullscreenElement) { e.preventDefault(); if (getTvMode()) chrome.close(); else chrome.minimize() } break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  // A new request that keeps the place: a change of track shouldn't lose it.
  const ask = (change: Partial<typeof want>) => {
    const v = video.current
    pendingSeek.current = v ? v.currentTime + offset : 0
    setWant((w) => ({ ...w, restartAt: undefined, ...change }))
  }
  // Styled copies of a subtitle sit in the list as its number plus this.
  const STYLED = 10000
  const switchTrack = (kind: 'audio' | 'sub', id: number) => {
    if (kind === 'audio') { ask({ audio: id }); return }
    if (id >= STYLED) { setSub(id - STYLED); ask({ burn: { index: id - STYLED, styled: true } }); return }
    const delivery = info?.subtitles.find((s) => s.index === id)?.delivery
    setSub(id)
    if (id >= 0 && (delivery === 'burn' || !delivery)) { ask({ burn: { index: id, styled: false } }); return }
    // Text the player draws. A stream that had one drawn in goes back to none.
    if (burned) ask({ burn: null })
  }
  const duration = info?.durationSec || item.runtimeSec || 0
  const chapters = info?.chapters?.map((c) => c.start) || []
  // A copy of a disc steps through its chapters, as the disc would. At the last one, next goes
  // on to the next episode, if there is one.
  const chapterStep = copy && chapters.length > 1
  const stepChapter = (dir: 1 | -1) => {
    const v = video.current
    if (!v) return
    const t = v.currentTime + offset
    const cur = Math.max(0, chapters.filter((c) => c <= t + 1).length - 1)
    if (dir === -1) { seek(t - chapters[cur] > 3 || cur === 0 ? chapters[cur] : chapters[cur - 1]); return }
    if (cur < chapters.length - 1) seek(chapters[cur + 1])
    else if (item.nextId) nav(nextHref)
  }
  // The same remote as for discs. What a key does without a disc's program is in remoteKeys.ts.
  const onRemote = (k: RemoteKey) => {
    const v = video.current
    switch (k) {
      case 'eject': chrome.close(); break
      case 'audio': if ((info?.audio.length || 0) > 1) setPicker('audio'); break
      case 'subtitle': if (info?.subtitles.length) setPicker('subtitle'); break
      case 'prev': if (chapterStep) stepChapter(-1); else if (v) seek(v.currentTime + offset - 60); break
      case 'rew': if (v) seek(v.currentTime + offset - 10); break
      case 'ff': if (v) seek(v.currentTime + offset + 30); break
      case 'next': if (chapterStep) stepChapter(1); else if (item.nextId) nav(nextHref); else if (v) seek(v.currentTime + offset + 60); break
      case 'play': v?.play().catch(() => {}); break
      case 'pause': v?.pause(); break
      case 'stop': if (v) { v.pause(); seek(0) } break
      default:
        if (k.startsWith('digit') && keyWorks(form, k, remoteState)) seek(chapters[Number(k.slice(5)) - 1])
    }
  }
  const remoteState = { audio: info?.audio.length, subtitles: info?.subtitles.length, chapters: chapters.length }
  const remoteEnabled = (k: RemoteKey) => keyWorks(form, k, remoteState)
  const keysNote = remoteNote(form, remoteState)
  const label = remoteLabel(form)
  const subtitle = [item.showTitle, episodeLabel(item)].filter(Boolean).join(' · ')
  const trackLabel = (t: { language: string; title?: string }) => [langName(t.language), t.title].filter(Boolean).join(' · ')
  const togglePlay = () => { const v = video.current; if (v && !needClick) v.paused ? v.play().catch(() => {}) : v.pause() }

  // Another device controlling this one (docs/REMOTE-PROTOCOL.md): what it may ask of this
  // player, and what this player says it is doing. Track ids are the playback answer's own.
  const remote = useRef<{ command: (c: RemoteCommand) => void; state: () => RemoteState } | null>(null)
  remote.current = {
    command: (c) => {
      const v = video.current
      if (!v) throw new Error('The player isn’t ready yet.')
      switch (c.type) {
        case 'pause': v.pause(); break
        case 'resume': v.play().catch(() => {}); break
        case 'stop': chrome.close(); break
        case 'seek': seek(c.positionSec != null ? c.positionSec : v.currentTime + offset + (c.bySec || 0)); break
        case 'volume': v.volume = Math.max(0, Math.min(1, c.level ?? v.volume)); break
        case 'mute': v.muted = !!c.muted; break
        case 'rate': setSpeed(v, c.rate || 1); break
        case 'audio': {
          const id = Number(c.audio)
          if (!info?.audio.some((a) => a.index === id)) throw new Error('That sound track isn’t on this title.')
          switchTrack('audio', id)
          break
        }
        case 'subtitle': {
          const id = c.subtitle === 'off' ? -1 : Number(c.subtitle)
          if (id >= 0 && !info?.subtitles.some((x) => x.index === id)) throw new Error('Those subtitles aren’t on this title.')
          switchTrack('sub', id)
          break
        }
        default: throw new Error('This device can’t do that.')
      }
    },
    state: () => {
      const v = video.current
      return {
        status: !v || !info ? 'buffering' : v.paused ? 'paused' : st.waiting ? 'buffering' : 'playing',
        itemId: item.id, positionSec: v ? v.currentTime + offset : 0, durationSec: duration, rate: v?.playbackRate || 1, volume: v?.volume, muted: v?.muted,
        audio: String(audio), subtitle: sub == null || sub < 0 ? 'off' : String(sub),
        audioTracks: info?.audio.map((a) => ({ id: String(a.index), label: trackLabel(a), lang: a.language })),
        subtitleTracks: info?.subtitles.map((x) => ({ id: String(x.index), label: trackLabel(x), lang: x.language })),
        screen: 'player',
      }
    },
  }
  useEffect(() => {
    const v = video.current
    const done = registerPlayer({ itemId: item.id, command: (c) => remote.current!.command(c), state: () => remote.current!.state() })
    const changes = ['play', 'pause', 'seeked', 'ratechange', 'volumechange', 'ended', 'waiting', 'playing']
    changes.forEach((e) => v?.addEventListener(e, reportSoon))
    return () => { changes.forEach((e) => v?.removeEventListener(e, reportSoon)); done() }
  }, [item.id])
  useEffect(() => { reportSoon() }, [info, audio, sub])
  const openOver = (what: 'device' | 'party') => { if (document.fullscreenElement) document.exitFullscreen().catch(() => {}); setSendTo(what) }
  const waitingFor = party.room?.state.waiting ? party.room.members.filter((m) => !m.you && (m.state === 'loading' || m.state === 'buffering')).map((m) => m.name) : null
  return (
    <>
    <div className={'player' + (mini ? ' mini' : '')} ref={root} tabIndex={-1} onClick={togglePlay}
      onDoubleClick={() => (mini ? chrome.expand() : toggleFullscreen(root.current))}>
      <div className="stage">
        <video ref={video} playsInline preload="auto" style={{ objectFit: prefs.picture === 'fill' ? 'cover' : 'contain' }} />
      </div>
      {st.waiting && !needClick && <div className="center-spinner"><div className="spinner" /></div>}
      {note && !mini && <div className={'net-note' + (note.kind === 'lost' ? ' lost' : '')} role="status" aria-live="polite">{note.text}</div>}
      {needClick && <div className="big-play"><button onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {waitingFor && !mini && <div className="wp-note" role="status" aria-live="polite">{waitingFor.length ? `Waiting for ${waitingFor.join(', ')}` : 'Waiting for everyone to be ready'}</div>}
      {err && <div className="overlay-msg"><div className="box">Playback failed: {err}<br /><button className="btn" style={{ marginTop: 10 }} onClick={chrome.close}>Back</button></div></div>}
      {mini ? (
        <div className="mini-bar" onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
          <div className="mini-info" onClick={chrome.expand}>
            <div className="mini-title">{item.showTitle || item.title}</div>
            <div className="mini-sub">{item.showTitle ? [episodeLabel(item), item.title].filter(Boolean).join(' · ') : item.year}</div>
          </div>
          <div className="mini-btns">
            <button className="ibtn" onClick={() => onRemote('rew')} title="Back 10 s"><IconBack /></button>
            <button className="ibtn big" onClick={togglePlay} title="Play/Pause">{st.playing ? <IconPause /> : <IconPlay />}</button>
            {item.nextId ? <button className="ibtn" onClick={() => nav(nextHref)} title="Next episode"><IconNext /></button> : <span />}
            <button className={'ibtn keep' + (remoteOn ? ' on' : '')} onClick={() => setRemoteOn(!remoteOn)} title="Remote" aria-label="Remote" aria-pressed={remoteOn}><IconRemote /></button>
            <button className="ibtn" onClick={chrome.expand} title="Full player"><IconExpand /></button>
            <button className="ibtn" onClick={chrome.close} title="Stop"><IconClose /></button>
          </div>
          {remoteOn && <RemoteSheet docked onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label={label} note={keysNote} />}
        </div>
      ) : (
        <>
          <div className="top" onClick={(e) => e.stopPropagation()}>
            <button className="ibtn" onClick={chrome.minimize} title="Back (keeps playing in the mini player)"><IconBack /></button>
            <div className="grow"><h1>{item.title}</h1><div className="sub">{[subtitle || item.year, info && MODE_WORDS[info.decision]].filter(Boolean).join(' · ')}</div></div>
            {info && <button className="ibtn" onClick={() => setInfoOpen(true)} title="How this is playing (I)" aria-label="How this is playing"><IconInfo /></button>}
            {(may.party || party.room) && <button className={'ibtn' + (party.active ? ' on' : '')} onClick={() => openOver('party')} title="Watch party" aria-label={party.room ? `Watch party, ${party.room.members.length} watching` : 'Watch party'}><IconUsers /></button>}
            {may.playOn && <button className="ibtn" onClick={() => openOver('device')} title="Play on another device" aria-label="Play on another device"><IconDevices /></button>}
            {item.nextId && !party.active && <button className="btn sm" onClick={() => nav(nextHref, { replace: true })} title="Next episode"><IconNext /> <span className="hide-narrow">Next episode</span></button>}
            <button className="ibtn" onClick={chrome.minimize} title="Mini player"><IconMini /></button>
          </div>
          {remoteOn && (sheetLayout
            ? <RemoteSheet onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label={label} note={keysNote} />
            : prefs.remoteStyle === 'bar'
              ? <RemoteBar onKey={onRemote} enabled={remoteEnabled} disc={copy} note={keysNote} />
              : <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label={label} note={keysNote} />)}
          {skip && <button className="btn skip-intro" onClick={(e) => { e.stopPropagation(); seek(skip.to) }}>{skip.label} ›</button>}
          {pickIntro && <PickIntroBar item={item} time={now} mode={pickIntro} />}
          {next && (nearEnd || ended) && !nextDismissed && !party.active && (
            <div className="watch-next" onClick={(e) => e.stopPropagation()}>
              <div className="wn-head">{next.recommended
                ? (next.show ? `Because you watched ${item.showTitle || item.title}` : 'You might also like')
                : upNext != null ? `Next episode in ${upNext}s` : 'Watch next'}</div>
              <div className="wn-body" onClick={() => nav(`/play/${next.item.id}`, { replace: true })}>
                <div className={'wn-thumb' + (next.item.showId != null ? ' wide' : '')}>{next.item.poster && <img src={sized(posterUrl(next.item), fitWidth(240))} alt="" />}</div>
                <div>
                  <div className="wn-title">{next.show ? next.show.title : next.item.showTitle || next.item.title}</div>
                  <div className="muted" style={{ fontSize: 12 }}>{next.item.showId != null ? [episodeLabel(next.item), next.item.title].filter(Boolean).join(' · ') : next.item.year}</div>
                </div>
              </div>
              <div className="wn-actions">
                <button className="btn sm primary" onClick={() => nav(`/play/${next.item.id}`, { replace: true })}><IconPlay /> Play</button>
                <button className="btn sm ghost" onClick={() => { setNextDismissed(true); setUpNext(null) }}>Dismiss</button>
              </div>
            </div>
          )}
          <Controls video={video} duration={duration} offset={offset} onSeek={seek} onFullscreen={() => toggleFullscreen(root.current)}
            chapters={chapters} chapterNames={info?.chapters?.map((c) => c.title)} copy={copy}
            onPrev={chapterStep ? () => stepChapter(-1) : undefined} onNext={chapterStep ? () => stepChapter(1) : undefined}
            audio={info?.audio.map((a) => ({ id: a.index, label: [langName(a.language), a.title, a.codec?.toUpperCase(), a.channels ? a.channels + 'ch' : ''].filter(Boolean).join(' · '), active: a.index === audio }))}
            subtitles={info?.subtitles.map((s) => ({ id: s.index, label: trackLabel(s), active: s.index === sub }))}
            onAudio={(id) => switchTrack('audio', id)} onSubtitle={(id) => switchTrack('sub', id)} onPick={(k) => { if (k !== 'angle') setPicker(k) }}
            showSeek={prefs.seekBar} remoteOn={remoteOn} onRemote={() => setRemoteOn(!remoteOn)} onSettings={() => setSettingsOpen((o) => !o)} />
          {settingsOpen && <ViewSettings prefs={prefs} scope={view.scope} isDisc={false} onClose={() => setSettingsOpen(false)} onChange={(p) => view.update(p, view.scope)} onScope={view.setScope}
            streaming={streamPref} onStreaming={(v) => { setDeviceStreaming(v); setStreamPref(v); ask({ reopen: (want.reopen || 0) + 1 }) }} />}
          {picker && info && (
            <TrackPicker heading={picker === 'audio' ? 'Audio & language' : 'Subtitles'} onClose={() => setPicker(null)}
              onPick={(id) => switchTrack(picker === 'audio' ? 'audio' : 'sub', id)}
              tiles={picker === 'audio'
                ? info.audio.map((a) => ({ id: a.index, title: langName(a.language), sub: a.title, detail: [a.codec?.toUpperCase(), a.channels ? a.channels + ' ch' : ''].filter(Boolean).join(' · '), active: a.index === audio }))
                : [{ id: -1, title: 'Off', active: sub == null || sub < 0 }, ...info.subtitles.flatMap((x) => {
                  const plain = { id: x.index, title: langName(x.language), sub: x.title, detail: x.codec?.toUpperCase(), active: x.index === sub && !burn?.styled }
                  // ASS can carry placement and styling that plain text loses: keeping it means drawing it into the picture.
                  return x.codec === 'ass' || x.codec === 'ssa' ? [plain, { id: x.index + STYLED, title: langName(x.language), sub: 'With its styling', detail: 'Drawn into the picture', active: x.index === sub && !!burn?.styled }] : [plain]
                })]} />
          )}
          {infoOpen && info && <PlayInfoPanel info={info} netLog={netLog} flattened={flat3d} onClose={() => setInfoOpen(false)} onFlat3d={(on) => ask({ flat3d: on })} onVideo={(i) => ask({ video: i })} />}
        </>
      )}
    </div>
    {/* Beside the player, not inside it: a click in a dialog isn't a click on the picture. */}
    {sendTo && (
      <Suspense fallback={null}>
        {sendTo === 'device'
          ? <PlayOn onClose={() => setSendTo(null)} source={{ kind: 'playing', itemId: item.id, title: item.title, positionSec: () => (video.current?.currentTime || 0) + offset, audio: String(audio), subtitle: sub == null || sub < 0 ? 'off' : String(sub), stop: chrome.close }} />
          : <WatchParty onClose={() => setSendTo(null)} item={{ id: item.id, title: item.title, positionSec: () => (video.current?.currentTime || 0) + offset }} />}
      </Suspense>
    )}
    </>
  )
}
