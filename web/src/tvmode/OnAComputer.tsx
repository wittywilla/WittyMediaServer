// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '../api'
import QrCode from '../pages/settings/QrCode'
import { Button, Dialog } from '../ui'
import { IconDevices } from '../icons'
import { Brand, BrandMark } from '../components/common'
import { useApp } from '../state'
import { t, tx, bidi } from '../i18n'
import { pageAddress, type TvLevel } from './levels'
import { TV_LEVELS, tvPartLevel, tvPartTitle } from './levelTable'
import './levelWords'
import { leaveTvMode, tvClient, useTvMode } from './mode'
import './levels.css'

// What TV mode shows in place of a screen it doesn't offer (docs/TV-MODE.md). Two kinds of client:
//   a TV device (Android TV, Apple TV, Plasma Bigscreen, a TV's own browser) has nowhere else to go: the card
//     says to use the WittyMediaClient app on a computer, or a web browser, with the page's address in words
//     and as a QR code a phone can open;
//   a computer in TV mode (the desktop app started with --tv, a browser) can leave it: the card says so, and
//     its Leave TV mode button opens this same page in the normal layout, which offers the way back.
// Both keep a summary of what is set now. Nothing here is a dead end: a button has focus, and Back goes
// where the screen was opened from.

type Summary = { lines: string[]; addresses: string[] }

function useSummary(id: string | undefined): Summary {
  const [s, setS] = useState<Summary>({ lines: [], addresses: [] })
  useEffect(() => {
    let gone = false
    api.get<Summary>('/sdui/tv-summary/' + encodeURIComponent(id || 'none'))
      .then((r) => { if (!gone) setS({ lines: r.lines || [], addresses: r.addresses || [] }) })
      .catch(() => { /* the card still says where to go */ })
    return () => { gone = true }
  }, [id])
  return s
}

const addressOf = (path: string, home: string[]) => pageAddress(window.location.origin, path, home)

// tvElsewhere: a row's short label for a screen TV mode doesn't offer.
export const tvElsewhere = () => (tvClient() === 'tvDevice' ? t('On a computer') : t('Outside TV mode'))

// tvNote: the words for this client.
const TV_NOTE = () => tx(TV_LEVELS.note)
export const tvNote = () => (tvClient() === 'tvDevice' ? TV_NOTE() : TV_LEVELS.noteDesktop ? tx(TV_LEVELS.noteDesktop) : t('Leave TV mode to change this.'))

// The Leave TV mode button: the normal layout at the page that was showing.
function LeaveButton({ buttonRef, primary = true, small }: { buttonRef?: React.Ref<HTMLButtonElement>; primary?: boolean; small?: boolean }) {
  return <button ref={buttonRef} type="button" className={'btn' + (primary ? ' primary' : '') + (small ? ' sm' : '')} data-tv-first={primary || undefined} onClick={leaveTvMode}>{t('Leave TV mode')}</button>
}

// An address in words, with places to break only after each "/", never inside a name.
function Address({ text }: { text: string }) {
  const parts = text.split('/')
  return <span className="tvl-addr">{parts.map((p, i) => <span key={i}>{p}{i < parts.length - 1 && <>/<wbr /></>}</span>)}</span>
}

function SetNow({ lines, centred }: { lines: string[]; centred?: boolean }) {
  if (!lines.length) return null
  return (
    <div className={'tvl-now' + (centred ? ' tvl-now-centred' : '')}>
      <span className="tvl-now-label">{t('Set now')}</span>
      {centred ? <p className="tvl-now-line">{lines.map(bidi).join(' · ')}</p> : <ul>{lines.map((l) => <li key={l}>{l}</li>)}</ul>}
    </div>
  )
}

// OnAComputer: the whole screen's card. With level "view" it is a banner instead, and the screen's own
// content follows it, shown and not changeable.
export function OnAComputer({ level, title, path, summary, children }: { level: TvLevel; title: string; path: string; summary?: string; children?: ReactNode }) {
  const nav = useNavigate()
  const s = useSummary(summary)
  const { brandName } = useApp()
  const device = tvClient() === 'tvDevice'
  const address = addressOf(path, s.addresses)
  const first = useRef<HTMLButtonElement>(null)
  // Focus starts on the card's main button: Back on a TV device, Leave TV mode on a computer.
  useEffect(() => { first.current?.focus({ preventScroll: true }) }, [path])
  const goBack = () => {
    const idx = (window.history.state as { idx?: number } | null)?.idx
    if (idx) nav(-1)
    else nav('/you', { replace: true })
  }
  const actions = (
    <div className="tvl-actions" data-nav-group>
      {device
        ? <button ref={first} type="button" className="btn primary" data-tv-first onClick={goBack}>{t('Back')}</button>
        : <><LeaveButton buttonRef={first} /><button type="button" className="btn" onClick={goBack}>{t('Back')}</button></>}
    </div>
  )
  if (level === 'view') {
    return (
      <div className="tvl-page tvl-viewing" data-tv-client={device ? 'tv' : 'computer'}>
        <section className="tvl-banner" aria-labelledby="tvl-title">
          <span className="tvl-banner-ic" aria-hidden="true"><IconDevices /></span>
          <div className="tvl-banner-text">
            <h1 id="tvl-title" className="tvl-banner-title">{t('{title}: shown only', { title })}</h1>
            <p>{device ? <>{TV_NOTE()} <Address text={address} /></> : tvNote()}</p>
          </div>
          {device && <QrCode text={address} label={t('QR code that opens {title} on a phone or computer', { title })} size={112} />}
          {actions}
        </section>
        {children && <ViewOnly>{children}</ViewOnly>}
      </div>
    )
  }
  // The whole screen's card: one centred column as the sign-in screens ("Who's watching?") have it, for across the room.
  // The server's name small under the glow, the screen's name, one short line, the QR code as the centrepiece with
  // the address under it, what is set now, the buttons, and the server's own words at the foot. No keyboard keys.
  return (
    <div className="tvl-page tvl-centred" data-tv-client={device ? 'tv' : 'computer'}>
      <section className={'tvl-stage' + (device ? '' : ' tvl-computer')} aria-labelledby="tvl-title">
        <div className="who-brand tvl-brand"><BrandMark /><Brand name={brandName} /></div>
        <h1 id="tvl-title" className="tvl-title"><span className="tvl-title-ic" aria-hidden="true"><IconDevices /></span>{title}</h1>
        <p className="tvl-note">{device ? t('Change this from a computer or phone.') : tvNote()}</p>
        {device && (
          <figure className="tvl-qr-big">
            <QrCode text={address} label={t('QR code that opens {title} on a phone or computer', { title })} size={300} />
            <figcaption><span>{t('or open')}</span><Address text={address} /></figcaption>
          </figure>
        )}
        <SetNow lines={s.lines} centred />
        {actions}
        {device && <p className="tvl-foot">{TV_NOTE()} {t('What you change there applies on this TV too.')}</p>}
      </section>
    </div>
  )
}

