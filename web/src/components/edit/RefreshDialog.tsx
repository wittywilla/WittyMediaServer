import { useEffect, useRef, useState } from 'react'
import { api } from '../../api'
import { undoBatch, type MatchPreview, type Target, type TitleEditor } from '../../metadata'
import { toast } from '../../state'
import LegalNote from '../LegalNote'
import { PreviewTable } from './MatchPanel'
import './edit.css'

// A refresh that shows what it will change before it does. The title keeps its match, and
// locked fields stay as they are.

export default function RefreshDialog({ target, id, onClose, onChanged }: { target: Target; id: number; onClose: () => void; onChanged: () => void }) {
  const [p, setP] = useState<MatchPreview | null>(null)
  const [err, setErr] = useState('')
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [undo, setUndo] = useState('')
  useEffect(() => {
    api.post<MatchPreview>(`/meta/${target}/${id}/preview`, {})
      .then((r) => { setP(r); setPicked(new Set(r.changes.filter((c) => c.apply).map((c) => c.field))) })
      .catch((e) => setErr(e.message))
  }, [target, id])
  const apply = async () => {
    setErr('')
    try {
      const r = await api.post<{ editor: TitleEditor; batch: string }>(`/meta/${target}/${id}/refresh`, { fields: [...picked] })
      onChanged()
      setUndo(r.batch)
      setP(null)
    } catch (e: any) { setErr(e.message) }
  }
  const pressed = useRef(false)
  return (
    <div className="modal-backdrop" onMouseDown={(e) => { pressed.current = e.target === e.currentTarget }} onClick={(e) => { if (e.target === e.currentTarget && (pressed.current || e.detail === 0)) onClose() }}>
      <div className="modal wide" role="dialog" aria-modal="true" aria-labelledby="refresh-h">
        <h2 id="refresh-h">Refresh details</h2>
        {err && <div className="error" role="alert">{err}</div>}
        {undo ? (
          <>
            <p role="status">Updated.</p>
            <div className="actions">
              <button type="button" className="btn ghost" onClick={() => undoBatch(undo).then(() => { toast('Undone'); onChanged(); onClose() }).catch((e) => setErr(e.message))}>Undo</button>
              <button type="button" className="btn primary" autoFocus onClick={onClose}>Done</button>
            </div>
          </>
        ) : !p ? (
          err ? <div className="actions"><button type="button" className="btn ghost" onClick={onClose}>Close</button></div> : <div className="loading-box"><div className="spinner" /></div>
        ) : (
          <>
            <PreviewTable p={p} picked={picked} setPicked={setPicked} onApply={apply} onBack={onClose} applyLabel={p.changes.length ? 'Update it' : 'Look again anyway'} />
            <LegalNote kind="webResults" />
          </>
        )}
      </div>
    </div>
  )
}
