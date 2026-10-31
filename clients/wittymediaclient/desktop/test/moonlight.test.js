// Nothing real is started here: every program is a stand-in.
const test = require('node:test')
const assert = require('node:assert')
const { EventEmitter } = require('node:events')
const path = require('node:path')
const { permissionAllowed, validArgs, validHost, validApp, findMoonlight, status, launcher, MIN_GAP_MS, NOT_INSTALLED, REFUSED, FAILED, TOO_SOON } = require('../moonlight.js')

test('only the server page can lock the mouse, and nothing else new is allowed', () => {
  const server = 'http://192.168.1.5:9797'
  assert.ok(permissionAllowed('pointerLock', 'http://192.168.1.5:9797/couch-rig/7', server))
  assert.ok(permissionAllowed('pointerLock', 'http://192.168.1.5:9797/', server))
  assert.ok(!permissionAllowed('pointerLock', 'http://192.168.1.5:9798/', server))
  assert.ok(!permissionAllowed('pointerLock', 'https://example.com/', server))
  assert.ok(!permissionAllowed('pointerLock', 'file:///connect/index.html', ''))
  assert.ok(!permissionAllowed('pointerLock', 'not a url', server))
  assert.ok(permissionAllowed('fullscreen', 'https://example.com/', server))
  for (const perm of ['media', 'geolocation', 'notifications', 'keyboardLock', 'display-capture', 'openExternal']) {
    assert.ok(!permissionAllowed(perm, 'http://192.168.1.5:9797/', server), perm)
  }
})

test('addresses: names, IPv4 and [IPv6], with a port if needed', () => {
  for (const good of ['gaming-pc', 'pc.local', 'pc.example.com', '192.168.1.20', '192.168.1.20:48010', 'pc:47989', '[::1]', '[fd00::20]:48010', '[2001:db8::1]']) {
    assert.ok(validHost(good), good)
  }
})

test('addresses: anything else is refused', () => {
  const bad = ['', '-pc', '--pin', 'pc-', '.pc', 'pc..local', 'pc name', 'pc/../x', 'pc:', 'pc:0', 'pc:65536', 'pc:12a', 'pc:80:80', '::1', '[::1', '[nope]', '[::1]:', '[::1]:99999',
    '[fe80::1%eth0]', 'pc\n', 'pc;calc', 'pc&calc', 'http://pc', 'a'.repeat(254), 7, null, undefined, ['pc']]
  for (const host of bad) assert.ok(!validHost(host), String(host))
})

test('game names: free text, as long as nothing reads as an option', () => {
  for (const good of ['Space Game', "Baldur's Gate 3", 'A & B; C | "D"', '100% $HOME `x`', '-'.padStart(5, 'x'), 'é'.repeat(200)]) assert.ok(validApp(good), good)
  const bad = ['', '   ', '-x', '--quit-after', ' --quit-after', 'a\nb', 'a\rb', 'a\tb', 'a\0b', 'a\x7fb', 'a\u0085b', 'x'.repeat(201), 3, null, {}, ['Space Game']]
  for (const app of bad) assert.ok(!validApp(app), String(app))
})

test('only exactly the two shapes get through', () => {
  assert.ok(validArgs(['stream', '192.168.1.20', 'Space Game']))
  assert.ok(validArgs(['pair', '192.168.1.20', '--pin', '1234']))
  const bad = [
    undefined, null, 'stream', {}, [], ['stream'], ['stream', 'pc'],
    ['stream', 'pc', 'Game', '--extra'], ['stream', 'pc', '--quit-after'], ['stream', '--opt', 'Game'],
    ['list', 'pc'], ['quit', 'pc'], ['STREAM', 'pc', 'Game'], ['--help'], [['stream'], 'pc', 'Game'],
    { 0: 'stream', 1: 'pc', 2: 'Game', length: 3 },
    ['pair', 'pc'], ['pair', 'pc', '--pin'], ['pair', 'pc', '1234', '--pin'], ['pair', 'pc', '--pin', '123'], ['pair', 'pc', '--pin', '12345'],
    ['pair', 'pc', '--pin', 'abcd'], ['pair', 'pc', '--pin', '١٢٣٤'], ['pair', 'pc', '--pin', '1234', 'x'], ['pair', 'pc', '--pin', 1234],
  ]
  for (const args of bad) assert.ok(!validArgs(args), JSON.stringify(args))
})

const only = (...files) => (file) => files.includes(file)

test('Windows: the install folder, then the 32-bit one', () => {
  const main = 'C:\\Program Files\\Moonlight Game Streaming\\Moonlight.exe'
  const x86 = 'D:\\Old\\Moonlight Game Streaming\\Moonlight.exe'
  const env = { 'ProgramFiles(x86)': 'D:\\Old', PATH: 'C:\\planted' }
  assert.deepStrictEqual(findMoonlight({ platform: 'win32', env, runnable: only(main, x86) }), { command: main, prefix: [] })
  assert.deepStrictEqual(findMoonlight({ platform: 'win32', env, runnable: only(x86) }), { command: x86, prefix: [] })
  assert.strictEqual(findMoonlight({ platform: 'win32', env, runnable: only('C:\\planted\\moonlight.exe') }), null)
})