// ViewOnly: a screen's content with every control switched off, and each section a stop for the
// remote, so a long page can still be read down to its end.
export function ViewOnly({ children }: { children: ReactNode }) {
  const box = useRef<HTMLFieldSetElement>(null)
  useEffect(() => {
    const el = box.current
    if (!el) return
    const mark = () => el.querySelectorAll<HTMLElement>('section, .set-card').forEach((s) => { if (!s.hasAttribute('tabindex')) s.tabIndex = 0 })
    mark()
    const watch = new MutationObserver(mark)
    watch.observe(el, { childList: true, subtree: true })
    return () => watch.disconnect()
  }, [])
  return <fieldset ref={box} className="tvl-view" disabled aria-label={t('Shown only')}>{children}</fieldset>
}

// TvPart: a part of a screen that is otherwise usable in TV mode (the theme maker in Appearance).
// Out of TV mode, or at the full level, it is simply its children.
export function TvPart({ id, children }: { id: string; children: ReactNode }) {
  const tv = useTvMode()
  const loc = useLocation()
  if (!tv) return <>{children}</>
  const level = tvPartLevel(id)
  if (level === 'full') return <>{children}</>
  if (level === 'view') return <ViewOnly>{children}</ViewOnly>
  return <PartNote title={tx(tvPartTitle(id))} path={loc.pathname} />
}

// tvPartBlocked: for a part opened by a button (a dialog, an editor), whether it opens here.
export function useTvPartBlocked(id: string): boolean {
  const tv = useTvMode()
  return tv && tvPartLevel(id) === 'desktopOnly'
}

export function PartNote({ title, path }: { title: string; path: string }) {
  const s = useSummary(undefined)
  const device = tvClient() === 'tvDevice'
  return (
    <div className="tvl-part" role="note">
      <span className="tvl-part-ic" aria-hidden="true"><IconDevices /></span>
      <p className="tvl-part-text">
        <b>{t('{title}.', { title })}</b> {device ? <>{TV_NOTE()} <Address text={addressOf(path, s.addresses)} /></> : tvNote()}
      </p>
      {!device && <LeaveButton primary={false} small />}
    </div>
  )
}

// OnAComputerDialog: for a button that opens an editor (Customise this page, Edit layout, a title's
// scenes and skips): in TV mode the button stays, and says where to go instead of opening it.
// `path` is the setting's own address (a settings row's, with its #part), so the QR code and the short link go
// straight to it; `summary` is its TV summary, shown as what is set now. Without them, the page that is open.
export function OnAComputerDialog({ part, title: named, path, summary, onClose }: { part?: string; title?: string; path?: string; summary?: string; onClose: () => void }) {
  const loc = useLocation()
  const s = useSummary(summary)
  const device = tvClient() === 'tvDevice'
  const address = addressOf(path || loc.pathname, s.addresses)
  const title = named || (part ? tx(tvPartTitle(part)) : '') || t('On a computer')
  const actions = device
    ? <Button variant="primary" data-tv-first onClick={onClose}>{t('OK')}</Button>
    : <><Button variant="primary" data-tv-first onClick={() => { onClose(); leaveTvMode() }}>{t('Leave TV mode')}</Button><Button onClick={onClose}>{t('Not now')}</Button></>
  return (
    <Dialog title={title} onClose={onClose} size="sm" actions={actions}>
      <div className="tvl-dialog">
        <div>
          <p>{tvNote()}</p>
          {device && <p><Address text={address} /></p>}
          {device && <p className="tvl-follow">{t('What you change there applies on this TV too.')}</p>}
          <SetNow lines={s.lines} />
        </div>
        {device && <QrCode text={address} label={t('QR code that opens this page on a phone or computer')} size={160} />}
      </div>
    </Dialog>
  )
}
