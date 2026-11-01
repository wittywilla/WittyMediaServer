import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { announce } from '../a11y'
import { api } from '../api'
import { IconDown, IconGrip, IconUp } from '../icons'
import { useApp } from '../state'
import { Button, IconButton } from '../ui/Button'
import { Segmented } from '../ui/Choice'
import { Select, Switch } from '../ui/Field'
import { ask } from '../ui/Overlay'
import { toast } from '../ui/Toast'
import { arrange, GROUPS, groupOf, moved, type NavItem, type NavPrefs } from './navModel'
import { navIcon, setOwnerNavPrefs, useNavItems, useNavPrefs } from './useNav'

// Arrange the main menu: drag a row by its handle, or use the arrow keys on it, or
// the Move buttons (which is what a remote has). Each change takes effect at once.
export default function NavCustomize() {
  const { user } = useApp()
  const items = useNavItems()
  const { prefs, mine, owner, setMine } = useNavPrefs()
  const [order, setOrder] = useState<NavItem[]>(() => arrange(items, prefs, true))
  const dragging = useRef<string | null>(null)
  const [held, setHeld] = useState<string | null>(null)
  const list = useRef<HTMLUListElement | null>(null)
  useEffect(() => { if (!dragging.current) setOrder(arrange(items, prefs, true)) }, [items, prefs])
  // "Arrange this menu" in the bar's More menu comes straight here.
  useEffect(() => { if (location.hash === '#navigation') document.getElementById('navigation')?.scrollIntoView() }, [])

  const save = (patch: Partial<NavPrefs>) => setMine({ ...prefs, ...patch }).catch(() => toast('Could not save that', { tone: 'danger' }))
  const saveOrder = (next: NavItem[]) => save({ order: next.map((it) => it.id) })
  const moveTo = (from: number, to: number) => {
    if (to < 0 || to >= order.length || to === from) return
    const next = moved(order, from, to)
    setOrder(next)
    saveOrder(next)
    announce(`${order[from].label} moved to position ${to + 1} of ${order.length}`)
  }
  const gripKeys = (e: KeyboardEvent, i: number) => {
    const to = e.key === 'ArrowUp' ? i - 1 : e.key === 'ArrowDown' ? i + 1 : -1
    if (to < 0 || to >= order.length) return
    e.preventDefault(); e.stopPropagation()
    const target = e.currentTarget as HTMLElement
    moveTo(i, to)
    requestAnimationFrame(() => target.focus({ preventScroll: true }))
  }
  // The rows' places are read when the drag starts: nothing is measured while it moves.
  const places = useRef<{ top: number; bottom: number }[]>([])
  const startDrag = (e: PointerEvent, it: NavItem) => {
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    dragging.current = it.id
    setHeld(it.id)
    places.current = [...(list.current?.children || [])].map((row) => row.getBoundingClientRect())
  }
  const onDrag = (e: PointerEvent) => {
    if (!dragging.current) return
    const over = places.current.findIndex((p) => e.clientY >= p.top && e.clientY < p.bottom)
    const from = order.findIndex((it) => it.id === dragging.current)
    if (over >= 0 && over !== from) setOrder(moved(order, from, over))
  }
  const endDrag = () => {
    if (!dragging.current) return
    dragging.current = null
    setHeld(null)
    saveOrder(order)
  }

  const hidden = prefs.hidden || []
  const setShown = (id: string, on: boolean) => save({ hidden: on ? hidden.filter((h) => h !== id) : [...hidden, id] })
  const shareIt = async () => {
    if (!await ask({ title: 'Use this arrangement for everyone?', body: 'People who have arranged their own menu keep theirs. Everyone else gets this one, and it can be changed again at any time.', confirm: 'Use for everyone' })) return
    const value = { ...prefs, order: order.map((it) => it.id) }
    try {
      await api.put('/settings', { navDefault: JSON.stringify(value) })
      setOwnerNavPrefs(value)
      toast('This is now the default for everyone', { tone: 'ok' })
    } catch (err) { toast((err as Error).message, { tone: 'danger' }) }
  }
  const useDefault = () => setMine(null).then(() => toast('Back to the server’s arrangement', { tone: 'ok' })).catch(() => toast('Could not save that', { tone: 'danger' }))

  return (
    <div className="nav-custom" id="navigation">
      <h3>Arrange the menu</h3>
      <div className="nav-custom-opts">
        <div className="nav-custom-opt"><span>When the top bar is full</span>
          <Segmented label="When the top bar is full" value={prefs.overflow || 'more'} onChange={(v) => save({ overflow: v })}
            options={[{ value: 'more', label: 'Use a More menu' }, { value: 'scroll', label: 'Scroll sideways' }]} />
        </div>
        <div className="nav-custom-opt"><span>When the side menu is folded</span>
          <Segmented label="When the side menu is folded" value={prefs.collapse || 'rail'} onChange={(v) => save({ collapse: v })}
            options={[{ value: 'rail', label: 'Keep the icons' }, { value: 'hide', label: 'Hide it' }]} />
        </div>
        <div className="nav-custom-toggle"><span id="nav-grouped">Group into Watch, Listen, Read and Play</span>
          <Switch labelledBy="nav-grouped" checked={!!prefs.grouped} onChange={(on) => save({ grouped: on })} />
        </div>
      </div>
      <ul className="nav-custom-list" ref={list} aria-label="Menu items, in order">
        {order.map((it, i) => (
          <li key={it.id} className={'nav-custom-row' + (held === it.id ? ' held' : '') + (hidden.includes(it.id) ? ' off' : '')}>
            <button type="button" className="nav-custom-grip" aria-label={`Move ${it.label}. Position ${i + 1} of ${order.length}. Use the up and down arrow keys.`} data-focus-key={'nav-grip-' + it.id}
              onKeyDown={(e) => gripKeys(e, i)} onPointerDown={(e) => startDrag(e, it)} onPointerMove={onDrag} onPointerUp={endDrag} onPointerCancel={endDrag}><IconGrip /></button>
            <span className="nav-custom-name">{navIcon(it)}<span>{it.label}</span></span>
            {prefs.grouped && (
              <Select aria-label={`Group for ${it.label}`} value={groupOf(it, prefs)} onChange={(e) => save({ groups: { ...prefs.groups, [it.id]: e.target.value } })}
                options={[{ value: '', label: 'No group' }, ...GROUPS.map((g) => ({ value: g, label: g }))]} />
            )}
            <span className="nav-custom-move">
              <IconButton label={`Move ${it.label} up`} size="sm" disabled={i === 0} onClick={() => moveTo(i, i - 1)}><IconUp /></IconButton>
              <IconButton label={`Move ${it.label} down`} size="sm" disabled={i === order.length - 1} onClick={() => moveTo(i, i + 1)}><IconDown /></IconButton>
            </span>
            <Switch label={`Show ${it.label}`} checked={!hidden.includes(it.id)} onChange={(on) => setShown(it.id, on)} />
          </li>
        ))}
      </ul>
      <div className="nav-custom-foot">
        {mine && <Button variant="ghost" onClick={useDefault}>{owner.order ? 'Use the server’s arrangement' : 'Start again'}</Button>}
        {user?.isAdmin && <Button onClick={shareIt}>Use for everyone</Button>}
      </div>
    </div>
  )
}
