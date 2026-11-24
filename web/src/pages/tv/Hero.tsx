// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, mediaUrl } from '../../api'
import { IconDown, IconFilm, IconFull, IconInfo, IconPrev, IconStar, IconTv, IconUp } from '../../icons'
import { ChannelLogo } from './ChannelLogo'
import { channelVars, setLook, surfaceOf, type Look } from './look'
import Preview from './Preview'
import { useReminders } from './reminders'
import { canOfferRecord, fmtClock, fmtDay, fmtLeft, type GuideChannel, type Programme } from './tvApi'
import { SHAPES } from './tvThemes'
import { canCatchup } from './guideLogic'
import { t, tx } from '../../i18n'

// Whether a title has logo artwork, asked once per title for as long as the page is open.
const titleArtKnown = new Map<string, Promise<boolean>>()
function hasTitleArt(target: 'item' | 'show', id: number): Promise<boolean> {
  const key = target + id
  let p = titleArtKnown.get(key)
  if (!p) {
    if (titleArtKnown.size > 200) titleArtKnown.clear()
    p = api.get<{ hasLogo: boolean }>(`/meta/${target}/${id}/attached`).then((a) => !!a.hasLogo).catch(() => false)
    titleArtKnown.set(key, p)
  }
  return p
}

// The top of the guide: everything about the highlighted programme. Its picture
// waits for the highlight to settle, so running down the channels loads nothing.
export default function Hero({ ch, p, next, look, now, previewKey, previewName, previewMax, previewStart, onWatch, onDetails, onDay, onFav, onUp, onDown, onRecord, onFromStart }: {
  ch?: GuideChannel; p?: Programme; next?: Programme; look: Look; now: number; previewKey: string; previewName: string; previewMax: number; previewStart: number
  onWatch: (key: string) => void; onDetails: () => void; onDay: () => void; onFav: () => void; onUp: () => void; onDown: () => void; onRecord: () => void
  // A server that keeps an archive of a channel (tv-catchup) can play a programme from its start.
  onFromStart?: (ch: GuideChannel, p: Programme) => void
}) {
  const nav = useNavigate()
  const reminders = useReminders()
  const itemId = p?.itemId
  const artShown = look.art && SHAPES[look.shape].art
  const [art, setArt] = useState<number | null>(null)
  useEffect(() => {
    setArt(null)
    if (!itemId || !artShown) return
    const timer = setTimeout(() => setArt(itemId), 400)
    return () => clearTimeout(timer)
  }, [itemId, artShown])
  // The title's own logo, in the themes that show pictures. The classic box is all type, as
  // such boxes were, and any title without one keeps its name.
  const showId = p?.showId
  const [titleArt, setTitleArt] = useState('')
  useEffect(() => {
    setTitleArt('')
    if (!itemId || !artShown) return
    let on = true
    const target = showId ? 'show' : 'item', id = showId || itemId
    const timer = setTimeout(() => {
      void hasTitleArt(target, id).then((has) => { if (on && has) setTitleArt(mediaUrl(`/api/meta/${target}/${id}/art/logo`)) })
    }, 400)
    return () => { on = false; clearTimeout(timer) }
  }, [itemId, showId, artShown])

  const real = !!p && p.kind !== 'none' && p.kind !== 'group'
  const live = !!p && p.start <= now && p.end > now
  const later = !!p && p.start > now
  const left = p ? p.end - Math.max(now, p.start) : 0
  const pct = p && live && p.end > p.start ? Math.max(0, Math.min(100, ((now - p.start) / (p.end - p.start)) * 100)) : 0
  const isTitle = real && !!p?.itemId
  const isFilm = isTitle && !p?.showId
  const reminded = !!p && !!ch && reminders.has(ch.key, p.start)
  const accent = ch?.accent ? channelVars(ch.accent, look.bg) : undefined
  const preview = look.preview !== 'off' && <Preview channel={previewKey} label={previewName} look={look} max={previewMax} start={previewStart} />
  return (
    <section className={'ctv-hero side-' + look.previewSide} aria-label={t('Highlighted programme')} style={accent}>
      {art ? <img className="ctv-hero-art" src={mediaUrl(`/api/items/${art}/backdrop`)} alt="" decoding="async" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} /> : null}
      {look.previewSide === 'left' && preview}
      <div className="ctv-hero-main">
        <div className="ctv-hero-ch">
          {ch ? <>
            <b className="ctv-hero-num">{ch.number}</b>
            <ChannelLogo ch={ch} look={look} on={surfaceOf(look)} />
            <span className="ctv-hero-chtext"><b>{ch.name}</b>{ch.description && <span>{ch.description}</span>}</span>
          </> : <IconTv size={22} />}
        </div>
        {titleArt && p
          ? <h2 className="ctv-hero-title has-art" aria-label={p.title}><img src={titleArt} alt="" onError={() => setTitleArt('')} /></h2>
          : <h2 className="ctv-hero-title">{p?.title || ch?.name || ' '}</h2>}
        <div className="ctv-hero-meta">
          {p?.sub && <span className="ctv-hero-sub">{p.sub}</span>}
          {p?.year ? <span>{p.year}</span> : null}
          {p?.genre && <span>{p.genre}</span>}
          {p?.rating && <span className="ctv-rate">{p.rating}</span>}
          {ch?.hd && <span className="ctv-rate">HD</span>}
          {ch?.atsc3 && <span className="ctv-rate">ATSC 3.0</span>}
          {ch?.security?.state === 'clear' && <span className="ctv-rate">{t('Not scrambled')}</span>}
          {ch?.security?.state === 'unknown' && <span className="ctv-rate" title={tx(ch.security.detail)}>{tx(ch.security.label)}</span>}
        </div>
        {real && p && (
          <div className="ctv-hero-when">
            <b>{later ? t('{day}, {start} to {end}', { day: fmtDay(p.start), start: fmtClock(p.start), end: fmtClock(p.end) }) : t('{start} to {end}', { start: fmtClock(p.start), end: fmtClock(p.end) })}</b>
            {live && <>
              <span className="ctv-hero-bar" role="progressbar" aria-label={t('How far through')} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)}><i style={{ width: pct + '%' }} /></span>
              <span>{t('{time} left', { time: fmtLeft(left) })}</span>
            </>}
          </div>
        )}
        {next && <div className="ctv-hero-next">{t('Next')} <b>{fmtClock(next.start)}</b> {next.title}</div>}
        {p?.desc && <p className="ctv-info-desc">{p.desc}</p>}
        {ch?.security?.state === 'encrypted' && <p className="ctv-info-desc"><b>{tx(ch.security.label)}.</b> {tx(ch.security.detail)} <Link className="ctv-link" to="/tv/encrypted">{t('What can I do?')}</Link></p>}
        {ch?.notices?.map((n) => <p key={n} className="ctv-info-desc">{tx(n)}</p>)}
        <div className="ctv-hero-actions" data-nav-group>
          {ch?.alternative && <button className="ctv-btn on" onClick={() => onWatch(ch.alternative!.key)}>{t('Watch {number} {name} instead', { number: ch.alternative.number, name: ch.alternative.name })}</button>}
          {ch && !ch.encrypted && (later
            ? <button className={'ctv-btn' + (reminded ? '' : ' on')} aria-pressed={reminded} onClick={() => p && void reminders.toggle(ch.key, p)}>{reminded ? t('✓ Reminder set') : t('Remind me')}</button>
            : <button className="ctv-btn on" data-tv-first onClick={() => onWatch(ch.key)}><IconFull size={16} /> <span>{t('Watch full screen')}</span></button>)}
          {ch && !ch.encrypted && later && <button className="ctv-btn" aria-label={t('Watch channel')} onClick={() => onWatch(ch.key)}><IconFull size={16} /> <span>{t('Watch channel')}</span></button>}
          {onFromStart && ch && p && canCatchup(ch, p, now) && <button className="ctv-btn" onClick={() => onFromStart(ch, p)} title={t('Watch this programme from its start')}><IconPrev size={16} /> <span>{t('Watch from the start')}</span></button>}
          {isTitle && p && <button className="ctv-btn" onClick={() => nav(`/play/${p.itemId}?mode=title&start=${p.clip?.start ? Math.floor(p.clip.start) : 0}${p.clip ? `&ttn=${p.clip.title}` : ''}`)} title={t('Play it from the beginning, on your own')}><IconPrev size={16} /> <span>{t('Start over')}</span></button>}
          {isTitle && p && <Link className="ctv-btn" aria-label={isFilm ? t('Open the film') : t('Open the show')} to={p.showId ? `/show/${p.showId}` : `/item/${p.itemId}`}><IconFilm size={16} /> <span>{isFilm ? t('Open the film') : t('Open the show')}</span></Link>}
          {canOfferRecord(ch, p, now) && <button className="ctv-btn" onClick={onRecord}><span aria-hidden="true">●</span> {t('Record')}</button>}
          {ch && <button className={'ctv-btn icon' + (ch.favourite ? ' fav' : '')} aria-pressed={!!ch.favourite} onClick={onFav} aria-label={t('Favourite channel')} title={t('Favourite channel (F)')}><IconStar size={17} /></button>}
          <button className="ctv-btn icon" onClick={onDown} aria-label={t('Channel down')} title={t('Channel down')}><IconDown size={18} /></button>
          <button className="ctv-btn icon" onClick={onUp} aria-label={t('Channel up')} title={t('Channel up')}><IconUp size={18} /></button>
          <button className="ctv-btn icon" onClick={onDetails} aria-haspopup="dialog" aria-label={t('More about this programme')} title={t('More (I)')}><IconInfo size={18} /></button>
          <button className="ctv-btn sm" onClick={onDay} aria-haspopup="dialog" aria-label={t('The whole day on this channel')} title={t('The whole day on this channel (D)')}>{t('Day')}</button>
          {look.preview === 'off' && <button className="ctv-btn sm" onClick={() => setLook({ preview: 'highlight' })}>{t('Show preview')}</button>}
        </div>
      </div>
      {look.previewSide === 'right' && preview}
      <div className="ctv-clock" aria-hidden="true"><b>{fmtClock(now)}</b><span>{fmtDay(now)}</span></div>
    </section>
  )
}
