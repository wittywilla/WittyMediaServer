// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The thinking behind the editor of server-driven pages (Settings, Pages & menu), with
// nothing here touching the page: what the palette offers, which fields a component, a
// source or a condition gets, moving sections about, where a validator's issue points
// and whether its fix can be done by a button, what changed between two revisions, and
// undo. Everything about a component, a source or a condition is read from the
// catalogue the server sends (GET /sdui/catalog): this file names none of them, so a
// component added to the server appears in the editor by itself. docs/SDUI.md, "The
// editor's contract".

// ---------------------------------------------------------------- the catalogue

export type Option = { value: string; label: string }

export type PropSpec = {
  name: string; type: string; label: string; help?: string; default?: unknown; required?: boolean
  options?: Option[] | null; min?: number; max?: number; maxLength?: number; formFactors?: string[] | null; since?: number
}

export type ComponentSpec = {
  type: string; label: string; description: string; icon: string; category: string; authored: boolean
  data: string; accepts?: string[] | null; container: boolean; children?: string[] | null; formFactors?: string[] | null
  props?: PropSpec[] | null; renders?: Record<string, string> | null; states?: Record<string, string> | null; a11y?: string; example?: unknown
}

export type SourceSpec = {
  type: string; label: string; description: string; icon: string; category: string; params?: PropSpec[] | null
  yields?: string[] | null; cardStyle?: string; pageKinds?: string[] | null; paged?: boolean; seeAll?: string; title?: string | null; example?: unknown
}

export type ConditionSpec = PropSpec & { field: string }

export type PageKindSpec = { kind: string; label: string; description: string; route: string; params?: string[] | null; variables?: string[] | null; match?: string[] | null }

export type NativeScreenSpec = { screen: string; label: string; description: string; route: string; icon: string; menu?: boolean }

export type Catalogue = {
  schemaVersion?: number; revision?: number
  components: ComponentSpec[]; sources: SourceSpec[]; conditions: ConditionSpec[]; pageKinds: PageKindSpec[]
  nativeScreens?: NativeScreenSpec[]; nativeViews?: Option[]; icons?: string[]; placeholders?: Option[]; repeats?: Option[]
  formFactors?: string[]; platforms?: string[]; profileKinds?: string[]; libraryKinds?: string[]; mediaKinds?: string[]
  personalKinds?: Option[]; placements?: Option[]; spacing?: string[]; limits?: Record<string, number>
  // Not sent today. If a later server lists what a source's map may pair, the editor offers them.
  mapProps?: string[]; mapFields?: string[]
}

// ---------------------------------------------------------------- what is edited

export type Condition = { [field: string]: unknown; all?: Condition[]; any?: Condition[]; not?: Condition }
export type Repeat = { over: string; kinds?: string[]; limit?: number }
export type Section = {
  id: string; component: string; props?: Record<string, unknown>; source?: Record<string, unknown>
  when?: Condition; repeat?: Repeat; scope?: string; locked?: boolean; children?: Section[]
}
export type Match = { library?: number; libraryKind?: string; mediaKind?: string }
export type Variant = { id: string; label?: string; when: Condition | null; sections: Section[] }
export type Page = {
  id: string; kind: string; title: string; icon?: string; description?: string; match?: Match; scope?: string
  sections: Section[]; variants?: Variant[]; personal?: { allow: string[] }; ttlSeconds?: number
}
export type NavItem = {
  id: string; type: string; label?: string; icon?: string; page?: string; library?: number; screen?: string; route?: string
  when?: Condition; placement?: Record<string, string>
}
export type Navigation = { items: NavItem[] }

export type Issue = { path: string; severity: string; message: string; fix?: string }
export type PageEntry = { id: string; kind: string; title: string; icon?: string; description?: string; match?: Match; source: string; sections: number; variants?: number }
export type Preset = { id: string; label: string; description: string; icon?: string; kind: string; pageKinds?: string[] | null; page?: Page; section?: Section }
export type LibraryRef = { id: number; name: string; type: string }

const clone = <T>(v: T): T => (v === undefined ? v : JSON.parse(JSON.stringify(v)))
const isObject = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)

// "media" → "Media", "pinnedViews" → "Pinned views": for the few names the catalogue gives without a label.
export function words(name: string): string {
  const spaced = name.replace(/[-_]+/g, ' ').replace(/([a-z0-9])([A-Z])/g, '$1 $2').trim().toLowerCase()
  return spaced ? spaced[0].toUpperCase() + spaced.slice(1) : ''
}

export const componentSpec = (cat: Catalogue, type: string) => cat.components.find((c) => c.type === type)
export const sourceSpec = (cat: Catalogue, type: string) => cat.sources.find((s) => s.type === type)
export const pageKindSpec = (cat: Catalogue, kind: string) => cat.pageKinds.find((k) => k.kind === kind)

// The catalogue's labels for the things it otherwise lists as bare names. They are the
// choices of the condition that asks about each, so nothing is spelled out here.
function labelFrom(cat: Catalogue, field: string, value: string): string {
  return cat.conditions.find((c) => c.field === field)?.options?.find((o) => o.value === value)?.label || words(value)
}
export const formFactorLabel = (cat: Catalogue, ff: string) => labelFrom(cat, 'formFactor', ff)
export const libraryKindLabel = (cat: Catalogue, kind: string) => labelFrom(cat, 'hasLibrary', kind)

// ---------------------------------------------------------------- the palette

export type PaletteEntry = { type: string; label: string; description: string; icon: string }
export type PaletteGroup = { category: string; label: string; items: PaletteEntry[] }

// A component a container names among its children belongs only inside such a container:
// a tab inside tabs, a list's row inside a list. The catalogue says so; nothing here does.
function homes(cat: Catalogue): Map<string, Set<string>> {
  const out = new Map<string, Set<string>>()
  for (const c of cat.components) for (const child of c.children || []) {
    if (!out.has(child)) out.set(child, new Set())
    out.get(child)!.add(c.type)
  }
  return out
}

// canHold: may a section of type child sit in parent ('' is the page itself)?
export function canHold(cat: Catalogue, parent: string, child: string): boolean {
  const spec = componentSpec(cat, child)
  if (!spec || !spec.authored) return false
  const only = homes(cat).get(child)
  if (parent === '') return !only
  const p = componentSpec(cat, parent)
  if (!p || !p.container) return false
  if (p.children && p.children.length) return p.children.includes(child)
  return !only
}

// palette: what can be added to parent ('' for the page), grouped as the catalogue groups it.
export function palette(cat: Catalogue, parent = ''): PaletteGroup[] {
  const groups: PaletteGroup[] = []
  for (const c of cat.components) {
    if (!canHold(cat, parent, c.type)) continue
    let g = groups.find((x) => x.category === c.category)
    if (!g) { g = { category: c.category, label: words(c.category), items: [] }; groups.push(g) }
    g.items.push({ type: c.type, label: c.label, description: c.description, icon: c.icon })
  }
  return groups
}

// ---------------------------------------------------------------- sources

export type SourceEntry = { type: string; label: string; description: string; icon: string; fits: boolean; why: string }
export type SourceGroup = { category: string; label: string; items: SourceEntry[] }

// sourceRule: whether a component takes a data source, as the catalogue states it.
export function sourceRule(cat: Catalogue, component: string): 'required' | 'optional' | 'none' {
  const d = componentSpec(cat, component)?.data
  return d === 'required' || d === 'optional' ? d : 'none'
}

