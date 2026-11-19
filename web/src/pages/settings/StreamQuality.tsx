// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, profileName, type QualityLimits, type QualityPreset, type StreamingPrefs, type UserView } from '../../api'
import { LoadError } from '../../components/common'
import { toast, useApp } from '../../state'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Overlay'
import { Field, Input, Select } from '../../ui/Field'
import { describe, deviceDefaults, entryOf, presetChoice, rate, setDeviceDefault, sizeName } from '../../player/quality'
import { PageHead, Row, Section } from './ui'

// Video quality: the size and data rate videos play at, as the player's quality menu sets them
// (docs/CLIENT-API.md, Quality). This profile keeps a default for home and for away from home,
// which every device and app follows; a device may keep its own, which wins on that device.

const ALL: QualityPreset[] = [
  { name: '4K', height: 2160, kbps: 20000 }, { name: '1080p', height: 1080, kbps: 8000 }, { name: '720p', height: 720, kbps: 4000 },
  { name: '480p', height: 480, kbps: 1500 }, { name: '360p', height: 360, kbps: 800 },
]

function ChoiceSelect({ label, value, onChange, presets, autoLabel }: { label: string; value: string; onChange: (v: string) => void; presets: QualityPreset[]; autoLabel: string }) {
  const e = entryOf(value, presets)
  return (
    <select className="select" aria-label={label} value={e} onChange={(ev) => {
      const v = ev.target.value
      onChange(v === 'auto' ? '' : v === 'original' ? 'original' : v === 'custom' ? value : presetChoice(presets.find((p) => p.name === v)!))
    }}>
      <option value="auto">{autoLabel}</option>
      <option value="original">Original (no limit of my own)</option>
      {presets.map((p) => <option key={p.name} value={p.name}>{p.name}, about {rate(p.kbps)}</option>)}
      {e === 'custom' && <option value="custom">{describe(value, presets)} (set in the player)</option>}
    </select>
  )
}

const limitWords = (l: QualityLimits) => [l.maxHeight ? `up to ${sizeName(l.maxHeight)}` : '', l.maxKbps ? `${rate(l.maxKbps)} at most` : ''].filter(Boolean).join(', ')

export default function StreamQualitySettings() {
  const { user } = useApp()
  const [data, setData] = useState<StreamingPrefs | null>(null)
  const [failed, setFailed] = useState(false)
  const [device, setDevice] = useState(deviceDefaults)
  const load = () => { setFailed(false); api.get<StreamingPrefs>('/me/streaming').then(setData).catch(() => setFailed(true)) }
  useEffect(load, [])
  if (!data) return failed ? <LoadError what="This page" onRetry={load} /> : null
  const cap = data.profile
  const presets = ALL.filter((p) => !cap.maxHeight || p.height <= cap.maxHeight)
  const save = (p: { home: string; away: string }) => {
    setData({ ...data, prefs: p })
    api.put('/me/streaming', p).then(() => toast('Saved')).catch((e) => { toast(e.message); load() })
  }
  const setDev = (where: 'home' | 'away', v: string) => { setDeviceDefault(where, v); setDevice(deviceDefaults()) }
  return (
    <>
      <PageHead title="Video quality" desc="The picture size and amount of data videos play at: films, episodes, discs, live TV and WittyTV channels. The player’s Quality button changes it while you watch." />
      <Section title="Your profile" desc="Every device and app you sign in on follows these, unless the device has its own below.">
        <Row label="At home" desc="On the same network as the server. Auto plays the file as it is wherever it can.">
          <ChoiceSelect label="Video quality at home" value={data.prefs.home} presets={presets} autoLabel="Auto (recommended)" onChange={(v) => save({ ...data.prefs, home: v })} />
        </Row>
        <Row label="Away from home" desc="Over the internet: a lower setting saves mobile data and keeps playing smooth on a slow connection.">
          <ChoiceSelect label="Video quality away from home" value={data.prefs.away} presets={presets} autoLabel="Auto (recommended)" onChange={(v) => save({ ...data.prefs, away: v })} />
        </Row>
      </Section>
      <Section title="This device" desc="Saved in this browser only. Choosing in the player’s Quality menu changes these.">
        <Row label="At home">
          <ChoiceSelect label="Video quality on this device at home" value={device.home} presets={presets} autoLabel="Follow my profile" onChange={(v) => setDev('home', v)} />
        </Row>
        <Row label="Away from home">
          <ChoiceSelect label="Video quality on this device away from home" value={device.away} presets={presets} autoLabel="Follow my profile" onChange={(v) => setDev('away', v)} />
        </Row>
      </Section>
      <Section title="Limits" desc="What the server allows. Your choices never go above these.">
        <Row label="Your profile">{cap.maxHeight || cap.maxKbps ? limitWords(cap) : 'No limit'}</Row>
        <Row label="Away from home, for everyone" desc={user?.isAdmin ? <>Set in <Link to="/settings/remote">Remote Access</Link>.</> : undefined}>{data.remote.maxHeight || data.remote.maxKbps ? limitWords(data.remote) : 'No limit'}</Row>
        {user?.isAdmin && <Row label="Each profile" desc={<>An administrator sets them in <Link to="/settings/users">Profiles</Link>, with each profile’s Streaming button.</>} />}
      </Section>
    </>
  )
}

