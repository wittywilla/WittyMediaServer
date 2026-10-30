import { useEffect, useRef, useState } from 'react'
import { api, langName, type PlayInfo } from '../api'
import { IconPlay } from '../icons'
import { Controls, setSpeed, useVideoState } from './Controls'
import { toggleFullscreen, useIdle, useSource } from './hooks'
import './inline.css'

// A video inside a page rather than filling the screen: the creator pages and
// the photo viewer. It has the full player's controls (speed, chapters, audio,
// subtitles), reports progress and watch time, and plays what the server says
// the same way the full player does, direct or HLS.

// One speed for the whole visit: the next video in a list starts at it too.
let sessionRate = 1

export default function InlinePlayer({ id, startAt, autoPlay = true, onEnded, durationSec = 0, compact, className = '' }: {
  id: number; startAt?: number; autoPlay?: boolean; onEnded?: () => void; durationSec?: number; compact?: boolean; className?: string
}) {
  const video = useRef<HTMLVideoElement | null>(null)
  const root = useIdle(3000, [])
  const [info, setInfo] = useState<PlayInfo | null>(null)
  const [audio, setAudio] = useState(0)
  const [sub, setSub] = useState(-1)
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const pendingSeek = useRef<number | null>(startAt ?? null)
  const st = useVideoState(video)
  const watched = useRef(0)
  const lastTime = useRef(0)

  useEffect(() => {
    setInfo(null); setErr('')
    api.get<PlayInfo>(`/items/${id}/play?audio=${audio}&subtitle=${sub}`).then(setInfo).catch((e) => setErr(e.message))
  }, [id, audio, sub])

  useSource(video, info?.url || null, {
    startAt: () => pendingSeek.current,
    onReady: () => {
      const v = video.current
      if (!v) return
      if (pendingSeek.current && pendingSeek.current > 0 && Math.abs(v.currentTime - pendingSeek.current) > 1) v.currentTime = pendingSeek.current
      pendingSeek.current = null
      setSpeed(v, sessionRate)
      if (autoPlay) v.play().catch(() => setNeedClick(true))
    },
    onError: (m) => setErr(m),
  })

  // Time really watched, for the numbers (a jump doesn't count), and the place
  // to come back to.
  useEffect(() => {
    const v = video.current
    if (!v) return
    const tick = () => { const d = v.currentTime - lastTime.current; if (d > 0 && d < 5) watched.current += d; lastTime.current = v.currentTime }
    const rate = () => { sessionRate = v.playbackRate }
    v.addEventListener('timeupdate', tick)
    v.addEventListener('ratechange', rate)
    return () => { v.removeEventListener('timeupdate', tick); v.removeEventListener('ratechange', rate) }
  }, [])
  useEffect(() => {
    watched.current = 0; lastTime.current = 0
    const save = () => {
      const v = video.current
      if (v && info && v.currentTime >= 5) api.post(`/items/${id}/progress`, { positionSec: v.currentTime, durationSec: info.durationSec || v.duration }).catch(() => {})
    }
    const t = setInterval(save, 10000)
    return () => {
      clearInterval(t); save()
      const dur = info?.durationSec || video.current?.duration || 0
      if (watched.current > 1) api.post(`/items/${id}/view`, { seconds: watched.current, completed: dur > 0 && watched.current >= dur * 0.9 }).catch(() => {})
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, info])

  const seek = (t: number) => { const v = video.current; if (v) v.currentTime = Math.max(0, Math.min(t, info?.durationSec || v.duration || t)) }
  const switchTrack = (set: (n: number) => void, n: number) => { pendingSeek.current = video.current?.currentTime || 0; set(n) }
  const togglePlay = () => { const v = video.current; if (v && !needClick) v.paused ? v.play().catch(() => {}) : v.pause() }
  const onKey = (e: React.KeyboardEvent) => {
    const v = video.current
    const tag = (e.target as HTMLElement).tagName
    if (!v || tag === 'INPUT' || tag === 'SELECT' || (tag === 'BUTTON' && e.key !== 'k')) return
    switch (e.key) {
      case ' ': case 'k': e.preventDefault(); togglePlay(); break
      case 'f': toggleFullscreen(root.current); break
      case 'm': v.muted = !v.muted; break
    }
  }
  const chapters = info?.chapters?.map((c) => c.start)
  return (
    <div className={'player inline ' + className + (compact ? ' compact' : '')} ref={root} tabIndex={0} onKeyDown={onKey} onClick={togglePlay} onDoubleClick={() => toggleFullscreen(root.current)}>
      <div className="stage"><video ref={video} playsInline preload="auto" onEnded={onEnded} /></div>
      {st.waiting && !needClick && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && <div className="big-play"><button aria-label="Play" onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {err && <div className="overlay-msg"><div className="box">Playback failed: {err}</div></div>}
      <Controls video={video} duration={info?.durationSec || durationSec} onSeek={seek} onFullscreen={() => toggleFullscreen(root.current)}
        chapters={chapters} chapterNames={info?.chapters?.map((c) => c.title)}
        audio={info?.audio.map((a) => ({ id: a.index, label: [langName(a.language), a.title, a.codec?.toUpperCase()].filter(Boolean).join(' · '), active: a.index === audio }))}
        subtitles={info?.subtitles.map((s) => ({ id: s.index, label: [langName(s.language), s.title].filter(Boolean).join(' · '), active: s.index === sub }))}
        onAudio={(n) => switchTrack(setAudio, n)} onSubtitle={(n) => switchTrack(setSub, n)} />
    </div>
  )
}
