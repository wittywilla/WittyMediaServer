import { useEffect, useState } from 'react'
import { api } from '../../api'
import { Spinner } from '../../components/common'
import { toast } from '../../state'
import type { RigView } from '../Rigs'
import { PageHead, Section } from './ui'
import '../rigs.css'

// Settings, Game rigs: adding your own gaming PC, pairing it, choosing which of its apps show
// in the library, and browser play. Sunshine and Moonlight are separate projects, installed by
// you, and this page only ever talks to them: see docs/GAME-RIGS.md.

type Found = { name: string; address: string; port: number; version: string; added: boolean }
type Pairing = { state: string; pin?: string; message?: string; kind?: string }
type App = { id: number; title: string; shown: boolean; itemId: number | null }
type Browser = { available: boolean; installed: boolean; running: boolean; name: string; version: string; license: string; source: string }

export default function GameRigsSettings() {
  const [rigs, setRigs] = useState<RigView[] | null>(null)
  const [browser, setBrowser] = useState<Browser | null>(null)
  const load = () => api.get<RigView[]>('/rigs').then(setRigs).catch((e) => toast(e.message))
  const loadBrowser = () => api.get<Browser>('/rigs/browser').then(setBrowser).catch(() => {})
  useEffect(() => { load(); loadBrowser() }, [])
  if (!rigs) return <Spinner />
  return (
    <>
      <PageHead title="Game rigs" desc="Play the games on your own gaming PC, from the couch or anywhere in the house. The PC runs Sunshine, and you play through Moonlight: both are free, separate projects you install yourself." />
      <AddRig onAdded={load} known={rigs} />
      {rigs.map((r) => <RigCard key={r.id} rig={r} browser={browser} onChange={load} />)}
      <BrowserPlay st={browser} reload={loadBrowser} />
      <Section title="Away from home" desc="The safest way is one that doesn't open your PC to the internet.">
        <ul className="rig-steps">
          <li><b>Browser play</b> reaches the PC through this server, so the PC itself isn’t exposed. You’d open this server’s browser play port and its UDP ports on your router, and it needs HTTPS.</li>
          <li><b>The Moonlight app</b> connects straight to the PC. Put the PC and your device on the same private network (Tailscale or ZeroTier both work, and the PC’s address there goes in “Address away from home” below).</li>
          <li><b>Don’t</b> forward Sunshine’s ports on your router, or turn on its UPnP setting, unless you really mean to: that puts your PC on the internet. And <b>never</b> forward port 47990, its web interface, which can run commands on that PC.</li>
          <li>The wake-up signal goes out from this server, so you can wake the PC while you’re away.</li>
        </ul>
      </Section>
    </>
  )
}

function SetupHelp() {
  return (
    <details>
      <summary>How to install Sunshine on that PC</summary>
      <ol className="rig-steps">
        <li>On the gaming PC, download Sunshine from <a href="https://github.com/LizardByte/Sunshine/releases/latest" target="_blank" rel="noreferrer noopener">its own releases page</a>. On Windows, choose <code>Sunshine-Windows-AMD64-installer.msi</code>. Linux and Mac versions are there too, and <a href="https://docs.lizardbyte.dev/projects/sunshine/latest/md_docs_2getting__started.html" target="_blank" rel="noreferrer noopener">its guide</a> covers them.</li>
        <li>Install it. Windows will ask for permission, as it sets itself up as a service and adds a virtual controller driver.</li>
        <li>On that PC, open <code>https://localhost:47990</code>. Your browser warns about the certificate, which is normal here. Choose a user name and password for Sunshine, and keep them somewhere safe.</li>
        <li>Come back here and press Find, or type the PC’s address. Then you’ll be given a PIN to type into Sunshine’s PIN page on that PC.</li>
        <li>To wake the PC from here later, turn on Wake-on-LAN in the PC’s BIOS and in its network adapter’s settings. It usually needs a wired connection.</li>
      </ol>
      <p className="muted">Sunshine is made by LizardByte and licensed under the GPL-3.0. It isn’t part of this server, and this server contains none of its code.</p>
    </details>
  )
}

