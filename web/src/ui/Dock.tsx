// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useRef, type KeyboardEvent, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { KeyLegend } from './KeyLegend'
import { Menu, type MenuEntry } from './Menu'
import { useInput } from './useInput'

// One button of a Dock: it runs something (onClick), goes somewhere (to), or opens a menu (items).
export type DockAction = { id: string; label: string; icon?: ReactNode; onClick?: () => void; to?: string; items?: MenuEntry[]; primary?: boolean; pressed?: boolean; hint?: string; disabled?: boolean; detail?: ReactNode }

// The Dock: one row of a screen's persistent controls, bottom of the page, that never moves. It is a zone
// (data-nav-zone="dock"): Down from the Stage reaches it, Back or Up returns, and it is the one place focus wraps
// (docs/plans/DESIGN-2.md section 3). Each button may show the key that reaches it directly. `btn` is the class a
// page's own look gives its buttons (WittyTV's themes), so the Dock is drawn in whatever look the page has.
export function Dock({ label, actions, legend = true, scope, className, sticky, btn }: { label: string; actions: DockAction[]; legend?: boolean; scope?: 'screen' | 'list'; className?: string; sticky?: boolean; btn?: string }) {
  const row = useRef<HTMLDivElement | null>(null)
  const { kind } = useInput()
  // Nothing to press and no input to show a legend for: nothing to draw.
  if (!actions.length && (!legend || kind === 'touch')) return null
  const wrap = (e: KeyboardEvent) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    const btns = Array.from(row.current?.querySelectorAll<HTMLElement>('button:not([disabled]), a[href]') || [])
    const i = btns.indexOf(document.activeElement as HTMLElement)
    if (i < 0 || btns.length < 2) return
    const at = e.key === 'ArrowRight' ? (i + 1 === btns.length ? 0 : -1) : (i === 0 ? btns.length - 1 : -1)
    if (at < 0) return
    e.preventDefault(); e.stopPropagation()
    btns[at].focus()
  }
  const cls = (a: DockAction) => 'dock-btn' + (btn ? ' ' + btn : '') + (a.primary ? ' primary' : '') + (a.pressed ? ' on' : '')
  const inner = (a: DockAction) => <>{a.icon}<span>{a.label}</span>{a.detail ? <small>{a.detail}</small> : null}{a.hint ? <kbd aria-hidden="true">{a.hint}</kbd> : null}</>
  return (
    <div className={'dock' + (sticky ? ' sticky' : '') + (className ? ' ' + className : '')} data-nav-zone="dock" role="toolbar" aria-label={label}>
      <div className="dock-row" ref={row} onKeyDown={wrap} data-nav-group>
        {actions.map((a) => a.items ? (
          <Menu key={a.id} label={a.label} items={a.items} align="start"
            trigger={(p, open) => <button {...p} type="button" className={cls(a) + (open ? ' on' : '')} disabled={a.disabled} data-dock={a.id}>{inner(a)}</button>} />
        ) : a.to ? (
          <Link key={a.id} to={a.to} className={cls(a)} data-dock={a.id}>{inner(a)}</Link>
        ) : (
          <button key={a.id} type="button" className={cls(a)} onClick={a.onClick} disabled={a.disabled} aria-pressed={a.pressed} data-dock={a.id}>{inner(a)}</button>
        ))}
      </div>
      {legend ? <KeyLegend scope={scope} /> : null}
    </div>
  )
}
