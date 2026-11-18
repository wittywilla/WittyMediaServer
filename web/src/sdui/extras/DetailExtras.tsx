// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What the web app adds to a title's server-driven header from its own stock: the tools
// an administrator has for a title (artwork, details, refresh), the credit a metadata
// source asks for, and the file's technical facts. A disc's sections come from its
// module's own sdui.json (WittyDVD, WittyBluray). The page
// model has no components for these; they are the app's own, as on its hand-built pages.

import { lazy, Suspense, useEffect, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { api, codecName, discName, getPrefs, isBluray, isDisc, langName, type Item, type Show } from '../../api'
import { toast, useApp } from '../../state'
import { onEpisodeViewChosen, setEpisodeView, useEpisodeView, type EpisodeView } from './episodeView'
import TitleArtPicker, { appliedOnPage } from '../../components/edit/TitleArtPicker'
import LegalNote from '../../components/LegalNote'
import { IconChapters, IconEdit, IconFilm, IconImage, IconMore, IconPlay, IconRefresh, IconSettings } from '../../icons'
import { sourceWords, usesWittyinfo, type MarkerAnswer } from '../../player/markers'
import '../../wittyinfo/badge.css'
import { Menu, type MenuEntry } from '../../ui/Menu'
import { bump } from '../client'
import { useModuleItemMenu } from '../../moduleRegistry'
import type { Document, Node } from '../types'

const EditDetails = lazy(() => import('../../components/edit/EditDetails'))
const RefreshDialog = lazy(() => import('../../components/edit/RefreshDialog'))
const TitleExtras = lazy(() => import('../../components/edit/TitleExtras'))
const CanvasManager = lazy(() => import('../../canvas/CanvasManager'))
const AlbumCanvasManager = lazy(() => import('../../canvas/AlbumCanvasManager'))
const InfoDialog = lazy(() => import('../../wittyinfo/InfoDialog'))

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
  const [open, setOpen] = useState<'' | 'art' | 'edit' | 'refresh' | 'canvas' | 'info'>('')
  // A disc's own tools (Disc settings, names of its titles) are WittyDisc's, with its player.
  const discItem = t?.target === 'item' && title && isDisc(title as Item) ? title as Item : null
  const discMenu = useModuleItemMenu(discItem ? 'disc' : null, discItem, !!user?.isAdmin)
  if (place === 'tools' && header.props?.kind === 'album' && user?.isAdmin) return <AlbumCanvasTool doc={doc} />
  if (!t || !title) return null
  const item = t.target === 'item' ? title as Item : null
  const show = t.target === 'show' ? title as Show : null
  const changed = () => { asked.clear(); bump() }
  if (place === 'tools') {
    if (show) return <ShowTools show={show} admin={!!user?.isAdmin} onPick={(w) => setOpen(w)} open={open} close={() => setOpen('')} t={t} changed={changed} />
    if (!user?.isAdmin) return null
    const entries: MenuEntry[] = [
      { key: 'art', label: 'Artwork', icon: <IconImage />, onSelect: () => setOpen('art') },
      { key: 'edit', label: 'Edit details', icon: <IconEdit />, onSelect: () => setOpen('edit') },
      ...(item && (item.kind === 'video' || isDisc(item)) ? [{ key: 'canvas', label: 'Canvas', icon: <IconFilm />, onSelect: () => setOpen('canvas') } as MenuEntry] : []),
      ...(item && (item.kind === 'video' || isDisc(item)) ? [{ key: 'info', label: 'Edit scenes and skips', icon: <IconChapters />, onSelect: () => setOpen('info') } as MenuEntry] : []),
      ...discMenu.entries.map((e) => ({ key: e.key, label: e.label, icon: e.icon === 'edit' ? <IconEdit /> : <IconSettings />, onSelect: () => discMenu.choose(e) } as MenuEntry)),
      ...(item?.showId == null ? [{ key: 'refresh', label: 'Check for new details', icon: <IconRefresh />, onSelect: () => setOpen('refresh') } as MenuEntry] : []),
    ]
    return (
      <>
        <Menu label="Manage this title" align="end" items={entries} trigger={(props) => <button type="button" {...props} className="btn ghost square" aria-label="Manage this title" title="Manage this title"><IconMore /></button>} />
        <Suspense fallback={null}>
          {open === 'art' && <TitleArtPicker target={t.target} id={t.id} onApplied={appliedOnPage(changed)} onClose={() => setOpen('')} />}
          {open === 'edit' && <EditDetails target={t.target} id={t.id} start="match" onClose={() => setOpen('')} onChanged={changed} />}
          {open === 'refresh' && <RefreshDialog target={t.target} id={t.id} onClose={() => setOpen('')} onChanged={changed} />}
          {discMenu.dialog(changed)}
          {open === 'info' && item && <InfoDialog target="item" id={item.id} title={item.showTitle ? `${item.showTitle} · ${item.title}` : item.title} onClose={() => setOpen('')} onChanged={changed} />}
          {open === 'canvas' && item && <CanvasManager itemId={item.id} title={item.title} kind={isDisc(item) ? 'disc' : 'video'} runtimeSec={item.runtimeSec} onClose={() => { setOpen(''); changed() }} />}
        </Suspense>
      </>
    )
  }
  const disc = item ? isDisc(item) : false
  const main = item?.disc?.titles.find((x) => x.number === item.disc?.mainTitle)
  const sizeGB = item?.sizeBytes ? (item.sizeBytes / 1e9).toFixed(2) + ' GB' : ''
  return (
    <AtPageEnd>
    <div className="sdui-extras">
      <Credit provider={title.provider} providerId={title.providerId} />
      {item && (item.kind === 'video' || disc) && <CustomInfoNote itemId={item.id} />}
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
    </AtPageEnd>
  )
}

