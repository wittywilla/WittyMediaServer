// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, fmtTime, posterUrl } from '../api'
import { Placeholder } from '../components/common'
import { IconBack15, IconChapters, IconClose, IconDown, IconForward30, IconFull, IconMic, IconNext, IconPause, IconPlay, IconPrev, IconVisualizer } from '../icons'
import { useArtColour } from './art'
import { Backdrop, NATIVE_STYLES, type NativeStyle } from './backdrop'
import { AddToPlaylist, Like, Pop, Seek, Volume } from './controls'
import { IconKaraoke, IconList, IconMoon, IconMore, IconReadOpen, IconRepeat, IconRepeatOne, IconShuffle, IconSliders, IconStation, IconTurtle } from './icons'
import { LyricsPanel } from './LyricsPanel'
import { useLyrics } from './lyrics'
import { QueuePanel } from './QueuePanel'
import { ReadPanel, useExtras } from './ReadPanel'
import { SoundPanel } from './SoundPanel'
import type { PlayerView } from './types'
import { placeIn, useMusicLinks } from '../pages/music/links'
import { acceptedFlashNotice, shaderEffects, shaderSession, trackingMedia, VisualizerView, type ShaderEffect } from './visualizer'
import './nowplaying.css'

const Karaoke = lazy(() => import('./Karaoke'))

// The full-screen player. Artwork with its reflection and a visualizer behind it, the
// transport, and from here everything else: lyrics, karaoke, the queue, the sound. It looks
// the same in every theme because it sits on black, whatever the page behind it is doing.

type Prefs = { viz: boolean; style: string; slow: boolean; shuffleEvery: number; shufflePool: 'all' | 'mine' }
const DEFAULT_PREFS: Prefs = { viz: true, style: 'bars', slow: false, shuffleEvery: 0, shufflePool: 'mine' }
const SHUFFLE_CHOICES = [0, 15, 30, 60, 120, 300, 600]
const everyLabel = (s: number) => (s === 0 ? 'Off: stay on this one' : s < 60 ? `Every ${s} seconds` : s === 60 ? 'Every minute' : `Every ${s / 60} minutes`)

let prefsSaving = Promise.resolve()
function savePrefs(p: Prefs) {
  prefsSaving = prefsSaving.then(async () => {
    const ui = await api.get<Record<string, unknown>>('/me/ui')
    await api.put('/me/ui', { ...ui, musicPlayer: p })
  }).catch(() => { /* the next change tries again */ })
}

function readPrefs(v: unknown): Prefs {
  const s = (typeof v === 'object' && v !== null ? v : {}) as Partial<Prefs>
  return {
    viz: s.viz !== false, style: typeof s.style === 'string' && s.style.length <= 48 ? s.style : 'bars', slow: s.slow === true,
    shuffleEvery: SHUFFLE_CHOICES.includes(s.shuffleEvery as number) ? (s.shuffleEvery as number) : 0, shufflePool: s.shufflePool === 'all' ? 'all' : 'mine',
  }
}

const isNative = (style: string): style is NativeStyle => NATIVE_STYLES.some((n) => n.id === style)

type Panel = '' | 'lyrics' | 'queue' | 'sound' | 'read'

