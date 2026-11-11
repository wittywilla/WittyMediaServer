// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The menu a right click (or the menu key) opens on a card: what the card itself does,
// and the commands the app has for a title.

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { View } from '../model'
import type { Action } from '../types'

const WATCHABLE = ['movie', 'episode', 'video', 'disc']

export default function CardMenu({ card, x, y, act, onClose }: { card: View; x: number; y: number; act: (a: Action | undefined, from?: View) => void; onClose: () => void }) {
  const box = useRef<HTMLDivElement | null>(null)
  const [at, setAt] = useState({ left: x, top: y })
  const p = card.node.props || {}
  const id = typeof p.itemId === 'string' ? p.itemId : ''
  const kind = typeof p.kind === 'string' ? p.kind : ''
  const playId = typeof p.playId === 'string' ? p.playId : ''
  const entries: { key: string; label: string; action: Action | undefined }[] = []
  if (card.node.action) entries.push({ key: 'open', label: 'Open', action: card.node.action })
  if (playId) {
    entries.push({ key: 'play', label: 'Play', action: { type: 'play', item: playId } })
    entries.push({ key: 'restart', label: 'Play from the start', action: { type: 'play', item: playId, params: { resume: false } } })
  }
  if (id && (WATCHABLE.includes(kind) || kind === 'show')) entries.push({ key: 'later', label: 'Add to Watch later', action: { type: 'command', op: 'watchLater.add', item: id } })
  if (/^\d+$/.test(id) && WATCHABLE.includes(kind)) {
    entries.push(p.watched === true
      ? { key: 'unwatch', label: 'Mark as not watched', action: { type: 'command', op: 'watched.clear', item: id } }
      : { key: 'watch', label: 'Mark as watched', action: { type: 'command', op: 'watched.set', item: id } })
  }
  // Kept inside the window.
  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    const r = el.getBoundingClientRect()
    setAt({ left: Math.max(8, Math.min(x, window.innerWidth - r.width - 8)), top: Math.max(8, Math.min(y, window.innerHeight - r.height - 8)) })
    el.querySelector<HTMLElement>('[role=menuitem]')?.focus()
  }, [x, y])
  useEffect(() => {
    const was = document.activeElement as HTMLElement | null
    const away = (e: Event) => { if (!box.current?.contains(e.target as Node)) onClose() }
    window.addEventListener('pointerdown', away, true)
    window.addEventListener('scroll', onClose, { passive: true })
    window.addEventListener('resize', onClose)
    return () => { window.removeEventListener('pointerdown', away, true); window.removeEventListener('scroll', onClose); window.removeEventListener('resize', onClose); was?.focus?.({ preventScroll: true }) }
  }, [onClose])
  const keys = (e: React.KeyboardEvent) => {
    const items = Array.from(box.current?.querySelectorAll<HTMLElement>('[role=menuitem]') || [])
    const i = items.indexOf(document.activeElement as HTMLElement)
    if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || e.key === 'Tab') { e.preventDefault(); e.stopPropagation(); onClose() }
    else if (e.key === 'ArrowDown') { e.preventDefault(); e.stopPropagation(); items[(i + 1) % items.length]?.focus() }
    else if (e.key === 'ArrowUp') { e.preventDefault(); e.stopPropagation(); items[(i - 1 + items.length) % items.length]?.focus() }
    else if (e.key === 'Home') { e.preventDefault(); items[0]?.focus() }
    else if (e.key === 'End') { e.preventDefault(); items[items.length - 1]?.focus() }
  }
  if (!entries.length) return null
  return createPortal(
    <div ref={box} className="ui-menu sdui-card-menu" role="menu" aria-label={typeof p.title === 'string' ? p.title : 'Options'} style={{ position: 'fixed', left: at.left, top: at.top }} onKeyDown={keys}>
      {entries.map((e) => <button key={e.key} type="button" role="menuitem" className="ui-menu-item" onClick={() => { onClose(); act(e.action, card) }}>{e.label}</button>)}
    </div>, document.body)
}
