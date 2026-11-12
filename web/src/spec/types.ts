// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The page spec: one data shape for the pages of docs/design-2 ("Every feature, drawn"). It mirrors the canvas's
// builders (mk, secR, secP, secN, secC, R, Cc, A1, A2 in docs/design-2/canvas/Exp.dc.html), so a drawn page and a
// built page are the same data. No colours and no layout live here: the active look and theme draw it.

// A line: a title, a value at its end, a smaller line under the title. `on` is the current choice (drawn with the
// accent and marked aria-current). It goes somewhere with `to`, or is handled by id (SpecPage's onAction).
export interface SpecRow { id?: string; title: string; value?: string; sub?: string; on?: boolean; to?: string; disabled?: boolean }
// A choice among a few: a toggle button, or a link with `to`.
export interface SpecChip { id?: string; title: string; on?: boolean; to?: string; disabled?: boolean }
// A tile: a title, a line under it, and either artwork (`art`, an address) or a tint pair taken from the theme.
export interface SpecCard { id?: string; title: string; sub?: string; on?: boolean; to?: string; art?: string; bg?: string; bg2?: string; disabled?: boolean }
export type SpecTone = 'info' | 'ok' | 'warn' | 'danger'

export type SpecSection =
  | { kind: 'rows'; h: string; items: SpecRow[] }
  | { kind: 'chips'; h: string; items: SpecChip[] }
  | { kind: 'cards'; h: string; items: SpecCard[] }
  | { kind: 'note'; h: string; text: string; tone?: boolean }

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
