import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, artPreviewUrl, posterUrl, type Item, type Library } from '../api'
import { useAudio } from '../audio/player'
import { BackLink, CardRow, EmptyState, Placeholder, SkeletonGrid, Spinner } from '../components/common'
import { IconCheck, IconGlobe, IconPause, IconPlay, IconPlus, IconRefresh, IconSearch, IconStar, IconTrash, libIcon } from '../icons'
import { toast, useApp } from '../state'
import '../radio.css'
import LegalNote from '../components/LegalNote'

// Internet radio: stations from a free community directory or by address, played
// through the server. docs/COLLECTIONS.md has what contacts what.

type Station = Item & { radio: NonNullable<Item['radio']> }
type Hit = { id: string; name: string; url: string; homepage?: string; logo?: string; tags: string[]; country?: string; countryCode?: string
  language?: string; codec?: string; bitrate?: number; hls: boolean; working: boolean; added: boolean }
type Count = { name: string; code?: string; stations: number }
type Facets = { countries: Count[]; tags: Count[]; languages: Count[] }

const PAGE = 30

// "MP3 · 128 kbps"
const quality = (codec?: string, bitrate?: number) => [codec, bitrate ? `${bitrate} kbps` : ''].filter(Boolean).join(' · ')

// A station's picture: its logo if it has a usable one, otherwise art made from its
// name, as for anything without a picture. A logo that fails to load leaves the art.
function StationArt({ title, src }: { title: string; src?: string }) {
  return (
    <>
      <Placeholder title={title} />
      {src && <img src={src} alt="" loading="lazy" className="rd-logo" onError={(e) => { (e.target as HTMLImageElement).style.display = 'none' }} />}
    </>
  )
}

function usePlay(list: Station[]) {
  const audio = useAudio()
  return {
    audio,
    isOn: (s: Station) => audio.current?.id === s.id,
    isPlaying: (s: Station) => audio.current?.id === s.id && audio.playing,
    toggle: (s: Station) => {
      if (audio.current?.id === s.id) audio.toggle()
      else audio.play(list, Math.max(0, list.findIndex((x) => x.id === s.id)), 'radio')
    },
  }
}

function StationCard({ s, play }: { s: Station; play: ReturnType<typeof usePlay> }) {
  const dead = s.radio.status === 'dead'
  const facts = [s.genres[0], s.radio.country].filter(Boolean).join(' · ')
  return (
    <div className={'card square rd-card' + (play.isOn(s) ? ' on' : '')}>
      <Link to={`/radio/${s.id}`} className="rd-card-link" title={s.title}>
        <div className="poster">
          <StationArt title={s.title} src={s.poster ? posterUrl(s) : undefined} />
          {dead && <span className="badge rd-dead">Off the air</span>}
          {s.radio.favorite && <span className="badge seen rd-fav" role="img" aria-label="Favourite"><IconStar /></span>}
        </div>
        <div className="title">{s.title}</div>
        <div className="sub">{facts || 'Internet radio'}</div>
      </Link>
      <div className="rd-play-box">
        <button className="rd-play" aria-label={`${play.isPlaying(s) ? 'Pause' : 'Play'} ${s.title}`} onClick={() => play.toggle(s)}>
          {play.isPlaying(s) ? <IconPause /> : <IconPlay />}
        </button>
      </div>
    </div>
  )
}

