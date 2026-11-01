import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { mediaUrl } from '../../api'
import { IconDown, IconFilm, IconFull, IconInfo, IconPrev, IconStar, IconTv, IconUp } from '../../icons'
import { channelVars, setLook, type Look } from './look'
import Preview from './Preview'
import { useReminders } from './reminders'
import { fmtClock, fmtDay, fmtLeft, logoSrc, type GuideChannel, type Programme } from './tvApi'

// A logo, when the channel has one, and otherwise its name as a clean text mark.
export function ChannelMark({ ch, look }: { ch: Pick<GuideChannel, 'logo' | 'name'>; look: Look }) {
  const [broken, setBroken] = useState(false)
  useEffect(() => setBroken(false), [ch.logo])
  const src = look.logos && !broken ? logoSrc(ch.logo) : ''
  if (src) return <img className="ctv-logo" src={src} alt={ch.name} loading="lazy" onError={() => setBroken(true)} />
  return <span className="ctv-name">{ch.name}</span>
}

// The top of the guide: everything about the highlighted programme. Its picture
// waits for the highlight to settle, so running down the channels loads nothing.
export default function Hero({ ch, p, next, look, now, previewKey, previewName, previewMax, onWatch, onDetails, onDay, onFav, onUp, onDown, onRecord }: {
  ch?: GuideChannel; p?: Programme; next?: Programme; look: Look; now: number; previewKey: string; previewName: string; previewMax: number
  onWatch: (key: string) => void; onDetails: () => void; onDay: () => void; onFav: () => void; onUp: () => void; onDown: () => void; onRecord: () => void
}) {
  const nav = useNavigate()
  const reminders = useReminders()
  const itemId = p?.itemId
  const [art, setArt] = useState<number | null>(null)
  useEffect(() => {
    setArt(null)
    if (!itemId || !look.art || look.layout !== 'streaming') return
    const t = setTimeout(() => setArt(itemId), 400)
    return () => clearTimeout(t)
  }, [itemId, look.art, look.layout])

  const real = !!p && p.kind !== 'none' && p.kind !== 'group'
  const live = !!p && p.start <= now && p.end > now
  const later = !!p && p.start > now
  const left = p ? p.end - Math.max(now, p.start) : 0
  const pct = p && live && p.end > p.start ? Math.max(0, Math.min(100, ((now - p.start) / (p.end - p.start)) * 100)) : 0
  const isTitle = real && !!p?.itemId
  const isFilm = isTitle && !p?.showId
  const reminded = !!p && !!ch && reminders.has(ch.key, p.start)
  const accent = ch?.accent ? channelVars(ch.accent) : undefined
  const preview = look.preview !== 'off' && <Preview channel={previewKey} label={previewName} look={look} max={previewMax} />
  return (
    <section className={'ctv-hero side-' + look.previewSide} aria-label="Highlighted programme" style={accent}>
      {art ? <img className="ctv-hero-art" src={mediaUrl(`/api/items/${art}/backdrop`)} alt="" decoding="async" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} /> : null}
      {look.previewSide === 'left' && preview}
      <div className="ctv-hero-main">
        <div className="ctv-hero-ch">
          {ch ? <>
            <b className="ctv-hero-num">{ch.number}</b>
            {look.logos && logoSrc(ch.logo) ? <ChannelMark ch={ch} look={look} /> : null}
            <span className="ctv-hero-chtext"><b>{ch.name}</b>{ch.description && <span>{ch.description}</span>}</span>
          </> : <IconTv size={22} />}
        </div>
        <h2 className="ctv-hero-title">{p?.title || ch?.name || ' '}</h2>
        <div className="ctv-hero-meta">
          {p?.sub && <span className="ctv-hero-sub">{p.sub}</span>}
          {p?.year ? <span>{p.year}</span> : null}
          {p?.genre && <span>{p.genre}</span>}
          {p?.rating && <span className="ctv-rate">{p.rating}</span>}
          {ch?.hd && <span className="ctv-rate">HD</span>}
          {ch?.atsc3 && <span className="ctv-rate">ATSC 3.0</span>}
          {ch?.security?.state === 'clear' && <span className="ctv-rate">Clear</span>}
          {ch?.security?.state === 'unknown' && <span className="ctv-rate" title={ch.security.detail}>{ch.security.label}</span>}
        </div>
        {real && p && (
          <div className="ctv-hero-when">
            <b>{later ? fmtDay(p.start) + ', ' : ''}{fmtClock(p.start)} to {fmtClock(p.end)}</b>
            {live && <>
              <span className="ctv-hero-bar" role="progressbar" aria-label="How far through" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)}><i style={{ width: pct + '%' }} /></span>
              <span>{fmtLeft(left)} left</span>
            </>}
          </div>
        )}
        {next && <div className="ctv-hero-next">Next <b>{fmtClock(next.start)}</b> {next.title}</div>}
        {p?.desc && <p className="ctv-info-desc">{p.desc}</p>}
        {ch?.security?.state === 'encrypted' && <p className="ctv-info-desc"><b>{ch.security.label}.</b> {ch.security.detail} <Link className="ctv-link" to="/tv/encrypted">What can I do?</Link></p>}
        {ch?.notices?.map((n) => <p key={n} className="ctv-info-desc">{n}</p>)}
        <div className="ctv-hero-actions">
          {ch?.alternative && <button className="ctv-btn on" onClick={() => onWatch(ch.alternative!.key)}>Watch {ch.alternative.number} {ch.alternative.name} instead</button>}
          {ch && !ch.encrypted && (later
            ? <button className={'ctv-btn' + (reminded ? '' : ' on')} aria-pressed={reminded} onClick={() => p && void reminders.toggle(ch.key, p)}>{reminded ? '✓ Reminder set' : 'Remind me'}</button>
            : <button className="ctv-btn on" onClick={() => onWatch(ch.key)}><IconFull size={16} /> <span>Watch full screen</span></button>)}
          {ch && !ch.encrypted && later && <button className="ctv-btn" onClick={() => onWatch(ch.key)}><IconFull size={16} /> <span>Watch channel</span></button>}
          {isTitle && p && <button className="ctv-btn" onClick={() => nav(`/play/${p.itemId}?mode=title&start=${p.clip?.start ? Math.floor(p.clip.start) : 0}${p.clip ? `&ttn=${p.clip.title}` : ''}`)} title="Play it from the beginning, on your own"><IconPrev size={16} /> <span>Start over</span></button>}
          {isTitle && p && <Link className="ctv-btn" to={p.showId ? `/show/${p.showId}` : `/item/${p.itemId}`}><IconFilm size={16} /> <span>{isFilm ? 'Open the film' : 'Open the show'}</span></Link>}
          {ch?.canRecord && p?.kind === 'broadcast' && p.end > now && <button className="ctv-btn" onClick={onRecord}>● Record</button>}
          {ch && <button className={'ctv-btn icon' + (ch.favourite ? ' fav' : '')} aria-pressed={!!ch.favourite} onClick={onFav} aria-label="Favourite channel" title="Favourite channel (F)"><IconStar size={17} /></button>}
          <button className="ctv-btn icon" onClick={onDown} aria-label="Channel down" title="Channel down"><IconDown size={18} /></button>
          <button className="ctv-btn icon" onClick={onUp} aria-label="Channel up" title="Channel up"><IconUp size={18} /></button>
          <button className="ctv-btn icon" onClick={onDetails} aria-haspopup="dialog" aria-label="More about this programme" title="More (I)"><IconInfo size={18} /></button>
          <button className="ctv-btn sm" onClick={onDay} aria-haspopup="dialog" title="The whole day on this channel (D)">Day</button>
          {look.preview === 'off' && <button className="ctv-btn sm" onClick={() => setLook({ preview: 'highlight' })}>Show preview</button>}
        </div>
      </div>
      {look.previewSide === 'right' && preview}
      <div className="ctv-clock" aria-hidden="true"><b>{fmtClock(now)}</b><span>{fmtDay(now)}</span></div>
    </section>
  )
}
