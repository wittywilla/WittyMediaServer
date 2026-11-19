// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { refreshModuleWeb } from '../../moduleRegistry'
import { bump } from '../../sdui/client'
import { currentLook } from '../../theme'
import { toast, useApp } from '../../state'
import { Button, Dialog } from '../../ui'
import { TvLookChoice } from '../setup/Look'
import { Section } from '../settings/ui'
import { t } from '../../i18n'

// After WittyTuner is freshly installed (not updated), the server asks once how the TV guide should look for everyone
// (wittytvLookAsk, internal/server/wittytv_look.go): the same picker as first-time setup's WittyTV page. It is shown in
// Settings, Appearance (where the Home notice opens) and, as a dialog, on the Modules page right after the install.
type Card = { id: string; name: string; tvTheme?: string }

// The server says "1" only while WittyTuner is installed and the question is open. On the Modules pages it is asked again
// every few seconds, as an install finishes there in the background.
function usePending(watch: boolean): [boolean, () => void] {
  const { user } = useApp()
  const [ask, setAsk] = useState(false)
  useEffect(() => {
    if (!user?.isAdmin) return
    let gone = false
    const look = () => api.get<Record<string, string>>('/settings').then((s) => {
      if (gone || s.wittytvLookAsk !== '1') return
      setAsk(true)
      void refreshModuleWeb() // WittyTV's parts of Appearance appear with WittyTuner
    }).catch(() => {})
    void look()
    if (!watch) return () => { gone = true }
    const timer = window.setInterval(() => { if (!document.hidden) void look() }, 5000)
    return () => { gone = true; window.clearInterval(timer) }
  }, [user?.isAdmin, watch])
  return [ask, () => setAsk(false)]
}

function Picker({ onDone }: { onDone: () => void }) {
  const [card, setCard] = useState<Card | null>(null)
  const [value, setValue] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<{ experiences: Card[]; default: string }>('/sdui/experiences').then((r) => {
      const id = currentLook()?.experience || r.default
      setCard(r.experiences.find((c) => c.id === id) || r.experiences[0] || null)
    }).catch(() => {})
    api.get<Record<string, string>>('/tv/settings').then((v) => setValue(v.tvTheme || '')).catch(() => {})
  }, [])
  const answer = async (use: boolean) => {
    setBusy(true)
    try {
      if (use) await api.put('/tv/settings', { tvTheme: value })
      await api.post('/setup/module-steps/skip', { module: 'wittytuner', step: 'wittytv-look', done: use })
      bump()
      onDone()
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) } finally { setBusy(false) }
  }
  return (
    <>
      {card && <TvLookChoice suggested={card.tvTheme} lookName={card.name} value={value} onChange={setValue} />}
      <div className="rem-actions su-gap">
        <Button variant="primary" busy={busy} onClick={() => void answer(true)}>{t('Use this')}</Button>
        <Button disabled={busy} onClick={() => void answer(false)}>{t('Later')}</Button>
      </div>
    </>
  )
}

/** In Settings, Appearance: a section at the WittyTV part while the question is open. */
export function WittyTvLookSection() {
  const [ask, done] = usePending(false)
  if (!ask) return null
  return (
    <Section title={t('How should the TV guide look?')} desc={t('WittyTuner is installed. Choose how WittyTV, its guide, looks for everyone; people can still choose their own below or in the guide.')}>
      <Picker onDone={done} />
    </Section>
  )
}

/** On the Modules page: a dialog as soon as WittyTuner is ready. */
export function WittyTvLookDialog() {
  const [ask, done] = usePending(true)
  if (!ask) return null
  return (
    <Dialog title={t('WittyTuner is ready. How should the TV guide look?')} onClose={done} size="wide">
      <Picker onDone={done} />
    </Dialog>
  )
}
