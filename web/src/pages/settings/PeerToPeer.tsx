// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import { IconCheck, IconInfo, IconTrash } from '../../icons'
import { toast } from '../../state'
import QrCode from './QrCode'
import { Row, Section, Switch } from './ui'
import { ConfirmOwner, type OwnerAsk } from './ConfirmOwner'
import { HelpLink } from '../../components/HelpLink'
import { ask } from '../../ui/Overlay'

// Settings → Remote Access → Peer-to-peer connections: reaching the server
// from away with nothing set up on the router. Off until the owner turns it
// on, and before that the page says who will be contacted and what they see.
// Once on, every way of connecting is shown in plain words, one line each.

type Layers = { dht: boolean; relay: boolean; holePunch: boolean; portMap: boolean; ipv6: boolean }
type P2PSettings = { enabled: boolean; port: number; layers: Layers; devicesOnly: boolean; relayKbps: number; relayForOthers: boolean; staticRelays: string[] }
type Step = { name: string; state: 'ok' | 'warn' | 'fail' | 'off' | 'wait'; detail: string }
type Device = { id: string; user?: string; name: string; platform: string; how: string; created: number; lastSeen: number; connected: boolean; path?: string }
type View = {
  settings: P2PSettings; running: boolean; serverId?: string; error?: string; steps: Step[]
  thirdParties: { layer: keyof Layers; who: string; sees: string }[]; devices: Device[]
}

const LAYERS: { id: keyof Layers; label: string; desc: string }[] = [
  { id: 'portMap', label: 'Ask my router for a port', desc: 'Asks the router at home to forward one port to this computer (UPnP, NAT-PMP or PCP), renews it, and removes it when this is turned off. Nothing leaves your home for it.' },
  { id: 'ipv6', label: 'Ask my router to let IPv6 in', desc: 'Where your connection has IPv6, asks the router to let connections reach this computer on that one port.' },
  { id: 'dht', label: 'Public address book', desc: 'Announces where this server is on the public IPFS network, so apps find it by its ID. Off, apps can only use addresses they already know, which stops working when your address changes.' },
  { id: 'relay', label: 'Relays', desc: 'Lets a relay introduce an app to this server, and carry the connection when nothing else works. A relay sees addresses and amounts, never what is sent.' },
  { id: 'holePunch', label: 'Hole punching', desc: 'Has the app and this server connect to each other at the same moment, so both routers let the connection through. Needs relays.' },
]

const ago = (t?: number) => {
  if (!t) return 'never'
  const s = Math.max(0, Math.floor(Date.now() / 1000) - t)
  if (s < 90) return 'just now'
  if (s < 5400) return `${Math.round(s / 60)} minutes ago`
  if (s < 129600) return `${Math.round(s / 3600)} hours ago`
  return `${Math.round(s / 86400)} days ago`
}

const pathLabel: Record<string, string> = { direct: 'Connected directly', relayed: 'Connected through a relay' }

function Steps({ steps }: { steps: Step[] }) {
  return (
    <ul className="rem-check" aria-live="polite">
      {steps.map((c) => <li key={c.name} className={c.state}><b>{c.name}</b><span>{c.detail}</span></li>)}
    </ul>
  )
}