// sourceGroups: the sources a component can be filled from on a page of this kind. One the
// component doesn't take is left out; one made for another kind of page is listed but marked,
// so it is plain why it isn't there to pick.
export function sourceGroups(cat: Catalogue, component: string, pageKind: string): SourceGroup[] {
  const spec = componentSpec(cat, component)
  if (!spec || sourceRule(cat, component) === 'none') return []
  const groups: SourceGroup[] = []
  for (const s of cat.sources) {
    if (spec.accepts && spec.accepts.length && !spec.accepts.includes(s.type)) continue
    const kinds = s.pageKinds || []
    const fits = !kinds.length || kinds.includes(pageKind)
    const why = fits ? '' : 'Only on ' + kinds.map((k) => (pageKindSpec(cat, k)?.label || k).toLowerCase()).join(' or ') + ' pages'
    let g = groups.find((x) => x.category === s.category)
    if (!g) { g = { category: s.category, label: words(s.category), items: [] }; groups.push(g) }
    g.items.push({ type: s.type, label: s.label, description: s.description, icon: s.icon, fits, why })
  }
  return groups
}

// The $names a source's parameter may stand in for on this page, with $each in a repeated section.
export function variablesFor(cat: Catalogue, pageKind: string, repeated: boolean): string[] {
  return [...(pageKindSpec(cat, pageKind)?.variables || []), ...(repeated ? ['$each'] : [])]
}

// ---------------------------------------------------------------- the form model

export type FieldKind = 'text' | 'multiline' | 'markdown' | 'int' | 'number' | 'bool' | 'enum' | 'enumList' | 'icon' | 'image' | 'route' | 'url' | 'section' | 'library' | 'ref' | 'scope' | 'json'

export type Field = {
  name: string
  kind: FieldKind
  type: string        // the catalogue's own word for it
  known: boolean      // false: a type this editor has never heard of, edited as plain text
  label: string
  help: string
  required: boolean
  options: Option[]
  min?: number
  max?: number
  maxLength?: number
  hasDefault: boolean
  defaultValue: unknown
  defaultText: string // "Default: Medium", or ''
  placeholder: string // the default, where a text box can show it
  limits: string      // "0 to 500", "At most 120 characters", or ''
  only: string        // "Only on television and phone", or ''
  variables: string[] // $names offered for it
  placeholders: boolean // text that may hold {user}, {library}, ...
}

const KINDS: Record<string, FieldKind> = {
  string: 'text', text: 'text', markdown: 'markdown', int: 'int', number: 'number', bool: 'bool', enum: 'enum', enumList: 'enumList',
  icon: 'icon', image: 'image', route: 'route', url: 'url', section: 'section', library: 'library', ref: 'ref', scope: 'scope',
  object: 'json', list: 'json',
}
// One line is enough up to here; longer text gets a box that grows.
const ONE_LINE = 200

function list(items: string[]): string {
  if (items.length < 2) return items.join('')
  return items.slice(0, -1).join(', ') + ' and ' + items[items.length - 1]
}

// valueText: a value in the words the form uses for it.
export function valueText(spec: Pick<PropSpec, 'type' | 'options'>, v: unknown): string {
  if (v === undefined || v === null) return ''
  if (typeof v === 'boolean') return v ? 'On' : 'Off'
  const label = (x: unknown) => spec.options?.find((o) => o.value === x)?.label || String(x)
  if (Array.isArray(v)) return v.length ? v.map(label).join(', ') : 'None'
  if (isObject(v)) return JSON.stringify(v)
  return label(v)
}

// fieldFor: everything the form needs to offer one property, from its descriptor alone.
export function fieldFor(cat: Catalogue, spec: PropSpec, variables: string[] = []): Field {
  const mapped = KINDS[spec.type]
  let kind: FieldKind = mapped || 'text'
  if (spec.type === 'text' && (spec.maxLength || 4000) > ONE_LINE) kind = 'multiline'
  const hasDefault = spec.default !== undefined && spec.default !== null && spec.default !== ''
  const limits = spec.min !== undefined && spec.max !== undefined ? `${spec.min} to ${spec.max}`
    : spec.min !== undefined ? `${spec.min} or more`
    : spec.max !== undefined ? `${spec.max} at most`
    : spec.maxLength ? `At most ${spec.maxLength} characters` : ''
  const ffs = spec.formFactors || []
  const takesVariable = spec.type === 'string' || spec.type === 'library' || spec.type === 'ref'
  return {
    name: spec.name, kind, type: spec.type, known: !!mapped, label: spec.label || words(spec.name), help: spec.help || '', required: !!spec.required,
    options: spec.options || [], min: spec.min, max: spec.max, maxLength: spec.maxLength,
    hasDefault, defaultValue: hasDefault ? spec.default : undefined,
    defaultText: hasDefault ? 'Default: ' + valueText(spec, spec.default) : '',
    placeholder: hasDefault && typeof spec.default !== 'boolean' && !Array.isArray(spec.default) ? valueText(spec, spec.default) : '',
    limits, only: ffs.length ? 'Only on ' + list(ffs.map((f) => formFactorLabel(cat, f).toLowerCase())) : '',
    variables: takesVariable ? variables : [], placeholders: spec.type === 'text' || spec.type === 'markdown',
  }
}

export const componentFields = (cat: Catalogue, type: string): Field[] => (componentSpec(cat, type)?.props || []).map((p) => fieldFor(cat, p))
export const sourceFields = (cat: Catalogue, type: string, variables: string[] = []): Field[] => (sourceSpec(cat, type)?.params || []).map((p) => fieldFor(cat, p, variables))
export const conditionFields = (cat: Catalogue): Field[] => cat.conditions.map((c) => fieldFor(cat, { ...c, name: c.field || c.name }))

// fromText: what a text box holds, as the value to store. Nothing typed is "not set".
export function fromText(field: Field, text: string): unknown {
  const t = text.trim()
  if (t === '') return undefined
  if (t.startsWith('$') && field.variables.length) return t
  switch (field.kind) {
    case 'int': { const n = Number(t); return Number.isInteger(n) ? n : t }
    case 'number': { const n = Number(t); return Number.isFinite(n) ? n : t }
    case 'library': case 'ref': return /^[0-9]+$/.test(t) ? Number(t) : t
    case 'json': try { return JSON.parse(t) } catch { return t }
    // Words keep their spaces as typed: the box would otherwise eat the space between two words.
    case 'text': case 'multiline': case 'markdown': return text
    default: return t
  }
}

// toText: the other way, for showing a stored value in a text box.
export function toText(v: unknown): string {
  if (v === undefined || v === null) return ''
  return typeof v === 'string' ? v : typeof v === 'object' ? JSON.stringify(v) : String(v)
}

// setValue: an object with one value set, or taken out when it is "not set", so what is
// saved holds only what someone chose.
export function setValue<T extends Record<string, unknown>>(obj: T | undefined, name: string, value: unknown): T {
  const next: Record<string, unknown> = { ...(obj || {}) }
  if (value === undefined || value === '' || value === null) delete next[name]
  else next[name] = value
  return next as T
}

// ---------------------------------------------------------------- ids

const ID = /^[a-z][a-z0-9_-]{0,63}$/
export const validId = (s: string) => ID.test(s)

// slug: any words as an ID: lower case, digits, - and _, starting with a letter.
export function slug(text: string, fallback = 'section'): string {
  let s = text.normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase().replace(/[^a-z0-9_]+/g, '-').replace(/^[^a-z]+/, '').replace(/-+$/, '').slice(0, 48)
  s = s.replace(/-+$/, '')
  return s || fallback
}

