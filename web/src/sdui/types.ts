// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What the server sends for a server-driven page (docs/SDUI.md, catalogue revision 5).
// Plain data: nothing here touches the page.

export const CATALOG = 6
export const VERSION = 1

export type Props = Record<string, unknown>

export type Action = {
  type: string
  to?: string
  item?: string
  url?: string
  op?: string
  params?: Record<string, unknown>
}

export type Focus = { order: number; group?: string; default?: boolean }

export type Node = {
  type: string
  id: string
  props?: Props
  children?: Node[]
  action?: Action
  focus?: Focus
  state?: string
  scope?: string
  tokens?: Record<string, string>
}

export type CardMetrics = { shape?: string; aspect?: string; width?: Record<string, number>; columns?: number }

// Shell is the layout option a page was made for and how the app's frame is drawn with
// it (docs/SDUI.md, "Layout options").
export type Shell = { experience: string; nav: string; overlay?: boolean; size: number; open?: number; drawer?: number; placements?: Record<string, { position?: string; visible?: boolean; style?: string }>
  // The module scope's destinations for the circular dock (region systemDock): where a game console's home goes.
  systemDock?: { id: string; label: string; icon?: string; route: string }[] }

export type Layout = {
  shell?: Shell
  formFactor: string
  class?: string
  canvas?: { width: number; height: number }
  scale?: number
  breakpoint: string
  safeArea?: { top: number; right: number; bottom: number; left: number }
  focusEngine?: boolean
  peek?: number
  dock?: { placement: string; height: number; hintSize: number }
  density?: string
  spacing: Record<string, number>
  text: Record<string, number>
  cards?: Record<string, CardMetrics>
  hero?: { width: number; height: number }
  imageWidths?: number[]
  budget?: { maxSections: number; eagerSections: number; shelfItems: number; gridItems: number; listItems: number; pageSize: number; heroItems: number; repeats: number }
  textLimits?: Record<string, number>
  externalLinks?: boolean
  reducedMotion?: boolean
  truncated?: boolean
}

export type PageInfo = { id: string; kind: string; title: string; icon?: string; route?: string; variant?: string; params?: Record<string, string> }

export type PersonalSection = { id: string; title: string; hidden?: boolean; pinned?: boolean; locked?: boolean; empty?: boolean; cardSize?: string; tabs?: { id: string; label: string }[] }

export type PersonalInfo = {
  editable: boolean
  allow?: string[]
  sections?: PersonalSection[]
  density?: string
  cardSize?: Record<string, string>
  defaultTab?: Record<string, string>
  url?: string
}

// What a profile changed about one page: the body of PUT …/personal.
export type Personal = {
  order?: string[]
  hidden?: string[]
  pinned?: string[]
  density?: string
  cardSize?: Record<string, string>
  defaultTab?: Record<string, string>
}

export type ScreenTime = { reason?: string; message?: string; nextAt?: number }

export type Meta = { etag?: string; generatedAt?: string; source?: string; configRevision?: number; ttlSeconds?: number }

export type Document = {
  version: number
  catalog: number
  page: PageInfo
  layout: Layout
  theme?: { scope?: string; tokens?: Record<string, string> }
  root: Node
  screenTime?: ScreenTime
  personal?: PersonalInfo
  meta: Meta
}

export type SectionItems = { version: number; section: string; items?: Node[]; next?: string; replace?: boolean; props?: Props; state?: string }

export type Patch = { op: string; path: string; node?: Node; value?: unknown; from?: string }

export type NavItem = {
  id: string
  label: string
  icon?: string
  kind: string // page | native | route
  page?: string
  screen?: string
  params?: Record<string, string>
  route: string
  placement: string // primary | more | end
  // A library's second column beside a rail with a drawer: its name, icon and count, and its views.
  drawer?: NavDrawer
}
export type NavDrawer = { title: string; icon: string; count?: number; views: { id: string; label: string; icon: string; route: string }[] }

// experiences: what this profile may choose on this device, for the Look row in Settings (docs/SDUI.md, "Experiences").
export type Required = { value: string; by?: string; at?: string; scope: string }
export type ExperienceChoice = { current: string; chosen?: string; options: { value: string; label: string }[]; url: string; required?: Required }
export type Navigation = { version: number; formFactor: string; etag?: string; items: NavItem[]; personal?: PersonalInfo; shell?: Shell; experiences?: ExperienceChoice }

// The spec's fallback profile, for a document that came without one: the web's.
export const WEB_LAYOUT: Layout = {
  formFactor: 'web', breakpoint: 'large', scale: 2, density: 'comfortable',
  safeArea: { top: 0, right: 0, bottom: 0, left: 0 },
  spacing: { none: 0, xs: 4, s: 8, m: 16, l: 24, xl: 48 },
  text: { display: 28, pageTitle: 28, heading: 20, title: 16, body: 14, caption: 12, badge: 11 },
  budget: { maxSections: 30, eagerSections: 8, shelfItems: 20, gridItems: 60, listItems: 50, pageSize: 48, heroItems: 6, repeats: 16 },
  imageWidths: [120, 160, 240, 320, 480, 640, 960, 1280, 1920, 2560, 3840],
  externalLinks: true,
}
