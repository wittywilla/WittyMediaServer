// Measures the web app the way a slow TV or phone meets it, in a headless browser on a running test
// server. No window opens. For each page it counts what is downloaded (by kind, in bytes as sent) and
// how many requests, the time until the page is ready with the processor slowed down, and the work the
// browser did. Then it scrolls a big library grid and counts React's renders and the requests on the
// way, and browses round the app for a while to see whether memory keeps growing.
//
// Byte and render counts are the numbers to compare: timings move with whatever else the machine is
// doing, so each page is loaded several times and the middle one is kept.
//
//   WMS_SERVER=http://perf.localhost:19790 WMS_USER=... WMS_PASSWORD=... WMS_GRID=/library/4?view=all node scripts/measure-perf.mjs [out.json]
//
// WMS_ITEM=/item/12 names an item page (the first item of the grid otherwise). WMS_SLOW=4 is how many
// times slower the processor is made (4 is a low-end phone or TV). WMS_RUNS=3 loads per page.
import { writeFileSync } from 'node:fs'
const { launch } = await import(new URL((process.env.WITTYVISUALIZER || '../../../WittyVisualizer') + '/tests/cdp.mjs', import.meta.url))

const server = process.env.WMS_SERVER || 'http://perf.localhost:19790'
const slow = Number(process.env.WMS_SLOW || 4)
const runs = Number(process.env.WMS_RUNS || 3)
const [out] = process.argv.slice(2)
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const median = (a) => [...a].sort((x, y) => x - y)[Math.floor(a.length / 2)]
const kb = (n) => Math.round(n / 102.4) / 10

// React tells a devtools hook about every commit, production builds included. This one counts the
// components that really ran their render: a fiber that is a fresh copy since the last commit and
// carries React's PerformedWork flag. Mounts are counted apart.
const HOOK = `(() => {
  const r = window.__wmsRenders = { commits: 0, renders: 0, mounts: 0, on: true }
  const last = new WeakMap()
  const COMPONENT = new Set([0, 1, 11, 14, 15])
  window.__REACT_DEVTOOLS_GLOBAL_HOOK__ = {
    supportsFiber: true, renderers: new Map(), isDisabled: false,
    inject() { return 1 }, checkDCE() {}, onScheduleFiberRoot() {}, onCommitFiberUnmount() {}, onPostCommitFiberRoot() {},
    onCommitFiberRoot(_id, root) {
      if (!r.on) return
      r.commits++
      const stack = [root.current]
      while (stack.length) {
        const f = stack.pop()
        if (COMPONENT.has(f.tag)) {
          const seen = last.get(f) || (f.alternate && last.get(f.alternate))
          if (!seen) r.mounts++
          else if (seen.fiber !== f && (f.flags & 1)) r.renders++
          const rec = seen || {}
          rec.fiber = f
          last.set(f, rec)
          if (f.alternate) last.set(f.alternate, rec)
        }
        if (f.sibling) stack.push(f.sibling)
        if (f.child) stack.push(f.child)
      }
    },
  }
})()`

const b = await launch({ width: 1440, height: 900 })
const page = await b.open()
await page.send('Network.enable')
await page.send('Performance.enable')

// What the network carried, by kind.
let reqs = new Map()
page.on('Network.requestWillBeSent', (e) => { reqs.set(e.requestId, { url: e.request.url, type: e.type || 'Other', bytes: 0, done: false }) })
page.on('Network.responseReceived', (e) => { const r = reqs.get(e.requestId); if (r) { r.type = e.type || r.type; r.cached = e.response.fromDiskCache || e.response.fromServiceWorker } })
page.on('Network.loadingFinished', (e) => { const r = reqs.get(e.requestId); if (r) { r.bytes = e.encodedDataLength; r.done = true } })
page.on('Network.loadingFailed', (e) => { const r = reqs.get(e.requestId); if (r) r.done = true })
const traffic = () => {
  const by = {}
  for (const r of reqs.values()) {
    const k = /\/api\//.test(r.url) && !/poster|backdrop|art|logo|thumb/.test(r.url) ? 'api' : r.type
    by[k] ||= { n: 0, kb: 0 }
    by[k].n++; by[k].kb += r.bytes / 1024
  }
  for (const v of Object.values(by)) v.kb = Math.round(v.kb * 10) / 10
  return { requests: reqs.size, kb: kb([...reqs.values()].reduce((s, r) => s + r.bytes, 0)), by }
}
const idle = async (quiet = 600, max = 30000) => {
  const end = Date.now() + max
  let still = Date.now()
  while (Date.now() < end) {
    if ([...reqs.values()].some((r) => !r.done)) still = Date.now()
    else if (Date.now() - still > quiet) return
    await wait(50)
  }
}
// Like page.until, but a page that is still being replaced counts as not ready yet.
const until = async (expression, ms, what) => {
  const end = Date.now() + ms
  for (;;) {
    try { if (await page.eval(`!!(${expression})`)) return } catch { /* the old page is going */ }
    if (Date.now() > end) throw new Error('timed out waiting for ' + what)
    await wait(50)
  }
}
const metrics = async () => Object.fromEntries((await page.send('Performance.getMetrics')).metrics.map((m) => [m.name, m.value]))

// Sign in, in the page, so the browser holds the cookie.
await page.go(server + '/')
await page.eval(`fetch('/api/login', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(${JSON.stringify({ name: process.env.WMS_USER, password: process.env.WMS_PASSWORD })}) }).then((r) => r.status)`)
await page.send('Page.addScriptToEvaluateOnNewDocument', { source: HOOK })

