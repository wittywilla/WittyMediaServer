// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api, type Library } from '../../api'
import { Dialog } from '../../ui/Overlay'
import { toast } from '../../state'
import { t, tn } from '../../i18n'

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
      toast(res.rescanning.length > 0
        ? (library ? t('Downloaded details removed; scanning {name} again', { name: library.name }) : t('Downloaded details removed; scanning every library again'))
        : t('Downloaded details removed'))
    } catch (e: any) {
      setErr(e.message)
    } finally {
      setBusy(false)
    }
  }

  if (result) {
    return (
      <Dialog title={t('Downloaded details removed')} onClose={onClose} size="md" className="fm-dialog"
        actions={<button className="btn primary" onClick={onClose} autoFocus>{t('Done')}</button>}>
        <ul className="fm-counts" role="status">
          {files && <li>{result.companions > 0
            ? tn('{count} file the server saved beside the media removed, with {companions}.', '{count} files the server saved beside the media removed, with {companions}.', result.removed,
              { companions: tn('{count} macOS “._” file', '{count} macOS “._” files', result.companions) })
            : tn('{count} file the server saved beside the media removed.', '{count} files the server saved beside the media removed.', result.removed)}</li>}
          {files && result.restored > 0 && <li>{tn('One NFO file of yours put back from the .nfo.bak copy the server kept.', '{count} NFO files of yours put back from the .nfo.bak copy the server kept.', result.restored)}</li>}
          {files && result.skipped > 0 && <li>{tn('{count} file was left as they were: changed since the server saved them, or couldn’t be removed.', '{count} files were left as they were: changed since the server saved them, or couldn’t be removed.', result.skipped)}</li>}
          {details && <li>{t('{titles} and {shows} back to their file names.', { titles: tn('{count} title', '{count} titles', result.titles), shows: tn('{count} show', '{count} shows', result.shows) })}</li>}
          {details && result.lockedKept > 0 && <li>{tn('{count} field you locked or typed kept.', '{count} fields you locked or typed kept.', result.lockedKept)}</li>}
          {details && result.cached > 0 && <li>{tn('{count} cached picture removed.', '{count} cached pictures removed.', result.cached)}</li>}
          {details && result.logos > 0 && <li>{tn('{count} downloaded logo removed.', '{count} downloaded logos removed.', result.logos)}</li>}
          {details && result.notReset.length > 0 && <li>{t('Not changed: {names}. Their details come from the files themselves, not from a lookup.', { names: result.notReset.join(', ') })}</li>}
          {result.rescanning.length > 0 && <li>{t('Scanning again now, which matches and downloads everything afresh. Activity shows how it’s going.')}</li>}
        </ul>
        {result.problems && result.problems.length > 0 && (
          <details className="fm-problems"><summary>{t('What couldn’t be removed')}</summary><ul>{result.problems.map((p) => <li key={p}><bdi>{p}</bdi></li>)}</ul></details>
        )}
      </Dialog>
    )
  }

  return (
    <Dialog title={library ? t('Remove downloaded details from {name}?', { name: library.name }) : t('Remove downloaded details from every library?')} onClose={onClose} size="md" className="fm-dialog"
      description={t('For trying matching and downloading again from a clean start. Your media files are never touched.')}
      actions={<>
        <button className="btn ghost" onClick={onClose} disabled={busy}>{t('Cancel')}</button>
        <button className="btn danger" onClick={run} disabled={busy || scanning || (!files && !details)}>{busy ? t('Removing…') : t('Remove')}</button>
      </>}>
      <fieldset className="fm-parts">
        <legend className="sr-only">{t('What to remove')}</legend>
        <label className="fm-part">
          <input type="checkbox" checked={files} onChange={(e) => setFiles(e.target.checked)} disabled={busy} />
          <span><b>{t('Files saved beside the media')}</b>
            <small>{library
              ? t('The artwork, NFO files and CD details the server wrote into this library’s folders. Only files the server wrote go; where it replaced an NFO of yours, yours goes back.')
              : t('The artwork, NFO files and CD details the server wrote into the libraries’ folders. Only files the server wrote go; where it replaced an NFO of yours, yours goes back.')}
              {readOnly && ' ' + t('This works on a read-only library too: read-only stops the server adding files, and these are its own.')}</small>
          </span>
        </label>
        <label className="fm-part">
          <input type="checkbox" checked={details} onChange={(e) => setDetails(e.target.checked)} disabled={busy} />
          <span><b>{t('Details in the server')}</b>
            <small>{library
              ? t('Matches, overviews, genres, ratings, artwork, cast and crew and where to watch, and their cached pictures. Titles go back to their file names.')
              : t('Matches, overviews, genres, ratings, artwork, cast and crew and where to watch, and their cached pictures, and downloaded logos. Titles go back to their file names.')}</small>
          </span>
        </label>
        <label className="fm-part">
          <input type="checkbox" checked={rescan} onChange={(e) => setRescan(e.target.checked)} disabled={busy} />
          <span><b>{t('Download again afterwards')}</b>
            <small>{library
              ? t('Scans the library straight away, which matches and downloads everything afresh.')
              : t('Scans every library straight away, which matches and downloads everything afresh.')}</small>
          </span>
        </label>
      </fieldset>
      <p className="fm-kept"><b>{t('Kept:')}</b> {t('your profiles, settings and libraries; what everyone has watched and how far; ratings, favourites, lists and playlists; fields you locked or typed yourself; modules, WittyTV and game saves.')}{details && ' ' + t('Matches you chose by hand are forgotten, so titles are matched again from their names.')}</p>
      {scanning && <p className="set-note bad" role="status">{library ? t('This library is being scanned. Try again once the scan has finished.') : t('A library is being scanned. Try again once the scan has finished.')}</p>}
      {err && <p className="set-note bad" role="alert">{err}</p>}
    </Dialog>
  )
}
