import { Link } from 'react-router-dom'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './ui'

// How things play: the player, TV episode lists, and DVDs and Blu-rays.

export default function PlaybackSettings() {
  const st = useServerSettings()
  const { s, set, on } = st
  if (!s) return null
  const fx = on('insertFx')
  return (
    <>
      <PageHead title="Playback & Discs" desc={<>How videos, TV shows and discs play for everyone on this server. Quality, hardware and formats are under <Link to="/settings/transcoding">Transcoding</Link>.</>} />

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
      </Section>
      <SaveBar {...st} />
    </>
  )
}
