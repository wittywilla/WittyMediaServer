// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, type Library } from '../../api'
import AddLibrary, { GroupingChoice, LibrarySummary, type Grouping } from '../../components/AddLibrary'
import { Button } from '../../ui'
import { Customize, Option, StepActions, StepHead, TickRow, type StepProps } from './parts'
import { Modules } from './Modules'
import { t } from '../../i18n'

// Step 3: libraries. Pick a kind, then its folder; a kind that needs a module offers it right there. Add-ons (Live TV and
// the rest), whether details are saved beside the files, and how a second folder of a kind is kept are under Customize.
export function Media({ facts, next, back, added, onAdded, refresh }: StepProps & { added: Library[]; onAdded: (l: Library) => void }) {
  // While a kind is chosen (its folder, then following podcasts) the panel has its own Back and
  // Add buttons, and Continue would be easy to press by mistake, so it waits until that's closed.
  const [inPanel, setInPanel] = useState(false)
  const [grouping, setGrouping] = useState<Grouping>('combine')
  // Adding a library starts its scan at once, so whether anything is written into the media folders is saved as it's
  // answered, before the first folder.
  const [beside, setBeside] = useState(true)
  useEffect(() => {
    api.get<Record<string, string>>('/settings').then((s) => {
      setGrouping(s.libraryGrouping === 'separate' ? 'separate' : 'combine')
      setBeside(s.saveMetadata !== '0')
    }).catch(() => {})
  }, [])
  const changeGrouping = (g: Grouping) => { setGrouping(g); api.put('/settings', { libraryGrouping: g }).catch(() => {}) }
  const changeBeside = (v: boolean) => { setBeside(v); api.put('/settings', { saveMetadata: v ? '1' : '0' }).catch(() => setBeside(!v)) }
  return (
    <section>
      <StepHead title={t('Add your media')} lead={t('Pick a kind of media, then the folder it’s in. Your files stay where they are.')} />
      <LibrarySummary libs={added} title={t('Libraries you’ve made')} />
      <AddLibrary onAdded={onAdded} onPanel={setInPanel} existing={added} grouping={grouping} setup onChanged={refresh} />
      {!inPanel && (
        <StepActions back={back} note={added.length ? t('Scanning has started in the background.') : undefined}>
          <Customize title={t('Media options')}>
            {facts.store && <Option title={t('Add-ons')} text={t('Live TV, light shows, server charts and more')}><Modules refresh={refresh} /></Option>}
            <Option title={t('Posters and details')} text={beside ? t('Saved beside your files') : t('Kept on this server only')}>
              <TickRow checked={beside} onChange={changeBeside} title={t('Save posters and details beside your files')}
                text={t('They move with your media. The videos and music themselves are never changed.')} />
            </Option>
            <Option title={t('More than one folder of a kind')} text={grouping === 'separate' ? t('Separate libraries') : t('One library')}>
              <GroupingChoice value={grouping} onChange={changeGrouping} />
            </Option>
          </Customize>
          <Button variant="primary" size="big" onClick={next}>{added.length ? t('Continue') : t('Skip for now')}</Button>
        </StepActions>
      )}
    </section>
  )
}
