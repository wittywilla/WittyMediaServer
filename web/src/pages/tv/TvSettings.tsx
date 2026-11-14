// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { Link, Navigate, Route, Routes, useParams } from 'react-router-dom'
import { ChooseFolderButton } from '../../components/FolderPicker'
import { api } from '../../api'
import { IconEdit, IconTv } from '../../icons'
import { toast, useApp } from '../../state'
import type { Channel } from './tvApi'
import { PageHead, Row, Section, Switch } from '../settings/ui'
import { forgetOwnerLook } from './look'
import { THEMES } from './tvThemes'
import './tvsettings.css'
import LegalNote from '../../components/LegalNote'

// Settings → Channels & tuners: library channels and WittyTV's options. Tuners, IPTV and guide data
// are WittyTuner's own section (Settings → Live TV & tuners).
// Channels from the library are made and changed on their own page, /tv/channels.

export default function TvSettings() {
  return (
    <Routes>
      <Route index element={<Overview />} />
      <Route path="new" element={<MovedEditor />} />
      <Route path="edit/:id" element={<MovedEditor />} />
      <Route path="tuner/:id" element={<MovedTuner />} />
      <Route path="duplicates" element={<Navigate to="/settings/tuners/duplicates" replace />} />
      <Route path="*" element={<Navigate to="/settings/channels" replace />} />
    </Routes>
  )
}

function Overview() {
  return (
    <>
      <PageHead title="Channels & tuners" desc="Channels from your library and WittyTV's options. Tuners and IPTV, with their guide data, are under Live TV & tuners, and sit beside these in one guide."
        actions={<Link className="btn" to="/tv"><IconTv /> Open the guide</Link>} />
      <LibraryChannels />
      <TunersElsewhere />
      <Options />
      <LegalNote kind="tv" className="set-legal" />
      <LegalNote kind="logos" className="set-legal" />
    </>
  )
}

// ------------------------------------------------------------ library channels

// The channels themselves have a page of their own (ChannelsPage), so that this
// one holds only settings.
function LibraryChannels() {
  const [list, setList] = useState<Channel[] | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<Channel[]>('/channels').then(setList).catch((e) => setErr(e.message)) }, [])
  const on = list?.filter((c) => c.enabled).length || 0
  return (
    <Section title="Channels from your library" desc="Always-on channels made from your films and shows. Making and changing them, and the ones suggested from your library, are on their own page.">
      <Row label="Your channels" desc={err ? `The list couldn't be read: ${err}` : list === null ? '' : list.length === 0 ? 'None yet.' : `${list.length} ${list.length === 1 ? 'channel' : 'channels'}, ${on} on the air.`}>
        <Link className="btn sm primary" to="/tv/channels"><IconEdit /> Open the channel list</Link>
      </Row>
    </Section>
  )
}

// Tuners, their channels and guide data are WittyTuner's own section of Settings.
function TunersElsewhere() {
  return (
    <Section title="Tuners and IPTV" desc="Live TV from tuners, IPTV playlists and streams, and guide data for their channels, are WittyTuner's.">
      <Row label="Live TV & tuners" desc="Add and check tuners, choose their channels, and add guide data.">
        <Link className="btn sm primary" to="/settings/tuners"><IconTv /> Open Live TV & tuners</Link>
      </Row>
    </Section>
  )
}

// A tuner's page is WittyTuner's now.
function MovedTuner() {
  const { id } = useParams()
  return <Navigate to={`/settings/tuners/tuner/${id}`} replace />
}

// Where the editor used to be, for bookmarks and links from before.
function MovedEditor() {
  const { id } = useParams()
  return <Navigate to={id ? `/tv/channels/${id}` : '/tv/channels/new'} replace />
}

// --------------------------------------------------------------------- options

