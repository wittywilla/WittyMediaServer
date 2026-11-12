// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The page spec: one data shape for the pages of docs/design-2 ("Every feature, drawn"). It mirrors the canvas's
// builders (mk, secR, secP, secN, secC, R, Cc, A1, A2 in docs/design-2/canvas/Exp.dc.html), so a drawn page and a
// built page are the same data. No colours and no layout live here: the active look and theme draw it.

// A line: a title, a value at its end, a smaller line under the title. `on` is the current choice (drawn with the
// accent and marked aria-current). It goes somewhere with `to`, or is handled by id (SpecPage's onAction).
// `icon` is a name from icons.tsx without its "Icon" prefix ("Users" is IconUsers); `tint` picks one of six tints the
// theme derives from its accent and status colours. `actions` are buttons at the row's end (Fix, Remove): the row
// itself then goes to `to` or is plain text.
export interface SpecRowAction { id?: string; title: string; label?: string; danger?: boolean; disabled?: boolean }
export interface SpecRow { id?: string; title: string; value?: string; sub?: string; on?: boolean; to?: string; disabled?: boolean; icon?: string; tint?: 'a' | 'b' | 'c' | 'd' | 'e' | 'f'; actions?: SpecRowAction[] }
// A choice among a few: a toggle button, or a link with `to`.
export interface SpecChip { id?: string; title: string; label?: string; on?: boolean; to?: string; disabled?: boolean }
// A tile: a title, a line under it, and either artwork (`art`, an address) or a tint pair taken from the theme.
export interface SpecCard { id?: string; title: string; sub?: string; on?: boolean; to?: string; art?: string; bg?: string; bg2?: string; disabled?: boolean }
// A form control. The page owns the value (it is in the spec, so a page that changes it builds the spec again) and
// hears changes through SpecPage's onField(id, value). `code` and `qr` only show: a code to read out, a QR to scan.
export type SpecFieldKind = 'time' | 'textarea' | 'text' | 'password' | 'number' | 'pin' | 'switch' | 'select' | 'slider' | 'code' | 'qr' | 'file'
export interface SpecField {
  id?: string
  kind: SpecFieldKind
  label: string
  value?: string | number | boolean
  hint?: string
  error?: string
  placeholder?: string
  required?: boolean
  disabled?: boolean
  options?: { value: string; label: string }[] // select
  min?: number; max?: number; step?: number; unit?: string // number, slider
  length?: number // pin: how many digits
  autoComplete?: string
  accept?: string // file
  autoFocus?: boolean
  // qr: what the picture says. code: the characters (value). Both need a words alternative.
  text?: string
}
export type SpecTone = 'info' | 'ok' | 'warn' | 'danger'

// A section may carry `anchor`, the DOM id of its heading (a page other code jumps to by section: Settings search),
// and a rows section may be a `choice`: one row of several is the setting (role radio, arrows move and choose).
export type SpecSection =
  | { kind: 'rows'; h: string; items: SpecRow[]; anchor?: string; choice?: boolean }
  | { kind: 'chips'; h: string; items: SpecChip[]; anchor?: string }
  | { kind: 'cards'; h: string; items: SpecCard[]; anchor?: string }
  | { kind: 'fields'; h: string; items: SpecField[]; anchor?: string }
  | { kind: 'note'; h: string; text: string; tone?: boolean; anchor?: string }

// A page's own buttons, at the Dock: one primary at most, in the order given.
export interface SpecAction { id?: string; title: string; kind: 'primary' | 'secondary'; to?: string; disabled?: boolean }

export interface SpecPageData {
  key: string
  eyebrow: string
  title: string
  lead: string
  sections: SpecSection[]
  actions: SpecAction[]
  foot: string
}
