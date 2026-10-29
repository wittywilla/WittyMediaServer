import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import WittyComics from '@wittywilla/wittycomics'
import WittyEbook from '@wittywilla/wittyebook'
import { HostContext, type ReaderHost } from '@wittywilla/wittyink'
import { exportNotesFile, importFile, importNotesFile, openBook } from './books'
import { books, type BookRec } from './store'

// WittyReader: a library of books, PDFs and comics kept on this device, read
// with the WittyEbook, WittyPDF and WittyComics plugins. Nothing leaves the
// device unless the reader turns on the online dictionary.

const WittyPDF = lazy(() => import('@wittywilla/wittypdf'))
const DICT_KEY = 'wittyreader.dictionary'
const ACCEPT = '.epub,.pdf,.cbz,application/epub+zip,application/pdf'

export default function App() {
  const [list, setList] = useState<BookRec[] | null>(null)
  const [open, setOpen] = useState<{ host: ReaderHost; cleanup: () => void } | null>(null)
  const [busy, setBusy] = useState('')
  const [drag, setDrag] = useState(false)
  const [settings, setSettings] = useState(false)
  const [dictionary, setDictionary] = useState(() => localStorage.getItem(DICT_KEY) === '1')
  const [msg, setMsg] = useState('')
  const picker = useRef<HTMLInputElement | null>(null)
  const notesPicker = useRef<HTMLInputElement | null>(null)
  const msgTimer = useRef<any>(null)
  const toast = useCallback((m: string) => { setMsg(m); clearTimeout(msgTimer.current); msgTimer.current = setTimeout(() => setMsg(''), 3500) }, [])
  const refresh = useCallback(() => books.all().then((l) => setList(l.sort((a, b) => Math.max(b.opened, b.added) - Math.max(a.opened, a.added)))).catch(() => setList([])), [])
  useEffect(() => { refresh() }, [refresh])

  const closeBook = useCallback(() => {
    setOpen((cur) => { cur?.cleanup(); return null })
    refresh()
  }, [refresh])
  // The browser's Back button (and Android's) closes the book.
  useEffect(() => {
    const pop = () => closeBook()
    window.addEventListener('popstate', pop)
    return () => window.removeEventListener('popstate', pop)
  }, [closeBook])

  const read = async (rec: BookRec) => {
    setBusy(`Opening ${rec.title}…`)
    try {
      const o = await openBook(rec, { toast, dictionary, close: () => history.back() })
      history.pushState({ book: rec.key }, '')
      setOpen(o)
    } catch (e: any) { toast(e.message) } finally { setBusy('') }
  }

  // New files: added to the library; a single file opens straight away.
  const add = async (fileList: File[]) => {
    if (!fileList.length) return
    navigator.storage?.persist?.().catch(() => {}) // ask the browser not to clear the library when space runs low
    let last: BookRec | null = null
    for (const f of fileList) {
      setBusy(`Adding ${f.name}…`)
      try { last = await importFile(f) } catch (e: any) { toast(e.message) }
    }
    setBusy('')
    await refresh()
    if (last && fileList.length === 1) read(last)
  }

  // Files dropped on the window, or opened with WittyReader from the system
  // once it is installed.
  useEffect(() => {
    const over = (e: DragEvent) => { if (e.dataTransfer?.types.includes('Files')) { e.preventDefault(); setDrag(true) } }
    const leave = (e: DragEvent) => { if (!e.relatedTarget) setDrag(false) }
    const drop = (e: DragEvent) => { e.preventDefault(); setDrag(false); if (!open) add(Array.from(e.dataTransfer?.files || [])) }
    window.addEventListener('dragover', over)
    window.addEventListener('dragleave', leave)
    window.addEventListener('drop', drop)
    const lq = (window as any).launchQueue
    lq?.setConsumer?.(async (p: { files: FileSystemFileHandle[] }) => add(await Promise.all(p.files.map((h) => h.getFile()))))
    return () => { window.removeEventListener('dragover', over); window.removeEventListener('dragleave', leave); window.removeEventListener('drop', drop) }
  })
  useEffect(() => {
    const key = (e: KeyboardEvent) => { if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'o' && !open) { e.preventDefault(); picker.current?.click() } }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [open])

  const setDict = (on: boolean) => { setDictionary(on); try { localStorage.setItem(DICT_KEY, on ? '1' : '0') } catch { /* ignore */ } }
  const remove = async (rec: BookRec) => {
    if (!confirm(`Remove “${rec.title}” and its notes from this device?`)) return
    await books.remove(rec.key)
    refresh()
  }

  const toastEl = msg && <div className="wr-toast" role="status">{msg}</div>
  if (open) {
    const f = open.host.format
    return (
      <HostContext.Provider value={open.host}>
        {f === 'pdf' ? <Suspense fallback={<div className="reader center"><div className="spinner" /></div>}><WittyPDF /></Suspense> : f === 'cbz' ? <WittyComics /> : <WittyEbook />}
        {toastEl}
      </HostContext.Provider>
    )
  }
  return (
    <div className="wr">
      <header className="wr-top">
        <img src="icon.svg" alt="" width={32} height={32} />
        <h1>WittyReader</h1>
        <span className="grow" />
        <button className="btn primary" onClick={() => picker.current?.click()} title="Open (Ctrl+O)">Open…</button>
        <button className="btn ghost" onClick={() => setSettings(!settings)} aria-expanded={settings}>Settings</button>
        <input ref={picker} type="file" accept={ACCEPT} multiple hidden onChange={(e) => { add(Array.from(e.target.files || [])); e.target.value = '' }} />
        <input ref={notesPicker} type="file" accept=".json,application/json" hidden onChange={async (e) => {
          const f = e.target.files?.[0]; e.target.value = ''
          if (f) try { toast(await importNotesFile(f)) } catch (err: any) { toast(err.message) }
        }} />
      </header>

      {settings && <Settings dictionary={dictionary} setDictionary={setDict} importNotes={() => notesPicker.current?.click()} close={() => setSettings(false)} />}

      <main className="wr-main">
        {list === null ? <div className="spinner" /> : list.length === 0 ? (
          <div className="wr-empty">
            <h2>Your reading, on your device</h2>
            <p>Open or drop an <b>EPUB</b> book, a <b>PDF</b> or a <b>CBZ</b> comic. Highlight, take notes and write with a pen; everything is kept on this device and never uploaded.</p>
            <button className="btn primary" onClick={() => picker.current?.click()}>Open a book</button>
          </div>
        ) : (
          <ul className="wr-grid" aria-label="Library">
            {list.map((b) => <Card key={b.key} b={b} read={() => read(b)} remove={() => remove(b)} exportNotes={() => exportNotesFile(b)} />)}
          </ul>
        )}
      </main>

      {drag && <div className="wr-drop" aria-hidden="true"><div>Drop to add to your library</div></div>}
      {busy && <div className="wr-busy" role="status"><div className="spinner" />{busy}</div>}
      {toastEl}
    </div>
  )
}

