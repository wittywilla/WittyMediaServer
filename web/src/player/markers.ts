// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Where a video's skippable parts are, as GET /api/media/{id}/markers answers: from what
// an administrator set, a .skip.json or .edl beside the file, chapter names, what comparing
// the season's episodes found, and the show's own intro. This picks what to offer now.

export type Range = { startSec: number; endSec: number }
export type Markers = { intro?: Range | null; recap?: Range | null; credits?: Range | null; commercials?: Range[] | null }
export type Skip = { kind: 'intro' | 'recap' | 'credits' | 'commercial'; label: string; to: number }

const LABEL: Record<Skip['kind'], string> = { intro: 'Skip intro', recap: 'Skip recap', credits: 'Skip credits', commercial: 'Skip adverts' }

// skipAt is the part playing at now, if skipping it is worth a button: more than a second
// of it is left, and for credits there is something after them (credits that run to the
// end are what "next episode" is for). null when there is nothing to skip.
export function skipAt(m: Markers | null, now: number, duration: number): Skip | null {
  if (!m) return null
  const inside = (r?: Range | null) => !!r && r.endSec > r.startSec && now >= r.startSec && now < r.endSec - 1
  if (inside(m.recap)) return { kind: 'recap', label: LABEL.recap, to: m.recap!.endSec }
  if (inside(m.intro)) return { kind: 'intro', label: LABEL.intro, to: m.intro!.endSec }
  for (const c of m.commercials || []) if (inside(c)) return { kind: 'commercial', label: LABEL.commercial, to: c.endSec }
  if (inside(m.credits) && duration > 0 && m.credits!.endSec < duration - 5) return { kind: 'credits', label: LABEL.credits, to: m.credits!.endSec }
  return null
}
