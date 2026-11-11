// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { ReactNode } from 'react'
import { safeLink } from '../safeLink'

// A link to an address that came from outside, opening in a new tab. Anything that isn't a
// web address shows its words without the link.
export function WebLink({ href, className, children }: { href?: string | null; className?: string; children: ReactNode }) {
  const to = safeLink(href)
  if (!to) return <>{children}</>
  return <a className={className} href={to} target="_blank" rel="noopener noreferrer nofollow">{children}</a>
}