function AddRig({ onAdded, known }: { onAdded: () => void; known: RigView[] }) {
  const [address, setAddress] = useState('')
  const [finding, setFinding] = useState(false)
  const [adding, setAdding] = useState(false)
  const [found, setFound] = useState<Found[] | null>(null)
  const [publicWarning, setPublicWarning] = useState('')
  const find = async () => {
    setFinding(true)
    try { setFound(await api.get<Found[]>('/rigs/find')) } catch (e: any) { toast(e.message) } finally { setFinding(false) }
  }
  const add = async (addr: string, confirmPublic = false) => {
    setAdding(true)
    try {
      await api.post('/rigs', { address: addr, confirmPublic })
      setAddress(''); setPublicWarning(''); setFound(null)
      toast('Added. Now pair it.')
      onAdded()
    } catch (e: any) {
      if (e.data?.public) setPublicWarning(e.message)
      else toast(e.message)
    } finally { setAdding(false) }
  }
  return (
    <Section title="Add a PC" desc={known.length ? undefined : 'Start with Sunshine installed and running on the PC.'}>
      <SetupHelp />
      <div className="rig-row-actions">
        <button className="btn" disabled={finding} onClick={find}>{finding ? 'Looking on your network…' : 'Find PCs on my network'}</button>
      </div>
      {found && (found.length === 0
        ? <p className="muted">None answered. Sunshine may not be running, or your network may not pass this kind of search: type its address instead.</p>
        : <ul className="rig-found">{found.map((f) => (
            <li key={f.address}><span><b>{f.name}</b> <span className="muted">{f.address}</span></span>
              {f.added ? <span className="muted">Already added</span> : <button className="btn sm" disabled={adding} onClick={() => add(`${f.address}:${f.port}`)}>Add</button>}
            </li>))}
          </ul>)}
      <form className="rig-form" onSubmit={(e) => { e.preventDefault(); if (address.trim()) add(address.trim()) }}>
        <label>Or type its address
          <input className="input" value={address} onChange={(e) => setAddress(e.target.value)} placeholder="192.168.1.20 or gaming-pc" autoCapitalize="off" autoCorrect="off" spellCheck={false} />
        </label>
        <div><button className="btn" disabled={adding || !address.trim()}>{adding ? 'Checking…' : 'Add this PC'}</button></div>
      </form>
      {publicWarning && (
        <div className="rig-warn" role="alert">
          <p>{publicWarning}</p>
          <button className="btn sm" onClick={() => add(address.trim(), true)}>Add it anyway</button>
        </div>
      )}
    </Section>
  )
}

