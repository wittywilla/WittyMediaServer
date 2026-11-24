// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Navigate } from 'react-router-dom'
import { useMedia } from '../shell/useNav'
import { SettingsHomePage } from './settings/menu/SettingsMenu'

// You, on a phone: the settings menu as the apps' You page draws it (settingsmenu.json, SettingsMenu.tsx): the
// person, then the groups, then this server. A wider screen has the same page as Settings.

export default function YouPage() {
  const phone = useMedia('(max-width: 760px)')
  if (!phone) return <Navigate to="/settings" replace />
  return <SettingsHomePage wide={false} tv={false} />
}
