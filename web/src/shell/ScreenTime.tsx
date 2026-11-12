// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, type MyLimits, type ScreenTime } from '../api'
import { useApp } from '../state'
import { publishLimits } from '../myLimits'
import { Button } from '../ui/Button'
import { Segmented } from '../ui/Choice'
import { Field, Input } from '../ui/Field'
import { Dialog } from '../ui/Overlay'
import { useSwitchProfile } from './SwitchProfile'
import './switchprofile.css'

// Screen time, for a profile that has time rules. The server does the deciding and the stopping:
// it refuses to play when the time is up and ends what is being sent. This only tells the person
// why, a few minutes ahead and when it happens, and lets a parent give more on the spot.
// A profile with no time rules pays one request when it signs in. A parent can change the rules
// while the profile is signed in, so they are read again, at most every few minutes and only
// while the page is in use: when it comes back into view, and as the person moves around.

const minutes = (sec: number) => {
  const m = Math.max(1, Math.round(sec / 60))
  return m === 1 ? '1 minute' : `${m} minutes`
}

const clock = (unix: number) => new Date(unix * 1000).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })

// When playing may start again, in words: "at 7:00" today, "tomorrow at 7:00" otherwise.
function nextText(st: ScreenTime) {
  if (!st.nextAt) return ''
  const next = new Date(st.nextAt * 1000), now = new Date()
  return next.toDateString() === now.toDateString() ? `You can watch again at ${clock(st.nextAt)}.` : `You can watch again tomorrow at ${clock(st.nextAt)}.`
}

// How long the rules read last are trusted for: when the page comes back into view, and while
// the person is moving around in it.
const LIMITS_ON_RETURN_MS = 60_000
const LIMITS_WHILE_BROWSING_MS = 5 * 60_000

