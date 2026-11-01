import { useEffect, useRef, useState } from 'react'
import { api } from '../../api'
import { IconDown, IconUp } from '../../icons'
import type { Category } from '../../metadata'

// Arranging a library's categories: rename, hide, merge and reorder. They're worked out
// from genres, so only these changes are kept, and Start again forgets them.

export default function CategoryEditor({ library, onClose, onChanged }: { library: number; onClose: () => void; onChanged: () => void }) {
  const [list, setList] = useState<Category[] | null>(null)
  const [err, setErr] = useState('')
  const load = () => api.get<{ categories: Category[] }>(`/browse/shelves?library=${library}&all=1`).then((r) => setList(r.categories)).catch((e) => setErr(e.message))
  useEffect(() => { load() }, [library]) // eslint-disable-line react-hooks/exhaustive-deps
  const send = async (body: object) => {
    setErr('')
    try {
      const r = await api.put<{ categories: Category[] }>('/browse/categories', { library, ...body })
      setList(r.categories)
      onChanged()
    } catch (e: any) { setErr(e.message) }
  }
  const move = (i: number, d: number) => {
    if (!list) return
    const order = list.map((c) => c.key)
    const [k] = order.splice(i, 1)
    order.splice(i + d, 0, k)
    send({ order })
  }
  const pressed = useRef(false)
  return (
    <div className="modal-backdrop" onMouseDown={(e) => { pressed.current = e.target === e.currentTarget }} onClick={(e) => { if (e.target === e.currentTarget && (pressed.current || e.detail === 0)) onClose() }}>
      <div className="modal wide lib-cat-editor" role="dialog" aria-modal="true" aria-labelledby="cats-h">
        <h2 id="cats-h">Categories</h2>
        <p className="muted">Made from the genres of what's in this library. Spellings of the same genre are already put together. Changes here are for this library alone.</p>
        {!list ? <div className="loading-box"><div className="spinner" /></div> : list.length === 0 ? <p className="muted">There are no genres here yet. They come from matching titles.</p> : (
          <ol className="lib-cat-list">
            {list.map((c, i) => (
              <li key={c.key} className={c.hidden ? 'off' : ''}>
                <div className="lib-cat-move">
                  <button type="button" className="btn sm ghost square" disabled={i === 0} aria-label={`Move ${c.name} up`} onClick={() => move(i, -1)}><IconUp /></button>
                  <button type="button" className="btn sm ghost square" disabled={i === list.length - 1} aria-label={`Move ${c.name} down`} onClick={() => move(i, 1)}><IconDown /></button>
                </div>
                <input className="input" aria-label={`Name of ${c.name}`} defaultValue={c.name} maxLength={60}
                  onBlur={(e) => { const v = e.target.value.trim(); if (v && v !== c.name) send({ changes: [{ key: c.key, name: v }] }) }} />
                <span className="muted lib-cat-count">{c.count}</span>
                <select className="select" aria-label={`Merge ${c.name} into another category`} value="" onChange={(e) => { if (e.target.value) send({ changes: [{ key: c.key, mergeInto: e.target.value }] }) }}>
                  <option value="">Merge into…</option>
                  {list.filter((o) => o.key !== c.key).map((o) => <option key={o.key} value={o.key}>{o.name}</option>)}
                </select>
                {c.merged?.length ? <button type="button" className="btn sm ghost" onClick={() => send({ changes: c.merged!.map((k) => ({ key: k, mergeInto: '' })) })}>Split {c.merged.length} back out</button> : null}
                <button type="button" className="btn sm" aria-pressed={c.hidden} onClick={() => send({ changes: [{ key: c.key, hidden: !c.hidden }] })}>{c.hidden ? 'Show' : 'Hide'}</button>
              </li>
            ))}
          </ol>
        )}
        {err && <div className="error" role="alert">{err}</div>}
        <div className="actions">
          <button type="button" className="btn ghost" onClick={() => send({ reset: true })}>Start again</button>
          <button type="button" className="btn primary" onClick={onClose}>Done</button>
        </div>
      </div>
    </div>
  )
}
