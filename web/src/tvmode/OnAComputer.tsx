// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '../api'
import QrCode from '../pages/settings/QrCode'
import { Button, Dialog } from '../ui'
import { pageAddress, type TvLevel } from './levels'
import { TV_LEVELS, TV_NOTE, tvPartLevel, tvPartTitle } from './levelTable'
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
export const tvElsewhere = () => (tvClient() === 'tvDevice' ? 'On a computer' : 'Outside TV mode')

// tvNote: the words for this client.
export const tvNote = () => (tvClient() === 'tvDevice' ? TV_NOTE : TV_LEVELS.noteDesktop || 'Leave TV mode to change this.')

// The Leave TV mode button: the normal layout at the page that was showing.
function LeaveButton({ buttonRef, primary = true }: { buttonRef?: React.Ref<HTMLButtonElement>; primary?: boolean }) {
  return <button ref={buttonRef} type="button" className={'btn' + (primary ? ' primary' : '')} data-tv-first onClick={leaveTvMode}>Leave TV mode</button>
}

// OnAComputer: the whole screen's card. With level "view" the screen's own content follows it,
// shown and not changeable.
export function OnAComputer({ level, title, path, summary, children }: { level: TvLevel; title: string; path: string; summary?: string; children?: ReactNode }) {
  const nav = useNavigate()
  const s = useSummary(summary)
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
  const view = level === 'view'
  return (
    <div className="tvl-page" data-tv-client={device ? 'tv' : 'computer'}>
      <section className={'tvl-card' + (device ? '' : ' tvl-computer')} aria-labelledby="tvl-title">
        <div className="tvl-text">
          <h1 id="tvl-title" className="tvl-title">{title}</h1>
          <p className="tvl-note">{view ? 'This shows what is set. ' : ''}{device ? TV_NOTE : tvNote()}</p>
          {device && <p className="tvl-addr" aria-label="Address">{address}</p>}
          {s.lines.length > 0 && (
            <ul className="tvl-now" aria-label="Set now">
              {s.lines.map((l) => <li key={l}>{l}</li>)}
            </ul>
          )}
          {device && <p className="tvl-follow">What you change there applies on this TV too.</p>}
          <div className="tvl-actions" data-nav-group>
            {device
              ? <button ref={first} type="button" className="btn primary" data-tv-first onClick={goBack}>Back</button>
              : <><LeaveButton buttonRef={first} /><button type="button" className="btn" onClick={goBack}>Back</button></>}
          </div>
        </div>
        {device && (
          <figure className="tvl-qr">
            <QrCode text={address} label={`QR code that opens ${title} on a phone or computer`} size={240} />
            <figcaption>Scan with a phone to open this page</figcaption>
          </figure>
        )}
      </section>
      {view && children && <ViewOnly>{children}</ViewOnly>}
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
  return <fieldset ref={box} className="tvl-view" disabled aria-label="Shown only">{children}</fieldset>
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
  return <PartNote title={tvPartTitle(id)} path={loc.pathname} />
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
      <b>{title}</b>
      <span>{device ? TV_NOTE : tvNote()}</span>
      {device ? <span className="tvl-addr">{addressOf(path, s.addresses)}</span> : <LeaveButton primary={false} />}
    </div>
  )
}

// OnAComputerDialog: for a button that opens an editor (Customise this page, Edit layout, a title's
// scenes and skips): in TV mode the button stays, and says where to go instead of opening it.
export function OnAComputerDialog({ part, onClose }: { part: string; onClose: () => void }) {
  const loc = useLocation()
  const s = useSummary(undefined)
  const device = tvClient() === 'tvDevice'
  const address = addressOf(loc.pathname, s.addresses)
  const title = tvPartTitle(part) || 'This'
  const actions = device
    ? <Button variant="primary" data-tv-first onClick={onClose}>OK</Button>
    : <><Button variant="primary" data-tv-first onClick={() => { onClose(); leaveTvMode() }}>Leave TV mode</Button><Button onClick={onClose}>Not now</Button></>
  return (
    <Dialog title={title} onClose={onClose} size="sm" actions={actions}>
      <div className="tvl-dialog">
        <div>
          <p>{device ? TV_NOTE : tvNote()}</p>
          {device && <p className="tvl-addr">{address}</p>}
          {device && <p className="tvl-follow">What you change there applies on this TV too.</p>}
        </div>
        {device && <QrCode text={address} label={`QR code that opens this page on a phone or computer`} size={160} />}
      </div>
    </Dialog>
  )
}
