import { useEffect, useState } from 'react'
import { api } from '../../api'
import { Spinner } from '../../components/common'
import { toast, useApp } from '../../state'
import type { RigView } from '../Rigs'
import { AddRig, RigCard } from './GameRigs'
import { PageHead, Row, Section, Switch } from './ui'
import '../rigs.css'

// Settings, Game PCs: who may connect to PCs running Sunshine from this server, and where. The
// defaults, and the reasons for them, are in docs/GAME-RIGS.md.

type Credit = { name: string; role: string; license: string; source: string }
type Status = { allowed: boolean; ready: boolean; program?: boolean; access?: string; credits: Credit[] }

export function Credits({ list }: { list: Credit[] }) {
  return (
    <ul className="rig-apps">
      {list.map((c) => (
        <li key={c.name}><b>{c.name}</b>: {c.role}. License: {c.license}. <a href={c.source} target="_blank" rel="noreferrer noopener">Source</a></li>
      ))}
    </ul>
  )
}

export default function CouchSettings() {
  const [st, setSt] = useState<Status | null>(null)
  const [settings, setSettings] = useState<Record<string, string> | null>(null)
  const [rigs, setRigs] = useState<RigView[] | null>(null)
  const { refreshLibraries } = useApp()
  const loadRigs = () => api.get<RigView[]>('/rigs').then(setRigs).catch((e) => toast(e.message))
  useEffect(() => {
    api.get<Status>('/couch').then(setSt).catch((e) => toast(e.message))
    api.get<Record<string, string>>('/settings').then(setSettings).catch(() => {})
    loadRigs()
  }, [])
  const put = async (patch: Record<string, string>) => {
    try { setSettings(await api.put<Record<string, string>>('/settings', patch)); toast('Saved') } catch (e: any) { toast(e.message) }
  }
  const changed = () => { loadRigs(); void refreshLibraries() }
  if (!st || !settings || !rigs) return <Spinner />
  const on = (k: string) => settings[k] === '1'
  const shared = rigs.filter((r) => r.address !== '127.0.0.1' && r.address !== 'localhost' && r.address !== '::1')
  return (
    <>
      <PageHead title="Game PCs" desc="WittyCouch is a Moonlight client in the page: people play the games on any PC running Sunshine, in their browser, through this server." />
      <Section title="This server" desc="Playing in the page needs a separate program, WittyCouch Host, built on Moonlight’s own code.">
        {st.ready
          ? <p>WittyCouch Host is installed and working.</p>
          : <div className="rig-warn" role="status">
              <p><b>WittyCouch Host isn’t installed on this server.</b> People can add and pair PCs, and the Moonlight app still works, but they can’t play in the page until it is.</p>
              <p>It’s GPL-3.0 software: its source comes with it, and it runs as a program of its own beside this one, which stays Apache-2.0. Put <code>wittycouch-host</code> beside this server, or in the <code>wittycouch/bin</code> folder in its data folder. Releases will be at <a href="https://github.com/wittywilla/WittyCouchHost" target="_blank" rel="noreferrer noopener">its own project</a>.</p>
            </div>}
      </Section>
      <Section title="Who may connect" desc="This has the server connect wherever a person says, so it’s yours to decide. It only ever speaks this one protocol, and nothing else is reachable through it.">
        <div className="set-form">
          <label>Who can use it
            <select className="select" value={settings.couchAccess || 'adults'} onChange={(e) => put({ couchAccess: e.target.value })}>
              <option value="adults">Adult profiles with no limits set (recommended)</option>
              <option value="everyone">Everyone, Kids and Guest profiles too</option>
              <option value="admins">Admins only</option>
            </select>
            <small className="muted">A PC’s desktop is outside anything age limits can reach, so Kids and Guest profiles and limited accounts start without this. Give a Kids profile single games through Shared PCs below.</small>
          </label>
        </div>
        <Row label="PCs on this server’s own network"><Switch label="PCs on this server’s own network" checked={on('couchPrivate')} onChange={(v) => put({ couchPrivate: v ? '1' : '0' })} /></Row>
        <Row label="PCs over the internet, such as a friend’s"><Switch label="PCs over the internet, such as a friend’s" checked={on('couchInternet')} onChange={(v) => put({ couchInternet: v ? '1' : '0' })} /></Row>
        <Row label="Ports other than Sunshine’s usual one" desc="Admins can always. Off makes it harder to use this to look for what’s open on a network."><Switch label="Ports other than Sunshine’s usual one" checked={on('couchCustomPorts')} onChange={(v) => put({ couchCustomPorts: v ? '1' : '0' })} /></Row>
        <div className="set-form">
          <label>Streams at once
            <input className="input" type="number" min={1} max={16} defaultValue={settings.couchMaxStreams || '2'} onBlur={(e) => put({ couchMaxStreams: e.target.value })} />
            <small className="muted">Each stream passes through this server, at up to its full bitrate. The server doesn’t re-encode it.</small>
          </label>
        </div>
      </Section>
      <Section title="Reaching browsers away from home" desc="Nothing needed at home. A stream goes from this server to a browser over UDP.">
        <div className="set-form">
          <label>UDP port
            <input className="input" inputMode="numeric" defaultValue={settings.couchUdpPort || ''} onBlur={(e) => put({ couchUdpPort: e.target.value.trim() })} placeholder="Any free port" />
            <small className="muted">Choose one to forward on your router, or to publish from a container.</small>
          </label>
          <label>This server’s public address
            <input className="input" defaultValue={settings.couchPublicIp || ''} onBlur={(e) => put({ couchPublicIp: e.target.value.trim() })} autoCapitalize="off" spellCheck={false} />
          </label>
          <label>STUN server
            <input className="input" defaultValue={settings.couchStun || ''} onBlur={(e) => put({ couchStun: e.target.value.trim() })} placeholder="None: nothing leaves your network" autoCapitalize="off" spellCheck={false} />
          </label>
        </div>
        <p className="muted">The PC being played is never opened to the internet by this: only this server is. Don’t forward Sunshine’s own ports, and <b>never</b> port 47990, its web interface, which can run commands on that PC.</p>
      </Section>
      <AddRig onAdded={changed} title="Share a PC with this server’s people" desc="A shared PC is paired once by you, and its games become a library. Everyone’s limits apply, so a Kids profile can be given single games from it." />
      {shared.map((r) => <RigCard key={r.id} rig={r} onChange={changed} />)}
      <Section title="Built on">
        <Credits list={st.credits} />
      </Section>
    </>
  )
}
