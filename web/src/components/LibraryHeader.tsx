import { useId, type ReactNode } from 'react'
import { IconSortDown, IconSortUp } from '../icons'
import './libraryheader.css'

// The one header every library page has: its name, how many things are in it, how
// they're sorted, which way, and whatever else the page offers. Sorting is a row of plain
// words, not a form, and each kind of library lists its own.

export type SortOption = { key: string; label: string; desc?: boolean }

export default function LibraryHeader({ title, sub, count, sorts, sort, desc, onSort, onDirection, tabs, actions, children }: {
  title: ReactNode
  sub?: ReactNode
  count?: ReactNode
  sorts?: SortOption[]
  sort?: string
  desc?: boolean
  onSort?: (key: string, desc: boolean) => void
  // Leave this out where a sort has only one sensible direction.
  onDirection?: (desc: boolean) => void
  tabs?: ReactNode
  actions?: ReactNode
  children?: ReactNode
}) {
  const id = useId()
  const current = sorts?.find((s) => s.key === sort)
  return (
    <header className="lh">
      <div className="lh-top">
        <div className="lh-name">
          <h1>{title}{sub && <span className="muted"> · {sub}</span>}</h1>
          {count != null && <span className="count">{count}</span>}
        </div>
        {tabs}
        <div className="lh-actions">{actions}</div>
      </div>
      {sorts && sorts.length > 0 && (
        <div className="lh-sort">
          <span className="lh-sort-label" id={id}>Sort</span>
          <div className="lh-sort-list" role="group" aria-labelledby={id}>
            {sorts.map((s) => (
              <button key={s.key} type="button" className={'lh-sort-btn' + (s.key === sort ? ' on' : '')} aria-pressed={s.key === sort} onClick={() => onSort?.(s.key, !!s.desc)}>{s.label}</button>
            ))}
          </div>
          {onDirection && current && (
            <button type="button" className="btn sm ghost square" onClick={() => onDirection(!desc)}
              aria-label={desc ? `${current.label}: last to first. Switch to first to last` : `${current.label}: first to last. Switch to last to first`} title={desc ? 'Last to first' : 'First to last'}>
              {desc ? <IconSortDown /> : <IconSortUp />}
            </button>
          )}
        </div>
      )}
      {children}
    </header>
  )
}
