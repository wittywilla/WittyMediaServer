import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { PromptBar, useInputKind, type Prompt } from '@wittywilla/wittyplay/web/prompts'
import { api, backdropUrl, fmtDateTime, mediaUrl, uploadFile, type GameDetailsInfo, type Item } from '../../api'
import ArtworkPicker from '../../components/ArtworkPicker'
import { Placeholder, Spinner } from '../../components/common'
import LegalNote from '../../components/LegalNote'
import { IconCheck, IconDownload, IconEdit, IconGamepad, IconImage, IconInfo, IconMore, IconPlay, IconRefresh, IconSettings, IconStar, IconTrash, IconTune } from '../../icons'
import { toast, useApp } from '../../state'
import { ask, Dialog, Menu, type MenuEntry } from '../../ui'
import { GameCredits, GameMatchBar, GameRatings, GameShots } from '../GameMeta'
import { SettingsScreen, useConsoles, useSettingsHost } from '../Games'
import { crispIfSmall, fmtPlayed, heroCover, saveThumb, useCoverTint, useGameLinks, type GameCheck } from './data'
import { Ambient } from './Library'
import type { SaveSlot } from '@wittywilla/wittyplay'
import './games-ui.css'

// A game's page: what it is, a big button to play it, where you left off, and everything
// else about it below. Whatever's stopping a game from starting says so here, with the
// way to fix it, rather than waiting for someone to press Play.

// Only administrators ever see this, so nobody else downloads the settings code for it.
const GameCorePicker = lazy(() => import('../settings/GamingCustom').then((m) => ({ default: m.GameCorePicker })))

type GamePersonal = { favourite: boolean; note: string; plays: number; lastPlayed: number; cheats: { name?: string; enabled?: boolean }[]; hasBattery?: boolean }

const slotName = (s: number) => (s === 0 ? 'Automatic save' : s === 100 ? 'Before the last load' : s === 101 ? 'Before starting over' : `Slot ${s}`)

const fmtReleased = (d: string) => {
  const [y, m, day] = d.split('-').map(Number)
  if (!m) return String(y)
  return new Date(Date.UTC(y, m - 1, day || 1)).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: day ? 'numeric' : undefined, timeZone: 'UTC' })
}

const pickSaveFile = (accept: string) => new Promise<File | null>((resolve) => {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = accept
  input.onchange = () => resolve(input.files?.[0] || null)
  input.oncancel = () => resolve(null)
  input.click()
})

