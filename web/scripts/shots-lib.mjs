// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Helpers for the scripts that take the pictures in the docs. A headless browser of its own, no
// window, always 1920 x 1080 unless a script says otherwise. See shots-setup.mjs and shots.mjs.
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import(process.env.WITTYVISUALIZER ? new URL(process.env.WITTYVISUALIZER + '/tests/cdp.mjs', import.meta.url).href : './cdp.mjs')

export const wait = (ms) => new Promise((d) => setTimeout(d, ms))

export async function open(server, out, { width = 1920, height = 1080, path = '/' } = {}) {
  mkdirSync(out, { recursive: true })
  const browser = await launch({ software: true, width, height })
  const page = await browser.open(server + path)
  const size = (w, h, mobile = false, scale = 1) => page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: scale, mobile })
  await size(width, height)
  const shot = async (name) => {
    await page.eval(`document.fonts?.ready`)
    writeFileSync(join(out, `${name}.png`), await page.shot())
    console.log('shot', name)
  }
  // A picture of one element at twice the size, however long it is, for guides read at phone width.
  const card = async (name, selector, pad = 16) => {
    const box = await page.eval(`(() => { const r = document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect(); return { x: r.x + scrollX, y: r.y + scrollY, width: r.width, height: r.height } })()`)
    const clip = { x: Math.max(0, box.x - pad), y: Math.max(0, box.y - pad), width: box.width + pad * 2, height: box.height + pad * 2, scale: 2 }
    const { data } = await page.send('Page.captureScreenshot', { format: 'png', clip, captureBeyondViewport: true })
    writeFileSync(join(out, `${name}.png`), Buffer.from(data, 'base64'))
    console.log('card', name, Math.round(clip.width * 2), Math.round(clip.height * 2))
  }
  const call = (method, path, body) => page.eval(`(async () => { const t = localStorage.getItem('wms.token'); const r = await fetch('/api${path}', { method: '${method}', headers: { 'Content-Type': 'application/json', ...(t ? { Authorization: 'Bearer ' + t } : {}) }, body: ${body ? `JSON.stringify(${JSON.stringify(body)})` : 'undefined'} }); return r.json().catch(() => null) })()`)
  // Presses the first visible control whose words match, as a person would pick it out.
  const press = (words, within = 'body') => page.eval(`(() => {
    const rx = ${words instanceof RegExp ? words.toString() : `new RegExp(${JSON.stringify(words.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))})`}
    const all = [...document.querySelectorAll(${JSON.stringify(within)} + ' button, ' + ${JSON.stringify(within)} + ' a, ' + ${JSON.stringify(within)} + ' [role=button], ' + ${JSON.stringify(within)} + ' [role=radio], ' + ${JSON.stringify(within)} + ' [role=checkbox], ' + ${JSON.stringify(within)} + ' [role=tab], ' + ${JSON.stringify(within)} + ' label, ' + ${JSON.stringify(within)} + ' summary')]
    const e = all.find((b) => { const r = b.getBoundingClientRect(); return r.width > 1 && r.height > 1 && rx.test((b.getAttribute('aria-label') || '') + ' ' + (b.title || '') + ' ' + b.textContent) })
    if (!e) return false
    e.scrollIntoView({ block: 'center' }); e.click(); return (e.textContent || '').trim().slice(0, 50)
  })()`)
  const typeIn = async (selector, text) => {
    await page.eval(`document.querySelector(${JSON.stringify(selector)})?.focus()`)
    await page.send('Input.insertText', { text })
  }
  // This machine's addresses stay out of the pictures: the example address the guides use, instead.
  const maskAddresses = () => page.eval(`(() => {
    const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
    const seen = []
    for (let n = w.nextNode(); n; n = w.nextNode()) if (/\\b(\\d{1,3}\\.){3}\\d{1,3}\\b/.test(n.nodeValue)) seen.push(n)
    seen.forEach((n, i) => {
      if (i > 0 && /^\\s*https?:\\/\\/[\\d.]+(:\\d+)?\\s*$/.test(n.nodeValue)) { n.parentElement.style.display = 'none'; return }
      n.nodeValue = n.nodeValue.replace(/\\b(\\d{1,3}\\.){3}\\d{1,3}(:\\d+)?\\b/g, '192.168.1.20:9797')
    })
  })()`)
  const until = (expression, ms = 30000, what) => page.until(expression, ms, what)
  const go = async (path, settle = 1400) => { await page.go(server + path); await wait(settle) }
  const words = () => page.eval(`document.body.innerText.slice(0, 1500)`)
  return { browser, page, size, shot, card, call, press, typeIn, until, go, words, maskAddresses }
}
