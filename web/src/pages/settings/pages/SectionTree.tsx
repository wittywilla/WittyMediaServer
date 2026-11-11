// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The sections of a page as a list that can be put in order, with the ones inside a
// container (a stack, two columns, tabs) set in under it. A section moves three ways, as
// the menu does (sdui/ArrangeList): dragged by its handle, with the arrow keys on the
// handle (up and down among its neighbours, left out of its container, right into the
// container above it), or with the Move buttons and the row's menu, which is what a
// remote and a touch screen have.

import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { announce } from '../../../a11y'
import { IconDown, IconGrip, IconMore, IconUp } from '../../../icons'
import { sduiIcon } from '../../../sdui/render/icons'
import { IconButton, Menu, toast, type MenuEntry } from '../../../ui'
import {
  canHold, componentSpec, duplicateAt, flatten, listIds, moveBy, moveIn, moveOut, moveTo, removeAt, samePath, sectionAt, sectionTitle,
  type Catalogue, type Path, type Section, type TreeRow,
} from './editorModel.ts'

type Drop = { row: number; zone: 'before' | 'after' | 'into' }

export function SectionTree({ cat, sections, selected, onSelect, onChange, onAddInside, marks, label }: {
  cat: Catalogue; sections: Section[]; selected: Path | null; label: string
  onSelect: (path: Path) => void
  // The tree after a change, where the section that was acted on now is, and what to call the change.
  onChange: (sections: Section[], select: Path | null, what: string) => void
  onAddInside: (parent: Path) => void
  // How many problems and warnings a section has, for the mark beside it.
  marks: (path: Path) => { errors: number; warnings: number }
}) {
  const rows = flatten(sections)
  const list = useRef<HTMLUListElement | null>(null)
  const dragging = useRef<number | null>(null)
  const places = useRef<{ top: number; bottom: number }[]>([])
  const [held, setHeld] = useState<number | null>(null)
  const [drop, setDrop] = useState<Drop | null>(null)
  const refocus = useRef<string | null>(null)
  useEffect(() => {
    if (!refocus.current) return
    const key = refocus.current
    refocus.current = null
    list.current?.querySelector<HTMLElement>(`[data-tree-key="${CSS.escape(key)}"]`)?.focus({ preventScroll: true })
  })

  const title = (s: Section) => sectionTitle(cat, s)
  const kind = (s: Section) => componentSpec(cat, s.component)?.label || s.component || 'Unknown'
  const parentOf = (r: TreeRow) => (r.path.length > 1 ? sectionAt(sections, r.path.slice(0, -1)) : undefined)

  const done = (got: { sections: Section[]; path: Path } | null, said: string, focus?: string) => {
    if (!got) return false
    if (focus) refocus.current = focus + ':' + got.path.join('.')
    onChange(got.sections, got.path, said)
    announce(said)
    return true
  }
  const place = (path: Path, inList: Section[]) => {
    const parent = path.length > 1 ? sectionAt(inList, path.slice(0, -1)) : undefined
    const size = parent ? (parent.children || []).length : inList.length
    return `position ${path[path.length - 1] + 1} of ${size}${parent ? ` in ${title(parent)}` : ''}`
  }
  const step = (r: TreeRow, delta: number, focus: string) => {
    const got = moveBy(sections, r.path, delta)
    if (got) done(got, `${title(r.section)} moved to ${place(got.path, got.sections)}`, focus)
  }
  const out = (r: TreeRow, focus: string) => {
    const got = moveOut(sections, r.path)
    const from = parentOf(r)
    if (!got || !from) return
    const to = got.path.length > 1 ? sectionAt(got.sections, got.path.slice(0, -1)) : undefined
    if (!canHold(cat, to?.component || '', r.section.component)) { toast(`${kind(r.section)} belongs inside ${kind(from)}, so it can’t be moved out`); return }
    done(got, `${title(r.section)} moved out of ${title(from)}, to ${place(got.path, got.sections)}`, focus)
  }
  const into = (r: TreeRow, focus: string) => {
    const got = moveIn(cat, sections, r.path)
    if (!got) return
    const holder = sectionAt(got.sections, got.path.slice(0, -1))
    done(got, `${title(r.section)} moved into ${holder ? title(holder) : 'the section above'}`, focus)
  }
  const canGoIn = (r: TreeRow) => {
    if (r.index < 1) return false
    const above = sectionAt(sections, [...r.path.slice(0, -1), r.index - 1])
    return !!above && canHold(cat, above.component, r.section.component)
  }

  const gripKeys = (e: KeyboardEvent, r: TreeRow) => {
    const key = e.key
    if (key !== 'ArrowUp' && key !== 'ArrowDown' && key !== 'ArrowLeft' && key !== 'ArrowRight') return
    // At an end the arrow is left alone, so a remote carries on to whatever is beside the list.
    if ((key === 'ArrowUp' && r.index === 0) || (key === 'ArrowDown' && r.index === r.siblings - 1) || (key === 'ArrowLeft' && r.path.length < 2) || (key === 'ArrowRight' && !canGoIn(r))) return
    e.preventDefault(); e.stopPropagation()
    if (key === 'ArrowUp') step(r, -1, 'grip')
    else if (key === 'ArrowDown') step(r, 1, 'grip')
    else if (key === 'ArrowLeft') out(r, 'grip')
    else into(r, 'grip')
  }

  // The rows' places are read when the drag starts: nothing is measured while it moves.
  const startDrag = (e: PointerEvent, i: number) => {
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    dragging.current = i
    setHeld(i)
    places.current = [...(list.current?.children || [])].map((row) => row.getBoundingClientRect())
  }
  const onDrag = (e: PointerEvent) => {
    if (dragging.current === null) return
    const at = places.current.findIndex((p) => e.clientY >= p.top && e.clientY < p.bottom)
    if (at < 0) { setDrop(null); return }
    const p = places.current[at]
    const part = (e.clientY - p.top) / Math.max(1, p.bottom - p.top)
    const holds = !!componentSpec(cat, rows[at].section.component)?.container
    setDrop({ row: at, zone: holds && part > 0.3 && part < 0.7 ? 'into' : part < 0.5 ? 'before' : 'after' })
  }
  const endDrag = () => {
    const from = dragging.current
    dragging.current = null
    setHeld(null)
    const d = drop
    setDrop(null)
    if (from === null || !d || !rows[from] || !rows[d.row]) return
    const moved = rows[from], target = rows[d.row]
    const toParent = d.zone === 'into' ? target.path : target.path.slice(0, -1)
    const toIndex = d.zone === 'into' ? (target.section.children || []).length : d.zone === 'before' ? target.index : target.index + 1
    const holder = toParent.length ? sectionAt(sections, toParent) : undefined
    if (samePath(moved.path, target.path)) return
    if (!canHold(cat, holder?.component || '', moved.section.component)) {
      toast(holder ? `${kind(holder)} can’t hold ${kind(moved.section)}` : `${kind(moved.section)} belongs inside ${kind(parentOf(moved) || moved.section)}`)
      return
    }
    const got = moveTo(sections, moved.path, toParent, toIndex)
    if (got) done(got, `${title(moved.section)} moved to ${place(got.path, got.sections)}`)
  }

  const remove = (r: TreeRow) => {
    const next = removeAt(sections, r.path)
    const said = `${title(r.section)} removed`
    onChange(next, null, said)
    announce(said)
  }
  const menu = (r: TreeRow): MenuEntry[] => {
    const holds = !!componentSpec(cat, r.section.component)?.container
    const from = parentOf(r)
    const items: MenuEntry[] = []
    if (holds) items.push({ key: 'add', label: 'Add a section inside', onSelect: () => onAddInside(r.path) })
    items.push({ key: 'dup', label: 'Duplicate', onSelect: () => { const got = duplicateAt(sections, r.path, listIds(sections)); if (got) done(got, `${title(r.section)} duplicated`) } })
    if (canGoIn(r)) items.push({ key: 'in', label: 'Move into the section above', onSelect: () => into(r, 'more') })
    if (from) items.push({ key: 'out', label: `Move out of ${title(from)}`, onSelect: () => out(r, 'more') })
    items.push({ kind: 'separator', key: 'sep' }, { key: 'del', label: holds && r.section.children?.length ? 'Remove, with what is inside' : 'Remove', danger: true, onSelect: () => remove(r) })
    return items
  }

  if (!rows.length) return null
  return (
    <ul className="pe-tree" ref={list} aria-label={label}>
      {rows.map((r, i) => {
        const key = r.path.join('.')
        const name = title(r.section)
        const on = !!selected && samePath(selected, r.path)
        const m = marks(r.path)
        const where = `Position ${r.index + 1} of ${r.siblings}${r.depth ? `, inside ${title(parentOf(r) || r.section)}` : ''}`
        return (
          <li key={key} className={'pe-tree-row' + (on ? ' on' : '') + (held === i ? ' held' : '') + (drop && drop.row === i && held !== null && held !== i ? ' drop-' + drop.zone : '')} style={{ '--pe-depth': r.depth } as React.CSSProperties}>
            <button type="button" className="nav-custom-grip" data-tree-key={'grip:' + key}
              aria-label={`Move ${name}. ${where}. Up and down arrows move it; left moves it out of its container, right into the one above.`}
              onKeyDown={(e) => gripKeys(e, r)} onPointerDown={(e) => startDrag(e, i)} onPointerMove={onDrag} onPointerUp={endDrag} onPointerCancel={endDrag}><IconGrip /></button>
            <button type="button" className="pe-tree-pick" aria-current={on ? 'true' : undefined} data-tree-key={'pick:' + key} onClick={() => onSelect(r.path)}>
              <span className="pe-tree-icon" aria-hidden="true">{sduiIcon(componentSpec(cat, r.section.component)?.icon)}</span>
              <span className="pe-tree-text"><span className="pe-tree-name">{name}</span><small>{kind(r.section)}{r.section.when && Object.keys(r.section.when).length ? ' · sometimes' : ''}{r.section.repeat?.over ? ' · repeated' : ''}{r.section.locked ? ' · locked' : ''}</small></span>
              {m.errors > 0 && <span className="pe-mark error">{m.errors === 1 ? '1 problem' : `${m.errors} problems`}</span>}
              {m.errors === 0 && m.warnings > 0 && <span className="pe-mark warn">Worth a look</span>}
            </button>
            <span className="pe-tree-move">
              <IconButton label={`Move ${name} up`} size="sm" disabled={r.index === 0} data-tree-key={'up:' + key} onClick={() => step(r, -1, r.index - 1 === 0 ? 'down' : 'up')}><IconUp /></IconButton>
              <IconButton label={`Move ${name} down`} size="sm" disabled={r.index === r.siblings - 1} data-tree-key={'down:' + key} onClick={() => step(r, 1, r.index + 1 === r.siblings - 1 ? 'up' : 'down')}><IconDown /></IconButton>
              <Menu label={`More for ${name}`} items={menu(r)}
                trigger={(p) => <button type="button" className="btn ghost sm square" aria-label={`More for ${name}`} data-tree-key={'more:' + key} {...p}><IconMore /></button>} />
            </span>
          </li>
        )
      })}
    </ul>
  )
}