export default function RadioLibrary({ lib }: { lib: Library }) {
  const { user } = useApp()
  const [stations, setStations] = useState<Station[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [adding, setAdding] = useState(false)
  const [params, setParams] = useSearchParams()
  const genre = params.get('genre') || ''
  const country = params.get('country') || ''
  const [text, setText] = useState('')
  const load = useCallback(() => {
    setFailed(false)
    api.get<Station[]>(`/radio?library=${lib.id}`).then(setStations).catch(() => setFailed(true))
  }, [lib.id])
  useEffect(load, [load])
  const filter = (key: string, v: string) => setParams((p) => { if (v) p.set(key, v); else p.delete(key); return p }, { replace: true })

  const all = stations || []
  const genres = useMemo(() => {
    // Stations spell genres differently ("Jazz", "jazz"): they count as one.
    const n = new Map<string, { name: string; count: number }>()
    for (const s of all) {
      for (const g of s.genres) {
        const e = n.get(g.toLowerCase())
        if (e) e.count++
        else n.set(g.toLowerCase(), { name: g, count: 1 })
      }
    }
    return [...n.values()].sort((a, b) => b.count - a.count || a.name.localeCompare(b.name)).slice(0, 16).map((g) => g.name)
  }, [stations]) // eslint-disable-line react-hooks/exhaustive-deps
  const countries = useMemo(() => [...new Set(all.map((s) => s.radio.country).filter(Boolean) as string[])].sort(), [stations]) // eslint-disable-line react-hooks/exhaustive-deps
  const needle = text.trim().toLowerCase()
  const shown = all.filter((s) => (!genre || s.genres.some((g) => g.toLowerCase() === genre.toLowerCase())) && (!country || s.radio.country === country) && (!needle || s.title.toLowerCase().includes(needle)))
    // What's off the air goes to the end, where it's seen but not in the way.
    .sort((a, b) => Number(a.radio.status === 'dead') - Number(b.radio.status === 'dead'))
  const browsing = !genre && !country && !needle
  const favourites = browsing ? all.filter((s) => s.radio.favorite) : []
  const recent = browsing ? all.filter((s) => s.radio.lastPlayed).sort((a, b) => (b.radio.lastPlayed || 0) - (a.radio.lastPlayed || 0)).slice(0, 12) : []
  const dead = all.filter((s) => s.radio.status === 'dead').length
  const play = usePlay(shown)
  const playFav = usePlay(favourites)
  const playRecent = usePlay(recent)

  const checkAll = async () => {
    try {
      const r = await api.post<{ stations: number }>('/radio/check', { libraryId: lib.id })
      toast(`Checking ${r.stations} ${r.stations === 1 ? 'station' : 'stations'}, one at a time. This page updates when you come back to it.`)
    } catch (e: any) { toast(e.message) }
  }

  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}</h1>
        {stations && <span className="count">{stations.length} {stations.length === 1 ? 'station' : 'stations'}</span>}
        <div className="grow" />
        {all.length > 8 && <input className="input rd-filter" type="search" placeholder="Find a station" aria-label="Find a station in this library" value={text} onChange={(e) => setText(e.target.value)} />}
        {user?.isAdmin && all.length > 0 && <button className="btn ghost" onClick={checkAll}><IconRefresh /> Check stations</button>}
        {user?.isAdmin && <button className="btn primary" onClick={() => setAdding(true)}><IconPlus /> Add stations</button>}
      </div>
      {!stations && !failed && <SkeletonGrid square />}
      {failed && <EmptyState icon={libIcon('radio')} title="The stations can't be loaded" text="The server didn't answer." action={<button className="btn" onClick={load}><IconRefresh /> Try again</button>} />}
      {stations && !all.length && (
        <EmptyState icon={libIcon('radio')} title="No stations yet"
          text={user?.isAdmin ? 'Search a free directory of stations, or add one by its stream address.' : 'An administrator can add stations to this library.'}
          action={user?.isAdmin ? <button className="btn primary" onClick={() => setAdding(true)}><IconPlus /> Add stations</button> : undefined} />
      )}
      {all.length > 0 && (
        <>
          {favourites.length > 0 && <section className="rd-row" aria-label="Favourites"><h2>Favourites</h2><CardRow>{favourites.map((s) => <StationCard key={s.id} s={s} play={playFav} />)}</CardRow></section>}
          {recent.length > 0 && <section className="rd-row" aria-label="Recently played"><h2>Recently played</h2><CardRow>{recent.map((s) => <StationCard key={s.id} s={s} play={playRecent} />)}</CardRow></section>}
          {(genres.length > 1 || countries.length > 1) && (
            <div className="chips rd-filters" role="group" aria-label="Browse by genre and country">
              {countries.length > 1 && (
                <select className="select sm" aria-label="Country" value={country} onChange={(e) => filter('country', e.target.value)}>
                  <option value="">All countries</option>
                  {countries.map((c) => <option key={c} value={c}>{c}</option>)}
                </select>
              )}
              {genres.length > 1 && <button className={'chip' + (!genre ? ' active' : '')} aria-pressed={!genre} onClick={() => filter('genre', '')}>All genres</button>}
              {genres.length > 1 && genres.map((g) => <button key={g} className={'chip' + (genre === g ? ' active' : '')} aria-pressed={genre === g} onClick={() => filter('genre', genre === g ? '' : g)}>{g}</button>)}
            </div>
          )}
          {browsing && dead > 0 && <p className="muted rd-note">{dead} {dead === 1 ? 'station is' : 'stations are'} off the air: {dead === 1 ? 'it' : 'they'} stopped answering. Open one to try again or remove it.</p>}
          {(favourites.length > 0 || recent.length > 0) && <h2 className="rd-all">All stations</h2>}
          {shown.length ? <div className="grid">{shown.map((s) => <StationCard key={s.id} s={s} play={play} />)}</div>
            : <EmptyState icon={<IconSearch />} title="No stations match" text="Try a different genre or country." action={<button className="btn" onClick={() => { setText(''); setParams({}, { replace: true }) }}>Show all stations</button>} />}
        </>
      )}
      <LegalNote kind="webMedia" />
      {adding && <AddStations lib={lib} onClose={() => setAdding(false)} onChanged={load} />}
    </>
  )
}

