// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState } from 'react'
import { api } from '../../api'
import { PartNote } from '../../tvmode/OnAComputer'
import { useTvMode } from '../../tvmode/mode'
import { Button, Spinner } from '../../ui'
import { httpsWords, type HttpsStatus } from './network'
import { Fill, LaterNote, StepActions, StepHead, type StepProps } from './parts'
import { list, t, tn } from '../../i18n'
import '../settings/remote.css'

const PeerToPeer = lazy(() => import('../settings/PeerToPeer'))

// Step: Away from home. Done here, not in a list of links out of setup: peer-to-peer connections (the same section as
// Settings, Remote Access, with who is contacted said before it's turned on, and its connection test), whether
// connections are encrypted, and how an app on a phone or TV is added. None of it is needed to watch at home.
export function Remote({ info, next, back }: StepProps) {
  const tv = useTvMode()
  const [https, setHttps] = useState<HttpsStatus | null>(null)
  useEffect(() => { api.get<{ https?: HttpsStatus }>('/remote').then((r) => setHttps(r.https || null)).catch(() => {}) }, [])
  const here = location.origin
  return (
    <section>
      <StepHead title={t('Watch away from home')} lead={t('At home everything works already. To use your server on a phone or laptop somewhere else, turn on peer-to-peer connections: your device connects straight to this server, with nothing to change on your router. If a direct connection can’t be made, a relay passes it along, encrypted. It stays off unless you turn it on.')} />
      {tv
        ? <PartNote title={t('Remote Access')} path="/settings/remote" />
        : <div className="su-p2p"><Suspense fallback={<Spinner />}><PeerToPeer /></Suspense></div>}
      <h2 className="oobe-sub">{t('Encrypted connections')}</h2>
      <p className="su-https">{httpsWords(https, { t, tn, list })}</p>
      <h2 className="oobe-sub">{t('Add an app')}</h2>
      <p><Fill text={t('Open WittyMediaClient on a phone, tablet or TV at home: it finds this server by itself. When it shows a code, approve it at {address} on a device that is signed in, or scan its QR code with your phone.')} parts={{ address: <code>{here}/pair</code> }} /></p>
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={next}>{t('Continue')}</Button></StepActions>
    </section>
  )
}
