const test = require('node:test')
const assert = require('node:assert')
const crypto = require('crypto')
const dnsPacket = require('dns-packet')
const { serversFromRecords, serverFromUdpReply, makeServer, ServerList, FORGET_AFTER_MS, MAX_SERVERS } = require('../discover.js')
const { signatureValid, judge, judgeMissing, fingerprintOf, signedBytes } = require('../identity.js')

const ID = '0123456789abcdef0123456789abcdef'
const INSTANCE = `Home (0123)._wittymedia._tcp.local`

// A real packet, encoded and decoded by the DNS library, shaped as the API guide describes.
function answer({ host = '192.168.1.20', port = 9797, tls = '0', id = ID, name = 'Home' } = {}) {
  const raw = dnsPacket.encode({
    type: 'response',
    answers: [
      { type: 'PTR', name: '_wittymedia._tcp.local', data: INSTANCE },
      { type: 'SRV', name: INSTANCE, data: { port, target: 'wms-01234567.local' } },
      { type: 'TXT', name: INSTANCE, data: [`id=${id}`, `name=${name}`, 'v=0.4.0', 'api=1', `tls=${tls}`] },
      { type: 'A', name: 'wms-01234567.local', data: host },
    ],
  })
  return dnsPacket.decode(raw).answers
}

test('reads a server from an mDNS answer', () => {
  assert.deepStrictEqual(serversFromRecords(answer()), [{ id: ID, name: 'Home', url: 'http://192.168.1.20:9797' }])
})

test('tls=1 means https', () => {
  assert.strictEqual(serversFromRecords(answer({ tls: '1' }))[0].url, 'https://192.168.1.20:9797')
})

test('refuses announcements that point outside the local network', () => {
  assert.deepStrictEqual(serversFromRecords(answer({ host: '8.8.8.8' })), [])
})

test('refuses a bad id or port, and other services', () => {
  assert.deepStrictEqual(serversFromRecords(answer({ id: 'not-an-id' })), [])
  assert.strictEqual(makeServer({ id: ID, name: 'x', host: '192.168.1.2', port: 70000, tls: false }), null)
  const other = [
    { type: 'SRV', name: 'x._http._tcp.local', data: { port: 80, target: 'h.local' } },
    { type: 'TXT', name: 'x._http._tcp.local', data: [`id=${ID}`] },
    { type: 'A', name: 'h.local', data: '192.168.1.2' },
  ]
  assert.deepStrictEqual(serversFromRecords(other), [])
})

test('cleans control characters from names and limits their length', () => {
  assert.strictEqual(makeServer({ id: ID, name: 'A\u0007B\n', host: '10.0.0.2', port: 9797 }).name, 'AB')
  assert.strictEqual(makeServer({ id: ID, name: 'x'.repeat(500), host: '10.0.0.2', port: 9797 }).name.length, 80)
  assert.strictEqual(makeServer({ id: ID, name: '', host: '10.0.0.2', port: 9797 }).name, 'WittyMediaServer')
})

test('reads a UDP reply, using the address it really came from', () => {
  const reply = Buffer.from(JSON.stringify({ service: 'wittymediaserver', id: ID, name: 'Home', version: '1', api: 1, port: 9797, tls: false }))
  assert.strictEqual(serverFromUdpReply(reply, '10.0.0.9').url, 'http://10.0.0.9:9797')
  assert.strictEqual(serverFromUdpReply(reply, '8.8.8.8'), null)
  assert.strictEqual(serverFromUdpReply(Buffer.from('{"service":"other"}'), '10.0.0.9'), null)
  assert.strictEqual(serverFromUdpReply(Buffer.from('not json'), '10.0.0.9'), null)
})

test('the list forgets silent servers and can not grow without limit', () => {
  let now = 1000
  const list = new ServerList(() => now)
  const s = { id: ID, name: 'Home', url: 'http://10.0.0.2:9797' }
  assert.strictEqual(list.add(s), true)
  assert.strictEqual(list.add(s), false, 'the same answer again is no change')
  assert.strictEqual(list.add({ ...s, url: 'http://10.0.0.3:9797' }), true)
  now += FORGET_AFTER_MS + 1
  assert.strictEqual(list.expire(), true)
  assert.deepStrictEqual(list.list(), [])
  for (let i = 0; i < MAX_SERVERS + 10; i++) list.add({ id: String(i), name: String(i), url: 'http://10.0.0.2' })
  assert.strictEqual(list.list().length, MAX_SERVERS)
})

function signedAnswer(nonce, serverId = ID, keys = crypto.generateKeyPairSync('ed25519')) {
  const raw = keys.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32)
  return {
    keys,
    answer: {
      serverId,
      algorithm: 'ed25519',
      publicKey: raw.toString('base64'),
      fingerprint: 'AB12-CD34-EF56',
      signature: crypto.sign(null, signedBytes(serverId, nonce), keys.privateKey).toString('base64'),
    },
  }
}

test('accepts a true signature and refuses tampering', () => {
  const { answer: a } = signedAnswer('nonce')
  assert.strictEqual(signatureValid(a, 'nonce'), true)
  assert.strictEqual(signatureValid(a, 'another nonce'), false, 'a replayed answer')
  assert.strictEqual(signatureValid({ ...a, serverId: 'someone else' }, 'nonce'), false)
  assert.strictEqual(signatureValid({ ...a, algorithm: 'none' }, 'nonce'), false)
  assert.strictEqual(signatureValid({ ...a, publicKey: 'AAAA' }, 'nonce'), false)
  assert.strictEqual(signatureValid(null, 'nonce'), false)
})

test('checks a real answer from a server', () => {
  const fixture = require('../../shared/fixtures/identity.json')
  assert.strictEqual(signatureValid(fixture.answer, fixture.nonce), true)
})

test('works out the same fingerprint the server shows', () => {
  const { answer } = require('../../shared/fixtures/identity.json')
  assert.strictEqual(fingerprintOf(answer.publicKey), answer.fingerprint)
})

test('a known server that stops proving itself is refused, an unknown one may be old', () => {
  const url = 'http://10.0.0.2:9797'
  assert.strictEqual(judgeMissing(url, []), 'none')
  assert.strictEqual(judgeMissing(url, [{ url, publicKey: 'k' }]), 'bad')
  assert.strictEqual(judgeMissing(url, [{ url: 'http://10.0.0.3:9797', publicKey: 'k' }]), 'none')
})

test('judges new, same and different keys', () => {
  const url = 'http://10.0.0.2:9797'
  const { answer: a } = signedAnswer('n')
  assert.strictEqual(judge(a, 'n', url, []), 'new')
  assert.strictEqual(judge(a, 'n', url, [{ url, serverId: ID, publicKey: a.publicKey }]), 'same')
  assert.strictEqual(judge(a, 'wrong', url, [{ url, serverId: ID, publicKey: a.publicKey }]), 'bad')
  const { answer: other } = signedAnswer('n')
  assert.strictEqual(judge(other, 'n', url, [{ url, serverId: ID, publicKey: a.publicKey }]), 'different')
  // The same server ID at a new address must still hold the same key.
  assert.strictEqual(judge(other, 'n', 'http://10.0.0.3:9797', [{ url, serverId: ID, publicKey: a.publicKey }]), 'different')
})
