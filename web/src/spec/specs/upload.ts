// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, R, mk, secR } from '../model.ts'
import type { SpecPageData } from '../types.ts'

// Upload (canvas key "upload"): what the server refused, one row a file with its reason. Action id: close.
export function uploadErrorsSpec(errors: { name: string; error?: string }[]): SpecPageData {
  return mk('upload', 'Upload', `${errors.length} ${errors.length === 1 ? 'file' : 'files'} not added`, 'The rest were added.',
    [secR('Refused', errors.map((e, i) => ({ ...R(e.name, 'Refused', false, e.error), id: 'file' + i })))], [{ ...A1('Close'), id: 'close' }])
}
