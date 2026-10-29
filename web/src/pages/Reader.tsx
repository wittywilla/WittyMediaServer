import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, mediaUrl, type Item } from '../api'
import WittyComics from '../modules/wittycomics'
import WittyEbook from '../modules/wittyebook'
import { HostContext, type Ann, type EpubInfo, type ReaderHost } from '../modules/wittyink'
import { toast } from '../state'

// Books libraries in WittyMediaServer: the WittyEbook, WittyPDF and
// WittyComics readers, with a host that saves the place and notes per account
// on the server (so every device continues where the reader left off) and
// fetches pages and definitions through the server.

// pdf.js is large: WittyPDF loads only when a PDF is opened.
const WittyPDF = lazy(() => import('../modules/wittypdf'))

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

  if (err) return <div className="reader center"><div className="error">{err}<br /><button className="btn" onClick={close}>Back</button></div></div>
  if (!host) return <div className="reader center"><div className="spinner" /></div>
  return (
    <HostContext.Provider value={host}>
      {host.format === 'pdf' ? <Suspense fallback={<div className="reader center"><div className="spinner" /></div>}><WittyPDF /></Suspense>
        : host.format === 'cbz' ? <WittyComics /> : <WittyEbook />}
    </HostContext.Provider>
  )
}
