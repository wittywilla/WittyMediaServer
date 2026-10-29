import { strFromU8, unzipSync } from 'fflate'
import type { Ann, Definition, ReaderHost } from '@wittywilla/wittyink'
import { readEpub, serveEpub } from './epub'
import { books, fileKey, files, notes, type BookRec } from './store'

// Adding books to the library and opening them with a host that keeps the
// place and notes on this device.

const IMG = /\.(jpe?g|png|gif|webp|avif|bmp)$/i
const IMG_TYPE: Record<string, string> = { jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png', gif: 'image/gif', webp: 'image/webp', avif: 'image/avif', bmp: 'image/bmp' }
const natural = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })
const baseName = (name: string) => name.replace(/\.[^.]+$/, '').replace(/[_]+/g, ' ').trim()

// A comic's pages in reading order (images, without system files).
function comicPages(all: Record<string, Uint8Array>) {
  return Object.keys(all).filter((n) => IMG.test(n) && !n.startsWith('__MACOSX/') && !n.split('/').pop()!.startsWith('.')).sort(natural.compare)
}
const imageBlob = (name: string, bytes: Uint8Array) => new Blob([bytes as BlobPart], { type: IMG_TYPE[name.split('.').pop()!.toLowerCase()] || 'image/jpeg' })

// A small cover picture for the library (300 px wide), so the grid stays light.
async function thumb(src: Blob | HTMLCanvasElement): Promise<Blob | undefined> {
  try {
    const img = src instanceof Blob ? await createImageBitmap(src) : src
    const w = 300, h = Math.round((img.height / img.width) * w)
    const c = document.createElement('canvas')
    c.width = w; c.height = h
    c.getContext('2d')!.drawImage(img, 0, 0, w, h)
    return await new Promise<Blob | undefined>((done) => c.toBlob((b) => done(b || undefined), 'image/jpeg', 0.82))
  } catch { return undefined }
}

async function pdfjs() {
  const [lib, worker] = await Promise.all([import('pdfjs-dist/legacy/build/pdf.mjs'), import('pdfjs-dist/legacy/build/pdf.worker.min.mjs?url')])
  lib.GlobalWorkerOptions.workerSrc = worker.default
  return lib
}

// Adds a file to the library (or finds it there already) and returns it.
export async function importFile(file: File): Promise<BookRec> {
  const buf = await file.arrayBuffer()
  const key = await fileKey(buf)
  const known = await books.get(key)
  if (known) return known
  const bytes = new Uint8Array(buf)
  const now = Date.now()
  const rec: BookRec = { key, name: file.name, title: baseName(file.name), format: 'epub', size: file.size, added: now, opened: 0, position: 0, total: 0 }

  if (bytes[0] === 0x25 && bytes[1] === 0x50 && bytes[2] === 0x44 && bytes[3] === 0x46) { // %PDF
    rec.format = 'pdf'
    const lib = await pdfjs()
    const doc = await lib.getDocument({ data: bytes.slice(), isEvalSupported: false, enableXfa: false }).promise
    try {
      const info: any = (await doc.getMetadata()).info || {}
      if (info.Title?.trim()) rec.title = info.Title.trim()
      if (info.Author?.trim()) rec.author = info.Author.trim()
      const y = parseInt(String(info.CreationDate || '').replace(/^D:/, '').slice(0, 4), 10)
      if (y > 0) rec.year = y
      rec.total = doc.numPages
      const page = await doc.getPage(1)
      const vp = page.getViewport({ scale: 300 / page.getViewport({ scale: 1 }).width })
      const c = document.createElement('canvas')
      c.width = vp.width; c.height = vp.height
      await page.render({ canvasContext: c.getContext('2d')!, viewport: vp }).promise
      rec.cover = await thumb(c)
    } finally { doc.destroy() }
  } else if (bytes[0] === 0x50 && bytes[1] === 0x4b) { // a ZIP: an EPUB or a comic
    const head = unzipSync(bytes, { filter: (f) => f.name === 'mimetype' || f.name === 'META-INF/container.xml' })
    if (head['META-INF/container.xml'] || (head.mimetype && strFromU8(head.mimetype).trim() === 'application/epub+zip')) {
      const book = readEpub(bytes)
      rec.format = 'epub'
      if (book.title) rec.title = book.title
      rec.author = book.author
      rec.year = book.year
      rec.total = book.spine.length
      if (book.cover) rec.cover = await thumb(imageBlob(book.cover, book.files[book.cover]))
    } else {
      const all = unzipSync(bytes)
      const pages = comicPages(all)
      if (!pages.length) throw new Error(`${file.name} has no pages to read.`)
      rec.format = 'cbz'
      rec.total = pages.length
      Object.assign(rec, comicInfo(all))
      rec.cover = await thumb(imageBlob(pages[0], all[pages[0]]))
    }
  } else {
    throw new Error(`${file.name} is not a book WittyReader can open (EPUB, PDF or CBZ).`)
  }
  await files.put(key, file)
  await books.put(rec)
  return rec
}

