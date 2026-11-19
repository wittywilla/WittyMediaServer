// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { MusicOnlineStep, SeparatorStep } from '../MusicOnline'
import { Button } from '../../ui'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'
import { t } from '../../i18n'

// Lyrics and artists, asked only where music can be played. The questions themselves belong to the
// music pages, which keep them with what they send where.
export function Music({ info, next, back }: StepProps) {
  return (
    <section>
      <StepHead title={t('Lyrics and artists')} lead={t('Lyrics that follow along with the music, and karaoke. Lyrics beside your songs or inside their files work anyway, with nothing sent anywhere. These two add what they don’t have. Both are free, with no account and no key, and both stay off unless you turn them on.')} />
      <MusicOnlineStep />
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={next}>{t('Continue')}</Button></StepActions>
    </section>
  )
}

// Karaoke voices, where music can be played: Demucs takes the singer out properly. Karaoke works without it, with the quick
// version, so this is only offered, and installing goes on after Continue.
export function Karaoke({ info, next, back }: StepProps) {
  return (
    <section>
      <StepHead title={t('Separate voices for karaoke')} lead={t('Karaoke works already: the quick version turns the singer down. Demucs, a free program, takes the voice out properly. It is a large download, uses the graphics card when it can, and installs in the background while you carry on.')} />
      <SeparatorStep />
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={next}>{t('Continue')}</Button></StepActions>
    </section>
  )
}
