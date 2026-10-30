import { useEffect, useRef, useState } from 'react'
import { fmtTime, getPrefs } from '../api'
import { Thumb } from '../pages/DiscSections'
import { announce } from '../a11y'
import { IconAngle, IconAudio, IconChapters, IconClose, IconFull, IconMute, IconNext, IconPause, IconPlay, IconPrev, IconRemote, IconSettings, IconSubs, IconVolume } from '../icons'

export type TrackOpt = { id: number; label: string; active: boolean }

type Props = {
  video: React.RefObject<HTMLVideoElement | null>
  duration: number
  chapters?: number[]
  chapterNames?: string[]
  // chapterThumb: a picture of each scene (discs), for the scene picker.
  chapterThumb?: (i: number) => string
  onSeek: (t: number) => void
  onPrev?: () => void
  onNext?: () => void
  audio?: TrackOpt[]
  subtitles?: TrackOpt[]
  onAudio?: (id: number) => void
  onSubtitle?: (id: number) => void
  // When set, the audio/subtitle/angle buttons open the big tile picker
  // instead of the small popover lists.
  onPick?: (kind: 'audio' | 'subtitle' | 'angle') => void
  angles?: number
  onFullscreen: () => void
  onSettings?: () => void
  remoteOn?: boolean
  onRemote?: () => void
  showSeek?: boolean
  showChapterMarks?: boolean
  extra?: React.ReactNode
  disc?: boolean // the keyboard help lists the disc keys
  offset?: number // media time -> displayed time offset
}

export const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3, 3.5, 4, 5, 6, 8, 10, 12, 16]