function RigCard({ rig, browser, onChange }: { rig: RigView; browser: Browser | null; onChange: () => void }) {
  const [pairing, setPairing] = useState<Pairing | null>(null)
  const [apps, setApps] = useState<App[] | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [remote, setRemote] = useState(rig.remoteAddress || '')
  const url = `/rigs/${rig.id}`
  const loadApps = () => api.get<App[]>(`${url}/apps`).then(setApps).catch(() => {})
  useEffect(() => { loadApps() }, [rig.id, rig.paired]) // eslint-disable-line react-hooks/exhaustive-deps
  // While a PIN is waiting, watch for it being typed.
  useEffect(() => {
    if (pairing?.state !== 'waiting') return
    const t = window.setInterval(async () => {
      const p = await api.get<Pairing>(`${url}/pair`).catch(() => null)
      if (!p) return
      setPairing(p)
      if (p.state !== 'waiting') { onChange(); loadApps() }
    }, 1500)
    return () => clearInterval(t)
  }, [pairing?.state]) // eslint-disable-line react-hooks/exhaustive-deps
  const startPair = async (kind: 'pair' | 'browser') => {
    try { setPairing(await api.post<Pairing>(`${url}/${kind}`)) } catch (e: any) { toast(e.message) }
  }
  const cancel = async () => { await api.del(`${url}/pair`).catch(() => {}); setPairing(null) }
  const sync = async () => {
    try { setApps(await api.post<App[]>(`${url}/sync`)); toast('Games updated') } catch (e: any) { toast(e.message) }
  }
  const toggle = async (a: App) => {
    try { setApps(await api.put<App[]>(`${url}/apps/${a.id}`, { shown: !a.shown })) } catch (e: any) { toast(e.message) }
  }
  const saveRemote = async () => {
    try { await api.patch(url, { remoteAddress: remote }); toast('Saved'); onChange() } catch (e: any) { toast(e.message) }
  }
  const remove = async () => {
    try {
      const r = await api.del<{ note?: string }>(url)
      toast(r?.note || 'Removed')
      onChange()
    } catch (e: any) { toast(e.message) }
  }
  const forgetBrowser = async () => {
    try { const r = await api.del<{ note?: string }>(`${url}/browser`); toast(r?.note || 'Done'); onChange() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title={rig.name} desc={[rig.address && (rig.port ? `${rig.address}:${rig.port}` : rig.address), rig.version && `Sunshine host ${rig.version}`].filter(Boolean).join(' · ')}>
      <div className={'rig-status' + (rig.state === 'online' ? ' ok' : '')} role="status"><span className="rig-dot" aria-hidden="true" /><div className="grow">{rig.message}</div></div>
      {rig.public && <p className="rig-warn">This PC’s address is on the internet, so its streaming ports are open to anyone who finds them. A private network is much safer.</p>}

      {pairing?.state === 'waiting' && (
        <div className="rig-warn" role="status">
          <p>{pairing.kind === 'browser' ? 'Pairing browser play.' : 'Pairing this server.'} On that PC, open Sunshine{rig.webUrl ? <> at <code>{rig.webUrl}</code></> : ''}, go to <b>PIN</b>, and type:</p>
          <div className="rig-pin" aria-label={`PIN ${pairing.pin?.split('').join(' ')}`}>{pairing.pin}</div>
          <p className="muted">Waiting for it to be typed. Do this on your home network.</p>
          <button className="btn sm ghost" onClick={cancel}>Cancel</button>
        </div>
      )}
      {pairing && pairing.state !== 'waiting' && pairing.message && <p className={pairing.state === 'failed' ? 'rig-warn' : ''} role="status">{pairing.message}</p>}

      <div className="rig-row-actions">
        {(!rig.paired || rig.state === 'not-paired' || rig.state === 'cert-changed') && pairing?.state !== 'waiting' && <button className="btn primary" onClick={() => startPair('pair')}>{rig.paired ? 'Pair again' : 'Pair this PC'}</button>}
        {rig.paired && <button className="btn sm" onClick={sync}>Read its games again</button>}
        {rig.paired && browser?.installed && !rig.browserPlay && pairing?.state !== 'waiting' && <button className="btn sm" onClick={() => startPair('browser')}>Set up browser play</button>}
        {rig.browserPlay && <button className="btn sm ghost" onClick={forgetBrowser}>Stop browser play on this PC</button>}
        {!confirming && <button className="btn sm ghost" onClick={() => setConfirming(true)}>Remove</button>}
      </div>
      {confirming && (
        <div className="rig-warn" role="alert">
          <p>Remove {rig.name} and its library from this server? Nothing changes on the PC itself: to finish, remove this server from the paired devices in Sunshine there.</p>
          <div className="rig-row-actions">
            <button className="btn sm" onClick={remove}>Remove it</button>
            <button className="btn sm ghost" onClick={() => setConfirming(false)}>Keep it</button>
          </div>
        </div>
      )}

      {apps && apps.length > 0 && (
        <details open={apps.length < 12}>
          <summary>What shows in your library ({apps.filter((a) => a.shown).length} of {apps.length})</summary>
          <p className="muted">Anyone who can see this library can play what’s shown here. Kids and Guest profiles see nothing until you give them a game in Profiles. Remember that keyboard and mouse can often get out of a game to the PC’s desktop, which holds whatever that PC does: allow people accordingly. Desktop apps start hidden for this reason.</p>
          <ul className="rig-apps">
            {apps.map((a) => (
              <li key={a.id} className="rig-app"><span>{a.title}</span>
                <label><input type="checkbox" checked={a.shown} onChange={() => toggle(a)} /> Show</label>
              </li>
            ))}
          </ul>
        </details>
      )}

      {rig.paired && <DevicePIN url={url} />}

      <details>
        <summary>Address away from home</summary>
        <form className="rig-form" onSubmit={(e) => { e.preventDefault(); saveRemote() }}>
          <label>This PC’s address on a private network such as Tailscale or ZeroTier. Moonlight apps use it when they’re away from home.
            <input className="input" value={remote} onChange={(e) => setRemote(e.target.value)} placeholder="100.64.0.5" autoCapitalize="off" autoCorrect="off" spellCheck={false} />
          </label>
          <div><button className="btn sm">Save</button></div>
        </form>
      </details>
    </Section>
  )
}

// Typing a PIN for someone's Moonlight app, without walking to the PC.
function DevicePIN({ url }: { url: string }) {
  const [f, setF] = useState({ pin: '', device: '', username: '', password: '' })
  const [busy, setBusy] = useState(false)
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })
  const send = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await api.post(`${url}/devices`, f)
      setF({ pin: '', device: '', username: f.username, password: '' })
      toast('Paired')
    } catch (err: any) { toast(err.message) } finally { setBusy(false) }
  }
  return (
    <details>
      <summary>Pair a Moonlight app from here</summary>
      <p className="muted">A device pairs once: Moonlight shows a PIN, and it goes into Sunshine. You can type it there, or here. <b>A paired Moonlight app can reach everything on that PC, including things hidden from your library,</b> and this server can’t limit it, so only pair devices you’d hand the PC’s keyboard to.</p>
      <p className="muted">Your Sunshine password goes straight to that PC for this one request. It isn’t saved.</p>
      <form className="rig-form" onSubmit={send}>
        <label>PIN shown by Moonlight<input className="input" inputMode="numeric" maxLength={4} value={f.pin} onChange={set('pin')} /></label>
        <label>Device name, if several are waiting<input className="input" value={f.device} onChange={set('device')} /></label>
        <label>Sunshine user name<input className="input" autoComplete="off" value={f.username} onChange={set('username')} /></label>
        <label>Sunshine password<input className="input" type="password" autoComplete="off" value={f.password} onChange={set('password')} /></label>
        <div><button className="btn sm" disabled={busy || f.pin.length !== 4 || !f.password}>{busy ? 'Sending…' : 'Pair the device'}</button></div>
      </form>
    </details>
  )
}

