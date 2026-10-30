// What each console's controller looks like. libretro speaks one controller, the
// RetroPad, so a console only needs its own names for RetroPad buttons, and which
// of them it has. The physical layout of the RetroPad is bottom B, right A,
// left Y, top X.

export const B = 0, Y = 1, SELECT = 2, START = 3, UP = 4, DOWN = 5, LEFT = 6, RIGHT = 7, A = 8, X = 9, L = 10, R = 11, L2 = 12, R2 = 13, L3 = 14, R3 = 15

export type PadButton = { id: number; label: string }
export type PadLayout = {
  face: PadButton[] // the main buttons, in the order they sit left to right
  shoulder?: PadButton[]
  meta: PadButton[] // Select, Start and their relatives
  sticks?: boolean // has analog sticks a core may read
}

const startSelect: PadButton[] = [{ id: SELECT, label: 'Select' }, { id: START, label: 'Start' }]
const startOnly: PadButton[] = [{ id: START, label: 'Start' }]

export const PADS: Record<string, PadLayout> = {
  nes: { face: [{ id: B, label: 'B' }, { id: A, label: 'A' }], meta: startSelect },
  snes: { face: [{ id: Y, label: 'Y' }, { id: X, label: 'X' }, { id: B, label: 'B' }, { id: A, label: 'A' }], shoulder: [{ id: L, label: 'L' }, { id: R, label: 'R' }], meta: startSelect },
  gb: { face: [{ id: B, label: 'B' }, { id: A, label: 'A' }], meta: startSelect },
  gba: { face: [{ id: B, label: 'B' }, { id: A, label: 'A' }], shoulder: [{ id: L, label: 'L' }, { id: R, label: 'R' }], meta: startSelect },
  md: { face: [{ id: Y, label: 'A' }, { id: B, label: 'B' }, { id: A, label: 'C' }], shoulder: [{ id: L, label: 'X' }, { id: X, label: 'Y' }, { id: R, label: 'Z' }], meta: [{ id: SELECT, label: 'Mode' }, { id: START, label: 'Start' }] },
  sms: { face: [{ id: B, label: '1' }, { id: A, label: '2' }], meta: [{ id: START, label: 'Pause' }] },
  gg: { face: [{ id: B, label: '1' }, { id: A, label: '2' }], meta: startOnly },
  ps1: { face: [{ id: Y, label: '□' }, { id: X, label: '△' }, { id: B, label: '✕' }, { id: A, label: '○' }], shoulder: [{ id: L, label: 'L1' }, { id: R, label: 'R1' }, { id: L2, label: 'L2' }, { id: R2, label: 'R2' }], meta: startSelect, sticks: true },
  pce: { face: [{ id: B, label: 'II' }, { id: A, label: 'I' }], meta: [{ id: SELECT, label: 'Select' }, { id: START, label: 'Run' }] },
  a2600: { face: [{ id: B, label: 'Fire' }], meta: [{ id: SELECT, label: 'Select' }, { id: START, label: 'Reset' }] },
  a7800: { face: [{ id: B, label: 'A' }, { id: A, label: 'B' }], meta: [{ id: SELECT, label: 'Select' }, { id: START, label: 'Pause' }] },
  lynx: { face: [{ id: B, label: 'B' }, { id: A, label: 'A' }], shoulder: [{ id: L, label: 'Opt 1' }, { id: R, label: 'Opt 2' }], meta: [{ id: START, label: 'Pause' }] },
  ws: { face: [{ id: B, label: 'B' }, { id: A, label: 'A' }], meta: startOnly },
  ngp: { face: [{ id: B, label: 'A' }, { id: A, label: 'B' }], meta: [{ id: START, label: 'Option' }] },
  vb: { face: [{ id: B, label: 'B' }, { id: A, label: 'A' }], shoulder: [{ id: L, label: 'L' }, { id: R, label: 'R' }], meta: startSelect },
  wittytest: { face: [{ id: Y, label: 'Y' }, { id: X, label: 'X' }, { id: B, label: 'B' }, { id: A, label: 'A' }], shoulder: [{ id: L, label: 'L' }, { id: R, label: 'R' }], meta: startSelect },
}

// The default for anything not listed: everything a RetroPad has.
const FULL: PadLayout = {
  face: [{ id: Y, label: 'Y' }, { id: X, label: 'X' }, { id: B, label: 'B' }, { id: A, label: 'A' }],
  shoulder: [{ id: L, label: 'L' }, { id: R, label: 'R' }, { id: L2, label: 'L2' }, { id: R2, label: 'R2' }],
  meta: startSelect, sticks: true,
}

export const padFor = (consoleId: string): PadLayout => PADS[consoleId] || FULL

// Every control of a console, for lists and the remap screen.
export type ControlInfo = { id: number; label: string }
export const DPAD: ControlInfo[] = [{ id: UP, label: 'Up' }, { id: DOWN, label: 'Down' }, { id: LEFT, label: 'Left' }, { id: RIGHT, label: 'Right' }]
export function controlsOf(consoleId: string): ControlInfo[] {
  const p = padFor(consoleId)
  return [...DPAD, ...p.face, ...(p.shoulder || []), ...p.meta]
}