export default function NowPlaying({ p }: { p: PlayerView }) {
  const it = p.current!
  const root = useRef<HTMLDivElement>(null)
  const [prefs, setPrefsState] = useState<Prefs>(DEFAULT_PREFS)
  const [prefsLoaded, setPrefsLoaded] = useState(false)
  const [panel, setPanel] = useState<Panel>(p.fullOpts.panel || '')
  const [karaoke, setKaraoke] = useState(false)
  const [shown, setShown] = useState('') // what shuffle is showing now, if it's on
  const [shaders, setShaders] = useState<ShaderEffect[]>([])
  const [wholeShader, setWholeShader] = useState(false)
  const [said, setSaid] = useState('')
  const artUrl = it.poster ? posterUrl(it) : ''
  const rgb = useArtColour(artUrl)
  const trackMedia = useMemo(() => trackingMedia(p.media), [p.media])
  const canSing = p.mode === 'music' && !p.cdActive
  const extras = useExtras(canSing ? it.id : undefined)
  const lyricsWanted = panel === 'lyrics' || panel === 'read' || karaoke
  const ly = useLyrics(canSing ? it.id : undefined, lyricsWanted)

  const setPrefs = useCallback((patch: Partial<Prefs>) => {
    setPrefsState((old) => {
      const n = { ...old, ...patch }
      savePrefs(n)
      return n
    })
  }, [])
  useEffect(() => {
    api.get<{ musicPlayer?: unknown }>('/me/ui').then((u) => {
      const saved = readPrefs(u.musicPlayer)
      setPrefsState(p.fullOpts.visualizer ? { ...saved, viz: true } : saved)
    }).catch(() => {}).finally(() => setPrefsLoaded(true))
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
  // The plugin comes when it's needed: most people's Bars, Waves and Drift don't need it.
  const needShaders = prefsLoaded && prefs.viz && (!isNative(prefs.style) || (prefs.shuffleEvery > 0 && prefs.shufflePool === 'all'))
  const loadShaders = useCallback(() => { shaderEffects().then(setShaders).catch(() => {}) }, [])
  useEffect(() => { if (needShaders) loadShaders() }, [needShaders, loadShaders])
  useEffect(() => { root.current?.focus({ preventScroll: true }) }, [])
  useEffect(() => { if (p.fullOpts.panel) setPanel(p.fullOpts.panel) }, [p.fullOpts])

  const style = shown || prefs.style

  // Shuffle: only ever because someone chose it. Their own styles to pick from, or every
  // effect there is, the lively shaders among them only once the flashing notice has been
  // read, and never with Slow on.
  useEffect(() => {
    if (!prefs.viz || !prefs.shuffleEvery) { setShown(''); return }
    const t = setInterval(() => {
      void acceptedFlashNotice().then((read) => {
        const pool = [...NATIVE_STYLES.map((n) => n.id as string)]
        const gentleOnly = prefs.slow || matchMedia('(prefers-reduced-motion: reduce)').matches
        if (prefs.shufflePool === 'all' && read) pool.push(...shaders.filter((s) => !gentleOnly || s.motion === 'gentle').map((s) => s.id))
        setShown((cur) => {
          const others = pool.filter((x) => x !== (cur || prefs.style))
          return others[Math.floor(Math.random() * others.length)] || cur
        })
      })
    }, prefs.shuffleEvery * 1000)
    return () => clearInterval(t)
  }, [prefs.viz, prefs.shuffleEvery, prefs.shufflePool, prefs.slow, prefs.style, shaders])

  const toggleFullscreen = () => {
    const el = root.current
    if (!el) return
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
    else void el.requestFullscreen?.({ navigationUI: 'hide' }).catch(() => {})
  }
  useEffect(() => () => { if (document.fullscreenElement) void document.exitFullscreen().catch(() => {}) }, [])

  // Keys. Over the screen itself (focus on nothing in particular) they control the music;
  // once a remote has moved onto a control, arrows move between controls, so everything can
  // be reached with arrows alone. A press with a mouse puts the focus back on the screen.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Anything open on top has its own keys, Escape first of all.
      if (document.querySelector('.wv-dialog-back, .np-fix, .mp-pop-body') || karaoke || e.ctrlKey || e.metaKey || e.altKey) return
      const t = e.target as HTMLElement
      const tag = t?.tagName
      const typing = tag === 'TEXTAREA' || tag === 'SELECT' || (tag === 'INPUT' && (t as HTMLInputElement).type !== 'range') || t?.isContentEditable
      const key = e.key
      if (key === 'Escape' || key === 'GoBack' || key === 'BrowserBack') {
        if (wholeShader) return
        e.preventDefault()
        e.stopImmediatePropagation()
        if (panel) { setPanel(''); root.current?.focus({ preventScroll: true }) } else p.closeFull()
        return
      }
      if (typing) return
      const onScreen = t === root.current || t === document.body
      const consume = () => { e.preventDefault(); e.stopImmediatePropagation() }
      if (key === ' ' && (onScreen || tag !== 'BUTTON')) { consume(); p.toggle(); return }
      if (onScreen && key === 'ArrowLeft') { consume(); p.prev(); return }
      if (onScreen && key === 'ArrowRight') { consume(); p.next(); return }
      if (onScreen && (key === 'ArrowUp' || key === 'ArrowDown' || key === 'Enter')) { consume(); root.current?.querySelector<HTMLElement>('.np-play')?.focus(); return }
      if (tag === 'INPUT') return // a slider keeps the rest of its keys
      switch (key.toLowerCase()) {
        case 'l': if (canSing) { consume(); setPanel(panel === 'lyrics' ? '' : 'lyrics') } return
        case 'k': if (canSing) { consume(); setKaraoke(true) } return
        case 'q': consume(); setPanel(panel === 'queue' ? '' : 'queue'); return
        case 'm': consume(); p.setSound({ muted: !p.sound.muted }); return
        case 's': if (!p.books && !p.live) { consume(); p.setShuffle(!p.shuffle) } return
        case 'r': if (!p.books && !p.live) { consume(); p.cycleRepeat() } return
        case 'v': if (p.vizHere) { consume(); setPrefs({ viz: !prefs.viz }) } return
        case 'f': consume(); toggleFullscreen(); return
        case ',': if (!p.live) { consume(); p.seek(Math.max(0, p.position() - 10)) } return
        case '.': if (!p.live) { consume(); p.seek(p.position() + 10) } return
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }) // Changes with the time; re-adding a listener is nothing, and it can't go stale.

  const pickStyle = (id: string) => {
    setShown('')
    setPrefs({ style: id, viz: true })
    const name = NATIVE_STYLES.find((n) => n.id === id)?.name || shaders.find((s) => s.id === id)?.name
    if (name) setSaid(`Visualizer: ${name}`)
  }
  // An effect picked on the whole screen stays picked, if an effect is what was showing.
  const leaveWhole = () => {
    setWholeShader(false)
    const now = shaderSession()?.status.effect
    if (now && !isNative(style)) { setShown(''); setPrefs({ style: now }) }
    loadShaders()
  }
  // A song's music video, to go to at the same place.
  const video = useMusicLinks(p.mode === 'music' && !p.live && !p.cdActive ? it.id : undefined)?.tied[0]
  const watchVideo = () => {
    if (!video) return
    const at = placeIn('video', p.position(), video, video.video.runtimeSec)
    if (p.playing) p.toggle()
    p.goTowards(`/watch/${video.videoId}${at ? `?t=${Math.floor(at)}` : ''}`)
  }
  const subtitle = p.live ? (p.retrying ? 'Reconnecting…' : p.stationNow || 'Live') : it.artist || it.albumArtist || ''
  const placeLine = p.books ? 'Chapters' : p.live ? 'Stations' : 'Queue'

  return (
    <div className={'np' + (panel ? ' has-panel panel-' + panel : '')} ref={root} tabIndex={-1} role="dialog" aria-modal="true" aria-label={`Now playing: ${it.title}`}
      style={{ '--art': rgb } as React.CSSProperties} onClickCapture={(e) => { if (e.detail > 0) requestAnimationFrame(() => { if (!document.activeElement?.closest('input, select, textarea, .mp-pop-body, .np-panel')) root.current?.focus({ preventScroll: true }) }) }}>
      <div className="np-glow" aria-hidden="true" />
      {/* One canvas: it goes to the whole-screen view while that's open, and comes back after. */}
      {p.vizHere && prefsLoaded && prefs.viz && !wholeShader && <NpBackground style={style} slow={prefs.slow} rgb={rgb} playing={p.playing} media={p.media} trackMedia={trackMedia} onDecline={() => { setShown(''); setPrefs({ style: 'bars' }) }} />}
      {wholeShader && <VisualizerView media={trackMedia} place="full" onPlace={(pl) => { if (pl !== 'full') leaveWhole() }} onClose={leaveWhole} />}

      <header className="np-top">
        <button type="button" className="mp-ibtn np-close" onClick={p.closeFull} aria-label="Close the full player (Esc)" title="Close (Esc)"><IconDown /></button>
        <div className="np-where">
          <span className="np-label">{p.live ? 'Live now' : p.books ? (p.mode === 'podcasts' ? 'Podcast' : 'Audiobook') : 'Now playing'}</span>
          <span className="np-where-sub">{p.live ? it.title : it.album || subtitle}</span>
        </div>
        <div className="np-top-right">
          <Volume p={p} />
          {!p.live && <AddToPlaylist items={[it]} />}
          <SleepButton p={p} />
          <button type="button" className={'mp-ibtn' + (panel === 'queue' ? ' on' : '')} aria-pressed={panel === 'queue'} aria-label={placeLine} title={`${placeLine} (Q)`} onClick={() => setPanel(panel === 'queue' ? '' : 'queue')}>{p.books ? <IconChapters /> : <IconList />}</button>
          <Like p={p} />
          <button type="button" className="mp-ibtn np-fill" onClick={toggleFullscreen} aria-label="Fill the screen (F)" title="Fill the screen (F)"><IconFull /></button>
          <Pop label="More" icon={<IconMore />}>
            {(close) => (
              <div className="mp-menu" role="menu">
                <button type="button" role="menuitem" onClick={() => { setPanel(panel === 'sound' ? '' : 'sound'); close() }}><IconSliders size={18} /> Sound: equaliser and more</button>
                {canSing && <button type="button" role="menuitem" onClick={() => { p.startRadio(it); close() }}><IconStation size={18} /> Start a station from this song</button>}
                {p.albumUrl && <button type="button" role="menuitem" onClick={() => p.goTowards(p.albumUrl)}>{p.live ? 'Go to station' : p.books ? 'Go to book' : 'Go to album'}</button>}
                {p.artistUrl && <button type="button" role="menuitem" onClick={() => p.goTowards(p.artistUrl)}>Go to artist</button>}
                {video && <button type="button" role="menuitem" onClick={() => { close(); watchVideo() }}>Watch the video</button>}
                <button type="button" role="menuitem" onClick={p.stop}><IconClose size={18} /> Stop and close</button>
              </div>
            )}
          </Pop>
        </div>
      </header>

      <div className="np-stage">
        <div className="np-main">
          <div className="np-artbox">
            <div className="np-art" style={p.live ? { backgroundSize: 'contain' } : undefined}>
              {artUrl ? <img src={artUrl} alt={`Cover of ${it.album || it.title}`} style={p.live ? { objectFit: 'contain' } : undefined} /> : <Placeholder title={it.title} />}
            </div>
            {artUrl && !p.live && <div className="np-reflect" aria-hidden="true"><img src={artUrl} alt="" /></div>}
          </div>
          <div className="np-title">
            <h2 className="ab-title">{it.title}</h2>
            {p.artistUrl && !p.live
              ? <button type="button" className="np-artist" onClick={() => p.goTowards(p.artistUrl)}>{subtitle}</button>
              : <p className="np-artist plain">{subtitle}</p>}
            {video && <button type="button" className="np-chip np-video" onClick={watchVideo} title="Watch the video, from here when the two line up"><IconPlay size={16} /> Watch the video</button>}
          </div>
          {!p.live && (
            <div className="np-seek">
              <Seek p={p} />
              <div className="np-times"><span>{fmtTime(p.time)}</span><span>{fmtTime(p.duration)}</span></div>
            </div>
          )}
          <div className="np-transport">
            {!p.books && !p.live && <button type="button" className={'mp-ibtn tall' + (p.shuffle ? ' on' : '')} aria-pressed={p.shuffle} aria-label="Shuffle" title="Shuffle (S)" onClick={() => p.setShuffle(!p.shuffle)}><IconShuffle /></button>}
            {p.books && <button type="button" className="mp-ibtn tall" onClick={() => p.seek(Math.max(0, p.position() - 15))} aria-label="Back 15 seconds"><IconBack15 /></button>}
            <button type="button" className="mp-ibtn taller" onClick={p.prev} aria-label="Previous"><IconPrev /></button>
            <button type="button" className="np-play" onClick={p.toggle} aria-label={p.playing ? 'Pause' : 'Play'}>{p.playing ? <IconPause size={36} /> : <IconPlay size={36} />}</button>
            <button type="button" className="mp-ibtn taller" onClick={p.next} aria-label="Next"><IconNext /></button>
            {p.books && <button type="button" className="mp-ibtn tall" onClick={() => p.seek(p.position() + 30)} aria-label="Forward 30 seconds"><IconForward30 /></button>}
            {!p.books && !p.live && <button type="button" className={'mp-ibtn tall' + (p.repeat !== 'off' ? ' on' : '')} aria-label={p.repeat === 'one' ? 'Repeat this song' : p.repeat === 'all' ? 'Repeat all' : 'Repeat off'} title="Repeat (R)" onClick={p.cycleRepeat}>{p.repeat === 'one' ? <IconRepeatOne /> : <IconRepeat />}</button>}
          </div>

          <div className="np-tools">
            {p.vizHere && (
              <>
                <button type="button" className={'np-viz-toggle ab-viz-btn' + (prefs.viz ? ' on' : '')} aria-pressed={prefs.viz} title="Visualizer (V)" onClick={() => setPrefs({ viz: !prefs.viz })}><IconVisualizer size={22} /> Visualizer</button>
                {prefs.viz && <div className="np-styles" role="radiogroup" aria-label="Visualizer style">
                  {NATIVE_STYLES.map((n) => <button type="button" role="radio" key={n.id} aria-checked={style === n.id} className={style === n.id ? 'on' : ''} onClick={() => pickStyle(n.id)}>{n.name}</button>)}
                  <Pop label="More visualizer styles" className={'np-more-styles' + (!isNative(style) ? ' on' : '')} icon={<span>{isNative(style) ? 'More' : shaders.find((s) => s.id === style)?.name || 'More'}</span>}>
                    {(close) => (
                      <div className="mp-menu np-style-menu" role="menu" ref={(el) => { if (el && !shaders.length) loadShaders() }}>
                        {!shaders.length && <span className="mp-menu-note">Loading…</span>}
                        {shaders.map((s) => <button type="button" role="menuitemradio" aria-checked={style === s.id} key={s.id} className={style === s.id ? 'on' : ''} onClick={() => { pickStyle(s.id); close() }}>{s.name}</button>)}
                        <span className="mp-menu-rule" />
                        <button type="button" role="menuitem" onClick={() => { setWholeShader(true); close() }}>Effect settings and your own effects…</button>
                      </div>
                    )}
                  </Pop>
                </div>}
                {prefs.viz && <button type="button" className={'np-chip slow-chip' + (prefs.slow ? ' on' : '')} aria-pressed={prefs.slow} title="A slower, gentler pace" onClick={() => setPrefs({ slow: !prefs.slow })}><IconTurtle size={20} /> Slow</button>}
                {prefs.viz && (
                  <Pop label={prefs.shuffleEvery ? `Shuffling visualizers: ${everyLabel(prefs.shuffleEvery)}` : 'Shuffle visualizers'} className={'np-chip-pop' + (prefs.shuffleEvery ? ' on' : '')} icon={<><IconShuffle size={18} /><span>{prefs.shuffleEvery ? everyLabel(prefs.shuffleEvery).replace('Every ', '') : 'Styles'}</span></>}>
                    {(close) => (
                      <div className="mp-menu np-style-menu">
                        <span className="mp-menu-note">Change style by itself. It stays still unless you choose this.</span>
                        {SHUFFLE_CHOICES.map((c) => <button type="button" key={c} role="menuitemradio" aria-checked={prefs.shuffleEvery === c} className={prefs.shuffleEvery === c ? 'on' : ''} onClick={() => { setPrefs({ shuffleEvery: c }); close() }}>{everyLabel(c)}</button>)}
                        <span className="mp-menu-rule" />
                        <button type="button" role="menuitemradio" aria-checked={prefs.shufflePool === 'mine'} className={prefs.shufflePool === 'mine' ? 'on' : ''} onClick={() => setPrefs({ shufflePool: 'mine' })}>Bars, Waves and Drift</button>
                        <button type="button" role="menuitemradio" aria-checked={prefs.shufflePool === 'all'} className={prefs.shufflePool === 'all' ? 'on' : ''} onClick={() => setPrefs({ shufflePool: 'all' })}>Every effect</button>
                      </div>
                    )}
                  </Pop>
                )}
                <span className="np-tools-rule" aria-hidden="true" />
              </>
            )}
            {canSing && <button type="button" className={'np-chip' + (panel === 'lyrics' ? ' on' : '')} aria-pressed={panel === 'lyrics'} title="Lyrics (L)" onClick={() => setPanel(panel === 'lyrics' ? '' : 'lyrics')}><IconMic size={20} /> Lyrics</button>}
            {canSing && <button type="button" className="np-chip" title="Karaoke (K)" onClick={() => setKaraoke(true)}><IconKaraoke size={20} /> Karaoke</button>}
            {canSing && <button type="button" className={'np-chip' + (panel === 'read' ? ' on' : '')} aria-pressed={panel === 'read'} title="Read the words, or the album's booklet" onClick={() => setPanel(panel === 'read' ? '' : 'read')}><IconReadOpen size={20} /> Read{extras.length > 0 && <span className="np-count">{extras.length}</span>}</button>}
            {!p.live && !p.cdActive && (
              <select className="np-speed" aria-label="Playback speed" title="Playback speed (< and >)" value={p.rate} onChange={(e) => p.changeRate(Number(e.target.value))}>
                {[0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3].filter((r) => r <= p.maxSpeed).map((r) => <option key={r} value={r}>{r}×</option>)}
              </select>
            )}
          </div>
        </div>

        {panel && (
          <aside className="np-panel" aria-label={panel === 'lyrics' ? 'Lyrics' : panel === 'queue' ? placeLine : panel === 'sound' ? 'Sound' : 'Read'}>
            {panel === 'lyrics' && <LyricsPanel p={p} ly={ly} onClose={() => setPanel('')} />}
            {panel === 'queue' && <QueuePanel p={p} heading={placeLine} onClose={() => setPanel('')} />}
            {panel === 'sound' && <SoundPanel p={p} onClose={() => setPanel('')} />}
            {panel === 'read' && <ReadPanel itemId={it.id} extras={extras} ly={ly} onClose={() => setPanel('')} />}
          </aside>
        )}
      </div>

      <p className="np-hints" aria-hidden="true">
        <kbd>Esc</kbd> close · <kbd>Space</kbd> play or pause · <kbd>←</kbd> <kbd>→</kbd> previous or next
        {canSing && <> · <kbd>L</kbd> lyrics · <kbd>K</kbd> karaoke</>}{p.vizHere && <> · <kbd>V</kbd> visualizer</>}
      </p>
      <div className="mp-sr" aria-live="polite">{said}</div>
      {karaoke && <Suspense fallback={null}><Karaoke p={p} ly={ly} onClose={() => { setKaraoke(false); root.current?.focus({ preventScroll: true }) }} /></Suspense>}
    </div>
  )
}

function SleepButton({ p }: { p: PlayerView }) {
  const on = p.sleepAt !== null
  const at = typeof p.sleepAt === 'number' ? new Date(p.sleepAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : ''
  return (
    <Pop label={on ? (at ? `Sleep timer: stops at ${at}` : 'Sleep timer: stops after this one') : 'Sleep timer'} on={on} icon={<IconMoon />}>
      {(close) => (
        <div className="mp-menu" role="menu">
          {on && <button type="button" role="menuitem" onClick={() => { p.setSleepAt(null); close() }}>Turn it off{at ? ` (stops at ${at})` : ''}</button>}
          {[15, 30, 45, 60, 90].map((m) => <button type="button" role="menuitem" key={m} onClick={() => { p.setSleepAt(Date.now() + m * 60000); close() }}>In {m < 60 ? `${m} minutes` : m === 60 ? '1 hour' : '90 minutes'}</button>)}
          {!p.live && <button type="button" role="menuitem" onClick={() => { p.setSleepAt('chapter'); close() }}>{p.books ? 'At the end of this chapter' : 'After this song'}</button>}
        </div>
      )}
    </Pop>
  )
}

// What's behind the artwork: the player's own light styles, or one of the plugin's shaders.
function NpBackground({ style, slow, rgb, playing, media, trackMedia, onDecline }: { style: string; slow: boolean; rgb: string; playing: boolean; media: () => HTMLAudioElement | null; trackMedia: () => HTMLMediaElement | null; onDecline: () => void }) {
  const native = isNative(style)
  return native
    ? <NativeBackground style={style} slow={slow} rgb={rgb} playing={playing} media={media} />
    : <ShaderBackground id={style} slow={slow} media={trackMedia} onDecline={onDecline} />
}

function NativeBackground({ style, slow, rgb, playing, media }: { style: NativeStyle; slow: boolean; rgb: string; playing: boolean; media: () => HTMLAudioElement | null }) {
  const canvas = useRef<HTMLCanvasElement>(null)
  const drawer = useRef<Backdrop | null>(null)
  useEffect(() => {
    if (!canvas.current) return
    const b = new Backdrop(canvas.current, media)
    drawer.current = b
    return () => { b.destroy(); drawer.current = null }
  }, [media])
  useEffect(() => {
    const b = drawer.current
    if (!b) return
    b.style = style
    b.slow = slow
    b.setColour(rgb)
    b.active = playing
  }, [style, slow, rgb, playing])
  return <canvas className="np-canvas" ref={canvas} aria-hidden="true" />
}

function ShaderBackground({ id, slow, media, onDecline }: { id: string; slow: boolean; media: () => HTMLMediaElement | null; onDecline: () => void }) {
  // It may still be arriving, or waiting on the flashing notice: choose once it's ready.
  useEffect(() => {
    const t = setInterval(() => {
      const s = shaderSession()
      if (!s?.loaded || !s.settings.notice) return
      clearInterval(t)
      s.update({ calm: slow ? true : null })
      s.select(id)
    }, 120)
    return () => clearInterval(t)
  }, [id, slow])
  return <div className="np-shader"><VisualizerView media={media} place="panel" onClose={onDecline} /></div>
}
