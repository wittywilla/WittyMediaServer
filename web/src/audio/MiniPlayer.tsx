import { fitWidth, fmtTime, posterUrl, sized } from '../api'
import { Placeholder } from '../components/common'
import { IconAudio, IconBack15, IconClose, IconDown, IconExpand, IconForward30, IconNext, IconPause, IconPlay, IconPrev, IconUp, IconVisualizer } from '../icons'
import { AddToPlaylist, Like, Seek, Volume } from './controls'
import { IconList, IconRepeat, IconRepeatOne, IconShuffle } from './icons'
import type { PlayerView } from './types'

// The bar: floating along the bottom while you browse, with room left under the page so it
// never hides the last row. It collapses to a small button in the corner for anyone who
// wants the screen back. Everything else is one press away in the full player.

export default function MiniPlayer({ p, cdSpin }: { p: PlayerView; cdSpin: boolean }) {
  const it = p.current
  if (!it) return null
  const sub = p.live ? (p.retrying ? 'Reconnecting…' : p.stationNow || 'Live') : [it.artist || it.albumArtist, it.album].filter(Boolean).join(' · ')
  const art = it.poster
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
  return (
    <div className={'audio-bar' + (p.live ? ' live' : '')} role="region" aria-label="Now playing">
      <button className={'ab-art' + (cdSpin ? ' cd' + (p.playing ? ' spin' : '') : '')} onClick={() => p.openFull()} aria-label="Show the full player" title="Show the full player">{art}</button>
      {!p.live && <div className="ab-progress"><Seek p={p} slim /></div>}
      <button className="ab-info" onClick={() => p.openFull()} title="Show the full player">
        <div className="ab-title">{it.title}</div>
        <div className="ab-sub">{sub}</div>
      </button>
      <div className="ab-side wide">
        <Like p={p} />
        {!p.live && <AddToPlaylist items={[it]} up />}
      </div>
      <div className="ab-ctl">
        {!p.books && !p.live && <button className={'mp-ibtn wide' + (p.shuffle ? ' on' : '')} aria-pressed={p.shuffle} aria-label="Shuffle" title="Shuffle" onClick={() => p.setShuffle(!p.shuffle)}><IconShuffle /></button>}
        {p.books && <button className="mp-ibtn wide" onClick={() => p.seek(Math.max(0, p.position() - 15))} aria-label="Back 15 seconds"><IconBack15 /></button>}
        <button className="mp-ibtn wide" onClick={p.prev} aria-label="Previous" title="Previous"><IconPrev /></button>
        <button className="mp-ibtn big" onClick={p.toggle} aria-label={p.playing ? 'Pause' : 'Play'}>{p.playing ? <IconPause /> : <IconPlay />}</button>
        <button className="mp-ibtn" onClick={p.next} aria-label="Next" title="Next"><IconNext /></button>
        {p.books && <button className="mp-ibtn wide" onClick={() => p.seek(p.position() + 30)} aria-label="Forward 30 seconds"><IconForward30 /></button>}
        {!p.books && !p.live && <button className={'mp-ibtn wide' + (p.repeat !== 'off' ? ' on' : '')} aria-label={p.repeat === 'one' ? 'Repeat this song' : p.repeat === 'all' ? 'Repeat all' : 'Repeat off'} title="Repeat" onClick={p.cycleRepeat}>{p.repeat === 'one' ? <IconRepeatOne /> : <IconRepeat />}</button>}
      </div>
      <div className="ab-side wide">
        {!p.live && <span className="ab-time">{fmtTime(p.time)} / {fmtTime(p.duration)}</span>}
        <Volume p={p} up />
        <button className="mp-ibtn" onClick={() => p.openFull({ panel: 'queue' })} aria-label="Queue" title="Queue"><IconList /></button>
        {p.vizHere && <button className="ab-viz-pill" onClick={() => p.openFull({ visualizer: true })} title="Visualizer (V)"><IconVisualizer /><span>Visualizer</span></button>}
        <button className="mp-ibtn ab-expand" onClick={() => p.openFull()} aria-label="Full screen" title="Full screen"><IconExpand /></button>
        <button className="mp-ibtn" onClick={() => p.setCollapsed(true)} aria-label="Collapse the player" title="Collapse"><IconDown /></button>
        <button className="mp-ibtn" onClick={p.stop} aria-label="Stop playing and close" title="Stop and close"><IconClose /></button>
      </div>
      <button className="mp-ibtn phone-only" onClick={() => p.openFull()} aria-label="Full screen"><IconUp /></button>
    </div>
  )
}