export default function PeerToPeer() {
  const [v, setV] = useState<View | null>(null)
  const [busy, setBusy] = useState(false)
  const [accept, setAccept] = useState(false)
  const [test, setTest] = useState<Step[] | null>(null)
  const [testing, setTesting] = useState(false)
  const [invite, setInvite] = useState<{ link: string; until: number } | null>(null)
  const [joinCode, setJoinCode] = useState('')
  const [joined, setJoined] = useState('')
  const [owner, setOwner] = useState<OwnerAsk | null>(null)
  const [relays, setRelays] = useState<string | null>(null)
  const load = useCallback(() => api.get<View>('/p2p/settings').then(setV).catch(() => {}), [])
  useEffect(() => { load() }, [load])
  const running = !!v?.running
  useEffect(() => {
    if (!running) return
    const t = setInterval(() => { if (!document.hidden) load() }, 10000)
    return () => clearInterval(t)
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
      <Section id="p2p" title="Peer-to-peer connections" desc={<>Reach this server from away with nothing set up on your router and nothing rented: the app and this server find a way to each other, and what passes between them is encrypted to a key only this server holds. Only devices you have paired can connect. <HelpLink to="remote#peer-to-peer-connections">How it works, and how safe it is</HelpLink></>}>
        <p className="muted">It’s off. Turning it on contacts computers outside your home, which nothing else on this page does without being asked. These are all of them:</p>
        <ul className="rem-check">
          {v.thirdParties.map((t) => <li key={t.layer} className="off"><b>{t.who}</b><span>They see: {t.sees}</span></li>)}
        </ul>
        <p className="rem-note">None of them is run by this project, and none sees your media, your passwords or anything you watch. Each can be turned off separately once this is on.</p>
        <label className="check-row rem-accept"><input type="checkbox" checked={accept} onChange={(e) => setAccept(e.target.checked)} /> <span>I’ve read who is contacted, and I want this on.</span></label>
        <div className="rem-actions">
          <button type="button" className="btn primary sm" disabled={busy || !accept} onClick={() => save({ enabled: true, accept: true }, 'Peer-to-peer connections are on')}>Turn on peer-to-peer connections</button>
        </div>
      </Section>
    )
  }

  const runTest = async () => {
    setTesting(true); setTest(null)
    try { setTest((await api.post<{ steps: Step[] }>('/p2p/test')).steps); load() } catch (e: any) { toast(e.message) } finally { setTesting(false) }
  }
  const makeInvite = async () => {
    try {
      const r = await api.post<{ link: string; expiresInSec: number }>('/p2p/invite', {})
      setInvite({ link: r.link, until: Date.now() + r.expiresInSec * 1000 })
    } catch (e: any) { toast(e.message) }
  }
  // A TV away from home can't take a link in, so it shows a short code instead, and that is typed here.
  // It signs the TV in as the owner, which takes the owner's password again, as approving a TV at home does.
  const codeTyped = joinCode.replace(/[\s-]/g, '')
  const takeCode = () => setOwner({
    title: 'Sign that device in as you?',
    body: <p>The device showing <b>{joinCode.trim().toUpperCase()}</b> is put on the list above and signed in as you, wherever it is. Only type in a code you can see on your own device: whoever is showing it gets in.</p>,
    action: 'Add the device',
    run: async (password, code) => {
      await api.post('/p2p/join', { code: joinCode, password, otp: code })
      setJoined(joinCode.trim().toUpperCase()); setJoinCode('')
    },
  })
  const remove = async (d: Device) => {
    if (!await ask({ title: `Remove ${d.name}?`, body: 'It can’t connect from away any more, and is cut off now if it is connected. It stays signed in, so it still works at home; sign it out under Sign-in & security to stop that too.', confirm: 'Remove', danger: true })) return
    try { await api.del(`/p2p/devices/${d.id}`); toast('Removed'); load() } catch (e: any) { toast(e.message) }
  }
  const anyDevice = async (on: boolean) => {
    if (on) { await save({ devicesOnly: true }, 'Only paired devices can connect'); return }
    if (!await ask({ title: 'Let any device reach the sign-in page?', body: 'Anyone who gets hold of this server’s connection ID could then reach its sign-in page from anywhere, and try passwords. Paired devices only is safer, and is all the apps need.', confirm: 'Let any device try', danger: true })) return
    await save({ devicesOnly: false, accept: true }, 'Any device with the ID can reach the sign-in page')
  }
  const copy = async (text: string) => {
    try { await navigator.clipboard.writeText(text); toast('Copied') } catch { toast('Select it and copy it by hand') }
  }
  const rotate = () => setOwner({
    title: 'Give this server a new connection ID?',
    body: <p>Use this if the ID has got to someone it shouldn’t have. Every device then has to learn the new ID before it can connect from away again: the apps do that by themselves the next time they are at home, or you can send an invite.</p>,
    action: 'Make a new ID',
    run: async (password, code) => { setV(await api.post<View>('/p2p/rotate', { password, code })); toast('This server has a new connection ID') },
  })

  return (
    <Section id="p2p" title="Peer-to-peer connections" desc={<>On. Apps on paired devices reach this server from away without anything set up on your router. <HelpLink to="remote#peer-to-peer-connections">How it works</HelpLink></>}
      actions={<button type="button" className="btn sm" disabled={busy} onClick={() => save({ enabled: false }, 'Peer-to-peer connections are off')}>Turn off</button>}>
      {v.error && <p className="rem-note warn"><IconInfo size={14} /> {v.error}</p>}
      <Steps steps={v.steps} />
      <div className="rem-actions p2p-gap">
        <button type="button" className="btn primary sm" disabled={testing || !v.running} onClick={runTest}>{testing ? 'Testing… (up to a minute)' : 'Test my connection'}</button>
      </div>
      {test && <>
        <p className="rem-note"><IconCheck size={14} /> The test connects to this server the way an app away from home would, from a second connection made on this computer.</p>
        <Steps steps={test} />
      </>}

      <h4 className="p2p-head">Devices that may connect</h4>
      {v.devices.length === 0 && <p className="muted">None yet. Open the app on a phone, TV or computer while it’s at home and signed in: it adds itself. Or invite a device that isn’t at home.</p>}
      {v.devices.length > 0 && (
        <ul className="rem-sessions">
          {v.devices.map((d) => (
            <li key={d.id}>
              <div>
                <b>{d.name}</b>{d.user ? ` · ${d.user}` : ''}
                {d.connected && <span className="set-pill live">{pathLabel[d.path || 'direct']}</span>}
                <small>{d.platform || 'app'} · added {ago(d.created)} {d.how === 'invite' ? 'by invite' : 'while signed in'} · last connected {ago(d.lastSeen)} · ID ending {d.id.slice(-8)}</small>
              </div>
              <button type="button" className="btn sm ghost danger-hover" onClick={() => remove(d)} aria-label={`Remove ${d.name}`}><IconTrash /> Remove</button>
            </li>
          ))}
        </ul>
      )}
      <div className="rem-actions p2p-gap">
        <button type="button" className="btn sm" onClick={makeInvite}>Invite a device that isn’t at home</button>
      </div>
      {invite && (
        <div className="rem-test">
          <QrCode text={invite.link} label="QR code holding the invite" size={200} />
          <div>
            <p>In the app on the other device, choose <b>Connect with an invite</b> and scan this, or send the link below. It signs the device in as you.</p>
            <span className="rem-copy"><code>{invite.link}</code><button type="button" className="btn sm ghost" onClick={() => copy(invite.link)} aria-label="Copy the invite link">Copy</button></span>
            <small className="muted">Works once, for fifteen minutes, and only while you stay signed in here. Treat it like a password until then: whoever has it gets in as you.</small>
            <div><button type="button" className="btn sm ghost" onClick={() => setInvite(null)}>Hide</button></div>
          </div>
        </div>
      )}

      {s.layers.dht && (
        <Row label="A TV that shows a code" desc="A TV that isn’t at home has no way to take an invite in. In its app, choose Join from away: it shows a code of eight letters and digits. Type it here." stack>
          <form className="rem-actions" onSubmit={(e) => { e.preventDefault(); if (codeTyped.length === 8) takeCode() }}>
            <input className="input p2p-code" value={joinCode} onChange={(e) => { setJoinCode(e.target.value); setJoined('') }} placeholder="K7QM-2XHD" maxLength={12} autoCapitalize="characters" autoComplete="off" spellCheck={false} aria-label="The code the device shows" />
            <button type="submit" className="btn sm" disabled={codeTyped.length !== 8}>Add the device…</button>
          </form>
          {joined && <small className="muted" role="status">This server is now looking out for the device showing {joined}. It joins within a minute or so and appears in the list above; the code works once, for fifteen minutes.</small>}
        </Row>
      )}

      <h4 className="p2p-head">Who may try</h4>
      <Row label="Only paired devices" desc="A device that isn’t on the list above gets no answer at all, not even the sign-in page. Leave this on unless you have a reason.">
        <Switch label="Only paired devices" checked={s.devicesOnly} disabled={busy} onChange={anyDevice} />
      </Row>

      <h4 className="p2p-head">Ways of connecting</h4>
      <p className="muted">Each is tried by itself, best first. Turn off the ones you’d rather not use.</p>
      {LAYERS.map((l) => (
        <Row key={l.id} label={l.label} desc={l.desc} dim={l.id === 'holePunch' && !s.layers.relay}>
          <Switch label={l.label} checked={s.layers[l.id]} disabled={busy || (l.id === 'holePunch' && !s.layers.relay)} onChange={(on) => save({ layers: { ...s.layers, [l.id]: on } })} />
        </Row>
      ))}
      <Row label="Limit through a relay" desc="A relay is someone else’s internet connection. This is the most this server sends through one; the quality limits further down this page apply as well.">
        <select className="select" value={s.relayKbps} disabled={busy} onChange={(e) => save({ relayKbps: Number(e.target.value) })} aria-label="Limit through a relay">
          {[1000, 2000, 4000, 8000, 20000].map((k) => <option key={k} value={k}>{k / 1000} Mbit/s</option>)}
          <option value={0}>No limit</option>
        </select>
      </Row>
      <Row label="Help other servers" desc="While this server can be reached from outside, lets up to 16 other WittyMediaServer owners’ connections pass through it when theirs can’t connect directly. It uses your internet connection (at most 2 GiB per connection), and you see none of what passes: it is encrypted between their devices and their servers.">
        <Switch label="Help other servers" checked={s.relayForOthers} disabled={busy} onChange={(on) => save({ relayForOthers: on }, on ? 'This server offers itself as a relay' : undefined)} />
      </Row>

      <details className="p2p-more">
        <summary>More</summary>
        <Row label="This server’s connection ID" desc="What apps dial. It isn’t a password, but only paired devices need it." stack>
          <span className="rem-copy"><code>{v.serverId}</code><button type="button" className="btn sm ghost" onClick={() => copy(v.serverId || '')} aria-label="Copy the connection ID">Copy</button></span>
          <div className="rem-actions"><button type="button" className="btn sm" onClick={rotate}>Make a new ID…</button></div>
        </Row>
        <Row label="Port" desc="Used for UDP and TCP. Chosen when this was first turned on and kept, so the router’s mapping and the addresses apps remember stay right.">
          <code>{s.port}</code>
        </Row>
        <Row label="Relays of your own" desc="A friend’s server that has “Help other servers” on, or one of your own elsewhere: one address per line, ending in /p2p/ and its ID. They’re used before any other relay." stack>
          <textarea className="input" rows={3} spellCheck={false} value={relays ?? s.staticRelays.join('\n')} onChange={(e) => setRelays(e.target.value)} aria-label="Relays of your own" />
          <div className="rem-actions"><button type="button" className="btn sm" disabled={busy || relays === null} onClick={async () => { if (await save({ staticRelays: (relays || '').split('\n').map((x) => x.trim()).filter(Boolean) }, 'Saved')) setRelays(null) }}>Save</button></div>
        </Row>
      </details>
      {owner && <ConfirmOwner ask={owner} onClose={() => setOwner(null)} />}
    </Section>
  )
}
