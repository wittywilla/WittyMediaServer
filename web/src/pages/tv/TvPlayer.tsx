// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getTvMode } from '../../tvmode/mode'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import Hls from 'hls.js'
import { api, type StreamWork } from '../../api'
import { announce } from '../../a11y'
import { IconBack, IconChannels, IconDown, IconHistory, IconPlay, IconRefresh, IconTv, IconUp } from '../../icons'
import { Controls, useCompactBar } from '../../player/Controls'
import { haveHls, toggleFullscreen, useIdle } from '../../player/hooks'
import Remote, { type RemoteKey } from '../../player/Remote'
import { Dialog } from '../../ui'
import { useLogo } from '../../components/BrandLogo'
import LegalNote from '../../components/LegalNote'
import BackgroundPlayChoice from './BackgroundPlayChoice'
import { ChannelLogo } from './ChannelLogo'
import { channelVars, previousChannel, rememberChannel, surfaceOf, themeRoot, useLook, type Look } from './look'
import MoreInfo from './MoreInfo'
import MiniGuide from './MiniGuide'
import Slate from './Slate'
import { useLiveTracks } from './tracks'
import { liveCatchUp, liveHls, liveVideo, retune, setLive, setLiveCatchUp, useLive } from './live'
import QualityMenu, { StreamInfo } from '../../player/QualityMenu'
import { describe, here, noteAway, setDeviceDefault, setShowStreamInfo, showStreamInfo, sizeName, streamLines } from '../../player/quality'
import { fmtClock, fmtSpan, getLineup, type Alternative, type GuideChannel, type GuidePage, type TuneInfo } from './tvApi'
import { catchupKey } from './guideLogic'
import { breakAt, isLibraryKey, knownEndAfter, LIVE_EDGE_SEC, liveNowFrom, planSeek, resumePlan, startOverPath, type Timeline } from './timeshift'
import { useTvRemote } from '../../remote/tvTarget'
import './tv.css'
import './tvthemes.css'
import { t, tx } from '../../i18n'

// How long the channel banner stays up after a channel is chosen.
const BANNER_MS = 3000

haveHls(Hls)

// Watching a channel: the app's player and on-screen remote, with channel keys
// and the classic banner that drops in on every tune. Everyone on a channel
// shares one stream; pausing keeps a place in the server's buffer. The picture is
// the TV section's one player (live.ts), carried over from the guide's preview
// and back, so changing where it's shown never tunes again.
//
// With ?from=&to= it plays that programme from its start instead (tv-catchup): the viewer's own
// stream from the channel's archive, which the banner says, and Back to live leaves. A library
// channel starts over the same way, on a stream of this viewer's own that is the channel's schedule
// played behind the clock (#648): its program date-times say where in the programme the picture
// is, so the bar is the programme's, a break can be skipped, and rewinding further back than the
// server keeps, or coming back from a long pause, starts that stream again from the moment wanted.

const REMOTE_KEY = 'wms.remote.tv'
const remoteSaved = () => { try { return localStorage.getItem(REMOTE_KEY) === '1' } catch { return false } }