// Title, writer, year and reading direction from a comic's ComicInfo.xml.
function comicInfo(all: Record<string, Uint8Array>): Partial<BookRec> {
  const name = Object.keys(all).find((n) => n.split('/').pop()!.toLowerCase() === 'comicinfo.xml')
  if (!name) return {}
  const d = new DOMParser().parseFromString(strFromU8(all[name]), 'application/xml')
  const get = (t: string) => d.getElementsByTagName(t)[0]?.textContent?.trim() || ''
  const out: Partial<BookRec> = {}
  const series = get('Series'), num = get('Number'), title = get('Title')
  if (series) out.title = num ? `${series} ${num}` : series
  else if (title) out.title = title
  if (get('Writer')) out.author = get('Writer')
  if (parseInt(get('Year'), 10) > 0) out.year = parseInt(get('Year'), 10)
  if (get('Manga') === 'YesAndRightToLeft') out.reading = 'rtl'
  return out
}

// Definitions from the free dictionary API, only when the reader turned it on.
export async function lookUp(word: string): Promise<Definition[]> {
  const r = await fetch('https://api.dictionaryapi.dev/api/v2/entries/en/' + encodeURIComponent(word.toLowerCase()))
  if (r.status === 404) return []
  if (!r.ok) throw new Error('The dictionary did not answer')
  const list: Definition[] = await r.json()
  for (const e of list) for (const m of e.meanings || []) m.definitions = m.definitions.slice(0, 3)
  return list
}

// Opens a book from the library. The returned cleanup frees what it used.
export async function openBook(rec: BookRec, opts: { toast: (m: string) => void; close: () => void; dictionary: boolean }): Promise<{ host: ReaderHost; cleanup: () => void }> {
  const blob = await files.get(rec.key)
  if (!blob) throw new Error('This book is no longer stored on this device. Open the file again.')
  const bytes = new Uint8Array(await blob.arrayBuffer())
  books.patch(rec.key, { opened: Date.now() })
  const host: ReaderHost = {
    key: rec.key,
    book: { title: rec.title, author: rec.author, year: rec.year },
    format: rec.format,
    reading: rec.reading,
    start: rec.position,
    saveProgress: (position, total) => { books.patch(rec.key, { position, total }) },
    annotations: {
      list: () => notes.list(rec.key),
      add: (a) => notes.add(rec.key, a),
      update: (a, patch) => notes.update(rec.key, a, patch),
      remove: (a) => notes.remove(a.id),
    },
    define: opts.dictionary ? lookUp : undefined,
    toast: opts.toast,
    close: opts.close,
  }
  if (rec.format === 'pdf') {
    host.pdf = { data: bytes }
    return { host, cleanup: () => {} }
  }
  if (rec.format === 'cbz') {
    const all = unzipSync(bytes)
    const urls = comicPages(all).map((p) => URL.createObjectURL(imageBlob(p, all[p])))
    host.comic = { pages: urls.length, pageUrl: (n) => urls[n] }
    return { host, cleanup: () => urls.forEach((u) => URL.revokeObjectURL(u)) }
  }
  const served = await serveEpub(rec.key, readEpub(bytes))
  host.epub = served
  return { host, cleanup: served.close }
}

// ---------------------------------------------------------------- notes backup

type Backup = { app: 'wittyreader'; version: 1; book: { key: string; title: string; author?: string }; annotations: Ann[] }

// A book's notes as a file, to keep or to move to another device.
export async function exportNotesFile(rec: BookRec) {
  const data: Backup = { app: 'wittyreader', version: 1, book: { key: rec.key, title: rec.title, author: rec.author }, annotations: await notes.list(rec.key) }
  const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 1)], { type: 'application/json' }))
  const a = document.createElement('a')
  a.href = url
  a.download = `${rec.title} - notes.json`
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// Adds notes from a backup to the same book (matched by its file, else its
// title). Notes already there are not added twice.
export async function importNotesFile(file: File): Promise<string> {
  let data: Backup
  try { data = JSON.parse(await file.text()) } catch { throw new Error('This is not a WittyReader notes file.') }
  if (data?.app !== 'wittyreader' || !Array.isArray(data.annotations)) throw new Error('This is not a WittyReader notes file.')
  const all = await books.all()
  const rec = all.find((b) => b.key === data.book?.key) || all.find((b) => b.title === data.book?.title)
  if (!rec) throw new Error(`Add “${data.book?.title || 'the book'}” to the library first, then import its notes.`)
  const have = new Set((await notes.list(rec.key)).map((a) => `${a.kind}|${a.chapter}|${a.quote}|${a.data || ''}`))
  let added = 0
  for (const a of data.annotations.slice(0, 20000)) {
    if (!a || !['bookmark', 'highlight', 'ink'].includes(a.kind) || have.has(`${a.kind}|${a.chapter}|${a.quote}|${a.data || ''}`)) continue
    await notes.add(rec.key, a)
    added++
  }
  return `Added ${added} ${added === 1 ? 'note' : 'notes'} to “${rec.title}”.`
}
