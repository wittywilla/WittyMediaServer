const test = require('node:test')
const assert = require('node:assert')
const { normalizeAddress } = require('./address.js')
const { cases } = require('../connection-cases.json')

for (const c of cases) {
  test(`address ${JSON.stringify(c.input)}`, () => {
    assert.deepStrictEqual(normalizeAddress(c.input), c.url ? { url: c.url } : { error: c.error })
  })
}
