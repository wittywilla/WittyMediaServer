import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { api, type Session } from '../../api'
import { IconCheck, IconClose, IconInfo, IconRefresh, IconShield, IconTrash } from '../../icons'
import { toast } from '../../state'
import QrCode from './QrCode'
import { PageHead, Row, Section, Switch } from './ui'
import './remote.css'
import LegalNote from '../../components/LegalNote'

// Settings → Remote Access: reaching the server from outside the home. It says
// plainly whether that works, from where and how; guides the owner through one
// way of connecting; and proves it with their own phone instead of asking some
// service on the internet.

type Settings = {
  enabled: boolean; method: '' | 'tailscale' | 'netbird' | 'router' | 'proxy' | 'vps'; publicUrl: string
  trustedProxies: string; homeNets: string; awayNets: string; maxHeight: number; maxBitrate: string
  httpsMode: 'off' | 'acme' | 'cert'; httpsPort: number; domain: string; email: string; acceptTerms: boolean
  certFile: string; keyFile: string; portMapping: boolean; externalPort: number
}
type Route = { kind: string; label: string; state: 'ok' | 'waiting' | 'problem'; urls: string[]; detail?: string; lastSeen?: number }
type Notice = { level: 'warn' | 'info'; text: string; trust?: string; link?: string }
type Hit = { at: number; where: string; via: string; ip: string; proof: boolean }
type Probe = { found: boolean; protocol?: string; externalIp?: string; shared?: boolean; error?: string; at: number }
type Status = {
  settings: Settings; forced: boolean; state: 'off' | 'ok' | 'waiting' | 'problem'; headline: string; detail: string
  routes: Route[]; notices: Notice[]; you: { where: string; via: string; ip: string; through?: string }
  vpns: { kind: string; ip: string }[]; publicUrl: string; homePort: string
  https: { mode: string; running: boolean; port: number; error?: string; obtaining?: boolean; cert?: { names: string[]; issuer: string; notAfter: number; daysLeft: number; selfSigned: boolean } }
  mapping: { on: boolean; working: boolean; protocol?: string; externalIp?: string; externalPort?: number; shared?: boolean; error?: string }
  router?: Probe; test?: { url: string; expires: number; hit?: Hit }; lastProof?: Hit; addresses: string[]
  lockouts: { kind: string; addr?: string; name?: string; until: number; fails: number }[]; failures: number
  weakAccounts: { name: string; isAdmin: boolean; unknown: boolean }[]; adminsWithout2fa: number; error?: string
}
type Step = { name: string; state: 'ok' | 'warn' | 'fail'; detail: string }

const whereLabel: Record<string, string> = { machine: 'on this computer', home: 'on your home network', vpn: 'through your VPN', internet: 'from the internet' }
const shortWhere: Record<string, string> = { machine: 'This computer', home: 'At home', vpn: 'VPN', internet: 'Internet' }

function viaLabel(where: string, via: string) {
  if (via === 'https') return 'encrypted with HTTPS'
  if (via === 'proxy') return 'through a reverse proxy'
  if (where === 'vpn') return 'encrypted by the VPN'
  return where === 'internet' ? 'not encrypted' : 'over your own network'
}

const ago = (t?: number) => {
  if (!t) return 'not yet'
  const s = Math.max(0, Math.floor(Date.now() / 1000) - t)
  if (s < 90) return 'just now'
  if (s < 5400) return `${Math.round(s / 60)} minutes ago`
  if (s < 129600) return `${Math.round(s / 3600)} hours ago`
  return `${Math.round(s / 86400)} days ago`
}

function CopyLine({ value }: { value: string }) {
  const copy = async () => {
    try { await navigator.clipboard.writeText(value); toast('Copied') } catch { toast('Select the address and copy it by hand') }
  }
  return <span className="rem-copy"><code>{value}</code><button type="button" className="btn sm ghost" onClick={copy} aria-label={`Copy ${value}`}>Copy</button></span>
}

function Badge({ state }: { state: string }) {
  const text = state === 'ok' ? 'Working' : state === 'problem' ? 'Problem' : state === 'off' ? 'Off' : 'Waiting'
  return <span className={'rem-badge ' + state}>{state === 'ok' ? <IconCheck size={14} /> : state === 'problem' ? <IconClose size={14} /> : <IconInfo size={14} />}{text}</span>
}

