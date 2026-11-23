// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { getTvMode } from '../tvmode/mode'
import { HELP_EVENT } from '../ui/KeyHelp.event'
import { useEffect, useRef, useState } from 'react'
import { fmtTime, getPrefs } from '../api'
import { Thumb } from './ChapterThumb'
import { announce } from '../a11y'
import { SPEEDS, setSpeed } from './speed'
import { IconAngle, IconAudio, IconBolt, IconChapters, IconCheck, IconTune, IconBack, IconClose, IconFull, IconMute, IconInfo, IconMore, IconNext, IconPause, IconPlay, IconPrev, IconRemote, IconSettings, IconSubs, IconVolume } from '../icons'
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
  // onEditInfo: open the scenes and skips editor beside the picture (administrators).
  onEditInfo?: () => void
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
  // The quality menu (QualityMenu.tsx), opened from the chip; without it the chip opens onQuality.
  onQualityMenu?: () => void
  qualityMenuOpen?: boolean
  // A phone (or a phone on its side): the picture first. Play and the skips sit big in the middle of the picture, the
  // time, Options and Full screen along the bottom over the seek bar, and everything else in the Options sheet.
  compact?: boolean
  // pauseForScenes: the film waits while the scenes panel is open, and plays on when it closes (ViewSettings, Pause for
  // scenes: phones, tablets and TVs by default, every screen if chosen).
  pauseForScenes?: boolean
  // More rows for the Options sheet from the player around these controls (watch party, mini player).
  sheetItems?: SheetItem[]
}

