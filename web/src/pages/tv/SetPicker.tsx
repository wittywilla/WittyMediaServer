// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { IconCheck, IconDown, IconEdit, IconPlus } from '../../icons'
import { Button, Dialog, Input } from '../../ui'
import { sectionsOf, type TvSet } from './guideLogic'
import { num, t, tn } from '../../i18n'

// Which channels the guide shows: one button on the guide's bar with the set's name, and a list
// of every set to choose from, with a search, in sections. A playlist can have hundreds of
// groups, which no row of buttons could hold. The list is the app's dialog, so the keyboard and
// a remote stay in it and Back closes it.

export function SetButton({ set, onOpen }: { set?: TvSet; onOpen: () => void }) {
  const name = set?.name || t('All channels')
  return (
    <button type="button" className="ctv-btn sm ctv-setbtn" aria-haspopup="dialog" onClick={onOpen} title={t('Which channels (C, [ and ])')}
      aria-label={set ? t('Which channels: {name}, {count}', { name, count: set.count }) : t('Which channels: {name}', { name })}>
      <span className="ctv-setbtn-name">{name}</span>
      {set && <small>{num(set.count)}</small>}
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
    <Dialog className="ctv-sets" title={t('Which channels')} onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>{t('Close')}</Button>
        <Button icon={<IconEdit />} onClick={() => { onClose(); nav('/tv/sets') }}>{t('Edit sets…')}</Button>
        <Button icon={<IconPlus />} onClick={() => { onClose(); nav('/tv/sets/new') }}>{t('New set…')}</Button>
      </>}>
      {sets.length > 8 && <Input type="search" autoFocus aria-label={t('Find a set')} placeholder={t('Find a set or a group…')} value={q} onChange={(e) => setQ(e.target.value)} />}
      <div aria-live="polite" className="sr-only">{q ? t('{count} found', { count: found }) : ''}</div>
      {found === 0 && <p className="muted">{t('No set or group is called that.')}</p>}
      <div className="ctv-sets-list">
        {sections.map((sec) => (
          <section key={sec.title} aria-label={t(sec.title)}>
            <h3>{t(sec.title)}</h3>
            <ul>
              {sec.sets.map((s) => {
                const on = s.id === current
                return (
                  <li key={s.id}>
                    <button type="button" className={'ctv-set' + (on ? ' on' : '')} aria-current={on ? 'true' : undefined} autoFocus={on && sets.length <= 8}
                      onClick={() => { onPick(s.id); onClose() }}>
                      <span className="ctv-set-check" aria-hidden="true">{on && <IconCheck size={14} />}</span>
                      <b>{s.name}</b>
                      {s.shared && <span className="ctv-set-tag">{t('Shared')}</span>}
                      <small>{tn('{count} channel', '{count} channels', s.count)}</small>
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