// setSpeed changes the playback rate without changing the pitch of voices
// (the browser time-stretches the audio). defaultPlaybackRate keeps the
// speed when the player swaps streams (audio track, angle, next clip).
export function setSpeed(v: HTMLMediaElement, rate: number) {
  const a = v as HTMLMediaElement & { mozPreservesPitch?: boolean; webkitPreservesPitch?: boolean }
  a.preservesPitch = true
  a.mozPreservesPitch = true
  a.webkitPreservesPitch = true
  v.defaultPlaybackRate = rate
  v.playbackRate = rate
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

export function Controls({ video, duration, chapters = [], chapterNames, chapterThumb, onSeek, onPrev, onNext, audio, subtitles, onAudio, onSubtitle, onPick, angles = 1,
  onFullscreen, onSettings, remoteOn, onRemote, showSeek = true, showChapterMarks = true, extra, offset = 0, disc = false }: Props) {
  const st = useVideoState(video)
  const [pop, setPop] = useState<'audio' | 'subs' | 'chapters' | 'speed' | null>(null)
  const [maxSpeed, setMaxSpeed] = useState(4)
  const [rate, setRate] = useState(() => video.current?.playbackRate || 1)
  useEffect(() => { getPrefs().then((p) => setMaxSpeed(p.maxSpeed || 4)).catch(() => {}) }, [])
  const speeds = SPEEDS.filter((s) => s <= maxSpeed)
  const changeSpeed = (r: number) => { const v = video.current; if (v) { setSpeed(v, r); setRate(r) } }
  // "<" and ">" step through the speeds, like most video players.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || (e.key !== '<' && e.key !== '>')) return
      const i = speeds.indexOf(rate)
      const next = speeds[Math.max(0, Math.min(speeds.length - 1, (i < 0 ? speeds.indexOf(1) : i) + (e.key === '>' ? 1 : -1)))]
      if (next) changeSpeed(next)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })
  // What changed, said aloud for screen readers: playing, paused, muted,
  // the volume, where a jump landed. Seeks and volume wait a moment so a
  // held key gives one message.
  const [help, setHelp] = useState(false)
  useEffect(() => {
    const v = video.current
    if (!v) return
    let tSeek: ReturnType<typeof setTimeout>, tVol: ReturnType<typeof setTimeout>
    let muted = v.muted
    const onPlay = () => announce('Playing')
    const onPause = () => { if (!v.ended) announce('Paused') }
    const onSeeked = () => { clearTimeout(tSeek); tSeek = setTimeout(() => announce(`At ${fmtTime(v.currentTime + offset)}`), 400) }
    const onVol = () => {
      clearTimeout(tVol)
      if (v.muted !== muted) { muted = v.muted; announce(v.muted ? 'Muted' : 'Sound on'); return }
      tVol = setTimeout(() => announce(`Volume ${Math.round(v.volume * 100)}%`), 300)
    }
    v.addEventListener('play', onPlay); v.addEventListener('pause', onPause); v.addEventListener('seeked', onSeeked); v.addEventListener('volumechange', onVol)
    return () => {
      clearTimeout(tSeek); clearTimeout(tVol)
      v.removeEventListener('play', onPlay); v.removeEventListener('pause', onPause); v.removeEventListener('seeked', onSeeked); v.removeEventListener('volumechange', onVol)
    }
  }, [video, offset])
  // "?" (Shift+/) lists every key.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || e.key !== '?') return
      e.preventDefault(); setHelp((h) => !h)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
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
  // Keyboard/remote seeking while the timeline has focus.
  const seekKey = (e: React.KeyboardEvent) => {
    const to = e.key === 'ArrowRight' ? t + 10 : e.key === 'ArrowLeft' ? t - 10 : e.key === 'PageUp' ? t + 60 : e.key === 'PageDown' ? t - 60 : e.key === 'Home' ? 0 : e.key === 'End' ? duration - 5 : null
    if (to !== null) {
      e.preventDefault(); e.stopPropagation()
      onSeek(Math.max(0, Math.min(duration, to)))
    }
  }
  const curChapter = chapters.filter((c) => c <= t + 0.5).length
  const chapName = (i: number) => chapterNames?.[i] || `Scene ${i + 1}`
  return (
    <div className="bottom" onClick={(e) => e.stopPropagation()}>
      {showSeek && (
        <div className="seek" ref={seekRef} onClick={seekAt} tabIndex={0} role="slider" aria-label="Position" aria-valuemin={0}
          aria-valuemax={Math.round(duration)} aria-valuenow={Math.round(t)} aria-valuetext={`${fmtTime(t)} of ${fmtTime(duration)}`} onKeyDown={seekKey}>
          <div className="track" />
          <div className="buf" style={{ width: bpct + '%' }} />
          <div className="fill" style={{ width: pct + '%' }} />
          {showChapterMarks && duration > 0 && chapters.slice(1).map((c, i) => <div key={i} className="chap" style={{ left: (c / duration) * 100 + '%' }} title={chapName(i + 1)} />)}
          <div className="knob" style={{ left: pct + '%' }} />
        </div>
      )}
      <div className="ctl">
        {onPrev && <button className="ibtn chap-step" onClick={onPrev} title="Previous scene"><IconPrev /></button>}
        <button className="ibtn big" onClick={toggle} title={st.playing ? 'Pause (Space)' : 'Play (Space)'} aria-label={st.playing ? 'Pause' : 'Play'}>{st.playing ? <IconPause /> : <IconPlay />}</button>
        {onNext && <button className="ibtn chap-step" onClick={onNext} title="Next scene"><IconNext /></button>}
        <button className="ibtn hide-narrow" onClick={() => { const v = video.current; if (v) v.muted = !v.muted }} title="Mute (M)" aria-label="Mute" aria-pressed={st.muted}>{st.muted || st.volume === 0 ? <IconMute /> : <IconVolume />}</button>
        <input className="range vol hide-narrow" type="range" min={0} max={1} step={0.02} value={st.muted ? 0 : st.volume} aria-label="Volume" aria-valuetext={st.muted ? 'Muted' : `${Math.round(st.volume * 100)}%`}
          onChange={(e) => { const v = video.current; if (v) { v.volume = Number(e.target.value); v.muted = false } }} style={{ '--pct': (st.muted ? 0 : st.volume * 100) + '%' } as React.CSSProperties} />
        <span className="time" aria-hidden="true">
          {fmtTime(t)}{showSeek ? ` / ${fmtTime(duration)}` : ''}
          {!showSeek && chapters.length > 1 ? ` · ${chapName(Math.max(0, curChapter - 1))}` : ''}
        </span>
        <div className="spacer" />
        {extra}
        <button className="btn sm speed-btn" title="Playback speed (< and >)" aria-label={`Playback speed ${rate} times`} onClick={() => setPop(pop === 'speed' ? null : 'speed')}>{rate}×</button>
        {chapters.length > 1 && <button className={'ibtn' + (pop === 'chapters' ? ' on' : '')} title="Scenes" aria-expanded={pop === 'chapters'} onClick={() => setPop(pop === 'chapters' ? null : 'chapters')}><IconChapters /></button>}
        {audio && audio.length > 1 && <button className="ibtn" title="Audio & language" onClick={() => (onPick ? onPick('audio') : setPop(pop === 'audio' ? null : 'audio'))}><IconAudio /></button>}
        {subtitles && subtitles.length > 0 && <button className="ibtn" title="Subtitles" onClick={() => (onPick ? onPick('subtitle') : setPop(pop === 'subs' ? null : 'subs'))}><IconSubs /></button>}
        {onPick && angles > 1 && <button className="ibtn" title="Video version (camera angle)" onClick={() => onPick('angle')}><IconAngle /></button>}
        {onRemote && (
          <button className={'btn sm remote-toggle' + (remoteOn ? ' primary' : '')} onClick={onRemote} aria-pressed={remoteOn} title="Show or hide the on-screen remote (r)">
            <IconRemote /> <span>Remote</span>
          </button>
        )}
        {onSettings && <button className="ibtn" title="Viewing settings" onClick={onSettings}><IconSettings /></button>}
        <button className="ibtn hide-narrow kbd-help" title="Keyboard shortcuts (?)" aria-label="Keyboard shortcuts" aria-haspopup="dialog" onClick={() => setHelp(true)}>?</button>
        <button className="ibtn" title="Fullscreen (F)" aria-label="Fullscreen" onClick={onFullscreen}><IconFull /></button>
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
      {pop === 'speed' && (
        <div className="popover speed-pop"><h4>Playback speed</h4>
          {speeds.map((s) => <button key={s} className={s === rate ? 'active' : ''} onClick={() => { changeSpeed(s); setPop(null) }}>{s === 1 ? 'Normal' : `${s}×`}{s > 4 && <span className="muted" style={{ marginLeft: 'auto', fontSize: 11 }}>may be muted</span>}</button>)}
        </div>
      )}
      {help && <ShortcutHelp disc={disc} onClose={() => setHelp(false)} />}
      {pop === 'chapters' && (
        <ScenePanel chapters={chapters} name={chapName} thumb={chapterThumb} current={curChapter - 1}
          onPick={(c) => { onSeek(c); setPop(null) }} onClose={() => setPop(null)} />
      )}
    </div>
  )
}

