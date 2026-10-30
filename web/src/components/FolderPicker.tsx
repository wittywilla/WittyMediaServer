import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { api } from '../api'
import { IconAudio, IconBook, IconCheck, IconClose, IconDisc, IconDrive, IconFile, IconFilm, IconFolder, IconHome, IconImage, IconLeft, IconLibrary, IconRefresh, IconRight, IconSearch, IconSubs, IconUp } from '../icons'
import { useApp } from '../state'

// Choosing folders on the server. "Choose folder…" opens the operating
// system's own folder window when the browser runs on the server's machine
// (Explorer, Finder, the desktop's dialog). "Browse folders" opens the
// built-in browser, which lists the server's drives and folders in the page
// and so works from any device: a phone, a TV, another computer.
export function ChooseFolderButton({ onPick, label = 'Choose folder…', className = 'btn' }: { onPick: (path: string) => void; label?: string; className?: string }) {
  const { pickerLocal } = useApp()
  const [browse, setBrowse] = useState(false)
  const [busy, setBusy] = useState(false)
  const choose = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ path?: string; cancelled?: boolean }>('/fs/pick')
      if (r.path) onPick(r.path)
    } catch {
      setBrowse(true) // no desktop (service, container): browse in the page instead
    } finally { setBusy(false) }
  }
  return (
    <>
      {pickerLocal && (
        <button type="button" className={className} onClick={choose} disabled={busy} title="Opens this computer's own folder window">
          <IconFolder /> {busy ? 'Waiting for the folder window…' : label}
        </button>
      )}
      <button type="button" className={pickerLocal ? 'btn' : className} onClick={() => setBrowse(true)}
        title="Browse the server's drives and folders from this device">
        <IconDrive /> {pickerLocal ? 'Browse folders' : label.replace('Choose', 'Browse')}
      </button>
      {browse && <FileExplorer onClose={() => setBrowse(false)} onPick={(ps) => { ps.forEach(onPick); setBrowse(false) }} />}
    </>
  )
}

type Entry = { name: string; path: string; dir: boolean; kind?: string; disc?: 'dvd' | 'bluray'; size?: number; modified?: number; hidden?: boolean }
type Place = { name: string; path: string; group: 'drives' | 'folders' | 'libraries'; label?: string; free?: number; total?: number }
type Listing = { path: string; parent: string | null; sep: string; entries?: Entry[]; places?: Place[]; counts?: Record<string, number>; truncated?: boolean }

const KIND_ICON: Record<string, React.ReactNode> = {
  video: <IconFilm />, audio: <IconAudio />, book: <IconBook />, disc: <IconDisc />, subtitle: <IconSubs />, image: <IconImage />,
}
const KIND_NAME: Record<string, string> = { video: 'Video', audio: 'Audio', book: 'Book', disc: 'Disc image', subtitle: 'Subtitles', image: 'Picture', other: 'File' }
const COUNT_NAMES: [string, string, string][] = [
  ['video', 'video', 'videos'], ['disc', 'disc image', 'disc images'], ['audio', 'audio file', 'audio files'],
  ['book', 'book', 'books'], ['folder', 'folder', 'folders'],
]

const fmtSize = (b?: number) => {
  if (b == null) return ''
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0, n = b
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
  return `${n >= 10 || i === 0 ? Math.round(n) : n.toFixed(1)} ${u[i]}`
}
const fmtDate = (t?: number) => (t ? new Date(t * 1000).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }) : '')

const RECENT_KEY = 'wms.fs.recent'
const recentFolders = (): string[] => { try { return JSON.parse(localStorage.getItem(RECENT_KEY) || '[]').slice(0, 6) } catch { return [] } }
const rememberFolders = (ps: string[]) => { try { localStorage.setItem(RECENT_KEY, JSON.stringify([...new Set([...ps, ...recentFolders()])].slice(0, 6))) } catch { /* ignore */ } }

