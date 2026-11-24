// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState } from 'react'
import { announce } from '../a11y'
import { api, getToken, mediaUrl, posterUrl, serverBase, sized, fitWidth, type Item } from '../api'
import { HelpLink } from '../components/HelpLink'
import { TaskLine } from '../extras/jobs'
import '../extras/jobs.css'
import { IconFilm, IconFolder, IconSearch } from '../icons'
import { useMusicLinks } from '../pages/music/links'
import { Button, Dialog, Notice, ask, toast } from '../ui'
import LoopEditor, { type Range } from './LoopEditor'
import { GEN_MAX_SEC, GEN_MIN_SEC, SOURCE_WORDS } from './rules'
import { noteCanvasChanged, useCanvas } from './useCanvas'
import { num, t, tx, bidi } from '../i18n'
import './canvas.css'
import './studio.css'

// The Canvas studio: a song's or a title's looping video, for an administrator. It shows the one there is (and can
// set the part of it that loops, with no new file), and makes a new one from a clip on this device, from any video on
// the server (a song's own music videos first), or from the title's own video. Every new one is cut on the server,
// small and silent, with its end blended into its start unless asked not to. Docs: docs/MUSIC.md, "Canvas".

const MAX_CLIP = 512 * 1024 * 1024
type Pick = { kind: 'device'; file: File; url: string; duration: number } | { kind: 'server'; item: Item } | { kind: 'self' } | { kind: 'current' }

