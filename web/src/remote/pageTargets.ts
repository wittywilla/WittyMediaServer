// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The readers, the picture viewer and the game player as things another device steers
// (docs/REMOTE-PROTOCOL.md). None of them is a film, so each says honestly what it has:
//   a book     no time and nothing that plays: `paused`, with the reader's place and length as
//              the reader counts them (pages, or chapters for an ebook) in positionSec and
//              durationSec, the same numbers the server keeps as its progress. Next and
//              previous turn the page.
//   a picture  `playing` while a slideshow runs, else `paused`; no place, no length. Next and
//              previous step through the pictures; pause and resume are the slideshow's.
//   a game     `playing`, and stop. The game has the keys and the controller.
import { useRef } from 'react'
import { reportSoon, type RemoteCommand } from './device'
import { bookState, gameState, photoState } from './playerStates'
import { useRemoteTarget } from './target'

// A page turn, as every reader takes it whichever way its book reads: Page Down and Page Up.
const turn = (dir: 1 | -1) => document.body.dispatchEvent(new KeyboardEvent('keydown', { key: dir > 0 ? 'PageDown' : 'PageUp', bubbles: true, cancelable: true }))

// useBookRemote gives back what the reader's host calls when the place changes.
export function useBookRemote(itemId: number, start: number, close: () => void) {
  const place = useRef<{ position: number; total: number } | null>(null)
  useRemoteTarget({
    kind: 'book', itemId, on: itemId > 0,
    state: () => bookState(itemId, place.current || (start > 0 ? { position: start, total: 0 } : null)),
    command: (c: RemoteCommand) => {
      if (c.type === 'next') turn(1)
      else if (c.type === 'previous') turn(-1)
      else if (c.type === 'stop') close()
      else throw new Error('This device can’t do that.')
    },
  }, [itemId])
  return (position: number, total: number) => { place.current = { position, total }; reportSoon() }
}

export function usePhotoRemote(p: { id: number; slideshow: boolean; next: (() => void) | null; previous: (() => void) | null; setSlideshow: (on: boolean) => void; close: () => void }) {
  useRemoteTarget({
    kind: 'photo', itemId: p.id,
    state: () => photoState(p.id, p.slideshow),
    command: (c: RemoteCommand) => {
      switch (c.type) {
        case 'next': if (!p.next) throw new Error('That is the last picture.'); p.next(); break
        case 'previous': if (!p.previous) throw new Error('That is the first picture.'); p.previous(); break
        case 'pause': p.setSlideshow(false); break
        case 'resume': p.setSlideshow(true); break
        case 'stop': p.close(); break
        default: throw new Error('This device can’t do that.')
      }
    },
  }, [p.id, p.slideshow])
}

export function useGameRemote(itemId: number, close: () => void) {
  useRemoteTarget({
    kind: 'game', itemId, on: itemId > 0,
    state: () => gameState(itemId),
    command: (c: RemoteCommand) => { if (c.type === 'stop') close(); else throw new Error('This device can’t do that.') },
  }, [itemId])
}
