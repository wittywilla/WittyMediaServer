import { useEffect, useRef, useState } from 'react'
import { fmtTime } from '../api'
import { IconAudio, IconChapters, IconFull, IconMute, IconNext, IconPause, IconPlay, IconPrev, IconSubs, IconVolume } from '../icons'

export type TrackOpt = { id: number; label: string; active: boolean }

type Props = {
  video: React.RefObject<HTMLVideoElement | null>
  duration: number
  chapters?: number[]
  onSeek: (t: number) => void
  onPrev?: () => void
  onNext?: () => void
  audio?: TrackOpt[]
  subtitles?: TrackOpt[]
  onAudio?: (id: number) => void
  onSubtitle?: (id: number) => void
  onFullscreen: () => void
  extra?: React.ReactNode
  offset?: number // media time -> displayed time offset
}

export function useVideoState(video: React.RefObject<HTMLVideoElement | null>) {
  const [playing, setPlaying] = useState(false)
  const [time, setTime] = useState(0)
  const [buffered, setBuffered] = useState(0)
  const [waiting, setWaiting] = useState(false)
  const [muted, setMuted] = useState(false)
  const [volume, setVolume] = useState(1)
  useEffect(() => {
    const v = video.current
    if (!v) return
    const upd = () => {
      setPlaying(!v.paused && !v.ended)
      setTime(v.currentTime)
      try { setBuffered(v.buffered.length ? v.buffered.end(v.buffered.length - 1) : 0) } catch { /* ignore */ }
      setMuted(v.muted); setVolume(v.volume)
    }
    const ev = ['play', 'pause', 'timeupdate', 'progress', 'volumechange', 'ended', 'seeking', 'seeked', 'loadedmetadata']
    ev.forEach((e) => v.addEventListener(e, upd))
    const w1 = () => setWaiting(true), w2 = () => setWaiting(false)
    v.addEventListener('waiting', w1); v.addEventListener('playing', w2); v.addEventListener('canplay', w2); v.addEventListener('seeked', w2)
    upd()
    return () => { ev.forEach((e) => v.removeEventListener(e, upd)); v.removeEventListener('waiting', w1); v.removeEventListener('playing', w2); v.removeEventListener('canplay', w2); v.removeEventListener('seeked', w2) }
  }, [video])
  return { playing, time, buffered, waiting, muted, volume }
}

export function Controls({ video, duration, chapters = [], onSeek, onPrev, onNext, audio, subtitles, onAudio, onSubtitle, onFullscreen, extra, offset = 0 }: Props) {
  const st = useVideoState(video)
  const [pop, setPop] = useState<'audio' | 'subs' | 'chapters' | null>(null)
  const seekRef = useRef<HTMLDivElement | null>(null)
  const t = st.time + offset
  const pct = duration > 0 ? Math.min(100, (t / duration) * 100) : 0
  const bpct = duration > 0 ? Math.min(100, ((st.buffered + offset) / duration) * 100) : 0
  const toggle = () => { const v = video.current; if (!v) return; v.paused ? v.play().catch(() => {}) : v.pause() }
  const seekAt = (e: React.MouseEvent) => {
    const r = seekRef.current!.getBoundingClientRect()
    const f = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width))
    onSeek(f * duration)
  }
  const curChapter = chapters.filter((c) => c <= t + 0.5).length
  return (
    <div className="bottom" onClick={(e) => e.stopPropagation()}>
      <div className="seek" ref={seekRef} onClick={seekAt}>
        <div className="track" />
        <div className="buf" style={{ width: bpct + '%' }} />
        <div className="fill" style={{ width: pct + '%' }} />
        {chapters.slice(1).map((c, i) => <div key={i} className="chap" style={{ left: (c / duration) * 100 + '%' }} />)}
        <div className="knob" style={{ left: pct + '%' }} />
      </div>
      <div className="ctl">
        {onPrev && <button className="ibtn" onClick={onPrev} title="Previous chapter"><IconPrev /></button>}
        <button className="ibtn big" onClick={toggle} title="Play/Pause">{st.playing ? <IconPause /> : <IconPlay />}</button>
        {onNext && <button className="ibtn" onClick={onNext} title="Next chapter"><IconNext /></button>}
        <button className="ibtn" onClick={() => { const v = video.current; if (v) v.muted = !v.muted }}>{st.muted || st.volume === 0 ? <IconMute /> : <IconVolume />}</button>
        <input type="range" min={0} max={1} step={0.02} value={st.muted ? 0 : st.volume} onChange={(e) => { const v = video.current; if (v) { v.volume = Number(e.target.value); v.muted = false } }} style={{ width: 90, accentColor: 'var(--accent)' }} />
        <span className="time">{fmtTime(t)} / {fmtTime(duration)}</span>
        <div className="spacer" />
        {extra}
        {chapters.length > 1 && <button className="ibtn" title="Chapters" onClick={() => setPop(pop === 'chapters' ? null : 'chapters')}><IconChapters /></button>}
        {audio && audio.length > 1 && <button className="ibtn" title="Audio" onClick={() => setPop(pop === 'audio' ? null : 'audio')}><IconAudio /></button>}
        {subtitles && subtitles.length > 0 && <button className="ibtn" title="Subtitles" onClick={() => setPop(pop === 'subs' ? null : 'subs')}><IconSubs /></button>}
        <button className="ibtn" title="Fullscreen" onClick={onFullscreen}><IconFull /></button>
      </div>
      {pop === 'audio' && audio && (
        <div className="popover"><h4>Audio</h4>{audio.map((a) => <button key={a.id} className={a.active ? 'active' : ''} onClick={() => { onAudio?.(a.id); setPop(null) }}>{a.label}</button>)}</div>
      )}
      {pop === 'subs' && subtitles && (
        <div className="popover"><h4>Subtitles</h4>
          <button className={!subtitles.some((s) => s.active) ? 'active' : ''} onClick={() => { onSubtitle?.(-1); setPop(null) }}>Off</button>
          {subtitles.map((s) => <button key={s.id} className={s.active ? 'active' : ''} onClick={() => { onSubtitle?.(s.id); setPop(null) }}>{s.label}</button>)}
        </div>
      )}
      {pop === 'chapters' && (
        <div className="popover" style={{ maxHeight: '50vh', overflow: 'auto' }}><h4>Chapters</h4>
          {chapters.map((c, i) => <button key={i} className={i + 1 === curChapter ? 'active' : ''} onClick={() => { onSeek(c); setPop(null) }}>Chapter {i + 1} <span className="muted" style={{ marginLeft: 'auto' }}>{fmtTime(c)}</span></button>)}
        </div>
      )}
    </div>
  )
}