// uniqueId: base, or base-2, base-3, ... whichever nothing has yet.
export function uniqueId(base: string, taken: Iterable<string>): string {
  const has = new Set(taken)
  if (!has.has(base)) return base
  const stem = base.replace(/-[0-9]+$/, '')
  for (let n = 2; ; n++) { const id = `${stem}-${n}`.slice(-64); if (!has.has(id)) return id }
}

function collectIds(sections: Section[] | undefined, into: string[]) {
  for (const s of sections || []) { into.push(s.id); collectIds(s.children, into) }
}

// Every section ID on the page, variants included: how many sections it has in all.
export function pageIds(page: Page): string[] {
  const out: string[] = []
  collectIds(page.sections, out)
  for (const v of page.variants || []) collectIds(v.sections, out)
  return out
}

// The IDs in one list of sections, however deep. An ID is unique within the page's own
// sections and within each variant's; a variant may reuse the page's IDs, and does well to,
// so a profile's own arrangement of "jump" carries over to the variant's "jump".
export function listIds(sections: Section[]): string[] {
  const out: string[] = []
  collectIds(sections, out)
  return out
}

// ---------------------------------------------------------------- the tree

// A section's place: its index at each level, from the top of its list.
export type Path = number[]
export type TreeRow = { path: Path; depth: number; section: Section; parent: string; index: number; siblings: number }

export const samePath = (a: Path, b: Path) => a.length === b.length && a.every((n, i) => n === b[i])
const startsWith = (a: Path, prefix: Path) => prefix.length <= a.length && prefix.every((n, i) => n === a[i])

export function sectionAt(sections: Section[], path: Path): Section | undefined {
  let level: Section[] | undefined = sections
  let found: Section | undefined
  for (const i of path) { found = level?.[i]; if (!found) return undefined; level = found.children }
  return found
}

// flatten: the tree as the rows of a list, top to bottom.
export function flatten(sections: Section[], parent = '', base: Path = []): TreeRow[] {
  const out: TreeRow[] = []
  sections.forEach((s, i) => {
    const path = [...base, i]
    out.push({ path, depth: base.length, section: s, parent, index: i, siblings: sections.length })
    if (s.children?.length) out.push(...flatten(s.children, s.component, path))
  })
  return out
}

function editList(sections: Section[], parent: Path, fn: (list: Section[]) => Section[]): Section[] {
  if (!parent.length) return fn([...sections])
  const [i, ...rest] = parent
  return sections.map((s, n) => (n === i ? { ...s, children: editList(s.children || [], rest, fn) } : s))
}

export function updateAt(sections: Section[], path: Path, fn: (s: Section) => Section): Section[] {
  if (!path.length) return sections
  return editList(sections, path.slice(0, -1), (l) => l.map((s, n) => (n === path[path.length - 1] ? fn(s) : s)))
}

export function insertAt(sections: Section[], parent: Path, index: number, section: Section): Section[] {
  return editList(sections, parent, (l) => { l.splice(Math.max(0, Math.min(index, l.length)), 0, section); return l })
}

export function removeAt(sections: Section[], path: Path): Section[] {
  if (!path.length) return sections
  return editList(sections, path.slice(0, -1), (l) => l.filter((_, n) => n !== path[path.length - 1]))
}

// moveTo: take the section at from and put it in the list of toParent at toIndex (an index
// in that list as it is before the move). Nothing moves into itself. Answers the new tree
// and where the section now is, or null when the move makes no sense.
export function moveTo(sections: Section[], from: Path, toParent: Path, toIndex: number): { sections: Section[]; path: Path } | null {
  const node = sectionAt(sections, from)
  if (!node || startsWith(toParent, from)) return null
  const fromParent = from.slice(0, -1), fromIndex = from[from.length - 1]
  let parent = toParent, index = toIndex
  if (samePath(fromParent, toParent)) {
    if (index > fromIndex) index--
    if (index === fromIndex) return null
  } else if (startsWith(toParent, fromParent) && toParent[fromParent.length] > fromIndex) {
    // Taking it out shifts the list the target is in.
    parent = [...toParent]
    parent[fromParent.length]--
  }
  const without = removeAt(sections, from)
  const target = parent.length ? sectionAt(without, parent) : undefined
  const size = parent.length ? (target?.children || []).length : without.length
  if (parent.length && !target) return null
  index = Math.max(0, Math.min(index, size))
  return { sections: insertAt(without, parent, index, node), path: [...parent, index] }
}

// moveBy: up or down among its siblings.
export function moveBy(sections: Section[], path: Path, delta: number): { sections: Section[]; path: Path } | null {
  const parent = path.slice(0, -1), i = path[path.length - 1]
  const size = parent.length ? (sectionAt(sections, parent)?.children || []).length : sections.length
  const to = i + delta
  if (to < 0 || to >= size) return null
  return moveTo(sections, path, parent, delta > 0 ? to + 1 : to)
}

// moveOut: out of its container, to just after it.
export function moveOut(sections: Section[], path: Path): { sections: Section[]; path: Path } | null {
  if (path.length < 2) return null
  const parent = path.slice(0, -1)
  return moveTo(sections, path, parent.slice(0, -1), parent[parent.length - 1] + 1)
}

// moveIn: into the container just above it, as its last section.
export function moveIn(cat: Catalogue, sections: Section[], path: Path): { sections: Section[]; path: Path } | null {
  const i = path[path.length - 1]
  if (i < 1) return null
  const before = [...path.slice(0, -1), i - 1]
  const into = sectionAt(sections, before), node = sectionAt(sections, path)
  if (!into || !node || !canHold(cat, into.component, node.component)) return null
  return moveTo(sections, path, before, (into.children || []).length)
}

function reId(section: Section, taken: Set<string>): Section {
  const id = uniqueId(validId(section.id) ? section.id : slug(section.id || section.component), taken)
  taken.add(id)
  const next: Section = { ...section, id }
  if (section.children) next.children = section.children.map((c) => reId(c, taken))
  return next
}

// fresh: a copy of a section (a preset's, an example's, one being duplicated) whose IDs,
// and those of everything in it, are ones the page doesn't have yet.
export function fresh(section: Section, taken: Iterable<string>): Section {
  return reId(clone(section), new Set(taken))
}

export function duplicateAt(sections: Section[], path: Path, taken: Iterable<string>): { sections: Section[]; path: Path } | null {
  const node = sectionAt(sections, path)
  if (!node) return null
  const at = [...path.slice(0, -1), path[path.length - 1] + 1]
  return { sections: insertAt(sections, at.slice(0, -1), at[at.length - 1], fresh(node, taken)), path: at }
}

// renameId: give a section another ID, and keep everything that pointed at the old one
// (a filter bar's grid, the tab a set of tabs opens on) pointing at it.
export function renameId(cat: Catalogue, sections: Section[], path: Path, id: string): Section[] {
  const old = sectionAt(sections, path)?.id
  if (old === undefined || old === id) return sections
  const follow = (list: Section[]): Section[] => list.map((s) => {
    let props = s.props
    for (const p of componentSpec(cat, s.component)?.props || []) {
      if (p.type === 'section' && old && props?.[p.name] === old) props = { ...props, [p.name]: id }
    }
    const next = props === s.props ? s : { ...s, props }
    return s.children ? { ...next, children: follow(s.children) } : next
  })
  return follow(updateAt(sections, path, (s) => ({ ...s, id })))
}

