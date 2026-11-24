#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""What an app does with a Connect a device code at a web address (Enter a code, or a QR code's h= and c=).

    webjoin.py <web address> <code> [device name] [platform]

The same steps as WittyMediaClient's desktop/code.js and Android's WebJoin.kt, and the server's
internal/server/api_connect.go: the code becomes two keys with scrypt (p2p.DeriveWebJoin); the app sends the first,
the server has to prove it knows the code, bound to its identity key and both random numbers, before the app proves
it does. Prints one JSON line: {"ok": true, "token": "…"} with the token hidden, or {"ok": false, "step": ..., ...}
naming the step that failed. Exits 0 only when signed in.
"""
import base64
import hashlib
import hmac
import json
import os
import sys
import urllib.error
import urllib.request

ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ'


def clean(code):
    out = ''
    for ch in code.upper():
        if ch in ' -\t':
            continue
        ch = {'O': '0', 'I': '1', 'L': '1'}.get(ch, ch)
        if ch not in ALPHABET:
            return ''
        out += ch
    return out if len(out) == 8 else ''


def proof(key, role, device_nonce, server_nonce, identity_key):
    m = hmac.new(key, digestmod=hashlib.sha256)
    for part in (b'wittymedia web join 1', role.encode(), device_nonce, server_nonce, identity_key.encode()):
        m.update(len(part).to_bytes(2, 'big'))
        m.update(part)
    return m.digest()


def post(base, path, body):
    req = urllib.request.Request(base.rstrip('/') + '/api' + path, data=json.dumps(body).encode(),
                                 headers={'Content-Type': 'application/json'}, method='POST')
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, json.loads(r.read() or b'{}')
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read() or b'{}')
        except ValueError:
            return e.code, {}


def main():
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2
    base, typed = sys.argv[1], sys.argv[2]
    name = sys.argv[3] if len(sys.argv) > 3 else 'Smoke phone'
    platform = sys.argv[4] if len(sys.argv) > 4 else 'android'

    def fail(step, **more):
        print(json.dumps({'ok': False, 'step': step, **more}))
        return 1

    code = clean(typed)
    if not code:
        return fail('the code', error='not a code: eight letters and digits')
    try:
        status, ident = post(base, '/identity', {'nonce': base64.b64encode(os.urandom(32)).decode()})
    except OSError as e:
        return fail('reaching the server', error=str(e))
    key = ident.get('publicKey', '') if status == 200 else ''
    if not key:
        return fail('the server saying who it is', status=status)
    out = hashlib.scrypt(code.encode(), salt=b'wittymedia-web-join-v1', n=1 << 15, r=8, p=1, maxmem=64 << 20, dklen=64)
    find, pkey = out[:32].hex(), out[32:]
    dn = os.urandom(16)
    status, hello = post(base, '/connect/hello', {'v': 1, 'find': find, 'nonce': base64.b64encode(dn).decode()})
    if status != 200:
        return fail('the server knowing the code', status=status, error=hello.get('error', ''))
    sn = base64.b64decode(hello.get('nonce', ''))
    said = base64.b64decode(hello.get('proof', ''))
    if hello.get('publicKey') != key or len(sn) != 16 or not hmac.compare_digest(said, proof(pkey, 'server', dn, sn, key)):
        return fail('the server proving it made the code')
    status, ans = post(base, '/connect/answer', {'v': 1, 'find': find, 'nonce': base64.b64encode(dn).decode(),
                                                 'serverNonce': base64.b64encode(sn).decode(),
                                                 'proof': base64.b64encode(proof(pkey, 'device', dn, sn, key)).decode(),
                                                 'device': name, 'platform': platform})
    if status != 200 or not isinstance(ans.get('token'), str):
        return fail('signing in', status=status, error=ans.get('error', ''))
    print(json.dumps({'ok': True, 'token': '…', 'serverName': ans.get('serverName', '')}))
    return 0


if __name__ == '__main__':
    sys.exit(main())
