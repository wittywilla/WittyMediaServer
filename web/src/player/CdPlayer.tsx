import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, fmtRuntime, mediaUrl } from '../api'
import { useCd } from '../audio/player'
import { type CdDisc, type CdNow, type CdRepeat } from '../audio/cdEngine'
import { BackLink, EmptyState, Spinner } from '../components/common'
import { IconClose, IconDisc, IconEject, IconFf, IconInfo, IconNext, IconPause, IconPlay, IconPlus, IconPrev, IconRew, IconSearch, IconStop, IconTrash } from '../icons'
import { toast, useApp } from '../state'
import { CdGraphics } from './CdGraphics'
import InsertDisc from './InsertDisc'
import './cd.css'

// The CD player: an original deck in the same family as the disc loading screen.
// The disc goes in, spins behind a smoked window while it plays, and a lit display
// shows the track, the index and the time. The music itself lives in the audio
// player, so it carries on, with the bar along the bottom, when you leave this page.

const SECTORS_PER_SEC = 75

const clock = (sec: number) => {
  const s = Math.floor(Math.abs(sec))
  return `${sec < 0 ? '-' : ''}${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}
const spoken = (sec: number) => {
  const s = Math.floor(Math.abs(sec))
  return `${Math.floor(s / 60)} minutes ${s % 60} seconds`
}
const two = (n: number) => String(n).padStart(2, '0')

// A label for a disc with no picture: colours worked out from its ID, so one disc
// always looks the same.
function generatedArt(seed: string) {
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0
  const a = h % 360, b = (a + 40 + (h >> 9) % 80) % 360, c = (a + 180 + (h >> 17) % 60) % 360
  return `conic-gradient(from ${(h >> 3) % 360}deg, hsl(${a} 62% 42%), hsl(${b} 58% 30%), hsl(${c} 54% 38%), hsl(${a} 62% 42%))`
}

const prefersReduced = () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

// Two level bars, taken from what the audio element is playing. Nothing is wired
// through it (music must carry on if this stops), and it runs only while on screen.
function useLevels(el: HTMLAudioElement | null, on: boolean, left: React.RefObject<HTMLElement | null>, right: React.RefObject<HTMLElement | null>) {
  useEffect(() => {
    const capture = (el as (HTMLAudioElement & { captureStream?: () => MediaStream }) | null)?.captureStream
    const AC = window.AudioContext
    if (!el || !on || !capture || !AC || prefersReduced() || (navigator.hardwareConcurrency || 4) <= 2) return
    let ctx: AudioContext | null = null
    let raf = 0, last = 0, source: MediaStreamAudioSourceNode | null = null
    let la: AnalyserNode, ra: AnalyserNode, split: ChannelSplitterNode
    const buf = new Float32Array(256)
    const connect = () => {
      try {
        if (!ctx) {
          ctx = new AC()
          split = ctx.createChannelSplitter(2)
          la = ctx.createAnalyser(); ra = ctx.createAnalyser()
          la.fftSize = ra.fftSize = 256
          split.connect(la, 0); split.connect(ra, 1)
        }
        source?.disconnect()
        const stream = capture.call(el)
        if (!stream.getAudioTracks().length) return
        source = ctx.createMediaStreamSource(stream)
        source.connect(split)
        ctx.resume().catch(() => {})
      } catch { /* no meter, then: the music is unaffected */ }
    }
    const level = (an: AnalyserNode) => {
      an.getFloatTimeDomainData(buf)
      let peak = 0
      for (let i = 0; i < buf.length; i++) { const v = Math.abs(buf[i]); if (v > peak) peak = v }
      return peak
    }
    const tick = (t: number) => {
      raf = requestAnimationFrame(tick)
      if (t - last < 66 || !ctx) return // about 15 a second is plenty for a bar
      last = t
      if (left.current) left.current.style.transform = `scaleX(${Math.min(1, level(la) * 1.15).toFixed(3)})`
      if (right.current) right.current.style.transform = `scaleX(${Math.min(1, level(ra) * 1.15).toFixed(3)})`
    }
    el.addEventListener('playing', connect)
    if (!el.paused) connect()
    raf = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(raf)
      el.removeEventListener('playing', connect)
      source?.disconnect()
      ctx?.close().catch(() => {})
      if (left.current) left.current.style.transform = 'scaleX(0)'
      if (right.current) right.current.style.transform = 'scaleX(0)'
    }
  }, [el, on]) // eslint-disable-line react-hooks/exhaustive-deps
}

function useVisible() {
  const [visible, setVisible] = useState(!document.hidden)
  useEffect(() => {
    const f = () => setVisible(!document.hidden)
    document.addEventListener('visibilitychange', f)
    return () => document.removeEventListener('visibilitychange', f)
  }, [])
  return visible
}

export default function CdPlayer() {
  const { id } = useParams()
  const discId = Number(id)
  const [params] = useSearchParams()
  const nav = useNavigate()
  const { user } = useApp()
  const { now, actions, audioEl, playing, toggle } = useCd()
  const [disc, setDisc] = useState<CdDisc | null>(null)
  const [err, setErr] = useState('')
  const [phase, setPhase] = useState<'loading' | 'confirm' | 'inserting' | 'ready'>('loading')
  const [timeMode, setTimeMode] = useState<'elapsed' | 'remain' | 'disc'>('elapsed')
  const [editing, setEditing] = useState(false)
  const [details, setDetails] = useState(false)
  const [identify, setIdentify] = useState(false)
  const [showGraphics, setShowGraphics] = useState(true)
  const [ejecting, setEjecting] = useState(false)
  const playKey = useRef<HTMLButtonElement>(null)
  const meterL = useRef<HTMLElement>(null)
  const meterR = useRef<HTMLElement>(null)
  const visible = useVisible()
  const mine = now?.disc.id === discId ? now : null
  const autoPlay = params.get('play') === '1'
  const startTrack = params.get('track') ? Number(params.get('track')) : undefined

  const begin = (d: CdDisc) => {
    actions.insert(d, { play: autoPlay || startTrack != null, track: startTrack })
    setPhase('ready')
  }
  useEffect(() => {
    let live = true
    setErr('')
    api.get<CdDisc>(`/cd/${discId}`).then((d) => {
      if (!live) return
      setDisc(d)
      if (now?.disc.id === discId) {
        actions.refresh(d)
        if (startTrack != null) actions.goTrack(startTrack, true)
        setPhase('ready')
      } else if (d.problem) setPhase('confirm')
      else if (d.insertEffect && !prefersReduced()) setPhase('inserting')
      else begin(d)
    }).catch((e) => { if (live) setErr(e.message) })
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [discId])

  useEffect(() => { if (phase === 'ready') playKey.current?.focus({ preventScroll: true }) }, [phase])
  useLevels(audioEl, phase === 'ready' && visible && playing && !!mine, meterL, meterR)

  const eject = () => {
    if (ejecting) return
    setEjecting(true)
    setTimeout(() => {
      actions.eject()
      nav(`/library/${disc?.libraryId ?? ''}`)
    }, prefersReduced() ? 0 : 420)
  }

  // Keys. Arrows stay with spatial navigation, so a remote can move around the deck.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (t?.tagName === 'INPUT' && (t as HTMLInputElement).type !== 'range' || t?.tagName === 'TEXTAREA' || t?.tagName === 'SELECT') return
      if (document.querySelector('[role=dialog]') || e.ctrlKey || e.metaKey || e.altKey) return
      if (!mine) return
      const act: Record<string, () => void> = {
        ' ': toggle, k: toggle, MediaPlayPause: toggle,
        j: () => actions.nudge(-10), l: () => actions.nudge(10), MediaRewind: () => actions.nudge(-10), MediaFastForward: () => actions.nudge(10),
        n: actions.next, p: actions.prev, PageDown: actions.next, PageUp: actions.prev, MediaTrackNext: actions.next, MediaTrackPrevious: actions.prev,
        s: actions.stop, MediaStop: actions.stop, e: eject,
        r: () => actions.setRepeat(mine.repeat === 'off' ? 'all' : mine.repeat === 'all' ? 'one' : 'off'),
        h: () => actions.setShuffle(!mine.shuffle),
      }
      const f = act[e.key]
      if (!f) return
      // Space on a button presses that button.
      if (e.key === ' ' && t?.tagName === 'BUTTON' && t !== playKey.current) return
      e.preventDefault()
      f()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  if (err) return <EmptyState icon={<IconInfo />} title="This disc can’t be played" text={err} action={<button className="btn" onClick={() => nav(-1)}>Back</button>} />
  if (!disc) return <Spinner />

  const art = disc.poster ? mediaUrl(`/api/items/${disc.id}/poster`) : ''
  const label = art ? { backgroundImage: `url(${art})` } : { backgroundImage: generatedArt(disc.discId) }
  const audioTracks = disc.tracks.filter((t) => t.audio)
  const spinning = !!mine && playing && visible && !ejecting

  if (phase === 'confirm') {
    return (
      <div className="cdp-page">
        <EmptyState icon={<IconDisc />} title={disc.title}
          text={disc.warnings.find((w) => w.code === disc.problem)?.message || 'This disc may not sound like music.'}
          action={<><button className="btn primary" onClick={() => begin(disc)}>Play it anyway</button> <button className="btn ghost" onClick={() => nav(-1)}>Back</button></>} />
      </div>
    )
  }

  return (
    <div className="cdp-page">
      {phase === 'inserting' && (
        <div className="cdp-insert">
          <InsertDisc poster={art || undefined} title={disc.title} label="CD" sound={disc.insertSound} onDone={() => begin(disc)} />
        </div>
      )}
      <BackLink to={`/library/${disc.libraryId}`} label="Back to the library" />
      <div className="cdp-layout">
        <section className={'cdp-deck' + (ejecting ? ' ejecting' : '')} aria-label="CD player">
          <div className="cdp-lid">
            <span className="cdp-shine" />
            <div className="cdp-well">
              <div className="cdp-plane">
                <div className={'cdp-disc' + (spinning ? ' spin' : '')}>
                  <div className="cdp-label" style={label}>{!art && <span>{disc.title}</span>}</div>
                  <div className="cdp-hole" />
                </div>
              </div>
            </div>
            {disc.hasGraphics && showGraphics && mine && <CdGraphics disc={disc} now={mine} el={audioEl} />}
          </div>
          <div className="cdp-front">
            <span className="cdp-slot" />
            <div className="cdp-brand" aria-hidden="true"><span className={'cdp-led' + (mine && playing ? ' on' : '')} />Witty<b>CD</b></div>
            <Display disc={disc} now={mine} playing={playing} timeMode={timeMode} cycleTime={() => setTimeMode(timeMode === 'elapsed' ? 'remain' : timeMode === 'remain' ? 'disc' : 'elapsed')} meterL={meterL} meterR={meterR} />
            <div className="cdp-keys" role="group" aria-label="Transport">
              <button className="cdp-key" onClick={eject} aria-label="Eject" title="Eject (E)" aria-keyshortcuts="e"><IconEject /></button>
              <button className="cdp-key" onClick={actions.prev} disabled={!mine} aria-label="Previous track" title="Previous track (P)" aria-keyshortcuts="p"><IconPrev /></button>
              <button className="cdp-key" onClick={() => actions.nudge(-10)} disabled={!mine} aria-label="Back 10 seconds" title="Back 10 seconds (J)" aria-keyshortcuts="j"><IconRew /></button>
              <button ref={playKey} className="cdp-key play" onClick={toggle} disabled={!mine} aria-label={playing && mine ? 'Pause' : 'Play'} title="Play or pause (Space)" aria-keyshortcuts="Space">{playing && mine ? <IconPause /> : <IconPlay />}</button>
              <button className="cdp-key" onClick={() => actions.nudge(10)} disabled={!mine} aria-label="Forward 10 seconds" title="Forward 10 seconds (L)" aria-keyshortcuts="l"><IconFf /></button>
              <button className="cdp-key" onClick={actions.next} disabled={!mine} aria-label="Next track" title="Next track (N)" aria-keyshortcuts="n"><IconNext /></button>
              <button className="cdp-key" onClick={actions.stop} disabled={!mine || mine.stopped} aria-label="Stop" title="Stop: back to the start of the disc (S)" aria-keyshortcuts="s"><IconStop /></button>
            </div>
            {mine && mine.track && (
              <input className="range cdp-seek" type="range" min={0} max={Math.max(1, Math.floor(mine.trackLen))} step={1} value={Math.max(0, Math.min(mine.trackSec, mine.trackLen))}
                style={{ '--pct': (mine.trackLen > 0 ? Math.max(0, Math.min(100, (mine.trackSec / mine.trackLen) * 100)) : 0) + '%' } as React.CSSProperties}
                aria-label="Position in this track" aria-valuetext={spoken(mine.trackSec)} onChange={(e) => actions.seekInTrack(Number(e.target.value))} />
            )}
            <div className="cdp-modes" role="group" aria-label="Play order">
              <button className={'cdp-mode' + (mine?.repeat !== 'off' && mine ? ' on' : '')} disabled={!mine} aria-label={`Repeat: ${mine?.repeat === 'one' ? 'this track' : mine?.repeat === 'all' ? 'the whole disc' : 'off'}`} title="Repeat (R)"
                onClick={() => mine && actions.setRepeat(({ off: 'all', all: 'one', one: 'off' } as Record<CdRepeat, CdRepeat>)[mine.repeat])}>Repeat{mine?.repeat === 'one' ? ' 1' : mine?.repeat === 'all' ? ' all' : ''}</button>
              <button className={'cdp-mode' + (mine?.shuffle ? ' on' : '')} disabled={!mine} aria-pressed={!!mine?.shuffle} title="Shuffle (H)" onClick={() => mine && actions.setShuffle(!mine.shuffle)}>Shuffle</button>
              <button className={'cdp-mode' + (editing || mine?.program ? ' on' : '')} disabled={!mine} aria-pressed={editing} title="Choose your own order" onClick={() => setEditing(!editing)}>Program</button>
              {mine?.track && mine.track.indexes.length > 0 && <>
                <button className="cdp-mode" onClick={() => actions.nextIndex(-1)} aria-label="Previous index">Index −</button>
                <button className="cdp-mode" onClick={() => actions.nextIndex(1)} aria-label="Next index">Index +</button>
              </>}
              {disc.hasGraphics && <button className={'cdp-mode' + (showGraphics ? ' on' : '')} aria-pressed={showGraphics} onClick={() => setShowGraphics(!showGraphics)}>Pictures</button>}
            </div>
          </div>
        </section>
        <section className="cdp-side" aria-label="Disc">
          <header className="cdp-head">
            <h1>{disc.title}</h1>
            <p className="cdp-artist">{[disc.artist, disc.year].filter(Boolean).join(' · ')}</p>
            <p className="muted cdp-sum">{audioTracks.length} {audioTracks.length === 1 ? 'track' : 'tracks'} · {fmtRuntime(disc.durationSec)}</p>
            <div className="cdp-actions">
              <button className="btn ghost sm" onClick={() => setDetails(!details)} aria-expanded={details}><IconInfo /> About this disc</button>
              {user?.isAdmin && <button className={'btn sm ' + (disc.lookup === 'ask' ? 'primary' : 'ghost')} onClick={() => setIdentify(true)}><IconSearch /> {disc.lookup === 'ask' ? `Choose from ${disc.matches} matches` : disc.lookup === 'matched' ? 'Change album' : 'Find this album'}</button>}
            </div>
          </header>
          {disc.warnings.filter((w) => w.code !== 'data-track' && w.code !== 'no-sheet').map((w) => <div key={w.code} className="cdp-note" role="note">{w.message}</div>)}
          {details && <Details disc={disc} now={mine} actions={actions} />}
          {editing && <ProgramBar program={mine?.program || null} disc={disc} onChange={actions.setProgram} onDone={() => setEditing(false)} />}
          <TrackRows disc={disc} now={mine} playing={playing} editing={editing}
            onPlay={(n) => { if (mine) actions.goTrack(n, true) }}
            onAdd={(n) => actions.setProgram([...(mine?.program || []), n])}
            onHidden={() => actions.goHidden()} />
          {disc.attribution && <p className="muted cdp-credit">{disc.attribution}{disc.releaseId && <> <a href={`https://musicbrainz.org/release/${disc.releaseId}`} target="_blank" rel="noreferrer">See this release</a>.</>}</p>}
          <p className="muted cdp-keyhelp"><kbd>Space</kbd> play or pause, <kbd>N</kbd>/<kbd>P</kbd> next or previous, <kbd>J</kbd>/<kbd>L</kbd> back or forward 10 seconds, <kbd>S</kbd> stop, <kbd>R</kbd> repeat, <kbd>H</kbd> shuffle, <kbd>E</kbd> eject.</p>
        </section>
      </div>
      {identify && <IdentifyDialog disc={disc} onClose={() => setIdentify(false)} onApplied={(d) => { setDisc(d); actions.refresh(d); setIdentify(false) }} />}
    </div>
  )
}

