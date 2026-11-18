// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api, getPrefs } from '../../../api'
import { IconShield } from '../../../icons'
import { toast } from '../../../state'
import { Row, Section, Switch } from '../ui'
import type { LogsView } from './types'
import { t, tk, tn, tx } from '../../../i18n'

// Labels are shown with tx().
export const LEVELS: { v: string; label: string }[] = [
  { v: 'off', label: tk('Off') },
  { v: 'error', label: tk('Errors only') },
  { v: 'warn', label: tk('Warnings and errors') },
  { v: 'info', label: tk('Normal') },
  { v: 'debug', label: tk('Everything') },
]

export function PrivacyNote() {
  return (
    <div className="lg-privacy" role="note">
      <IconShield />
      <p><b>{t('Your logs stay with you.')}</b> {t('They are written only to this server’s own disk. They are never uploaded, and nobody, including the people who make this software, sees them unless you choose to send a file yourself.')}</p>
    </div>
  )
}

// Each area's level takes effect at once, with no restart.
export default function WhatToLog({ view, onView }: { view: LogsView; onView: (v: LogsView) => void }) {
  const [busy, setBusy] = useState('')
  const [minutes, setMinutes] = useState(30)
  const change = async (id: string, label: string, level: string) => {
    setBusy(id)
    try {
      onView(await api.put<LogsView>('/logs', { levels: { [id]: level } }))
      toast(t('{area}: {level}', { area: tx(label), level: tx(LEVELS.find((l) => l.v === level)?.label) || level }))
      if (id === 'clients') void getPrefs(true) // this app stops or starts reporting
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const keep = async (what: 'flight' | 'trace', on: boolean) => {
    setBusy(what)
    try { onView(await api.put<LogsView>('/logs', { [what]: on })) } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  const record = async () => {
    setBusy('record')
    try { onView(await api.put<LogsView>('/logs', { recordMinutes: minutes })); toast(t('Recording everything')) } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }
  return (
    <>
      <PrivacyNote />
      <Section title={tk('Catch a problem')} desc={t('Records everything for a while, in every area and module, including the titles of what is played. Then try to make the problem happen. It stops by itself, and your settings below aren’t changed.')}>
        <Row label={t('How long')}>
          <select className="select" value={minutes} disabled={!!view.recordingUntil} onChange={(e) => setMinutes(Number(e.target.value))}>
            {[10, 30, 60, 120].map((m) => <option key={m} value={m}>{m < 60 ? tn('{count} minute', '{count} minutes', m) : tn('{count} hour', '{count} hours', m / 60)}</option>)}
          </select>
          <button type="button" className="btn primary" disabled={busy === 'record' || !!view.recordingUntil} onClick={() => void record()}>{view.recordingUntil ? t('Recording…') : t('Record everything')}</button>
        </Row>
      </Section>
      <Section title={tk('Keep the last minutes')} desc={t('Kept in memory only, and written down when the server stops answering, when part of it crashes, or when you press Save what just happened. Nothing is written otherwise.')}>
        <Row label={t('The last 10 minutes, in full detail')} desc={t('Everything every area and module says, whatever its level below, including the titles of what is played. Up to 16 MB of memory.')}>
          <Switch checked={view.flight} disabled={busy === 'flight'} label={t('Keep the last 10 minutes')} onChange={(v) => void keep('flight', v)} />
        </Row>
        <Row label={t('What the program was doing')} desc={t('Go’s execution trace of the last seconds, for a developer to read with go tool trace. It holds places in the program and timings, nothing of yours, and costs a little speed.')}>
          <Switch checked={view.trace} disabled={busy === 'trace'} label={t('Keep an execution trace')} onChange={(v) => void keep('trace', v)} />
        </Row>
      </Section>
      <Section title={tk('What to log')} desc={t('How much each part of the server writes down. Changes work straight away, with no restart. Off still records crashes. Modules can have a level of their own, under Modules.')}>
        {view.categories.map((c) => (
          <Row key={c.id} label={tx(c.label)} desc={tx(c.description)}>
            <select className="select" value={view.levels[c.id] || c.default} disabled={busy === c.id} onChange={(e) => void change(c.id, c.label, e.target.value)}>
              {LEVELS.map((l) => <option key={l.v} value={l.v}>{l.v === c.default ? t('{level} (default)', { level: tx(l.label) }) : tx(l.label)}</option>)}
            </select>
          </Row>
        ))}
      </Section>
    </>
  )
}
