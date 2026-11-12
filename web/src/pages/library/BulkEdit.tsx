// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMemo, useState } from 'react'
import { api } from '../../api'
import { TokenField } from '../../components/edit/EditDetails'
import RatingField from '../../components/RatingField'
import { undoBatch, type Target } from '../../metadata'
import { toast } from '../../state'
import SpecPage from '../../spec/SpecPage'
import { bulkRequest, bulkSpec, bulkTitle } from '../../spec/specs/bulkedit'
import { Button, Dialog, Field } from '../../ui'
import './library.css'

// Changing many titles at once. Everything can be undone in one go.

export type BulkTitle = { target: Target; id: number; title: string }

export default function BulkEdit({ titles, onClose, onDone }: { titles: BulkTitle[]; onClose: () => void; onDone: () => void }) {
  const [addGenres, setAddGenres] = useState<string[]>([])
  const [removeGenres, setRemoveGenres] = useState<string[]>([])
  const [addTags, setAddTags] = useState<string[]>([])
  const [removeTags, setRemoveTags] = useState<string[]>([])
  const [year, setYear] = useState('')
  const [rating, setRating] = useState('')
  const [setRatingOn, setSetRatingOn] = useState(false)
  const [lock, setLock] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<{ batch: string; done: number; failed: string[] } | null>(null)
  const apply = async () => {
    setBusy(true); setErr('')
    const { set } = bulkRequest({ year, setRating: setRatingOn, rating, lock })
    const all = ['title', 'sortTitle', 'year', 'overview', 'tagline', 'genres', 'tags', 'contentRating', 'rating', 'studios', 'network', 'director', 'edition', 'poster', 'backdrop', 'banner', 'logo']
    try {
      const r = await api.post<{ done: number; failed: string[]; batch: string }>('/meta/bulk', {
        titles: titles.map((t) => ({ target: t.target, id: t.id })), set, addGenres, removeGenres, addTags, removeTags,
        lock: lock === 'lock' ? all : [], unlock: lock === 'unlock' ? all : [],
      })
      setDone(r)
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const nothing = bulkRequest({ year, setRating: setRatingOn, rating, lock }).nothing(!!(addGenres.length || removeGenres.length || addTags.length || removeTags.length))
  const spec = useMemo(() => bulkSpec({ count: titles.length, year, setRating: setRatingOn, lock, error: err, result: done }), [titles.length, year, setRatingOn, lock, err, done])
  const onField = (id: string, v: string | number | boolean | File) => {
    if (id === 'year') setYear(String(v).replace(/\D/g, '').slice(0, 4))
    else if (id === 'setRating') setSetRatingOn(!!v)
    else if (id === 'lock') setLock(String(v))
  }
  return (
    <Dialog title={bulkTitle(titles.length)} className="lib-bulk-dialog" onClose={done ? onDone : onClose}
      description={done ? undefined : "Only what you fill in changes. What you change is locked on each title, so nothing changes it back."}
      actions={done
        ? <>
            <Button variant="ghost" onClick={async () => { try { await undoBatch(done.batch); toast('Undone'); onDone() } catch (e: any) { setErr(e.message) } }}>Undo all of it</Button>
            <Button variant="primary" autoFocus onClick={onDone}>Done</Button>
          </>
        : <>
            <Button variant="ghost" onClick={onClose}>Cancel</Button>
            <Button variant="primary" busy={busy} disabled={nothing} onClick={apply}>Change {titles.length}</Button>
          </>}>
      <SpecPage embedded page={spec} onField={onField} after={done ? undefined : {
        0: (
          <div className="lib-bulk-tokens">
            <Field label="Add genres"><TokenField id="bulk-ag" value={addGenres} onChange={setAddGenres} placeholder="Comedy" /></Field>
            <Field label="Take away genres"><TokenField id="bulk-rg" value={removeGenres} onChange={setRemoveGenres} /></Field>
            <Field label="Add tags"><TokenField id="bulk-at" value={addTags} onChange={setAddTags} placeholder="Box set" /></Field>
            <Field label="Take away tags"><TokenField id="bulk-rt" value={removeTags} onChange={setRemoveTags} /></Field>
          </div>
        ),
        1: setRatingOn ? <RatingField value={rating} onChange={setRating} /> : null,
      }} />
    </Dialog>
  )
}
