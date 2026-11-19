// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Dialog } from './Overlay'
import { HELP_EVENT } from './KeyHelp.event'
import { legend, type InputKind } from './inputModel'
import { useInput } from './useInput'
import { t, tk, tx } from '../i18n'

// The help for the keys, asked for with ? (or Back and Start together on a pad). It lists the one input model for
// the input in use, and the keyboard's own shortcuts besides when a keyboard is there.

const KEYBOARD_EXTRA: [string, string][] = [
  [tk('/ or Ctrl+K'), tk('Search')], [tk('Ctrl+[ and Ctrl+]'), tk('Previous and next tab')], [tk('.  or Shift+F10'), tk('Options for the thing in focus')],
  ['Ctrl+Shift+L', tk('Edit the layout')], ['F6', tk('Next area: page, controls, menu')], [tk('Page Up and Page Down'), tk('Page through a long list')], [tk('Home'), tk('Jump to the top')], [tk('Esc or Backspace'), tk('Back')],
]

export function KeyHelp() {
  const [open, setOpen] = useState(false)
  const { kind, family } = useInput()
  useEffect(() => {
    const on = () => setOpen((o) => !o)
    window.addEventListener(HELP_EVENT, on)
    return () => window.removeEventListener(HELP_EVENT, on)
  }, [])
  if (!open) return null
  const shown: InputKind = kind === 'touch' ? 'keys' : kind
  const items = legend(shown, family, { scope: 'list' })
  return (
    <Dialog title={t('Keys')} size="sm" onClose={() => setOpen(false)} className="key-help">
      <dl className="key-help-list">
        {items.map((i) => <div key={i.action}><dt><kbd>{tx(i.glyph)}</kbd></dt><dd>{tx(i.label)}</dd></div>)}
        {shown === 'keys' && KEYBOARD_EXTRA.map(([k, v]) => <div key={k}><dt><kbd>{tx(k)}</kbd></dt><dd>{tx(v)}</dd></div>)}
      </dl>
    </Dialog>
  )
}
