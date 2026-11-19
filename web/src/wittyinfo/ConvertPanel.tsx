// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { t, tn } from '../i18n'
import { IconCheck } from '../icons'
import { Progress } from '../ui'
import './info.css'

// Converting a library's metadata (first-run setup's Posters and details, and Settings,
// Metadata): what lies beside the media in the usual files (NFO files, posters, lyrics, album
// details, a game's details) gathered into WittyMediaServer's own files, or written back out
// from them. One row a format, each a task with its progress shown here:
//
//   .wittyinfo   films and TV (docs/formats/wittyinfo.md)
//   .wittyaudio  music (docs/formats/wittyaudio.md)
//   .wittyplay   games, when WittyPlay is installed (WittyPlay's own; its module answers
//                GET /api/modules/wittyplay/convert/counts and POST /api/modules/wittyplay/convert)
//
// onBusy tells the page whether something chosen is still to run or running, so setup can
// hold Continue until it is done.

type Direction = 'keep' | 'gather' | 'spread'
type Counts = {
  titles: number; withNfo: number; withArt: number; withSkips: number; withInfo: number; shows: number; showsWithNfo: number; showsWithInfo: number
  songs: number; songsWithLyrics: number; songsWithInfo: number; albums: number; albumsWithArt: number; albumsWithInfo: number; readOnlyLibraries: number
}
type GameCounts = { games: number; withMetadata: number; withInfo: number }
type TaskView = { id: string; key?: string; state: string; percent: number; done: number; total: number; item?: string; result?: string; error?: string }
type Row = { key: 'wittyinfo' | 'wittyaudio' | 'wittyplay'; ext: string; what: string; found: string; has: number; hasOwn: number; standard: string }

