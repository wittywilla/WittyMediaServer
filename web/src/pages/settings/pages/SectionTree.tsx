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
import { t, tn, tx } from '../../../i18n'
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
  const kind = (s: Section) => tx(componentSpec(cat, s.component)?.label) || s.component || t('Unknown')
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
    const at = path[path.length - 1] + 1
    return parent ? t('position {at} of {size} in {parent}', { at, size, parent: title(parent) }) : t('position {at} of {size}', { at, size })
  }
  const step = (r: TreeRow, delta: number, focus: string) => {
    const got = moveBy(sections, r.path, delta)
    if (got) done(got, t('{name} moved to {place}', { name: title(r.section), place: place(got.path, got.sections) }), focus)
  }
  const out = (r: TreeRow, focus: string) => {
    const got = moveOut(sections, r.path)
    const from = parentOf(r)
    if (!got || !from) return
    const to = got.path.length > 1 ? sectionAt(got.sections, got.path.slice(0, -1)) : undefined
    if (!canHold(cat, to?.component || '', r.section.component)) { toast(t('{kind} belongs inside {holder}, so it can’t be moved out', { kind: kind(r.section), holder: kind(from) })); return }
    done(got, t('{name} moved out of {from}, to {place}', { name: title(r.section), from: title(from), place: place(got.path, got.sections) }), focus)
  }
  const into = (r: TreeRow, focus: string) => {
    const got = moveIn(cat, sections, r.path)
    if (!got) return
    const holder = sectionAt(got.sections, got.path.slice(0, -1))
    done(got, t('{name} moved into {holder}', { name: title(r.section), holder: holder ? title(holder) : t('the section above') }), focus)
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
      toast(holder ? t('{holder} can’t hold {kind}', { holder: kind(holder), kind: kind(moved.section) }) : t('{kind} belongs inside {holder}', { kind: kind(moved.section), holder: kind(parentOf(moved) || moved.section) }))
      return
    }
    const got = moveTo(sections, moved.path, toParent, toIndex)
    if (got) done(got, t('{name} moved to {place}', { name: title(moved.section), place: place(got.path, got.sections) }))
  }

  const remove = (r: TreeRow) => {
    const next = removeAt(sections, r.path)
    const said = t('{name} removed', { name: title(r.section) })
    onChange(next, null, said)
    announce(said)
  }
  const menu = (r: TreeRow): MenuEntry[] => {
    const holds = !!componentSpec(cat, r.section.component)?.container
    const from = parentOf(r)
    const items: MenuEntry[] = []
    if (holds) items.push({ key: 'add', label: t('Add a section inside'), onSelect: () => onAddInside(r.path) })
    items.push({ key: 'dup', label: t('Duplicate'), onSelect: () => { const got = duplicateAt(sections, r.path, listIds(sections)); if (got) done(got, t('{name} duplicated', { name: title(r.section) })) } })
    if (canGoIn(r)) items.push({ key: 'in', label: t('Move into the section above'), onSelect: () => into(r, 'more') })
    if (from) items.push({ key: 'out', label: t('Move out of {name}', { name: title(from) }), onSelect: () => out(r, 'more') })
    items.push({ kind: 'separator', key: 'sep' }, { key: 'del', label: holds && r.section.children?.length ? t('Remove, with what is inside') : t('Remove'), danger: true, onSelect: () => remove(r) })
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
        const where = r.depth ? t('Position {at} of {size}, inside {parent}', { at: r.index + 1, size: r.siblings, parent: title(parentOf(r) || r.section) }) : t('Position {at} of {size}', { at: r.index + 1, size: r.siblings })
        return (
          <li key={key} className={'pe-tree-row' + (on ? ' on' : '') + (held === i ? ' held' : '') + (drop && drop.row === i && held !== null && held !== i ? ' drop-' + drop.zone : '')} style={{ '--pe-depth': r.depth } as React.CSSProperties}>
            <button type="button" className="nav-custom-grip" data-tree-key={'grip:' + key}
              aria-label={t('Move {name}. {where}. Up and down arrows move it; left moves it out of its container, right into the one above.', { name, where })}
              onKeyDown={(e) => gripKeys(e, r)} onPointerDown={(e) => startDrag(e, i)} onPointerMove={onDrag} onPointerUp={endDrag} onPointerCancel={endDrag}><IconGrip /></button>
            <button type="button" className="pe-tree-pick" aria-current={on ? 'true' : undefined} data-tree-key={'pick:' + key} onClick={() => onSelect(r.path)}>
              <span className="pe-tree-icon" aria-hidden="true">{sduiIcon(componentSpec(cat, r.section.component)?.icon)}</span>
              <span className="pe-tree-text"><span className="pe-tree-name">{name}</span><small>{kind(r.section)}{r.section.when && Object.keys(r.section.when).length ? ' · ' + t('sometimes') : ''}{r.section.repeat?.over ? ' · ' + t('repeated') : ''}{r.section.locked ? ' · ' + t('locked') : ''}</small></span>
              {m.errors > 0 && <span className="pe-mark error">{tn('{count} problem', '{count} problems', m.errors)}</span>}
              {m.errors === 0 && m.warnings > 0 && <span className="pe-mark warn">{t('Worth a look')}</span>}
            </button>
            <span className="pe-tree-move">
              <IconButton label={t('Move {name} up', { name })} size="sm" disabled={r.index === 0} data-tree-key={'up:' + key} onClick={() => step(r, -1, r.index - 1 === 0 ? 'down' : 'up')}><IconUp /></IconButton>
              <IconButton label={t('Move {name} down', { name })} size="sm" disabled={r.index === r.siblings - 1} data-tree-key={'down:' + key} onClick={() => step(r, 1, r.index + 1 === r.siblings - 1 ? 'up' : 'down')}><IconDown /></IconButton>
              <Menu label={t('More for {name}', { name })} items={menu(r)}
                trigger={(p) => <button type="button" className="btn ghost sm square" aria-label={t('More for {name}', { name })} data-tree-key={'more:' + key} {...p}><IconMore /></button>} />
            </span>
          </li>
        )
      })}
    </ul>
  )
}
