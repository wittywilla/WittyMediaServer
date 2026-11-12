// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { applyPatches } from './patch.ts'
import { plan, resolve, withQuery } from './routes.ts'
import { focusMap, next } from './focus.ts'
import { body, draftOf, fromOldHome, fromOldNav, move, setCardSize, setHidden, setPinned } from './personal.ts'
import { breakpoint, describe, deviceKey, query } from './device.ts'

const fixture = (name) => JSON.parse(readFileSync(new URL(`../../../pkg/sdui/conformance/fixtures/${name}.json`, import.meta.url), 'utf8')).expected.document

const doc = () => ({
  version: 1, catalog: 2, page: { id: 'p', kind: 'custom', title: 'P' }, layout: { formFactor: 'web', breakpoint: 'large', spacing: {}, text: {} }, meta: {},
  root: { type: 'Page', id: 'page', children: [
    { type: 'Shelf', id: 'a', props: { title: 'A' }, children: [{ type: 'Card', id: 'a:1' }, { type: 'Card', id: 'a:2' }] },
    { type: 'Text', id: 't', props: { text: 'hello' } },
  ] },
})

test('patches: add, remove, replace keeps children, move, and document fields', () => {
  const out = applyPatches(doc(), [
    { op: 'add', path: '#a/children/1', node: { type: 'Card', id: 'a:9' } },
    { op: 'remove', path: '#a:1' },
    { op: 'replace', path: '#a', node: { type: 'Shelf', id: 'a', props: { title: 'Again' } } },
    { op: 'move', path: '#page/children/0', from: '#t' },
    { op: 'replace', path: '/page', value: { id: 'p', kind: 'custom', title: 'Renamed' } },
    { op: 'replace', path: '/screenTime', value: { message: 'Enough for today' } },
    { op: 'replace', path: '/theme', value: null },
  ])
  assert.deepEqual(out.root.children.map((c) => c.id), ['t', 'a'])
  assert.equal(out.root.children[1].props.title, 'Again')
  assert.deepEqual(out.root.children[1].children.map((c) => c.id), ['a:9', 'a:2'])
  assert.equal(out.page.title, 'Renamed')
  assert.equal(out.screenTime.message, 'Enough for today')
  assert.equal('theme' in out, false)
})

test('patches: the document held is never changed, and a step that fails gives nothing', () => {
  const d = doc()
  const before = JSON.stringify(d)
  assert.equal(applyPatches(d, [{ op: 'remove', path: '#a:1' }, { op: 'remove', path: '#nobody' }]), null)
  assert.equal(applyPatches(d, [{ op: 'add', path: '#a/children/7', node: { type: 'Card', id: 'x' } }]), null)
  assert.equal(applyPatches(d, [{ op: 'move', path: '#a/children/0', from: '#t' }]), null)
  assert.equal(applyPatches(d, [{ op: 'explode', path: '#a' }]), null)
  assert.equal(JSON.stringify(d), before)
  const root = applyPatches(d, [{ op: 'replace', path: '/root', node: { type: 'Page', id: 'page' } }])
  assert.equal(root.root.children, undefined)
})

test('routes: an address is a page with parameters, or one of the app\'s own screens', () => {
  assert.deepEqual(resolve('/'), { kind: 'page', page: 'home', params: {} })
  assert.deepEqual(resolve('/library/3', '?genre=Comedy&sort=rating'), { kind: 'page', page: 'library', params: { library: '3', sort: 'rating', genre: 'Comedy' } })
  assert.deepEqual(resolve('/album/4', '?album=Blue%20Train&artist=A%26B'), { kind: 'page', page: 'detail', params: { album: 'Blue Train', artist: 'A&B', library: '4' } })
  assert.deepEqual(resolve('/show/9/'), { kind: 'page', page: 'detail', params: { show: '9' } })
  assert.deepEqual(resolve('/collections/12'), { kind: 'page', page: 'detail', params: { collection: '12' } })
  assert.deepEqual(resolve('/search', '?q=moon'), { kind: 'page', page: 'search', params: { q: 'moon' } })
  assert.deepEqual(resolve('/page/family-night', '?tab=b'), { kind: 'page', page: 'family-night', params: { tab: 'b' } })
  assert.deepEqual(resolve('/settings/appearance'), { kind: 'native', screen: 'settings', vars: { section: 'appearance' } })
  assert.deepEqual(resolve('/tv/guide'), { kind: 'native', screen: 'guide', vars: {} })
  assert.equal(resolve('/help/getting-started'), null)
})

