// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { HelpLink } from '../../components/HelpLink'
import { Link } from 'react-router-dom'
import { LoadError } from '../../components/common'
import { PageHead, Row, SaveBar, Section, useServerSettings } from './ui'
import { Segmented } from '../../ui'
import { deviceStreaming, setDeviceStreaming, type DeviceStreaming } from '../../player/streaming'
import AutoSkipSection, { ContentFiltersSection } from './AutoSkip'
import { QualityLimitsSection } from './StreamQuality'
import { t, tk } from '../../i18n'

// How things play: the player, TV episode lists, skipping intros and streaming. DVDs and Blu-rays
// are WittyDisc's own section of Settings.

export default function PlaybackSettings() {
  const st = useServerSettings()
  const { s, set } = st
  if (!s) return st.failed ? <LoadError what={t('This page')} onRetry={st.retry} /> : null
  return (
    <>
      <PageHead title={tk('Playback')} desc={<>{t('How videos and TV shows play for everyone on this server. Quality, hardware and formats are under')} <Link to="/settings/transcoding">{t('Live transcoding')}</Link>.</>} />

      <Section title={tk('Player')}>
        <Row label={t('Fastest playback speed')} desc={t('Viewers pick a speed in every player: video, discs, music, audiobooks, podcasts and vertical videos. Voices keep their pitch at every speed. Above 4×: most browsers mute the sound, and a video that has to be converted must convert faster than it plays, which takes strong hardware. Files that play as they are, in H.264 or VP9 with AAC or Opus, cope best.')}>
          <select className="select" aria-label={t('Fastest playback speed')} value={s.maxSpeed || '4'} onChange={(e) => set('maxSpeed', e.target.value)}>
            <option value="2">2×</option><option value="4">{t('4× (recommended)')}</option><option value="6">6×</option><option value="8">8×</option><option value="16">{t('16× (browser limit)')}</option>
          </select>
        </Row>
        <Row label={t('TV episode layout')} desc={t('The default for every show; each show can change it on its own page.')}>
          <select className="select" value={s.episodeView || 'cards'} onChange={(e) => set('episodeView', e.target.value)}>
            <option value="cards">{t('Cards with pictures')}</option><option value="list">{t('Compact list')}</option>
          </select>
        </Row>
      </Section>

      <AutoSkipSection />
      <ContentFiltersSection />

      <StreamingSection value={s.remoteStreaming || 'standard'} set={(v) => set('remoteStreaming', v)} />

      <QualityLimitsSection />

      <SaveBar {...st} />
    </>
  )
}

// Streaming: standard, or robust for poor connections. The server's choice is for viewers
// away from home; each device can choose for itself, which wins wherever it is.
function StreamingSection({ value, set }: { value: string; set: (v: string) => void }) {
  const [device, setDevice] = useState<DeviceStreaming>(deviceStreaming)
  return (
    <Section title={tk('Streaming')} desc={<>{t('How videos travel to viewers on a slow or unreliable connection.')} <HelpLink to="playing#robust-streaming">{t('How robust streaming works')}</HelpLink></>}>
      <Row label={t('For viewers away from home')} desc={t('Standard sends the best picture the settings allow, as it always has. Robust favours staying smooth: it measures the viewer’s connection, starts at a steady rate that fits, steps down to a lower quality instead of stalling, keeps a bigger buffer and picks up where it was after the connection drops. It costs the server a little more, since converting is more often needed.')}>
        <Segmented label={t('Streaming for viewers away from home')} value={value === 'robust' ? 'robust' : 'standard'} onChange={set} options={[{ value: 'standard', label: t('Standard') }, { value: 'robust', label: t('Robust') }]} />
      </Row>
      <Row label={t('On this device')} desc={t('This browser’s own choice, which wins over the server’s, at home or away: robust helps on poor Wi-Fi at home too. It’s saved on this device straight away, and other devices aren’t changed.')}>
        <Segmented label={t('Streaming on this device')} value={device} onChange={(v) => { setDeviceStreaming(v); setDevice(v) }}
          options={[{ value: 'auto', label: t('Follow the server') }, { value: 'standard', label: t('Standard') }, { value: 'robust', label: t('Robust') }]} />
      </Row>
    </Section>
  )
}
