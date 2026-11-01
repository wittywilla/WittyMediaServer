import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type GameConsole, type GameCore } from '../../api'
import { LoadError, Spinner } from '../../components/common'
import { toast } from '../../state'
import { getConsoles } from '../Games'
import CustomCoresSection, { ConsoleCoreSelect, licenseText } from './GamingCustom'
import { PageHead, Section } from './ui'
import LegalNote from '../../components/LegalNote'

// Settings, Consoles & cores: which cores this server has, and how games reach players.
// A core is someone else's program, so nothing is installed without being asked
// for, with its licence in view first.

type Status = {
  runner: string; encoders: string[]; consoles: GameConsole[]; cores: GameCore[]; canDownload: boolean
  coresDir: string; systemDir: string; running: number; unpackedBytes?: number; biosFolder: string; gpu?: { name?: string; error?: string; checking?: boolean }
}

export default function GamingSettings() {
  const [st, setSt] = useState<Status | null>(null)
  const [busy, setBusy] = useState('')
  const [confirm, setConfirm] = useState<{ id: string; name: string; license: string } | null>(null)
  const [settings, setSettings] = useState<Record<string, string> | null>(null)
  const [failed, setFailed] = useState(false)
  const load = () => api.get<Status>('/gaming').then(setSt).catch(() => setFailed(true))
  useEffect(() => { load(); api.get<Record<string, string>>('/settings').then(setSettings).catch(() => {}) }, [])
  const emptyUnpacked = async () => {
    setBusy('unpacked')
    try {
      const r = await api.del<{ unpackedBytes: number }>('/gaming/unpacked')
      setSt((cur) => (cur ? { ...cur, unpackedBytes: r.unpackedBytes } : cur))
      toast(r.unpackedBytes ? 'Emptied, apart from the games being played' : 'Emptied. Games unpack again when next played')
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  // The graphics card is checked in the background, and the answer comes in by itself.
  const checking = !!st?.gpu?.checking
  useEffect(() => {
    if (!checking) return
    const t = setTimeout(load, 2000)
    return () => clearTimeout(t)
  }, [checking, st])
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
  if (failed && !st) return <LoadError what="This page" onRetry={() => { setFailed(false); load() }} />
  if (!st) return <Spinner />
  const installed = new Map(st.cores.map((c) => [c.id, c]))
  const known = new Set(st.consoles.flatMap((c) => c.cores.map((x) => x.id)))
  const offered = new Set(st.consoles.filter((c) => !c.id.startsWith('custom-')).flatMap((c) => c.cores.map((x) => x.id)))
  const extra = st.cores.filter((c) => !known.has(c.id) && c.source !== 'custom')
  const refresh = async () => { await load(); await getConsoles(true) }
  return (
    <>
      <PageHead title="Consoles & cores" desc="Games run on this server, one separate boxed-in program for each game, and stream to the browser." />
      <Section title="This server">
        <dl className="gaming-facts">
          <dt>Game runner</dt><dd>{st.runner || <><b>Missing from this install</b><br /><span className="muted">Games can’t start without it. It ships with the release downloads; if you built this server yourself, build the runner too (see the gaming guide).</span></>}</dd>
          <dt>Encoders</dt><dd>{st.encoders.join(', ') || 'None found'}</dd>
          <dt>Graphics card</dt><dd>{st.gpu?.name || (checking && !st.gpu?.error ? 'Checking…' : <><b>None this server can use</b><br /><span className="muted">{st.gpu?.error ? `${st.gpu.error}. ` : ''}Consoles that draw with a graphics card won’t start. Older consoles still play.</span></>)}</dd>
          <dt>Games running</dt><dd>{st.running}</dd>
          <dt>Cores folder</dt><dd><code>{st.coresDir}</code></dd>
          <dt>BIOS folder</dt><dd><code>{st.systemDir}</code></dd>
        </dl>
        <p className="muted gaming-bios">Where BIOS files go: the easy way is a folder named <code>{st.biosFolder}</code> inside a console’s games folder, such as <code>PS2/{st.biosFolder}</code>. Folders inside it are fine, it’s never listed as a game, and the server copies what it needs to the BIOS folder above when you press Play. You can also put files straight into that folder: a PlayStation 2 BIOS goes in <code>pcsx2/bios</code> inside it. BIOS files are never downloaded for you.</p>
      </Section>
      <Section title="Consoles and cores" desc="A core is the program that emulates a console. Each has its own license and comes from the libretro project’s build server when you ask for it. Games and BIOS files are never downloaded: you supply your own, legally obtained." actions={<a className="btn sm ghost" href="#custom-cores">Add your own</a>}>
        {!st.canDownload && <p className="muted">There are no prebuilt cores for this platform. Install them with your system’s packages, or place them in the cores folder.</p>}
        <div className="gaming-consoles">
          {st.consoles.map((c) => (
            <article key={c.id} className="gaming-console">
              <h3>{c.name}</h3>
              {c.unavailable && <p className="muted">{c.unavailable}</p>}
              {c.needsGpu && <p className="muted">Needs a graphics card on this server.</p>}
              {!c.unavailable && (c.cores.length > 0 || (c.customCores?.length ?? 0) > 0) && (
                <ul>
                  {c.cores.map((core, i) => {
                    const have = installed.get(core.id)
                    return (
                      <li key={core.id}>
                        <span className="gaming-core"><span><b>{core.name}</b>{i === 0 && <span className="pill gaming-rec">Recommended</span>}</span><small className="muted">License: {core.license}{have && have.source !== 'installed' && have.source !== 'custom' ? ` · found in ${have.source === 'retroarch' ? 'your RetroArch folder' : 'a system folder'}` : have?.source === 'custom' ? ' · custom' : ''}</small></span>
                        {have
                          ? have.source === 'installed'
                            ? <button className="btn sm ghost" disabled={!!busy} onClick={() => remove(core.id, core.name)} aria-label={`Remove ${core.name}`}>{busy === core.id ? 'Removing…' : 'Remove'}</button>
                            : <span className="muted">Ready</span>
                          : st.canDownload && offered.has(core.id) && <button className="btn sm" disabled={!!busy} onClick={() => setConfirm(core)} aria-label={`Install ${core.name}`}>{busy === core.id ? 'Installing…' : 'Install'}</button>}
                      </li>
                    )
                  })}
                  {c.customCores?.filter((x) => !c.cores.some((y) => y.id === x.id)).map((x) => (
                    <li key={x.id}>
                      <span className="gaming-core"><span><b>{x.name}</b><span className="pill gaming-rec">Custom</span></span><small className="muted">{licenseText(x)}. Third-party, unverified.</small></span>
                      <span className="muted">Ready</span>
                    </li>
                  ))}
                </ul>
              )}
              <ConsoleCoreSelect console={c} onChanged={refresh} />
              {c.bios && c.bios.length > 0 && (
                <p className="muted gaming-bios">{c.bios.some((b) => b.required) ? 'BIOS files this console needs, which you supply: ' : 'BIOS files some games use, which you supply: '}{c.bios.map((b) => <span key={b.file}><code>{b.file}</code> ({b.for}) </span>)}Put them in a <code>{st.biosFolder}</code> folder beside the games.</p>
              )}
            </article>
          ))}
        </div>
        {extra.length > 0 && (
          <>
            <h3>Other cores found</h3>
            <p className="muted">These aren’t in this server’s list, so nothing plays with them until you add one under Custom cores below, which tests it first.</p>
            <ul className="gaming-extra">
              {extra.map((c) => <li key={c.id}><b>{c.name}</b> <small className="muted">{licenseText(c)}</small></li>)}
            </ul>
          </>
        )}
      </Section>
      <CustomCoresSection onChanged={refresh} />
      {settings && (
        <Section title="Streaming" desc="Most homes need nothing here. Open a UDP port when playing from outside your network, or from a container.">
          <div className="set-form gaming-form">
            <label>Games at once
              <input className="input" type="number" min={1} max={16} defaultValue={settings.gamingMaxSessions || '2'} onBlur={(e) => put({ gamingMaxSessions: e.target.value })} />
              <small className="muted">Each game takes one CPU core or more. Two is a safe start.</small>
            </label>
            <label>Games on the graphics card at once
              <input className="input" type="number" min={1} max={16} defaultValue={settings.gamingMaxGpuSessions || '2'} onBlur={(e) => put({ gamingMaxGpuSessions: e.target.value })} />
              <small className="muted">For consoles that draw with a graphics card. More games share it, so each one has less.</small>
            </label>
            <label>Internal resolution
              <select className="select" value={settings.gamingResolution || ''} onChange={(e) => put({ gamingResolution: e.target.value })}>
                <option value="">Automatic (twice native on most consoles)</option>
                <option value="1">Native: fastest, works on any graphics card</option>
                <option value="2">2×</option>
                <option value="3">3×</option>
                <option value="4">4×: sharpest, needs a fast card</option>
              </select>
              <small className="muted">How sharply hardware-rendered games are drawn. Applies the next time a game starts.</small>
            </label>
            <label>Picture quality
              <select className="select" value={settings.gamingQuality || ''} onChange={(e) => put({ gamingQuality: e.target.value })}>
                <option value="">Automatic</option>
                <option value="low">Data saver</option>
                <option value="high">High</option>
              </select>
            </label>
            <label>Fastest fast-forward
              <input className="input" type="number" min={1} max={16} step={0.5} defaultValue={settings.gamingMaxSpeed || ''} placeholder="Automatic" onBlur={(e) => put({ gamingMaxSpeed: e.target.value })} />
              <small className="muted">Times normal speed, for everyone. Automatic is 4×, or 2× on a machine with few CPU threads. 1 turns it off.</small>
            </label>
            <label>Memory for rewind, per game
              <input className="input" type="number" min={0} max={512} defaultValue={settings.gamingRewindMB || ''} placeholder="Automatic" onBlur={(e) => put({ gamingRewindMB: e.target.value })} />
              <small className="muted">In MB. Automatic is 32, or 16 on a machine with few CPU threads. 0 turns rewind off for everyone.</small>
            </label>
            <label>Run-ahead
              <select className="select" value={settings.gamingRunAhead === '0' ? '0' : '1'} onChange={(e) => put({ gamingRunAhead: e.target.value })}>
                <option value="1">Allowed</option>
                <option value="0">Not allowed</option>
              </select>
              <small className="muted">Lowers delay, and uses several times the CPU of a normal game.</small>
            </label>
            <label>Game runner
              <select className="select" value={settings.gamingRunner || 'auto'} onChange={(e) => put({ gamingRunner: e.target.value })}>
                <option value="auto">Automatic</option>
                <option value="builtin">Built in</option>
                <option value="retroarch">A separate RetroArch-based runner, when installed</option>
              </select>
            </label>
            <label>Space for unpacked games (GB)
              <input className="input" type="number" min={1} max={100000} defaultValue={settings.gamingUnpackGB || '16'} onBlur={(e) => put({ gamingUnpackGB: e.target.value })} />
              <small className="muted">Games in archives are unpacked here when played, and kept for next time. The longest unplayed go first when it fills up.{st?.unpackedBytes ? ` In use now: ${(st.unpackedBytes / 1073741824).toFixed(1)} GB.` : ''}</small>
              {!!st?.unpackedBytes && <button type="button" className="btn sm" disabled={busy === 'unpacked'} onClick={emptyUnpacked}>Empty it now</button>}
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
      <Section title="What everyone starts with" desc="Set a picture, shortcuts or anything else as the starting point for everyone on this server. People can still change anything for themselves.">
        <p><Link to="/settings/games">Open game settings</Link>, then choose <b>Everyone</b> at the top of a panel.</p>
      </Section>
      <LegalNote kind="cores" className="set-legal" />
      {confirm && (
        <div className="modal-backdrop" onClick={() => setConfirm(null)}>
          <div className="modal" role="dialog" aria-label={`Install ${confirm.name}`} onClick={(e) => e.stopPropagation()}>
            <h2>Install {confirm.name}?</h2>
            <p>This downloads a program from <b>buildbot.libretro.com</b>, the libretro project’s build server, and runs it on this server in a boxed-in process whenever someone plays.</p>
            <p>Its license is <b>{confirm.license}</b>. The core is its authors’ work and stays a separate program from this server.</p>
            <LegalNote kind="cores" />
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
