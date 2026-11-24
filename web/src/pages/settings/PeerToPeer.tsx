// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import { IconCheck, IconInfo } from '../../icons'
import { toast } from '../../state'
import { Row, Section } from './ui'
import SpecPage from '../../spec/SpecPage'
import { devicesSpec, optionsSpec } from '../../spec/specs/peerToPeer'
import { ConfirmOwner, type OwnerAsk } from './ConfirmOwner'
import { HelpLink } from '../../components/HelpLink'
import { ask } from '../../ui/Overlay'
import { t, tk, tn, tx } from '../../i18n'

// Settings → Remote Access → Peer-to-peer connections: reaching the server
// from away with nothing set up on the router. Off until the owner turns it
// on, and before that the page says who will be contacted and what they see.
// Once on, every way of connecting is shown in plain words, one line each.

type Layers = { dht: boolean; relay: boolean; holePunch: boolean; portMap: boolean; ipv6: boolean }
type OwnRelay = { addr: string; key?: string; hasKey?: boolean }
type P2PSettings = {
  enabled: boolean; port: number; layers: Layers; devicesOnly: boolean; relayKbps: number; relayForOthers: boolean; staticRelays: string[]
  shareKbps: number; shareMaxHeight: number; ownRelays: OwnRelay[]
}
type Step = { name: string; state: 'ok' | 'warn' | 'fail' | 'off' | 'wait'; detail: string }
type Device = { id: string; user?: string; name: string; platform: string; how: string; created: number; lastSeen: number; connected: boolean; path?: string }
type Verdict = { state: 'ok' | 'warn' | 'fail' | 'wait'; title: string; detail: string; todo: string[] }
type RelayHealth = { state: 'ok' | 'busy' | 'full' | 'starting'; load: number }
type Offer = { name?: string; note?: string; software?: string; access: 'open' | 'key'; kbps?: number; maxHeight?: number; health?: RelayHealth }
type OwnRelayStatus = { addr: string; id: string; state: 'ok' | 'refused' | 'unreachable' | 'wait'; error?: string; offer?: Offer; allows?: string }
type RelayStatus = { id: string; community: boolean; allows?: string; offer?: Offer }
type View = {
  settings: P2PSettings; running: boolean; serverId?: string; error?: string; steps: Step[]; verdict?: Verdict
  status?: { ownRelays: OwnRelayStatus[]; relays: RelayStatus[] }
  thirdParties: { layer: keyof Layers; who: string; sees: string }[]; devices: Device[]
}

// The answer to "will it work from away?", in a line, and what to do about it. The words are
// the server's, as the lines under it are.
function VerdictCard({ v }: { v: Verdict }) {
  return (
    <div className={`p2p-verdict ${v.state}`} role="status" aria-live="polite">
      <b>{v.title}</b>
      <p>{tx(v.detail)}</p>
      {v.todo.length > 0 && <ul>{v.todo.map((x) => <li key={x}>{x}</li>)}</ul>}
    </div>
  )
}

const RELAY_STATE: Record<OwnRelayStatus['state'], string> = { ok: tk('Working'), refused: tk('Refused'), unreachable: tk('Can’t be reached'), wait: tk('Connecting…') }

// What a WittyRelay says about how full it is (WittyRelay 0.2 and later), renewed with every registration.
const RELAY_HEALTH: Record<RelayHealth['state'], string> = {
  ok: tk('Relay health: good ({load}% in use)'),
  busy: tk('Relay health: busy ({load}% in use). Streams through it may be slower.'),
  full: tk('Relay health: full. It takes no new servers; this one keeps its place.'),
  starting: tk('Relay health: starting'),
}

function HealthLine({ h }: { h: RelayHealth }) {
  return <small className="muted p2p-allows p2p-health" data-health={h.state}>{t(RELAY_HEALTH[h.state], { load: String(h.load) })}</small>
}

