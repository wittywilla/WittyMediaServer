const test = require('node:test')
const assert = require('node:assert')
const fs = require('fs')
const os = require('os')
const path = require('path')
const { load, save, remember, MAX_RECENT } = require('../state.js')

test('remember puts the newest server first, once', () => {
  let s = remember({}, { url: 'http://a', name: 'A' })
  s = remember(s, { url: 'http://b', name: 'B' })
  s = remember(s, { url: 'http://a', name: 'A2' })
  assert.strictEqual(s.server, 'http://a')
  assert.deepStrictEqual(s.recent, [{ url: 'http://a', name: 'A2' }, { url: 'http://b', name: 'B' }])
})

test('remember keeps a bounded list', () => {
  let s = {}
  for (let i = 0; i < MAX_RECENT + 5; i++) s = remember(s, { url: `http://${i}`, name: String(i) })
  assert.strictEqual(s.recent.length, MAX_RECENT)
})

test('a missing or damaged state file is an empty state', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'wmc-'))
  const file = path.join(dir, 'state.json')
  assert.deepStrictEqual(load(file), {})
  fs.writeFileSync(file, '{not json')
  assert.deepStrictEqual(load(file), {})
  save(file, { server: 'http://a' })
  assert.deepStrictEqual(load(file), { server: 'http://a' })
  fs.rmSync(dir, { recursive: true })
})
