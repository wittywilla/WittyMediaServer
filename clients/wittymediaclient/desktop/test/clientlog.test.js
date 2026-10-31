const test = require('node:test')
const assert = require('node:assert')
const fs = require('fs')
const os = require('os')
const path = require('path')
const { createClientLog, redact, toLine, MAX_EVENTS, MAX_MESSAGE, MAX_STACK, MAX_WAITING, SEND_PER_MINUTE } = require('../clientlog.js')

const CASES = JSON.parse(fs.readFileSync(path.join(__dirname, '..', '..', 'shared', 'data', 'redaction-cases.json'), 'utf8')).cases
const HOME = process.platform === 'win32' ? 'C:\\Users\\Someone' : '/home/someone'

function tempDir(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'wmc-log-'))
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }))
  return path.join(dir, 'logs')
}

const fileText = (log) => fs.readFileSync(log.file, 'utf8')
const fileLines = (log) => fileText(log).trim().split('\n').map((l) => JSON.parse(l))

// A server that remembers what it was sent. Long flush times: tests flush by hand.
function withServer(t, answer = () => ({ status: 204 }), extra = {}) {
  const sent = []
  const log = createClientLog({
    dir: tempDir(t), app: 'desktop', version: '1.2.3', device: 'Test OS', home: HOME, flushMs: 3600 * 1000,
    target: async () => ({ origin: 'http://server.test', token: 'the-sign-in-token' }),
    post: async (_to, body) => { sent.push(JSON.parse(body)); return answer(sent.length) },
    ...extra,
  })
  return { log, sent }
}

// ---------------------------------------------------------------- redaction

test('hides every secret in the shared cases and keeps the rest', () => {
  for (const c of CASES) {
    const out = redact(c.input)
    for (const gone of c.gone) assert.ok(!out.includes(gone), `"${gone}" survived in: ${out}`)
    for (const kept of c.kept) assert.ok(out.includes(kept), `"${kept}" was lost from: ${out}`)
  }
})

test('planted secrets reach neither the file nor the server', async (t) => {
  const { log, sent } = withServer(t)
  const planted = ['PLANTED-token', 'PLANTED-stream', 'PLANTED-bearer', 'PLANTED-pass', 'PLANTED-cookie', 'PLANTED-header', 'PLANTED-stack']
  log.error('network', 'GET /x?token=PLANTED-token and ?st=PLANTED-stream', {
    stack: 'Error: at /y?password=PLANTED-pass\nCookie: wms_token=PLANTED-cookie\nAuthorization: Bearer PLANTED-bearer\nX-WMS-Token: PLANTED-header\n"token":"PLANTED-stack"',
    where: 'http://server.test/z?token=PLANTED-token',
  })
  await log.flush()
  await log.idle()
  const everything = fileText(log) + JSON.stringify(sent) + log.diagnostics()
  for (const secret of planted) assert.ok(!everything.includes(secret), `${secret} leaked`)
  assert.strictEqual(sent.length, 1)
})

test('a token it has been told about is hidden anywhere, even with nothing around it', async (t) => {
  const { log } = withServer(t)
  log.addSecret('plainwords-that-are-a-token')
  log.warn('js', 'the value plainwords-that-are-a-token turned up')
  await log.idle()
  assert.ok(!fileText(log).includes('plainwords-that-are-a-token'))
  assert.ok(fileText(log).includes('turned up'))
})

test('the home folder does not leave the computer in a stack', async (t) => {
  const { log, sent } = withServer(t)
  log.error('crash', 'failed', { stack: `at ${HOME}${path.sep}AppData${path.sep}main.js:1:1` })
  await log.flush()
  await log.idle()
  assert.ok(!(fileText(log) + JSON.stringify(sent)).includes(HOME))
  assert.ok(sent[0].events[0].stack.startsWith('at ~'))
})

test('an unwieldy string is cleaned quickly, on a slow machine too', () => {
  const started = Date.now()
  for (const s of ['a'.repeat(1 << 22), 'a.'.repeat(1 << 21), '%41'.repeat(1 << 20), '"'.repeat(1 << 21), '"a":"'.repeat(1 << 19), 'x://a:'.repeat(1 << 19)]) redact(s)
  assert.ok(Date.now() - started < 5000, `took ${Date.now() - started} ms`)
})

// ---------------------------------------------------------------- the file

