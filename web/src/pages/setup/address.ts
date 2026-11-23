// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The server's address for setup's last page (#536): always a best guess, so there is something to write down or copy.
// It once showed nothing when the server listened on this computer only, or was opened at localhost. Nothing here
// touches the page or the network, so it's tested on its own (address.test.mjs).

// What GET /api/system says about the home network.
export type HomeAddresses = {
  // Addresses other devices at home can open now, the main one first; empty when it listens on this computer only.
  lanUrls?: string[]
  // Its address on the home network, there even when it listens on this computer only.
  lanGuess?: string
  localOnly?: boolean
}

export type BestAddress = {
  // The address to show and copy.
  url: string
  // Other devices at home can open it: worth a QR code.
  reachable: boolean
  // Another address that works too, if there is one.
  also?: string
  // Where it would be once it listens on the network, when it doesn't yet.
  later?: string
}

// A name only this computer answers to: localhost, 127.x, ::1 and any *.localhost.
export const onlyHere = (hostname: string) =>
  /^(localhost|127\.\d+\.\d+\.\d+|\[::1\]|::1)$/i.test(hostname) || hostname.toLowerCase().endsWith('.localhost')

export function bestAddress(page: { origin: string; hostname: string }, sys: HomeAddresses | null): BestAddress {
  const lan = sys?.lanUrls || []
  // Opened at an address other devices can use too (a LAN address or a name): that one, as people already know it.
  if (!onlyHere(page.hostname) && !sys?.localOnly) return { url: page.origin, reachable: true, also: lan.find((u) => u !== page.origin) }
  if (lan.length) return { url: lan[0], reachable: true, also: lan[1] }
  return { url: page.origin, reachable: false, later: sys?.lanGuess || undefined }
}
