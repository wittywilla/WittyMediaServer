// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Segmented } from '../ui/Choice'
import { HelpLink } from '../components/HelpLink'
import { IconClose, IconFull, IconNext, IconPause, IconPlay, IconPrev } from '../icons'
import { IconSliders } from './icons'
import { useMedia } from '../shell/useNav'
import { UseAComputer, advancedHere } from '../ui/advanced'
import { lineAt, ownTiming, useLineIndex, wordTimes, type LyricLine, type useLyrics } from './lyrics'
import { KaraokeStemsControl, useKaraokeStems } from './KaraokeStems'
import { Pop } from './controls'
import { VoiceFxPanel } from './VoiceFxPanel'
import { isOff } from './voiceFx'
import { KAR_PARTS } from './stemMix'
import type { PlayerView } from './types'

// Karaoke. Big words that fill in as they're sung, a count-in before each line after a
// break, the next line waiting below, the server's karaoke version with the voice turned
// down, and the key moved to suit the singer. Made to read from across a room, and to work from a TV remote: arrows change line
// on the screen itself, and move between the controls once you're on them.

const COUNT_FROM_MS = 3000 // the count-in before a line
const LONG_GAP_MS = 5000 // a break long enough to count back in from

const NO_LINES: LyricLine[] = []

const NO_HIDE: string[] = []

