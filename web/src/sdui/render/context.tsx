// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { createContext, useContext } from 'react'
import type { Device } from '../device'
import type { View } from '../model'
import type { Action, Document, Layout } from '../types'

// What every drawn component can ask of the page it is on.
export type PageCtx = {
  doc: Document
  layout: Layout
  device: Device
  tv: boolean
  touch: boolean
  reducedMotion: boolean
  // One point on this screen, in CSS pixels: a television's canvas is 1920 points wide
  // whatever the window is.
  pt: number
  // A static drawing with nothing to press: the editor's preview.
  inert?: boolean
  act: (a: Action | undefined, from?: View) => void
  // More of a section's items: its next page, a season's episodes, or another try.
  more: (sectionId: string, url?: string, replace?: boolean) => void
  busy: (sectionId: string) => boolean
  failed: (sectionId: string) => boolean
  setQuery: (change: Record<string, string>) => void
  columns: (gridId: string, n: number) => void
  menu: (card: View, at: { x: number; y: number }) => void
}

export const Ctx = createContext<PageCtx>(null!)
export const usePage = () => useContext(Ctx)

export const px = (points: number, pt: number) => Math.round(points * pt * 100) / 100 + 'px'
