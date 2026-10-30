import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import Hls from 'hls.js'
import { api } from '../../api'
import { announce } from '../../a11y'
import { IconBack, IconPlay } from '../../icons'
import { Controls } from '../../player/Controls'
import { toggleFullscreen, useIdle, useSource } from '../../player/hooks'
import Remote, { type RemoteKey } from '../../player/Remote'
import { rememberChannel, useLook } from './look'
import { fmtClock, fmtSpan, forgetLineup, getLineup, leaveChannel, logoSrc, makeViewer, preflight, tune, type Alternative, type GuideChannel, type Programme, type TuneInfo } from './tvApi'
import './tv.css'

// Watching a channel: the app's player and on-screen remote, with channel keys
// and the classic banner that drops in on every tune. Everyone on a channel
// shares one stream; pausing keeps a place in the server's buffer.

const REMOTE_KEY = 'wms.remote.tv'
const remoteSaved = () => { try { return localStorage.getItem(REMOTE_KEY) === '1' } catch { return false } }

export default function TvPlayer() {
  const { key = '' } = useParams()
  const nav = useNavigate()
  const video = useRef<HTMLVideoElement | null>(null)
  const root = useIdle(4000, [])
  const [info, setInfo] = useState<TuneInfo | null>(null)
  const [url, setUrl] = useState<string | null>(null)
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const [banner, setBanner] = useState(true)
  const [remoteOn, setRemoteOnState] = useState(remoteSaved)
  const [dial, setDial] = useState('')
  const [win, setWin] = useState({ start: 0, end: 0, behind: 0 })
  const [alt, setAlt] = useState<Alternative | null>(null)
  const lastKey = useRef<string | null>(null)
  const prevKey = useRef('')
  const bannerTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const look = useLook()
  const viewer = useRef(makeViewer())

  const showBanner = useCallback(() => {
    setBanner(true)
    clearTimeout(bannerTimer.current)
    bannerTimer.current = setTimeout(() => setBanner(false), 7000)
  }, [])
  useEffect(() => () => clearTimeout(bannerTimer.current), [])

  // Tune: ask what is on, then wait for the first picture before the player
  // starts, so a refused or broken channel says why.
  useEffect(() => {
    if (prevKey.current && prevKey.current !== key) lastKey.current = prevKey.current
    prevKey.current = key
    setInfo(null); setUrl(null); setErr(''); setNeedClick(false); setAlt(null)
    const ac = new AbortController()
    showBanner()
    rememberChannel(key)
    const v = viewer.current
    tune(key, v, ac.signal)
      .then(async (i) => {
        if (ac.signal.aborted) return
        setInfo(i)
        announce(`Channel ${i.number}, ${i.name}${i.now?.title ? ', ' + i.now.title : ''}${i.notices?.length ? '. ' + i.notices.join(' ') : ''}`)
        const u = i.url + '?viewer=' + v
        await preflight(u, ac.signal)
        if (!ac.signal.aborted) setUrl(u)
      })
      .catch((e) => {
        if (ac.signal.aborted) return
        setErr(e.message)
        // A channel refused now may have just shown itself to be encrypted.
        forgetLineup()
        getLineup().then((l) => setAlt(l.find((c) => c.key === key)?.alternative || null)).catch(() => {})
      })
    const gone = () => leaveChannel(key, v)
    window.addEventListener('pagehide', gone)
    return () => { ac.abort(); window.removeEventListener('pagehide', gone); gone() }
  }, [key, showBanner])

  const hlsRef = useSource(video, url, {
    onReady: () => { video.current?.play().catch(() => setNeedClick(true)) },
    onError: (m) => setErr(m),
  })
  // The stream can be refused after it started: a tuner taken for a recording,
  // or a channel that turns out to be encrypted.
  useEffect(() => {
    const h = hlsRef.current
    if (!h || !url) return
    const onErr = (_: unknown, d: { fatal?: boolean; response?: { code?: number } }) => {
      const code = d.response?.code
      if (d.fatal && code && code !== 503) {
        preflight(url, new AbortController().signal).catch((e) => { setErr(e.message); h.destroy() })
      }
    }
    h.on(Hls.Events.ERROR, onErr)
    return () => { h.off(Hls.Events.ERROR, onErr) }
  }, [url, hlsRef])

  // When a programme ends, the banner moves on.
  const refresh = useCallback(() => {
    api.get<TuneInfo>(`/tv/${key}/tune?viewer=${viewer.current}`).then((i) => { setInfo(i); showBanner() }).catch(() => {})
  }, [key, showBanner])
  useEffect(() => {
    if (!info?.now?.end) return
    const wait = info.now.end * 1000 - Date.now() + 1500
    if (wait <= 0 || wait > 86400000) return
    const t = setTimeout(refresh, wait)
    return () => clearTimeout(t)
  }, [info?.now?.end, refresh])

  // Counting towards watch progress, on the channels set up for it.
  useEffect(() => {
    if (!info?.countProgress || !info.now?.itemId) return
    const p = info.now
    const send = () => {
      const v = video.current
      if (!v || v.paused) return
      const pos = Date.now() / 1000 - p.start
      api.post(`/items/${p.itemId}/progress`, { positionSec: pos, durationSec: p.end - p.start }).catch(() => {})
    }
    const t = setInterval(send, 30000)
    return () => { clearInterval(t); send() }
  }, [info?.countProgress, info?.now])

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

  const seekTo = (t: number) => { const v = video.current; if (v) v.currentTime = Math.max(win.start, Math.min(win.end, t)) }
  const goLive = () => { const h = hlsRef.current; const v = video.current; if (v) { v.currentTime = h?.liveSyncPosition ?? win.end; v.play().catch(() => {}) } }

  const step = useCallback(async (dir: 1 | -1) => {
    const list = await getLineup().catch(() => [] as GuideChannel[])
    const usable = list.filter((c) => !c.encrypted)
    if (!usable.length) return
    const i = usable.findIndex((c) => c.key === key)
    const next = usable[(i + dir + usable.length) % usable.length]
    nav(`/tv/watch/${next.key}`, { replace: true })
  }, [key, nav])

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
      if (c) nav(`/tv/watch/${c.key}`, { replace: true })
      else announce(`No channel ${n}`)
    }, 1300)
  }
  useEffect(() => () => clearTimeout(dialTimer.current), [])

  const last = () => { if (lastKey.current) nav(`/tv/watch/${lastKey.current}`, { replace: true }) }
  const guide = () => nav('/tv')
  const setRemoteOn = (on: boolean) => { setRemoteOnState(on); try { localStorage.setItem(REMOTE_KEY, on ? '1' : '0') } catch { /* ignore */ } }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const v = video.current
      const tag = (e.target as HTMLElement)?.tagName
      if (!v || tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
      if (document.querySelector('.modal-backdrop, .scene-panel')) return
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
        case 'i': case 'Enter': e.preventDefault(); showBanner(); break
        case 'g': case 'ContextMenu': guide(); break
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
    }
  }
  const remoteEnabled = (k: RemoteKey) => k !== 'audio' && k !== 'subtitle' && k !== 'angle'
  const togglePlay = () => { const v = video.current; if (v && !needClick) v.paused ? v.play().catch(() => {}) : v.pause() }
  const windowLen = Math.max(0, win.end - win.start)
  const isBehind = win.behind > 20

  return (
    <div className={'player ctv-player'} ref={root} tabIndex={-1} onClick={togglePlay} onDoubleClick={() => toggleFullscreen(root.current)} data-skin={look.colours} data-layout={look.layout}>
      <div className="stage"><video ref={video} playsInline preload="auto" aria-label={info ? `Channel ${info.number}, ${info.name}` : 'TV'} /></div>
      {!url && !err && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && <div className="big-play"><button aria-label="Play" onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {err && (
        <div className="overlay-msg" role="alert">
          <div className="box">
            <b>{info ? `Channel ${info.number}, ${info.name}` : 'This channel'}</b><br />{err}<br />
            <div className="ctv-msg-actions" onClick={(e) => e.stopPropagation()}>
              {alt && <button className="btn primary" onClick={() => nav(`/tv/watch/${alt.key}`, { replace: true })}>Watch {alt.number} {alt.name} instead</button>}
              {err.includes('Encrypted:') && <button className="btn" onClick={() => nav('/tv/encrypted')}>What can I do?</button>}
              <button className="btn" onClick={guide}>Back to the guide</button>
              <button className="btn" onClick={() => void step(1)}>Next channel</button>
            </div>
          </div>
        </div>
      )}
      {info?.now?.kind === 'slate' && !err && <Slate name={info.name} next={info.next} />}
      {dial && <div className="ctv-osd-dial" role="status">{dial}<span className="ctv-blink">_</span></div>}
      <div className="top" onClick={(e) => e.stopPropagation()}>
        <button className="ibtn" onClick={guide} title="Guide (G)" aria-label="Back to the guide"><IconBack /></button>
        <div className="grow"><h2>{info ? `${info.number} ${info.name}` : 'Tuning…'}</h2><div className="sub">{isBehind ? `${Math.round(win.behind / 60)} min behind live` : 'Live'}</div></div>
      </div>
      {remoteOn && <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label="WittyTV" />}
      {banner && info && <Banner info={info} />}
      <Controls video={video} duration={windowLen} offset={-win.start} onSeek={(t) => seekTo(win.start + t)} onFullscreen={() => toggleFullscreen(root.current)}
        remoteOn={remoteOn} onRemote={() => setRemoteOn(!remoteOn)} showChapterMarks={false}
        extra={<>
          {isBehind && <button className="btn sm" onClick={goLive} title="Back to live (L)">● Live</button>}
          <button className="btn sm" onClick={() => void step(-1)} title="Channel down (Page Down)" aria-label="Channel down">CH −</button>
          <button className="btn sm" onClick={() => void step(1)} title="Channel up (Page Up)" aria-label="Channel up">CH +</button>
          <button className="btn sm hide-narrow" onClick={last} disabled={!lastKey.current} title="Last channel (Backspace)">Last</button>
          <button className="btn sm" onClick={guide} title="Guide (G)">Guide</button>
        </>} />
    </div>
  )
}

