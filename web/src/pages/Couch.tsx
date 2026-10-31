import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, mediaUrl, type Item } from '../api'
import { BackLink, EmptyState, Placeholder, Spinner } from '../components/common'
import LegalNote from '../components/LegalNote'
import { IconGamepad, IconPlay } from '../icons'
import { toast, useApp } from '../state'
import type { CouchHost } from '@wittywilla/wittycouch'
import './rigs.css'

// Your own list of PCs running Sunshine, and the games on them. Anyone allowed can connect to any
// PC they can pair with: nothing here is shared with other accounts.

const Player = lazy(() => import('@wittywilla/wittycouch'))

type Host = { id: number; name: string; address: string; port?: number; paired: boolean; state: string; message: string; currentGame?: string; canWake: boolean; public?: boolean }
type App = { id: number; title: string; hasArt: boolean; isDesktop?: boolean }
type Found = { name: string; address: string; port: number; added: boolean }
type Pairing = { state: string; pin?: string; message?: string }
type Status = { allowed: boolean; ready: boolean; program?: boolean }

export function useCouchStatus() {
  const [st, setSt] = useState<Status | null>(null)
  useEffect(() => { api.get<Status>('/couch').then(setSt).catch(() => setSt({ allowed: false, ready: false })) }, [])
  return st
}

function NotReady({ st }: { st: Status }) {
  const { user } = useApp()
  if (st.ready) return null
  return (
    <div className="rig-warn" role="status">
      <strong>Playing here needs a separate program on this server.</strong>{' '}
      {user?.isAdmin ? <>It’s a GPL-3.0 program built on Moonlight’s own code, installed separately. <Link to="/settings/couch">See Settings, Game PCs.</Link></> : 'Ask whoever runs this server to install it. You can still add and pair your PCs.'}
    </div>
  )
}

// The list of PCs.
export default function CouchPage() {
  const st = useCouchStatus()
  const [hosts, setHosts] = useState<Host[] | null>(null)
  const [address, setAddress] = useState('')
  const [adding, setAdding] = useState(false)
  const [finding, setFinding] = useState(false)
  const [found, setFound] = useState<Found[] | null>(null)
  const load = () => api.get<Host[]>('/couch/hosts').then(setHosts).catch((e) => toast(e.message))
  useEffect(() => { load() }, [])
  const add = async (addr: string) => {
    setAdding(true)
    try {
      await api.post('/couch/hosts', { address: addr })
      setAddress(''); setFound(null)
      await load()
    } catch (e: any) { toast(e.message) } finally { setAdding(false) }
  }
  const find = async () => {
    setFinding(true)
    try { setFound(await api.get<Found[]>('/couch/find')) } catch (e: any) { toast(e.message) } finally { setFinding(false) }
  }
  if (st && !st.allowed) return <EmptyState icon={<IconGamepad />} title="This isn’t turned on for this account" text="Ask whoever runs this server." />
  return (
    <>
      <div className="toolbar"><h1>Game PCs</h1></div>
      <p className="muted">Play the games on any PC running Sunshine, here in your browser. It might be yours, or a friend’s. Each PC pairs once, with a PIN typed on that PC, and your pairings are yours alone.</p>
      <LegalNote kind="thirdPartyServer" />
      {st && <NotReady st={st} />}
      {!hosts ? <Spinner /> : hosts.length > 0 && (
        <ul className="ml-hosts">
          {hosts.map((h) => (
            <li key={h.id}>
              <Link to={`/couch/${h.id}`} className="rig-status">
                <span className={'rig-dot' + (h.state === 'online' || h.state === 'busy' || h.state === 'not-paired' ? ' on' : '')} aria-hidden="true" />
                <div className="grow"><strong>{h.name}</strong><br /><span className="muted">{h.state === 'busy' && h.currentGame ? `Running ${h.currentGame}` : h.paired ? h.message : 'Needs pairing'}</span></div>
              </Link>
            </li>
          ))}
        </ul>
      )}
      <section className="rig-method" aria-label="Add a PC">
        <h2>Add a PC</h2>
        <div className="rig-row-actions"><button className="btn" disabled={finding} onClick={find}>{finding ? 'Looking…' : 'Find PCs on the network'}</button></div>
        {found && (found.length === 0
          ? <p className="muted">None answered. Type its address instead.</p>
          : <ul className="rig-found">{found.map((f) => <li key={f.address}><span><b>{f.name}</b> <span className="muted">{f.address}</span></span>{f.added ? <span className="muted">Already added</span> : <button className="btn sm" disabled={adding} onClick={() => add(f.port && f.port !== 47989 ? `${f.address}:${f.port}` : f.address)}>Add</button>}</li>)}</ul>)}
        <form className="rig-form" onSubmit={(e) => { e.preventDefault(); if (address.trim()) add(address.trim()) }}>
          <label>Or type its address
            <input className="input" value={address} onChange={(e) => setAddress(e.target.value)} placeholder="192.168.1.20 or a friend’s address" autoCapitalize="off" autoCorrect="off" spellCheck={false} />
          </label>
          <div><button className="btn" disabled={adding || !address.trim()}>{adding ? 'Checking…' : 'Add'}</button></div>
        </form>
        <details>
          <summary>The PC needs Sunshine</summary>
          <p>Sunshine is a free program that shares a PC’s games. Whoever owns the PC installs it from <a href="https://github.com/LizardByte/Sunshine/releases/latest" target="_blank" rel="noreferrer noopener">its own releases page</a>, and sets a user name and password at <code>https://localhost:47990</code> on that PC.</p>
          <p className="muted">Connecting to a PC over the internet needs its owner to have opened its ports, which puts it on the internet: a private network such as Tailscale is safer.</p>
        </details>
      </section>
    </>
  )
}

