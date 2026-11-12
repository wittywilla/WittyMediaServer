// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import Hls from 'hls.js'
import { api } from '../../api'
import { announce } from '../../a11y'
import { IconBack, IconPlay } from '../../icons'
import { Controls } from '../../player/Controls'
import { haveHls, toggleFullscreen, useIdle } from '../../player/hooks'
import Remote, { type RemoteKey } from '../../player/Remote'
import { Dialog } from '../../ui'
import { useLogo } from '../../components/BrandLogo'
import LegalNote from '../../components/LegalNote'
import BackgroundPlayChoice from './BackgroundPlayChoice'
import { ChannelLogo } from './ChannelLogo'
import { channelVars, previousChannel, rememberChannel, surfaceOf, themeRoot, useLook, type Look } from './look'
import MiniGuide from './MiniGuide'
import Slate from './Slate'
import { useLiveTracks } from './tracks'
import { liveHls, liveVideo, setLive, useLive } from './live'
import { fmtClock, fmtSpan, getLineup, type Alternative, type GuideChannel, type GuidePage, type TuneInfo } from './tvApi'
import { catchupKey } from './guideLogic'
import { useTvRemote } from '../../remote/tvTarget'
import './tv.css'
import './tvthemes.css'

haveHls(Hls)

// Watching a channel: the app's player and on-screen remote, with channel keys
// and the classic banner that drops in on every tune. Everyone on a channel
// shares one stream; pausing keeps a place in the server's buffer. The picture is
// the TV section's one player (live.ts), carried over from the guide's preview
// and back, so changing where it's shown never tunes again.
//
// With ?from=&to= it plays that programme from its start instead (tv-catchup): the viewer's own
// stream from the channel's archive, which the banner says, and Back to live leaves.

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
  const video = useRef<HTMLVideoElement | null>(liveVideo())
  const stage = useRef<HTMLDivElement | null>(null)
  const root = useIdle(4000, [])
  const live = useLive(liveKey, stage)
  const mine = live.key === liveKey
  const info = mine ? live.info : null
  const err = mine ? live.err : ''
  const picture = mine && live.playing
  const tracks = useLiveTracks(video.current, liveHls, mine && live.ready)
  const [needClick, setNeedClick] = useState(false)
  const [banner, setBanner] = useState(true)
  const [remoteOn, setRemoteOnState] = useState(remoteSaved)
  const [dial, setDial] = useState('')
  const [win, setWin] = useState({ start: 0, end: 0, behind: 0 })
  const [alt, setAlt] = useState<Alternative | null>(null)
  const [mini, setMini] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  // Shown the moment a channel is chosen, from what the list already knows: the
  // number and name don't wait for the stream.
  const [zap, setZap] = useState<GuideChannel | null>(null)
  const lastKey = useRef<string | null>(previousChannel() || null)
  const prevKey = useRef('')
  const bannerTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const look = useLook()

  const showBanner = useCallback(() => {
    setBanner(true)
    clearTimeout(bannerTimer.current)
    bannerTimer.current = setTimeout(() => setBanner(false), 7000)
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
    if (v) v.setAttribute('aria-label', info ? `Channel ${info.number}, ${info.name}` : 'TV')
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
    announce(`Channel ${info.number}, ${info.name}${info.now?.title ? ', ' + info.now.title : ''}${info.notices?.length ? '. ' + info.notices.join(' ') : ''}`)
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

  // Where the live edge is, and how far back the buffer goes.
  useEffect(() => {
    const t = setInterval(() => {
      const v = video.current
      if (!v || !v.seekable.length) return
      const start = v.seekable.start(0), end = v.seekable.end(v.seekable.length - 1)
      setWin({ start, end, behind: Math.max(0, end - v.currentTime) })
    }, 1000)
    return () => clearInterval(t)
  }, [])

  const seekTo = (t: number) => { const v = video.current; if (v) { setLive({ away: true }); v.currentTime = Math.max(win.start, Math.min(win.end, t)) } }
  // Back to where everyone on the channel is: live sync takes it from there.
  const goLive = () => {
    // From the start, live is the channel itself.
    if (fromStart) { nav(`/tv/watch/${key}`, { replace: true, state: from }); return }
    const h = liveHls(); const v = video.current; if (v) { setLive({ away: false }); v.currentTime = h?.liveSyncPosition ?? win.end; v.play().catch(() => {}) }
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
      else announce(`No channel ${n}`)
    }, 1300)
  }
  useEffect(() => () => clearTimeout(dialTimer.current), [])

  const last = () => { if (lastKey.current) nav(`/tv/watch/${lastKey.current}`, { replace: true, state: from }) }
  // Back to the guide this came from, or to one in this player's place: leaving a guide opened from
  // here mustn't lead back into the player, or Back could never get out.
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
        case 'ArrowRight': seekTo(v.currentTime + 10); break
        case 'ArrowLeft': seekTo(v.currentTime - 10); break
        case '+': case '=': v.volume = Math.min(1, v.volume + 0.1); break
        case '-': v.volume = Math.max(0, v.volume - 0.1); break
        case 'm': v.muted = !v.muted; break
        case 'f': toggleFullscreen(root.current); break
        case 'r': setRemoteOn(!remoteOn); break
        case 'a': tracks.nextAudio(); break
        case 's': tracks.nextSubtitle(); break
        case 'i': case 'Enter': e.preventDefault(); showBanner(); break
        case 'g': case 'G': case 'ContextMenu': e.preventDefault(); setMini(true); break
        case 'l': goLive(); break
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
      case 'right': case 'ff': case 'next': if (v) seekTo(v.currentTime + 30); break
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
    seekBy: (sec) => { const v = video.current; if (v) seekTo(v.currentTime + sec) }, step: (dir) => void step(dir), stop: guide })
  const windowLen = Math.max(0, win.end - win.start)
  const isBehind = win.behind > 20

  const root_ = themeRoot(look)
  const playerRoot = { ...root_, style: { ...root_.style, ...channelVars(info?.accent, look.bg) } }

  return (
    <div className={'player ctv-player'} ref={root} tabIndex={-1} onClick={togglePlay} onDoubleClick={() => toggleFullscreen(root.current)} {...playerRoot}>{/* design-ok: a click on the picture is the mouse's shortcut for Play and Pause */}
      <h1 className="sr-only">{info ? `Channel ${info.number}, ${info.name}` : 'Live TV'}</h1>
      <div className="stage" ref={stage} />
      {!picture && !err && !needClick && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && <div className="big-play"><button aria-label="Play" onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {err && (
        <div className="overlay-msg" role="alert">
          <div className="box">
            <b>{info ? `Channel ${info.number}, ${info.name}` : 'This channel'}</b><br />{err}<br />
            <div className="ctv-msg-actions" onClick={(e) => e.stopPropagation()}>
              {alt && <button className="btn primary" onClick={() => nav(`/tv/watch/${alt.key}`, { replace: true })}>Watch {alt.number} {alt.name} instead</button>}
              {err.includes('Encrypted:') && <button className="btn" onClick={() => nav('/tv/encrypted')}>What can I do?</button>}
              {fromStart && <button className="btn primary" onClick={goLive}>Watch the channel live</button>}
              <button className="btn" onClick={guide}>Back to the guide</button>
              {!fromStart && <button className="btn" onClick={() => void step(1)}>Next channel</button>}
            </div>
          </div>
        </div>
      )}
      {info?.now?.kind === 'slate' && !err && <Slate name={info.name} next={info.next} />}
      {dial && <div className="ctv-osd-dial" role="status">{dial}<span className="ctv-blink">_</span></div>}
      <div className="top" onClick={(e) => e.stopPropagation()}>
        <button className="ibtn" onClick={guide} title="Guide (G)" aria-label="Back to the guide"><IconBack /></button>
        <div className="grow"><h2>{info ? `${info.number} ${info.name}` : 'Tuning…'}</h2><div className="sub">{fromStart ? 'From the start' : isBehind ? `${Math.round(win.behind / 60)} min behind live` : 'Live'}</div></div>
      </div>
      {remoteOn && <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label="WittyTV" />}
      {banner && !mini && (info ? <Banner info={info} look={look} fromStart={fromStart} /> : zap && <ZapBanner ch={zap} look={look} />)}
      {mini && <MiniGuide channel={key} onTune={tuneTo} onClose={() => setMini(false)} onFull={guide} />}
      {settingsOpen && <Dialog title="Player settings" onClose={() => setSettingsOpen(false)}><BackgroundPlayChoice /></Dialog>}
      <Controls video={video} duration={windowLen} offset={-win.start} onSeek={(t) => seekTo(win.start + t)} onFullscreen={() => toggleFullscreen(root.current)}
        remoteOn={remoteOn} onRemote={() => setRemoteOn(!remoteOn)} showChapterMarks={false} onSettings={() => setSettingsOpen(true)}
        audio={tracks.audio} subtitles={tracks.subtitles} onAudio={tracks.chooseAudio} onSubtitle={tracks.chooseSubtitle}
        extra={<>
          {fromStart ? <button className="btn sm primary" onClick={goLive} title="Back to live (L)">● Back to live</button>
            : isBehind && <button className="btn sm" onClick={goLive} title="Back to live (L)">● Live</button>}
          <button className="btn sm" onClick={() => void step(-1)} title="Channel down (Page Down)" aria-label="Channel down">CH −</button>
          <button className="btn sm" onClick={() => void step(1)} title="Channel up (Page Up)" aria-label="Channel up">CH +</button>
          <button className="btn sm hide-narrow" onClick={last} disabled={!lastKey.current} title="Last channel (Backspace)">Last</button>
          <button className="btn sm" onClick={() => setMini(true)} title="Now and next (G)">Mini guide</button>
          <button className="btn sm hide-narrow" onClick={guide} title="The full guide">Guide</button>
        </>} />
    </div>
  )
}

