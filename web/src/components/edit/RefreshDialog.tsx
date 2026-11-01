import { useEffect, useState } from 'react'
import { api } from '../../api'
import { undoBatch, type MatchPreview, type Target, type TitleEditor } from '../../metadata'
import { toast } from '../../state'
import { Button, Dialog } from '../../ui'
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
  return (
    <Dialog title="Refresh details" size="wide" onClose={onClose}
      actions={undo
        ? <>
            <Button variant="ghost" onClick={() => undoBatch(undo).then(() => { toast('Undone'); onChanged(); onClose() }).catch((e) => setErr(e.message))}>Undo</Button>
            <Button variant="primary" autoFocus onClick={onClose}>Done</Button>
          </>
        : !p && err ? <Button variant="ghost" onClick={onClose}>Close</Button> : undefined}>
      {err && <div className="error" role="alert">{err}</div>}
      {undo ? <p role="status">Updated.</p>
        : !p ? (err ? null : <div className="loading-box"><div className="spinner" /></div>)
        : <>
            <PreviewTable p={p} picked={picked} setPicked={setPicked} onApply={apply} onBack={onClose} applyLabel={p.changes.length ? 'Update it' : 'Look again anyway'} />
            <LegalNote kind="webResults" />
          </>}
    </Dialog>
  )
}
