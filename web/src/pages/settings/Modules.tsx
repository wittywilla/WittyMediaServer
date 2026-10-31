import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { IconRefresh } from '../../icons'
import { toast } from '../../state'
import { PageHead, Row, Section, Switch } from './ui'

type Offer = { version: string; minServer: string; maxServer: string; notes?: string; url?: string; published?: number; prerelease: boolean; size?: number; reason?: string; blocked?: string }
type ModuleStatus = {
  id: string; name: string; version: string; builtinVersion: string; delivery: string; updatable: boolean
  minServer: string; maxServer: string; api: number; repo: string; state: 'builtin' | 'installed' | 'fallback'
  notice?: string; previous?: string; canRollback: boolean; latest?: Offer; tooNew?: Offer
  phase: 'idle' | 'downloading' | 'verifying' | 'installing' | 'failed'; done?: number; total?: number; error?: string
}
type Report = { server: string; api: number; minApi: number; checkedAt?: number; checkError?: string; modules: ModuleStatus[] }

const PHASE: Record<string, string> = { downloading: 'Downloading', verifying: 'Checking the signature', installing: 'Installing' }
const busy = (m: ModuleStatus) => !!PHASE[m.phase]
const mb = (n = 0) => (n / 1048576).toFixed(1)
const stateLabel = (m: ModuleStatus) => m.state === 'installed' ? 'Updated' : m.state === 'fallback' ? 'Needs a look' : 'Built in'

