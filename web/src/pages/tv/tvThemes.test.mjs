// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { baseColours, chooseTheme, contrast, fromLegacy, isHex, legacyOf, PAIRS, previewAuto, previewLimit, PREVIEW_MIN, rowHeight, SHAPES, SLOTS, themeById, themeTokens, THEMES, upgradeSaved } from './tvThemes.ts'

const COLOURS = ['#ff0000', '#ffff00', '#00ff00', '#00ffff', '#0000ff', '#ff00ff', '#ffffff', '#000000', '#808080', '#7955ff', '#e8a33a', '#123456', '#fedcba', '#2fb67c', '#777777', '#f5f0e4']
const APPS = {
  dark: { bg: '#0f1115', bg2: '#16181d', bg3: '#23262d', bg4: '#30343c', fg: '#ffffff', fg2: '#c9ccd3', accent: '#7955ff', accentText: '#b39dff', accentFg: '#ffffff', border: '#4a4f5a' },
  light: { bg: '#f7f7f8', bg2: '#ffffff', bg3: '#ececef', bg4: '#dedee3', fg: '#14151a', fg2: '#4b4e57', accent: '#5a35e0', accentText: '#4a27c9', accentFg: '#ffffff', border: '#8a8d96' },
}

function checkPairs(t, say) {
  for (const [fg, bg, need] of PAIRS) {
    if (!isHex(t[fg]) || !isHex(t[bg])) continue
    const c = contrast(t[fg], t[bg])
    assert.ok(c >= need - 0.001, `${say}: ${fg} ${t[fg]} on ${bg} ${t[bg]} is ${c.toFixed(2)}`)
  }
}

test('every theme\'s own colours are readable', () => {
  for (const th of THEMES) for (const [name, app] of Object.entries(APPS)) checkPairs(themeTokens(th.id, undefined, app), `${th.id} on ${name}`)
})

test('any one colour someone picks, in any theme, stays readable', () => {
  for (const th of THEMES) for (const app of Object.values(APPS)) for (const { slot } of SLOTS) for (const colour of COLOURS) {
    const t = themeTokens(th.id, { [slot]: colour }, app)
    checkPairs(t, `${th.id} ${slot}=${colour}`)
    for (const [fg, bg] of PAIRS) assert.ok(isHex(t[fg]) && isHex(t[bg]), `${th.id} ${slot}: ${fg} on ${bg} isn't worked out`)
  }
})

test('any pair of colours stays readable, a black page with white cells included', () => {
  const slots = SLOTS.map((s) => s.slot)
  for (const a of slots) for (const b of slots) {
    if (a === b) continue
    for (const x of COLOURS) for (const y of COLOURS) checkPairs(themeTokens('custom', { [a]: x, [b]: y }, APPS.dark), `custom ${a}=${x} ${b}=${y}`)
  }
})

