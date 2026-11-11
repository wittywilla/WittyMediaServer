// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useMemo, useRef, useState, type ReactNode } from 'react'
import { artPreviewUrl } from '../api'
import { artHue } from './common'
import { IconCamera, IconCheck, IconDownload, IconEdit as IconText, IconImage } from '../icons'
import { announce } from '../a11y'
import { Button, Dialog, Tabs } from '../ui'
import LegalNote from './LegalNote'
import './artpicker.css'

// The artwork picker: one window, over whatever opened it, for choosing a title's
// poster, backdrop, banner or logo. A grid of what the sources have, a large preview of
// the one highlighted at the shape it'll be shown, and three ways to bring your own: a
// file, an address, or a frame from the video. Nothing changes until Apply, which closes
// it; focus goes back to the button that opened it (a11y.tsx).
//
// Keyboard and remote: arrows move in the grid, Enter previews, Enter again applies,
// Escape or Back cancels. On a phone it fills the screen, two to a row.

export type PickerKind = { kind: string; label: string; shape: 'tall' | 'wide' | 'strip' | 'logo' }
// own: brought in by hand, and named for what it is ("Your file") rather than where it's from.
export type PickerChoice = { url: string; sourceLabel: string; own?: boolean }
type Found = { candidates: PickerChoice[]; warnings: string[] }

export type MakeOptions = { title: string; line: string; hue: number | null; frame: boolean }
export type Maker = { title: string; line: string; hasVideo: boolean; make: (o: MakeOptions) => Promise<{ url: string }> }

export const ART_KINDS: PickerKind[] = [
  { kind: 'poster', label: 'Poster', shape: 'tall' },
  { kind: 'backdrop', label: 'Backdrop', shape: 'wide' },
  { kind: 'banner', label: 'Banner', shape: 'strip' },
  { kind: 'logo', label: 'Title logo', shape: 'logo' },
]

// The names of the sources the older artwork route reports by key.
export const ART_SOURCE: Record<string, string> = { local: 'In the folder', tmdb: 'TMDB', tvdb: 'TheTVDB', tvmaze: 'TVmaze', anilist: 'AniList', current: 'In use' }

const ADDRESS = /^(https?:\/\/\S+|\/\S.*|[a-zA-Z]:\\.+|\\\\.+)$/

