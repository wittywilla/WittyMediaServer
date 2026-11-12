// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The renderer's pure layout step: a document from the server becomes a tree of views,
// each saying what is drawn (which text parts, how many lines, which picture, how big the
// cards are, how many placeholders) and what assistive technology is told. The React
// components in render/ take their props from these views and decide nothing of their
// own, so the conformance fixtures (model.test.mjs) check what is really drawn.
// Rules: docs/SDUI.md, "Rules for renderers".

import { cardStyle, UNKNOWN_STYLE } from './cardStyles.ts'
import { WEB_LAYOUT, type Action, type Document, type Layout, type Node, type Props, type Shell } from './types.ts'

export type View = {
  id: string
  // The component it is drawn as: an unknown type is a Stack.
  type: string
  depth: number
  role: string
  label: string
  state?: string
  text?: Record<string, string>
  lines?: Record<string, number>
  textSize?: number
  cards?: number
  placeholders?: number
  cardStyle?: string
  cardWidth?: number
  cardHeight?: number
  columns?: number
  hasMore?: boolean
  seeAll?: string
  seeAllCard?: boolean
  peek?: number
  image?: string
  shows?: string[]
  progress?: number
  buttons?: string[]
  action?: string
  scope?: string
  node: Node
  children: View[]
}

export type PageFrame = { title: string; formFactor: string; breakpoint: string; density: string; contentWidth: number; sectionGap: number; scope?: string; shell?: Shell }
export type FocusModel = { order: string[]; default: string; groups: Record<string, string[]> }
export type PageModel = { frame: PageFrame; root: View; focus: FocusModel; layout: Layout }

const str = (p: Props | undefined, k: string): string => (p && typeof p[k] === 'string' ? p[k] as string : '')
const num = (p: Props | undefined, k: string): number | undefined => (p && typeof p[k] === 'number' ? p[k] as number : undefined)
const obj = (p: Props | undefined, k: string): Props | undefined => (p && p[k] && typeof p[k] === 'object' && !Array.isArray(p[k]) ? p[k] as Props : undefined)
const list = (p: Props | undefined, k: string): unknown[] => (p && Array.isArray(p[k]) ? p[k] as unknown[] : [])
const join = (...parts: string[]) => parts.filter(Boolean).join(', ')

// contentWidth: the width sections have to themselves, in points.
export function contentWidth(l: Layout): number {
  const fallback: Record<string, number> = { compact: 390, medium: 720, expanded: 1024, large: 1440 }
  const width = l.canvas && l.canvas.width > 0 ? l.canvas.width : fallback[l.breakpoint] || 1440
  const safe = l.safeArea ? l.safeArea.left + l.safeArea.right : 0
  return safe > 0 ? width - safe : width - 2 * (l.spacing.m || 0)
}

// cardGap: the distance between cards in a row or grid.
export const cardGap = (l: Layout): number => (l.breakpoint === 'compact' ? (l.spacing.s || 0) + (l.spacing.xs || 0) : l.spacing.m || 0)

export const sectionGap = (l: Layout): number => (l.breakpoint === 'compact' ? l.spacing.m : l.spacing.l) || 0

// placeholders: how many grey cards a loading row shows: those at least partly on
// screen, and no more than the row's first page.
export function placeholders(l: Layout, cardWidth: number): number {
  if (cardWidth <= 0) return 0
  const gap = cardGap(l)
  const n = Math.floor((contentWidth(l) + gap) / (cardWidth + gap)) + 1
  return Math.max(1, Math.min(n, l.budget?.shelfItems ?? n))
}

// gridColumns: for a window that changed size without crossing a breakpoint. `content`
// is the width the grid really has.
export function gridColumns(l: Layout, cardWidth: number, content: number, sent?: number): number {
  if (l.focusEngine || l.formFactor === 'tv') return sent || 1
  if (cardWidth <= 0) return sent || 1
  const gap = cardGap(l)
  return Math.max(cardWidth < 130 ? 3 : 2, Math.floor((content + gap) / (cardWidth + gap)))
}

export function describeAction(a: Action | undefined): string {
  if (!a) return ''
  switch (a.type) {
    case 'navigate': case 'native': return a.type + ' ' + (a.to || '')
    case 'play': return 'play ' + (a.item || '') + (a.params?.resume === false ? ' from the start' : '')
    case 'external': return 'external ' + (a.url || '')
    case 'command': {
      let s = 'command ' + (a.op || '')
      if (a.item) s += ' ' + a.item
      const tab = a.params?.tab
      if (typeof tab === 'string' && tab) s += ' ' + tab
      return s
    }
  }
  return a.type
}