test('a theme that follows the app uses only the app\'s colours until overridden', () => {
  for (const th of THEMES.filter((t) => t.follows)) {
    const t = themeTokens(th.id, {}, APPS.dark)
    for (const k of ['--ctv-bg', '--ctv-cell', '--ctv-text', '--ctv-hi', '--ctv-accent']) assert.match(t[k], /^var\(--/, `${th.id} ${k}`)
    assert.equal(themeTokens(th.id, { bg: 'not a colour' }, APPS.dark)['--ctv-bg'], 'var(--bg)')
    assert.equal(themeTokens(th.id, { accent: '#ff0000' }, APPS.dark)['--ctv-bg'], APPS.dark.bg)
  }
  for (const th of THEMES.filter((t) => !t.follows)) assert.match(themeTokens(th.id, {}, APPS.dark)['--ctv-bg'], /^#/)
})

test('a theme stays as tuned while only one colour is changed', () => {
  const t = themeTokens('classic', { hi: '#ff00ff' })
  assert.equal(t['--ctv-cell-2'], '#143594')
  assert.equal(t['--ctv-panel'], '#0a2270')
  assert.equal(t['--ctv-hi'], '#ff00ff')
  assert.equal(themeTokens('classic')['--ctv-key-b'], '#4ce06a')
})

test('every earlier look has a home, and keeps its colours', () => {
  const pairs = [['classic', 'cable', 'classic'], ['classic', 'satellite', 'satellite'], ['classic', 'midnight', 'custom'], ['classic', 'theme', 'custom'], ['classic', 'custom', 'custom'],
    ['streaming', 'midnight', 'modern'], ['streaming', 'theme', 'modern'], ['streaming', 'custom', 'modern'], ['streaming', 'cable', 'modern'], ['streaming', 'satellite', 'modern']]
  for (const [layout, colours, theme] of pairs) {
    const got = fromLegacy(layout, colours, '#10203a', '#ffb300')
    assert.equal(got.theme, theme, `${layout} ${colours}`)
    if (theme === 'custom') assert.equal(got.custom.shape, 'box')
  }
  assert.deepEqual(fromLegacy('classic', 'custom', '#10203a', '#ffb300').themeColours.custom, { bg: '#10203a', accent: '#ffb300', hi: '#ffb300' })
  assert.equal(fromLegacy('streaming', 'midnight').themeColours.modern.bg, '#000000')
  assert.equal(fromLegacy('streaming', 'theme').themeColours, undefined)
})

test('an old save is read, and what it left out comes from the owner at the time', () => {
  const s = upgradeSaved({ layout: 'streaming', colours: 'midnight', previewPos: 'tl', previewSize: 'l', density: 'large', previewX: 3 })
  assert.equal(s.theme, 'modern')
  assert.equal(s.previewSide, 'left')
  assert.equal(s.previewW, 460)
  assert.equal(s.density, 'large')
  for (const k of ['layout', 'colours', 'previewPos', 'previewSize', 'previewX']) assert.ok(!(k in s), k)
  assert.equal(upgradeSaved({ colours: 'satellite' }, { layout: 'classic' }).theme, 'satellite')
  assert.equal(upgradeSaved({ colours: 'midnight' }, { layout: 'streaming' }).theme, 'modern')
  assert.equal(upgradeSaved({ layout: 'streaming' }, { skin: 'theme' }).theme, 'modern')
  const kept = upgradeSaved({ theme: 'teletext', layout: 'classic', colours: 'cable' })
  assert.equal(kept.theme, 'teletext')
  assert.deepEqual(upgradeSaved({ previewW: 512 }), { previewW: 512 }, 'a size someone chose stays')
  assert.equal(upgradeSaved({ theme: 'nonsense' }).theme, undefined)
})

test('a viewer\'s choice wins, then the app theme, then the owner', () => {
  const owner = { theme: 'teletext' }
  assert.deepEqual(chooseTheme({ theme: 'compact' }, { theme: 'listings' }, owner).theme, 'compact')
  assert.equal(chooseTheme({ theme: 'compact' }, { theme: 'listings' }, owner).source, 'you')
  const fromApp = chooseTheme({}, { theme: 'listings' }, owner)
  assert.equal(fromApp.theme, 'listings')
  assert.equal(fromApp.source, 'app')
  const fromOwner = chooseTheme({}, {}, owner)
  assert.equal(fromOwner.theme, 'teletext')
  assert.equal(fromOwner.source, 'owner')
  assert.equal(chooseTheme({}, {}, {}).theme, 'classic')
  assert.equal(chooseTheme({}, {}, { layout: 'streaming', skin: 'theme' }).theme, 'modern')
  assert.equal(chooseTheme({}, { colours: 'midnight' }, { layout: 'streaming' }).theme, 'modern')
  assert.equal(chooseTheme({}, { colours: 'midnight' }, { layout: 'streaming' }).source, 'app')
  assert.equal(chooseTheme({}, { theme: 'custom' }, {}).theme, 'classic', 'custom is only ever a viewer\'s own')
})

test('the preview starts well above the old 320, in every theme, and always fits', () => {
  for (const shape of Object.keys(SHAPES)) {
    for (const [w, h, narrow] of [[1142, 836, false], [1656, 1016, false], [1920, 1016, false], [1024, 600, false], [358, 700, true], [760, 500, false]]) {
      const auto = previewAuto(shape, w, h, narrow)
      const max = previewLimit(w, h, narrow)
      assert.ok(auto <= max, `${shape} at ${w}x${h}: ${auto} over ${max}`)
      assert.ok(auto >= PREVIEW_MIN)
      assert.ok(Math.round(auto * 9 / 16) + 44 <= h * 0.55 || narrow, `${shape} at ${w}x${h} leaves the guide no room`)
    }
  }
  assert.ok(previewAuto('modern', 1142, 836, false) >= 400, 'desktop')
  assert.ok(previewAuto('modern', 1920, 1016, false) >= 700, 'TV')
  assert.ok(previewAuto('cinema', 1142, 836, false) >= 500, 'big picture')
  assert.ok(previewAuto('modern', 358, 700, true) >= 300, 'phone')
})

test('rows keep room for a thumb on a phone', () => {
  for (const shape of Object.keys(SHAPES)) assert.ok(rowHeight(shape, 390, 'normal') >= 44, shape)
})

test('every theme is complete', () => {
  for (const th of THEMES) {
    assert.ok(th.name && th.blurb, th.id)
    assert.ok(SHAPES[th.shape], th.id)
    assert.equal(themeById(th.id), th)
    const base = baseColours(th.id)
    for (const { slot } of SLOTS) assert.ok(isHex(base[slot]), `${th.id} ${slot}`)
    const l = legacyOf(th.id, th.shape)
    assert.ok(l.layout === 'classic' || l.layout === 'streaming')
  }
})