export default function ScreenTimeWatch() {
  const { user } = useApp()
  const [limits, setLimits] = useState<MyLimits | null>(null)
  const [st, setSt] = useState<ScreenTime | null>(null)
  const [telling, setTelling] = useState(false)
  const was = useRef(true)
  const nav = useNavigate()
  const loc = useLocation()
  const timed = !!limits?.time

  const stopPlaying = () => document.querySelectorAll<HTMLMediaElement>('video, audio').forEach((m) => { try { m.pause() } catch { /* nothing to stop */ } })

  const take = useCallback((next: ScreenTime) => {
    setSt(next)
    if (!next.allowed && was.current) { stopPlaying(); setTelling(true) }
    was.current = next.allowed
  }, [])

  // Reads the rules, for the profile that is signed in now. With rules, the time left comes with
  // them; without (any more), whatever this was showing goes.
  const who = useRef(0)
  const lastRead = useRef(0)
  const readLimits = useCallback(() => {
    const id = who.current
    if (!id) return
    lastRead.current = Date.now()
    api.get<MyLimits>('/me/limits').then((l) => {
      if (who.current !== id) return
      setLimits(l); publishLimits(l)
      if (l.time) take(l.screenTime)
      else if (l.screenTime.allowed) { setSt(null); setTelling(false); was.current = true }
    }).catch(() => {})
  }, [take])

  useEffect(() => {
    setLimits(null); publishLimits(null); setSt(null); setTelling(false); was.current = true
    who.current = user && !user.isAdmin ? user.id : 0
    readLimits()
    return () => { who.current = 0 }
  }, [user?.id, readLimits]) // eslint-disable-line react-hooks/exhaustive-deps

  // Have the rules changed? Asked when the page comes back into view (a minute apart at most),
  // and every few minutes as the person goes from page to page.
  useEffect(() => {
    const back = () => { if (!document.hidden && Date.now() - lastRead.current > LIMITS_ON_RETURN_MS) readLimits() }
    document.addEventListener('visibilitychange', back)
    return () => document.removeEventListener('visibilitychange', back)
  }, [readLimits])
  useEffect(() => { if (Date.now() - lastRead.current > LIMITS_WHILE_BROWSING_MS) readLimits() }, [loc.pathname, readLimits])

  // Ask again each minute while this page is showing, and at the moment the time runs out.
  useEffect(() => {
    if (!timed) return
    const ask = () => { if (!document.hidden) api.get<ScreenTime>('/me/screen-time').then(take).catch(() => {}) }
    const every = window.setInterval(ask, 60_000)
    const untilMs = st?.allowed && st.until ? st.until * 1000 - Date.now() + 1500 : 0
    const at = untilMs > 0 && untilMs < 3_600_000 ? window.setTimeout(ask, untilMs) : 0
    document.addEventListener('visibilitychange', ask)
    return () => { window.clearInterval(every); if (at) window.clearTimeout(at); document.removeEventListener('visibilitychange', ask) }
  }, [timed, st?.until, st?.allowed, take])

  // The server refused to play something: say why, whatever page asked.
  useEffect(() => {
    // The rules may be newer than the ones read at sign-in (time rules set since, a parents' PIN
    // made since): read them again, so the notice offers what is true now.
    const refused = (e: Event) => { const d = (e as CustomEvent<ScreenTime>).detail; if (d) { setSt(d); was.current = false } stopPlaying(); setTelling(true); readLimits() }
    window.addEventListener('wms:screen-time', refused)
    return () => window.removeEventListener('wms:screen-time', refused)
  }, [readLimits])

  if (!user || !st) return null
  const close = () => {
    setTelling(false)
    // Whatever was playing can't go on: leave the player for the page it came from.
    if (!st.allowed && /^\/(watch|play|game|big|tv\/watch)\//.test(loc.pathname)) nav('/', { replace: true })
  }
  if (telling && !st.allowed) return <TimeUp st={st} pin={!!limits?.parentPin} onClose={close} onMore={(next) => { was.current = next.allowed; setSt(next); if (next.allowed) setTelling(false) }} />
  if (st.allowed && st.remainingSec >= 0 && st.remainingSec <= 600) {
    return <div className="st-left" role="status">{minutes(st.remainingSec)} of screen time left</div>
  }
  return null
}

function TimeUp({ st, pin, onClose, onMore }: { st: ScreenTime; pin: boolean; onClose: () => void; onMore: (st: ScreenTime) => void }) {
  const [asking, setAsking] = useState(false)
  const [more, setMore] = useState('30')
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [byPassword, setByPassword] = useState(!pin)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const sw = useSwitchProfile()
  const title = st.reason === 'bedtime' ? 'That’s all for tonight' : st.reason === 'early' ? 'It’s a bit early' : 'Screen time is up'
  const give = async () => {
    setErr(''); setBusy(true)
    try {
      const body = byPassword ? { minutes: Number(more), name, password } : { minutes: Number(more), pin: code }
      onMore(await api.post<ScreenTime>('/me/screen-time/extra', body))
    } catch (e) { setErr((e as Error).message); setCode(''); setPassword('') } finally { setBusy(false) }
  }
  const ready = byPassword ? !!name.trim() && !!password : code.length >= 4
  if (asking) {
    return (
      <Dialog title="Ask a parent" size="sm" onClose={onClose}
        actions={<><Button variant="ghost" onClick={() => { setAsking(false); setErr('') }}>Back</Button><Button variant="primary" busy={busy} disabled={!ready} onClick={give}>Give more time</Button></>}>
        <form className="form full st-ask" onSubmit={(e) => { e.preventDefault(); if (ready && !busy) give() }}>
          <p className="st-when">A parent can give this profile more time today.</p>
          <Segmented label="How much more" value={more} onChange={setMore} options={[{ value: '15', label: '15 minutes' }, { value: '30', label: '30 minutes' }, { value: '60', label: '1 hour' }]} />
          {byPassword ? <>
            <Field label="A parent’s username"><Input autoFocus={!pin} autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <Field label="Their password" error={err || undefined}><Input type="password" autoComplete="off" value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
          </> : <Field label="The parents’ PIN" error={err || undefined}><Input autoFocus type="password" inputMode="numeric" autoComplete="off" value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 12))} /></Field>}
          {pin && <Button variant="ghost" size="sm" onClick={() => { setByPassword(!byPassword); setErr('') }}>{byPassword ? 'Use the parents’ PIN instead' : 'Use a parent’s password instead'}</Button>}
          <button type="submit" hidden aria-hidden="true" tabIndex={-1} />
        </form>
      </Dialog>
    )
  }
  return (
    <>
      <Dialog title={title} size="sm" alert onClose={onClose}
        actions={<><Button variant="ghost" onClick={sw.start}>Switch profile</Button><Button onClick={() => setAsking(true)}>Ask a parent</Button><Button variant="primary" onClick={onClose}>OK</Button></>}>
        <p>{st.message || 'This profile can’t watch any more right now.'}</p>
        <p className="st-when">{nextText(st)}</p>
      </Dialog>
      {sw.dialog}
    </>
  )
}
