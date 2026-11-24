// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { createContext, use, useContext } from 'react'
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
  // now: a person asked (Retry), so a section that failed is asked at once, whatever its back-off.
  more: (sectionId: string, url?: string, replace?: boolean, now?: boolean) => void
  busy: (sectionId: string) => boolean
  failed: (sectionId: string) => boolean
  setQuery: (change: Record<string, string>) => void
  columns: (gridId: string, n: number) => void
  menu: (card: View, at: { x: number; y: number }) => void
}

export const Ctx = createContext<PageCtx>(null!)
export const usePage = () => useContext(Ctx)

// What a card needs to be pressed, kept apart from the page: the page changes with every page of
// a grid's items, and every card reading it was drawn again each time (#643). The page gives this
// once; its act and menu always reach the page's latest handlers. Where nothing gives it (another
// renderer of the same components), it is read from the page.
export type PressCtx = { act: PageCtx['act']; menu: PageCtx['menu']; inert: boolean; externalLinks?: boolean }
export const PressContext = createContext<PressCtx | null>(null)
export function usePress(): PressCtx {
  const given = useContext(PressContext)
  if (given) return given
  const page = use(Ctx) // read only when nothing gave the press context (use, unlike useContext, may be)
  return { act: page.act, menu: page.menu, inert: !!page.inert, externalLinks: page.layout.externalLinks }
}

export const px = (points: number, pt: number) => Math.round(points * pt * 100) / 100 + 'px'
