import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, mediaUrl, type Item } from '../api'
import type { Ann, EpubInfo, ReaderHost } from '@wittywilla/wittyink'
import { ModuleMissing } from '../components/ModuleMissing'
import { toast } from '../state'
import { BundledReaderView, useBundledReader } from '../moduleBundles'

// Books libraries in WittyMediaServer: the WittyEbook, WittyPDF and
// WittyComics readers, with a host that saves the place and notes per account
// on the server (so every device continues where the reader left off) and
// fetches pages and definitions through the server.

const READER_MODULE = { pdf: 'wittypdf', cbz: 'wittycomics', epub: 'wittyebook' }

type BookInfo = Partial<EpubInfo> & { format: string; reading: string; title: string; pages?: number }
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

  const host = useMemo((): ReaderHost | null => {
    if (!it || !info) return null
    const base = `/items/${it.id}`
    return {
      key: String(it.id),
      book: { title: it.title, author: it.artist, year: it.year },
      format: info.format === 'pdf' ? 'pdf' : info.format === 'cbz' ? 'cbz' : 'epub',
      reading: info.reading,
      start: it.progress && !it.progress.watched ? it.progress.positionSec : 0,
      saveProgress: (positionSec, durationSec) => { api.post(`${base}/progress`, { positionSec, durationSec }).catch(() => {}) },
      annotations: {
        list: () => api.get<Ann[]>(`${base}/annotations`),
        add: (a) => api.post<Ann>(`${base}/annotations`, a),
        update: async (a, patch) => { await api.put(`/annotations/${a.id}`, { ...a, ...patch }) },
        remove: async (a) => { await api.del(`/annotations/${a.id}`) },
      },
      pdf: { url: mediaUrl(`/api${base}/file`) },
      comic: { pages: info.pages || 0, pageUrl: (n) => mediaUrl(`/api${base}/page/${n}`) },
      epub: {
        info: { spine: info.spine || [], toc: info.toc || [], words: info.words || [] },
        chapterUrl: (path) => mediaUrl(`/api${base}/epub/${path}?v=${READER_V}`),
        search: (q) => api.get(`${base}/search?q=${encodeURIComponent(q)}`),
      },
      define: (word) => api.get(`/dictionary?word=${encodeURIComponent(word)}`),
      toast,
      close,
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
