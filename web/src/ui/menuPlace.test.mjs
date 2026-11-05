import test from 'node:test'
import assert from 'node:assert/strict'
import { movedAway, placeMenu } from './menuPlace.ts'

const VIEW = { width: 1280, height: 800 }
const EDGE = 8
const onScreen = (p, panel, view = VIEW) => {
  const h = Math.min(panel.height, p.maxHeight)
  return p.left >= EDGE && p.top >= EDGE && p.left + panel.width <= view.width - EDGE + 0.001 && p.top + h <= view.height - EDGE + 0.001
}

test('the account menu opens under its button, lined up with its end', () => {
  const button = { left: 1157, top: 14, right: 1241, bottom: 50 }
  const panel = { width: 220, height: 450 }
  const p = placeMenu(button, panel, VIEW, 'end')
  assert.equal(p.top, 58)
  assert.equal(p.left, 1241 - 220)
  assert.ok(onScreen(p, panel))
})

test('a menu is wholly on screen wherever its button is and whatever its size', () => {
  const panels = [{ width: 220, height: 450 }, { width: 220, height: 2000 }, { width: 1400, height: 300 }, { width: 50, height: 40 }]
  const views = [VIEW, { width: 375, height: 667 }, { width: 1024, height: 520 }]
  for (const view of views) {
    for (const panel of panels) {
      if (panel.width > view.width - 2 * EDGE) continue // wider than the screen: max-width in CSS stops it
      for (const x of [-500, 0, 10, view.width / 2, view.width - 10, view.width + 500]) {
        for (const y of [-900, 0, 20, view.height / 2, view.height - 30, view.height + 900]) {
          const button = { left: x - 20, right: x + 20, top: y - 18, bottom: y + 18 }
          for (const align of ['start', 'end']) {
            const p = placeMenu(button, panel, view, align)
            assert.ok(onScreen(p, panel, view), `${JSON.stringify({ view, panel, x, y, align, p })}`)
          }
        }
      }
    }
  }
})

test('with no button to measure, a menu still opens, at the top on its side', () => {
  const panel = { width: 220, height: 450 }
  const end = placeMenu(null, panel, VIEW, 'end')
  assert.ok(onScreen(end, panel))
  assert.equal(end.left, VIEW.width - EDGE - 220)
  const start = placeMenu(null, panel, VIEW, 'start')
  assert.equal(start.left, EDGE)
})

test('a menu near the foot of the screen opens upwards', () => {
  const button = { left: 300, right: 340, top: 760, bottom: 790 }
  const p = placeMenu(button, { width: 200, height: 300 }, VIEW, 'start')
  assert.ok(p.top + 300 <= 760)
})

test('only the page moving the button away closes a menu on scroll', () => {
  const at = { left: 1157, top: 14, right: 1241, bottom: 50 }
  assert.equal(movedAway(at, { ...at }), false) // a scroll event that moved nothing
  assert.equal(movedAway(at, { ...at, top: 15 }), false) // sub-pixel and rounding noise
  assert.equal(movedAway(at, { ...at, top: -120, bottom: -84 }), true) // the page scrolled
  assert.equal(movedAway(at, null), true) // the button went away
})
