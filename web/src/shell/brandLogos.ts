// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Brand logos in WittyPlay: real console logos on its cartridges and cases, and as a console's mark. Off until an administrator turns
// them on in WittyPlay Settings › Game pictures and confirms the warning; then the server downloads them (Wikimedia Commons, through its
// logo service), keeps them with their source and licence, and serves them itself, so a browser never contacts anyone else
// (GET /api/games/brand-logos). Off, or with no logo for a console, WittyPlay draws its own generic shapes and the console's name.
import { useEffect, useState } from 'react'
import { api } from '../api'
import { logoImageUrl } from '../components/BrandLogo'

export type BrandLogoInfo = { id: number; version: number; opaque: boolean; name: string; source?: string; sourceUrl?: string; license?: string; licenseUrl?: string; credit?: string }
export type BrandLogosState = { enabled: boolean; logos: Record<string, BrandLogoInfo>; warning: string[]; appWide?: string; pending?: number; agreedAt?: string }

// Where it is turned on, and the words a person confirms (the server sends its own copy; these are for before it answers).
export const BRAND_LOGOS_SETTING = '/wittyplay/settings/pictures#brand-logos'
export const BRAND_LOGO_WARNING = [
  'Console and game studio logos are trademarks of their owners.',
  'They are downloaded from Wikimedia Commons for personal, non-commercial display on this server only.',
  'This server and WittyPlay are not endorsed by, sponsored by or affiliated with any of the owners.',
  'You are responsible for how they are used here.',
  'Each logo’s source and licence is recorded and shown in the credits. You can remove every downloaded logo at any time.',
]

const OFF: BrandLogosState = { enabled: false, logos: {}, warning: BRAND_LOGO_WARNING }
let asked: Promise<BrandLogosState> | null = null
const CHANGED = 'wms-brand-logos-changed'

export function loadBrandLogos(fresh = false): Promise<BrandLogosState> {
  if (!asked || fresh) {
    const p = api.get<BrandLogosState>('/games/brand-logos').then((s) => ({ ...OFF, ...s, logos: s.logos || {} }))
    asked = p
    p.catch(() => { if (asked === p) asked = null })
  }
  return asked.catch(() => OFF)
}

// After turning them on or off, or removing them: whatever shows them asks again.
export function brandLogosChanged(next?: BrandLogosState) {
  asked = next ? Promise.resolve({ ...OFF, ...next, logos: next.logos || {} }) : null
  window.dispatchEvent(new Event(CHANGED))
}

export function useBrandLogos(): BrandLogosState | null {
  const [s, setS] = useState<BrandLogosState | null>(null)
  useEffect(() => {
    let on = true
    const ask = () => { void loadBrandLogos().then((x) => { if (on) setS(x) }) }
    ask()
    window.addEventListener(CHANGED, ask)
    return () => { on = false; window.removeEventListener(CHANGED, ask) }
  }, [])
  return s
}

// A console's downloaded logo, by its id ("snes"), as an address on this server, with its credit: null while asking, when brand logos
// are off, or when there is none (show the console's name then). variant: white on a dark ground, dark on a light one; an opaque logo
// (one whose background couldn't be taken off) is always its original, to be shown on a plate.
export function useConsoleLogo(consoleId: string | undefined, variant: 'white' | 'dark' | 'original' = 'white'): { url: string; info: BrandLogoInfo } | null {
  const s = useBrandLogos()
  const info = consoleId && s?.enabled ? s.logos[consoleId] : undefined
  if (!info) return null
  return { url: logoImageUrl(info, info.opaque ? 'original' : variant), info }
}
