// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useSyncExternalStore } from 'react'
import { announce } from './a11y'
import { api } from './api'
import { t, tk } from './i18n'

// The bell's notifications (docs/NOTIFICATIONS.md). One small request a minute, and only while a tab is
// showing: the count and the newest id. The list is read when the bell is opened or something new arrives.

export type NoteKind = 'episodes' | 'recordings' | 'scans' | 'modules' | 'server'
export type Note = { id: number; kind: NoteKind; title: string; body?: string; link?: string; at: number; read: boolean }
export type NoteChoice = { kind: NoteKind; on: boolean }
export type NotesView = { loaded: boolean; unread: number; latest: number; notes: Note[]; more: boolean }

const POLL_MS = 60000
const PAGE = 30

let view: NotesView = { loaded: false, unread: 0, latest: 0, notes: [], more: false }
const listeners = new Set<() => void>()
let timer: ReturnType<typeof setTimeout> | null = null
let panelOpen = false
let listed = false // the list has been read at least once since the store started

function publish(next: Partial<NotesView>) {
  view = { ...view, ...next }
  listeners.forEach((l) => l())
}

const tabHidden = () => typeof document !== 'undefined' && document.visibilityState === 'hidden'

async function poll() {
  if (timer) { clearTimeout(timer); timer = null }
  if (!listeners.size || tabHidden()) return
  try {
    const c = await api.get<{ unread: number; latest: number }>('/me/notifications/count')
    const before = view.latest
    publish({ loaded: true, unread: c.unread })
    if (c.latest !== before) {
      // Something new (or cleared elsewhere): the newest few, read out once each, unless this is the first look.
      if (panelOpen || listed) await loadNotes()
      else publish({ latest: c.latest })
      if (before > 0 && c.latest > before) {
        const fresh = view.notes.filter((n) => n.id > before && !n.read)
        if (fresh.length === 1) announce(t('New notification: {title}', { title: fresh[0].title }))
        else if (fresh.length > 1) announce(t('{count} new notifications', { count: fresh.length }))
      }
    }
  } catch { /* the server is away; the next look tries again */ }
  if (listeners.size && !tabHidden()) timer = setTimeout(poll, POLL_MS)
}

const onVisibility = () => { if (tabHidden()) { if (timer) clearTimeout(timer); timer = null } else poll() }

function subscribe(l: () => void) {
  listeners.add(l)
  if (listeners.size === 1) {
    document.addEventListener('visibilitychange', onVisibility)
    poll()
  }
  return () => {
    listeners.delete(l)
    if (!listeners.size) {
      document.removeEventListener('visibilitychange', onVisibility)
      if (timer) clearTimeout(timer)
      timer = null
      listed = false
      view = { loaded: false, unread: 0, latest: 0, notes: [], more: false }
    }
  }
}

export const useNotes = () => useSyncExternalStore(subscribe, () => view)
export const setNotesOpen = (open: boolean) => { panelOpen = open; if (open) void loadNotes() }

type NoteList = { notes: Note[]; unread: number; latest: number }

// The newest page, or the next older one.
export async function loadNotes(older = false) {
  const before = older && view.notes.length ? view.notes[view.notes.length - 1].id : 0
  const r = await api.get<NoteList>(`/me/notifications?limit=${PAGE}` + (before ? `&before=${before}` : ''))
  listed = true
  publish({ loaded: true, notes: older ? [...view.notes, ...r.notes] : r.notes, unread: r.unread, latest: r.latest, more: r.notes.length === PAGE })
}

export async function markRead(ids?: number[]) {
  const c = await api.post<{ unread: number }>('/me/notifications/read', ids?.length ? { ids } : {})
  const all = !ids?.length
  publish({ unread: c.unread, notes: view.notes.map((n) => (all || ids!.includes(n.id) ? { ...n, read: true } : n)) })
}

export async function clearNotes(id?: number) {
  const c = await api.del<{ unread: number; latest: number }>('/me/notifications' + (id ? '/' + id : ''))
  publish({ unread: c.unread, notes: id ? view.notes.filter((n) => n.id !== id) : [], more: id ? view.more : false })
}

export const noteChoices = () => api.get<{ choices: NoteChoice[] }>('/me/notifications/choices').then((r) => r.choices)
export const setNoteChoices = (choices: NoteChoice[]) => api.put<{ choices: NoteChoice[] }>('/me/notifications/choices', { choices }).then((r) => r.choices)

// Marked here and shown with tx().
export const KIND_NAMES: Record<NoteKind, { name: string; about: string }> = {
  episodes: { name: tk('New episodes'), about: tk('When a scan finds new episodes of a show you can watch.') },
  recordings: { name: tk('Recordings'), about: tk('When a recording you set up finishes, or fails.') },
  scans: { name: tk('Failed scans'), about: tk('When a library scan stops on an error.') },
  modules: { name: tk('Module updates'), about: tk('When modules you have get new versions.') },
  server: { name: tk('Server updates'), about: tk('When a new version of WittyMediaServer is out.') },
}

// ---------------------------------------------------------------- an administrator's channels

export type Smtp = { host: string; port: number; security: 'starttls' | 'tls' | 'none'; user: string; from: string; password?: string; passwordSet: boolean }
export type NoteChannel = { emailOn: boolean; emailTo: string; webhookOn: boolean; webhookUrl: string; webhookSecret?: string; secretSet: boolean }
export type NoteChannels = { smtp: Smtp; channel: NoteChannel }

export const noteChannels = () => api.get<NoteChannels>('/notifications/channels')
export const saveNoteChannels = (v: { smtp?: Smtp; clearPassword?: boolean; channel: NoteChannel; clearSecret?: boolean }) => api.put<NoteChannels>('/notifications/channels', v)
export const testNoteChannels = () => api.post<{ ok: boolean }>('/notifications/channels/test')
