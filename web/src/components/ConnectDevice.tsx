// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { IconCheck, IconInfo } from '../icons'
import { toast } from '../state'
import { Button, Dialog } from '../ui'
import QrCode from '../pages/settings/QrCode'
import { ConfirmOwner, type OwnerAsk } from '../pages/settings/ConfirmOwner'
import { t } from '../i18n'
import './connect-device.css'

// Connecting another device (Settings, Remote Access; From away): one screen, one way. It says first who can
// connect, then makes a code for one device, with a QR code holding the same way in as a link: the code for
// a device on the same network, the QR code or the link for one away from home. Whichever is used first
// ends the other (api_connect.go).
// Nothing technical here: the details are further down the Remote Access page.

type Status = { away: 'off' | 'checking' | 'yes' | 'partly' | 'no'; words: string; detail?: string; p2p: boolean; canTurnOn: boolean; other?: string }
type Offer = { code: string; link?: string; qrLink?: string; expiresInSec: number; away: boolean; homeAddresses: string[]; until: number }
type Party = { layer: string; who: string; sees: string }

function Copy({ value, label }: { value: string; label: string }) {
  const copy = async () => {
    try { await navigator.clipboard.writeText(value); toast(t('Copied')) } catch { toast(t('Select it and copy it by hand')) }
  }
  return <Button size="sm" variant="ghost" onClick={() => void copy()} aria-label={label}>{t('Copy')}</Button>
}

