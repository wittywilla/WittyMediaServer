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

// Marks English shown to people, translated where it's shown (the app's tk, kept here so the node tests need no app).
const tk = <S extends string>(english: S): S => english

export const sectionId = (title: string) => title.toLowerCase().replace(/[’'"]/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')

export const ENTRIES: Entry[] = [
  // My game settings
  // Appearance
  { page: 'appearance', title: tk('Language and time zone'), section: tk('Language and time zone'), words: 'language of the app pages words translate translation locale time zone timezone clock disc menus sound tracks audio', admin: true },
  { page: 'appearance', title: tk('Theme'), section: tk('Theme'), words: 'themes look skin style dark mode light mode night automatic witty cinema evergreen high contrast light midnight paper true black oled' },
  { page: 'appearance', title: tk('Light or dark, as this device is set'), section: tk('Theme'), words: 'automatic dark mode light mode night follow system' },
  { page: 'appearance', title: tk('The theme everyone starts with'), section: tk('Theme'), words: 'make default server theme for everyone', admin: true },
  { page: 'appearance', title: tk('Install a theme ZIP'), section: tk('Theme'), words: 'upload download add theme file', admin: true },
  { page: 'appearance', title: tk('Colours'), section: tk('Colours'), words: 'make a theme custom theme accent background tint corners round square' },
  { page: 'appearance', title: tk('Accent colour'), section: tk('Colours'), words: 'buttons highlights custom' },
  { page: 'appearance', title: tk('Corners'), section: tk('Colours'), words: 'rounded square soft radius' },
  { page: 'appearance', title: tk('Look'), section: tk('Look'), words: 'layout experience look navigation side menu top bar sidebar rail finish type spacing' },
  { page: 'appearance', title: tk('Arrange the menu'), at: 'navigation', words: 'reorder hide show menu items libraries group watch listen read play more menu folded' },
  { page: 'appearance', title: tk('Moving posters'), section: tk('Motion and big screens'), words: 'tilt animation motion' },
  { page: 'appearance', title: tk('Moving backgrounds'), section: tk('Motion and big screens'), words: 'canvas canvases looping video background animation motion data saver' },
  { page: 'appearance', title: tk('TV mode'), section: tk('Motion and big screens'), words: 'tv television apple android big screen picture ten foot larger text bigger focus outline sofa remote controller gamepad couch' },
  { page: 'appearance', title: tk('WittyTV look'), section: tk('WittyTV'), words: 'tv guide theme classic satellite teletext newspaper big picture outline compact modern custom live tv' },
  { page: 'appearance', title: tk('WittyTV colours'), section: tk('WittyTV'), words: 'tv guide background programmes on now highlight accent text' },
  { page: 'appearance', title: tk('TV guide preview, rows and hours'), section: tk('WittyTV'), words: 'preview size side row size compact large hours now line progress logos episode names' },
  { page: 'appearance', title: tk('Colours for everyone'), section: tk('Colours for everyone'), words: 'theme colours override global tokens background text accent links status contrast readable server', admin: true },
  { page: 'appearance', title: tk('Module colours'), section: tk('Module colours'), words: 'module accent wittytv wittyplay override tokens contrast own colours', admin: true },
  { page: 'appearance', title: tk('Player'), section: tk('Player'), words: 'video player controls seek bar timeline chapters hide controls picture fit fill zoom black bars subtitle size' },
  { page: 'appearance', title: tk('Name and logo'), section: tk('Name and logo'), words: 'rename brand server name logo title browser tab', admin: true },

  // Libraries and server
  { page: 'libraries', title: tk('Folders of the same kind'), section: tk('Folders of the same kind'), words: 'combine separate libraries grouping' },
  { page: 'libraries', title: tk('Add a library'), words: 'folder scan media movies shows music' },
  { page: 'libraries', title: tk('Remove downloaded details'), words: 'metadata forget clear reset nfo artwork posters fanart download again rematch match test clean' },
  { page: 'pages', title: tk('Change Home for everyone'), words: 'home page rows sections shelves hero spotlight layout arrange server-driven sdui editor customise customize everyone', admin: true },
  { page: 'pages', title: tk('Change a library’s page or a title’s page'), words: 'library page detail page item page rows grid filter tabs sections editor', admin: true },
  { page: 'pages', title: tk('The menu for everyone'), words: 'navigation menu entries tab bar sidebar top menu order hide add entry phone tablet television', admin: true },
  { page: 'pages', title: tk('Make a page of your own'), words: 'new page custom page family night collection page create', admin: true },
  { page: 'pages', title: tk('Ready-made pages and sections'), words: 'presets gallery templates streaming minimal kids music live tv photo frame', admin: true },
  { page: 'pages', title: tk('Draft, preview and publish'), words: 'draft preview publish discard try phone tablet television desktop', admin: true },
  { page: 'pages', title: tk('Page history, import and export'), words: 'history revisions go back rollback undo import export file json backup pages', admin: true },
  { page: 'metadata', title: tk('Built-in agents'), section: tk('Built-in agents'), words: 'tvmaze wikidata wikipedia musicbrainz anilist radio browser posters plots' },
  { page: 'metadata', title: tk('Anime'), section: tk('Anime'), words: 'anilist title language romaji japanese english' },
  { page: 'metadata', title: tk('Save information and artwork next to your media'), section: tk('Saving information'), words: 'nfo poster fanart files write media folders' },
  { page: 'metadata', title: tk('Custom agents'), section: tk('Custom agents') },
  { page: 'playback', title: tk('Player'), section: tk('Player'), words: 'playback speed episode layout' },
  { page: 'playback', title: tk('Skipping'), section: tk('Skipping'), words: 'auto skip intro recap credits adverts commercials next episode automatically' },
  { page: 'playback', title: tk('Video quality limits'), section: tk('Video quality'), words: 'quality resolution bitrate limit transcode per profile remote away' },
  { page: 'quality', title: tk('Video quality'), section: tk('Your profile'), words: 'quality resolution bitrate 4k 1080p 720p 480p data mobile away home device auto original' },
  { page: 'channels', title: tk('WittyTV theme for new viewers'), words: 'tv guide look default' },
  { page: 'channels', title: tk('Tuners and IPTV'), section: tk('Tuners and IPTV'), words: 'm3u playlist hdhomerun antenna live tv' },
  { page: 'transcoding', title: tk('Hardware encoders'), section: tk('Hardware encoders'), words: 'gpu nvenc quick sync vaapi amf videotoolbox acceleration' },
  { page: 'transcoding', title: tk('Picture quality'), section: tk('Picture quality'), words: 'bitrate encoder speed preset crf' },
  { page: 'transcoding', title: tk('Allow direct play'), section: tk('Playback'), words: 'maximum resolution sound format' },
  { page: 'network', title: tk('Port'), section: tk('Port'), words: 'address listen 9797 port number url settings.ini change port in use' },
  { page: 'network', title: tk('What follows the port'), section: tk('What follows the port'), words: 'firewall apps find discovery bookmarks reverse proxy' },
  { page: 'data-folder', title: tk('Where the data is now'), section: tk('Where it is now'), words: 'data folder appdata documents database wms.db settings location storage disk size old copy delete' },
  { page: 'data-folder', title: tk('Move the data folder'), section: tk('Move it'), words: 'move data folder documents install folder portable custom drive onedrive icloud synced storage location' },
  { page: 'remote', title: tk('Limits for viewers away from home'), section: tk('Limits for viewers away from home'), words: 'bandwidth resolution data' },
  { page: 'remote', title: tk('Sign-in protection'), section: tk('Sign-in protection'), words: 'wrong passwords lockout' },
  { page: 'remote', title: tk('Peer-to-peer connections'), section: tk('Peer-to-peer connections'), words: 'p2p away from home no port forwarding router nat hole punching relay invite paired devices upnp cgnat libp2p direct connection' },

  // You
  { page: 'users', title: tk('Kids profiles'), words: 'children parental limits ratings' },
  { page: 'users', title: tk('Sign-in screen'), section: tk('Signing in'), words: 'profiles login pin who is watching', admin: true },
  { page: 'security', title: tk('Two-factor sign-in'), section: tk('Two-factor sign-in'), words: '2fa authenticator code' },
  { page: 'security', title: tk('Your devices'), section: tk('Your devices'), words: 'signed in sessions sign out' },
  { page: 'activity', title: tk('History'), section: tk('History'), words: 'statistics keep clear' },
  { page: 'module-settings', title: tk('Module settings'), words: 'module modules plugin options preferences' },
  { page: 'sharing', title: tk('Uploads'), section: tk('Uploads'), words: 'upload size room storage' },
  { page: 'sharing', title: tk('Links for people without an account'), section: tk('Links for people without an account'), words: 'share link guest' },
  { page: 'sharing', title: tk('Play on other devices'), section: tk('Play on other devices'), words: 'remote control cast device tv phone household other accounts' },
  { page: 'sharing', title: tk('Watch parties'), section: tk('Watch parties'), words: 'syncplay watch together party code sync friends' },
  { page: 'sharing', title: tk('Local AI'), section: tk('Local AI'), words: 'faces people places photos recognition' },

  // System
  { page: 'backup', title: tk('Back up by itself'), section: tk('Back up by itself'), words: 'schedule automatic daily weekly' },
  { page: 'backup', title: tk('Restore a backup'), section: tk('Restore a backup'), words: 'bring back import zip' },
  { page: 'health', title: tk('Possible duplicates'), words: 'duplicate twice copies versions merge library health' },
  { page: 'health', title: tk('Files that have gone'), words: 'missing deleted gone files library health' },
  { page: 'health', title: tk('NFO files that can’t be read'), words: 'nfo broken metadata xml sidecar' },
  { page: 'health', title: tk('Files that can’t be read'), words: 'unplayable corrupt probe ffprobe broken video' },
  { page: 'repair', title: tk('Moved a drive?'), section: tk('Moved a drive?'), words: 'folder path changed drive letter' },
  { page: 'about', title: tk('Updates'), section: tk('Updates'), words: 'check for updates version new' },
  { page: 'about', title: tk('Supported formats'), section: tk('Supported formats'), words: 'codecs file types containers' },
  { page: 'about', title: tk('Third-party licenses'), section: tk('Third-party licenses'), words: 'open source credits notices' },
]

// Spelling and words that mean the same, so either finds it.
const SAME: [RegExp, string][] = [[/colou?rs?/g, 'colour'], [/gr[ae]y/g, 'grey'], [/licen[cs]e/g, 'license'], [/\bdark\s*mode\b/g, 'dark'], [/[’']/g, '']]
export const norm = (s: string) => {
  let t = s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
  for (const [rx, to] of SAME) t = t.replace(rx, to)
  return t.replace(/&/g, ' and ').replace(/[^\p{L}\p{M}\p{N}?-]+/gu, ' ').trim()
}
const startsWord = (hay: string, w: string) => hay.startsWith(w) || hay.includes(' ' + w) || hay.includes('-' + w)

// searchSettings: what matches every word typed, best first. pages are the ones this person may
// open, so a setting on a page they can't see isn't offered, and nor is one only administrators see.
// tr translates a title for showing and matching: what's typed may be in English or in the app's language.
export function searchSettings(query: string, pages: SettingsPage[], extra: Result[] = [], admin = true, tr: (s: string) => string = (s) => s): Result[] {
  const words = norm(query).split(' ').filter(Boolean)
  if (!words.length) return []
  const byPage = new Map(pages.map((p) => [p.to, p]))
  const found: { r: Result; score: number }[] = []
  const consider = (r: Result, title: string, rest: string) => {
    const here = tr(title), t = norm(here === title ? title : here + ' ' + title), all = t + ' ' + norm(rest)
    let score = 0
    for (const w of words) {
      if (startsWord(t, w)) score += 3
      else if (startsWord(all, w)) score += 1
      else return
    }
    if (t.startsWith(words.join(' '))) score += 4
    found.push({ r, score })
  }
  for (const p of pages) consider({ to: `/settings/${p.to}`, title: tr(p.label), where: tr('Settings'), hint: tr(p.hint) }, p.label, `${p.hint} ${tr(p.hint)}`)
  for (const e of ENTRIES) {
    const p = byPage.get(e.page)
    if (!p || (e.admin && !admin)) continue
    const at = e.at || (e.section ? sectionId(e.section) : '')
    consider({ to: `/settings/${p.to}${at ? '#' + at : ''}`, title: tr(e.title), where: e.section && e.section !== e.title ? `${tr(p.label)}, ${tr(e.section)}` : tr(p.label) }, e.title, `${e.section || ''} ${tr(e.section || '')} ${p.label} ${tr(p.label)} ${e.words || ''}`)
  }
  for (const r of extra) consider({ ...r, title: tr(r.title), where: tr(r.where) }, r.title, `${r.where} ${r.hint || ''}`)
  found.sort((a, b) => b.score - a.score)
  const seen = new Set<string>()
  return found.filter(({ r }) => !seen.has(r.to + r.title) && seen.add(r.to + r.title)).slice(0, 30).map(({ r }) => r)
}
