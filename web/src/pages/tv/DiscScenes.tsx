// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState } from 'react'
import { api, fmtRuntime, mediaUrl, mediaUrlQ } from '../../api'
import { IconDisc, IconDown, IconPlay, IconPlus, IconTrash, IconUp } from '../../icons'
import { useSource } from '../../player/hooks'
import { toast } from '../../state'
import { Button, Dialog, IconButton } from '../../ui'
import { Section } from '../settings/ui'
import { fmtTimecode, parseTimecode, type DiscClip, type DiscOption, type DiscPage, type DiscTitle, type Rules } from './tvApi'
import './tveditor.css'

// Discs on a channel: a whole main feature in one click, or any scenes from any
// title, picked by chapter from pictures and fine-tuned to the second. Encrypted
// discs are shown with the normal refusal, and can't be chosen.

const newClipId = () => 'c' + Math.random().toString(36).slice(2, 9)
const thumb = (item: number, ttn: number, n: number) => mediaUrl(`/api/items/${item}/disc/thumb/${ttn}/${n}`)

export function clipLabel(c: DiscClip): string {
  if (c.label) return c.label
  if (!c.start && !c.end && !c.title) return 'Main feature'
  if (c.fromChapter && c.toChapter && c.toChapter > c.fromChapter) return `Chapters ${c.fromChapter} to ${c.toChapter}`
  if (c.fromChapter) return `Chapter ${c.fromChapter}`
  return 'A scene'
}

export default function DiscScenes({ rules, set, id }: { rules: Rules; set: (p: Partial<Rules>) => void; id?: string }) {
  const [discs, setDiscs] = useState<DiscOption[] | null>(null)
  const [refusal, setRefusal] = useState('')
  const [picking, setPicking] = useState<DiscOption | null>(null)
  const [q, setQ] = useState('')
  useEffect(() => {
    api.get<{ discs: DiscOption[]; refusal: string }>('/channels/discs').then((r) => { setDiscs(r.discs); setRefusal(r.refusal) }).catch((e) => toast(e.message))
  }, [])
  const clips = rules.clips || []
  const setClips = (next: DiscClip[]) => set({ clips: next.length ? next : undefined })
  const mainOf = (d: DiscOption) => clips.find((c) => c.item === d.id && !c.title && !c.start && !c.end)
  const toggleMain = (d: DiscOption) => {
    const m = mainOf(d)
    setClips(m ? clips.filter((c) => c !== m) : [...clips, { id: newClipId(), item: d.id }])
  }
  const others = useMemo(() => {
    const rest = { ...rules }
    delete rest.clips
    return Object.values(rest).some((v) => v !== undefined && v !== '' && v !== false && !(Array.isArray(v) && v.length === 0))
  }, [rules])
  const title = (id: number) => discs?.find((d) => d.id === id)?.title || 'A disc'
  const move = (i: number, by: number) => {
    const n = [...clips]
    const [c] = n.splice(i, 1)
    n.splice(i + by, 0, c)
    setClips(n)
  }
  if (discs && discs.length === 0) {
    return <Section id={id} title="Discs and scenes" desc="Play a DVD or Blu-ray's main feature on this channel, or just the scenes you choose."><p className="muted">No discs in your libraries yet. Add a library with DVD or Blu-ray images or folders, and they appear here.</p></Section>
  }
  const shown = (discs || []).filter((d) => !q || d.title.toLowerCase().includes(q.toLowerCase()))
  return (
    <Section id={id} title="Discs and scenes" desc="Play a DVD or Blu-ray's main feature on this channel, or just the scenes you choose. Each is a programme of its own in the schedule.">
      {clips.length > 0 && (
        <p className="tvs-note" role="status">
          {others ? 'These play along with whatever the rules above choose.' : 'This channel plays only these. Set a rule above to mix in anything else.'}
        </p>
      )}
      {(discs?.length || 0) > 8 && <input className="input tve-find" type="search" placeholder="Find a disc…" aria-label="Find a disc" value={q} onChange={(e) => setQ(e.target.value)} />}
      <ul className="tve-discs">
        {shown.map((d) => {
          const n = clips.filter((c) => c.item === d.id && (c.title || c.start || c.end)).length
          const main = !!mainOf(d)
          return (
            <li key={d.id} className={'tve-disc' + (d.scrambled ? ' locked' : '') + (main || n ? ' chosen' : '')}>
              <div className="tve-disc-art">
                <img src={mediaUrlQ(`/api/items/${d.id}/poster`, `v=${d.updatedAt}`)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} />
                <span className="tve-disc-badge">{d.format === 'bluray' ? 'Blu-ray' : 'DVD'}</span>
                {d.scrambled && <span className="tve-disc-lock" aria-hidden="true">🔒</span>}
              </div>
              <div className="tve-disc-text">
                <b>{d.title}</b>
                <span>{[d.year, fmtRuntime(d.runtimeSec)].filter(Boolean).join(' · ')}</span>
                {d.scrambled ? <p className="tve-refused">{refusal}</p> : (
                  <div className="tve-disc-actions">
                    <Button size="sm" variant={main ? 'primary' : 'default'} aria-pressed={main} onClick={() => toggleMain(d)}>{main ? '✓ Main feature' : 'Main feature'}</Button>
                    <Button size="sm" aria-haspopup="dialog" onClick={() => setPicking(d)}>{n ? `Scenes (${n})` : 'Choose scenes'}</Button>
                  </div>
                )}
              </div>
            </li>
          )
        })}
      </ul>
      {clips.length > 0 && (
        <>
          <h3 className="tvs-h3">On this channel, in this order</h3>
          <ol className="tve-picked">
            {clips.map((c, i) => (
              <li key={c.id}>
                <img src={thumb(c.item, c.title || 0, c.fromChapter || 0)} alt="" loading="lazy" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} />
                <div className="tve-picked-text">
                  <b>{title(c.item)}</b>
                  <input className="input sm" value={c.label || ''} placeholder={clipLabel(c)} maxLength={200} aria-label={`Name of scene ${i + 1}`} onChange={(e) => setClips(clips.map((x) => (x === c ? { ...x, label: e.target.value || undefined } : x)))} />
                  <span>{c.end ? `${fmtTimecode(c.start || 0)} to ${fmtTimecode(c.end)} · ${fmtRuntime(c.end - (c.start || 0))}` : c.start ? `From ${fmtTimecode(c.start)} to the end` : 'The whole title'}</span>
                </div>
                <div className="tve-picked-actions">
                  <IconButton size="sm" disabled={i === 0} onClick={() => move(i, -1)} label={`Move ${clipLabel(c)} earlier`}><IconUp size={16} /></IconButton>
                  <IconButton size="sm" disabled={i === clips.length - 1} onClick={() => move(i, 1)} label={`Move ${clipLabel(c)} later`}><IconDown size={16} /></IconButton>
                  <IconButton size="sm" onClick={() => setClips(clips.filter((x) => x !== c))} label={`Remove ${clipLabel(c)}`}><IconTrash size={16} /></IconButton>
                </div>
              </li>
            ))}
          </ol>
        </>
      )}
      {picking && <ScenePicker disc={picking} clips={clips.filter((c) => c.item === picking.id)} onAdd={(c) => setClips([...clips, ...c])} onRemove={(c) => setClips(clips.filter((x) => x.id !== c.id))} onClose={() => setPicking(null)} />}
    </Section>
  )
}

