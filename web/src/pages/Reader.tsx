// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, mediaUrl, type Item } from '../api'
import type { Ann, EpubInfo, ReaderHost } from '@wittywilla/wittyink'
import { ModuleMissing } from '../components/ModuleMissing'
import { toast, useApp } from '../state'
import { reportClient } from '../errorReport'
import { BundledReaderView, readerKitOf, readerOf, useBundledReader, useReaderBundles } from '../moduleBundles'
import { useBookRemote } from '../remote/pageTargets'
import { t } from '../i18n'
import './reader.css'

// Books libraries in WittyMediaServer: whichever module reads the kind of book (its
// integration.json's readers hook: WittyEbook, WittyPDF, WittyComics), built on WittyInk, with a
// host that saves the place and notes per account on the server (so every device continues where
// the reader left off) and fetches pages and definitions through the server.

type BookInfo = Partial<EpubInfo> & { format: string; reader?: string; reading: string; title: string; pages?: number }
// Bumped when the server's book headers change, so cached chapters refresh.
const READER_V = 2

export default function ReaderPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const [it, setIt] = useState<Item | null>(null)
  const [info, setInfo] = useState<BookInfo | null>(null)
  const [err, setErr] = useState('')
  // The profile's settings of WittyInk and the readers (their sdui.json), drawn in Settings under
  // Modules: a book opens with them, and choices made while reading are saved to them.
  const [prefs, setPrefs] = useState<Record<string, Record<string, unknown>> | null>(null)
  const bundles = useReaderBundles()
  const format = info ? (info.reader === 'pdf' ? 'pdf' : info.reader === 'cbz' ? 'cbz' : 'epub') : undefined
  const reader = format && bundles ? readerOf(format, bundles) : null
  const kit = bundles ? readerKitOf(bundles) : null
  useEffect(() => {
    api.get<Item>(`/items/${id}`).then(setIt).catch((e) => setErr(e.message))
    api.get<BookInfo>(`/items/${id}/book`).then(setInfo).catch((e) => setErr(e.message))
  }, [id])
  useEffect(() => {
    if (!bundles || !format) return
    const mods = [kit?.id, reader?.id].filter(Boolean) as string[]
    Promise.all(mods.map((m) => api.get<{ values?: Record<string, unknown> }>(`/sdui/modules/${m}/settings`).then((r) => r.values ?? {}).catch(() => ({}))))
      .then((vals) => setPrefs(Object.fromEntries(mods.map((m, i) => [m, vals[i]]))))
  }, [bundles, format, kit?.id, reader?.id])
  const close = () => (window.history.state?.idx > 0 ? nav(-1) : nav(`/item/${id}`))
  // Another device steering this one (docs/REMOTE-PROTOCOL.md): it is told the place each time the reader saves it.
  const placeChanged = useBookRemote(it?.id || 0, it?.progress && !it.progress.watched ? it.progress.positionSec : 0, close)

  const host = useMemo((): ReaderHost | null => {
    if (!it || !info || !prefs) return null
    const base = `/items/${it.id}`
    return {
      key: String(it.id),
      book: { title: it.title, author: it.artist, year: it.year },
      format: format as ReaderHost['format'],
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
      settings: {
        get: (m) => prefs[m] ?? {},
        save: (m, values) => {
          prefs[m] = { ...(prefs[m] ?? {}), ...values }
          api.put(`/sdui/modules/${m}/settings/personal`, { values }).catch(() => { /* kept on this device; the next change tries again */ })
        },
      },
      toast,
      close,
      // What goes wrong in the reader, for this server's log. The reader sends
      // fixed text; reportClient groups repeats, keeps to its limits and hides tokens.
      report: (level, message, detail) => reportClient({
        level, kind: 'reader', module: reader?.id || 'wittyink', message, stack: [detail?.page && `page ${detail.page}`, detail?.stack].filter(Boolean).join('\n') || undefined,
      }),
    }
  }, [it, info, prefs, format, reader?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  // The reader from its own release, when one is in use; otherwise null.
  const bundled = useBundledReader(host?.format, bundles)

  if (err) return <div className="reader center"><div className="error">{err}<br /><button className="btn" onClick={close}>{t('Back')}</button></div></div>
  if (!host || bundled === undefined) return <div className="reader center"><div className="spinner" /></div>
  const missing = <div className="reader center">{kit && !reader ? <NoReader format={host.format} /> : <ModuleMissing ids={[reader?.id, 'wittyink'].filter(Boolean) as string[]} what={it?.title} onReady={() => window.location.reload()} />}</div>
  if (!bundled) return missing
  return <BundledReaderView r={bundled} host={host} fallback={missing}><h1 className="sr-only">{it?.title}</h1></BundledReaderView>
}

// No module in use says it reads this kind of book. The store doesn't say which would (its
// summaries don't carry the readers hook), so this names the kind and where to look.
function NoReader({ format }: { format: string }) {
  const { user } = useApp()
  return (
    <div className="empty" role="status">
      <h2>{t('No reader for {format} books is in use here', { format: format.toUpperCase() })}</h2>
      <p className="muted">{user?.isAdmin ? t('Add one from the module store.') : t('Ask whoever runs this server to add one.')}</p>
      {user?.isAdmin && <Link className="btn" to="/settings/modules/store">{t('Open the module store')}</Link>}
    </div>
  )
}