// sectionTitle: what a section is called in the tree: its own title, else its source's, else its component's.
export function sectionTitle(cat: Catalogue, s: Section): string {
  for (const k of ['title', 'label', 'text', 'alt', 'placeholder']) {
    const v = s.props?.[k]
    if (typeof v === 'string' && v.trim()) return v.length > 60 ? v.slice(0, 57) + '…' : v
  }
  const src = typeof s.source?.type === 'string' ? sourceSpec(cat, s.source.type) : undefined
  return src?.label || componentSpec(cat, s.component)?.label || s.component || 'Section'
}

// newSection: a section of a type, ready to be filled in: required properties at their
// defaults, a source when one is required (the first that needs nothing chosen, with the
// page's own variables for what it does need), and a first child where only one type fits.
export function newSection(cat: Catalogue, type: string, pageKind: string, taken: Iterable<string>): Section {
  const spec = componentSpec(cat, type)
  const used = new Set(taken)
  const id = uniqueId(slug(spec?.label || type), used)
  used.add(id)
  const s: Section = { id, component: type }
  const props: Record<string, unknown> = {}
  for (const p of spec?.props || []) if (p.required && p.default !== undefined && p.default !== null) props[p.name] = p.default
  if (Object.keys(props).length) s.props = props
  if (spec && spec.data === 'required') {
    const vars = variablesFor(cat, pageKind, false)
    const filled = (src: SourceSpec) => {
      const out: Record<string, unknown> = { type: src.type }
      for (const p of src.params || []) {
        if (!p.required) continue
        if (vars.includes('$' + p.name)) out[p.name] = '$' + p.name
        else return null
      }
      return out
    }
    for (const g of sourceGroups(cat, type, pageKind)) for (const e of g.items) {
      if (s.source || !e.fits) continue
      const src = sourceSpec(cat, e.type)
      const got = src && filled(src)
      if (got) s.source = got
    }
  }
  if (spec?.container && spec.children?.length === 1) s.children = [newSection(cat, spec.children[0], pageKind, used)]
  return s
}

// exampleSection: the catalogue's example of a component, as a section to drop in.
export function exampleSection(cat: Catalogue, type: string, taken: Iterable<string>): Section | null {
  const ex = componentSpec(cat, type)?.example
  return isObject(ex) && typeof ex.component === 'string' ? fresh(ex as unknown as Section, taken) : null
}

// sectionChoices: the sections a "which section" property can name: the ones inside the
// section itself first (a set of tabs names one of its own), then the rest of the list.
export function sectionChoices(cat: Catalogue, sections: Section[], self: Path): Option[] {
  const mine = sectionAt(sections, self)
  const inside = flatten(mine?.children || []).map((r) => r.section)
  const rest = flatten(sections).filter((r) => !startsWith(r.path, self)).map((r) => r.section)
  return [...inside, ...rest].filter((s) => s.id).map((s) => ({ value: s.id, label: `${sectionTitle(cat, s)} (${s.id})` }))
}

// ---------------------------------------------------------------- conditions

const COMBINERS = ['all', 'any', 'not']

export function isEmptyCondition(c: Condition | null | undefined): boolean {
  return !c || Object.keys(tidyCondition(c) || {}).length === 0
}

// tidyCondition: without the fields that say nothing, or undefined when nothing is left.
// {"not": {}} stays: it means "never".
export function tidyCondition(c: Condition | null | undefined): Condition | undefined {
  if (!c) return undefined
  const out: Condition = {}
  for (const [k, v] of Object.entries(c)) {
    if (k === 'all' || k === 'any') { if (Array.isArray(v) && v.length) out[k] = (v as Condition[]).map((x) => tidyCondition(x) || {}) }
    else if (k === 'not') { if (isObject(v)) out.not = tidyCondition(v as Condition) || {} }
    else if (v === undefined || v === null || v === '' || (Array.isArray(v) && !v.length)) continue
    else out[k] = v
  }
  return Object.keys(out).length ? out : undefined
}

// conditionText: a condition in words, for a row that only has room for a line.
export function conditionText(cat: Catalogue, c: Condition | null | undefined): string {
  if (!c || isEmptyCondition(c)) return 'Always'
  const parts: string[] = []
  for (const spec of cat.conditions) {
    const v = c[spec.field]
    if (v === undefined || v === null || v === '' || (Array.isArray(v) && !v.length)) continue
    if (typeof v === 'boolean') parts.push(v ? spec.label : `Not: ${spec.label.toLowerCase()}`)
    else parts.push(`${spec.label}: ${Array.isArray(v) ? v.map((x) => valueText(spec, x)).join(' or ') : valueText(spec, v)}`)
  }
  for (const k of Object.keys(c)) if (!COMBINERS.includes(k) && !cat.conditions.some((s) => s.field === k)) parts.push(`${words(k)}: ${toText(c[k])}`)
  const group = (l: Condition[], joiner: string) => l.map((x) => { const t = conditionText(cat, x); return l.length > 1 && /, and |, or /.test(t) ? `(${t})` : t }).join(joiner)
  if (Array.isArray(c.all) && c.all.length) parts.push(group(c.all, ', and '))
  if (Array.isArray(c.any) && c.any.length) parts.push(c.any.length > 1 ? `(${group(c.any, ', or ')})` : group(c.any, ''))
  if (isObject(c.not)) parts.push(isEmptyCondition(c.not as Condition) ? 'Never' : `Not (${conditionText(cat, c.not as Condition)})`)
  return parts.join(', and ')
}

// ---------------------------------------------------------------- pages

export const SOURCE_WORDS: Record<string, string> = { builtin: 'Built-in', changed: 'Changed', custom: 'Yours' }
export const SOURCE_HELP: Record<string, string> = {
  builtin: 'As it comes with the server.',
  changed: 'A built-in page you have changed. Resetting it brings the built-in one back.',
  custom: 'A page of your own.',
}

// matchText: what a page is for, in words.
export function matchText(cat: Catalogue, entry: { kind: string; match?: Match | null }, libraries: LibraryRef[] = []): string {
  const kind = pageKindSpec(cat, entry.kind)
  const m = entry.match || {}
  const parts: string[] = []
  if (m.library) parts.push(`the library ${libraries.find((l) => l.id === m.library)?.name || '#' + m.library}`)
  if (m.libraryKind) parts.push(`${libraryKindLabel(cat, m.libraryKind).toLowerCase()} libraries`)
  if (m.mediaKind) parts.push(`${words(m.mediaKind).toLowerCase()} titles`)
  if (parts.length) return `${kind?.label || words(entry.kind)}: only ${parts.join(', ')}`
  return kind ? `${kind.label}. ${kind.description}` : words(entry.kind)
}

// pageName: what to call a page in a list. A page whose title fills itself in ("{library}",
// "{title}") is called by what it is for instead: "A library: Movies".
export function pageName(cat: Catalogue, p: { id: string; title?: string; kind: string; match?: Match | null }): string {
  if (p.title && !p.title.includes('{')) return p.title
  const kind = pageKindSpec(cat, p.kind)?.label || words(p.kind) || p.id
  const m = p.match || {}
  const what = m.libraryKind ? libraryKindLabel(cat, m.libraryKind) : m.mediaKind ? words(m.mediaKind) : m.library ? `library #${m.library}` : ''
  return what ? `${kind}: ${what}` : kind
}

