import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, type Item, type Library } from '../api'
import { useAudio } from '../audio/player'
import { EmptyState, Spinner } from '../components/common'
import { IconPause, IconPlay, IconPlus, IconTrash } from '../icons'
import { toast, useApp } from '../state'
import '../gallery.css'

// Internet radio: stations the owner adds by address. The server contacts a
// station only while someone is listening (docs/RADIO.md).

export default function RadioLibrary({ lib }: { lib: Library }) {
  const { user } = useApp()
  const audio = useAudio()
  const [stations, setStations] = useState<Item[] | null>(null)
  const [adding, setAdding] = useState(false)
  const [genre, setGenre] = useState('')
  const load = useCallback(() => { api.get<Item[]>(`/radio?library=${lib.id}`).then(setStations).catch(() => setStations([])) }, [lib.id])
  useEffect(load, [load])
  const genres = useMemo(() => [...new Set((stations || []).flatMap((s) => s.genres))].sort(), [stations])
  const shown = (stations || []).filter((s) => !genre || s.genres.includes(genre))
  const remove = async (s: Item) => {
    if (!confirm(`Remove ${s.title}?`)) return
    try { await api.del(`/radio/${s.id}`); load() } catch (e: any) { toast(e.message) }
  }
  if (!stations) return <Spinner />
  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}</h1>
        <span className="count">{stations.length} {stations.length === 1 ? 'station' : 'stations'}</span>
        <div style={{ flex: 1 }} />
        {user?.isAdmin && <button className="btn sm primary" onClick={() => setAdding(true)}><IconPlus /> Add stations</button>}
      </div>
      {genres.length > 1 && (
        <div className="ch-tags" role="group" aria-label="Genres">
          <button className={'ch-tag' + (!genre ? ' on' : '')} onClick={() => setGenre('')}>All</button>
          {genres.map((g) => <button key={g} className={'ch-tag' + (genre === g ? ' on' : '')} aria-pressed={genre === g} onClick={() => setGenre(g)}>{g}</button>)}
        </div>
      )}
      {!stations.length ? <EmptyState title="No stations yet" text={user?.isAdmin ? 'Add a station by its stream address, or import an M3U or PLS playlist.' : 'An administrator can add stations by their stream address.'} /> : (
        <ul className="radio-list">
          {shown.map((s, i) => {
            const on = audio.current?.id === s.id
            return (
              <li key={s.id} className={on ? 'on' : ''}>
                <button className="radio-play" aria-label={`${on && audio.playing ? 'Pause' : 'Play'} ${s.title}`} onClick={() => (on ? audio.toggle() : audio.play(shown, i, 'radio'))}>{on && audio.playing ? <IconPause /> : <IconPlay />}</button>
                <span className="radio-name"><b>{s.title}</b>{s.genres[0] && <span className="muted">{s.genres[0]}</span>}</span>
                {user?.isAdmin && <button className="btn sm ghost" aria-label={`Remove ${s.title}`} onClick={() => remove(s)}><IconTrash /></button>}
              </li>
            )
          })}
        </ul>
      )}
      {adding && <AddStations lib={lib} onClose={() => setAdding(false)} onDone={() => { setAdding(false); load() }} />}
    </>
  )
}

function AddStations({ lib, onClose, onDone }: { lib: Library; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [genre, setGenre] = useState('')
  const [playlist, setPlaylist] = useState('')
  const [err, setErr] = useState('')
  const add = async (e: React.FormEvent) => {
    e.preventDefault(); setErr('')
    try {
      if (playlist.trim()) {
        const r = await api.post<{ added: number }>('/radio/import', { libraryId: lib.id, playlist })
        toast(`${r.added} ${r.added === 1 ? 'station' : 'stations'} added`)
      }
      if (url.trim()) await api.post('/radio', { libraryId: lib.id, name, url, genre })
      onDone()
    } catch (x: any) { setErr(x.message) }
  }
  const readFile = async (f?: File) => { if (f) setPlaylist(await f.text()) }
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <form className="modal" role="dialog" aria-modal="true" aria-labelledby="ra-h" onClick={(e) => e.stopPropagation()} onSubmit={add}>
        <h2 id="ra-h">Add stations</h2>
        <label className="oobe-field">Stream address<input className="input" type="url" autoFocus placeholder="https://…/stream" value={url} onChange={(e) => setUrl(e.target.value)} /></label>
        <label className="oobe-field">Name<input className="input" maxLength={200} value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className="oobe-field">Genre<input className="input" maxLength={80} value={genre} onChange={(e) => setGenre(e.target.value)} /></label>
        <label className="oobe-field">…or an M3U or PLS playlist<input className="input" type="file" accept=".m3u,.m3u8,.pls,text/plain" onChange={(e) => readFile(e.target.files?.[0])} /></label>
        <textarea className="input" rows={3} aria-label="Playlist text" placeholder="…or paste the playlist here" value={playlist} onChange={(e) => setPlaylist(e.target.value)} />
        <p className="muted" style={{ fontSize: 13 }}>Only addresses on the internet work: the server won't reach into your own network on a station's behalf.</p>
        {err && <div className="error" role="alert">{err}</div>}
        <div className="actions"><button type="button" className="btn ghost" onClick={onClose}>Cancel</button><button className="btn primary" disabled={!url.trim() && !playlist.trim()}>Add</button></div>
      </form>
    </div>
  )
}
