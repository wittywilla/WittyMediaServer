import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import WittyComics from '@wittywilla/wittycomics'
import WittyEbook from '@wittywilla/wittyebook'
import { HostContext, IconClose, IconMore, IconSearch, IconSettings, type ReaderHost } from '@wittywilla/wittyink'
import { exportNotesFile, importFile, importNotesFile, openBook } from './books'
import { books, type BookRec } from './store'

// WittyReader: a library of books, PDFs and comics kept on this device, read
// with the WittyEbook, WittyPDF and WittyComics plugins. Nothing leaves the
// device unless the reader turns on the online dictionary.

const WittyPDF = lazy(() => import('@wittywilla/wittypdf'))
const DICT_KEY = 'wittyreader.dictionary'
const ACCEPT = '.epub,.pdf,.cbz,application/epub+zip,application/pdf'
const VERSION = '0.1.0'
const SORT_KEY = 'wittyreader.sort'

export default function App() {
  const [list, setList] = useState<BookRec[] | null>(null)
  const [open, setOpen] = useState<{ host: ReaderHost; cleanup: () => void } | null>(null)
  const [busy, setBusy] = useState('')
  const [drag, setDrag] = useState(false)
  const [settings, setSettings] = useState(false)
  const [dictionary, setDictionary] = useState(() => localStorage.getItem(DICT_KEY) === '1')
  const [msg, setMsg] = useState('')
  const [q, setQ] = useState('')
  const [kind, setKind] = useState<'all' | 'epub' | 'pdf' | 'cbz'>('all')
  const [sort, setSort] = useState<'recent' | 'title' | 'author' | 'added'>(() => { try { return (localStorage.getItem(SORT_KEY) as any) || 'recent' } catch { return 'recent' } })
  const setSortKept = (v: typeof sort) => { setSort(v); try { localStorage.setItem(SORT_KEY, v) } catch { /* ignore */ } }
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
  const all = list || []
  const kinds = (['epub', 'pdf', 'cbz'] as const).filter((k) => all.some((b) => b.format === k))
  const shown = all.filter((b) => (kind === 'all' || b.format === kind) && (!q || `${b.title} ${b.author || ''}`.toLowerCase().includes(q.toLowerCase())))
    .sort(sort === 'title' ? (a, b) => a.title.localeCompare(b.title) : sort === 'author' ? (a, b) => (a.author || '~').localeCompare(b.author || '~') || a.title.localeCompare(b.title)
      : sort === 'added' ? (a, b) => b.added - a.added : (a, b) => Math.max(b.opened, b.added) - Math.max(a.opened, a.added))
  // The book being read: opened most recently and neither new nor finished.
  const current = all.filter((b) => b.opened && progress(b) > 0 && progress(b) < 100).sort((a, b) => b.opened - a.opened)[0]
  return (
    <div className="wr">
      <header className="wr-top">
        <div className="wr-brand"><img src="icon.svg" alt="" width={30} height={30} /><h1>WittyReader</h1></div>
        {all.length > 0 && (
          <label className="wr-search">
            <IconSearch size={18} />
            <input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search your library" aria-label="Search your library" />
          </label>
        )}
        <span className="grow" />
        <button className="btn primary" onClick={() => picker.current?.click()} title="Add books (Ctrl+O)"><IconAdd /><span className="wr-hide-sm">Add books</span></button>
        <button className="ibtn wr-icon" onClick={() => setSettings(true)} aria-label="Settings" title="Settings"><IconSettings /></button>
        <input ref={picker} type="file" accept={ACCEPT} multiple hidden onChange={(e) => { add(Array.from(e.target.files || [])); e.target.value = '' }} />
        <input ref={notesPicker} type="file" accept=".json,application/json" hidden onChange={async (e) => {
          const f = e.target.files?.[0]; e.target.value = ''
          if (f) try { toast(await importNotesFile(f)) } catch (err: any) { toast(err.message) }
        }} />
      </header>

      <main className="wr-main">
        {list === null ? <div className="spinner" /> : list.length === 0 ? (
          <div className="wr-empty">
            <div className="wr-empty-art" aria-hidden="true"><span /><span /><span /></div>
            <h2>Your reading, on your device</h2>
            <p>Add an <b>EPUB</b> book, a <b>PDF</b> or a <b>CBZ</b> comic, or drop files anywhere on this window. Highlight, take notes and write with a pen; everything stays on this device.</p>
            <button className="btn primary" onClick={() => picker.current?.click()}><IconAdd />Add books</button>
          </div>
        ) : (
          <>
            {current && !q && kind === 'all' && <Hero b={current} read={() => read(current)} />}
            <div className="wr-shelf-head">
              <h2>Library <span>{shown.length}</span></h2>
              {kinds.length > 1 && (
                <div className="wr-chips" role="radiogroup" aria-label="Show">
                  {(['all', ...kinds] as const).map((k) => (
                    <button key={k} role="radio" aria-checked={kind === k} className={kind === k ? 'active' : ''} onClick={() => setKind(k)}>{KIND_NAMES[k]}</button>
                  ))}
                </div>
              )}
              <span className="grow" />
              <select className="select wr-sort" value={sort} onChange={(e) => setSortKept(e.target.value as typeof sort)} aria-label="Sort by">
                <option value="recent">Recently read</option><option value="added">Recently added</option><option value="title">Title</option><option value="author">Author</option>
              </select>
            </div>
            {shown.length === 0 ? <p className="wr-none">Nothing in your library matches “{q}”.</p> : (
              <ul className="wr-grid" aria-label="Library">
                {shown.map((b) => <Card key={b.key} b={b} read={() => read(b)} remove={() => remove(b)} exportNotes={() => exportNotesFile(b)} />)}
              </ul>
            )}
          </>
        )}
      </main>

      {settings && <Settings dictionary={dictionary} setDictionary={setDict} importNotes={() => notesPicker.current?.click()} close={() => setSettings(false)} />}
      {drag && <div className="wr-drop" aria-hidden="true"><div><IconAdd size={40} />Drop to add to your library</div></div>}
      {busy && <div className="wr-busy" role="status"><div className="spinner" />{busy}</div>}
      {toastEl}
    </div>
  )
}

