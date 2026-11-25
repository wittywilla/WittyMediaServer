// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Link } from 'react-router-dom'
import { useApp } from '../state'
import ConnectDevice from '../components/ConnectDevice'
import { StandaloneCard } from '../components/StandaloneCard'
import { Fill } from './setup/parts'
import { t, tk } from '../i18n'
import { PageHead } from './settings/ui'
import { useInSettings } from './settings/open'

// From away: connecting another device, for everyone who can sign devices in. The same screen as the top of
// Settings, Remote Access (components/ConnectDevice), so the words and the way are the same wherever it's
// found. Signed out, it says where the code comes from.
export default function AwayPage() {
  const { user } = useApp()
  const inSettings = useInSettings()
  // Opened from Settings it is a Settings page: its head, start-aligned, with the menu beside it (#763).
  if (user && inSettings) return <><PageHead title={tk('Connect a device')} desc={t('Sign in another phone, tablet, TV or computer with a code or a QR code, at home or away.')} /><ConnectDevice titled={false} /></>
  if (user) return <div className="away"><h1>{t('Connect a device')}</h1><ConnectDevice titled={false} /></div>
  // Signed out it is a standalone page (docs/DESIGN.md): the steps, in a card with the server's name.
  return (
    <StandaloneCard className="away-sa">
      <h1>{t('Connect a device')}</h1>
      <ol className="away-steps">
        <li><Fill text={t('{signIn} on this server, here or on another device that is already signed in.')} parts={{ signIn: <Link to="/login">{t('Sign in')}</Link> }} /></li>
        <li><Fill text={t('Open {page}, and choose {button}. It shows a code and a QR code.')} parts={{ page: <b>{t('Connect a device')}</b>, button: <b>{t('Show a code')}</b> }} /></li>
        <li><Fill text={t('On the new device, open WittyMediaClient, choose {add} and then {code}, and type the code. Or scan the QR code with it.')} parts={{ add: <b>{t('Add a server')}</b>, code: <b>{t('Enter a code')}</b> }} /></li>
      </ol>
      <Link className="btn primary" to="/login">{t('Sign in')}</Link>
    </StandaloneCard>
  )
}
