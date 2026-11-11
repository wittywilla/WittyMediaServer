// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMemo, useState, type ReactNode } from 'react'
import { IconSortDown, IconSortUp } from '../icons'

// A table of records. A column with sort can be ordered by its header, and the
// header says how it's ordered. On phones each row stacks into a small card with
// the column's name beside each value, so nothing needs sideways scrolling.

export type Column<T> = {
  key: string
  header: string
  cell: (row: T) => ReactNode
  sort?: (a: T, b: T) => number
  align?: 'start' | 'end'
  width?: string
}

export function DataTable<T>({ columns, rows, rowKey, caption, empty, initialSort, compact }: {
  columns: Column<T>[]; rows: T[]; rowKey: (row: T) => string | number; caption: string; empty?: ReactNode; initialSort?: { key: string; desc?: boolean }; compact?: boolean
}) {
  const [sort, setSort] = useState(initialSort || null)
  const sorted = useMemo(() => {
    const col = sort && columns.find((c) => c.key === sort.key)
    if (!col?.sort) return rows
    const out = [...rows].sort(col.sort)
    return sort!.desc ? out.reverse() : out
  }, [rows, columns, sort])
  if (!rows.length && empty) return <>{empty}</>
  const toggle = (key: string) => setSort((s) => (s?.key === key ? { key, desc: !s.desc } : { key }))
  return (
    <div className={'ui-table-wrap' + (compact ? ' compact' : '')}>
      <table className="ui-table">
        <caption className="sr-only">{caption}</caption>
        <thead>
          <tr>
            {columns.map((c) => {
              const on = sort?.key === c.key
              return (
                <th key={c.key} scope="col" className={c.align === 'end' ? 'end' : undefined} style={c.width ? { width: c.width } : undefined} aria-sort={on ? (sort!.desc ? 'descending' : 'ascending') : c.sort ? 'none' : undefined}>
                  {c.sort
                    ? <button type="button" className="ui-table-sort" onClick={() => toggle(c.key)}>{c.header}{on && (sort!.desc ? <IconSortDown size={14} /> : <IconSortUp size={14} />)}</button>
                    : c.header}
                </th>
              )
            })}
          </tr>
        </thead>
        <tbody>
          {sorted.map((r) => (
            <tr key={rowKey(r)}>
              {columns.map((c) => <td key={c.key} className={c.align === 'end' ? 'end' : undefined} data-label={c.header}>{c.cell(r)}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