// tidyPage: the page as it is saved: nothing empty left lying in it.
export function tidyPage(page: Page): Page {
  const section = (s: Section): Section => {
    const out: Section = { id: s.id, component: s.component }
    if (s.props && Object.keys(s.props).length) out.props = s.props
    if (s.source && Object.keys(s.source).length) out.source = s.source
    const when = tidyCondition(s.when)
    if (when) out.when = when
    if (s.repeat?.over) {
      out.repeat = { over: s.repeat.over }
      if (s.repeat.kinds?.length) out.repeat.kinds = s.repeat.kinds
      if (s.repeat.limit) out.repeat.limit = s.repeat.limit
    }
    if (s.scope) out.scope = s.scope
    if (s.locked) out.locked = true
    if (s.children?.length) out.children = s.children.map(section)
    return out
  }
  const out: Page = { id: page.id, kind: page.kind, title: page.title, sections: (page.sections || []).map(section) }
  if (page.icon) out.icon = page.icon
  if (page.description) out.description = page.description
  const m: Match = {}
  if (page.match?.library) m.library = page.match.library
  if (page.match?.libraryKind) m.libraryKind = page.match.libraryKind
  if (page.match?.mediaKind) m.mediaKind = page.match.mediaKind
  if (Object.keys(m).length) out.match = m
  if (page.scope) out.scope = page.scope
  if (page.variants?.length) out.variants = page.variants.map((v) => ({ id: v.id, ...(v.label ? { label: v.label } : {}), when: tidyCondition(v.when) || {}, sections: (v.sections || []).map(section) }))
  if (page.personal) out.personal = { allow: [...(page.personal.allow || [])] }
  if (page.ttlSeconds) out.ttlSeconds = page.ttlSeconds
  return out
}

// editPath: where a page is edited. The editor's own pages (the menu, the gallery, the
// history) have the short addresses, so a page that happens to share a name with one of them gets a longer one.
const OWN_ADDRESSES = ['navigation', 'presets', 'history']
export const editPath = (id: string) => (!id ? '/settings/pages' : `/settings/pages/${OWN_ADDRESSES.includes(id) ? 'page/' : ''}${encodeURIComponent(id)}`)

export const samePage = (a: Page, b: Page) => JSON.stringify(tidyPage(a)) === JSON.stringify(tidyPage(b))

export function newPage(id: string, title: string, kind: string, match?: Match): Page {
  const p: Page = { id, kind, title, sections: [] }
  if (match && Object.keys(match).length) p.match = match
  return p
}

// copyPage: a page under another ID. Its sections keep their IDs: those are per page.
export function copyPage(page: Page, id: string, title: string): Page {
  return { ...clone(page), id, title }
}

// The sections of the page itself (variant -1) or of one of its variants.
export function rootSections(page: Page, variant: number): Section[] {
  return variant < 0 ? page.sections || [] : page.variants?.[variant]?.sections || []
}

export function withRootSections(page: Page, variant: number, sections: Section[]): Page {
  if (variant < 0) return { ...page, sections }
  return { ...page, variants: (page.variants || []).map((v, i) => (i === variant ? { ...v, sections } : v)) }
}

// parsePageJson: a page typed as JSON, or what is wrong with it. Never a half-read page.
export function parsePageJson(text: string, id: string): { page: Page } | { error: string } {
  let v: unknown
  try { v = JSON.parse(text) } catch (e) { return { error: 'That isn’t JSON yet: ' + (e instanceof Error ? e.message : String(e)) } }
  if (!isObject(v)) return { error: 'A page is written between { and }.' }
  if (v.id !== undefined && v.id !== id) return { error: `This is the page “${id}”. The JSON calls itself “${String(v.id)}”: keep the ID, or make a new page for it.` }
  if (v.sections !== undefined && !Array.isArray(v.sections)) return { error: '“sections” is a list, between [ and ].' }
  if (v.variants !== undefined && !Array.isArray(v.variants)) return { error: '“variants” is a list, between [ and ].' }
  return { page: { ...(v as unknown as Page), id, kind: typeof v.kind === 'string' ? v.kind : '', title: typeof v.title === 'string' ? v.title : '', sections: (v.sections as Section[]) || [] } }
}

export function parseNavigationJson(text: string): { navigation: Navigation } | { error: string } {
  let v: unknown
  try { v = JSON.parse(text) } catch (e) { return { error: 'That isn’t JSON yet: ' + (e instanceof Error ? e.message : String(e)) } }
  if (!isObject(v) || !Array.isArray(v.items)) return { error: 'A menu is written as { "items": [ … ] }.' }
  if (v.items.some((x) => !isObject(x))) return { error: 'Every entry is written between { and }.' }
  return { navigation: { items: v.items as NavItem[] } }
}

// limitNote: a word of warning as a page nears what the server allows.
export function limitNote(cat: Catalogue, page: Page): string {
  const max = cat.limits?.maxSections, n = pageIds(page).length
  if (max && n >= max) return `This page has ${n} sections, which is the most a page can hold (${max}).`
  const variants = cat.limits?.maxVariants
  if (variants && (page.variants?.length || 0) >= variants) return `This page has the most variants a page can have (${variants}).`
  return ''
}

// ---------------------------------------------------------------- the menu

// The kinds of entry a menu holds. The catalogue doesn't list them (the validator names
// them), so they are spelled out here: the one list in this file that is.
export const NAV_TYPES: { value: string; label: string; help: string }[] = [
  { value: 'page', label: 'A page', help: 'Opens one of the pages.' },
  { value: 'libraries', label: 'Every library', help: 'One entry for each library the profile can open.' },
  { value: 'library', label: 'One library', help: 'Opens a library you pick.' },
  { value: 'native', label: 'A screen of the app', help: 'Opens something the app draws itself, such as the TV guide.' },
  { value: 'route', label: 'A place in the app', help: 'Opens an address in the app, such as /library/3.' },
]

// The app's own screens a menu entry can open: the catalogue marks them `menu` (a server
// from before that flag: the ones with a place of their own that needs nothing filled in;
// the player needs a title, so it isn't one).
export function navScreens(cat: Catalogue): NativeScreenSpec[] {
  const list = cat.nativeScreens || []
  if (list.some((s) => s.menu !== undefined)) return list.filter((s) => s.menu)
  return list.filter((s) => s.route && !s.route.includes('{'))
}

export const isSettingsEntry = (it: NavItem) => it.type === 'native' && it.screen === 'settings'

export const defaultPlacement = (cat: Catalogue) => cat.placements?.[0]?.value || 'primary'
export const placementOf = (cat: Catalogue, it: NavItem, formFactor: string) => it.placement?.[formFactor] || defaultPlacement(cat)

// setPlacement: where an entry sits on one form factor; the usual place isn't written down.
export function setPlacement(cat: Catalogue, it: NavItem, formFactor: string, where: string): NavItem {
  const placement = { ...(it.placement || {}) }
  if (where === defaultPlacement(cat)) delete placement[formFactor]
  else placement[formFactor] = where
  const next = { ...it }
  if (Object.keys(placement).length) next.placement = placement
  else delete next.placement
  return next
}

// navTitle: what an entry is called in the list, and what it is called when it has no label of its own.
export function navTitle(cat: Catalogue, it: NavItem, pages: PageEntry[] = [], libraries: LibraryRef[] = []): string {
  if (it.label) return it.label
  if (it.type === 'page') { const p = pages.find((x) => x.id === it.page); return p ? pageName(cat, p) : it.page || 'A page' }
  if (it.type === 'libraries') return 'Every library'
  if (it.type === 'library') return libraries.find((l) => l.id === it.library)?.name || (it.library ? `Library #${it.library}` : 'A library')
  if (it.type === 'native') return cat.nativeScreens?.find((s) => s.screen === it.screen)?.label || it.screen || 'A screen'
  return it.route || 'A place in the app'
}

