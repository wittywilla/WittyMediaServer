import { useEffect, useState } from 'react'
import { api, type Library } from '../api'
import { IconFolder, libIcon } from '../icons'
import { PodcastFinder, PodcastWays } from '../pages/Podcasts'
import { ChooseFolderButton } from './FolderPicker'
import { BiosHint, ConsolePicker } from '../pages/Games'

// Adding a library in two steps: pick what kind of media it holds, then the
// folder it's in. Used by the setup wizard and the empty home page.

export const LIB_TYPES = [
  { type: 'movies', name: 'Movies', hint: 'Movie files. DVD and Blu-ray images of movies can go here too, and play with their menus' },
  { type: 'shows', name: 'TV shows', hint: 'One folder per show, with season folders inside. Disc images of a series are welcome too' },
  { type: 'discs', name: 'DVDs & Blu-rays', hint: 'A shelf just for disc images and folders, movies and TV series together, played with their real menus' },
  { type: 'anime', name: 'Anime', hint: 'Series with AniList information; also collects anime found in your TV shows' },
  { type: 'music', name: 'Music', hint: 'Albums by artist; tags and covers are read from the files' },
  { type: 'audiobooks', name: 'Audiobooks', hint: 'One folder per book; picks up where you stopped' },
  { type: 'podcasts', name: 'Podcasts', hint: 'Follow shows by RSS feed or search a free directory; add episode files you already have' },
  { type: 'books', name: 'Books & comics', hint: 'EPUB, PDF, comics and manga, with highlights, notes and pen' },
  { type: 'videos', name: 'Home videos', hint: 'Your own recordings; nothing is looked up online' },
  { type: 'photos', name: 'Photos', hint: 'A timeline, albums and a map of your pictures and clips, RAW files too; nothing leaves this server' },
  { type: 'shorts', name: 'Vertical videos', hint: 'A feed of short videos with likes, comments and a For You page worked out on this server alone' },
  { type: 'musicvideos', name: 'Music videos & concerts', hint: 'Music videos and live recordings by artist, with setlists from their chapters' },
  { type: 'radio', name: 'Internet radio', hint: 'Stations you add by their stream address or an M3U or PLS playlist; no folder needed' },
  { type: 'channels', name: 'Creator videos', hint: 'Your archive of creators’ videos, laid out by channel with subscriptions, comments and up next' },
  { type: 'commercials', name: 'Commercials', hint: 'Your own ads and bumpers to play between shows on TV channels. Have as many of these as you like' },
  { type: 'games', name: 'Games', hint: 'Your own game files, by console, played right in the browser with a gamepad, keyboard or touch' },
]

// The kinds, by what you do with them, so the list can be scanned.
const LIB_GROUPS: [string, string[]][] = [
  ['Watch', ['movies', 'shows', 'discs', 'anime', 'videos', 'musicvideos', 'channels', 'shorts']],
  ['Listen', ['music', 'audiobooks', 'podcasts', 'radio']],
  ['Read and look', ['books', 'photos']],
  ['Play', ['games']],
  ['For your TV channels', ['commercials']],
]

// What each kind of library expects in its folder, shown once it's chosen.
const FOLDER_HINT: Record<string, string> = {
  movies: 'Disc images (.iso) and VIDEO_TS or BDMV folders in here are listed as DVDs and Blu-rays, and you can mark each one as a movie or a TV series.',
  shows: 'Disc images of a series are listed with the show’s discs and keep their menus.',
  discs: 'Only DVD and Blu-ray images and folders are picked up. Each disc is sorted as a movie or a TV series from its name; you can change that on the disc’s page.',
  shorts: 'One folder per creator works best. Sidecar files beside a video can add its creator, tags, date and where it came from.',
  musicvideos: 'Name files Artist - Title, or keep one folder per artist. An .nfo or sidecar beside a file adds more. Recordings of 20 minutes or more appear under Concerts, with their chapters as the setlist.',
  channels: 'One folder per channel, with subfolders as playlists. Sidecar files beside a video add its title, date, description and saved comments, and a picture with its name becomes its thumbnail. Nothing is downloaded or looked up.',
  photos: 'Every picture and clip in here and in folders below it. Thumbnails are made when first looked at, so even a large collection scans quickly. Your files are never changed; uploads are saved in an Uploads folder inside it.',
  games: 'Bring your own game files: nothing is bundled or downloaded. Zipped games work, and a .cue or .m3u keeps a multi-file game together. BIOS files go in a folder named BIOS beside the games.',
}

export type Grouping = 'combine' | 'separate'