export default function CanvasStudio({ itemId, title, kind, runtimeSec, artist = '', picker, onClose }: {
  itemId: number; title: string; kind: 'video' | 'song' | 'disc'; runtimeSec: number; artist?: string
  picker?: React.ReactNode // an album's page chooses which song first
  onClose: () => void
}) {
  const c = useCanvas(itemId, true)
  const info = c.info
  const [pick, setPick] = useState<Pick | null>(null)
  const [range, setRange] = useState<Range>({ start: 0, end: 6 })
  const [fade, setFade] = useState(true)
  const [busy, setBusy] = useState(false)
  const [sent, setSent] = useState(0) // a clip's upload, 0 to 1
  const hadTask = useRef('')
  // The canvas as a local copy while its loop is edited: a few MB, and every frame the editor draws is then instant.
  const [local, setLocal] = useState('')
  useEffect(() => {
    if (pick?.kind !== 'current' || !info?.url) return
    let url = '', live = true
    fetch(mediaUrl(info.url)).then((r) => r.blob()).then((b) => { if (live) { url = URL.createObjectURL(b); setLocal(url) } }).catch(() => { if (live) setLocal(mediaUrl(info.url!)) })
    return () => { live = false; if (url) URL.revokeObjectURL(url); setLocal('') }
  }, [pick?.kind, info?.url])
  useEffect(() => {
    if (!info) return
    if (hadTask.current && !info.task) { announce(info.error ? t('The canvas couldn’t be made.') : t('The canvas is ready.')); noteCanvasChanged(itemId); if (!info.error) setPick(null) }
    hadTask.current = info.task || ''
  }, [info, itemId])
  useEffect(() => () => { if (pick?.kind === 'device') URL.revokeObjectURL(pick.url) }, [pick])

  const canCut = !!info?.canCut
  const sourceLen = pick?.kind === 'device' ? pick.duration : pick?.kind === 'server' ? pick.item.runtimeSec : pick?.kind === 'self' ? runtimeSec : (info?.durationMs || 0) / 1000
  const choose = (p: Pick) => {
    setPick(p)
    const len = p.kind === 'device' ? p.duration : p.kind === 'server' ? p.item.runtimeSec : p.kind === 'self' ? runtimeSec : (info?.durationMs || 0) / 1000
    if (p.kind === 'current') setRange({ start: (info?.loopStartMs || 0) / 1000, end: (info?.loopEndMs || info?.durationMs || 0) / 1000 })
    else { const s = len > 30 ? len * 0.25 : 0; setRange({ start: +s.toFixed(1), end: +Math.min(len, s + 6).toFixed(1) }) }
  }
  const failed = (e: Error) => { toast(e.message, { tone: 'danger' }); setBusy(false) }
  const make = async () => {
    if (!pick) return
    setBusy(true)
    const startSec = +range.start.toFixed(2), durationSec = +(range.end - range.start).toFixed(2)
    try {
      if (pick.kind === 'current') {
        await api.put(`/media/${itemId}/canvas/loop`, { startMs: Math.round(range.start * 1000), endMs: Math.round(range.end * 1000) })
        toast(t('Loop saved')); noteCanvasChanged(itemId); c.reload(); setPick(null)
      } else if (pick.kind === 'device') {
        await sendClip(`/media/${itemId}/canvas/source?startSec=${startSec}&durationSec=${durationSec}&crossfade=${fade ? 1 : 0}`, pick.file, setSent)
        c.reload()
      } else {
        await api.post(`/media/${itemId}/canvas/generate`, { startSec, durationSec, crossfade: fade, ...(pick.kind === 'server' ? { fromId: pick.item.id } : {}) })
        c.reload()
      }
    } catch (e) { failed(e as Error); return } finally { setSent(0) }
    setBusy(false)
  }
  const remove = async () => {
    if (!await ask({ title: t('Remove this canvas?'), body: t('The still cover shows again. A canvas file beside the media, if there is one, takes its place.'), confirm: t('Remove'), danger: true })) return
    api.del(`/media/${itemId}/canvas`).then(() => { toast(t('Canvas removed')); c.reload(); noteCanvasChanged(itemId) }).catch(failed)
  }
  const own = info?.source === 'upload' || info?.source === 'generated'
  const cutting = pick && pick.kind !== 'current'
  const tooShort = !!pick && sourceLen > 0 && sourceLen < (pick.kind === 'current' ? 0.5 : GEN_MIN_SEC)

  return (
    <Dialog title={t('Canvas studio')} onClose={onClose} size="wide"
      description={<>{t('A short, silent video that loops with “{title}”: behind the player, or in the cover’s place.', { title })} <HelpLink to="music#canvas-a-looping-video-behind-the-song" newTab>{t('About Canvas')}</HelpLink></>}
      actions={pick ? <>
        <Button variant="ghost" onClick={() => setPick(null)} disabled={busy}>{t('Back')}</Button>
        <Button variant="primary" busy={busy} disabled={busy || !!info?.task || tooShort} onClick={make}>{pick.kind === 'current' ? t('Save the loop') : t('Make the canvas')}</Button>
      </> : <Button variant="ghost" onClick={onClose}>{t('Done')}</Button>}>
      <div className="cs">
        {picker && !pick && picker}
        {!info && !c.failed && <p className="muted" role="status">{t('Looking…')}</p>}
        {c.failed && <Notice tone="danger">{c.failed}</Notice>}
        {info?.task && <TaskLine id={info.task} label={t('Cutting the canvas')} />}
        {!info?.task && info?.error && <Notice tone="danger" title={t('The last one couldn’t be made')}>{info.error}</Notice>}
        {sent > 0 && sent < 1 && <div className="cs-sending" role="status"><span style={{ width: `${Math.round(sent * 100)}%` }} />{t('Sending the clip… {percent}', { percent: num(sent, { style: 'percent' }) })}</div>}

        {info && !pick && (
          <>
            <section className="cs-now" aria-label={t('The canvas now')}>
              {info.available && info.url ? (
                <>
                  <video src={mediaUrl(info.url)} muted loop playsInline autoPlay preload="metadata" aria-label={t('The canvas for {title}', { title })} />
                  <div className="cs-now-facts">
                    <h3>{t('The canvas now')}</h3>
                    <p className="muted">{tx(SOURCE_WORDS[info.source || ''] || '')} {info.width && info.height ? `${info.width}×${info.height}` : ''}{info.durationMs ? ' · ' + t('{n} s', { n: num(info.durationMs / 1000, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) }) : ''}</p>
                    {info.loopEndMs ? <p className="muted">{t('Loops {start} to {end} s.', { start: num(info.loopStartMs! / 1000, { minimumFractionDigits: 1, maximumFractionDigits: 1 }), end: num(info.loopEndMs / 1000, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) })}</p> : null}
                    <div className="cs-row">
                      {canCut && <Button size="sm" onClick={() => choose({ kind: 'current' })}>{t('Set its loop')}</Button>}
                      {info.loopEndMs ? <Button size="sm" variant="ghost" onClick={() => api.del(`/media/${itemId}/canvas/loop`).then(() => { c.reload(); noteCanvasChanged(itemId) }).catch(failed)}>{t('Loop all of it')}</Button> : null}
                      {own && <Button size="sm" variant="danger" onClick={remove}>{t('Remove')}</Button>}
                    </div>
                  </div>
                </>
              ) : <p className="cs-empty">{t('“{title}” has no canvas yet. Make one below.', { title })}</p>}
            </section>
            {canCut ? <Sources itemId={itemId} kind={kind} artist={artist} title={title} canSelf={!!info.canGenerate} onPick={choose} />
              : <Notice tone="info">{t('This server can’t cut a canvas: its FFmpeg has no H.264 encoder. You can still put a canvas.mp4 beside the media.')}</Notice>}
          </>
        )}

        {pick && sourceLen > 0 && !tooShort && (pick.kind !== 'current' || local) && (
          <section className="cs-edit" aria-label={t('Choose the loop')}>
            <h3>{pick.kind === 'current' ? t('The part that loops') : pick.kind === 'device' ? pick.file.name : pick.kind === 'server' ? pick.item.title : t('This title’s video')}</h3>
            <LoopEditor duration={sourceLen} value={range} onChange={setRange}
              min={pick.kind === 'current' ? 0.5 : GEN_MIN_SEC} max={pick.kind === 'current' ? sourceLen : Math.min(GEN_MAX_SEC, sourceLen)} crossfade={!!cutting && fade}
              source={pick.kind === 'device' ? { kind: 'video', url: pick.url }
                : pick.kind === 'current' ? { kind: 'video', url: local }
                  : { kind: 'frames', frame: frameOf(pick.kind === 'server' ? pick.item.id : itemId) }} />
            {cutting && (
              <label className="cs-fade"><input type="checkbox" checked={fade} onChange={(e) => setFade(e.target.checked)} /> {t('Blend the end into the start, so the loop has no jump')}</label>
            )}
            {pick.kind === 'device' && <p className="muted">{t('The clip is sent to the server, the part between the handles is cut from it, and the clip is deleted.')}</p>}
          </section>
        )}
        {tooShort && <Notice tone="danger">{t('That video is too short to loop.')}</Notice>}
      </div>
    </Dialog>
  )
}

