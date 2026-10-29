import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, langName, type Item, type PlayInfo } from '../api'
import { IconBack, IconPlay } from '../icons'
import { Controls, useVideoState } from './Controls'
import { toggleFullscreen, useIdle, useSource } from './hooks'

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
        case 'Escape': if (!document.fullscreenElement) nav(`/item/${item.id}`); break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [seek, nav, root])

  const switchTrack = (kind: 'audio' | 'sub', id: number) => {
    const v = video.current
    pendingSeek.current = v ? v.currentTime : 0
    kind === 'audio' ? setAudio(id) : setSub(id)
  }
  const duration = info?.durationSec || item.runtimeSec || 0
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
        <div><h2>{item.title}</h2><div className="sub">{[item.year, info?.type === 'direct' ? 'Direct play' : 'Transcoding'].filter(Boolean).join(' · ')}</div></div>
      </div>
      <Controls video={video} duration={duration} onSeek={seek} onFullscreen={() => toggleFullscreen(root.current)}
        audio={info?.audio.map((a) => ({ id: a.index, label: [langName(a.language), a.title, a.codec?.toUpperCase(), a.channels ? a.channels + 'ch' : ''].filter(Boolean).join(' · '), active: a.index === audio }))}
        subtitles={info?.subtitles.map((s) => ({ id: s.index, label: [langName(s.language), s.title].filter(Boolean).join(' · '), active: s.index === sub }))}
        onAudio={(id) => switchTrack('audio', id)} onSubtitle={(id) => switchTrack('sub', id)} />
    </div>
  )
}
