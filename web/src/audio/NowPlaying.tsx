// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMedia } from '../shell/useNav'
import { useApp } from '../state'
import { UseAComputer, advancedHere } from '../ui/advanced'
import ArtView from './ArtView'
import LookPanel, { type LookMode } from './LookPanel'
import { DEFAULT_LOOK, lookFor, publishLook, readLook, readSongLooks, setSongLook, type Look, type SongLook } from './look'
import { IconPalette } from '../icons'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, fmtTime, posterUrl } from '../api'
import { Placeholder } from '../components/common'
import { IconBack15, IconChapters, IconClose, IconDown, IconForward30, IconFull, IconImage, IconMic, IconNext, IconPause, IconPlay, IconPrev, IconVisualizer } from '../icons'
import { useArtColour } from './art'
import { Backdrop, NATIVE_STYLES, type NativeStyle } from './backdrop'
import { AddToPlaylist, Like, Pop, Seek, Volume } from './controls'
import { AudioPlayOn } from '../remote/PlayOnButton'
import { IconKaraoke, IconList, IconMoon, IconMore, IconReadOpen, IconRepeat, IconRepeatOne, IconShuffle, IconSliders, IconStation, IconTurtle } from './icons'
import { LyricsPanel } from './LyricsPanel'
import { useLyrics } from './lyrics'
import { useIdleHide } from './idle'
import { Segmented } from '../ui/Choice'
import { NowPlayingCanvas } from '../canvas/NowPlayingCanvas'
import { useCanvas } from '../canvas/useCanvas'
import { QueuePanel } from './QueuePanel'
import { ReadPanel, useExtras } from './ReadPanel'
import { SleepPanel } from './SleepPanel'
import { SoundPanel } from './SoundPanel'
import PartyButton from './PartyButton'
import { KAR_PARTS } from './stemMix'
import type { PlayerView } from './types'
import { placeIn, useMusicLinks } from '../pages/music/links'
import { acceptedFlashNotice, shaderEffects, shaderSession, trackingMedia, VisualizerView, type ShaderEffect } from './visualizer'
import { num, t, tk, tn, tx } from '../i18n'
import './nowplaying.css'

const Karaoke = lazy(() => import('./Karaoke'))

// The full-screen player. Artwork with its reflection and a visualizer behind it, the
// transport, and from here everything else: lyrics, karaoke, the queue, the sound. It looks
// the same in every theme because it sits on black, whatever the page behind it is doing.

// karArt: the album's cover beside the words in karaoke (off unless asked for). karHide: the karaoke controls hidden.
type Prefs = { viz: boolean; canvas: boolean; style: string; slow: boolean; shuffleEvery: number; shufflePool: 'all' | 'mine'; vizArt: VizArt; vizText: VizText; fade: boolean; karArt: boolean; karHide: string[]; look: Look; songLooks: Record<string, SongLook> }
// Where the album's cover sits while the visualizer plays: hidden, or in one of these places.
// And where the song's name and the controls sit over it.
type VizText = 'top' | 'middle' | 'bottom'
const VIZ_TEXT: { id: VizText; name: string }[] = [{ id: 'top', name: tk('Top') }, { id: 'middle', name: tk('Middle') }, { id: 'bottom', name: tk('Bottom') }]
type VizArt = 'off' | 'centre' | 'left' | 'right' | 'corner' | 'canvas'
const VIZ_ART: { id: VizArt; name: string }[] = [{ id: 'off', name: tk('Hidden') }, { id: 'centre', name: tk('In the middle') }, { id: 'left', name: tk('On the left') }, { id: 'right', name: tk('On the right') }, { id: 'corner', name: tk('Small, in the corner') }, { id: 'canvas', name: tk('Canvas video, in the middle') }]
// The visualizer is off until someone turns it on (docs/plans/DESIGN-2.md section 10).
const DEFAULT_PREFS: Prefs = { viz: false, canvas: true, style: 'bars', slow: false, shuffleEvery: 0, shufflePool: 'mine', vizArt: 'off', vizText: 'middle', fade: false, karArt: false, karHide: [], look: DEFAULT_LOOK, songLooks: {} }
const SHUFFLE_CHOICES = [0, 15, 30, 60, 120, 300, 600]
const everyLabel = (s: number) => (s === 0 ? t('Off: stay on this one') : s < 60 ? tn('Every {count} second', 'Every {count} seconds', s) : s === 60 ? t('Every minute') : tn('Every {count} minute', 'Every {count} minutes', s / 60))
// The same, short, for the chip.
const everyShort = (s: number) => (s < 60 ? tn('{count} second', '{count} seconds', s) : s === 60 ? t('minute') : tn('{count} minute', '{count} minutes', s / 60))

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
    viz: s.viz === true, canvas: s.canvas !== false, style: typeof s.style === 'string' && s.style.length <= 48 ? s.style : 'bars', slow: s.slow === true,
    shuffleEvery: SHUFFLE_CHOICES.includes(s.shuffleEvery as number) ? (s.shuffleEvery as number) : 0, shufflePool: s.shufflePool === 'all' ? 'all' : 'mine',
    vizArt: VIZ_ART.some((a) => a.id === s.vizArt) ? (s.vizArt as VizArt) : 'off',
    vizText: VIZ_TEXT.some((a) => a.id === s.vizText) ? (s.vizText as VizText) : 'middle',
    fade: s.fade === true,
    karArt: s.karArt === true,
    karHide: Array.isArray(s.karHide) ? KAR_PARTS.map((k) => k.id).filter((id) => (s.karHide as unknown[]).includes(id)) : [],
    look: readLook(s.look), songLooks: readSongLooks(s.songLooks),
  }
}

