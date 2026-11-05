import { useId, useState, type ReactNode } from 'react'
import { IconMore, IconSortDown, IconSortUp } from '../icons'
import { IconButton, RowSheet, Rows } from '../ui'
import { useMedia } from '../shell/useNav'
import './libraryheader.css'

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
  const more = phone && !!actions && (
    <IconButton label={`More for ${typeof title === 'string' ? title : 'this library'}`} hideTip aria-haspopup="dialog" onClick={() => setTools(true)}><IconMore /></IconButton>
  )
  return (
    <header className="lh">
      <div className="lh-bar">
        <div className="lh-name">
          <h1>{title}{sub && <span className="muted"> · {sub}</span>}</h1>
          {count != null && <span className="count">{count}</span>}
        </div>
        {sorts && sorts.length > 0 && (
          <div className="lh-sort">
            <span className="lh-sort-label" id={id}>Sort</span>
            <div className="lh-sort-list" role="group" aria-labelledby={id} data-nav-group>
              {sorts.map((s) => (
                <button key={s.key} type="button" className={'lh-sort-btn' + (s.key === sort ? ' on' : '')} aria-pressed={s.key === sort} onClick={() => onSort?.(s.key, !!s.desc)}>{s.label}</button>
              ))}
            </div>
            {onDirection && current && (
              <IconButton size="sm" onClick={() => onDirection(!desc)} label={desc ? `${current.label}: last to first. Switch to first to last` : `${current.label}: first to last. Switch to last to first`}>
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
      {tools && phone && <RowSheet title={typeof title === 'string' ? title : 'Library'} onClose={() => setTools(false)}><Rows>{actions}</Rows></RowSheet>}
      {children}
    </header>
  )
}