function BrowserPlay({ st, reload }: { st: Browser | null; reload: () => void }) {
  const [busy, setBusy] = useState(false)
  const [sure, setSure] = useState(false)
  if (!st) return null
  const install = async () => {
    setBusy(true); setSure(false)
    try { await api.post('/rigs/browser/install'); toast('Browser play installed'); reload() } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const remove = async () => {
    setBusy(true)
    try { const r = await api.del<{ note?: string }>('/rigs/browser'); toast(r?.note || 'Removed'); reload() } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  return (
    <Section title="Browser play (optional)" desc="Plays a PC’s games in any modern browser with nothing to install on the device. Moonlight apps are smoother where you can use them.">
      {!st.available && <p className="muted">{st.name} doesn’t publish a build for this kind of server.</p>}
      {st.available && !st.installed && !sure && <button className="btn" disabled={busy} onClick={() => setSure(true)}>{busy ? 'Installing…' : 'Install browser play'}</button>}
      {sure && (
        <div className="rig-warn" role="alert">
          <p>This downloads <b>{st.name} {st.version}</b> from <a href={st.source} target="_blank" rel="noreferrer noopener">its own project</a> and runs it as a separate program on this server. It’s made by other people and licensed under the {st.license}. The download is checked against the exact version this server knows.</p>
          <p className="muted">It’s new software, and has been tested less than the rest of this feature. Earlier versions have been flagged by Windows Defender, which would stop it from starting.</p>
          <div className="rig-row-actions">
            <button className="btn sm" onClick={install}>Download and install</button>
            <button className="btn sm ghost" onClick={() => setSure(false)}>Not now</button>
          </div>
        </div>
      )}
      {st.installed && <>
        <p>{st.name} {st.version} is installed, and {st.running ? 'running' : 'starts when someone presses Play'}. Set it up on each PC above. License: {st.license}, <a href={st.source} target="_blank" rel="noreferrer noopener">source</a>.</p>
        <p className="muted">It listens on its own port (47900 unless you change it) and UDP 47910 to 47920. Game controllers in a browser need HTTPS: turn that on in Remote Access.</p>
        <button className="btn sm ghost" disabled={busy} onClick={remove}>Remove browser play</button>
      </>}
    </Section>
  )
}