test('actions: each type comes to one step, and anything unknown or unsafe to nothing', () => {
  assert.deepEqual(plan({ type: 'navigate', to: '/item/3' }), { do: 'go', to: '/item/3' })
  assert.deepEqual(plan({ type: 'navigate', to: '//elsewhere.example' }), { do: 'nothing' })
  assert.deepEqual(plan({ type: 'play', item: '3' }), { do: 'play', item: '3', resume: true })
  assert.deepEqual(plan({ type: 'play', item: '3', params: { resume: false } }), { do: 'play', item: '3', resume: false })
  assert.deepEqual(plan({ type: 'native', to: 'settings' }), { do: 'go', to: '/settings' })
  assert.deepEqual(plan({ type: 'native', to: 'share', params: { item: '7' } }), { do: 'screen', screen: 'share', item: '7' })
  assert.deepEqual(plan({ type: 'external', url: 'https://example.org/' }), { do: 'open', url: 'https://example.org/' })
  assert.deepEqual(plan({ type: 'external', url: 'javascript:alert(1)' }), { do: 'nothing' })
  assert.deepEqual(plan({ type: 'external', url: 'https://example.org/' }, { externalLinks: false }), { do: 'nothing' })
  assert.deepEqual(plan({ type: 'back' }), { do: 'back' })
  assert.deepEqual(plan({ type: 'command', op: 'watched.set', item: '5' }), { do: 'request', method: 'POST', path: '/items/5/progress', body: { watched: true } })
  assert.deepEqual(plan({ type: 'command', op: 'watched.clear', item: '5' }), { do: 'request', method: 'DELETE', path: '/items/5/progress' })
  assert.deepEqual(plan({ type: 'command', op: 'watchLater.add', item: '5' }), { do: 'request', method: 'POST', path: '/collections/watch-later/items', body: { items: [5] } })
  assert.deepEqual(plan({ type: 'command', op: 'watchLater.add', item: 'show-8' }), { do: 'request', method: 'POST', path: '/collections/watch-later/items', body: { shows: [8] } })
  assert.deepEqual(plan({ type: 'command', op: 'selectTab', params: { tab: 'artists' } }), { do: 'tab', tabs: '', tab: 'artists' })
  assert.deepEqual(plan({ type: 'command', op: 'refresh' }), { do: 'refresh' })
  assert.deepEqual(plan({ type: 'command', op: 'format-disk' }), { do: 'nothing' })
  assert.deepEqual(plan({ type: 'teleport', to: '/x' }), { do: 'nothing' })
  assert.deepEqual(plan(undefined), { do: 'nothing' })
  assert.equal(withQuery('/library/1', '?genre=Drama&sort=title', { genre: '', decade: '1990' }), '/library/1?sort=title&decade=1990')
})

test('focus: the page\'s order decides where the arrows go, never geometry', () => {
  const home = fixture('home-tv-hd')
  const map = focusMap(home.root)
  assert.equal(map.start, 'hero')
  const rows = map.stops.filter((s) => s.kind === 'row')
  const first = rows[0], second = rows[1]
  assert.ok(first.members.length > 1 && second.members.length > 1)
  // Along a row, and stopping at its ends.
  assert.equal(next(map, first.members[0], 'right'), first.members[1])
  assert.equal(next(map, first.members[0], 'left'), null)
  assert.equal(next(map, first.members.at(-1), 'right'), null)
  // Down to the next row's first card, or to where that row was left.
  assert.equal(next(map, first.members[1], 'down'), second.members[0])
  assert.equal(next(map, first.members[1], 'down', { [second.id]: second.members[2] }), second.members[2])
  assert.equal(next(map, second.members[2], 'up', { [first.id]: first.members[1] }), first.members[1])
  // The spotlight is one stop above the first row; nothing is above it.
  assert.equal(next(map, 'hero', 'down'), map.stops[1].members[0])
  assert.equal(next(map, 'hero', 'up'), null)
  // Every focusable node is in exactly one stop, in the page's order.
  assert.deepEqual(map.stops.flatMap((s) => s.members).length, Object.keys(map.stopOf).length)
})

test('focus: a grid moves by row and column', () => {
  const lib = fixture('library-films-tv')
  const map = focusMap(lib.root)
  const grid = map.stops.find((s) => s.kind === 'grid')
  assert.equal(grid.columns, 6)
  const m = grid.members
  assert.equal(next(map, m[0], 'right'), m[1])
  assert.equal(next(map, m[5], 'right'), null)
  assert.equal(next(map, m[6], 'left'), null)
  assert.equal(next(map, m[1], 'down'), m[7])
  assert.equal(next(map, m[7], 'up'), m[1])
  const above = map.stops[map.stopOf[m[0]] - 1]
  assert.equal(next(map, m[2], 'up'), above.members[0])
  // The window is narrower than the server was told: the page says how many columns it drew.
  const narrow = focusMap(lib.root, () => 4)
  assert.equal(next(narrow, m[1], 'down'), m[5])
  // A short last row is still reachable from the row above it.
  const short = focusMap({ type: 'Page', id: 'page', children: [{ type: 'Grid', id: 'g', props: { columns: 3 }, children: [1, 2, 3, 4].map((n) => ({ type: 'Card', id: 'g:' + n, focus: { order: n, group: 'g' } })) }] })
  assert.equal(next(short, 'g:3', 'down'), 'g:4')
  assert.equal(next(short, 'g:4', 'down'), null)
})

