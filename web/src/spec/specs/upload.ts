// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, R, mk, secR } from '../model.ts'
import type { SpecPageData } from '../types.ts'
import { t, tn } from '../../sdui/words.ts'

// Upload (canvas key "upload"): what the server refused, one row a file with its reason. Action id: close.
export function uploadErrorsSpec(errors: { name: string; error?: string }[]): SpecPageData {
  return mk('upload', t('Upload'), tn('{count} file not added', '{count} files not added', errors.length), t('The rest were added.'),
    [secR(t('Refused'), errors.map((e, i) => ({ ...R(e.name, t('Refused'), false, e.error), id: 'file' + i })))], [{ ...A1(t('Close')), id: 'close' }])
}