export type SheetItem = { key: string; icon: React.ReactNode; label: string; value?: string; on?: boolean; onClick: () => void }

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
  onFullscreen, onSettings, onEditInfo, remoteOn, onRemote, showSeek = true, showChapterMarks = true, extra, offset = 0, disc = false, copy = false, quality, onQuality, onQualityMenu, qualityMenuOpen, compact = false, sheetItems = [], pauseForScenes = false }: Props) {
  const st = useVideoState(video)
  const [pop, setPop] = useState<'audio' | 'subs' | 'chapters' | 'speed' | 'more' | null>(null)
  const resumeAfterScenes = useRef(false)
  useEffect(() => {
    const v = video.current
    if (!v) return
    if (pop === 'chapters') {
      if (pauseForScenes && !v.paused) { v.pause(); resumeAfterScenes.current = true }
    } else if (resumeAfterScenes.current) {
      resumeAfterScenes.current = false
      v.play().catch(() => {})
    }
  }, [pop]) // eslint-disable-line react-hooks/exhaustive-deps
  const barRef = useRef<HTMLDivElement | null>(null)
  const [sheet, setSheet] = useState<SheetPage | null>(null)
  useEffect(() => { if (!compact) setSheet(null) }, [compact])
  // A menu closed by Esc or Back hands focus back to the button that opened it, so the remote isn't lost.
  const closePop = (refocus: boolean) => {
    const opener = barRef.current?.querySelector<HTMLElement>('.ctl [aria-expanded="true"]')
    setPop(null)
    if (refocus) opener?.focus({ preventScroll: true })
  }
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
    <div className="bottom" ref={barRef} onClick={(e) => e.stopPropagation()}>
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
      {compact ? (
        <div className="ctl compact">
          <span className="time" aria-hidden="true">{fmtTime(now)}{showSeek ? ` / ${fmtTime(duration)}` : ''}</span>
          <div className="ctl-mid" role="group" aria-label={t('Playback')}>
            <button className="pbig step" onClick={() => onSeek(Math.max(0, now - 10))} aria-label={t('Back 10 seconds')}><span aria-hidden="true">−10</span></button>
            <button className="ibtn big play" onClick={toggle} aria-label={st.playing ? t('Pause') : t('Play')}>{st.playing ? <IconPause /> : <IconPlay />}</button>
            <button className="pbig step" onClick={() => onSeek(Math.min(duration || now + 30, now + 30))} aria-label={t('Forward 30 seconds')}><span aria-hidden="true">+30</span></button>
          </div>
          <div className="ctl-right">
            {onRemote && <button className={'ibtn remote-toggle' + (remoteOn ? ' on' : '')} onClick={onRemote} aria-label="WittyRemote" aria-pressed={!!remoteOn} title={t('Show or hide WittyRemote (R)')}><IconRemote /></button>}
            <button className="ibtn" onClick={() => setSheet('main')} aria-label={t('Options')} title={t('Options')} aria-haspopup="dialog" aria-expanded={!!sheet}><IconSettings /></button>
            <button className="ibtn" onClick={onFullscreen} aria-label={t('Full screen')} title={t('Full screen')}><IconFull /></button>
          </div>
        </div>
      ) : (
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
          {audio && audio.length > 1 && <button className="cbtn" title={t('Audio & language')} aria-label={t('Audio and language')} aria-haspopup={onPick ? 'dialog' : 'menu'} aria-expanded={pop === 'audio'} onClick={() => (onPick ? onPick('audio') : setPop(pop === 'audio' ? null : 'audio'))}><IconAudio /><span className="lbl">{t('Audio')}</span></button>}
          {subtitles && subtitles.length > 0 && <button className="cbtn" title={t('Subtitles')} aria-label={t('Subtitles')} aria-haspopup={onPick ? 'dialog' : 'menu'} aria-expanded={pop === 'subs'} onClick={() => (onPick ? onPick('subtitle') : setPop(pop === 'subs' ? null : 'subs'))}><IconSubs /><span className="lbl">{t('Subtitles')}</span></button>}
          {onPick && angles > 1 && <button className="cbtn" title={t('Video version (camera angle)')} aria-label={t('Video version')} onClick={() => onPick('angle')}><IconAngle /><span className="lbl">{t('Angle')}</span></button>}
          {quality && (onQualityMenu
            ? <button className={'cbtn chip-q' + (qualityMenuOpen ? ' on' : '')} title={t('Quality (Q)')} aria-label={t('Quality: {quality}', { quality })} aria-haspopup="dialog" aria-expanded={!!qualityMenuOpen} onClick={onQualityMenu}><span className="lbl always">{quality}</span></button>
            : <button className="cbtn chip-q" title={t('How this is playing (I)')} aria-label={t('Picture {quality}. How this is playing', { quality })} onClick={onQuality} disabled={!onQuality}><span className="lbl always">{quality}</span></button>)}
          <button className="cbtn chip-s" title={t('Playback speed (< and >)')} aria-label={t('Playback speed {rate} times', { rate })} aria-haspopup="menu" aria-expanded={pop === 'speed'} onClick={() => setPop(pop === 'speed' ? null : 'speed')}><span className="lbl always">{rate}×</span></button>
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
      )}
      {sheet && (() => {
        const close = () => setSheet(null)
        const then = (f?: () => void) => () => { setSheet(null); f?.() }
        const activeAudio = audio?.find((a) => a.active)
        const activeSub = subtitles?.find((x) => x.active)
        const rows: SheetItem[] = [
          ...(audio && audio.length > 1 ? [{ key: 'audio', icon: <IconAudio />, label: t('Audio'), value: activeAudio?.label, onClick: onPick ? then(() => onPick('audio')) : () => setSheet('audio') }] : []),
          ...(subtitles && subtitles.length > 0 ? [{ key: 'subs', icon: <IconSubs />, label: t('Subtitles'), value: activeSub?.label || t('Off'), onClick: onPick ? then(() => onPick('subtitle')) : () => setSheet('subs') }] : []),
          ...(onPick && angles > 1 ? [{ key: 'angle', icon: <IconAngle />, label: t('Video version'), onClick: then(() => onPick('angle')) }] : []),
          ...(quality && (onQualityMenu || onQuality) ? [{ key: 'quality', icon: <IconTune />, label: t('Quality'), value: quality, onClick: then(onQualityMenu || onQuality) }] : []),
          { key: 'speed', icon: <IconBolt />, label: t('Playback speed'), value: rate === 1 ? t('Normal') : `${rate}×`, onClick: () => setSheet('speed') },
          ...(chapters.length > 1 ? [{ key: 'scenes', icon: <IconChapters />, label: t('Scenes'), value: chapName(Math.max(0, curChapter - 1)), onClick: then(() => setPop('chapters')) }] : []),
          ...sheetItems.map((x) => ({ ...x, onClick: then(x.onClick) })),
          ...(onSettings ? [{ key: 'view', icon: <IconSettings />, label: t('Viewing settings'), onClick: then(onSettings) }] : []),
          ...(onQuality && onQualityMenu ? [{ key: 'info', icon: <IconInfo />, label: t('How this is playing'), onClick: then(onQuality) }] : []),
          ...(onEditInfo ? [{ key: 'wittyinfo', icon: <IconChapters />, label: t('Edit scenes and skips'), onClick: then(onEditInfo) }] : []),
        ]
        return (
          <OptionsSheet page={sheet} onPage={setSheet} onClose={close} rows={rows}
            speeds={speeds.map((x) => ({ id: x, label: x === 1 ? t('Normal') : `${x}×`, on: x === rate, note: x > 4 ? t('may be muted') : undefined }))} onSpeed={(x) => { changeSpeed(x); close() }}
            audio={audio?.map((a) => ({ id: a.id, label: a.label, on: a.active }))} onAudio={(id) => { onAudio?.(id); close() }}
            subtitles={subtitles && [{ id: -1, label: t('Off'), on: !activeSub }, ...subtitles.map((x) => ({ id: x.id, label: x.label, on: x.active }))]} onSubtitle={(id) => { onSubtitle?.(id); close() }} />
        )
      })()}
      {pop === 'more' && (
        <Menu label={t('More')} className="more-pop" onClose={closePop}>
          {onSettings && <button role="menuitem" onClick={() => { setPop(null); onSettings() }}><IconSettings /> {t('Viewing settings')}</button>}
          {onQuality && <button role="menuitem" onClick={() => { setPop(null); onQuality() }}><IconInfo /> {t('How this is playing')}</button>}
          {onEditInfo && <button role="menuitem" onClick={() => { setPop(null); onEditInfo() }}><IconChapters /> {t('Edit scenes and skips')}</button>}
          {/* A TV shows the remote's keys (the app's Keys dialog), never a keyboard's shortcuts. */}
          {getTvMode()
            ? <button role="menuitem" onClick={() => { setPop(null); window.dispatchEvent(new Event(HELP_EVENT)) }}><IconInfo /> {t('Keys')}</button>
            : <button role="menuitem" onClick={() => { setPop(null); setHelp(true) }}><span className="kbd-help" aria-hidden="true">?</span> {t('Keyboard shortcuts')}</button>}
        </Menu>
      )}
      {pop === 'audio' && audio && (
        <Menu label={t('Audio')} onClose={closePop}>
          {audio.map((a) => <MenuChoice key={a.id} on={a.active} onPick={() => { onAudio?.(a.id); setPop(null) }}>{a.label}</MenuChoice>)}
        </Menu>
      )}
      {pop === 'subs' && subtitles && (
        <Menu label={t('Subtitles')} onClose={closePop}>
          <MenuChoice on={!subtitles.some((s) => s.active)} onPick={() => { onSubtitle?.(-1); setPop(null) }}>{t('Off')}</MenuChoice>
          {subtitles.map((s) => <MenuChoice key={s.id} on={s.active} onPick={() => { onSubtitle?.(s.id); setPop(null) }}>{s.label}</MenuChoice>)}
        </Menu>
      )}
      {pop === 'speed' && (
        <Menu label={t('Playback speed')} className="speed-pop" onClose={closePop}>
          {speeds.map((s) => <MenuChoice key={s} on={s === rate} onPick={() => { changeSpeed(s); setPop(null) }} note={s > 4 ? t('may be muted') : undefined}>{s === 1 ? t('Normal') : `${s}×`}</MenuChoice>)}
        </Menu>
      )}
      {help && <ShortcutHelp disc={disc} copy={copy} onClose={() => setHelp(false)} />}
      {pop === 'chapters' && (
        <ScenePanel chapters={chapters} name={chapName} thumb={chapterThumb} current={curChapter - 1}
          onPick={(c) => { onSeek(c); setPop(null) }} onClose={() => setPop(null)} />
      )}
    </div>
  )
}

