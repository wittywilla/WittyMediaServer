import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useHost, type Ann, type BookMeta } from './host'
import { IconClose } from './icons'
import { MARKER_COLORS } from './ink'

export type { Ann } from './host'

// What the EPUB and PDF readers share: the account's annotations with undo,
// highlight colours and their meanings, the notes panel, definitions,
// citations and Markdown export. "Chapter" is a chapter in an EPUB and a
// page in a PDF.

export const NAMED: Record<string, string> = { yellow: '#ffd60a', green: '#34c759', blue: '#0a84ff', pink: '#ff375f' }
export const QUICK = ['yellow', 'green', 'blue', 'pink']
export const hex = (c: string) => NAMED[c] || c || NAMED.yellow
// One name per colour, so labels and filters treat #ffd60a and "yellow" alike.
export const colorName = (c: string) => Object.keys(NAMED).find((k) => NAMED[k] === c.toLowerCase()) || c
// A highlight colour as a see-through fill.
export function fill(c: string, a = 0.38) {
  const h = hex(c)
  return `rgba(${parseInt(h.slice(1, 3), 16)},${parseInt(h.slice(3, 5), 16)},${parseInt(h.slice(5, 7), 16)},${a})`
}

// ---------------------------------------------------------------- annotations

type Op = { kind: 'add' | 'del'; ann: Ann }
const byPos = (a: Ann, b: Ann) => a.pos - b.pos || a.id - b.id

// The book's annotations (from the host), with undo and redo. Deletes show
// at once and come back if saving fails.
export function useAnnotations() {
  const host = useHost()
  const toast = host.toast
  const [anns, setAnns] = useState<Ann[]>([])
  const [, setHistory] = useState(0)
  const undoStack = useRef<Op[]>([])
  const redoStack = useRef<Op[]>([])
  useEffect(() => { host.annotations.list().then(setAnns).catch(() => {}) }, [host.key]) // eslint-disable-line react-hooks/exhaustive-deps

  const create = async (a: Partial<Ann>): Promise<Ann | null> => {
    try {
      const n = await host.annotations.add({ prefix: '', suffix: '', note: '', ...a, id: 0 })
      setAnns((x) => [...x, n].sort(byPos))
      return n
    } catch (e: any) { toast(e.message); return null }
  }
  const update = async (a: Ann, patch: Partial<Ann>) => {
    try { await host.annotations.update(a, patch); setAnns((x) => x.map((y) => (y.id === a.id ? { ...y, ...patch } : y))) } catch (e: any) { toast(e.message) }
  }
  const record = (op: Op) => { undoStack.current = [...undoStack.current.slice(-199), op]; redoStack.current = []; setHistory((h) => h + 1) }
  const remove = async (a: Ann, undoable = true) => {
    setAnns((x) => x.filter((y) => y.id !== a.id))
    try { await host.annotations.remove(a); if (undoable) record({ kind: 'del', ann: a }) } catch (e: any) { toast(e.message); setAnns((x) => [...x, a].sort(byPos)) }
  }
  const add = async (a: Partial<Ann>) => { const n = await create(a); if (n) record({ kind: 'add', ann: n }); return n }
  // A re-created annotation gets a new id; the op keeps the object, so
  // later steps follow it.
  const apply = async (op: Op, reverse: boolean) => {
    if ((op.kind === 'add') !== reverse) { const n = await create(op.ann); if (n) op.ann = n }
    else await remove(op.ann, false)
  }
  const undo = async () => { const op = undoStack.current.pop(); if (!op) return; await apply(op, true); redoStack.current.push(op); setHistory((h) => h + 1) }
  const redo = async () => { const op = redoStack.current.pop(); if (!op) return; await apply(op, false); undoStack.current.push(op); setHistory((h) => h + 1) }
  return { anns, create, add, update, remove, undo, redo, canUndo: undoStack.current.length > 0, canRedo: redoStack.current.length > 0 }
}

// ---------------------------------------------------------------- labels