// FileExplorer: the built-in folder browser. Read-only; it lists the
// server's drives, folders and files. Pick the folder you are in, or tick
// several folders and add them all.
export function FileExplorer({ onPick, onClose, multiple = true }: { onPick: (paths: string[]) => void; onClose: () => void; multiple?: boolean }) {
  const [hist, setHist] = useState<{ list: string[]; at: number }>({ list: [''], at: 0 })
  const cur = hist.list[hist.at]
  const [data, setData] = useState<Listing | null>(null)
  const [places, setPlaces] = useState<Place[]>([])
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [filter, setFilter] = useState('')
  const [showFiles, setShowFiles] = useState(true)
  const [showHidden, setShowHidden] = useState(false)
  const [sort, setSort] = useState<'name' | 'modified' | 'size'>('name')
  const [picked, setPicked] = useState<string[]>([])
  const [focus, setFocus] = useState(0)
  const [editing, setEditing] = useState(false)
  const [typed, setTyped] = useState('')
  const [reload, setReload] = useState(0)
  const list = useRef<HTMLDivElement | null>(null)
  const search = useRef<HTMLInputElement | null>(null)

  const go = useCallback((p: string) => {
    setHist((h) => (h.list[h.at] === p ? h : { list: [...h.list.slice(0, h.at + 1), p], at: h.at + 1 }))
    setFilter(''); setFocus(0); setEditing(false)
  }, [])
  useEffect(() => { api.get<Listing>('/fs').then((d) => setPlaces(d.places || [])).catch(() => {}) }, [])
  useEffect(() => {
    setLoading(true); setErr('')
    api.get<Listing>(`/fs?path=${encodeURIComponent(cur)}${showHidden ? '&hidden=1' : ''}`)
      .then((d) => { setData(d); if (!cur) setPlaces(d.places || []) })
      .catch((e) => setErr(e.message))
      .finally(() => setLoading(false))
  }, [cur, showHidden, reload])
  useEffect(() => {
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape' && !editing) onClose() }
    window.addEventListener('keydown', esc)
    return () => window.removeEventListener('keydown', esc)
  }, [onClose, editing])

  const rows = useMemo(() => {
    let es = (data?.entries || []).filter((e) => e.dir || showFiles)
    const f = filter.trim().toLowerCase()
    if (f) es = es.filter((e) => e.name.toLowerCase().includes(f))
    const by = (a: Entry, b: Entry) => sort === 'modified' ? (b.modified || 0) - (a.modified || 0) : sort === 'size' ? (b.size || 0) - (a.size || 0) : a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' })
    return [...es].sort((a, b) => (a.dir !== b.dir ? (a.dir ? -1 : 1) : by(a, b)))
  }, [data, filter, showFiles, sort])

  // The path as clickable pieces: "D:\ › Media › Movies".
  const crumbs = useMemo(() => {
    if (!data?.path) return []
    const sep = data.sep || '/'
    const out: { name: string; path: string }[] = []
    let p = data.path
    while (p) {
      const parentIdx = p.replace(/[\\/]+$/, '').lastIndexOf(sep)
      const isRoot = parentIdx < 0 || p.length <= 3 && /^[A-Za-z]:/.test(p) || p === '/' || /^\\\\[^\\]+\\[^\\]+\\?$/.test(p)
      out.unshift({ name: isRoot ? p.replace(/[\\]+$/, '') || '/' : p.slice(parentIdx + 1).replace(/[\\/]+$/, ''), path: p })
      if (isRoot) break
      const up = p.slice(0, parentIdx) || '/'
      p = /^[A-Za-z]:$/.test(up) ? up + '\\' : up
    }
    return out
  }, [data])

  const toggle = (p: string) => setPicked((x) => (x.includes(p) ? x.filter((y) => y !== p) : multiple ? [...x, p] : [p]))
  const finish = (ps: string[]) => { if (ps.length) { rememberFolders(ps); onPick(ps) } }
  const open = (e: Entry) => { if (e.dir) go(e.path) }
  const onKey = (e: React.KeyboardEvent) => {
    if (editing || (e.target as HTMLElement).tagName === 'INPUT') return
    const n = rows.length
    if (e.key === 'ArrowDown') { e.preventDefault(); setFocus((i) => Math.min(n - 1, i + 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setFocus((i) => Math.max(0, i - 1)) }
    else if (e.key === 'Enter' && rows[focus]) { e.preventDefault(); open(rows[focus]) }
    else if (e.key === ' ' && rows[focus]?.dir) { e.preventDefault(); toggle(rows[focus].path) }
    else if ((e.key === 'Backspace' || (e.altKey && e.key === 'ArrowLeft')) && data?.parent != null) { e.preventDefault(); go(data.parent) }
    else if (e.key === '/' || (e.ctrlKey && e.key === 'f')) { e.preventDefault(); search.current?.focus() }
    else if (e.ctrlKey && e.key === 'l') { e.preventDefault(); setTyped(data?.path || ''); setEditing(true) }
  }
  useEffect(() => { list.current?.querySelector<HTMLElement>(`[data-i="${focus}"]`)?.scrollIntoView({ block: 'nearest' }) }, [focus])

  const counts = data?.counts || {}
  const summary = COUNT_NAMES.filter(([k]) => counts[k]).map(([k, one, many]) => `${counts[k]} ${counts[k] === 1 ? one : many}`)
  const recent = recentFolders()
  const groups: [Place['group'], string][] = [['drives', 'Drives'], ['folders', 'Your folders'], ['libraries', 'In your libraries']]
  const placeIcon = (g: Place['group']) => (g === 'drives' ? <IconDrive /> : g === 'libraries' ? <IconLibrary /> : <IconFolder />)

  return createPortal(
    <div className="modal-backdrop" onClick={onClose}>
      <div className="fx" role="dialog" aria-modal="true" aria-label="Browse the server's folders" onClick={(e) => e.stopPropagation()} onKeyDown={onKey}>
        <header className="fx-head">
          <div className="fx-title"><b>Browse folders</b><span className="muted">on the server</span></div>
          <button type="button" className="btn sm ghost square" onClick={onClose} aria-label="Close"><IconClose /></button>
        </header>

        <div className="fx-bar">
          <div className="fx-nav">
            <button type="button" className="btn sm ghost square" disabled={hist.at === 0} onClick={() => setHist((h) => ({ ...h, at: h.at - 1 }))} aria-label="Back" title="Back"><IconLeft /></button>
            <button type="button" className="btn sm ghost square" disabled={hist.at >= hist.list.length - 1} onClick={() => setHist((h) => ({ ...h, at: h.at + 1 }))} aria-label="Forward" title="Forward"><IconRight /></button>
            <button type="button" className="btn sm ghost square" disabled={data?.parent == null && !cur} onClick={() => go(data?.parent ?? '')} aria-label="Up one folder" title="Up one folder (Backspace)"><IconUp /></button>
            <button type="button" className="btn sm ghost square" onClick={() => setReload((n) => n + 1)} aria-label="Refresh" title="Refresh"><IconRefresh /></button>
          </div>
          {editing ? (
            <form className="fx-path editing" onSubmit={(e) => { e.preventDefault(); go(typed.trim()) }}>
              <input className="input" autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} onBlur={() => setEditing(false)}
                onKeyDown={(e) => { if (e.key === 'Escape') { e.stopPropagation(); setEditing(false) } }}
                placeholder="Type a folder, e.g. D:\Movies, /mnt/media or \\NAS\Media" aria-label="Folder path" spellCheck={false} />
            </form>
          ) : (
            <div className="fx-path" onClick={(e) => { if (e.target === e.currentTarget) { setTyped(data?.path || ''); setEditing(true) } }} title="Click to type a path (Ctrl+L)">
              <button type="button" className="fx-crumb" onClick={() => go('')}><IconHome /> <span>This server</span></button>
              {crumbs.map((c) => <span key={c.path} className="fx-crumb-wrap"><span className="fx-sep">›</span><button type="button" className="fx-crumb" onClick={() => go(c.path)}>{c.name}</button></span>)}
            </div>
          )}
          <label className="fx-search"><IconSearch /><input ref={search} value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Filter this folder" aria-label="Filter this folder" /></label>
        </div>

        <div className="fx-body">
          <nav className="fx-places" aria-label="Places">
            {groups.map(([g, title]) => {
              const ps = places.filter((p) => p.group === g)
              if (!ps.length) return null
              return (
                <div key={g} className="fx-group">
                  <div className="fx-group-title">{title}</div>
                  {ps.map((p) => (
                    <button type="button" key={p.path} className={'fx-place' + (data?.path === p.path ? ' on' : '')} onClick={() => go(p.path)} title={p.path}>
                      {placeIcon(g)}
                      <span className="fx-place-txt">
                        <span>{p.label ? `${p.label} (${p.name})` : p.name}</span>
                        {p.total ? <span className="fx-usage" title={`${fmtSize(p.free)} free of ${fmtSize(p.total)}`}><i style={{ width: `${Math.round(100 - ((p.free || 0) / p.total) * 100)}%` }} /></span> : null}
                        {p.total ? <span className="muted">{fmtSize(p.free)} free</span> : null}
                      </span>
                    </button>
                  ))}
                </div>
              )
            })}
            {recent.length > 0 && (
              <div className="fx-group">
                <div className="fx-group-title">Recently chosen</div>
                {recent.map((p) => <button type="button" key={p} className="fx-place" onClick={() => go(p)} title={p}><IconFolder /><span className="fx-place-txt"><span>{p.split(/[\\/]/).filter(Boolean).pop() || p}</span></span></button>)}
              </div>
            )}
          </nav>

          <div className="fx-main">
            {cur && (
              <div className="fx-tools">
                <span className="muted fx-summary" aria-live="polite">{loading ? 'Loading…' : summary.length ? summary.join(' · ') : 'Empty folder'}{data?.truncated ? ' · showing the first 3,000' : ''}</span>
                <label className="fx-check"><input type="checkbox" checked={showFiles} onChange={(e) => setShowFiles(e.target.checked)} /> Files</label>
                <label className="fx-check"><input type="checkbox" checked={showHidden} onChange={(e) => setShowHidden(e.target.checked)} /> Hidden</label>
                <select className="select sm" value={sort} onChange={(e) => setSort(e.target.value as typeof sort)} aria-label="Sort by">
                  <option value="name">Name</option><option value="modified">Date modified</option><option value="size">Size</option>
                </select>
              </div>
            )}
            {err && <div className="error" role="alert">{err}</div>}
            {!cur ? (
              <div className="fx-start">
                <p className="muted">Pick a drive or folder on the left, or click the path bar and type one. Network folders work too, if the server can reach them (for example <code>\\NAS\Media</code> or <code>/mnt/nas</code>).</p>
                <div className="fx-tiles">
                  {places.filter((p) => p.group === 'drives').map((p) => (
                    <button type="button" key={p.path} className="fx-tile" onClick={() => go(p.path)}>
                      <IconDrive /><b>{p.label ? `${p.label} (${p.name})` : p.name}</b>
                      {p.total ? <><span className="fx-usage"><i style={{ width: `${Math.round(100 - ((p.free || 0) / p.total) * 100)}%` }} /></span><span className="muted">{fmtSize(p.free)} free of {fmtSize(p.total)}</span></> : null}
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              <div className="fx-list" ref={list} role="listbox" aria-multiselectable={multiple} tabIndex={0} aria-label="Folders and files">
                <div className="fx-row fx-row-head" aria-hidden="true"><span /><span>Name</span><span>Modified</span><span>Size</span></div>
                {rows.map((e, i) => {
                  const on = picked.includes(e.path)
                  return (
                    <div key={e.path} data-i={i} role="option" aria-selected={on} className={'fx-row' + (e.dir ? ' dir' : ' file') + (i === focus ? ' focus' : '') + (on ? ' picked' : '') + (e.hidden ? ' hidden' : '')}
                      onClick={() => { setFocus(i); if (e.dir) toggle(e.path) }} onDoubleClick={() => open(e)}>
                      <span className="fx-ic">
                        {e.dir
                          ? <span className={'fx-tick' + (on ? ' on' : '')} aria-hidden="true">{on ? <IconCheck /> : <IconFolder />}</span>
                          : (KIND_ICON[e.kind || ''] || <IconFile />)}
                      </span>
                      <span className="fx-name">
                        {e.dir ? <button type="button" className="fx-open" onClick={(ev) => { ev.stopPropagation(); open(e) }} title="Open this folder">{e.name}</button> : <span>{e.name}</span>}
                        {e.disc && <span className="fx-badge">{e.disc === 'dvd' ? 'DVD' : 'Blu-ray'}</span>}
                        {!e.dir && e.kind && e.kind !== 'other' && <span className="fx-kind">{KIND_NAME[e.kind]}</span>}
                      </span>
                      <span className="fx-date muted">{fmtDate(e.modified)}</span>
                      <span className="fx-size muted">{e.dir ? '' : fmtSize(e.size)}</span>
                    </div>
                  )
                })}
                {!loading && rows.length === 0 && <div className="fx-empty muted">{filter ? `Nothing here matches “${filter}”.` : 'No folders here. You can still use this folder.'}</div>}
              </div>
            )}
          </div>
        </div>

        <footer className="fx-foot">
          <span className="muted fx-hint">{picked.length
            ? `${picked.length} folder${picked.length === 1 ? '' : 's'} ticked`
            : cur ? 'Click a folder to tick it, or open it to look inside.' : ''}</span>
          <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
          {picked.length > 0
            ? <button type="button" className="btn primary" onClick={() => finish(picked)}>{picked.length === 1 ? 'Use ticked folder' : `Use ${picked.length} ticked folders`}</button>
            : <button type="button" className="btn primary" disabled={!data?.path} onClick={() => finish([data!.path])}>Use this folder</button>}
        </footer>
      </div>
    </div>,
    document.body,
  )
}