// ---------------------------------------------------------------------------
// Adding stations
// ---------------------------------------------------------------------------

function AddStations({ lib, onClose, onChanged }: { lib: Library; onClose: () => void; onChanged: () => void }) {
  const [mode, setMode] = useState<'search' | 'address' | 'playlist'>('search')
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal wide rd-add" onClick={(e) => e.stopPropagation()}>
        <h2>Add stations</h2>
        <div className="seg rd-modes" role="tablist" aria-label="How to add a station">
          <button role="tab" aria-selected={mode === 'search'} className={mode === 'search' ? 'active' : ''} onClick={() => setMode('search')}>Search</button>
          <button role="tab" aria-selected={mode === 'address'} className={mode === 'address' ? 'active' : ''} onClick={() => setMode('address')}>Stream address</button>
          <button role="tab" aria-selected={mode === 'playlist'} className={mode === 'playlist' ? 'active' : ''} onClick={() => setMode('playlist')}>Playlist</button>
        </div>
        {mode === 'search' && <DirectoryFinder lib={lib} onChanged={onChanged} />}
        {mode === 'address' && <AddressForm lib={lib} onDone={() => { onChanged(); onClose() }} />}
        {mode === 'playlist' && <PlaylistForm lib={lib} onDone={() => { onChanged(); onClose() }} />}
        <div className="actions"><button className="btn ghost" onClick={onClose}>Done</button></div>
      </div>
    </div>
  )
}

