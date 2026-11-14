// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A module's card Provider (ModuleCards) and the module's own screens (ModuleRouteView), drawn as the
// app draws them. Opening a game from its item page used to start it twice: the full-window frame
// drew the game screen at once, then wrapped it in WittyPlay's card Provider once the page was idle,
// which drew the screen again and started a second game. Each render here is a component's first.
import './stubs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { renderToString } from 'react-dom/server'
import { MemoryRouter } from 'react-router-dom'
import { ModuleCards, ModuleRouteView, matchModuleRoute, registrationOf, useModuleWeb } from '../../src/moduleRegistry'
import { GAME_ROUTE, MODULE, drawn, letBundleArrive } from './stubs'

const route = { module: MODULE, route: { path: GAME_ROUTE, frame: 'fullscreen' as const }, params: { id: '7' } }

// The app's full-window frame around a module's screen, as App.tsx draws it.
const frame = () => renderToString(
  <MemoryRouter initialEntries={['/game/7']}>
    <ModuleCards hold fallback={<i>loading</i>}><ModuleRouteView match={route} fallback={<i>screen loading</i>} /></ModuleCards>
  </MemoryRouter>,
)

test('the module claims the game route and the game cards', () => {
  let list: ReturnType<typeof useModuleWeb> = []
  function Read() { list = useModuleWeb(); return null }
  renderToString(<Read />)
  assert.equal(matchModuleRoute(list, '/game/7', ['fullscreen'])?.module, MODULE)
})

test('before the bundle arrives, a full-window frame holds its screen back instead of drawing it without the Provider', () => {
  assert.equal(frame(), '<i>loading</i>')
  assert.equal(drawn.screen, 0)
})

test('once the bundle has arrived, a newly drawn frame has the Provider around its screen from its first render', async () => {
  letBundleArrive()
  await registrationOf(MODULE)
  assert.equal(frame(), '<section data-cards="wittyplay"><main>game</main></section>')
  assert.equal(drawn.screen, 1, 'the game screen is drawn once')
})

test('the app frame, which does not hold, also has the Provider at once after the bundle arrived', () => {
  assert.equal(renderToString(<ModuleCards><p>page</p></ModuleCards>), '<section data-cards="wittyplay"><p>page</p></section>')
})
