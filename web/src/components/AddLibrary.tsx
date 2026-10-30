import { useState } from 'react'
import { api, type Library } from '../api'
import { IconFolder, libIcon } from '../icons'
import { PodcastFinder, PodcastWays } from '../pages/Podcasts'
import { ChooseFolderButton } from './FolderPicker'

// Adding a library in two steps: pick what kind of media it holds, then the
// folder it's in. Used by the setup wizard and the empty home page.

export const LIB_TYPES = [
  { type: 'movies', name: 'Movies', hint: 'Film files. DVD and Blu-ray images of films can go here too, and play with their menus' },
  { type: 'shows', name: 'TV shows', hint: 'One folder per show, with season folders inside. Disc images of a series are welcome too' },
  { type: 'discs', name: 'DVDs & Blu-rays', hint: 'A shelf just for disc images and folders, films and TV series together, played with their real menus' },
  { type: 'anime', name: 'Anime', hint: 'Series with AniList information; also collects anime found in your TV shows' },
  { type: 'music', name: 'Music', hint: 'Albums by artist; tags and covers are read from the files' },
  { type: 'audiobooks', name: 'Audiobooks', hint: 'One folder per book; picks up where you stopped' },
  { type: 'podcasts', name: 'Podcasts', hint: 'Follow shows by RSS feed or search a free directory; add episode files you already have' },
  { type: 'books', name: 'Books & comics', hint: 'EPUB, PDF, comics and manga, with highlights, notes and pen' },
  { type: 'videos', name: 'Home videos', hint: 'Your own recordings; nothing is looked up online' },
]

// What each kind of library expects in its folder, shown once it's chosen.
const FOLDER_HINT: Record<string, string> = {
  movies: 'Disc images (.iso) and VIDEO_TS or BDMV folders in here are listed as DVDs and Blu-rays, and you can mark each one as a film or a TV series.',
  shows: 'Disc images of a series are listed with the show’s discs and keep their menus.',
  discs: 'Only DVD and Blu-ray images and folders are picked up. Each disc is sorted as a film or a TV series from its name; you can change that on the disc’s page.',
}

export default function AddLibrary({ onAdded, compact }: { onAdded: (l: Library) => void; compact?: boolean }) {
  const [type, setType] = useState<string | null>(null)
  const [paths, setPaths] = useState<string[]>([])
  const [name, setName] = useState('')
  const [manual, setManual] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [following, setFollowing] = useState<Library | null>(null) // a new podcasts library
  const t = LIB_TYPES.find((x) => x.type === type)
  const pods = t?.type === 'podcasts'
  const addPath = (p: string) => { if (p.trim()) setPaths((x) => [...new Set([...x, p.trim()])]); setManual('') }
  const reset = () => { setType(null); setPaths([]); setName('') }
  const add = async () => {
    setBusy(true); setErr('')
    try {
      const lib = await api.post<Library>('/libraries', { name: name.trim() || t!.name, type, paths })
      if (pods) setFollowing(lib) // straight on to following shows
      else { onAdded(lib); reset() }
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }

  if (following) {
    return (
      <div className="oobe-lib">
        <div className="oobe-lib-head"><span className="ic">{libIcon('podcasts')}</span><b>Follow some shows</b></div>
        <PodcastFinder libraryId={following.id} onAdded={() => {}} />
        <p className="muted pod-note" style={{ marginTop: 10 }}>
          {following.paths.length
            ? <>Episodes stream until you save them. On a show’s page, <b>Keep downloaded</b> saves its newest episodes to <b>{following.paths[0]}</b> each time its feed updates.</>
            : <>Episodes stream from the web. To download them, add a folder to this library later in Settings, Libraries.</>}
        </p>
        <div className="oobe-actions"><span /><button type="button" className="btn primary" onClick={() => { onAdded(following); setFollowing(null); reset() }}>Done</button></div>
      </div>
    )
  }
  if (!t) {
    return (
      <div className={'oobe-types' + (compact ? ' compact' : '')}>
        {LIB_TYPES.map((x) => (
          <button key={x.type} type="button" className="oobe-type" onClick={() => { setType(x.type); setName('') }}>
            <span className="ic">{libIcon(x.type)}</span><b>{x.name}</b><span className="muted">{x.hint}</span>
          </button>
        ))}
      </div>
    )
  }
  return (
    <div className="oobe-lib">
      <div className="oobe-lib-head"><span className="ic">{libIcon(t.type)}</span><b>{t.name}</b><button type="button" className="linkish" onClick={() => { setType(null); setPaths([]) }}>Choose another kind</button></div>
      {pods && <PodcastWays />}
      <label className="oobe-field">Library name<input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder={t.name} /></label>
      {pods && <p className="pod-folder"><b>Folder for episode files</b> <span className="muted">optional: episodes you already have, and where downloads are saved. Leave it empty to only stream.</span></p>}
      <div className="oobe-paths">
        {paths.map((p) => <div key={p} className="oobe-path"><IconFolder /> <span>{p}</span><button type="button" className="btn sm ghost" onClick={() => setPaths(paths.filter((x) => x !== p))} aria-label={`Remove ${p}`}>Remove</button></div>)}
      </div>
      <div className="field-row">
        <ChooseFolderButton className={'btn ' + (paths.length || pods ? '' : 'primary')} label={paths.length ? 'Add another folder…' : 'Choose folder…'} onPick={addPath} />
        <input className="input" placeholder={`…or type a path, e.g. ${pods ? 'D:\\Podcasts' : 'D:\\Movies'}`} aria-label="Folder path" value={manual} onChange={(e) => setManual(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addPath(manual) } }} />
      </div>
      {FOLDER_HINT[t.type] && <p className="muted oobe-hint" style={{ marginTop: 10 }}>{FOLDER_HINT[t.type]}</p>}
      {err && <div className="error" role="alert">{err}</div>}
      <div className="oobe-actions"><span /><button type="button" className="btn primary" disabled={(!paths.length && !pods) || busy} onClick={add}>{busy ? 'Adding…' : pods ? 'Add, then follow shows' : `Add ${name.trim() || t.name}`}</button></div>
    </div>
  )
}
