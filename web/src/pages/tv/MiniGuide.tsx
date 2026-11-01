import { useEffect, useRef, useState } from 'react'
import { api } from '../../api'
import { announce } from '../../a11y'
import { fmtClock, fmtLeft, type GuideChannel, type GuidePage } from './tvApi'

// The mini guide: a few channels over the picture, with what is on them now and
// next, without leaving what's playing. Up and Down move, OK tunes.

const ROWS = 7

export default function MiniGuide({ channel, onTune, onClose, onFull }: { channel: string; onTune: (key: string) => void; onClose: () => void; onFull: () => void }) {
  const [rows, setRows] = useState<GuideChannel[] | null>(null)
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [at, setAt] = useState(0)
  const [err, setErr] = useState('')
  const list = useRef<HTMLUListElement | null>(null)
  const now = Date.now() / 1000
  const started = useRef(false)

  // Where this channel is in the list, the first time; then whatever window is asked for.
  useEffect(() => {
    let on = true
    const load = (off: number, pick?: (l: GuideChannel[]) => number) =>
      api.get<GuidePage>(`/guide?hours=4&offset=${off}&limit=${ROWS}`).then((g) => {
        if (!on) return
        setRows(g.channels); setTotal(g.total); setOffset(off)
        if (pick) setAt(Math.max(0, pick(g.channels)))
      })
    if (!started.current) {
      started.current = true
      api.get<GuidePage>('/guide?list=1&limit=20000').then((g) => {
        const i = Math.max(0, g.channels.findIndex((c) => c.key === channel))
        const off = Math.max(0, i - Math.floor(ROWS / 2))
        return load(off, (l) => l.findIndex((c) => c.key === channel))
      }).catch((e) => { if (on) setErr(e.message) })
    }
    return () => { on = false }
  }, [channel])

  const page = (off: number, where: number) => {
    const next = Math.max(0, Math.min(Math.max(0, total - ROWS), off))
    api.get<GuidePage>(`/guide?hours=4&offset=${next}&limit=${ROWS}`).then((g) => { setRows(g.channels); setOffset(next); setAt(Math.max(0, Math.min(g.channels.length - 1, where))) }).catch((e) => setErr(e.message))
  }
  const move = (by: number) => {
    if (!rows) return
    const n = at + by
    if (n < 0) { if (offset > 0) page(offset - 1, 0); return }
    if (n >= rows.length) { if (offset + rows.length < total) page(offset + 1, rows.length - 1); return }
    setAt(n)
  }
  useEffect(() => { list.current?.querySelector<HTMLElement>('[aria-current="true"]')?.focus({ preventScroll: true }) }, [at, rows])
  const cur = rows?.[at]
  useEffect(() => { if (cur) announce(`Channel ${cur.number}, ${cur.name}`) }, [cur?.key]) // eslint-disable-line react-hooks/exhaustive-deps

  const onKey = (e: React.KeyboardEvent) => {
    let done = true
    switch (e.key) {
      case 'ArrowUp': case 'ChannelUp': move(-1); break
      case 'ArrowDown': case 'ChannelDown': move(1); break
      case 'PageUp': page(offset - ROWS, at); break
      case 'PageDown': page(offset + ROWS, at); break
      case 'Enter': if (cur && !cur.encrypted) onTune(cur.key); break
      case 'Escape': case 'GoBack': case 'BrowserBack': case 'g': case 'G': case 'ContextMenu': onClose(); break
      default: done = false
    }
    if (done) { e.preventDefault(); e.stopPropagation() }
  }
  return (
    <div className="ctv-mini" role="region" aria-label="Mini guide" onClick={(e) => e.stopPropagation()} onKeyDown={onKey}>
      <div className="ctv-mini-head"><b>Now and next</b><button type="button" className="ctv-btn sm" onClick={onFull}>Full guide</button><button type="button" className="ctv-btn sm" onClick={onClose}>Close</button></div>
      {err && <div className="ctv-err" role="alert">{err}</div>}
      {!rows && !err && <div className="ctv-mini-wait">Loading…</div>}
      <ul ref={list}>
        {rows?.map((c, i) => {
          const on = c.programmes.find((p) => p.start <= now && p.end > now)
          const next = c.programmes.find((p) => p.start > now && p.kind !== 'slate')
          return (
            <li key={c.key}>
              <button type="button" className={'ctv-mini-row' + (i === at ? ' sel' : '') + (c.key === channel ? ' tuned' : '')} tabIndex={i === at ? 0 : -1} aria-current={i === at} disabled={!!c.encrypted}
                onClick={() => { if (i === at) onTune(c.key); else setAt(i) }} onFocus={() => { if (i !== at) setAt(i) }}>
                <span className="ctv-mini-ch"><b>{c.number}</b> {c.name}</span>
                <span className="ctv-mini-now"><b>{on?.title || (c.kind === 'library' ? 'Off air' : 'No guide data')}</b>{on && on.end > now && <small>{fmtLeft(on.end - now)} left</small>}</span>
                <span className="ctv-mini-next">{next && <><time>{fmtClock(next.start)}</time> {next.title}</>}</span>
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
