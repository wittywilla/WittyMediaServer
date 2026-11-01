import { useEffect, useRef, useState } from 'react'
import type { Library } from '../api'
import { fmtSize, uploadFile, type UploadResult } from '../gallery'
import { IconPlus } from '../icons'
import { ACCEPT_GALLERY } from '../mediaTypes.generated'
import { toast } from '../state'
import { Dialog } from '../ui/Overlay'
import '../gallery.css'

// Upload, for Photos and Vertical videos libraries. Nothing shows for a library with no folder
// or one set to read-only, where the server would refuse.
export function UploadButton({ lib, onDone }: { lib: Library; onDone: () => void }) {
  const input = useRef<HTMLInputElement | null>(null)
  const [state, setState] = useState<{ name: string; n: number; of: number; pct: number } | null>(null)
  const [errors, setErrors] = useState<UploadResult[]>([])
  const run = async (files: File[]) => {
    const failed: UploadResult[] = []
    let total = 0
    for (let i = 0; i < files.length; i++) {
      total += files[i].size
      setState({ name: files[i].name, n: i + 1, of: files.length, pct: 0 })
      const r = await uploadFile(lib, files[i], (d) => setState({ name: files[i].name, n: i + 1, of: files.length, pct: d }))
      if (r.error) failed.push(r)
    }
    setState(null); setErrors(failed)
    if (!failed.length) toast(`${files.length} ${files.length === 1 ? 'file' : 'files'} added (${fmtSize(total)})`)
    onDone()
  }
  const usable = lib.paths.length > 0 && !lib.readOnly
  // Dropping files anywhere on the page works too.
  useEffect(() => {
    if (!usable) return
    const over = (e: DragEvent) => { if (e.dataTransfer?.types.includes('Files')) e.preventDefault() }
    const drop = (e: DragEvent) => { if (e.dataTransfer?.files.length) { e.preventDefault(); run([...e.dataTransfer.files]) } }
    window.addEventListener('dragover', over)
    window.addEventListener('drop', drop)
    return () => { window.removeEventListener('dragover', over); window.removeEventListener('drop', drop) }
  })
  if (!usable) return null
  return (
    <>
      <input ref={input} type="file" multiple hidden accept={(lib.type === 'photos' ? 'image/*,' : '') + 'video/*,' + ACCEPT_GALLERY}
        onChange={(e) => { const f = [...(e.target.files || [])]; e.target.value = ''; if (f.length) run(f) }} />
      <button className="btn sm" disabled={!!state} onClick={() => input.current?.click()}><IconPlus /> {state ? `Uploading ${state.n}/${state.of} (${Math.round(state.pct * 100)}%)` : 'Upload'}</button>
      {state && <span className="sr-only" role="status">Uploading {state.name}</span>}
      {errors.length > 0 && (
        <Dialog title={<>{errors.length} {errors.length === 1 ? 'file' : 'files'} not added</>} onClose={() => setErrors([])} actions={<><button className="btn primary" onClick={() => setErrors([])}>Close</button></>}>
            <ul className="gal-errs">{errors.map((x, i) => <li key={i}><b>{x.name}</b>: {x.error}</li>)}</ul>
        </Dialog>
      )}
    </>
  )
}
