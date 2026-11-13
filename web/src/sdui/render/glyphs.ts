// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// How a button's pad key (a, b, x, y) is drawn. By default the letter in a circle; a shell that knows which buttons the person holds
// (WittyPlay Online: Keyboard, Xbox, PlayStation or Nintendo) gives its own drawing here.
import { createContext, useContext, type ReactNode } from 'react'

export type GlyphKey = 'a' | 'b' | 'x' | 'y'
export const GlyphContext = createContext<((g: GlyphKey) => ReactNode) | null>(null)
export const useGlyph = () => useContext(GlyphContext)
