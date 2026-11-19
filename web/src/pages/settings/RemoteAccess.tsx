// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { HelpLink } from '../../components/HelpLink'
import { Link } from 'react-router-dom'
import { api, type Session } from '../../api'
import { IconCheck, IconClose, IconInfo, IconRefresh, IconShield } from '../../icons'
import { toast } from '../../state'
import QrCode from './QrCode'
import { LoadError } from '../../components/common'
import { PageHead, Row, Section, Switch } from './ui'
import { OwnerFields } from './ConfirmOwner'
import { Button, Dialog } from '../../ui'
import './remote.css'
import LegalNote from '../../components/LegalNote'
import { ask } from '../../ui/Overlay'
import { Segmented } from '../../ui/Choice'
import PeerToPeer from './PeerToPeer'
import SpecPage from '../../spec/SpecPage'
import { handledIds } from '../../spec/model'
import { advancedSpec, chooserSpec, limitsSpec, outsideSpec, protectionSpec, type Choice } from '../../spec/specs/remoteAccess'
import { date, t, tk, tn, tx } from '../../i18n'

// Settings → Remote Access: who can use the server, and from where. It says
// plainly whether that works, from where and how; guides the owner through one
// way of connecting; and proves it with their own phone instead of asking some
// service on the internet.

type Settings = {
  enabled: boolean; method: '' | 'tailscale' | 'netbird' | 'router' | 'proxy' | 'vps'; publicUrl: string
  trustedProxies: string; homeNets: string; awayNets: string; maxHeight: number; maxBitrate: string
  httpsMode: 'off' | 'acme' | 'cert'; httpsPort: number; domain: string; email: string; acceptTerms: boolean
  certFile: string; keyFile: string; portMapping: boolean; externalPort: number; allowPlainAway: boolean
}
type Route = { kind: string; label: string; state: 'ok' | 'waiting' | 'problem'; urls: string[]; detail?: string; lastSeen?: number }
type Notice = { level: 'warn' | 'info'; text: string; trust?: string; link?: string }
type Hit = { at: number; where: string; via: string; ip: string; proof: boolean }
type Probe = { found: boolean; protocol?: string; externalIp?: string; shared?: boolean; error?: string; at: number }
type Standing = { level: 'good' | 'waiting' | 'fault'; subject: string; summary: string; home: string; away: string; checked: string; warning?: string }
type Status = Standing & {
  settings: Settings; forced: boolean; state: 'off' | 'ok' | 'waiting' | 'problem'
  routes: Route[]; notices: Notice[]; you: { where: string; via: string; ip: string; through?: string }
  vpns: { kind: string; ip: string }[]; publicUrl: string; homePort: string
  https: { mode: string; running: boolean; port: number; error?: string; obtaining?: boolean; cert?: { names: string[]; issuer: string; notAfter: number; daysLeft: number; selfSigned: boolean } }
  mapping: { on: boolean; working: boolean; protocol?: string; externalIp?: string; externalPort?: number; shared?: boolean; error?: string }
  router?: Probe; test?: { url: string; expires: number; hit?: Hit }; lastProof?: Hit; addresses: string[]
  lockouts: { kind: string; addr?: string; name?: string; until: number; fails: number }[]; failures: number
  weakAccounts: { name: string; isAdmin: boolean; unknown: boolean }[]; adminsWithout2fa: number; error?: string
  plainSessions: number; ended?: number
}
type Step = { name: string; state: 'ok' | 'warn' | 'fail'; detail: string }
type Save = (c: Partial<Settings>, d?: string) => Promise<boolean>

const whereLabels: Record<string, string> = { machine: tk('on this computer'), home: tk('on your home network'), vpn: tk('through your VPN'), internet: tk('from the internet') }
const whereLabel = (where: string) => tx(whereLabels[where]) || where

function viaLabel(where: string, via: string) {
  if (via === 'p2p') return t('peer to peer, encrypted to this server')
  if (via === 'p2p-relay') return t('peer to peer through a relay, encrypted to this server')
  if (via === 'https') return t('encrypted with HTTPS')
  if (via === 'proxy') return t('through a reverse proxy')
  if (where === 'vpn') return t('encrypted by the VPN')
  return where === 'internet' ? t('not encrypted') : t('over your own network')
}

const ago = (at?: number) => {
  if (!at) return t('not yet')
  const s = Math.max(0, Math.floor(Date.now() / 1000) - at)
  if (s < 90) return t('just now')
  if (s < 5400) return tn('{count} minute ago', '{count} minutes ago', Math.round(s / 60))
  if (s < 129600) return tn('{count} hour ago', '{count} hours ago', Math.round(s / 3600))
  return tn('{count} day ago', '{count} days ago', Math.round(s / 86400))
}

function CopyLine({ value }: { value: string }) {
  const copy = async () => {
    try { await navigator.clipboard.writeText(value); toast(t('Copied')) } catch { toast(t('Select the address and copy it by hand')) }
  }
  return <span className="rem-copy"><code>{value}</code><button type="button" className="btn sm ghost" onClick={copy} aria-label={t('Copy {value}', { value })}>{t('Copy')}</button></span>
}

// One way in, in words. Never a bare on or off: each says what it means.
function Badge({ state }: { state: string }) {
  const text = state === 'ok' ? t('Working') : state === 'problem' ? t('Problem') : t('Not confirmed yet')
  return <span className={'rem-badge ' + state}>{state === 'ok' ? <IconCheck size={14} /> : state === 'problem' ? <IconClose size={14} /> : <IconInfo size={14} />}{text}</span>
}

