// Run with: node --experimental-strip-types --test web/src/ui/backEntry.test.mjs
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { openBackEntry } from './backEntry.ts'

// A browser's history, as far as a sheet sees it: Back is answered later (popstate), as browsers do.
function browser() {
  const listeners = new Set()
  const h = {
    entries: [{ idx: 4, key: 'game' }],
    at: 0,
    waiting: 0,
    get state() { return this.entries[this.at] },
    pushState(s) { this.entries = this.entries.slice(0, this.at + 1); this.entries.push(s); this.at++ },
    back() { this.waiting++ },
    pop() { if (this.at > 0) { this.at--; for (const f of [...listeners]) f() } },
    // The browser gets round to the Back it was asked for.
    settle() { while (this.waiting > 0) { this.waiting--; this.pop() } },
  }
  const target = { addEventListener: (_t, f) => listeners.add(f), removeEventListener: (_t, f) => listeners.delete(f) }
  return { h, target, listeners }
}

test('a phone’s Back closes the sheet, and the page stays', () => {
  const { h, target } = browser()
  let closed = 0
  openBackEntry(h, target, 'wmsSheet', () => closed++)
  assert.equal(h.entries.length, 2, 'one entry while it’s open')
  h.pop()
  assert.equal(closed, 1)
  assert.equal(h.state.key, 'game', 'back on the page’s own entry')
})

test('the page’s own state rides along, so the router sees the same place', () => {
  const { h, target } = browser()
  openBackEntry(h, target, 'wmsSheet', () => {})
  assert.equal(h.state.key, 'game')
  assert.equal(h.state.idx, 4)
  assert.equal(h.state.wmsSheet, true)
})

test('closing with ×, Esc or a tap outside takes the entry back out', () => {
  const { h, target, listeners } = browser()
  let closed = 0
  const done = openBackEntry(h, target, 'wmsSheet', () => closed++)
  done()
  h.settle()
  assert.equal(h.at, 0, 'Back from here leaves the page, as before the sheet opened')
  assert.equal(closed, 0, 'its own Back isn’t taken for someone pressing Back')
  assert.equal(listeners.size, 0)
})

test('closing after Back took the entry away goes back no further', () => {
  const { h, target } = browser()
  const done = openBackEntry(h, target, 'wmsSheet', () => {})
  h.pop()
  done()
  h.settle()
  assert.equal(h.at, 0)
  assert.equal(h.waiting, 0)
})

test('a sheet over a sheet closes first', () => {
  const { h, target } = browser()
  const shut = []
  openBackEntry(h, target, 'wmsSettings', () => shut.push('settings'))
  openBackEntry(h, target, 'wmsCore', () => shut.push('core'))
  h.pop()
  assert.deepEqual(shut, ['core'])
  h.pop()
  assert.deepEqual(shut, ['core', 'settings'])
})

test('moving to another page leaves its entry alone', () => {
  const { h, target } = browser()
  const done = openBackEntry(h, target, 'wmsSheet', () => {})
  h.pushState({ idx: 5, key: 'next' }, '') // the router going somewhere from inside it
  done()
  assert.equal(h.waiting, 0, 'nothing taken back')
  assert.equal(h.state.key, 'next')
})
