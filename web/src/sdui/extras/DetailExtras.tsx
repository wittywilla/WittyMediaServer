// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What the web app adds to a title's server-driven header from its own stock: the tools
// an administrator has for a title (artwork, details, refresh), the credit a metadata
// source asks for, and the file's technical facts. A disc's sections come from its
// module's own sdui.json (WittyDVD, WittyBluray). The page
// model has no components for these; they are the app's own, as on its hand-built pages.

import { lazy, Suspense, useEffect, useState } from 'react'
import { api, codecName, discName, getPrefs, isBluray, isDisc, langName, type Item, type Show } from '../../api'
import { toast, useApp } from '../../state'
import { onEpisodeViewChosen, setEpisodeView, useEpisodeView, type EpisodeView } from './episodeView'
import TitleArtPicker, { appliedOnPage } from '../../components/edit/TitleArtPicker'
import LegalNote from '../../components/LegalNote'
import { IconDevices, IconEdit, IconFilm, IconImage, IconMore, IconPlay, IconRefresh, IconUsers } from '../../icons'
import { useRemoteAllowed } from '../../remote/allowed'
import { Menu, type MenuEntry } from '../../ui/Menu'
import { bump } from '../client'
import type { Document, Node } from '../types'

const EditDetails = lazy(() => import('../../components/edit/EditDetails'))
const RefreshDialog = lazy(() => import('../../components/edit/RefreshDialog'))
const TitleExtras = lazy(() => import('../../components/edit/TitleExtras'))
const PlayOn = lazy(() => import('../../remote/PlayOn'))
const CanvasManager = lazy(() => import('../../canvas/CanvasManager'))
const AlbumCanvasManager = lazy(() => import('../../canvas/AlbumCanvasManager'))
const WatchParty = lazy(() => import('../../remote/WatchParty'))

type Target = { target: 'item'; id: number } | { target: 'show'; id: number } | null

function targetOf(header: Node): Target {
  const id = typeof header.props?.itemId === 'string' ? header.props.itemId : ''
  if (/^\d+$/.test(id)) return { target: 'item', id: Number(id) }
  const show = /^show-(\d+)$/.exec(id)
  return show ? { target: 'show', id: Number(show[1]) } : null
}

// One fetch of the title for both places it is drawn in.
const asked = new Map<string, Promise<Item | Show | null>>()
function useTitle(t: Target, version: string): Item | Show | null {
  const [got, setGot] = useState<Item | Show | null>(null)
  const key = t ? `${t.target}:${t.id}:${version}` : ''
  useEffect(() => {
    if (!t) { setGot(null); return }
    let live = true
    let p = asked.get(key)
    if (!p) {
      p = (t.target === 'item' ? api.get<Item>(`/items/${t.id}`) : api.get<{ show: Show }>(`/shows/${t.id}`).then((r) => r.show)).catch(() => null)
      asked.set(key, p)
      if (asked.size > 8) asked.delete(asked.keys().next().value as string)
    }
    p.then((v) => { if (live) setGot(v) })
    return () => { live = false }
  }, [key]) // eslint-disable-line react-hooks/exhaustive-deps
  return got
}

function Credit({ provider, providerId }: { provider?: string; providerId?: string }) {
  if (provider === 'wikidata') return <p className="fine-print">Facts from <a href={`https://www.wikidata.org/wiki/${providerId}`} target="_blank" rel="noreferrer">Wikidata</a>; plot summary from Wikipedia, shared under <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.</p>
  if (provider === 'tvmaze') return <p className="fine-print">TV information from <a href={`https://www.tvmaze.com/shows/${providerId}`} target="_blank" rel="noreferrer">TVmaze</a>, shared under <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noreferrer">CC BY-SA 4.0</a>.</p>
  if (provider === 'tvdb') return <p className="fine-print">Metadata provided by <a href="https://thetvdb.com" target="_blank" rel="noreferrer">TheTVDB</a>. Please consider adding missing information or subscribing.</p>
  if (provider === 'anilist') return <p className="fine-print">Anime information from <a href={`https://anilist.co/anime/${providerId}`} target="_blank" rel="noreferrer">AniList</a>.</p>
  if (provider === 'tmdb' || provider === 'tmdb-tv') return <p className="fine-print">Details from <a href="https://www.themoviedb.org" target="_blank" rel="noreferrer">TMDB</a>. This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
  return null
}

