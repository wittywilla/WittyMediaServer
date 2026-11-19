// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../api'
import { announce } from '../../a11y'
import { IconPlus } from '../../icons'
import { toast } from '../../state'
import { Button, Dialog } from '../../ui'
import { editableSets, setNumber, type TvSet } from './guideLogic'
import { changeSet, type GuideChannel, type SetDetail } from './tvApi'
import { t, tn } from '../../i18n'

// "Add to set…", from the guide (T) or a programme's details: each set this person may change,
// ticked when the channel is in it. A tick puts it in or takes it out at once.
export default function AddToSet({ ch, sets, onClose, onChanged }: { ch: GuideChannel; sets: TvSet[]; onClose: () => void; onChanged: (setId: string) => void }) {
  const nav = useNavigate()
  const mine = editableSets(sets)
  const [inSet, setInSet] = useState<Record<string, boolean> | null>(null)
  const [busy, setBusy] = useState('')
  useEffect(() => {
    let on = true
    Promise.all(mine.map((s) => api.get<SetDetail>(`/tv-sets/${setNumber(s.id)}`).then((d) => [s.id, d.channels.includes(ch.key)] as const).catch(() => [s.id, false] as const)))
      .then((l) => { if (on) setInSet(Object.fromEntries(l)) })
    return () => { on = false }
  }, [ch.key, sets]) // eslint-disable-line react-hooks/exhaustive-deps
  const toggle = async (s: TvSet, want: boolean) => {
    setBusy(s.id)
    try {
      await changeSet(s.id, want ? [ch.key] : [], want ? [] : [ch.key])
      setInSet((m) => ({ ...m, [s.id]: want }))
      announce(want ? t('{channel} added to {set}', { channel: ch.name, set: s.name }) : t('{channel} taken out of {set}', { channel: ch.name, set: s.name }))
      onChanged(s.id)
    } catch (e) { toast((e as Error).message) } finally { setBusy('') }
  }
  return (
    <Dialog className="ctv-sets" size="sm" title={t('Channel {number}, {name}', { number: ch.number, name: ch.name })} onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>{t('Done')}</Button>
        <Button icon={<IconPlus />} onClick={() => { onClose(); nav(`/tv/sets/new?add=${encodeURIComponent(ch.key)}`) }}>{t('New set with this channel…')}</Button>
      </>}>
      {mine.length === 0 && <p className="muted">{t("You haven't made a set of channels yet. A set is your own list of channels, in your own order, to show in the guide on its own.")}</p>}
      {mine.length > 0 && inSet === null && <div className="center" role="status" aria-label={t('Loading')}><div className="spinner" /></div>}
      {mine.length > 0 && inSet && (
        <fieldset className="ctv-addto">
          <legend>{t('In your sets')}</legend>
          {mine.map((s) => (
            <label key={s.id} className="ctv-addto-row">
              <input type="checkbox" checked={!!inSet[s.id]} disabled={busy === s.id} onChange={(e) => void toggle(s, e.target.checked)} />
              <b>{s.name}</b>
              {s.shared && <span className="ctv-set-tag">{t('Shared')}</span>}
              <small>{tn('{count} channel', '{count} channels', s.count)}</small>
            </label>
          ))}
        </fieldset>
      )}
    </Dialog>
  )
}
