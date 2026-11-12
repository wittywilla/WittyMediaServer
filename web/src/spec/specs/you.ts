// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secP, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'

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

type T = SpecRow['tint']
const row = (title: string, to: string, icon: string, tint: T, value?: string | number | null): SpecRow => ({ title, to, icon, tint, ...(value !== undefined && value !== null && value !== '' ? { value: String(value) } : {}) })
const quiet = 'e' as const

export function youSpec(d: YouIn): SpecPageData {
  const s: SpecSection[] = []
  const account: SpecRow[] = [row('Name and username', '/settings/users', 'Person', 'a', d.shown)]
  if (d.adult) account.push(row('Password', '/settings/users', 'Key', 'f'))
  if (d.adult && d.twoStep !== null) account.push(row('Two-step sign-in', '/settings/security', 'Lock', 'b', d.twoStep === undefined ? '' : d.twoStep ? 'On' : 'Off'))
  account.push(row('Your devices', '/settings/security', 'Devices', 'f', d.devices))
  s.push(secR('Account', account))
  if (d.adult) s.push(secR('Household', [row('Sign in a TV', '/pair', 'Tv', 'a'), row('From away', '/away', 'Tv', 'a'), row('Profiles and parental limits', '/settings/users', 'Users', 'c')]))
  const yours: SpecRow[] = [row('Your year', '/review', 'Sparkles', 'c'), row('Collections and playlists', '/collections', 'Stack', 'b')]
  if (!d.sharingHidden) yours.push(row('Share links', '/settings/sharing', 'Link', 'a'))
  if (!d.activityHidden) yours.push(row('Activity', '/settings/activity', 'Activity', 'c'))
  if (d.games && !d.gamesOff) yours.push(row('Game settings', '/settings/games', 'Contrast', 'f'))
  if (d.games) yours.push(row('Controllers', '/controllers', 'Gamepad', 'f'))
  s.push(secR('Yours', yours))
  s.push(secR('On this device', [row('Theme', '/settings/appearance#theme', 'Palette', 'b', d.themeName)]))
  if (d.isAdmin) {
    s.push(secR('Server', [
      row('Libraries', '/settings/libraries', 'Library', quiet), row('Metadata and artwork', '/settings/metadata', 'Image', quiet), row('Fix matches', '/fix-matches', 'Wand', quiet),
      row('Playback and discs', '/settings/playback', 'Play', quiet), row('Live TV and tuners', '/settings/channels', 'Channels', quiet), row('Consoles and cores', '/settings/gaming', 'Gamepad', quiet),
      row('Game PCs', '/settings/couch', 'Drive', quiet), row('Live transcoding', '/settings/transcoding', 'Bolt', quiet), row('Library transcoding', '/settings/library-transcoding', 'Refresh', quiet),
      row('Remote access', '/settings/remote', 'Globe', quiet), row('Appearance and branding', '/settings/appearance', 'Brush', quiet), row('WittyRigs', '/settings/wittyrigs', 'Server', quiet),
    ]))
    s.push(secR('Maintenance', [
      row('WittyLog', '/settings/logs', 'File', quiet), row('WittyDiagnoser', '/settings/diagnoser', 'Tune', quiet), row('Modules', '/settings/modules', 'Plug', quiet),
      row('Backup', '/settings/backup', 'Download', quiet), row('Repair', '/settings/repair', 'Wrench', quiet), row('Reset', '/settings/reset', 'Refresh', 'd'),
    ]))
  }
  s.push(secR('Help and about', [
    row('Help', '/help', 'Help', 'c'),
    row('About this server', d.newer ? '/settings/about#updates' : '/settings/about', 'Info', quiet, d.newer ? 'Update available' : d.version),
    row('Legal', '/legal', 'Shield', quiet), row('Licences', '/settings/about#licenses', 'Book', quiet),
  ]))
  s.push(secR('This server', [{ title: 'Address', value: d.address, ...(d.print ? { sub: `Server fingerprint ${d.print}` } : {}) }]))
  s.push(secP('', [...(d.native ? [{ id: 'change-server', title: 'Change server' }] : []), { id: 'sign-out', title: 'Sign out' }]))
  return mk('you', 'You', d.shown, '', s)
}
