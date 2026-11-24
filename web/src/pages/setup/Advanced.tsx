// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense } from 'react'
import { MusicOnlineStep, SeparatorStep } from '../MusicOnline'
import { Spinner } from '../../ui'
import { t } from '../../i18n'
import { PanelStep, type StepProps } from './parts'
import { Details } from './Details'
import { HomeNetwork } from './HomeNetwork'
import { Listings } from './Listings'
import { LanguageField, LocaleMore, useLocaleChoice } from './Locale'
import { Logos } from './Logos'

// The pages only advanced setup has, each one question with what it is in a sentence. Quick setup has the same panels
// under its steps' Customize buttons (Welcome, Ready), already answered.

const ConnectDevice = lazy(() => import('../../components/ConnectDevice'))

export function LocalePage({ next, back }: Pick<StepProps, 'next' | 'back'>) {
  const c = useLocaleChoice()
  return (
    <PanelStep title={t('Language and time zone')} lead={t('Disc menus and sound tracks follow the language unless you choose another.')} back={back} next={() => { c.confirm(); next() }}>
      <div className="su-net"><LanguageField c={c} /></div>
      <LocaleMore c={c} />
    </PanelStep>
  )
}

export const DetailsPage = ({ facts, next, back }: StepProps) => (
  <PanelStep title={t('Posters and details')} lead={t('Where posters, plots and ratings come from.')} back={back} next={next}><Details facts={facts} /></PanelStep>
)

export const LogosPage = ({ next, back }: StepProps) => (
  <PanelStep title={t('Logos')} lead={t('Real logos for consoles, studios and TV networks, instead of plain names.')} back={back} next={next}><Logos /></PanelStep>
)

export const MusicPage = ({ next, back }: StepProps) => (
  <PanelStep title={t('Lyrics and artists')} lead={t('Lyrics beside your songs work already. These find the rest online.')} back={back} next={next}><MusicOnlineStep /></PanelStep>
)

export const KaraokePage = ({ next, back }: StepProps) => (
  <PanelStep title={t('Karaoke voices')} lead={t('Karaoke works already. This takes the singer out more cleanly.')} back={back} next={next}><SeparatorStep /></PanelStep>
)

export const ListingsPage = ({ next, back }: StepProps) => (
  <PanelStep title={t('Streaming lists')} lead={t('Sort your own titles by which streaming service lists them.')} back={back} next={next}><Listings /></PanelStep>
)

export const NetworkPage = ({ next, back }: StepProps) => (
  <PanelStep title={t('Home network')} lead={t('How phones, TVs and computers at home find this server.')} back={back} next={next}><HomeNetwork /></PanelStep>
)

// The same screen as Settings, Remote Access: one short code and a QR code for each device, details further in.
export const RemotePage = ({ next, back }: StepProps) => (
  <PanelStep title={t('Away from home')} lead={t('Connect a phone or laptop, and choose whether it works away from home.')} back={back} next={next}>
    <div className="su-connect"><Suspense fallback={<Spinner />}><ConnectDevice /></Suspense></div>
  </PanelStep>
)