const LABELS_KEY = 'wms.reader.labels'
// What each highlight colour means to the reader ("Definition", "Question"…),
// the same in every book.
export function useColorLabels() {
  const [labels, setLabels] = useState<Record<string, string>>(() => {
    try {
      const v = JSON.parse(localStorage.getItem(LABELS_KEY) || 'null')
      if (v) return v
      return JSON.parse(localStorage.getItem('wms.ereader') || '{}').labels || {} // from the first e-reader release
    } catch { return {} }
  })
  const set = (color: string, label: string) => setLabels((cur) => {
    const n = { ...cur, [color]: label }
    if (!label) delete n[color]
    try { localStorage.setItem(LABELS_KEY, JSON.stringify(n)) } catch { /* ignore */ }
    return n
  })
  return [labels, set] as const
}

// ---------------------------------------------------------------- pieces

// A side panel (contents, notes, search, settings): a header with its title
// and a close button, a body that scrolls, and a backdrop that closes it.
// The current entry (`.on`) is scrolled into view when it opens.
export function Sheet({ title, actions, onClose, children }: { title: string; actions?: ReactNode; onClose: () => void; children: ReactNode }) {
  const body = useRef<HTMLDivElement | null>(null)
  useEffect(() => { body.current?.querySelector('.on')?.scrollIntoView({ block: 'center' }) }, [title])
  return (
    <>
      <div className="rd-scrim" onClick={onClose} aria-hidden="true" />
      <aside className="rd-panel" role="dialog" aria-label={title}>
        <header className="rd-panel-head">
          <h2>{title}</h2>
          {actions}
          <button className="ibtn sm" onClick={onClose} aria-label="Close" title="Close (Esc)"><IconClose /></button>
        </header>
        <div className="rd-panel-body" ref={body}>{children}</div>
      </aside>
    </>
  )
}

// One setting: its name (and current value) over its control.
export function Field({ label, value, children }: { label: string; value?: ReactNode; children: ReactNode }) {
  return (
    <div className="rd-field" role="group" aria-label={label}>
      <div className="rd-field-head"><span>{label}</span>{value !== undefined && <span className="rd-value">{value}</span>}</div>
      {children}
    </div>
  )
}

// A range input whose filled part shows the value.
export function Range({ value, min, max, step, onChange, label, valueText, dir }: {
  value: number; min: number; max: number; step?: number; onChange: (v: number) => void; label: string; valueText?: string; dir?: 'ltr' | 'rtl'
}) {
  const pct = max > min ? ((value - min) / (max - min)) * 100 : 0
  return (
    <input type="range" className={'rd-range' + (dir === 'rtl' ? ' rtl' : '')} min={min} max={max} step={step} value={value} aria-label={label} aria-valuetext={valueText}
      style={{ ['--pct' as string]: `${pct}%`, direction: dir }} onChange={(e) => onChange(Number(e.target.value))} />
  )
}

export function Palette({ current, labels, onPick }: { current: string; labels: Record<string, string>; onPick: (c: string) => void }) {
  return (
    <div className="ep-palette">
      {[...QUICK, ...MARKER_COLORS.filter((c) => !Object.values(NAMED).includes(c))].map((c) => (
        <button key={c} className={'ep-dot' + (current === c ? ' on' : '')} style={{ background: fill(c, 0.9) }} title={labels[c] || c} aria-label={labels[c] || c} onClick={() => onPick(c)} />
      ))}
      <input type="color" aria-label="Any colour" value={hex(current)} onChange={(e) => onPick(colorName(e.target.value))} />
    </div>
  )
}