const grid = process.env.WMS_GRID || '/library/1?view=all'
let item = process.env.WMS_ITEM
if (!item) {
  const lib = /\/library\/(\d+)/.exec(grid)?.[1]
  const r = await page.eval(`fetch('/api/browse?library=${lib}&limit=1').then((r) => r.json())`)
  item = '/item/' + (r.results?.[0]?.item?.id ?? 1)
}
const PAGES = [
  ['Home', '/', `document.querySelector('main h1, main .shelf, main .empty')`],
  ['Big grid', grid, `document.querySelectorAll('main .grid > *').length > 10`],
  ['Item page', item, `document.querySelector('main h1')`],
  ['TV guide', '/tv', `document.querySelector('main .ctv, main [class*=guide], main h1')`],
  ['Settings', '/settings', `document.querySelector('main h1, main .set-card')`],
]

const result = { server, slow, when: new Date().toISOString(), pages: {} }

// First visits, with nothing cached, and repeat visits, with the cache.
await page.send('Emulation.setCPUThrottlingRate', { rate: slow })
for (const [name, path, ready] of PAGES) {
  const one = {}
  for (const cold of [true, false]) {
    const times = [], fcps = [], lcps = [], scripts = [], heaps = [], nodes = []
    let net
    for (let i = 0; i < runs; i++) {
      await page.send('Network.setCacheDisabled', { cacheDisabled: cold })
      if (!cold && i === 0) { await page.go(server + path); await idle() } // fill the cache
      await page.send('Page.navigate', { url: 'about:blank' }); await wait(200)
      reqs = new Map()
      const before = await metrics()
      const t0 = Date.now()
      await page.send('Page.navigate', { url: server + path })
      await until(ready, 30000, name)
      const t = Date.now() - t0
      await idle()
      const m = await metrics()
      const paint = await page.eval(`(() => { const p = performance.getEntriesByName('first-contentful-paint')[0]; return new Promise((d) => { let l = 0; new PerformanceObserver((list) => { for (const e of list.getEntries()) l = e.startTime }).observe({ type: 'largest-contentful-paint', buffered: true }); setTimeout(() => d({ fcp: p ? p.startTime : 0, lcp: l }), 50) }) })()`)
      times.push(t); fcps.push(paint.fcp); lcps.push(paint.lcp)
      scripts.push((m.ScriptDuration - (before.ScriptDuration || 0)) * 1000)
      heaps.push(m.JSHeapUsedSize / 1048576); nodes.push(m.Nodes)
      net = traffic()
    }
    one[cold ? 'first' : 'repeat'] = { readyMs: median(times), fcpMs: Math.round(median(fcps)), lcpMs: Math.round(median(lcps)), scriptMs: Math.round(median(scripts)), heapMB: Math.round(median(heaps) * 10) / 10, domNodes: median(nodes), ...net }
  }
  result.pages[name] = one
  console.log(name, JSON.stringify(one))
}
await page.send('Network.setCacheDisabled', { cacheDisabled: false })
await page.send('Emulation.setCPUThrottlingRate', { rate: 1 })

// Scrolling the big grid to its end: renders, requests and what stays in the page.
{
  await page.go(server + grid)
  await until(PAGES[1][2], 30000, 'the grid')
  await idle()
  reqs = new Map()
  await page.eval(`(window.__wmsRenders.commits = window.__wmsRenders.renders = window.__wmsRenders.mounts = 0)`)
  const t0 = Date.now()
  let lastHeight = 0, same = 0
  while (same < 15 && Date.now() - t0 < 120000) {
    const h = await page.eval(`(scrollBy(0, 700), document.scrollingElement.scrollHeight + ':' + Math.round(scrollY + innerHeight))`)
    const [height, bottom] = h.split(':').map(Number)
    if (bottom >= height - 2 && height === lastHeight) same++; else same = 0
    lastHeight = height
    await wait(120)
  }
  await idle()
  const r = await page.eval(`({ ...window.__wmsRenders, cards: document.querySelectorAll('main .grid > *').length, imgs: document.querySelectorAll('main .grid img').length, decodedMB: Math.round([...document.querySelectorAll('main .grid img')].filter((i) => i.complete).reduce((s, i) => s + i.naturalWidth * i.naturalHeight * 4, 0) / 1048576) })`)
  await page.send('HeapProfiler.collectGarbage')
  const m = await metrics()
  result.scroll = { ...r, ...traffic(), heapMB: Math.round(m.JSHeapUsedSize / 104857.6) / 10, domNodes: m.Nodes, listeners: m.JSEventListeners }
  console.log('Scroll the grid', JSON.stringify(result.scroll))
}

// Browsing round the app inside one page load, as someone would on a TV: is memory given back?
{
  await page.go(server + '/')
  await until(PAGES[0][2], 30000, 'home')
  await page.eval(`(window.__wmsRenders.on = false)`)
  const go = (p) => page.eval(`(history.pushState({}, '', ${JSON.stringify(p)}), dispatchEvent(new PopStateEvent('popstate')), 1)`)
  const heaps = []
  for (let lap = 0; lap < 6; lap++) {
    for (const [, path, ready] of PAGES) {
      await go(path)
      await until(ready, 30000, path)
      await idle(400)
      if (path === grid) for (let i = 0; i < 8; i++) { await page.eval('scrollBy(0, 900)'); await wait(100) }
    }
    await page.send('HeapProfiler.collectGarbage')
    const m = await metrics()
    heaps.push({ heapMB: Math.round(m.JSHeapUsedSize / 104857.6) / 10, domNodes: m.Nodes, listeners: m.JSEventListeners, documents: m.Documents })
  }
  result.browse = heaps
  console.log('Browse laps', JSON.stringify(heaps))
}

await b.close()
if (out) writeFileSync(out, JSON.stringify(result, null, 2))