export default function ArtworkPicker({ kinds, start, current, load, apply, stage, frame, onClose, title = 'Change artwork', note, maker }: {
  kinds: PickerKind[]
  start?: string
  // The picture each kind has now.
  current: (kind: string) => string
  load: (kind: string) => Promise<Found>
  // Every kind with a new picture, at once: one change, one undo.
  apply: (changes: Record<string, string>) => Promise<void>
  // Keeps an uploaded file on the server, checked, without using it yet: its address.
  stage?: (kind: string, f: File) => Promise<string>
  // A picture from the video, kept the same way.
  frame?: () => Promise<string>
  onClose: () => void
  title?: string
  note?: ReactNode
  // Makes a poster from words: for the Poster tab.
  maker?: Maker
}) {
  const [kind, setKind] = useState(() => kinds.find((k) => k.kind === start)?.kind || kinds[0]?.kind || 'poster')
  const [found, setFound] = useState<Record<string, Found | undefined>>({})
  // What's been brought in by hand, per kind, shown first.
  const [own, setOwn] = useState<Record<string, PickerChoice[]>>({})
  const [chosen, setChosen] = useState<Record<string, string>>({})
  const [sizes, setSizes] = useState<Record<string, [number, number]>>({})
  const [broken, setBroken] = useState<Record<string, true>>({})
  const [focusAt, setFocusAt] = useState(-1)
  const [address, setAddress] = useState('')
  const [busy, setBusy] = useState<'' | 'apply' | 'upload' | 'frame' | 'make'>('')
  const [err, setErr] = useState('')
  const grid = useRef<HTMLDivElement | null>(null)
  const file = useRef<HTMLInputElement | null>(null)
  const ids = useId()
  const info = kinds.find((k) => k.kind === kind) || kinds[0]

  useEffect(() => {
    if (found[kind]) return
    let live = true
    load(kind)
      .then((r) => { if (live) setFound((f) => ({ ...f, [kind]: r })) })
      .catch((e) => { if (live) setFound((f) => ({ ...f, [kind]: { candidates: [], warnings: [e.message] } })) })
    return () => { live = false }
  }, [kind]) // eslint-disable-line react-hooks/exhaustive-deps

  const now = current(kind)
  const list = useMemo(() => {
    const seen = new Set<string>()
    const out: PickerChoice[] = []
    for (const c of [...(own[kind] || []), ...(found[kind]?.candidates || [])]) {
      if (!c.url || seen.has(c.url) || broken[c.url]) continue
      seen.add(c.url)
      out.push(c)
    }
    return out
  }, [own, found, kind, broken])
  const pick = chosen[kind] ?? now
  const shown = list.find((c) => c.url === pick)
  const changes = Object.fromEntries(Object.entries(chosen).filter(([k, u]) => u !== current(k)))
  const changed = Object.keys(changes).length

  // Focus stays on the grid's highlighted one when the list changes under it.
  const at = focusAt >= 0 && focusAt < list.length ? focusAt : Math.max(0, list.findIndex((c) => c.url === pick))

  const choose = (u: string, say = true) => {
    setChosen((c) => ({ ...c, [kind]: u }))
    setErr('')
    if (say) {
      const c = list.find((x) => x.url === u)
      announce(`Previewing ${info.label.toLowerCase()}${c?.sourceLabel ? (c.own ? ', ' + c.sourceLabel.toLowerCase() : ' from ' + c.sourceLabel) : ''}`)
    }
  }
  const bring = (c: PickerChoice) => {
    setOwn((o) => ({ ...o, [kind]: [c, ...(o[kind] || []).filter((x) => x.url !== c.url)] }))
    setChosen((x) => ({ ...x, [kind]: c.url }))
    setFocusAt(0)
    setErr('')
    announce(`Previewing ${c.sourceLabel.toLowerCase()}`)
  }

  const doApply = async (also?: Record<string, string>) => {
    const all = Object.fromEntries(Object.entries({ ...changes, ...also }).filter(([k, u]) => u !== current(k)))
    if (!Object.keys(all).length) return
    setBusy('apply'); setErr('')
    try { await apply(all); onClose() } catch (e: any) { setErr(e.message); setBusy('') }
  }
  const doUpload = async (f?: File) => {
    if (!f || !stage) return
    setBusy('upload'); setErr('')
    try { bring({ url: await stage(kind, f), sourceLabel: 'Your file', own: true }) } catch (e: any) { setErr(e.message) } finally { setBusy('') }
  }
  const doFrame = async () => {
    if (!frame) return
    setBusy('frame'); setErr('')
    try { bring({ url: await frame(), sourceLabel: 'A frame from the video', own: true }) } catch (e: any) { setErr(e.message) } finally { setBusy('') }
  }
  const [making, setMaking] = useState(false)
  const doMake = async (o: MakeOptions) => {
    if (!maker) return
    setBusy('make'); setErr('')
    try { bring({ url: (await maker.make(o)).url, sourceLabel: 'Made from its title', own: true }) } catch (e: any) { setErr(e.message) } finally { setBusy('') }
  }
  const doAddress = () => {
    const u = address.trim()
    if (!ADDRESS.test(u)) { setErr("That isn't a picture's address. It starts with https://, or is a file in one of your libraries."); return }
    bring({ url: u, sourceLabel: 'From an address', own: true })
    setAddress('')
  }

  // Arrows along and between rows, Home and End. At an edge the key is left alone, so a
  // remote can carry on to the tabs or the buttons.
  const keys = (e: React.KeyboardEvent) => {
    const btns = [...(grid.current?.querySelectorAll<HTMLButtonElement>('.ap-choice') || [])]
    const i = btns.indexOf(document.activeElement as HTMLButtonElement)
    if (i < 0) return
    if (e.key === 'Enter' && list[i]?.url === pick && !e.repeat) { e.preventDefault(); if (changed) void doApply(); return }
    const top = btns[0].offsetTop
    const cols = Math.max(1, btns.findIndex((b) => b.offsetTop !== top) === -1 ? btns.length : btns.findIndex((b) => b.offsetTop !== top))
    const to = ({ ArrowRight: i + 1, ArrowLeft: i - 1, ArrowDown: i + cols, ArrowUp: i - cols, Home: 0, End: btns.length - 1 } as Record<string, number>)[e.key]
    if (to === undefined) return
    if (e.key === 'ArrowRight' && (i + 1) % cols === 0) return
    if (e.key === 'ArrowLeft' && i % cols === 0) return
    if (to < 0 || to >= btns.length) return
    e.preventDefault()
    setFocusAt(to)
    btns[to].focus()
  }

  const sizeOf = (u: string) => sizes[u] ? `${sizes[u][0]} × ${sizes[u][1]}` : ''
  const nameOf = (c: PickerChoice, i: number) => [
    `${info.label} ${i + 1} of ${list.length}`,
    c.own ? c.sourceLabel.toLowerCase() : c.sourceLabel ? `from ${c.sourceLabel}` : '',
    sizes[c.url] ? `${sizes[c.url][0]} by ${sizes[c.url][1]}` : '',
    c.url === now ? 'in use now' : '',
  ].filter(Boolean).join(', ')
  const warnings = found[kind]?.warnings || []
  const loading = !found[kind]

  // Once the pictures are in, the keyboard starts on the one in use, unless it's already
  // somewhere of its own choosing in the picker.
  const started = useRef<Record<string, boolean>>({})
  useEffect(() => {
    if (loading || started.current[kind] || !list.length) return
    started.current[kind] = true
    const a = document.activeElement
    if (a?.closest('.art-picker') && !a.matches('.ui-dialog-head button')) return
    grid.current?.querySelector<HTMLElement>('.ap-choice[tabindex="0"]')?.focus({ preventScroll: true })
  }, [loading, kind, list.length])

  return (
    <Dialog title={title} onClose={onClose} size="wide" className="art-picker"
      actions={<>
        {changed > 1 && <span className="ap-count muted" aria-live="polite">{changed} pictures to apply</span>}
        <Button variant="ghost" onClick={onClose}>Cancel</Button>
        <Button variant="primary" busy={busy === 'apply'} disabled={!changed || !!busy} onClick={() => void doApply()}>Apply</Button>
      </>}>
      {kinds.length > 1 && (
        <Tabs label="Kind of picture" value={kind} panelId={ids + 'p'} className="ap-tabs"
          onChange={(k) => { setKind(k); setFocusAt(-1); setErr('') }}
          items={kinds.map((k) => ({ value: k.kind, label: k.label, badge: chosen[k.kind] !== undefined && chosen[k.kind] !== current(k.kind) ? <span className="ap-dot" aria-label=", changed" /> : undefined }))} />
      )}
      <div className={'ap-body ' + info.shape} id={ids + 'p'} role={kinds.length > 1 ? 'tabpanel' : undefined}>
        <div className="ap-left">
          {warnings.map((w) => <p key={w} className="muted ap-warn">{w}</p>)}
          {loading ? <div className="loading-box"><div className="spinner" /></div> : list.length === 0 ? (
            <p className="muted ap-none">No pictures were found for this. Bring your own below.</p>
          ) : (
            <div className={'ap-grid ' + info.shape} ref={grid} role="group" aria-label={`${info.label} choices`} data-nav-group onKeyDown={keys}>
              {list.map((c, i) => {
                const on = c.url === pick
                return (
                  <button type="button" key={c.url} className={'ap-choice' + (on ? ' on' : '') + (c.url === now ? ' now' : '')}
                    tabIndex={i === at ? 0 : -1} aria-pressed={on} aria-label={nameOf(c, i)}
                    onFocus={() => setFocusAt(i)} onClick={() => choose(c.url)} onDoubleClick={() => { choose(c.url, false); void doApply({ [kind]: c.url }) }}>
                    <img src={artPreviewUrl(c.url)} alt="" loading="lazy" draggable={false}
                      onLoad={(e) => { const m = e.currentTarget; if (m.naturalWidth) setSizes((s) => ({ ...s, [c.url]: [m.naturalWidth, m.naturalHeight] })) }}
                      onError={() => setBroken((b) => ({ ...b, [c.url]: true }))} />
                    <span className="ap-meta" aria-hidden="true">
                      <span>{c.url === now ? 'In use' : c.sourceLabel}</span>
                      {sizeOf(c.url) && <span>{sizeOf(c.url)}</span>}
                    </span>
                    {on && <span className="ap-check" aria-hidden="true"><IconCheck size={18} /></span>}
                  </button>
                )
              })}
            </div>
          )}
          <div className="ap-own" role="group" aria-label="Bring your own">
            {stage && <>
              <Button size="sm" icon={<IconImage size={16} />} busy={busy === 'upload'} disabled={!!busy && busy !== 'upload'} onClick={() => file.current?.click()}>Upload a file</Button>
              <input ref={file} type="file" accept="image/png,image/jpeg,image/webp,image/gif" hidden aria-label={`Upload a picture for the ${info.label.toLowerCase()}`}
                onChange={(e) => { void doUpload(e.target.files?.[0]); e.target.value = '' }} />
            </>}
            {frame && (info.shape === 'tall' || info.shape === 'wide') && (
              <Button size="sm" icon={<IconCamera size={16} />} busy={busy === 'frame'} disabled={!!busy && busy !== 'frame'} onClick={doFrame}>Use a frame from the video</Button>
            )}
            {maker && info.shape === 'tall' && (
              <Button size="sm" icon={<IconText size={16} />} aria-expanded={making} onClick={() => setMaking((m) => !m)}>Make a title poster</Button>
            )}
            <form className="ap-address" onSubmit={(e) => { e.preventDefault(); doAddress() }}>
              <input className="input" aria-label="A picture's address" placeholder="Or paste a picture's address" value={address} onChange={(e) => setAddress(e.target.value)} />
              <Button size="sm" type="submit" icon={<IconDownload size={16} />} disabled={!address.trim()}>Preview</Button>
            </form>
          </div>
          {maker && making && info.shape === 'tall' && <TitleMaker maker={maker} busy={busy === 'make'} onMake={doMake} />}
          {err && <div className="error ap-err" role="alert">{err}</div>}
        </div>
        <aside className="ap-preview" aria-label="Preview">
          <div className={'ap-big ' + info.shape}>
            {pick ? <img src={artPreviewUrl(pick)} alt={`The ${info.label.toLowerCase()} being previewed`} /> : <span className="muted">None yet</span>}
          </div>
          <div className="ap-facts">
            <b>{!pick ? 'No picture' : pick === now ? 'In use now' : 'Not applied yet'}</b>
            {shown?.sourceLabel && <span>{shown.own ? shown.sourceLabel : `From ${shown.sourceLabel}`}</span>}
            {sizeOf(pick) && <span>{sizeOf(pick)} pixels</span>}
          </div>
          {pick && pick !== now && <p className="muted ap-hint">Apply to use it. Cancel leaves everything as it was.</p>}
        </aside>
      </div>
      {note && <p className="muted ap-note">{note}</p>}
      <LegalNote kind="webResults" />
    </Dialog>
  )
}