const KIND_NAMES = { all: 'All', epub: 'Books', pdf: 'PDFs', cbz: 'Comics' }
const FORMAT_NAMES = { epub: 'EPUB', pdf: 'PDF', cbz: 'Comic' }
const progress = (b: BookRec) => (b.total ? Math.min(100, Math.round((b.position / b.total) * 100)) : 0)
const IconAdd = ({ size = 20 }: { size?: number }) => <svg viewBox="0 0 24 24" width={size} height={size} fill="currentColor" aria-hidden="true"><path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z" /></svg>

// A cover from the book's stored picture, or its title on a coloured card.
function useCover(b: BookRec) {
  const [cover, setCover] = useState('')
  useEffect(() => {
    if (!b.cover) return
    const u = URL.createObjectURL(b.cover)
    setCover(u)
    return () => URL.revokeObjectURL(u)
  }, [b.cover])
  return cover
}
function Cover({ b }: { b: BookRec }) {
  const cover = useCover(b)
  if (cover) return <img src={cover} alt="" loading="lazy" draggable={false} />
  // A stable colour per book, so a library without covers is still easy to scan.
  let h = 0
  for (const c of b.key.slice(0, 8)) h = (h * 31 + c.charCodeAt(0)) % 360
  return (
    <span className="wr-blank" style={{ background: `linear-gradient(160deg, hsl(${h} 38% 34%), hsl(${(h + 40) % 360} 42% 20%))` }}>
      <b>{b.title}</b>{b.author && <small>{b.author}</small>}
    </span>
  )
}

function Hero({ b, read }: { b: BookRec; read: () => void }) {
  const pct = progress(b)
  return (
    <section className="wr-hero" aria-label="Continue reading">
      <button className="wr-hero-cover" onClick={read} aria-label={`Continue ${b.title}`}><Cover b={b} /></button>
      <div className="wr-hero-text">
        <span className="wr-eyebrow">Continue reading</span>
        <h2>{b.title}</h2>
        {b.author && <p className="wr-hero-author">{b.author}</p>}
        <div className="wr-hero-prog"><span className="wr-bar"><i style={{ width: `${pct}%` }} /></span><span>{pct}%</span></div>
        <button className="btn primary" onClick={read}>Continue</button>
      </div>
    </section>
  )
}

