// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { SpecAction, SpecCard, SpecChip, SpecField, SpecPageData, SpecRow, SpecSection } from './types.ts'

// The canvas's builders, with their names and argument orders, so a page written here reads like the drawing.
export const R = (title: string, value?: string, on?: boolean, sub?: string): SpecRow => ({ title, ...(value ? { value } : {}), ...(sub ? { sub } : {}), on: !!on })
export const Cc = (title: string, on?: boolean): SpecChip => ({ title, on: !!on })
export const secR = (h: string, items: SpecRow[]): SpecSection => ({ kind: 'rows', h, items })
export const secP = (h: string, items: SpecChip[]): SpecSection => ({ kind: 'chips', h, items })
export const secC = (h: string, items: SpecCard[]): SpecSection => ({ kind: 'cards', h, items })
export const secF = (h: string, items: SpecField[]): SpecSection => ({ kind: 'fields', h, items })
// Name a section's heading id, and mark a rows section as one choice among its rows.
export const anchored = (s: SpecSection, anchor: string): SpecSection => ({ ...s, anchor })
export const choice = (s: SpecSection): SpecSection => (s.kind === 'rows' ? { ...s, choice: true } : s)
export const secN = (h: string, text: string, tone?: boolean): SpecSection => ({ kind: 'note', h, text, tone: !!tone })
export const A1 = (title: string, to?: string): SpecAction => ({ title, kind: 'primary', ...(to ? { to } : {}) })
export const A2 = (title: string, to?: string): SpecAction => ({ title, kind: 'secondary', ...(to ? { to } : {}) })
export const mk = (key: string, eyebrow: string, title: string, lead: string, sections: SpecSection[], actions: SpecAction[] = [], foot = ''): SpecPageData =>
  ({ key, eyebrow, title, lead, sections, actions, foot })

// Every row, chip, card and action gets an id (kept if it has one): "<page>.<section>.<item>", the handle a page's
// handlers and the tests refer to. Returns a new page; the input is not changed.
export function withIds(p: SpecPageData): SpecPageData {
  const id = (given: string | undefined, ...path: number[]) => given || [p.key, ...path].join('.')
  return {
    ...p,
    sections: p.sections.map((s, si): SpecSection => {
      if (s.kind === 'rows') return { ...s, items: s.items.map((r, i) => ({ ...r, id: id(r.id, si, i) })) }
      if (s.kind === 'chips') return { ...s, items: s.items.map((c, i) => ({ ...c, id: id(c.id, si, i) })) }
      if (s.kind === 'cards') return { ...s, items: s.items.map((c, i) => ({ ...c, id: id(c.id, si, i) })) }
      if (s.kind === 'fields') return { ...s, items: s.items.map((c, i) => ({ ...c, id: id(c.id, si, i) })) }
      return s
    }),
    actions: p.actions.map((a, i) => ({ ...a, id: id(a.id, 99, i) })),
  }
}

// What is wrong with a page, in words, for the tests and for a page's author: empty list means it can be drawn.
export function problems(p: SpecPageData): string[] {
  const out: string[] = []
  const at = `page ${p.key || '(no key)'}`
  if (!p.key) out.push(`${at}: no key`)
  if (!p.title.trim()) out.push(`${at}: no title`)
  if (!p.eyebrow.trim()) out.push(`${at}: no eyebrow`)
  const ids = new Set<string>()
  const seen = (id: string | undefined, what: string) => { if (id) { if (ids.has(id)) out.push(`${at}: id ${id} used twice (${what})`); ids.add(id) } }
  p.sections.forEach((s, si) => {
    if (s.kind === 'note') { if (!s.text.trim()) out.push(`${at}: section ${si} is a note with no text`); return }
    if (!s.items.length) out.push(`${at}: section ${si} (${s.h || 'no heading'}) has no items`)
    if (s.kind === 'fields') {
      for (const f of s.items) {
        if (!f.label.trim()) out.push(`${at}: section ${si} has a field with no label`)
        if (f.kind === 'select' && !f.options?.length) out.push(`${at}: select ${f.label} has no options`)
        if ((f.kind === 'qr') && !f.text) out.push(`${at}: qr ${f.label} says nothing`)
        if (f.kind === 'pin' && !(f.length && f.length > 0)) out.push(`${at}: pin ${f.label} has no length`)
        if (f.kind === 'slider' && (f.min === undefined || f.max === undefined || f.max <= f.min)) out.push(`${at}: slider ${f.label} has no range`)
        seen(f.id, f.label)
      }
      return
    }
    for (const it of s.items) { if (!('title' in it) || !it.title.trim()) out.push(`${at}: section ${si} has an item with no title`); seen(it.id, it.title) }
  })
  if (p.actions.filter((a) => a.kind === 'primary').length > 1) out.push(`${at}: more than one primary action`)
  p.actions.forEach((a) => { if (!a.title.trim()) out.push(`${at}: an action has no title`); seen(a.id, a.title) })
  return out
}

// A heading id for a section, stable and safe in an attribute.
export const sectionId = (key: string, i: number) => `spec-${key.replace(/[^a-z0-9]+/gi, '-')}-h${i}`

// What a person can reach with Tab: the items that go somewhere or are handled by id, and are not disabled.
// handled: the ids the page's onAction answers (SpecPage), or null for "every id".
export function reachable(p: SpecPageData, handled: ReadonlySet<string> | null): string[] {
  const out: string[] = []
  const ok = (it: { id?: string; to?: string; disabled?: boolean }) => !it.disabled && (!!it.to || (!!it.id && (!handled || handled.has(it.id))))
  for (const s of p.sections) if (s.kind === 'fields') { for (const f of s.items) if (f.kind !== 'code' && f.kind !== 'qr' && !f.disabled) out.push(f.id || '') } else if (s.kind !== 'note') for (const it of s.items) if (ok(it)) out.push(it.id || '')
  for (const a of p.actions) if (ok(a)) out.push(a.id || '')
  return out
}

// Where the canvas's invented wording says a thing is on: the first item marked, per section.
export const current = (s: SpecSection): number => (s.kind === 'note' || s.kind === 'fields' ? -1 : (s.items as { on?: boolean }[]).findIndex((x) => x.on))

// The ids a page answers itself (onAction): every item with no address of its own and not disabled. A page whose
// every item is handled passes this as SpecPage's `handled`.
export function handledIds(p: SpecPageData): Set<string> {
  const out = new Set<string>()
  const add = (it: { id?: string; to?: string; disabled?: boolean }) => { if (it.id && !it.to && !it.disabled) out.add(it.id) }
  for (const s of p.sections) if (s.kind !== 'note') (s.items as { id?: string; to?: string; disabled?: boolean }[]).forEach(add)
  p.actions.forEach(add)
  return out
}
