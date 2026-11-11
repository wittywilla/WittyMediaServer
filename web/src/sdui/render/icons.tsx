// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { ReactNode } from 'react'
import {
  IconAnime, IconAudio, IconBook, IconCamera, IconChannels, IconChart, IconCheck, IconDevices, IconDisc, IconDownload, IconFilm, IconFolder,
  IconGamepad, IconGlobe, IconHeadphones, IconHome, IconImage, IconInfo, IconLibrary, IconLink, IconMic, IconMore, IconPerson, IconPlay, IconPlug,
  IconPlus, IconRadio, IconRefresh, IconSearch, IconSettings, IconSortDown, IconSparkles, IconStack, IconStar, IconTune, IconTv, IconUsers, IconBolt, IconHelp, IconChapters, IconTag,
} from '../../icons'

// The catalogue's icon names (docs/SDUI.md, "Icons") as this app's own icons. A name
// this app doesn't know is drawn as nothing rather than as a guess.
const ICONS: Record<string, () => ReactNode> = {
  home: () => <IconHome />, library: () => <IconLibrary />, movie: () => <IconFilm />, show: () => <IconTv />, tv: () => <IconTv />, live: () => <IconChannels />,
  guide: () => <IconChapters />, record: () => <IconDisc />, music: () => <IconAudio />, album: () => <IconDisc />, artist: () => <IconPerson />, mic: () => <IconMic />,
  radio: () => <IconRadio />, podcast: () => <IconMic />, book: () => <IconBook />, audiobook: () => <IconHeadphones />, comic: () => <IconBook />, photo: () => <IconImage />,
  video: () => <IconCamera />, disc: () => <IconDisc />, game: () => <IconGamepad />, collection: () => <IconStack />, list: () => <IconStack />, folder: () => <IconFolder />,
  search: () => <IconSearch />, settings: () => <IconSettings />, user: () => <IconPerson />, users: () => <IconUsers />, kids: () => <IconPerson />, star: () => <IconStar />,
  heart: () => <IconStar />, clock: () => <IconRefresh />, history: () => <IconRefresh />, calendar: () => <IconChapters />, sparkles: () => <IconSparkles />, fire: () => <IconBolt />,
  shuffle: () => <IconRefresh />, play: () => <IconPlay />, download: () => <IconDownload />, share: () => <IconLink />, globe: () => <IconGlobe />, info: () => <IconInfo />,
  warning: () => <IconHelp />, check: () => <IconCheck />, plus: () => <IconPlus />, grid: () => <IconLibrary />, filter: () => <IconTune />, sort: () => <IconSortDown />,
  plug: () => <IconPlug />, chart: () => <IconChart />, devices: () => <IconDevices />, more: () => <IconMore />, anime: () => <IconAnime />, tag: () => <IconTag />,
}

export const sduiIcon = (name: unknown): ReactNode => (typeof name === 'string' && ICONS[name] ? ICONS[name]() : null)

// The icon for a kind of item, where a card has no picture.
const KIND_ICON: Record<string, string> = {
  movie: 'movie', show: 'show', episode: 'show', video: 'video', disc: 'disc', album: 'album', artist: 'artist', track: 'music', book: 'book', audiobook: 'audiobook',
  podcast: 'podcast', game: 'game', photo: 'photo', channel: 'live', recording: 'record', collection: 'collection', library: 'library', genre: 'tag',
}
export const kindIcon = (kind: unknown): ReactNode => sduiIcon(typeof kind === 'string' ? KIND_ICON[kind] || 'movie' : 'movie')
