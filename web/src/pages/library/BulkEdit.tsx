import { useRef, useState } from 'react'
import { api } from '../../api'
import { TokenField } from '../../components/edit/EditDetails'
import RatingField from '../../components/RatingField'
import { undoBatch, type Target } from '../../metadata'
import { toast } from '../../state'

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
  const pressed = useRef(false)
  const nothing = !addGenres.length && !removeGenres.length && !addTags.length && !removeTags.length && !year.trim() && !setRatingOn && !lock
  return (
    <div className="modal-backdrop" onMouseDown={(e) => { pressed.current = e.target === e.currentTarget }} onClick={(e) => { if (e.target === e.currentTarget && (pressed.current || e.detail === 0)) (done ? onDone : onClose)() }}>
      <div className="modal lib-bulk-dialog" role="dialog" aria-modal="true" aria-labelledby="bulk-h">
        <h2 id="bulk-h">Edit {titles.length} {titles.length === 1 ? 'title' : 'titles'} together</h2>
        {done ? (
          <>
            <p role="status">Changed {done.done}. {done.failed.length ? `${done.failed.length} couldn't be changed.` : ''}</p>
            {done.failed.length > 0 && <ul className="muted">{done.failed.slice(0, 10).map((f) => <li key={f}>{f}</li>)}</ul>}
            <div className="actions">
              <button type="button" className="btn ghost" onClick={async () => {
                try { await undoBatch(done.batch); toast('Undone'); onDone() } catch (e: any) { setErr(e.message) }
              }}>Undo all of it</button>
              <button type="button" className="btn primary" autoFocus onClick={onDone}>Done</button>
            </div>
          </>
        ) : (
          <>
            <p className="muted">Only what you fill in changes. What you change is locked on each title, so nothing changes it back.</p>
            <div className="form full">
              <div className="row2">
                <div><label htmlFor="bulk-ag">Add genres</label><TokenField id="bulk-ag" value={addGenres} onChange={setAddGenres} placeholder="Comedy" /></div>
                <div><label htmlFor="bulk-rg">Take away genres</label><TokenField id="bulk-rg" value={removeGenres} onChange={setRemoveGenres} /></div>
              </div>
              <div className="row2">
                <div><label htmlFor="bulk-at">Add tags</label><TokenField id="bulk-at" value={addTags} onChange={setAddTags} placeholder="Box set" /></div>
                <div><label htmlFor="bulk-rt">Take away tags</label><TokenField id="bulk-rt" value={removeTags} onChange={setRemoveTags} /></div>
              </div>
              <label>Year<input className="input" inputMode="numeric" maxLength={4} value={year} onChange={(e) => setYear(e.target.value.replace(/\D/g, ''))} /></label>
              <label className="lib-check"><input type="checkbox" checked={setRatingOn} onChange={(e) => setSetRatingOn(e.target.checked)} /> Set the content rating</label>
              {setRatingOn && <RatingField value={rating} onChange={setRating} />}
              <label>Locks
                <select className="select" value={lock} onChange={(e) => setLock(e.target.value)}>
                  <option value="">Leave them as they are</option>
                  <option value="lock">Lock every field</option>
                  <option value="unlock">Unlock every field</option>
                </select>
              </label>
              {err && <div className="error" role="alert">{err}</div>}
            </div>
            <div className="actions">
              <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
              <button type="button" className="btn primary" disabled={busy || nothing} onClick={apply}>{busy ? 'Changing…' : `Change ${titles.length}`}</button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
