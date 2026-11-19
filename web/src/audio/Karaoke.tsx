// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Segmented } from '../ui/Choice'
import { HelpLink } from '../components/HelpLink'
import { IconClose, IconFull, IconMic, IconNext, IconPause, IconPlay, IconPrev } from '../icons'
import { IconSliders } from './icons'
import { useMedia } from '../shell/useNav'
import { useTvMode } from '../tvmode/mode'
import { useIdleHide } from './idle'
import { lineAt, ownTiming, useLineIndex, wordTimes, type LyricLine, type useLyrics } from './lyrics'
import { KaraokeStemsControl, useKaraokeStems } from './KaraokeStems'
import { Pop } from './controls'
import { VoiceFxPanel } from './VoiceFxPanel'
import { isOff } from './voiceFx'
import { KAR_PARTS } from './stemMix'
import type { PlayerView } from './types'
import { num, t, tx } from '../i18n'

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
// person has hidden, kept there too. choice: what's behind the words (album art, Canvas video, visualizer), and styles: the
// visualizer's styles while it's the one behind, both Now playing's own, so they're one setting for the two.
//
// The first screen is the words, play and the lines either side, the guide voice, and a few buttons: Mix (the levels,
// the key, the words' timing and the notes), Voice settings and Controls. What's behind the words and the top row step
// aside when nothing is touched (idle.ts): a mouse brings them back on a computer, a tap on a phone, any press of the
// remote on a TV.
type KarPanel = '' | 'mix' | 'fx'
export default function Karaoke({ p, ly, onClose, behind, choice, styles, cover = '', coverArt, coverBeside = false, onCoverBeside, hide = NO_HIDE, onHide }: { p: PlayerView; ly: ReturnType<typeof useLyrics>; onClose: () => void; behind?: ReactNode; choice?: ReactNode; styles?: ReactNode; cover?: string; coverArt?: ReactNode; coverBeside?: boolean; onCoverBeside?: (on: boolean) => void; hide?: string[]; onHide?: (hide: string[]) => void }) {
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
  // Mix or Voice settings, at the side (a sheet on a phone); one at a time.
  const [panel, setPanel] = useState<KarPanel>('')
  const readWords = lyrics?.plain || lines.map((l) => l.text).join('\n')
  const stems = useKaraokeStems(p)
  const sideCover = coverBeside && cover ? cover : ''
  const phone = useMedia('(max-width: 700px)')
  const coarse = useMedia('(pointer: coarse)')
  const tv = useTvMode()
  // Before karaoke's own keys, so a TV's first press can be only "show the controls".
  const idle = useIdleHide(root, true, {
    startHidden: phone || tv, tap: phone || coarse, firstKey: tv, controls: '.kar-top, .kar-choice',
    busy: () => !!document.querySelector('.mp-pop-body') || !!root.current?.querySelector(':is(.kar-top, .kar-choice):hover'),
  })

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
  // A panel closed hands the focus back to the button that opened it, so a remote is never left nowhere.
  const panelNow = useRef<KarPanel>('')
  panelNow.current = panel
  const closePanel = useCallback(() => {
    const was = panelNow.current
    setPanel('')
    if (was) requestAnimationFrame(() => root.current?.querySelector<HTMLElement>(was === 'mix' ? '.kar-mix-btn' : '.kar-fx-btn')?.focus({ preventScroll: true }))
  }, [])
  const togglePanel = (which: KarPanel) => { if (panel === which) closePanel(); else setPanel(which) }
  // The Mix opens on its first control.
  useEffect(() => {
    if (panel === 'mix') requestAnimationFrame(() => root.current?.querySelector<HTMLElement>('.kar-mixp :is(input, button):not([disabled]):not(.kar-mixp-close)')?.focus({ preventScroll: true }))
  }, [panel])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return
      const t = e.target as HTMLElement
      const onScreen = t === root.current || t === document.body
      const consume = () => { e.preventDefault(); e.stopImmediatePropagation() }
      switch (e.key) {
        case 'Escape': case 'GoBack': case 'BrowserBack':
          if (document.querySelector('.kar .mp-pop-body')) return // an open menu closes first, by its own key
          consume()
          if (panel) { closePanel(); return }
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
        case 'v': consume(); if (stems.active) stems.toggleGuide(); return
        case 'm': consume(); togglePanel('mix'); return
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
  const wordsChoice = <Segmented label={t('Words')} value={lines.length ? mode : 'read'} onChange={setMode} options={[{ value: 'follow', label: t('Follow along'), disabled: !lines.length }, { value: 'read', label: t('Read') }]} />
  // What's behind the words, and the visualizer's style while it's the one: near the top, easy to find, stepping aside when idle.
  const behindChoice = choice ? (
    <div className="kar-choice">
      <span className="kar-choice-l">{t('Behind the words')}</span>{choice}
      {styles}
    </div>
  ) : null
  const voiceOn = stems.active || stems.waiting
  const note = p.soundNote || (stems.waiting ? '' : stems.note) || (p.sound.keyShift !== 0 ? t('A changed key can warble a little on long notes.')
    : lines.length > 0 && !timedWords ? t('Lines are timed; the fill within a line is an estimate.') : '')
  const controlsPop = onHide && (
    <Pop up label={hide.length ? t('Controls shown: {shown} of {total}', { shown: KAR_PARTS.length - hide.length, total: KAR_PARTS.length }) : t('Choose the controls shown')} className={'np-chip-pop kar-show' + (hide.length ? ' on' : '')} icon={<span>{t('Controls')}</span>}>
      {() => (
        <div className="mp-menu" role="menu" aria-label={t('Controls shown')}>
          <span className="mp-menu-note">{t('Show these controls')}</span>
          {KAR_PARTS.map((c) => {
            const shown = !hide.includes(c.id)
            return <button type="button" key={c.id} role="menuitemcheckbox" aria-checked={shown} className={shown ? 'on' : ''} onClick={() => onHide(shown ? [...hide, c.id] : hide.filter((h) => h !== c.id))}>{tx(c.label)}</button>
          })}
        </div>
      )}
    </Pop>
  )
  const fxButton = !hide.includes('fx') && <button type="button" className={'kar-chip kar-fx-btn' + (panel === 'fx' || !isOff(stems.fx) ? ' on' : '')} aria-expanded={panel === 'fx'} onClick={() => togglePanel('fx')} title={t('Echo, reverb, tone and more, for the voice and the music')}>{t('Voice settings')}</button>
  return (
    <div className={'kar' + (idle ? ' kar-idle' : '')} ref={root} tabIndex={-1} role="dialog" aria-modal="true" aria-label={t('Karaoke: {title}', { title: it.title })} onClickCapture={(e) => { if (e.detail > 0) requestAnimationFrame(() => { if (!document.activeElement?.closest('input, .kar-fx, .mp-pop-body')) root.current?.focus({ preventScroll: true }) }) }}>
      <div className="kar-bg" aria-hidden="true" />
      {behind}
      <header className="kar-top">
        <div className="kar-song"><b>{it.title}</b><span>{it.artist || it.albumArtist}</span></div>
        {!phone && wordsChoice}
        {!phone && <button type="button" className="mp-ibtn" onClick={fullscreen} aria-label={t('Fill the screen (F)')} title={t('Fill the screen (F)')}><IconFull /></button>}
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label={t('Leave karaoke (Esc)')} title={t('Leave karaoke (Esc)')}><IconClose /></button>
      </header>
      {behindChoice}

      <main className={'kar-stage' + (sideCover ? ' with-cover' : '')}>
        {sideCover && (coverArt ? <div className="kar-cover kar-cover-art" aria-hidden="true">{coverArt}</div> : <img className="kar-cover" src={sideCover} alt="" draggable={false} decoding="async" />)}
        <div className="kar-lyrics">
        {mode === 'read' && readWords ? <pre className="kar-read" tabIndex={0}>{readWords}</pre>
          : !lyrics || loading ? <p className="kar-msg">{t('Looking for the words…')}</p>
          : lines.length === 0 ? (
            <div className="kar-msg">
              <p>{lyrics.plain ? t('These words have no times, so they can’t follow the music yet.') : t('No words for this song.')}</p>
              {lyrics.plain && <pre className="kar-plain" tabIndex={0}>{lyrics.plain}</pre>}
              <p className="kar-sub">{t('A timed .lrc file beside the song gives karaoke what it needs, or use Lyrics, then Fix, then Tap along.')} <HelpLink to="lyrics-karaoke#adding-your-own-lyrics" newTab>{t('How to add your own')}</HelpLink></p>
            </div>
          ) : (
            <>
              <div className={'kar-count' + (count ? ' on' : '')} aria-hidden="true">{count > 0 && <span key={count}>{count}</span>}</div>
              {index < 0 && <p className="kar-wait">{firstAt > 5000 ? t('Get ready…') : ''}</p>}
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

      <footer className="kar-bar" role="toolbar" aria-label={t('Karaoke controls')}>
        <button type="button" className="mp-ibtn" onClick={() => goLine(-1)} aria-label={t('Previous line')}><IconPrev /></button>
        <button type="button" className="kar-play" onClick={p.toggle} aria-label={p.playing ? t('Pause') : t('Play')}>{p.playing ? <IconPause size={30} /> : <IconPlay size={30} />}</button>
        <button type="button" className="mp-ibtn" onClick={() => goLine(1)} aria-label={t('Next line')}><IconNext /></button>
        <span className="kar-rule" aria-hidden="true" />
        {/* The singer starts out (useKaraokeStems); the guide voice brings them back quietly, and takes them out again (V). */}
        {voiceOn && !hide.includes('vocals') && <button type="button" className={'kar-chip kar-guide' + (stems.guide ? ' on' : '')} aria-pressed={stems.guide} disabled={!stems.active} onClick={stems.toggleGuide} title={t('Keep the singer quietly, as a guide')}><IconMic size={20} /><span className="kar-bar-label">{t('Guide voice')}</span></button>}
        <button type="button" className={'kar-chip kar-mix-btn' + (panel === 'mix' ? ' on' : '')} aria-expanded={panel === 'mix'} aria-busy={stems.waiting || undefined} onClick={() => togglePanel('mix')} title={t('The voice, the music and the track, the key and the words’ timing')}><IconSliders size={20} /><span className="kar-bar-label">{t('Mix')}</span>{stems.waiting && <span className="kar-busy" aria-hidden="true" />}</button>
        {!phone && fxButton}
        {!phone && controlsPop}
      </footer>
      {panel === 'mix' && (
        <aside className="kar-fx kar-mixp" role="dialog" aria-label={t('Mix')}>
          <header className="kar-fx-head">
            <h2>{t('Mix')}</h2>
            <button type="button" className="mp-ibtn kar-mixp-close" onClick={closePanel} aria-label={t('Close')} title={t('Close')}><IconClose /></button>
          </header>
          <KaraokeStemsControl k={stems} hide={hide} />
          {(!hide.includes('key') || !hide.includes('timing')) && <div className="kar-mixp-row">
            {!hide.includes('key') && <div className="kar-stepper" role="group" aria-label={t('Musical key')}>
              <button type="button" onClick={() => setKey(p.sound.keyShift - 1)} disabled={p.sound.keyShift <= -6} aria-label={t('Lower key')}>−</button>
              <span aria-live="polite">{t('Key {n}', { n: (p.sound.keyShift > 0 ? '+' : '') + p.sound.keyShift })}</span>
              <button type="button" onClick={() => setKey(p.sound.keyShift + 1)} disabled={p.sound.keyShift >= 6} aria-label={t('Raise key')}>+</button>
            </div>}
            {!hide.includes('timing') && <div className="kar-stepper" role="group" aria-label={t('Timing of the words')}>
              <button type="button" onClick={() => setOffset(offset - 100)} aria-label={t('Words sooner')}>−</button>
              <span aria-live="polite">{offset === 0 ? t('Words on time') : t('Words {n} s', { n: (offset > 0 ? '+' : '') + num(offset / 1000, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) })}{lyrics?.autoSync === 'set' && offset === ownTiming(lyrics) ? ' ' + t('(by listening)') : ''}</span>
              <button type="button" onClick={() => setOffset(offset + 100)} aria-label={t('Words later')}>+</button>
            </div>}
          </div>}
          {cover && onCoverBeside && <div className="kar-mixp-row"><button type="button" className={'kar-chip kar-cover-chip' + (coverBeside ? ' on' : '')} aria-pressed={coverBeside} onClick={() => onCoverBeside(!coverBeside)}>{t('Album art beside the words')}</button></div>}
          {phone && <div className="kar-mixp-row">
            <span className="kar-choice-l">{t('Words')}</span>{wordsChoice}
          </div>}
          {phone && <div className="kar-mixp-row">
            {fxButton}
            {controlsPop}
            <button type="button" className="kar-chip" onClick={() => { closePanel(); fullscreen() }}><IconFull size={18} /> {t('Fill the screen')}</button>
          </div>}
          {note && <p className="kar-fx-note" role="status">{note}</p>}
        </aside>
      )}
      {panel === 'fx' && <VoiceFxPanel fx={stems.fx} onFx={stems.setFx} ready={stems.active} quick={stems.answer?.mix === 'blend'} onClose={closePanel} />}
      {/* The keys karaoke answers, said as the player says its own (they were there, but nowhere to be seen). */}
      <p className="np-hints kar-hints" aria-hidden="true">
        <kbd>Esc</kbd> {t('back')} · <kbd>{t('Space')}</kbd> {t('play or pause')} · <kbd>←</kbd> <kbd>→</kbd> {t('previous or next line')} · <kbd>V</kbd> {t('guide voice')} · <kbd>M</kbd> {t('mix')} · <kbd>−</kbd> <kbd>+</kbd> {t('key')} · <kbd>[</kbd> <kbd>]</kbd> {t('words sooner or later')} · <kbd>F</kbd> {t('full screen')}
      </p>
    </div>
  )
}