// Who each choice lets in. The rest of the page follows the one chosen.
const PRIVATE: Settings['method'][] = ['tailscale', 'netbird']
const INTERNET: Settings['method'][] = ['router', 'proxy', 'vps']

function choiceOf(st: Status): Choice {
  const s = st.settings
  if (!s.enabled && !st.forced) return 'home'
  if (PRIVATE.includes(s.method)) return 'private'
  if (INTERNET.includes(s.method)) return 'internet'
  return s.httpsMode !== 'off' || s.publicUrl || s.trustedProxies ? 'internet' : 'private'
}


export default function RemoteAccess() {
  const [st, setSt] = useState<Status | null>(null)
  const [busy, setBusy] = useState(false)
  const load = useCallback(() => api.get<Status>('/remote').then(setSt), [])
  const [failed, setFailed] = useState(false)
  useEffect(() => { load().catch(() => setFailed(true)) }, [load])
  // While the page is open and being looked at, so a test or a certificate
  // shows up by itself.
  const live = !!st?.test && !st.test.hit?.proof || !!st?.https.obtaining
  useEffect(() => {
    const t = setInterval(() => { if (!document.hidden) load().catch(() => {}) }, live ? 2500 : 15000)
    return () => clearInterval(t)
  }, [load, live])
  // The bar on every page reads the same status, so it hears of changes at once.
  const applied = (n: Status) => { setSt(n); window.dispatchEvent(new Event('wms-remote-changed')) }
  const save: Save = async (change, done) => {
    setBusy(true)
    try {
      const n = await api.put<Status>('/remote', change)
      applied(n)
      if (n.error) toast(n.error)
      else if (done) toast(n.ended ? tn('{done}. One device was signed out.', '{done}. {count} devices were signed out.', n.ended, { done }) : done)
      return true
    } catch (e: any) { toast(e.message); return false } finally { setBusy(false) }
  }
  if (!st && failed) return <LoadError what={t('Remote Access')} onRetry={() => { setFailed(false); load().catch(() => setFailed(true)) }} />
  if (!st) return <PageHead title={tk('Remote Access')} desc={t('Loading…')} />
  const s = st.settings
  const choice = choiceOf(st)
  return (
    <>
      <PageHead title={tk('Remote Access')} desc={t("Who can use this server, and from where. It starts as home network only, and this server doesn't contact anyone for remote access unless a step below says so.")} />
      <StatusCard st={st} save={save} busy={busy} />
      <Chooser st={st} choice={choice} save={save} busy={busy} />
      <PeerToPeer />
      {(choice !== 'home' || s.allowPlainAway) && <PlainAway st={st} save={save} busy={busy} applied={applied} reload={load} />}
      {choice !== 'home' && <OutsideTest st={st} reload={load} />}
      {choice !== 'home' && <Limits s={s} save={save} busy={busy} />}
      <People />
      <Protection st={st} reload={load} />
      <Advanced s={s} save={save} busy={busy} />
      <LegalNote kind="remote" className="set-legal" />
    </>
  )
}

// ------------------------------------------------------------------ status

// The answer first: who can connect right now, from where, and how that is
// known, in the words the server gives every surface.
function StatusCard({ st, save, busy }: { st: Status; save: Save; busy: boolean }) {
  const you = t('{where}, {how}', { where: whereLabel(st.you.where), how: viaLabel(st.you.where, st.you.via) })
  const level = st.level === 'good' ? 'ok' : st.level === 'fault' ? 'problem' : 'waiting'
  // Not plain ok and problem: those are app-wide classes that colour all the text.
  const homeOk = st.routes[0]?.urls.length > 0 || st.you.where === 'machine'
  const awayOk = st.state === 'ok'
  const ways = st.routes.slice(1)
  return (
    <section className={'set-card rem-status rem-' + st.level} aria-labelledby="rem-subject">
      <div aria-live="polite">
        <h2 id="rem-subject" className="rem-subject"><span className="rem-level" aria-hidden="true">{level === 'ok' ? <IconCheck size={20} /> : level === 'problem' ? <IconClose size={20} /> : <IconInfo size={20} />}</span>{st.subject}</h2>
        <p className="rem-summary">{st.summary}</p>
        <ul className="rem-who">
          <li className={homeOk ? 'yes' : 'part'}><span aria-hidden="true">{homeOk ? <IconCheck size={16} /> : <IconInfo size={16} />}</span>{st.home}</li>
          <li className={awayOk ? 'yes' : 'no'}><span aria-hidden="true">{awayOk ? <IconCheck size={16} /> : <IconClose size={16} />}</span>{st.away}</li>
        </ul>
        <p className="rem-checked">{st.checked}</p>
        {st.warning && <p className="rem-plain-warn"><span aria-hidden="true"><IconShield size={18} /></span><span><b>{t('Warning:')}</b> {st.warning}{st.plainSessions > 0 && <> {tn('One device is signed in that way.', '{count} devices are signed in that way.', st.plainSessions)}</>}</span></p>}
      </div>
      {st.notices.length > 0 && (
        <ul className="rem-notices">
          {st.notices.map((n, i) => (
            <li key={i} className={n.level}>
              <span className="rem-notice-icon" aria-hidden="true">{n.level === 'warn' ? <IconShield size={18} /> : <IconInfo size={18} />}</span>
              <span>
                {n.level === 'warn' && <span className="sr-only">{t('Warning:')} </span>}{n.text}
                {n.link && <> <Link to={n.link}>{t('Go there')}</Link></>}
                {n.trust && <> <button type="button" className="btn sm" disabled={busy} onClick={() => save({ trustedProxies: [st.settings.trustedProxies, n.trust].filter(Boolean).join(', ') }, t('Proxy trusted'))}>{t('Trust {address}', { address: n.trust })}</button></>}
              </span>
            </li>
          ))}
        </ul>
      )}
      <ul className="rem-routes" aria-label={t('Addresses')}>
        <li>
          <div className="rem-route-top"><b>{t('At home')}</b></div>
          {st.routes[0]?.urls.map((u) => <CopyLine key={u} value={u} />)}
          {st.routes[0]?.urls.length === 0 && <small>{t('This server only accepts connections from this computer.')}</small>}
        </li>
        {ways.map((r) => (
          <li key={r.kind + r.label}>
            <div className="rem-route-top"><b>{r.kind === 'vpn' ? t('Away, through {name}', { name: r.label }) : t('Away, {way}', { way: tx(r.label).charAt(0).toLowerCase() + tx(r.label).slice(1) })}</b><Badge state={r.state} /></div>
            {r.urls.map((u) => <CopyLine key={u} value={u} />)}
            {r.detail && <small>{r.detail}</small>}
            <small>{t('Last connection: {when}', { when: ago(r.lastSeen) })}</small>
          </li>
        ))}
      </ul>
      <p className="rem-you">{st.you.through ? t('You’re connected {how}, via {through}.', { how: you, through: st.you.through }) : t('You’re connected {how}.', { how: you })}</p>
    </section>
  )
}

