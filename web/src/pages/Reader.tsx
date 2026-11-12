// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, mediaUrl, type Item } from '../api'
import type { Ann, EpubInfo, ReaderHost } from '@wittywilla/wittyink'
import { ModuleMissing } from '../components/ModuleMissing'
import { toast } from '../state'
import { reportClient } from '../errorReport'
import { BundledReaderView, useBundledReader } from '../moduleBundles'
import { useBookRemote } from '../remote/pageTargets'

// Books libraries in WittyMediaServer: the WittyEbook, WittyPDF and
// WittyComics readers, with a host that saves the place and notes per account
// on the server (so every device continues where the reader left off) and
// fetches pages and definitions through the server.

const READER_MODULE = { pdf: 'wittypdf', cbz: 'wittycomics', epub: 'wittyebook' }

type BookInfo = Partial<EpubInfo> & { format: string; reader?: string; reading: string; title: string; pages?: number }
// Bumped when the server's book headers change, so cached chapters refresh.
const READER_V = 2

export default function ReaderPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const [it, setIt] = useState<Item | null>(null)
  const [info, setInfo] = useState<BookInfo | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    api.get<Item>(`/items/${id}`).then(setIt).catch((e) => setErr(e.message))
    api.get<BookInfo>(`/items/${id}/book`).then(setInfo).catch((e) => setErr(e.message))
  }, [id])
  const close = () => (window.history.state?.idx > 0 ? nav(-1) : nav(`/item/${id}`))
  // Another device steering this one (docs/REMOTE-PROTOCOL.md): it is told the place each time the reader saves it.
  const placeChanged = useBookRemote(it?.id || 0, it?.progress && !it.progress.watched ? it.progress.positionSec : 0, close)

  const host = useMemo((): ReaderHost | null => {
    if (!it || !info) return null
    const base = `/items/${it.id}`
    const format = info.reader === 'pdf' ? 'pdf' : info.reader === 'cbz' ? 'cbz' : 'epub'
    return {
      key: String(it.id),
      book: { title: it.title, author: it.artist, year: it.year },
      format,
      reading: info.reading,
      start: it.progress && !it.progress.watched ? it.progress.positionSec : 0,
      saveProgress: (positionSec, durationSec) => { placeChanged(positionSec, durationSec); api.post(`${base}/progress`, { positionSec, durationSec }).catch(() => {}) },
      annotations: {
        list: () => api.get<Ann[]>(`${base}/annotations`),
        add: (a) => api.post<Ann>(`${base}/annotations`, a),
        update: async (a, patch) => { await api.put(`/annotations/${a.id}`, { ...a, ...patch }) },
        remove: async (a) => { await api.del(`/annotations/${a.id}`) },
      },
      pdf: { url: mediaUrl(`/api${base}/file`), wasmUrl: new URL(__PDF_ASSETS__ + 'wasm/', location.href).href, iccUrl: new URL(__PDF_ASSETS__ + 'iccs/', location.href).href },
      comic: { pages: info.pages || 0, pageUrl: (n) => mediaUrl(`/api${base}/page/${n}`) },
      epub: {
        info: { spine: info.spine || [], toc: info.toc || [], words: info.words || [] },
        chapterUrl: (path) => mediaUrl(`/api${base}/epub/${path}?v=${READER_V}`),
        search: (q) => api.get(`${base}/search?q=${encodeURIComponent(q)}`),
      },
      define: (word) => api.get(`/dictionary?word=${encodeURIComponent(word)}`),
      toast,
      close,
      // What goes wrong in the reader, for this server's log. The reader sends
      // fixed text; reportClient groups repeats, keeps to its limits and hides tokens.
      report: (level, message, detail) => reportClient({
        level, kind: 'reader', module: READER_MODULE[format], message, stack: [detail?.page && `page ${detail.page}`, detail?.stack].filter(Boolean).join('\n') || undefined,
      }),
    }
  }, [it, info]) // eslint-disable-line react-hooks/exhaustive-deps
  // A newer reader from its own release, when one is in use; otherwise null and the copies above.
  const bundled = useBundledReader(host?.format)

  if (err) return <div className="reader center"><div className="error">{err}<br /><button className="btn" onClick={close}>Back</button></div></div>
  if (!host || bundled === undefined) return <div className="reader center"><div className="spinner" /></div>
  const missing = <div className="reader center"><ModuleMissing ids={[READER_MODULE[host.format], 'wittyink']} what={it?.title} onReady={() => window.location.reload()} /></div>
  if (!bundled) return missing
  return <BundledReaderView r={bundled} host={host} fallback={missing}><h1 className="sr-only">{it?.title}</h1></BundledReaderView>
}