// Picking scenes: a strip of chapter pictures for each title. Press one for the
// start and another for the end; the times can then be nudged to the second.
function ScenePicker({ disc, clips, onAdd, onRemove, onClose }: { disc: DiscOption; clips: DiscClip[]; onAdd: (c: DiscClip[]) => void; onRemove: (c: DiscClip) => void; onClose: () => void }) {
  const [page, setPage] = useState<DiscPage | null>(null)
  const [err, setErr] = useState('')
  const [ttn, setTtn] = useState(0)
  const [first, setFirst] = useState<number | null>(null) // chapter numbers, 1-based
  const [last, setLast] = useState<number | null>(null)
  const [start, setStart] = useState(0)
  const [end, setEnd] = useState(0)
  const [label, setLabel] = useState('')
  const [previewing, setPreviewing] = useState(false)
  useEffect(() => {
    api.get<DiscPage>(`/items/${disc.id}/disc`).then((p) => { setPage(p); setTtn(p.main?.ttn || p.extras[0]?.ttn || 0) }).catch((e) => setErr(e.message))
  }, [disc.id])
  const titles = useMemo(() => (page ? [...(page.main ? [page.main] : []), ...page.extras] : []), [page])
  const cur: DiscTitle | undefined = titles.find((t) => t.ttn === ttn)
  useEffect(() => { setFirst(null); setLast(null); setStart(0); setEnd(0); setLabel(''); setPreviewing(false) }, [ttn])
  const chapterEnd = (t: DiscTitle, n: number) => (n < t.chapters.length ? t.chapters[n].start : t.durationSec)
  const pick = (n: number) => {
    if (!cur) return
    let a = first, b = last
    if (a == null || (b != null && b !== a)) { a = n; b = n } else if (n < a) { b = a; a = n } else b = n
    setFirst(a); setLast(b)
    setStart(cur.chapters[a - 1].start)
    setEnd(chapterEnd(cur, b))
  }
  const chosen = first != null && last != null
  const add = () => {
    if (!cur || !chosen) return
    if (end - start < 3) { toast('A scene must run for at least 3 seconds'); return }
    onAdd([{ id: newClipId(), item: disc.id, title: cur.ttn, start: Math.round(start * 10) / 10, end: Math.round(end * 10) / 10, fromChapter: first, toChapter: last, label: label.trim() || undefined }])
    setFirst(null); setLast(null); setLabel(''); setPreviewing(false)
    toast('Scene added')
  }
  const addEach = () => {
    if (!cur) return
    onAdd(cur.chapters.map((c, i) => ({ id: newClipId(), item: disc.id, title: cur.ttn, start: c.start, end: chapterEnd(cur, i + 1), fromChapter: c.n, toChapter: c.n, label: c.label || undefined })).filter((c) => c.end - c.start >= 3))
    toast(`${cur.chapters.length} scenes added`)
  }
  const here = clips.filter((c) => c.title === ttn)
  const titleName = (t: DiscTitle, i: number) => t.label || (i === 0 && page?.main ? 'Main feature' : `Title ${t.ttn}`)
  return (
    <Dialog size="wide" className="tve-picker" title={`Scenes from ${disc.title}`} onClose={onClose}>
        {err && <p className="tvs-error" role="alert">{err}</p>}
        {!page && !err && <div className="center"><div className="spinner" /></div>}
        {titles.length > 0 && (
          <>
            <div className="tve-tabs" role="group" aria-label="Which title on the disc" data-nav-group>
              {titles.map((t, i) => <button key={t.ttn} type="button" className={'tve-tab' + (t.ttn === ttn ? ' on' : '')} aria-pressed={t.ttn === ttn} onClick={() => setTtn(t.ttn)}><b>{titleName(t, i)}</b><span>{fmtRuntime(t.durationSec)}</span></button>)}
            </div>
            {cur && (
              <>
                <p className="muted tve-how">{cur.chapters.length > 1 ? 'Press a chapter to start from it, and another to end with it.' : 'This title has one chapter. Press it, then set the times below.'}</p>
                <div className="tve-chapters" role="group" aria-label="Chapters" data-nav-group>
                  {cur.chapters.map((c) => {
                    const inRange = chosen && c.n >= first! && c.n <= last!
                    return (
                      <button key={c.n} type="button" className={'tve-chapter' + (inRange ? ' on' : '')} aria-pressed={inRange} onClick={() => pick(c.n)} aria-label={`Chapter ${c.n}${c.label ? `, ${c.label}` : ''}, at ${fmtTimecode(c.start)}`}>
                        <img src={thumb(disc.id, cur.ttn, c.n)} alt="" loading="lazy" decoding="async" onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden' }} />
                        <span><b>{c.label || `Chapter ${c.n}`}</b><small>{fmtTimecode(c.start)}</small></span>
                      </button>
                    )
                  })}
                </div>
                <Timeline title={cur} start={chosen ? start : null} end={chosen ? end : null} clips={here} />
                {chosen && (
                  <div className="tve-range">
                    <TimeField label="Starts" value={start} max={end - 3} onChange={setStart} />
                    <TimeField label="Ends" value={end} min={start + 3} max={cur.durationSec} onChange={setEnd} />
                    <label className="tve-range-name">Name (optional)<input className="input" value={label} maxLength={200} placeholder={first === last ? `Chapter ${first}` : `Chapters ${first} to ${last}`} onChange={(e) => setLabel(e.target.value)} /></label>
                    <span className="tve-range-len">{fmtRuntime(end - start)}</span>
                    <Button icon={<IconPlay />} onClick={() => setPreviewing((v) => !v)} aria-expanded={previewing}>{previewing ? 'Stop trying it' : 'Try it'}</Button>
                    <Button variant="primary" icon={<IconPlus />} onClick={add}>Add this scene</Button>
                  </div>
                )}
                {chosen && previewing && <ClipTry item={disc.id} ttn={cur.ttn} start={start} end={end} />}
                {here.length > 0 && (
                  <>
                    <h3 className="tvs-h3">Chosen from this title</h3>
                    <ul className="tve-chosen">
                      {here.map((c) => (
                        <li key={c.id}><span><b>{clipLabel(c)}</b> {fmtTimecode(c.start || 0)} to {c.end ? fmtTimecode(c.end) : 'the end'}</span>
                          <IconButton size="sm" onClick={() => onRemove(c)} label={`Remove ${clipLabel(c)}`}><IconTrash size={16} /></IconButton></li>
                      ))}
                    </ul>
                  </>
                )}
                <div className="actions">
                  {cur.chapters.length > 1 && <Button variant="ghost" onClick={addEach}>Add every chapter as a scene of its own</Button>}
                  <Button variant="primary" onClick={onClose}>Done</Button>
                </div>
              </>
            )}
          </>
        )}
        {page && titles.length === 0 && <><p className="muted"><IconDisc /> This disc has no titles that can be played.</p><div className="actions"><Button onClick={onClose}>Close</Button></div></>}
    </Dialog>
  )
}