// The banner: what every box of the era showed on a channel change.
function Banner({ info }: { info: TuneInfo }) {
  const [now, setNow] = useState(() => Date.now() / 1000)
  useEffect(() => { const t = setInterval(() => setNow(Date.now() / 1000), 5000); return () => clearInterval(t) }, [])
  const p = info.now
  const pct = p && p.end > p.start ? Math.max(0, Math.min(100, ((now - p.start) / (p.end - p.start)) * 100)) : 0
  return (
    <div className="ctv-banner" onClick={(e) => e.stopPropagation()}>
      <div className="ctv-banner-ch">
        {info.logo ? <img src={logoSrc(info.logo)} alt="" /> : null}
        <b>{info.number}</b><span>{info.name}</span>
      </div>
      <div className="ctv-banner-main">
        <div className="ctv-banner-title">{p?.title || info.name}{p?.rating && <em>{p.rating}</em>}</div>
        <div className="ctv-banner-sub">{p?.sub || (p ? '' : 'No guide data')}</div>
        {p && <div className="ctv-banner-progress"><span>{fmtClock(p.start)}</span><i><u style={{ width: pct + '%' }} /></i><span>{fmtClock(p.end)}</span></div>}
        {info.next && <div className="ctv-banner-next">Next: {info.next.title} · {fmtSpan(info.next)}</div>}
        {info.notices?.map((n) => <div key={n} className="ctv-banner-notice">{n}{info.alternative ? ` Channel ${info.alternative.number} has the same station with sound.` : ''}</div>)}
      </div>
      <div className="ctv-banner-clock">{fmtClock(now)}</div>
    </div>
  )
}

function Slate({ name, next }: { name: string; next: Programme | null }) {
  return (
    <div className="ctv-slate" aria-live="polite">
      <div className="ctv-slate-card">
        <div className="ctv-slate-ch">{name}</div>
        <div className="ctv-slate-msg">We'll be right back</div>
        {next && <div className="ctv-slate-next">Coming up at {fmtClock(next.start)}: {next.title}</div>}
      </div>
    </div>
  )
}
