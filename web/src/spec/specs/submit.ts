// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Enter in a text, password, number or PIN field presses the page's primary button, as a form would. A field that
// has its own meaning for Enter (a select, a file picker, a button) is left alone.
export function isSubmitKey(e: { key: string; defaultPrevented: boolean; shiftKey?: boolean; target: EventTarget }): boolean {
  if (e.key !== 'Enter' || e.defaultPrevented || e.shiftKey) return false
  const t = e.target as { tagName?: string; type?: string }
  if (t.tagName !== 'INPUT') return false
  return !['file', 'checkbox', 'radio', 'range', 'button', 'submit'].includes(t.type || 'text')
}