export function newNavItem(type: string, taken: Iterable<string>): NavItem {
  const it: NavItem = { id: uniqueId(type === 'libraries' ? 'libraries' : 'entry', taken), type }
  if (type === 'route') { it.route = '/'; it.label = 'New entry' }
  return it
}

// tidyNavigation: the menu as it is saved.
export function tidyNavigation(nav: Navigation): Navigation {
  return { items: nav.items.map((it) => {
    const out: NavItem = { id: it.id, type: it.type }
    if (it.label) out.label = it.label
    if (it.icon) out.icon = it.icon
    if (it.type === 'page' && it.page) out.page = it.page
    if (it.type === 'library' && it.library) out.library = it.library
    if (it.type === 'native' && it.screen) out.screen = it.screen
    if (it.type === 'route' && it.route) out.route = it.route
    const when = tidyCondition(it.when)
    if (when) out.when = when
    if (it.placement && Object.keys(it.placement).length) out.placement = it.placement
    return out
  }) }
}

export const sameNavigation = (a: Navigation, b: Navigation) => JSON.stringify(tidyNavigation(a)) === JSON.stringify(tidyNavigation(b))

export function moveItem<T>(items: T[], from: number, to: number): T[] {
  if (from === to || from < 0 || to < 0 || from >= items.length || to >= items.length) return items
  const next = [...items]
  const [it] = next.splice(from, 1)
  next.splice(to, 0, it)
  return next
}

// ---------------------------------------------------------------- issues

// Where a validator's issue points. field is what a form control is known by:
// "props/cardStyle", "source/library", "repeat/over", "match/libraryKind", "when", "id", "title".
export type IssueTarget = {
  pointer: string  // the path inside the page or the menu, without /pages/<id> or /navigation
  where: 'page' | 'section' | 'variant' | 'nav' | 'other'
  variant: number  // which variant's sections, -1 for the page's own
  path: Path       // the section's place
  item: number     // the menu entry, -1 when it isn't one
  field: string
}

const TWO_PART = ['props', 'source', 'repeat', 'match', 'personal', 'placement']

function fieldOf(rest: string[]): string {
  if (!rest.length) return ''
  return TWO_PART.includes(rest[0]) && rest[1] !== undefined ? `${rest[0]}/${rest[1]}` : rest[0]
}

// locateIssue: an issue's path, from a check of one page ("/sections/2/source/library"),
// of the menu ("/items/3/page") or of the whole configuration ("/pages/night/sections/0/…",
// "/navigation/items/3/page"), as the section and field it is about.
export function locateIssue(issuePath: string, pageId = '', menu = false): IssueTarget {
  let parts = issuePath.split('/').slice(1)
  const none: IssueTarget = { pointer: issuePath, where: 'other', variant: -1, path: [], item: -1, field: '' }
  if (parts[0] === 'pages') {
    if (menu || (pageId && parts[1] !== pageId)) return none
    parts = parts.slice(2)
  } else if (parts[0] === 'navigation') {
    if (!menu) return none
    parts = parts.slice(1)
  } else if (['theme', 'personalisation', 'version', 'notes'].includes(parts[0])) return none
  const pointer = parts.length ? '/' + parts.join('/') : ''
  if (menu) {
    if (parts[0] === 'items' && /^[0-9]+$/.test(parts[1] || '')) return { pointer, where: 'nav', variant: -1, path: [], item: Number(parts[1]), field: fieldOf(parts.slice(2)) }
    return { pointer, where: 'nav', variant: -1, path: [], item: -1, field: fieldOf(parts) }
  }
  let variant = -1
  let rest = parts
  if (rest[0] === 'variants' && /^[0-9]+$/.test(rest[1] || '')) {
    variant = Number(rest[1])
    rest = rest.slice(2)
    if (rest[0] !== 'sections' || rest.length < 2) return { pointer, where: 'variant', variant, path: [], item: -1, field: fieldOf(rest) }
  }
  if (rest[0] === 'sections' && /^[0-9]+$/.test(rest[1] || '')) {
    const path: Path = [Number(rest[1])]
    rest = rest.slice(2)
    while (rest[0] === 'children' && /^[0-9]+$/.test(rest[1] || '')) { path.push(Number(rest[1])); rest = rest.slice(2) }
    return { pointer, where: 'section', variant, path, item: -1, field: fieldOf(rest) }
  }
  return { pointer, where: 'page', variant: -1, path: [], item: -1, field: fieldOf(rest) }
}

// issuesFor: the issues about one field of one thing.
export function issuesAt(located: { issue: Issue; at: IssueTarget }[], match: (at: IssueTarget) => boolean, field: string): Issue[] {
  return located.filter((l) => match(l.at) && (l.at.field === field || l.at.field.startsWith(field + '/'))).map((l) => l.issue)
}

// ---------------------------------------------------------------- JSON by pointer

const parts = (pointer: string) => (pointer ? pointer.split('/').slice(1) : [])

export function getIn(doc: unknown, pointer: string): unknown {
  let v = doc
  for (const p of parts(pointer)) {
    if (Array.isArray(v)) v = v[Number(p)]
    else if (isObject(v)) v = v[p]
    else return undefined
  }
  return v
}

function editIn(doc: unknown, path: string[], fn: (holder: unknown, key: string) => unknown): unknown {
  if (path.length === 1) return fn(doc, path[0])
  const [head, ...rest] = path
  if (Array.isArray(doc)) return doc.map((x, i) => (i === Number(head) ? editIn(x, rest, fn) : x))
  if (isObject(doc)) return { ...doc, [head]: editIn(doc[head], rest, fn) }
  return doc
}

export function setIn<T>(doc: T, pointer: string, value: unknown): T {
  const path = parts(pointer)
  if (!path.length) return value as T
  return editIn(doc, path, (holder, key) => {
    if (Array.isArray(holder)) return holder.map((x, i) => (i === Number(key) ? value : x))
    return { ...(isObject(holder) ? holder : {}), [key]: value }
  }) as T
}

export function removeIn<T>(doc: T, pointer: string): T {
  const path = parts(pointer)
  if (!path.length) return doc
  return editIn(doc, path, (holder, key) => {
    if (Array.isArray(holder)) return holder.filter((_, i) => i !== Number(key))
    if (!isObject(holder)) return holder
    const next = { ...holder }
    delete next[key]
    return next
  }) as T
}

// renameIn: the same value under another name, in the same place among its neighbours.
export function renameIn<T>(doc: T, pointer: string, to: string): T {
  const path = parts(pointer)
  if (!path.length) return doc
  return editIn(doc, path, (holder, key) => {
    if (!isObject(holder) || !(key in holder)) return holder
    const next: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(holder)) { if (k === key) next[to] = v; else if (k !== to) next[k] = v }
    return next
  }) as T
}

export function moveToEndIn<T>(doc: T, pointer: string): T {
  const path = parts(pointer)
  if (!path.length) return doc
  return editIn(doc, path, (holder, key) => {
    if (!Array.isArray(holder)) return holder
    const i = Number(key)
    return [...holder.filter((_, n) => n !== i), holder[i]]
  }) as T
}

// ---------------------------------------------------------------- fixes