// Your own WittyRelay: an address and a key, checked first so the owner reads
// what the relay allows before this server uses it.
function OwnRelays({ s, status, busy, save }: { s: P2PSettings; status: OwnRelayStatus[]; busy: boolean; save: (c: Partial<P2PSettings>, done?: string) => Promise<boolean> }) {
  const [addr, setAddr] = useState('')
  const [key, setKey] = useState('')
  const [checked, setChecked] = useState<{ addr: string; ok: boolean; allows?: string; offer?: Offer; error?: string } | null>(null)
  const [checking, setChecking] = useState(false)
  const check = async () => {
    setChecking(true); setChecked(null)
    try {
      const r = await api.post<{ ok: boolean; allows?: string; offer?: Offer; error?: string }>('/p2p/relay-check', { addr })
      setChecked({ addr, ...r })
    } catch (e: any) { setChecked({ addr, ok: false, error: e.message }) } finally { setChecking(false) }
  }
  const use = async () => {
    if (await save({ ownRelays: [...s.ownRelays, { addr: addr.trim(), key: key.trim() }] }, t('This server now uses your WittyRelay'))) {
      setAddr(''); setKey(''); setChecked(null)
    }
  }
  const drop = async (r: OwnRelay) => {
    if (!await ask({ title: t('Stop using this WittyRelay?'), body: t('Devices that can’t connect directly then have no relay to carry their connection, unless you add another.'), confirm: t('Stop using it') })) return
    await save({ ownRelays: s.ownRelays.filter((x) => x.addr !== r.addr) }, t('Removed'))
  }
  const ready = checked?.ok && checked.addr === addr
  return (
    <Row label={t('Your own WittyRelay')} desc={<>{t('For networks that block direct connections. A WittyRelay is a small program you (or someone you trust) run in Docker on a machine both ends can reach. It carries the connection without being able to read it.')} <HelpLink to="remote#your-own-wittyrelay">{t('How to run one')}</HelpLink></>} stack>
      <div className="p2p-relay">
        {s.ownRelays.map((r) => {
          const st = status.find((x) => x.addr === r.addr)
          return (
            <div key={r.addr} className="rem-copy">
              <span>
                <code>{r.addr}</code>
                <small className="muted p2p-allows"> {st ? t(RELAY_STATE[st.state]) : t('Starting…')}{st?.error ? `: ${st.error}` : ''}</small>
                {st?.state === 'ok' && st.offer?.health && <HealthLine h={st.offer.health} />}
                {st?.allows && <small className="muted p2p-allows"> {t('It allows: {limits}', { limits: st.allows })}</small>}
              </span>
              <button type="button" className="btn sm ghost" disabled={busy} onClick={() => void drop(r)}>{t('Remove')}</button>
            </div>
          )
        })}
        {s.ownRelays.length < 4 && <>
          <input className="input" value={addr} onChange={(e) => { setAddr(e.target.value); setChecked(null) }} placeholder="/dns4/relay.example.com/udp/4001/quic-v1/p2p/12D3KooW…" spellCheck={false} autoComplete="off" aria-label={t('The WittyRelay’s address')} />
          <input className="input" type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder={t('Access key (if the relay needs one)')} autoComplete="off" aria-label={t('The WittyRelay’s access key')} />
          <div className="rem-actions">
            <button type="button" className="btn sm" disabled={checking || !addr.trim()} onClick={() => void check()}>{checking ? t('Checking…') : t('Check')}</button>
            <button type="button" className="btn primary sm" disabled={busy || !ready} onClick={() => void use()}>{t('Use this relay')}</button>
          </div>
          {checked && (checked.ok
            ? <p className="rem-note" role="status"><IconCheck size={14} /> <b>{checked.offer?.name || t('A WittyRelay')}</b>{checked.offer?.note ? ` (${checked.offer.note})` : ''} {t('answered.')} {t('It allows: {limits}', { limits: checked.allows || '' })} {checked.offer?.access === 'key' ? t('It needs an access key.') : t('Any server may use it.')} {t('Streams through it are made to fit these limits.')}</p>
            : <p className="rem-note warn" role="status"><IconInfo size={14} /> {checked.error}{checked.allows ? ' ' + t('It allows: {limits}', { limits: checked.allows }) : ''}</p>)}
        </>}
        <small className="muted">{t('A hosted WittyRelay that needs no setup is planned, as a free service. It doesn’t exist yet.')}</small>
      </div>
    </Row>
  )
}

const ago = (at?: number) => {
  if (!at) return t('never')
  const s = Math.max(0, Math.floor(Date.now() / 1000) - at)
  if (s < 90) return t('just now')
  if (s < 5400) return tn('{count} minute ago', '{count} minutes ago', Math.round(s / 60))
  if (s < 129600) return tn('{count} hour ago', '{count} hours ago', Math.round(s / 3600))
  return tn('{count} day ago', '{count} days ago', Math.round(s / 86400))
}

function Steps({ steps }: { steps: Step[] }) {
  return (
    <ul className="rem-check" aria-live="polite">
      {steps.map((c) => <li key={c.name} className={c.state}><b>{tx(c.name)}</b><span>{tx(c.detail)}</span></li>)}
    </ul>
  )
}

