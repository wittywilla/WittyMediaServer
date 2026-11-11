// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { fitWidth, fmtTime, posterUrl, sized } from '../api'
import { IconClose, IconDown, IconUp } from '../icons'
import { AddToPlaylist } from './controls'
import type { PlayerView } from './types'

// The queue: what's coming, in the order it will play. Drag a row with a mouse or a finger,
// or use its buttons from a keyboard or a remote. The song playing keeps playing throughout.

export function QueuePanel({ p, heading, onClose }: { p: PlayerView; heading: string; onClose: () => void }) {
  const [drag, setDrag] = useState<number | null>(null)
  const [over, setOver] = useState<number | null>(null)
  const total = p.queue.reduce((s, q, i) => (i > p.index ? s + (q.runtimeSec || 0) : s), 0)
  const left = p.queue.length - p.index - 1
  const canEdit = !p.cdActive
  return (
    <div className="np-side">
      <div className="np-side-head">
        <h3>{heading}</h3>
        {!p.live && <span className="np-side-meta">{left > 0 ? `${left} to come · ${fmtTime(total)}` : 'Nothing after this'}</span>}
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label={`Close the ${heading.toLowerCase()}`}><IconClose /></button>
      </div>
      {canEdit && !p.live && !p.books && (
        <div className="np-side-actions">
          <AddToPlaylist items={p.queue} label="Save the queue as a playlist" />
          <button type="button" className="np-chip small" disabled={left === 0} onClick={p.clearUpNext}>Clear what's to come</button>
        </div>
      )}
      <ol className="np-queue">
        {p.queue.map((q, i) => (
          <li key={q.id + '-' + i} className={(i === p.index ? 'on' : '') + (over === i && drag !== null && drag !== i ? ' over' : '')}
            draggable={canEdit && !p.live} onDragStart={(e) => { setDrag(i); e.dataTransfer.effectAllowed = 'move' }}
            onDragOver={(e) => { if (drag !== null) { e.preventDefault(); setOver(i) } }} onDragEnd={() => { setDrag(null); setOver(null) }}
            onDrop={(e) => { e.preventDefault(); if (drag !== null && drag !== i) p.moveInQueue(drag, i); setDrag(null); setOver(null) }}>
            <button type="button" className="np-q-play" onClick={() => p.jump(i)} aria-current={i === p.index ? 'true' : undefined}>
              <span className="np-q-art">{q.poster && <img src={sized(posterUrl(q), fitWidth(64))} alt="" loading="lazy" />}{i === p.index && p.playing && <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i>}</span>
              <span className="np-q-text"><b>{q.title}</b><small>{p.live ? (q.radio?.country || '') : q.artist || q.albumArtist || ''}</small></span>
              {!p.live && <span className="np-q-time">{fmtTime(q.runtimeSec)}</span>}
            </button>
            {canEdit && !p.live && (
              <span className="np-q-edit">
                <button type="button" className="mp-ibtn small" aria-label={`Move ${q.title} up`} disabled={i === 0} onClick={() => p.moveInQueue(i, i - 1)}><IconUp size={18} /></button>
                <button type="button" className="mp-ibtn small" aria-label={`Move ${q.title} down`} disabled={i === p.queue.length - 1} onClick={() => p.moveInQueue(i, i + 1)}><IconDown size={18} /></button>
                <button type="button" className="mp-ibtn small" aria-label={`Remove ${q.title} from the queue`} onClick={() => p.removeFromQueue(i)}><IconClose size={18} /></button>
              </span>
            )}
          </li>
        ))}
      </ol>
    </div>
  )
}
