// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useState } from 'react'
import { setNotesOpen, useNotes } from '../notes'
import { IconBell } from '../icons'
import { IconButton } from '../ui'
import { t, tn } from '../i18n'
import './notes.css'

const NotePanel = lazy(() => import('./NotePanel'))

// The bell beside the activity button: how many notifications haven't been read, and the list behind
// it. In TV mode it sits at the end of the row of tabs, as a pill the remote reaches like the rest.
export default function NoteSlot({ tv }: { tv?: 'row' | 'side' }) {
  const n = useNotes()
  const [open, setOpen] = useState(false)
  const show = (on: boolean) => { setOpen(on); setNotesOpen(on) }
  const label = n.unread ? tn('Notifications, {count} unread', 'Notifications, {count} unread', n.unread) : t('Notifications')
  const badge = n.unread > 0 && <span className="note-count" aria-hidden="true">{n.unread > 99 ? '99+' : n.unread}</span>
  return (
    <>
      {tv
        ? <button type="button" className={'tvm-pill tvm-bell' + (tv === 'side' ? ' tvm-bell-side' : '')} aria-label={label} aria-haspopup="dialog" aria-expanded={open} onClick={() => show(true)}><IconBell />{badge}{tv === 'side' && <span aria-hidden="true">{t('Notifications')}</span>}</button>
        : <IconButton label={label} aria-haspopup="dialog" aria-expanded={open} className={'note-btn' + (n.unread ? ' on' : '')} onClick={() => show(true)}><IconBell size={18} />{badge}</IconButton>}
      {open && <Suspense fallback={null}><NotePanel tv={!!tv} onClose={() => show(false)} /></Suspense>}
    </>
  )
}