export default function RemoteAccess() {
  const [st, setSt] = useState<Status | null>(null)
  const [busy, setBusy] = useState(false)
  const load = useCallback(() => api.get<Status>('/remote').then(setSt), [])
  useEffect(() => { load().catch((e) => toast(e.message)) }, [load])
  // While the page is open and being looked at, so a test or a certificate
  // shows up by itself.
  const live = !!st?.test && !st.test.hit?.proof || !!st?.https.obtaining
  useEffect(() => {
    const t = setInterval(() => { if (!document.hidden) load().catch(() => {}) }, live ? 2500 : 15000)
    return () => clearInterval(t)
  }, [load, live])
  const save = async (change: Partial<Settings>, done?: string) => {
    setBusy(true)
    try {
      const n = await api.put<Status>('/remote', change)
      setSt(n)
      if (n.error) toast(n.error)
      else if (done) toast(done)
      return true
    } catch (e: any) { toast(e.message); return false } finally { setBusy(false) }
  }
  if (!st) return <PageHead title="Remote Access" desc="Loading…" />
  const s = st.settings
  const on = s.enabled || st.forced
  const turn = (v: boolean) => {
    if (!v && !['machine', 'home'].includes(st.you.where) && !confirm('You are connected from outside your home. Turning this off cuts this connection too, and you can only turn it on again from home. Turn it off?')) return
    save({ enabled: v }, v ? 'Remote Access is on' : 'Remote Access is off')
  }
  return (
    <>
      <PageHead title="Remote Access" desc="Watch and listen when you are away from home, without opening your server to the whole internet by accident. Everything here is off until you turn it on, and this server doesn't contact anyone to do it unless a step below says so." />
      <LegalNote kind="remote" className="set-legal" />
      <StatusCard st={st} />
      <Section title="Allow connections from outside your home">
        <Row label="Remote Access" desc={st.forced
          ? 'Turned on by the server’s configuration (the remoteAccess or publicUrl setting), so it can’t be turned off here.'
          : 'While this is off, the server refuses every connection from outside your home network, even if your router lets it through.'}>
          <Switch label="Remote Access" checked={on} disabled={st.forced || busy} onChange={turn} />
        </Row>
        {st.notices.length > 0 && (
          <ul className="rem-notices">
            {st.notices.map((n, i) => (
              <li key={i} className={n.level}>
                <span className="rem-notice-icon" aria-hidden="true">{n.level === 'warn' ? <IconShield size={18} /> : <IconInfo size={18} />}</span>
                <span>
                  {n.level === 'warn' && <span className="sr-only">Warning: </span>}{n.text}
                  {n.link && <> <Link to={n.link}>Go there</Link></>}
                  {n.trust && <> <button type="button" className="btn sm" disabled={busy} onClick={() => save({ trustedProxies: [s.trustedProxies, n.trust].filter(Boolean).join(', ') }, 'Proxy trusted')}>Trust {n.trust}</button></>}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Section>
      <Chooser st={st} save={save} busy={busy} />
      <OutsideTest st={st} reload={load} />
      <Limits s={s} save={save} busy={busy} />
      <People />
      <Protection st={st} reload={load} />
      <Advanced s={s} save={save} busy={busy} />
    </>
  )
}

// ------------------------------------------------------------------ status

function StatusCard({ st }: { st: Status }) {
  const you = `${whereLabel[st.you.where] || st.you.where}, ${viaLabel(st.you.where, st.you.via)}`
  return (
    <section className={'set-card rem-status ' + st.state} aria-label="Status" aria-live="polite">
      <div className="rem-status-head">
        <div><h2>{st.headline}</h2><p>{st.detail}</p></div>
        <Badge state={st.state} />
      </div>
      <ul className="rem-routes">
        {st.routes.map((r) => (
          <li key={r.kind + r.label}>
            <div className="rem-route-top"><b>{r.label}</b><Badge state={r.state} /></div>
            {r.urls.map((u) => <CopyLine key={u} value={u} />)}
            {r.kind === 'home' && r.urls.length === 0 && <small>This server only accepts connections from this computer.</small>}
            {r.detail && <small>{r.detail}</small>}
            {r.kind !== 'home' && <small>Last connection: {ago(r.lastSeen)}</small>}
          </li>
        ))}
      </ul>
      <p className="rem-you">You are connected {you}{st.you.through ? `, via ${st.you.through}` : ''}.</p>
    </section>
  )
}

// ------------------------------------------------------------------ choosing a way in

const METHODS: { id: Settings['method']; name: string; tag?: string; text: string }[] = [
  { id: 'tailscale', name: 'Tailscale', tag: 'Easiest', text: 'A private network between your own devices. Install one app on each; nothing is opened to the internet, and it works even when your provider shares its addresses.' },
  { id: 'netbird', name: 'NetBird', text: 'The same idea as Tailscale, and open source: use their service, or run its coordinator yourself.' },
  { id: 'router', name: 'Forward a port on your router', text: 'Anyone with the address can reach the sign-in page, so it needs HTTPS and strong passwords. Works from any device, with no app to install.' },
  { id: 'proxy', name: 'A reverse proxy you already run', text: 'Caddy, nginx or Traefik in front of this server handles HTTPS. For people who have one, or want one.' },
  { id: 'vps', name: 'A rented server (VPS)', text: 'A small server on the internet receives visitors and passes them to your home over a private tunnel, so your home address stays private and no port opens at home.' },
]

function Chooser({ st, save, busy }: { st: Status; save: (c: Partial<Settings>, d?: string) => Promise<boolean>; busy: boolean }) {
  const s = st.settings
  const method = s.method
  return (
    <Section title="How do you want to connect?" desc="Pick one to see exactly what to do. You can change your mind at any time.">
      <div className="rem-methods" role="radiogroup" aria-label="Way of connecting">
        {METHODS.map((m) => (
          <label key={m.id} className={'rem-method' + (method === m.id ? ' on' : '')}>
            <input type="radio" name="remote-method" checked={method === m.id} disabled={busy} onChange={() => save({ method: m.id })} />
            <span><b>{m.name}{m.tag && <span className="set-badge">{m.tag}</span>}</b><small>{m.text}</small></span>
          </label>
        ))}
        <div className="rem-method disabled" aria-disabled="true">
          <input type="radio" name="remote-method" disabled aria-label="Hosted relay (coming later)" />
          <span><b>WittyMedia relay<span className="set-badge">Coming later</span></b><small>A hosted relay that connects you without any setup. It doesn't exist yet: it is planned as a future paid service, and nothing on this server uses it.</small></span>
        </div>
      </div>
      {(method === 'tailscale' || method === 'netbird') && <VpnSteps st={st} kind={method === 'tailscale' ? 'Tailscale' : 'NetBird'} />}
      {method === 'router' && <RouterSteps st={st} save={save} busy={busy} />}
      {method === 'proxy' && <ProxySteps st={st} save={save} busy={busy} />}
      {method === 'vps' && <VpsSteps st={st} save={save} busy={busy} />}
    </Section>
  )
}

function Done({ yes, children }: { yes: boolean; children: ReactNode }) {
  return <li className={yes ? 'done' : ''}>{yes && <span className="rem-done"><IconCheck size={14} /><span className="sr-only">Done: </span></span>}{children}</li>
}

function VpnSteps({ st, kind }: { st: Status; kind: 'Tailscale' | 'NetBird' }) {
  const found = st.vpns.filter((v) => v.kind === kind || v.kind === 'VPN')
  const site = kind === 'Tailscale' ? 'https://tailscale.com/download' : 'https://netbird.io/download'
  const port = st.homePort
  return (
    <div className="rem-steps">
      <ol>
        <Done yes={found.length > 0}>On this computer, install {kind} from <a href={site} target="_blank" rel="noreferrer">{site.replace('https://', '')}</a> and sign in. You make a free account with {kind}, a service that isn't part of WittyMediaServer.{found.length > 0 && <> Found it: this computer is <code>{found[0].ip}</code>.</>}</Done>
        <Done yes={false}>Install {kind} on your phone or laptop too, and sign in with the same account.</Done>
        <Done yes={st.settings.enabled || st.forced}>Turn on <b>Remote Access</b> above.</Done>
        <Done yes={!!st.lastProof}>Keep {kind} switched on on your phone and open <code>http://{found[0]?.ip || `your-${kind.toLowerCase()}-address`}:{port}</code>. Bookmark it, or enter it in the WittyMedia app. Then run the test below.</Done>
      </ol>
      <p className="rem-note">{kind} helps your devices find each other. Your video travels directly between them whenever it can, encrypted so that only your devices can read it. No port is opened on your router. Devices on your VPN count as away from home: they get the limits below and need a password.</p>
    </div>
  )
}

function CertForm({ st, save, busy }: { st: Status; save: (c: Partial<Settings>, d?: string) => Promise<boolean>; busy: boolean }) {
  const s = st.settings
  const [mode, setMode] = useState(s.httpsMode)
  const [domain, setDomain] = useState(s.domain)
  const [email, setEmail] = useState(s.email)
  const [agree, setAgree] = useState(s.acceptTerms)
  const [certFile, setCert] = useState(s.certFile)
  const [keyFile, setKey] = useState(s.keyFile)
  const [port, setPort] = useState(String(s.httpsPort))
  const apply = () => save({ httpsMode: mode, domain, email, acceptTerms: agree, certFile, keyFile, httpsPort: Number(port) || 0, portMapping: mode === 'off' ? false : s.portMapping }, mode === 'off' ? 'HTTPS is off' : 'HTTPS settings saved')
  return (
    <div className="set-form rem-form">
      <div className="seg" role="radiogroup" aria-label="Certificate">
        {([['off', 'Off'], ['acme', 'Let’s Encrypt'], ['cert', 'My own certificate']] as const).map(([v, label]) => (
          <button key={v} type="button" role="radio" aria-checked={mode === v} className={mode === v ? 'active' : ''} onClick={() => setMode(v)}>{label}</button>
        ))}
      </div>
      {mode === 'acme' && <>
        <label>Your domain name<input className="input" value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="media.example.com" autoCapitalize="none" spellCheck={false} /></label>
        <label>Email for expiry notices (optional)<input className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} /></label>
        <label className="check-row"><input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} /> <span>I accept the <a href="https://letsencrypt.org/repository/" target="_blank" rel="noreferrer">Let’s Encrypt subscriber agreement</a>.</span></label>
        <small>Your server asks Let’s Encrypt, a free non-profit service, for a certificate, and renews it by itself. Let’s Encrypt learns your domain name and the email above, and every certificate is listed publicly, so your domain name will be public. To prove the domain is yours, their servers connect to your outside port 443, which must already lead here.</small>
      </>}
      {mode === 'cert' && <>
        <label>Certificate file (full path)<input className="input mono" value={certFile} onChange={(e) => setCert(e.target.value)} placeholder="/etc/ssl/media/fullchain.pem" spellCheck={false} /></label>
        <label>Private key file (full path)<input className="input mono" value={keyFile} onChange={(e) => setKey(e.target.value)} placeholder="/etc/ssl/media/privkey.pem" spellCheck={false} /></label>
        <small>When the files are renewed, the server picks up the new ones within a minute.</small>
      </>}
      {mode !== 'off' && <label>HTTPS port on this computer<input className="input" inputMode="numeric" value={port} onChange={(e) => setPort(e.target.value)} /></label>}
      <div className="rem-actions">
        <button type="button" className="btn primary sm" disabled={busy} onClick={apply}>Save</button>
        {s.httpsMode === 'acme' && st.https.running && !st.https.cert && (
          <button type="button" className="btn sm" disabled={st.https.obtaining} onClick={() => api.post<Status>('/remote/obtain').then(() => toast('Asking Let’s Encrypt…')).catch((e) => toast(e.message))}>{st.https.obtaining ? 'Asking Let’s Encrypt…' : 'Get the certificate now'}</button>
        )}
      </div>
      {st.https.running && <p className="rem-note"><Badge state={st.https.error ? 'problem' : 'ok'} /> HTTPS port {st.https.port} is open.{st.https.cert ? ` Certificate for ${st.https.cert.names.join(', ')} from ${st.https.cert.issuer}, good for ${st.https.cert.daysLeft} more days.` : ' No certificate yet.'}</p>}
      {st.https.error && <p className="error" role="alert">{st.https.error}</p>}
    </div>
  )
}

function RouterSteps({ st, save, busy }: { st: Status; save: (c: Partial<Settings>, d?: string) => Promise<boolean>; busy: boolean }) {
  const s = st.settings
  const [looking, setLooking] = useState(false)
  const [probe, setProbe] = useState<Probe | undefined>(st.router)
  const look = async () => {
    setLooking(true)
    try { setProbe(await api.post<Probe>('/remote/router')) } catch (e: any) { toast(e.message) } finally { setLooking(false) }
  }
  const lan = st.routes[0]?.urls[0]?.replace(/^https?:\/\//, '').split(':')[0] || 'this computer'
  return (
    <div className="rem-steps">
      <ol>
        <Done yes={!!s.domain}>Get a name for your home, so you don't have to know its address, which can change. A free dynamic DNS name works (for example from DuckDNS or No-IP), or a domain of your own. Set it to your router's outside address.</Done>
        <Done yes={st.https.running}>
          Choose how to get a certificate, so that passwords and video are encrypted:
          <CertForm st={st} save={save} busy={busy} />
        </Done>
        <Done yes={st.mapping.working}>
          On your router, forward outside port <b>{s.externalPort}</b> (TCP) to <code>{lan}</code> port <b>{s.httpsPort}</b>. The router's manual calls this “port forwarding” or “virtual server”. Forward only this port: the server's home port {st.homePort} stays private.
          <div className="rem-subrow">
            <button type="button" className="btn sm" disabled={looking} onClick={look}><IconRefresh /> {looking ? 'Looking…' : 'Check my router'}</button>
            <span className="muted">Only asks the router on your home network what it can do; changes nothing.</span>
          </div>
          {probe && (
            <p className={'rem-note' + (probe.shared ? ' warn' : '')} role="status">
              {!probe.found && <>{probe.error}</>}
              {probe.found && !probe.shared && <>Your router answers ({probe.protocol}). Its outside address is <code>{probe.externalIp}</code>.</>}
              {probe.found && probe.shared && <>Your router's outside address, <code>{probe.externalIp}</code>, is not on the public internet: your provider shares addresses between customers, or there's a second router in front. <b>Port forwarding can't work like this.</b> Use Tailscale or NetBird instead, or ask your provider for a public address.</>}
            </p>
          )}
          <Row label="Do this automatically" desc="Ask the router to open this one port with UPnP or NAT-PMP, and close it again when this is turned off or the server stops. Some routers don't offer this. Only the HTTPS port is ever opened.">
            <Switch label="Automatic port mapping" checked={s.portMapping} disabled={busy || s.httpsMode === 'off'} onChange={(v) => save({ portMapping: v })} />
          </Row>
          {st.mapping.on && (
            <p className={'rem-note' + (st.mapping.working && !st.mapping.shared ? '' : ' warn')} role="status">
              {st.mapping.working
                ? <>Port {st.mapping.externalPort} is open on your router ({st.mapping.protocol}){st.mapping.externalIp && <>, at <code>{st.mapping.externalIp}</code></>}.{st.mapping.shared && ' That address is shared with others, so the internet still can’t reach it.'}</>
                : st.mapping.error || 'Asking the router…'}
            </p>
          )}
        </Done>
        <Done yes={st.settings.enabled || st.forced}>Turn on <b>Remote Access</b> above.</Done>
        <Done yes={!!st.lastProof}>Run the test below with your phone on mobile data.</Done>
      </ol>
      <p className="rem-note">This server refuses unencrypted connections from the internet, so a wrongly forwarded plain port can't expose your password. Anyone can still reach the sign-in page, so use long passwords, and consider two-factor sign-in.</p>
    </div>
  )
}

function TrustForm({ st, save, busy }: { st: Status; save: (c: Partial<Settings>, d?: string) => Promise<boolean>; busy: boolean }) {
  const s = st.settings
  const [url, setUrl] = useState(s.publicUrl)
  const [proxies, setProxies] = useState(s.trustedProxies)
  return (
    <div className="set-form rem-form">
      <label>The outside address<input className="input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://media.example.com" autoCapitalize="none" spellCheck={false} /></label>
      <label>The proxy's address, as this server sees it<input className="input mono" value={proxies} onChange={(e) => setProxies(e.target.value)} placeholder="127.0.0.1" spellCheck={false} /></label>
      <small>Forwarded addresses are believed from these machines only. Until yours is listed, everyone counts as one visitor from the internet, which is safe but makes sign-in limits and the limits below apply to everybody at once.</small>
      <div className="rem-actions"><button type="button" className="btn primary sm" disabled={busy} onClick={() => save({ publicUrl: url, trustedProxies: proxies }, 'Saved')}>Save</button></div>
    </div>
  )
}

const CADDY = (host: string, target: string) => `${host} {\n    reverse_proxy ${target}\n}`

function ProxySteps({ st, save, busy }: { st: Status; save: (c: Partial<Settings>, d?: string) => Promise<boolean>; busy: boolean }) {
  const host = st.settings.publicUrl.replace(/^https:\/\//, '') || 'media.example.com'
  return (
    <div className="rem-steps">
      <ol>
        <li>Point your proxy at this server. For Caddy, this is all it takes (it gets its own certificate):
          <pre className="rem-pre">{CADDY(host, `127.0.0.1:${st.homePort}`)}</pre>
          For nginx, also pass <code>X-Forwarded-For</code> and <code>X-Forwarded-Proto</code>, and turn off body-size limits. <code>docs/REMOTE.md</code> has a full example.
        </li>
        <Done yes={!!st.settings.trustedProxies && !!st.settings.publicUrl}>Tell this server about it:<TrustForm st={st} save={save} busy={busy} /></Done>
        <Done yes={st.settings.enabled || st.forced}>Turn on <b>Remote Access</b> above.</Done>
        <Done yes={!!st.lastProof}>Run the test below with your phone on mobile data.</Done>
      </ol>
    </div>
  )
}

function VpsSteps({ st, save, busy }: { st: Status; save: (c: Partial<Settings>, d?: string) => Promise<boolean>; busy: boolean }) {
  const vpn = st.vpns[0]?.ip
  return (
    <div className="rem-steps">
      <ol>
        <li>Rent a small server, and point your domain name at it.</li>
        <li>Connect it to this computer with a private network: install Tailscale or NetBird on both{vpn && <> (this computer is <code>{vpn}</code>)</>}, or WireGuard if you know it.</li>
        <li>On the rented server, install Caddy and pass visitors on to this computer over that network:
          <pre className="rem-pre">{CADDY(st.settings.publicUrl.replace(/^https:\/\//, '') || 'media.example.com', `${vpn || '100.x.y.z'}:${st.homePort}`)}</pre>
        </li>
        <Done yes={!!st.settings.trustedProxies && !!st.settings.publicUrl}>Tell this server about it. The proxy's address is the rented server's address on your private network:<TrustForm st={st} save={save} busy={busy} /></Done>
        <Done yes={st.settings.enabled || st.forced}>Turn on <b>Remote Access</b> above.</Done>
        <Done yes={!!st.lastProof}>Run the test below with your phone on mobile data.</Done>
      </ol>
      <p className="rem-note">Running this server itself on a rented machine? Reach this page through an SSH tunnel, or start it with <code>-remote-access</code>: see <code>docs/REMOTE.md</code>.</p>
    </div>
  )
}

// ------------------------------------------------------------------ the outside test

function OutsideTest({ st, reload }: { st: Status; reload: () => Promise<void> }) {
  const [address, setAddress] = useState('')
  const [steps, setSteps] = useState<Step[] | null>(null)
  const [checking, setChecking] = useState(false)
  const addr = st.addresses.includes(address) ? address : st.addresses[0] || ''
  const start = async () => {
    try { await api.post('/remote/test', { address: addr }); await reload() } catch (e: any) { toast(e.message) }
  }
  const check = async () => {
    setChecking(true)
    try { setSteps((await api.post<{ steps: Step[] }>('/remote/selfcheck')).steps) } catch (e: any) { toast(e.message) } finally { setChecking(false) }
  }
  const hit = st.test?.hit
  return (
    <Section title="Test it from outside" desc="The only real test is a device that is really outside. Your own phone does it, so no service on the internet is involved and nothing is sent anywhere.">
      {st.addresses.length === 0
        ? <p className="muted">There is nothing to test yet: turn on Remote Access and finish the steps above.</p>
        : <>
          {st.addresses.length > 1 && (
            <Row label="Address to test"><select className="select" value={addr} onChange={(e) => setAddress(e.target.value)}>{st.addresses.map((a) => <option key={a} value={a}>{a}</option>)}</select></Row>
          )}
          <div className="rem-actions">
            <button type="button" className="btn primary sm" onClick={start}>{st.test ? 'Start again' : 'Start the test'}</button>
            {st.publicUrl && <button type="button" className="btn sm" disabled={checking} onClick={check}>{checking ? 'Checking…' : 'Check from this computer'}</button>}
          </div>
          {st.test && (
            <div className="rem-test">
              <QrCode text={st.test.url} label="QR code holding the test address" size={180} />
              <div>
                <ol>
                  <li>On your phone, <b>turn off Wi-Fi</b> so it uses mobile data{addr.includes('//100.') ? ', and keep your VPN on' : ''}.</li>
                  <li>Scan this code with its camera, or type the address:</li>
                </ol>
                <CopyLine value={st.test.url} />
                <div aria-live="polite" role="status">
                  {!hit && <p className="rem-note"><span className="set-pill live">Waiting for your phone</span></p>}
                  {hit && hit.proof && <p className="rem-result ok"><IconCheck size={18} /> It works! Your phone reached the server {whereLabel[hit.where]}, {viaLabel(hit.where, hit.via)}, from {hit.ip}.</p>}
                  {hit && !hit.proof && <p className="rem-result warn"><IconInfo size={18} /> That connection came from {whereLabel[hit.where]}, so it proves nothing yet. Turn off Wi-Fi and open it again.</p>}
                </div>
                <small className="muted">The address is secret, works for 15 minutes, and shows nothing about your server.</small>
              </div>
            </div>
          )}
          {!st.test && st.lastProof && <p className="rem-note"><IconCheck size={14} /> Last confirmed {ago(st.lastProof.at)}, {whereLabel[st.lastProof.where]}, {viaLabel(st.lastProof.where, st.lastProof.via)}.</p>}
          {steps && (
            <ul className="rem-check" aria-live="polite">
              {steps.map((c) => <li key={c.name} className={c.state}><b>{c.name}</b><span>{c.detail}</span></li>)}
            </ul>
          )}
        </>}
    </Section>
  )
}

// ------------------------------------------------------------------ limits

function Limits({ s, save, busy }: { s: Settings; save: (c: Partial<Settings>, d?: string) => Promise<boolean>; busy: boolean }) {
  return (
    <Section title="Limits for viewers away from home" desc="Home internet connections are often slow when sending. These apply to each stream of anyone away from home, including over a VPN. Playback at home is never affected.">
      <Row label="Highest resolution" desc="Larger files are converted first, which takes work from this computer.">
        <select className="select" disabled={busy} value={s.maxHeight} onChange={(e) => save({ maxHeight: Number(e.target.value) }, 'Limit saved')}>
          <option value={0}>No limit</option>
          <option value={2160}>4K</option><option value={1080}>1080p</option><option value={720}>720p</option><option value={480}>480p</option><option value={360}>360p</option>
        </select>
      </Row>
      <Row label="Most data per stream" desc="Video and sound together. Keep it below your connection's upload speed, divided by how many people may watch at once.">
        <select className="select" disabled={busy} value={s.maxBitrate} onChange={(e) => save({ maxBitrate: e.target.value }, 'Limit saved')}>
          <option value="">No limit</option>
          {['20M', '12M', '8M', '6M', '4M', '3M', '2M', '1M'].map((r) => <option key={r} value={r}>{r.replace('M', '')} Mb/s</option>)}
        </select>
      </Row>
    </Section>
  )
}

// ------------------------------------------------------------------ who is connected

function People() {
  const [list, setList] = useState<Session[] | null>(null)
  const load = useCallback(() => api.get<Session[]>('/sessions?all=1').then(setList).catch(() => setList([])), [])
  useEffect(() => {
    load()
    const t = setInterval(() => { if (!document.hidden) load() }, 15000)
    return () => clearInterval(t)
  }, [load])
  const away = (list || []).filter((v) => v.where === 'vpn' || v.where === 'internet')
  const end = async (v: Session) => {
    if (!confirm(`Sign ${v.userName} out on ${v.device || 'this device'}? They'd have to sign in again.`)) return
    try { await api.del(`/sessions/${v.id}`); toast('Signed out'); load() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title="People connected from outside" desc={<>Devices that last used this server from outside your home. Your own devices and sign-ins are under <Link to="/settings/security">Sign-in &amp; security</Link>.</>}>
      {list && away.length === 0 && <p className="muted">Nobody yet.</p>}
      {away.length > 0 && (
        <ul className="rem-sessions">
          {away.map((v) => (
            <li key={v.id}>
              <div>
                <b>{v.userName}</b> · {v.device || 'Unknown device'}
                {v.streaming && <span className="set-pill live">Streaming</span>}
                {v.current && <span className="set-badge">This device</span>}
                <small>{shortWhere[v.where]} · {viaLabel(v.where, v.via)} · {v.ip} · {ago(v.lastSeen)}</small>
              </div>
              {!v.current && <button type="button" className="btn sm ghost danger-hover" onClick={() => end(v)} aria-label={`Sign ${v.userName} out on ${v.device}`}><IconTrash /> Sign out</button>}
            </li>
          ))}
        </ul>
      )}
    </Section>
  )
}

// ------------------------------------------------------------------ protection

function Protection({ st, reload }: { st: Status; reload: () => Promise<void> }) {
  const clear = async () => {
    try { await api.del('/remote/lockouts'); toast('Everyone may try again'); await reload() } catch (e: any) { toast(e.message) }
  }
  const lockText = (l: Status['lockouts'][number]) => {
    const until = new Date(l.until * 1000).toLocaleTimeString()
    if (l.kind === 'address') return `The address ${l.addr} can't sign in until ${until}, after ${l.fails} wrong tries.`
    if (l.kind === 'account') return `“${l.name}” can't sign in from outside your home until ${until}, after ${l.fails} wrong tries. It still works at home.`
    return `“${l.name}” from ${l.addr} can't try again until ${until}.`
  }
  const weak = st.weakAccounts.filter((w) => !w.unknown)
  const unknown = st.weakAccounts.filter((w) => w.unknown)
  return (
    <Section title="Sign-in protection" desc="Always on. Repeated wrong passwords make whoever is guessing wait longer and longer. Nobody on the internet can lock you out at home.">
      <Row label="Wrong sign-ins in the last day"><span className="mod-ver">{st.failures}</span></Row>
      {st.lockouts.length > 0 && (
        <>
          <ul className="rem-notices">{st.lockouts.map((l, i) => <li key={i} className="info"><span className="rem-notice-icon" aria-hidden="true"><IconShield size={18} /></span><span>{lockText(l)}</span></li>)}</ul>
          <div className="rem-actions"><button type="button" className="btn sm" onClick={clear}>Let everyone try again</button></div>
        </>
      )}
      <Row label="Passwords" desc="How strong they are is only known when someone signs in with them, and they are never stored.">
        <span className={weak.length ? 'rem-text warn' : 'rem-text'}>
          {weak.length > 0 ? `Weak: ${weak.map((w) => w.name).join(', ')}` : unknown.length > 0 ? `Not checked yet: ${unknown.map((w) => w.name).join(', ')}` : 'All fine'}
        </span>
      </Row>
      <Row label="Two-factor sign-in" desc="A code from an app on your phone, on top of your password. Each adult turns it on for themselves.">
        <Link className="btn sm" to="/settings/security">{st.adminsWithout2fa > 0 ? 'Set it up' : 'On for administrators'}</Link>
      </Row>
    </Section>
  )
}

// ------------------------------------------------------------------ advanced

function Advanced({ s, save, busy }: { s: Settings; save: (c: Partial<Settings>, d?: string) => Promise<boolean>; busy: boolean }) {
  const [url, setUrl] = useState(s.publicUrl)
  const [home, setHome] = useState(s.homeNets)
  const [away, setAway] = useState(s.awayNets)
  const [proxies, setProxies] = useState(s.trustedProxies)
  const dirty = url !== s.publicUrl || home !== s.homeNets || away !== s.awayNets || proxies !== s.trustedProxies
  return (
    <details className="set-card rem-advanced">
      <summary><h2>Advanced</h2></summary>
      <div className="set-form">
        <label>Outside address<input className="input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://media.example.com" autoCapitalize="none" spellCheck={false} />
          <small>Where visitors from the internet are sent when they use the unencrypted port.</small></label>
        <label>Trusted reverse proxies<input className="input mono" value={proxies} onChange={(e) => setProxies(e.target.value)} placeholder="127.0.0.1, 10.0.0.2" spellCheck={false} />
          <small>Addresses or networks, separated by commas. X-Forwarded-For is believed from these only, and from nobody by default.</small></label>
        <label>Also count as my home network<input className="input mono" value={home} onChange={(e) => setHome(e.target.value)} placeholder="203.0.113.0/24" spellCheck={false} />
          <small>Private addresses already count. Devices here can see profile names and sign in without a password where allowed, so list only what you control.</small></label>
        <label>Count as away from home<input className="input mono" value={away} onChange={(e) => setAway(e.target.value)} placeholder="10.8.0.0/24" spellCheck={false} />
          <small>A VPN of your own that uses private addresses, for example WireGuard. Tailscale and NetBird are recognised by themselves.</small></label>
        <div className="rem-actions"><button type="button" className="btn primary sm" disabled={busy || !dirty} onClick={() => save({ publicUrl: url, homeNets: home, awayNets: away, trustedProxies: proxies }, 'Saved')}>Save</button></div>
      </div>
    </details>
  )
}