// What a button can do about an issue. Most fixes are advice ("Give the page a title");
// these are the ones the validator words so that there is only one thing to do.
export type Fix =
  | { kind: 'set'; value: unknown; label: string }
  | { kind: 'rename'; to: string; label: string }
  | { kind: 'remove'; label: string }
  | { kind: 'moveToEnd'; label: string }

const lastPart = (pointer: string) => pointer.slice(pointer.lastIndexOf('/') + 1)

// parseFix: the fix of an issue as something to do to doc at pointer, or null when it is
// advice for a person. doc is the page (or menu) the issue is about.
export function parseFix(issue: Pick<Issue, 'message' | 'fix'>, doc: unknown, pointer: string): Fix | null {
  const fix = (issue.fix || '').trim()
  if (!fix || !pointer) return null
  const value = getIn(doc, pointer)
  const meant = /^Did you mean "([^"]+)"\?$/.exec(fix)
  if (meant) {
    // The wrong word is the one the message quotes: it is either the value there or the name it is under.
    const wrong = /"([^"]*)"/.exec(issue.message)?.[1]
    if (typeof value === 'string' && (wrong === undefined || value === wrong)) return { kind: 'set', value: meant[1], label: `Change to “${meant[1]}”` }
    if (wrong !== undefined && lastPart(pointer) === wrong && value !== undefined) return { kind: 'rename', to: meant[1], label: `Rename to “${meant[1]}”` }
    return null
  }
  if (/^Remove (it|kinds|the source|the condition|the repeat)\b/.test(fix)) return value === undefined ? null : { kind: 'remove', label: 'Remove it' }
  if (fix === 'Remove the control characters.' && typeof value === 'string') {
    // eslint-disable-next-line no-control-regex
    return { kind: 'set', value: value.replace(/[\u0000\u001b]/g, ''), label: 'Remove them' }
  }
  if (fix === 'Write a whole number without quotes.' && typeof value === 'string' && /^-?[0-9]+$/.test(value.trim())) return { kind: 'set', value: Number(value.trim()), label: `Make it the number ${value.trim()}` }
  if (fix === 'Write true or false without quotes.' && typeof value === 'string') {
    const t = value.trim().toLowerCase()
    if (['true', 'yes', 'on', '1'].includes(t)) return { kind: 'set', value: true, label: 'Turn it on' }
    if (['false', 'no', 'off', '0'].includes(t)) return { kind: 'set', value: false, label: 'Turn it off' }
    return null
  }
  if (fix === 'Write it in quotes.' && (typeof value === 'number' || typeof value === 'boolean')) return { kind: 'set', value: String(value), label: 'Make it text' }
  if (fix === 'Use 0 for the default.') return { kind: 'set', value: 0, label: 'Use the default' }
  if (/^Move (it|the grid) to the end\b/.test(fix) && /\/[0-9]+$/.test(pointer) && value !== undefined) return { kind: 'moveToEnd', label: 'Move it to the end' }
  return null
}

export function applyFix<T>(doc: T, pointer: string, fix: Fix): T {
  switch (fix.kind) {
    case 'set': return setIn(doc, pointer, fix.value)
    case 'rename': return renameIn(doc, pointer, fix.to)
    case 'remove': return removeIn(doc, pointer)
    case 'moveToEnd': return moveToEndIn(doc, pointer)
  }
}

// issueWhere: where an issue is, in words, for the list of them.
export function issueWhere(cat: Catalogue, page: Page, at: IssueTarget): string {
  const fieldWords = (f: string) => {
    if (!f) return ''
    const [a, b] = f.split('/')
    if (b === undefined) return words(a)
    return a === 'props' ? b : `${words(a).toLowerCase()} ${b}`
  }
  if (at.where === 'section') {
    const s = sectionAt(rootSections(page, at.variant), at.path)
    const name = s ? sectionTitle(cat, s) : 'A section'
    const spec = s && componentSpec(cat, s.component)
    let f = fieldWords(at.field)
    if (s && at.field.startsWith('props/')) f = spec?.props?.find((p) => p.name === at.field.slice(6))?.label || f
    if (s && at.field.startsWith('source/') && typeof s.source?.type === 'string') f = sourceSpec(cat, s.source.type)?.params?.find((p) => p.name === at.field.slice(7))?.label || f
    return f ? `${name}, ${f}` : name
  }
  if (at.where === 'variant') return `Variant ${page.variants?.[at.variant]?.label || page.variants?.[at.variant]?.id || at.variant + 1}${at.field ? ', ' + fieldWords(at.field).toLowerCase() : ''}`
  if (at.where === 'page') return at.field ? `The page’s ${fieldWords(at.field).toLowerCase()}` : 'The page'
  return 'Elsewhere'
}

// ---------------------------------------------------------------- what changed

export type Change = { path: string; change: string; added?: string[]; removed?: string[]; changed?: string[]; reordered?: boolean; other?: string[] }
export type ChangeLine = { title: string; details: string[]; page: string }

const count = (n: number, one: string, many = one + 's') => `${n} ${n === 1 ? one : many}`

// describeChanges: the server's list of differences between two configurations, in sentences.
// isBuiltin says whether a page ID is one the server ships, which is the difference between
// "a new page" and "a built-in page, changed" (the configuration holds only what was changed).
export function describeChanges(changes: Change[], opts: { titleOf?: (id: string) => string; isBuiltin?: (id: string) => boolean } = {}): ChangeLine[] {
  return changes.map((c) => {
    if (c.path.startsWith('/pages/')) {
      const id = c.path.slice(7)
      const name = opts.titleOf?.(id) || id
      const shown = name !== id ? `${name} (${id})` : id
      const builtin = opts.isBuiltin?.(id)
      if (c.change === 'added') return { page: id, title: `${shown}: ${builtin === undefined ? 'new, or changed from the built-in page' : builtin ? 'changed from the built-in page' : 'a new page'}`, details: [] }
      if (c.change === 'removed') return { page: id, title: `${shown}: ${builtin === undefined ? 'removed, or back to the built-in page' : builtin ? 'back to the built-in page' : 'removed'}`, details: [] }
      const details: string[] = []
      if (c.added?.length) details.push(`${count(c.added.length, 'section')} added: ${c.added.join(', ')}`)
      if (c.removed?.length) details.push(`${count(c.removed.length, 'section')} removed: ${c.removed.join(', ')}`)
      if (c.changed?.length) details.push(`${count(c.changed.length, 'section')} changed: ${c.changed.join(', ')}`)
      if (c.reordered) details.push('The sections are in another order')
      if (c.other?.length) details.push(`Also different: ${c.other.map((o) => words(o).toLowerCase()).join(', ')}`)
      return { page: id, title: `${shown}: changed`, details }
    }
    if (c.path === '/navigation') return { page: '', title: 'The menu changed', details: [] }
    if (c.path === '/theme') return { page: '', title: 'Theme colours changed', details: [] }
    if (c.path === '/personalisation') return { page: '', title: 'Whether profiles may arrange pages changed', details: [] }
    return { page: '', title: `${c.path}: ${c.change}`, details: [] }
  })
}

export type DiffLine = { op: ' ' | '+' | '-'; text: string }