test('each record is one line of JSON, with control characters escaped', async (t) => {
  const { log } = withServer(t)
  log.warn('js', 'two\nlines\r\u001b[31mred\u0007 \u202eevil\u2028 \u0085end')
  await log.idle()
  const text = fileText(log)
  assert.strictEqual(text.split('\n').length, 2)
  assert.ok(!/[\u0000-\u0009\u000b-\u001f\u007f-\u009f\u2028\u2029\u202e]/.test(text), 'a control character got through')
  assert.strictEqual(JSON.parse(text).message, 'two\nlines\r\u001b[31mred\u0007 \u202eevil\u2028 \u0085end')
})

test('toLine ends in exactly one line break', () => {
  assert.ok(toLine({ a: 'x\ny' }).endsWith('"}\n'))
  assert.strictEqual(toLine({ a: 'x\ny' }).split('\n').length, 2)
})

test('the file rotates by size and keeps only so many', async (t) => {
  const { log } = withServer(t, undefined, { maxFileBytes: 2000, keepFiles: 3, linesPerMinute: 100000 })
  for (let i = 0; i < 300; i++) {
    log.info('app', `line ${i} ${'x'.repeat(40)}`)
    if (i % 20 === 0) await log.idle()
  }
  await log.idle()
  const names = fs.readdirSync(log.dir).sort()
  assert.deepStrictEqual(names, ['client.log', 'client.log.1', 'client.log.2'])
  for (const n of names) assert.ok(fs.statSync(path.join(log.dir, n)).size <= 2000, `${n} is too big`)
  assert.match(fileText(log), /line 299 /)
  assert.match(fs.readFileSync(path.join(log.dir, 'client.log.1'), 'utf8'), /line 2\d\d /, 'the next file holds what came before')
})

test('a file that can be neither moved nor emptied still stops growing', async (t) => {
  const stubborn = { ...fs, rename: (_a, _b, cb) => cb(new Error('locked')), truncate: (_a, _b, cb) => cb(new Error('locked')) }
  const { log } = withServer(t, undefined, { fs: stubborn, maxFileBytes: 1000, linesPerMinute: 100000 })
  for (let i = 0; i < 100; i++) log.info('app', 'x'.repeat(100))
  await log.idle()
  for (let i = 0; i < 100; i++) log.info('app', 'x'.repeat(100))
  await log.idle()
  assert.ok(fs.statSync(log.file).size <= 1000)
})

test('a file that is full when the app starts is rotated first', async (t) => {
  const dir = tempDir(t)
  fs.mkdirSync(dir, { recursive: true })
  fs.writeFileSync(path.join(dir, 'client.log'), 'x'.repeat(1990) + '\n')
  const { log } = withServer(t, undefined, { dir, maxFileBytes: 2000 })
  log.info('app', 'a line that does not fit')
  await log.idle()
  assert.ok(fs.existsSync(path.join(dir, 'client.log.1')))
  assert.strictEqual(fileLines(log).length, 1)
})

test('what is queued is written by the last call before quitting', (t) => {
  const { log } = withServer(t)
  log.error('crash', 'the last thing that happened')
  log.drainSync()
  assert.ok(fs.readFileSync(log.file, 'utf8').includes('the last thing that happened'))
})

test('errors carry the version and system in the file', async (t) => {
  const { log } = withServer(t)
  log.error('crash', 'it went wrong')
  log.info('app', 'all fine')
  await log.idle()
  const [error, info] = fileLines(log)
  assert.strictEqual(error.version, '1.2.3')
  assert.strictEqual(error.device, 'Test OS')
  assert.strictEqual(info.version, undefined)
})

// ---------------------------------------------------------------- sending

test('a batch has the shape the server reads', async (t) => {
  const { log, sent } = withServer(t)
  log.error('crash', 'it stopped', { stack: 'at here', page: 'main process', session: 'v-1', request: 'r-2', where: 'http://other.test' })
  await log.flush()
  assert.strictEqual(sent.length, 1)
  const { app, version, device, events } = sent[0]
  assert.deepStrictEqual([app, version, device], ['desktop', '1.2.3', 'Test OS'])
  const [e] = events
  assert.deepStrictEqual([e.level, e.kind, e.message, e.stack, e.page, e.session, e.request, e.count],
    ['error', 'crash', 'it stopped', 'at here', 'main process', 'v-1', 'r-2', 1])
  assert.ok(!Number.isNaN(Date.parse(e.time)))
})