export default function ConvertPanel({ onBusy, compact, beforeStart }: { onBusy?: (busy: boolean) => void; compact?: boolean; beforeStart?: () => Promise<unknown> }) {
  const [counts, setCounts] = useState<Counts | null>(null)
  const [games, setGames] = useState<GameCounts | null>(null)
  const [canWrite, setCanWrite] = useState(true)
  const [err, setErr] = useState('')
  const [dir, setDir] = useState<Record<string, Direction>>({})
  const [subs, setSubs] = useState(false)
  const [retire, setRetire] = useState(false)
  const [tasks, setTasks] = useState<Record<string, TaskView>>({})
  const [started, setStarted] = useState<Record<string, string>>({})
  const poll = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = () => {
    api.get<{ counts: Counts; canWrite: boolean }>('/wittyinfo/convert/counts').then((r) => { setCounts(r.counts); setCanWrite(r.canWrite) }).catch((e) => setErr((e as Error).message))
    api.get<GameCounts>('/modules/wittyplay/convert/counts').then(setGames).catch(() => setGames(null))
  }
  useEffect(load, [])

  const rows: Row[] = []
  if (counts && (counts.titles || counts.shows)) rows.push({
    key: 'wittyinfo', ext: '.wittyinfo', what: t('Films and TV'), standard: t('NFO files, posters and skip files'),
    found: t('{titles} films and episodes, {nfo} with NFO files and {art} with pictures; {shows} shows', { titles: counts.titles, nfo: counts.withNfo, art: counts.withArt, shows: counts.shows }),
    has: counts.withNfo + counts.withArt + counts.withSkips + counts.showsWithNfo, hasOwn: counts.withInfo + counts.showsWithInfo,
  })
  if (counts && counts.songs) rows.push({
    key: 'wittyaudio', ext: '.wittyaudio', what: t('Music'), standard: t('.lrc lyrics, album covers and album.nfo files'),
    found: t('{songs} songs, {lyrics} with lyrics files; {albums} albums, {art} with a cover or album.nfo', { songs: counts.songs, lyrics: counts.songsWithLyrics, albums: counts.albums, art: counts.albumsWithArt }),
    has: counts.songsWithLyrics + counts.albumsWithArt, hasOwn: counts.songsWithInfo + counts.albumsWithInfo,
  })
  if (games && games.games) rows.push({
    key: 'wittyplay', ext: '.wittyplay', what: t('Games'), standard: t('game lists, NFO files, box art and screenshots'),
    found: t('{games} games, {meta} with details beside them', { games: games.games, meta: games.withMetadata }),
    has: games.withMetadata, hasOwn: games.withInfo,
  })

  const chosen = rows.filter((r) => (dir[r.key] || 'keep') !== 'keep')
  const running = Object.values(tasks).some((x) => x.state === 'running' || x.state === 'queued')
  const pending = chosen.some((r) => !started[r.key])
  useEffect(() => { onBusy?.(running || pending) }, [running, pending]) // eslint-disable-line react-hooks/exhaustive-deps

  // Follow the started tasks until each has finished.
  useEffect(() => {
    const ids = Object.values(started)
    if (!ids.length) return
    const tick = async () => {
      try {
        const r = await api.get<{ tasks: TaskView[]; recent: TaskView[] }>('/system/tasks')
        const all = [...(r.tasks || []), ...(r.recent || [])]
        const next: Record<string, TaskView> = {}
        // A module's task (WittyPlay's) is known by its key; the server's own by its id.
        for (const [k, id] of Object.entries(started)) { const x = all.find((y) => y.id === id || y.key === id); if (x) next[k] = x }
        setTasks(next)
        if (Object.values(next).length === ids.length && Object.values(next).every((x) => x.state !== 'running' && x.state !== 'queued')) {
          if (poll.current) clearInterval(poll.current)
          poll.current = null
          load()
        }
      } catch { /* the next look tries again */ }
    }
    void tick()
    poll.current = setInterval(tick, 1000)
    return () => { if (poll.current) clearInterval(poll.current) }
  }, [started])

  const convert = async () => {
    setErr('')
    try { await beforeStart?.() } catch { /* the choice is the server's as it is */ }
    const out: Record<string, string> = { ...started }
    for (const r of chosen) {
      if (out[r.key]) continue
      const body = { direction: dir[r.key], subtitles: subs, retire }
      try {
        const res = r.key === 'wittyplay'
          ? await api.post<{ task: string }>('/modules/wittyplay/convert', body)
          : await api.post<{ task: string }>('/wittyinfo/convert', { ...body, format: r.key })
        out[r.key] = res.task
      } catch (e) { setErr((e as Error).message); break }
    }
    setStarted(out)
  }

  if (err && !counts) return <p className="error" role="alert">{err}</p>
  if (!counts) return <p className="muted">{t('Looking at what your libraries already have…')}</p>
  return (
    <div className={'wi-convert' + (compact ? ' compact' : '')}>
      <p className="muted wi-help">{t('WittyMediaServer keeps what people add by hand in a file of its own beside each title: .wittyinfo for films and TV, .wittyaudio for music and .wittyplay for games. Each is one small archive with the details, pictures, skips, lyrics or settings in it, and every one can be turned back into the usual files at any time, here or in Settings.')}</p>
      {rows.length === 0 && <p className="muted">{t('Your libraries have nothing to convert yet. Come back here from Settings, Metadata once they are scanned.')}</p>}
      {rows.map((r) => {
        const d = dir[r.key] || 'keep'
        const tk = tasks[r.key]
        const name = 'wi-conv-' + r.key
        return (
          <fieldset key={r.key} className="wi-conv-row">
            <legend><b>{r.what}</b> <code>{r.ext}</code></legend>
            <p className="muted wi-help">{r.found}</p>
            <label className="wi-check"><input type="radio" name={name} checked={d === 'keep'} disabled={!!started[r.key]} onChange={() => setDir({ ...dir, [r.key]: 'keep' })} /> {t('Leave them as they are')}</label>
            <label className="wi-check"><input type="radio" name={name} checked={d === 'gather'} disabled={!!started[r.key] || !r.has} onChange={() => setDir({ ...dir, [r.key]: 'gather' })} /> {t('Gather the {standard} into {ext} files', { standard: r.standard, ext: r.ext })}</label>
            <label className="wi-check"><input type="radio" name={name} checked={d === 'spread'} disabled={!!started[r.key] || !r.hasOwn} onChange={() => setDir({ ...dir, [r.key]: 'spread' })} /> {tn('Write {standard} from the {count} {ext} file', 'Write {standard} from the {count} {ext} files', r.hasOwn, { standard: r.standard, ext: r.ext })}</label>
            {tk && (
              <div className="wi-conv-progress" role="status" aria-live="polite">
                <Progress value={tk.percent >= 0 ? tk.percent : undefined} label={r.what} />
                <span className="muted">{tk.state === 'done' ? <><IconCheck size={16} /> {tk.result || t('Done')}</> : tk.state === 'failed' ? tk.error : tk.state === 'cancelled' ? t('Stopped') : tk.total ? t('{done} of {total}: {item}', { done: tk.done, total: tk.total, item: tk.item || '' }) : tk.item || t('Starting…')}</span>
              </div>
            )}
          </fieldset>
        )
      })}
      {rows.length > 0 && (
        <>
          <div className="wi-conv-opts">
            {rows.some((r) => r.key === 'wittyinfo') && <label className="wi-check"><input type="checkbox" checked={subs} onChange={(e) => setSubs(e.target.checked)} /> {t('Include subtitle files (SRT, WebVTT, ASS)')}</label>}
            <label className="wi-check"><input type="checkbox" checked={retire} onChange={(e) => setRetire(e.target.checked)} /> {t('Rename the old files with .bak on the end once converted (nothing is deleted)')}</label>
          </div>
          {!canWrite && chosen.some((r) => dir[r.key] === 'gather') && <p className="muted wi-help">{t('Writing files beside your media is off, so gathered files are kept on this server instead. Turn on “Save information and artwork next to your media” to have them written beside each title.')}</p>}
          {counts.readOnlyLibraries > 0 && <p className="muted wi-help">{tn('{count} library is read-only and is left as it is.', '{count} libraries are read-only and are left as they are.', counts.readOnlyLibraries)}</p>}
          {err && <p className="error" role="alert">{err}</p>}
          <div className="wi-add">
            <button type="button" className="btn primary" disabled={!pending || running} onClick={convert}>{t('Convert now')}</button>
            {running && <span className="muted">{t('This can take a while for a big library. It waits for any scan to finish first, and goes on in Tasks if you leave.')}</span>}
          </div>
        </>
      )}
    </div>
  )
}
