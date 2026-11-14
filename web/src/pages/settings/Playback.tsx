// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { HelpLink } from '../../components/HelpLink'
import { Link } from 'react-router-dom'
import { LoadError } from '../../components/common'
import { PageHead, Row, SaveBar, Section, useServerSettings } from './ui'
import { Segmented } from '../../ui'
import { deviceStreaming, setDeviceStreaming, type DeviceStreaming } from '../../player/streaming'
import AutoSkipSection from './AutoSkip'

// How things play: the player, TV episode lists, skipping intros and streaming. DVDs and Blu-rays
// are WittyDisc's own section of Settings.

export default function PlaybackSettings() {
  const st = useServerSettings()
  const { s, set } = st
  if (!s) return st.failed ? <LoadError what="This page" onRetry={st.retry} /> : null
  return (
    <>
      <PageHead title="Playback" desc={<>How videos and TV shows play for everyone on this server. Quality, hardware and formats are under <Link to="/settings/transcoding">Live transcoding</Link>.</>} />

      <Section title="Player">
        <Row label="Fastest playback speed" desc="Viewers pick a speed in every player: video, discs, music, audiobooks, podcasts and vertical videos. Voices keep their pitch at every speed. Above 4×: most browsers mute the sound, and a video that has to be converted must convert faster than it plays, which takes strong hardware. Files that play as they are, in H.264 or VP9 with AAC or Opus, cope best.">
          <select className="select" aria-label="Fastest playback speed" value={s.maxSpeed || '4'} onChange={(e) => set('maxSpeed', e.target.value)}>
            <option value="2">2×</option><option value="4">4× (recommended)</option><option value="6">6×</option><option value="8">8×</option><option value="16">16× (browser limit)</option>
          </select>
        </Row>
        <Row label="TV episode layout" desc="The default for every show; each show can change it on its own page.">
          <select className="select" value={s.episodeView || 'cards'} onChange={(e) => set('episodeView', e.target.value)}>
            <option value="cards">Cards with pictures</option><option value="list">Compact list</option>
          </select>
        </Row>
      </Section>

      <AutoSkipSection />

      <StreamingSection value={s.remoteStreaming || 'standard'} set={(v) => set('remoteStreaming', v)} />

      <SaveBar {...st} />
    </>
  )
}

// Streaming: standard, or robust for poor connections. The server's choice is for viewers
// away from home; each device can choose for itself, which wins wherever it is.
function StreamingSection({ value, set }: { value: string; set: (v: string) => void }) {
  const [device, setDevice] = useState<DeviceStreaming>(deviceStreaming)
  return (
    <Section title="Streaming" desc={<>How videos travel to viewers on a slow or unreliable connection. <HelpLink to="playing#robust-streaming">How robust streaming works</HelpLink></>}>
      <Row label="For viewers away from home" desc="Standard sends the best picture the settings allow, as it always has. Robust favours staying smooth: it measures the viewer’s connection, starts at a steady rate that fits, steps down to a lower quality instead of stalling, keeps a bigger buffer and picks up where it was after the connection drops. It costs the server a little more, since converting is more often needed.">
        <Segmented label="Streaming for viewers away from home" value={value === 'robust' ? 'robust' : 'standard'} onChange={set} options={[{ value: 'standard', label: 'Standard' }, { value: 'robust', label: 'Robust' }]} />
      </Row>
      <Row label="On this device" desc="This browser’s own choice, which wins over the server’s, at home or away: robust helps on poor Wi-Fi at home too. It’s saved on this device straight away, and other devices aren’t changed.">
        <Segmented label="Streaming on this device" value={device} onChange={(v) => { setDeviceStreaming(v); setDevice(v) }}
          options={[{ value: 'auto', label: 'Follow the server' }, { value: 'standard', label: 'Standard' }, { value: 'robust', label: 'Robust' }]} />
      </Row>
    </Section>
  )
}
