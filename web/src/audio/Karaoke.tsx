import { useEffect, useMemo, useRef, useState } from 'react'
import { IconClose, IconFull, IconNext, IconPause, IconPlay, IconPrev } from '../icons'
import { lineAt, useLineIndex, wordTimes, type LyricLine, type useLyrics } from './lyrics'
import { soundGraph } from './sound'
import type { PlayerView } from './types'

// Karaoke. Big words that fill in as they're sung, a count-in before each line after a
// break, the next line waiting below, the voice turned down, and the key moved to suit the
// singer. Made to read from across a room, and to work from a TV remote: arrows change line
// on the screen itself, and move between the controls once you're on them.

const VOCALS = [{ v: 0, name: 'Voice on' }, { v: 0.6, name: 'Voice lower' }, { v: 1, name: 'Voice lowest' }]
const COUNT_FROM_MS = 3000 // the count-in before a line
const LONG_GAP_MS = 5000 // a break long enough to count back in from

const NO_LINES: LyricLine[] = []

export default function Karaoke({ p, ly, onClose }: { p: PlayerView; ly: ReturnType<typeof useLyrics>; onClose: () => void }) {
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
  const [mono, setMono] = useState(false)

  useEffect(() => { root.current?.focus({ preventScroll: true }) }, [])
  // Coming back, the voice and key are as they were: this was for the song, not for good.
  useEffect(() => () => { p.setSound({ vocalCut: 0, keyShift: 0 }) }, []) // eslint-disable-line react-hooks/exhaustive-deps
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

  // Whether there's a voice to take away at all, while it's trying to.
  useEffect(() => {
    if (!p.sound.vocalCut) { setMono(false); return }
    const t = setInterval(() => { const m = soundGraph()?.nearlyMono(); if (m !== null && m !== undefined) setMono(m) }, 1500)
    return () => clearInterval(t)
  }, [p.sound.vocalCut])

  const goLine = (d: number) => {
    const to = lines[Math.max(0, index + d)]
    if (to) p.seek(Math.max(0, (to.t + offset) / 1000))
  }
  const setKey = (k: number) => p.setSound({ keyShift: Math.max(-6, Math.min(6, k)) })
  const fullscreen = () => {
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
    else void root.current?.requestFullscreen?.({ navigationUI: 'hide' }).catch(() => {})
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return
      const t = e.target as HTMLElement
      const onScreen = t === root.current || t === document.body
      const consume = () => { e.preventDefault(); e.stopImmediatePropagation() }
      switch (e.key) {
        case 'Escape': case 'GoBack': case 'BrowserBack':
          consume()
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
        case 'v': consume(); p.setSound({ vocalCut: VOCALS[(VOCALS.findIndex((x) => x.v === p.sound.vocalCut) + 1) % VOCALS.length].v }); return
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
  return (
    <div className="kar" ref={root} tabIndex={-1} role="dialog" aria-modal="true" aria-label={`Karaoke: ${it.title}`} onClickCapture={(e) => { if (e.detail > 0) requestAnimationFrame(() => root.current?.focus({ preventScroll: true })) }}>
      <div className="kar-bg" aria-hidden="true" />
      <header className="kar-top">
        <div className="kar-song"><b>{it.title}</b><span>{it.artist || it.albumArtist}</span></div>
        <button type="button" className="mp-ibtn" onClick={fullscreen} aria-label="Fill the screen (F)" title="Fill the screen (F)"><IconFull /></button>
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label="Leave karaoke (Esc)" title="Leave karaoke (Esc)"><IconClose /></button>
      </header>

      <main className="kar-stage">
        {!lyrics || loading ? <p className="kar-msg">Looking for the words…</p>
          : lines.length === 0 ? (
            <div className="kar-msg">
              <p>{lyrics.plain ? 'These words have no times, so they can\'t follow the music yet.' : 'No words for this song.'}</p>
              {lyrics.plain && <pre className="kar-plain" tabIndex={0}>{lyrics.plain}</pre>}
              <p className="kar-sub">A timed <code>.lrc</code> file beside the song gives karaoke what it needs, or use Lyrics, then Fix, then Tap along.</p>
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
      </main>

      <footer className="kar-bar" role="toolbar" aria-label="Karaoke controls">
        <button type="button" className="mp-ibtn" onClick={() => goLine(-1)} aria-label="Previous line"><IconPrev /></button>
        <button type="button" className="kar-play" onClick={p.toggle} aria-label={p.playing ? 'Pause' : 'Play'}>{p.playing ? <IconPause size={30} /> : <IconPlay size={30} />}</button>
        <button type="button" className="mp-ibtn" onClick={() => goLine(1)} aria-label="Next line"><IconNext /></button>
        <span className="kar-rule" aria-hidden="true" />
        <div className="kar-seg" role="radiogroup" aria-label="Singer's voice">
          {VOCALS.map((x) => <button type="button" role="radio" key={x.v} aria-checked={p.sound.vocalCut === x.v} className={p.sound.vocalCut === x.v ? 'on' : ''} onClick={() => p.setSound({ vocalCut: x.v })}>{x.name}</button>)}
        </div>
        <div className="kar-stepper" role="group" aria-label="Key">
          <button type="button" onClick={() => setKey(p.sound.keyShift - 1)} disabled={p.sound.keyShift <= -6} aria-label="Lower key">−</button>
          <span aria-live="polite">Key {p.sound.keyShift > 0 ? '+' : ''}{p.sound.keyShift}</span>
          <button type="button" onClick={() => setKey(p.sound.keyShift + 1)} disabled={p.sound.keyShift >= 6} aria-label="Raise key">+</button>
        </div>
        <div className="kar-stepper" role="group" aria-label="Timing of the words">
          <button type="button" onClick={() => setOffset(offset - 100)} aria-label="Words sooner">−</button>
          <span aria-live="polite">Words {offset === 0 ? 'on time' : `${offset > 0 ? '+' : ''}${(offset / 1000).toFixed(1)} s`}</span>
          <button type="button" onClick={() => setOffset(offset + 100)} aria-label="Words later">+</button>
        </div>
      </footer>
      <p className="kar-note">
        {p.soundNote || (mono
          ? 'This recording is close to mono, so there\'s no way to separate the voice from it.'
          : p.sound.vocalCut > 0
            ? 'The voice is turned down by removing what\'s in the middle of the mix. Echoes of it, and songs that spread it wide, will still come through.'
            : p.sound.keyShift !== 0 ? 'A changed key can warble a little on long notes.'
              : lines.length > 0 && !timedWords ? 'Lines are timed; the fill within a line is an estimate.' : ' ')}
      </p>
    </div>
  )
}