// AtPageEnd draws its children at the end of the page (SduiPage's end slot), so a title's facts, listings and small
// print come after what the page is about; with no slot (a preview), they stay where they are.
function AtPageEnd({ children }: { children: ReactNode }) {
  const [slot, setSlot] = useState<Element | null>(null)
  useEffect(() => { setSlot(document.querySelector('[data-sdui-end]')) }, [])
  return slot ? createPortal(children, slot) : <>{children}</>
}

// A show's tools: its episodes' layout (the show's own, which an administrator keeps for
// everyone, else the server's default), and for an administrator the same menu as a film's
// with "Find intros and credits", which compares a season's episodes by ear as a task.
function ShowTools({ show, admin, onPick, open, close, t, changed }: {
  show: Show; admin: boolean; onPick: (w: 'art' | 'edit' | 'refresh' | 'info') => void; open: string; close: () => void; t: NonNullable<Target>; changed: () => void
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
    { key: 'info', label: 'Skip defaults for every episode', icon: <IconChapters />, onSelect: () => onPick('info') },
  ]
  const seasons = ((show as Show & { seasons?: number[] }).seasons || []) as number[]
  const season = Number(new URLSearchParams(location.search).get('season'))
  return (
    <>
      <Menu label="Manage this show" align="end" items={entries} trigger={(props) => <button type="button" {...props} className="btn ghost square" aria-label="Manage this show" title="Manage this show"><IconMore /></button>} />
      <Suspense fallback={null}>
        {open === 'art' && <TitleArtPicker target={t.target} id={t.id} onApplied={appliedOnPage(changed)} onClose={close} />}
        {open === 'edit' && <EditDetails target={t.target} id={t.id} start="match" onClose={close} onChanged={changed} />}
        {open === 'refresh' && <RefreshDialog target={t.target} id={t.id} onClose={close} onChanged={changed} />}
        {open === 'info' && <ShowInfo show={show} seasons={seasons} season={season >= 0 && new URLSearchParams(location.search).has('season') ? season : null} close={close} changed={changed} />}
      </Suspense>
    </>
  )
}

// The show's seasons, for its skip defaults: one file for the whole show, or one a season.
function ShowInfo({ show, seasons, season, close, changed }: { show: Show; seasons: number[]; season: number | null; close: () => void; changed: () => void }) {
  const [list, setList] = useState<number[]>(seasons)
  useEffect(() => {
    if (seasons.length) return
    api.get<{ seasons?: { number: number }[] }>(`/shows/${show.id}`).then((r) => setList((r.seasons || []).map((s) => s.number))).catch(() => {})
  }, [show.id]) // eslint-disable-line react-hooks/exhaustive-deps
  return <InfoDialog target="show" id={show.id} title={show.title} seasons={list} season={season != null && season > 0 && list.includes(season) ? season : null} onClose={close} onChanged={changed} />
}

// CustomInfoNote says when a .wittyinfo decides where this video's skips are, and which one.
function CustomInfoNote({ itemId }: { itemId: number }) {
  const [m, setM] = useState<MarkerAnswer | null>(null)
  useEffect(() => {
    const stop = new AbortController()
    api.get<MarkerAnswer>(`/media/${itemId}/markers`, stop.signal).then(setM).catch(() => {})
    return () => stop.abort()
  }, [itemId])
  if (!usesWittyinfo(m)) return null
  const from = [...new Set(Object.values(m?.sources || {}).filter((s) => s.startsWith('wittyinfo')))].map((s) => sourceWords(s).replace(/^From the /, 'the ')).join(' and ')
  const parts = (m?.segments || []).filter((s) => s.source.startsWith('wittyinfo')).length
  return (
    <p className="wi-note" role="note">
      <span className="wi-badge file">Custom info</span> {parts} skip{parts === 1 ? '' : 's'} from {from || 'a .wittyinfo'}, in place of what was found by itself.
    </p>
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