// A picture of the whole title: its chapters, what is already chosen, and the
// scene being made. The list and the fields say the same in words.
function Timeline({ title, start, end, clips }: { title: DiscTitle; start: number | null; end: number | null; clips: DiscClip[] }) {
  const pct = (s: number) => (s / Math.max(1, title.durationSec)) * 100
  return (
    <div className="tve-timeline" aria-hidden="true">
      {title.chapters.map((c) => <i key={c.n} className="tick" style={{ left: pct(c.start) + '%' }} />)}
      {clips.map((c) => <i key={c.id} className="done" style={{ left: pct(c.start || 0) + '%', width: pct((c.end || title.durationSec) - (c.start || 0)) + '%' }} />)}
      {start != null && end != null && <i className="now" style={{ left: pct(start) + '%', width: pct(end - start) + '%' }} />}
    </div>
  )
}

// A time that can be typed, or nudged with buttons a remote can reach.
function TimeField({ label, value, min = 0, max, onChange }: { label: string; value: number; min?: number; max: number; onChange: (v: number) => void }) {
  const [text, setText] = useState(fmtTimecode(value))
  useEffect(() => setText(fmtTimecode(value)), [value])
  const put = (v: number) => onChange(Math.max(min, Math.min(max, v)))
  const commit = () => {
    const v = parseTimecode(text)
    if (v == null) setText(fmtTimecode(value))
    else put(v)
  }
  return (
    <div className="tve-time" role="group" aria-label={label}>
      <label>{label}<input className="input" value={text} inputMode="numeric" onChange={(e) => setText(e.target.value)} onBlur={commit} onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); commit() } }} /></label>
      <div className="tve-nudge">
        <Button size="sm" variant="ghost" onClick={() => put(value - 5)} aria-label={`${label} 5 seconds earlier`}>−5</Button>
        <Button size="sm" variant="ghost" onClick={() => put(value - 1)} aria-label={`${label} 1 second earlier`}>−1</Button>
        <Button size="sm" variant="ghost" onClick={() => put(value + 1)} aria-label={`${label} 1 second later`}>+1</Button>
        <Button size="sm" variant="ghost" onClick={() => put(value + 5)} aria-label={`${label} 5 seconds later`}>+5</Button>
      </div>
    </div>
  )
}