const frameOf = (id: number) => (sec: number) => mediaUrl(`/api/items/${id}/scene/${Math.max(0, sec)}`)

// Where a new canvas comes from.
function Sources({ itemId, kind, artist, title, canSelf, onPick }: { itemId: number; kind: string; artist: string; title: string; canSelf: boolean; onPick: (p: Pick) => void }) {
  const [tab, setTab] = useState<'device' | 'server'>('device')
  const file = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)
  const takeFile = (f: File | undefined) => {
    if (!f) return
    if (f.size > MAX_CLIP) { toast(t('That clip is too big: up to 512 MB.'), { tone: 'danger' }); return }
    const url = URL.createObjectURL(f)
    const v = document.createElement('video')
    v.preload = 'metadata'; v.src = url
    v.onloadedmetadata = () => onPick({ kind: 'device', file: f, url, duration: v.duration })
    v.onerror = () => { URL.revokeObjectURL(url); toast(t('This browser can’t open that video. MP4, MOV or WebM works best.'), { tone: 'danger' }) }
  }
  return (
    <section className="cs-new" aria-label={t('Make a new canvas')}>
      <h3>{t('Make a new one')}</h3>
      <div className="cs-tabs" role="tablist" aria-label={t('From')}>
        <button type="button" role="tab" aria-selected={tab === 'device'} className={tab === 'device' ? 'on' : ''} onClick={() => setTab('device')}><IconFolder size={18} /> {t('From this device')}</button>
        <button type="button" role="tab" aria-selected={tab === 'server'} className={tab === 'server' ? 'on' : ''} onClick={() => setTab('server')}><IconFilm size={18} /> {t('A video on the server')}</button>
        {canSelf && <button type="button" role="tab" aria-selected={false} onClick={() => onPick({ kind: 'self' })}><IconFilm size={18} /> {kind === 'video' ? t('This title’s video') : t('Its own video')}</button>}
      </div>
      {tab === 'device' ? (
        <div className={'cs-drop' + (over ? ' over' : '')} onDragOver={(e) => { e.preventDefault(); setOver(true) }} onDragLeave={() => setOver(false)}
          onDrop={(e) => { e.preventDefault(); setOver(false); takeFile(e.dataTransfer.files?.[0]) }}>
          <input ref={file} type="file" accept="video/mp4,video/quicktime,video/webm,.mp4,.mov,.m4v,.webm" hidden onChange={(e) => takeFile(e.target.files?.[0])} />
          <p>{t('Drop a video here, or')}</p>
          <Button variant="primary" onClick={() => file.current?.click()}>{t('Choose a video…')}</Button>
          <p className="muted">{t('MP4, MOV or WebM, up to 512 MB. Only the part you choose is kept.')}</p>
        </div>
      ) : <ServerVideos itemId={itemId} artist={artist} title={title} songs={kind === 'song'} onPick={(item) => onPick({ kind: 'server', item })} />}
    </section>
  )
}

