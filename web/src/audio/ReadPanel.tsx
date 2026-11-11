// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, mediaUrl } from '../api'
import { IconClose, IconFile } from '../icons'
import type { useLyrics } from './lyrics'
import { WebLink } from '../components/WebLink'

// Something to read while it plays: the words, at your own pace, and whatever came with the
// album. A plain page that a keyboard or a remote can scroll, apart from the following lyrics.

export type Extra = { name: string; kind: 'booklet' | 'text'; size: number }

export function useExtras(itemId: number | undefined) {
  const [list, setList] = useState<Extra[]>([])
  useEffect(() => {
    setList([])
    if (!itemId) return
    let gone = false
    api.get<Extra[]>(`/music/extras/${itemId}`).then((l) => { if (!gone) setList(l) }).catch(() => {})
    return () => { gone = true }
  }, [itemId])
  return list
}

export function ReadPanel({ itemId, extras, ly, onClose }: { itemId: number; extras: Extra[]; ly: ReturnType<typeof useLyrics>; onClose: () => void }) {
  const [shown, setShown] = useState<{ name: string; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { setShown(null); setError('') }, [itemId])
  const open = async (e: Extra) => {
    if (e.kind === 'booklet') return
    setBusy(true)
    setError('')
    try {
      const r = await api.get<{ text: string }>(`/music/extras/${itemId}/file?name=${encodeURIComponent(e.name)}`)
      setShown({ name: e.name, text: r.text })
    } catch (err) { setError((err as Error).message) }
    setBusy(false)
  }
  const words = ly.lyrics?.plain || ''
  return (
    <div className="np-side">
      <div className="np-side-head">
        <h3>{shown ? shown.name : 'Read'}</h3>
        {shown && <button type="button" className="np-chip small" onClick={() => setShown(null)}>Back</button>}
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label="Close"><IconClose /></button>
      </div>
      {/* It holds only text, so it's made a stop of its own for arrows and Page Down. */}
      <div className="np-read" tabIndex={0} role="document">
        {error && <p className="np-note" role="alert">{error}</p>}
        {shown ? <pre className="np-read-text">{shown.text}</pre> : (
          <>
            {extras.length > 0 && (
              <ul className="np-extras">
                {extras.map((e) => (
                  <li key={e.name}>
                    {e.kind === 'booklet'
                      ? <a className="np-chip" href={mediaUrl(`/api/music/extras/${itemId}/file?name=${encodeURIComponent(e.name)}`)} target="_blank" rel="noreferrer"><IconFile size={18} /> {e.name}</a>
                      : <button type="button" className="np-chip" disabled={busy} onClick={() => void open(e)}><IconFile size={18} /> {e.name}</button>}
                  </li>
                ))}
              </ul>
            )}
            {ly.loading && <p className="np-hint">Looking for the words…</p>}
            {words
              ? <pre className="np-read-text">{words}</pre>
              : !ly.loading && extras.length === 0 && <p className="np-hint">Nothing to read for this one yet: no words were found, and no booklet is beside the album. A PDF or a text file in the album's folder appears here.</p>}
            {ly.lyrics?.sourceName && words && <p className="np-credit">Words from {ly.lyrics.creditUrl ? <WebLink href={ly.lyrics.creditUrl}>{ly.lyrics.sourceName}</WebLink> : ly.lyrics.sourceName}.</p>}
          </>
        )}
      </div>
    </div>
  )
}
