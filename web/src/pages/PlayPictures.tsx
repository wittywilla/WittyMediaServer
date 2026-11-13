// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyPlay Settings › Game pictures: how games are pictured, for every game, per console and per game (shell/gamePictures.ts), each
// choice shown live on made-up covers of every shape; the person's own choices or, for an administrator, the server's for everyone;
// export and import as a .wittyplay file (docs/specs/wittyplay-file.md); and brand logos, off until an administrator turns them on and
// confirms the warning (shell/brandLogos.ts).
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, downloadFile, type Item } from '../api'
import { useApp } from '../state'
import { ask, Button, Dialog, Notice, Segmented, Switch } from '../ui'
import { WebLink } from '../components/WebLink'
import { BRAND_LOGO_WARNING, brandLogosChanged, useBrandLogos, type BrandLogosState } from '../shell/brandLogos'
import { consoleMedia, GameCardArt, GamePictureDialog, LookChooser, loadServerPictures, PanelArtChooser, serverPicturesChanged, useConsoleIndex, usePicturesPref, useServerPictures } from '../shell/GamePictureArt'
import {
  CASE_STYLES, LABELS, PICTURES, cleanLook, emptyLook, exportName, importDoc, lookFor, prefToDoc, readDoc, withLook,
  WITTYPLAY_EXT, WITTYPLAY_TYPE, type Look, type PicturesPref, type Resolved, type ServerPictures, type WittyplayDoc,
} from '../shell/gamePictures'
import { logoImageUrl } from '../components/BrandLogo'
import { PageHead } from './settings/ui'

// Made-up covers for the previews, drawn here: one tall, one wide, one square, so every choice shows what it does with each shape.
function cover(w: number, h: number, a: string, b: string, mark: string) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${a}"/><stop offset="1" stop-color="${b}"/></linearGradient></defs>` +
    `<rect width="${w}" height="${h}" fill="url(#g)"/><circle cx="${w * 0.7}" cy="${h * 0.38}" r="${Math.min(w, h) * 0.22}" fill="#fff" fill-opacity=".85"/>` +
    `<path d="M0 ${h * 0.78} L${w * 0.35} ${h * 0.55} L${w * 0.6} ${h * 0.72} L${w} ${h * 0.5} V${h} H0 Z" fill="${mark}"/>` +
    `<rect x="${w * 0.08}" y="${h * 0.08}" width="${w * 0.5}" height="${h * 0.09}" rx="${h * 0.02}" fill="#fff" fill-opacity=".9"/></svg>`
  return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
}
const SAMPLES = [
  { id: 'tall', title: 'A tall cover', console: 'ps2', image: cover(300, 420, '#4b3bd8', '#c93d8a', '#1a1446') },
  { id: 'wide', title: 'A wide cover', console: 'snes', image: cover(420, 300, '#0f8f7c', '#2a5ad8', '#0b2a33') },
  { id: 'square', title: 'A square cover', console: 'ps1', image: cover(320, 320, '#e0752b', '#d8323f', '#3a1209') },
]

const words = (list: { id: string; label: string }[], v?: string) => list.find((x) => x.id === v)?.label || ''
const summary = (l: Look | undefined) => {
  const c = cleanLook(l)
  if (emptyLook(c)) return 'As every game'
  return [words(PICTURES, c.picture), c.case && c.case !== 'auto' ? words(CASE_STYLES, c.case) : '', c.label ? 'label: ' + words(LABELS, c.label).toLowerCase() : ''].filter(Boolean).join(', ')
}

// The previews: each made-up cover as it would be drawn now.
function Samples({ resolve }: { resolve: (console: string) => Resolved }) {
  return (
    <ul className="gpic-samples" aria-label="Preview">
      {SAMPLES.map((s) => {
        const look = resolve(s.console)
        return (
          <li key={s.id}>
            <span className="gpic-preview">{look.picture === 'filled' ? <span className="gpic-filled"><img src={s.image} alt="" /></span> : <GameCardArt look={look} itemId="" consoleId={s.console} image={s.image} title={s.title} />}</span>
            <span className="muted">{s.title}</span>
          </li>
        )
      })}
    </ul>
  )
}

