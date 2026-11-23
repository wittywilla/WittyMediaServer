// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

import { num, t } from './i18n'

/** A file's size in the unit that fits it: a 700 KB book reads "700 KB", not "0.00 GB". Decimal units, as drives sell them. */
export function fileSizeText(bytes: number): string {
  if (bytes >= 1e9) return t('{size} GB', { size: num(bytes / 1e9, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) })
  if (bytes >= 1e6) return t('{size} MB', { size: num(bytes / 1e6, { maximumFractionDigits: bytes < 1e7 ? 1 : 0 }) })
  return t('{size} KB', { size: num(Math.max(1, Math.round(bytes / 1e3))) })
}
