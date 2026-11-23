// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secP, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// The You page on a phone (the app's own page, canvas "You"): groups of rows that each open a Settings or app page.
// The person at the top and the server's coloured name are the page's own. Chip ids: change-server, sign-out.
export interface YouIn {
  shown: string; adult: boolean; isAdmin: boolean
  twoStep: boolean | null | undefined // null: the server doesn't offer it; undefined: not known yet
  devices: number | null
  games: boolean; gamesOff: boolean; sharingHidden: boolean; activityHidden: boolean
  themeName: string; newer: boolean; version: string
  address: string; print: string; native: boolean
}

type Tint = SpecRow['tint']
const row = (title: string, to: string, icon: string, tint: Tint, value?: string | number | null): SpecRow => ({ title, to, icon, tint, ...(value !== undefined && value !== null && value !== '' ? { value: String(value) } : {}) })
const quiet = 'e' as const

export function youSpec(d: YouIn): SpecPageData {
  const s: SpecSection[] = []
  const account: SpecRow[] = [row(t('Name and username'), '/settings/users', 'Person', 'a', d.shown)]
  if (d.adult) account.push(row(t('Password'), '/settings/users', 'Key', 'f'))
  if (d.adult && d.twoStep !== null) account.push(row(t('Two-step sign-in'), '/settings/security', 'Lock', 'b', d.twoStep === undefined ? '' : d.twoStep ? t('On') : t('Off')))
  account.push(row(t('Your devices'), '/settings/security', 'Devices', 'f', d.devices))
  s.push(secR(t('Account'), account))
  if (d.adult) s.push(secR(t('Household'), [row(t('Sign in a TV'), '/pair', 'Tv', 'a'), row(t('Connect a device'), '/away', 'Tv', 'a'), row(t('Profiles and parental limits'), '/settings/users', 'Users', 'c')]))
  const yours: SpecRow[] = [row(t('Your year'), '/review', 'Sparkles', 'c'), row(t('Collections and playlists'), '/collections', 'Stack', 'b')]
  if (!d.sharingHidden) yours.push(row(t('Share links'), '/settings/sharing', 'Link', 'a'))
  if (!d.activityHidden) yours.push(row(t('Activity'), '/settings/activity', 'Activity', 'c'))
  if (d.games && !d.gamesOff) yours.push(row(t('Game settings'), '/settings/games', 'Contrast', 'f'))
  if (d.games) yours.push(row(t('Controllers'), '/controllers', 'Gamepad', 'f'))
  s.push(secR(t('Yours'), yours))
  s.push(secR(t('On this device'), [row(t('Theme'), '/settings/appearance#theme', 'Palette', 'b', d.themeName)]))
  if (d.isAdmin) {
    s.push(secR(t('Server'), [
      row(t('Libraries'), '/manage/libraries', 'Library', quiet), row(t('Metadata and artwork'), '/settings/metadata', 'Image', quiet), row(t('Fix matches'), '/fix-matches', 'Wand', quiet),
      row(t('Playback'), '/settings/playback', 'Play', quiet), row(t('Live TV and tuners'), '/settings/channels', 'Channels', quiet), row(t('Consoles and cores'), '/settings/gaming', 'Gamepad', quiet),
      row(t('Live transcoding'), '/settings/transcoding', 'Bolt', quiet), row(t('Library transcoding'), '/settings/library-transcoding', 'Refresh', quiet),
      row(t('Remote access'), '/settings/remote', 'Globe', quiet), row(t('Appearance and branding'), '/settings/appearance', 'Brush', quiet),
    ]))
    s.push(secR(t('Maintenance'), [
      row('WittyLog', '/settings/logs', 'File', quiet), row('WittyDiagnoser', '/settings/diagnoser', 'Tune', quiet), row(t('Modules'), '/settings/modules', 'Plug', quiet),
      row(t('Backup'), '/settings/backup', 'Download', quiet), row(t('Repair'), '/settings/repair', 'Wrench', quiet), row(t('Reset'), '/settings/reset', 'Refresh', 'd'),
    ]))
  }
  s.push(secR(t('Help and about'), [
    row(t('Help'), '/help', 'Help', 'c'),
    row(t('About this server'), d.newer ? '/settings/about#updates' : '/settings/about', 'Info', quiet, d.newer ? t('Update available') : d.version),
    row(t('Legal'), '/legal', 'Shield', quiet), row(t('Licences'), '/settings/about#licenses', 'Book', quiet),
  ]))
  s.push(secR(t('This server'), [{ title: t('Address'), value: d.address, ...(d.print ? { sub: t('Server fingerprint {fingerprint}', { fingerprint: d.print }) } : {}) }]))
  s.push(secP('', [...(d.native ? [{ id: 'change-server', title: t('Change server') }] : []), { id: 'sign-out', title: t('Sign out') }]))
  return mk('you', t('You'), d.shown, '', s)
}
