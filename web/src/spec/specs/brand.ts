// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secP } from '../model.ts'
import type { SpecChip, SpecPageData } from '../types.ts'

// A studio or network's page (canvas key "brand"): its two controls beside the sort. The titles stay a grid.
// Chip ids: all-libraries, change-logo.
export function brandSpec(kind: 'studio' | 'network', name: string, inLibrary: boolean, canChangeLogo: boolean): SpecPageData {
  const chips: SpecChip[] = []
  if (inLibrary) chips.push({ id: 'all-libraries', title: 'All libraries' })
  if (canChangeLogo) chips.push({ id: 'change-logo', title: 'Change logo' })
  return mk('brand', `${kind === 'network' ? 'Network' : 'Studio'} · ${name}`, name, '', chips.length ? [secP('', chips)] : [])
}
