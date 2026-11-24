// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, type MyLimits, type ScreenTime } from '../api'
import { useApp } from '../state'
import { publishLimits } from '../myLimits'
import { Button } from '../ui/Button'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { askParentSpec } from '../spec/specs/screentime'
import { Dialog } from '../ui/Overlay'
import { useSwitchProfile } from './SwitchProfile'
import { StandaloneCard } from '../components/StandaloneCard'
import './switchprofile.css'
import { date, t, tn, tx } from '../i18n'

// Screen time, for a profile that has time rules. The server does the deciding and the stopping:
// it refuses to play when the time is up and ends what is being sent. This only tells the person
// why, a few minutes ahead and when it happens, and lets a parent give more on the spot.
// A profile with no time rules pays one request when it signs in. A parent can change the rules
// while the profile is signed in, so they are read again, at most every few minutes and only
// while the page is in use: when it comes back into view, and as the person moves around.

const minutes = (sec: number) => {
  const m = Math.max(1, Math.round(sec / 60))
  return tn('{count} minute', '{count} minutes', m)
}

const clock = (unix: number) => date(unix * 1000, { hour: 'numeric', minute: '2-digit' })

// When playing may start again, in words: "at 7:00" today, "tomorrow at 7:00" otherwise.
function nextText(st: ScreenTime) {
  if (!st.nextAt) return ''
  const next = new Date(st.nextAt * 1000), now = new Date()
  return next.toDateString() === now.toDateString() ? t('You can watch again at {time}.', { time: clock(st.nextAt) }) : t('You can watch again tomorrow at {time}.', { time: clock(st.nextAt) })
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
  const more = (next: ScreenTime) => { was.current = next.allowed; setSt(next); if (next.allowed) setTelling(false) }
  // A kids' profile gets a page of its own, friendly and plain; others a dialog over what they were doing.
  if (telling && !st.allowed && user.kind === 'kids') return <KidsTimeUp st={st} pin={!!limits?.parentPin} onClose={close} onMore={more} />
  if (telling && !st.allowed) return <TimeUp st={st} pin={!!limits?.parentPin} onClose={close} onMore={more} />
  if (st.allowed && st.remainingSec >= 0 && st.remainingSec <= 600) {
    return <div className="scr-left" role="status">{t('{time} of screen time left', { time: minutes(st.remainingSec) })}</div>
  }
  return null
}

// The words of the time's-up page, by why playing stopped.
function upWords(st: ScreenTime) {
  const when = nextText(st)
  switch (st.reason) {
    case 'bedtime': return { title: t('Time for bed'), lead: [t('It’s past bedtime for this profile.'), when].filter(Boolean).join(' ') }
    case 'early': return { title: t('It’s a bit early'), lead: when || t('It’s too early for this profile to watch.') }
    default: return { title: t('That’s all for today'), lead: [t('Today’s screen time is used up.'), when].filter(Boolean).join(' ') }
  }
}

// Asking a parent for more time: how much, and the parents' PIN or a parent's own name and password.
function useAskParent(pin: boolean, onMore: (st: ScreenTime) => void) {
  const [more, setMore] = useState('30')
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [byPassword, setByPassword] = useState(!pin)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const give = async () => {
    setErr(''); setBusy(true)
    try {
      const body = byPassword ? { minutes: Number(more), name, password } : { minutes: Number(more), pin: code }
      onMore(await api.post<ScreenTime>('/me/screen-time/extra', body))
    } catch (e) { setErr((e as Error).message); setCode(''); setPassword('') } finally { setBusy(false) }
  }
  const page = askParentSpec({ more, byPassword, hasPin: pin, name, password, code, error: err })
  const ready = byPassword ? !!name.trim() && !!password : code.length >= 4
  const onAction = (id: string) => { if (id.startsWith('more:')) setMore(id.slice(5)); else if (id === 'switch') { setByPassword(!byPassword); setErr('') } }
  const onField = (id: string, v: string | number | boolean | File) => { if (id === 'code') setCode(String(v).replace(/\D/g, '').slice(0, 12)); else if (id === 'name') setName(String(v)); else if (id === 'password') setPassword(String(v)) }
  return { page, handled: handledIds(page), ready, busy, give, onAction, onField, reset: () => setErr('') }
}

