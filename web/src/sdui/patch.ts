// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The diff endpoint's patches, applied to the document a client holds (docs/SDUI.md,
// "Patch protocol"). Nothing is changed in place: the answer is a new document, or null
// when a step can't be carried out, which means "drop the tree and ask for the page".

import type { Document, Node, Patch } from './types.ts'

const FIELDS = ['page', 'layout', 'theme', 'personal', 'screenTime', 'catalog', 'meta']

function clone<T>(v: T): T { return JSON.parse(JSON.stringify(v)) as T }

function locate(root: Node, id: string): { node: Node; parent: Node | null; index: number } | null {
  if (root.id === id) return { node: root, parent: null, index: -1 }
  const stack: Node[] = [root]
  while (stack.length) {
    const p = stack.pop()!
    const kids = p.children || []
    for (let i = 0; i < kids.length; i++) {
      if (kids[i].id === id) return { node: kids[i], parent: p, index: i }
      stack.push(kids[i])
    }
  }
  return null
}

// "#id" or "#id/children/n". An ID may itself hold "/"-free punctuation only, so the
// last "/children/" is the split.
function parse(path: string): { id: string; index: number } | null {
  if (!path.startsWith('#')) return null
  const at = path.lastIndexOf('/children/')
  if (at < 0) return { id: path.slice(1), index: -1 }
  const n = Number(path.slice(at + 10))
  if (!Number.isInteger(n) || n < 0) return null
  return { id: path.slice(1, at), index: n }
}

export function applyPatches(doc: Document, patches: Patch[]): Document | null {
  const out = clone(doc)
  for (const p of patches) {
    if (p.path.startsWith('/')) {
      const field = p.path.slice(1)
      if (p.op !== 'replace') return null
      if (field === 'root') { if (!p.node) return null; out.root = clone(p.node); continue }
      if (!FIELDS.includes(field)) return null
      const bag = out as unknown as Record<string, unknown>
      if (p.value === null || p.value === undefined) delete bag[field]
      else bag[field] = clone(p.value)
      continue
    }
    const at = parse(p.path)
    if (!at) return null
    const hit = locate(out.root, at.id)
    if (!hit) return null
    switch (p.op) {
      case 'add': {
        const kids = (hit.node.children ||= [])
        if (!p.node || at.index < 0 || at.index > kids.length) return null
        kids.splice(at.index, 0, clone(p.node))
        break
      }
      case 'remove':
        if (!hit.parent || at.index >= 0) return null
        hit.parent.children!.splice(hit.index, 1)
        break
      case 'replace': {
        if (!p.node || at.index >= 0) return null
        const keep = hit.node.children
        const next = clone(p.node)
        if (keep) next.children = keep; else delete next.children
        if (hit.parent) hit.parent.children![hit.index] = next; else out.root = next
        break
      }
      case 'move': {
        const from = p.from?.startsWith('#') ? locate(out.root, p.from.slice(1)) : null
        if (!from || from.parent !== hit.node || at.index < 0) return null
        const kids = hit.node.children!
        const [c] = kids.splice(from.index, 1)
        if (at.index > kids.length) return null
        kids.splice(at.index, 0, c)
        break
      }
      default: return null
    }
  }
  return out
}