function DirectoryFinder({ lib, onChanged }: { lib: Library; onChanged: () => void }) {
  const [q, setQ] = useState('')
  const [tag, setTag] = useState('')
  const [country, setCountry] = useState('')
  const [language, setLanguage] = useState('')
  const [facets, setFacets] = useState<Facets | null>(null)
  const [hits, setHits] = useState<Hit[] | null>(null)
  const [more, setMore] = useState(false)
  const [busy, setBusy] = useState('')
  const [added, setAdded] = useState<string[]>([])
  const [err, setErr] = useState('')
  const [tries, setTries] = useState(0)
  const asked = useRef(0)
  useEffect(() => { api.get<Facets>('/radio/directory/browse').then(setFacets).catch(() => {}) }, [tries])
  const query = (offset: number) => {
    const p = new URLSearchParams({ library: String(lib.id), limit: String(PAGE), offset: String(offset) })
    if (q.trim()) p.set('q', q.trim())
    if (tag) p.set('tag', tag)
    if (country) p.set('country', country)
    if (language) p.set('language', language)
    return `/radio/directory/search?${p}`
  }
  useEffect(() => {
    const term = q.trim()
    if (term.length === 1) return
    const mine = ++asked.current
    // Only typing waits: choosing a filter asks straight away.
    const t = setTimeout(() => {
      setErr('')
      api.get<Hit[]>(query(0)).then((r) => { if (mine === asked.current) { setHits(r); setMore(r.length === PAGE) } })
        .catch((e) => { if (mine === asked.current) { setErr(e.message); setHits(null) } })
    }, term ? 350 : 0)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q, tag, country, language, tries])
  const loadMore = async () => {
    try {
      const r = await api.get<Hit[]>(query(hits?.length || 0))
      setHits((h) => [...(h || []), ...r.filter((x) => !(h || []).some((y) => y.id === x.id))]); setMore(r.length === PAGE)
    } catch (e: any) { setErr(e.message) }
  }
  const add = async (h: Hit) => {
    setBusy(h.id); setErr('')
    try {
      await api.post('/radio/directory/add', { libraryId: lib.id, id: h.id })
      setAdded((a) => [...a, h.id]); toast(`${h.name} added`); onChanged()
    } catch (e: any) { setErr(e.message) } finally { setBusy('') }
  }
  const filtered = !!(q.trim() || tag || country || language)
  return (
    <div className="rd-finder">
      <input className="input" type="search" autoFocus placeholder="Search by station name or genre" aria-label="Search stations" value={q} onChange={(e) => setQ(e.target.value)} />
      <div className="rd-facets">
        <label>Genre
          <select className="select" value={tag} onChange={(e) => setTag(e.target.value)}>
            <option value="">Any</option>
            {facets?.tags.map((t) => <option key={t.name} value={t.name}>{t.name}</option>)}
          </select>
        </label>
        <label>Country
          <select className="select" value={country} onChange={(e) => setCountry(e.target.value)}>
            <option value="">Any</option>
            {facets?.countries.map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}
          </select>
        </label>
        <label>Language
          <select className="select" value={language} onChange={(e) => setLanguage(e.target.value)}>
            <option value="">Any</option>
            {facets?.languages.map((l) => <option key={l.name} value={l.name}>{l.name}</option>)}
          </select>
        </label>
      </div>
      {err && <div className="error" role="alert">{err} <button className="btn sm" onClick={() => setTries((n) => n + 1)}>Try again</button></div>}
      {!err && !hits && <Spinner />}
      {hits && (
        <>
          <h3 className="rd-hits-title" aria-live="polite">{hits.length === 0 ? `No stations found${q.trim() ? ` for “${q.trim()}”` : ''}` : filtered ? 'Results' : 'Popular stations'}</h3>
          <ul className="rd-hits">
            {hits.map((h) => {
              const done = h.added || added.includes(h.id)
              return (
                <li key={h.id} className="rd-hit">
                  <span className="rd-hit-art"><StationArt title={h.name} src={h.logo ? artPreviewUrl(h.logo) : undefined} /></span>
                  <span className="rd-hit-text">
                    <b>{h.name}</b>
                    <span className="muted">{[h.tags.slice(0, 2).join(', '), h.country].filter(Boolean).join(' · ')}</span>
                    {quality(h.codec, h.bitrate) && <span className="muted rd-small">{quality(h.codec, h.bitrate)}</span>}
                  </span>
                  <button className={'btn sm ' + (done ? 'ghost' : 'primary')} disabled={done || !!busy} aria-label={`${done ? 'Added' : 'Add'} ${h.name}`} onClick={() => add(h)}>
                    {done ? <><IconCheck /> Added</> : busy === h.id ? 'Adding…' : 'Add'}
                  </button>
                </li>
              )
            })}
          </ul>
          {more && <button className="btn" onClick={loadMore}>Show more</button>}
        </>
      )}
      <p className="muted rd-note">Stations come from <a href="https://www.radio-browser.info/" target="_blank" rel="noreferrer noopener">Radio Browser</a>, a free community directory. What you search for is sent to it from your server. Stations themselves are contacted only when someone listens.</p>
      <LegalNote kind="webResults" />
    </div>
  )
}

