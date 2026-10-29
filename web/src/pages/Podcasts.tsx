import { useEffect, useState } from 'react'
import { api, type Item } from '../api'
import { toast, useApp } from '../state'

// Podcasts: adding a show (search Apple's free directory or paste a feed
// address) and the admin controls on a podcast's page.

export type Podcast = { id: number; libraryId: number; url: string; title: string; author: string; image: string; description: string; lastChecked: number; autoDownload: number }
type Hit = { title: string; author: string; feed: string; image: string; genre: string }

export function AddPodcast({ libraryId, onClose, onAdded }: { libraryId: number; onClose: () => void; onAdded: () => void }) {
  const [q, setQ] = useState('')
  const [hits, setHits] = useState<Hit[]>([])
  const [busy, setBusy] = useState<string | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    if (q.trim().length < 2 || /^https?:\/\//i.test(q.trim())) { setHits([]); return }
    const t = setTimeout(() => api.get<Hit[]>(`/podcasts/search?q=${encodeURIComponent(q.trim())}`).then(setHits).catch((e) => setErr(e.message)), 350)
    return () => clearTimeout(t)
  }, [q])
  const add = async (url: string) => {
    setBusy(url); setErr('')
    try { await api.post('/podcasts', { library: libraryId, url }); toast('Podcast added'); onAdded() } catch (e: any) { setErr(e.message) } finally { setBusy(null) }
  }
  const isUrl = /^https?:\/\//i.test(q.trim())
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal wide" role="dialog" aria-label="Add a podcast" onClick={(e) => e.stopPropagation()}>
        <h2>Add a podcast</h2>
        <input className="input" autoFocus placeholder="Search by name, or paste the feed address (https://…)" aria-label="Podcast name or feed address" value={q} onChange={(e) => setQ(e.target.value)} />
        {isUrl && <button className="btn primary" style={{ marginTop: 10 }} disabled={!!busy} onClick={() => add(q.trim())}>{busy ? 'Adding…' : 'Add this feed'}</button>}
        {err && <div className="error" role="alert">{err}</div>}
        <div className="podcast-hits">
          {hits.map((h) => (
            <div key={h.feed} className="podcast-hit">
              {h.image && <img src={h.image} alt="" loading="lazy" />}
              <div className="grow"><b>{h.title}</b><span className="muted">{h.author}{h.genre ? ` · ${h.genre}` : ''}</span></div>
              <button className="btn sm primary" disabled={!!busy} onClick={() => add(h.feed)}>{busy === h.feed ? 'Adding…' : 'Add'}</button>
            </div>
          ))}
        </div>
        <p className="muted" style={{ fontSize: 12 }}>Search results come from the Apple Podcasts directory. Episodes play through this server; download any you want to keep.</p>
        <div className="actions"><button className="btn ghost" onClick={onClose}>Close</button></div>
      </div>
    </div>
  )
}

// PodcastAdmin: refresh, auto-download and unfollow on a podcast's page.
export function PodcastAdmin({ libraryId, title, onChanged }: { libraryId: number; title: string; onChanged: () => void }) {
  const { user } = useApp()
  const [p, setP] = useState<Podcast | null>(null)
  useEffect(() => { api.get<Podcast[]>(`/podcasts?library=${libraryId}`).then((ps) => setP(ps.find((x) => x.title === title) || null)).catch(() => {}) }, [libraryId, title])
  if (!user?.isAdmin || !p) return null
  const refresh = async () => { try { await api.post(`/podcasts/${p.id}/refresh`); toast('Checked for new episodes'); onChanged() } catch (e: any) { toast(e.message) } }
  const auto = async (n: number) => { try { setP(await api.put<Podcast>(`/podcasts/${p.id}`, { autoDownload: n })) } catch (e: any) { toast(e.message) } }
  const remove = async () => {
    if (!confirm(`Stop following “${p.title}”? Downloaded episodes stay in the library.`)) return
    try { await api.del(`/podcasts/${p.id}`); toast('Unfollowed'); history.back() } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="podcast-admin">
      <button className="btn sm" onClick={refresh}>Check for new episodes</button>
      <label className="muted">Keep downloaded:
        <select className="select sm" value={p.autoDownload} onChange={(e) => auto(Number(e.target.value))}>
          <option value={0}>none (stream)</option>{[1, 3, 5, 10, 25].map((n) => <option key={n} value={n}>newest {n}</option>)}
        </select>
      </label>
      <button className="btn sm danger" onClick={remove}>Unfollow</button>
    </div>
  )
}

export function DownloadButton({ item, onDone }: { item: Item; onDone: (it: Item) => void }) {
  const { user } = useApp()
  const [busy, setBusy] = useState(false)
  if (!user?.isAdmin) return null
  const online = /^https?:\/\//.test(item.path)
  if (!online) return <span className="muted dl-done" title="Downloaded">✓ saved</span>
  return (
    <button className="btn sm ghost" disabled={busy} onClick={async (e) => {
      e.stopPropagation(); setBusy(true)
      try { onDone(await api.post<Item>(`/items/${item.id}/download`)) } catch (err: any) { toast(err.message) } finally { setBusy(false) }
    }}>{busy ? 'Saving…' : 'Download'}</button>
  )
}
