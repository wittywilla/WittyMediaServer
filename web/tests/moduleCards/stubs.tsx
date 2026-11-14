// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What moduleRegistry.tsx reaches beyond itself, for moduleCards.test.tsx: one module in use,
// "wittyplay", whose bundle registers a card Provider and a full-window game screen. Nothing loads
// until the test lets it (arrive), as a real bundle arrives some time after the page is drawn.
import type { ReactNode } from 'react'

export const MODULE = 'wittyplay'
export const GAME_ROUTE = '/game/:id'

// The answer the app keeps from GET /modules/web, so the module is known before anything loads.
const part = { routes: [{ path: GAME_ROUTE, frame: 'fullscreen' }], cards: [{ mediaKind: 'game' }] }
const store = new Map<string, string>([['wms.moduleWeb.v1', JSON.stringify([{ module: MODULE, version: '1.0.0', part }])]])
;(globalThis as unknown as { localStorage: unknown }).localStorage = {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => { store.set(k, v) },
  removeItem: (k: string) => { store.delete(k) },
}

/** How many times the game screen was drawn: each one is a game started. */
export const drawn = { screen: 0 }

function Provider({ children }: { children: ReactNode }) {
  return <section data-cards="wittyplay">{children}</section>
}
function GameScreen() {
  drawn.screen++
  return <main>game</main>
}

let arrive: () => void = () => {}
const loaded = new Promise<void>((done) => { arrive = done })
/** The bundle arrives. */
export const letBundleArrive = () => arrive()

// ../src/moduleBundles
export async function bundledModule(id: string) {
  await loaded
  if (id !== MODULE) return null
  return { register: () => ({ cards: { game: { Provider } }, routes: { [GAME_ROUTE]: GameScreen } }) }
}
// ../src/moduleHost
export const hostFor = () => ({})
// ../src/api
export const api = { get: async () => ({ bundles: [] }), post: async () => ({}) }
// ../src/components/ModuleMissing
export const ModuleMissing = () => <p>missing</p>