// ScenePanel: every scene as a picture with its start time and name, like
// the disc page, with the one playing now marked.
function ScenePanel({ chapters, name, thumb, current, onPick, onClose }: {
  chapters: number[]; name: (i: number) => string; thumb?: (i: number) => string; current: number
  onPick: (t: number) => void; onClose: () => void
}) {
  const grid = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    grid.current?.querySelector<HTMLElement>('.scene.now')?.scrollIntoView({ block: 'nearest' })
    grid.current?.querySelector<HTMLButtonElement>('.scene.now .scene-thumb')?.focus({ preventScroll: true })
  }, [])
  return (
    <div className="scene-panel" role="dialog" aria-label="Scenes" onKeyDown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); onClose() } }}>
      <div className="scene-panel-head">
        <h3>Scenes</h3>
        <span className="muted">{chapters.length} scenes</span>
        <button className="ibtn" onClick={onClose} title="Close" aria-label="Close scenes"><IconClose /></button>
      </div>
      <div className="scene-grid" ref={grid}>
        {chapters.map((c, i) => (
          <div key={i} className={'scene' + (i === current ? ' now' : '')}>
            <button className="scene-thumb" onClick={() => onPick(c)} aria-label={`Play ${name(i)}, from ${fmtTime(c)}`} aria-current={i === current || undefined}>
              {thumb ? <Thumb src={thumb(i)} /> : <span className="scene-num">{i + 1}</span>}
              {i === current && <span className="scene-now">Now playing</span>}
              <span className="scene-time">{fmtTime(c)}</span>
            </button>
            <div className="scene-name"><span className="muted">{i + 1}.</span> {name(i)}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

// ShortcutHelp: every key the player understands, in one list.
const KEYS: [string, string, boolean?][] = [
  ['Space or K', 'Play or pause'],
  ['← and →', 'Back 10 seconds, forward 10 seconds'],
  ['Page Up and Page Down', 'Forward or back a minute (on the timeline)'],
  ['Home and End', 'The start or the end (on the timeline)'],
  ['↑ and ↓', 'Volume up or down'],
  ['M', 'Mute or unmute'],
  ['< and >', 'Slower or faster'],
  ['F', 'Fullscreen'],
  ['R', 'Show or hide the remote'],
  ['1 to 9', 'Jump to that scene, or press that menu button', true],
  ['Arrow keys and Enter', 'Move around a disc menu and choose', true],
  ['Esc', 'Close a window, or shrink the player'],
  ['?', 'This list'],
]
function ShortcutHelp({ disc, onClose }: { disc: boolean; onClose: () => void }) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal kbd-modal" role="dialog" aria-modal="true" aria-labelledby="kbd-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="kbd-h">Keyboard shortcuts</h2>
        <dl className="kbd-list">
          {KEYS.filter(([, , d]) => !d || disc).map(([k, what]) => <div key={k}><dt><kbd>{k}</kbd></dt><dd>{what}</dd></div>)}
        </dl>
        <p className="small-note">Game controllers and TV remotes work too: the D-pad moves, A or OK chooses, B goes back.</p>
        <div className="actions"><button className="btn primary" onClick={onClose}>Close</button></div>
      </div>
    </div>
  )
}
