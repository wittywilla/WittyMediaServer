// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { api, type Library } from '../../api'
import BrandLogo, { forgetLogos, useLogo, type LogoInfo } from '../../components/BrandLogo'
import { BackLink, EmptyState, LoadError, SkeletonGrid, Spinner } from '../../components/common'
import LegalNote from '../../components/LegalNote'
import LibraryHeader, { type SortOption } from '../../components/LibraryHeader'
import LogoChooser from '../../components/edit/LogoChooser'
import type { BrowseResult, BrowseRow } from '../../metadata'
import { useApp } from '../../state'
import SpecPage from '../../spec/SpecPage'
import { handledIds } from '../../spec/model'
import { brandSpec } from '../../spec/specs/brand'
import { RowCard } from './VideoLibrary'
import './library.css'

// Everything on this server from one studio or network, across libraries.

const SORTS: SortOption[] = [
  { key: 'title', label: 'Title A to Z' }, { key: 'newest', label: 'Newest' }, { key: 'oldest', label: 'Oldest' },
  { key: 'added', label: 'Recently added', desc: true }, { key: 'rating', label: 'Highest rated', desc: true }, { key: 'quality', label: 'Best quality', desc: true },
]

const PAGE = 120

export default function BrandPage() {
  const { kind = 'studio', name = '' } = useParams()
  const { user, libraries } = useApp()
  const [params, setParams] = useSearchParams()
  const sort = params.get('sort') || 'title'
  const desc = params.get('desc') === '1'
  const libId = params.get('library')
  const [rows, setRows] = useState<BrowseRow[] | null>(null)
  const [total, setTotal] = useState(0)
  const [failed, setFailed] = useState(false)
  const [changing, setChanging] = useState<LogoInfo | null>(null)
  const [tick, setTick] = useState(0)
  const logoKind = kind === 'network' ? 'network' : 'studio'
  const logo = useLogo(logoKind, name)
  const q = `${logoKind}=${encodeURIComponent(name)}&sort=${sort}&desc=${desc ? 1 : 0}${libId ? `&library=${libId}` : ''}`
  useEffect(() => {
    setRows(null); setFailed(false)
    api.get<BrowseResult>(`/browse?${q}&limit=${PAGE}`).then((r) => { setRows(r.results); setTotal(r.total) }).catch(() => setFailed(true))
  }, [q])
  const more = useRef<HTMLDivElement | null>(null)
  const loaded = rows?.length ?? 0
  const hasMore = loaded > 0 && loaded < total
  useEffect(() => {
    const el = more.current
    if (!el || !hasMore) return
    let busy = false
    const io = new IntersectionObserver((es) => {
      if (!es.some((e) => e.isIntersecting) || busy) return
      busy = true
      api.get<BrowseResult>(`/browse?${q}&limit=${PAGE}&offset=${loaded}`).then((r) => setRows((cur) => [...(cur || []), ...r.results])).catch(() => {}).finally(() => { busy = false })
    }, { rootMargin: '800px' })
    io.observe(el)
    return () => io.disconnect()
  }, [hasMore, loaded, q])
  const lib: Library | undefined = libraries.find((l) => String(l.id) === libId)
  const brand = brandSpec(logoKind, name, !!lib, !!(user?.isAdmin && logo?.id))
  return (
    <div className="lib">
      <div className="lib-brand-hero" key={tick}>
        <span className="lib-brand-mark"><BrandLogo kind={logoKind} name={name} label={false} /></span>
      </div>
      <LibraryHeader
        title={name}
        count={rows ? `${total} ${total === 1 ? 'title' : 'titles'}` : undefined}
        sub={lib?.name}
        sorts={SORTS}
        sort={sort}
        desc={desc}
        onSort={(s, d) => { const p = new URLSearchParams(params); p.set('sort', s); if (d) p.set('desc', '1'); else p.delete('desc'); setParams(p, { replace: true }) }}
        onDirection={sort !== 'newest' && sort !== 'oldest' ? (d) => { const p = new URLSearchParams(params); if (d) p.set('desc', '1'); else p.delete('desc'); setParams(p, { replace: true }) } : undefined}
        actions={brand.sections.length ? <SpecPage embedded page={brand} handled={handledIds(brand)} onAction={(id) => {
          if (id === 'change-logo' && logo) setChanging(logo)
          else if (id === 'all-libraries') { const p = new URLSearchParams(params); p.delete('library'); setParams(p, { replace: true }) }
        }} /> : undefined}
      />
      {!rows ? (failed ? <LoadError what="This page" /> : <SkeletonGrid />) : rows.length === 0 ? <EmptyState title="Nothing here" text="Nothing on this server has this name any more." />
        : <div className="grid">{rows.map((r) => <RowCard key={r.type + (r.type === 'item' ? r.item.id : r.show.id)} row={r} />)}</div>}
      {hasMore && <div ref={more} style={{ minHeight: 1 }}><Spinner /></div>}
      <div className="page-back"><BackLink to={lib ? `/library/${lib.id}` : '/'} label={lib ? `Back to ${lib.name}` : 'Back to home'} /></div>
      <LegalNote kind="logos" />
      {changing && <LogoChooser logo={changing} onClose={() => setChanging(null)} onChanged={() => { forgetLogos(); setTick((n) => n + 1) }} />}
    </div>
  )
}