type Scope = 'me' | 'server'

export default function PlayPictures() {
  const { user } = useApp()
  const admin = !!user?.isAdmin
  const [scope, setScope] = useState<Scope>('me')
  const [pref, setPref] = usePicturesPref()
  const server = useServerPictures()
  const consoles = useConsoleIndex()
  const [err, setErr] = useState('')
  const [said, setSaid] = useState('')
  const [editing, setEditing] = useState<string | null>(null)
  const [editingGame, setEditingGame] = useState<{ id: string; title: string; console: string; image: string } | null>(null)
  const ids = { all: useId(), cons: useId(), games: useId(), share: useId() }

  // The level being changed, and the one under it.
  const serverPref: PicturesPref = { default: server.server?.default, consoles: server.server?.consoles }
  const mine = scope === 'me' ? pref : serverPref
  const resolveFor = (p: PicturesPref, s: ServerPictures | undefined) => (consoleId: string) => {
    const c = consoles.find(consoleId)
    return lookFor(p, s, '', consoleId, consoleMedia(c, consoleId))
  }
  const now = scope === 'me' ? resolveFor(pref, server) : resolveFor({}, { server: server.server })
  const aboveAll = (consoleId: string) => (scope === 'me' ? resolveFor({}, server)(consoleId) : resolveFor({}, undefined)(consoleId))

  const saveServer = async (next: PicturesPref) => {
    setErr('')
    const doc: WittyplayDoc = { ...(server.server as WittyplayDoc | undefined), wittyplay: 1, default: cleanLook(next.default), consoles: next.consoles || {} }
    delete doc.games
    try { await api.put('/games/pictures', doc); serverPicturesChanged() } catch (e: any) { setErr(e.message) }
  }
  const change = (level: { console?: string }, l: Look) => {
    const next = withLook(mine, level, emptyLook(l) ? null : l)
    if (scope === 'me') setPref(next); else void saveServer(next)
  }

  const gamesShown = Object.keys(scope === 'me' ? pref.games || {} : server.games || {})
  const consoleList = useMemo(() => {
    const withGames = consoles.list.filter((c) => (c.totalGames ?? c.count ?? 0) > 0)
    return (withGames.length ? withGames : consoles.list).slice().sort((a, b) => a.name.localeCompare(b.name))
  }, [consoles.list])
  const editingConsole = editing ? consoles.find(editing) : undefined

  return (
    <div className="gpic-settings">
      <PageHead title="Game pictures" desc="How each game’s picture is shown in WittyPlay: its box art fitted into the tile, filling it, or the game drawn in a case or cartridge like the ones its console used. Choose for every game, for a console, or for one game." />
      {admin && (
        <Segmented<Scope> label="Whose choices" value={scope} onChange={setScope}
          options={[{ value: 'me', label: 'Your own' }, { value: 'server', label: 'This server’s, for everyone' }]} />
      )}
      {scope === 'server' && <p className="muted">These are what everyone sees until they choose their own. A choice someone makes for themselves wins.</p>}

      <section className="wplay-pads-setting" aria-labelledby={ids.all}>
        <h2 id={ids.all}>Every game</h2>
        <LookChooser look={mine.default || {}} onChange={(l) => change({}, l)} above={aboveAll('ps2')} sample={SAMPLES[0]} consoleId="ps2" media="dvd" everyMedium />
        <h3 className="gpic-sub">With covers of every shape</h3>
        <Samples resolve={now} />
      </section>

      {scope === 'me' && <PanelArtChooser />}

      <section className="wplay-pads-setting" aria-labelledby={ids.cons}>
        <h2 id={ids.cons}>Each console</h2>
        <p className="wplay-console-note">A console can have its own. Each console’s case is the one it used, unless you choose another.</p>
        <ul className="gpic-console-list">
          {consoleList.map((c) => {
            const look = now(c.id)
            return (
              <li key={c.id}>
                <span className="gpic-preview sm" aria-hidden="true">{look.picture === 'filled' ? <span className="gpic-filled"><img src={SAMPLES[0].image} alt="" /></span> : <GameCardArt look={look} itemId="" consoleId={c.id} image={SAMPLES[0].image} title={c.short} />}</span>
                <span className="gpic-console-text"><b>{c.name}</b><span className="muted">{summary(mine.consoles?.[c.id])}</span></span>
                <Button size="sm" onClick={() => setEditing(c.id)} aria-label={`Change ${c.name}`}>Change</Button>
              </li>
            )
          })}
          {!consoleList.length && <li className="muted">No consoles yet.</li>}
        </ul>
      </section>

      <section className="wplay-pads-setting" aria-labelledby={ids.games}>
        <h2 id={ids.games}>Single games</h2>
        <p className="wplay-console-note">Choose a game’s own from its panel in WittyPlay: Picture style. {scope === 'me' ? 'Yours are listed here.' : 'This server’s, including those from .wittyplay files beside games, are listed here.'}</p>
        {gamesShown.length ? (
          <ul className="gpic-console-list">
            {gamesShown.map((id) => <GameRow key={id} id={id} look={(scope === 'me' ? pref.games : server.games)?.[id]} canRemove={scope === 'me' || admin}
              onEdit={(g) => setEditingGame(g)} onRemove={async () => {
                if (scope === 'me') setPref(withLook(pref, { game: id }, null))
                else { try { await api.put(`/games/${id}/picture`, {}); serverPicturesChanged() } catch (e: any) { setErr(e.message) } }
              }} />)}
          </ul>
        ) : <p className="muted">None yet.</p>}
      </section>

      <Share scope={scope} pref={pref} setPref={setPref} onSaid={setSaid} onErr={setErr} />
      {said && <p className="gpic-said" role="status">{said}</p>}
      {err && <p className="error" role="alert">{err}</p>}

      <BrandLogos />

      {editing && (
        <Dialog title={`Games for ${editingConsole?.name || editing}`} size="wide" className="gpic-dialog" onClose={() => setEditing(null)}
          actions={<><Button variant="ghost" disabled={emptyLook(mine.consoles?.[editing])} onClick={() => change({ console: editing }, {})}>As every game</Button><Button variant="primary" onClick={() => setEditing(null)}>Done</Button></>}>
          <LookChooser look={mine.consoles?.[editing] || {}} onChange={(l) => change({ console: editing }, l)} consoleId={editing} media={consoleMedia(editingConsole, editing)}
            above={scope === 'me' ? lookFor({ default: pref.default }, { server: { default: server.server?.default, consoles: server.server?.consoles } }, '', editing, consoleMedia(editingConsole, editing)) : lookFor({ default: serverPref.default }, undefined, '', editing, consoleMedia(editingConsole, editing))}
            sample={{ image: SAMPLES[0].image, title: editingConsole?.short || editing }} />
        </Dialog>
      )}
      {editingGame && <GamePictureDialog itemId={editingGame.id} title={editingGame.title} console={editingGame.console} image={editingGame.image} onClose={() => setEditingGame(null)} />}
    </div>
  )
}

