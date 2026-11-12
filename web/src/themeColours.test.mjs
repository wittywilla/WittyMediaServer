// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import { CONTRAST_PAIRS, TOKEN_GROUPS, checkContrast, contrastRatio, draftTokens, normalizeColour, normalizedColours, overlay, pickerValue, sameColours, tokenErrors, tokenGroups, tokenLabel, withPicked } from './themeColours.ts'
import { tokenVar } from './themeOverrides.ts'

const themesDir = new URL('../../themes/', import.meta.url)
const builtin = (id) => JSON.parse(readFileSync(new URL(id + '/theme.json', themesDir), 'utf8')).colors

test('every colour the built-in themes have is on the page, once', () => {
  // The server takes exactly the names the built-in themes give in theme.json "colors".
  const server = new Set()
  for (const d of readdirSync(themesDir, { withFileTypes: true })) if (d.isDirectory()) for (const k of Object.keys(builtin(d.name))) server.add(k)
  const here = TOKEN_GROUPS.flatMap((g) => g.tokens.map((t) => t.name))
  assert.deepEqual([...here].sort(), [...server].sort())
  assert.equal(new Set(here).size, here.length)
  assert.equal(tokenGroups(server).some((g) => g.id === 'other'), false)
})

test('a name the server takes that this page has no words for is listed, and one it doesn’t take is not', () => {
  const groups = tokenGroups(['bg', 'accent', 'sparkle'])
  assert.deepEqual(groups.map((g) => [g.id, g.tokens.map((t) => t.name)]), [['surfaces', ['bg']], ['accent', ['accent']], ['other', ['sparkle']]])
  assert.equal(tokenLabel('sparkle'), 'sparkle')
  assert.equal(tokenLabel('accentText'), 'Links')
})

test('every colour name is a CSS variable the stylesheet has', () => {
  const css = readFileSync(new URL('./styles.css', import.meta.url), 'utf8')
  for (const g of TOKEN_GROUPS) for (const t of g.tokens) assert.ok(css.includes(tokenVar(t.name) + ':'), `${t.name} -> ${tokenVar(t.name)}`)
})

test('colours are read the way the server reads them', () => {
  // Each answer is what PUT /api/theme/modules/{module} stored for that value.
  const same = [
    ['#555', '#555555'], ['#abcd', '#AABBCCDD'], ['#e0a030', '#E0A030'], [' #E0A030FF ', '#E0A030FF'],
    ['rgb(0 0 0)', '#000000'], ['rgba(58, 210, 159, 50%)', '#3AD29F80'], ['rgb(255,255,255)', '#FFFFFF'], ['rgba(0,0,0,.6)', '#00000099'],
    ['rgb(20 24 32 / 0.45)', '#14182073'], ['rgb(1.5, 2.4, 3)', '#020203'], ['rgba(0,0,0,7)', '#000000FF'],
    ['transparent', '#00000000'], ['Black', '#000000'], ['WHITE', '#FFFFFF'],
  ]
  for (const [given, want] of same) assert.equal(normalizeColour(given), want, given)
  for (const bad of ['', 'red', '#12', '#12345', '#1234567', '#gggggg', 'rgb(256,0,0)', 'rgb(1,2)', 'rgb(1.2.3, 0, 0)', 'hsl(0 0% 0%)', 'var(--bg)', 'url(https://example.org/x)', '#fff;}', 'rgb(0,0,0);x']) {
    assert.equal(normalizeColour(bad), null, bad)
  }
})

test('the colour picker gets six digits, and a see-through colour keeps how see-through it was', () => {
  assert.equal(pickerValue('#3AD29F80'), '#3ad29f')
  assert.equal(pickerValue('#abc'), '#aabbcc')
  assert.equal(pickerValue('nonsense', '#123456'), '#123456')
  assert.equal(withPicked('#ff8800', '#E8EAF01A'), '#FF88001A')
  assert.equal(withPicked('#ff8800', '#7955FF'), '#FF8800')
  assert.equal(withPicked('#ff8800', '#7955FFFF'), '#FF8800')
  assert.equal(withPicked('#ff8800', ''), '#FF8800')
})

test('contrast is WCAG’s, as scripts/theme-contrast.py works it out', () => {
  // The second column is ratio() from scripts/theme-contrast.py.
  const py = [
    ['#e8eaf0', '#0f1115', 15.709738678432977], ['#848b99', '#1f232c', 4.594707552054966], ['#ffffff', '#7955ff', 4.6089124950293385],
    ['#9174ff', '#1f232c', 4.603672908998264], ['#000000', '#ffffff', 21], ['#777777', '#ffffff', 4.478089453577214], ['#0a0a0a', '#0b0b0b', 1.0058690363860938],
  ]
  for (const [a, b, want] of py) {
    assert.ok(Math.abs(contrastRatio(a, b) - want) < 1e-9, `${a} on ${b}`)
    assert.equal(contrastRatio(b, a), contrastRatio(a, b))
  }
  assert.equal(contrastRatio('#3AD29F80', '#000000'), null)
  assert.equal(contrastRatio('#3AD29FFF', '#000000'), contrastRatio('#3AD29F', '#000'))
  assert.equal(contrastRatio('nonsense', '#000000'), null)
  assert.equal(contrastRatio(undefined, '#000000'), null)
})