// A word's definitions, from the host's dictionary.
export function Define({ word, onClose }: { word: string; onClose: () => void }) {
  const host = useHost()
  const [res, setRes] = useState<{ list: any[] | null; error?: string }>({ list: null })
  useEffect(() => {
    setRes({ list: null })
    if (!host.define) { setRes({ list: [], error: 'No dictionary is available here.' }); return }
    host.define(word).then((list) => setRes({ list })).catch((e) => setRes({ list: [], error: e.message }))
  }, [word]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="ep-define" role="dialog" aria-label={`Definition of ${word}`}>
      <div className="ep-head"><h3>{word}</h3><button className="ibtn sm" onClick={onClose} aria-label="Close"><IconClose /></button></div>
      {res.list === null && <div className="spinner" />}
      {res.list?.length === 0 && <p className="muted">{res.error || 'No definition found.'}</p>}
      {res.list?.slice(0, 2).map((e: any, i: number) => (
        <div key={i}>
          {e.phonetic && <div className="muted">{e.phonetic}</div>}
          {e.meanings?.map((m: any, j: number) => (
            <div key={j} className="ep-sense"><i>{m.partOfSpeech}</i>
              <ol>{m.definitions.map((d: any, k: number) => <li key={k}>{d.definition}{d.example && <div className="muted">“{d.example}”</div>}</li>)}</ol>
            </div>
          ))}
        </div>
      ))}
      <a className="ep-more-link" href={`https://en.wiktionary.org/wiki/${encodeURIComponent(word)}`} target="_blank" rel="noreferrer">More on Wiktionary ↗</a>
    </div>
  )
}

// "Quote" (Author, Title (Year), place) — place is a chapter or "p. 12".
export function cite(it: BookMeta, quote: string, place: string) {
  return `“${quote.trim().replace(/\s+/g, ' ')}” (${[it.author, `${it.title}${it.year ? ` (${it.year})` : ''}`, place].filter(Boolean).join(', ')})`
}

