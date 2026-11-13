// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api, type Library } from '../../api'
import { Dialog } from '../../ui/Overlay'
import { toast } from '../../state'

// Removing downloaded details, so the owner can watch matching and downloading happen again from
// a clean start: one library, or every library at once (POST /api/libraries/{id}/forget-metadata
// and /api/libraries/forget-metadata). The server says what went; nothing a person made goes.

interface ForgetResult {
  libraries: number
  removed: number
  restored: number
  companions: number
  skipped: number
  titles: number
  shows: number
  cached: number
  logos: number
  lockedKept: number
  notReset: string[]
  problems?: string[]
  rescanning: number[]
}

const n = (count: number, one: string, many: string) => `${count.toLocaleString()} ${count === 1 ? one : many}`

export function ForgetMetadataDialog({ library, libraries, onClose, onDone }: {
  library?: Library // left out: every library
  libraries: Library[]
  onClose: () => void
  onDone: () => void
}) {
  const [files, setFiles] = useState(true)
  const [details, setDetails] = useState(true)
  const [rescan, setRescan] = useState(true)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [result, setResult] = useState<ForgetResult | null>(null)
  const which = library ? library.name : 'every library'
  const readOnly = library ? !!library.readOnly : libraries.some((l) => l.readOnly)
  const scanning = library ? library.scanning : libraries.some((l) => l.scanning)

  const run = async () => {
    setBusy(true)
    setErr('')
    try {
      const path = library ? `/libraries/${library.id}/forget-metadata` : '/libraries/forget-metadata'
      const res = await api.post<ForgetResult>(path, { files, details, rescan })
      setResult(res)
      onDone()
      toast(res.rescanning.length > 0 ? `Downloaded details removed; scanning ${which} again` : 'Downloaded details removed')
    } catch (e: any) {
      setErr(e.message)
    } finally {
      setBusy(false)
    }
  }

  if (result) {
    return (
      <Dialog title="Downloaded details removed" onClose={onClose} size="md" className="fm-dialog"
        actions={<button className="btn primary" onClick={onClose} autoFocus>Done</button>}>
        <ul className="fm-counts" role="status">
          {files && <li>{n(result.removed, 'file', 'files')} the server saved beside the media removed{result.companions > 0 && `, with ${n(result.companions, 'macOS “._” file', 'macOS “._” files')}`}.</li>}
          {files && result.restored > 0 && <li>{result.restored === 1 ? 'One NFO file of yours' : `${result.restored.toLocaleString()} NFO files of yours`} put back from the .nfo.bak copy the server kept.</li>}
          {files && result.skipped > 0 && <li>{n(result.skipped, 'file was', 'files were')} left as they were: changed since the server saved them, or couldn’t be removed.</li>}
          {details && <li>{n(result.titles, 'title', 'titles')} and {n(result.shows, 'show', 'shows')} back to their file names.</li>}
          {details && result.lockedKept > 0 && <li>{n(result.lockedKept, 'field', 'fields')} you locked or typed kept.</li>}
          {details && result.cached > 0 && <li>{n(result.cached, 'cached picture', 'cached pictures')} removed.</li>}
          {details && result.logos > 0 && <li>{n(result.logos, 'downloaded logo', 'downloaded logos')} removed.</li>}
          {details && result.notReset.length > 0 && <li>Not changed: {result.notReset.join(', ')}. Their details come from the files themselves, not from a lookup.</li>}
          {result.rescanning.length > 0 && <li>Scanning again now, which matches and downloads everything afresh. Activity shows how it’s going.</li>}
        </ul>
        {result.problems && result.problems.length > 0 && (
          <details className="fm-problems"><summary>What couldn’t be removed</summary><ul>{result.problems.map((p) => <li key={p}><bdi>{p}</bdi></li>)}</ul></details>
        )}
      </Dialog>
    )
  }

  return (
    <Dialog title={library ? `Remove downloaded details from ${library.name}?` : 'Remove downloaded details from every library?'} onClose={onClose} size="md" className="fm-dialog"
      description="For trying matching and downloading again from a clean start. Your media files are never touched."
      actions={<>
        <button className="btn ghost" onClick={onClose} disabled={busy}>Cancel</button>
        <button className="btn danger" onClick={run} disabled={busy || scanning || (!files && !details)}>{busy ? 'Removing…' : 'Remove'}</button>
      </>}>
      <fieldset className="fm-parts">
        <legend className="sr-only">What to remove</legend>
        <label className="fm-part">
          <input type="checkbox" checked={files} onChange={(e) => setFiles(e.target.checked)} disabled={busy} />
          <span><b>Files saved beside the media</b>
            <small>The artwork, NFO files and CD details the server wrote into {library ? 'this library’s' : 'the libraries’'} folders. Only files the server wrote go; where it replaced an NFO of yours, yours goes back.{readOnly && ' This works on a read-only library too: read-only stops the server adding files, and these are its own.'}</small>
          </span>
        </label>
        <label className="fm-part">
          <input type="checkbox" checked={details} onChange={(e) => setDetails(e.target.checked)} disabled={busy} />
          <span><b>Details in the server</b>
            <small>Matches, overviews, genres, ratings, artwork, cast and crew and where to watch, and their cached pictures{library ? '' : ', and downloaded logos'}. Titles go back to their file names.</small>
          </span>
        </label>
        <label className="fm-part">
          <input type="checkbox" checked={rescan} onChange={(e) => setRescan(e.target.checked)} disabled={busy} />
          <span><b>Download again afterwards</b>
            <small>Scans {library ? 'the library' : 'every library'} straight away, which matches and downloads everything afresh.</small>
          </span>
        </label>
      </fieldset>
      <p className="fm-kept"><b>Kept:</b> your profiles, settings and libraries; what everyone has watched and how far; ratings, favourites, lists and playlists; fields you locked or typed yourself; modules, WittyTV and game saves.{details && ' Matches you chose by hand are forgotten, so titles are matched again from their names.'}</p>
      {scanning && <p className="set-note bad" role="status">{library ? 'This library is' : 'A library is'} being scanned. Try again once the scan has finished.</p>}
      {err && <p className="set-note bad" role="alert">{err}</p>}
    </Dialog>
  )
}