export default function PeerToPeer() {
  const [v, setV] = useState<View | null>(null)
  const [busy, setBusy] = useState(false)
  const [accept, setAccept] = useState(false)
  const [test, setTest] = useState<{ steps: Step[]; verdict?: Verdict } | null>(null)
  const [testing, setTesting] = useState(false)
  const [owner, setOwner] = useState<OwnerAsk | null>(null)
  const [relays, setRelays] = useState<string | null>(null)
  const load = useCallback(() => api.get<View>('/p2p/settings').then(setV).catch(() => {}), [])
  useEffect(() => { load() }, [load])
  const running = !!v?.running
  useEffect(() => {
    if (!running) return
    const timer = setInterval(() => { if (!document.hidden) load() }, 10000)
    return () => clearInterval(timer)
  }, [load, running])

  const save = async (change: Partial<P2PSettings> & { accept?: boolean }, done?: string) => {
    setBusy(true)
    try {
      const n = await api.put<View>('/p2p/settings', change)
      setV(n)
      if (n.error) toast(n.error)
      else if (done) toast(done)
      return true
    } catch (e: any) { toast(e.message); return false } finally { setBusy(false) }
  }
  if (!v) return null
  const s = v.settings

  if (!s.enabled) {
    return (
      <Section id="p2p" title={tk('Peer-to-peer connections')} desc={<>{t('Reach this server from away with nothing set up on your router and nothing rented: the app and this server find a way to each other, and what passes between them is encrypted to a key only this server holds. Only devices you have paired can connect.')} <HelpLink to="remote#peer-to-peer-connections">{t('How it works, and how safe it is')}</HelpLink></>}>
        <p className="muted">{t('It’s off. Turning it on contacts computers outside your home, which nothing else on this page does without being asked. These are all of them:')}</p>
        <ul className="rem-check">
          {v.thirdParties.map((p) => <li key={p.layer} className="off"><b>{tx(p.who)}</b><span>{t('They see: {what}', { what: tx(p.sees) })}</span></li>)}
        </ul>
        <p className="rem-note">{t('None of them is run by this project, and none sees your media, your passwords or anything you watch. Each can be turned off separately once this is on.')}</p>
        <label className="check-row rem-accept"><input type="checkbox" checked={accept} onChange={(e) => setAccept(e.target.checked)} /> <span>{t('I’ve read who is contacted, and I want this on.')}</span></label>
        <div className="rem-actions">
          <button type="button" className="btn primary sm" disabled={busy || !accept} onClick={() => save({ enabled: true, accept: true }, t('Peer-to-peer connections are on'))}>{t('Turn on peer-to-peer connections')}</button>
        </div>
      </Section>
    )
  }

  const runTest = async () => {
    setTesting(true); setTest(null)
    try { setTest(await api.post<{ steps: Step[]; verdict?: Verdict }>('/p2p/test')); load() } catch (e: any) { toast(e.message) } finally { setTesting(false) }
  }
  const remove = async (d: Device) => {
    if (!await ask({ title: t('Remove {name}?', { name: d.name }), body: t('It can’t connect from away any more, and is cut off now if it is connected. It stays signed in, so it still works at home; sign it out under Sign-in & security to stop that too.'), confirm: t('Remove'), danger: true })) return
    try { await api.del(`/p2p/devices/${d.id}`); toast(t('Removed')); load() } catch (e: any) { toast(e.message) }
  }
  const anyDevice = async (on: boolean) => {
    if (on) { await save({ devicesOnly: true }, t('Only paired devices can connect')); return }
    if (!await ask({ title: t('Let any device reach the sign-in page?'), body: t('Anyone who gets hold of this server’s connection ID could then reach its sign-in page from anywhere, and try passwords. Paired devices only is safer, and is all the apps need.'), confirm: t('Let any device try'), danger: true })) return
    await save({ devicesOnly: false, accept: true }, t('Any device with the ID can reach the sign-in page'))
  }
  const copy = async (text: string) => {
    try { await navigator.clipboard.writeText(text); toast(t('Copied')) } catch { toast(t('Select it and copy it by hand')) }
  }
  const rotate = () => setOwner({
    title: t('Give this server a new connection ID?'),
    body: <p>{t('Use this if the ID has got to someone it shouldn’t have. Every device then has to learn the new ID before it can connect from away again: the apps do that by themselves the next time they are at home, or you can connect them again with Connect a device.')}</p>,
    action: t('Make a new ID'),
    run: async (password, code) => { setV(await api.post<View>('/p2p/rotate', { password, code })); toast(t('This server has a new connection ID')) },
  })

  // A device's row isn't a button: only its own Remove button, inside the row, acts.
  const devicesPage = devicesSpec(v.devices, ago)
  const removeDevice = (id: string) => {
    if (!id.startsWith('remove:')) return
    const d = v.devices.find((x) => x.id === id.slice('remove:'.length))
    if (d) void remove(d)
  }
  const optionsPage = optionsSpec({ devicesOnly: s.devicesOnly, layers: s.layers, relayKbps: s.relayKbps, relayForOthers: s.relayForOthers, shareKbps: s.shareKbps, shareMaxHeight: s.shareMaxHeight })
  return (
    <Section id="p2p" title={tk('Peer-to-peer connections')} desc={<>{t('On. Apps on paired devices reach this server from away without anything set up on your router.')} <HelpLink to="remote#peer-to-peer-connections">{t('How it works')}</HelpLink></>}
      actions={<button type="button" className="btn sm" disabled={busy} onClick={() => save({ enabled: false }, t('Peer-to-peer connections are off'))}>{t('Turn off')}</button>}>
      {v.error && <p className="rem-note warn"><IconInfo size={14} /> {v.error}</p>}
      {(test?.verdict || v.verdict) && <VerdictCard v={(test?.verdict || v.verdict)!} />}
      <div className="rem-actions p2p-gap">
        <button type="button" className="btn primary sm" disabled={testing || !v.running} onClick={runTest}>{testing ? t('Testing… (up to a minute)') : t('Test my connection')}</button>
      </div>
      {test && <>
        <p className="rem-note"><IconCheck size={14} /> {t('The test connects to this server the way an app away from home would, from a second connection made on this computer.')}</p>
        <Steps steps={test.steps} />
      </>}
      <details className="p2p-more">
        <summary>{t('Each way of connecting')}</summary>
        <Steps steps={v.steps} />
      </details>
      {s.layers.relay && <OwnRelays s={s} status={v.status?.ownRelays || []} busy={busy} save={save} />}

      <h4 className="p2p-head">{t('Devices that may connect')}</h4>
      <SpecPage embedded bare page={devicesPage} onAction={removeDevice} />

      <SpecPage embedded bare page={optionsPage} onField={(id, val) => {
        if (busy) return
        if (id === 'devicesOnly') void anyDevice(!!val)
        else if (id.startsWith('layer:')) { const l = id.slice(6) as keyof Layers; void save({ layers: { ...s.layers, [l]: !!val } }) }
        else if (id === 'relayKbps') void save({ relayKbps: Number(val) })
        else if (id === 'relayForOthers') void save({ relayForOthers: !!val }, val ? t('This server offers itself as a relay') : undefined)
        else if (id === 'shareKbps') void save({ shareKbps: Number(val) })
        else if (id === 'shareMaxHeight') void save({ shareMaxHeight: Number(val) })
      }} />

      <details className="p2p-more">
        <summary>{t('More')}</summary>
        <Row label={t('This server’s connection ID')} desc={t('What apps dial. It isn’t a password, but only paired devices need it.')} stack>
          <span className="rem-copy"><code>{v.serverId}</code><button type="button" className="btn sm ghost" onClick={() => copy(v.serverId || '')} aria-label={t('Copy the connection ID')}>{t('Copy')}</button></span>
          <div className="rem-actions"><button type="button" className="btn sm" onClick={rotate}>{t('Make a new ID…')}</button></div>
        </Row>
        <Row label={t('Port')} desc={t('Used for UDP and TCP. Chosen when this was first turned on and kept, so the router’s mapping and the addresses apps remember stay right.')}>
          <code>{s.port}</code>
        </Row>
        <Row label={t('Relays of your own')} desc={t('A friend’s server that has “Help other servers” on, or one of your own elsewhere: one address per line, ending in /p2p/ and its ID. They’re used before any other relay.')} stack>
          <textarea className="input" rows={3} spellCheck={false} value={relays ?? s.staticRelays.join('\n')} onChange={(e) => setRelays(e.target.value)} aria-label={t('Relays of your own')} />
          <div className="rem-actions"><button type="button" className="btn sm" disabled={busy || relays === null} onClick={async () => { if (await save({ staticRelays: (relays || '').split('\n').map((x) => x.trim()).filter(Boolean) }, t('Saved'))) setRelays(null) }}>{t('Save')}</button></div>
        </Row>
      </details>
      {owner && <ConfirmOwner ask={owner} onClose={() => setOwner(null)} />}
    </Section>
  )
}
