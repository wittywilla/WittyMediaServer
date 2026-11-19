// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { api } from '../api'
import { t, tx } from '../i18n'
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

// A translated sentence with the address drawn in it: {address} in the text is where it goes.
function withAddress(text: string, address: ReactNode) {
  const [before, after = ''] = text.split('{address}')
  return <>{before}{address}{after}</>
}

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

// waitGone resolves once nothing answers at an origin any more, or after the time given.
async function waitGone(origin: string, seconds: number) {
  const until = Date.now() + seconds * 1000
  while (Date.now() < until) {
    try { await fetch(origin + '/api/health', { cache: 'no-store' }) } catch { return }
    await sleep(500)
  }
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
    const timer = setTimeout(() => {
      setChecking(true)
      api.post<PortCheck>('/network/check', { port }).then((c) => { if (n === seq.current) setCheck(c) })
        .catch((e) => { if (n === seq.current) setError(e.message) })
        .finally(() => { if (n === seq.current) setChecking(false) })
    }, 400)
    return () => clearTimeout(timer)
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
    // At the same address, the old server has to be gone first, or it would be the one answering.
    if (!follow) await waitGone(target, 30)
    if (await waitFor(target, 90)) location.href = target + location.pathname + location.search
    else setRestart('lost')
  }

  const newAddr = `http://${location.hostname}:${view.next}`
  const invalid = value.trim() !== '' && Number.isNaN(port)
  const problems = invalid ? [t('A port is a whole number, such as 8096.')] : (check?.problems || []).map(tx)
  return (
    <div className={'port-setting' + (compact ? ' compact' : '')}>
      {locked && <Notice tone="warn" title={view.docker ? t('Set by Docker') : t('Set somewhere else')}>{tx(view.locked)}</Notice>}
      <div className="port-row">
        <label htmlFor={id + 'p'} className="port-label">{t('Port')}</label>
        <Input id={id + 'p'} inputMode="numeric" autoComplete="off" className="port-input" value={value} disabled={locked || saving || !!restart}
          aria-describedby={id + 'h'} aria-invalid={problems.length > 0 || undefined}
          onChange={(e) => setValue(e.target.value.replace(/[^\d]/g, '').slice(0, 5))}
          onKeyDown={(e) => { if (e.key === 'Enter' && changed && !problems.length) void save() }} />
        <Button variant="primary" busy={saving} disabled={locked || !changed || problems.length > 0 || checking || !!restart} onClick={save}>{t('Save')}</Button>
      </div>
      <small id={id + 'h'} className="port-hint">
        {locked
          ? t('It answers on port {port}.', { port: String(view.port) })
          : <>{t('A number from 1024 to 65535 that nothing else on this computer uses.')} {view.next !== view.port
            ? t('Now {port}, {next} after a restart.', { port: String(view.port), next: String(view.next) })
            : t('Now {port}.', { port: String(view.port) })}</>}
      </small>
      <div aria-live="polite" className="port-messages">
        {changed && checking && !problems.length && <small className="port-hint">{t('Checking port {port}…', { port: String(port) })}</small>}
        {problems.map((p) => <Notice key={p} tone="danger">{p}</Notice>)}
        {(check?.warnings || []).map((w) => <Notice key={w} tone="warn">{tx(w)}</Notice>)}
        {changed && check?.ok && !check.current && !checking && <small className="port-ok">{t('Port {port} is free.', { port: String(port) })}</small>}
        {error && <Notice tone="danger">{error}</Notice>}
      </div>
      {!locked && view.next !== view.port && (
        <Notice tone="ok" title={t('Saved: port {port} from the next start', { port: String(view.next) })}
          action={view.canRestart && restart !== 'waiting' ? <Button variant="primary" busy={restart === 'going'} onClick={restartNow}>{t('Restart now')}</Button> : undefined}>
          {restart === 'waiting'
            ? <span>{t('Restarting…')} {followsPort(view) ? withAddress(t('This page opens again at {address} when it’s ready.'), <code>{newAddr}</code>) : t('This page comes back when it’s ready.')}</span>
            : restart === 'lost'
              ? <span>{withAddress(t('It hasn’t answered yet. Try opening {address} in a moment, or restart it from its tray or menu-bar icon.'), <a href={newAddr}>{newAddr}</a>)}</span>
              : <span>{view.canRestart ? t('Restart now, or it moves the next time the server starts.') : t('Restart the server the way you started it.')} {withAddress(t('Then it’s at {address}, and anything that uses the old address, such as a bookmark, needs the new one.'), <code>{newAddr}</code>)}</span>}
        </Notice>
      )}
    </div>
  )
}
