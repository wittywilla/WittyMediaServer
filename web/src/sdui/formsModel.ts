// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What forms and controls work out, with nothing of the page in it (forms.test.mjs).
import type { Action, Node, Props } from './types'

export type Control = { type: 'switch' | 'select' | 'field' | 'slider'; name?: string; value?: unknown; options?: { value: string; label: string }[]; placeholder?: string; maxLength?: number; multiline?: boolean; input?: string; min?: number; max?: number; step?: number; unit?: string; disabled?: boolean; why?: string }

export const controlOf = (p: Props | undefined): Control | null => {
  const c = p?.control
  return c && typeof c === 'object' && !Array.isArray(c) && ['switch', 'select', 'field', 'slider'].includes((c as Control).type) ? c as Control : null
}

// The action a changed control runs outside a Form: the row's own, with the new value added to its params (a switch's is already there).
export const withValue = (a: Action | undefined, c: Control, v: unknown): Action | undefined =>
  a && (c.type === 'switch' ? a : { ...a, params: { ...(a.params || {}), value: v } })

// What Save runs: the Form's command with every named control's value under its name.
export const saveAction = (a: Action | undefined, values: Record<string, unknown>): Action | undefined => a && { ...a, params: { ...(a.params || {}), ...values } }


// formValues: every named control inside a Form with its value, the changes laid over what the server sent. A number field's
// value is a number; a control left empty is left out (an object param then has no such key).
export function formValues(nodes: Node[] | undefined, changed: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  const numbers = new Set<string>()
  const walk = (list: Node[] | undefined) => {
    for (const n of list || []) {
      const c = controlOf(n.props)
      if (c?.name) { if (c.value !== undefined) out[c.name] = c.value; if (c.type === 'field' && c.input === 'number') numbers.add(c.name) }
      walk(n.children)
    }
  }
  walk(nodes)
  Object.assign(out, changed)
  for (const [k, v] of Object.entries(out)) {
    if (v === '' || v === undefined || v === null) delete out[k]
    else if (numbers.has(k) && typeof v === 'string' && v.trim() !== '' && Number.isFinite(Number(v))) out[k] = Number(v)
  }
  return out
}