// One game with a choice of its own: its name (asked for), what was chosen, Change and Remove.
function GameRow({ id, look, canRemove, onEdit, onRemove }: { id: string; look?: Look; canRemove: boolean; onEdit: (g: { id: string; title: string; console: string; image: string }) => void; onRemove: () => void }) {
  const [it, setIt] = useState<Item | null>(null)
  useEffect(() => { api.get<Item>(`/items/${encodeURIComponent(id)}`).then(setIt).catch(() => {}) }, [id])
  const title = it?.title || `Game ${id}`
  return (
    <li>
      <span className="gpic-console-text"><b>{title}</b><span className="muted">{summary(look)}</span></span>
      <Button size="sm" onClick={() => onEdit({ id, title, console: it?.console || '', image: '' })} aria-label={`Change ${title}`}>Change</Button>
      {canRemove && <Button size="sm" variant="ghost" onClick={onRemove} aria-label={`Remove the choice for ${title}`}>Remove</Button>}
    </li>
  )
}

// Export and import as a .wittyplay file: the person's own choices, or the server's for an administrator.
function Share({ scope, pref, setPref, onSaid, onErr }: { scope: Scope; pref: PicturesPref; setPref: (p: PicturesPref) => void; onSaid: (s: string) => void; onErr: (s: string) => void }) {
  const id = useId()
  const input = useRef<HTMLInputElement | null>(null)
  const [busy, setBusy] = useState(false)
  const save = (doc: WittyplayDoc, name: string) => {
    const blob = new Blob([JSON.stringify(doc, null, 2) + '\n'], { type: WITTYPLAY_TYPE })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = exportName(name)
    document.body.appendChild(a); a.click(); a.remove()
    setTimeout(() => URL.revokeObjectURL(a.href), 1000)
  }
  const exportMine = async () => {
    onErr(''); setBusy(true)
    try {
      if (scope === 'server') { await downloadFile('/games/pictures/file', 'server-game-pictures' + WITTYPLAY_EXT); onSaid('Saved this server’s choices as a .wittyplay file.'); return }
      // Each game's name and console go with it, so another server can find it.
      const games: Record<string, { title?: string; console?: string; file?: string }> = {}
      await Promise.all(Object.keys(pref.games || {}).map((g) => api.get<Item>(`/items/${encodeURIComponent(g)}`).then((it) => { games[g] = { title: it.title, console: it.console || undefined, file: it.path ? it.path.split(/[/\\]/).pop() : undefined } }).catch(() => {})))
      save(prefToDoc(pref, { name: 'My game pictures', generator: 'WittyMediaServer', games }), 'my-game-pictures')
      onSaid('Saved your choices as a .wittyplay file.')
    } catch (e: any) { onErr(e.message) } finally { setBusy(false) }
  }
  const importFile = async (f?: File) => {
    if (!f) return
    onErr(''); onSaid('')
    if (f.size > 64 * 1024) { onErr('That file is too big: a .wittyplay file is at most 64 KB.'); return }
    const read = readDoc(await f.text())
    if ('error' in read) { onErr(read.error); return }
    setBusy(true)
    try {
      if (scope === 'server') {
        const r = await api.put<{ applied: number; unmatched: number }>('/games/pictures', read.doc)
        serverPicturesChanged(); void loadServerPictures(true)
        onSaid(`Imported for everyone. ${r.applied} ${r.applied === 1 ? 'game' : 'games'} matched${r.unmatched ? `, ${r.unmatched} not found here` : ''}.`)
        return
      }
      const r = await api.post<{ doc: WittyplayDoc; matches: number[][] }>('/games/pictures/check', read.doc)
      const out = importDoc(pref, r.doc, r.matches)
      setPref(out.pref)
      onSaid(`Imported. ${out.games} ${out.games === 1 ? 'game' : 'games'} matched${out.unmatched ? `, ${out.unmatched} not found here` : ''}.`)
    } catch (e: any) { onErr(e.message) } finally { setBusy(false) }
  }
  return (
    <section className="wplay-pads-setting" aria-labelledby={id}>
      <h2 id={id}>Share or move {scope === 'me' ? 'your choices' : 'this server’s choices'}</h2>
      <p className="wplay-console-note">A <code>{WITTYPLAY_EXT}</code> file holds these choices and nothing else: no pictures, nothing about you. Its format is open, for any program to use. A <code>{WITTYPLAY_EXT}</code> file beside a game, with the game’s name, is read when the library is scanned.</p>
      <div className="gpic-share">
        <Button onClick={() => void exportMine()} disabled={busy}>Export…</Button>
        <Button onClick={() => input.current?.click()} disabled={busy}>Import…</Button>
        <input ref={input} hidden type="file" accept={WITTYPLAY_EXT + ',' + WITTYPLAY_TYPE + ',application/json'} aria-label="Import a .wittyplay file" onChange={(e) => { void importFile(e.target.files?.[0]); e.target.value = '' }} />
      </div>
    </section>
  )
}

