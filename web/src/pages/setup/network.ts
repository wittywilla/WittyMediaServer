// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The words and lists for the Name and address step's network part, with nothing of the page in them (network.test.mjs).
import { ENGLISH, type Words } from './words.ts'

export type HttpsStatus = { mode: string; running?: boolean; port?: number; error?: string; cert?: { names?: string[]; daysLeft?: number; selfSigned?: boolean } }

// What the server says about encrypted connections, for someone who has not set anything up.
export function httpsWords(h: HttpsStatus | null, { t, tn }: Words = ENGLISH): string {
  if (!h) return t('Not read yet.')
  if (h.mode === 'off' || !h.mode) return t('Off. At home this is fine: a certificate needs a domain name, so it is set up under Remote access if you want to reach the server from away.')
  if (h.error) return t('Set up, but not working: {error}.', { error: h.error })
  if (!h.running) return t('Set up, and starting.')
  const days = h.cert?.daysLeft
  // The port is text, so it isn't written with a thousands separator.
  const port = String(h.port || 9443)
  const name = h.cert?.names?.[0]
  if (typeof days === 'number') {
    return name
      ? tn('On, on port {port} for {name}, the certificate good for {count} more day.', 'On, on port {port} for {name}, the certificate good for {count} more days.', days, { port, name })
      : tn('On, on port {port}, the certificate good for {count} more day.', 'On, on port {port}, the certificate good for {count} more days.', days, { port })
  }
  return name ? t('On, on port {port} for {name}.', { port, name }) : t('On, on port {port}.', { port })
}

// The zones to choose from: the computer's own first, then every zone the browser knows, the saved one kept even if it is not in the list.
export function timeZoneChoices(computer: string, saved: string, known: string[] = defaultZones(), { t }: Words = ENGLISH): { value: string; label: string }[] {
  const all = new Set(known)
  if (saved) all.add(saved)
  // Go names Windows' zone "Local", which says nothing: the computer's own, plainly.
  const named = computer && computer !== 'Local' ? computer : ''
  return [{ value: '', label: named ? t('The computer’s own ({zone})', { zone: named }) : t('The computer’s own') }, ...[...all].sort().map((z) => ({ value: z, label: z.replace(/_/g, ' ') }))]
}

function defaultZones(): string[] {
  try { return (Intl as unknown as { supportedValuesOf: (k: string) => string[] }).supportedValuesOf('timeZone') } catch { return [] }
}
