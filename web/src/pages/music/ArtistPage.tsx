import { useEffect, useMemo, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, fmtRuntime, type Album, type Item, type Library } from '../../api'
import { useArtColour } from '../../audio/art'
import { IconStation } from '../../audio/icons'
import { useAudio } from '../../audio/player'
import { BackLink, EmptyState, Spinner } from '../../components/common'
import { IconAudio, IconPlay } from '../../icons'
import { useApp } from '../../state'
import { TrackList } from '../AudioLibrary'
import { AlbumTile, artistPhoto, Shelf, shuffledCopy } from './parts'

// One artist or author: a photo if there is one, a biography if the owner turned that on,
// the songs they've played most, and their albums. Singles and EPs by size, as tags rarely say.

type ArtistInfo = { name: string; albums: Album[]; tracks: Item[]; top: Item[]; genres: string[]; online: boolean; bio?: string; bioUrl?: string; bioTitle?: string }

export default function ArtistPage() {
  const { lib: libId } = useParams()
  const [params] = useSearchParams()
  const name = params.get('name') ?? ''
  const { libraries, user } = useApp()
  const lib: Library | undefined = libraries.find((l) => String(l.id) === libId)
  const [info, setInfo] = useState<ArtistInfo | null>(null)
  const [failed, setFailed] = useState(false)
  const [order, setOrder] = useState<'year' | 'title'>('year')
  const audio = useAudio()
  const numericLib = Number(libId)
  useEffect(() => {
    setInfo(null)
    setFailed(false)
    api.get<ArtistInfo>(`/music/artist?library=${libId}&name=${encodeURIComponent(name)}`).then(setInfo).catch(() => setFailed(true))
  }, [libId, name])
  const photo = artistPhoto(numericLib, name)
  const rgb = useArtColour(photo)
  const sorted = useMemo(() => {
    const list = [...(info?.albums || [])]
    return order === 'year' ? list.sort((a, b) => (b.year || 0) - (a.year || 0)) : list.sort((a, b) => a.title.localeCompare(b.title))
  }, [info, order])
  if (failed) return <EmptyState icon={<IconAudio />} title="Nobody by that name here" text="They may have been moved or renamed." />
  if (!info) return <Spinner />
  const books = lib?.type === 'audiobooks'
  const mode = books ? 'audiobooks' : 'music'
  const total = info.tracks.reduce((s, t) => s + t.runtimeSec, 0)
  // Few songs to an album means it's most likely a single or an EP.
  const full = sorted.filter((a) => a.tracks >= 7 || books)
  const small = sorted.filter((a) => a.tracks < 7 && !books)
  const top = info.top.length ? info.top : info.tracks.slice(0, 5)
  return (
    <div className="album-page mh-artist-page" style={{ '--art': rgb } as React.CSSProperties}>
      <div className="mh-bg" aria-hidden="true" />
      <BackLink to={`/library/${libId}?tab=artists`} label={books ? 'Authors' : 'Artists'} />
      <header className="mh-artist-head">
        <span className="mh-artist-photo big"><span className="mh-initial" aria-hidden="true">{(name || '?')[0]}</span><img src={photo} alt="" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} /></span>
        <div className="album-meta">
          <span className="muted">{books ? 'Author' : 'Artist'}</span>
          <h1>{name || 'Unknown'}</h1>
          <div className="muted">{[`${info.albums.length} ${books ? (info.albums.length === 1 ? 'book' : 'books') : info.albums.length === 1 ? 'album' : 'albums'}`, `${info.tracks.length} ${info.tracks.length === 1 ? 'song' : 'songs'}`, fmtRuntime(total), info.genres.slice(0, 3).join(', ')].filter(Boolean).join(' · ')}</div>
          <div className="actions">
            <button className="btn primary" onClick={() => audio.play(info.tracks, 0, mode)}><IconPlay /> Play</button>
            {!books && <button className="btn" onClick={() => audio.play(shuffledCopy(info.tracks), 0, mode)}>Shuffle</button>}
            {!books && info.tracks[0] && <button className="btn ghost" onClick={() => audio.startRadio(info.tracks[0])}><IconStation size={18} /> Station</button>}
          </div>
          {info.bio && <p className="mh-bio">{info.bio}</p>}
          {info.bio && info.bioUrl && <p className="mh-credit">From <a href={info.bioUrl} target="_blank" rel="noreferrer">Wikipedia</a>, available under CC BY-SA 4.0. It's the article with this name, which may not always be the right person.</p>}
          {!info.online && !info.bio && user?.isAdmin && <p className="mh-credit">Biographies and photos can come from Wikipedia, if you turn on <Link to="/settings/metadata#lyrics">online music information</Link>. An <code>artist.jpg</code> in their folder is used first.</p>}
        </div>
      </header>
      {top.length > 0 && !books && <Shelf title={info.top.length ? 'Your most played' : 'Songs'}><TrackList items={top} showAlbum onPlay={(i) => audio.play(top, i, mode)} /></Shelf>}
      <div className="mh-shelf-head mh-disco-head">
        <h2>{books ? 'Books' : 'Discography'}</h2>
        <select className="select" value={order} aria-label="Sort albums" onChange={(e) => setOrder(e.target.value as 'year' | 'title')}><option value="year">Newest first</option><option value="title">By title</option></select>
      </div>
      <div className="album-grid">{full.map((a) => <AlbumTile key={a.title} libId={numericLib} a={a} book={books} noArtist />)}</div>
      {small.length > 0 && <Shelf title="Singles and EPs"><div className="album-grid">{small.map((a) => <AlbumTile key={a.title} libId={numericLib} a={a} noArtist />)}</div></Shelf>}
    </div>
  )
}