test('the pairs are the ones the server checks', () => {
  assert.equal(CONTRAST_PAIRS.length, 22)
  assert.deepEqual(CONTRAST_PAIRS[0], ['fg', 'bg'])
  assert.deepEqual(CONTRAST_PAIRS[11], ['accentText', 'bg3'])
  assert.deepEqual(CONTRAST_PAIRS.at(-1), ['accentFg', 'accent'])
})

// What the server answered (GET /api/theme, `warnings`) for these overrides on a module, with
// the automatic theme: its dark look is the Witty theme and its light look the Light theme.
const OVERRIDES = { accentFg: '#000000', accentText: '#7955FF', bg3: '#808080', fg: '#555555', ok: '#3AD29F80', warn: '#E0A030FF' }
const SERVER_DARK = [['fg', 'bg', 2.53], ['fg', 'bg2', 2.34], ['fg', 'bg3', 1.89], ['fg2', 'bg3', 1.8], ['fg3', 'bg3', 1.15], ['accentText', 'bg', 4.1], ['accentText', 'bg2', 3.78], ['accentText', 'bg3', 1.17], ['danger', 'bg3', 1.32], ['warn', 'bg3', 1.74]]
const SERVER_LIGHT = [['fg', 'bg3', 1.89], ['fg2', 'bg3', 2.02], ['fg3', 'bg3', 1.36], ['accentText', 'bg', 4.23], ['accentText', 'bg3', 1.17], ['danger', 'bg3', 1.34], ['warn', 'bg', 2.08], ['warn', 'bg2', 2.27], ['warn', 'bg3', 1.74], ['accentFg', 'accent', 3.84]]

test('the warnings worked out here are the server’s, pair for pair and to the same two places', () => {
  for (const [id, want] of [['default', SERVER_DARK], ['light', SERVER_LIGHT]]) {
    const got = checkContrast(overlay(normalizedColours(builtin(id)), OVERRIDES), OVERRIDES)
    assert.deepEqual(got.map((w) => [w.text, w.surface, w.ratio]), want, id)
    for (const w of got) assert.equal(w.need, 4.5)
  }
})

test('the built-in themes pass, and only pairs an override touches are reported', () => {
  for (const d of readdirSync(themesDir, { withFileTypes: true })) if (d.isDirectory()) assert.deepEqual(checkContrast(normalizedColours(builtin(d.name))), [], d.name)
  const colours = overlay(normalizedColours(builtin('default')), { fg: '#555555', accentFg: '#7955FF' })
  assert.deepEqual(checkContrast(colours, { accentFg: '#7955FF' }).map((w) => w.text), ['accentFg'])
  assert.deepEqual(checkContrast(colours, {}), [])
  assert.equal(checkContrast(colours).length, 4)
  // A see-through text colour isn't judged: what shows through it isn't known.
  assert.deepEqual(checkContrast(overlay(colours, { fg: '#55555580', accentFg: '#FFFFFF' })), [])
})

test('a draft sends only real colours, and names the rest', () => {
  assert.deepEqual(draftTokens({ accent: '#f80', fg: '  ', bg: 'rgb(0,0,0)', ok: 'green', warn: '#12' }), { tokens: { accent: '#FF8800', bg: '#000000' }, bad: ['ok', 'warn'] })
  assert.equal(sameColours({ accent: '#ff8800' }, { accent: '#FF8800' }), true)
  assert.equal(sameColours({ accent: '#ff8800' }, { accent: '#FF8800', fg: '#FFFFFF' }), false)
  assert.equal(sameColours({ accent: '#ff8800' }, { fg: '#FF8800' }), false)
  assert.equal(sameColours({}, {}), true)
})

test('the server’s refusal is read into its words for each colour', () => {
  // Word for word what PUT /api/theme/modules/{module} answered for {"accent":"red","nope":"#fff","fg":"#12"}.
  const said = 'accent: not a colour (#rrggbb, #rrggbbaa or rgb()/rgba()); fg: not a colour (#rrggbb, #rrggbbaa or rgb()/rgba()); nope: not a colour token themes have. Known tokens: accent, accentFg, accentHover, accentLine, accentSoft, accentText, bg, bg2, bg3, bg4, border, borderStrong, brandAccent, controlLine, danger, dangerSoft, fg, fg2, fg3, focus, hover, ok, okSoft, onImage, overlay, pressed, scrim, warn, warnSoft'
  assert.deepEqual(tokenErrors(said, ['accent', 'fg', 'nope']), {
    fields: { accent: 'not a colour (#rrggbb, #rrggbbaa or rgb()/rgba())', fg: 'not a colour (#rrggbb, #rrggbbaa or rgb()/rgba())', nope: 'not a colour token themes have' }, other: '',
  })
  // A colour that isn't on the page keeps its sentence, so nothing the server said is lost.
  assert.deepEqual(tokenErrors(said, ['accent', 'fg']).other, 'nope: not a colour token themes have')
  assert.deepEqual(tokenErrors('too many tokens: at most 64', ['accent']), { fields: {}, other: 'too many tokens: at most 64' })
  assert.deepEqual(tokenErrors('ui_config.json was changed by someone else: read it again and redo the change', ['accent']), { fields: {}, other: 'ui_config.json was changed by someone else: read it again and redo the change' })
})