// StreamLimitsDialog: what an administrator allows one profile.
export function StreamLimitsDialog({ user, onClose }: { user: UserView; onClose: () => void }) {
  const [l, setL] = useState<QualityLimits | null>(null)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<QualityLimits>(`/users/${user.id}/stream-limits`).then(setL).catch((e) => { toast(e.message); onClose() }) }, [user.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async () => {
    if (!l) return
    setBusy(true)
    try { await api.put(`/users/${user.id}/stream-limits`, l); toast('Streaming limits saved'); onClose() } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  const name = profileName(user)
  return (
    <Dialog title={`Streaming for ${name}`} onClose={onClose} actions={<><Button variant="ghost" onClick={onClose}>Cancel</Button><Button variant="primary" busy={busy} disabled={!l} onClick={save}>Save</Button></>}>
      {l && (
        <div style={{ display: 'grid', gap: 'var(--sp-3)' }}>
          <p className="muted">The most {name}’s streams are made to, on every device, at home and away. Their own choices in the player stay within these. Files already within them still play as they are.</p>
          <Field label="Largest picture">
            <Select value={l.maxHeight} onChange={(e) => setL({ ...l, maxHeight: Number(e.target.value) })}>
              <option value={0}>No limit</option>
              {[2160, 1440, 1080, 720, 576, 480, 360].map((h) => <option key={h} value={h}>{h}p{h === 2160 ? ' (4K)' : ''}</option>)}
            </Select>
          </Field>
          <Field label="Most data, in Mbps" hint="0 for no limit. Picture and sound together.">
            <Input type="number" min={0} max={400} step={0.5} inputMode="decimal" value={l.maxKbps ? l.maxKbps / 1000 : 0}
              onChange={(e) => { const v = Number(e.target.value); setL({ ...l, maxKbps: Number.isFinite(v) && v > 0 ? Math.round(Math.max(0.2, v) * 1000) : 0 }) }} />
          </Field>
        </div>
      )}
    </Dialog>
  )
}

// For the admin's Playback page: where the limits are set.
export function QualityLimitsSection() {
  return (
    <Section title="Video quality" desc="Viewers choose a size and data rate in the player’s Quality menu, and keep defaults for home and away in Settings → Video quality.">
      <Row label="Limits for viewers away from home" desc={<>The largest picture and the most data any stream over the internet uses: <Link to="/settings/remote">Remote Access</Link>.</>} />
      <Row label="Limits for each profile" desc={<>The same for one profile, everywhere: the Streaming button beside it in <Link to="/settings/users">Profiles</Link>.</>} />
      <Row label="How conversions are made" desc={<>The encoder, hardware and quality: <Link to="/settings/transcoding">Live transcoding</Link>. A viewer’s choice caps the size and rate; it never turns hardware encoding on or off.</>} />
    </Section>
  )
}