// Notes as Markdown, grouped by chapter or page.
export function exportNotes(it: BookMeta, anns: Ann[], labels: Record<string, string>, heading: (c: number) => string) {
  const lines = [`# ${it.title}`, it.author ? `*${it.author}*` : '', '']
  let last = -1
  for (const a of anns.filter((x) => x.kind !== 'ink')) {
    if (a.chapter !== last) { lines.push(`## ${heading(a.chapter)}`, ''); last = a.chapter }
    if (a.kind === 'bookmark') lines.push(`- Bookmark: ${a.quote}…`)
    else lines.push(`> ${a.quote.replace(/\s+/g, ' ').trim()}`, ...(labels[a.color] ? ['', `— **${labels[a.color]}**`] : []))
    if (a.note) lines.push('', a.note)
    lines.push('')
  }
  const strokes = anns.filter((x) => x.kind === 'ink').length
  if (strokes) lines.push(`*${strokes} handwritten strokes are kept in the reader.*`)
  const url = URL.createObjectURL(new Blob([lines.join('\n')], { type: 'text/markdown' }))
  const link = document.createElement('a')
  link.href = url
  link.download = `${it.title} - notes.md`
  link.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// The notes panel: highlights, notes, bookmarks and handwriting, filtered by
// kind, colour or text, with colour meanings. The reader shows it in a
// Sheet, with Export in the header.
export function NotesPanel({ anns, labels, setLabel, place, open, remove }: {
  anns: Ann[]; labels: Record<string, string>; setLabel: (c: string, v: string) => void
  place: (chapter: number) => string; open: (a: Ann) => void; remove: (a: Ann) => void
}) {
  const [f, setF] = useState({ kind: 'all', color: '', text: '' })
  const used = [...new Set(anns.filter((a) => a.kind === 'highlight').map((a) => a.color))]
  const shown = anns.filter((a) => {
    if (a.kind === 'ink') return false // listed per chapter below
    if (f.kind === 'ink' || (f.kind === 'highlight' && a.kind !== 'highlight') || (f.kind === 'note' && !a.note) || (f.kind === 'bookmark' && a.kind !== 'bookmark')) return false
    if (f.color && a.color !== f.color) return false
    return !f.text || `${a.quote} ${a.note} ${labels[a.color] || ''}`.toLowerCase().includes(f.text.toLowerCase())
  })
  const inkGroups = (f.kind === 'all' || f.kind === 'ink') && !f.color && !f.text ? [...new Set(anns.filter((a) => a.kind === 'ink').map((a) => a.chapter))] : []
  return (
    <>
      <div className="rd-chips" role="radiogroup" aria-label="Show">{([['all', 'All'], ['highlight', 'Highlights'], ['note', 'Notes'], ['bookmark', 'Bookmarks'], ['ink', 'Handwriting']] as const).map(([v, l]) => (
        <button key={v} role="radio" aria-checked={f.kind === v} className={f.kind === v ? 'active' : ''} onClick={() => setF({ ...f, kind: v })}>{l}</button>
      ))}</div>
      <input className="input rd-search" type="search" value={f.text} onChange={(e) => setF({ ...f, text: e.target.value })} placeholder="Filter notes" aria-label="Filter notes" />
      {used.length > 0 && (
        <details className="ep-labels">
          <summary>Colours{f.color ? ' (filtered)' : ''}</summary>
          <p className="muted small">Name what each colour means to you, like “Definition” or “Question”. Pick a colour to show only that one.</p>
          {used.map((c) => (
            <div key={c} className="ep-label">
              <button className={'ep-dot' + (f.color === c ? ' on' : '')} style={{ background: fill(c, 0.9) }} aria-pressed={f.color === c} aria-label={`Only ${labels[c] || c}`} onClick={() => setF({ ...f, color: f.color === c ? '' : c })} />
              <input className="input" value={labels[c] || ''} placeholder="Name this colour" aria-label={`Meaning of ${c}`} onChange={(e) => setLabel(c, e.target.value)} />
            </div>
          ))}
        </details>
      )}
      {anns.length === 0 && (
        <div className="rd-empty">
          <b>No notes yet</b>
          <p>Select text to highlight it, add a note or look a word up. Write with the pen tools or any stylus, and bookmark pages to find them again.</p>
        </div>
      )}
      {anns.length > 0 && shown.length === 0 && !inkGroups.length && <p className="rd-empty">Nothing matches.</p>}
      {shown.map((a) => (
        <div key={a.id} className={'ep-ann kind-' + a.kind}>
          <button onClick={() => open(a)}>
            <span className="ep-kind" style={{ background: a.kind === 'highlight' ? fill(a.color, 0.85) : undefined }} />
            <span className="ep-body">
              <span className="ep-where">{a.kind === 'bookmark' ? 'Bookmark · ' : ''}{place(a.chapter)}{labels[a.color] ? ` · ${labels[a.color]}` : ''}</span>
              <span className="ep-quote">{a.kind === 'bookmark' ? (a.quote || 'Bookmark') + '…' : `“${a.quote.slice(0, 220).trim()}”`}</span>
              {a.note && <span className="ep-note">{a.note}</span>}
            </span>
          </button>
          <button className="ibtn sm ep-del" onClick={() => remove(a)} aria-label="Delete" title="Delete"><IconClose size={16} /></button>
        </div>
      ))}
      {inkGroups.map((c) => {
        const strokes = anns.filter((a) => a.kind === 'ink' && a.chapter === c)
        return (
          <div key={'ink' + c} className="ep-ann kind-ink">
            <button onClick={() => open(strokes[0])}>
              <span className="ep-kind" />
              <span className="ep-body">
                <span className="ep-where">Handwriting · {place(c)}</span>
                <span className="ep-quote">{strokes.length} {strokes.length === 1 ? 'stroke' : 'strokes'}</span>
              </span>
            </button>
          </div>
        )
      })}
    </>
  )
}

// Keeps a pop-up of the given half-width on screen, then nudges it once it
// has been measured (its width depends on the buttons it shows).
const clampX = (x: number, half: number) => Math.min(Math.max(Math.min(half, window.innerWidth / 2), x), window.innerWidth - Math.min(half, window.innerWidth / 2))
function useOnScreen<T extends HTMLElement>(deps: unknown[]) {
  const ref = useRef<T | null>(null)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    el.style.marginLeft = '0px'
    const r = el.getBoundingClientRect()
    const dx = r.left < 8 ? 8 - r.left : r.right > window.innerWidth - 8 ? window.innerWidth - 8 - r.right : 0
    el.style.marginLeft = `${dx}px`
  }, deps) // eslint-disable-line react-hooks/exhaustive-deps
  return ref
}