// Modules: each one's version, the servers it works with and what it is doing, with
// updates that need no new server for the ones that can have them. Nothing here
// goes online unless an admin presses a button or turned the automatic check on.
export default function ModulesSettings() {
  const [r, setR] = useState<Report | null>(null)
  const [checking, setChecking] = useState(false)
  const [settings, setSettings] = useState<Record<string, string> | null>(null)

  useEffect(() => {
    api.get<Report>('/modules').then(setR).catch((e) => toast(e.message))
    api.get<Record<string, string>>('/settings').then(setSettings).catch(() => {})
  }, [])

  // While something installs, follow it.
  const active = !!r?.modules.some(busy)
  useEffect(() => {
    if (!active) return
    const t = setInterval(() => { api.get<Report>('/modules').then(setR).catch(() => {}) }, 1000)
    return () => clearInterval(t)
  }, [active])

  const check = async () => {
    setChecking(true)
    try {
      const n = await api.post<Report>('/modules/check')
      setR(n)
      if (n.checkError) toast(n.checkError)
      else if (!n.modules.some((m) => m.latest)) toast('Every module is up to date')
    } catch (e: any) { toast(e.message) } finally { setChecking(false) }
  }
  const install = async (m: ModuleStatus) => {
    if (m.latest?.blocked && !confirm(`${m.name} ${m.latest.version} was taken out of use: ${m.latest.blocked}\n\nInstall it again?`)) return
    try { setR(await api.post<Report>(`/modules/${m.id}/install`)) } catch (e: any) { toast(e.message) }
  }
  const rollback = async (m: ModuleStatus) => {
    const to = m.previous ? `version ${m.previous}` : `the built-in copy (${m.builtinVersion})`
    if (!confirm(`Stop using ${m.name} ${m.version} and go back to ${to}?`)) return
    try { setR(await api.post<Report>(`/modules/${m.id}/rollback`)); toast(`${m.name} went back to ${to}`) } catch (e: any) { toast(e.message) }
  }
  const save = async (k: string, v: boolean) => {
    const old = settings
    setSettings((s) => ({ ...(s || {}), [k]: v ? '1' : '0' }))
    try { await api.put('/settings', { [k]: v ? '1' : '0' }) } catch (e: any) { setSettings(old); toast(e.message) }
  }

  // Until it is set, the automatic check follows the one for server updates.
  const autoCheck = (settings?.moduleUpdateCheck || settings?.updateCheck || '1') === '1'
  const updatable = r?.modules.filter((m) => m.updatable) || []
  const withServer = r?.modules.filter((m) => !m.updatable) || []

  return (
    <div>
      <PageHead title="Modules" desc="The parts of WittyMediaServer that have versions of their own. Some can be updated without a new server. Each says which servers it works with, and every one has a built-in copy to fall back on." />

      <Section title="Updates" desc="Each update is signed, and this server installs only what it can verify. It takes effect straight away, with no restart."
        actions={<button className="btn ghost sm" onClick={check} disabled={checking}><IconRefresh /> {checking ? 'Checking…' : 'Check now'}</button>}>
        <Row label="Look for updates automatically" desc={r?.checkedAt ? `Asks GitHub twice a day. Last checked ${new Date(r.checkedAt).toLocaleString()}.` : 'Asks GitHub twice a day. When this is off, the server only asks when you press Check now.'}>
          <Switch label="Look for updates automatically" checked={autoCheck} disabled={!settings} onChange={(v) => save('moduleUpdateCheck', v)} />
        </Row>
        <Row label="Install updates automatically" desc="Installs what the check finds, if it works with this server. A version that fails is taken back out by itself." dim={!autoCheck}>
          <Switch label="Install updates automatically" checked={settings?.moduleAutoInstall === '1'} disabled={!settings || !autoCheck} onChange={(v) => save('moduleAutoInstall', v)} />
        </Row>
        {r?.checkError && <p className="upd-msg warn">The last check didn’t work: {r.checkError} Everything keeps working as it is.</p>}
      </Section>

      <Section title="Can update on their own" desc={r ? `This server is version ${r.server} and has module API ${r.minApi === r.api ? r.api : `${r.minApi} to ${r.api}`}.` : undefined}>
        <div className="mod-list">
          {updatable.map((m) => (
            <div key={m.id} className="mod-card">
              <div className="mod-row">
                <span className="mod-name"><b>{m.name}</b><span className="muted">Works with server {m.minServer} to {m.maxServer}, module API {m.api}</span></span>
                <span className="mod-ver">{m.version}<span className={'mod-local' + (m.state === 'fallback' ? ' warn' : '')}>{stateLabel(m)}</span></span>
                <Link className="btn ghost sm" to={`/settings/logs?module=${m.id}`} aria-label={`Logging for ${m.name}`}>Logging</Link>
              </div>
              {m.notice && <p className="upd-msg warn">{m.notice}</p>}
              {busy(m) && (
                <div className="upd-progress" role="status">
                  <span>{PHASE[m.phase]}{m.phase === 'downloading' && m.total ? ` · ${mb(m.done)} of ${mb(m.total)} MB` : ''}…</span>
                  <progress value={m.phase === 'downloading' && m.total ? m.done : undefined} max={m.total || undefined} />
                </div>
              )}
              {m.phase === 'failed' && <p className="upd-msg warn">The update wasn’t installed, and nothing changed: {m.error}</p>}
              {m.latest && !busy(m) && (
                <div className="upd-new">
                  <div className="upd-head">
                    <div>
                      <b>Version {m.latest.version} is available</b>
                      <span className="muted">Works with server {m.latest.minServer} to {m.latest.maxServer}{m.latest.prerelease && ' · pre-release'}{m.latest.size ? ` · ${mb(m.latest.size)} MB` : ''}</span>
                    </div>
                    <button className="btn primary sm" onClick={() => install(m)}>Update now</button>
                  </div>
                  {m.latest.blocked && <p className="upd-msg warn">This version was taken out of use before: {m.latest.blocked}</p>}
                  {m.latest.notes && <div className="upd-notes">{m.latest.notes}</div>}
                </div>
              )}
              {m.tooNew && <p className="upd-msg">Version {m.tooNew.version} exists but doesn’t fit this server. {m.tooNew.reason}</p>}
              {m.canRollback && !busy(m) && (
                <div className="mod-actions">
                  <button className="btn ghost sm" onClick={() => rollback(m)}>Go back to {m.previous ? `version ${m.previous}` : `the built-in copy (${m.builtinVersion})`}</button>
                </div>
              )}
            </div>
          ))}
        </div>
      </Section>

      <Section title="Update with the server" desc="These are part of the server today, so a new server brings their new versions.">
        <div className="mod-list">
          {withServer.map((m) => (
            <div key={m.id} className="mod-row">
              <span className="mod-name"><b>{m.name}</b><span className="muted">Works with server {m.minServer} to {m.maxServer}, module API {m.api}</span></span>
              <span className="mod-ver">{m.version}<span className="mod-local">{stateLabel(m)}</span></span>
              <Link className="btn ghost sm" to={`/settings/logs?module=${m.id}`} aria-label={`Logging for ${m.name}`}>Logging</Link>
            </div>
          ))}
        </div>
      </Section>
    </div>
  )
}
