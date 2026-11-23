// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A page with something in progress on it (docs/SDUI.md, "Pages that are in progress"):
// meta.refreshSec says to ask for it again every so many seconds while it is open. These
// are the rules; SduiPage keeps the timer.

import type { Document, Node } from './types.ts'

// A section whose items didn't come (a module's source that failed) is asked for again by itself
// after these waits, then every 15 s: never in a tight loop, while its error shows once. Retry asks
// at once.
export const SECTION_RETRY_MS = [1000, 2000, 5000, 15000]

// sectionRetryDelay: how long to wait before asking again after a section has failed n times (1 on).
export function sectionRetryDelay(n: number): number {
  return SECTION_RETRY_MS[Math.min(Math.max(n, 1), SECTION_RETRY_MS.length) - 1]
}

// The server never asks for more than MAX; a client never asks sooner than MIN, whatever it says.
export const MIN_REFRESH_SEC = 1
export const MAX_REFRESH_SEC = 300

// refreshDelay: how long after an answer to ask for the page again, in milliseconds; 0: don't.
export function refreshDelay(doc: Pick<Document, 'meta'> | null | undefined): number {
  const n = Number(doc?.meta?.refreshSec) || 0
  if (!(n > 0)) return 0
  return Math.min(MAX_REFRESH_SEC, Math.max(MIN_REFRESH_SEC, n)) * 1000
}

const words = (n: Node) => {
  const p = n.props || {}
  const title = typeof p.title === 'string' ? p.title : ''
  const end = typeof p.trailing === 'string' && p.trailing !== title ? p.trailing : ''
  return [title, end].filter(Boolean).join(', ')
}

function rows(root: Node, into: Map<string, string>) {
  if (root.type === 'ListItem') into.set(root.id, words(root))
  for (const c of root.children || []) rows(c, into)
}

// changedRows: the rows of `now` whose words differ from the same rows in `was` (matched by id),
// in page order. When a page in progress settles the first of them ("Last scanned …") is read
// out once, politely, rather than the whole page again or every step of the count.
export function changedRows(was: Node | null | undefined, now: Node): string[] {
  if (!was) return []
  const before = new Map<string, string>(), after = new Map<string, string>()
  rows(was, before)
  rows(now, after)
  const out: string[] = []
  for (const [id, text] of after) {
    if (text && before.has(id) && before.get(id) !== text) out.push(text)
  }
  return out
}