// Menu: one of the bar's small menus (speed, audio, subtitles, More). It opens with focus on the choice in use, so a
// keyboard or a remote can confirm it or move on with the arrows; Esc or Back closes it, and so does a press anywhere
// outside it (that press does nothing else: it doesn't pause the film). While it's open the bar doesn't hide (useIdle).
function Menu({ label, className = '', onClose, children }: { label: string; className?: string; onClose: (refocus: boolean) => void; children: React.ReactNode }) {
  const box = useRef<HTMLDivElement | null>(null)
  const items = () => Array.from(box.current?.querySelectorAll<HTMLElement>('[role^="menuitem"]') || [])
  useEffect(() => {
    const all = items()
    ;(all.find((b) => b.getAttribute('aria-checked') === 'true') || all[0])?.focus({ preventScroll: true })
    // Esc and Back reach the menu before the player, which would otherwise shrink to the corner.
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || e.key === 'Backspace') { e.preventDefault(); e.stopImmediatePropagation(); onClose(true) }
    }
    // A press outside the menu and the bar closes the menu and is swallowed, so it doesn't also pause or play.
    const onDown = (e: PointerEvent) => {
      const t = e.target as Element | null
      if (!t || box.current?.contains(t) || t.closest('.ctl')) return
      const swallow = (c: Event) => { c.stopPropagation(); c.preventDefault() }
      window.addEventListener('click', swallow, { capture: true, once: true })
      setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 600)
      onClose(false)
    }
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('pointerdown', onDown, true)
    return () => { window.removeEventListener('keydown', onKey, true); window.removeEventListener('pointerdown', onDown, true) }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const onKeyDown = (e: React.KeyboardEvent) => {
    const all = items()
    const i = all.indexOf(document.activeElement as HTMLElement)
    const to = e.key === 'ArrowDown' ? (i + 1) % all.length : e.key === 'ArrowUp' ? (i - 1 + all.length) % all.length : e.key === 'Home' ? 0 : e.key === 'End' ? all.length - 1 : -1
    if (to >= 0) { e.preventDefault(); e.stopPropagation(); all[to]?.focus() }
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') { e.preventDefault(); e.stopPropagation() } // not a seek while choosing
    else if (e.key === 'Tab') onClose(false)
  }
  return (
    <div className={'popover ' + className} ref={box} role="menu" aria-label={label} onKeyDown={onKeyDown}>
      {className !== 'more-pop' && <h4 aria-hidden="true">{label}</h4>}
      {children}
    </div>
  )
}

