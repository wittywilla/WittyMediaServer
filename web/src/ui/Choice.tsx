import { useRef, type KeyboardEvent, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { IconClose } from '../icons'

// Picking one of a few: Segmented (a radio group), Tabs (sections of a page),
// and Chips (filters that can be on or off). All three keep the Tab key to one
// stop and move along with the arrows. At either end an arrow isn't used, so a
// remote carries on to whatever is beside the control (see spatial.ts).

function moveAlong(e: KeyboardEvent, count: number, index: number, go: (i: number) => void, vertical = false) {
  const back = vertical ? 'ArrowUp' : 'ArrowLeft', on = vertical ? 'ArrowDown' : 'ArrowRight'
  let to = -1
  if (e.key === back && index > 0) to = index - 1
  else if (e.key === on && index < count - 1) to = index + 1
  else if (e.key === 'Home') to = 0
  else if (e.key === 'End') to = count - 1
  if (to < 0) return
  e.preventDefault()
  go(to)
}

export type Option<T extends string> = { value: T; label: ReactNode; icon?: ReactNode; disabled?: boolean; name?: string }

export function Segmented<T extends string>({ value, onChange, options, label, big, className }: { value: T; onChange: (v: T) => void; options: Option<T>[]; label: string; big?: boolean; className?: string }) {
  const box = useRef<HTMLDivElement | null>(null)
  const enabled = options.filter((o) => !o.disabled)
  const at = Math.max(0, enabled.findIndex((o) => o.value === value))
  const go = (i: number) => { onChange(enabled[i].value); requestAnimationFrame(() => box.current?.querySelector<HTMLElement>('[tabindex="0"]')?.focus()) }
  return (
    <div ref={box} className={'seg' + (big ? ' big' : '') + (className ? ' ' + className : '')} role="radiogroup" aria-label={label} onKeyDown={(e) => moveAlong(e, enabled.length, at, go)}>
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={o.value === value} aria-label={o.name} disabled={o.disabled} tabIndex={o.value === enabled[at]?.value ? 0 : -1}
          className={o.value === value ? 'active' : ''} onClick={() => onChange(o.value)}>{o.icon}{o.label}</button>
      ))}
    </div>
  )
}

export type TabItem<T extends string> = { value: T; label: ReactNode; to?: string; badge?: ReactNode }

// Tabs with `to` are links to their own address; the others call onChange.
// panelId names the element the tabs control, when there is one to point at.
export function Tabs<T extends string>({ value, onChange, items, label, panelId, className }: { value: T; onChange?: (v: T) => void; items: TabItem<T>[]; label: string; panelId?: string; className?: string }) {
  const box = useRef<HTMLDivElement | null>(null)
  const at = Math.max(0, items.findIndex((t) => t.value === value))
  const go = (i: number) => { const t = items[i]; if (!t.to) onChange?.(t.value); box.current?.querySelectorAll<HTMLElement>('[role=tab]')[i]?.focus() }
  return (
    <div ref={box} className={'tabs' + (className ? ' ' + className : '')} role="tablist" aria-label={label} onKeyDown={(e) => moveAlong(e, items.length, at, go)}>
      {items.map((t) => {
        const on = t.value === value
        const props = { role: 'tab', 'aria-selected': on, 'aria-controls': on ? panelId : undefined, tabIndex: on ? 0 : -1, className: on ? 'active' : '' }
        return t.to
          ? <Link key={t.value} to={t.to} {...props} onClick={() => onChange?.(t.value)}>{t.label}{t.badge}</Link>
          : <button key={t.value} type="button" {...props} onClick={() => onChange?.(t.value)}>{t.label}{t.badge}</button>
      })}
    </div>
  )
}

export function Chips({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return <div className={'chips' + (className ? ' ' + className : '')} role="group" aria-label={label}>{children}</div>
}

// A filter that is on or off, or with onRemove a value that can be taken away.
export function Chip({ selected, onClick, onRemove, removeLabel, children, icon }: { selected?: boolean; onClick?: () => void; onRemove?: () => void; removeLabel?: string; children: ReactNode; icon?: ReactNode }) {
  if (onRemove) {
    return <span className="chip tag">{icon}{children}<button type="button" aria-label={removeLabel || 'Remove'} onClick={onRemove}><IconClose /></button></span>
  }
  return <button type="button" className={'chip' + (selected ? ' active' : '')} aria-pressed={!!selected} onClick={onClick}>{icon}{children}</button>
}