export default function DetailExtras({ place, header, doc }: { place: 'tools' | 'below'; header: Node; doc: Document }) {
  const { user } = useApp()
  const t = targetOf(header)
  const title = useTitle(t, doc.meta?.etag || '')
  const [open, setOpen] = useState<'' | 'art' | 'edit' | 'refresh' | 'canvas'>('')
  if (place === 'tools' && header.props?.kind === 'album' && user?.isAdmin) return <AlbumCanvasTool doc={doc} />
  if (!t || !title) return null
  const item = t.target === 'item' ? title as Item : null
  const show = t.target === 'show' ? title as Show : null
  const changed = () => { asked.clear(); bump() }
  if (place === 'tools') {
    if (show) return <ShowTools show={show} admin={!!user?.isAdmin} onPick={(w) => setOpen(w)} open={open} close={() => setOpen('')} t={t} changed={changed} />
    const sending = item ? <SendTools item={item} header={header} /> : null
    if (!user?.isAdmin) return sending
    const entries: MenuEntry[] = [
      { key: 'art', label: 'Artwork', icon: <IconImage />, onSelect: () => setOpen('art') },
      { key: 'edit', label: 'Edit details', icon: <IconEdit />, onSelect: () => setOpen('edit') },
      ...(item && (item.kind === 'video' || isDisc(item)) ? [{ key: 'canvas', label: 'Canvas', icon: <IconFilm />, onSelect: () => setOpen('canvas') } as MenuEntry] : []),
      ...(item?.showId == null ? [{ key: 'refresh', label: 'Check for new details', icon: <IconRefresh />, onSelect: () => setOpen('refresh') } as MenuEntry] : []),
    ]
    return (
      <>
        {sending}
        <Menu label="Manage this title" align="end" items={entries} trigger={(props) => <button type="button" {...props} className="btn ghost square" aria-label="Manage this title" title="Manage this title"><IconMore /></button>} />
        <Suspense fallback={null}>
          {open === 'art' && <TitleArtPicker target={t.target} id={t.id} onApplied={appliedOnPage(changed)} onClose={() => setOpen('')} />}
          {open === 'edit' && <EditDetails target={t.target} id={t.id} start="match" onClose={() => setOpen('')} onChanged={changed} />}
          {open === 'refresh' && <RefreshDialog target={t.target} id={t.id} onClose={() => setOpen('')} onChanged={changed} />}
          {open === 'canvas' && item && <CanvasManager itemId={item.id} title={item.title} kind={isDisc(item) ? 'disc' : 'video'} runtimeSec={item.runtimeSec} onClose={() => { setOpen(''); changed() }} />}
        </Suspense>
      </>
    )
  }
  const disc = item ? isDisc(item) : false
  const main = item?.disc?.titles.find((x) => x.number === item.disc?.mainTitle)
  const sizeGB = item?.sizeBytes ? (item.sizeBytes / 1e9).toFixed(2) + ' GB' : ''
  return (
    <div className="sdui-extras">
      <Credit provider={title.provider} providerId={title.providerId} />
      {item && (item.width > 0 || item.container || disc) && (
        <div className="facts">
          {item.width > 0 && <div className="fact"><div className="k">Video</div><div className="v">{item.width}×{item.height} {codecName(item.videoCodec)}{disc && main ? ` · ${isBluray(item) ? main.standard : ''}` : ''}</div></div>}
          {(item.audioLangs || []).length > 0 && <div className="fact"><div className="k">Audio</div><div className="v">{[...new Set(item.audioLangs.map(langName))].join(', ')}</div></div>}
          {(item.subLangs || []).length > 0 && <div className="fact"><div className="k">Subtitles</div><div className="v">{[...new Set(item.subLangs.map(langName))].join(', ')}</div></div>}
          <div className="fact"><div className="k">Source</div><div className="v">{disc ? `${discName(item)} ${item.kind.endsWith('-iso') ? 'image' : 'folder'}` : item.container?.toUpperCase()} {sizeGB && `· ${sizeGB}`}</div></div>
          {disc && item.disc && <div className="fact"><div className="k">Disc</div><div className="v">{item.disc.label} · {item.disc.titles.length} titles{item.disc.hasMenus ? ' · menus' : ''}</div></div>}
          {user?.isAdmin && <div className="fact wide"><div className="k">File</div><div className="v">{item.path}</div></div>}
        </div>
      )}
      <Suspense fallback={null}>
        <TitleExtras target={t.target} id={t.id} libraryId={title.libraryId} />
      </Suspense>
      <LegalNote kind="metadata" />
    </div>
  )
}

