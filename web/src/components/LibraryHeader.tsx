// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useId, useRef, useState, type ReactNode } from 'react'
import { IconMore, IconSortDown, IconSortUp } from '../icons'
import { IconButton, RowSheet, Rows } from '../ui'
import { useMedia } from '../shell/useNav'
import { useSubBar } from '../shell/useSubBar'
import './libraryheader.css'
import { t, tx } from '../i18n'

// The one header every library page has: a bar that stays in view with its name, how
// many things are in it, how they're sorted and which way, and under it whatever else the
// page offers. Sorting is a row of plain words, not a form, and each kind of library
// lists its own. On a phone, the page's actions (an admin's tools: All films, Fix matches,
// Categories, Scan) go behind one "⋯" beside Filters, as a sheet of rows, rather than
// taking two rows of the screen.

export type SortOption = { key: string; label: string; desc?: boolean }

export default function LibraryHeader({ title, sub, count, sorts, sort, desc, onSort, onDirection, end, tabs, actions, children }: {
  title: ReactNode
  sub?: ReactNode
  count?: ReactNode
  sorts?: SortOption[]
  sort?: string
  desc?: boolean
  onSort?: (key: string, desc: boolean) => void
  // Leave this out where a sort has only one sensible direction.
  onDirection?: (desc: boolean) => void
  // What stays at the end of the bar, such as Filters.
  end?: ReactNode
  tabs?: ReactNode
  actions?: ReactNode
  children?: ReactNode
}) {
  const id = useId()
  const current = sorts?.find((s) => s.key === sort)
  const phone = useMedia('(max-width: 760px)')
  const [tools, setTools] = useState(false)
  const bar = useRef<HTMLDivElement>(null)
  useSubBar(bar)
  const more = phone && !!actions && (
    <IconButton label={typeof title === 'string' ? t('More for {name}', { name: title }) : t('More for this library')} hideTip aria-haspopup="dialog" onClick={() => setTools(true)}><IconMore /></IconButton>
  )
  return (
    <header className="lh">
      <div className="lh-bar" ref={bar}>
        <div className="lh-name">
          <h1>{title}{sub && <span className="muted"> · {sub}</span>}</h1>
          {count != null && <span className="count">{count}</span>}
        </div>
        {sorts && sorts.length > 0 && (
          <div className="lh-sort">
            <span className="lh-sort-label" id={id}>{t('Sort')}</span>
            <div className="lh-sort-list" role="group" aria-labelledby={id} data-nav-group>
              {sorts.map((s) => (
                <button key={s.key} type="button" className={'lh-sort-btn' + (s.key === sort ? ' on' : '')} aria-pressed={s.key === sort} onClick={() => onSort?.(s.key, !!s.desc)}>{tx(s.label)}</button>
              ))}
            </div>
            {onDirection && current && (
              <IconButton size="sm" onClick={() => onDirection(!desc)} label={desc ? t('{sort}: last to first. Switch to first to last', { sort: tx(current.label) }) : t('{sort}: first to last. Switch to last to first', { sort: tx(current.label) })}>
                {desc ? <IconSortDown /> : <IconSortUp />}
              </IconButton>
            )}
          </div>
        )}
        {(end || more) && <div className="lh-end">{end}{more}</div>}
      </div>
      {(tabs || (actions && !phone)) && (
        <div className="lh-more">
          {tabs}
          {!phone && <div className="lh-actions">{actions}</div>}
        </div>
      )}
      {tools && phone && <RowSheet title={typeof title === 'string' ? title : t('Library')} onClose={() => setTools(false)}><Rows>{actions}</Rows></RowSheet>}
      {children}
    </header>
  )
}