// Trying a scene before adding it: plays the range round and round.
function ClipTry({ item, ttn, start, end }: { item: number; ttn: number; start: number; end: number }) {
  const video = useRef<HTMLVideoElement | null>(null)
  const [url, setUrl] = useState<string | null>(null)
  const [msg, setMsg] = useState('')
  const span = useRef({ start, end })
  span.current = { start, end }
  useEffect(() => {
    let on = true
    let stream = ''
    api.post<{ url: string }>('/channels/clip-preview', { item, title: ttn }).then((r) => { if (on) { stream = r.url; setUrl(r.url) } }).catch((e) => { if (on) setMsg(e.message) })
    return () => { on = false; if (stream) api.del(stream.replace('/api', '').replace('/playlist.m3u8', '')).catch(() => {}) }
  }, [item, ttn])
  useSource(video, url, {
    startAt: () => span.current.start,
    onReady: () => { const v = video.current; if (v) { v.currentTime = span.current.start; void v.play().catch(() => {}) } },
    onError: setMsg,
  })
  const onTime = () => {
    const v = video.current
    if (v && (v.currentTime >= span.current.end || v.currentTime < span.current.start - 2)) v.currentTime = span.current.start
  }
  useEffect(() => { const v = video.current; if (v && url) v.currentTime = start }, [start, url])
  return (
    <div className="tve-try">
      <video ref={video} controls playsInline onTimeUpdate={onTime} aria-label="Trying the scene" />
      {msg && <p className="tvs-error" role="alert">{msg}</p>}
    </div>
  )
}