function Card({ b, read, remove, exportNotes }: { b: BookRec; read: () => void; remove: () => void; exportNotes: () => void }) {
  const [cover, setCover] = useState('')
  const [menu, setMenu] = useState(false)
  useEffect(() => {
    if (!b.cover) return
    const u = URL.createObjectURL(b.cover)
    setCover(u)
    return () => URL.revokeObjectURL(u)
  }, [b.cover])
  const pct = b.total ? Math.min(100, Math.round((b.position / b.total) * 100)) : 0
  return (
    <li className="wr-card">
      <button className="wr-cover" onClick={read} aria-label={`Read ${b.title}`}>
        {cover ? <img src={cover} alt="" loading="lazy" /> : <span className="wr-blank">{b.title}</span>}
        <span className="wr-badge">{b.format === 'cbz' ? 'Comic' : b.format.toUpperCase()}</span>
        {pct > 0 && <span className="wr-prog" style={{ width: `${pct}%` }} />}
      </button>
      <div className="wr-meta">
        <div className="grow"><b title={b.title}>{b.title}</b>{b.author && <span className="muted">{b.author}</span>}</div>
        <button className="ibtn sm" aria-label={`More for ${b.title}`} aria-expanded={menu} onClick={() => setMenu(!menu)}>⋯</button>
      </div>
      {menu && (
        <div className="wr-menu" role="menu" onMouseLeave={() => setMenu(false)}>
          <button role="menuitem" onClick={() => { setMenu(false); read() }}>{pct > 0 ? 'Continue reading' : 'Read'}</button>
          <button role="menuitem" onClick={() => { setMenu(false); exportNotes() }}>Export notes</button>
          <button role="menuitem" className="danger" onClick={() => { setMenu(false); remove() }}>Remove from this device</button>
        </div>
      )}
    </li>
  )
}

function Settings({ dictionary, setDictionary, importNotes, close }: { dictionary: boolean; setDictionary: (on: boolean) => void; importNotes: () => void; close: () => void }) {
  const [used, setUsed] = useState('')
  useEffect(() => {
    navigator.storage?.estimate?.().then((e) => setUsed(`${((e.usage || 0) / 1048576).toFixed(1)} MB`)).catch(() => {})
  }, [])
  return (
    <section className="wr-settings" aria-label="Settings">
      <label className="check-row"><input type="checkbox" checked={dictionary} onChange={(e) => setDictionary(e.target.checked)} /> Look up words online</label>
      <p className="muted small">Definitions come from dictionaryapi.dev. Only the word you look up is sent, and only when this is on.</p>
      <div className="wr-row">
        <button className="btn sm" onClick={importNotes}>Import notes…</button>
        <span className="muted small">Adds notes exported from WittyReader on another device.</span>
      </div>
      {used && <p className="muted small">Books and notes on this device: {used}.</p>}
      <p className="muted small">WittyReader keeps your books and notes in this browser on this device. Clearing the browser's site data removes them, so export notes you want to keep.</p>
      <button className="btn sm ghost" onClick={close}>Close</button>
    </section>
  )
}