function left(until: number, now: number) {
  const s = Math.max(0, Math.round((until - now) / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// titled: false where the page's own title already says Connect a device (From away), so it isn't said twice.
export default function ConnectDevice({ titled = true }: { titled?: boolean } = {}) {
  const [st, setSt] = useState<Status | null>(null)
  const [offer, setOffer] = useState<Offer | null>(null)
  const [busy, setBusy] = useState(false)
  const [owner, setOwner] = useState<OwnerAsk | null>(null)
  const [turnOn, setTurnOn] = useState(false)
  const [now, setNow] = useState(Date.now())
  const load = useCallback(() => api.get<Status>('/connect').then(setSt).catch(() => {}), [])
  useEffect(() => {
    load()
    const timer = setInterval(() => { if (!document.hidden) load() }, 15000)
    return () => clearInterval(timer)
  }, [load])
  useEffect(() => {
    if (!offer) return
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [offer])
  // A code that ran out is gone: the screen goes back to its button.
  useEffect(() => { if (offer && now >= offer.until) setOffer(null) }, [offer, now])

  const got = (o: Omit<Offer, 'until'>) => { setNow(Date.now()); setOffer({ ...o, until: Date.now() + o.expiresInSec * 1000 }) }
  const make = async () => {
    setBusy(true)
    try { got(await api.post<Omit<Offer, 'until'>>('/connect/offer', {})) } catch (e) {
      // A device signed in as an administrator can change everything: that takes the password again.
      if (e instanceof ApiError && e.status === 403 && (e.data as { reason?: string } | undefined)?.reason === 'password-required') {
        setOwner({
          title: t('Connect a device as you?'),
          body: <p>{t('The device will be signed in as you, an administrator. Only connect a device of your own.')}</p>,
          action: t('Show the code'),
          run: async (password, otp) => { got(await api.post<Omit<Offer, 'until'>>('/connect/offer', { password, otp })) },
        })
      } else toast((e as Error).message)
    } finally { setBusy(false) }
  }
  const done = async () => {
    setOffer(null)
    try { await api.del('/connect/offer') } catch { /* it runs out by itself */ }
  }

  // Not plain ok and bad: those are app-wide classes that colour all the text.
  const level = !st ? 'cd-wait' : st.away === 'yes' ? 'cd-ok' : st.away === 'off' ? 'cd-off' : st.away === 'no' ? 'cd-bad' : 'cd-wait'
  return (
    <section className="set-card cd" {...(titled ? { 'aria-labelledby': 'cd-title' } : { 'aria-label': t('Connect a device') })}>
      {titled && <h2 id="cd-title">{t('Connect a device')}</h2>}
      <ul className="cd-who" aria-live="polite">
        <li className="cd-ok"><span aria-hidden="true"><IconCheck size={16} /></span>{t('Devices on your home network can connect.')}</li>
        {st && st.away !== 'off' && <li className={level}><span aria-hidden="true">{level === 'cd-ok' ? <IconCheck size={16} /> : <IconInfo size={16} />}</span>{st.words}</li>}
        {st && st.away === 'off' && <li className="cd-off"><span aria-hidden="true"><IconInfo size={16} /></span>{t('Devices away from home can’t connect yet.')}</li>}
      </ul>
      {st?.away === 'off' && st.canTurnOn && !offer && (
        <p className="cd-turn">
          <Button onClick={() => setTurnOn(true)}>{t('Let devices connect from away…')}</Button>
        </p>
      )}

      {!offer && (
        <div className="cd-start">
          <p>{t('This shows a code. Type it on the other device, or scan the QR code with it, and it signs in as you.')}</p>
          <Button variant="primary" busy={busy} onClick={() => void make()}>{t('Show a code')}</Button>
        </div>
      )}

      {offer && (
        <div className="cd-offer">
          <div className="cd-steps">
            <p className="cd-label" id="cd-code-label">{t('Code')}</p>
            <p className="cd-code" aria-labelledby="cd-code-label" data-code={offer.code}>{offer.code}</p>
            <ol>
              <li>{t('On the other device, open WittyMediaClient and choose Add a server.')}</li>
              <li>{t('On your home network: choose Enter a code, and type the code.')}</li>
              {offer.link && <li>{t('Away from home: scan the QR code with the device’s camera, or send it the link under More options and choose Paste a link.')}</li>}
            </ol>
            {!offer.away && <p className="cd-note">{st?.canTurnOn ? t('This code works on your home network. For a device away from home, let devices connect from away first.') : t('This code works on your home network.')}</p>}
            <p className="cd-time" role="timer" aria-live="off">{t('Works once, for {time}.', { time: left(offer.until, now) })}</p>
            <div className="cd-actions"><Button onClick={() => void done()}>{t('Done')}</Button></div>
          </div>
          {offer.link && <div className="cd-qr"><QrCode text={offer.qrLink || offer.link} label={t('QR code to connect a device')} size={220} /></div>}
          <details className="cd-more">
            <summary>{t('More options')}</summary>
            {offer.link && (
              <div className="cd-opt">
                <p><b>{t('The link')}</b> {t('(the same way in as the QR code). Send it to the device, then paste it in Add a server, Paste a link. Treat it like a password until it’s used.')}</p>
                <p className="cd-line"><code className="cd-link">{offer.link}</code><Copy value={offer.link} label={t('Copy the link')} /></p>
              </div>
            )}
            {offer.homeAddresses.length > 0 && (
              <div className="cd-opt">
                <p><b>{t('This server’s address at home')}</b> {t('for Add a server, Type an address. Then sign in with your password, or with the code.')}</p>
                {offer.homeAddresses.map((a) => <p key={a} className="cd-line"><code>{a}</code><Copy value={a} label={t('Copy {value}', { value: a })} /></p>)}
              </div>
            )}
          </details>
        </div>
      )}
      {owner && <ConfirmOwner ask={owner} onClose={() => setOwner(null)} />}
      {turnOn && <TurnOnAway onClose={() => setTurnOn(false)} onDone={() => { setTurnOn(false); load() }} />}
    </section>
  )
}

// Connections from away contact computers outside the home, which nothing else does unasked. Who they are is
// said before it is turned on, and the person agrees to it.
function TurnOnAway({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [parties, setParties] = useState<Party[] | null>(null)
  const [accept, setAccept] = useState(false)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<{ thirdParties: Party[] }>('/p2p/settings').then((v) => setParties(v.thirdParties)).catch((e) => toast(e.message)) }, [])
  const go = async () => {
    setBusy(true)
    try {
      const v = await api.put<{ error?: string }>('/p2p/settings', { enabled: true, accept: true })
      if (v.error) toast(v.error)
      else toast(t('Devices can now connect from away'))
      onDone()
    } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  return (
    <Dialog size="md" title={t('Let devices connect from away?')} onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>{t('Not now')}</Button>
        <Button variant="primary" busy={busy} disabled={!accept} onClick={() => void go()}>{t('Turn it on')}</Button>
      </>}>
      <p>{t('Your devices then reach this server from anywhere, with nothing to set up on your router. What passes between them is encrypted so that only this server and your device can read it, and only devices you connect here can use it.')}</p>
      <p>{t('To find each other, this server talks to computers outside your home. None of them sees your media, your passwords or what you watch:')}</p>
      <ul className="cd-parties">{(parties || []).map((p) => <li key={p.layer}><b>{p.who}</b> <span>{t('They see: {what}', { what: p.sees })}</span></li>)}</ul>
      <label className="check-row rem-accept"><input type="checkbox" checked={accept} onChange={(e) => setAccept(e.target.checked)} /> <span>{t('I’ve read who is contacted, and I want this on.')}</span></label>
    </Dialog>
  )
}
