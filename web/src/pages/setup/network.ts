// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The words and lists for the Name and address step's network part, with nothing of the page in them (network.test.mjs).

export type HttpsStatus = { mode: string; running?: boolean; port?: number; error?: string; cert?: { names?: string[]; daysLeft?: number; selfSigned?: boolean } }

// What the server says about encrypted connections, for someone who has not set anything up.
export function httpsWords(h: HttpsStatus | null): string {
  if (!h) return 'Not read yet.'
  if (h.mode === 'off' || !h.mode) return 'Off. At home this is fine: a certificate needs a domain name, so it is set up under Remote Access if you want to reach the server from away.'
  if (h.error) return `Set up, but not working: ${h.error}.`
  if (!h.running) return 'Set up, and starting.'
  const days = h.cert?.daysLeft
  return `On, on port ${h.port || 9443}${h.cert?.names?.length ? ` for ${h.cert.names[0]}` : ''}${typeof days === 'number' ? `, the certificate good for ${days} more ${days === 1 ? 'day' : 'days'}` : ''}.`
}

// The zones to choose from: the computer's own first, then every zone the browser knows, the saved one kept even if it is not in the list.
export function timeZoneChoices(computer: string, saved: string, known: string[] = defaultZones()): { value: string; label: string }[] {
  const all = new Set(known)
  if (saved) all.add(saved)
  return [{ value: '', label: computer ? `The computer’s own (${computer})` : 'The computer’s own' }, ...[...all].sort().map((z) => ({ value: z, label: z.replace(/_/g, ' ') }))]
}

function defaultZones(): string[] {
  try { return (Intl as unknown as { supportedValuesOf: (k: string) => string[] }).supportedValuesOf('timeZone') } catch { return [] }
}