function Display({ disc, now, playing, timeMode, cycleTime, meterL, meterR }: { disc: CdDisc; now: CdNow | null; playing: boolean; timeMode: string; cycleTime: () => void; meterL: React.RefObject<HTMLElement | null>; meterR: React.RefObject<HTMLElement | null> }) {
  const tracks = disc.tracks.filter((t) => t.audio)
  const idle = !now || now.stopped
  const t = now?.track
  const announcer = t && !idle ? `Track ${t.number}: ${t.title}${t.artist ? ` by ${t.artist}` : ''}` : ''
  let tval = '', tlabel = ''
  if (idle) { tval = clock(disc.durationSec); tlabel = 'TOTAL' }
  else if (now && timeMode === 'remain') { tval = clock(-(now.trackLen - Math.max(0, now.trackSec))); tlabel = 'REMAIN' }
  else if (now && timeMode === 'disc') { tval = clock(-Math.max(0, disc.durationSec - now.discSec)); tlabel = 'DISC LEFT' }
  else if (now) { tval = clock(now.trackSec); tlabel = now.trackSec < 0 ? 'PREGAP' : 'ELAPSED' }
  const trackNum = idle ? two(tracks.length) : t ? two(t.number) : '--'
  const flag = (on: unknown, text: string) => <span className={on ? 'on' : ''}>{text}</span>
  const mark = (n: number) => (now?.order ? now.order.includes(n) : true)
  return (
    <div className="cdp-display">
      <div className="cdp-lcd">
        <div className="d-cell"><small>{idle ? 'TRACKS' : 'TRACK'}</small><b aria-hidden="true">{trackNum}</b></div>
        <div className="d-cell"><small>INDEX</small><b aria-hidden="true">{idle || !now ? '--' : two(now.index)}</b></div>
        <button className="d-cell d-time" onClick={cycleTime} aria-label={`Time: ${tlabel.toLowerCase()} ${spoken(idle ? disc.durationSec : now?.trackSec ?? 0)}. Change what it shows.`} title="Change what the time shows">
          <small>{tlabel}</small><b>{tval}</b>
        </button>
        <div className="d-title" title={t?.title || disc.title}><span>{idle ? (now ? 'READY  ·  ' : 'LOADING  ·  ') + disc.title : (t?.title || '') + (t?.artist ? ` · ${t.artist}` : '')}</span></div>
        <div className="d-cal" aria-hidden="true">
          {tracks.slice(0, 24).map((x) => <i key={x.number} className={(x.number === t?.number && !idle ? 'cur ' : '') + (mark(x.number) ? '' : 'off')}>{x.number}</i>)}
          {tracks.length > 24 && <i>+</i>}
        </div>
        <div className="d-flags" aria-hidden="true">
          {flag(playing && !idle, '▶')}
          {flag(now?.repeat === 'all', 'RPT')}
          {flag(now?.repeat === 'one', 'RPT 1')}
          {flag(now?.shuffle, 'SHUF')}
          {flag(now?.program, 'PROG')}
          {flag(t?.preEmphasis, 'PRE')}
          {flag(now?.format === 'flac' || now?.format === 'wav', 'LOSSLESS')}
          {flag(now?.format === 'aac', 'AAC')}
        </div>
        <div className="d-meter" aria-hidden="true">
          <span>L</span><div><i ref={meterL as React.RefObject<HTMLElement>} /></div>
          <span>R</span><div><i ref={meterR as React.RefObject<HTMLElement>} /></div>
        </div>
      </div>
      <div className="sr-only" role="status" aria-live="polite">{announcer}</div>
    </div>
  )
}

