// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { IconCheck, IconDown, IconEdit, IconPlus } from '../../icons'
import { Button, Dialog, Input } from '../../ui'
import { sectionsOf, type TvSet } from './guideLogic'

// Which channels the guide shows: one button on the guide's bar with the set's name, and a list
// of every set to choose from, with a search, in sections. A playlist can have hundreds of
// groups, which no row of buttons could hold. The list is the app's dialog, so the keyboard and
// a remote stay in it and Back closes it.

export function SetButton({ set, onOpen }: { set?: TvSet; onOpen: () => void }) {
  const name = set?.name || 'All channels'
  return (
    <button type="button" className="ctv-btn sm ctv-setbtn" aria-haspopup="dialog" onClick={onOpen} title="Which channels (C, [ and ])"
      aria-label={`Which channels: ${name}${set ? `, ${set.count}` : ''}`}>
      <span className="ctv-setbtn-name">{name}</span>
      {set && <small>{set.count.toLocaleString()}</small>}
      <IconDown size={14} />
    </button>
  )
}

export default function SetPicker({ sets, current, onPick, onClose }: { sets: TvSet[]; current: string; onPick: (id: string) => void; onClose: () => void }) {
  const nav = useNavigate()
  const [q, setQ] = useState('')
  const sections = useMemo(() => sectionsOf(sets, q), [sets, q])
  const found = sections.reduce((n, s) => n + s.sets.length, 0)
  return (
    <Dialog className="ctv-sets" title="Which channels" onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>Close</Button>
        <Button icon={<IconEdit />} onClick={() => { onClose(); nav('/tv/sets') }}>Edit sets…</Button>
        <Button icon={<IconPlus />} onClick={() => { onClose(); nav('/tv/sets/new') }}>New set…</Button>
      </>}>
      {sets.length > 8 && <Input type="search" autoFocus aria-label="Find a set" placeholder="Find a set or a group…" value={q} onChange={(e) => setQ(e.target.value)} />}
      <div aria-live="polite" className="sr-only">{q ? `${found} found` : ''}</div>
      {found === 0 && <p className="muted">No set or group is called that.</p>}
      <div className="ctv-sets-list">
        {sections.map((sec) => (
          <section key={sec.title} aria-label={sec.title}>
            <h3>{sec.title}</h3>
            <ul>
              {sec.sets.map((s) => {
                const on = s.id === current
                return (
                  <li key={s.id}>
                    <button type="button" className={'ctv-set' + (on ? ' on' : '')} aria-current={on ? 'true' : undefined} autoFocus={on && sets.length <= 8}
                      onClick={() => { onPick(s.id); onClose() }}>
                      <span className="ctv-set-check" aria-hidden="true">{on && <IconCheck size={14} />}</span>
                      <b>{s.name}</b>
                      {s.shared && <span className="ctv-set-tag">Shared</span>}
                      <small>{s.count.toLocaleString()} {s.count === 1 ? 'channel' : 'channels'}</small>
                    </button>
                  </li>
                )
              })}
            </ul>
          </section>
        ))}
      </div>
    </Dialog>
  )
}