// ------------------------------------------------------------------ choosing who can connect

// Rows that choose with a click, Enter or the space bar, never just by passing over them: arrowing through to
// reach what's below mustn't let the internet in.
function Chooser({ st, choice, save, busy }: { st: Status; choice: Choice; save: Save; busy: boolean }) {
  const s = st.settings
  const away = !['machine', 'home'].includes(st.you.where)
  const page = useMemo(() => chooserSpec(choice, st.forced, busy), [choice, st.forced, busy])
  const handled = useMemo(() => handledIds(page), [page])
  const choose = async (c: Choice) => {
    if (c === choice || busy) return
    if (c === 'home') {
      if (away && !await ask({ title: t('Choose home network only while you’re away from home?'), body: t('You’re connected from outside your home, so this cuts your connection too. You can only choose something else again from home.'), confirm: t('Home network only'), danger: true })) return
      save({ enabled: false }, t('Home network only: connections from away are refused'))
      return
    }
    if (c === 'private') {
      const closes = s.httpsMode !== 'off' || s.portMapping
      if (closes && !await ask({ title: t('Close the HTTPS port as well?'), body: t('A private network doesn’t need it. This turns off HTTPS and automatic port mapping, so nothing on this server is open to the internet. Also delete any port forward you made on your router.'), confirm: t('Close it and use a private network') })) return
      save({ enabled: true, method: s.method === 'netbird' ? 'netbird' : 'tailscale', ...(closes ? { httpsMode: 'off' as const, portMapping: false } : {}) }, t('Home and away, through a private network'))
      return
    }
    if (choice === 'home' && !await ask({ title: t('Let devices away from home in, through the internet?'), body: t('This takes effect at once: as soon as a way in is set up below, anyone with the address can reach the sign-in page. Before you set one up, give every adult account a long, unique password and turn on two-factor sign-in.'), confirm: t('Choose it') })) return
    save({ enabled: true, method: INTERNET.includes(s.method) ? s.method : 'router' }, t('Home and away, through the internet'))
  }
  return (
    <Section title={tk('Who can connect')} desc={t('Choose one. You can change your mind at any time, and only what your choice needs is shown below it.')}>
      <SpecPage embedded page={page} handled={handled} onAction={(id) => void choose(id.slice(7) as Choice)} />
      <div className="rem-method disabled" role="note">
        <span><b>{t('Home and away, through WittyRelay')}<span className="set-badge">{t('Coming later')}</span></b><small>{t('A hosted relay that would connect you without any setup. It doesn’t exist yet: it’s planned as a free service, and nothing on this server uses it.')}</small></span>
      </div>
      {choice === 'home' && <HomeOnly />}
      {choice === 'private' && <PrivateSteps st={st} save={save} busy={busy} />}
      {choice === 'internet' && <InternetSteps st={st} save={save} busy={busy} />}
    </Section>
  )
}

function HomeOnly() {
  return (
    <div className="rem-steps">
      <p className="rem-note">{t('Nothing to set up. Your TV, phone and computers at home use the addresses above. Connections from anywhere else are refused by this server itself, even if your router lets them through.')} {t('To watch away from home, a private network is the safe way, and')} <HelpLink to="remote#the-recommended-way-a-private-network">{t('the guide')}</HelpLink> {t('walks you through it.')}</p>
    </div>
  )
}

