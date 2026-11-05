import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, fmtTime, type Item } from '../../api'
import { useAudio } from '../../audio/player'
import { IconAudio, IconChannels, IconPlay } from '../../icons'
import { toast } from '../../state'
import { Dialog } from '../../ui/Overlay'
import { Button } from '../../ui/Button'
import { albumLink, artistLink } from './parts'
import { howWords, placeIn, useMusicLinks, type MusicLink, type MusicLinkSide } from './links'

// Going between a song and its music video: Watch the video from a song, Play the song from
// a video, at the same place when the two line up. Admins tie and untie them here too.

// Watch a song's video. If the song is the one playing, it stops there and the video takes
// over at the same place.
export function useWatchVideo() {
  const audio = useAudio()
  const nav = useNavigate()
  return (songId: number, link: MusicLink) => {
    let at: number | undefined
    if (audio.current?.id === songId) {
      at = placeIn('video', audio.position(), link, link.video.runtimeSec)
      if (audio.playing) audio.toggle()
    }
    nav(`/watch/${link.videoId}${at ? `?t=${Math.floor(at)}` : ''}`)
  }
}

// In a song's menu: Watch the video, and for admins, tying it to one (onTie opens the
// dialog, which lives outside the menu so closing the menu doesn't take it away).
export function SongVideoItems({ t, close, onTie }: { t: Item; close: () => void; onTie: () => void }) {
  const links = useMusicLinks(t.id)
  const watch = useWatchVideo()
  if (!links || links.role !== 'song') return null
  return <>
    {links.tied.map((l, i) => (
      <button key={l.videoId} type="button" role="menuitem" onClick={() => { watch(t.id, l); close() }}>
        <IconChannels size={18} /> {i === 0 ? 'Watch the video' : `Watch "${l.video.title}"`}
      </button>
    ))}
    {links.canEdit && <button type="button" role="menuitem" onClick={() => { onTie(); close() }}>{links.tied.length ? 'Music videos of this song…' : links.suggested.length ? `Tie to a music video (${links.suggested.length} suggested)…` : 'Tie to a music video…'}</button>}
  </>
}

// On a music video's page: the song it's tied to, with Play the song and its album and
// artist. videoTime says where the video is.
export function VideoSongPanel({ videoId, videoTime, onPlaySong }: { videoId: number; videoTime: () => number; onPlaySong: () => void }) {
  const [reload, setReload] = useState(0)
  const links = useMusicLinks(videoId, reload)
  const audio = useAudio()
  const [tying, setTying] = useState(false)
  if (!links || links.role !== 'video') return null
  const l = links.tied[0]
  const play = async () => {
    try {
      const it = await api.get<Item>(`/items/${l.songId}`)
      const at = placeIn('song', videoTime(), l, l.song.runtimeSec)
      onPlaySong()
      audio.play([it], 0, 'music', at)
    } catch (e) { toast((e as Error).message) }
  }
  return (
    <section className="mv-song" aria-label="The song">
      {l ? <>
        <span className="mv-song-icon" aria-hidden="true"><IconAudio /></span>
        <span className="mv-song-text">
          <b>{l.song.title}</b>
          <span className="muted">
            <Link to={artistLink(l.song.libraryId, l.song.artist)}>{l.song.artist || 'Unknown artist'}</Link>
            {l.song.album && <> · <Link to={albumLink(l.song.libraryId, { artist: l.song.artist, title: l.song.album })}>{l.song.album}</Link></>}
          </span>
        </span>
        <Button variant="primary" size="sm" icon={<IconPlay size={16} />} onClick={() => void play()}>Play the song</Button>
      </> : <span className="muted mv-song-text">{links.canEdit ? (links.suggested.length ? `This might be ${links.suggested.map((s) => `"${s.song.title}"`).join(' or ')} in your music.` : 'Not tied to a song in your music.') : ''}</span>}
      {links.canEdit && <Button variant="ghost" size="sm" onClick={() => setTying(true)}>{l ? 'Change' : 'Tie to a song…'}</Button>}
      {tying && <TieDialog item={{ id: videoId, title: '' }} role="video" onClose={() => { setTying(false); setReload((n) => n + 1) }} />}
    </section>
  )
}