// The toolbar over selected text, or under it (y2) when there is no room.
export function SelectionBar({ x, y, y2 = y + 24, quote, labels, onHighlight, onNote, onDefine, onCite, onSearch, onRead, onClose }: {
  x: number; y: number; y2?: number; quote: string; labels: Record<string, string>; onHighlight: (c: string) => void; onNote: () => void
  onDefine?: (w: string) => void; onCite: () => void; onSearch: (q: string) => void; onRead?: () => void; onClose: () => void
}) {
  const { toast } = useHost()
  const [more, setMore] = useState(false)
  const then = (f: () => void) => () => { f(); onClose() }
  const ref = useOnScreen<HTMLDivElement>([x, y, quote])
  return (
    <div ref={ref} className="ep-pop" style={{ left: clampX(x, 210), top: y - 64 >= 64 ? y - 64 : Math.min(y2 + 12, window.innerHeight - 140) }} role="toolbar" aria-label="Selected text">
      {QUICK.map((c) => <button key={c} className="ep-dot" style={{ background: fill(c, 0.9) }} onClick={() => onHighlight(c)} title={labels[c] || c} aria-label={`Highlight ${labels[c] || c}`} />)}
      <button className={'ep-dot more' + (more ? ' on' : '')} onClick={() => setMore(!more)} aria-expanded={more} aria-label="More colours" title="More colours" />
      <span className="ep-sep" />
      <button onClick={onNote}>Note</button>
      {onDefine && quote.trim().split(/\s+/).length <= 3 && <button onClick={then(() => onDefine(quote.trim()))}>Define</button>}
      <button onClick={then(() => { navigator.clipboard?.writeText(quote); toast('Copied') })}>Copy</button>
      <button onClick={then(() => { onCite(); toast('Copied with citation') })} title="Copy with author, title and place">Cite</button>
      <button onClick={then(() => onSearch(quote.trim().slice(0, 80)))}>Search</button>
      {onRead && <button onClick={then(onRead)}>Read aloud</button>}
      {more && <div className="ep-pop-more"><Palette current="" labels={labels} onPick={onHighlight} /></div>}
    </div>
  )
}

// Colour, note, citation and removal for one highlight.
export function HighlightEditor({ ann, x, y, labels, onColor, onNote, onCite, onRemove, onDone }: {
  ann: Ann; x: number; y: number; labels: Record<string, string>; onColor: (c: string) => void; onNote: (n: string) => void
  onCite: () => void; onRemove: () => void; onDone: () => void
}) {
  const { toast } = useHost()
  const ref = useOnScreen<HTMLDivElement>([x, y])
  return (
    <div ref={ref} className="ep-pop ep-edit" style={{ left: clampX(x, 182), top: Math.max(64, Math.min(y + 10, window.innerHeight - 300)) }} role="dialog" aria-label="Highlight">
      <Palette current={ann.color} labels={labels} onPick={onColor} />
      {labels[ann.color] && <div className="ep-label-name">{labels[ann.color]}</div>}
      <textarea className="input" rows={4} placeholder="Add a note…" defaultValue={ann.note} autoFocus
        onBlur={(e) => { if (e.target.value !== ann.note) onNote(e.target.value) }} aria-label="Note" />
      <div className="ep-colors">
        <button className="btn sm ghost" onClick={() => { onCite(); toast('Copied with citation') }}>Cite</button>
        <button className="btn sm danger" onClick={onRemove}>Remove</button>
        <span className="grow" />
        <button className="btn sm primary" onClick={onDone}>Done</button>
      </div>
    </div>
  )
}
