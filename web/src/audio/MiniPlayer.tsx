// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { fitWidth, fmtTime, posterUrl, sized } from '../api'
import { Placeholder } from '../components/common'
import { IconAudio, IconMic, IconBack15, IconClose, IconDown, IconExpand, IconForward30, IconNext, IconPause, IconPlay, IconPrev, IconUp, IconVisualizer } from '../icons'
import { AddToPlaylist, Like, Seek, Volume } from './controls'
import { musicCatalogue } from '../sdui/catalogue'
import { IconKaraoke, IconList, IconRepeat, IconRepeatOne, IconShuffle } from './icons'
import type { PlayerView } from './types'
import { t } from '../i18n'

// The bar: floating along the bottom while you browse, with room left under the page so it
// never hides the last row. It collapses to a small button in the corner for anyone who
// wants the screen back. Everything else is one press away in the full player.

export default function MiniPlayer({ p, cdSpin }: { p: PlayerView; cdSpin: boolean }) {
  const it = p.current
  if (!it) return null
  // Which controls the bar offers is the catalogue's list (music.miniPlayer); until it is read, all of them.
  const listed = musicCatalogue()?.miniPlayer
  const wants = (id: string) => !listed || listed.includes(id)
  const sing = p.mode === 'music' && !p.cdActive && !p.live && !p.books
  const sub = p.live ? (p.retrying ? t('Reconnecting…') : p.stationNow || t('Live')) : [it.artist || it.albumArtist, it.album].filter(Boolean).join(' · ')
  const art = it.poster
    ? <img src={sized(posterUrl(it), fitWidth(64))} alt="" style={p.live ? { objectFit: 'contain' } : undefined} onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />
    : p.live ? <Placeholder title={it.title} /> : <IconAudio />
  if (p.collapsed) {
    const pct = p.duration > 0 ? (p.time / p.duration) * 100 : 0
    return (
      <div className="audio-bar collapsed" role="region" aria-label={t('Now playing')}>
        <span className="ab-ring" style={{ '--pct': pct + '%' } as React.CSSProperties} aria-hidden="true" />
        <button className="ab-art" onClick={() => p.setCollapsed(false)} aria-label={t('Show the player: {title}', { title: it.title })} title={t('Show the player')}>{art}</button>
        <button className="ab-mini-play" onClick={p.toggle} aria-label={p.playing ? t('Pause') : t('Play')}>{p.playing ? <IconPause /> : <IconPlay />}</button>
      </div>
    )
  }
  return (
    <div className={'audio-bar' + (p.live ? ' live' : '')} role="region" aria-label={t('Now playing')}>
      <button className={'ab-art' + (cdSpin ? ' cd' + (p.playing ? ' spin' : '') : '')} onClick={() => p.openFull()} aria-label={t('Show the full player')} title={t('Show the full player')}>{art}</button>
      {!p.live && <div className="ab-progress"><Seek p={p} slim /></div>}
      <button className="ab-info" onClick={() => p.openFull()} title={t('Show the full player')}>
        <div className="ab-title">{it.title}</div>
        <div className="ab-sub">{sub}</div>
      </button>
      <div className="ab-side wide">
        <Like p={p} />
        {!p.live && <AddToPlaylist items={[it]} up />}
      </div>
      <div className="ab-ctl">
        {!p.books && !p.live && <button className={'mp-ibtn wide' + (p.shuffle ? ' on' : '')} aria-pressed={p.shuffle} aria-label={t('Shuffle')} title={t('Shuffle')} onClick={() => p.setShuffle(!p.shuffle)}><IconShuffle /></button>}
        {p.books && <button className="mp-ibtn wide" onClick={() => p.seek(Math.max(0, p.position() - 15))} aria-label={t('Back 15 seconds')}><IconBack15 /></button>}
        <button className="mp-ibtn wide" onClick={p.prev} aria-label={t('Previous')} title={t('Previous')}><IconPrev /></button>
        <button className="mp-ibtn big" onClick={p.toggle} aria-label={p.playing ? t('Pause') : t('Play')}>{p.playing ? <IconPause /> : <IconPlay />}</button>
        <button className="mp-ibtn" onClick={p.next} aria-label={t('Next')} title={t('Next')}><IconNext /></button>
        {p.books && <button className="mp-ibtn wide" onClick={() => p.seek(p.position() + 30)} aria-label={t('Forward 30 seconds')}><IconForward30 /></button>}
        {!p.books && !p.live && <button className={'mp-ibtn wide' + (p.repeat !== 'off' ? ' on' : '')} aria-label={p.repeat === 'one' ? t('Repeat this song') : p.repeat === 'all' ? t('Repeat all') : t('Repeat off')} title={t('Repeat')} onClick={p.cycleRepeat}>{p.repeat === 'one' ? <IconRepeatOne /> : <IconRepeat />}</button>}
      </div>
      <div className="ab-side wide">
        {!p.live && <span className="ab-time">{fmtTime(p.time)} / {fmtTime(p.duration)}</span>}
        <Volume p={p} up />
        {sing && wants('lyrics') && <button className="mp-ibtn" onClick={() => p.openFull({ panel: 'lyrics' })} aria-label={t('Lyrics')} title={t('Lyrics (L)')}><IconMic size={20} /></button>}
        {sing && wants('karaoke') && <button className="ab-viz-pill" onClick={() => p.openFull({ panel: 'karaoke' })} title={t('Karaoke (K)')}><IconKaraoke /><span>{t('Karaoke')}</span></button>}
        {wants('queue') && <button className="mp-ibtn" onClick={() => p.openFull({ panel: 'queue' })} aria-label={t('Queue')} title={t('Queue')}><IconList /></button>}
        {!p.live && wants('visualizer') && <button className="ab-viz-pill" onClick={() => p.openFull({ visualizer: true })} title={t('Visualizer (V)')}><IconVisualizer /><span>{t('Visualizer')}</span></button>}
        <button className="mp-ibtn ab-expand" onClick={() => p.openFull()} aria-label={t('Full screen')} title={t('Full screen')}><IconExpand /></button>
        <button className="mp-ibtn" onClick={() => p.setCollapsed(true)} aria-label={t('Collapse the player')} title={t('Collapse')}><IconDown /></button>
        <button className="mp-ibtn" onClick={p.stop} aria-label={t('Stop playing and close')} title={t('Stop and close')}><IconClose /></button>
      </div>
      <button className="mp-ibtn phone-only" onClick={() => p.openFull()} aria-label={t('Full screen')}><IconUp /></button>
    </div>
  )
}