// What a new viewer starts with. Someone's own choice comes first, then what their app theme
// suggests, and then this, so that's what it says.
function OwnerTheme({ s, set }: { s: Record<string, string>; set: (k: string, v: string) => Promise<void> }) {
  const current = s.tvTheme || ''
  return (
    <Row label="WittyTV theme for new viewers" desc={<>What someone starts with until they choose. Their own choice in <Link to="/settings/appearance#wittytv">Settings, Appearance</Link> or the guide's Look panel comes first, then what their app theme suggests, and then this. Custom is each viewer's own, so it isn't offered here.</>}>
      <select className="select" value={current} onChange={(e) => { void set('tvTheme', e.target.value).then(forgetOwnerLook) }}>
        {!current && <option value="">Classic</option>}
        {THEMES.filter((t) => t.id !== 'custom').map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
      </select>
    </Row>
  )
}

function Options() {
  const { libraries } = useApp()
  const [s, setS] = useState<Record<string, string> | null>(null)
  const saved = useRef<Record<string, string>>({})
  useEffect(() => { api.get<Record<string, string>>('/tv/settings').then((v) => { setS(v); saved.current = v }).catch((e) => toast(e.message)) }, [])
  const set = async (k: string, v: string) => {
    setS((x) => ({ ...(x || {}), [k]: v }))
    if (saved.current[k] === v) return
    try { saved.current = await api.put<Record<string, string>>('/tv/settings', { [k]: v }) } catch (e: any) { toast(e.message); setS(saved.current) }
  }
  if (!s) return null
  return (
    <Section title="Options">
      <OwnerTheme s={s} set={set} />
      <Row label="Pass H.264 through" desc="Video the browser can already play is sent as it is. Much lighter on the machine; turn it off only if channels stutter when shows change.">
        <Switch checked={s.tvCopyVideo === '1'} label="Pass H.264 through" onChange={(v) => void set('tvCopyVideo', v ? '1' : '0')} />
      </Row>
      <Row label="Keep playing in the background, on new devices" desc="Whether a channel carries on when a viewer switches to another tab or app. Each device can choose for itself in WittyTV's look and in the player; this is what one starts with.">
        <Switch checked={s.tvBackgroundPlay !== '0'} label="Keep playing in the background, on new devices" onChange={(v) => void set('tvBackgroundPlay', v ? '1' : '0')} />
      </Row>
      <Row label="Pause buffer, library channels (minutes)" desc="How far back viewers can pause and rewind. Kept on disk only while someone is watching.">
        <input className="input tvs-short" type="number" min={1} max={240} defaultValue={s.tvLibraryBufferMin} onBlur={(e) => void set('tvLibraryBufferMin', e.target.value)} />
      </Row>
      <Row label="Pause buffer, tuner channels (minutes)" desc="An hour of HD can take a few gigabytes of disk.">
        <input className="input tvs-short" type="number" min={1} max={240} defaultValue={s.tvTunerBufferMin} onBlur={(e) => void set('tvTunerBufferMin', e.target.value)} />
      </Row>
      <Row label="Largest picture on channels" desc="Height in lines; 0 uses the server's general limit. Lower is lighter on small machines.">
        <input className="input tvs-short" type="number" min={0} max={4320} defaultValue={s.tvMaxHeight} onBlur={(e) => void set('tvMaxHeight', e.target.value)} />
      </Row>
      <Row label="Recordings folder" desc={s.dvrFolder || 'In the server’s data folder.'} stack>
        <div className="tvs-toolbar">
          <ChooseFolderButton className="btn sm" onPick={(p) => void set('dvrFolder', p)} label="Choose folder…" />
          {s.dvrFolder && <button className="btn sm ghost" onClick={() => void set('dvrFolder', '')}>Use the default</button>}
        </div>
      </Row>
      <Row label="Library to update after a recording" desc="To see recordings in a library, choose a recordings folder inside it, and that library here.">
        <select className="select" value={s.dvrLibrary} onChange={(e) => void set('dvrLibrary', e.target.value)}>
          <option value="">None</option>
          {libraries.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
        </select>
      </Row>
    </Section>
  )
}
