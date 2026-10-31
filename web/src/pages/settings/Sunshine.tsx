import { useEffect, useState } from 'react'
import { api } from '../../api'
import { Spinner } from '../../components/common'
import { toast, useApp } from '../../state'
import type { RigView } from '../Rigs'
import { RigCard, SetupHelp } from './GameRigs'
import { PageHead, Section } from './ui'
import '../rigs.css'

// Settings, Advanced, WittySunshine: streaming demanding PC games hosted on this server's own
// hardware. It's off unless asked for, and has nothing to do with reaching other PCs, which is
// Game PCs: see docs/GAME-RIGS.md.

type Status = { enabled: boolean; installed: string; running: boolean; rig?: RigView; license: string; source: string }

export default function SunshineSettings() {
  const [st, setSt] = useState<Status | null>(null)
  const [busy, setBusy] = useState(false)
  const [sure, setSure] = useState(false)
  const { refreshLibraries } = useApp()
  const load = () => api.get<Status>('/sunshine').then(setSt).catch((e) => toast(e.message))
  useEffect(() => { load() }, [])
  const set = async (enabled: boolean) => {
    setBusy(true); setSure(false)
    try { setSt(await api.put<Status>('/sunshine', { enabled })); void refreshLibraries() } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  if (!st) return <Spinner />
  return (
    <>
      <PageHead title="WittySunshine" desc="Advanced, and most servers don’t need it. It’s for playing demanding PC games installed on this server itself, using this machine’s graphics card. To play on other PCs, use Game PCs." />
      <Section title="Hosting games on this server">
        {!st.enabled && !sure && (
          <>
            <p>This uses <b>Sunshine</b>, a separate program you install on this machine. It streams this machine’s screen, and everything on it, to whoever plays, and adds a service and virtual controller drivers. It needs a good graphics card, and a screen or a virtual one. Sunshine is GPL-3.0 software, with its own license and source.</p>
            <button className="btn" disabled={busy} onClick={() => setSure(true)}>Turn on</button>
          </>
        )}
        {sure && (
          <div className="rig-warn" role="alert">
            <p>Sunshine is GPL-3.0 software by LizardByte, installed separately from <a href={st.source} target="_blank" rel="noreferrer noopener">its own project</a>. This server only talks to it over the network. Anyone you give a game from it can often get from that game to this machine’s desktop, so give them out with care.</p>
            <div className="rig-row-actions">
              <button className="btn sm" onClick={() => set(true)}>Turn it on</button>
              <button className="btn sm ghost" onClick={() => setSure(false)}>Not now</button>
            </div>
          </div>
        )}
        {st.enabled && (
          <>
            <p>{st.running ? 'Sunshine is running on this machine.' : st.installed ? <>Sunshine is installed at <code>{st.installed}</code>, but isn’t running. Start it, then come back.</> : 'Sunshine isn’t installed on this machine, or isn’t running.'}</p>
            {!st.running && <SetupHelp />}
            <div className="rig-row-actions"><button className="btn sm ghost" disabled={busy} onClick={() => set(false)}>Turn off and remove its library</button></div>
          </>
        )}
      </Section>
      {st.enabled && st.rig && <RigCard rig={st.rig} onChange={load} />}
      <Section title="License"><p>WittySunshine is Apache-2.0. Sunshine itself is {st.license}: <a href={st.source} target="_blank" rel="noreferrer noopener">source</a>. Neither contains the other.</p></Section>
    </>
  )
}