// cover: the album's cover, '' for none. coverBeside: the setting that shows it at the side of the words (beside them on
// a wide screen and a TV, above them on a phone held upright), kept with the music player's other settings. hide: the parts of the bar (KAR_PARTS) the
// person has hidden, kept there too.
export default function Karaoke({ p, ly, onClose, behind, choice, cover = '', coverArt, coverBeside = false, onCoverBeside, hide = NO_HIDE, onHide }: { p: PlayerView; ly: ReturnType<typeof useLyrics>; onClose: () => void; behind?: ReactNode; choice?: ReactNode; cover?: string; coverArt?: ReactNode; coverBeside?: boolean; onCoverBeside?: (on: boolean) => void; hide?: string[]; onHide?: (hide: string[]) => void }) {
  const { lyrics, loading, setOffset } = ly
  const lines = lyrics?.synced ? lyrics.lines : NO_LINES
  const offset = lyrics?.offsetMs || 0
  const index = useLineIndex(lines, p.position, offset, p.playing)
  const root = useRef<HTMLDivElement>(null)
  const nowLine = lines[index]
  const nextLine = lines[index + 1]
  const words = useMemo(() => (nowLine ? wordTimes(nowLine, nextLine?.t) : []), [nowLine, nextLine])
  const timedWords = !!nowLine?.words?.length
  const wordEls = useRef<(HTMLSpanElement | null)[]>([])
  const [count, setCount] = useState(0)
  // Words fill in as they are sung (Follow along), or the whole text to read at your own pace (Read).
  const [mode, setMode] = useState<'follow' | 'read'>('follow')
  const [fxOpen, setFxOpen] = useState(false)
  const readWords = lyrics?.plain || lines.map((l) => l.text).join('\n')
  const stems = useKaraokeStems(p)
  const sideCover = coverBeside && cover ? cover : ''

  useEffect(() => { root.current?.focus({ preventScroll: true }) }, [])
  // Coming back, the key is as it was: this was for the song, not for good.
  useEffect(() => () => { p.setSound({ keyShift: 0 }) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  // The fill and the count-in, a frame at a time: written straight to the page, as React
  // has no need to know about either, and a cheap TV has better things to do.
  useEffect(() => {
    let raf = 0
    const step = () => {
      raf = requestAnimationFrame(step)
      const ms = p.position() * 1000 - offset
      for (let i = 0; i < words.length; i++) {
        const el = wordEls.current[i]
        if (!el) continue
        const w = words[i]
        const f = ms <= w.from ? 0 : ms >= w.to ? 1 : (ms - w.from) / Math.max(1, w.to - w.from)
        el.style.setProperty('--fill', (f * 100).toFixed(1) + '%')
      }
      const up = lines[lineAt(lines, ms) + 1]
      const here = lines[lineAt(lines, ms)]
      let c = 0
      if (up) {
        const left = up.t - ms
        const quiet = !here || up.t - (here.end ?? here.t) > 0 || !here.text
        const longGap = !here || up.t - here.t >= LONG_GAP_MS
        if (left > 0 && left <= COUNT_FROM_MS && quiet && longGap) c = Math.ceil(left / 1000)
      }
      setCount((old) => (old === c ? old : c))
    }
    raf = requestAnimationFrame(step)
    return () => cancelAnimationFrame(raf)
  }, [words, lines, offset, p.position])

  const goLine = (d: number) => {
    const to = lines[Math.max(0, index + d)]
    if (to) p.seek(Math.max(0, (to.t + offset) / 1000))
  }
  const setKey = (k: number) => p.setSound({ keyShift: Math.max(-6, Math.min(6, k)) })
  const fullscreen = () => {
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
    else void root.current?.requestFullscreen?.({ navigationUI: 'hide' }).catch(() => {})
  }

  // A phone: the words have the screen. The mix, key, timing and what's behind the words are in a sheet ("Mix and
  // more"), opened at the thumb beside play, instead of a page that scrolls the words away to reach them.
  const phone = useMedia('(max-width: 700px)')
  const [sheet, setSheet] = useState(false)
  const [walled, setWalled] = useState(false) // Voice settings asked for on a phone or TV (ui/advanced.tsx)
  useEffect(() => { if (!phone) setSheet(false) }, [phone])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return
      const t = e.target as HTMLElement
      const onScreen = t === root.current || t === document.body
      const consume = () => { e.preventDefault(); e.stopImmediatePropagation() }
      switch (e.key) {
        case 'Escape': case 'GoBack': case 'BrowserBack':
          consume()
          if (sheet) { setSheet(false); root.current?.querySelector<HTMLElement>('.kar-sheet-btn')?.focus({ preventScroll: true }); return }
          if (fxOpen) { setFxOpen(false); root.current?.querySelector<HTMLElement>('.kar-fx-btn')?.focus({ preventScroll: true }); return }
          if (document.fullscreenElement === root.current) void document.exitFullscreen().catch(() => {})
          onClose()
          return
        case ' ': if (onScreen) { consume(); p.toggle() } return
        case 'ArrowLeft': if (onScreen) { consume(); goLine(-1) } return
        case 'ArrowRight': if (onScreen) { consume(); goLine(1) } return
        case 'ArrowUp': case 'ArrowDown': case 'Enter':
          if (onScreen) { consume(); root.current?.querySelector<HTMLElement>('.kar-play')?.focus() }
          return
      }
      if (t.tagName === 'INPUT' || t.tagName === 'SELECT') return
      switch (e.key.toLowerCase()) {
        case 'v': consume(); if (stems.active) stems.setSing(!stems.sing); return
        case '-': consume(); setKey(p.sound.keyShift - 1); return
        case '+': case '=': consume(); setKey(p.sound.keyShift + 1); return
        case '[': consume(); setOffset(offset - 100); return
        case ']': consume(); setOffset(offset + 100); return
        case 'f': consume(); fullscreen(); return
        default:
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  })

  const it = p.current!
  const firstAt = lines[0]?.t ?? 0
  const wordsChoice = <Segmented label="Words" value={lines.length ? mode : 'read'} onChange={setMode} options={[{ value: 'follow', label: 'Follow along', disabled: !lines.length }, { value: 'read', label: 'Read' }]} />
  const behindChoice = (choice || (cover && onCoverBeside)) ? (
    <div className="kar-choice">
      {choice && <><span className="kar-choice-l">Behind the words</span>{choice}</>}
      {cover && onCoverBeside && <button type="button" className={'kar-chip kar-cover-chip' + (coverBeside ? ' on' : '')} aria-pressed={coverBeside} onClick={() => onCoverBeside(!coverBeside)}>Album art beside the words</button>}
    </div>
  ) : null
  const tools = (
    <>
      <KaraokeStemsControl k={stems} hide={hide} />
      {!hide.includes('key') && <div className="kar-stepper" role="group" aria-label="Key">
        <button type="button" onClick={() => setKey(p.sound.keyShift - 1)} disabled={p.sound.keyShift <= -6} aria-label="Lower key">−</button>
        <span aria-live="polite">Key {p.sound.keyShift > 0 ? '+' : ''}{p.sound.keyShift}</span>
        <button type="button" onClick={() => setKey(p.sound.keyShift + 1)} disabled={p.sound.keyShift >= 6} aria-label="Raise key">+</button>
      </div>}
      {!hide.includes('timing') && <div className="kar-stepper" role="group" aria-label="Timing of the words">
        <button type="button" onClick={() => setOffset(offset - 100)} aria-label="Words sooner">−</button>
        <span aria-live="polite">Words {offset === 0 ? 'on time' : `${offset > 0 ? '+' : ''}${(offset / 1000).toFixed(1)} s`}{lyrics?.autoSync === 'set' && offset === ownTiming(lyrics) ? ' (by listening)' : ''}</span>
        <button type="button" onClick={() => setOffset(offset + 100)} aria-label="Words later">+</button>
      </div>}
      {!hide.includes('fx') && <button type="button" className={'kar-chip kar-fx-btn' + (fxOpen || !isOff(stems.fx) ? ' on' : '')} aria-expanded={fxOpen} onClick={() => { setSheet(false); if (advancedHere()) setFxOpen(!fxOpen); else setWalled(true) }} title="Echo, reverb, tone and more, for the voice and the music">Voice settings</button>}
      {onHide && (
        <Pop up label={hide.length ? `Controls shown: ${KAR_PARTS.length - hide.length} of ${KAR_PARTS.length}` : 'Choose the controls shown'} className={'np-chip-pop kar-show' + (hide.length ? ' on' : '')} icon={<span>Controls</span>}>
          {() => (
            <div className="mp-menu" role="menu" aria-label="Controls shown">
              <span className="mp-menu-note">Show these controls</span>
              {KAR_PARTS.map((c) => {
                const shown = !hide.includes(c.id)
                return <button type="button" key={c.id} role="menuitemcheckbox" aria-checked={shown} className={shown ? 'on' : ''} onClick={() => onHide(shown ? [...hide, c.id] : hide.filter((h) => h !== c.id))}>{c.label}</button>
              })}
            </div>
          )}
        </Pop>
      )}
    </>
  )
  return (
    <div className="kar" ref={root} tabIndex={-1} role="dialog" aria-modal="true" aria-label={`Karaoke: ${it.title}`} onClickCapture={(e) => { if (e.detail > 0) requestAnimationFrame(() => root.current?.focus({ preventScroll: true })) }}>
      <div className="kar-bg" aria-hidden="true" />
      {behind}
      <header className="kar-top">
        <div className="kar-song"><b>{it.title}</b><span>{it.artist || it.albumArtist}</span></div>
        {!phone && wordsChoice}
        {!phone && <button type="button" className="mp-ibtn" onClick={fullscreen} aria-label="Fill the screen (F)" title="Fill the screen (F)"><IconFull /></button>}
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label="Leave karaoke (Esc)" title="Leave karaoke (Esc)"><IconClose /></button>
      </header>
      {!phone && behindChoice}

      <main className={'kar-stage' + (sideCover ? ' with-cover' : '')}>
        {sideCover && (coverArt ? <div className="kar-cover kar-cover-art" aria-hidden="true">{coverArt}</div> : <img className="kar-cover" src={sideCover} alt="" draggable={false} decoding="async" />)}
        <div className="kar-lyrics">
        {mode === 'read' && readWords ? <pre className="kar-read" tabIndex={0}>{readWords}</pre>
          : !lyrics || loading ? <p className="kar-msg">Looking for the words…</p>
          : lines.length === 0 ? (
            <div className="kar-msg">
              <p>{lyrics.plain ? 'These words have no times, so they can\'t follow the music yet.' : 'No words for this song.'}</p>
              {lyrics.plain && <pre className="kar-plain" tabIndex={0}>{lyrics.plain}</pre>}
              <p className="kar-sub">A timed <code>.lrc</code> file beside the song gives karaoke what it needs, or use Lyrics, then Fix, then Tap along. <HelpLink to="lyrics-karaoke#adding-your-own-lyrics" newTab>How to add your own</HelpLink></p>
            </div>
          ) : (
            <>
              <div className={'kar-count' + (count ? ' on' : '')} aria-hidden="true">{count > 0 && <span key={count}>{count}</span>}</div>
              {index < 0 && <p className="kar-wait">{firstAt > 5000 ? 'Get ready…' : ''}</p>}
              <p className="kar-now" aria-live="off">
                {nowLine && nowLine.text
                  ? words.map((w, i) => <span key={index + '-' + i} className="kar-word" ref={(el) => { wordEls.current[i] = el }} data-word={w.text}>{w.text}</span>)
                  : <span className="kar-rest" aria-hidden="true">♪</span>}
              </p>
              <p className="kar-next">{nextLine?.text || ' '}</p>
              <span className="mp-sr" aria-live="polite">{nowLine?.text || ''}</span>
            </>
          )}
        </div>
      </main>

      <footer className="kar-bar" role="toolbar" aria-label="Karaoke controls">
        <button type="button" className="mp-ibtn" onClick={() => goLine(-1)} aria-label="Previous line"><IconPrev /></button>
        <button type="button" className="kar-play" onClick={p.toggle} aria-label={p.playing ? 'Pause' : 'Play'}>{p.playing ? <IconPause size={30} /> : <IconPlay size={30} />}</button>
        <button type="button" className="mp-ibtn" onClick={() => goLine(1)} aria-label="Next line"><IconNext /></button>
        {phone
          ? <button type="button" className="kar-chip kar-sheet-btn" aria-haspopup="dialog" aria-expanded={sheet} onClick={() => setSheet(true)}><IconSliders size={20} /> Mix and more</button>
          : <><span className="kar-rule" aria-hidden="true" />{tools}</>}
      </footer>
      {walled && <UseAComputer what="Voice settings" onClose={() => setWalled(false)} />}
      {phone && sheet && (
        <div className="kar-sheet-backdrop" onClick={() => setSheet(false)}>
          <div className="kar-sheet" role="dialog" aria-modal="true" aria-label="Mix and more" onClick={(e) => e.stopPropagation()}>
            <div className="kar-sheet-head">
              <h3>Mix and more</h3>
              <button type="button" className="mp-ibtn" onClick={() => setSheet(false)} aria-label="Close"><IconClose /></button>
            </div>
            <div className="kar-sheet-tools">{tools}</div>
            <div className="kar-sheet-row"><span className="kar-choice-l">Words</span>{wordsChoice}</div>
            {behindChoice}
            <button type="button" className="kar-chip" onClick={() => { setSheet(false); fullscreen() }}><IconFull size={18} /> Fill the screen</button>
          </div>
        </div>
      )}
      {fxOpen && <VoiceFxPanel fx={stems.fx} onFx={stems.setFx} ready={stems.active} quick={stems.answer?.mix === 'blend'} onClose={() => { setFxOpen(false); root.current?.querySelector<HTMLElement>('.kar-fx-btn')?.focus({ preventScroll: true }) }} />}
      <p className="kar-note">
        {p.soundNote || (stems.waiting ? '' : stems.note) || (p.sound.keyShift !== 0 ? 'A changed key can warble a little on long notes.'
          : lines.length > 0 && !timedWords ? 'Lines are timed; the fill within a line is an estimate.' : ' ')}
      </p>
      {/* The keys karaoke answers, said as the player says its own (they were there, but nowhere to be seen). */}
      <p className="np-hints kar-hints" aria-hidden="true">
        <kbd>Esc</kbd> back · <kbd>Space</kbd> play or pause · <kbd>←</kbd> <kbd>→</kbd> previous or next line · <kbd>V</kbd> voice on or off · <kbd>−</kbd> <kbd>+</kbd> key · <kbd>[</kbd> <kbd>]</kbd> words sooner or later · <kbd>F</kbd> full screen
      </p>
    </div>
  )
}
