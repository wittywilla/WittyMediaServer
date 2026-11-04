import { useId, useMemo, useRef } from 'react'
import { Link } from 'react-router-dom'
import { IconClose, IconSearch } from '../../icons'
import { searchSettings, type Result, type SettingsPage } from './searchIndex'

// Search at the top of the settings menu. While there are words in it, the menu shows what
// they find instead of its pages. Enter goes to the first; the arrows (or Tab) reach the rest,
// and Escape empties it again.
export function SettingsSearch({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const input = useRef<HTMLInputElement | null>(null)
  return (
    <div className="set-search" role="search">
      <IconSearch />
      <input ref={input} type="search" className="input" placeholder="Search settings" aria-label="Search settings" aria-describedby="set-search-count"
        value={value} onChange={(e) => onChange(e.target.value)} enterKeyHint="go"
        onKeyDown={(e) => {
          if (e.key === 'Escape' && value) { e.preventDefault(); e.stopPropagation(); onChange('') }
          if (e.key === 'Enter') { e.preventDefault(); document.querySelector<HTMLAnchorElement>('.set-results a')?.click() }
        }} />
      {value && <button type="button" className="btn icon sm ghost" aria-label="Clear the search" onClick={() => { onChange(''); input.current?.focus() }}><IconClose /></button>}
    </div>
  )
}

export function SettingsResults({ query, pages, extra, admin, onPick }: { query: string; pages: SettingsPage[]; extra: Result[]; admin: boolean; onPick: () => void }) {
  const results = useMemo(() => searchSettings(query, pages, extra, admin), [query, pages, extra, admin])
  const head = useId()
  return (
    <div className="set-results" aria-labelledby={head}>
      <div className="set-nav-head" id={head}>Results</div>
      <p id="set-search-count" className="sr-only" role="status">{results.length ? `${results.length} found` : 'Nothing found'}</p>
      {results.length ? (
        <ul>
          {results.map((r) => (
            <li key={r.to + r.title}>
              <Link to={r.to} className="set-nav-link set-result" onClick={onPick}
                onKeyDown={(e) => { if (e.key === 'Escape') { e.preventDefault(); onPick(); document.querySelector<HTMLInputElement>('.set-search input')?.focus() } }}>
                <span className="set-nav-text"><span>{r.title}</span><small>{r.where}</small></span>
              </Link>
            </li>
          ))}
        </ul>
      ) : (
        <p className="set-results-none">Nothing in settings matches “{query.trim()}”. Try fewer or other words, or <Link to={`/help?q=${encodeURIComponent(query.trim())}`} onClick={onPick}>search Help</Link>.</p>
      )}
    </div>
  )
}
