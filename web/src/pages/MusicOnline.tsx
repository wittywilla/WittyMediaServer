import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { api } from '../api'
import { IconCheck } from '../icons'
import { toast } from '../state'
import { Row, Section, Switch } from './settings/ui'

// Lyrics and artist information from online. Both are off until the owner agrees, here or in
// first-time setup, and this is where it says exactly what goes where. What's beside the music
// and in its tags works anyway, with nothing sent anywhere.

type Draft = { s: Record<string, string>; set: (k: string, v: string) => void; on: (k: string, d?: string) => boolean }

// In Settings, part of the page's draft and saved with it.
export function MusicOnlineSettings({ s, set, on }: Draft) {
  const box = useRef<HTMLDivElement>(null)
  const { hash } = useLocation()
  const [cleared, setCleared] = useState(false)
  useEffect(() => { if (hash === '#lyrics') box.current?.scrollIntoView({ block: 'start' }) }, [hash])
  const useOnline = on('lyricsOnline', '0')
  return (
    <div ref={box} id="lyrics">
      <Section title="Lyrics and music information" desc="A .lrc file beside a song and the lyrics in its tags are always used first, with nothing sent anywhere. These add what they don't have.">
        <Row label="Find lyrics online" badge="No API key needed" desc={<>Timed lyrics from <a href="https://lrclib.net" target="_blank" rel="noreferrer">LRCLIB</a>, a free and open database. Only when someone opens the lyrics of a song with none of its own, your server sends LRCLIB that song's artist, title, album and length. Never a file name, or who's listening. Answers are kept in the cache on this server, aren't shared, and don't go into backups. The words belong to their writers.</>}>
          <Switch label="Find lyrics online" checked={useOnline} onChange={(v) => set('lyricsOnline', v ? '1' : '0')} />
        </Row>
        <Row label="Use LRCLIB's public service" dim={!useOnline} desc="Turn this off if you only want to use your own server or a key below.">
          <Switch label="Use LRCLIB's public service" disabled={!useOnline} checked={on('lyricsLrclib')} onChange={(v) => set('lyricsLrclib', v ? '1' : '0')} />
        </Row>
        <Row label="Your own LRCLIB server" dim={!useOnline} stack desc="Optional. LRCLIB is open source, and a copy can run on your own network, so the look-ups stay at home. It's asked first.">
          <input className="input" disabled={!useOnline} placeholder="http://192.168.1.20:3300" value={s.lyricsMirror || ''} spellCheck={false} autoComplete="off" onChange={(e) => set('lyricsMirror', e.target.value)} aria-label="Address of your LRCLIB server" />
        </Row>
        <Row label="Musixmatch key" dim={!useOnline} stack desc={<>Optional and paid. <a href="https://developer.musixmatch.com" target="_blank" rel="noreferrer">Musixmatch</a> only gives whole lyrics on its paid plans; the free plan is refused and that's what's shown. Your key is kept on this server. It's asked before LRCLIB, and is told the same four things.</>}>
          <input className="input" disabled={!useOnline} placeholder="API key" value={s.lyricsMusixmatchKey || ''} spellCheck={false} autoComplete="off" onChange={(e) => set('lyricsMusixmatchKey', e.target.value)} aria-label="Musixmatch key" />
        </Row>
        <Row label="Artist biographies and photos" badge="No API key needed" desc={<>From <a href="https://www.wikipedia.org" target="_blank" rel="noreferrer">Wikipedia</a>, credited on the artist's page. When an artist has no <code>artist.jpg</code> in their folder, your server sends Wikipedia the artist's name. The text is CC BY-SA 4.0, and each picture has its own licence, linked from its article. Nothing is written beside your music.</>}>
          <Switch label="Artist biographies and photos" checked={on('musicInfoOnline', '0')} onChange={(v) => set('musicInfoOnline', v ? '1' : '0')} />
        </Row>
        <Row label="Lyrics kept from online" desc="Forget everything fetched so far. Corrections made on this server are kept.">
          <button type="button" className="btn sm" disabled={cleared} onClick={() => { api.del('/music/lyrics-cache').then(() => { setCleared(true); toast('Lyrics from online forgotten') }).catch((e: Error) => toast(e.message)) }}>{cleared ? <><IconCheck size={16} /> Forgotten</> : 'Forget them'}</button>
        </Row>
      </Section>
    </div>
  )
}

// In first-time setup: the same two questions, in fewer words, saved as they're answered.
export function MusicOnlineStep() {
  const [lyrics, setLyrics] = useState(false)
  const [info, setInfo] = useState(false)
  const put = (key: string, on: boolean, then: (v: boolean) => void) => {
    then(on)
    api.put('/settings', { [key]: on ? '1' : '0' }).catch(() => { then(!on) })
  }
  return (
    <>
      <h2 className="oobe-sub">Optional: lyrics and artists</h2>
      <p className="muted oobe-hint">Off unless you turn them on. Lyrics beside your music or inside your files work either way. You can change these later in Settings → Metadata agents.</p>
      <div className="oobe-consents">
        <label className="oobe-consent">
          <input type="checkbox" checked={lyrics} onChange={(e) => put('lyricsOnline', e.target.checked, setLyrics)} />
          <span><b>Find lyrics online, free and with no account</b><small>Lyrics that follow the music and karaoke for songs with none of their own, from LRCLIB. When someone opens a song's lyrics, your server sends LRCLIB its artist, title, album and length, and keeps the answer on this server. The words belong to their writers.</small></span>
        </label>
        <label className="oobe-consent">
          <input type="checkbox" checked={info} onChange={(e) => put('musicInfoOnline', e.target.checked, setInfo)} />
          <span><b>Artist biographies and photos</b><small>From Wikipedia, credited on the artist's page. Your server sends Wikipedia each artist's name.</small></span>
        </label>
      </div>
    </>
  )
}