// MenuChoice: one choice in a Menu. The one in use is ticked and bold, not marked by colour or a bar alone.
function MenuChoice({ on, onPick, note, children }: { on: boolean; onPick: () => void; note?: string; children: React.ReactNode }) {
  return (
    <button role="menuitemradio" aria-checked={on} className={on ? 'active' : ''} onClick={onPick}>
      <span className="pm-label">{children}</span>
      {note && <span className="pm-note">{note}</span>}
      <span className="pm-tick" aria-hidden="true">{on ? <IconCheck /> : null}</span>
    </button>
  )
}

// OptionsSheet: everything the phone's bar leaves out, as a sheet from the bottom edge. Each row says what it's set to
// now; speed (and audio and subtitles where there's no big picker) choose on a page of the sheet itself. Esc, Back, a
// press above the sheet or its close button put it away.
type SheetPage = 'main' | 'speed' | 'audio' | 'subs'
type Choice = { id: number; label: string; on: boolean; note?: string }
function OptionsSheet({ page, onPage, onClose, rows, speeds, onSpeed, audio, onAudio, subtitles, onSubtitle }: {
  page: SheetPage; onPage: (p: SheetPage) => void; onClose: () => void; rows: SheetItem[]
  speeds: Choice[]; onSpeed: (x: number) => void; audio?: Choice[]; onAudio: (id: number) => void; subtitles?: Choice[]; onSubtitle: (id: number) => void
}) {
  const box = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const el = box.current?.querySelector<HTMLElement>('[aria-checked="true"]') || box.current?.querySelector<HTMLElement>('.psheet-row, [role="menuitemradio"]')
    el?.focus({ preventScroll: true })
  }, [page])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' && e.key !== 'GoBack' && e.key !== 'BrowserBack') return
      e.preventDefault(); e.stopImmediatePropagation()
      if (page === 'main') onClose(); else onPage('main')
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [page, onClose, onPage])
  const list = page === 'speed' ? speeds : page === 'audio' ? audio : page === 'subs' ? subtitles : null
  const pick = page === 'speed' ? onSpeed : page === 'audio' ? onAudio : onSubtitle
  const title = page === 'speed' ? t('Playback speed') : page === 'audio' ? t('Audio') : page === 'subs' ? t('Subtitles') : t('Options')
  return (
    <div className="psheet-backdrop" onClick={onClose}>
      <div className="psheet" ref={box} role="dialog" aria-modal="true" aria-label={title} onClick={(e) => e.stopPropagation()}>
        <div className="psheet-head">
          {page !== 'main'
            ? <button className="ibtn" onClick={() => onPage('main')} aria-label={t('Back to options')}><IconBack /></button>
            : <span className="psheet-grip" aria-hidden="true" />}
          <h3>{title}</h3>
          <button className="ibtn" onClick={onClose} aria-label={t('Close')}><IconClose /></button>
        </div>
        {list ? (
          <div role="menu" aria-label={title}>
            {list.map((c) => <MenuChoice key={c.id} on={c.on} note={c.note} onPick={() => pick(c.id)}>{c.label}</MenuChoice>)}
          </div>
        ) : rows.map((r) => (
          <button key={r.key} className={'psheet-row' + (r.on ? ' on' : '')} onClick={r.onClick}>
            <span className="psheet-icon" aria-hidden="true">{r.icon}</span>
            <span className="psheet-label">{r.label}</span>
            {r.value && <span className="psheet-value">{r.value}</span>}
          </button>
        ))}
      </div>
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
  ['Q', tk('Quality: picture size and data'), 'file'],
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
