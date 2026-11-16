// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { fmtTime, getPrefs } from '../api'
import { Thumb } from './ChapterThumb'
import { announce } from '../a11y'
import { SPEEDS, setSpeed } from './speed'
import { IconAngle, IconAudio, IconChapters, IconClose, IconFull, IconMute, IconInfo, IconMore, IconNext, IconPause, IconPlay, IconPrev, IconRemote, IconSettings, IconSubs, IconVolume } from '../icons'
import { t, tk, tn, tx } from '../i18n'

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
  copy?: boolean // a copy of a disc's film: N and P step through its chapters
  offset?: number // media time -> displayed time offset
  // The chip beside the pickers ("4K", "720p"): what the picture is, and a press says how it is playing.
  quality?: string
  onQuality?: () => void
}

export { SPEEDS, setSpeed }

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
  onFullscreen, onSettings, remoteOn, onRemote, showSeek = true, showChapterMarks = true, extra, offset = 0, disc = false, copy = false, quality, onQuality }: Props) {
  const st = useVideoState(video)
  const [pop, setPop] = useState<'audio' | 'subs' | 'chapters' | 'speed' | 'more' | null>(null)
  // Where the pointer is on the timeline, or being dragged to (0 to 1): the time and scene there are shown above it.
  const [aim, setAim] = useState<{ f: number; drag: boolean } | null>(null)
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
    const onPlay = () => announce(t('Playing'))
    const onPause = () => { if (!v.ended) announce(t('Paused')) }
    const onSeeked = () => { clearTimeout(tSeek); tSeek = setTimeout(() => announce(t('At {time}', { time: fmtTime(v.currentTime + offset) })), 400) }
    const onVol = () => {
      clearTimeout(tVol)
      if (v.muted !== muted) { muted = v.muted; announce(v.muted ? t('Muted') : t('Sound on')); return }
      tVol = setTimeout(() => announce(t('Volume {percent}%', { percent: Math.round(v.volume * 100) })), 300)
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
  const now = st.time + offset
  const pct = duration > 0 ? Math.min(100, (now / duration) * 100) : 0
  const bpct = duration > 0 ? Math.min(100, ((st.buffered + offset) / duration) * 100) : 0
  const toggle = () => { const v = video.current; if (!v) return; v.paused ? v.play().catch(() => {}) : v.pause() }
  const fracAt = (e: React.PointerEvent | React.MouseEvent) => {
    const r = seekRef.current!.getBoundingClientRect()
    return Math.min(1, Math.max(0, (e.clientX - r.left) / r.width))
  }
  // Pressing and dragging scrubs: the knob follows, and the place is asked for once, on release.
  const seekDown = (e: React.PointerEvent) => {
    if (e.button !== 0) return
    e.currentTarget.setPointerCapture(e.pointerId)
    setAim({ f: fracAt(e), drag: true })
  }
  const seekMove = (e: React.PointerEvent) => setAim((a) => ({ f: fracAt(e), drag: !!a?.drag }))
  const seekUp = (e: React.PointerEvent) => {
    const f = fracAt(e)
    setAim(e.pointerType === 'mouse' ? { f, drag: false } : null)
    onSeek(f * duration)
  }
  // Keyboard/remote seeking while the timeline has focus.
  const seekKey = (e: React.KeyboardEvent) => {
    const to = e.key === 'ArrowRight' ? now + 10 : e.key === 'ArrowLeft' ? now - 10 : e.key === 'PageUp' ? now + 60 : e.key === 'PageDown' ? now - 60 : e.key === 'Home' ? 0 : e.key === 'End' ? duration - 5 : null
    if (to !== null) {
      e.preventDefault(); e.stopPropagation()
      onSeek(Math.max(0, Math.min(duration, to)))
    }
  }
  const curChapter = chapters.filter((c) => c <= now + 0.5).length
  const chapName = (i: number) => chapterNames?.[i] || t('Scene {n}', { n: i + 1 })
  return (
    <div className="bottom" onClick={(e) => e.stopPropagation()}>
      {showSeek && (
        <div className={'seek' + (aim?.drag ? ' scrubbing' : '')} ref={seekRef} onPointerDown={seekDown} onPointerMove={seekMove} onPointerUp={seekUp} onPointerLeave={() => setAim((a) => (a?.drag ? a : null))} onPointerCancel={() => setAim(null)} tabIndex={0} role="slider" aria-label={t('Position')} aria-valuemin={0}
          aria-valuemax={Math.round(duration)} aria-valuenow={Math.round(now)} aria-valuetext={t('{time} of {total}', { time: fmtTime(now), total: fmtTime(duration) })} onKeyDown={seekKey}>
          <div className="track" />
          <div className="buf" style={{ width: bpct + '%' }} />
          <div className="fill" style={{ width: (aim?.drag ? aim.f * 100 : pct) + '%' }} />
          {showChapterMarks && duration > 0 && chapters.slice(1).map((c, i) => <div key={i} className="chap" style={{ left: (c / duration) * 100 + '%' }} title={chapName(i + 1)} />)}
          <div className="knob" style={{ left: (aim?.drag ? aim.f * 100 : pct) + '%' }} />
          {aim && duration > 0 && <SeekTip f={aim.f} duration={duration} chapters={chapters} name={chapName} thumb={chapterThumb} />}
        </div>
      )}
      <div className="ctl">
        <div className="ctl-left">
          <span className="time" aria-hidden="true">
            {fmtTime(now)}{showSeek ? ` / ${fmtTime(duration)}` : ''}
            {!showSeek && chapters.length > 1 ? ` · ${chapName(Math.max(0, curChapter - 1))}` : ''}
          </span>
          <button className="ibtn hide-narrow" onClick={() => { const v = video.current; if (v) v.muted = !v.muted }} title={t('Mute (M)')} aria-label={t('Mute')} aria-pressed={st.muted}>{st.muted || st.volume === 0 ? <IconMute /> : <IconVolume />}</button>
          <input className="range vol hide-narrow" type="range" min={0} max={1} step={0.02} value={st.muted ? 0 : st.volume} aria-label={t('Volume')} aria-valuetext={st.muted ? t('Muted') : `${Math.round(st.volume * 100)}%`}
            onChange={(e) => { const v = video.current; if (v) { v.volume = Number(e.target.value); v.muted = false } }} style={{ '--pct': (st.muted ? 0 : st.volume * 100) + '%' } as React.CSSProperties} />
        </div>
        <div className="ctl-mid" role="group" aria-label={t('Playback')}>
          {onPrev && <button className="cbtn chap-step" onClick={onPrev} title={t('Previous chapter, or the start')} aria-label={t('Previous')}><IconPrev /><span className="lbl">{t('Prev')}</span></button>}
          <button className="cbtn step" onClick={() => onSeek(Math.max(0, now - 10))} title={t('Back 10 seconds (←)')} aria-label={t('Back 10 seconds')}><span className="lbl">−10</span></button>
          <button className="ibtn big play" onClick={toggle} title={st.playing ? t('Pause (Space)') : t('Play (Space)')} aria-label={st.playing ? t('Pause') : t('Play')}>{st.playing ? <IconPause /> : <IconPlay />}</button>
          <button className="cbtn step" onClick={() => onSeek(Math.min(duration || now + 30, now + 30))} title={t('Forward 30 seconds')} aria-label={t('Forward 30 seconds')}><span className="lbl">+30</span></button>
          {onNext && <button className="cbtn chap-step" onClick={onNext} title={t('Next chapter, or the next one')} aria-label={t('Next')}><span className="lbl">{t('Next')}</span><IconNext /></button>}
        </div>
        <div className="ctl-right">
          {extra}
          {audio && audio.length > 1 && <button className="cbtn" title={t('Audio & language')} aria-label={t('Audio and language')} onClick={() => (onPick ? onPick('audio') : setPop(pop === 'audio' ? null : 'audio'))}><IconAudio /><span className="lbl">{t('Audio')}</span></button>}
          {subtitles && subtitles.length > 0 && <button className="cbtn" title={t('Subtitles')} aria-label={t('Subtitles')} onClick={() => (onPick ? onPick('subtitle') : setPop(pop === 'subs' ? null : 'subs'))}><IconSubs /><span className="lbl">{t('Subtitles')}</span></button>}
          {onPick && angles > 1 && <button className="cbtn" title={t('Video version (camera angle)')} aria-label={t('Video version')} onClick={() => onPick('angle')}><IconAngle /><span className="lbl">{t('Angle')}</span></button>}
          {quality && <button className="cbtn chip-q" title={t('How this is playing (I)')} aria-label={t('Picture {quality}. How this is playing', { quality })} onClick={onQuality} disabled={!onQuality}><span className="lbl always">{quality}</span></button>}
          <button className="cbtn chip-s" title={t('Playback speed (< and >)')} aria-label={t('Playback speed {rate} times', { rate })} aria-expanded={pop === 'speed'} onClick={() => setPop(pop === 'speed' ? null : 'speed')}><span className="lbl always">{rate}×</span></button>
          {chapters.length > 1 && <button className={'cbtn' + (pop === 'chapters' ? ' on' : '')} title={t('Scenes')} aria-label={t('Chapters')} aria-expanded={pop === 'chapters'} onClick={() => setPop(pop === 'chapters' ? null : 'chapters')}><IconChapters /><span className="lbl">{t('Chapters')}</span></button>}
          {onRemote && (
            <button className={'cbtn remote-toggle' + (remoteOn ? ' on' : '')} onClick={onRemote} aria-pressed={remoteOn} title={t('Show or hide WittyRemote (R)')}>
              <IconRemote /><span className="lbl">WittyRemote</span>
            </button>
          )}
          <button className={'cbtn more' + (pop === 'more' ? ' on' : '')} title={t('More')} aria-label={t('More')} aria-haspopup="menu" aria-expanded={pop === 'more'} onClick={() => setPop(pop === 'more' ? null : 'more')}><IconMore /></button>
          <button className="ibtn" title={t('Full screen (F)')} aria-label={t('Full screen')} onClick={onFullscreen}><IconFull /></button>
        </div>
      </div>
      {pop === 'more' && (
        <div className="popover more-pop" role="menu" aria-label={t('More')}>
          {onSettings && <button role="menuitem" onClick={() => { setPop(null); onSettings() }}><IconSettings /> {t('Viewing settings')}</button>}
          {onQuality && <button role="menuitem" onClick={() => { setPop(null); onQuality() }}><IconInfo /> {t('How this is playing')}</button>}
          <button role="menuitem" onClick={() => { setPop(null); setHelp(true) }}><span className="kbd-help" aria-hidden="true">?</span> {t('Keyboard shortcuts')}</button>
        </div>
      )}
      {pop === 'audio' && audio && (
        <div className="popover"><h4>{t('Audio')}</h4>{audio.map((a) => <button key={a.id} className={a.active ? 'active' : ''} onClick={() => { onAudio?.(a.id); setPop(null) }}>{a.label}</button>)}</div>
      )}
      {pop === 'subs' && subtitles && (
        <div className="popover"><h4>{t('Subtitles')}</h4>
          <button className={!subtitles.some((s) => s.active) ? 'active' : ''} onClick={() => { onSubtitle?.(-1); setPop(null) }}>{t('Off')}</button>
          {subtitles.map((s) => <button key={s.id} className={s.active ? 'active' : ''} onClick={() => { onSubtitle?.(s.id); setPop(null) }}>{s.label}</button>)}
        </div>
      )}
      {pop === 'speed' && (
        <div className="popover speed-pop"><h4>{t('Playback speed')}</h4>
          {speeds.map((s) => <button key={s} className={s === rate ? 'active' : ''} onClick={() => { changeSpeed(s); setPop(null) }}>{s === 1 ? t('Normal') : `${s}×`}{s > 4 && <span className="muted" style={{ marginLeft: 'auto', fontSize: 11 }}>{t('may be muted')}</span>}</button>)}
        </div>
      )}
      {help && <ShortcutHelp disc={disc} copy={copy} onClose={() => setHelp(false)} />}
      {pop === 'chapters' && (
        <ScenePanel chapters={chapters} name={chapName} thumb={chapterThumb} current={curChapter - 1}
          onPick={(c) => { onSeek(c); setPop(null) }} onClose={() => setPop(null)} />
      )}
    </div>
  )
}

// SeekTip: the time under the pointer on the timeline, and the scene it falls in, with its picture when there are pictures.
function SeekTip({ f, duration, chapters, name, thumb }: { f: number; duration: number; chapters: number[]; name: (i: number) => string; thumb?: (i: number) => string }) {
  const at = f * duration
  const cur = chapters.length > 1 ? Math.max(0, chapters.filter((c) => c <= at + 0.5).length - 1) : -1
  return (
    <div className="seek-tip" style={{ left: `clamp(4.5rem, ${f * 100}%, calc(100% - 4.5rem))` }} aria-hidden="true">
      {cur >= 0 && thumb && <span className="seek-tip-pic"><Thumb src={thumb(cur)} /></span>}
      <span className="seek-tip-time">{fmtTime(at)}</span>
      {cur >= 0 && <span className="seek-tip-chap">{name(cur)}</span>}
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
    <div className="scene-panel" role="dialog" aria-label={t('Scenes')} onKeyDown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); onClose() } }}>
      <div className="scene-panel-head">
        <h3>{t('Scenes')}</h3>
        <span className="muted">{tn('{count} scene', '{count} scenes', chapters.length)}</span>
        <button className="ibtn" onClick={onClose} title={t('Close')} aria-label={t('Close scenes')}><IconClose /></button>
      </div>
      <div className="scene-grid" ref={grid}>
        {chapters.map((c, i) => (
          <div key={i} className={'scene' + (i === current ? ' now' : '')}>
            <button className="scene-thumb" onClick={() => onPick(c)} aria-label={t('Play {name}, from {time}', { name: name(i), time: fmtTime(c) })} aria-current={i === current || undefined}>
              {thumb ? <Thumb src={thumb(i)} /> : <span className="scene-num">{i + 1}</span>}
              {i === current && <span className="scene-now">{t('Now playing')}</span>}
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
// The third column keeps a line to discs, or to everything else: the two handle some keys differently.
// chapters: discs, and copies of a disc's film.
const KEYS: [string, string, ('disc' | 'file' | 'chapters')?][] = [
  [tk('Space or K'), tk('Play or pause')],
  ['← and →', tk('Back 10 seconds, forward 10 seconds')],
  [tk('Page Up and Page Down'), tk('Forward or back a minute (on the timeline)')],
  [tk('Home and End'), tk('The start or the end (on the timeline)')],
  ['↑ and ↓', tk('Volume up or down'), 'file'],
  ['M', tk('Mute or unmute'), 'file'],
  ['I', tk('Details of what’s playing'), 'file'],
  ['< and >', tk('Slower or faster')],
  ['F', tk('Fullscreen')],
  ['R', tk('Show or hide the remote')],
  ['M', tk('The disc’s menu, or its pop-up menu'), 'disc'],
  ['T', tk('The disc’s title menu'), 'disc'],
  ['A and S', tk('Next sound track, next subtitles'), 'disc'],
  ['N and P', tk('Next chapter, previous chapter'), 'chapters'],
  [tk('1 to 9'), tk('Jump to that scene, or press that menu button'), 'disc'],
  [tk('Arrow keys and Enter'), tk('Move around a disc menu and choose'), 'disc'],
  [tk('F1 to F4'), tk('The red, green, yellow and blue keys of a Blu-ray’s Java menu'), 'disc'],
  ['↑ and ↓', tk('Reach the player’s buttons'), 'disc'],
  [tk('Esc'), tk('Close a window, or shrink the player')],
  ['?', tk('This list')],
]
function ShortcutHelp({ disc, copy, onClose }: { disc: boolean; copy: boolean; onClose: () => void }) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal kbd-modal" role="dialog" aria-modal="true" aria-labelledby="kbd-h" onClick={(e) => e.stopPropagation()}>
        <h2 id="kbd-h">{t('Keyboard shortcuts')}</h2>
        <dl className="kbd-list">
          {KEYS.filter(([, , only]) => !only || (only === 'chapters' ? disc || copy : only === (disc ? 'disc' : 'file'))).map(([k, what]) => <div key={k + what}><dt><kbd>{tx(k)}</kbd></dt><dd>{tx(what)}</dd></div>)}
        </dl>
        <p className="small-note">{t('Game controllers and TV remotes work too: the D-pad moves, A or OK chooses, B goes back.')}</p>
        <div className="actions"><button className="btn primary" onClick={onClose}>{t('Close')}</button></div>
      </div>
    </div>
  )
}
