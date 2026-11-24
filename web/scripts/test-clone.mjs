// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Clone a station (#622), the whole flow in a browser: Channels → Clone a station, a guide file
// chosen through the page's own file picker, the station, the matched schedule with its colours,
// a swap, the advanced options, and saving. It makes one channel and deletes it again.
//   WMS_SERVER=http://clone.localhost:19622 WMS_TOKEN_FILE=<a file holding an admin's sign-in token> node scripts/test-clone.mjs <out folder>
// The guide is internal/channels/testdata/clone/station.xml: a made-up evening on "KWIT 4". Any
// library will do: what isn't in it is filled by stand-ins, and that is checked too.
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const token = readFileSync(process.env.WMS_TOKEN_FILE, 'utf8').trim()
const [out] = process.argv.slice(2)
const guide = fileURLToPath(new URL('../../internal/channels/testdata/clone/station.xml', import.meta.url))
const results = []
const check = (name, ok, detail) => { results.push({ name, ok: !!ok, detail }); console.log(ok ? 'PASS' : 'FAIL', name, detail ?? '') }

const s = await open(server, out, { path: '/legal', width: 1440, height: 1000 })
const { page, shot, press, until, go, call } = s
let made = 0
try {
  await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.removeItem('wms.channelDraft.new')`)
  await go('/tv/channels', 2500)
  check('Channels offers Clone a station', await press('Clone a station'))
  await until(`!!document.querySelector('#ch-clone')`, 20000, 'the clone section')
  check('the editor opens on Clone a station', await page.eval(`document.querySelector('.tvs-level.on')?.textContent.includes('Clone a station')`))
  await press('Guide file or link')
  await wait(400)
  // The page's own file picker, given the guide as a person would choose it.
  const { root } = await page.send('DOM.getDocument', { depth: -1, pierce: true })
  const { nodeId } = await page.send('DOM.querySelector', { nodeId: root.nodeId, selector: '#ch-clone input[type=file]' })
  await page.send('DOM.setFileInputFiles', { nodeId, files: [guide] })
  await until(`document.querySelector('#ch-clone')?.innerText.includes('station.xml')`, 20000, 'the upload')
  check('the guide file uploads', true)
  await page.eval(`(() => { const i = document.querySelector('#ch-clone input[type=date]'); const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set; set.call(i, '2026-11-23'); i.dispatchEvent(new Event('input', { bubbles: true })) })()`)
  await wait(300)
  await shot('clone-1-source')
  await press('Find stations')
  await until(`document.querySelectorAll('.clone-station').length > 0`, 30000, 'stations')
  const stations = await page.eval(`[...document.querySelectorAll('.clone-station')].map((b) => b.innerText.replace(/\\n/g, ' | '))`)
  check('stations are listed with their programmes', stations.some((t) => t.startsWith('KWIT 4') && t.includes('5 programmes')), stations)
  await press(/^\s*KWIT 4/)
  await until(`document.querySelectorAll('.clone-slot').length > 0`, 60000, 'the matched schedule')
  await wait(500)
  await shot('clone-2-matched')
  const rows = await page.eval(`[...document.querySelectorAll('.clone-slot')].map((r) => ({ cls: r.className, time: r.querySelector('.clone-time')?.firstChild?.textContent, guide: r.querySelector('.clone-guide b')?.textContent, chip: r.querySelector('.clone-chip')?.textContent }))`)
  check('every programme has a row, the gap one of its own', rows.length === 5 && rows[3].guide === 'Nothing in the guide', rows.map((r) => r.time + ' ' + r.guide))
  check('each row says how it was found, in a colour and a word', rows.every((r) => /\b(exact|stand|missing)\b/.test(r.cls) && r.chip), rows.map((r) => r.chip))
  check('the channel takes the station’s name', await page.eval(`[...document.querySelectorAll('input')].some((i) => i.value === 'KWIT 4')`))
  const tally = await page.eval(`document.querySelector('.clone-tally')?.innerText`)
  check('a tally of exact, stand-ins and missing', /exact/.test(tally) && /stand-ins/.test(tally) && /missing/.test(tally), tally)

  // Swap the first slot for the second thing offered.
  await page.eval(`[...document.querySelectorAll('.clone-slot button')].find((b) => /Swap/.test(b.textContent))?.click()`)
  await until(`document.querySelectorAll('.clone-cand').length > 0 || document.querySelector('.ui-dialog')?.innerText.includes('Nothing in your library')`, 30000, 'the swap list')
  await shot('clone-3-swap')
  const cands = await page.eval(`document.querySelectorAll('.clone-cand').length`)
  if (cands > 0) {
    await page.eval(`(document.querySelectorAll('.clone-cand')[1] || document.querySelector('.clone-cand')).click()`)
    await wait(500)
    check('a swap is your pick', await page.eval(`document.querySelector('.clone-slot .clone-chip')?.textContent.includes('Your pick')`))
  } else {
    check('a swap is your pick (the library has nothing to offer)', true)
    await press('Cancel', '.ui-dialog')
  }

  // Advanced: many breaks, follow the station.
  await press('Advanced options')
  await until(`!!document.querySelector('.clone-adv')`, 10000, 'the advanced options')
  await page.eval(`(() => { const sel = [...document.querySelectorAll('.clone-adv select')][0]; const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set; set.call(sel, 'high'); sel.dispatchEvent(new Event('change', { bubbles: true })) })()`)
  await page.eval(`document.querySelector('.clone-adv [role=switch][aria-label="Follow the station"]').click()`)
  await shot('clone-4-advanced')
  await press(/^\s*Apply\s*$/, '.ui-dialog')
  await until(`!document.querySelector('.clone-adv')`, 10000, 'the dialog to close')
  await wait(2500)
  check('the options show on the schedule', await page.eval(`document.querySelector('.clone-summary')?.innerText.includes('Follows the station each week')`))

  await press(/Create the channel/)
  await until(`location.pathname === '/tv/channels'`, 30000, 'back to the channel list')
  const list = await call('GET', '/channels')
  const ch = (list || []).find((c) => c.name === 'KWIT 4')
  made = ch?.id || 0
  const def = ch?.definition
  check('saved as a cloned channel', def?.mode === 'clone' && def.clone?.slots?.length === 5, def?.mode)
  check('its blocks were made from the guide', (def?.blocks || []).length >= 4 && def.blocks.every((b) => (b.once && b.fit === 'cut') || (!b.once && b.fit === 'fill' && b.rules?.shows?.length === 1)), (def?.blocks || []).map((b) => b.start + '-' + b.end))
  check('the swap and the options were kept', def?.clone?.slots?.[0]?.pinned === (cands > 0) && def?.clone?.options?.density === 'high' && def?.clone?.options?.refresh === true)
  const guideNow = await call('GET', '/guide?hours=24')
  const row = (guideNow?.channels || []).find((c) => c.name === 'KWIT 4')
  check('it is on the guide', !!row, (guideNow?.channels || []).map((c) => c.name))
  await shot('clone-5-list')
} catch (e) {
  check('ran', false, String(e && e.message || e))
} finally {
  if (made) await call('DELETE', `/channels/${made}`).catch(() => {})
  await s.browser.close()
}
writeFileSync(out + '/clone-report.json', JSON.stringify(results, null, 1))
process.exit(results.every((r) => r.ok) ? 0 : 1)
