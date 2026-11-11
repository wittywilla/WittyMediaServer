// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { IconRight } from '../icons'
import { Sheet } from './Overlay'

// A sheet of rows, from the bottom of a phone: what a page's "⋯" button holds (an item's
// other actions, a library's tools) and the account's You sheet. Choosing a row closes the
// sheet, whatever the row does next. Any button or link put in it becomes a row, so a page
// can hand it the same buttons it shows in a row on a wide screen.
export function RowSheet({ title, onClose, children, head, className }: { title: string; onClose: () => void; children: ReactNode; head?: ReactNode; className?: string }) {
  return (
    <Sheet title={title} onClose={onClose} className={'ui-rowsheet' + (className ? ' ' + className : '')}>
      {head}
      {/* Delegated: a row's own click runs first, then the sheet closes. */}
      <div className="ui-rows-wrap" onClickCapture={(e) => { if ((e.target as HTMLElement).closest('a, button:not([disabled])')) setTimeout(onClose) }}>
        {children}
      </div>
    </Sheet>
  )
}

// A group of rows on one card. Separate groups get a little space between them.
export function Rows({ children, label }: { children: ReactNode; label?: string }) {
  return <div className="ui-rows" role="group" aria-label={label}>{children}</div>
}

// One row: an icon, its words, and a chevron when it opens a page.
export function SheetRow({ icon, label, to, onSelect, danger, disabled, end }: { icon?: ReactNode; label: ReactNode; to?: string; onSelect?: () => void; danger?: boolean; disabled?: boolean; end?: ReactNode }) {
  const cls = 'ui-row' + (danger ? ' danger' : '')
  const body = <>{icon && <span className="ui-row-ic" aria-hidden="true">{icon}</span>}<span className="ui-row-text">{label}</span>{end}{to && <IconRight className="ui-row-go" />}</>
  return to && !disabled
    ? <Link to={to} className={cls}>{body}</Link>
    : <button type="button" className={cls} disabled={disabled} onClick={onSelect}>{body}</button>
}