function Card({ b, read, remove, exportNotes }: { b: BookRec; read: () => void; remove: () => void; exportNotes: () => void }) {
  const [menu, setMenu] = useState(false)
  const box = useRef<HTMLLIElement | null>(null)
  // The menu closes on a click anywhere else or on Esc.
  useEffect(() => {
    if (!menu) return
    const off = (e: Event) => { if (!box.current?.contains(e.target as Node)) setMenu(false) }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setMenu(false) }
    document.addEventListener('pointerdown', off)
    document.addEventListener('keydown', esc)
    return () => { document.removeEventListener('pointerdown', off); document.removeEventListener('keydown', esc) }
  }, [menu])
  const pct = progress(b)
  return (
    <li className="wr-card" ref={box}>
      <button className="wr-cover" onClick={read} aria-label={`Read ${b.title}`}>
        <Cover b={b} />
        <span className="wr-badge">{FORMAT_NAMES[b.format]}</span>
        {pct > 0 && pct < 100 && <span className="wr-prog"><i style={{ width: `${pct}%` }} /></span>}
        {pct >= 100 && <span className="wr-done" title="Finished">✓</span>}
      </button>
      <div className="wr-meta">
        <div className="grow">
          <b title={b.title}>{b.title}</b>
          <span className="wr-sub">{b.author || (pct ? '' : 'New')}{b.author && pct > 0 && pct < 100 ? ' · ' : ''}{pct > 0 && pct < 100 ? `${pct}%` : ''}</span>
        </div>
        <button className="ibtn sm wr-more" aria-label={`More for ${b.title}`} aria-haspopup="menu" aria-expanded={menu} onClick={() => setMenu(!menu)}><IconMore /></button>
      </div>
      {menu && (
        <div className="wr-menu" role="menu">
          <button role="menuitem" onClick={() => { setMenu(false); read() }}>{pct > 0 && pct < 100 ? 'Continue reading' : 'Read'}</button>
          <button role="menuitem" onClick={() => { setMenu(false); exportNotes() }}>Export notes</button>
          <hr />
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
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') close() }
    window.addEventListener('keydown', esc)
    return () => window.removeEventListener('keydown', esc)
  }, [close])
  return (
    <div className="wr-modal" onClick={(e) => { if (e.target === e.currentTarget) close() }}>
      <section className="wr-dialog" role="dialog" aria-modal="true" aria-label="Settings">
        <header><h2>Settings</h2><button className="ibtn sm" onClick={close} aria-label="Close"><IconClose /></button></header>
        <div className="wr-set">
          <label className="wr-switch">
            <span><b>Look up words online</b><small>Definitions come from dictionaryapi.dev. Only the word you look up is sent, and only when this is on.</small></span>
            <input type="checkbox" role="switch" checked={dictionary} onChange={(e) => setDictionary(e.target.checked)} />
          </label>
          <div className="wr-set-row">
            <span><b>Notes from another device</b><small>Adds notes exported from WittyReader elsewhere to the matching books here.</small></span>
            <button className="btn sm" onClick={importNotes}>Import notes…</button>
          </div>
          <div className="wr-set-row">
            <span><b>Storage</b><small>Books and notes live in this browser on this device{used ? ` (${used})` : ''}. Clearing the browser's site data removes them, so export notes you want to keep.</small></span>
          </div>
        </div>
        <p className="wr-about">
          WittyReader {VERSION}, developed by WittyWilla · Apache License 2.0 · <a href="https://github.com/wittywilla/WittyReader" target="_blank" rel="noreferrer">Source and licences</a> · Part of the <a href="https://github.com/wittywilla/WittyMediaServer" target="_blank" rel="noreferrer">WittyMediaServer</a> family
        </p>
      </section>
    </div>
  )
}
