// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { IconClose } from '../icons'
import { Button, IconButton } from './Button'
import { openBackEntry } from './backEntry'

// Dialog and Sheet. Both render into <body>, so nothing around them can clip or
// cover them. a11y.tsx gives every one of them, wherever it came from, the same
// keyboard behaviour: focus moves in, Tab stays in, Escape or Back closes, and
// focus returns to what opened it.

type DialogProps = {
  title: ReactNode
  onClose: () => void
  children?: ReactNode
  actions?: ReactNode // the buttons along the bottom; the safest choice comes first
  size?: 'sm' | 'md' | 'wide'
  className?: string
  alert?: boolean // needs an answer: read out at once, and a click outside doesn't dismiss it
  hideClose?: boolean
  description?: ReactNode
}

function Layer({ onClose, alert, wrap, children }: { onClose: () => void; alert?: boolean; wrap?: string; children: ReactNode }) {
  // Pressing inside and letting go outside (selecting text) isn't a click outside.
  const pressedInside = useRef(false)
  return createPortal(
    <div className={'modal-backdrop' + (wrap ? ' ' + wrap : '')}
      onMouseDown={(e) => { pressedInside.current = e.target !== e.currentTarget }}
      onClick={(e) => { const inside = pressedInside.current; pressedInside.current = false; if (e.target === e.currentTarget && !inside && !alert) onClose() }}>
      {children}
    </div>, document.body)
}

export function Dialog({ title, onClose, children, actions, size = 'md', className, alert, hideClose, description }: DialogProps) {
  const id = useId()
  return (
    <Layer onClose={onClose} alert={alert}>
      <div className={'modal ui-dialog ' + size + (className ? ' ' + className : '')} role={alert ? 'alertdialog' : 'dialog'} aria-modal="true" aria-labelledby={id + 't'} aria-describedby={description ? id + 'd' : undefined}
        onKeyDown={alert ? (e) => { if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack') { e.preventDefault(); onClose() } } : undefined}>
        <header className="ui-dialog-head">
          <h2 id={id + 't'}>{title}</h2>
          {!hideClose && !alert && <IconButton label="Close" size="sm" hideTip onClick={onClose}><IconClose /></IconButton>}
        </header>
        {description && <p className="ui-dialog-desc" id={id + 'd'}>{description}</p>}
        {children}
        {actions && <div className="actions">{actions}</div>}
      </div>
    </Layer>
  )
}

// Comes in from the side on wide screens and up from the bottom on phones, for
// things that sit beside the page rather than interrupt it.
export function Sheet({ title, onClose, children, actions, className }: Pick<DialogProps, 'title' | 'onClose' | 'children' | 'actions' | 'className'>) {
  const id = useId()
  return (
    <Layer onClose={onClose} wrap="ui-sheet-wrap">
      <div className={'modal ui-sheet' + (className ? ' ' + className : '')} role="dialog" aria-modal="true" aria-labelledby={id}>
        <header className="ui-dialog-head">
          <h2 id={id}>{title}</h2>
          <IconButton label="Close" size="sm" hideTip onClick={onClose}><IconClose /></IconButton>
        </header>
        <div className="ui-sheet-body">{children}</div>
        {actions && <div className="actions">{actions}</div>}
      </div>
    </Layer>
  )
}

// useBackCloses: Back (a phone's gesture, the browser's button, a mouse's) closes something open
// over the page, rather than leaving the page, for as long as open is true (backEntry.ts). mark
// names its entry in the history; a sheet over a sheet needs its own.
export function useBackCloses(open: boolean, close: () => void, mark: string) {
  const closeRef = useRef(close)
  closeRef.current = close
  useEffect(() => (open ? openBackEntry(window.history, window, mark, () => closeRef.current()) : undefined), [open, mark])
}

// ---------------------------------------------------------------- asking

type Ask = { title: string; body?: ReactNode; confirm?: string; cancel?: string; danger?: boolean; resolve: (yes: boolean) => void }
let openAsk: ((a: Ask) => void) | null = null

// ask: "are you sure?" in the app's own dialog. Resolves true or false.
//   if (!await ask({ title: 'Remove this library?', body: 'Your files stay where they are.', confirm: 'Remove', danger: true })) return
export function ask(a: Omit<Ask, 'resolve'>): Promise<boolean> {
  return new Promise((resolve) => {
    if (openAsk) openAsk({ ...a, resolve })
    else resolve(false)
  })
}

// AskHost is mounted once, beside the toasts.
export function AskHost() {
  const [cur, setCur] = useState<Ask | null>(null)
  useEffect(() => {
    openAsk = (a) => setCur((was) => { was?.resolve(false); return a })
    return () => { openAsk = null }
  }, [])
  if (!cur) return null
  const answer = (yes: boolean) => { cur.resolve(yes); setCur(null) }
  return (
    <Dialog alert size="sm" title={cur.title} onClose={() => answer(false)} description={cur.body}
      actions={<>
        <Button variant="ghost" autoFocus onClick={() => answer(false)}>{cur.cancel || 'Cancel'}</Button>
        <Button variant={cur.danger ? 'danger' : 'primary'} onClick={() => answer(true)}>{cur.confirm || 'OK'}</Button>
      </>} />
  )
}
