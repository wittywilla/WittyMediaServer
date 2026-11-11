// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What the web app adds to a title's server-driven header from its own stock: the tools
// an administrator has for a title (artwork, details, refresh), the credit a metadata
// source asks for, the file's technical facts, and a disc's titles and extras. The page
// model has no components for these; they are the app's own, as on its hand-built pages.

import { lazy, Suspense, useEffect, useState } from 'react'
import { api, codecName, discName, isBluray, isDisc, langName, type Item, type Show } from '../../api'
import TitleArtPicker, { appliedOnPage } from '../../components/edit/TitleArtPicker'
import LegalNote from '../../components/LegalNote'
import { IconEdit, IconImage, IconMore, IconRefresh } from '../../icons'
import { useApp } from '../../state'
import { Menu, type MenuEntry } from '../../ui/Menu'
import { bump } from '../client'
import type { Document, Node } from '../types'

const EditDetails = lazy(() => import('../../components/edit/EditDetails'))
const RefreshDialog = lazy(() => import('../../components/edit/RefreshDialog'))
const TitleExtras = lazy(() => import('../../components/edit/TitleExtras'))
const DiscSections = lazy(() => import('../../pages/DiscSections'))

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
  const [open, setOpen] = useState<'' | 'art' | 'edit' | 'refresh'>('')
  if (!t || !title) return null
  const item = t.target === 'item' ? title as Item : null
  const show = t.target === 'show' ? title as Show : null
  const changed = () => { asked.clear(); bump() }
  if (place === 'tools') {
    if (!user?.isAdmin) return null
    const entries: MenuEntry[] = [
      { key: 'art', label: 'Artwork', icon: <IconImage />, onSelect: () => setOpen('art') },
      { key: 'edit', label: 'Edit details', icon: <IconEdit />, onSelect: () => setOpen('edit') },
      ...(show || item?.showId == null ? [{ key: 'refresh', label: 'Check for new details', icon: <IconRefresh />, onSelect: () => setOpen('refresh') } as MenuEntry] : []),
    ]
    return (
      <>
        <Menu label="Manage this title" align="end" items={entries} trigger={(props) => <button type="button" {...props} className="btn ghost square" aria-label="Manage this title" title="Manage this title"><IconMore /></button>} />
        <Suspense fallback={null}>
          {open === 'art' && <TitleArtPicker target={t.target} id={t.id} onApplied={appliedOnPage(changed)} onClose={() => setOpen('')} />}
          {open === 'edit' && <EditDetails target={t.target} id={t.id} start="match" onClose={() => setOpen('')} onChanged={changed} />}
          {open === 'refresh' && <RefreshDialog target={t.target} id={t.id} onClose={() => setOpen('')} onChanged={changed} />}
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
      {item && disc && item.disc?.scrambled && <div className="error">This disc is encrypted, so it can't be played: encrypted media isn't supported, and nothing here bypasses copy protection.</div>}
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
        {item && disc && <DiscSections item={item} admin={!!user?.isAdmin} playable={!item.disc?.scrambled} />}
      </Suspense>
      <LegalNote kind="metadata" />
    </div>
  )
}