// The banner before the server has answered: the channel, straight away.
function ZapBanner({ ch, look }: { ch: GuideChannel; look: Look }) {
  return (
    <div className="ctv-banner" onClick={(e) => e.stopPropagation()}>
      <div className="ctv-banner-ch"><ChannelLogo ch={ch} look={look} on={surfaceOf(look)} /><b>{ch.number}</b><span>{ch.name}</span></div>
      <div className="ctv-banner-main"><div className="ctv-banner-title">{ch.name}</div><div className="ctv-banner-sub">Tuning…</div></div>
    </div>
  )
}

// The banner: what every box of the era showed on a channel change.
function Banner({ info, look, fromStart }: { info: TuneInfo; look: Look; fromStart?: boolean }) {
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
        <div className="ctv-banner-sub">{fromStart && <em className="ctv-banner-tag">From the start</em>}{p?.sub || (p ? '' : 'No guide data')}</div>
        {fromStart && <div className="ctv-banner-next">Press L or Back to live for the channel as it is now.</div>}
        {p && !fromStart && <div className="ctv-banner-progress"><span>{fmtClock(p.start)}</span><i><u style={{ width: pct + '%' }} /></i><span>{fmtClock(p.end)}</span></div>}
        {info.next && <div className="ctv-banner-next">Next: {info.next.title} · {fmtSpan(info.next)}</div>}
        {info.playing && <div className="ctv-banner-next">Playing now: {info.playing.title}</div>}
        {info.notices?.map((n) => <div key={n} className="ctv-banner-notice">{n}{info.alternative ? ` Channel ${info.alternative.number} has the same station with sound.` : ''}</div>)}
        <BannerRights ch={info} look={look} />
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

