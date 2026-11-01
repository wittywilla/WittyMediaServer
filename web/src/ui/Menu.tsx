import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import { isBackKey } from './spatial'

// A menu that opens from a button. It renders into <body> at a place worked out
// from the button, so a bar, a scrolling panel or a player beside it can never
// cover or clip it. Keyboard and remote: arrows move, Home and End jump, typing a
// letter finds, Enter chooses, Escape or Back closes, and focus returns to the button.

export type MenuEntry =
  | { kind?: 'item'; key: string; label: ReactNode; text?: string; icon?: ReactNode; to?: string; onSelect?: () => void; danger?: boolean; disabled?: boolean; current?: boolean; checked?: boolean; hint?: ReactNode }
  | { kind: 'separator'; key: string }
  | { kind: 'heading'; key: string; label: ReactNode }

type TriggerProps = { ref: (el: HTMLElement | null) => void; 'aria-haspopup': 'menu'; 'aria-expanded': boolean; 'aria-controls'?: string; onClick: () => void; onKeyDown: (e: KeyboardEvent) => void }

const EDGE = 8

export function Menu({ trigger, items, label, head, align = 'end', className }: { trigger: (props: TriggerProps, open: boolean) => ReactNode; items: MenuEntry[]; label: string; head?: ReactNode; align?: 'start' | 'end'; className?: string }) {
  const [open, setOpen] = useState(false)
  const [start, setStart] = useState<'first' | 'last'>('first')
  const button = useRef<HTMLElement | null>(null)
  const panel = useRef<HTMLDivElement | null>(null)
  const id = useId()
  const [pos, setPos] = useState<{ left: number; top: number; maxHeight: number } | null>(null)

  const close = useCallback((refocus: boolean) => { setOpen(false); setPos(null); if (refocus) button.current?.focus({ preventScroll: true }) }, [])

  const place = useCallback(() => {
    const a = button.current?.getBoundingClientRect(), p = panel.current
    if (!a || !p) return
    const w = p.offsetWidth, h = p.scrollHeight
    const below = innerHeight - a.bottom - EDGE * 2, above = a.top - EDGE * 2
    const up = h > below && above > below
    const maxHeight = Math.max(120, up ? above : below)
    const left = align === 'end' ? a.right - w : a.left
    setPos({ left: Math.max(EDGE, Math.min(left, innerWidth - w - EDGE)), top: up ? Math.max(EDGE, a.top - Math.min(h, maxHeight) - EDGE) : a.bottom + EDGE, maxHeight })
  }, [align])

  useLayoutEffect(() => { if (open) place() }, [open, place, items.length])

  useEffect(() => {
    if (!open) return
    const items = () => [...(panel.current?.querySelectorAll<HTMLElement>('[role^=menuitem]:not([aria-disabled=true])') || [])]
    const list = items()
    const current = list.find((el) => el.getAttribute('aria-current') === 'page')
    ;(start === 'last' ? list[list.length - 1] : current || list[0])?.focus({ preventScroll: true })
    const outside = (e: Event) => { if (!panel.current?.contains(e.target as Node) && !button.current?.contains(e.target as Node)) close(false) }
    const scrolled = (e: Event) => { if (!panel.current?.contains(e.target as Node)) close(false) }
    window.addEventListener('pointerdown', outside)
    window.addEventListener('resize', place)
    window.addEventListener('scroll', scrolled, { capture: true, passive: true })
    return () => { window.removeEventListener('pointerdown', outside); window.removeEventListener('resize', place); window.removeEventListener('scroll', scrolled, { capture: true }) }
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps

  const typed = useRef({ text: '', at: 0 })
  const onMenuKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const list = [...e.currentTarget.querySelectorAll<HTMLElement>('[role^=menuitem]:not([aria-disabled=true])')]
    const at = list.indexOf(document.activeElement as HTMLElement)
    const go = (i: number) => { e.preventDefault(); e.stopPropagation(); list[(i + list.length) % list.length]?.focus() }
    if (isBackKey(e)) { e.preventDefault(); e.stopPropagation(); close(true); return }
    switch (e.key) {
      case 'ArrowDown': return go(at + 1)
      case 'ArrowUp': return go(at - 1)
      case 'Home': return go(0)
      case 'End': return go(list.length - 1)
      case 'Escape': case 'Backspace': case 'ArrowLeft': case 'ArrowRight':
        e.preventDefault(); e.stopPropagation(); close(true); return
      case 'Tab': close(false); return
    }
    if (e.key.length === 1 && e.key !== ' ' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const now = Date.now()
      typed.current = { text: (now - typed.current.at > 700 ? '' : typed.current.text) + e.key.toLowerCase(), at: now }
      const hit = list.findIndex((el) => el.textContent?.trim().toLowerCase().startsWith(typed.current.text))
      if (hit >= 0) go(hit)
    }
  }
  const triggerProps: TriggerProps = {
    ref: (el) => { button.current = el },
    'aria-haspopup': 'menu',
    'aria-expanded': open,
    'aria-controls': open ? id : undefined,
    onClick: () => { setStart('first'); setOpen((o) => !o) },
    onKeyDown: (e) => {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        if (!open && e.key === 'ArrowUp') { /* stays with the page */ return }
        e.preventDefault(); e.stopPropagation()
        setStart(e.key === 'ArrowUp' ? 'last' : 'first')
        setOpen(true)
      }
    },
  }
  return (
    <>
      {trigger(triggerProps, open)}
      {open && createPortal(
        <div ref={panel} id={id} role="menu" aria-label={label} className={'ui-menu' + (className ? ' ' + className : '')} onKeyDown={onMenuKey}
          style={pos ? { left: pos.left, top: pos.top, maxHeight: pos.maxHeight } : { visibility: 'hidden' }}>
          {head && <div className="ui-menu-head" role="presentation">{head}</div>}
          {items.map((it) => {
            if (it.kind === 'separator') return <div key={it.key} className="ui-menu-sep" role="separator" />
            if (it.kind === 'heading') return <div key={it.key} className="ui-menu-heading" role="presentation">{it.label}</div>
            const body = <>{it.icon}<span className="ui-menu-text">{it.label}{it.hint && <small>{it.hint}</small>}</span></>
            const shared = {
              role: it.checked === undefined ? 'menuitem' : 'menuitemradio', tabIndex: -1, 'aria-disabled': it.disabled || undefined, 'aria-checked': it.checked,
              'aria-current': it.current ? 'page' as const : undefined, className: 'ui-menu-item' + (it.danger ? ' danger' : '') + (it.current || it.checked ? ' on' : ''),
            }
            const choose = () => { if (it.disabled) return; close(!it.to); it.onSelect?.() }
            return it.to && !it.disabled
              ? <Link key={it.key} to={it.to} {...shared} onClick={choose}>{body}</Link>
              : <button key={it.key} type="button" {...shared} onClick={choose}>{body}</button>
          })}
        </div>, document.body)}
    </>
  )
}
