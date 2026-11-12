// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { useApp } from '../../state'
import { Dialog, Sheet } from '../../ui/Overlay'
import { Dock, type DockAction } from '../../ui/Dock'
import { Select } from '../../ui/Field'
import { KeyLegend } from '../../ui/KeyLegend'
import { fieldOf, type View } from '../model'
import { getList, letterOf, setList } from '../gridView'
import type { Node, Props } from '../types'
import { usePage } from './context'
import { sduiIcon } from './icons'

const find = (n: Node, id: string): Node | undefined => {
  if (n.id === id) return n
  for (const c of n.children || []) { const hit = find(c, id); if (hit) return hit }
  return undefined
}
const findType = (n: Node, type: string): Node | undefined => {
  if (n.type === type) return n
  for (const c of n.children || []) { const hit = findType(c, type); if (hit) return hit }
  return undefined
}
const LETTERS = ['#', ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ']
// At most this many pages are asked for on the way to a letter (docs/SDUI.md, `control.open`).
const MAX_PAGES = 20

// A Dock from the server: its own Buttons (sort, filter, view, jump, or any the page adds) in one row, with the key
// legend at the end. `control.open` is this client's: it opens the page's own choices for the grid it names.
export function DockView({ view }: { view: View }) {
  const page = usePage()
  const { user } = useApp()
  const who = user?.id ?? 0
  const [open, setOpen] = useState<'' | 'sort' | 'filter' | 'jump'>('')
  const [sheet, setSheet] = useState(false)
  const [want, setWant] = useState<{ letter: string; grid: string; pages: number } | null>(null)
  const root = page.doc.root
  const bar = findType(root, 'FilterBar')
  const legend = view.children.some((c) => c.type === 'KeyLegend') || view.node.props?.legend !== false
  const control = (a: { op?: string; params?: Record<string, unknown> } | undefined) => {
    const c = String(a?.params?.control || ''), grid = String(a?.params?.target || '')
    if (c === 'sort' || c === 'filter') setOpen(c)
    else if (c === 'jump') setOpen('jump')
    else if (c === 'view') setList(page.doc.page.id, grid, who, !getList(page.doc.page.id, grid, who))
  }
  const actions: DockAction[] = view.children.filter((c) => c.type === 'Button').map((b) => {
    const a = b.node.action
    const p = b.node.props || {}
    return {
      id: b.id, label: String(p.label || ''), icon: p.icon ? sduiIcon(p.icon) : undefined, primary: p.variant === 'primary',
      onClick: () => { if (a?.type === 'command' && a.op === 'control.open') control(a); else page.act(a, b) },
    }
  })

  // Jump to a letter: ask for pages until a title with it has come, then go to it.
  const grid = want && find(root, want.grid)
  const busy = want ? page.busy(want.grid) : false
  const moved = useRef(false)
  useEffect(() => {
    if (!want || !grid || busy) return
    const hit = (grid.children || []).find((c) => letterOf(String((c.props as Props | undefined)?.title || '')) === want.letter)
    if (hit) {
      const el = document.querySelector<HTMLElement>(`[data-card-id="${CSS.escape(hit.id)}"]`)
      setWant(null)
      if (el) { el.scrollIntoView({ block: 'center' }); el.focus({ preventScroll: true }) }
      return
    }
    const more = !!(grid.props as Props | undefined)?.more
    if (!more || want.pages >= MAX_PAGES) { setWant(null); return }
    moved.current = true
    setWant({ ...want, pages: want.pages + 1 })
    page.more(want.grid)
  }, [want, grid, busy, page.doc]) // eslint-disable-line react-hooks/exhaustive-deps

  const targetGrid = String(view.node.props?.target || '')
  const sortField = bar && (bar.props as Props | undefined)?.sort ? fieldOf((bar.props as Props).sort as Props, 'sort') : null
  const fields = bar ? (Array.isArray((bar.props as Props | undefined)?.fields) ? ((bar.props as Props).fields as Props[]) : []).map((f) => fieldOf(f)) : []
  const first = sortField?.options[0]?.value || ''
  return (
    <>
      {page.layout.dock?.placement === 'sheet' ? (
        // A phone has no bar: one Options button, and the same controls in a sheet, without the legend.
        <div className="sdui-dock-sheet">
          <button type="button" className="btn sm" aria-haspopup="dialog" onClick={() => setSheet(true)}>{view.label || 'Options'}</button>
          {sheet && (
            <Sheet title={view.label || 'Options'} onClose={() => setSheet(false)}>
              <div className="sdui-dock-sheet-list">
                {actions.map((a) => <button key={a.id} type="button" className="btn" onClick={() => { setSheet(false); a.onClick?.() }}>{a.icon}{a.label}</button>)}
              </div>
            </Sheet>
          )}
        </div>
      ) : <Dock label={view.label || 'Options'} actions={actions} legend={legend} sticky />}
      {open === 'sort' && sortField && (
        <Dialog title="Sort" size="sm" onClose={() => setOpen('')}>
          <Select aria-label="Sort" value={sortField.value} options={sortField.options} onChange={(e) => { page.setQuery({ sort: e.target.value === first ? '' : e.target.value }); setOpen('') }} />
        </Dialog>
      )}
      {open === 'filter' && (
        <Dialog title="Filter" size="sm" onClose={() => setOpen('')}>
          {fields.map((f) => (
            <label key={f.id} className="sdui-filter"><span>{f.label}</span>
              <Select aria-label={f.label} value={f.value} options={[{ value: '', label: 'Any' }, ...f.options]} onChange={(e) => page.setQuery({ [f.id]: e.target.value })} />
            </label>
          ))}
          {fields.length === 0 && <p className="muted">Nothing to filter by.</p>}
        </Dialog>
      )}
      {open === 'jump' && (
        <Dialog title="Jump to a letter" size="sm" onClose={() => setOpen('')}>
          <div className="sdui-letters" role="group" aria-label="Letters">
            {LETTERS.map((l) => <button key={l} type="button" className="btn sm" onClick={() => { setOpen(''); setWant({ letter: l, grid: targetGrid, pages: 0 }) }}>{l}</button>)}
          </div>
        </Dialog>
      )}
    </>
  )
}

// A KeyLegend the server sends on its own, outside a Dock.
export const KeyLegendView = () => <KeyLegend />