function AddressForm({ lib, onDone }: { lib: Library; onDone: () => void }) {
  const [url, setUrl] = useState('')
  const [name, setName] = useState('')
  const [genre, setGenre] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const add = async (e: React.FormEvent) => {
    e.preventDefault(); setErr(''); setBusy(true)
    try { await api.post('/radio', { libraryId: lib.id, name, url, genre }); onDone() } catch (x: any) { setErr(x.message) } finally { setBusy(false) }
  }
  return (
    <form className="rd-form" onSubmit={add}>
      <label className="oobe-field">Stream address<input className="input" type="url" autoFocus required placeholder="https://…/stream" value={url} onChange={(e) => setUrl(e.target.value)} /></label>
      <div className="row2">
        <label className="oobe-field">Name<input className="input" maxLength={200} placeholder="Optional" value={name} onChange={(e) => setName(e.target.value)} /></label>
        <label className="oobe-field">Genre<input className="input" maxLength={80} placeholder="Optional" value={genre} onChange={(e) => setGenre(e.target.value)} /></label>
      </div>
      <p className="muted rd-note">Only addresses on the internet work: the server won't reach into your own network on a station's behalf. A station's name and details are filled in the first time it plays, if it gives them.</p>
      <LegalNote kind="webMedia" />
      {err && <div className="error" role="alert">{err}</div>}
      <button className="btn primary" disabled={!url.trim() || busy}>{busy ? 'Adding…' : 'Add station'}</button>
    </form>
  )
}

function PlaylistForm({ lib, onDone }: { lib: Library; onDone: () => void }) {
  const [playlist, setPlaylist] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const add = async (e: React.FormEvent) => {
    e.preventDefault(); setErr(''); setBusy(true)
    try {
      const r = await api.post<{ added: number }>('/radio/import', { libraryId: lib.id, playlist })
      if (!r.added) { setErr('No stations were found in that playlist.'); return }
      toast(`${r.added} ${r.added === 1 ? 'station' : 'stations'} added`); onDone()
    } catch (x: any) { setErr(x.message) } finally { setBusy(false) }
  }
  const readFile = async (f?: File) => { if (f) setPlaylist(await f.text()) }
  return (
    <form className="rd-form" onSubmit={add}>
      <label className="oobe-field">M3U or PLS playlist file<input className="input" type="file" accept=".m3u,.m3u8,.pls,text/plain" onChange={(e) => readFile(e.target.files?.[0])} /></label>
      <label className="oobe-field">…or paste it here<textarea className="input" rows={5} value={playlist} onChange={(e) => setPlaylist(e.target.value)} /></label>
      {err && <div className="error" role="alert">{err}</div>}
      <button className="btn primary" disabled={!playlist.trim() || busy}>{busy ? 'Adding…' : 'Add stations'}</button>
    </form>
  )
}

// ---------------------------------------------------------------------------
// A station's page
// ---------------------------------------------------------------------------

