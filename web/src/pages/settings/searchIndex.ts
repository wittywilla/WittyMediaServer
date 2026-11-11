// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Finding a setting by name. Each settings page is found by its name and hint (they come from
// the menu); this list adds the sections inside pages and the settings people look for most,
// in the words they might use. A section's anchor is its title, as sectionId makes it, so a
// renamed section must be renamed here too: searchIndex.test.mjs checks every title still
// exists. Nothing here touches the page.

export type SettingsPage = { to: string; label: string; hint: string }
// section: the title of the section card it's in, which is also where the result jumps to.
// at: an anchor that isn't a section title. words: other ways of saying it. admin: only
// administrators see it, on a page others see too.
export type Entry = { page: string; title: string; section?: string; at?: string; words?: string; admin?: boolean }
export type Result = { to: string; title: string; where: string; hint?: string }

export const sectionId = (title: string) => title.toLowerCase().replace(/[’'"]/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')

export const ENTRIES: Entry[] = [
  // Appearance
  { page: 'appearance', title: 'Theme', section: 'Theme', words: 'themes look skin style dark mode light mode night automatic witty cinema evergreen high contrast light midnight paper true black oled' },
  { page: 'appearance', title: 'Light or dark, as this device is set', section: 'Theme', words: 'automatic dark mode light mode night follow system' },
  { page: 'appearance', title: 'The theme everyone starts with', section: 'Theme', words: 'make default server theme for everyone', admin: true },
  { page: 'appearance', title: 'Install a theme ZIP', section: 'Theme', words: 'upload download add theme file', admin: true },
  { page: 'appearance', title: 'Colours', section: 'Colours', words: 'make a theme custom theme accent background tint corners round square' },
  { page: 'appearance', title: 'Accent colour', section: 'Colours', words: 'buttons highlights custom' },
  { page: 'appearance', title: 'Corners', section: 'Colours', words: 'rounded square soft radius' },
  { page: 'appearance', title: 'Layout', section: 'Layout', words: 'navigation side menu top bar sidebar' },
  { page: 'appearance', title: 'Arrange the menu', at: 'navigation', words: 'reorder hide show menu items libraries group watch listen read play more menu folded' },
  { page: 'appearance', title: 'Moving posters', section: 'Motion and big screens', words: 'tilt animation motion' },
  { page: 'appearance', title: 'Moving backgrounds', section: 'Motion and big screens', words: 'canvas canvases looping video background animation motion data saver' },
  { page: 'appearance', title: 'TV mode', section: 'Motion and big screens', words: 'tv television apple android big screen picture ten foot larger text bigger focus outline sofa remote controller gamepad couch' },
  { page: 'appearance', title: 'WittyTV look', section: 'WittyTV', words: 'tv guide theme classic satellite teletext newspaper big picture outline compact modern custom live tv' },
  { page: 'appearance', title: 'WittyTV colours', section: 'WittyTV', words: 'tv guide background programmes on now highlight accent text' },
  { page: 'appearance', title: 'TV guide preview, rows and hours', section: 'WittyTV', words: 'preview size side row size compact large hours now line progress logos episode names' },
  { page: 'appearance', title: 'Colours for everyone', section: 'Colours for everyone', words: 'theme colours override global tokens background text accent links status contrast readable server', admin: true },
  { page: 'appearance', title: 'Module colours', section: 'Module colours', words: 'module accent wittytv wittyplay override tokens contrast own colours', admin: true },
  { page: 'appearance', title: 'Player', section: 'Player', words: 'video player controls seek bar timeline chapters hide controls picture fit fill zoom black bars subtitle size' },
  { page: 'appearance', title: 'Name and logo', section: 'Name and logo', words: 'rename brand server name logo title browser tab', admin: true },

  // Libraries and server
  { page: 'libraries', title: 'Folders of the same kind', section: 'Folders of the same kind', words: 'combine separate libraries grouping' },
  { page: 'libraries', title: 'Add a library', words: 'folder scan media movies shows music' },
  { page: 'metadata', title: 'Built-in agents', section: 'Built-in agents', words: 'tvmaze wikidata wikipedia musicbrainz anilist radio browser posters plots' },
  { page: 'metadata', title: 'Anime', section: 'Anime', words: 'anilist title language romaji japanese english' },
  { page: 'metadata', title: 'Save information and artwork next to your media', section: 'Saving information', words: 'nfo poster fanart files write media folders' },
  { page: 'metadata', title: 'Custom agents', section: 'Custom agents' },
  { page: 'playback', title: 'Player', section: 'Player', words: 'playback speed episode layout' },
  { page: 'playback', title: 'Skipping', section: 'Skipping', words: 'auto skip intro recap credits adverts commercials next episode automatically' },
  { page: 'playback', title: 'DVDs and Blu-rays', section: 'DVDs and Blu-rays', words: 'disc region languages drive sounds insert animation parental bd-j java low power' },
  { page: 'playback', title: 'DVD and Blu-ray region', section: 'DVDs and Blu-rays', words: 'region code' },
  { page: 'channels', title: 'WittyTV theme for new viewers', words: 'tv guide look default' },
  { page: 'channels', title: 'Tuners and IPTV', section: 'Tuners and IPTV', words: 'm3u playlist hdhomerun antenna live tv' },
  { page: 'channels', title: 'Guide data', section: 'Guide data', words: 'xmltv epg listings schedule' },
  { page: 'gaming', title: 'BIOS files', section: 'BIOS files', words: 'firmware consoles' },
  { page: 'gaming', title: 'How games play', section: 'How games play', words: 'streaming emulator cores' },
  { page: 'transcoding', title: 'Hardware encoders', section: 'Hardware encoders', words: 'gpu nvenc quick sync vaapi amf videotoolbox acceleration' },
  { page: 'transcoding', title: 'Picture quality', section: 'Picture quality', words: 'bitrate encoder speed preset crf' },
  { page: 'transcoding', title: 'Allow direct play', section: 'Playback', words: 'maximum resolution sound format' },
  { page: 'network', title: 'Port', section: 'Port', words: 'address listen 9797 port number url settings.ini change port in use' },
  { page: 'network', title: 'What follows the port', section: 'What follows the port', words: 'firewall apps find discovery bookmarks reverse proxy' },
  { page: 'remote', title: 'Limits for viewers away from home', section: 'Limits for viewers away from home', words: 'bandwidth resolution data' },
  { page: 'remote', title: 'Sign-in protection', section: 'Sign-in protection', words: 'wrong passwords lockout' },

  // You
  { page: 'users', title: 'Kids profiles', words: 'children parental limits ratings' },
  { page: 'users', title: 'Sign-in screen', section: 'Signing in', words: 'profiles login pin who is watching', admin: true },
  { page: 'security', title: 'Two-factor sign-in', section: 'Two-factor sign-in', words: '2fa authenticator code' },
  { page: 'security', title: 'Your devices', section: 'Your devices', words: 'signed in sessions sign out' },
  { page: 'activity', title: 'History', section: 'History', words: 'statistics keep clear' },
  { page: 'sharing', title: 'Uploads', section: 'Uploads', words: 'upload size room storage' },
  { page: 'sharing', title: 'Links for people without an account', section: 'Links for people without an account', words: 'share link guest' },
  { page: 'sharing', title: 'Photo map', section: 'Photo map', words: 'map tiles address places' },
  { page: 'sharing', title: 'Use an online map', section: 'Photo map', words: 'online map streets tiles provider internet outline privacy credit attribution licence' },
  { page: 'sharing', title: 'Play on other devices', section: 'Play on other devices', words: 'remote control cast device tv phone household other accounts' },
  { page: 'sharing', title: 'Watch parties', section: 'Watch parties', words: 'syncplay watch together party code sync friends' },
  { page: 'sharing', title: 'Local AI', section: 'Local AI', words: 'faces people places photos recognition' },

  // System
  { page: 'backup', title: 'Back up by itself', section: 'Back up by itself', words: 'schedule automatic daily weekly' },
  { page: 'backup', title: 'Restore a backup', section: 'Restore a backup', words: 'bring back import zip' },
  { page: 'repair', title: 'Moved a drive?', section: 'Moved a drive?', words: 'folder path changed drive letter' },
  { page: 'about', title: 'Updates', section: 'Updates', words: 'check for updates version new' },
  { page: 'about', title: 'Supported formats', section: 'Supported formats', words: 'codecs file types containers' },
  { page: 'about', title: 'Third-party licenses', section: 'Third-party licenses', words: 'open source credits notices' },
]

// Spelling and words that mean the same, so either finds it.
const SAME: [RegExp, string][] = [[/colou?rs?/g, 'colour'], [/gr[ae]y/g, 'grey'], [/licen[cs]e/g, 'license'], [/\bdark\s*mode\b/g, 'dark'], [/[’']/g, '']]
export const norm = (s: string) => {
  let t = s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
  for (const [rx, to] of SAME) t = t.replace(rx, to)
  return t.replace(/&/g, ' and ').replace(/[^a-z0-9?-]+/g, ' ').trim()
}
const startsWord = (hay: string, w: string) => hay.startsWith(w) || hay.includes(' ' + w) || hay.includes('-' + w)

// searchSettings: what matches every word typed, best first. pages are the ones this person may
// open, so a setting on a page they can't see isn't offered, and nor is one only administrators see.
export function searchSettings(query: string, pages: SettingsPage[], extra: Result[] = [], admin = true): Result[] {
  const words = norm(query).split(' ').filter(Boolean)
  if (!words.length) return []
  const byPage = new Map(pages.map((p) => [p.to, p]))
  const found: { r: Result; score: number }[] = []
  const consider = (r: Result, title: string, rest: string) => {
    const t = norm(title), all = t + ' ' + norm(rest)
    let score = 0
    for (const w of words) {
      if (startsWord(t, w)) score += 3
      else if (startsWord(all, w)) score += 1
      else return
    }
    if (t.startsWith(words.join(' '))) score += 4
    found.push({ r, score })
  }
  for (const p of pages) consider({ to: `/settings/${p.to}`, title: p.label, where: 'Settings', hint: p.hint }, p.label, p.hint)
  for (const e of ENTRIES) {
    const p = byPage.get(e.page)
    if (!p || (e.admin && !admin)) continue
    const at = e.at || (e.section ? sectionId(e.section) : '')
    consider({ to: `/settings/${p.to}${at ? '#' + at : ''}`, title: e.title, where: e.section && e.section !== e.title ? `${p.label}, ${e.section}` : p.label }, e.title, `${e.section || ''} ${p.label} ${e.words || ''}`)
  }
  for (const r of extra) consider(r, r.title, `${r.where} ${r.hint || ''}`)
  found.sort((a, b) => b.score - a.score)
  const seen = new Set<string>()
  return found.filter(({ r }) => !seen.has(r.to + r.title) && seen.add(r.to + r.title)).slice(0, 30).map(({ r }) => r)
}
