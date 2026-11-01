import { useEffect, useRef, useState } from 'react'
import { IconClose } from '../icons'

// Small messages about something that has just happened. They wait their turn,
// never take focus, and pause while the pointer or focus is on them. Something
// the person must act on belongs in a dialog or on the page instead.
//
//   toast('Library saved')
//   toast('Could not save', { tone: 'danger' })
//   toast('Theme removed', { action: { label: 'Undo', run: restore } })

export type ToastOptions = { tone?: 'neutral' | 'ok' | 'warn' | 'danger'; action?: { label: string; run: () => void }; ms?: number }
type Shown = ToastOptions & { id: number; text: string }

let push: ((t: Shown) => void) | null = null
let nextId = 1

export const toast = (text: string, options?: ToastOptions) => push?.({ ...options, id: nextId++, text })

const READ_TIME = 3000
const WITH_ACTION = 7000

export function ToastHost() {
  const [queue, setQueue] = useState<Shown[]>([])
  const [paused, setPaused] = useState(false)
  useEffect(() => {
    push = (t) => setQueue((q) => (q[q.length - 1]?.text === t.text ? q : [...q, t]))
    return () => { push = null }
  }, [])
  const cur = queue[0]
  const left = useRef(0)
  useEffect(() => { left.current = cur ? cur.ms ?? (cur.action ? WITH_ACTION : READ_TIME) : 0 }, [cur?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!cur || paused) return
    const began = Date.now()
    const t = setTimeout(() => setQueue((q) => q.slice(1)), left.current)
    return () => { clearTimeout(t); left.current -= Date.now() - began }
  }, [cur?.id, paused]) // eslint-disable-line react-hooks/exhaustive-deps
  const done = () => setQueue((q) => q.slice(1))
  // Always mounted so screen readers announce new messages.
  return (
    <div className={cur ? 'toast' + (cur.tone && cur.tone !== 'neutral' ? ' ' + cur.tone : '') : 'sr-only'} role="status" aria-live="polite"
      onPointerEnter={() => setPaused(true)} onPointerLeave={() => setPaused(false)} onFocus={() => setPaused(true)} onBlur={() => setPaused(false)}>
      {cur && <>
        <span className="toast-text">{cur.text}</span>
        {cur.action && <button type="button" className="linkish" onClick={() => { cur.action!.run(); done() }}>{cur.action.label}</button>}
        {(cur.action || queue.length > 1) && <button type="button" className="toast-x" aria-label="Dismiss" onClick={done}><IconClose size={16} /></button>}
      </>}
    </div>
  )
}