export default function TvPlayer() {
  const { key = '' } = useParams()
  const nav = useNavigate()
  // Changing channel takes the place of the channel left, and remembers where they all came from.
  const from = useLocation().state as { fromGuide?: boolean } | null
  const [params] = useSearchParams()
  const cuStart = Number(params.get('from')) || 0, cuEnd = Number(params.get('to')) || 0
  const fromStart = cuStart > 0 && cuEnd > cuStart
  // What the TV section's one player is asked for: the channel, or this programme of it from its start.
  const liveKey = fromStart ? catchupKey(key, cuStart, cuEnd) : key
  // Started over on a library channel: the viewer's own shifted stream.
  const shifted = fromStart && isLibraryKey(key)
  const video = useRef<HTMLVideoElement | null>(liveVideo())
  const stage = useRef<HTMLDivElement | null>(null)
  const root = useIdle(4000, [])
  // A phone or tablet gets the simple bar, as films do; the channel keys go into its Options sheet.
  const compact = useCompactBar()
  const live = useLive(liveKey, stage)
  const mine = live.key === liveKey
  const info = mine ? live.info : null
  useEffect(() => { noteAway(info?.quality?.away) }, [info])
  const err = mine ? live.err : ''
  // Library channels can go back further than the window (start over); a tuner can't.
  const canShift = isLibraryKey(key) && (shifted || !!info?.startOver)
  const [tl, setTl] = useState<Timeline | null>(null)
  const heardAt = useRef(Date.now() / 1000)
  useEffect(() => { setTl(info?.timeline ?? null); heardAt.current = Date.now() / 1000 }, [info])
  // The schedule's moment at media time t, from the stream's program date-times: hls.js knows the
  // one playing; a browser playing HLS itself, where the stream's dates start.
  const schedOf = (t: number): number | null => {
    const v = video.current
    if (!v) return null
    const d = liveHls()?.playingDate
    if (d) return d.getTime() / 1000 + (t - v.currentTime)
    const sd = (v as HTMLVideoElement & { getStartDate?: () => Date }).getStartDate?.()
    return sd && !isNaN(sd.getTime()) ? sd.getTime() / 1000 + t : null
  }
  // Where live is now, by the server's clock.
  const liveNow = () => info ? liveNowFrom(info.serverTime, heardAt.current, Date.now() / 1000) : Date.now() / 1000
  const picture = mine && live.playing
  const tracks = useLiveTracks(video.current, liveHls, mine && live.ready)
  const [needClick, setNeedClick] = useState(false)
  const [banner, setBanner] = useState(true)
  const [remoteOn, setRemoteOnState] = useState(remoteSaved)
  const [dial, setDial] = useState('')
  // sched: the moment of the channel's schedule the picture is at, from the program date-times.
  const [win, setWin] = useState<{ start: number; end: number; behind: number; sched: number | null }>({ start: 0, end: 0, behind: 0, sched: null })
  const [alt, setAlt] = useState<Alternative | null>(null)
  const [mini, setMini] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [qualityOpen, setQualityOpen] = useState(false)
  const [statsOn, setStatsOn] = useState(showStreamInfo)
  // What converts the channel (GPU or CPU), asked again every few seconds while it shows.
  const [work, setWork] = useState<StreamWork | null>(null)
  useEffect(() => {
    setWork(null)
    if (!statsOn || !live.url) return
    const q = new URL(live.url, location.href).searchParams.get('quality')
    const path = `/tv/${encodeURIComponent(liveKey)}/live/work${q ? '?quality=' + encodeURIComponent(q) : ''}`
    const ask = () => { api.get<StreamWork>(path).then(setWork).catch(() => {}) }
    ask()
    const t = setInterval(ask, 2000)
    return () => clearInterval(t)
  }, [statsOn, live.url, liveKey])
  // Shown the moment a channel is chosen, from what the list already knows: the
  // number and name don't wait for the stream.
  const [zap, setZap] = useState<GuideChannel | null>(null)
  const lastKey = useRef<string | null>(previousChannel() || null)
  const prevKey = useRef('')
  const bannerTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const look = useLook()

  // The channel banner shows for three seconds after a channel is chosen, then gets out of the way.
  const showBanner = useCallback(() => {
    setBanner(true)
    clearTimeout(bannerTimer.current)
    bannerTimer.current = setTimeout(() => setBanner(false), BANNER_MS)
  }, [])
  useEffect(() => () => clearTimeout(bannerTimer.current), [])

  // The watch view is watched with sound, whatever the preview had.
  useLayoutEffect(() => {
    const v = video.current
    if (!v) return
    v.muted = false
    v.removeAttribute('aria-hidden')
    v.removeAttribute('tabindex')
  }, [])
  useEffect(() => {
    const v = video.current
    if (v) v.setAttribute('aria-label', info ? t('Channel {number}, {name}', { number: info.number, name: info.name }) : t('TV'))
  }, [info])

  // Each channel: the banner straight away, from what the list already knows. The stream
  // itself is tuned by the TV section's player, which waits for the first picture so a
  // refused or broken channel says why.
  useEffect(() => {
    if (prevKey.current && prevKey.current !== key) lastKey.current = prevKey.current
    prevKey.current = key
    setNeedClick(false); setAlt(null)
    showBanner()
    rememberChannel(key)
    setZap(null)
    getLineup().then((l) => { if (prevKey.current === key) setZap(l.find((c) => c.key === key) || null) }).catch(() => {})
  }, [key, liveKey, showBanner])
  useEffect(() => {
    if (!info) return
    announce(t('Channel {number}, {name}', { number: info.number, name: info.name }) + (info.now?.title ? ', ' + info.now.title : '') + (info.notices?.length ? '. ' + info.notices.map((n) => tx(n)).join(' ') : ''))
  }, [info?.key]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!err) return
    // A channel refused now may have just shown itself to be encrypted: the guide says what to watch instead.
    api.get<GuidePage>(`/guide?hours=1&limit=1&channel=${encodeURIComponent(key)}`).then((g) => { if (prevKey.current === key) setAlt(g.channels[0]?.alternative || null) }).catch(() => {})
  }, [err, key])
  // Ready, or already playing in the preview: play, or ask for a click if the browser won't.
  useEffect(() => {
    if (!mine || !live.ready) return
    video.current?.play().catch(() => setNeedClick(true))
  }, [mine, live.ready])
  useEffect(() => { if (picture) showBanner() }, [picture, showBanner])
  // From the start means from the start, wherever the stream's playlist would have a player begin.
  const startedAt = useRef('')
  useEffect(() => {
    const v = video.current
    if (!fromStart || !mine || !live.ready || !v || startedAt.current === liveKey) return
    const toStart = () => {
      if (!v.seekable.length) return
      startedAt.current = liveKey
      if (v.currentTime - v.seekable.start(0) > 5) v.currentTime = v.seekable.start(0)
    }
    if (v.seekable.length) toStart()
    else { v.addEventListener('loadedmetadata', toStart, { once: true }); return () => v.removeEventListener('loadedmetadata', toStart) }
  }, [fromStart, mine, live.ready, liveKey])

  // When a programme ends the TV section's player asks what's on next, and the banner moves on.
  const nowStart = info?.now?.start
  const seenStart = useRef(nowStart)
  useEffect(() => {
    if (nowStart !== undefined && seenStart.current !== undefined && nowStart !== seenStart.current) showBanner()
    seenStart.current = nowStart
  }, [nowStart, showBanner])

  // Counting towards watch progress, on the channels set up for it.
  useEffect(() => {
    if (!info?.countProgress || !info.now?.itemId) return
    const p = info.now
    const at = info.positionSec, atTime = info.serverTime, stop = info.partEnd || Infinity
    const send = () => {
      const v = video.current
      if (!v || v.paused) return
      // Only this stretch counts: a break after it isn't the programme.
      const pos = at + (Math.min(Date.now() / 1000, stop) - atTime)
      api.post(`/items/${p.itemId}/progress`, { positionSec: pos, durationSec: p.end - p.start }).catch(() => {})
    }
    const t = setInterval(send, 30000)
    return () => { clearInterval(t); send() }
  }, [info?.countProgress, info?.now, info?.positionSec, info?.serverTime, info?.partEnd])

  // How far back the buffer goes, and how far behind live this player is. Behind is counted from
  // where live sync keeps everyone on the channel, not from the end of the seekable range: a live
  // stream always plays a hold-back (three segments or more) short of that end, so counting from
  // it, a pause or a press of -10 never looked behind and Live didn't show.
  useEffect(() => {
    const t = setInterval(() => {
      const v = video.current
      if (!v || !v.seekable.length) return
      const start = v.seekable.start(0), end = v.seekable.end(v.seekable.length - 1)
      const sync = liveHls()?.liveSyncPosition
      const behind = liveCatchUp() ?? Math.max(0, (sync ?? end) - v.currentTime)
      setWin({ start, end, behind, sched: schedOf(v.currentTime) })
    }, 1000)
    return () => clearInterval(t)
  }, [])

  const seekTo = (t: number) => {
    const v = video.current
    if (!v) return
    // Live TV never skips ahead: Back to live, or Skip break in a break, is the way forward (#688).
    if (t > v.currentTime + 0.25) return
    // Further back than the window on a library channel, or anywhere on a start-over stream: by
    // the schedule, which may mean starting that stream again from there.
    if (canShift && (shifted || t < win.start - 0.5)) {
      const s = schedOf(v.currentTime)
      if (s != null && goTo(s + (t - v.currentTime))) return
    }
    setLive({ away: true }); v.currentTime = Math.max(win.start, Math.min(win.end, t))
  }
  // A moment of the schedule: within what's loaded it's a seek; further back, the viewer's own
  // stream from there; at live, the channel everyone shares.
  const goTo = (target: number): boolean => {
    const v = video.current
    const sched = v ? schedOf(v.currentTime) : null
    if (!v || sched == null) return false
    const now = liveNow()
    const plan = planSeek(target, sched, v.currentTime, win, now, tl?.earliest ?? now - 6 * 3600)
    // Live on the channel everyone shares: there's no further back to go than its window.
    if (plan.kind === 'live') { if (!shifted) return false; goLive(); return true }
    if (plan.kind === 'seek') { setLive({ away: true }); v.currentTime = plan.to; return true }
    restartAt(plan.at)
    return true
  }
  // Past the programme it knows of (Skip break on the break that ends it), the next one's end comes from the
  // server first, so the address holds the programme that's on and not to=from+1 (#669).
  const restartAt = async (at: number) => {
    let end = knownEndAfter(at, tl?.programme.end, info?.now?.end)
    if (end == null) end = await api.get<Timeline>(`/tv/${key}/timeline?at=${Math.floor(at)}`).then((t) => knownEndAfter(at, t.programme.end)).catch(() => null)
    nav(startOverPath(key, at, end ?? at + 3600), { replace: true, state: from })
  }
  const startOver = () => { if (info?.now) nav(startOverPath(key, info.now.start, info.now.end), { replace: true, state: from }) }
  const canStartOver = !fromStart && !!info?.startOver && !!info.now && info.now.kind !== 'slate' && liveNow() - info.now.start > LIVE_EDGE_SEC
  // In a break on a start-over stream: one press skips all of it. Live keeps its breaks.
  const inBreak = shifted ? breakAt(tl?.breaks, win.sched) : null
  const skipBreak = () => { if (inBreak) goTo(inBreak.end + 0.5) }
  // Past the end of the programme the timeline told of: what's on from there, and its breaks.
  const askedAt = useRef(0)
  useEffect(() => {
    if (!shifted || !tl || win.sched == null || win.sched < tl.programme.end || askedAt.current === tl.programme.end) return
    askedAt.current = tl.programme.end
    api.get<Timeline>(`/tv/${key}/timeline?at=${Math.floor(win.sched)}`).then(setTl).catch(() => {})
  }, [shifted, tl, win.sched, key])
  // Back from a pause longer than the server keeps: from where it paused, not what's left.
  const pausedAt = useRef<{ ms: number; sched: number | null } | null>(null)
  useEffect(() => {
    const v = video.current
    if (!v || !canShift || !picture) return
    const onPause = () => { pausedAt.current = { ms: Date.now(), sched: schedOf(v.currentTime) } }
    const onPlay = () => {
      const p = pausedAt.current
      pausedAt.current = null
      if (p?.sched != null && resumePlan(p.ms, Date.now(), info?.bufferSec || 300) === 'restart') restartAt(p.sched)
    }
    v.addEventListener('pause', onPause)
    v.addEventListener('play', onPlay)
    return () => { v.removeEventListener('pause', onPause); v.removeEventListener('play', onPlay) }
  })
  // Back to where everyone on the channel is: live sync takes it from there.
  const goLive = () => {
    // From the start, live is the channel itself.
    if (fromStart) { nav(`/tv/watch/${key}`, { replace: true, state: from }); return }
    const v = video.current
    if (!v) return
    const catchUp = liveCatchUp()
    setLive({ away: false }); setLiveCatchUp(0)
    v.currentTime = catchUp != null ? v.currentTime + catchUp : liveHls()?.liveSyncPosition ?? win.end
    v.play().catch(() => {})
    setWin((w) => ({ ...w, behind: 0 }))
  }

  const step = useCallback(async (dir: 1 | -1) => {
    const list = await getLineup().catch(() => [] as GuideChannel[])
    const usable = list.filter((c) => !c.encrypted)
    if (!usable.length) return
    const i = usable.findIndex((c) => c.key === key)
    const next = usable[(i + dir + usable.length) % usable.length]
    nav(`/tv/watch/${next.key}`, { replace: true, state: from })
  }, [key, nav, from])

  const dialRef = useRef('')
  const dialTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const pressDigit = (d: string) => {
    dialRef.current = (dialRef.current + d).slice(0, 8)
    setDial(dialRef.current)
    clearTimeout(dialTimer.current)
    dialTimer.current = setTimeout(async () => {
      const n = dialRef.current
      dialRef.current = ''
      setDial('')
      const list = await getLineup().catch(() => [] as GuideChannel[])
      const c = list.find((x) => x.number === n)
      if (c) nav(`/tv/watch/${c.key}`, { replace: true, state: from })
      else announce(t('No channel {number}', { number: n }))
    }, 1300)
  }
  useEffect(() => () => clearTimeout(dialTimer.current), [])

  const last = () => { if (lastKey.current) nav(`/tv/watch/${lastKey.current}`, { replace: true, state: from }) }
  // Back to the guide this came from, or to one in this player's place: leaving a guide opened from
  // here mustn't lead back into the player, or Back could never get out.
  const [more, setMore] = useState(false)
  const guide = () => { if (from?.fromGuide) nav(-1); else nav('/tv', { replace: true }) }
  const tuneTo = (k: string) => { setMini(false); if (k !== key) nav(`/tv/watch/${k}`, { replace: true, state: from }) }
  const setRemoteOn = (on: boolean) => { setRemoteOnState(on); try { localStorage.setItem(REMOTE_KEY, on ? '1' : '0') } catch { /* ignore */ } }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const v = video.current
      const tag = (e.target as HTMLElement)?.tagName
      if (!v || tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
      if (document.querySelector('.modal-backdrop, .scene-panel')) return
      if (mini) return // it has the keys while it is open
      if (e.key.startsWith('Arrow') && (e.target as HTMLElement)?.closest?.('.bottom, .top, .remote')) return
      if (/^[0-9.]$/.test(e.key)) { pressDigit(e.key); return }
      switch (e.key) {
        case ' ': case 'k': case 'MediaPlayPause': e.preventDefault(); v.paused ? v.play() : v.pause(); break
        case 'ArrowUp': case 'PageUp': case 'ChannelUp': e.preventDefault(); void step(1); break
        case 'ArrowDown': case 'PageDown': case 'ChannelDown': e.preventDefault(); void step(-1); break
        case 'ArrowLeft': seekTo(v.currentTime - 10); break
        case '+': case '=': v.volume = Math.min(1, v.volume + 0.1); break
        case '-': v.volume = Math.max(0, v.volume - 0.1); break
        case 'm': v.muted = !v.muted; break
        case 'f': toggleFullscreen(root.current); break
        case 'r': setRemoteOn(!remoteOn); break
        case 'a': tracks.nextAudio(); break
        case 's': tracks.nextSubtitle(); break
        case 'Enter': e.preventDefault(); showBanner(); break
        // More info: the programme's pop-up over the picture, without leaving the channel.
        case 'i': case 'I': e.preventDefault(); if (info?.now) setMore(true); else showBanner(); break
        case 'g': case 'G': case 'ContextMenu': e.preventDefault(); setMini(true); break
        case 'l': goLive(); break
        case 'n': if (inBreak) skipBreak(); break
        case 'Backspace': e.preventDefault(); last(); break
        case 'Escape': case 'GoBack': case 'BrowserBack': if (!document.fullscreenElement) { e.preventDefault(); guide() } break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const onRemote = (k: RemoteKey) => {
    const v = video.current
    if (k.startsWith('digit')) { pressDigit(k.slice(5)); return }
    switch (k) {
      case 'up': void step(1); break
      case 'down': void step(-1); break
      case 'left': case 'rew': case 'prev': if (v) seekTo(v.currentTime - 10); break
      case 'next': if (inBreak) skipBreak(); break
      case 'enter': case 'menu': showBanner(); break
      case 'topmenu': guide(); break
      case 'return': last(); break
      case 'play': if (v) { void v.play().catch(() => {}) } break
      case 'pause': v?.pause(); break
      case 'stop': case 'eject': guide(); break
      case 'audio': tracks.nextAudio(); break
      case 'subtitle': tracks.nextSubtitle(); break
    }
  }
  const remoteEnabled = (k: RemoteKey) => k === 'audio' ? tracks.audio.length > 1 : k === 'subtitle' ? tracks.subtitles.length > 0 : k !== 'angle'
  const togglePlay = () => { const v = video.current; if (v && !needClick) v.paused ? v.play().catch(() => {}) : v.pause() }
  // Another device steering this one (docs/REMOTE-PROTOCOL.md): see remote/tvTarget.ts.
  useTvRemote({ video, now: info?.now, positionSec: info?.positionSec || 0, serverTime: info?.serverTime || 0, ready: picture,
    audio: tracks.audio, subtitles: tracks.subtitles, chooseAudio: tracks.chooseAudio, chooseSubtitle: tracks.chooseSubtitle,
    seekBy: (sec) => { const v = video.current; if (v && sec < 0) seekTo(v.currentTime + sec) }, step: (dir) => void step(dir), stop: guide })
  const windowLen = Math.max(0, win.end - win.start)
  // Off live: once the viewer has paused or gone back, live sync leaves this player be, so it's
  // behind until Live; otherwise only when it has fallen further behind than live sync catches up by
  // itself. How far is said when it's known: near a programme change the segments' times can jump.
  // Never before the first picture: a channel still starting has nothing to be behind.
  const isBehind = !fromStart && picture && (live.away || win.behind > 10)
  const behindBy = (s: number) => s < 3 ? t('Behind live') : s < 60 ? t('{n} s behind live', { n: Math.round(s) }) : t('{n} min behind live', { n: Math.round(s / 60) })
  const behindText = behindBy(win.behind)
  const tv = !!getTvMode()
  // On a start-over stream the bar is the programme's, by the schedule.
  const prog = shifted && tl && win.sched != null ? tl.programme : null
  const shiftBehind = shifted && win.sched != null ? Math.max(0, liveNow() - win.sched) : 0

  const root_ = themeRoot(look)
  const playerRoot = { ...root_, style: { ...root_.style, ...channelVars(info?.accent, look.bg) } }

  return (
    <div className={'player ctv-player'} ref={root} tabIndex={-1} onClick={togglePlay} onDoubleClick={() => toggleFullscreen(root.current)} {...playerRoot}>{/* design-ok: a click on the picture is the mouse's shortcut for Play and Pause */}
      <h1 className="sr-only">{info ? t('Channel {number}, {name}', { number: info.number, name: info.name }) : t('Live TV')}</h1>
      <div className="stage" ref={stage} />
      {!picture && !err && !needClick && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && <div className="big-play"><button aria-label={t('Play')} onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {err && (
        <div className="overlay-msg" role="alert">
          <div className="box">
            <b>{info ? t('Channel {number}, {name}', { number: info.number, name: info.name }) : t('This channel')}</b><br />{tx(err)}<br />
            <div className="ctv-msg-actions" onClick={(e) => e.stopPropagation()}>
              {alt && <button className="btn primary" onClick={() => nav(`/tv/watch/${alt.key}`, { replace: true })}>{t('Watch {number} {name} instead', { number: alt.number, name: alt.name })}</button>}
              {err.includes('Encrypted:') && <button className="btn" onClick={() => nav('/tv/encrypted')}>{t('What can I do?')}</button>}
              {fromStart && <button className="btn primary" onClick={goLive}>{t('Watch the channel live')}</button>}
              <button className="btn" onClick={guide}>{t('Back to the guide')}</button>
              {!fromStart && <button className="btn" onClick={() => void step(1)}>{t('Next channel')}</button>}
            </div>
          </div>
        </div>
      )}
      {info?.now?.kind === 'slate' && !err && <Slate name={info.name} next={info.next} />}
      {dial && <div className="ctv-osd-dial" role="status">{dial}<span className="ctv-blink">_</span></div>}
      {inBreak && picture && (
        <button type="button" className="ctv-btn ctv-skip-break" onClick={(e) => { e.stopPropagation(); skipBreak() }} aria-label={t('Skip this break')}>
          {t('Skip break')} <span aria-hidden="true">▸▸</span>{!tv && <> <kbd aria-hidden="true">N</kbd></>}
        </button>
      )}
      <div className="top" onClick={(e) => e.stopPropagation()}>
        <button className="ibtn" onClick={guide} title={t('Guide (G)')} aria-label={t('Back to the guide')}><IconBack /></button>
        <div className="grow"><h2>{info ? `${info.number} ${info.name}` : t('Tuning…')}</h2><div className="sub">{shifted && shiftBehind ? behindBy(shiftBehind) : fromStart ? t('From the start') : isBehind ? behindText : t('Live')}</div></div>
      </div>
      {remoteOn && <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label="WittyTV" />}
      {banner && !mini && (info ? <Banner info={info} look={look} fromStart={fromStart} onMore={() => setMore(true)} /> : zap && <ZapBanner ch={zap} look={look} />)}
      {more && info?.now && <MoreInfo channelKey={info.key} at={Math.max(info.now.start, Date.now() / 1000)} onClose={() => setMore(false)} />}
      {mini && <MiniGuide channel={key} onTune={tuneTo} onClose={() => setMini(false)} onFull={guide} />}
      {settingsOpen && <Dialog title={t('Player settings')} onClose={() => setSettingsOpen(false)}><BackgroundPlayChoice /></Dialog>}
      {statsOn && <StreamInfo lines={streamLines('live', { width: video.current?.videoWidth, height: video.current?.videoHeight, work: work || undefined }, { width: video.current?.videoWidth || 0, height: video.current?.videoHeight || 0, kbps: (() => { const h = liveHls(); const l = h && h.currentLevel >= 0 ? h.levels[h.currentLevel] : null; return l?.bitrate ? Math.round(l.bitrate / 1000) : 0 })() })} />}
      {qualityOpen && info?.quality && (
        <QualityMenu view={info.quality} decision="live" live where={here()} showInfo={statsOn} onShowInfo={(on) => { setStatsOn(on); setShowStreamInfo(on) }} onClose={() => setQualityOpen(false)}
          onChoose={(c) => { setQualityOpen(false); setDeviceDefault(here(), c); announce(t('Quality: {quality}', { quality: describe(c, info.quality?.presets || []) })); if (c !== info.quality?.asked) retune() }} />
      )}
      <Controls video={video} duration={prog ? prog.end - prog.start : windowLen} offset={prog && win.sched != null ? win.sched - prog.start - (video.current?.currentTime ?? 0) : -win.start}
        onSeek={(t) => { if (prog) { if (win.sched == null || prog.start + t <= win.sched + 0.25) goTo(prog.start + t) } else seekTo(win.start + t) }} onFullscreen={() => toggleFullscreen(root.current)}
        remoteOn={remoteOn} onRemote={() => setRemoteOn(!remoteOn)} showChapterMarks={false} onSettings={() => setSettingsOpen(true)}
        quality={info?.quality ? sizeName(video.current?.videoHeight || 0, video.current?.videoWidth || 0) || describe(info.quality.asked, info.quality.presets) : undefined}
        onQualityMenu={info?.quality ? () => setQualityOpen((o) => !o) : undefined} qualityMenuOpen={qualityOpen}
        audio={tracks.audio} subtitles={tracks.subtitles} onAudio={tracks.chooseAudio} onSubtitle={tracks.chooseSubtitle}
        live compact={compact} sheetItems={[
          ...(fromStart || isBehind ? [{ key: 'live', icon: <IconTv />, label: t('Back to live'), onClick: goLive }] : []),
          // The compact bar shows only this sheet, so Start over is here too (#648).
          ...(canStartOver ? [{ key: 'startover', icon: <IconRefresh />, label: t('Start over'), onClick: startOver }] : []),
          { key: 'chup', icon: <IconUp />, label: t('Channel up'), onClick: () => void step(1) },
          { key: 'chdown', icon: <IconDown />, label: t('Channel down'), onClick: () => void step(-1) },
          ...(lastKey.current ? [{ key: 'last', icon: <IconHistory />, label: t('Last channel'), onClick: last }] : []),
          { key: 'mini', icon: <IconChannels />, label: t('Now and next'), onClick: () => setMini(true) },
          { key: 'guide', icon: <IconChannels />, label: t('The full guide'), onClick: guide },
        ]}
        extra={<>
          {fromStart ? <button className="btn sm primary" onClick={goLive} title={tv ? undefined : t('Back to live (L)')}>● {t('Back to live')}</button>
            : isBehind && <button className="btn sm" onClick={goLive} title={tv ? undefined : t('Back to live (L)')}>● {t('Live')}</button>}
          {canStartOver && <button className="btn sm" onClick={startOver} aria-label={t('Start this programme over')}>⟲ {t('Start over')}</button>}
          <button className="btn sm" onClick={() => void step(-1)} title={t('Channel down (Page Down)')} aria-label={t('Channel down')}>{t('CH −')}</button>
          <button className="btn sm" onClick={() => void step(1)} title={t('Channel up (Page Up)')} aria-label={t('Channel up')}>{t('CH +')}</button>
          <button className="btn sm hide-narrow" onClick={last} disabled={!lastKey.current} title={t('Last channel (Backspace)')}>{t('Last')}</button>
          <button className="btn sm" onClick={() => setMini(true)} title={t('Now and next (G)')}>{t('Mini guide')}</button>
          <button className="btn sm hide-narrow" onClick={guide} title={t('The full guide')}>{t('Guide')}</button>
        </>} />
    </div>
  )
}

// The banner before the server has answered: the channel, straight away.
function ZapBanner({ ch, look }: { ch: GuideChannel; look: Look }) {
  return (
    <div className="ctv-banner" onClick={(e) => e.stopPropagation()}>
      <div className="ctv-banner-ch"><ChannelLogo ch={ch} look={look} on={surfaceOf(look)} /><b>{ch.number}</b><span>{ch.name}</span></div>
      <div className="ctv-banner-main"><div className="ctv-banner-title">{ch.name}</div><div className="ctv-banner-sub">{t('Tuning…')}</div></div>
    </div>
  )
}

// The banner: what every box of the era showed on a channel change.
function Banner({ info, look, fromStart, onMore }: { info: TuneInfo; look: Look; fromStart?: boolean; onMore: () => void }) {
  const [now, setNow] = useState(() => Date.now() / 1000)
  useEffect(() => { const t = setInterval(() => setNow(Date.now() / 1000), 5000); return () => clearInterval(t) }, [])
  const p = info.now
  const pct = p && p.end > p.start ? Math.max(0, Math.min(100, ((now - p.start) / (p.end - p.start)) * 100)) : 0
  return (
    <div className="ctv-banner" onClick={(e) => e.stopPropagation()}>
      <div className="ctv-banner-ch">
        <ChannelLogo ch={info} look={look} on={surfaceOf(look)} />
        <b>{info.number}</b><span>{info.name}</span>
      </div>
      <div className="ctv-banner-main">
        <div className="ctv-banner-title">{p?.title || info.name}{p?.rating && <em>{p.rating}</em>}</div>
        <div className="ctv-banner-sub">{fromStart && <em className="ctv-banner-tag">{t('From the start')}</em>}{p?.sub || (p ? '' : t('No guide data'))}</div>
        {fromStart && <div className="ctv-banner-next">{getTvMode() ? t('Choose Back to live for the channel as it is now.') : t('Press L or Back to live for the channel as it is now.')}</div>}
        {p && !fromStart && <div className="ctv-banner-progress"><span>{fmtClock(p.start)}</span><i><u style={{ width: pct + '%' }} /></i><span>{fmtClock(p.end)}</span></div>}
        {info.next && <div className="ctv-banner-next">{t('Next: {title}', { title: info.next.title })} · {fmtSpan(info.next)}</div>}
        {info.playing && <div className="ctv-banner-next">{t('Playing now: {title}', { title: info.playing.title })}</div>}
        {info.notices?.map((n) => <div key={n} className="ctv-banner-notice">{tx(n)}{info.alternative ? ' ' + t('Channel {number} has the same station with sound.', { number: info.alternative.number }) : ''}</div>)}
        <BannerRights ch={info} look={look} />
        {p && <button type="button" className="ctv-btn sm ctv-banner-more" onClick={() => onMore()} aria-label={t('More info')}>{t('More info')}{!getTvMode() && <> <kbd aria-hidden="true">I</kbd></>}</button>}
      </div>
      <div className="ctv-banner-clock">{fmtClock(now)}</div>
    </div>
  )
}

// A logo on the banner has its rights notice with it, however briefly it's up.
function BannerRights({ ch, look }: { ch: Pick<TuneInfo, 'logo' | 'brand'>; look: Look }) {
  const service = useLogo(ch.brand?.kind || 'studio', look.logos && !ch.logo && ch.brand ? ch.brand.name : '')
  if (!look.logos || !(ch.logo || service?.has)) return null
  return <LegalNote kind="logosBrief" className="ctv-banner-legal" />
}