function PrivateSteps({ st, save, busy }: { st: Status; save: Save; busy: boolean }) {
  const kind = st.settings.method === 'netbird' ? 'NetBird' : 'Tailscale'
  return (
    <>
      <div className="rem-subrow">
        <Segmented label={t('Private network app')} value={kind} onChange={(v) => { if (!busy) save({ method: v === 'NetBird' ? 'netbird' : 'tailscale' }) }}
          options={[{ value: 'Tailscale', label: 'Tailscale' }, { value: 'NetBird', label: 'NetBird' }]} />
      </div>
      <VpnSteps st={st} kind={kind} />
    </>
  )
}

const INTERNET_WAYS: { id: Settings['method']; name: string; text: string }[] = [
  { id: 'router', name: tk('A port on your router'), text: tk('Anyone with the address can reach the sign-in page, so it needs HTTPS and strong passwords. Works from any device, with no app to install.') },
  { id: 'proxy', name: tk('A reverse proxy you already run'), text: tk('Caddy, nginx or Traefik in front of this server handles HTTPS. For people who already run one and keep it updated.') },
  { id: 'vps', name: tk('A rented server (VPS)'), text: tk('A small server on the internet receives visitors and passes them to your home over a private tunnel, so your home address stays private and no port opens at home.') },
]

function InternetSteps({ st, save, busy }: { st: Status; save: Save; busy: boolean }) {
  const method = INTERNET.includes(st.settings.method) ? st.settings.method : 'router'
  const way = INTERNET_WAYS.find((w) => w.id === method)!
  return (
    <>
      <p className="rem-note warn"><b>{t('With this choice your sign-in page is on the internet.')}</b> {t('Scanners find it and keep trying. Use long, unique passwords and two-factor sign-in, and keep this server updated. Never forward port {port}: it’s the unencrypted door, for home only. A private network avoids all of this.', { port: st.homePort })}</p>
      <div className="rem-subrow">
        <Segmented label={t('Way in from the internet')} value={method} onChange={(v) => { if (!busy) save({ method: v }) }}
          options={INTERNET_WAYS.map((w) => ({ value: w.id, label: tx(w.name) }))} />
      </div>
      <p className="muted rem-way">{tx(way.text)}</p>
      {method === 'router' && <RouterSteps st={st} save={save} busy={busy} />}
      {method === 'proxy' && <ProxySteps st={st} save={save} busy={busy} />}
      {method === 'vps' && <VpsSteps st={st} save={save} busy={busy} />}
    </>
  )
}

function Done({ yes, children }: { yes: boolean; children: ReactNode }) {
  return <li className={yes ? 'done' : ''}>{yes && <span className="rem-done"><IconCheck size={14} /><span className="sr-only">{t('Done:')} </span></span>}{children}</li>
}

function VpnSteps({ st, kind }: { st: Status; kind: 'Tailscale' | 'NetBird' }) {
  const found = st.vpns.filter((v) => v.kind === kind || v.kind === 'VPN')
  const site = kind === 'Tailscale' ? 'https://tailscale.com/download' : 'https://netbird.io/download'
  const port = st.homePort
  return (
    <div className="rem-steps">
      <ol>
        <Done yes={found.length > 0}>{t('On this computer, install {name} from', { name: kind })} <a href={site} target="_blank" rel="noreferrer">{site.replace('https://', '')}</a> {t("and sign in. You make a free account with {name}, a service that isn't part of WittyMediaServer.", { name: kind })}{found.length > 0 && <> {t('Found it: this computer is')} <code>{found[0].ip}</code>.</>}</Done>
        <Done yes={false}>{t('Install {name} on your phone or laptop too, and sign in with the same account.', { name: kind })}</Done>
        <Done yes={!!st.lastProof}>{t('Keep {name} switched on on your phone and open', { name: kind })} <code>http://{found[0]?.ip || `your-${kind.toLowerCase()}-address`}:{port}</code>. {t('Bookmark it, or enter it in the WittyMedia app. Then run the check below.')}</Done>
      </ol>
      <p className="rem-note">{t('{name} helps your devices find each other. Your video travels directly between them whenever it can, encrypted so that only your devices can read it. No port is opened on your router. Devices on your VPN count as away from home: they get the limits below and need a password.', { name: kind })}</p>
    </div>
  )
}

