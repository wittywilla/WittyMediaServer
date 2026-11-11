// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What an editor with unsaved work needs from the page around it: a question before it is
// left, Undo and Redo on the keys people expect, and a file to hand over.

import { useEffect, useRef } from 'react'
import { useBlocker } from 'react-router-dom'
import { ask } from '../../../ui'
import { useEditor } from './context'

// useUnsavedGuard: leaving for another page, or closing the tab, asks first while there
// are changes that aren't in the draft. Returns a way to leave without being asked, for
// after the page itself was deleted.
export function useUnsavedGuard(dirty: boolean, what: string): () => void {
  const { unsaved } = useEditor()
  const free = useRef(false)
  const dirtyNow = useRef(dirty)
  dirtyNow.current = dirty
  useEffect(() => { unsaved.current = dirty ? what : ''; return () => { unsaved.current = '' } }, [dirty, what, unsaved])
  useEffect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])
  const blocker = useBlocker(({ currentLocation, nextLocation }) => currentLocation.pathname !== nextLocation.pathname && dirtyNow.current && !free.current)
  const blocked = blocker.state === 'blocked'
  const blockerNow = useRef(blocker)
  useEffect(() => { blockerNow.current = blocker }, [blocker])
  // Once per time it is blocked, whatever else redraws the page meanwhile.
  useEffect(() => {
    if (!blocked) return
    void ask({ title: 'Leave without saving?', body: `Your latest changes to ${what} aren’t saved to the draft. If you leave now they are lost.`, confirm: 'Leave', cancel: 'Keep editing', danger: true })
      .then((yes) => { const b = blockerNow.current; if (b.state === 'blocked') { if (yes) b.proceed(); else b.reset() } })
  }, [blocked]) // eslint-disable-line react-hooks/exhaustive-deps
  return () => { free.current = true }
}

// useEditKeys: Ctrl or Cmd with Z undoes, with Shift and Z (or Y) redoes, with S saves.
// While typing in a field the browser's own undo is left to do its job, and nothing here
// reaches under an open dialog.
export function useEditKeys(keys: { undo: () => void; redo: () => void; save: () => void }) {
  const now = useRef(keys)
  now.current = keys
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.altKey) return
      const k = e.key.toLowerCase()
      if (k !== 'z' && k !== 'y' && k !== 's') return
      if (document.querySelector('[role=dialog], [role=alertdialog]')) return
      if (k === 's') { e.preventDefault(); now.current.save(); return }
      const t = e.target as HTMLElement | null
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return
      e.preventDefault()
      if (k === 'y' || e.shiftKey) now.current.redo(); else now.current.undo()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
}

// download: a file made here, handed to the browser to keep.
export function download(name: string, data: unknown) {
  const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2) + '\n'], { type: 'application/json' }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}
