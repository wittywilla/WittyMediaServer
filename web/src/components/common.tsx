// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Fragment, memo, useSyncExternalStore, type CSSProperties } from 'react'
import { brandMark, servedBrandParts, subscribeMark } from '../theme'
import { partStyle, type StyledPart } from '../brandStyle.ts'
import { avatarColor } from '../ui/avatarColor'
import { Link } from 'react-router-dom'
import { PosterCard } from '../ui/Surface'
import { brandParts, DEFAULT_NAME } from '../brandName'
import { api, artSet, CARD_SIZES, episodeLabel, fmtRuntime, discName, isBluray, isDisc, posterUrl, serverBase, showPosterUrl, sized, WIDE_CARD_SIZES, type Item, type Show } from '../api'
import { toast, useApp } from '../state'
import { IconCheck, IconLeft, IconRefresh } from '../icons'

// The shared parts, under the names pages import them by.
export { CardRow, EmptyState, LoadError, PageSkeleton, SkeletonGrid, Spinner } from '../ui/Surface'

// The way back up: the library (or show) a page belongs to.
export function BackLink({ to, label }: { to: string; label: string }) {
  return <Link to={to} className="back-link"><IconLeft size={18} /><span>{label}</span></Link>
}

// A frame from the video as a poster, the scan's or one chosen in the artwork picker: shown whole.
export const isGeneratedArt = (it: Item) => it.poster.startsWith('cache:thumb-') || it.poster.startsWith('cache:frame-')

// A colour of its own for something without a real poster: worked out from its
// title, so it never changes.
export const artHue = (title: string) => {
  let h = 0
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) % 360
  return h
}

// Artwork for something without a picture: a colour of its own, worked out from the
// title, so it never changes. On a wide screen the title is printed under the card, so it
// isn't repeated on the art; label is a short word for the art alone, such as a console's
// name. On a phone it's the title's avatar colour (the same in every app) with the title
// on it, as a poster would have (styles.css).
export function Placeholder({ title, label }: { title: string; label?: string }) {
  const h = artHue(title)
  return (
    <div className={'ph art-ph' + (label ? ' labelled' : '')} style={{ '--ph-art': `linear-gradient(155deg, hsl(${h} 32% 32%), hsl(${(h + 40) % 360} 38% 15%))`, '--ph': avatarColor(title) } as CSSProperties} aria-hidden="true">
      <b>{label || title}</b>
    </div>
  )
}

// ScanButton: look through a library's folders again, for administrators.
export function ScanButton({ lib, onStarted }: { lib: { id: number; name: string }; onStarted?: () => void }) {
  const { user } = useApp()
  if (!user?.isAdmin) return null
  const scan = async () => {
    try { await api.post(`/libraries/${lib.id}/scan`); toast(`Scanning ${lib.name}`); onStarted?.() } catch (e: any) { toast(e.message) }
  }
  return <button className="btn ghost" onClick={scan}><IconRefresh /> Scan</button>
}

// Episodes render as wide stills (16:9) with the show name; `wide` is set
// automatically for episodes. A card draws again only when its title changes: a grid
// grows a page at a time and its page re-renders while a scan counts up, and a library
// of a thousand titles would otherwise draw every card each time.
export const ItemCard = memo(function ItemCard({ item, wide = item.showId != null }: { item: Item; wide?: boolean }) {
  const pct = item.progress && item.progress.durationSec > 0 && !item.progress.watched
    ? Math.min(100, (item.progress.positionSec / item.progress.durationSec) * 100) : 0
  const gen = isGeneratedArt(item)
  const cd = item.kind === 'cd-image'
  const audio = item.kind === 'audio' || cd
  // Books count pages or chapters in runtimeSec, so their length isn't a time.
  const length = item.kind === 'book' || audio ? '' : fmtRuntime(item.runtimeSec)
  return (
    <PosterCard to={`/item/${item.id}`} name={item.title} shape={audio ? 'square' : wide ? 'wide' : 'poster'} progress={pct}
      posterClass={gen && !wide ? 'generated' : undefined}
      posterStyle={gen && !wide ? { '--art-hue': artHue(item.title) } as React.CSSProperties : undefined}
      art={item.poster ? <img src={sized(posterUrl(item), 320)} srcSet={artSet(posterUrl(item))} sizes={wide ? WIDE_CARD_SIZES : CARD_SIZES} alt="" loading="lazy" /> : <Placeholder title={item.title} />}
      badges={<>
        {(isDisc(item) || cd) && <span className={'badge disc' + (isBluray(item) ? ' bluray' : '')}>{discName(item)}</span>}
        {item.progress?.watched && <SeenBadge />}
      </>}
      title={item.showTitle && wide ? item.showTitle : item.title}
      sub={item.showId != null
        ? [episodeLabel(item), wide ? item.title : fmtRuntime(item.runtimeSec)].filter(Boolean).join(' · ')
        : audio ? item.albumArtist || item.artist || ''
        : item.kind === 'book' ? item.artist || item.year || ''
        : [item.year, length].filter(Boolean).join(' · ')} />
  )
})

// SeenBadge: the mark on a finished title.
export function SeenBadge() {
  return <span className="badge seen" role="img" aria-label="Watched"><IconCheck /></span>
}

