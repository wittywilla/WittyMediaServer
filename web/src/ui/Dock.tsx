// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useRef, type KeyboardEvent, type ReactNode } from 'react'
import { KeyLegend } from './KeyLegend'

export type DockAction = { id: string; label: string; icon?: ReactNode; onClick: () => void; primary?: boolean; pressed?: boolean; hint?: string; disabled?: boolean; menu?: boolean }

// The Dock: one row of a screen's persistent controls, bottom of the page, that never moves. It is a zone
// (data-nav-zone="dock"): Down from the Stage reaches it, Back or Up returns, and it is the one place focus wraps
// (docs/plans/DESIGN-2.md section 3). Each button may show the key that reaches it directly.
export function Dock({ label, actions, legend = true, scope, className, sticky }: { label: string; actions: DockAction[]; legend?: boolean; scope?: 'screen' | 'list'; className?: string; sticky?: boolean }) {
  const row = useRef<HTMLDivElement | null>(null)
  const wrap = (e: KeyboardEvent) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    const btns = Array.from(row.current?.querySelectorAll<HTMLButtonElement>('button:not([disabled])') || [])
    const i = btns.indexOf(document.activeElement as HTMLButtonElement)
    if (i < 0 || btns.length < 2) return
    const at = e.key === 'ArrowRight' ? (i + 1 === btns.length ? 0 : -1) : (i === 0 ? btns.length - 1 : -1)
    if (at < 0) return
    e.preventDefault(); e.stopPropagation()
    btns[at].focus()
  }
  return (
    <div className={'dock' + (sticky ? ' sticky' : '') + (className ? ' ' + className : '')} data-nav-zone="dock" role="toolbar" aria-label={label}>
      <div className="dock-row" ref={row} onKeyDown={wrap} data-nav-group>
        {actions.map((a) => (
          <button key={a.id} type="button" className={'dock-btn' + (a.primary ? ' primary' : '')} onClick={a.onClick} disabled={a.disabled}
            aria-pressed={a.pressed} aria-haspopup={a.menu ? 'menu' : undefined} data-dock={a.id}>
            {a.icon}<span>{a.label}</span>
            {a.hint ? <kbd aria-hidden="true">{a.hint}</kbd> : null}
          </button>
        ))}
      </div>
      {legend ? <KeyLegend scope={scope} /> : null}
    </div>
  )
}
