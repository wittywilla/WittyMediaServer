// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { HelpLink } from '../../components/HelpLink'
import { AUTO_SKIP_KEY, autoSkipPrefs, type AutoSkipPrefs } from '../../player/markers'
import { toast } from '../../state'
import { useUserPref } from '../../ui/userPrefs'
import { FILTER_CATEGORIES, FILTER_PREFS_KEY, filterPrefs, type FilterChoice, type FilterPrefs } from '../../wittyinfo/model'
import { Row, Section, Switch } from './ui'
import { t, tk, tx } from '../../i18n'

// Skipping without pressing anything: each profile's own choice, kept with the account so it
// follows the person to every device. Saved as it's changed, like the other choices that
// aren't the server's.
export default function AutoSkipSection() {
  const [raw, setRaw] = useUserPref<unknown>(AUTO_SKIP_KEY, null)
  const p = autoSkipPrefs(raw)
  const set = (patch: Partial<AutoSkipPrefs>) => { setRaw({ ...p, ...patch }).catch(() => toast(t('Could not save that'), { tone: 'danger' })) }
  return (
    <Section title={tk('Skipping')} desc={<>{t('What the player skips by itself, for the profile signed in here. It follows you to your other devices, and is saved straight away. Each time, a note offers Undo, and a part you went back to is left to play. With all of these off, the player shows a button to press instead.')} <HelpLink to="playing#skipping-without-pressing-anything">{t('How skipping works')}</HelpLink></>}>
      <Row label={t('Skip intros')} desc={t('Jumps to where the intro ends.')}>
        <Switch label={t('Skip intros')} checked={p.intro} onChange={(intro) => set({ intro })} />
      </Row>
      <Row label={t('Skip recaps')} desc={t('The “previously on” before an episode.')}>
        <Switch label={t('Skip recaps')} checked={p.recap} onChange={(recap) => set({ recap })} />
      </Row>
      <Row label={t('Skip credits')} desc={t('Goes on to the next episode when the credits start, after a short countdown you can stop. With no next episode, it jumps past the credits only when something comes after them.')}>
        <Switch label={t('Skip credits')} checked={p.credits} onChange={(credits) => set({ credits })} />
      </Row>
      <Row label={t('Skip adverts')} desc={t('Advert breaks left in a recording.')}>
        <Switch label={t('Skip adverts')} checked={p.adverts} onChange={(adverts) => set({ adverts })} />
      </Row>
      <Row label={t('Skip previews')} desc={t('A look at the next episode, where someone marked one.')}>
        <Switch label={t('Skip previews')} checked={p.preview} onChange={(preview) => set({ preview })} />
      </Row>
      <Row label={t('Skip other marked parts')} desc={t('Anything else someone marked to skip in a video’s scenes and skips.')}>
        <Switch label={t('Skip other marked parts')} checked={p.custom} onChange={(custom) => set({ custom })} />
      </Row>
      <Row label={t('Do as the video’s file says')} desc={t('A video’s scenes and skips can say to skip a part by itself. Turn this off to be offered a button for those instead.')}>
        <Switch label={t('Do as the video’s file says')} checked={p.fileAuto} onChange={(fileAuto) => set({ fileAuto })} />
      </Row>
    </Section>
  )
}

// Content: what this profile does about what a video's scenes and skips mark (violence,
// strong language...). Nothing is done unless chosen here. Each profile chooses for itself,
// so a child's profile can skip what an adult's plays.
export function ContentFiltersSection() {
  const [raw, setRaw] = useUserPref<unknown>(FILTER_PREFS_KEY, null)
  const p = filterPrefs(raw)
  const set = (patch: FilterPrefs) => { setRaw({ ...p, ...patch }).catch(() => toast(t('Could not save that'), { tone: 'danger' })) }
  const choices: { value: FilterChoice; label: string }[] = [{ value: 'off', label: t('Play it') }, { value: 'warn', label: t('Warn me') }, { value: 'mute', label: t('Mute it') }, { value: 'skip', label: t('Skip it') }]
  return (
    <Section title={t('Content')} desc={t('What to do about parts of a video someone marked in its scenes and skips, for the profile signed in here. Videos without such marks play as they are.')}>
      {FILTER_CATEGORIES.map((c) => (
        <Row key={c.value} label={tx(c.label)}>
          <select className="select sm" aria-label={tx(c.label)} value={p[c.value] || 'off'} onChange={(e) => set({ [c.value]: e.target.value as FilterChoice })}>
            {choices.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </Row>
      ))}
      <Row label={t('Only when it is at least')} desc={t('Marks that say how strong something is: milder ones play as they are.')}>
        <select className="select sm" aria-label={t('Only when it is at least')} value={String(p.minSeverity || 0)} onChange={(e) => set({ minSeverity: Number(e.target.value) || undefined })}>
          <option value="0">{t('Any strength')}</option><option value="2">{t('Moderate')}</option><option value="3">{t('Strong')}</option>
        </select>
      </Row>
    </Section>
  )
}
