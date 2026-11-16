// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '../../api'
import { IconCheck, IconInfo } from '../../icons'
import { toast } from '../../state'
import QrCode from './QrCode'
import { Row, Section } from './ui'
import SpecPage from '../../spec/SpecPage'
import { handledIds } from '../../spec/model'
import { devicesSpec, optionsSpec } from '../../spec/specs/peerToPeer'
import { ConfirmOwner, type OwnerAsk } from './ConfirmOwner'
import { HelpLink } from '../../components/HelpLink'
import { ask } from '../../ui/Overlay'
import { t, tk, tn } from '../../i18n'

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
          {v.thirdParties.map((p) => <li key={p.layer} className="off"><b>{p.who}</b><span>{t('They see: {what}', { what: p.sees })}</span></li>)}
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
    try { setTest((await api.post<{ steps: Step[] }>('/p2p/test')).steps); load() } catch (e: any) { toast(e.message) } finally { setTesting(false) }
  }
  // An invite signs the device in as you; for an administrator that takes your password
  // again, as approving a TV at home does (the server answers password-required).
  const invited = (r: { link: string; expiresInSec: number }) => setInvite({ link: r.link, until: Date.now() + r.expiresInSec * 1000 })
  const makeInvite = async () => {
    try {
      invited(await api.post<{ link: string; expiresInSec: number }>('/p2p/invite', {}))
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 403 && e.data?.reason === 'password-required') {
        setOwner({
          title: t('Invite a device as you?'),
          body: <p>{t('The invite signs a device in as you, an administrator, wherever it is. Only send it to a device of your own.')}</p>,
          action: t('Make the invite'),
          run: async (password, code) => { invited(await api.post<{ link: string; expiresInSec: number }>('/p2p/invite', { password, otp: code })) },
        })
        return
      }
      toast(e.message)
    }
  }
  // A TV away from home can't take a link in, so it shows a short code instead, and that is typed here.
  // It signs the TV in as the owner, which takes the owner's password again, as approving a TV at home does.
  const codeTyped = joinCode.replace(/[\s-]/g, '')
  const takeCode = () => setOwner({
    title: t('Sign that device in as you?'),
    body: <p>{t('The device showing')} <b>{joinCode.trim().toUpperCase()}</b> {t('is put on the list above and signed in as you, wherever it is. Only type in a code you can see on your own device: whoever is showing it gets in.')}</p>,
    action: t('Add the device'),
    run: async (password, code) => {
      await api.post('/p2p/join', { code: joinCode, password, otp: code })
      setJoined(joinCode.trim().toUpperCase()); setJoinCode('')
    },
  })
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
    body: <p>{t('Use this if the ID has got to someone it shouldn’t have. Every device then has to learn the new ID before it can connect from away again: the apps do that by themselves the next time they are at home, or you can send an invite.')}</p>,
    action: t('Make a new ID'),
    run: async (password, code) => { setV(await api.post<View>('/p2p/rotate', { password, code })); toast(t('This server has a new connection ID')) },
  })

  const devicesPage = devicesSpec(v.devices, ago)
  const devicesHandled = handledIds(devicesPage)
  const optionsPage = optionsSpec({ devicesOnly: s.devicesOnly, layers: s.layers, relayKbps: s.relayKbps, relayForOthers: s.relayForOthers })
  return (
    <Section id="p2p" title={tk('Peer-to-peer connections')} desc={<>{t('On. Apps on paired devices reach this server from away without anything set up on your router.')} <HelpLink to="remote#peer-to-peer-connections">{t('How it works')}</HelpLink></>}
      actions={<button type="button" className="btn sm" disabled={busy} onClick={() => save({ enabled: false }, t('Peer-to-peer connections are off'))}>{t('Turn off')}</button>}>
      {v.error && <p className="rem-note warn"><IconInfo size={14} /> {v.error}</p>}
      <Steps steps={v.steps} />
      <div className="rem-actions p2p-gap">
        <button type="button" className="btn primary sm" disabled={testing || !v.running} onClick={runTest}>{testing ? t('Testing… (up to a minute)') : t('Test my connection')}</button>
      </div>
      {test && <>
        <p className="rem-note"><IconCheck size={14} /> {t('The test connects to this server the way an app away from home would, from a second connection made on this computer.')}</p>
        <Steps steps={test} />
      </>}

      <h4 className="p2p-head">{t('Devices that may connect')}</h4>
      <SpecPage embedded bare page={devicesPage} handled={devicesHandled} onAction={(id) => { if (id === 'invite') void makeInvite(); else { const d = v.devices.find((x) => x.id === id.slice(7)); if (d) void remove(d) } }} />
      {invite && (
        <div className="rem-test">
          <QrCode text={invite.link} label={t('QR code holding the invite')} size={200} />
          <div>
            <p>{t('In the app on the other device, choose')} <b>{t('Connect with an invite')}</b> {t('and scan this, or send the link below. It signs the device in as you.')}</p>
            <span className="rem-copy"><code>{invite.link}</code><button type="button" className="btn sm ghost" onClick={() => copy(invite.link)} aria-label={t('Copy the invite link')}>{t('Copy')}</button></span>
            <small className="muted">{t('Works once, for fifteen minutes, and only while you stay signed in here. Treat it like a password until then: whoever has it gets in as you.')}</small>
            <div><button type="button" className="btn sm ghost" onClick={() => setInvite(null)}>{t('Hide')}</button></div>
          </div>
        </div>
      )}

      {s.layers.dht && (
        <Row label={t('A TV that shows a code')} desc={t('A TV that isn’t at home has no way to take an invite in. In its app, choose Join from away: it shows a code of eight letters and digits. Type it here.')} stack>
          <form className="rem-actions" onSubmit={(e) => { e.preventDefault(); if (codeTyped.length === 8) takeCode() }}>
            <input className="input p2p-code" value={joinCode} onChange={(e) => { setJoinCode(e.target.value); setJoined('') }} placeholder="K7QM-2XHD" maxLength={12} autoCapitalize="characters" autoComplete="off" spellCheck={false} aria-label={t('The code the device shows')} />
            <button type="submit" className="btn sm" disabled={codeTyped.length !== 8}>{t('Add the device…')}</button>
          </form>
          {joined && <small className="muted" role="status">{t('This server is now looking out for the device showing {code}. It joins within a minute or so and appears in the list above; the code works once, for fifteen minutes.', { code: joined })}</small>}
        </Row>
      )}

      <SpecPage embedded bare page={optionsPage} onField={(id, val) => {
        if (busy) return
        if (id === 'devicesOnly') void anyDevice(!!val)
        else if (id.startsWith('layer:')) { const l = id.slice(6) as keyof Layers; void save({ layers: { ...s.layers, [l]: !!val } }) }
        else if (id === 'relayKbps') void save({ relayKbps: Number(val) })
        else if (id === 'relayForOthers') void save({ relayForOthers: !!val }, val ? t('This server offers itself as a relay') : undefined)
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
