// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, type Access, type MyLimits, type TimeRules } from '../../api'
import { toast } from '../../state'
import { publishLimits } from '../../myLimits'
import { Button } from '../../ui/Button'
import { Field, Input } from '../../ui/Field'
import { Dialog } from '../../ui/Overlay'
import { AgePicker } from '../Profiles'
import { TimeRulesEditor } from './ParentalControls'
import { t } from '../../i18n'

// A parent, at a child's device, changes that profile's screen time, bedtime and age limit with the parents' PIN
// (or their own name and password when no PIN is set). The server checks it and counts wrong tries; everything else
// about the profile is changed in Settings, by whoever manages it (PUT /api/me/limits).
export default function ParentLimits({ onClose }: { onClose: () => void }) {
  const [mine, setMine] = useState<MyLimits | null>(null)
  const [a, setA] = useState<Partial<Access>>({})
  const [pin, setPin] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<MyLimits>('/me/limits').then((l) => { setMine(l); setA({ maxAge: l.maxAge, time: l.time }) }).catch((e) => setErr((e as Error).message))
  }, [])
  const byPin = !!mine?.parentPin
  const ready = byPin ? pin.length >= 4 : !!name.trim() && !!password
  const save = async () => {
    if (!ready || busy) return
    setErr(''); setBusy(true)
    try {
      const body = { ...(byPin ? { pin } : { name, password }), time: a.time || ({ schoolDayMinutes: 0, weekendMinutes: 0, from: '', schoolNightUntil: '', weekendUntil: '' } as TimeRules), maxAge: a.maxAge }
      const l = await api.put<MyLimits>('/me/limits', body)
      publishLimits(l)
      toast(t('Limits saved'), { tone: 'ok' })
      onClose()
    } catch (e) { setErr((e as Error).message); setPin(''); setPassword('') } finally { setBusy(false) }
  }
  const set = (p: Partial<Access>) => setA((x) => ({ ...x, ...p }))
  return (
    <Dialog title={t('Limits, for a parent')} size="wide" onClose={onClose}
      actions={<><Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button><Button variant="primary" busy={busy} disabled={!ready || !mine} onClick={save}>{t('Save limits')}</Button></>}>
      {!mine ? <div className="loading-box" role="status" aria-label={t('Loading')}><div className="spinner" /></div> : (
        <form className="limits" onSubmit={(e) => { e.preventDefault(); void save() }}>
          <p className="muted">{t('Changes this profile’s screen time and age limit. Everything else is in Settings, on a parent’s own device.')}</p>
          <TimeRulesEditor access={a as Access} set={set} />
          <AgePicker value={a.maxAge ?? 7} onChange={(n) => set({ maxAge: n })} />
          {byPin ? (
            <Field label={t('The parents’ PIN')} error={err || undefined}>
              <Input type="password" inputMode="numeric" autoComplete="off" value={pin} maxLength={12} onChange={(e) => setPin(e.target.value.replace(/\D/g, '').slice(0, 12))} />
            </Field>
          ) : <>
            <Field label={t('A parent’s username')}><Input autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <Field label={t('Their password')} error={err || undefined}><Input type="password" autoComplete="off" value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
          </>}
          <button type="submit" hidden aria-hidden="true" tabIndex={-1} />
        </form>
      )}
    </Dialog>
  )
}
