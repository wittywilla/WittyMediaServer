// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, type Library } from '../../api'
import AddLibrary, { GroupingChoice, LibrarySummary, type Grouping } from '../../components/AddLibrary'
import { Button } from '../../ui'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'

export function Media({ info, next, back, added, onAdded, refresh }: StepProps & { added: Library[]; onAdded: (l: Library) => void }) {
  // While a kind is chosen (its folder, then following podcasts) the panel has its own Back and
  // Add buttons, and Continue would be easy to press by mistake, so it waits until that's closed.
  const [inPanel, setInPanel] = useState(false)
  const [grouping, setGrouping] = useState<Grouping>('combine')
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setGrouping(s.libraryGrouping === 'separate' ? 'separate' : 'combine')).catch(() => {}) }, [])
  const changeGrouping = (g: Grouping) => { setGrouping(g); api.put('/settings', { libraryGrouping: g }).catch(() => {}) }
  return (
    <section>
      <StepHead title="Where is your media?" lead="Pick a kind of media, then the folder it’s in. Add as many as you like. Each one becomes a library you can browse. Your files stay where they are." />
      <LibrarySummary libs={added} title="Libraries you’ve made" />
      {!inPanel && added.length > 0 && <GroupingChoice value={grouping} onChange={changeGrouping} />}
      <AddLibrary onAdded={onAdded} onPanel={setInPanel} existing={added} grouping={grouping} setup onChanged={refresh} />
      {!inPanel && (
        <>
          <LaterNote info={info} />
          <StepActions back={back} note={added.length ? 'Scanning has started in the background. Add another, or continue.' : undefined}>
            <Button variant={added.length ? 'primary' : 'default'} size="big" onClick={next}>{added.length ? 'Continue' : 'Skip for now'}</Button>
          </StepActions>
        </>
      )}
    </section>
  )
}
