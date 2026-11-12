// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What a library of each kind holds, in its own words: "1,204 titles", "68 episodes".
// Shared by the hand-built Home and the server-driven pages' library tiles.

export const LIBRARY_NOUNS: Record<string, [string, string]> = {
  movies: ['title', 'titles'], discs: ['disc', 'discs'], videos: ['video', 'videos'], commercials: ['commercial', 'commercials'], shows: ['episode', 'episodes'], anime: ['episode', 'episodes'],
  games: ['game', 'games'], music: ['track', 'tracks'], audiobooks: ['file', 'files'], podcasts: ['episode', 'episodes'], books: ['book', 'books'],
  photos: ['picture', 'pictures'], shorts: ['video', 'videos'], channels: ['video', 'videos'], musicvideos: ['video', 'videos'], radio: ['station', 'stations'],
}

// countWords: n things in a library of this type ("1 track", "6 tracks"); "items" for a type it doesn't know.
export function countWords(type: string | undefined, n: number): string {
  const [one, many] = (type && LIBRARY_NOUNS[type]) || ['item', 'items']
  return `${n.toLocaleString()} ${n === 1 ? one : many}`
}