function CertForm({ st, save, busy }: { st: Status; save: Save; busy: boolean }) {
  const s = st.settings
  const [mode, setMode] = useState(s.httpsMode)
  const [domain, setDomain] = useState(s.domain)
  const [email, setEmail] = useState(s.email)
  const [agree, setAgree] = useState(s.acceptTerms)
  const [certFile, setCert] = useState(s.certFile)
  const [keyFile, setKey] = useState(s.keyFile)
  const [port, setPort] = useState(String(s.httpsPort))
  const apply = () => save({ httpsMode: mode, domain, email, acceptTerms: agree, certFile, keyFile, httpsPort: Number(port) || 0, portMapping: mode === 'off' ? false : s.portMapping }, mode === 'off' ? t('HTTPS is off') : t('HTTPS settings saved'))
  return (
    <div className="set-form rem-form">
      <Segmented label={t('Certificate')} value={mode} onChange={setMode} options={[{ value: 'off', label: t('Off') }, { value: 'acme', label: 'Let’s Encrypt' }, { value: 'cert', label: t('My own certificate') }]} />
      {mode === 'acme' && <>
        <label>{t('Your domain name')}<input className="input" value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="media.example.com" autoCapitalize="none" spellCheck={false} /></label>
        <label>{t('Email for expiry notices (optional)')}<input className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} /></label>
        <label className="check-row"><input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} /> <span>{t('I accept the')} <a href="https://letsencrypt.org/repository/" target="_blank" rel="noreferrer">{t('Let’s Encrypt subscriber agreement')}</a>.</span></label>
        <small>{t('Your server asks Let’s Encrypt, a free non-profit service, for a certificate, and renews it by itself. Let’s Encrypt learns your domain name and the email above, and every certificate is listed publicly, so your domain name will be public. To prove the domain is yours, their servers connect to your outside port 443, which must already lead here.')}</small>
      </>}
      {mode === 'cert' && <>
        <label>{t('Certificate file (full path)')}<input className="input mono" value={certFile} onChange={(e) => setCert(e.target.value)} placeholder="/etc/ssl/media/fullchain.pem" spellCheck={false} /></label>
        <label>{t('Private key file (full path)')}<input className="input mono" value={keyFile} onChange={(e) => setKey(e.target.value)} placeholder="/etc/ssl/media/privkey.pem" spellCheck={false} /></label>
        <small>{t('When the files are renewed, the server picks up the new ones within a minute.')}</small>
      </>}
      {mode !== 'off' && <label>{t('HTTPS port on this computer')}<input className="input" inputMode="numeric" value={port} onChange={(e) => setPort(e.target.value)} /></label>}
      <div className="rem-actions">
        <button type="button" className="btn primary sm" disabled={busy} onClick={apply}>{t('Save')}</button>
        {s.httpsMode === 'acme' && st.https.running && !st.https.cert && (
          <button type="button" className="btn sm" disabled={st.https.obtaining} onClick={() => api.post<Status>('/remote/obtain').then(() => toast(t('Asking Let’s Encrypt…'))).catch((e) => toast(e.message))}>{st.https.obtaining ? t('Asking Let’s Encrypt…') : t('Get the certificate now')}</button>
        )}
      </div>
      {st.https.running && <p className="rem-note"><Badge state={st.https.error ? 'problem' : 'ok'} /> {t('HTTPS port {port} is open.', { port: st.https.port })} {st.https.cert ? tn('Certificate for {names} from {issuer}, good for {count} more day.', 'Certificate for {names} from {issuer}, good for {count} more days.', st.https.cert.daysLeft, { names: st.https.cert.names.join(', '), issuer: st.https.cert.issuer }) : t('No certificate yet.')}</p>}
      {st.https.error && <p className="error" role="alert">{st.https.error}</p>}
    </div>
  )
}