export default function GameDetails({ it, reload }: { it: Item; reload?: () => void }) {
  const links = useGameLinks()
  const nav = useNavigate()
  const { user, libraries } = useApp()
  const consoles = useConsoles()
  const input = useInputKind()
  const c = consoles.find((x) => x.id === it.console)
  const [saves, setSaves] = useState<SaveSlot[]>([])
  const [meta, setMeta] = useState<GameDetailsInfo | null>(null)
  const [check, setCheck] = useState<GameCheck | null>(null)
  const [mine, setMine] = useState<GamePersonal | null>(null)
  const [resumeMode, setResumeMode] = useState('ask')
  const [showSettings, setShowSettings] = useState(false)
  const [note, setNote] = useState('')
  const [longRead, setLongRead] = useState(false)
  const [edit, setEdit] = useState(false)
  const [art, setArt] = useState(false)
  const loadSaves = useCallback(() => api.get<SaveSlot[]>(`/games/${it.id}/saves`).then(setSaves).catch(() => {}), [it.id])
  const loadCheck = useCallback(() => api.get<GameCheck>(`/games/${it.id}/check`).then(setCheck).catch(() => {}), [it.id])
  const loadMeta = useCallback(() => api.get<GameDetailsInfo>(`/games/${it.id}/metadata`).then(setMeta).catch(() => {}), [it.id])
  useEffect(() => { void loadSaves(); void loadCheck() }, [loadSaves, loadCheck])
  useEffect(() => { void loadMeta() }, [loadMeta, it.updatedAt])
  useEffect(() => {
    api.get<GamePersonal>(`/games/${it.id}/meta`).then((m) => { setMine(m); setNote(m.note) }).catch(() => {})
    api.get<{ resolved: { values: Record<string, unknown> } }>(`/games/settings?item=${it.id}`).then((v) => setResumeMode(String(v.resolved.values['states.resume'] || 'ask'))).catch(() => {})
  }, [it.id])
  // Still being looked up: show what's found as it arrives.
  useEffect(() => {
    if (meta?.status !== 'pending') return
    const t = setTimeout(() => { void loadMeta(); reload?.() }, 6000)
    return () => clearTimeout(t)
  }, [meta]) // eslint-disable-line react-hooks/exhaustive-deps
  // The Play button is where the eye and the controller land.
  useEffect(() => { if (input.kind !== 'pointer') document.querySelector<HTMLElement>('.gm-game-actions .primary')?.focus({ preventScroll: true }) }, [it.id, input.kind === 'pointer']) // eslint-disable-line react-hooks/exhaustive-deps

  const tint = useCoverTint(it.poster ? heroCover(it) : null)
  const settingsHost = useSettingsHost(!!user?.isAdmin)
  const say = useCallback((m: string) => toast(m), [])
  const consoleList = useMemo(() => consoles.map((x) => ({ id: x.id, name: x.name, short: x.short })), [consoles])
  const auto = saves.find((s) => s.slot === 0)
  const others = saves.filter((s) => s.slot !== 0)
  const ready = check ? check.ready : c?.ready ?? false
  const startsResumed = !!auto && resumeMode === 'always'
  const offerResume = !!auto && resumeMode === 'ask'
  const played = it.progress?.positionSec || 0
  const lib = libraries.find((l) => l.id === it.libraryId)
  const back = backdropUrl(it)
  const sizeText = it.sizeBytes ? (it.sizeBytes >= 1e9 ? `${(it.sizeBytes / 1e9).toFixed(1)} GB` : `${Math.max(1, Math.round(it.sizeBytes / 1e6))} MB`) : ''
  const cheats = mine?.cheats?.length || 0

  const putMeta = async (patch: Partial<GamePersonal>) => {
    try { setMine(await api.put<GamePersonal>(`/games/${it.id}/meta`, patch)) } catch (e: any) { toast(e.message) }
  }
  const removeSave = async (slot: number) => {
    if (!await ask({ title: `Delete ${slotName(slot).toLowerCase()}?`, body: 'This save can’t be brought back.', confirm: 'Delete', danger: true })) return
    try { await api.del(`/games/${it.id}/saves/${slot}`); void loadSaves() } catch (e: any) { toast(e.message) }
  }
  const battery = async () => {
    const f = await pickSaveFile('.srm,.sav,.ram')
    if (!f) return
    try { await uploadFile(`/games/${it.id}/battery`, f, 'PUT'); toast('Battery save replaced. The old one is kept beside it on the server.') } catch (e: any) { toast(e.message) }
  }
  const moreItems: MenuEntry[] = [
    ...(ready ? [{ key: 'safe', label: 'Start with default settings', icon: <IconRefresh />, onSelect: () => nav(links.play(it.id, `safe=1${auto ? '&resume=1' : ''}`)) }] : []),
    { key: 'settings', label: showSettings ? 'Hide settings for this game' : 'Settings for this game', icon: <IconTune />, onSelect: () => setShowSettings((s) => !s) },
    { key: 'pads', label: 'Controllers', icon: <IconGamepad />, to: `/controllers?console=${encodeURIComponent(it.console || '')}` },
    ...(user?.isAdmin ? [
      { kind: 'separator' as const, key: 'sep' },
      { key: 'art', label: 'Change artwork', icon: <IconImage />, onSelect: () => setArt(true) },
      { key: 'edit', label: 'Edit details', icon: <IconEdit />, onSelect: () => setEdit(true) },
    ] : []),
  ]
  const prompts: Prompt[] = [{ button: 'dpad', label: 'Move' }, { button: 'confirm', label: 'Select' }, { button: 'back', label: 'Back' }, { button: 'north', label: 'Favourite', key: 'F' }]
  // The same button as on a game's card.
  const toggleFav = useRef(() => {})
  toggleFav.current = () => { if (mine) void putMeta({ favourite: !mine.favourite }) }
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.ctrlKey || e.altKey || e.metaKey || document.querySelector('.modal')) return
      if ((e.target as Element | null)?.matches?.('input, textarea, select, [contenteditable]')) return
      if ((!e.isTrusted && e.key === 'm') || (e.isTrusted && e.key === 'f')) { e.preventDefault(); toggleFav.current() }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div className="gm-page gm-detail">
      <Ambient colour={tint} />
      {back && <div className="gm-detail-backdrop" style={{ backgroundImage: `url(${back})` }} aria-hidden="true" />}
      <div className="gm-game-head">
        <div className="gm-game-cover">
          <div className="poster game-poster">
            {it.poster ? <img src={heroCover(it)} alt="" /> : <Placeholder title={it.title} label={c?.short} />}
          </div>
        </div>
        <div className="gm-game-info">
          <nav className="crumbs" aria-label="Where this is">
            {lib && <Link to={links.library(it.libraryId)}>{lib.name}</Link>}
            {lib && c && <span aria-hidden="true"> / </span>}
            {c && <Link to={links.library(it.libraryId, 'console=' + encodeURIComponent(c.id))}>{c.short}</Link>}
          </nav>
          <h1>{it.title}</h1>
          {it.tagline && <div className="tagline">{it.tagline}</div>}
          <div className="meta">
            {c && <span>{c.name}</span>}
            {it.year && <span>{it.year}</span>}
            {meta && meta.players > 0 && <span>{meta.players === 1 ? '1 player' : `Up to ${meta.players} players`}</span>}
            {it.contentRating && !meta?.ages.length && <span className="pill">{it.contentRating}</span>}
            {it.rating != null && <span className="star"><IconStar /> {it.rating.toFixed(1)}</span>}
            {(it.genres || []).length > 0 && <span title={it.genres.join(', ')}>{it.genres.slice(0, 3).join(', ')}{it.genres.length > 3 ? '…' : ''}</span>}
          </div>
          {(played > 60 || (mine && mine.plays > 0)) && (
            <div className="gm-game-stats">
              {played > 60 && <span><b>{fmtPlayed(played)}</b> played</span>}
              {mine && mine.plays > 0 && <span><b>{mine.plays.toLocaleString()}</b> {mine.plays === 1 ? 'session' : 'sessions'}</span>}
            </div>
          )}
          <div className="gm-game-actions">
            {ready && (offerResume || startsResumed)
              ? <Link className="btn primary big gm-play" to={links.play(it.id, 'resume=1')}><IconPlay /> Resume</Link>
              : ready && <Link className="btn primary big gm-play" to={links.play(it.id)}><IconPlay /> Play</Link>}
            {ready && auto && <Link className="btn big" to={links.play(it.id)}>Start over</Link>}
            {!ready && check?.reason === 'core' && user?.isAdmin && <Link className="btn primary big" to="/settings/gaming"><IconSettings size={16} /> Install a core</Link>}
            {!ready && check?.reason === 'bios' && user?.isAdmin && <Link className="btn primary big" to="/settings/gaming"><IconSettings size={16} /> Set up {c?.short}</Link>}
            {mine && (
              <button type="button" className={'btn big square gm-fav' + (mine.favourite ? ' on' : '')} aria-pressed={mine.favourite} aria-label={mine.favourite ? 'Favourite' : 'Add to favourites'} title={mine.favourite ? 'Favourite' : 'Add to favourites'} onClick={() => putMeta({ favourite: !mine.favourite })}>
                <IconStar size={20} />
              </button>
            )}
            <Menu label="More for this game" items={moreItems} align="start"
              trigger={(p) => <button type="button" {...p} ref={p.ref as React.Ref<HTMLButtonElement>} className="btn big square" aria-label="More"><IconMore size={20} /></button>} />
          </div>
          {auto && ready && (
            <Link className="gm-resume" to={links.play(it.id, 'resume=1')} aria-label={`Resume from ${fmtDateTime(auto.updatedAt * 1000)}`}>
              {auto.hasThumb ? <img src={saveThumb(it.id, 0, auto.updatedAt)} alt="" loading="lazy" onLoad={crispIfSmall} /> : <span className="gm-save-blank" />}
              <span><b>Pick up where you left off</b><small>{fmtDateTime(auto.updatedAt * 1000)}</small></span>
            </Link>
          )}
          {check && !check.ready && (
            <div className="gm-blocked" role="status">
              <IconInfo />
              <div>
                <strong>{check.reason === 'core' ? 'This game needs a core first' : check.reason === 'bios' ? 'This game needs a BIOS file' : 'This console can’t play here yet'}</strong>
                <p>{check.message}{check.reason === 'core' && (user?.isAdmin ? ' A core is the free program that plays these games. It takes one click to install.' : ' Ask whoever runs this server to install one.')}</p>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="gm-about-wrap">
      {it.overview && (
        <section className="gm-section gm-about" data-gm-section aria-labelledby="gm-about-h">
          <h2 id="gm-about-h">About this game</h2>
          <p className={'gm-overview' + (longRead ? '' : ' short')}>{it.overview}</p>
          {it.overview.length > 420 && <button type="button" className="btn sm ghost" aria-expanded={longRead} onClick={() => setLongRead((o) => !o)}>{longRead ? 'Show less' : 'Read more'}</button>}
        </section>
      )}
      <section className="gm-section gm-facts-box" aria-label="Details">
        {meta && <GameRatings d={meta} />}
        <dl className="gm-facts">
          {it.year && !meta?.released && <div><dt>Year</dt><dd>{it.year}</dd></div>}
          {meta?.released && <div><dt>Released</dt><dd>{fmtReleased(meta.released)}</dd></div>}
          {it.studio && <div><dt>Developer</dt><dd>{it.studio}</dd></div>}
          {meta?.publisher && <div><dt>Publisher</dt><dd>{meta.publisher}</dd></div>}
          {meta && meta.players > 0 && <div><dt>Players</dt><dd>{meta.players === 1 ? '1' : `Up to ${meta.players}`}</dd></div>}
          {meta?.franchise && <div><dt>Series</dt><dd><Link to={links.library(it.libraryId, 'series=' + encodeURIComponent(meta.franchise))}>{meta.franchise}</Link></dd></div>}
          {(it.genres || []).length > 0 && <div><dt>Genres</dt><dd>{it.genres.map((g, i) => <span key={g}>{i > 0 && ', '}<Link to={links.library(it.libraryId, 'genre=' + encodeURIComponent(g))}>{g}</Link></span>)}</dd></div>}
        </dl>
      </section>
      </div>

      {meta && meta.art.length > 0 && <div className="gm-section" data-gm-section><GameShots d={meta} title={it.title} /></div>}

      {saves.length > 0 && (
        <section className="hub gm-section" data-gm-section aria-labelledby="gm-saves-h">
          <div className="hub-head"><h2 id="gm-saves-h">Save states</h2></div>
          <ul className="gm-saves">
            {[...(auto ? [auto] : []), ...others].map((s) => (
              <li key={s.slot}>
                <Link to={links.play(it.id, s.slot === 0 ? 'resume=1' : `slot=${s.slot}`)} className="gm-save-open" aria-label={`Play from ${slotName(s.slot)}, saved ${fmtDateTime(s.updatedAt * 1000)}`}>
                  {s.hasThumb ? <img src={saveThumb(it.id, s.slot, s.updatedAt)} alt="" loading="lazy" decoding="async" onLoad={crispIfSmall} /> : <span className="gm-save-blank" />}
                  <span className="gm-save-play"><IconPlay /></span>
                </Link>
                <div className="gm-save-text"><b>{slotName(s.slot)}</b><small>{fmtDateTime(s.updatedAt * 1000)}</small></div>
                <div className="gm-save-tools">
                  <a className="btn sm ghost square" href={mediaUrl(`/api/games/${it.id}/saves/${s.slot}/file`)} download aria-label={`Download ${slotName(s.slot)}`} title="Download"><IconDownload size={16} /></a>
                  <button type="button" className="btn sm ghost square" onClick={() => removeSave(s.slot)} aria-label={`Delete ${slotName(s.slot)}`} title="Delete"><IconTrash size={16} /></button>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}

      <div className="gm-panels" data-gm-section>
        <section className="gm-panel" aria-labelledby="gm-play-h">
          <h2 id="gm-play-h">How it plays</h2>
          <ul className="gm-checks">
            <li className="ok"><IconCheck size={16} /><span><b>Runs on your server</b><small>and streams to this device, so this device needs no emulator and nothing downloads. Saves stay on the server, and follow you to every device. Playing on the device itself, without the server, isn’t available yet.</small></span></li>
            {check?.core && <li className="ok"><IconCheck size={16} /><span><b>{check.core.name}</b><small>The core that plays this game. License: {check.core.license || 'not known'}.</small></span></li>}
            {check?.reason === 'core' && <li className="no"><IconInfo size={16} /><span><b>No core installed</b><small>{user?.isAdmin ? <>Install one in <Link to="/settings/gaming">Consoles &amp; cores</Link>.</> : 'Ask whoever runs this server.'}</small></span></li>}
            {check?.needsGpu && <li className="info"><IconInfo size={16} /><span><b>Uses the server’s graphics card</b><small>This console is drawn with one, so it needs a server that has one.</small></span></li>}
            {(check?.bios || []).filter((b) => b.required || b.found).map((b) => (
              <li key={b.file} className={b.found ? 'ok' : 'no'}>
                {b.found ? <IconCheck size={16} /> : <IconInfo size={16} />}
                <span><b>BIOS: <code>{b.file}</code></b><small>{b.found ? `Found. ${b.for}.` : `Not found yet. ${b.for}. Put your own copy in a folder named ${check?.biosFolder || 'BIOS'} beside these games.`}</small></span>
              </li>
            ))}
            {c?.unavailable && <li className="no"><IconInfo size={16} /><span><b>Not playable here yet</b><small>{c.unavailable}</small></span></li>}
          </ul>
          {user?.isAdmin && <Suspense fallback={null}><GameCorePicker itemId={it.id} /></Suspense>}
        </section>
        <section className="gm-panel" aria-labelledby="gm-controls-h">
          <h2 id="gm-controls-h">Controls</h2>
          {c?.pad && (
            <p className="gm-pad-list">
              <span className="pill">D-pad</span>
              {c.pad.sticks && <span className="pill">Stick</span>}
              {[...c.pad.face, ...(c.pad.shoulder || []), ...c.pad.meta].map((b) => <span key={b.id} className="pill">{b.label}</span>)}
            </p>
          )}
          <p className="muted">Controllers, keyboards, TV remotes and touch screens all work, and pressing Escape or Start and Select together opens the game menu.</p>
          <div className="gm-panel-actions"><Link className="btn sm" to={`/controllers?console=${encodeURIComponent(it.console || '')}`}><IconGamepad /> Set up controllers</Link></div>
        </section>
        <section className="gm-panel" aria-labelledby="gm-cheats-h">
          <h2 id="gm-cheats-h">Cheats</h2>
          <p className="muted">{cheats > 0 ? `${cheats.toLocaleString()} ${cheats === 1 ? 'cheat' : 'cheats'} saved for this game. Turn them on and off from the game menu or below.` : 'No cheats saved. Add codes here, or from the game menu while you play.'}</p>
          <div className="gm-panel-actions"><button type="button" className="btn sm" aria-expanded={showSettings} onClick={() => setShowSettings((s) => !s)}><IconTune size={16} /> {showSettings ? 'Hide' : 'Cheats and settings'}</button></div>
        </section>
        {mine && (
          <section className="gm-panel" aria-labelledby="gm-notes-h">
            <h2 id="gm-notes-h">Your notes</h2>
            <textarea className="input gm-notes" rows={3} maxLength={8000} value={note} aria-labelledby="gm-notes-h" onChange={(e) => setNote(e.target.value)} onBlur={() => { if (note !== mine.note) void putMeta({ note }) }} placeholder="Passwords, where you got to, anything. Only you can see these." />
          </section>
        )}
      </div>

      {showSettings && (
        <section className="gm-section" aria-label="Settings for this game">
          <h2 className="shelf-head">Settings for this game</h2>
          <Suspense fallback={<Spinner />}><SettingsScreen host={settingsHost} consoles={consoleList} itemId={it.id} itemTitle={it.title} consoleId={it.console} onError={say} /></Suspense>
        </section>
      )}

      <details className="gm-section gm-file">
        <summary>Files and advanced</summary>
        <dl className="gm-facts">
          <div><dt>File</dt><dd>{[it.container?.toUpperCase(), sizeText].filter(Boolean).join(' · ')}</dd></div>
          {meta?.tags && meta.tags !== it.tagline && <div><dt>File name says</dt><dd>{meta.tags}</dd></div>}
          {user?.isAdmin && <div className="wide"><dt>Path</dt><dd><code>{it.path}</code></dd></div>}
        </dl>
        <div className="gm-panel-actions">
          {mine?.hasBattery && <a className="btn sm ghost" href={mediaUrl(`/api/games/${it.id}/battery`)} download>Download battery save</a>}
          <button type="button" className="btn sm ghost" onClick={() => void battery()}>{mine?.hasBattery ? 'Replace battery save…' : 'Import a battery save…'}</button>
        </div>
        {user?.isAdmin && meta && <GameMatchBar it={it} d={meta} onChanged={() => { void loadMeta(); reload?.() }} />}
      </details>
      {meta && <GameCredits d={meta} />}
      <LegalNote kind="games" />
      <PromptBar className="gm-prompts" always={links.couch} items={prompts} />
      {edit && <EditGame item={it} onClose={() => setEdit(false)} onSaved={() => { setEdit(false); reload?.() }} />}
      {art && <ArtworkPicker base={`/items/${it.id}`} onClose={() => setArt(false)}
        onPick={async (kind, url) => { await api.put<Item>(`/items/${it.id}`, { [kind]: url }); reload?.(); toast(kind === 'poster' ? 'Cover updated' : 'Backdrop updated') }} />}
    </div>
  )
}

// Someone's edits are theirs: sources leave what was typed here alone.
function EditGame({ item, onClose, onSaved }: { item: Item; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ title: item.title, year: item.year ? String(item.year) : '', overview: item.overview, genres: (item.genres || []).join(', '), contentRating: item.contentRating || '' })
  const [err, setErr] = useState('')
  const save = async () => {
    const body: Record<string, unknown> = {}
    if (f.title !== item.title) body.title = f.title
    if (f.year !== (item.year ? String(item.year) : '')) body.year = f.year ? Number(f.year) : null
    if (f.overview !== item.overview) body.overview = f.overview
    if (f.genres !== (item.genres || []).join(', ')) body.genres = f.genres.split(',').map((s) => s.trim()).filter(Boolean)
    if (f.contentRating !== (item.contentRating || '')) body.contentRating = f.contentRating
    try { await api.put<Item>(`/items/${item.id}`, body); onSaved() } catch (e: any) { setErr(e.message) }
  }
  return (
    <Dialog title="Edit details" onClose={onClose} actions={<><button type="button" className="btn ghost" onClick={onClose}>Cancel</button><button type="button" className="btn primary" onClick={save}>Save</button></>}>
      <div className="form full">
        <div className="row2">
          <label>Title<input className="input" value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} /></label>
          <label>Year<input className="input" inputMode="numeric" value={f.year} onChange={(e) => setF({ ...f, year: e.target.value.replace(/\D/g, '').slice(0, 4) })} /></label>
        </div>
        <label>Summary<textarea className="input" rows={5} value={f.overview} onChange={(e) => setF({ ...f, overview: e.target.value })} /></label>
        <label>Genres (comma separated)<input className="input" value={f.genres} onChange={(e) => setF({ ...f, genres: e.target.value })} /></label>
        <label>Age rating <span className="muted">(decides what Kids and Guest profiles can see)</span>
          <input className="input" placeholder="e.g. ESRB T, PEGI 12, CERO B" value={f.contentRating} onChange={(e) => setF({ ...f, contentRating: e.target.value })} />
        </label>
        {err && <div className="error" role="alert">{err}</div>}
      </div>
    </Dialog>
  )
}