const when = (t?: number) => (t ? new Date(t * 1000).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' }) : '')

export function RadioStationPage() {
  const { id } = useParams()
  const { user, libraries } = useApp()
  const nav = useNavigate()
  const [s, setS] = useState<Station | null>(null)
  const [others, setOthers] = useState<Station[]>([])
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState('')
  const load = useCallback(() => {
    setErr('')
    api.get<Station>(`/radio/${id}`).then((st) => {
      setS(st)
      api.get<Station[]>(`/radio?library=${st.libraryId}`).then(setOthers).catch(() => {})
    }).catch((e) => setErr(e.message))
  }, [id])
  useEffect(load, [load])
  const row = useMemo(() => (s ? others.filter((o) => o.id !== s.id && o.genres.some((g) => s.genres.includes(g))).slice(0, 12) : []), [others, s])
  const play = usePlay(s ? [s] : [])
  const playOthers = usePlay(row)
  if (err) return <EmptyState icon={libIcon('radio')} title="This station can't be found" text={err} action={<Link className="btn" to="/">Go home</Link>} />
  if (!s) return <Spinner />
  const lib = libraries.find((l) => l.id === s.libraryId)
  const m = s.radio
  const on = play.isOn(s)
  const act = async (what: string, f: () => Promise<void>) => { setBusy(what); try { await f() } catch (e: any) { toast(e.message) } finally { setBusy('') } }
  const favourite = () => act('fav', async () => {
    await api.put(`/items/${s.id}/reaction`, { value: m.favorite ? 0 : 1 })
    setS({ ...s, radio: { ...m, favorite: !m.favorite } })
  })
  const check = () => act('check', async () => {
    const r = await api.post<Station>(`/radio/${s.id}/check`)
    setS(r); toast(r.radio.status === 'ok' ? 'The station answered' : r.radio.reason || 'The station did not answer')
  })
  const lookup = () => act('lookup', async () => { setS(await api.post<Station>(`/radio/${s.id}/lookup`)); toast('Details filled in from the directory') })
  const remove = () => act('remove', async () => {
    if (!confirm(`Remove ${s.title}?`)) return
    await api.del(`/radio/${s.id}`); nav(`/library/${s.libraryId}`, { replace: true })
  })
  const facts = [m.country, m.language, quality(m.codec, m.bitrate), m.hls ? 'HLS' : ''].filter(Boolean)
  return (
    <div className="rd-page">
      <BackLink to={`/library/${s.libraryId}`} label={lib?.name || 'Internet radio'} />
      <div className="rd-head">
        <div className="rd-head-art"><StationArt title={s.title} src={s.poster ? posterUrl(s) : undefined} /></div>
        <div className="rd-head-text">
          <h1>{s.title}</h1>
          {facts.length > 0 && <div className="meta">{facts.map((f) => <span key={f}>{f}</span>)}</div>}
          <div className="rd-actions">
            <button className="btn primary big" onClick={() => play.toggle(s)}>{play.isPlaying(s) ? <><IconPause /> Pause</> : <><IconPlay /> Play</>}</button>
            <button className={'btn big' + (m.favorite ? ' rd-faved' : '')} aria-pressed={m.favorite} disabled={busy === 'fav'} onClick={favourite}><IconStar /> {m.favorite ? 'Favourite' : 'Add to favourites'}</button>
            {m.homepage && <a className="btn big ghost" href={m.homepage} target="_blank" rel="noreferrer noopener"><IconGlobe /> Website</a>}
          </div>
          {on && <NowLine />}
        </div>
      </div>
      {m.status === 'dead' && (
        <div className="warn-box rd-status" role="status"><b>This station seems to be off the air.</b> {m.reason} {m.lastOk ? `It last answered on ${when(m.lastOk)}.` : ''}</div>
      )}
      {m.status === 'trouble' && <div className="warn-box rd-status" role="status"><b>This station had trouble lately.</b> {m.reason}</div>}
      {s.overview && <p className="rd-about">{s.overview}</p>}
      {s.genres.length > 0 && (
        <div className="chips" role="group" aria-label="Genres">
          {s.genres.map((g) => <Link key={g} className="chip" to={`/library/${s.libraryId}?genre=${encodeURIComponent(g)}`}>{g}</Link>)}
        </div>
      )}
      {user?.isAdmin && (
        <section className="panel rd-admin" aria-label="Manage this station">
          <h2>Manage</h2>
          <p className="panel-note">{[m.source ? 'From the Radio Browser directory.' : 'Added by its address.', m.lastCheck ? `Last tried ${when(m.lastCheck)}.` : 'Not tried yet.'].join(' ')}</p>
          {s.path && <p className="rd-address">{s.path}</p>}
          <div className="panel-actions">
            <button className="btn" disabled={!!busy} onClick={check}><IconRefresh /> {busy === 'check' ? 'Trying…' : 'Try it now'}</button>
            {!m.source && <button className="btn" disabled={!!busy} onClick={lookup}><IconSearch /> {busy === 'lookup' ? 'Looking…' : 'Find details in the directory'}</button>}
            <button className="btn danger" disabled={!!busy} onClick={remove}><IconTrash /> Remove</button>
          </div>
        </section>
      )}
      {row.length > 0 && <section className="rd-row" aria-label="Similar stations"><h2>More like this</h2><CardRow>{row.map((o) => <StationCard key={o.id} s={o} play={playOthers} />)}</CardRow></section>}
      <LegalNote kind="webMedia" />
    </div>
  )
}

// What's on now, from the player, which keeps it up to date.
function NowLine() {
  const { nowPlaying } = useAudio()
  if (!nowPlaying) return null
  return <p className="rd-now" aria-live="polite"><span className="muted">Now playing</span> {nowPlaying}</p>
}
