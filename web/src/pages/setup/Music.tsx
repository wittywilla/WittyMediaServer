// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { MusicOnlineStep } from '../MusicOnline'
import { Button } from '../../ui'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'

// Lyrics and artists, asked only where music can be played. The questions themselves belong to the
// music pages, which keep them with what they send where.
export function Music({ info, next, back }: StepProps) {
  return (
    <section>
      <StepHead title="Lyrics and artists" lead="Lyrics that follow along with the music, and karaoke. Lyrics beside your songs or inside their files work anyway, with nothing sent anywhere. These two add what they don’t have. Both are free, with no account and no key, and both stay off unless you turn them on." />
      <MusicOnlineStep />
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={next}>Continue</Button></StepActions>
    </section>
  )
}
