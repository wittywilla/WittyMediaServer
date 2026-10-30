import { strFromU8, unzipSync } from 'fflate'
import type { EpubInfo, SearchHit } from '@wittywilla/wittyink'

// EPUBs unpacked on the device. The files are put in Cache Storage under
// book/<key>/<path> with a strict Content-Security-Policy, and the service
// worker (public/sw.js) serves them to the reader's sandboxed frame, so
// chapters find their images and styles by their usual relative links and no
// script in a book can run.

const BOOK_CACHE = 'wittyreader-book'
const CSP = "default-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'none'; object-src 'none'; form-action 'none'; frame-ancestors 'self'; base-uri 'none'; sandbox allow-same-origin"
const TYPES: Record<string, string> = {
  xhtml: 'application/xhtml+xml', xht: 'application/xhtml+xml', html: 'text/html', htm: 'text/html', css: 'text/css', svg: 'image/svg+xml',
  jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png', gif: 'image/gif', webp: 'image/webp', avif: 'image/avif',
  ttf: 'font/ttf', otf: 'font/otf', woff: 'font/woff', woff2: 'font/woff2', xml: 'application/xml', ncx: 'application/x-dtbncx+xml',
}
const typeOf = (path: string) => TYPES[path.split('.').pop()!.toLowerCase()] || 'application/octet-stream'
// A path in the book, relative to another file in it.
const resolve = (base: string, rel: string) => decodeURIComponent(new URL(rel, 'https://book/' + base).pathname.slice(1))
const xml = (s: string, type: DOMParserSupportedType = 'application/xml') => new DOMParser().parseFromString(s, type)
const byTag = (d: Document | Element, tag: string) => Array.from(d.getElementsByTagNameNS('*', tag))
// The address of a file in the book. The URL parser encodes it exactly as the
// browser will when a chapter links to it; only ?, # and % need doing first.
const fileUrl = (base: URL, path: string) => { const u = new URL(path.replace(/[?#%]/g, encodeURIComponent), base); return u.origin + u.pathname }

export type Epub = { files: Record<string, Uint8Array>; title: string; author?: string; year?: number | null; spine: string[]; toc: EpubInfo['toc']; cover?: string }

export function readEpub(bytes: Uint8Array): Epub {
  const files = unzipSync(bytes)
  const text = (p: string) => (files[p] ? strFromU8(files[p]) : '')
  const opfPath = xml(text('META-INF/container.xml')).querySelector('rootfile')?.getAttribute('full-path')
  if (!opfPath || !files[opfPath]) throw new Error('This is not an EPUB book (no package file).')
  const opf = xml(text(opfPath))
  const meta = (tag: string) => byTag(opf, tag)[0]?.textContent?.trim() || ''
  const items = new Map<string, { path: string; type: string; props: string }>()
  for (const it of byTag(opf, 'item')) {
    items.set(it.getAttribute('id') || '', { path: resolve(opfPath, it.getAttribute('href') || ''), type: it.getAttribute('media-type') || '', props: it.getAttribute('properties') || '' })
  }
  const spineEl = byTag(opf, 'spine')[0]
  const spine = byTag(opf, 'itemref').map((r) => items.get(r.getAttribute('idref') || '')?.path).filter((p): p is string => !!p && !!files[p])
  if (!spine.length) throw new Error('This EPUB has no chapters.')

  // Contents: the EPUB 3 navigation document, else the older NCX.
  const toc: EpubInfo['toc'] = []
  const nav = [...items.values()].find((i) => i.props.split(' ').includes('nav'))
  if (nav && files[nav.path]) {
    const d = xml(text(nav.path), 'application/xhtml+xml')
    const navs = byTag(d, 'nav')
    const main = navs.find((n) => (n.getAttribute('epub:type') || n.getAttributeNS('http://www.idpf.org/2007/ops', 'type') || '').includes('toc')) || navs[0]
    for (const a of main ? byTag(main, 'a') : []) {
      const href = a.getAttribute('href')
      if (!href) continue
      let depth = 0
      for (let e = a.parentElement; e && e !== main; e = e.parentElement) if (e.localName === 'ol') depth++
      toc.push({ title: '  '.repeat(Math.max(0, depth - 1)) + (a.textContent || '').replace(/\s+/g, ' ').trim(), href: resolveHref(nav.path, href) })
    }
  }
  if (!toc.length) {
    const ncx = items.get(spineEl?.getAttribute('toc') || '') || [...items.values()].find((i) => i.type === 'application/x-dtbncx+xml')
    if (ncx && files[ncx.path]) {
      for (const np of byTag(xml(text(ncx.path)), 'navPoint')) {
        const src = byTag(np, 'content')[0]?.getAttribute('src')
        if (!src) continue
        let depth = 0
        for (let e = np.parentElement; e; e = e.parentElement) if (e.localName === 'navPoint') depth++
        toc.push({ title: '  '.repeat(depth) + (byTag(np, 'text')[0]?.textContent || '').trim(), href: resolveHref(ncx.path, src) })
      }
    }
  }

  // The cover: the item marked as one (EPUB 3), else the one the metadata names.
  const coverId = byTag(opf, 'meta').find((m) => m.getAttribute('name') === 'cover')?.getAttribute('content') || ''
  const cover = [...items.values()].find((i) => i.props.split(' ').includes('cover-image'))?.path || items.get(coverId)?.path
  const year = parseInt(meta('date').slice(0, 4), 10)
  return { files, title: meta('title'), author: meta('creator') || undefined, year: year > 0 ? year : null, spine, toc, cover: cover && files[cover] ? cover : undefined }
}

function resolveHref(base: string, href: string) {
  const [file, frag] = href.split('#')
  return resolve(base, file || base.split('/').pop()!) + (frag ? '#' + frag : '')
}

// A chapter's readable text (for word counts and search).
function chapterText(bytes: Uint8Array): string {
  const s = strFromU8(bytes)
  let d = xml(s, 'application/xhtml+xml')
  if (d.querySelector('parsererror')) d = xml(s, 'text/html')
  return (d.body?.textContent || d.documentElement.textContent || '').replace(/\s+/g, ' ').trim()
}

// Puts the book where the service worker serves it, and returns what the
// reader needs: its info, chapter addresses and search.
export async function serveEpub(key: string, book: Epub) {
  if (!('serviceWorker' in navigator) || !('caches' in window)) throw new Error('This browser cannot show EPUB books here. Try a current browser, or open WittyReader from http://localhost or an https address.')
  await navigator.serviceWorker.ready
  if (!navigator.serviceWorker.controller) {
    await new Promise<void>((done) => {
      navigator.serviceWorker.addEventListener('controllerchange', () => done(), { once: true })
      setTimeout(done, 3000)
    })
  }
  const base = new URL(`book/${key}/`, document.baseURI)
  await caches.delete(BOOK_CACHE) // one open book at a time
  const cache = await caches.open(BOOK_CACHE)
  await Promise.all(Object.entries(book.files).map(([path, bytes]) => {
    if (path.endsWith('/')) return null
    const type = typeOf(path)
    const headers: Record<string, string> = { 'Content-Type': type, 'X-Content-Type-Options': 'nosniff' }
    headers['Content-Security-Policy'] = CSP // every file: a book's files are untrusted
    return cache.put(fileUrl(base, path), new Response(bytes as BodyInit, { headers }))
  }))
  const texts = book.spine.map((p) => chapterText(book.files[p]))
  const info: EpubInfo = { spine: book.spine, toc: book.toc, words: texts.map((t) => (t ? t.split(' ').length : 0)) }
  return {
    info,
    chapterUrl: (path: string) => fileUrl(base, path),
    search: async (q: string) => searchTexts(texts, q),
    close: () => { caches.delete(BOOK_CACHE) },
  }
}

// Case-insensitive search through the chapters, at most 200 hits.
function searchTexts(texts: string[], q: string): SearchHit[] {
  const want = q.trim().toLowerCase()
  const hits: SearchHit[] = []
  if (want.length < 2) return hits
  texts.forEach((text, chapter) => {
    const low = text.toLowerCase()
    if (low.length !== text.length) { if (low.includes(want)) hits.push({ chapter, snippet: q, quote: q, prefix: '' }); return }
    for (let at = low.indexOf(want); at >= 0 && hits.length < 200; at = low.indexOf(want, at + want.length)) {
      hits.push({ chapter, quote: text.slice(at, at + want.length), prefix: text.slice(Math.max(0, at - 40), at), snippet: '…' + text.slice(Math.max(0, at - 60), at + want.length + 60) + '…' })
    }
  })
  return hits
}
