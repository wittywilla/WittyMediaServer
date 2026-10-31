// A console's controller comes from the server as data, so a TV app and this player
// read the same thing (GET /api/consoles/{console}/controls, docs/GAMING-API.md). Only
// the RetroPad's button numbers are written down here.

export const SELECT = 2, START = 3, UP = 4, DOWN = 5, LEFT = 6, RIGHT = 7

export type Control = { key: string; id: number; label: string; kind: 'dpad' | 'face' | 'shoulder' | 'meta' }
export type PadBinding = { b: number } | { a: number; d: 1 | -1 } // a button, or an axis direction
export type PadMap = Record<number, PadBinding>
export type KeyMap = Record<number, string> // RetroPad id -> key name
export type Placement = { x: number; y: number; size: number; opacity: number; hidden?: boolean }
export type Layout = Record<string, Placement>
export type Orientation = 'landscape' | 'portrait'

export type ControlsData = {
  console: string
  pad?: { pointer?: boolean } // the console has a touch screen
  controls: Control[]
  defaults: { gamepad: PadMap; keyboard: KeyMap; remote: KeyMap; touch: Record<Orientation, Layout> }
  saved: { gamepad: Record<string, PadMap>; keyboard: KeyMap | null; touch: Partial<Record<Orientation, Layout>> }
}

export type SaveControl = { kind: 'gamepad' | 'keyboard' | 'touch'; key?: string; orientation?: Orientation; data: PadMap | KeyMap | Layout | null }

const DPAD = [{ id: UP, label: 'Up' }, { id: DOWN, label: 'Down' }, { id: LEFT, label: 'Left' }, { id: RIGHT, label: 'Right' }]

// Every single button, for the remap screen: the D-pad's directions, then the rest.
export const buttonsOf = (c: ControlsData) => [...DPAD, ...c.controls.filter((x) => x.kind !== 'dpad').map((x) => ({ id: x.id, label: x.label }))]

// A gamepad's saved layout is filed under its model name.
export const padKey = (id: string) => id.slice(0, 120)