function TrackRows({ disc, now, playing, editing, onPlay, onAdd, onHidden }: { disc: CdDisc; now: CdNow | null; playing: boolean; editing: boolean; onPlay: (n: number) => void; onAdd: (n: number) => void; onHidden: () => void }) {
  const first = disc.tracks.find((t) => t.audio)
  return (
    <ol className="cdp-tracks">
      {disc.hiddenTrack && first && (
        <li>
          <button className="cdp-row" onClick={onHidden} disabled={!now}>
            <span className="n">00</span>
            <span className="t"><b>Hidden track</b><small>Before track 1, where most players never look</small></span>
            <span className="d">{clock((first.start - first.pregap) / SECTORS_PER_SEC)}</span>
          </button>
        </li>
      )}
      {disc.tracks.map((t) => {
        const on = now?.track?.number === t.number && !now.stopped
        const pos = now?.order ? now.order.indexOf(t.number) : -1
        return (
          <li key={t.number} className={on ? 'on' : ''}>
            <button className="cdp-row" onClick={() => t.audio && onPlay(t.number)} disabled={!t.audio || !now} aria-current={on ? 'true' : undefined}>
              <span className="n">{on && playing ? <i className="cdp-eq" aria-hidden="true"><b /><b /><b /></i> : two(t.number)}</span>
              <span className="t"><b>{t.title}</b>{t.artist && <small>{t.artist}</small>}{!t.audio && <small>Not played: this is the disc’s data</small>}</span>
              {pos >= 0 && <span className="p" title="Place in the order">{pos + 1}</span>}
              {t.preEmphasis && <span className="p" title="This track has pre-emphasis, which is being corrected">PRE</span>}
              <span className="d">{t.audio ? clock((t.end - t.start) / SECTORS_PER_SEC) : ''}</span>
            </button>
            {editing && t.audio && <button className="ibtn cdp-add" onClick={() => onAdd(t.number)} aria-label={`Add ${t.title} to the program`}><IconPlus /></button>}
          </li>
        )
      })}
    </ol>
  )
}

