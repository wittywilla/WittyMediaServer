// Checks that an address is answered by the server we think it is: it has to
// sign a fresh random value with its Ed25519 key (docs/CLIENT-API.md, "Server identity").
const crypto = require('crypto')

// The fixed DER header that turns a raw 32-byte Ed25519 public key into a key object.
const ED25519_SPKI_PREFIX = Buffer.from('302a300506032b6570032100', 'hex')

function signedBytes(serverId, nonce) {
  return Buffer.from(`WittyMediaServer identity v1\n${serverId}\n${nonce}`, 'utf8')
}

// Is this answer really signed by the key it names?
function signatureValid(answer, nonce) {
  if (!answer || answer.algorithm !== 'ed25519' || typeof answer.serverId !== 'string') return false
  try {
    const raw = Buffer.from(answer.publicKey, 'base64')
    if (raw.length !== 32) return false
    const key = crypto.createPublicKey({ key: Buffer.concat([ED25519_SPKI_PREFIX, raw]), format: 'der', type: 'spki' })
    return crypto.verify(null, signedBytes(answer.serverId, nonce), key, Buffer.from(answer.signature, 'base64'))
  } catch {
    return false
  }
}

// What an identity answer means, given the saved servers:
//   bad        the signature doesn't hold: nothing about it can be believed
//   new        never seen before
//   same       the key we remembered, for this address or this server's ID
//   different  we remembered another key: possibly a look-alike
function judge(answer, nonce, url, saved) {
  if (!signatureValid(answer, nonce)) return 'bad'
  const known = (saved || []).find((s) => s.publicKey && (s.url === url || s.serverId === answer.serverId))
  if (!known) return 'new'
  return known.publicKey === answer.publicKey ? 'same' : 'different'
}

function newNonce() {
  return crypto.randomBytes(32).toString('base64')
}

module.exports = { signatureValid, judge, newNonce, signedBytes }
