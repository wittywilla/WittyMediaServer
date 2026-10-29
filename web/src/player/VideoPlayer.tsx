import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, episodeLabel, langName, posterUrl, type Item, type PlayInfo, type Show } from '../api'
import { IconBack, IconDisc, IconNext, IconPlay } from '../icons'
import { Controls, useVideoState } from './Controls'
import { toggleFullscreen, useIdle, useSource } from './hooks'
import Remote, { DVD_ONLY, useRemoteToggle, type RemoteKey } from './Remote'

export default function VideoPlayer({ item, startAt }: { item: Item; startAt: number | null }) {
  const nav = useNavigate()
  const video = useRef<HTMLVideoElement | null>(null)
  const root = useIdle(3000, [])
  const [info, setInfo] = useState<PlayInfo | null>(null)
  const [audio, setAudio] = useState(0)
  const [sub, setSub] = useState(-1)
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const pendingSeek = useRef<number | null>(startAt ?? (item.progress && !item.progress.watched && item.progress.positionSec > 60 ? item.progress.positionSec : 0))
  const st = useVideoState(video)
  const [remoteOn, setRemoteOn] = useRemoteToggle('video')
  const [upNext, setUpNext] = useState<number | null>(null)
  const [next, setNext] = useState<{ item: Item; show?: Show | null; recommended: boolean } | null>(null)
  const [nextDismissed, setNextDismissed] = useState(false)
  const [ended, setEnded] = useState(false)

  // "Watch next": the next episode of this show, or (end of a show / a
  // movie) a recommendation, loaded once the credits are near.
  const nearEnd = (info?.durationSec || item.runtimeSec || 0) > 0 && st.time >= (info?.durationSec || item.runtimeSec) - 30
  useEffect(() => {
    if ((!nearEnd && !ended) || next) return
    if (item.nextId) api.get<Item>(`/items/${item.nextId}`).then((it) => setNext({ item: it, recommended: false })).catch(() => {})
    else api.get<{ item: Item | null; show: Show | null }>(`/items/${item.id}/recommend`).then((r) => { if (r.item) setNext({ item: r.item, show: r.show, recommended: true }) }).catch(() => {})
  }, [nearEnd, ended, next, item.id, item.nextId])

  // Episodes: count down into the next one when this one ends.
  useEffect(() => {
    const v = video.current
    if (!v) return
    const onEnded = () => { setEnded(true); if (item.nextId) setUpNext(8) }
    v.addEventListener('ended', onEnded)
    return () => v.removeEventListener('ended', onEnded)
  }, [item.nextId])
  useEffect(() => {
    if (upNext == null || nextDismissed) return
    if (upNext <= 0) { nav(`/play/${item.nextId}`, { replace: true }); return }
    const t = setTimeout(() => setUpNext(upNext - 1), 1000)
    return () => clearTimeout(t)
  }, [upNext, item.nextId, nav, nextDismissed])

  // "Skip intro" while inside the intro range (chapter or show setting).
  const inIntro = item.introEnd != null && st.time >= (item.introStart || 0) && st.time < item.introEnd - 1

  useEffect(() => {
    setInfo(null)
    api.get<PlayInfo>(`/items/${item.id}/play?audio=${audio}&subtitle=${sub}`).then(setInfo).catch((e) => setErr(e.message))
  }, [item.id, audio, sub])

  // HLS seeking is "seek the playlist"; our server playlist covers the whole
  // file so a plain currentTime assignment works for both modes.
  const seek = useCallback((t: number) => { const v = video.current; if (v) v.currentTime = Math.max(0, Math.min(t, (info?.durationSec || v.duration || t))) }, [info])

  useSource(video, info?.url || null, {
    onReady: () => {
      const v = video.current
      if (!v) return
      if (pendingSeek.current && pendingSeek.current > 0) { v.currentTime = pendingSeek.current }
      pendingSeek.current = null
      v.play().catch(() => setNeedClick(true))
    },
    onError: (m) => setErr(m),
  })

  // Progress reporting.
  useEffect(() => {
    const send = () => {
      const v = video.current
      if (!v || !info || v.currentTime < 5) return
      api.post(`/items/${item.id}/progress`, { positionSec: v.currentTime, durationSec: info.durationSec || v.duration }).catch(() => {})
    }
    const t = setInterval(send, 10000)
    window.addEventListener('beforeunload', send)
    return () => { clearInterval(t); window.removeEventListener('beforeunload', send); send() }
  }, [item.id, info])

  // Keyboard.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const v = video.current
      if (!v) return
      switch (e.key) {
        case ' ': case 'k': e.preventDefault(); v.paused ? v.play() : v.pause(); break
        case 'ArrowRight': seek(v.currentTime + 10); break
        case 'ArrowLeft': seek(v.currentTime - 10); break
        case 'ArrowUp': v.volume = Math.min(1, v.volume + 0.1); break
        case 'ArrowDown': v.volume = Math.max(0, v.volume - 0.1); break
        case 'f': toggleFullscreen(root.current); break
        case 'm': v.muted = !v.muted; break
        case 'r': setRemoteOn(!remoteOn); break
        case 'Escape': if (!document.fullscreenElement) nav(`/item/${item.id}`); break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [seek, nav, root, remoteOn])

  const switchTrack = (kind: 'audio' | 'sub', id: number) => {
    const v = video.current
    pendingSeek.current = v ? v.currentTime : 0
    kind === 'audio' ? setAudio(id) : setSub(id)
  }
  const duration = info?.durationSec || item.runtimeSec || 0
  const cycle = (kind: 'audio' | 'sub') => {
    if (!info) return
    if (kind === 'audio') {
      if (info.audio.length < 2) return
      const i = info.audio.findIndex((a) => a.index === audio)
      switchTrack('audio', info.audio[(i + 1) % info.audio.length].index)
    } else {
      if (!info.subtitles.length) return
      const i = info.subtitles.findIndex((s) => s.index === sub)
      switchTrack('sub', i + 1 >= info.subtitles.length ? -1 : info.subtitles[i + 1].index)
    }
  }
  // The same remote as for discs; disc-only keys are greyed out.
  const onRemote = (k: RemoteKey) => {
    const v = video.current
    switch (k) {
      case 'eject': nav(`/item/${item.id}`); break
      case 'audio': cycle('audio'); break
      case 'subtitle': cycle('sub'); break
      case 'rew': case 'prev': if (v) seek(v.currentTime - (k === 'prev' ? 60 : 10)); break
      case 'ff': if (v) seek(v.currentTime + 30); break
      case 'next': if (item.nextId) nav(`/play/${item.nextId}`); else if (v) seek(v.currentTime + 60); break
      case 'play': v?.play().catch(() => {}); break
      case 'pause': v?.pause(); break
      case 'stop': if (v) { v.pause(); seek(0) } break
    }
  }
  const remoteEnabled = (k: RemoteKey) => {
    if (DVD_ONLY.includes(k)) return false
    if (k === 'audio') return (info?.audio.length || 0) > 1
    if (k === 'subtitle') return (info?.subtitles.length || 0) > 0
    return true
  }
  const subtitle = [item.showTitle, episodeLabel(item)].filter(Boolean).join(' · ')
  return (
    <div className="player" ref={root} tabIndex={0} onClick={() => { const v = video.current; if (v && !needClick) v.paused ? v.play() : v.pause() }} onDoubleClick={() => toggleFullscreen(root.current)}>
      <div className="stage">
        <video ref={video} playsInline preload="auto" />
      </div>
      {st.waiting && !needClick && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && <div className="big-play"><button onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {err && <div className="overlay-msg"><div className="box">Playback failed: {err}<br /><button className="btn" style={{ marginTop: 10 }} onClick={() => nav(`/item/${item.id}`)}>Back</button></div></div>}
      <div className="top">
        <button className="ibtn" onClick={(e) => { e.stopPropagation(); nav(`/item/${item.id}`) }}><IconBack /></button>
        <div className="grow"><h2>{item.title}</h2><div className="sub">{[subtitle || item.year, info?.type === 'direct' ? 'Direct play' : 'Transcoding'].filter(Boolean).join(' · ')}</div></div>
        {item.nextId && <button className="btn sm" onClick={(e) => { e.stopPropagation(); nav(`/play/${item.nextId}`) }} title="Next episode"><IconNext /> Next episode</button>}
        <button className={'btn sm' + (remoteOn ? ' primary' : '')} onClick={(e) => { e.stopPropagation(); setRemoteOn(!remoteOn) }} title="Show the remote (r)"><IconDisc /> Remote</button>
      </div>
      {remoteOn && <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label="WittyMedia" />}
      {inIntro && <button className="btn skip-intro" onClick={(e) => { e.stopPropagation(); seek(item.introEnd!) }}>Skip intro ›</button>}
      {next && (nearEnd || ended) && !nextDismissed && (
        <div className="watch-next" onClick={(e) => e.stopPropagation()}>
          <div className="wn-head">{next.recommended
            ? (next.show ? `Because you watched ${item.showTitle || item.title}` : 'You might also like')
            : upNext != null ? `Next episode in ${upNext}s` : 'Watch next'}</div>
          <div className="wn-body" onClick={() => nav(`/play/${next.item.id}`, { replace: true })}>
            <div className={'wn-thumb' + (next.item.showId != null ? ' wide' : '')}>{next.item.poster && <img src={posterUrl(next.item)} alt="" />}</div>
            <div>
              <div className="wn-title">{next.show ? next.show.title : next.item.showTitle || next.item.title}</div>
              <div className="muted" style={{ fontSize: 12 }}>{next.item.showId != null ? [episodeLabel(next.item), next.item.title].filter(Boolean).join(' \u00b7 ') : next.item.year}</div>
            </div>
          </div>
          <div className="wn-actions">
            <button className="btn sm primary" onClick={() => nav(`/play/${next.item.id}`, { replace: true })}><IconPlay /> Play</button>
            <button className="btn sm ghost" onClick={() => { setNextDismissed(true); setUpNext(null) }}>Dismiss</button>
          </div>
        </div>
      )}
      <Controls video={video} duration={duration} onSeek={seek} onFullscreen={() => toggleFullscreen(root.current)}
        audio={info?.audio.map((a) => ({ id: a.index, label: [langName(a.language), a.title, a.codec?.toUpperCase(), a.channels ? a.channels + 'ch' : ''].filter(Boolean).join(' · '), active: a.index === audio }))}
        subtitles={info?.subtitles.map((s) => ({ id: s.index, label: [langName(s.language), s.title].filter(Boolean).join(' · '), active: s.index === sub }))}
        onAudio={(id) => switchTrack('audio', id)} onSubtitle={(id) => switchTrack('sub', id)} />
    </div>
  )
}