test('customise: move, hide and pin as the server will apply them', () => {
  const info = { editable: true, allow: ['reorder', 'hide', 'pin', 'density', 'cardSize'], url: '/x', sections: [
    { id: 'setup', title: 'Notice', locked: true }, { id: 'hero', title: 'Featured' }, { id: 'jump', title: 'Jump back in' }, { id: 'recent-1', title: 'Films' },
  ] }
  let d = draftOf(info)
  d = move(d, 'recent-1', 1)
  assert.deepEqual(d.rows.map((r) => r.id), ['setup', 'recent-1', 'hero', 'jump'])
  assert.equal(move(d, 'setup', 2), d, 'a locked row never moves')
  assert.equal(move(d, 'hero', 0), d, 'and nothing takes its place')
  d = setPinned(d, 'jump', true)
  assert.deepEqual(d.rows.map((r) => r.id), ['setup', 'jump', 'recent-1', 'hero'])
  d = setHidden(d, 'hero', true)
  d = setHidden(d, 'setup', true)
  d = setCardSize(d, 'recent-1', 'l')
  d = setCardSize(d, 'jump', 'm')
  assert.deepEqual(body(d, info), { order: ['recent-1', 'hero'], hidden: ['hero'], pinned: ['jump'], cardSize: { 'recent-1': 'l' } })
  // Hiding a pinned row unpins it; only what the page allows is sent.
  d = setHidden(d, 'jump', true)
  assert.deepEqual(body(d, { ...info, allow: ['hide'] }), { hidden: ['jump', 'hero'] })
  assert.deepEqual(body(d, { ...info, editable: false }), {})
})

test('customise: what the web app kept before carries over', () => {
  assert.deepEqual(fromOldHome({ v: 2, order: ['jump', 'hero'], hidden: ['continue-reading', 'onnow'] }), { order: ['jump', 'hero'], hidden: ['jump', 'onnow'] })
  assert.deepEqual(fromOldHome({ v: 1, order: ['continue'], hidden: [] }), null)
  assert.deepEqual(fromOldNav({ order: ['lib:2', 'home', 'tv'], hidden: ['collections', 'settings'], grouped: true }), { order: ['lib-2', 'home', 'livetv'], hidden: ['collections'] })
  assert.equal(fromOldNav(null), null)
})

test('device: what each kind of window says it is', () => {
  const facts = { width: 1440, height: 900, tvMode: false, desktopShell: false, touch: false, pointer: true, reducedMotion: false, hdr: false, locale: 'en-GB', uhd: false }
  assert.equal(describe(facts).formFactor, 'web')
  assert.equal(describe({ ...facts, desktopShell: true }).formFactor, 'desktop')
  assert.equal(describe({ ...facts, desktopShell: true }).platform, 'desktop')
  assert.equal(describe({ ...facts, width: 390 }).formFactor, 'phone')
  assert.equal(describe({ ...facts, width: 820, touch: true, pointer: false }).formFactor, 'tablet')
  const tv = describe({ ...facts, tvMode: true, uhd: true })
  assert.deepEqual([tv.formFactor, tv.deviceClass, tv.viewport], ['tv', '4k', undefined])
  assert.equal(breakpoint(599), 'compact'); assert.equal(breakpoint(600), 'medium'); assert.equal(breakpoint(840), 'expanded'); assert.equal(breakpoint(1200), 'large')
  assert.notEqual(deviceKey(describe(facts), 1440), deviceKey(describe(facts), 1000))
  assert.equal(deviceKey(describe(facts), 1440), deviceKey(describe(facts), 1300))
  const q = new URLSearchParams(query(describe({ ...facts, reducedMotion: true }), { library: '3', tab: undefined }))
  assert.equal(q.get('catalog'), '6'); assert.equal(q.get('viewport'), '1440x900'); assert.equal(q.get('library'), '3')
  assert.equal(q.get('views'), 'guide,photoTimeline,photoMap,coverFlow'); assert.ok(q.get('capabilities').includes('reducedMotion')); assert.equal(q.has('tab'), false)
})

