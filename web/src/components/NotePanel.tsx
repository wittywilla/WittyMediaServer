// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState, type ReactElement } from 'react'
import { useNavigate } from 'react-router-dom'
import { fmtAgo } from '../activity'
import { clearNotes, loadNotes, markRead, useNotes, type Note, type NoteKind } from '../notes'
import { IconActivity, IconBell, IconCheck, IconDownload, IconPlug, IconSettings, IconTrash, IconTv } from '../icons'
import { Button, EmptyState, IconButton, LinkButton, Sheet, Spinner, ask, toast } from '../ui'
import { t, tx } from '../i18n'

// The list behind the bell, newest first. Choosing one marks it read and opens what it's about.
// A sheet on phones; on a television, the same, moved through with the remote.

const ICONS: Record<NoteKind, (p: { size?: number }) => ReactElement> = {
  episodes: IconTv, recordings: IconTv, scans: IconActivity, modules: IconPlug, server: IconDownload,
}

export default function NotePanel({ onClose, tv }: { onClose: () => void; tv?: boolean }) {
  const n = useNotes()
  const nav = useNavigate()
  const [busy, setBusy] = useState(false)
  const act = async (f: () => Promise<void>) => {
    setBusy(true)
    try { await f() } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  const open = (note: Note) => {
    if (!note.read) void markRead([note.id]).catch(() => {})
    if (note.link) { onClose(); nav(note.link) }
  }
  const clearAll = async () => {
    if (!await ask({ title: t('Clear every notification?'), body: t('They stop being listed here, on every device you use.'), confirm: t('Clear all'), danger: true })) return
    await act(() => clearNotes())
  }
  return (
    <Sheet title={t('Notifications')} onClose={onClose} className="note-sheet" actions={tv ? undefined : (
      <LinkButton to="/settings/notifications" variant="ghost" size="sm" icon={<IconSettings size={16} />} onClick={onClose}>{t('Notification settings')}</LinkButton>
    )}>
      {n.notes.length > 0 && (
        <div className="note-bar">
          <Button size="sm" disabled={busy || n.unread === 0} icon={<IconCheck size={16} />} onClick={() => act(() => markRead())}>{t('Mark all read')}</Button>
          <Button size="sm" variant="ghost" disabled={busy} icon={<IconTrash size={16} />} onClick={clearAll}>{t('Clear all')}</Button>
        </div>
      )}
      {!n.loaded ? <Spinner /> : n.notes.length === 0
        ? <EmptyState icon={<IconBell />} title={t('Nothing new')} text={t('New episodes, finished recordings and updates show up here.')} />
        : (
          <ul className="note-list">
            {n.notes.map((note) => {
              const Icon = ICONS[note.kind] ?? IconBell
              return (
                <li key={note.id} className={'note' + (note.read ? '' : ' unread')}>
                  <button type="button" className="note-open" onClick={() => open(note)} aria-describedby={'note-when-' + note.id}>
                    <span className="note-icon" aria-hidden="true"><Icon size={20} /></span>
                    <span className="note-text">
                      <span className="note-title">{tx(note.title)}{!note.read && <span className="sr-only">{t(', unread')}</span>}</span>
                      {note.body && <span className="note-body">{tx(note.body)}</span>}
                      <span className="note-when" id={'note-when-' + note.id}>{fmtAgo(note.at)}</span>
                    </span>
                  </button>
                  <IconButton size="sm" label={t('Clear {title}', { title: note.title })} disabled={busy} onClick={() => act(() => clearNotes(note.id))}><IconTrash size={16} /></IconButton>
                </li>
              )
            })}
          </ul>
        )}
      {n.more && <div className="note-more"><Button size="sm" disabled={busy} onClick={() => act(() => loadNotes(true))}>{t('Show more')}</Button></div>}
    </Sheet>
  )
}