// The words, colour and background of a title poster, made on the server when previewed.
function TitleMaker({ maker, busy, onMake }: { maker: Maker; busy: boolean; onMake: (o: MakeOptions) => void }) {
  const [words, setWords] = useState(maker.title)
  const [line, setLine] = useState(maker.line)
  const [hue, setHue] = useState<number | null>(null)
  const [frame, setFrame] = useState(false)
  const id = useId()
  // The server's colour for these words: spaces don't count.
  const own = artHue(words.trim().split(/\s+/).join(' ') || maker.title)
  const shown = hue ?? own
  return (
    <form className="ap-maker" aria-label="Title poster" onSubmit={(e) => { e.preventDefault(); onMake({ title: words, line, hue, frame }) }}>
      <label htmlFor={id + 'w'}>Words</label>
      <input id={id + 'w'} className="input" value={words} maxLength={120} onChange={(e) => setWords(e.target.value)} />
      <label htmlFor={id + 'l'}>Second line <span className="muted">(optional)</span></label>
      <input id={id + 'l'} className="input" value={line} maxLength={60} placeholder="The year, an edition" onChange={(e) => setLine(e.target.value)} />
      <span className="ap-maker-label" id={id + 'b'}>Behind the words</span>
      <div className="ap-maker-bg" role="radiogroup" aria-labelledby={id + 'b'}>
        <label><input type="radio" name={id + 'bg'} checked={!frame} onChange={() => setFrame(false)} /> A colour</label>
        {maker.hasVideo && <label><input type="radio" name={id + 'bg'} checked={frame} onChange={() => setFrame(true)} /> A frame from the video</label>}
      </div>
      {!frame && <>
        <label htmlFor={id + 'h'}>Colour</label>
        <div className="ap-maker-hue">
          <span className="ap-swatch" aria-hidden="true" style={{ background: `linear-gradient(155deg, hsl(${shown} 32% 32%), hsl(${(shown + 40) % 360} 38% 15%))` }} />
          <input id={id + 'h'} type="range" min={0} max={359} value={shown} aria-valuetext={hue === null ? 'Its own colour' : `Hue ${shown}`} onChange={(e) => setHue(Number(e.target.value))} />
          {hue !== null && <Button size="sm" variant="ghost" onClick={() => setHue(null)}>Its own colour</Button>}
        </div>
      </>}
      <div className="ap-maker-go">
        <Button size="sm" type="submit" variant="primary" busy={busy} disabled={!words.trim()}>Preview it</Button>
        <span className="muted">Then Apply to use it, over any poster it has.</span>
      </div>
    </form>
  )
}
