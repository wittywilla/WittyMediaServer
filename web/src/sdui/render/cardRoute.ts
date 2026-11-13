// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where a card's link goes, as a frame around the page would have it: WittyPlay sends a game's card to the game's own page first
// (shell/playPaths.ts cardRoute). Outside a frame that changes it, a card goes where the server says.
import { createContext } from 'react'

export type CardRoute = (to: string) => string
export const CardRouteContext = createContext<CardRoute>((to) => to)