// One PC, its pairing, and what it offers.
export function CouchHostPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const st = useCouchStatus()
  const [host, setHost] = useState<Host | null>(null)
  const [apps, setApps] = useState<App[] | null>(null)
  const [pairing, setPairing] = useState<Pairing | null>(null)
  const [appsError, setAppsError] = useState('')
  const [showAll, setShowAll] = useState(false)
  const [sure, setSure] = useState(false)
  const [waking, setWaking] = useState<'' | 'waiting' | 'late'>('')
  const load = () => api.get<Host[]>('/couch/hosts').then((l) => setHost(l.find((h) => String(h.id) === id) || null)).catch((e) => toast(e.message))
  const loadApps = () => api.get<App[]>(`/couch/hosts/${id}/apps`).then((a) => { setApps(a); setAppsError('') }).catch((e) => setAppsError(e.message))
  useEffect(() => {
    load()
    const t = window.setInterval(() => { if (!document.hidden) load() }, 10000)
    return () => clearInterval(t)
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (host?.paired && (host.state === 'online' || host.state === 'busy') && !apps) loadApps() }, [host?.paired, host?.state]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (pairing?.state !== 'waiting') return
    const t = window.setInterval(async () => {
      const p = await api.get<Pairing>(`/couch/hosts/${id}/pair`).catch(() => null)
      if (!p) return
      setPairing(p)
      if (p.state === 'paired') { await load(); loadApps() }
    }, 1500)
    return () => clearInterval(t)
  }, [pairing?.state, id]) // eslint-disable-line react-hooks/exhaustive-deps
  const pair = async () => { try { setPairing(await api.post<Pairing>(`/couch/hosts/${id}/pair`)) } catch (e: any) { toast(e.message) } }
  const cancelPair = async () => { await api.del(`/couch/hosts/${id}/pair`).catch(() => {}); setPairing(null) }
  const remove = async () => {
    try { const r = await api.del<{ note?: string }>(`/couch/hosts/${id}`); toast(r?.note || 'Removed'); nav('/couch') } catch (e: any) { toast(e.message) }
  }
  const wake = async () => { try { await api.post(`/couch/hosts/${id}/wake`); setWaking('waiting') } catch (e: any) { toast(e.message) } }
  // After a wake-up signal, look often: the games appear by themselves as soon as the PC answers.
  useEffect(() => {
    if (waking !== 'waiting') return
    const t = window.setInterval(load, 3000)
    const late = window.setTimeout(() => setWaking('late'), 120000)
    return () => { clearInterval(t); clearTimeout(late) }
  }, [waking, id]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (host && host.state !== 'asleep') setWaking('') }, [host?.state]) // eslint-disable-line react-hooks/exhaustive-deps
  const quit = async () => { try { await api.post(`/couch/hosts/${id}/quit`); load() } catch (e: any) { toast(e.message) } }
  if (!host) return <Spinner />
  const list = (apps || []).filter((a) => showAll || !a.isDesktop)
  const hidden = (apps || []).length - list.length
  return (
    <div className="game-page">
      <BackLink to="/couch" label="Game PCs" />
      <LegalNote kind="thirdPartyServer" />
      <div className="toolbar"><h1>{host.name}</h1><div className="grow" />{!sure && <button className="btn sm ghost" onClick={() => setSure(true)}>Remove</button>}</div>
      {sure && (
        <div className="rig-warn" role="alert">
          <p>Remove {host.name} from your list? To finish, remove this device from the paired devices in Sunshine on that PC.</p>
          <div className="rig-row-actions"><button className="btn sm" onClick={remove}>Remove it</button><button className="btn sm ghost" onClick={() => setSure(false)}>Keep it</button></div>
        </div>
      )}
      <div className={'rig-status' + (host.state === 'online' ? ' ok' : '')} role="status">
        <span className="rig-dot" aria-hidden="true" />
        <div className="grow">{host.state === 'busy' && host.currentGame ? `Running ${host.currentGame}.` : host.message}</div>
        {host.canWake && waking !== 'waiting' && <button className="btn sm" onClick={wake}>{waking === 'late' ? 'Try again' : 'Wake it up'}</button>}
        {host.state === 'busy' && <button className="btn sm ghost" onClick={quit}>Close what’s running</button>}
      </div>
      {waking === 'waiting' && <p className="muted" role="status">A wake-up signal was sent. Waiting for it to answer, which usually takes under a minute.</p>}
      {waking === 'late' && <p className="rig-warn" role="status">It hasn’t answered after two minutes. Wake-on-LAN has to be on in that PC’s BIOS and network adapter, it usually needs a wired connection, and Sunshine has to start with the PC.</p>}
      {st && <NotReady st={st} />}
      {pairing?.state === 'waiting' ? (
        <div className="rig-warn" role="status">
          <p>On that PC, open Sunshine (<code>https://localhost:47990</code>), go to <b>PIN</b>, and type:</p>
          <div className="rig-pin" aria-label={`PIN ${pairing.pin?.split('').join(' ')}`}>{pairing.pin}</div>
          <p className="muted">Waiting for it to be typed. Don’t pair over a network you don’t trust: someone listening could work out the PIN.</p>
          <button className="btn sm ghost" onClick={cancelPair}>Cancel</button>
        </div>
      ) : (!host.paired || host.state === 'not-paired' || host.state === 'cert-changed') && (
        <div>
          {pairing?.message && <p className="rig-warn" role="status">{pairing.message}</p>}
          <button className="btn primary" onClick={pair}>{host.paired ? 'Pair again' : 'Pair this PC'}</button>
        </div>
      )}
      {appsError && <p className="rig-warn" role="alert">{appsError}</p>}
      {host.paired && (apps ? (
        <>
          <div className="grid">
            {list.map((a) => (
              <Link key={a.id} to={`/couch/${id}/play/${a.id}?t=${encodeURIComponent(a.title)}`} className="card" title={a.title}>
                <div className="poster">
                  {a.hasArt ? <img src={mediaUrl(`/api/couch/hosts/${id}/apps/${a.id}/art`)} alt="" loading="lazy" /> : <Placeholder title={a.title} label="PC" />}
                  <div className="play"><span><IconPlay /></span></div>
                </div>
                <div className="title">{a.title}</div>
                <div className="sub">{host.state === 'busy' && host.currentGame === a.title ? 'Running now: pick up where it is' : host.name}</div>
              </Link>
            ))}
          </div>
          {hidden > 0 && <p><button className="btn sm ghost" onClick={() => setShowAll(true)}>Show the PC’s desktop too</button></p>}
        </>
      ) : !appsError && <Spinner />)}
    </div>
  )
}

