import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'

// Accessibility shared by every page: dialogs that hold the keyboard while
// open, page changes announced and focused, and a polite voice for things
// that change on screen without focus moving (the player saying "Paused").

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable=true]'
const DIALOGS = '[role=dialog], [role=alertdialog], .modal, .fx'

const visible = (e: Element) => (e as HTMLElement).offsetParent !== null || getComputedStyle(e).position === 'fixed'

// useDialogs: every dialog, whoever made it, gets the same behaviour.
// - announced as a modal dialog, named by its first heading;
// - focus moves into it (unless something inside already took it) and
//   Tab stays inside while it is open;
// - Escape closes it (by its backdrop, as clicking outside does), unless
//   the dialog handled Escape itself;
// - when it closes, focus goes back to what opened it.
export function useDialogs() {
  useEffect(() => {
    const opened = new Map<Element, HTMLElement | null>()
    const top = () => [...opened.keys()].filter((d) => d.isConnected).pop() as HTMLElement | undefined
    const prepare = (d: HTMLElement) => {
      if (opened.has(d)) return
      opened.set(d, document.activeElement as HTMLElement | null)
      if (!d.getAttribute('role')) d.setAttribute('role', 'dialog')
      if (!d.hasAttribute('aria-modal')) d.setAttribute('aria-modal', 'true')
      if (!d.hasAttribute('aria-label') && !d.hasAttribute('aria-labelledby')) {
        const h = d.querySelector('h1, h2, h3')
        if (h) { h.id ||= 'dlg-' + Math.random().toString(36).slice(2, 8); d.setAttribute('aria-labelledby', h.id) }
      }
      requestAnimationFrame(() => {
        if (d.contains(document.activeElement)) return
        const first = [...d.querySelectorAll<HTMLElement>('[autofocus], ' + FOCUSABLE)].find(visible)
        ;(first || d).focus({ preventScroll: true })
        if (!first && !d.hasAttribute('tabindex')) d.setAttribute('tabindex', '-1')
      })
    }
    const sweep = () => {
      document.querySelectorAll<HTMLElement>(DIALOGS).forEach((d) => { if (!d.parentElement?.closest(DIALOGS)) prepare(d) })
      for (const [d, back] of opened) {
        if (d.isConnected) continue
        opened.delete(d)
        if (back?.isConnected && !top()) back.focus({ preventScroll: true })
      }
    }
    const mo = new MutationObserver(sweep)
    mo.observe(document.body, { childList: true, subtree: true })
    sweep()
    const keys = (e: KeyboardEvent) => {
      const d = top()
      if (!d) return
      if (e.key === 'Tab') {
        const items = [...d.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(visible)
        if (!items.length) { e.preventDefault(); return }
        const first = items[0], last = items[items.length - 1]
        if (!d.contains(document.activeElement)) { e.preventDefault(); first.focus(); return }
        if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
      } else if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || (e.key === 'Backspace' && !(e.target as HTMLElement)?.matches?.('input, textarea, select, [contenteditable=true]'))) {
        if (e.key !== 'Escape') e.preventDefault()
        // Let the dialog's own handlers run first; close by the backdrop if
        // nothing did (a still-open dialog after this tick).
        setTimeout(() => {
          if ((e.key === 'Escape' && e.defaultPrevented) || !d.isConnected) return
          const backdrop = d.closest('.modal-backdrop') as HTMLElement | null
          backdrop?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
        }, 0)
      }
    }
    window.addEventListener('keydown', keys)
    return () => { mo.disconnect(); window.removeEventListener('keydown', keys) }
  }, [])
}

let say: (text: string) => void = () => {}

// announce: a short message for screen readers, read when they are idle
// ("Paused", "Subtitles: English").
export function announce(text: string) { say(text) }

// Announcer: the live region announce() speaks through, and the page-change
// announcement: after a navigation it reads the new page's heading and moves
// focus to the content, as a full page load would.
export function Announcer() {
  const [msg, setMsg] = useState('')
  const loc = useLocation()
  const first = useRef(true)
  useEffect(() => {
    let t: ReturnType<typeof setTimeout>
    say = (text) => { setMsg(''); clearTimeout(t); t = setTimeout(() => setMsg(text), 60) }
    return () => { say = () => {} }
  }, [])
  useEffect(() => {
    if (first.current) { first.current = false; return }
    const t = setTimeout(() => {
      if (document.querySelector(DIALOGS)) return
      const main = document.getElementById('main')
      const h1 = main?.querySelector('h1')?.textContent?.trim()
      announce(h1 ? `${h1}, page loaded` : `${document.title}, page loaded`)
      const active = document.activeElement
      if (main && (!active || active === document.body || !main.contains(active))) main.focus({ preventScroll: true })
    }, 350)
    return () => clearTimeout(t)
  }, [loc.pathname])
  return <div className="sr-only" role="status" aria-live="polite" aria-atomic="true">{msg}</div>
}
