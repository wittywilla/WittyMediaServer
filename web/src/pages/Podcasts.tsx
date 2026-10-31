import { useEffect, useState } from 'react'
import { api, type Item } from '../api'
import { toast, useApp } from '../state'
import LegalNote from '../components/LegalNote'

// Podcasts: following a show (search Apple's free directory or paste its RSS
// feed), and the controls on a podcast's page.

export type Podcast = { id: number; libraryId: number; url: string; title: string; author: string; image: string; description: string; lastChecked: number; autoDownload: number }
type Hit = { title: string; author: string; feed: string; image: string; genre: string }

// PodcastFinder: two ways to follow a show. Search the free Apple Podcasts
// directory by name, or paste the RSS feed address a show publishes.
export function PodcastFinder({ libraryId, onAdded }: { libraryId: number; onAdded: () => void }) {
  const [mode, setMode] = useState<'search' | 'feed'>('search')
  const [q, setQ] = useState('')
  const [feed, setFeed] = useState('')
  const [hits, setHits] = useState<Hit[] | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [added, setAdded] = useState<string[]>([])
  const [err, setErr] = useState('')
  useEffect(() => {
    const term = q.trim()
    if (term.length < 2) { setHits(null); return }
    if (/^https?:\/\//i.test(term)) { setMode('feed'); setFeed(term); setQ(''); return }
    const t = setTimeout(() => { setErr(''); api.get<Hit[]>(`/podcasts/search?q=${encodeURIComponent(term)}`).then(setHits).catch((e) => setErr(e.message)) }, 350)
    return () => clearTimeout(t)
  }, [q])
  const add = async (url: string) => {
    setBusy(url); setErr('')
    try {
      await api.post('/podcasts', { library: libraryId, url })
      setAdded((a) => [...a, url]); toast('Following'); onAdded()
      if (url === feed.trim()) setFeed('')
    } catch (e: any) { setErr(e.message) } finally { setBusy(null) }
  }
  return (
    <div className="pod-finder">
      <div className="seg" role="tablist" aria-label="How to find a podcast">
        <button role="tab" aria-selected={mode === 'search'} className={mode === 'search' ? 'active' : ''} onClick={() => setMode('search')}>Search the directory</button>
        <button role="tab" aria-selected={mode === 'feed'} className={mode === 'feed' ? 'active' : ''} onClick={() => setMode('feed')}>RSS feed address</button>
      </div>
      {mode === 'search' ? (
        <>
          <input className="input" autoFocus placeholder="Search podcasts by name, host or topic" aria-label="Search podcasts" value={q} onChange={(e) => setQ(e.target.value)} />
          <p className="muted pod-note">Results come from the free, public Apple Podcasts directory. Shows you follow <b>stream</b> through your server; nothing is saved until you download it.</p>
        </>
      ) : (
        <>
          <form className="field-row" onSubmit={(e) => { e.preventDefault(); if (feed.trim()) add(feed.trim()) }}>
            <input className="input" type="url" autoFocus placeholder="https://example.com/feed.xml" aria-label="RSS feed address" value={feed} onChange={(e) => setFeed(e.target.value)} />
            <button className="btn primary" disabled={!/^https?:\/\/./i.test(feed.trim()) || !!busy}>{busy === feed.trim() ? 'Following…' : 'Follow'}</button>
          </form>
          <p className="muted pod-note">Every podcast publishes an RSS feed; look for “RSS” on the show’s website. Private and members-only feeds work too.</p>
        </>
      )}
      {err && <div className="error" role="alert">{err}</div>}
      <LegalNote kind="webResults" />
      {mode === 'search' && hits && (
        <div className="podcast-hits">
          {hits.length === 0 && <p className="muted">No podcasts found for “{q.trim()}”.</p>}
          {hits.map((h) => {
            const done = added.includes(h.feed)
            return (
              <div key={h.feed} className="podcast-hit">
                {h.image ? <img src={h.image} alt="" loading="lazy" /> : <span className="podcast-hit-ph" />}
                <div className="grow"><b>{h.title}</b><span className="muted">{h.author}{h.genre ? ` · ${h.genre}` : ''}</span></div>
                <button className={'btn sm ' + (done ? 'ghost' : 'primary')} disabled={!!busy || done} onClick={() => add(h.feed)}>{done ? 'Following' : busy === h.feed ? 'Following…' : 'Follow'}</button>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

// PodcastWays: what a podcasts library can hold, shown when adding one and
// on an empty podcasts library.
export function PodcastWays() {
  return (
    <div className="pod-ways">
      <div><b>Follow shows</b><span className="muted">Search a free online directory or paste an RSS feed. New episodes appear as they’re published and <b>stream</b> from the web.</span></div>
      <div><b>Download episodes</b><span className="muted">Save any episode to your server, or have a show keep its newest episodes downloaded automatically each time its feed updates.</span></div>
      <div><b>Your own files</b><span className="muted">Already have episodes as MP3 or M4A files? Point the library at their folder and they play like any other episode.</span></div>
    </div>
  )
}

export function AddPodcast({ libraryId, onClose, onAdded }: { libraryId: number; onClose: () => void; onAdded: () => void }) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal wide" role="dialog" aria-label="Follow a podcast" onClick={(e) => e.stopPropagation()}>
        <h2>Follow a podcast</h2>
        <PodcastFinder libraryId={libraryId} onAdded={onAdded} />
        <p className="muted pod-note" style={{ marginTop: 12 }}>To keep episodes, open a show and choose <b>Keep downloaded</b>: its newest episodes are saved to this library’s folder each time the feed updates.</p>
        <div className="actions"><button className="btn primary" onClick={onClose}>Done</button></div>
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
        <select className="select sm" value={p.autoDownload} onChange={(e) => auto(Number(e.target.value))} title="Saved to this library's folder each time the feed updates">
          <option value={0}>none, stream them</option>{[1, 3, 5, 10, 25].map((n) => <option key={n} value={n}>newest {n} episode{n > 1 ? 's' : ''}</option>)}
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