test('Mac: the one place it installs', () => {
  const program = '/Applications/Moonlight.app/Contents/MacOS/Moonlight'
  assert.deepStrictEqual(findMoonlight({ platform: 'darwin', env: {}, runnable: only(program) }), { command: program, prefix: [] })
  assert.strictEqual(findMoonlight({ platform: 'darwin', env: { PATH: '/usr/bin' }, runnable: only('/usr/bin/moonlight') }), null)
})

test('Linux: PATH first, skipping anything relative', () => {
  const env = { PATH: ':.:bin:/usr/local/bin:/usr/bin' }
  const seen = []
  const found = findMoonlight({ platform: 'linux', env, home: '/home/a', folder: () => false, runnable: (f) => { seen.push(f); return f === '/usr/bin/moonlight' } })
  assert.deepStrictEqual(found, { command: '/usr/bin/moonlight', prefix: [] })
  assert.ok(seen.every((f) => f.startsWith('/')), seen.join())
})

test('Linux: Flatpak only when both it and the app are there', () => {
  const env = { PATH: '/usr/bin' }
  const app = (dir) => (d) => d === dir
  const flatpak = only('/usr/bin/flatpak')
  const expected = { command: '/usr/bin/flatpak', prefix: ['run', 'com.moonlight_stream.Moonlight'] }
  assert.deepStrictEqual(findMoonlight({ platform: 'linux', env, home: '/home/a', runnable: flatpak, folder: app('/var/lib/flatpak/app/com.moonlight_stream.Moonlight') }), expected)
  assert.deepStrictEqual(findMoonlight({ platform: 'linux', env, home: '/home/a', runnable: flatpak, folder: app('/home/a/.local/share/flatpak/app/com.moonlight_stream.Moonlight') }), expected)
  assert.strictEqual(findMoonlight({ platform: 'linux', env, home: '/home/a', runnable: flatpak, folder: () => false }), null)
  assert.strictEqual(findMoonlight({ platform: 'linux', env, home: '/home/a', runnable: () => false, folder: () => true }), null)
})

test('status says only whether it is there', () => {
  assert.deepStrictEqual(status(() => ({ command: '/secret/path', prefix: [] })), { installed: true })
  assert.deepStrictEqual(status(() => null), { installed: false })
})

// A stand-in for spawn, which never runs anything.
function fakeRun(event = 'spawn') {
  const calls = []
  const run = (command, args, options) => {
    const child = new EventEmitter()
    child.unref = () => { child.unrefCalled = true }
    calls.push({ command, args, options, child })
    setImmediate(() => child.emit(event, new Error('stand-in')))
    return child
  }
  return { calls, run }
}

const here = () => ({ command: path.join('stand', 'in'), prefix: [] })

test('a launch has no shell, carries on alone, and ignores its output', async () => {
  const { calls, run } = fakeRun()
  const result = await launcher({ find: here, run })(['stream', '192.168.1.20', 'Space Game; rm -rf /'])
  assert.deepStrictEqual(result, { ok: true })
  assert.strictEqual(calls.length, 1)
  assert.strictEqual(calls[0].command, here().command)
  assert.deepStrictEqual(calls[0].args, ['stream', '192.168.1.20', 'Space Game; rm -rf /'])
  assert.deepStrictEqual(calls[0].options, { detached: true, stdio: 'ignore', shell: false })
  assert.ok(calls[0].child.unrefCalled)
})

test('Flatpak goes in front of what was asked', async () => {
  const { calls, run } = fakeRun()
  await launcher({ find: () => ({ command: '/usr/bin/flatpak', prefix: ['run', 'com.moonlight_stream.Moonlight'] }), run })(['pair', 'pc', '--pin', '1234'])
  assert.deepStrictEqual(calls[0].args, ['run', 'com.moonlight_stream.Moonlight', 'pair', 'pc', '--pin', '1234'])
})

test('anything else starts nothing', async () => {
  const { calls, run } = fakeRun()
  const launch = launcher({ find: here, run })
  assert.deepStrictEqual(await launch(['list', 'pc']), { ok: false, error: REFUSED })
  assert.deepStrictEqual(await launch('stream pc Game'), { ok: false, error: REFUSED })
  assert.deepStrictEqual(await launcher({ find: () => null, run })(['stream', 'pc', 'Game']), { ok: false, error: NOT_INSTALLED })
  assert.strictEqual(calls.length, 0)
})

test('a program that will not start says so', async () => {
  assert.deepStrictEqual(await launcher({ find: here, run: fakeRun('error').run })(['stream', 'pc', 'Game']), { ok: false, error: FAILED })
  const throwing = () => { throw new Error('stand-in') }
  assert.deepStrictEqual(await launcher({ find: here, run: throwing })(['stream', 'pc', 'Game']), { ok: false, error: FAILED })
})

test('a page cannot start it over and over', async () => {
  const { calls, run } = fakeRun()
  let clock = 1000
  const launch = launcher({ find: here, run, now: () => clock })
  assert.deepStrictEqual(await launch(['stream', 'pc', 'Game']), { ok: true })
  assert.deepStrictEqual(await launch(['stream', 'pc', 'Game']), { ok: false, error: TOO_SOON })
  clock += MIN_GAP_MS
  assert.deepStrictEqual(await launch(['stream', 'pc', 'Game']), { ok: true })
  assert.strictEqual(calls.length, 2)
})
