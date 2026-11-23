// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Link } from 'react-router-dom'
import { useApp } from '../state'
import ConnectDevice from '../components/ConnectDevice'
import { Fill } from './setup/parts'
import { t } from '../i18n'

// From away: connecting another device, for everyone who can sign devices in. The same screen as the top of
// Settings, Remote Access (components/ConnectDevice), so the words and the way are the same wherever it's
// found. Signed out, it says where the code comes from.
export default function AwayPage() {
  const { user } = useApp()
  return (
    <div className="away">
      <h1>{t('Connect a device')}</h1>
      {!user && (
        <section className="away-card" aria-labelledby="away-in">
          <h2 id="away-in">{t('How to connect a device')}</h2>
          <ol className="away-steps">
            <li><Fill text={t('{signIn} on this server, here or on another device that is already signed in.')} parts={{ signIn: <Link to="/login">{t('Sign in')}</Link> }} /></li>
            <li><Fill text={t('Open {page}, and choose {button}. It shows a code and a QR code.')} parts={{ page: <b>{t('Connect a device')}</b>, button: <b>{t('Show a code')}</b> }} /></li>
            <li><Fill text={t('On the new device, open WittyMediaClient, choose {add} and then {code}, and type the code. Or scan the QR code with it.')} parts={{ add: <b>{t('Add a server')}</b>, code: <b>{t('Enter a code')}</b> }} /></li>
          </ol>
        </section>
      )}
      {user && <ConnectDevice />}
    </div>
  )
}