const imageOf = (p: Props | undefined, k: string): string => {
  const m = obj(p, k)
  return m ? str(m, 'kind') + ' ' + Math.trunc(num(m, 'width') || 0) : ''
}

const percent = (p: Props | undefined): number | undefined => {
  const f = num(p, 'progress')
  return f === undefined ? undefined : Math.trunc(f * 100 + 0.5)
}

// plainText: Markdown as assistive technology reads it: links by their text, marks
// removed, white space folded.
export function plainText(s: string): string {
  return s.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[*_`#>]+/g, '').replace(/\s+/g, ' ').trim()
}

export type ButtonSpec = { id: string; label: string; icon?: string; primary?: boolean; action?: Action }

export function buttonsOf(p: Props | undefined): ButtonSpec[] {
  return list(p, 'actions').map((a) => {
    const m = (a || {}) as Props
    return { id: str(m, 'id') || str(m, 'label'), label: str(m, 'label'), icon: str(m, 'icon') || undefined, primary: m.primary === true, action: m.action as Action | undefined }
  })
}

const buttonLabels = (p: Props | undefined) => buttonsOf(p).map((b) => (b.primary ? '*' : '') + b.label)

export const factsOf = (p: Props | undefined): string[] => list(p, 'facts').map((f) => str(f as Props, 'value'))

export type Option = { value: string; label: string }
export type FilterField = { id: string; label: string; value: string; options: Option[] }

export function fieldOf(m: Props | undefined, id = ''): FilterField {
  return {
    id: str(m, 'id') || id, label: str(m, 'label'), value: str(m, 'value'),
    options: list(m, 'options').map((o) => ({ value: str(o as Props, 'value'), label: str(o as Props, 'label') })),
  }
}

const optionLabel = (f: FilterField) => f.options.find((o) => o.value === f.value)?.label || ''

export function spokenDuration(sec: number): string {
  const m = Math.trunc(sec / 60), s = sec % 60
  if (m === 0) return `${s} seconds`
  if (s === 0) return `${m} minutes`
  return `${m} minutes ${s} seconds`
}

export function clock(sec: number): string {
  const two = (n: number) => String(n).padStart(2, '0')
  if (sec >= 3600) return `${Math.trunc(sec / 3600)}:${two(Math.trunc(sec % 3600 / 60))}:${two(sec % 60)}`
  return `${Math.trunc(sec / 60)}:${two(sec % 60)}`
}

// cardLabel: a card's accessibility label: the parts its style shows, in the style's order.
export function cardLabel(p: Props): string {
  const parts: string[] = []
  const rank = num(p, 'rank')
  if (rank !== undefined) parts.push('Number ' + Math.trunc(rank))
  const shows = cardStyle(p.style)?.shows ?? ['title', 'subtitle']
  if (!shows.includes('title')) parts.push(str(p, 'title')) // a photo's name is read, though not drawn
  for (const s of shows) {
    switch (s) {
      case 'title': case 'subtitle': case 'remaining': parts.push(str(p, s)); break
      case 'number': if (str(p, s)) parts.push('Channel ' + str(p, s)); break
      case 'now': case 'next': { const m = obj(p, s); if (m) parts.push(s + ' ' + str(m, 'title')); break }
      case 'progress': { const pc = percent(p); if (pc !== undefined && !obj(p, 'now')) parts.push(pc + ' percent watched'); break }
      case 'watched': if (p.watched === true) parts.push('Watched'); break
      case 'unwatched': { const c = num(p, s); if (c !== undefined) parts.push(Math.trunc(c) + ' unwatched'); break }
      case 'count': { const c = num(p, s); if (c !== undefined) parts.push(Math.trunc(c) + ' items'); break }
      case 'badges': for (const b of list(p, s)) if (typeof b === 'string') parts.push(b); break
    }
  }
  return join(...parts)
}

function cardView(e: View, x: Node, parent: Node | undefined) {
  const p = x.props || {}
  e.role = 'button'
  e.image = imageOf(p, 'image') || undefined
  if (parent?.type === 'Hero') {
    // A slide of a Hero: a group, not a button; its buttons are listed.
    e.role = 'group'
    e.label = join(str(p, 'title'), factsOf(p).join(', '))
    e.text = { title: str(p, 'title') }
    const f = factsOf(p).join(' · ')
    if (f) e.text.facts = f
    if (str(p, 'synopsis')) e.text.synopsis = str(p, 'synopsis')
    e.lines = { title: 2, synopsis: 3 }
    e.buttons = buttonLabels(p)
    e.action = undefined
    return
  }
  e.label = cardLabel(p)
  const style = cardStyle(p.style) ?? UNKNOWN_STYLE
  const titles = parent?.props?.showTitles !== false
  const text: Record<string, string> = {}, lines: Record<string, number> = {}, shows: string[] = []
  for (const s of style.shows) {
    switch (s) {
      case 'title': case 'subtitle':
        if (titles && str(p, s)) { text[s] = str(p, s); shows.push(s); lines[s] = s === 'title' ? style.titleLines : style.subtitleLines }
        break
      case 'synopsis': if (str(p, s)) { text[s] = str(p, s); lines[s] = 2; shows.push(s) } break
      case 'remaining': case 'number': if (str(p, s)) { text[s] = str(p, s); shows.push(s) } break
      case 'now': case 'next': { const m = obj(p, s); if (m) { text[s] = str(m, 'title'); lines[s] = 1; shows.push(s) } break }
      case 'progress': { const pc = percent(p); if (pc !== undefined) { e.progress = pc; shows.push(s) } break }
      case 'watched': if (p.watched === true) shows.push(s); break
      case 'unwatched': case 'count': { const c = num(p, s); if (c !== undefined) { text[s] = String(Math.trunc(c)); shows.push(s) } break }
      case 'badges': { const b = list(p, s).filter((v): v is string => typeof v === 'string'); if (b.length) { text[s] = b.join(' '); shows.push(s) } break }
    }
  }
  const rank = num(p, 'rank')
  if (rank !== undefined) { text.rank = String(Math.trunc(rank)); shows.unshift('rank') }
  if (!e.image && str(p, 'icon')) e.image = 'icon ' + str(p, 'icon')
  // A line limit of nothing is no limit.
  for (const k of Object.keys(lines)) if (lines[k] <= 0) delete lines[k]
  e.text = Object.keys(text).length ? text : undefined
  e.lines = Object.keys(lines).length ? lines : undefined
  e.shows = shows.length ? shows : undefined
}

function viewOf(x: Node, depth: number, parent: Node | undefined, l: Layout, pageTitle: string): View {
  const p = x.props || {}
  const e: View = { id: x.id, type: x.type, depth, role: 'none', label: '', state: x.state || undefined, scope: x.scope || undefined, action: describeAction(x.action) || undefined, node: x, children: [] }
  const text = (k: string, v: string) => { if (v) (e.text ||= {})[k] = v }
  const lines = (k: string, v: number) => { if (v > 0) (e.lines ||= {})[k] = v }
  const kids = x.children?.length || 0
  const collection = () => {
    e.cards = kids
    const c = obj(p, 'card')
    if (c) {
      e.cardStyle = str(p, 'cardStyle') || undefined
      e.cardWidth = Math.trunc(num(c, 'width') || 0)
      e.cardHeight = Math.trunc(num(c, 'height') || 0)
      if (x.state === 'deferred') e.placeholders = placeholders(l, e.cardWidth)
    }
    const cols = num(p, 'columns')
    if (cols !== undefined) e.columns = Math.trunc(cols)
    e.hasMore = !!obj(p, 'more') && x.state !== 'deferred' && x.state !== 'error'
    const sa = obj(p, 'seeAll')
    if (sa) { e.seeAll = str(sa, 'route') || undefined; if (p.seeAllCard === true) e.seeAllCard = true }
    if (x.type === 'Shelf' && p.peek !== false && l.peek) e.peek = l.peek
    text('title', str(p, 'title'))
    text('subtitle', str(p, 'subtitle'))
    text('empty', str(p, 'emptyText'))
    lines('title', 1)
    e.textSize = l.text.title
  }
  switch (x.type) {
    case 'Page': e.role = 'main'; e.label = str(p, 'title'); break
    case 'Shelf': case 'Carousel': collection(); e.role = 'list'; e.label = str(p, 'title'); break
    case 'Grid':
      collection()
      e.role = !l.focusEngine && l.formFactor !== 'desktop' && l.formFactor !== 'web' ? 'list' : 'grid'
      e.label = str(p, 'title') || pageTitle
      break
    case 'List':
      e.cards = kids; e.role = 'list'; e.label = str(p, 'title')
      text('title', str(p, 'title')); text('empty', str(p, 'emptyText'))
      e.hasMore = !!obj(p, 'more')
      break
    case 'Card': cardView(e, x, parent); break
    case 'Hero': {
      e.role = 'region'; e.label = str(p, 'title') || 'Featured'; e.cards = kids
      const size = obj(p, 'size')
      if (size) { e.cardWidth = Math.trunc(num(size, 'width') || 0); e.cardHeight = Math.trunc(num(size, 'height') || 0) }
      if (p.peek === true && l.peek) e.peek = l.peek
      const adv = num(p, 'autoAdvanceSec')
      if (adv !== undefined) text('autoAdvance', Math.trunc(adv) + ' s')
      break
    }
    case 'Dock': {
      e.role = 'toolbar'; e.label = str(p, 'title') || 'Options'
      e.cards = kids
      e.buttons = (x.children || []).filter((c) => c.type === 'Button').map((c) => str(c.props, 'label'))
      e.cardHeight = l.dock?.height
      text('placement', str(p, 'placement'))
      break
    }
    case 'KeyLegend':
      text('keys', list(p, 'entries').map((en) => str(en as Props, 'label')).join(' · '))
      e.textSize = l.dock?.hintSize
      break
    case 'WidgetGrid': e.role = 'group'; e.label = pageTitle; e.columns = Math.trunc(num(p, 'columns') ?? 4); e.cards = kids; break
    case 'Widget': {
      e.role = 'region'; e.label = str(p, 'title'); text('title', str(p, 'title')); text('kind', str(p, 'kind'))
      e.columns = Math.trunc(num(p, 'w') || 0); e.cardHeight = Math.trunc(num(p, 'h') || 0); e.cards = kids
      break
    }
    case 'NumberedRow':
      e.cards = kids; e.role = 'list'; e.label = str(p, 'title')
      text('title', str(p, 'title')); text('empty', str(p, 'emptyText'))
      e.hasMore = !!obj(p, 'more')
      text('numbers', (x.children || []).map((c) => str(c.props, 'number')).join(' '))
      break
    case 'Tabs': case 'CrossBar': e.role = 'tablist'; e.cards = kids; break
    case 'Tab':
      e.role = 'tab'; e.label = str(p, 'label') + (p.selected === true ? ', selected' : '')
      text('title', str(p, 'label')); lines('title', 1)
      break
    case 'FilterBar': {
      e.role = 'toolbar'; e.label = 'Filter and sort'
      const b: string[] = []
      const sort = obj(p, 'sort')
      if (sort) b.push('Sort: ' + optionLabel(fieldOf(sort)))
      for (const f of list(p, 'fields')) { const fl = fieldOf(f as Props); b.push(fl.label + ': ' + (optionLabel(fl) || 'Any')) }
      if (b.length) e.buttons = b
      const c = num(p, 'count')
      if (str(p, 'countText')) text('count', str(p, 'countText'))
      else if (c !== undefined) text('count', Math.trunc(c) + ' titles')
      break
    }
    case 'DetailHeader': {
      e.role = 'region'; e.label = str(p, 'title')
      text('title', str(p, 'title')); text('tagline', str(p, 'tagline')); text('facts', factsOf(p).join(' · ')); text('overview', str(p, 'overview'))
      lines('title', 2); lines('overview', 4)
      e.textSize = l.text.pageTitle
      const b = buttonLabels(p)
      if (b.length) e.buttons = b
      e.progress = percent(p)
      const im = obj(p, 'images')
      if (im) e.image = join(imageOf(im, 'backdrop'), imageOf(im, 'poster')) || undefined
      break
    }
    case 'SeasonList': {
      e.role = 'region'; e.label = 'Episodes'; e.cards = kids
      const b = list(p, 'seasons').map((s) => ((s as Props).selected === true ? '*' : '') + str(s as Props, 'title'))
      if (b.length) e.buttons = b
      e.hasMore = !!obj(p, 'more')
      const c = obj(p, 'card')
      if (c) { e.cardStyle = 'episode'; e.cardWidth = Math.trunc(num(c, 'width') || 0); e.cardHeight = Math.trunc(num(c, 'height') || 0) }
      break
    }
    case 'TrackList': e.role = 'list'; e.label = 'Tracks'; e.cards = kids; break
    case 'Credits': {
      // The cast's and guests' portraits, then each department's people; the parts in order with
      // how many of each were sent, and Show all when a part holds more (docs/SDUI.md, "Cast and crew").
      e.role = 'region'; e.label = str(p, 'title'); e.cards = kids
      text('title', str(p, 'title')); lines('title', 1)
      e.textSize = l.text.heading
      text('groups', creditGroups(p).map((g) => g.label + ' ' + g.shown + (g.total > g.shown ? ' of ' + g.total : '')).join(' · '))
      const c = obj(p, 'card')
      if (c) { e.cardStyle = 'person'; e.cardWidth = Math.trunc(num(c, 'width') || 0); e.cardHeight = Math.trunc(num(c, 'height') || 0) }
      const more = obj(p, 'more')
      if (more) { e.hasMore = true; e.buttons = [str(more, 'label') || 'Show all'] }
      break
    }
    case 'Track': {
      e.role = 'button'
      const parts: string[] = []
      const n = num(p, 'number')
      if (n !== undefined && parent?.props?.showNumbers !== false) parts.push('Track ' + Math.trunc(n))
      parts.push(str(p, 'title'))
      text('title', str(p, 'title')); lines('title', 1)
      if (parent?.props?.showArtist === true) { parts.push(str(p, 'artist')); text('subtitle', str(p, 'artist')) }
      const d = num(p, 'durationSec')
      if (d !== undefined && parent?.props?.showDuration !== false) { parts.push(spokenDuration(Math.trunc(d))); text('duration', clock(Math.trunc(d))) }
      e.label = join(...parts)
      e.progress = percent(p)
      break
    }
    case 'Banner':
      e.role = x.action ? 'button' : 'group'
      e.label = join(str(p, 'title'), str(p, 'text'), str(p, 'buttonLabel'))
      text('title', str(p, 'title')); text('text', str(p, 'text')); text('button', str(p, 'buttonLabel'))
      lines('title', 2); lines('text', 2)
      e.image = imageOf(p, 'image') || undefined
      break
    case 'Text': {
      const role = str(p, 'role') || 'body'
      e.textSize = l.text[role]
      e.role = ({ pageTitle: 'heading 1', heading: 'heading 2', title: 'heading 3' } as Record<string, string>)[role] || 'text'
      const shown = str(p, 'text')
      e.label = str(p, 'format') === 'markdown' ? plainText(shown) : shown
      text('text', shown)
      const ml = num(p, 'maxLines')
      if (ml !== undefined) lines('text', Math.trunc(ml))
      break
    }
    case 'Notice': {
      const tone = str(p, 'tone')
      e.role = tone === 'warning' || tone === 'danger' ? 'alert' : 'status'
      e.label = (str(p, 'title') + '. ' + str(p, 'text')).replace(/^\. /, '')
      text('title', str(p, 'title')); text('text', str(p, 'text')); text('button', str(p, 'buttonLabel'))
      break
    }
    case 'Button': e.role = 'button'; e.label = str(p, 'label'); text('title', str(p, 'label')); lines('title', 1); break
    case 'Image':
      e.image = imageOf(p, 'image') || undefined
      if (str(p, 'alt') || x.action) { e.role = 'img'; e.label = str(p, 'alt') }
      break
    case 'SearchBox': e.role = 'searchbox'; e.label = 'Search'; text('placeholder', str(p, 'placeholder')); text('query', str(p, 'query')); break
    case 'ListItem':
      e.role = x.action ? 'button' : 'text'
      e.label = join(str(p, 'title'), str(p, 'subtitle'), str(p, 'trailing'))
      text('title', str(p, 'title')); text('subtitle', str(p, 'subtitle')); text('trailing', str(p, 'trailing'))
      lines('title', 1); lines('subtitle', 1)
      e.image = imageOf(p, 'image') || undefined
      break
    case 'Divider': e.role = 'separator'; break
    case 'NativeView': e.role = 'group'; e.label = str(p, 'view'); break
    case 'Stack': case 'Split': case 'Spacer': break
    // A type this catalogue doesn't have is drawn as a plain column of its children.
    default: e.type = 'Stack'
  }
  e.children = (x.children || []).map((c) => viewOf(c, depth + 1, x, l, pageTitle))
  return e
}

// initials: the first letters of a person's first and last names, "AL" for Ana Lima.
export function initials(name: string): string {
  const words = name.replace(/\(.*?\)/g, ' ').split(/[\s\-–]+/).filter((w) => /\p{L}|\p{N}/u.test(w))
  if (!words.length) return name.slice(0, 1).toUpperCase()
  const first = (w: string) => Array.from(w.replace(/^[^\p{L}\p{N}]+/u, ''))[0] || ''
  return (first(words[0]) + (words.length > 1 ? first(words[words.length - 1]) : '')).toUpperCase()
}

export type CreditGroup = { id: string; label: string; role: string; total: number; shown: number }

// creditGroups: a Credits section's parts in order: cast, guests, then each department.
export function creditGroups(p: Props): CreditGroup[] {
  return list(p, 'groups').map((g) => {
    const m = g as Props
    return { id: str(m, 'id'), label: str(m, 'label'), role: str(m, 'role'), total: Math.trunc(num(m, 'total') || 0), shown: Math.trunc(num(m, 'shown') || 0) }
  }).filter((g) => g.id !== '')
}

// focusModel: everything that can take focus, in the order Tab visits it, where focus
// starts, and each focus group's members.
export function focusModel(root: Node): FocusModel {
  const byOrder = new Map<number, Node>()
  const collect = (x: Node) => { if (x.focus) byOrder.set(x.focus.order, x); x.children?.forEach(collect) }
  collect(root)
  const out: FocusModel = { order: [], default: '', groups: {} }
  for (let i = 1; i <= byOrder.size; i++) {
    const x = byOrder.get(i)
    if (!x?.focus) continue
    out.order.push(x.id)
    if (x.focus.default) out.default = x.id
    if (x.focus.group) (out.groups[x.focus.group] ||= []).push(x.id)
  }
  return out
}

// buildPage: the whole of a document as the renderer lays it out.
export function buildPage(doc: Document): PageModel {
  const l = doc.layout || WEB_LAYOUT
  const frame: PageFrame = {
    title: doc.page?.title || '', formFactor: l.formFactor, breakpoint: l.breakpoint, density: l.density || '',
    contentWidth: contentWidth(l), sectionGap: sectionGap(l), scope: doc.theme?.scope || undefined,
  }
  if (l.shell) frame.shell = l.shell
  return { frame, root: viewOf(doc.root, 0, undefined, l, frame.title), focus: focusModel(doc.root), layout: l }
}

// rebuildSection: one section's views again, after more of its items arrived.
export function buildView(x: Node, depth: number, parent: Node | undefined, l: Layout, pageTitle: string): View {
  return viewOf(x, depth, parent, l, pageTitle)
}

// normalise: the model in the conformance kit's shape (pkg/sdui/conformance/README.md).
export function normalise(m: PageModel): unknown {
  const visible: unknown[] = []
  const walk = (v: View) => {
    const { node: _node, children, ...rest } = v
    visible.push(rest)
    children.forEach(walk)
  }
  walk(m.root)
  return { kit: 1, page: m.frame, visible, focus: m.focus }
}

// applyItems: a section's node after a page of items arrived (docs/SDUI.md, "Paging"):
// appended, or in place of the ones there; its props as they are now; and where the
// next page is. An answer whose state is `empty` takes the section off the page: null.
export function applyItems(section: Node, answer: { items?: Node[]; next?: string; replace?: boolean; props?: Props; state?: string }): Node | null {
  if (answer.state === 'empty' && !(answer.items?.length)) {
    const keeps = answer.props && typeof answer.props.emptyText === 'string' && answer.props.emptyText !== ''
    if (!keeps) return null
  }
  const had = section.state === 'deferred' || section.state === 'error' || answer.replace ? [] : section.children || []
  const props: Props = { ...(section.props || {}), ...(answer.props || {}) }
  if (answer.next) props.more = { url: answer.next }
  else delete props.more
  // Cards that arrive later join the section's focus group at the end, in the order sent.
  const arrived = (answer.items || []).map((c) => (c.focus || !c.action ? c : { ...c, focus: { order: 0, group: section.id } }))
  const next: Node = { ...section, props, children: [...had, ...arrived] }
  if (answer.state === 'empty' || answer.state === 'error') next.state = answer.state
  else delete next.state
  return next
}
