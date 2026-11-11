// SPDX-License-Identifier: AGPL-3-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { AVATAR_COLORS } from './avatarColor'

// The pictures a profile can choose (auth.AvatarNames on the server, which keeps only the choice,
// as "name:colour"). Each is one filled shape on the 24 grid, like the icons, so it is the same on
// every device. The colours are the avatar colours every app shares, by name.

const SHAPES: Record<string, string> = {
  star: 'M12 2.5l2.9 6.2 6.6.8-4.9 4.6 1.3 6.6L12 17.4l-5.9 3.3 1.3-6.6L2.5 9.5l6.6-.8z',
  heart: 'M12 20.5C6.5 16.3 3 13 3 9.2 3 6.6 5 4.6 7.5 4.6c1.7 0 3.4.9 4.5 2.4 1.1-1.5 2.8-2.4 4.5-2.4C19 4.6 21 6.6 21 9.2c0 3.8-3.5 7.1-9 11.3z',
  moon: 'M20 14.5A8.5 8.5 0 019.5 4a8.5 8.5 0 1010.5 10.5z',
  sun: 'M12 7a5 5 0 100 10 5 5 0 000-10zM11 1h2v4h-2zM11 19h2v4h-2zM1 11h4v2H1zM19 11h4v2h-4zM4.2 5.6l1.4-1.4 2.8 2.8L7 8.4zM15.6 17l1.4-1.4 2.8 2.8-1.4 1.4zM4.2 18.4L7 15.6 8.4 17l-2.8 2.8zM15.6 7l2.8-2.8 1.4 1.4L17 8.4z',
  bolt: 'M13.5 2L5 13.5h5.5L9.5 22 19 10h-5.5z',
  diamond: 'M12 2.5l8.5 9.5-8.5 9.5L3.5 12z',
  crown: 'M4 18.5h16v2H4zM3 7.5l4.8 3.8L12 4.5l4.2 6.8L21 7.5l-1.6 9H4.6z',
  flower: 'M12 9.2a2.8 2.8 0 100 5.6 2.8 2.8 0 000-5.6zM12 2a3 3 0 100 6 3 3 0 000-6zM12 16a3 3 0 100 6 3 3 0 000-6zM5 9a3 3 0 100 6 3 3 0 000-6zM19 9a3 3 0 100 6 3 3 0 000-6z',
  music: 'M10 3.5v10.2a3.5 3.5 0 102 3.1V7.5h5v-4z',
  planet: 'M12 6a6 6 0 100 12 6 6 0 000-12zM2.6 15.6l.9 1.7 17.9-8.9-.9-1.7z',
  paw: 'M12 12c-3 0-5.5 2.5-5.5 5 0 1.7 1.4 2.5 2.8 2.5 1 0 1.8-.5 2.7-.5s1.7.5 2.7.5c1.4 0 2.8-.8 2.8-2.5 0-2.5-2.5-5-5.5-5zM6.5 7.7a1.8 1.8 0 100 3.6 1.8 1.8 0 000-3.6zM10 4.2a1.8 1.8 0 100 3.6 1.8 1.8 0 000-3.6zM14 4.2a1.8 1.8 0 100 3.6 1.8 1.8 0 000-3.6zM17.5 7.7a1.8 1.8 0 100 3.6 1.8 1.8 0 000-3.6z',
  tree: 'M12 2l7 10h-4l4 6h-6v4h-2v-4H5l4-6H5z',
  rocket: 'M12 2c3 2.5 4.5 6 4.5 10v5h-9v-5C7.5 8 9 4.5 12 2zM7.5 13L4 17v3l3.5-2zM16.5 13l3.5 4v3l-3.5-2zM10.5 18h3L12 22z',
  gamepad: 'M7 7h10a5 5 0 015 5v3.5a3.5 3.5 0 01-6.3 2.1L14.5 16h-5l-1.2 1.6A3.5 3.5 0 012 15.5V12a5 5 0 015-5zM7 10v1.5H5.5v2H7V15h2v-1.5h1.5v-2H9V10zM16 10a1.25 1.25 0 100 2.5 1.25 1.25 0 000-2.5zM18.5 12.5a1.25 1.25 0 100 2.5 1.25 1.25 0 000-2.5z',
  book: 'M5 3h12a2 2 0 012 2v16H7a2 2 0 01-2-2zM7 17v2h10v-2zM8 6v2h8V6z',
}

// Which of the shared avatar colours each colour name is.
const COLOURS: Record<string, number> = { red: 0, orange: 1, yellow: 2, green: 3, teal: 4, blue: 5, purple: 7, pink: 8 }

export const AVATAR_NAMES = Object.keys(SHAPES)
export const AVATAR_COLOUR_NAMES = Object.keys(COLOURS)

// What an avatar is called, for a screen reader choosing one.
export const avatarLabel = (name: string) => name[0].toUpperCase() + name.slice(1)

export const avatarColourOf = (colour: string) => AVATAR_COLORS[COLOURS[colour] ?? 5]

// A profile's choice, taken apart. Null for none, or for one this app doesn't know.
export function parseAvatar(avatar?: string): { name: string; colour: string; shape: string; background: string } | null {
  if (!avatar) return null
  const [name, colour] = avatar.split(':')
  if (!SHAPES[name] || COLOURS[colour] === undefined) return null
  return { name, colour, shape: SHAPES[name], background: avatarColourOf(colour) }
}

export function AvatarShape({ shape }: { shape: string }) {
  return <svg viewBox="0 0 24 24" width="58%" height="58%" fill="currentColor" aria-hidden="true"><path d={shape} /></svg>
}
