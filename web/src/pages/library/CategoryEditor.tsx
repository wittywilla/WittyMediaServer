// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { IconDown, IconUp } from '../../icons'
import type { Category } from '../../metadata'
import { Button, Dialog, IconButton } from '../../ui'
import { t } from '../../i18n'

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
  return (
    <Dialog title={t('Categories')} size="wide" className="lib-cat-editor" onClose={onClose}
      description={t('Made from the genres of what’s in this library. Spellings of the same genre are already put together. Changes here are for this library alone.')}
      actions={<>
        <Button variant="ghost" onClick={() => send({ reset: true })}>{t('Start again')}</Button>
        <Button variant="primary" onClick={onClose}>{t('Done')}</Button>
      </>}>
        {!list ? <div className="loading-box"><div className="spinner" /></div> : list.length === 0 ? <p className="muted">{t('There are no genres here yet. They come from matching titles.')}</p> : (
          <ol className="lib-cat-list">
            {list.map((c, i) => (
              <li key={c.key} className={c.hidden ? 'off' : ''}>
                <div className="lib-cat-move">
                  <IconButton size="sm" hideTip disabled={i === 0} label={t('Move {name} up', { name: c.name })} onClick={() => move(i, -1)}><IconUp /></IconButton>
                  <IconButton size="sm" hideTip disabled={i === list.length - 1} label={t('Move {name} down', { name: c.name })} onClick={() => move(i, 1)}><IconDown /></IconButton>
                </div>
                <input className="input" aria-label={t('Name of {name}', { name: c.name })} defaultValue={c.name} maxLength={60}
                  onBlur={(e) => { const v = e.target.value.trim(); if (v && v !== c.name) send({ changes: [{ key: c.key, name: v }] }) }} />
                <span className="muted lib-cat-count">{c.count}</span>
                <select className="select" aria-label={t('Merge {name} into another category', { name: c.name })} value="" onChange={(e) => { if (e.target.value) send({ changes: [{ key: c.key, mergeInto: e.target.value }] }) }}>
                  <option value="">{t('Merge into…')}</option>
                  {list.filter((o) => o.key !== c.key).map((o) => <option key={o.key} value={o.key}>{o.name}</option>)}
                </select>
                {c.merged?.length ? <Button size="sm" variant="ghost" onClick={() => send({ changes: c.merged!.map((k) => ({ key: k, mergeInto: '' })) })}>{t('Split {count} back out', { count: c.merged.length })}</Button> : null}
                <Button size="sm" aria-pressed={c.hidden} onClick={() => send({ changes: [{ key: c.key, hidden: !c.hidden }] })}>{c.hidden ? t('Show') : t('Hide')}</Button>
              </li>
            ))}
          </ol>
        )}
        {err && <div className="error" role="alert">{err}</div>}
    </Dialog>
  )
}