export const ShowCard = memo(function ShowCard({ show }: { show: Show }) {
  return (
    <PosterCard to={`/show/${show.id}`} name={show.title} title={show.title}
      art={<>
        <Placeholder title={show.title} />
        {/* The server falls back to an episode still when there is no poster yet. */}
        <img src={sized(showPosterUrl(show), 320)} srcSet={artSet(showPosterUrl(show))} sizes={CARD_SIZES} alt="" loading="lazy" style={{ position: 'relative' }} onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} />
      </>}
      badges={<>
        {show.unwatched > 0 && show.unwatched < show.episodeCount && <span className="badge count" role="img" aria-label={`${show.unwatched} unwatched`}>{show.unwatched}</span>}
        {show.unwatched === 0 && <SeenBadge />}
      </>}
      sub={<span title={`${show.episodeCount} episodes`}>{[show.year, `${show.seasonCount} ${show.seasonCount === 1 ? 'season' : 'seasons'}`].filter(Boolean).join(' · ')}</span>} />
  )
})

// VersionTag is the release line shown at the bottom of every page, e.g.
// "WittyMediaServer 0.4.0 · ALPHA".
export function VersionTag({ version }: { version: string }) {
  const m = /^v?(\d+\.\d+\.\d+)(?:-([a-z]+)[.\d]*)?/i.exec(version || '')
  const num = m ? m[1] : version || 'dev'
  const stage = m?.[2] || (version === 'dev' ? 'dev' : '')
  return (
    <span className="version-tag" title={`WittyMediaServer ${version}`}>
      WittyMediaServer {num}{stage && <b className={'stage ' + stage.toLowerCase()}>{stage.toUpperCase()}</b>}
    </span>
  )
}

// logoUrl: the server's custom logo, or the app icon.
export const logoUrl = (version: string) => (version ? `${serverBase()}/api/branding/logo?v=${encodeURIComponent(version)}` : '/icon.svg')

// The badge's letter follows the name's colours: the first part that is not the plain text colour, unless it is the accent.
export const markStyle = (parts?: StyledPart[]): CSSProperties | undefined => {
  const c = parts?.find((p) => p.color && p.color.kind !== 'fg' && p.color.kind !== 'accent')?.color
  const st = partStyle(c)
  return st ? (c?.kind === 'gradient' ? st : { color: st.color }) as CSSProperties : undefined
}

// BrandMark is the picture beside the name: the custom logo or the app icon.
// src overrides it (a logo being previewed before it is saved).
// With the theme engine's mark it is a square badge with its content centred (a letter, initials, the owner's own
// characters, or a logo fitted inside); until that has arrived, or for a preview, the picture.
export function BrandMark({ src }: { src?: string }) {
  const { brandLogo } = useApp()
  const mark = useSyncExternalStore(subscribeMark, brandMark)
  if (!src && mark) {
    if (mark.kind === 'image' && mark.imageUrl) return <span className="brand-mark brand-badge image" aria-hidden="true"><img src={mark.imageUrl} alt="" onError={(e) => { e.currentTarget.replaceWith(document.createTextNode(mark.shown)) }} /></span>
    return <span className="brand-mark brand-badge" aria-hidden="true" style={markStyle(servedBrandParts())}>{mark.shown}</span>
  }
  return <img className="brand-mark" src={src || logoUrl(brandLogo)} alt="" />
}

// Brand is the name in the top-left corner: the custom name from
// Settings -> Appearance, or WittyMediaServer, with every other word in the
// theme colour (brandName.ts). colouring overrides the server's (a preview).
// Screen readers get the name once, whole, rather than in coloured pieces; the
// tooltip (for a long name cut short) is on the hidden part, so it isn't read again.
// Where it has to wrap, it wraps between the words of the name, not inside one.
export function Brand({ name, colouring, parts: given }: { name: string; colouring?: string; parts?: StyledPart[] }) {
  const { brandColouring, brandName } = useApp()
  const served = useSyncExternalStore(subscribeMark, servedBrandParts)
  const shown = name || DEFAULT_NAME
  // The server's own colouring of its name is drawn as it sent it (any colour of any part); a name or colouring being
  // tried, or a preview, is worked out here.
  const own = given ?? (colouring === undefined && (name || '') === brandName && served?.length && served.map((p) => p.text).join('') === shown ? served : undefined)
  const parts: StyledPart[] = own ?? brandParts(shown, colouring ?? brandColouring)
  const joined = (i: number) => i > 0 && !/\s$/u.test(parts[i - 1].text) && !/^\s/u.test(parts[i].text)
  return (
    <span className="brand-name">
      <span className="sr-only">{shown}</span>
      <span aria-hidden="true" title={name || undefined}>{parts.map((p, i) => {
        const style = partStyle(p.color)
        return <Fragment key={i}>{joined(i) && <wbr />}{p.accent ? <b>{p.text}</b> : style ? <span className="brand-part" style={style}>{p.text}</span> : p.text}</Fragment>
      })}</span>
    </span>
  )
}

// UpdateDot: admins see that a newer version can be installed (Settings,
// About, Updates).
export function UpdateDot() {
  const { update, user } = useApp()
  if (!update || !user?.isAdmin) return null
  return <i className="upd-dot" role="img" aria-label={`Version ${update} is available`} title={`Version ${update} is available: Settings, About`} />
}
