import { useEffect, useMemo, useState } from 'react'
import { api } from '../../api'

// What this server is made of, and under which licenses. All of it comes from the
// server (GET /licenses), which builds the list from what's really compiled in.

type Group = { id: string; title: string; about: string }
type Component = { id: number; group: string; name: string; version?: string; license: string; url: string; note?: string; hasText: boolean }
type Licenses = { groups: Group[]; components: Component[] }

function Row({ c }: { c: Component }) {
  const [text, setText] = useState<string | null>(null)
  const [open, setOpen] = useState(false)
  const [failed, setFailed] = useState(false)
  const toggle = () => {
    setOpen(!open)
    if (text === null) api.get<{ text: string }>(`/licenses/${c.id}`).then((r) => setText(r.text)).catch(() => setFailed(true))
  }
  return (
    <li className="lic-row">
      <div className="lic-head">
        <span className="lic-name"><a href={c.url} target="_blank" rel="noreferrer">{c.name}</a>{c.version && <span className="muted"> {c.version}</span>}</span>
        <span className="lic-tag">{c.license}</span>
        {c.hasText && <button type="button" className="btn sm ghost" aria-expanded={open} onClick={toggle}>{open ? 'Hide license' : 'Read license'}</button>}
      </div>
      {c.note && <p className="lic-note muted">{c.note}</p>}
      {open && (failed
        ? <p className="lic-note" role="alert">This license couldn’t be loaded.</p>
        : <pre className="lic-text" tabIndex={0}>{text ?? 'Loading…'}</pre>)}
    </li>
  )
}

export default function LicenseList() {
  const [data, setData] = useState<Licenses | null>(null)
  const [failed, setFailed] = useState(false)
  const [find, setFind] = useState('')
  useEffect(() => { api.get<Licenses>('/licenses').then(setData).catch(() => setFailed(true)) }, [])
  const q = find.trim().toLowerCase()
  const shown = useMemo(() => (data?.components || []).filter((c) => !q || c.name.toLowerCase().includes(q) || c.license.toLowerCase().includes(q)), [data, q])
  const totals = useMemo(() => {
    const n = new Map<string, number>()
    for (const c of data?.components || []) if (c.hasText) n.set(c.license, (n.get(c.license) || 0) + 1)
    return [...n.entries()].sort((a, b) => b[1] - a[1])
  }, [data])
  if (failed) return <p className="muted" role="alert">The list of licenses couldn’t be loaded.</p>
  if (!data) return <p className="muted">Loading the list of licenses…</p>
  const built = data.components.filter((c) => c.hasText).length
  return (
    <div className="lic">
      <p className="lic-summary">
        WittyMediaServer itself, and every Witty module, is <b>Apache-2.0</b>. It’s built with {built} open-source libraries,
        all listed here with their full license text.
      </p>
      <ul className="lic-totals" aria-label="Libraries by license">
        {totals.map(([l, n]) => <li key={l} className="lic-tag">{n} {l}</li>)}
      </ul>
      <label className="lic-find">
        <span className="sr-only">Find a component or license</span>
        <input className="input" type="search" placeholder="Find a component or license" value={find} onChange={(e) => setFind(e.target.value)} />
      </label>
      {q && shown.length === 0 && <p className="muted">Nothing here matches.</p>}
      {data.groups.map((g) => {
        const rows = shown.filter((c) => c.group === g.id)
        if (rows.length === 0) return null
        // The long list of libraries waits to be opened; the rest are worth reading as they are.
        return (
          <details key={g.id} className="lic-group" open={!!q || rows.length <= 30}>
            <summary><b>{g.title}</b> <span className="muted">({rows.length})</span></summary>
            <p className="lic-about muted">{g.about}</p>
            <ul className="lic-list">{rows.map((c) => <Row key={c.id} c={c} />)}</ul>
          </details>
        )
      })}
    </div>
  )
}