// Videos on the server: a song's own music videos and close matches first, then whatever is searched for.
function ServerVideos({ itemId, artist, title, songs, onPick }: { itemId: number; artist: string; title: string; songs: boolean; onPick: (it: Item) => void }) {
  const links = useMusicLinks(songs ? itemId : undefined)
  const [q, setQ] = useState('')
  const [found, setFound] = useState<Item[] | null>(null)
  useEffect(() => {
    let live = true
    const timer = setTimeout(() => {
      api.get<{ items: Item[] } | Item[]>(`/items?kind=video&limit=24&search=${encodeURIComponent(q.trim())}`)
        .then((r) => { if (live) setFound((Array.isArray(r) ? r : r.items).filter((x) => x.id !== itemId)) }).catch(() => { if (live) setFound([]) })
    }, q ? 300 : 0)
    return () => { live = false; clearTimeout(timer) }
  }, [q, itemId])
  const suggested = useMemo(() => [...(links?.tied || []), ...(links?.suggested || [])].map((l) => l.video), [links])
  const row = (v: { id: number; title: string; runtimeSec: number; artist?: string }, it?: Item) => (
    <li key={v.id}>
      <button type="button" className="cs-vid" onClick={() => onPick(it || ({ id: v.id, title: v.title, runtimeSec: v.runtimeSec } as Item))}>
        <span className="cs-vid-pic" style={{ backgroundImage: `url("${it?.poster ? sized(posterUrl(it), fitWidth(200)) : mediaUrl(`/api/items/${v.id}/scene/${Math.floor(v.runtimeSec / 4)}`)}")` }} />
        <span className="cs-vid-t"><b>{v.title}</b><small>{[v.artist, v.runtimeSec ? `${Math.floor(v.runtimeSec / 60)}:${String(Math.floor(v.runtimeSec % 60)).padStart(2, '0')}` : ''].filter(Boolean).map(bidi).join(' · ')}</small></span>
      </button>
    </li>
  )
  return (
    <div className="cs-server">
      {suggested.length > 0 && <><h4>{t('Music videos of “{title}”', { title })}</h4><ul className="cs-vids">{suggested.map((v) => row(v))}</ul></>}
      <label className="cs-search"><IconSearch size={18} /><input type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder={songs && artist ? t('Search the server’s videos, “{artist}” for one', { artist }) : t('Search the server’s videos')} aria-label={t('Search the server’s videos')} /></label>
      {found === null ? <p className="muted" role="status">{t('Looking…')}</p> : found.length === 0 ? <p className="muted">{t('No videos match.')}</p> : <ul className="cs-vids">{found.map((it) => row(it, it))}</ul>}
    </div>
  )
}

// sendClip: a clip to the server with the share of it sent so far (fetch can't say that).
function sendClip(path: string, f: File, progress: (x: number) => void): Promise<void> {
  return new Promise((done, fail) => {
    const x = new XMLHttpRequest()
    x.open('PUT', serverBase() + '/api' + path)
    const tok = getToken()
    if (tok) x.setRequestHeader('Authorization', 'Bearer ' + tok)
    x.setRequestHeader('Content-Type', 'application/octet-stream')
    x.upload.onprogress = (e) => { if (e.lengthComputable) progress(e.loaded / e.total) }
    x.onload = () => {
      if (x.status >= 200 && x.status < 300) { progress(1); done(); return }
      let m = t('The clip couldn’t be sent.')
      try { m = JSON.parse(x.responseText).error || m } catch { /* plain text */ }
      fail(new Error(m))
    }
    x.onerror = () => fail(new Error(t('Can’t reach your server.')))
    x.send(f)
  })
}