// Choosing the other end: what's tied, what's suggested, and a search. Admins only.
export function TieDialog({ item, role, onClose }: { item: { id: number; title: string }; role: 'song' | 'video'; onClose: () => void }) {
  const [reload, setReload] = useState(0)
  const links = useMusicLinks(item.id, reload)
  const [q, setQ] = useState('')
  const [found, setFound] = useState<MusicLinkSide[] | null>(null)
  const [busy, setBusy] = useState(false)
  const other = role === 'song' ? 'video' : 'song'
  useEffect(() => {
    const t = setTimeout(() => {
      api.get<MusicLinkSide[]>(`/music/links/${item.id}/candidates?q=${encodeURIComponent(q.trim())}`).then(setFound).catch(() => setFound([]))
    }, q ? 250 : 0)
    return () => clearTimeout(t)
  }, [item.id, q])
  const act = async (f: () => Promise<unknown>, done: string) => {
    setBusy(true)
    try { await f(); toast(done); setReload((n) => n + 1) } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  const pair = (otherId: number) => role === 'song' ? { songId: item.id, videoId: otherId } : { songId: otherId, videoId: item.id }
  const tie = (otherId: number) => act(() => api.put('/music/links', pair(otherId)), 'Tied')
  const untie = (l: MusicLink) => act(() => api.del(`/music/links?song=${l.songId}&video=${l.videoId}`), 'Untied')
  const tiedIds = new Set((links?.tied || []).map((l) => (role === 'song' ? l.videoId : l.songId)))
  return (
    <Dialog title={role === 'song' ? 'Music videos of this song' : 'The song of this video'} onClose={onClose} size="wide"
      description={role === 'song' ? 'A song can have several videos, such as a live one. What you tie or untie here stays that way, whatever later scans find.' : 'A video has one song: tying another unties this one. What you tie or untie here stays that way, whatever later scans find.'}
      actions={<Button onClick={onClose}>Done</Button>}>
      {!links ? <p className="muted">Loading…</p> : <>
        {links.tied.length > 0 && <>
          <h3 className="mv-tie-head">Tied</h3>
          <ul className="mv-tie-list">{links.tied.map((l) => <TiedRow key={l.songId + ':' + l.videoId} l={l} other={other} busy={busy} onUntie={() => void untie(l)} onSaved={() => setReload((n) => n + 1)} />)}</ul>
        </>}
        {links.suggested.length > 0 && <>
          <h3 className="mv-tie-head">Suggested</h3>
          <ul className="mv-tie-list">{links.suggested.map((l) => (
            <li key={l.songId + ':' + l.videoId}>
              <Side s={l[other]} kind={other} /><span className="muted mv-tie-why">{howWords[l.how]}</span>
              <span className="mv-tie-acts">
                <Button size="sm" variant="primary" busy={busy} onClick={() => void tie(other === 'video' ? l.videoId : l.songId)}>Tie</Button>
                <Button size="sm" variant="ghost" busy={busy} onClick={() => void untie(l)}>Not this one</Button>
              </span>
            </li>
          ))}</ul>
        </>}
        <h3 className="mv-tie-head">{role === 'song' ? 'Find a video' : 'Find a song'}</h3>
        <input className="input mv-tie-find" type="search" aria-label={role === 'song' ? 'Find a music video' : 'Find a song'} placeholder="Title or artist…" value={q} onChange={(e) => setQ(e.target.value)} />
        {found && found.length === 0 && <p className="muted">Nothing found.</p>}
        <ul className="mv-tie-list">{(found || []).filter((f) => !tiedIds.has(f.id)).map((f) => (
          <li key={f.id}><Side s={f} kind={other} /><span className="mv-tie-acts"><Button size="sm" busy={busy} onClick={() => void tie(f.id)}>Tie</Button></span></li>
        ))}</ul>
      </>}
    </Dialog>
  )
}

function Side({ s, kind }: { s: MusicLinkSide; kind: 'song' | 'video' }) {
  return (
    <span className="mv-tie-side">
      <b>{s.title}</b>
      <span className="muted">{[s.artist, kind === 'song' ? s.album : 'Music video', s.runtimeSec ? fmtTime(s.runtimeSec) : ''].filter(Boolean).join(' · ')}</span>
    </span>
  )
}

// A tie, with where the song starts in the video.
function TiedRow({ l, other, busy, onUntie, onSaved }: { l: MusicLink; other: 'song' | 'video'; busy: boolean; onUntie: () => void; onSaved: () => void }) {
  const [sync, setSync] = useState(l.syncMs !== undefined ? String(l.syncMs / 1000) : '')
  const save = async () => {
    const v = sync.trim()
    const n = Number(v)
    if (v !== '' && !Number.isFinite(n)) { toast('That isn\'t a number of seconds'); return }
    try {
      await api.put('/music/links', v === '' ? { songId: l.songId, videoId: l.videoId, noSync: true } : { songId: l.songId, videoId: l.videoId, syncMs: Math.round(n * 1000) })
      toast('Saved'); onSaved()
    } catch (e) { toast((e as Error).message) }
  }
  const diff = l.video.runtimeSec - l.song.runtimeSec
  return (
    <li className="mv-tie-tied">
      <Side s={l[other]} kind={other} /><span className="muted mv-tie-why">{howWords[l.how]}{l.by ? ` by ${l.by}` : ''}</span>
      <label className="mv-tie-sync">
        <span>The song starts this many seconds into the video</span>
        <input className="input" inputMode="decimal" value={sync} placeholder="They don't line up" onChange={(e) => setSync(e.target.value)} onBlur={() => { if (sync !== (l.syncMs !== undefined ? String(l.syncMs / 1000) : '')) void save() }} />
        {Math.abs(diff) > 2 && <small className="muted">The video is {fmtTime(Math.abs(diff))} {diff > 0 ? 'longer' : 'shorter'}. Leave this empty when the two are different takes.</small>}
      </label>
      <span className="mv-tie-acts"><Button size="sm" variant="ghost" busy={busy} onClick={onUntie}>Untie</Button></span>
    </li>
  )
}