// A show's tools: its episodes' layout (the show's own, which an administrator keeps for
// everyone, else the server's default), and for an administrator the same menu as a film's
// with "Find intros and credits", which compares a season's episodes by ear as a task.
function ShowTools({ show, admin, onPick, open, close, t, changed }: {
  show: Show; admin: boolean; onPick: (w: 'art' | 'edit' | 'refresh') => void; open: string; close: () => void; t: NonNullable<Target>; changed: () => void
}) {
  const key = `show-${show.id}`
  const now = useEpisodeView(key)
  useEffect(() => {
    if (now) return
    let live = true
    const own = (show as Show & { view?: string }).view
    if (own === 'cards' || own === 'list') { setEpisodeView(key, own); return }
    getPrefs().then((p) => { const v = (p as { episodeView?: string }).episodeView; if (live && (v === 'cards' || v === 'list')) setEpisodeView(key, v as EpisodeView) }).catch(() => {})
    return () => { live = false }
  }, [key]) // eslint-disable-line react-hooks/exhaustive-deps
  // An administrator's choice is kept for the show; anyone else's lasts while they are here.
  useEffect(() => {
    if (!admin) return
    return onEpisodeViewChosen((which, v) => {
      if (which !== key) return
      api.put(`/shows/${show.id}/view`, { view: v }).catch((e) => toast((e as Error).message, { tone: 'danger' }))
    })
  }, [admin, key, show.id])
  if (!admin) return null
  const findIntros = async () => {
    try { await api.post(`/shows/${show.id}/detect-intros`, {}); toast('Looking for intros and credits. Tasks shows how far it has got.') } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const entries: MenuEntry[] = [
    { key: 'art', label: 'Artwork', icon: <IconImage />, onSelect: () => onPick('art') },
    { key: 'edit', label: 'Edit details', icon: <IconEdit />, onSelect: () => onPick('edit') },
    { key: 'refresh', label: 'Check for new details', icon: <IconRefresh />, onSelect: () => onPick('refresh') },
    { key: 'intros', label: 'Find intros and credits', icon: <IconPlay />, onSelect: () => void findIntros() },
  ]
  return (
    <>
      <Menu label="Manage this show" align="end" items={entries} trigger={(props) => <button type="button" {...props} className="btn ghost square" aria-label="Manage this show" title="Manage this show"><IconMore /></button>} />
      <Suspense fallback={null}>
        {open === 'art' && <TitleArtPicker target={t.target} id={t.id} onApplied={appliedOnPage(changed)} onClose={close} />}
        {open === 'edit' && <EditDetails target={t.target} id={t.id} start="match" onClose={close} onChanged={changed} />}
        {open === 'refresh' && <RefreshDialog target={t.target} id={t.id} onClose={close} onChanged={changed} />}
      </Suspense>
    </>
  )
}

// Play on another device and Watch party, for a title that plays, where the page's own
// header doesn't offer them (a page written before they were, or an older server).
const SENDABLE = (it: Item) => it.kind === 'video' || isDisc(it)
function SendTools({ item, header }: { item: Item; header: Node }) {
  const may = useRemoteAllowed()
  const [open, setOpen] = useState<'' | 'device' | 'party'>('')
  const offered = new Set((Array.isArray(header.props?.actions) ? header.props.actions : []).map((a) => (a as { id?: string }).id))
  if (!SENDABLE(item)) return null
  const device = may.playOn && !offered.has('playOn')
  const party = may.party && !offered.has('watchParty')
  if (!device && !party) return null
  return (
    <>
      {device && <button type="button" className="btn ghost square" aria-label="Play on another device" title="Play on another device" aria-haspopup="dialog" onClick={() => setOpen('device')}><IconDevices /></button>}
      {party && <button type="button" className="btn ghost square" aria-label="Watch party" title="Watch party" aria-haspopup="dialog" onClick={() => setOpen('party')}><IconUsers /></button>}
      <Suspense fallback={null}>
        {open === 'device' && <PlayOn source={{ kind: 'item', itemId: item.id, title: item.title }} onClose={() => setOpen('')} />}
        {open === 'party' && <WatchParty item={{ id: item.id, title: item.title, kind: item.kind, showId: item.showId ?? undefined }} onClose={() => setOpen('')} />}
      </Suspense>
    </>
  )
}

// An album's canvas, for an administrator: each song's own, chosen from the page's tracks.
function AlbumCanvasTool({ doc }: { doc: Document }) {
  const [open, setOpen] = useState(false)
  const songs: { id: number; title: string; track?: number | null; runtimeSec: number }[] = []
  const walk = (n: Node) => {
    const id = typeof n.props?.itemId === 'string' ? n.props.itemId : ''
    if (n.type === 'Track' && /^\d+$/.test(id)) songs.push({ id: Number(id), title: String(n.props?.title || ''), track: typeof n.props?.number === 'number' ? n.props.number : null, runtimeSec: typeof n.props?.durationSec === 'number' ? n.props.durationSec : 0 })
    for (const c of n.children || []) walk(c)
  }
  walk(doc.root)
  if (!songs.length) return null
  return (
    <>
      <button type="button" className="btn ghost" aria-haspopup="dialog" onClick={() => setOpen(true)} title="The looping video behind these songs"><IconFilm /> Canvas</button>
      {open && <Suspense fallback={null}><AlbumCanvasManager songs={songs} onClose={() => { setOpen(false); bump() }} /></Suspense>}
    </>
  )
}