// lineDiff: two texts line by line: what stayed, what went and what came. Used on two
// pretty-printed pages, which are short; past a sensible size it stops looking for the
// smallest difference and shows the middle as all gone and all new.
export function lineDiff(a: string, b: string, budget = 4_000_000): DiffLine[] {
  const x = a.split('\n'), y = b.split('\n')
  let start = 0
  while (start < x.length && start < y.length && x[start] === y[start]) start++
  let endX = x.length, endY = y.length
  while (endX > start && endY > start && x[endX - 1] === y[endY - 1]) { endX--; endY-- }
  const out: DiffLine[] = x.slice(0, start).map((text) => ({ op: ' ', text }))
  const mx = x.slice(start, endX), my = y.slice(start, endY)
  const n = mx.length, m = my.length
  if (n * m > budget) {
    out.push(...mx.map((text): DiffLine => ({ op: '-', text })), ...my.map((text): DiffLine => ({ op: '+', text })))
  } else {
    // The longest run of lines both keep, from the ends backwards.
    const w = m + 1
    const t = new Uint32Array((n + 1) * w)
    for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) t[i * w + j] = mx[i] === my[j] ? t[(i + 1) * w + j + 1] + 1 : Math.max(t[(i + 1) * w + j], t[i * w + j + 1])
    let i = 0, j = 0
    while (i < n && j < m) {
      if (mx[i] === my[j]) { out.push({ op: ' ', text: mx[i] }); i++; j++ }
      else if (t[(i + 1) * w + j] >= t[i * w + j + 1]) out.push({ op: '-', text: mx[i++] })
      else out.push({ op: '+', text: my[j++] })
    }
    while (i < n) out.push({ op: '-', text: mx[i++] })
    while (j < m) out.push({ op: '+', text: my[j++] })
  }
  out.push(...x.slice(endX).map((text): DiffLine => ({ op: ' ', text })))
  return out
}

// foldDiff: a diff with the long unchanged stretches folded away, keeping a few lines round each change.
export function foldDiff(lines: DiffLine[], keep = 3): (DiffLine | { op: '…'; text: string })[] {
  const near = lines.map(() => false)
  lines.forEach((l, i) => { if (l.op !== ' ') for (let k = Math.max(0, i - keep); k <= Math.min(lines.length - 1, i + keep); k++) near[k] = true })
  const out: (DiffLine | { op: '…'; text: string })[] = []
  let skipped = 0
  lines.forEach((l, i) => {
    if (near[i]) { if (skipped) { out.push({ op: '…', text: `${count(skipped, 'line')} the same` }); skipped = 0 } out.push(l) }
    else skipped++
  })
  if (skipped && out.length) out.push({ op: '…', text: `${count(skipped, 'line')} the same` })
  return out
}

export const pretty = (v: unknown) => JSON.stringify(v, null, 2)

// ---------------------------------------------------------------- undo

export type Undo<T> = { past: T[]; present: T; future: T[]; tag: string; at: number }

export const undoStart = <T>(present: T): Undo<T> => ({ past: [], present, future: [], tag: '', at: 0 })

// How long typing in one field counts as one change.
const TYPING_MS = 1200

// undoPush: a new state. Changes with the same tag in quick succession (typing in one
// field) are one step back, not one a letter.
export function undoPush<T>(u: Undo<T>, next: T, tag = '', now = 0, limit = 100): Undo<T> {
  if (next === u.present) return u
  if (tag && tag === u.tag && now - u.at < TYPING_MS && u.past.length) return { ...u, present: next, future: [], at: now }
  return { past: [...u.past, u.present].slice(-limit), present: next, future: [], tag, at: now }
}

export function undoBack<T>(u: Undo<T>): Undo<T> {
  if (!u.past.length) return u
  return { past: u.past.slice(0, -1), present: u.past[u.past.length - 1], future: [u.present, ...u.future], tag: '', at: 0 }
}

export function undoForward<T>(u: Undo<T>): Undo<T> {
  if (!u.future.length) return u
  return { past: [...u.past, u.present], present: u.future[0], future: u.future.slice(1), tag: '', at: 0 }
}

// ---------------------------------------------------------------- preview

export type FormFactor = 'tv' | 'phone' | 'tablet' | 'desktop'
export const PREVIEW_FRAMES: Record<FormFactor, { width: number; height: number; label: string }> = {
  phone: { width: 390, height: 844, label: 'Phone' },
  tablet: { width: 820, height: 1180, label: 'Tablet' },
  desktop: { width: 1440, height: 900, label: 'Desktop' },
  tv: { width: 1920, height: 1080, label: 'TV' },
}

// What the preview tells the server about the device it is pretending to be.
export function previewDevice(formFactor: FormFactor, locale: string) {
  const f = PREVIEW_FRAMES[formFactor]
  const base = { platform: 'web', formFactor: formFactor as string, capabilities: formFactor === 'phone' || formFactor === 'tablet' ? ['touch', 'keyboard'] : ['pointer', 'keyboard'], locale, views: [] as string[] }
  return formFactor === 'tv' ? { ...base, deviceClass: 'hd' } : { ...base, viewport: `${f.width}x${f.height}` }
}

export const previewScale = (pane: number, formFactor: FormFactor) => (pane > 0 ? Math.min(1, pane / PREVIEW_FRAMES[formFactor].width) : 1)

export type PreviewInput = { name: string; required: boolean; kind: 'library' | 'text' }

// previewInputs: what has to be chosen before a page of this kind can be drawn: the
// library of a library page, the title of a title's page. From the kind's own route.
export function previewInputs(cat: Catalogue, kind: string): PreviewInput[] {
  const spec = pageKindSpec(cat, kind)
  if (!spec) return []
  const inRoute = [...spec.route.matchAll(/\{([a-zA-Z]+)\}/g)].map((m) => m[1])
  return (spec.params || []).filter((p) => inRoute.includes(p)).map((p) => ({ name: p, required: true, kind: p === 'library' ? 'library' : 'text' }))
}

// previewLibrary: the library a page would most likely be shown for.
export function previewLibrary(page: { match?: Match | null }, libraries: LibraryRef[]): number {
  if (page.match?.library) return page.match.library
  return (libraries.find((l) => l.type === page.match?.libraryKind) || libraries[0])?.id || 0
}

// pageRoute: the page's place in the app, or '' when something it needs hasn't been chosen.
export function pageRoute(cat: Catalogue, page: { id: string; kind: string }, params: Record<string, string> = {}): string {
  const spec = pageKindSpec(cat, page.kind)
  if (!spec) return ''
  let missing = false
  const route = spec.route.replace(/\{([a-zA-Z]+)\}/g, (_, name: string) => {
    const v = name === 'id' ? page.id : params[name]
    if (!v) missing = true
    return encodeURIComponent(v || '')
  })
  return missing ? '' : route
}

// ---------------------------------------------------------------- files

export const exportName = (page?: string) => (page ? `wittymedia-page-${page}.json` : 'wittymedia-ui.json')

// readExport: a file someone chose to import, looked at before it is sent anywhere.
export function readExport(text: string): { kind: 'page' | 'config'; name: string; body: unknown } | { error: string } {
  let v: unknown
  try { v = JSON.parse(text) } catch { return { error: 'That file isn’t JSON. Choose a file that was exported from here.' } }
  if (!isObject(v) || v.format !== 'wittymedia-ui') return { error: 'That isn’t a file exported from a WittyMediaServer’s pages. Choose one that Export made.' }
  if (isObject(v.page)) return { kind: 'page', name: String((v.page as Record<string, unknown>).title || (v.page as Record<string, unknown>).id || 'A page'), body: v }
  if (isObject(v.config)) return { kind: 'config', name: 'Every page and the menu', body: v }
  return { error: 'That file holds neither a page nor a whole configuration.' }
}