test('the app\'s own tools: which pages get them, and which music tabs open the app\'s own page', async () => {
  const { toolsFor, ownLibraryTab } = await import('./native.ts')
  const libs = [{ id: 1, type: 'movies' }, { id: 2, type: 'music' }, { id: 3, type: 'photos' }]
  const page = (kind, params, root = { type: 'Page', id: 'p', children: [] }) => ({ page: { id: kind, kind, params }, root })
  assert.equal(toolsFor(page('library', { library: '1' }), libs), 'video')
  assert.equal(toolsFor(page('library', { library: '2' }), libs), 'audio')
  assert.equal(toolsFor(page('library', { library: '3' }), libs), null)
  assert.equal(toolsFor(page('collections', {}), libs), 'collections')
  assert.equal(toolsFor(page('detail', { collection: '4' }, { type: 'Page', id: 'p', children: [{ type: 'DetailHeader', id: 'h', props: { kind: 'collection' } }] }), libs), 'collection')
  assert.equal(toolsFor(page('detail', { item: '4' }, { type: 'Page', id: 'p', children: [{ type: 'DetailHeader', id: 'h', props: { kind: 'movie' } }] }), libs), null)
  assert.equal(ownLibraryTab({ library: '2', tab: 'songs' }, libs), true)
  assert.equal(ownLibraryTab({ library: '2', tab: 'playlists' }, libs), true)
  assert.equal(ownLibraryTab({ library: '2', tab: 'albums' }, libs), false)
  assert.equal(ownLibraryTab({ library: '1', tab: 'songs' }, libs), false)
})

test('a browser names itself once, randomly, and the name goes with every request', async () => {
  const { deviceName, describe, query } = await import('./device.ts')
  const kept = new Map()
  const store = { getItem: (k) => kept.get(k) ?? null, setItem: (k, v) => kept.set(k, v) }
  const a = deviceName(false, store, () => 'ab12cd34ef')
  assert.match(a, /^web-[a-z0-9]+$/)
  assert.equal(deviceName(false, store, () => 'zzzz'), a, 'kept')
  assert.match(deviceName(true, new Map() && { getItem: () => null, setItem: () => {} }, () => 'q1'), /^desktop-q1$/)
  const d = describe({ width: 1280, height: 800, tvMode: false, desktopShell: false, touch: false, pointer: true, reducedMotion: false, hdr: false, locale: 'en', uhd: false }, [], a)
  assert.equal(new URLSearchParams(query(d)).get('device'), a)
})

test('a streaming-service filter goes to a page as service, never as the device\'s platform', () => {
  const r = resolve('/library/3', '?platform=netflix&genre=Drama')
  assert.equal(r.params.service, 'netflix')
  assert.equal(r.params.platform, undefined)
  assert.equal(r.params.genre, 'Drama')
  assert.equal(resolve('/library/3', '?service=hulu').params.service, 'hulu')
})

test('catalogue commands: {item} and the parameters fill the path and the body', async () => {
  const { commandRequest, plan, setCommandSpecs } = await import('./routes.ts')
  const join = { op: 'wittyplay.room.join', method: 'POST', path: '/api/play/join', body: '{"room":"{item}","code":"{code}"}', params: [{ name: 'code', type: 'string' }] }
  assert.deepEqual(commandRequest(join, { type: 'command', op: join.op, item: 'abc' + '"', params: { code: 'K 9"' } }), { do: 'request', method: 'POST', path: '/play/join', body: { room: 'abc"', code: 'K 9"' } })
  // Not given: an empty string.
  assert.deepEqual(commandRequest(join, { type: 'command', op: join.op, item: 's1' }).body, { room: 's1', code: '' })
  const assign = { op: 'wittyplay.controller.assign', method: 'PUT', path: '/api/play/{item}/seat/{user}', body: '{"user":"{user}","seat":"{seat}"}', params: [{ name: 'user', type: 'string', required: true }, { name: 'seat', type: 'int', required: true }] }
  const step = commandRequest(assign, { type: 'command', op: assign.op, item: 'a/b', params: { user: 'x y', seat: 2 } })
  assert.deepEqual(step, { do: 'request', method: 'PUT', path: '/play/a%2Fb/seat/x%20y', body: { user: 'x y', seat: 2 } }, 'an int parameter is a number, a path value is escaped')
  assert.deepEqual(commandRequest(assign, { type: 'command', op: assign.op, item: 'a', params: { user: 'x' } }), { do: 'nothing' }, 'a required parameter that is missing')
  // Through plan, once the catalogue is known; the two client-handled ops need none.
  setCommandSpecs([join])
  assert.equal(plan({ type: 'command', op: join.op, item: 's', params: { code: 'z' } }).do, 'request')
  assert.deepEqual(plan({ type: 'command', op: 'rewind.open' }), { do: 'client', op: 'rewind.open', params: {} })
  assert.deepEqual(plan({ type: 'command', op: 'nothing.here' }), { do: 'nothing' })
  setCommandSpecs([])
})
