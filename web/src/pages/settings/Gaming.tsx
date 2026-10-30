import { useEffect, useState } from 'react'
import { api, type GameConsole, type GameCore } from '../../api'
import { Spinner } from '../../components/common'
import { toast } from '../../state'
import { getConsoles } from '../Games'
import { PageHead, Section } from './ui'

// Settings, Gaming: which cores this server has, and how games reach players.
// A core is someone else's program, so nothing is installed without being asked
// for, with its licence in view first.

type Status = {
  runner: string; encoders: string[]; consoles: GameConsole[]; cores: GameCore[]; canDownload: boolean
  coresDir: string; systemDir: string; running: number
}

export default function GamingSettings() {
  const [st, setSt] = useState<Status | null>(null)
  const [busy, setBusy] = useState('')
  const [confirm, setConfirm] = useState<{ id: string; name: string; license: string } | null>(null)
  const [settings, setSettings] = useState<Record<string, string> | null>(null)
  const load = () => api.get<Status>('/gaming').then(setSt).catch((e) => toast(e.message))
  useEffect(() => { load(); api.get<Record<string, string>>('/settings').then(setSettings).catch(() => {}) }, [])
  const install = async () => {
    if (!confirm) return
    const c = confirm
    setConfirm(null)
    setBusy(c.id)
    try {
      await api.post(`/gaming/cores/${c.id}/install`)
      toast(`${c.name} installed`)
      await load()
      await getConsoles(true)
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const remove = async (id: string, name: string) => {
    setBusy(id)
    try { await api.del(`/gaming/cores/${id}`); toast(`${name} removed`); await load(); await getConsoles(true) } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const put = async (patch: Record<string, string>) => {
    try { setSettings(await api.put<Record<string, string>>('/settings', patch)); toast('Saved') } catch (e: any) { toast(e.message) }
  }
  if (!st) return <Spinner />
  const installed = new Map(st.cores.map((c) => [c.id, c]))
  const known = new Set(st.consoles.flatMap((c) => c.cores.map((x) => x.id)))
  const extra = st.cores.filter((c) => !known.has(c.id))
  return (
    <>
      <PageHead title="Gaming" desc="Games run on this server, one separate boxed-in program for each game, and stream to the browser." />
      <Section title="This server">
        <dl className="gaming-facts">
          <dt>Game runner</dt><dd>{st.runner || <b>Missing from this install</b>}</dd>
          <dt>Encoders</dt><dd>{st.encoders.join(', ') || 'None found'}</dd>
          <dt>Games running</dt><dd>{st.running}</dd>
          <dt>Cores folder</dt><dd><code>{st.coresDir}</code></dd>
          <dt>BIOS folder</dt><dd><code>{st.systemDir}</code></dd>
        </dl>
      </Section>
      <Section title="Consoles and cores" desc="A core is the program that emulates a console. Each has its own licence and comes from the libretro project’s build server when you ask for it. Games and BIOS files are never downloaded: you supply your own.">
        {!st.canDownload && <p className="muted">There are no prebuilt cores for this platform. Install them with your system’s packages, or place them in the cores folder.</p>}
        <div className="gaming-consoles">
          {st.consoles.map((c) => (
            <article key={c.id} className="gaming-console">
              <h3>{c.name}</h3>
              {c.unavailable && <p className="muted">{c.unavailable}</p>}
              {!c.unavailable && c.cores.length > 0 && (
                <ul>
                  {c.cores.map((core, i) => {
                    const have = installed.get(core.id)
                    return (
                      <li key={core.id}>
                        <span className="gaming-core"><b>{core.name}</b>{i === 0 && <span className="muted"> · recommended</span>}<small className="muted">Licence: {core.license}{have && have.source !== 'installed' ? ` · found in ${have.source === 'retroarch' ? 'your RetroArch folder' : 'a system folder'}` : ''}</small></span>
                        {have
                          ? have.source === 'installed'
                            ? <button className="btn sm ghost" disabled={!!busy} onClick={() => remove(core.id, core.name)} aria-label={`Remove ${core.name}`}>{busy === core.id ? 'Removing…' : 'Remove'}</button>
                            : <span className="muted">Ready</span>
                          : st.canDownload && <button className="btn sm" disabled={!!busy} onClick={() => setConfirm(core)} aria-label={`Install ${core.name}`}>{busy === core.id ? 'Installing…' : 'Install'}</button>}
                      </li>
                    )
                  })}
                </ul>
              )}
              {c.bios && c.bios.length > 0 && (
                <p className="muted gaming-bios">BIOS files some games use, which you supply: {c.bios.map((b) => <span key={b.file}><code>{b.file}</code> ({b.for}) </span>)}</p>
              )}
            </article>
          ))}
        </div>
        {extra.length > 0 && (
          <>
            <h3>Other cores found</h3>
            <ul className="gaming-extra">
              {extra.map((c) => <li key={c.id}><b>{c.name}</b> <small className="muted">{c.license || 'Licence unknown'}{c.nonCommercial ? ': non-commercial use only' : ''}</small></li>)}
            </ul>
          </>
        )}
      </Section>
      {settings && (
        <Section title="Streaming" desc="Most homes need nothing here. Open a UDP port when playing from outside your network, or from a container.">
          <div className="set-form gaming-form">
            <label>Games at once
              <input className="input" type="number" min={1} max={16} defaultValue={settings.gamingMaxSessions || '2'} onBlur={(e) => put({ gamingMaxSessions: e.target.value })} />
              <small className="muted">Each game takes one CPU core or more. Two is a safe start.</small>
            </label>
            <label>Picture quality
              <select className="select" value={settings.gamingQuality || ''} onChange={(e) => put({ gamingQuality: e.target.value })}>
                <option value="">Automatic</option>
                <option value="low">Data saver</option>
                <option value="high">High</option>
              </select>
            </label>
            <label>Game runner
              <select className="select" value={settings.gamingRunner || 'auto'} onChange={(e) => put({ gamingRunner: e.target.value })}>
                <option value="auto">Automatic</option>
                <option value="builtin">Built in</option>
                <option value="retroarch">WittyRetroArch, when installed</option>
              </select>
            </label>
            <label>UDP port for WebRTC
              <input className="input" type="number" min={0} max={65535} defaultValue={settings.gamingUdpPort || ''} placeholder="Any free port" onBlur={(e) => put({ gamingUdpPort: e.target.value })} />
            </label>
            <label>Public IP addresses
              <input className="input" defaultValue={settings.gamingPublicIp || ''} placeholder="Only when behind port forwarding" onBlur={(e) => put({ gamingPublicIp: e.target.value })} />
            </label>
            <label>STUN servers
              <input className="input" defaultValue={settings.gamingStun || ''} placeholder="stun:stun.example.org:3478" onBlur={(e) => put({ gamingStun: e.target.value })} />
            </label>
          </div>
        </Section>
      )}
      {confirm && (
        <div className="modal-backdrop" onClick={() => setConfirm(null)}>
          <div className="modal" role="dialog" aria-label={`Install ${confirm.name}`} onClick={(e) => e.stopPropagation()}>
            <h2>Install {confirm.name}?</h2>
            <p>This downloads a program from <b>buildbot.libretro.com</b>, the libretro project’s build server, and runs it on this server in a boxed-in process whenever someone plays.</p>
            <p>Its licence is <b>{confirm.license}</b>. The core is its authors’ work and stays a separate program from this server.</p>
            <div className="actions">
              <button className="btn ghost" onClick={() => setConfirm(null)}>Cancel</button>
              <button className="btn primary" onClick={install}>Install</button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
