// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef } from 'react'
import { t } from '../i18n'
import { IconClose } from '../icons'

// TrackPicker is a full-screen chooser with large square tiles (in the style
// of a TV app's dialogs) for audio tracks, subtitles and video angles. Works
// with mouse, touch, keyboard and TV remotes/controllers (arrows + Enter,
// Back/Escape closes).

export type Tile = { id: number; title: string; sub?: string; detail?: string; active: boolean }

export default function TrackPicker({ heading, hint, tiles, onPick, onClose }: {
  heading: string; hint?: string; tiles: Tile[]; onPick: (id: number) => void; onClose: () => void
}) {
  const grid = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    // Focus the active tile so a remote's Enter confirms the current choice.
    const el = grid.current?.querySelector<HTMLButtonElement>('button.active') || grid.current?.querySelector('button')
    el?.focus()
  }, [])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const btns = Array.from(grid.current?.querySelectorAll<HTMLButtonElement>('button') || [])
      const i = btns.indexOf(document.activeElement as HTMLButtonElement)
      const cols = Math.max(1, Math.round((grid.current?.clientWidth || 1) / ((btns[0]?.offsetWidth || 1) + 16)))
      let next = -1
      switch (e.key) {
        case 'ArrowRight': next = Math.min(btns.length - 1, i + 1); break
        case 'ArrowLeft': next = Math.max(0, i - 1); break
        case 'ArrowDown': next = Math.min(btns.length - 1, i + cols); break
        case 'ArrowUp': next = Math.max(0, i - cols); break
        case 'Escape': case 'Backspace': case 'GoBack': case 'BrowserBack': e.preventDefault(); e.stopPropagation(); onClose(); return
        default: return
      }
      e.preventDefault(); e.stopPropagation()
      btns[next]?.focus()
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onClose])
  return (
    <div className="picker-backdrop" onClick={(e) => { e.stopPropagation(); onClose() }} onDoubleClick={(e) => e.stopPropagation()}>
      <div className="picker" onClick={(e) => e.stopPropagation()} role="dialog" aria-label={heading}>
        <div className="picker-head">
          <h2>{heading}</h2>
          <button type="button" className="ibtn picker-x" onClick={onClose} aria-label={t('Close')} title={t('Close')}><IconClose /></button>
        </div>
        {hint && <p className="picker-hint">{hint}</p>}
        <div className="picker-grid" ref={grid}>
          {tiles.map((tile) => (
            <button key={tile.id} type="button" className={'picker-tile' + (tile.active ? ' active' : '')} onClick={() => { onPick(tile.id); onClose() }}>
              <span className="pt-title">{tile.title}</span>
              {tile.sub && <span className="pt-sub">{tile.sub}</span>}
              {tile.detail && <span className="pt-detail">{tile.detail}</span>}
              {tile.active && <span className="pt-check">✓</span>}
            </button>
          ))}
        </div>
        <button type="button" className="btn ghost picker-close" onClick={onClose}>{t('Close')}</button>
      </div>
    </div>
  )
}
