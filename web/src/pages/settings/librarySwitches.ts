// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The words under a library's switches in Settings, Libraries: they say what the switch does as it
// stands now, so a switch that is on never reads "Off: …".

// nfoOn: whether a library reads .nfo files other programs wrote (foreignNfo). On unless turned off.
// The words are English, marked with tk: the page shows them with tx(). (The app's tk, kept here so the node tests need no app.)
const tk = <S extends string>(english: S): S => english

export const nfoOn = (lib: { foreignNfo?: boolean }): boolean => lib.foreignNfo !== false

export function nfoSwitchText(on: boolean): string {
  return on
    ? tk("On: titles, years and details are read from other programs' .nfo files beside the media, where there are any. The server's own .nfo files are read either way.")
    : tk("Off: titles, years and details come from the file names and what is found online. The server's own .nfo files are read either way.")
}

export function commercialsSwitchText(on: boolean): string {
  return on
    ? tk('On: it shows in the menus and on Home, and plays on the channels you choose it for.')
    : tk('Off: only channels use it, on the channels you choose it for.')
}

// nfoSwitchShown: the kinds of library whose scan the switch changes: films and other videos read
// movie.nfo and <name>.nfo, shows and anime tvshow.nfo and each episode's. Music, books,
// audiobooks, photos and the rest never read another program's .nfo, so it isn't offered there.
const NFO_KINDS = new Set(['movies', 'shows', 'anime', 'discs', 'videos', 'commercials'])
export const nfoSwitchShown = (type: string): boolean => NFO_KINDS.has(type)
