// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Dialog } from './Overlay'
import { HELP_EVENT } from './KeyHelp.event'
import { legend, type InputKind } from './inputModel'
import { useInput } from './inputKind'

// The help for the keys, asked for with ? (or Back and Start together on a pad). It lists the one input model for
// the input in use, and the keyboard's own shortcuts besides when a keyboard is there.

const KEYBOARD_EXTRA: [string, string][] = [
  ['/ or Ctrl+K', 'Search'], ['Ctrl+[ and Ctrl+]', 'Previous and next tab'], ['.  or Shift+F10', 'Options for the thing in focus'],
  ['F6', 'Next area: page, controls, menu'], ['Page Up and Page Down', 'Page through a long list'], ['Home', 'Jump to the top'], ['Esc or Backspace', 'Back'],
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
    <Dialog title="Keys" size="sm" onClose={() => setOpen(false)} className="key-help">
      <dl className="key-help-list">
        {items.map((i) => <div key={i.action}><dt><kbd>{i.glyph}</kbd></dt><dd>{i.label}</dd></div>)}
        {shown === 'keys' && KEYBOARD_EXTRA.map(([k, v]) => <div key={k}><dt><kbd>{k}</kbd></dt><dd>{v}</dd></div>)}
      </dl>
    </Dialog>
  )
}