function ProgramBar({ program, disc, onChange, onDone }: { program: number[] | null; disc: CdDisc; onChange: (p: number[] | null) => void; onDone: () => void }) {
  const name = (n: number) => disc.tracks.find((t) => t.number === n)?.title || `Track ${n}`
  return (
    <div className="cdp-program" role="group" aria-label="Program">
      <p className="muted">{program?.length ? 'Plays in this order, without gaps between neighbouring tracks of the disc.' : 'Add tracks with the plus buttons below to choose your own order.'}</p>
      {!!program?.length && <ol>{program.map((n, i) => <li key={i}><span>{two(n)} {name(n)}</span><button className="ibtn" onClick={() => onChange(program.filter((_, j) => j !== i))} aria-label={`Remove ${name(n)} from the program`}><IconClose /></button></li>)}</ol>}
      <div className="cdp-actions">
        <button className="btn ghost sm" onClick={() => onChange(null)} disabled={!program?.length}><IconTrash /> Clear</button>
        <button className="btn sm" onClick={onDone}>Done</button>
      </div>
    </div>
  )
}

function Details({ disc, now, actions }: { disc: CdDisc; now: CdNow | null; actions: { setQuality: (q: 'auto' | 'lossless' | 'compressed') => void } }) {
  const facts: [string, string][] = [
    ['Image', `${disc.format.toUpperCase()}${disc.sizeBytes ? `, ${(disc.sizeBytes / 1048576).toFixed(0)} MB` : ''}`],
    ['Length', `${clock(disc.durationSec)} (${(disc.samples * 4 / 1048576).toFixed(0)} MB of CD audio)`],
    ['Sessions', String(disc.sessions)],
    ['Disc ID', disc.discId],
    ...(disc.cddbId ? [['CDDB ID', disc.cddbId] as [string, string]] : []),
    ['Names from', { musicbrainz: 'MusicBrainz', sheet: 'the disc’s own text', sidecar: 'a saved file beside the image', file: 'the file’s name' }[disc.source || 'file'] || disc.source || ''],
    ['Now sending', now ? { flac: 'FLAC: lossless', wav: 'WAV: lossless, uncompressed', aac: 'AAC: compressed' }[now.format] : '—'],
  ]
  return (
    <div className="cdp-details">
      <dl>{facts.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
      {now && (
        <label className="cdp-q">Sound quality{' '}
          <select className="select sm" value={now.quality} onChange={(e) => actions.setQuality(e.target.value as 'auto')}>
            <option value="auto">Automatic: lossless, compressed if the connection struggles</option>
            <option value="lossless">Always lossless</option>
            {disc.formats.includes('aac') && <option value="compressed">Always compressed, to save data</option>}
          </select>
        </label>
      )}
      {disc.warnings.filter((w) => w.code === 'data-track' || w.code === 'no-sheet').map((w) => <p key={w.code} className="muted">{w.message}</p>)}
    </div>
  )
}

type Match = { releaseId: string; title: string; artist: string; date?: string; country?: string; barcode?: string; label?: string; disc: number; discs: number; exact: boolean; hasCover: boolean; tracks: { number: number; title: string; artist?: string }[]; url: string; group?: string }

function IdentifyDialog({ disc, onClose, onApplied }: { disc: CdDisc; onClose: () => void; onApplied: (d: CdDisc) => void }) {
  const [matches, setMatches] = useState<Match[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const [open, setOpen] = useState('')
  useEffect(() => {
    api.get<{ matches: Match[] }>(`/cd/${disc.id}/matches`).then((r) => setMatches(r.matches)).catch((e) => { setMatches([]); setError(e.message) })
  }, [disc.id])
  const choose = async (releaseId: string) => {
    setBusy(releaseId || 'reset')
    try {
      onApplied(await api.post<CdDisc>(`/cd/${disc.id}/identify`, { releaseId }))
      toast(releaseId ? 'Album updated' : 'Back to the disc’s own names')
    } catch (e) {
      setError((e as Error).message)
      setBusy('')
    }
  }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal cdp-identify" role="dialog" aria-modal="true" aria-labelledby="cdp-id-h" onClick={(e) => e.stopPropagation()}>
        <div className="cdp-id-head">
          <h2 id="cdp-id-h">Which album is this?</h2>
          <button className="ibtn" onClick={onClose} aria-label="Close"><IconClose /></button>
        </div>
        <p className="muted">Looked up by the disc’s table of contents, the track lengths and nothing else. No names and no audio leave your server.</p>
        {error && <div className="error">{error}</div>}
        {!matches ? <Spinner /> : matches.length === 0 && !error ? <p>Nobody has told MusicBrainz about this disc yet, so it keeps the names it came with.</p> : (
          <ul className="cdp-matches">
            {matches.map((m) => (
              <li key={m.releaseId}>
                <div className="cdp-match">
                  <div className="cdp-thumb">{m.hasCover ? <img src={mediaUrl(`/api/cd-covers/${m.releaseId}`)} alt="" loading="lazy" /> : <IconDisc />}</div>
                  <div className="cdp-mt">
                    <b>{m.title}</b>
                    <span>{m.artist}</span>
                    <small className="muted">{[m.date?.slice(0, 4), m.country, m.label, m.discs > 1 ? `disc ${m.disc} of ${m.discs}` : '', `${m.tracks.length} tracks`, m.barcode].filter(Boolean).join(' · ')}</small>
                    {!m.exact && <small className="cdp-fuzzy">Close match: its track lengths are similar, but it isn’t listed as this exact disc.</small>}
                    <button className="cdp-link" onClick={() => setOpen(open === m.releaseId ? '' : m.releaseId)} aria-expanded={open === m.releaseId}>{open === m.releaseId ? 'Hide tracks' : 'Show tracks'}</button>
                    {open === m.releaseId && <ol className="cdp-mtracks">{m.tracks.map((t) => <li key={t.number}>{t.title}</li>)}</ol>}
                  </div>
                  <button className="btn sm primary" disabled={!!busy} onClick={() => choose(m.releaseId)}>{busy === m.releaseId ? 'Using…' : 'This one'}</button>
                </div>
              </li>
            ))}
          </ul>
        )}
        <div className="cdp-actions">
          {disc.lookup === 'matched' && <button className="btn ghost sm" disabled={!!busy} onClick={() => choose('')}>Use the disc’s own names</button>}
          <span className="muted cdp-credit">Album data from <a href="https://musicbrainz.org" target="_blank" rel="noreferrer">MusicBrainz</a> (CC0). Covers from the <a href="https://coverartarchive.org" target="_blank" rel="noreferrer">Cover Art Archive</a>.</span>
        </div>
      </div>
    </div>
  )
}

