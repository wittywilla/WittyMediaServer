// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '../api'
import QrCode from '../pages/settings/QrCode'
import { Button, Dialog } from '../ui'
import { pageAddress, type TvLevel } from './levels'
import { TV_NOTE, tvPartLevel, tvPartTitle } from './levelTable'
import { useTvMode } from './mode'
import './levels.css'

// What a television shows in place of a screen it doesn't offer (docs/TV-MODE.md): what it is, that a
// computer changes it, the page's address in words and as a QR code a phone can open, and what is set
// now. Nothing here is a dead end: Back has focus, and goes where the screen was opened from.

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

// OnAComputer: the whole screen's card. With level "view" the screen's own content follows it,
// shown and not changeable.
export function OnAComputer({ level, title, path, summary, children }: { level: TvLevel; title: string; path: string; summary?: string; children?: ReactNode }) {
  const nav = useNavigate()
  const s = useSummary(summary)
  const address = addressOf(path, s.addresses)
  const back = useRef<HTMLButtonElement>(null)
  // Focus starts on Back, which is always there and always leads somewhere.
  useEffect(() => { back.current?.focus({ preventScroll: true }) }, [path])
  const goBack = () => {
    const idx = (window.history.state as { idx?: number } | null)?.idx
    if (idx) nav(-1)
    else nav('/you', { replace: true })
  }
  const view = level === 'view'
  return (
    <div className="tvl-page">
      <section className="tvl-card" aria-labelledby="tvl-title">
        <div className="tvl-text">
          <h1 id="tvl-title" className="tvl-title">{title}</h1>
          <p className="tvl-note">{view ? 'This shows what is set. ' : ''}{TV_NOTE}</p>
          <p className="tvl-addr" aria-label="Address">{address}</p>
          {s.lines.length > 0 && (
            <ul className="tvl-now" aria-label="Set now">
              {s.lines.map((l) => <li key={l}>{l}</li>)}
            </ul>
          )}
          <p className="tvl-follow">What you change there applies on this TV too.</p>
          <div className="tvl-actions" data-nav-group>
            <button ref={back} type="button" className="btn primary" data-tv-first onClick={goBack}>Back</button>
          </div>
        </div>
        <figure className="tvl-qr">
          <QrCode text={address} label={`QR code that opens ${title} on a phone or computer`} size={240} />
          <figcaption>Scan with a phone to open this page</figcaption>
        </figure>
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

// TvPart: a part of a screen that is otherwise usable on a television (the theme maker in Appearance).
// Off a television, or at the full level, it is simply its children.
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
  return (
    <div className="tvl-part" role="note">
      <b>{title}</b>
      <span>{TV_NOTE}</span>
      <span className="tvl-addr">{addressOf(path, s.addresses)}</span>
    </div>
  )
}

// OnAComputerDialog: for a button that opens an editor (Customise this page, Edit layout, a title's
// scenes and skips): on a television the button stays, and says where to go instead of opening it.
export function OnAComputerDialog({ part, onClose }: { part: string; onClose: () => void }) {
  const loc = useLocation()
  const s = useSummary(undefined)
  const address = addressOf(loc.pathname, s.addresses)
  const title = tvPartTitle(part) || 'This'
  return (
    <Dialog title={title} onClose={onClose} size="sm" actions={<Button variant="primary" data-tv-first onClick={onClose}>OK</Button>}>
      <div className="tvl-dialog">
        <div>
          <p>{TV_NOTE}</p>
          <p className="tvl-addr">{address}</p>
          <p className="tvl-follow">What you change there applies on this TV too.</p>
        </div>
        <QrCode text={address} label={`QR code that opens this page on a phone or computer`} size={160} />
      </div>
    </Dialog>
  )
}
