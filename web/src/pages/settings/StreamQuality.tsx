// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Fragment, useEffect, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { api, profileName, type QualityLimits, type QualityPreset, type StreamingPrefs, type UserView } from '../../api'
import { LoadError } from '../../components/common'
import { toast, useApp } from '../../state'
import { Button } from '../../ui/Button'
import { Dialog } from '../../ui/Overlay'
import { Field, Input, Select } from '../../ui/Field'
import { describe, deviceDefaults, entryOf, presetChoice, rate, setDeviceDefault, sizeName } from '../../player/quality'
import { PageHead, Row, Section } from './ui'
import { t } from '../../i18n'

// Video quality: the size and data rate videos play at, as the player's quality menu sets them
// (docs/CLIENT-API.md, Quality). This profile keeps a default for home and for away from home,
// which every device and app follows; a device may keep its own, which wins on that device.

// A translated sentence with links in it: each {name} in the text is replaced by links[name].
const linked = (text: string, links: Record<string, ReactNode>) => <>{text.split(/(\{\w+\})/).map((part, i) => {
  const m = /^\{(\w+)\}$/.exec(part)
  return m && links[m[1]] !== undefined ? <Fragment key={i}>{links[m[1]]}</Fragment> : part
})}</>

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
      <option value="original">{t('Original (no limit of my own)')}</option>
      {presets.map((p) => <option key={p.name} value={p.name}>{t('{name}, about {rate}', { name: p.name, rate: rate(p.kbps) })}</option>)}
      {e === 'custom' && <option value="custom">{t('{quality} (set in the player)', { quality: describe(value, presets) })}</option>}
    </select>
  )
}

const limitWords = (l: QualityLimits) => l.maxHeight && l.maxKbps ? t('up to {size}, {rate} at most', { size: sizeName(l.maxHeight), rate: rate(l.maxKbps) })
  : l.maxHeight ? t('up to {size}', { size: sizeName(l.maxHeight) }) : l.maxKbps ? t('{rate} at most', { rate: rate(l.maxKbps) }) : ''

export default function StreamQualitySettings() {
  const { user } = useApp()
  const [data, setData] = useState<StreamingPrefs | null>(null)
  const [failed, setFailed] = useState(false)
  const [device, setDevice] = useState(deviceDefaults)
  const load = () => { setFailed(false); api.get<StreamingPrefs>('/me/streaming').then(setData).catch(() => setFailed(true)) }
  useEffect(load, [])
  if (!data) return failed ? <LoadError what={t('This page')} onRetry={load} /> : null
  const cap = data.profile
  const presets = ALL.filter((p) => !cap.maxHeight || p.height <= cap.maxHeight)
  const save = (p: { home: string; away: string }) => {
    setData({ ...data, prefs: p })
    api.put('/me/streaming', p).then(() => toast(t('Saved'))).catch((e) => { toast(e.message); load() })
  }
  const setDev = (where: 'home' | 'away', v: string) => { setDeviceDefault(where, v); setDevice(deviceDefaults()) }
  return (
    <>
      <PageHead title={t('Video quality')} desc={t('The picture size and amount of data videos play at: films, episodes, discs, live TV and WittyTV channels. The player’s Quality button changes it while you watch.')} />
      <Section title={t('Your profile')} desc={t('Every device and app you sign in on follows these, unless the device has its own below.')}>
        <Row label={t('At home')} desc={t('On the same network as the server. Auto plays the file as it is wherever it can.')}>
          <ChoiceSelect label={t('Video quality at home')} value={data.prefs.home} presets={presets} autoLabel={t('Auto (recommended)')} onChange={(v) => save({ ...data.prefs, home: v })} />
        </Row>
        <Row label={t('Away from home')} desc={t('Over the internet: a lower setting saves mobile data and keeps playing smooth on a slow connection.')}>
          <ChoiceSelect label={t('Video quality away from home')} value={data.prefs.away} presets={presets} autoLabel={t('Auto (recommended)')} onChange={(v) => save({ ...data.prefs, away: v })} />
        </Row>
      </Section>
      <Section title={t('This device')} desc={t('Saved in this browser only. Choosing in the player’s Quality menu changes these.')}>
        <Row label={t('At home')}>
          <ChoiceSelect label={t('Video quality on this device at home')} value={device.home} presets={presets} autoLabel={t('Follow my profile')} onChange={(v) => setDev('home', v)} />
        </Row>
        <Row label={t('Away from home')}>
          <ChoiceSelect label={t('Video quality on this device away from home')} value={device.away} presets={presets} autoLabel={t('Follow my profile')} onChange={(v) => setDev('away', v)} />
        </Row>
      </Section>
      <Section title={t('Limits')} desc={t('What the server allows. Your choices never go above these.')}>
        <Row label={t('Your profile')}>{cap.maxHeight || cap.maxKbps ? limitWords(cap) : t('No limit')}</Row>
        <Row label={t('Away from home, for everyone')} desc={user?.isAdmin ? linked(t('Set in {page}.'), { page: <Link to="/settings/remote">{t('Remote Access')}</Link> }) : undefined}>{data.remote.maxHeight || data.remote.maxKbps ? limitWords(data.remote) : t('No limit')}</Row>
        {user?.isAdmin && <Row label={t('Each profile')} desc={linked(t('An administrator sets them in {page}, with each profile’s Streaming button.'), { page: <Link to="/settings/users">{t('Profiles')}</Link> })} />}
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
    try { await api.put(`/users/${user.id}/stream-limits`, l); toast(t('Streaming limits saved')); onClose() } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  const name = profileName(user)
  return (
    <Dialog title={t('Streaming for {name}', { name })} onClose={onClose} actions={<><Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button><Button variant="primary" busy={busy} disabled={!l} onClick={save}>{t('Save')}</Button></>}>
      {l && (
        <div style={{ display: 'grid', gap: 'var(--sp-3)' }}>
          <p className="muted">{t('The most {name}’s streams are made to, on every device, at home and away. Their own choices in the player stay within these. Files already within them still play as they are.', { name })}</p>
          <Field label={t('Largest picture')}>
            <Select value={l.maxHeight} onChange={(e) => setL({ ...l, maxHeight: Number(e.target.value) })}>
              <option value={0}>{t('No limit')}</option>
              {[2160, 1440, 1080, 720, 576, 480, 360].map((h) => <option key={h} value={h}>{h}p{h === 2160 ? ' (4K)' : ''}</option>)}
            </Select>
          </Field>
          <Field label={t('Most data, in Mbps')} hint={t('0 for no limit. Picture and sound together.')}>
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
    <Section title={t('Video quality')} desc={t('Viewers choose a size and data rate in the player’s Quality menu, and keep defaults for home and away in Settings → Video quality.')}>
      <Row label={t('Limits for viewers away from home')} desc={linked(t('The largest picture and the most data any stream over the internet uses: {page}.'), { page: <Link to="/settings/remote">{t('Remote Access')}</Link> })} />
      <Row label={t('Limits for each profile')} desc={linked(t('The same for one profile, everywhere: the Streaming button beside it in {page}.'), { page: <Link to="/settings/users">{t('Profiles')}</Link> })} />
      <Row label={t('How conversions are made')} desc={linked(t('The encoder, hardware and quality: {page}. A viewer’s choice caps the size and rate; it never turns hardware encoding on or off.'), { page: <Link to="/settings/transcoding">{t('Live transcoding')}</Link> })} />
    </Section>
  )
}
