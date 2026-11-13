// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What a shell around a page can tell the renderer (WittyPlay Online's frame does all three; with none given, everything draws as before):
// - how a button's pad key (a, b, x, y) is drawn: by default the letter in a circle; WittyPlay draws the buttons the person holds;
// - how a row whose focused card grows is laid out: a row sideways (shelf), a grid that scrolls down, or a list;
// - tabs of its own beside a page's tabs (links to its own pages), and something at the end of the tab row (a layout switch).
import { createContext, useContext, type ReactNode } from 'react'

export type GlyphKey = 'a' | 'b' | 'x' | 'y'
export const GlyphContext = createContext<((g: GlyphKey) => ReactNode) | null>(null)
export const useGlyph = () => useContext(GlyphContext)

export type RowLayout = 'shelf' | 'grid' | 'list'
export const RowLayoutContext = createContext<RowLayout>('shelf')

// page: the page whose first tab row gets them; after: the tab they follow.
// order and hidden: the person's own arrangement of the row (ids); a tab not named keeps its place after those that are, and the tab
// showing is never hidden.
export type ExtraTabs = { page: string; tabs: { id: string; label: string; to: string; after: string }[]; end?: ReactNode; order?: string[]; hidden?: string[] }
export const TabsContext = createContext<ExtraTabs | null>(null)