// A kids' profile's time's-up page: one card in the middle of the screen with the server's logo and name, a
// friendly heading and one sentence, that fits a TV, a computer and a phone without scrolling (the standalone page
// standard, components/StandaloneCard.tsx). It covers whatever was on screen, which has stopped. OK goes back to
// browsing: nothing plays until a parent gives more time or the next day starts.
function KidsTimeUp({ st, pin, onClose, onMore }: { st: ScreenTime; pin: boolean; onClose: () => void; onMore: (st: ScreenTime) => void }) {
  const [asking, setAsking] = useState(false)
  const ask = useAskParent(pin, onMore)
  const sw = useSwitchProfile()
  const words = upWords(st)
  // A video in the browser's own full screen would sit over the page.
  useEffect(() => { if (document.fullscreenElement) document.exitFullscreen?.().catch(() => {}) }, [])
  return (
    <div className="scr-page" role="alertdialog" aria-modal="true" aria-labelledby="scr-page-title">
      <StandaloneCard className="scr-card">
        {asking ? (
          <form className="scr-ask" onSubmit={(e) => { e.preventDefault(); if (ask.ready && !ask.busy) void ask.give() }}>
            <h1 id="scr-page-title">{t('Ask a parent')}</h1>
            <SpecPage embedded page={ask.page} handled={ask.handled} onAction={ask.onAction} onField={ask.onField} />
            <div className="sa-actions">
              <Button variant="primary" busy={ask.busy} disabled={!ask.ready} type="submit">{t('Give more time')}</Button>
              <button type="button" className="sa-text-btn" onClick={() => { setAsking(false); ask.reset() }}>{t('Back')}</button>
            </div>
          </form>
        ) : (
          <>
            <h1 id="scr-page-title">{words.title}</h1>
            <p className="sa-lead">{words.lead}</p>
            <div className="sa-actions">
              <Button variant="primary" autoFocus onClick={onClose}>{t('OK')}</Button>
              <button type="button" className="sa-text-btn" onClick={() => setAsking(true)}>{t('Ask a parent')}</button>
              <button type="button" className="sa-text-btn" onClick={sw.start}>{t('Switch profile')}</button>
            </div>
          </>
        )}
      </StandaloneCard>
      {sw.dialog}
    </div>
  )
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
  const title = st.reason === 'bedtime' ? t('That’s all for tonight') : st.reason === 'early' ? t('It’s a bit early') : t('Screen time is up')
  const give = async () => {
    setErr(''); setBusy(true)
    try {
      const body = byPassword ? { minutes: Number(more), name, password } : { minutes: Number(more), pin: code }
      onMore(await api.post<ScreenTime>('/me/screen-time/extra', body))
    } catch (e) { setErr((e as Error).message); setCode(''); setPassword('') } finally { setBusy(false) }
  }
  const page = askParentSpec({ more, byPassword, hasPin: pin, name, password, code, error: err })
  const handled = handledIds(page)
  const ready = byPassword ? !!name.trim() && !!password : code.length >= 4
  if (asking) {
    return (
      <Dialog title={t('Ask a parent')} size="sm" onClose={onClose}
        actions={<><Button variant="ghost" onClick={() => { setAsking(false); setErr('') }}>{t('Back')}</Button><Button variant="primary" busy={busy} disabled={!ready} onClick={give}>{t('Give more time')}</Button></>}>
        <form className="scr-ask" onSubmit={(e) => { e.preventDefault(); if (ready && !busy) give() }}>
          <SpecPage embedded page={page} handled={handled} onAction={(id) => { if (id.startsWith('more:')) setMore(id.slice(5)); else if (id === 'switch') { setByPassword(!byPassword); setErr('') } }}
            onField={(id, v) => { if (id === 'code') setCode(String(v).replace(/\D/g, '').slice(0, 12)); else if (id === 'name') setName(String(v)); else if (id === 'password') setPassword(String(v)) }} />
          <button type="submit" hidden aria-hidden="true" tabIndex={-1} />
        </form>
      </Dialog>
    )
  }
  return (
    <>
      <Dialog title={title} size="sm" alert onClose={onClose}
        actions={<><Button variant="ghost" onClick={sw.start}>{t('Switch profile')}</Button><Button onClick={() => setAsking(true)}>{t('Ask a parent')}</Button><Button variant="primary" onClick={onClose}>{t('OK')}</Button></>}>
        <p>{tx(st.message) || t('This profile can’t watch any more right now.')}</p>
        <p className="scr-when">{nextText(st)}</p>
      </Dialog>
      {sw.dialog}
    </>
  )
}