test('an address given as where stays in the file and never goes to a server', async (t) => {
  const { log, sent } = withServer(t)
  log.warn('network', 'not reachable', { where: 'http://other-server.test:9797' })
  await log.flush()
  await log.idle()
  assert.ok(fileText(log).includes('other-server.test'))
  assert.ok(!JSON.stringify(sent).includes('other-server.test'))
})

test('repeats are sent once, with a count', async (t) => {
  const { log, sent } = withServer(t)
  for (let i = 0; i < 40; i++) log.error('crash', 'the same thing')
  log.error('crash', 'something else')
  await log.flush()
  assert.strictEqual(sent[0].events.length, 2)
  assert.strictEqual(sent[0].events[0].count, 40)
})

test('info stays on this computer', async (t) => {
  const { log, sent } = withServer(t)
  log.info('app', 'started')
  await log.flush()
  await log.idle()
  assert.strictEqual(sent.length, 0)
  assert.ok(fileText(log).includes('started'))
})

test('nothing is sent without a server, and it waits for one', async (t) => {
  let to = null
  const { log, sent } = withServer(t, undefined, { target: async () => to })
  log.error('crash', 'before connecting')
  await log.flush()
  assert.strictEqual(sent.length, 0)
  to = { origin: 'http://server.test', token: 'the-sign-in-token' }
  await log.flush()
  assert.strictEqual(sent[0].events[0].message, 'before connecting')
})

test('the size limits hold, however long the messages', async (t) => {
  const { log, sent } = withServer(t)
  for (let i = 0; i < MAX_WAITING; i++) log.error('crash', `${i} ${'m'.repeat(5000)}`, { stack: 's'.repeat(50000) })
  await log.flush()
  const body = JSON.stringify(sent[0])
  assert.ok(Buffer.byteLength(body) <= 64 * 1024, `body is ${Buffer.byteLength(body)} bytes`)
  assert.ok(sent[0].events.length <= MAX_EVENTS)
  for (const e of sent[0].events) {
    assert.ok(e.message.length <= MAX_MESSAGE + 1)
    assert.ok(e.stack.length <= MAX_STACK + 1)
  }
})

test('a flood leaves bounded memory, bounded requests and a count of what was lost', async (t) => {
  let clock = 1000000
  const { log, sent } = withServer(t, undefined, { now: () => clock })
  for (let i = 0; i < 20000; i++) log.error('js', `flood ${i}`)
  await log.idle()
  for (let i = 0; i < 20; i++) await log.flush()
  const events = sent.flatMap((s) => s.events)
  assert.ok(events.length <= SEND_PER_MINUTE, `${events.length} events in one minute`)
  clock += 61 * 1000
  for (let i = 0; i < 20; i++) await log.flush()
  clock += 61 * 1000
  for (let i = 0; i < 20; i++) await log.flush()
  clock += 61 * 1000
  for (let i = 0; i < 20; i++) await log.flush()
  const all = sent.flatMap((s) => s.events)
  assert.ok(all.filter((e) => e.message.startsWith('flood')).length <= MAX_WAITING)
  const lost = all.find((e) => /earlier reports/.test(e.message))
  assert.ok(lost, 'the count of what was lost is reported')
  assert.match(lost.message, new RegExp(`^${20000 - MAX_WAITING} `))
  assert.ok(fs.statSync(log.file).size < 2 * 1024 * 1024, 'the file did not take the whole flood')
  assert.ok(fileLines(log).length <= 121)
})

test('a failed report is dropped and not tried again in a loop', async (t) => {
  let clock = 1000000
  const { log, sent } = withServer(t, () => { throw new Error('connection refused') }, { now: () => clock })
  log.error('crash', 'one')
  await log.flush()
  assert.strictEqual(sent.length, 1)
  log.error('crash', 'two')
  for (let i = 0; i < 10; i++) await log.flush()
  assert.strictEqual(sent.length, 1, 'held off after failing')
  clock += 61 * 1000
  await log.flush()
  assert.strictEqual(sent.length, 2)
  assert.deepStrictEqual(sent[1].events.map((e) => e.message), ['1 earlier reports from this app were left out', 'two'])
})

