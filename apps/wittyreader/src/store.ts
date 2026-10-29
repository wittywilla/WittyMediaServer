import type { Ann } from '@wittywilla/wittyink'

// WittyReader keeps everything on the device, in the browser's own database
// (IndexedDB): the library, the book files themselves and the notes. Nothing
// is uploaded anywhere.

export type Format = 'epub' | 'pdf' | 'cbz'
export type BookRec = {
  key: string // SHA-256 of the file, so the same book is found again
  name: string; title: string; author?: string; year?: number | null; format: Format; reading?: string
  size: number; added: number; opened: number
  position: number; total: number
  cover?: Blob
}
type NoteRec = Ann & { book: string }

let dbp: Promise<IDBDatabase> | null = null
function db(): Promise<IDBDatabase> {
  dbp ??= new Promise((resolve, reject) => {
    const r = indexedDB.open('wittyreader', 1)
    r.onupgradeneeded = () => {
      const d = r.result
      d.createObjectStore('books', { keyPath: 'key' })
      d.createObjectStore('files')
      d.createObjectStore('notes', { keyPath: 'id', autoIncrement: true }).createIndex('book', 'book')
    }
    r.onsuccess = () => resolve(r.result)
    r.onerror = () => reject(r.error)
  })
  return dbp
}

function run<T>(stores: string[], mode: IDBTransactionMode, fn: (t: IDBTransaction) => IDBRequest<T> | void): Promise<T> {
  return db().then((d) => new Promise<T>((resolve, reject) => {
    const t = d.transaction(stores, mode)
    const req = fn(t)
    t.oncomplete = () => resolve(req ? req.result : (undefined as T))
    t.onerror = () => reject(t.error)
    t.onabort = () => reject(t.error || new Error('Storage is full or unavailable'))
  }))
}

export const books = {
  all: () => run<BookRec[]>(['books'], 'readonly', (t) => t.objectStore('books').getAll()),
  get: (key: string) => run<BookRec | undefined>(['books'], 'readonly', (t) => t.objectStore('books').get(key)),
  put: (b: BookRec) => run(['books'], 'readwrite', (t) => { t.objectStore('books').put(b) }),
  async patch(key: string, patch: Partial<BookRec>) {
    const b = await books.get(key)
    if (b) await books.put({ ...b, ...patch })
  },
  // A book and everything kept for it.
  remove: (key: string) => run(['books', 'files', 'notes'], 'readwrite', (t) => {
    t.objectStore('books').delete(key)
    t.objectStore('files').delete(key)
    const idx = t.objectStore('notes').index('book').openKeyCursor(IDBKeyRange.only(key))
    idx.onsuccess = () => { const c = idx.result; if (c) { t.objectStore('notes').delete(c.primaryKey); c.continue() } }
  }),
}

export const files = {
  get: (key: string) => run<Blob | undefined>(['files'], 'readonly', (t) => t.objectStore('files').get(key)),
  put: (key: string, blob: Blob) => run(['files'], 'readwrite', (t) => { t.objectStore('files').put(blob, key) }),
}

const strip = ({ book: _book, ...a }: NoteRec): Ann => a
export const notes = {
  list: (book: string) => run<NoteRec[]>(['notes'], 'readonly', (t) => t.objectStore('notes').index('book').getAll(IDBKeyRange.only(book)))
    .then((l) => l.map(strip).sort((a, b) => a.pos - b.pos || a.id - b.id)),
  async add(book: string, a: Partial<Ann>): Promise<Ann> {
    const rec = { kind: 'highlight', chapter: 0, pos: 0, quote: '', prefix: '', suffix: '', color: '', note: '', ...a, book, created: Math.floor(Date.now() / 1000) } as Partial<NoteRec>
    delete rec.id // a new id from the database
    const id = await run<IDBValidKey>(['notes'], 'readwrite', (t) => t.objectStore('notes').add(rec))
    return strip({ ...(rec as NoteRec), id: id as number })
  },
  update: (book: string, a: Ann, patch: Partial<Ann>) => run<void>(['notes'], 'readwrite', (t) => { t.objectStore('notes').put({ ...a, ...patch, id: a.id, book }) }),
  remove: (id: number) => run<void>(['notes'], 'readwrite', (t) => { t.objectStore('notes').delete(id) }),
}

// A stable id for a file: the start of its SHA-256 in hex.
export async function fileKey(buf: ArrayBuffer): Promise<string> {
  const h = new Uint8Array(await crypto.subtle.digest('SHA-256', buf))
  return Array.from(h.slice(0, 12), (b) => b.toString(16).padStart(2, '0')).join('')
}
