import { useState } from 'react'
import { api } from '../../api'
import { TokenField } from '../../components/edit/EditDetails'
import RatingField from '../../components/RatingField'
import { undoBatch, type Target } from '../../metadata'
import { toast } from '../../state'
import { Button, Dialog, Field, Input, Select } from '../../ui'
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
    const set: Record<string, string> = {}
    if (year.trim()) set.year = year.trim()
    if (setRatingOn) set.contentRating = rating
    const all = ['title', 'sortTitle', 'year', 'overview', 'tagline', 'genres', 'tags', 'contentRating', 'rating', 'studios', 'network', 'director', 'edition', 'poster', 'backdrop', 'banner', 'logo']
    try {
      const r = await api.post<{ done: number; failed: string[]; batch: string }>('/meta/bulk', {
        titles: titles.map((t) => ({ target: t.target, id: t.id })), set, addGenres, removeGenres, addTags, removeTags,
        lock: lock === 'lock' ? all : [], unlock: lock === 'unlock' ? all : [],
      })
      setDone(r)
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const nothing = !addGenres.length && !removeGenres.length && !addTags.length && !removeTags.length && !year.trim() && !setRatingOn && !lock
  return (
    <Dialog title={`Edit ${titles.length} ${titles.length === 1 ? 'title' : 'titles'} together`} className="lib-bulk-dialog" onClose={done ? onDone : onClose}
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
      {done ? (
        <>
          <p role="status">Changed {done.done}. {done.failed.length ? `${done.failed.length} couldn't be changed.` : ''}</p>
          {done.failed.length > 0 && <ul className="muted">{done.failed.slice(0, 10).map((f) => <li key={f}>{f}</li>)}</ul>}
        </>
      ) : (
        <div className="form full">
          <div className="row2">
            <Field label="Add genres"><TokenField id="bulk-ag" value={addGenres} onChange={setAddGenres} placeholder="Comedy" /></Field>
            <Field label="Take away genres"><TokenField id="bulk-rg" value={removeGenres} onChange={setRemoveGenres} /></Field>
          </div>
          <div className="row2">
            <Field label="Add tags"><TokenField id="bulk-at" value={addTags} onChange={setAddTags} placeholder="Box set" /></Field>
            <Field label="Take away tags"><TokenField id="bulk-rt" value={removeTags} onChange={setRemoveTags} /></Field>
          </div>
          <Field label="Year"><Input inputMode="numeric" maxLength={4} value={year} onChange={(e) => setYear(e.target.value.replace(/\D/g, ''))} /></Field>
          <label className="lib-check"><input type="checkbox" checked={setRatingOn} onChange={(e) => setSetRatingOn(e.target.checked)} /> Set the content rating</label>
          {setRatingOn && <RatingField value={rating} onChange={setRating} />}
          <Field label="Locks">
            <Select value={lock} onChange={(e) => setLock(e.target.value)} options={[{ value: '', label: 'Leave them as they are' }, { value: 'lock', label: 'Lock every field' }, { value: 'unlock', label: 'Unlock every field' }]} />
          </Field>
          {err && <div className="error" role="alert">{err}</div>}
        </div>
      )}
    </Dialog>
  )
}
