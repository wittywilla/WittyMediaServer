// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, type Library } from '../../api'
import AddLibrary, { GroupingChoice, LibrarySummary, type Grouping } from '../../components/AddLibrary'
import { Button } from '../../ui'
import { LaterNote, StepActions, StepHead, TickRow, type StepProps } from './parts'
import { t } from '../../i18n'

export function Media({ info, next, back, added, onAdded, refresh }: StepProps & { added: Library[]; onAdded: (l: Library) => void }) {
  // While a kind is chosen (its folder, then following podcasts) the panel has its own Back and
  // Add buttons, and Continue would be easy to press by mistake, so it waits until that's closed.
  const [inPanel, setInPanel] = useState(false)
  const [grouping, setGrouping] = useState<Grouping>('combine')
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setGrouping(s.libraryGrouping === 'separate' ? 'separate' : 'combine')).catch(() => {}) }, [])
  const changeGrouping = (g: Grouping) => { setGrouping(g); api.put('/settings', { libraryGrouping: g }).catch(() => {}) }
  // Adding a library starts its scan at once, so whether anything is written into the media folders is asked first, and
  // saved as it's answered.
  const [beside, setBeside] = useState(true)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setBeside(s.saveMetadata !== '0')).catch(() => {}) }, [])
  const changeBeside = (v: boolean) => { setBeside(v); api.put('/settings', { saveMetadata: v ? '1' : '0' }).catch(() => setBeside(!v)) }
  return (
    <section>
      <StepHead title={t('Where is your media?')} lead={t('Pick a kind of media, then the folder it’s in. Add as many as you like. Each one becomes a library you can browse. Your files stay where they are.')} />
      {!inPanel && (
        <div className="su-gap">
          <TickRow checked={beside} onChange={changeBeside} title={t('Save posters and details beside your files')}
            text={t('They stay with your media if you ever move it. Your videos and music themselves are never changed. Untick this to leave your media folders exactly as they are, and keep it all on this server instead.')} />
        </div>
      )}
      <LibrarySummary libs={added} title={t('Libraries you’ve made')} />
      {!inPanel && added.length > 0 && <GroupingChoice value={grouping} onChange={changeGrouping} />}
      <AddLibrary onAdded={onAdded} onPanel={setInPanel} existing={added} grouping={grouping} setup onChanged={refresh} />
      {!inPanel && (
        <>
          <LaterNote info={info} />
          <StepActions back={back} note={added.length ? t('Scanning has started in the background. Add another, or continue.') : undefined}>
            <Button variant={added.length ? 'primary' : 'default'} size="big" onClick={next}>{added.length ? t('Continue') : t('Skip for now')}</Button>
          </StepActions>
        </>
      )}
    </section>
  )
}
