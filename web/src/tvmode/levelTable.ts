// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import table from '../../../pkg/sdui/data/tvlevels.json'
import { partFor, screenFor, type TvLevel, type TvLevelTable } from './levels'

// The server's table of television levels (pkg/sdui/data/tvlevels.json), built into the app: it
// comes from the same server that serves the app, so there is nothing to wait for before a screen
// knows whether to draw itself.
export const TV_LEVELS = table as TvLevelTable
export const TV_NOTE = TV_LEVELS.note

export const tvScreenFor = (path: string) => screenFor(TV_LEVELS, path)

export function tvPartLevel(id: string): TvLevel {
  return partFor(TV_LEVELS, id)?.level || 'full'
}

export const tvPartTitle = (id: string) => partFor(TV_LEVELS, id)?.title || ''