function usePlayerRoute(start: CouchHost['start'], title: string, back: string): CouchHost {
  const nav = useNavigate()
  return useMemo<CouchHost>(() => ({
    title, start,
    answer: async (sid, sdp) => (await api.post<{ sdp: string }>(`/couch/sessions/${sid}/webrtc`, { sdp })).sdp,
    end: async (sid, quit) => { await api.del(`/couch/sessions/${sid}${quit ? '?quit=1' : ''}`) },
    close: () => nav(back, { replace: true }),
  }), [title, back]) // eslint-disable-line react-hooks/exhaustive-deps
}

// The full-window page around the player, for one of your own PCs.
export function CouchPlayPage() {
  const { id, app } = useParams()
  const title = new URLSearchParams(window.location.search).get('t') || 'Game'
  const host = usePlayerRoute((req) => api.post(`/couch/hosts/${id}/apps/${app}/play`, req), title, `/couch/${id}`)
  return <Suspense fallback={<Spinner />}><Player host={host} /></Suspense>
}

// The same for a game in a shared PC's library.
export function RigPlayPage() {
  const { id } = useParams()
  const [it, setIt] = useState<Item | null>(null)
  useEffect(() => { api.get<Item>(`/items/${id}`).then(setIt).catch((e) => toast(e.message)) }, [id])
  const host = usePlayerRoute((req) => api.post(`/rigs/games/${id}/stream`, req), it?.title || 'Game', `/item/${id}`)
  if (!it) return <Spinner />
  return <Suspense fallback={<Spinner />}><Player host={host} /></Suspense>
}
