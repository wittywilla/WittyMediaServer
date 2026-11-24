// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState } from 'react'
import { ApiError } from '../api'
import { Button } from '../ui/Button'
import { Field, Input } from '../ui/Field'
import { Dialog } from '../ui/Overlay'
import { t } from '../i18n'

// The parents' PIN, asked for when the server says leaving a kids' profile needs it (switching to another, signing
// out, the sign-in screen on a device a kids' profile is kept on). The server decides and counts wrong tries; this
// only asks, sends it again with the PIN, and says when it was wrong. Mounted once beside AskHost.
//
//   const r = await withParentPin((pin) => api.post('/logout', pin ? { pin } : {}))
//   if (r.cancelled) return

export type PinOutcome<T> = { cancelled: true } | { cancelled: false; value: T }

type Pending = { call: (pin: string) => Promise<unknown>; resolve: (v: PinOutcome<unknown>) => void; reject: (e: unknown) => void }
let openPin: ((p: Pending) => void) | null = null

/** Whether an error is the server asking for the parents' PIN. */
export const needsParentPin = (e: unknown) => e instanceof ApiError && e.status === 403 && e.data?.code === 'parent-pin'

/** Runs call; when the server answers that it needs the parents' PIN, asks for it and runs call again with it. Other
 *  errors are thrown as they come. */
export async function withParentPin<T>(call: (pin?: string) => Promise<T>): Promise<PinOutcome<T>> {
  try {
    return { cancelled: false, value: await call() }
  } catch (e) {
    if (!needsParentPin(e) || !openPin) throw e
    const open = openPin
    return new Promise<PinOutcome<T>>((resolve, reject) => open({ call, resolve: resolve as (v: PinOutcome<unknown>) => void, reject }))
  }
}

// "Limits, for a parent": a parent changes a kids' profile's screen time and age on the child's own device.
// Loaded when it is opened: most devices never show it.
const ParentLimits = lazy(() => import('../pages/settings/ParentLimits'))
let openLimits: (() => void) | null = null
export const openParentLimits = () => openLimits?.()

export function ParentLimitsHost() {
  const [open, setOpen] = useState(false)
  useEffect(() => { openLimits = () => setOpen(true); return () => { openLimits = null } }, [])
  return open ? <Suspense fallback={null}><ParentLimits onClose={() => setOpen(false)} /></Suspense> : null
}

export function ParentPinHost() {
  const [cur, setCur] = useState<Pending | null>(null)
  const [pin, setPin] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    openPin = (p) => setCur((was) => { was?.resolve({ cancelled: true }); setPin(''); setErr(''); return p })
    return () => { openPin = null }
  }, [])
  if (!cur) return null
  const cancel = () => { cur.resolve({ cancelled: true }); setCur(null) }
  const send = async () => {
    if (pin.length < 4 || busy) return
    setBusy(true); setErr('')
    try {
      const value = await cur.call(pin)
      cur.resolve({ cancelled: false, value }); setCur(null)
    } catch (e) {
      if (e instanceof ApiError && (e.data?.code === 'wrong-pin' || needsParentPin(e) || e.status === 429)) { setErr(e.message); setPin('') }
      else { cur.reject(e); setCur(null) }
    } finally { setBusy(false) }
  }
  return (
    <Dialog title={t('A parent’s PIN')} size="sm" onClose={cancel}
      description={t('This profile stays on this device until a parent types the parents’ PIN.')}
      actions={<><Button variant="ghost" onClick={cancel}>{t('Cancel')}</Button><Button variant="primary" busy={busy} disabled={pin.length < 4} onClick={send}>{t('OK')}</Button></>}>
      <form onSubmit={(e) => { e.preventDefault(); void send() }}>
        <Field label={t('The parents’ PIN')} error={err || undefined}>
          <Input autoFocus type="password" inputMode="numeric" autoComplete="off" value={pin} maxLength={12} onChange={(e) => setPin(e.target.value.replace(/\D/g, '').slice(0, 12))} />
        </Field>
        <button type="submit" hidden aria-hidden="true" tabIndex={-1} />
      </form>
    </Dialog>
  )
}