// onPanel: told when a kind is chosen (true) and when the list is back (false).
// existing and grouping: a folder of a kind that already has a library joins
// it ("combine") or starts a library of its own ("separate"); either way the
// panel offers the other choice.
export default function AddLibrary({ onAdded, compact, onPanel, existing = [], grouping = 'combine' }: {
  onAdded: (l: Library) => void; compact?: boolean; onPanel?: (open: boolean) => void; existing?: Library[]; grouping?: Grouping
}) {
  const [type, setType] = useState<string | null>(null)
  const [paths, setPaths] = useState<string[]>([])
  const [name, setName] = useState('')
  const [manual, setManual] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [following, setFollowing] = useState<Library | null>(null) // a new podcasts library
  const [target, setTarget] = useState(0) // the library the folders join; 0 = a new one
  const [console_, setConsole] = useState('auto') // games: which console this library holds
  const [consoleName, setConsoleName] = useState('Games')
  const t = LIB_TYPES.find((x) => x.type === type)
  const same = existing.filter((l) => l.type === type && (type !== 'games' || (l.console || 'auto') === console_))
  const joining = same.find((l) => l.id === target)
  const kindLabel = type === 'games' ? consoleName : t?.name
  const newName = name.trim() || (same.length ? `${kindLabel} ${same.length + 1}` : kindLabel || '')
  const choose = (k: string) => {
    const s = existing.filter((l) => l.type === k)
    setType(k); setName(''); setTarget(grouping === 'combine' && s.length ? s[0].id : 0)
  }
  const pods = t?.type === 'podcasts'
  const noFolder = t?.type === 'radio'
  const open = !!t || !!following
  useEffect(() => { onPanel?.(open) }, [open, onPanel])
  const addPath = (p: string) => { if (p.trim()) setPaths((x) => [...new Set([...x, p.trim()])]); setManual('') }
  const reset = () => { setType(null); setPaths([]); setName('') }
  // A path typed but not yet confirmed with Enter counts too: Add must not stay
  // greyed out while the folder is sitting right there in the box.
  const allPaths = manual.trim() ? [...new Set([...paths, manual.trim()])] : paths
  const add = async () => {
    setBusy(true); setErr('')
    try {
      const lib = joining
        ? await api.put<Library>(`/libraries/${joining.id}`, { name: joining.name, type: joining.type, console: joining.console, paths: [...new Set([...joining.paths, ...allPaths])] })
        : await api.post<Library>('/libraries', { name: newName, type, paths: allPaths, console: type === 'games' ? console_ : undefined })
      setManual('')
      if (pods) setFollowing(lib) // straight on to following shows
      else { onAdded(lib); reset() }
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }

  if (following) {
    return (
      <div className="oobe-lib">
        <div className="oobe-lib-head"><span className="ic">{libIcon('podcasts')}</span><b>Follow some shows</b></div>
        <PodcastFinder libraryId={following.id} onAdded={() => {}} />
        <p className="muted pod-note oobe-hint">
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
      <div className="oobe-type-groups">
        {LIB_GROUPS.map(([title, kinds]) => (
          <section key={title}>
            <h3 className="oobe-type-head">{title}</h3>
            <div className={'oobe-types' + (compact ? ' compact' : '')}>
              {kinds.map((k) => LIB_TYPES.find((x) => x.type === k)!).map((x) => (
                <button key={x.type} type="button" className="oobe-type" onClick={() => choose(x.type)}>
                  <span className="ic">{libIcon(x.type)}</span><b>{x.name}</b><span className="muted">{x.hint}</span>
                </button>
              ))}
            </div>
          </section>
        ))}
      </div>
    )
  }
  return (
    <div className="oobe-lib">
      <div className="oobe-lib-head"><span className="ic">{libIcon(t.type)}</span><b>{t.name}</b></div>
      {pods && <PodcastWays />}
      {type === 'games' && <ConsolePicker value={console_} onChange={(id, label) => {
        setConsole(id); setConsoleName(label); setName('')
        const s = existing.filter((l) => l.type === 'games' && (l.console || 'auto') === id)
        setTarget(grouping === 'combine' && s.length ? s[0].id : 0)
      }} />}
      {type === 'games' && <BiosHint console={console_} />}
      {same.length > 0 && (
        <div className="lib-dest" role="radiogroup" aria-label="Where these folders go">
          <label className={'lib-dest-opt' + (joining ? ' on' : '')}>
            <input type="radio" name="lib-dest" checked={!!joining} onChange={() => setTarget(same[0].id)} />
            <span><b>Add to {same.length === 1 ? `“${same[0].name}”` : 'an existing library'}</b>
              <span className="muted">Its folders and these show up together as one library.</span></span>
          </label>
          {joining && same.length > 1 && (
            <select className="select" value={target} onChange={(e) => setTarget(Number(e.target.value))} aria-label="Library to add to">
              {same.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
            </select>
          )}
          <label className={'lib-dest-opt' + (!joining ? ' on' : '')}>
            <input type="radio" name="lib-dest" checked={!joining} onChange={() => setTarget(0)} />
            <span><b>Make a separate library</b>
              <span className="muted">With its own name and its own place in the menu, e.g. “Kids’ movies” or “4K movies”.</span></span>
          </label>
        </div>
      )}
      {!joining && (
        <label className="oobe-field">Library name
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder={same.length ? `e.g. ${kindLabel} ${same.length + 1}` : kindLabel} maxLength={60} />
          {same.some((l) => l.name.toLowerCase() === newName.toLowerCase()) && <span className="muted oobe-hint">You already have a library with this name; a different one makes them easier to tell apart.</span>}
        </label>
      )}
      {pods && <p className="pod-folder"><b>Folder for episode files</b> <span className="muted">optional: episodes you already have, and where downloads are saved. Leave it empty to only stream.</span></p>}
      {noFolder && <p className="muted oobe-hint">Stations aren't files, so this library needs no folder. Add them on its page once it's made.</p>}
      {!noFolder && <div className="oobe-paths">
        {paths.map((p) => <div key={p} className="oobe-path"><IconFolder /> <span>{p}</span><button type="button" className="btn sm ghost" onClick={() => setPaths(paths.filter((x) => x !== p))} aria-label={`Remove ${p}`}>Remove</button></div>)}
      </div>}
      {!noFolder && <div className="field-row">
        <ChooseFolderButton className={'btn ' + (paths.length || pods ? '' : 'primary')} label={paths.length ? 'Add another folder…' : 'Choose folder…'} onPick={addPath} />
        <input className="input" placeholder={`…or type a path, e.g. ${pods ? 'D:\\Podcasts' : type === 'games' ? 'D:\\Games' : 'D:\\Movies'}`} aria-label="Folder path" value={manual} onChange={(e) => setManual(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addPath(manual) } }} />
      </div>}
      {FOLDER_HINT[t.type] && <p className="muted oobe-hint">{FOLDER_HINT[t.type]}</p>}
      {err && <div className="error" role="alert">{err}</div>}
      <div className="oobe-actions">
        <button type="button" className="btn ghost" onClick={() => { setType(null); setPaths([]); setManual(''); setErr('') }}>Back</button>
        <button type="button" className="btn primary" disabled={(!allPaths.length && !pods && !noFolder) || busy} onClick={add}>{busy ? 'Adding…' : pods ? 'Add, then follow shows' : joining ? `Add to ${joining.name}` : `Add ${newName}`}</button></div>
    </div>
  )
}

const kindName = (type: string) => LIB_TYPES.find((x) => x.type === type)?.name || type

// LibrarySummary: how many libraries there are, and each one at a glance.
export function LibrarySummary({ libs, title = 'Your libraries' }: { libs: Library[]; title?: string }) {
  if (!libs.length) return null
  return (
    <section className="lib-summary" aria-label={title}>
      <div className="lib-summary-head"><b>{title} · {libs.length}</b></div>
      <ul className="lib-summary-list">
        {libs.map((l) => (
          <li key={l.id}>
            <span className="ic">{libIcon(l.type)}</span>
            <span><b>{l.name}</b><span className="muted">{l.name === kindName(l.type) ? '' : `${kindName(l.type)} · `}{l.paths.length ? `${l.paths.length} folder${l.paths.length === 1 ? '' : 's'}` : 'no folder'}</span></span>
          </li>
        ))}
      </ul>
    </section>
  )
}

// GroupingChoice: what happens to another folder of a kind you already have.
export function GroupingChoice({ value, onChange }: { value: Grouping; onChange: (g: Grouping) => void }) {
  return (
    <div className="lib-grouping">
      <span className="muted">Another folder of a kind you already have:</span>
      <div className="seg" role="radiogroup" aria-label="Another folder of the same kind">
        <button type="button" role="radio" aria-checked={value === 'combine'} className={value === 'combine' ? 'active' : ''} onClick={() => onChange('combine')}
          title="For example, every movies folder goes into one Movies library">Combine into one library</button>
        <button type="button" role="radio" aria-checked={value === 'separate'} className={value === 'separate' ? 'active' : ''} onClick={() => onChange('separate')}
          title="Each folder becomes its own library with its own name">Keep separate, with own names</button>
      </div>
    </div>
  )
}
