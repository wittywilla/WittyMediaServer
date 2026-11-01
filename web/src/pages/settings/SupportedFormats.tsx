import { useEffect, useMemo, useState } from 'react'
import { api } from '../../api'
import { HelpLink } from '../../components/HelpLink'
import { Input } from '../../ui'

// Every file type the server knows, and what happens with it. All of it comes from the
// server's own list (GET /media-types), with what FFmpeg on this machine says added, so
// this can't say anything the server doesn't do.

type Format = {
  id: string; name: string; category: string; extensions: string[] | null; playsText: string; needs: string; notes?: string; available: boolean; folder?: boolean
}
type Codec = { id: string; name: string; kind: string }
type Data = { formats: Format[]; codecs: Codec[]; ffmpeg?: string }

const GROUPS: [string, string][] = [
  ['video', 'Video'], ['audio', 'Music and audio'], ['subtitle', 'Subtitles'], ['photo', 'Photos'],
  ['book', 'Books'], ['comic', 'Comics'], ['disc', 'Discs'], ['playlist', 'Playlists'], ['sidecar', 'Files beside your media'],
]

function matches(f: Format, q: string) {
  if (!q) return true
  const dot = q.startsWith('.') ? q : '.' + q
  return f.name.toLowerCase().includes(q) || (f.extensions || []).some((e) => e === dot || e.includes(q))
}

export default function SupportedFormats() {
  const [data, setData] = useState<Data | null>(null)
  const [failed, setFailed] = useState(false)
  const [find, setFind] = useState('')
  useEffect(() => { api.get<Data>('/media-types').then(setData).catch(() => setFailed(true)) }, [])
  const q = find.trim().toLowerCase()
  const groups = useMemo(() => GROUPS.map(([id, title]) => ({ id, title, rows: (data?.formats || []).filter((f) => f.category === id && matches(f, q)) })).filter((g) => g.rows.length > 0), [data, q])
  if (failed) return <p className="muted" role="alert">The list of formats couldn’t be loaded.</p>
  if (!data) return <p className="muted">Loading the list of formats…</p>
  const missing = data.formats.filter((f) => !f.available)
  return (
    <div className="formats">
      <p>
        A file’s content decides what it is, not its name. The server plays a file directly whenever your player can, rewraps it when
        only the wrapper is the trouble, converts the sound alone when only that is, and converts the picture only when nothing simpler would play.
      </p>
      {missing.length > 0 && (
        <p className="formats-missing">
          This server’s FFmpeg can’t handle {missing.length === 1 ? 'this one' : `these ${missing.length}`}: {missing.map((f) => f.name).join(', ')}.
          They’re listed below, marked “needs more”, and their files stay in your library.
        </p>
      )}
      <p className="muted"><HelpLink to="media-types">What each media type supports</HelpLink></p>
      {data.ffmpeg !== undefined && <p className="muted formats-ffmpeg">{data.ffmpeg || 'FFmpeg wasn’t found.'}</p>}
      <label className="formats-find">
        <span className="sr-only">Find a format or extension</span>
        <Input type="search" placeholder="Find a format or extension" value={find} onChange={(e) => setFind(e.target.value)} />
      </label>
      {groups.length === 0 && <p className="muted" role="status">Nothing here matches “{find}”. A file of a kind this server doesn’t list may still be recognised by its content.</p>}
      {groups.map((g) => (
        <section key={g.id} aria-labelledby={'formats-' + g.id}>
          <h3 id={'formats-' + g.id} className="formats-head">{g.title}</h3>
          <div className="formats-table" role="table" aria-labelledby={'formats-' + g.id}>
            <div className="formats-row formats-title" role="row">
              <span role="columnheader">Format</span><span role="columnheader">Extensions</span><span role="columnheader">How it plays</span><span role="columnheader">Needs</span>
            </div>
            {g.rows.map((f) => (
              <div className="formats-row" role="row" key={f.id}>
                <span role="cell"><b>{f.name}</b>{f.notes && <small className="muted">{f.notes}</small>}</span>
                <span role="cell" className="formats-exts">{f.folder ? 'A folder' : (f.extensions || []).join(' ')}</span>
                <span role="cell">{f.playsText}</span>
                <span role="cell">{f.available ? f.needs : <b className="formats-needs">Needs more: {f.needs}</b>}</span>
              </div>
            ))}
          </div>
        </section>
      ))}
      <section aria-labelledby="formats-codecs">
        <h3 id="formats-codecs" className="formats-head">Codecs the server knows by name</h3>
        {[['video', 'Video'], ['audio', 'Audio'], ['subtitle', 'Subtitle tracks']].map(([kind, label]) => (
          <p key={kind}><b>{label}: </b>{[...new Set(data.codecs.filter((c) => c.kind === kind).map((c) => c.name))].join(', ')}</p>
        ))}
      </section>
    </div>
  )
}