test('a 429 waits as long as the server says, and drops the batch', async (t) => {
  let clock = 1000000
  const { log, sent } = withServer(t, () => ({ status: 429, retryAfter: '120' }), { now: () => clock })
  log.error('crash', 'one')
  await log.flush()
  log.error('crash', 'two')
  clock += 90 * 1000
  await log.flush()
  assert.strictEqual(sent.length, 1)
  clock += 31 * 1000
  await log.flush()
  assert.strictEqual(sent.length, 2)
})

test('a failure to send is noted in the file once, and never sent as a report', async (t) => {
  let clock = 1000000
  const { log, sent } = withServer(t, () => ({ status: 500 }), { now: () => clock })
  for (let i = 0; i < 4; i++) {
    log.error('crash', `event ${i}`)
    await log.flush()
    clock += 61 * 1000
  }
  await log.idle()
  assert.strictEqual(fileLines(log).filter((l) => /were dropped/.test(l.message)).length, 1)
  assert.ok(!JSON.stringify(sent).includes('were dropped'))
})

test('sending starts by itself within the flush time, and stays quiet when empty', async (t) => {
  const { log, sent } = withServer(t, undefined, { flushMs: 20 })
  log.error('crash', 'on its own')
  await new Promise((r) => setTimeout(r, 200))
  assert.strictEqual(sent.length, 1)
  await new Promise((r) => setTimeout(r, 100))
  assert.strictEqual(sent.length, 1)
})

// ---------------------------------------------------------------- never throws

test('nothing thrown at it comes back out', async (t) => {
  const { log } = withServer(t, undefined, { target: async () => { throw new Error('no cookies today') } })
  const evil = { get message() { throw new Error('gotcha') }, get stack() { throw new Error('gotcha') }, toString() { throw new Error('gotcha') } }
  const circular = {}
  circular.self = circular
  assert.doesNotThrow(() => {
    log.noteError('error', 'crash', 'odd', evil)
    log.noteError('error', 'crash', 'odd', null)
    log.noteError('error', 'crash', 'odd', undefined)
    log.noteError('error', 'crash', 'odd', 'just a string')
    log.noteError('error', 'crash', 'odd', circular)
    log.noteError('error', 'crash', 'odd', Symbol('s'))
    log.error('crash', evil)
    log.error('crash', Symbol('s'))
    log.error(evil, 'odd kind')
    log.error('crash', 'odd extra', { stack: evil, page: Symbol('s'), where: circular })
    log.error('crash', 'no extra', null)
    log.warn()
    log.addSecret(evil)
    log.diagnostics([evil])
    log.kick()
  })
  await assert.doesNotReject(log.flush())
  await log.idle()
})

test('a folder that cannot be written to is not a problem', async (t) => {
  const dir = tempDir(t)
  fs.mkdirSync(path.dirname(dir), { recursive: true })
  fs.writeFileSync(dir, 'a file where the folder should go')
  const { log } = withServer(t, undefined, { dir })
  assert.doesNotThrow(() => { log.error('crash', 'first'); log.drainSync(); log.error('crash', 'second') })
  await log.idle()
})

// ---------------------------------------------------------------- diagnostic text

test('diagnostic info says what it is and hides what it should', (t) => {
  const { log } = withServer(t)
  log.error('network', 'failed with ?token=PLANTED-diag', { where: 'http://other-server.test' })
  const text = log.diagnostics(['Connected to a server: yes'])
  assert.match(text, /desktop 1\.2\.3/)
  assert.match(text, /Test OS/)
  assert.match(text, /Connected to a server: yes/)
  assert.match(text, /failed with/)
  assert.ok(!text.includes('PLANTED-diag'))
  assert.ok(!text.includes('other-server.test'))
})

// ---------------------------------------------------------------- the security model

test('a server\'s page gets nothing new: the logs link is for the connect screen only', () => {
  const preload = fs.readFileSync(path.join(__dirname, '..', 'preload.js'), 'utf8')
  const [local, serverPage] = preload.split('} else {')
  assert.ok(local.includes('openLogs'))
  assert.ok(!serverPage.includes('openLogs'))
  // Moonlight is all a server's page can ask for, and main.js checks that strictly.
  for (const name of ['probe', 'recent', 'open-server', 'redeem', 'discover']) {
    assert.ok(!serverPage.includes(name), `a server's page can't reach ${name}`)
  }
})