const isNative = (style: string): style is NativeStyle => NATIVE_STYLES.some((n) => n.id === style)

type Panel = '' | 'lyrics' | 'queue' | 'sound' | 'read' | 'sleep' | 'look'

// The Canvas studio opens on asking (an administrator's tool).
const CanvasStudio = lazy(() => import('../canvas/CanvasManager'))
// The speeds offered, here and in More on a phone.
const SPEEDS_NP = [0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3]

export default function NowPlaying({ p }: { p: PlayerView }) {
  const phone = useMedia('(max-width: 700px)')
  const it = p.current!
  const root = useRef<HTMLDivElement>(null)
  const [prefs, setPrefsState] = useState<Prefs>(DEFAULT_PREFS)
  const [prefsLoaded, setPrefsLoaded] = useState(false)
  const [panel, setPanel] = useState<Panel>(p.fullOpts.panel === 'lyrics' || p.fullOpts.panel === 'queue' ? p.fullOpts.panel : '')
  const [karaoke, setKaraoke] = useState(false)
  const [shown, setShown] = useState('') // what shuffle is showing now, if it's on
  const [shaders, setShaders] = useState<ShaderEffect[]>([])
  const [wholeShader, setWholeShader] = useState(false)
  // The effect presets and their sliders open on asking, so the visualizer isn't buried under them.
  const [effectsOpen, setEffectsOpen] = useState(false)
  // Said only when someone asks for the Canvas video of a song that has none, then gone again.
  const [noCanvasSaid, setNoCanvasSaid] = useState(false)
  useEffect(() => { if (!noCanvasSaid) return; const t = setTimeout(() => setNoCanvasSaid(false), 6000); return () => clearTimeout(t) }, [noCanvasSaid])
  const [said, setSaid] = useState('')
  const artUrl = it.poster ? posterUrl(it) : ''
  const rgb = useArtColour(artUrl)
  const trackMedia = useMemo(() => trackingMedia(p.media), [p.media])
  const canSing = p.mode === 'music' && !p.cdActive
  const extras = useExtras(canSing ? it.id : undefined)
  const canvasInfo = useCanvas(canSing ? it.id : undefined)
  const hasCanvas = !!canvasInfo.info?.available
  const { user } = useApp()
  const admin = !!user?.isAdmin
  const lyricsWanted = panel === 'lyrics' || panel === 'read' || karaoke
  // Lyrics or Read open: the one place for the words. Opening it again goes back to the way it was last read.
  const words = panel === 'lyrics' || panel === 'read'
  const [lastWords, setLastWords] = useState<'lyrics' | 'read'>('lyrics')
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
  const needShaders = prefsLoaded && prefs.viz && p.vizHere
  const loadShaders = useCallback(() => { shaderEffects().then(setShaders).catch(() => {}) }, [])
  useEffect(() => { if (needShaders) loadShaders() }, [needShaders, loadShaders])
  useEffect(() => { root.current?.focus({ preventScroll: true }) }, [])
  // With the visualizer behind it and Fade the controls chosen (Layout; off unless asked for), the controls fade after a few quiet seconds and come
  // back with any movement, key or focus, as a video player's do. Never while someone is using the controls: a menu open (they open outside the
  // player), a control with the keyboard's focus, or the pointer resting on them. Karaoke's top row does the same (idle.ts).
  const quietable = prefs.viz && prefs.fade && prefsLoaded && !panel && !karaoke && !effectsOpen && !wholeShader
  const idle = useIdleHide(root, quietable, { busy: () => !!document.querySelector('.mp-pop-body') || !!root.current?.querySelector(':is(.np-top, .np-seek, .np-transport, .np-tools, .np-hints):hover') })
  useEffect(() => {
    const o = p.fullOpts.panel
    if (o === 'karaoke') { if (canSing) setKaraoke(true) } else if (o) setPanel(o)
  }, [p.fullOpts]) // eslint-disable-line react-hooks/exhaustive-deps

  const style = shown || prefs.style

  // The look (look.ts): everyone's, with this song's own over it. The bar and karaoke read the same.
  useEffect(() => { if (prefsLoaded) publishLook({ look: prefs.look, songs: prefs.songLooks, canvas: prefs.canvas, viz: prefs.viz }) }, [prefsLoaded, prefs.look, prefs.songLooks, prefs.canvas, prefs.viz])
  const look = lookFor(prefs.look, prefs.songLooks, it.id)
  const canvasWanted = look.show ? look.show === 'canvas' : prefs.canvas
  const mode: LookMode = prefs.viz ? 'viz' : canSing && canvasWanted && hasCanvas ? 'canvas' : 'art'
  const canvasBehind = mode === 'canvas' && look.canvasPlace !== 'art'
  const canvasInArt = mode === 'canvas' && look.canvasPlace !== 'behind'
  const [lookScope, setLookScope] = useState<'all' | 'song'>('all')
  useEffect(() => { setLookScope(look.own ? 'song' : 'all') }, [it.id, prefsLoaded]) // eslint-disable-line react-hooks/exhaustive-deps
  const changeLook = (patch: SongLook) => {
    if (lookScope === 'song') setPrefs({ songLooks: setSongLook(prefs.songLooks, it.id, patch) })
    else setPrefs({ look: { ...prefs.look, ...patch } })
  }
  const changeMode = (m: LookMode) => {
    setNoCanvasSaid(false)
    if (m === 'viz') { setPrefs({ viz: true }); return }
    if (lookScope === 'song') setPrefs({ viz: false, songLooks: setSongLook(prefs.songLooks, it.id, { show: m }) })
    else setPrefs({ viz: false, canvas: m === 'canvas' })
  }
  const [studio, setStudio] = useState(false)
  // An advanced setting asked for on a phone, tablet or TV: where to make it instead (ui/advanced.tsx).
  const [walled, setWalled] = useState('')

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

  // Karaoke covers the player: a menu left open under it would sit over the words, so any open menu closes.
  useEffect(() => { if (karaoke) document.body.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true })) }, [karaoke])
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
        case 'l': if (canSing) { consume(); setPanel(words ? '' : lastWords) } return
        case 'k': if (canSing) { consume(); setKaraoke(true) } return
        case 'q': consume(); setPanel(panel === 'queue' ? '' : 'queue'); return
        case 'm': consume(); p.setSound({ muted: !p.sound.muted }); return
        case 's': if (!p.books && !p.live) { consume(); p.setShuffle(!p.shuffle) } return
        case 'r': if (!p.books && !p.live) { consume(); p.cycleRepeat() } return
        case 'v': consume(); setPrefs({ viz: !prefs.viz }); return
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
    if (name) setSaid(t('Visualizer: {name}', { name: tx(name) }))
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
  // The Canvas video in the cover's place, the visualizer around it; a song with none shows its cover there.
  const framedCanvas = prefs.viz && prefs.vizArt === 'canvas' && canSing && hasCanvas
  const behindChoice = (
    <Segmented label={t('Behind the player')} value={mode}
                  onChange={(v) => { if (v === 'canvas' && !hasCanvas) { setNoCanvasSaid(true); return } changeMode(v as LookMode) }}
                  options={[{ value: 'art', label: t('Album art') }, ...(canSing ? [{ value: 'canvas', label: t('Canvas video') }] : []), { value: 'viz', label: t('Visualizer'), icon: <IconVisualizer size={16} /> }]} />
  )
  // The visualizer's styles: Bars, Waves, Drift and More, here and in karaoke, one setting for both. Karaoke leaves out
  // the whole-screen effect settings, which would open under it.
  const stylePicker = (inKaraoke: boolean) => (
    <div className="np-styles" role="radiogroup" aria-label={t('Visualizer style')}>
      {NATIVE_STYLES.map((n) => <button type="button" role="radio" key={n.id} aria-checked={style === n.id} className={style === n.id ? 'on' : ''} onClick={() => pickStyle(n.id)}>{tx(n.name)}</button>)}
      {p.vizHere && <Pop label={t('More visualizer styles')} className={'np-more-styles' + (!isNative(style) ? ' on' : '')} icon={<span>{isNative(style) ? t('More') : tx(shaders.find((s) => s.id === style)?.name) || t('More')}</span>}>
        {(close) => (
          <div className="mp-menu np-style-menu" role="menu" ref={(el) => { if (el && !shaders.length) loadShaders() }}>
            {!shaders.length && <span className="mp-menu-note">{t('Loading…')}</span>}
            {shaders.map((s) => <button type="button" role="menuitemradio" aria-checked={style === s.id} key={s.id} className={style === s.id ? 'on' : ''} onClick={() => { pickStyle(s.id); close() }}>{tx(s.name)}</button>)}
            {!inKaraoke && <span className="mp-menu-rule" />}
            {!inKaraoke && <button type="button" role="menuitem" onClick={() => { setWholeShader(true); close() }}>{t('Effect settings and your own effects…')}</button>}
          </div>
        )}
      </Pop>}
    </div>
  )
  // The words: in time (Lyrics) or to read (Read, with the album's booklet). One switch, at the top of either panel.
  const wordsSwitch = (
    <Segmented className="np-words-seg" label={t('The words')} value={panel === 'read' ? 'read' : 'lyrics'} onChange={(v) => { setPanel(v); setLastWords(v) }}
      options={[{ value: 'lyrics', label: <><IconMic size={16} /> {t('Follow along')}</> },
        { value: 'read', label: <><IconReadOpen size={16} /> {t('Read')}{extras.length > 0 && <span className="np-count">{extras.length}</span>}</> }]} />
  )
  const karaokeBehind = prefs.viz
    ? (isNative(style) || p.vizHere) ? <NpBackground style={style} slow={prefs.slow} rgb={rgb} playing={p.playing} media={p.media} trackMedia={trackMedia} onDecline={() => { setShown(''); setPrefs({ style: 'bars' }) }} /> : null
    : canvasBehind ? <NowPlayingCanvas itemId={it.id} playing={p.playing} />
      : artUrl && look.backdrop !== 'plain' ? <div className={'kar-art' + (look.backdrop === 'blur' || look.backdrop === 'glow' ? ' blur' : '')} aria-hidden="true" style={{ backgroundImage: `url(${artUrl})` }} /> : null
  // The cover beside the words, in the same look as here, the Canvas in its place when it is there.
  const karCover = artUrl ? <ArtView look={look} src={artUrl} caption={it.title} playing={p.playing} media={p.media} size="kar" canvas={canvasInArt ? { info: canvasInfo.info, blocked: canvasInfo.blocked } : null} /> : null
  const subtitle = p.live ? (p.retrying ? t('Reconnecting…') : p.stationNow || t('Live')) : it.artist || it.albumArtist || ''
  const placeLine = p.books ? t('Chapters') : p.live ? t('Stations') : t('Queue')

  return (
    <div className={'np' + (mode === 'art' && look.backdrop !== 'glow' ? ' backdrop-plain' : '') + (panel ? ' has-panel panel-' + panel : '') + (prefs.viz && prefsLoaded ? ' viz-on' : '') + (idle && quietable ? ' np-idle' : '') + (prefs.viz && prefsLoaded ? ' viz-text-' + prefs.vizText : '') + (prefs.viz && prefsLoaded && prefs.vizArt !== 'off' && artUrl ? ' viz-art-' + (prefs.vizArt === 'canvas' ? 'centre' : prefs.vizArt) : '')} ref={root} tabIndex={-1} role="dialog" aria-modal="true" aria-label={t('Now playing: {title}', { title: it.title })}
      style={{ '--art': rgb } as React.CSSProperties} onClickCapture={(e) => { if (e.detail > 0) requestAnimationFrame(() => { if (!document.activeElement?.closest('input, select, textarea, .mp-pop-body, .np-panel')) root.current?.focus({ preventScroll: true }) }) }}>
      <div className="np-glow" aria-hidden="true" />
      {mode === 'art' && artUrl && !p.live && (look.backdrop === 'blur' || look.backdrop === 'cover') && <div className={'np-backdrop ' + look.backdrop} style={{ backgroundImage: `url("${artUrl}")` }} aria-hidden="true" />}
      {canvasBehind && <NowPlayingCanvas itemId={it.id} playing={p.playing} />}
      {/* One canvas: it goes to the whole-screen view or to karaoke while that's open, and comes back after
          (two at once shared one effect session, and closing karaoke ended it for the player too). */}
      {prefsLoaded && prefs.viz && !wholeShader && !karaoke && (isNative(style) || p.vizHere) && <NpBackground style={style} slow={prefs.slow} rgb={rgb} playing={p.playing} media={p.media} trackMedia={trackMedia} onDecline={() => { setShown(''); setPrefs({ style: 'bars' }) }} />}
      {wholeShader && <VisualizerView media={trackMedia} place="full" onPlace={(pl) => { if (pl !== 'full') leaveWhole() }} onClose={leaveWhole} />}

      <header className="np-top">
        <button type="button" className="mp-ibtn np-close" onClick={p.closeFull} aria-label={t('Close the full player (Esc)')} title={t('Close (Esc)')}><IconDown /></button>
        <div className="np-where">
          <span className="np-label">{p.live ? t('Live now') : p.books ? (p.mode === 'podcasts' ? t('Podcast') : t('Audiobook')) : t('Now playing')}</span>
          <span className="np-where-sub">{p.live ? it.title : it.album || subtitle}</span>
        </div>
        <div className="np-top-right">
          {/* A phone keeps the top row to what's reached for while listening: its own buttons do the volume, the
              player already fills the screen, and the sleep timer and speed are in More. */}
          {!phone && <Volume p={p} />}
          {!p.live && <AddToPlaylist items={[it]} />}
          {!phone && <button type="button" className={'mp-ibtn' + (panel === 'sleep' || p.sleepAt !== null ? ' on' : '')} aria-pressed={panel === 'sleep'} aria-label={p.sleepAt !== null ? t('Sleep timer: on') : t('Sleep timer')} title={t('Sleep timer')} onClick={() => setPanel(panel === 'sleep' ? '' : 'sleep')}><IconMoon /></button>}
          <button type="button" className={'mp-ibtn' + (panel === 'queue' ? ' on' : '')} aria-pressed={panel === 'queue'} aria-label={placeLine} title={t('{name} (Q)', { name: placeLine })} onClick={() => setPanel(panel === 'queue' ? '' : 'queue')}>{p.books ? <IconChapters /> : <IconList />}</button>
          <Like p={p} />
          <PartyButton p={p} />
          <AudioPlayOn p={p} />
          {!phone && <button type="button" className="mp-ibtn np-fill" onClick={toggleFullscreen} aria-label={t('Fill the screen (F)')} title={t('Fill the screen (F)')}><IconFull /></button>}
          <Pop label={t('More')} icon={<IconMore />}>
            {(close) => (
              <div className="mp-menu" role="menu">
                {phone && <button type="button" role="menuitem" onClick={() => { setPanel(panel === 'sleep' ? '' : 'sleep'); close() }}><IconMoon size={18} /> {p.sleepAt !== null ? t('Sleep timer: on') : t('Sleep timer')}</button>}
                {!p.live && !p.cdActive && (
                  <div className="np-menu-speed" role="group" aria-label={t('Playback speed')}>
                    <span className="mp-menu-note">{t('Speed')}</span>
                    <div className="np-menu-speeds">
                      {SPEEDS_NP.filter((r) => r <= p.maxSpeed).map((r) => <button type="button" key={r} role="menuitemradio" aria-checked={p.rate === r} className={p.rate === r ? 'on' : ''} onClick={() => p.changeRate(r)}>{num(r)}×</button>)}
                    </div>
                  </div>
                )}
                <button type="button" role="menuitem" onClick={() => { setPanel(panel === 'sound' ? '' : 'sound'); close() }}><IconSliders size={18} /> {t('Sound: equaliser and more')}</button>
                {canSing && <button type="button" role="menuitem" onClick={() => { p.startRadio(it); close() }}><IconStation size={18} /> {t('Start a station from this song')}</button>}
                {p.albumUrl && <button type="button" role="menuitem" onClick={() => p.goTowards(p.albumUrl)}>{p.live ? t('Go to station') : p.books ? t('Go to book') : t('Go to album')}</button>}
                {p.artistUrl && <button type="button" role="menuitem" onClick={() => p.goTowards(p.artistUrl)}>{t('Go to artist')}</button>}
                {video && <button type="button" role="menuitem" onClick={() => { close(); watchVideo() }}>{t('Watch the video')}</button>}
                <button type="button" role="menuitem" onClick={p.stop}><IconClose size={18} /> {t('Stop and close')}</button>
              </div>
            )}
          </Pop>
        </div>
      </header>

      <div className="np-stage">
        <div className="np-main">
          <div className="np-artbox">
            {framedCanvas ? <div className="np-art np-art-wide"><NowPlayingCanvas itemId={it.id} playing={p.playing} framed /></div>
              : p.live ? <div className="np-art" style={{ backgroundSize: 'contain' }}>{artUrl ? <img src={artUrl} alt={t('Cover of {title}', { title: it.title })} style={{ objectFit: 'contain' }} /> : <Placeholder title={it.title} />}</div>
              : <ArtView look={look} src={artUrl} alt={t('Cover of {title}', { title: it.album || it.title })} caption={it.title} playing={p.playing} media={p.media}
                  canvas={canvasInArt ? { info: canvasInfo.info, blocked: canvasInfo.blocked } : null} fallback={<Placeholder title={it.title} />} />}
            {artUrl && !p.live && !framedCanvas && !canvasInArt && look.reflection && (look.art === 'square' || look.art === 'drift' || look.art === 'tilt') && <div className="np-reflect" aria-hidden="true"><img src={artUrl} alt="" /></div>}
          </div>
          <div className="np-title">
            <h2 className="ab-title">{it.title}</h2>
            {p.artistUrl && !p.live
              ? <button type="button" className="np-artist" onClick={() => p.goTowards(p.artistUrl)}>{subtitle}</button>
              : <p className="np-artist plain">{subtitle}</p>}
            {video && <button type="button" className="np-chip np-video" onClick={watchVideo} title={t('Watch the video, from here when the two line up')}><IconPlay size={16} /> {t('Watch the video')}</button>}
          </div>
          {!p.live && (
            <div className="np-seek">
              <Seek p={p} />
              <div className="np-times"><span>{fmtTime(p.time)}</span><span>{fmtTime(p.duration)}</span></div>
            </div>
          )}
          <div className="np-transport">
            {!p.books && !p.live && <button type="button" className={'mp-ibtn tall' + (p.shuffle ? ' on' : '')} aria-pressed={p.shuffle} aria-label={t('Shuffle')} title={t('Shuffle (S)')} onClick={() => p.setShuffle(!p.shuffle)}><IconShuffle /></button>}
            {p.books && <button type="button" className="mp-ibtn tall" onClick={() => p.seek(Math.max(0, p.position() - 15))} aria-label={t('Back 15 seconds')}><IconBack15 /></button>}
            <button type="button" className="mp-ibtn taller" onClick={p.prev} aria-label={t('Previous')}><IconPrev /></button>
            <button type="button" className="np-play" onClick={p.toggle} aria-label={p.playing ? t('Pause') : t('Play')}>{p.playing ? <IconPause size={36} /> : <IconPlay size={36} />}</button>
            <button type="button" className="mp-ibtn taller" onClick={p.next} aria-label={t('Next')}><IconNext /></button>
            {p.books && <button type="button" className="mp-ibtn tall" onClick={() => p.seek(p.position() + 30)} aria-label={t('Forward 30 seconds')}><IconForward30 /></button>}
            {!p.books && !p.live && <button type="button" className={'mp-ibtn tall' + (p.repeat !== 'off' ? ' on' : '')} aria-label={p.repeat === 'one' ? t('Repeat this song') : p.repeat === 'all' ? t('Repeat all') : t('Repeat off')} title={t('Repeat (R)')} onClick={p.cycleRepeat}>{p.repeat === 'one' ? <IconRepeatOne /> : <IconRepeat />}</button>}
          </div>

          <div className="np-tools">
            {!p.live && (
              <>
                {/* What is behind the player: the album's art, its Canvas video, or the visualizer. */}
                {behindChoice}
                {prefs.viz && stylePicker(false)}
                {prefs.viz && p.vizHere && shaders.length > 0 && <button type="button" className={'np-chip' + (effectsOpen ? ' on' : '')} aria-expanded={effectsOpen} title={t('Effects and their settings')} onClick={() => (advancedHere() ? setEffectsOpen(!effectsOpen) : setWalled('Visualizer effects'))}><IconVisualizer size={18} /> {t('Effects')}</button>}
                {prefs.viz && p.vizHere && shaders.length > 0 && effectsOpen && <EffectChips effects={shaders} current={style} onPick={pickStyle} />}
                {prefs.viz && <button type="button" className={'np-chip slow-chip' + (prefs.slow ? ' on' : '')} aria-pressed={prefs.slow} title={t('A slower, gentler pace')} onClick={() => setPrefs({ slow: !prefs.slow })}><IconTurtle size={20} /> {t('Slow')}</button>}
                {prefs.viz && (
                  <Pop label={t('Layout: cover {cover}, words {words}', { cover: tx(VIZ_ART.find((a) => a.id === prefs.vizArt)?.name).toLowerCase(), words: tx(VIZ_TEXT.find((a) => a.id === prefs.vizText)?.name).toLowerCase() })} className={'np-chip-pop' + (prefs.vizArt !== 'off' || prefs.vizText !== 'middle' ? ' on' : '')} icon={<><IconImage size={18} /><span>{t('Layout')}</span></>}>
                    {(close) => (
                      <div className="mp-menu np-style-menu" role="menu">
                        {artUrl && <>
                          <span className="mp-menu-note">{t('The album’s cover')}</span>
                          {/* Beside the name needs a stage 700px wide (nowplaying.css): a phone has the choice greyed, and says why. */}
                          {VIZ_ART.filter((a) => a.id !== 'canvas' || (canSing && hasCanvas)).map((a) => {
                            const narrow = phone && (a.id === 'left' || a.id === 'right')
                            return <button type="button" key={a.id} role="menuitemradio" aria-checked={prefs.vizArt === a.id} aria-disabled={narrow || undefined} className={(prefs.vizArt === a.id ? 'on' : '') + (narrow ? ' np-menu-off' : '')} onClick={() => { if (narrow) return; setPrefs({ vizArt: a.id }); close() }}>{tx(a.name)}</button>
                          })}
                          {phone && <span className="mp-menu-note">{t('On the left and On the right need a wider screen. Here the cover stays above the song’s name.')}</span>}
                          <span className="mp-menu-rule" />
                        </>}
                        <span className="mp-menu-note">{t('The song’s name and the controls')}</span>
                        {VIZ_TEXT.map((a) => <button type="button" key={a.id} role="menuitemradio" aria-checked={prefs.vizText === a.id} className={prefs.vizText === a.id ? 'on' : ''} onClick={() => { setPrefs({ vizText: a.id }); close() }}>{tx(a.name)}</button>)}
                        <span className="mp-menu-rule" />
                        <button type="button" role="menuitemcheckbox" aria-checked={prefs.fade} className={prefs.fade ? 'on' : ''} onClick={() => { setPrefs({ fade: !prefs.fade }); close() }}>{t('Fade the controls when nothing is touched')}</button>
                      </div>
                    )}
                  </Pop>
                )}
                {prefs.viz && (
                  <Pop label={prefs.shuffleEvery ? t('Shuffling visualizers: {every}', { every: everyLabel(prefs.shuffleEvery) }) : t('Shuffle visualizers')} className={'np-chip-pop' + (prefs.shuffleEvery ? ' on' : '')} icon={<><IconShuffle size={18} /><span>{prefs.shuffleEvery ? everyShort(prefs.shuffleEvery) : t('Styles')}</span></>}>
                    {(close) => (
                      <div className="mp-menu np-style-menu">
                        <span className="mp-menu-note">{t('Change style by itself. It stays still unless you choose this.')}</span>
                        {SHUFFLE_CHOICES.map((c) => <button type="button" key={c} role="menuitemradio" aria-checked={prefs.shuffleEvery === c} className={prefs.shuffleEvery === c ? 'on' : ''} onClick={() => { setPrefs({ shuffleEvery: c }); close() }}>{everyLabel(c)}</button>)}
                        <span className="mp-menu-rule" />
                        <button type="button" role="menuitemradio" aria-checked={prefs.shufflePool === 'mine'} className={prefs.shufflePool === 'mine' ? 'on' : ''} onClick={() => setPrefs({ shufflePool: 'mine' })}>{t('Bars, Waves and Drift')}</button>
                        <button type="button" role="menuitemradio" aria-checked={prefs.shufflePool === 'all'} className={prefs.shufflePool === 'all' ? 'on' : ''} onClick={() => setPrefs({ shufflePool: 'all' })}>{t('Every effect')}</button>
                      </div>
                    )}
                  </Pop>
                )}
                {canSing && !hasCanvas && noCanvasSaid && <span className="np-note-inline" role="status">{canvasInfo.blocked ? t('This song and its album have no Canvas video to show on this device.') : t('This song and its album have no Canvas video.')}</span>}
                <span className="np-tools-rule" aria-hidden="true" />
              </>
            )}
            {!p.live && <button type="button" className={'np-chip' + (panel === 'look' ? ' on' : '')} aria-pressed={panel === 'look'} title={t("How the cover looks, and what's behind it")} onClick={() => setPanel(panel === 'look' ? '' : 'look')}><IconPalette size={20} /> {t('Look')}</button>}
            {/* Lyrics and Read are one place: the words in time, or to read (with the album's booklet), a switch at its top. */}
            {canSing && <button type="button" className={'np-chip' + (words ? ' on' : '')} aria-pressed={words} title={t('Lyrics (L): in time with the song, or to read')} onClick={() => setPanel(words ? '' : lastWords)}><IconMic size={20} /> {t('Lyrics')}{extras.length > 0 && <span className="np-count" title={tn('{count} in the booklet', '{count} in the booklet', extras.length)}>{extras.length}</span>}</button>}
            {canSing && <button type="button" className="np-chip" title={t('Karaoke (K)')} onClick={() => setKaraoke(true)}><IconKaraoke size={20} /> {t('Karaoke')}</button>}
            {/* Speed is chosen in More; away from normal, it says so here, and a press goes back to normal. */}
            {!p.live && p.rate !== 1 && <button type="button" className="np-chip on" title={t('Playing faster or slower: press for normal speed')} onClick={() => p.changeRate(1)}>{t('{rate}× speed', { rate: num(p.rate) })}</button>}
          </div>
        </div>

        {panel && (
          <aside className="np-panel" aria-label={panel === 'lyrics' ? t('Lyrics') : panel === 'queue' ? placeLine : panel === 'sound' ? t('Sound') : panel === 'sleep' ? t('Sleep timer') : panel === 'look' ? t('Look') : t('Read')}>
            {panel === 'lyrics' && <LyricsPanel p={p} ly={ly} switcher={wordsSwitch} onClose={() => setPanel('')} />}
            {panel === 'queue' && <QueuePanel p={p} heading={placeLine} onClose={() => setPanel('')} />}
            {panel === 'sound' && <SoundPanel p={p} onClose={() => setPanel('')} />}
            {panel === 'sleep' && <SleepPanel p={p} onClose={() => setPanel('')} />}
            {panel === 'read' && <ReadPanel itemId={it.id} extras={extras} ly={ly} switcher={wordsSwitch} onClose={() => setPanel('')} />}
            {panel === 'look' && (
              <div className="np-side">
                <div className="np-side-head">
                  <h3>{t('Look')}</h3>
                  <button type="button" className="mp-ibtn" onClick={() => setPanel('')} aria-label={t('Close the look')}><IconClose /></button>
                </div>
                <LookPanel look={look} scope={lookScope} onScope={setLookScope} own={look.own} onReset={() => setPrefs({ songLooks: setSongLook(prefs.songLooks, it.id, null) })}
                  onLook={changeLook} mode={mode} onMode={changeMode} hasCanvas={hasCanvas} canCanvas={canSing} artUrl={artUrl} title={it.title} admin={admin} onStudio={admin && canSing ? () => setStudio(true) : undefined} />
              </div>
            )}
          </aside>
        )}
      </div>

      <p className="np-hints" aria-hidden="true">
        <kbd>Esc</kbd> {t('close')} · <kbd>{t('Space')}</kbd> {t('play or pause')} · <kbd>←</kbd> <kbd>→</kbd> {t('previous or next')}
        {canSing && <> · <kbd>L</kbd> {t('lyrics')} · <kbd>K</kbd> {t('karaoke')}</>}{!p.live && <> · <kbd>V</kbd> {t('visualizer')}</>}
      </p>
      <div className="mp-sr" aria-live="polite">{said}</div>
      {walled && <UseAComputer what={walled} onClose={() => setWalled('')} />}
      {studio && <Suspense fallback={null}><CanvasStudio itemId={it.id} title={it.title} kind="song" runtimeSec={p.duration} artist={it.artist || it.albumArtist || ''} onClose={() => setStudio(false)} /></Suspense>}
      {karaoke && <Suspense fallback={null}><Karaoke p={p} ly={ly} choice={behindChoice} styles={prefs.viz ? stylePicker(true) : null} behind={karaokeBehind} cover={artUrl} coverArt={karCover} coverBeside={prefs.karArt} onCoverBeside={(on) => setPrefs({ karArt: on })} hide={prefs.karHide} onHide={(h) => setPrefs({ karHide: h })} onClose={() => { setKaraoke(false); root.current?.focus({ preventScroll: true }) }} /></Suspense>}
    </div>
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

// All the effects at once, as the board draws them (Effects: all 18 kept, the classics and Aurora Rings), with the
// two controls that belong to them: Calm mode, and how strongly they react. They talk to the running plugin.
function EffectChips({ effects, current, onPick }: { effects: ShaderEffect[]; current: string; onPick: (id: string) => void }) {
  const s = shaderSession()
  const [calm, setCalm] = useState<boolean>(() => s?.calm ?? false)
  const [intensity, setIntensity] = useState<number>(() => s?.settings.intensity ?? 1)
  // Speed (speedPct, 50 to 200) and Colour (effect or theme) are WittyVisualizer's own settings keys, saved through its module settings.
  const extra = (s?.settings ?? {}) as unknown as { speedPct?: number; colour?: string }
  const [speed, setSpeed] = useState<number>(extra.speedPct ?? 100)
  const [colour, setColour] = useState<string>(extra.colour ?? 'effect')
  const patch = (v: Record<string, unknown>) => shaderSession()?.update(v as never)
  const [cycle, setCycle] = useState<number>(s?.settings.cycleSec ?? 0)
  const lively = effects.find((e) => e.id === current)?.motion === 'lively'
  const keeps = (e: ShaderEffect) => !calm || e.motion === 'gentle'
  return (
    <div className="np-effects" role="group" aria-label={t('Visualizer effects')}>
      {lively && !calm && <p className="np-note np-callout" role="status">{t('This effect flashes. Calm mode leaves out the lively ones and keeps everything else.')}</p>}
      <div className="np-effect-chips" role="radiogroup" aria-label={t('Effect')}>
        {effects.map((e) => (
          <button type="button" role="radio" key={e.id} aria-checked={current === e.id} disabled={!keeps(e)} className={current === e.id ? 'on' : ''} onClick={() => onPick(e.id)}>
            {tx(e.name)}<small>{e.motion === 'gentle' ? t('Gentle') : t('Lively')}</small>
          </button>
        ))}
      </div>
      <div className="np-effect-opts">
        <label className="np-range"><span>{t('Auto-change')}</span>
          <select className="select sm" value={cycle} onChange={(e) => { const v = Number(e.target.value); setCycle(v); shaderSession()?.update({ cycleSec: v }) }}>
            {[0, 10, 15, 30, 60, 120, 300].map((c) => <option key={c} value={c}>{c === 0 ? t('Off') : c < 60 ? t('Every {count} s', { count: c }) : t('Every {count} min', { count: c / 60 })}</option>)}
          </select>
        </label>
        <button type="button" className={'np-chip small' + (calm ? ' on' : '')} aria-pressed={calm} title={t('Keeps to the gentle effects and slows everything')} onClick={() => { const v = !calm; setCalm(v); shaderSession()?.update({ calm: v }) }}>{t('Calm mode')}</button>
        <label className="np-range"><span>{t('Intensity')}</span>
          <input type="range" min={30} max={150} step={5} value={Math.round(intensity * 100)} aria-valuetext={`${Math.round(intensity * 100)}%`}
            onChange={(e) => { const v = Number(e.target.value) / 100; setIntensity(v); shaderSession()?.update({ intensity: v }) }} />
          <output>{Math.round(intensity * 100)}%</output>
        </label>
        <label className="np-range"><span>{t('Speed')}</span>
          <input type="range" min={50} max={200} step={5} value={speed} aria-valuetext={`${speed}%`} onChange={(e) => { const v = Number(e.target.value); setSpeed(v); patch({ speedPct: v }) }} />
          <output>{speed}%</output>
        </label>
        <Segmented label={t('Colour')} value={colour === 'theme' ? 'theme' : 'effect'} onChange={(v) => { setColour(v); patch({ colour: v }) }}
          options={[{ value: 'effect', label: t('Effect’s own') }, { value: 'theme', label: t('Follows the theme') }]} />
      </div>
    </div>
  )
}
