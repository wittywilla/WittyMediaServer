// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { fitWidth, fmtTime, posterUrl, sized } from '../api'
import { Placeholder } from '../components/common'
import { IconAudio, IconMic, IconBack15, IconClose, IconDown, IconExpand, IconForward30, IconNext, IconPause, IconPlay, IconPrev, IconUp, IconVisualizer } from '../icons'
import { AddToPlaylist, Like, Seek, Volume } from './controls'
import { musicCatalogue } from '../sdui/catalogue'
import { IconKaraoke, IconList, IconRepeat, IconRepeatOne, IconShuffle } from './icons'
import type { PlayerView } from './types'
import ArtView from './ArtView'
import { lookFor, useLookState } from './look'
import { useCanvas } from '../canvas/useCanvas'

// The bar: along the bottom while you browse, in three parts as music apps lay it out: what's
// playing on the left, the buttons over the place in the song in the middle, the rest on the
// right. Room is left under the page so it never hides the last row. It collapses to a small
// button in the corner for anyone who wants the screen back. Everything else is one press
// away in the full player.

export default function MiniPlayer({ p, cdSpin }: { p: PlayerView; cdSpin: boolean }) {
  const it = p.current
  // Hooks first: the bar renders nothing with nothing playing. (sing is said again below, for the rest.)
  const singing = !!it && p.mode === 'music' && !p.cdActive && !p.live && !p.books
  const ls = useLookState()
  const look = lookFor(ls.look, ls.songs, singing ? it!.id : undefined)
  const canvasHere = singing && !ls.viz && (look.show ? look.show === 'canvas' : ls.canvas) && look.canvasPlace !== 'behind'
  const canvas = useCanvas(canvasHere ? it!.id : undefined)
  if (!it) return null
  // Which controls the bar offers is the catalogue's list (music.miniPlayer); until it is read, all of them.
  const listed = musicCatalogue()?.miniPlayer
  const wants = (id: string) => !listed || listed.includes(id)
  const sing = p.mode === 'music' && !p.cdActive && !p.live && !p.books
  const sub = p.live ? (p.retrying ? 'Reconnecting…' : p.stationNow || 'Live') : [it.artist || it.albumArtist, it.album].filter(Boolean).join(' · ')
  // A song's cover in its look (look.ts), the Canvas video in its place when that's chosen: the same as the full player.
  const art = sing && look.art !== 'square'
    ? <ArtView look={{ art: look.art, pulse: false }} src={it.poster ? sized(posterUrl(it), fitWidth(96)) : ''} size="bar" playing={p.playing}
        canvas={canvasHere && canvas.info?.available ? { info: canvas.info, blocked: canvas.blocked } : null} fallback={<IconAudio />} />
    : sing && canvasHere && canvas.info?.available && !canvas.blocked
      ? <ArtView look={{ art: 'square', pulse: false }} src={it.poster ? sized(posterUrl(it), fitWidth(96)) : ''} size="bar" playing={p.playing} canvas={{ info: canvas.info, blocked: canvas.blocked }} fallback={<IconAudio />} />
      : it.poster
        ? <img src={sized(posterUrl(it), fitWidth(64))} alt="" style={p.live ? { objectFit: 'contain' } : undefined} onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
        : p.live ? <Placeholder title={it.title} /> : <IconAudio />
  if (p.collapsed) {
    const pct = p.duration > 0 ? (p.time / p.duration) * 100 : 0
    return (
      <div className="audio-bar collapsed" role="region" aria-label="Now playing">
        <span className="ab-ring" style={{ '--pct': pct + '%' } as React.CSSProperties} aria-hidden="true" />
        <button className="ab-art" onClick={() => p.setCollapsed(false)} aria-label={`Show the player: ${it.title}`} title="Show the player">{art}</button>
        <button className="ab-mini-play" onClick={p.toggle} aria-label={p.playing ? 'Pause' : 'Play'}>{p.playing ? <IconPause /> : <IconPlay />}</button>
      </div>
    )
  }
  const dur = p.duration
  return (
    <div className={'audio-bar' + (p.live ? ' live' : '')} role="region" aria-label="Now playing">
      <div className="ab-left">
        <button className={'ab-art' + (cdSpin ? ' cd' + (p.playing ? ' spin' : '') : '')} onClick={() => p.openFull()} aria-label="Show the full player" title="Show the full player">{art}</button>
        <button className="ab-info" onClick={() => p.openFull()} title="Show the full player">
          <div className="ab-title">{it.title}</div>
          <div className="ab-sub">{sub}</div>
        </button>
        <span className="ab-side ab-like"><Like p={p} /></span>
        {!p.live && <span className="ab-side wide"><AddToPlaylist items={[it]} up /></span>}
      </div>
      <div className="ab-centre">
        <div className="ab-ctl">
          {!p.books && !p.live && <button className={'mp-ibtn wide' + (p.shuffle ? ' on' : '')} aria-pressed={p.shuffle} aria-label="Shuffle" title="Shuffle" onClick={() => p.setShuffle(!p.shuffle)}><IconShuffle /></button>}
          {p.books && <button className="mp-ibtn wide" onClick={() => p.seek(Math.max(0, p.position() - 15))} aria-label="Back 15 seconds"><IconBack15 /></button>}
          <button className="mp-ibtn wide" onClick={p.prev} aria-label="Previous" title="Previous"><IconPrev /></button>
          <button className="mp-ibtn big" onClick={p.toggle} aria-label={p.playing ? 'Pause' : 'Play'}>{p.playing ? <IconPause /> : <IconPlay />}</button>
          <button className="mp-ibtn" onClick={p.next} aria-label="Next" title="Next"><IconNext /></button>
          {p.books && <button className="mp-ibtn wide" onClick={() => p.seek(p.position() + 30)} aria-label="Forward 30 seconds"><IconForward30 /></button>}
          {!p.books && !p.live && <button className={'mp-ibtn wide' + (p.repeat !== 'off' ? ' on' : '')} aria-label={p.repeat === 'one' ? 'Repeat this song' : p.repeat === 'all' ? 'Repeat all' : 'Repeat off'} title="Repeat" onClick={p.cycleRepeat}>{p.repeat === 'one' ? <IconRepeatOne /> : <IconRepeat />}</button>}
        </div>
        {!p.live && (
          <div className="ab-progress">
            <span className="ab-time" aria-hidden="true">{fmtTime(p.time)}</span>
            <Seek p={p} slim />
            <span className="ab-time" aria-hidden="true">{fmtTime(dur)}</span>
          </div>
        )}
      </div>
      <div className="ab-right">
        <div className="ab-side wide">
          {sing && wants('lyrics') && <button className="mp-ibtn" onClick={() => p.openFull({ panel: 'lyrics' })} aria-label="Lyrics" title="Lyrics (L)"><IconMic size={20} /></button>}
          {sing && wants('karaoke') && <button className="ab-viz-pill" onClick={() => p.openFull({ panel: 'karaoke' })} title="Karaoke (K)"><IconKaraoke /><span>Karaoke</span></button>}
          {wants('queue') && <button className="mp-ibtn" onClick={() => p.openFull({ panel: 'queue' })} aria-label="Queue" title="Queue"><IconList /></button>}
          {!p.live && wants('visualizer') && <button className="ab-viz-pill" onClick={() => p.openFull({ visualizer: true })} title="Visualizer (V)"><IconVisualizer /><span>Visualizer</span></button>}
          <Volume p={p} up />
          <button className="mp-ibtn ab-expand" onClick={() => p.openFull()} aria-label="Full screen" title="Full screen"><IconExpand /></button>
          <button className="mp-ibtn" onClick={() => p.setCollapsed(true)} aria-label="Collapse the player" title="Collapse"><IconDown /></button>
          <button className="mp-ibtn" onClick={p.stop} aria-label="Stop playing and close" title="Stop and close"><IconClose /></button>
        </div>
        <button className="mp-ibtn phone-only" onClick={() => p.openFull()} aria-label="Full screen"><IconUp /></button>
      </div>
    </div>
  )
}
