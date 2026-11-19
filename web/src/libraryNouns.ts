// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { tn } from './i18n'

// What a library of each kind holds, in its own words: "1,204 titles", "68 episodes".
// Shared by the hand-built Home and the server-driven pages' library tiles. Each is a whole
// phrase with its count, so a language can put the number where it belongs.

type Count = (n: number) => string
const titles: Count = (n) => tn('{count} title', '{count} titles', n)
const discs: Count = (n) => tn('{count} disc', '{count} discs', n)
const videos: Count = (n) => tn('{count} video', '{count} videos', n)
const commercials: Count = (n) => tn('{count} commercial', '{count} commercials', n)
const episodes: Count = (n) => tn('{count} episode', '{count} episodes', n)
const games: Count = (n) => tn('{count} game', '{count} games', n)
const tracks: Count = (n) => tn('{count} track', '{count} tracks', n)
const files: Count = (n) => tn('{count} file', '{count} files', n)
const books: Count = (n) => tn('{count} book', '{count} books', n)
const pictures: Count = (n) => tn('{count} picture', '{count} pictures', n)
const stations: Count = (n) => tn('{count} station', '{count} stations', n)
const items: Count = (n) => tn('{count} item', '{count} items', n)

const LIBRARY_COUNTS: Record<string, Count> = {
  movies: titles, discs, videos, commercials, shows: episodes, anime: episodes,
  games, music: tracks, audiobooks: files, podcasts: episodes, books,
  photos: pictures, shorts: videos, channels: videos, musicvideos: videos, radio: stations,
}

// countWords: n things in a library of this type ("1 track", "6 tracks"); "items" for a type it doesn't know.
export function countWords(type: string | undefined, n: number): string {
  return ((type && LIBRARY_COUNTS[type]) || items)(n)
}
