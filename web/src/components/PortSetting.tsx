import { useEffect, useId, useRef, useState } from 'react'
import { api } from '../api'
import { Button, Input, Notice } from '../ui'
import './portsetting.css'

// The port the server answers on, for Settings → Network and the setup wizard: check it as it's
// typed, save it to settings.ini, restart, and open this page again at the new address. Where
// something stronger sets the port (-listen, WMS_LISTEN, wms.json, Docker) it says where instead.

export type NetworkView = {
  port: number; host: string; from: 'flag' | 'env' | 'config' | 'settings' | 'default'
  settingsFile?: string; saveFile: string; locked?: string; docker: boolean
  next: number; canRestart: boolean; problems: string[]; addresses: string[]
  discoveryPort: number; httpsPort?: number
}
type PortCheck = { port: number; ok: boolean; problems: string[]; warnings: string[]; current: boolean }

export function useNetwork() {
  const [view, setView] = useState<NetworkView | null>(null)
  const [failed, setFailed] = useState(false)
  const load = () => { setFailed(false); api.get<NetworkView>('/network').then(setView).catch(() => setFailed(true)) }
  useEffect(load, [])
  return { view, setView, failed, retry: load }
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

// The page is on the server's home port, so it can follow it to the new one. Through Remote
// Access's HTTPS port, a proxy or the web app's dev server, the address doesn't change.
export function followsPort(v: NetworkView) {
  const here = location.port || (location.protocol === 'https:' ? '443' : '80')
  return location.protocol === 'http:' && here === String(v.port)
}

// waitFor resolves once something answers at an origin. A no-cors request can't read the answer,
// but it only fails while nothing is listening, which is all this needs to know.
async function waitFor(origin: string, seconds: number) {
  const until = Date.now() + seconds * 1000
  while (Date.now() < until) {
    try {
      await fetch(origin + '/api/health', { mode: 'no-cors', cache: 'no-store' })
      return true
    } catch { /* not yet */ }
    await sleep(1000)
  }
  return false
}

export function PortSetting({ view, onSaved, compact }: { view: NetworkView; onSaved: (v: NetworkView) => void; compact?: boolean }) {
  const id = useId()
  const [value, setValue] = useState(String(view.next))
  const [check, setCheck] = useState<PortCheck | null>(null)
  const [checking, setChecking] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [restart, setRestart] = useState<'' | 'going' | 'waiting' | 'lost'>('')
  const seq = useRef(0)
  const port = /^\d{1,5}$/.test(value.trim()) ? Number(value.trim()) : NaN
  const changed = !Number.isNaN(port) && port !== view.next
  const locked = !!view.locked

  // Checked a moment after typing stops, and again just before saving.
  useEffect(() => {
    setError('')
    if (locked || !changed) { setCheck(null); return }
    const n = ++seq.current
    const t = setTimeout(() => {
      setChecking(true)
      api.post<PortCheck>('/network/check', { port }).then((c) => { if (n === seq.current) setCheck(c) })
        .catch((e) => { if (n === seq.current) setError(e.message) })
        .finally(() => { if (n === seq.current) setChecking(false) })
    }, 400)
    return () => clearTimeout(t)
  }, [port, changed, locked])

  const save = async () => {
    setSaving(true); setError('')
    try {
      const c = await api.post<PortCheck>('/network/check', { port })
      setCheck(c)
      if (!c.ok) return
      const v = await api.put<NetworkView>('/network', { port })
      onSaved(v)
      setValue(String(v.next))
    } catch (e: any) { setError(e.message) } finally { setSaving(false) }
  }

  const restartNow = async () => {
    setRestart('going'); setError('')
    const follow = followsPort(view)
    const target = follow ? `${location.protocol}//${location.hostname}:${view.next}` : location.origin
    try {
      await api.post('/network/restart')
    } catch (e: any) { setRestart(''); setError(e.message); return }
    setRestart('waiting')
    // The old server has to let go first, or this origin would still answer.
    if (!follow) await sleep(4000)
    if (await waitFor(target, 90)) location.href = target + location.pathname + location.search
    else setRestart('lost')
  }

  const newAddr = `http://${location.hostname}:${view.next}`
  const invalid = value.trim() !== '' && Number.isNaN(port)
  const problems = invalid ? ['A port is a whole number, such as 8096.'] : check?.problems || []
  return (
    <div className={'port-setting' + (compact ? ' compact' : '')}>
      {locked && <Notice tone="warn" title={view.docker ? 'Set by Docker' : 'Set somewhere else'}>{view.locked}</Notice>}
      <div className="port-row">
        <label htmlFor={id + 'p'} className="port-label">Port</label>
        <Input id={id + 'p'} inputMode="numeric" autoComplete="off" className="port-input" value={value} disabled={locked || saving || !!restart}
          aria-describedby={id + 'h'} aria-invalid={problems.length > 0 || undefined}
          onChange={(e) => setValue(e.target.value.replace(/[^\d]/g, '').slice(0, 5))}
          onKeyDown={(e) => { if (e.key === 'Enter' && changed && !problems.length) void save() }} />
        <Button variant="primary" busy={saving} disabled={locked || !changed || problems.length > 0 || checking || !!restart} onClick={save}>Save</Button>
      </div>
      <small id={id + 'h'} className="port-hint">
        {locked ? <>It answers on port {view.port}.</> : <>A number from 1024 to 65535 that nothing else on this computer uses. Now {view.port}{view.next !== view.port && <>, {view.next} after a restart</>}.</>}
      </small>
      <div aria-live="polite" className="port-messages">
        {changed && checking && !problems.length && <small className="port-hint">Checking port {port}…</small>}
        {problems.map((p) => <Notice key={p} tone="danger">{p}</Notice>)}
        {(check?.warnings || []).map((w) => <Notice key={w} tone="warn">{w}</Notice>)}
        {changed && check?.ok && !check.current && !checking && <small className="port-ok">Port {port} is free.</small>}
        {error && <Notice tone="danger">{error}</Notice>}
      </div>
      {!locked && view.next !== view.port && (
        <Notice tone="ok" title={`Saved: port ${view.next} from the next start`}
          action={view.canRestart && restart !== 'waiting' ? <Button variant="primary" busy={restart === 'going'} onClick={restartNow}>Restart now</Button> : undefined}>
          {restart === 'waiting'
            ? <span>Restarting… {followsPort(view) ? <>This page opens again at <code>{newAddr}</code> when it’s ready.</> : 'This page comes back when it’s ready.'}</span>
            : restart === 'lost'
              ? <span>It hasn’t answered yet. Try opening <a href={newAddr}>{newAddr}</a> in a moment, or restart it from its tray or menu-bar icon.</span>
              : <span>{view.canRestart ? 'Restart now, or it moves the next time the server starts. ' : 'Restart the server the way you started it. '}Then it’s at <code>{newAddr}</code>, and anything that uses the old address, such as a bookmark, needs the new one.</span>}
        </Notice>
      )}
    </div>
  )
}
