// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { HelpLink } from '../../components/HelpLink'
import { AUTO_SKIP_KEY, autoSkipPrefs, type AutoSkipPrefs } from '../../player/markers'
import { toast } from '../../state'
import { useUserPref } from '../../ui/userPrefs'
import { Row, Section, Switch } from './ui'

// Skipping without pressing anything: each profile's own choice, kept with the account so it
// follows the person to every device. Saved as it's changed, like the other choices that
// aren't the server's.
export default function AutoSkipSection() {
  const [raw, setRaw] = useUserPref<unknown>(AUTO_SKIP_KEY, null)
  const p = autoSkipPrefs(raw)
  const set = (patch: Partial<AutoSkipPrefs>) => { setRaw({ ...p, ...patch }).catch(() => toast('Could not save that', { tone: 'danger' })) }
  return (
    <Section title="Skipping" desc={<>What the player skips by itself, for the profile signed in here. It follows you to your other devices, and is saved straight away. Each time, a note offers <b>Undo</b>, and a part you went back to is left to play. With all of these off, the player shows a button to press instead. <HelpLink to="playing#skipping-without-pressing-anything">How skipping works</HelpLink></>}>
      <Row label="Skip intros" desc="Jumps to where the intro ends.">
        <Switch label="Skip intros" checked={p.intro} onChange={(intro) => set({ intro })} />
      </Row>
      <Row label="Skip recaps" desc="The “previously on” before an episode.">
        <Switch label="Skip recaps" checked={p.recap} onChange={(recap) => set({ recap })} />
      </Row>
      <Row label="Skip credits" desc="Goes on to the next episode when the credits start, after a short countdown you can stop. With no next episode, it jumps past the credits only when something comes after them.">
        <Switch label="Skip credits" checked={p.credits} onChange={(credits) => set({ credits })} />
      </Row>
      <Row label="Skip adverts" desc="Advert breaks left in a recording.">
        <Switch label="Skip adverts" checked={p.adverts} onChange={(adverts) => set({ adverts })} />
      </Row>
    </Section>
  )
}
