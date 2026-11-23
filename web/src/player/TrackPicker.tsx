// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { language, t } from '../i18n'
import { IconClose } from '../icons'
import { getTvMode } from '../tvmode/mode'
import { SEARCH_FROM, matchTrack, orderTracks, recentLangs, rememberLang, type Group, type Track } from './trackOrder'

// TrackPicker chooses a sound track or subtitles. On a computer or a phone it is a compact list
// beside the controls, in the quality menu's style: Off, this device's languages, the ones picked
// lately, then the rest by name, with a search box once there are more than a handful. In TV mode
// it is a grid of large tiles read from across the room (same order, no typing).
//
// Keyboard and TV remotes: arrows move, Enter or OK picks, Esc or Back closes. Typing in the list
// goes to the search box. Screen readers hear a radio group with the current one checked.

export type Tile = Track

export default function TrackPicker({ heading, kind, tiles, onPick, onClose }: {
  heading: string; kind: 'audio' | 'subtitle'; tiles: Tile[]; onPick: (id: number) => void; onClose: () => void
}) {
  const tv = getTvMode()
  const root = useRef<HTMLDivElement | null>(null)
  const search = useRef<HTMLInputElement | null>(null)
  const [query, setQuery] = useState('')
  const titleId = useId()
  const ordered = useMemo(() => {
    const device = [language(), ...(typeof navigator !== 'undefined' ? navigator.languages || [navigator.language] : [])]
    return orderTracks(tiles, device, recentLangs(kind))
  }, [tiles, kind])
  const shown = ordered.filter(({ track }) => track.id < 0 || matchTrack(track, query))
  const searchable = !tv && tiles.length >= SEARCH_FROM
  const pick = (tile: Tile) => { rememberLang(kind, tile.lang); onPick(tile.id); onClose() }

  // Focus the one in use, so Enter keeps the current choice.
  useEffect(() => {
    const el = root.current?.querySelector<HTMLElement>('[aria-checked="true"]') || root.current?.querySelector<HTMLElement>('[role="radio"]')
    el?.focus()
  }, [])
  // A press outside the list closes it (the button that opened it toggles it itself).
  useEffect(() => {
    if (tv) return
    const away = (e: PointerEvent) => {
      const el = e.target as Element | null
      if (!el || root.current?.contains(el) || el.closest('[aria-haspopup]')) return
      onClose()
    }
    document.addEventListener('pointerdown', away, true)
    return () => document.removeEventListener('pointerdown', away, true)
  }, [tv, onClose])

  const onKey = (e: React.KeyboardEvent) => {
    const inSearch = e.target === search.current
    if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack' || (e.key === 'Backspace' && !inSearch)) {
      e.preventDefault(); e.stopPropagation()
      if (inSearch && query && e.key === 'Escape') setQuery('')
      else onClose()
      return
    }
    const items = Array.from(root.current?.querySelectorAll<HTMLElement>('[role="radio"]') || [])
    const i = items.indexOf(document.activeElement as HTMLElement)
    let next = -2
    if (tv) {
      const cols = Math.max(1, Math.round((root.current?.querySelector('.picker-grid')?.clientWidth || 1) / ((items[0]?.offsetWidth || 1) + 16)))
      if (e.key === 'ArrowRight') next = Math.min(items.length - 1, i + 1)
      if (e.key === 'ArrowLeft') next = Math.max(0, i - 1)
      if (e.key === 'ArrowDown') next = Math.min(items.length - 1, i + cols)
      if (e.key === 'ArrowUp') next = Math.max(0, i - cols)
    } else {
      if (e.key === 'ArrowDown') next = inSearch ? 0 : Math.min(items.length - 1, i + 1)
      if (e.key === 'ArrowUp' && !inSearch) next = i <= 0 && searchable ? -1 : Math.max(0, i - 1)
      if ((e.key === 'Home' || e.key === 'End') && !inSearch) next = e.key === 'Home' ? 0 : items.length - 1
      // Letters typed on the list go to the search box.
      if (searchable && !inSearch && e.key.length === 1 && /\S/.test(e.key) && !e.ctrlKey && !e.metaKey && !e.altKey) {
        search.current?.focus()
        e.stopPropagation()
        return
      }
    }
    if (next === -2) {
      // The player's own keys (seek, volume, play) stay out of the list.
      if (e.key.startsWith('Arrow') || e.key === ' ' || e.key === 'Enter' || inSearch) e.stopPropagation()
      return
    }
    e.preventDefault(); e.stopPropagation()
    if (next === -1) search.current?.focus()
    else items[next]?.focus()
  }

  const words: Record<Group, string> = { off: '', device: t('Your language'), recent: t('Picked lately'), other: kind === 'audio' ? t('Other tracks') : t('Other languages') }
  const groups = (['off', 'device', 'recent', 'other'] as Group[]).map((g) => ({ g, rows: shown.filter((x) => x.group === g) })).filter((x) => x.rows.length)
  // Headings only help when there is more than one kind of row.
  const headed = groups.filter((x) => x.g !== 'off').length > 1

  if (tv) {
    return (
      <div className="picker-backdrop" onClick={(e) => { e.stopPropagation(); onClose() }} onDoubleClick={(e) => e.stopPropagation()}>
        <div className="picker" ref={root} onClick={(e) => e.stopPropagation()} onKeyDown={onKey} role="dialog" aria-labelledby={titleId}>
          <div className="picker-head">
            <h2 id={titleId}>{heading}</h2>
            <button type="button" className="ibtn picker-x" onClick={onClose} aria-label={t('Close')}><IconClose /></button>
          </div>
          <div className="picker-grid" role="radiogroup" aria-labelledby={titleId}>
            {shown.map(({ track: tile }) => (
              <button key={tile.id} type="button" role="radio" aria-checked={tile.active} className={'picker-tile' + (tile.active ? ' active' : '')} onClick={() => pick(tile)}>
                <span className="pt-title">{tile.title}</span>
                {tile.sub && <span className="pt-sub">{tile.sub}</span>}
                {tile.detail && <span className="pt-detail">{tile.detail}</span>}
                {tile.active && <span className="pt-check" aria-hidden="true">✓</span>}
              </button>
            ))}
          </div>
          <button type="button" className="btn ghost picker-close" onClick={onClose}>{t('Close')}</button>
        </div>
      </div>
    )
  }

  return (
    <div className="qmenu tmenu" ref={root} role="dialog" aria-labelledby={titleId} onKeyDown={onKey} onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
      <div className="qm-head">
        <h4 id={titleId}>{heading}</h4>
        <button type="button" className="ibtn qm-close" onClick={onClose} aria-label={t('Close')} title={t('Close')}>✕</button>
      </div>
      {searchable && (
        <input ref={search} type="search" className="tm-search" value={query} onChange={(e) => setQuery(e.target.value)}
          placeholder={kind === 'audio' ? t('Find a track') : t('Find a language')} aria-label={kind === 'audio' ? t('Find a sound track') : t('Find subtitles by language')} />
      )}
      <div className="qm-list tm-list" role="radiogroup" aria-labelledby={titleId}>
        {groups.map(({ g, rows }) => (
          <div key={g} role="group" aria-label={words[g] || undefined} className="tm-group">
            {headed && words[g] && <div className="tm-head" aria-hidden="true">{words[g]}</div>}
            {rows.map(({ track: tile }) => (
              <button key={tile.id} type="button" role="radio" aria-checked={tile.active} className={'qm-item' + (tile.active ? ' active' : '')}
                tabIndex={tile.active ? 0 : -1} onClick={() => pick(tile)}>
                <span className="qm-label">{tile.title}</span>
                {(tile.sub || tile.detail) && <span className="qm-note">{[tile.sub, tile.detail].filter(Boolean).join(' · ')}</span>}
              </button>
            ))}
          </div>
        ))}
        {query && shown.length <= 1 && <p className="qm-small" role="status">{t('No match for “{query}”.', { query })}</p>}
      </div>
    </div>
  )
}