function RouterSteps({ st, save, busy }: { st: Status; save: Save; busy: boolean }) {
  const s = st.settings
  const [looking, setLooking] = useState(false)
  const [probe, setProbe] = useState<Probe | undefined>(st.router)
  const look = async () => {
    setLooking(true)
    try { setProbe(await api.post<Probe>('/remote/router')) } catch (e: any) { toast(e.message) } finally { setLooking(false) }
  }
  const lan = st.routes[0]?.urls[0]?.replace(/^https?:\/\//, '').split(':')[0] || t('this computer')
  return (
    <div className="rem-steps">
      <ol>
        <Done yes={!!s.domain}>{t("Get a name for your home, so you don't have to know its address, which can change. A free dynamic DNS name works (for example from DuckDNS or No-IP), or a domain of your own. Set it to your router's outside address.")}</Done>
        <Done yes={st.https.running}>
          {t('Choose how to get a certificate, so that passwords and video are encrypted:')}
          <CertForm st={st} save={save} busy={busy} />
        </Done>
        <Done yes={st.mapping.working}>
          {t("On your router, forward outside port {outside} (TCP) to {address} port {port}. The router's manual calls this “port forwarding” or “virtual server”. Forward only this port: the server's home port {home} stays private.", { outside: s.externalPort, address: lan, port: s.httpsPort, home: st.homePort })}
          <div className="rem-subrow">
            <button type="button" className="btn sm" disabled={looking} onClick={look}><IconRefresh /> {looking ? t('Looking…') : t('Check my router')}</button>
            <span className="muted">{t('Only asks the router on your home network what it can do; changes nothing.')}</span>
          </div>
          {probe && (
            <p className={'rem-note' + (probe.shared ? ' warn' : '')} role="status">
              {!probe.found && <>{probe.error}</>}
              {probe.found && !probe.shared && <>{t('Your router answers ({protocol}). Its outside address is', { protocol: probe.protocol || '' })} <code>{probe.externalIp}</code>.</>}
              {probe.found && probe.shared && <>{t("Your router's outside address, {address}, is not on the public internet: your provider shares addresses between customers, or there's a second router in front.", { address: probe.externalIp || '' })} <b>{t("Port forwarding can't work like this.")}</b> {t('Use Tailscale or NetBird instead, or ask your provider for a public address.')}</>}
            </p>
          )}
          <Row label={t('Do this automatically')} desc={t("Ask the router to open this one port with UPnP or NAT-PMP, and close it again when this is turned off or the server stops. Some routers don't offer this. Only the HTTPS port is ever opened.")}>
            <Switch label={t('Automatic port mapping')} checked={s.portMapping} disabled={busy || s.httpsMode === 'off'} onChange={(v) => save({ portMapping: v })} />
          </Row>
          {st.mapping.on && (
            <p className={'rem-note' + (st.mapping.working && !st.mapping.shared ? '' : ' warn')} role="status">
              {st.mapping.working
                ? <>{st.mapping.externalIp
                  ? t('Port {port} is open on your router ({protocol}), at {address}.', { port: st.mapping.externalPort ?? '', protocol: st.mapping.protocol || '', address: st.mapping.externalIp })
                  : t('Port {port} is open on your router ({protocol}).', { port: st.mapping.externalPort ?? '', protocol: st.mapping.protocol || '' })}{st.mapping.shared && <> {t('That address is shared with others, so the internet still can’t reach it.')}</>}</>
                : st.mapping.error || t('Asking the router…')}
            </p>
          )}
        </Done>
        <Done yes={!!st.lastProof}>{t('Run the check below with your phone on mobile data.')}</Done>
      </ol>
      <p className="rem-note">{t("This server refuses unencrypted connections from the internet, so a wrongly forwarded plain port can't expose your password. Anyone can still reach the sign-in page, so use long passwords, and consider two-factor sign-in.")}</p>
    </div>
  )
}

function TrustForm({ st, save, busy }: { st: Status; save: Save; busy: boolean }) {
  const s = st.settings
  const [url, setUrl] = useState(s.publicUrl)
  const [proxies, setProxies] = useState(s.trustedProxies)
  return (
    <div className="set-form rem-form">
      <label>{t('The outside address')}<input className="input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://media.example.com" autoCapitalize="none" spellCheck={false} /></label>
      <label>{t("The proxy's address, as this server sees it")}<input className="input mono" value={proxies} onChange={(e) => setProxies(e.target.value)} placeholder="127.0.0.1" spellCheck={false} /></label>
      <small>{t('Forwarded addresses are believed from these machines only. Until yours is listed, everyone counts as one visitor from the internet, which is safe but makes sign-in limits and the limits below apply to everybody at once.')}</small>
      <div className="rem-actions"><button type="button" className="btn primary sm" disabled={busy} onClick={() => save({ publicUrl: url, trustedProxies: proxies }, t('Saved'))}>{t('Save')}</button></div>
    </div>
  )
}

const CADDY = (host: string, target: string) => `${host} {\n    reverse_proxy ${target}\n}`

function ProxySteps({ st, save, busy }: { st: Status; save: Save; busy: boolean }) {
  const host = st.settings.publicUrl.replace(/^https:\/\//, '') || 'media.example.com'
  return (
    <div className="rem-steps">
      <ol>
        <li>{t('Point your proxy at this server. For Caddy, this is all it takes (it gets its own certificate):')}
          <pre className="rem-pre">{CADDY(host, `127.0.0.1:${st.homePort}`)}</pre>
          {t('For nginx, also pass {forwardedFor} and {forwardedProto}, and turn off body-size limits.', { forwardedFor: 'X-Forwarded-For', forwardedProto: 'X-Forwarded-Proto' })} <HelpLink to="remote#a-reverse-proxy-you-already-run">{t('The guide')}</HelpLink> {t('has a full example.')}
        </li>
        <Done yes={!!st.settings.trustedProxies && !!st.settings.publicUrl}>{t('Tell this server about it:')}<TrustForm st={st} save={save} busy={busy} /></Done>
        <Done yes={!!st.lastProof}>{t('Run the check below with your phone on mobile data.')}</Done>
      </ol>
    </div>
  )
}

function VpsSteps({ st, save, busy }: { st: Status; save: Save; busy: boolean }) {
  const vpn = st.vpns[0]?.ip
  return (
    <div className="rem-steps">
      <ol>
        <li>{t('Rent a small server, and point your domain name at it.')}</li>
        <li>{vpn ? t('Connect it to this computer with a private network: install Tailscale or NetBird on both (this computer is {address}), or WireGuard if you know it.', { address: vpn }) : t('Connect it to this computer with a private network: install Tailscale or NetBird on both, or WireGuard if you know it.')}</li>
        <li>{t('On the rented server, install Caddy and pass visitors on to this computer over that network:')}
          <pre className="rem-pre">{CADDY(st.settings.publicUrl.replace(/^https:\/\//, '') || 'media.example.com', `${vpn || '100.x.y.z'}:${st.homePort}`)}</pre>
        </li>
        <Done yes={!!st.settings.trustedProxies && !!st.settings.publicUrl}>{t("Tell this server about it. The proxy's address is the rented server's address on your private network:")}<TrustForm st={st} save={save} busy={busy} /></Done>
        <Done yes={!!st.lastProof}>{t('Run the check below with your phone on mobile data.')}</Done>
      </ol>
      <p className="rem-note">{t('Running this server itself on a rented machine? Reach this page through an SSH tunnel, or start it with {flag}: see', { flag: '-remote-access' })} <HelpLink to="remote#the-server-itself-on-a-rented-machine">{t('the guide')}</HelpLink>.</p>
    </div>
  )
}

// ------------------------------------------------------------------ unencrypted from away

// Off unless an administrator knowingly turns it on. Turning it on takes the
// warning accepted and their password; turning it off signs out every device
// that signed in that way.
function PlainAway({ st, save, busy, applied, reload }: { st: Status; save: Save; busy: boolean; applied: (n: Status) => void; reload: () => Promise<void> }) {
  const on = st.settings.allowPlainAway
  const [asking, setAsking] = useState(false)
  const turn = async (v: boolean) => {
    if (v) { setAsking(true); return }
    const n = st.plainSessions
    if (n > 0 && !await ask({ title: t('Refuse unencrypted connections from away again?'), body: tn('One device signed in without encryption, so it is signed out too.', '{count} devices signed in without encryption, so they are signed out too.', n), confirm: t('Refuse them and sign out') })) return
    save({ allowPlainAway: false }, t('Unencrypted connections from away are refused again'))
  }
  const endAll = async () => {
    if (!await ask({ title: t('Sign out every device that signed in without encryption?'), body: t('They’d have to sign in again.'), confirm: t('Sign them out'), danger: true })) return
    try { const r = await api.del<{ ended: number }>('/remote/plain-sessions'); toast(tn('One device signed out', '{count} devices signed out', r.ended)); await reload() } catch (e: any) { toast(e.message) }
  }
  return (
    <Section title={tk('Unencrypted connections from away')} desc={t('This server refuses unencrypted (http://) connections from the internet, so that passwords can’t be read on the way. Some people reach their server through a network or tunnel of their own that the server can’t recognise, and accept the risk. Leave this off unless you’re one of them.')}>
      <Row label={t('Allow unencrypted connections from away')} desc={on ? t('On. Sign-ins made this way are marked, last a day at most, and are signed out when you turn this off.') : t('Off. Connections from the internet must use HTTPS.')}>
        <Switch label={t('Allow unencrypted connections from away')} checked={on} disabled={busy} onChange={turn} />
      </Row>
      {on && st.plainSessions > 0 && (
        <div className="rem-actions">
          <button type="button" className="btn sm" onClick={endAll}>{tn('Sign out the device that signed in unencrypted', 'Sign out the {count} devices that signed in unencrypted', st.plainSessions)}</button>
        </div>
      )}
      {asking && <PlainAwayConfirm onClose={() => setAsking(false)} onDone={(n) => { applied(n); setAsking(false); toast(t('Unencrypted connections from away are allowed')) }} />}
    </Section>
  )
}

function PlainAwayConfirm({ onClose, onDone }: { onClose: () => void; onDone: (n: Status) => void }) {
  const [accept, setAccept] = useState(false)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [err, setErr] = useState('')
  const [working, setWorking] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (working || !accept) return
    setWorking(true); setErr('')
    try { onDone(await api.put<Status>('/remote', { allowPlainAway: true, acceptPlainAway: true, password, code })) } catch (x: any) { setErr(x.message); setCode(''); setWorking(false) }
  }
  return (
    <Dialog alert size="sm" title={t('Allow unencrypted connections from away?')} onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>{t('Keep them refused')}</Button>
        <Button variant="danger" type="submit" form="rem-plain-form" busy={working} disabled={!accept || !password}>{t('Allow them')}</Button>
      </>}>
      <form id="rem-plain-form" className="ui-dialog-form" onSubmit={submit}>
        <div className="ms-ask">
          <p>{t('Without encryption,')} <b>{t('anyone between a device and this server')}</b> {t('can read or change what passes: a café’s Wi-Fi, a hotel’s network, the mobile network, or anyone who has broken into one of them. That includes:')}</p>
          <ul>
            <li><b>{t('Passwords')}</b> {t('typed to sign in, and the sign-ins that keep devices signed in.')}</li>
            <li><b>{t('Stream addresses')}</b>{t(', which play your media without a password.')}</li>
            <li><b>{t('What you watch and listen to')}</b>{t(', and what you search for.')}</li>
            <li><b>{t('The pages themselves')}</b>{t(', which can be changed on the way to trick you.')}</li>
          </ul>
          <p>{t('Use it only on networks you trust, or behind a VPN or tunnel of your own. A private network such as Tailscale or NetBird is safer, and doesn’t need this.')}</p>
        </div>
        <label className="check-row rem-accept"><input type="checkbox" checked={accept} onChange={(e) => setAccept(e.target.checked)} /> <span>{t('I understand that anyone on the way can read my password and what I watch, and I accept that.')}</span></label>
        <OwnerFields password={password} code={code} setPassword={setPassword} setCode={setCode} />
        {err && <p className="error" role="alert">{err}</p>}
      </form>
    </Dialog>
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
    <Section title={tk('Check from outside')} desc={t('The only real check is a device that is really outside. Your own phone does it, on mobile data, so no service on the internet is involved and nothing is sent anywhere.')}>
      {st.addresses.length === 0
        ? <p className="muted">{t('Nothing to check yet: finish the steps above first.')}</p>
        : <>
          {st.addresses.length > 1 && (
            <Row label={t('Address to test')}><select className="select" value={addr} onChange={(e) => setAddress(e.target.value)}>{st.addresses.map((a) => <option key={a} value={a}>{a}</option>)}</select></Row>
          )}
          <div className="rem-actions">
            <button type="button" className="btn primary sm" onClick={start}>{st.test ? t('Start again') : t('Check with my phone')}</button>
            {st.publicUrl && <button type="button" className="btn sm" disabled={checking} onClick={check}>{checking ? t('Checking…') : t('Check from this computer')}</button>}
          </div>
          {st.test && (
            <div className="rem-test">
              <QrCode text={st.test.url} label={t('QR code holding the test address')} size={180} />
              <div>
                <ol>
                  <li>{t('On your phone,')} <b>{t('turn off Wi-Fi')}</b> {addr.includes('//100.') ? t('so it uses mobile data, and keep your VPN on.') : t('so it uses mobile data.')}</li>
                  <li>{t('Scan this code with its camera, or type the address:')}</li>
                </ol>
                <CopyLine value={st.test.url} />
                <div aria-live="polite" role="status">
                  {!hit && <p className="rem-note"><span className="set-pill live">{t('Waiting for your phone')}</span></p>}
                  {hit && hit.proof && <p className="rem-result ok"><IconCheck size={18} /> {t('It works! Your phone reached the server {where}, {how}, from {ip}.', { where: whereLabel(hit.where), how: viaLabel(hit.where, hit.via), ip: hit.ip })}</p>}
                  {hit && !hit.proof && <p className="rem-result warn"><IconInfo size={18} /> {t('That connection came from {where}, so it proves nothing yet. Turn off Wi-Fi and open it again.', { where: whereLabel(hit.where) })}</p>}
                </div>
                <small className="muted">{t('The address is secret, works for 15 minutes, and shows nothing about your server.')}</small>
              </div>
            </div>
          )}
          {!st.test && st.lastProof && <p className="rem-note"><IconCheck size={14} /> {t('Last confirmed {when}, {where}, {how}.', { when: ago(st.lastProof.at), where: whereLabel(st.lastProof.where), how: viaLabel(st.lastProof.where, st.lastProof.via) })}</p>}
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

function Limits({ s, save, busy }: { s: Settings; save: Save; busy: boolean }) {
  const page = useMemo(() => limitsSpec(s.maxHeight, s.maxBitrate), [s.maxHeight, s.maxBitrate])
  return (
    <Section title={tk('Limits for viewers away from home')} desc={t('Home internet connections are often slow when sending. These apply to each stream of anyone away from home, including over a VPN. Playback at home is never affected.')}>
      <SpecPage embedded page={page} onField={(id, v) => { if (!busy) save(id === 'maxHeight' ? { maxHeight: Number(v) } : { maxBitrate: String(v) }, t('Limit saved')) }} />
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
  const away = (list || []).filter((v) => v.where === 'vpn' || v.where === 'internet' || v.plainAway)
  const end = async (v: Session) => {
    if (!await ask({ title: t('Sign {name} out on {device}?', { name: v.userName, device: v.device || t('this device') }), body: t('They’d have to sign in again.'), confirm: t('Sign out'), danger: true })) return
    try { await api.del(`/sessions/${v.id}`); toast(t('Signed out')); load() } catch (e: any) { toast(e.message) }
  }
  const page = useMemo(() => outsideSpec(away, viaLabel, ago), [away]) // eslint-disable-line react-hooks/exhaustive-deps
  const handled = useMemo(() => handledIds(page), [page])
  return (
    <Section title={tk('People connected from outside')} desc={<>{t('Devices that last used this server from outside your home. Your own devices and sign-ins are under')} <Link to="/settings/security">{t('Sign-in & security')}</Link>.</>}>
      {list && <SpecPage embedded page={page} handled={handled} onAction={(id) => { const v = away.find((x) => x.id === id.slice(4)); if (v) void end(v) }} />}
    </Section>
  )
}

// ------------------------------------------------------------------ protection

function Protection({ st, reload }: { st: Status; reload: () => Promise<void> }) {
  const clear = async () => {
    try { await api.del('/remote/lockouts'); toast(t('Everyone may try again')); await reload() } catch (e: any) { toast(e.message) }
  }
  const page = protectionSpec({
    failures: st.failures, lockouts: st.lockouts, weak: st.weakAccounts.filter((w) => !w.unknown).map((w) => w.name), unknown: st.weakAccounts.filter((w) => w.unknown).map((w) => w.name),
    adminsWithout2fa: st.adminsWithout2fa, fmtTime: (at) => date(at * 1000, { timeStyle: 'medium' }),
  })
  return (
    <Section title={tk('Sign-in protection')} desc={t('Always on. Repeated wrong passwords make whoever is guessing wait longer and longer. Nobody on the internet can lock you out at home.')}>
      <SpecPage embedded page={page} handled={handledIds(page)} onAction={(id) => { if (id === 'clear') void clear() }} />
    </Section>
  )
}

// ------------------------------------------------------------------ advanced

function Advanced({ s, save, busy }: { s: Settings; save: Save; busy: boolean }) {
  const [url, setUrl] = useState(s.publicUrl)
  const [home, setHome] = useState(s.homeNets)
  const [away, setAway] = useState(s.awayNets)
  const [proxies, setProxies] = useState(s.trustedProxies)
  const dirty = url !== s.publicUrl || home !== s.homeNets || away !== s.awayNets || proxies !== s.trustedProxies
  const page = advancedSpec({ publicUrl: url, trustedProxies: proxies, homeNets: home, awayNets: away }, dirty, busy)
  return (
    <details className="set-card rem-advanced">
      <summary><h2>{t('Advanced')}</h2></summary>
      <form onSubmit={(e) => { e.preventDefault(); if (!busy && dirty) save({ publicUrl: url, homeNets: home, awayNets: away, trustedProxies: proxies }, t('Saved')) }}>
        <SpecPage embedded page={page} handled={handledIds(page)} onField={(id, v) => ({ publicUrl: setUrl, homeNets: setHome, awayNets: setAway, trustedProxies: setProxies } as Record<string, (x: string) => void>)[id]?.(String(v))}
          onAction={() => save({ publicUrl: url, homeNets: home, awayNets: away, trustedProxies: proxies }, t('Saved'))} />
        <button type="submit" hidden tabIndex={-1} aria-hidden="true" />
      </form>
    </details>
  )
}
