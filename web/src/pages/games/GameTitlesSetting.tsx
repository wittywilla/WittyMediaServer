// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Choosing how games are named (/me/ui gameTitles, see titles.ts): Settings' section. GameName.tsx has the sample
// each place offering the choice shows, so the choice can be seen before it's made.
import { Segmented } from '../../ui'
import { Section } from '../settings/ui'
import { GameTitleSample } from './GameName'
import { GAME_TITLE_CHOICES, type GameTitlePref } from './titles'
import { useGameTitlePref } from './useGameTitles'

const SHORT: Record<GameTitlePref, string> = { romanized: 'Romanized', native: 'Original', english: 'English', both: 'Both' }

// The choice and its sample, for Settings' My game settings.
export function GameTitlesSetting() {
  const [pref, setPref] = useGameTitlePref()
  const choice = GAME_TITLE_CHOICES.find((c) => c.id === pref)
  return (
    <Section title="Game titles" desc="How games are named for you, where a game’s details know its other names. A name that isn’t known shows the usual title instead.">
      <Segmented label="Game titles" value={pref} onChange={setPref} options={GAME_TITLE_CHOICES.map((c) => ({ value: c.id, label: SHORT[c.id] }))} />
      <p className="game-title-setting-note"><b>{choice?.label}</b>: {choice?.note}.</p>
      <GameTitleSample pref={pref} className="big" />
    </Section>
  )
}
