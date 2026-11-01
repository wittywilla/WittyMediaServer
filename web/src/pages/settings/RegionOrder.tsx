import { useEffect, useRef, useState } from 'react'
import { IconDown, IconUp } from '../../icons'
import { Section } from './ui'
import './regionorder.css'

// Which version of a game comes first. A list with buttons rather than dragging, so a
// keyboard, a TV remote and a screen reader get the same thing as a mouse.

const REGIONS: { id: string; name: string }[] = [
  { id: 'usa', name: 'USA and Canada' },
  { id: 'europe', name: 'Europe' },
  { id: 'australia', name: 'Australia' },
  { id: 'japan', name: 'Japan' },
  { id: 'asia', name: 'Korea, China and the rest of Asia' },
  { id: 'brazil', name: 'Brazil' },
]

const nameOf = (id: string) => REGIONS.find((r) => r.id === id)?.name || id

// What's saved may leave some out, or be from a version with fewer regions.
function orderOf(saved: string | undefined): string[] {
  const known = REGIONS.map((r) => r.id)
  const out = (saved || '').split(',').map((s) => s.trim()).filter((s, i, all) => known.includes(s) && all.indexOf(s) === i)
  return [...out, ...known.filter((id) => !out.includes(id))]
}

export default function RegionOrder({ saved, onSave }: { saved?: string; onSave: (order: string) => Promise<void> | void }) {
  const [order, setOrder] = useState(() => orderOf(saved))
  const [said, setSaid] = useState('')
  const list = useRef<HTMLOListElement>(null)
  const [moved, setMoved] = useState<{ id: string; by: number } | null>(null)
  // Saved once the moves stop, as each change goes over the games libraries again.
  const waiting = useRef<{ timer: number; order: string } | null>(null)
  const save = useRef(onSave)
  save.current = onSave
  const flush = () => {
    if (!waiting.current) return
    clearTimeout(waiting.current.timer)
    void save.current(waiting.current.order)
    waiting.current = null
  }
  useEffect(() => flush, [])
  // The button that was pressed may have just reached the end of the list and gone out of
  // use, and focus mustn't fall off the page with it.
  useEffect(() => {
    if (!moved) return
    const row = list.current?.querySelector<HTMLElement>(`[data-region="${moved.id}"]`)
    const again = row?.querySelector<HTMLButtonElement>(moved.by < 0 ? '.ro-up' : '.ro-down')
    const other = row?.querySelector<HTMLButtonElement>(moved.by < 0 ? '.ro-down' : '.ro-up')
    ;(again && !again.disabled ? again : other)?.focus()
  }, [moved])
  const move = (id: string, by: number) => {
    const from = order.indexOf(id)
    const to = from + by
    if (to < 0 || to >= order.length) return
    const next = [...order]
    next.splice(from, 1)
    next.splice(to, 0, id)
    setOrder(next)
    setSaid(`${nameOf(id)} moved to number ${to + 1} of ${next.length}.`)
    setMoved({ id, by })
    if (waiting.current) clearTimeout(waiting.current.timer)
    waiting.current = { order: next.join(','), timer: window.setTimeout(flush, 1500) }
  }
  return (
    <Section title="Which version comes first" desc="When there’s more than one version of a game, this decides which one its tile leads with, which file of an archive plays, and whose names and pictures are used. Anywhere not listed comes last.">
      <ol className="ro-list" ref={list}>
        {order.map((id, i) => (
          <li key={id} data-region={id} className="ro-row">
            <span className="ro-num" aria-hidden="true">{i + 1}</span>
            <span className="ro-name">{nameOf(id)}</span>
            <button type="button" className="btn sm ghost ro-up" disabled={i === 0} aria-label={`Move ${nameOf(id)} up`} onClick={() => move(id, -1)}><IconUp size={20} /></button>
            <button type="button" className="btn sm ghost ro-down" disabled={i === order.length - 1} aria-label={`Move ${nameOf(id)} down`} onClick={() => move(id, 1)}><IconDown size={20} /></button>
          </li>
        ))}
      </ol>
      <p className="sr-only" role="status" aria-live="polite">{said}</p>
      <p className="muted">Changing this goes over your games libraries again. A game someone has already played, or given a title of their own, keeps the version it has, as their saves belong to it.</p>
    </Section>
  )
}
