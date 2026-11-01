import test from 'node:test'
import assert from 'node:assert/strict'
import { contrast, makeTheme, themeCss } from './themeMaker.ts'

const COLOURS = ['#ff0000', '#ffff00', '#00ff00', '#00ffff', '#0000ff', '#ff00ff', '#ffffff', '#000000', '#808080', '#7955ff', '#e8a33a', '#123456', '#fedcba', '#2fb67c']

test('anything someone can pick comes out readable', () => {
  for (const scheme of ['dark', 'light']) for (const accent of COLOURS) for (const tint of COLOURS) {
    const { vars: v } = makeTheme({ name: 'x', scheme, accent, tint, corners: 'soft' })
    const on = [v.bg, v['bg-2'], v['bg-3']]
    const say = `${scheme} accent ${accent} tint ${tint}`
    for (const k of ['fg', 'fg-2', 'fg-3', 'accent-text', 'danger', 'ok', 'warn']) {
      for (const s of on) assert.ok(contrast(v[k], s) >= 4.5, `${say}: ${k} ${v[k]} on ${s}: ${contrast(v[k], s).toFixed(2)}`)
    }
    for (const s of on) assert.ok(contrast(v['control-line'], s) >= 3, `${say}: control line`)
    assert.ok(contrast(v['accent-fg'], v.accent) >= 4.5, `${say}: label on accent`)
    assert.ok(contrast(v['accent-fg'], v['accent-2']) >= 4.5, `${say}: label on hovered accent`)
  }
})

test('dark and light themes really are', () => {
  const dark = makeTheme({ name: 'x', scheme: 'dark', accent: '#7955ff', tint: '#7955ff', corners: 'soft' }).vars
  const light = makeTheme({ name: 'x', scheme: 'light', accent: '#7955ff', tint: '#7955ff', corners: 'soft' }).vars
  assert.ok(contrast(dark.bg, '#000000') < 2)
  assert.ok(contrast(light.bg, '#ffffff') < 1.3)
})

test('the stylesheet holds only what was made', () => {
  const css = themeCss({ name: '}body{display:none', scheme: 'light', accent: 'not a colour', tint: '#123456', corners: 'round' })
  assert.ok(!css.includes('display:none'))
  assert.ok(!css.includes('not a colour'))
  assert.ok(css.includes('color-scheme:light'))
  assert.ok(css.includes('--radius:18px'))
})

test('an accent that had to move says so; one that fits does not', () => {
  assert.equal(makeTheme({ name: 'x', scheme: 'dark', accent: '#2f6fed', tint: '#2f6fed', corners: 'soft' }).adjusted, false)
  assert.equal(makeTheme({ name: 'x', scheme: 'dark', accent: '#777777', tint: '#777777', corners: 'soft' }).adjusted, true)
})
