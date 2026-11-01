import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { LoadError } from '../../components/common'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './ui'

// How things play: the player, TV episode lists, and DVDs and Blu-rays.

export default function PlaybackSettings() {
  const st = useServerSettings()
  const { s, set, on } = st
  if (!s) return st.failed ? <LoadError what="This page" onRetry={st.retry} /> : null
  const fx = on('insertFx')
  return (
    <>
      <PageHead title="Playback & discs" desc={<>How videos, TV shows and discs play for everyone on this server. Quality, hardware and formats are under <Link to="/settings/transcoding">Transcoding</Link>.</>} />

      <Section title="Player">
        <Row label="Fastest playback speed" desc="Viewers pick a speed in every player: video, discs, music, audiobooks, podcasts and vertical videos. Voices keep their pitch at every speed. Above 4×: most browsers mute the sound, and a video that has to be converted must convert faster than it plays, which takes strong hardware. Files that play as they are, in H.264 or VP9 with AAC or Opus, cope best.">
          <select className="select" value={s.maxSpeed || '4'} onChange={(e) => set('maxSpeed', e.target.value)}>
            <option value="2">2×</option><option value="4">4× (recommended)</option><option value="6">6×</option><option value="8">8×</option><option value="16">16× (browser limit)</option>
          </select>
        </Row>
        <Row label="TV episode layout" desc="The default for every show; each show can change it on its own page.">
          <select className="select" value={s.episodeView || 'cards'} onChange={(e) => set('episodeView', e.target.value)}>
            <option value="cards">Cards with pictures</option><option value="list">Compact list</option>
          </select>
        </Row>
      </Section>

      <Section title="DVDs and Blu-rays" desc="Discs play with their real menus, like a disc player. Each disc can change these in its own Disc settings.">
        <Row label="When a disc is inserted">
          <select className="select" value={s.dvdStart || 'menu'} onChange={(e) => set('dvdStart', e.target.value)}>
            <option value="menu">Start with the disc menu</option><option value="title">Skip menus, play the movie</option>
          </select>
        </Row>
        <Row label="Disc insert animation" desc="A tray slides in when a DVD or Blu-ray starts.">
          <Switch label="Disc insert animation" checked={fx} onChange={(v) => set('insertFx', v ? '1' : '0')} />
        </Row>
        <Row label="Drive sounds" desc="The tray motor and the disc spinning up, with the animation." dim={!fx}>
          <Switch label="Drive sounds" checked={on('insertSound')} disabled={!fx} onChange={(v) => set('insertSound', v ? '1' : '0')} />
        </Row>
        <Row label="Preferred languages" desc="Two-letter codes such as en, nl or fr. Discs that offer them start in these languages." stack>
          <div className="set-trio">
            <label>Menus<input className="input" value={s.menuLanguage || ''} onChange={(e) => set('menuLanguage', e.target.value)} placeholder="en" maxLength={3} /></label>
            <label>Audio<input className="input" value={s.audioLanguage || ''} onChange={(e) => set('audioLanguage', e.target.value)} placeholder="en" maxLength={3} /></label>
            <label>Subtitles<input className="input" value={s.subtitleLanguage || ''} onChange={(e) => set('subtitleLanguage', e.target.value)} placeholder="none" maxLength={3} /></label>
          </div>
        </Row>
        <Row label="Low-power mode for DVDs" desc="Easier on small computers such as a Raspberry Pi: DVD menus and titles are converted faster, at a small cost in quality.">
          <Switch label="Low-power mode for DVDs" checked={s.lowPower === '1'} onChange={(v) => set('lowPower', v ? '1' : '0')} />
        </Row>
        <BDJRows s={s} set={set} dirty={st.dirty} />
        <Row label="DVD region" desc="1 to 8. Empty plays each disc as its own region.">
          <input className="input" inputMode="numeric" aria-label="DVD region" value={s.dvdRegion || ''} onChange={(e) => set('dvdRegion', e.target.value)} />
        </Row>
        <Row label="Blu-ray region" desc="A disc for another region shows its own message instead of the movie, so pick the region your discs are from.">
          <select className="select" aria-label="Blu-ray region" value={s.blurayRegion || '1'} onChange={(e) => set('blurayRegion', e.target.value)}>
            <option value="1">A: Americas, East and South-East Asia</option>
            <option value="2">B: Europe, Africa, Oceania, Middle East</option>
            <option value="4">C: Central and South Asia, Russia, China</option>
          </select>
        </Row>
        <Row label="Parental limits" desc="Discs with parental versions use these to choose which version plays. Empty means no limit. These are the disc’s own checks, not a lock on the server: use profiles to limit what people can open." stack>
          <div className="set-trio">
            <label>DVD level, 1 to 8<input className="input" inputMode="numeric" value={s.dvdParental || ''} onChange={(e) => set('dvdParental', e.target.value)} /></label>
            <label>Blu-ray viewer’s age<input className="input" inputMode="numeric" value={s.blurayAge || ''} onChange={(e) => set('blurayAge', e.target.value)} /></label>
            <label>Country, two letters<input className="input" maxLength={2} value={s.discCountry || ''} onChange={(e) => set('discCountry', e.target.value)} placeholder="US" /></label>
          </div>
        </Row>
      </Section>
      <SaveBar {...st} />
    </>
  )
}

// BD-J: Blu-ray menus written in Java. Java is the server owner's to install,
// and turning this on runs a disc's code, in a sandbox, so it starts off.
function BDJRows({ s, set, dirty }: { s: Record<string, string>; set: (k: string, v: string) => void; dirty: boolean }) {
  const [status, setStatus] = useState<{ version?: string; path?: string; error?: string } | null>(null)
  useEffect(() => { if (!dirty) api.get<{ version?: string; path?: string; error?: string }>('/bdj/status').then(setStatus).catch(() => setStatus(null)) }, [dirty])
  return (
    <>
      <Row label="Run Blu-ray Java menus (BD-J)" desc={<>Many commercial Blu-rays write their menus in Java. To show them, install any Java 8 or newer on this server, for example Eclipse Temurin, a free OpenJDK build. It isn’t included. A disc’s Java code runs in its own process with no network and no access to your files outside the disc, but it’s still code from the disc, so this is off until you turn it on. With it off, these discs play their main feature directly.{status && <><br />{status.version ? `Java ${status.version} found: ${status.path}` : `Java not usable: ${status.error}`}</>}</>}>
        <Switch label="Run Blu-ray Java menus" checked={s.bdjEnabled === '1'} onChange={(v) => set('bdjEnabled', v ? '1' : '0')} />
      </Row>
      <Row label="Java location" desc="Optional. Leave it empty to find Java by itself. Otherwise the Java folder, or the java program inside it." stack>
        <input className="input mono" aria-label="Java location" value={s.javaPath || ''} onChange={(e) => set('javaPath', e.target.value)} placeholder="C:\Program Files\Eclipse Adoptium\jdk-21" />
      </Row>
    </>
  )
}
