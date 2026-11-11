// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A game controller's buttons and sticks, read with the Gamepad API, as the key presses the rest of
// the app understands. Nothing here touches the page, so the mapping is easy to test
// (gamepad.test.mjs); tv.ts reads the controllers and presses the keys.
//
// The browser's "standard" mapping covers XInput/Xbox, DualShock, DualSense and most generic
// pads; Nintendo layouts get A/B swapped so the right-hand button confirms; a few pads browsers
// have no layout for are laid out here (KNOWN_LAYOUTS); other non-standard pads fall back to their
// first axes and buttons.

// What a controller looks like to this code: the parts of the Gamepad API's Gamepad it reads.
export type Pad = { id: string; mapping: string; buttons: readonly { pressed: boolean }[]; axes: readonly number[] }

export const STD: Record<number, string> = {
  0: 'Enter', 1: 'Escape', 2: ' ', 3: 'm', // A/Cross, B/Circle, X/Square (play/pause), Y/Triangle (disc menu)
  4: 'PageUp', 5: 'PageDown', // bumpers: previous / next chapter, and in TV mode the tab to the left or right
  6: 'MediaRewind', 7: 'MediaFastForward', // triggers
  8: 't', 9: 'ContextMenu', // View/Share/Select: title menu, Menu/Options/Start: disc menu, and in TV mode the screen's options
  12: 'ArrowUp', 13: 'ArrowDown', 14: 'ArrowLeft', 15: 'ArrowRight',
}

// Nintendo's vendor id, as Chromium writes it ("Vendor: 057e") or Firefox does ("057e-2009-" or, on
// some systems, "57e-2009-" with a name such as "Wireless Gamepad").
export const isNintendo = (id: string) => /nintendo|pro controller|joy-?con|vendor: 057e|^0?57e-/i.test(id)

// Pads a browser has no layout for but that are known all the same: for each of their buttons, its
// place in the standard layout (-1 for none). Two Joy-Cons that Linux's joycond puts together as one
// pad come to Chromium like this, with the D-pad after everything else.
const KNOWN_LAYOUTS: [RegExp, number[]][] = [
  [/combined joy-?cons|057e\D+2008/i, [0, 1, 3, 2, -1, 4, 5, 6, 7, 8, 9, 16, 10, 11, 12, 13, 14, 15]],
]

// A D-pad some pads report as one axis: -1 at up, on round clockwise in eight steps, and
// out of that range at rest, which is what gives one away. Read as a stick it would
// look like a direction held down for good.
const HAT_KEYS: string[][] = [['ArrowUp'], ['ArrowUp', 'ArrowRight'], ['ArrowRight'], ['ArrowDown', 'ArrowRight'], ['ArrowDown'], ['ArrowDown', 'ArrowLeft'], ['ArrowLeft'], ['ArrowUp', 'ArrowLeft']]

export type PadOptions = {
  // Android's web view lays out every pad it has a name for the standard way, whether or not it calls that
  // layout standard: a DualSense reports no mapping there, with its D-pad at 12 to 15 all the same.
  android?: boolean
  // The pads already seen to have a D-pad on one axis. It's only seen at rest, so it's remembered.
  hats: Set<string>
}

const standardLayout = (p: Pad, o: PadOptions) => p.mapping === 'standard' || !!o.android
const knownLayout = (p: Pad, o: PadOptions) => (standardLayout(p, o) ? null : KNOWN_LAYOUTS.find(([id]) => id.test(p.id))?.[1] || null)

// padKeys: the keys the controllers are holding down at the moment.
export function padKeys(pads: readonly (Pad | null)[], o: PadOptions): Set<string> {
  const down = new Set<string>()
  for (const p of pads) {
    if (!p) continue
    const swap = isNintendo(p.id)
    const known = knownLayout(p, o)
    const standard = standardLayout(p, o) || !!known
    if (standard) {
      for (let i = 0; i < p.buttons.length; i++) {
        if (!p.buttons[i].pressed) continue
        const s = known ? known[i] ?? -1 : i
        let k = STD[s]
        if (swap && (s === 0 || s === 1)) k = STD[1 - s]
        if (k) down.add(k)
      }
    } else {
      // Unknown layout: first two buttons confirm/back, last axes pair is often the D-pad hat.
      if (p.buttons[0]?.pressed) down.add(swap ? 'Escape' : 'Enter')
      if (p.buttons[1]?.pressed) down.add(swap ? 'Enter' : 'Escape')
    }
    // Left stick (and D-pad axes on non-standard pads) as arrows, with a dead zone.
    const last = p.axes[p.axes.length - 1] ?? 0
    if (!standard && Math.abs(last) > 1.05) o.hats.add(p.id)
    const hat = o.hats.has(p.id)
    if (hat && Math.abs(last) <= 1.05) for (const k of HAT_KEYS[Math.round((last + 1) * 3.5) & 7]) down.add(k)
    const axes = standard || hat ? [[0, 1]] : [[0, 1], [p.axes.length - 2, p.axes.length - 1]]
    for (const [x, y] of axes) {
      const ax = p.axes[x] ?? 0, ay = p.axes[y] ?? 0
      if (Math.abs(ax) > 0.6 && Math.abs(ax) >= Math.abs(ay)) down.add(ax > 0 ? 'ArrowRight' : 'ArrowLeft')
      else if (Math.abs(ay) > 0.6) down.add(ay > 0 ? 'ArrowDown' : 'ArrowUp')
    }
  }
  return down
}
