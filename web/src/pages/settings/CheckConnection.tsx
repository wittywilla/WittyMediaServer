// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import { toast } from '../../state'
import { t, tk, tn, tx } from '../../i18n'
import './connectcheck.css'
import { byDevice, stepStates, type Attempt, type StepInfo } from './connectCheckModel'

// Settings → Remote access → Peer-to-peer connections: "Check connection", can a device away from home reach this
// server, one line each with what to do; and "Recent tries", each device's last tries at connecting, so the owner
// can see a device tried and why it was refused. Both are the server's own knowledge: nothing is sent anywhere.

type Check = { id: string; title: string; state: 'ok' | 'warn' | 'fail' | 'off'; detail: string; next?: string }

const STATE_WORD: Record<Check['state'], string> = { ok: tk('Working'), warn: tk('Worth a look'), fail: tk('Not working'), off: tk('Off') }
const KIND_WORD: Record<Attempt['kind'], string> = { invite: tk('Invite'), code: tk('Code'), connect: tk('Connecting') }
const PATH_WORD: Record<string, string> = { local: tk('on the same network'), direct: tk('directly'), relayed: tk('through a relay') }

export function CheckConnection({ running }: { running: boolean }) {
  const [checks, setChecks] = useState<Check[] | null>(null)
  const [busy, setBusy] = useState(false)
  const run = async () => {
    setBusy(true)
    try { setChecks((await api.post<{ checks: Check[] }>('/p2p/check')).checks) } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const failing = checks?.filter((c) => c.state === 'fail').length ?? 0
  return (
    <div className="cc">
      <div className="rem-actions">
        <button type="button" className="btn sm" disabled={busy} onClick={run}>{busy ? t('Checking…') : t('Check connection')}</button>
        {!running && <span className="muted">{t('Peer-to-peer connections haven’t started.')}</span>}
      </div>
      {checks && <>
        <p className="cc-sum" role="status">{failing ? tn('{count} thing stops devices reaching this server from away.', '{count} things stop devices reaching this server from away.', failing) : t('Nothing stops devices reaching this server from away.')}</p>
        <ul className="rem-check cc-list" aria-label={t('Check connection')}>
          {checks.map((c) => (
            <li key={c.id} className={c.state}>
              <span className="cc-row"><b>{tx(c.title)}</b><span className={`cc-state ${c.state}`}>{t(STATE_WORD[c.state])}</span></span>
              <span>{tx(c.detail)}</span>
              {c.next && <span className="cc-next">{t('Next: {what}', { what: tx(c.next) })}</span>}
            </li>
          ))}
        </ul>
      </>}
    </div>
  )
}

const when = (at: number) => {
  const d = new Date(at * 1000)
  const s = Math.max(0, Math.floor(Date.now() / 1000) - at)
  if (s < 60) return t('just now')
  if (s < 3600) return tn('{count} minute ago', '{count} minutes ago', Math.round(s / 60))
  if (s < 86400) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return d.toLocaleString([], { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
}

export function RecentTries() {
  const [list, setList] = useState<Attempt[] | null>(null)
  const [steps, setSteps] = useState<StepInfo[]>([])
  const load = useCallback(() => api.get<{ attempts: Attempt[]; steps: StepInfo[] }>('/p2p/attempts').then((r) => { setList(r.attempts); setSteps(r.steps) }).catch(() => {}), [])
  useEffect(() => { load() }, [load])
  useEffect(() => {
    const timer = setInterval(() => { if (!document.hidden) load() }, 15000)
    return () => clearInterval(timer)
  }, [load])
  if (!list) return null
  const groups = byDevice(list)
  return (
    <div className="cc-tries">
      <div className="cc-row">
        <h4 className="p2p-head">{t('Recent tries')}</h4>
        <button type="button" className="btn sm ghost" onClick={load}>{t('Refresh')}</button>
      </div>
      <p className="muted cc-note">{t('Each device’s last tries at connecting from away since this server started, and why any were refused. Kept on this server only.')}</p>
      {!groups.length && <p className="muted">{t('No device has tried to connect from away yet.')}</p>}
      {groups.map((g) => {
        const last = g.tries[0]
        return (
          <details key={g.device} className="cc-device" open={!last.ok && groups.length <= 3}>
            <summary>
              <span className={`cc-dot ${last.ok ? 'ok' : 'fail'}`} aria-hidden="true" />
              <b>{g.name || t('Unknown device')}</b>
              <span className="muted"> · {g.listed ? t('paired') : t('not paired')} · <code>{last.short}</code></span>
              <span className="cc-when">{when(last.at)}</span>
            </summary>
            <ol className="cc-tryl">
              {g.tries.map((a, i) => (
                <li key={i} className={a.ok ? 'ok' : 'fail'}>
                  <span className="cc-row">
                    <b>{t(KIND_WORD[a.kind])}: {a.ok ? t('got through') : t('refused')}{a.count > 1 ? ` (${tn('{count} time', '{count} times', a.count)})` : ''}</b>
                    <span className="cc-when">{when(a.at)}</span>
                  </span>
                  {a.ok
                    ? (a.path ? <span>{t('Connected {how}.', { how: t(PATH_WORD[a.path] || a.path) })}</span> : null)
                    : <>
                        <StepTicks steps={steps} failed={a.step} />
                        <span>{tx(a.reason || '')}</span>
                      </>}
                </li>
              ))}
            </ol>
          </details>
        )
      })}
    </div>
  )
}

// The steps as ticks up to the one that failed, a cross on it, and the rest not reached.
export function StepTicks({ steps, failed }: { steps: StepInfo[]; failed?: string }) {
  const states = stepStates(steps, failed)
  if (!states) return null
  return (
    <ol className="cc-steps" aria-label={t('Steps')}>
      {steps.map((s, i) => {
        const state = states[i]
        const word = state === 'ok' ? t('done') : state === 'fail' ? t('failed') : t('not reached')
        return <li key={s.id} className={state}><span aria-hidden="true">{state === 'ok' ? '✓' : state === 'fail' ? '✕' : '·'}</span> {tx(s.title)}<span className="sr-only"> ({word})</span></li>
      })}
    </ol>
  )
}
