// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { legend } from './inputModel'
import { useInput } from './useInput'

// The key legend: what each key, button or remote key does here, drawn for the input in use and hidden when there
// is none (touch only). It sits in a screen's Dock. docs/plans/DESIGN-2.md section 3.
export function KeyLegend({ scope = 'screen', className }: { scope?: 'screen' | 'list'; className?: string }) {
  const { kind, family } = useInput()
  const items = legend(kind, family, { scope })
  if (!items.length) return null
  return (
    <ul className={'key-legend' + (className ? ' ' + className : '')} aria-label="Keys" data-nav-skip>
      {items.map((i) => (
        <li key={i.action}><kbd>{i.glyph}</kbd><span>{i.label}</span></li>
      ))}
    </ul>
  )
}