// Brand logos: an administrator's choice, off until turned on with the warning confirmed; credits for everyone when on.
function BrandLogos() {
  const { user } = useApp()
  const admin = !!user?.isAdmin
  const s = useBrandLogos()
  const id = useId()
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [said, setSaid] = useState('')
  const consoles = useConsoleIndex()
  if (!s || (!admin && !s.enabled)) return null
  const warning = s.warning?.length ? s.warning : BRAND_LOGO_WARNING
  const put = async (enabled: boolean) => {
    setBusy(true); setErr(''); setSaid('')
    try { brandLogosChanged(await api.put<BrandLogosState>('/games/brand-logos', { enabled, accepted: enabled })); setSaid(enabled ? 'Brand logos are on. The server is fetching them now.' : 'Brand logos are off.') } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const remove = async () => {
    if (!await ask({ title: 'Remove downloaded logos?', body: 'Every console and game studio logo downloaded to this server is deleted, and brand logos are turned off. Logos you uploaded yourself stay. You can turn them on again later.', confirm: 'Remove', danger: true })) return
    setBusy(true); setErr(''); setSaid('')
    try { const r = await api.del<{ removed: number }>('/games/brand-logos'); brandLogosChanged(); setSaid(`Removed ${r.removed} ${r.removed === 1 ? 'logo' : 'logos'}.`) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const credits = Object.entries(s.logos).sort(([a], [b]) => (consoles.find(a)?.name || a).localeCompare(consoles.find(b)?.name || b))
  return (
    <section className="wplay-pads-setting gpic-brands" id="brand-logos" aria-labelledby={id}>
      <h2 id={id}>Brand logos</h2>
      <p className="wplay-console-note">
        WittyPlay draws its own cases and cartridges with no maker’s name or logo on them. If you turn this on, this server downloads real console logos
        from Wikimedia Commons and puts them on the cases and as each console’s mark; you can also choose a game studio’s from a game’s Logos.
        Nothing is downloaded until you turn it on, and your browser only ever gets them from this server. With it off, or with no logo for a console,
        WittyPlay shows its own drawings and the console’s name.
      </p>
      {admin && (
        <div className="gpic-brand-switch">
          <span id={id + 's'}>Download and show brand logos</span>
          <Switch checked={s.enabled} disabled={busy} labelledBy={id + 's'} onChange={(on) => { if (on) setConfirm(true); else void put(false) }} />
        </div>
      )}
      {admin && s.appWide === 'yes' && <p className="muted">Logo downloads are also on for the whole app (Settings, Metadata agents, Logos), so the app shows console logos elsewhere whatever you choose here.</p>}
      {admin && s.enabled && !!s.pending && <p className="muted" role="status">Looking for {s.pending} more…</p>}
      {credits.length > 0 && (
        <>
          <h3 className="gpic-sub">Credits</h3>
          <ul className="gpic-credits">
            {credits.map(([cid, l]) => (
              <li key={cid}>
                <span className="gpic-credit-logo"><img src={logoImageUrl(l, l.opaque ? 'original' : 'white')} alt={`Logo of ${l.name}`} loading="lazy" /></span>
                <span><b>{consoles.find(cid)?.name || l.name}</b>{' '}
                  <span className="muted">{l.licenseUrl ? <WebLink href={l.licenseUrl}>{l.license}</WebLink> : l.license || 'Licence not recorded'}{l.credit ? `, by ${l.credit}` : ''}.{' '}
                    {l.sourceUrl && <WebLink href={l.sourceUrl}>Source</WebLink>}</span></span>
              </li>
            ))}
          </ul>
          <p className="muted">{warning[0]} This server and WittyPlay are not endorsed by or affiliated with their owners.</p>
        </>
      )}
      {admin && <div className="gpic-share"><Button variant="ghost" disabled={busy} onClick={() => void remove()}>Remove downloaded logos</Button></div>}
      {said && <p role="status">{said}</p>}
      {err && <p className="error" role="alert">{err}</p>}
      {confirm && (
        <Dialog title="Before brand logos are downloaded" alert onClose={() => setConfirm(false)} description="Please read this first."
          actions={<><Button variant="ghost" onClick={() => setConfirm(false)}>Cancel</Button><Button variant="primary" onClick={() => { setConfirm(false); void put(true) }}>I understand: turn them on</Button></>}>
          <Notice tone="warn"><ul className="gpic-warning">{warning.map((w) => <li key={w}>{w}</li>)}</ul></Notice>
          <p className="muted">Where logos come from is explained under <Link to="/settings/metadata#logos">Settings, Metadata agents, Logos</Link>.</p>
        </Dialog>
      )}
    </section>
  )
}
