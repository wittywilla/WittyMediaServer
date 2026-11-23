// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { Button } from '../../ui'
import { ChoiceCards, StepActions, StepHead, TickRow, type StepProps } from './parts'
import { t } from '../../i18n'

// Advanced setup: how copies of DVDs and Blu-rays start. Only where a disc module is.
export function Discs({ next, back }: StepProps) {
  const [start, setStart] = useState<'menu' | 'title'>('menu')
  const [fx, setFx] = useState(true)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => { setStart(s.dvdStart === 'title' ? 'title' : 'menu'); setFx(s.insertFx !== '0') }).catch(() => {}) }, [])
  const save = async () => {
    setBusy(true)
    try { await api.put('/settings', { dvdStart: start, insertFx: fx ? '1' : '0' }) } catch { /* can be changed later */ }
    setBusy(false)
    next()
  }
  return (
    <section>
      <StepHead title={t('DVDs and Blu-rays')} lead={t('What happens when someone presses play on a disc?')} />
      <ChoiceCards label={t('When a disc starts')} value={start} onChange={setStart} cards={[
        { value: 'menu', title: t('Show the disc’s menu'), text: t('Studio intros, then the menu with its buttons, chapters and extras.'), pic: <span className="oobe-choice-pic menu" aria-hidden="true"><i /><i /><i /></span> },
        { value: 'title', title: t('Play the movie straight away'), text: t('Skips the intros and menus. The menu is still one button away.'), pic: <span className="oobe-choice-pic film" aria-hidden="true"><i /></span> },
      ]} />
      <TickRow kind="checkbox" checked={fx} onChange={setFx} title={t('Disc animation')} text={t('A tray slides in, with drive sounds.')} />
      <p className="muted oobe-hint su-gap">{t('Discs with copy protection can’t be played.')}</p>
      <StepActions back={back}><Button variant="primary" size="big" busy={busy} onClick={save}>{t('Continue')}</Button></StepActions>
    </section>
  )
}
