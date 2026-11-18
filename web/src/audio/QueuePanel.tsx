// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { fitWidth, fmtTime, posterUrl, sized } from '../api'
import { IconClose, IconDown, IconUp } from '../icons'
import { AddToPlaylist } from './controls'
import type { PlayerView } from './types'

// The queue: what's coming, in the order it will play. Drag a row with a mouse or a finger,
// or use its buttons from a keyboard or a remote. The song playing keeps playing throughout.

// Past this many songs, the list is drawn a window at a time; each row is this tall.
const LONG = 150
const ROW = 58

export function QueuePanel({ p, heading, onClose }: { p: PlayerView; heading: string; onClose: () => void }) {
  const [drag, setDrag] = useState<number | null>(null)
  const [over, setOver] = useState<number | null>(null)
  const total = p.queue.reduce((s, q, i) => (i > p.index ? s + (q.runtimeSec || 0) : s), 0)
  const left = p.queue.length - p.index - 1
  const canEdit = !p.cdActive
  // A long queue only draws the rows near what's showing, with room kept above and below.
  const list = useRef<HTMLOListElement>(null)
  const [view, setView] = useState({ top: 0, height: 800 })
  const long = p.queue.length > LONG
  useEffect(() => {
    const el = list.current
    if (!el || !long) return
    // Open at the song playing.
    el.scrollTop = Math.max(0, p.index * ROW - el.clientHeight / 3)
    let frame = 0
    const look = () => { frame = 0; setView({ top: el.scrollTop, height: el.clientHeight }) }
    const later = () => { if (!frame) frame = requestAnimationFrame(look) }
    look()
    el.addEventListener('scroll', later, { passive: true })
    return () => { cancelAnimationFrame(frame); el.removeEventListener('scroll', later) }
  }, [long]) // eslint-disable-line react-hooks/exhaustive-deps
  const first = long ? Math.max(0, Math.floor(view.top / ROW) - 10) : 0
  const last = long ? Math.min(p.queue.length, Math.ceil((view.top + view.height) / ROW) + 10) : p.queue.length
  return (
    <div className="np-side">
      <div className="np-side-head">
        <h3>{p.live || p.books ? heading : 'Up next'}</h3>
        {!p.live && <span className="np-side-meta">{left > 0 ? `${left} to come · ${fmtTime(total)}` : 'Nothing after this'}</span>}
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label={`Close the ${heading.toLowerCase()}`}><IconClose /></button>
      </div>
      {canEdit && !p.live && <p className="np-hint np-lede">Drag to reorder, press to jump.</p>}
      <ol className="np-queue" ref={list}>
        {first > 0 && <li aria-hidden="true" style={{ height: first * ROW }} />}
        {p.queue.slice(first, last).map((q, k) => { const i = first + k; return (
          <li key={q.id + '-' + i} style={long ? { height: ROW, boxSizing: 'border-box' } : undefined} className={(i === p.index ? 'on' : '') + (over === i && drag !== null && drag !== i ? ' over' : '')}
            draggable={canEdit && !p.live} onDragStart={(e) => { setDrag(i); e.dataTransfer.effectAllowed = 'move' }}
            onDragOver={(e) => { if (drag !== null) { e.preventDefault(); setOver(i) } }} onDragEnd={() => { setDrag(null); setOver(null) }}
            onDrop={(e) => { e.preventDefault(); if (drag !== null && drag !== i) p.moveInQueue(drag, i); setDrag(null); setOver(null) }}>
            <button type="button" className="np-q-play" onClick={() => p.jump(i)} aria-current={i === p.index ? 'true' : undefined}>
              <span className="np-q-art">{q.poster && <img src={sized(posterUrl(q), fitWidth(64))} alt="" loading="lazy" />}{i === p.index && p.playing && <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i>}</span>
              <span className="np-q-text"><b>{q.title}</b><small>{p.live ? (q.radio?.country || '') : q.artist || q.albumArtist || ''}</small></span>
              {!p.live && <span className={'np-q-time np-pill' + (i === p.index ? ' now' : '')}>{i === p.index ? 'Now' : fmtTime(q.runtimeSec)}</span>}
            </button>
            {canEdit && !p.live && (
              <span className="np-q-edit">
                <button type="button" className="mp-ibtn small" aria-label={`Move ${q.title} up`} disabled={i === 0} onClick={() => p.moveInQueue(i, i - 1)}><IconUp size={18} /></button>
                <button type="button" className="mp-ibtn small" aria-label={`Move ${q.title} down`} disabled={i === p.queue.length - 1} onClick={() => p.moveInQueue(i, i + 1)}><IconDown size={18} /></button>
                <button type="button" className="mp-ibtn small" aria-label={`Remove ${q.title} from the queue`} onClick={() => p.removeFromQueue(i)}><IconClose size={18} /></button>
              </span>
            )}
          </li>
        ) })}
        {last < p.queue.length && <li aria-hidden="true" style={{ height: (p.queue.length - last) * ROW }} />}
      </ol>
      {canEdit && !p.live && !p.books && (
        <div className="np-side-actions np-pills">
          <AddToPlaylist items={p.queue} label="Save the queue as a playlist" />
          <button type="button" className="np-chip small" disabled={left === 0} onClick={p.clearUpNext}>Clear what's to come</button>
        </div>
      )}
    </div>
  )
}
