// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Fragment, useEffect, useRef, useState, type ReactNode } from 'react'
import { HelpLink, PageHelp } from '../components/HelpLink'
import { useLocation } from 'react-router-dom'
import { api } from '../api'
import { IconCheck } from '../icons'
import { toast } from '../state'
import { Select } from '../ui'
import { Row, Section, Switch } from './settings/ui'
import { TickRow } from './setup/parts'
import { num, t, tn } from '../i18n'

// Lyrics and artist information from online. Both are off until the owner agrees, here or in
// first-time setup, and this is where it says exactly what goes where. What's beside the music
// and in its tags works anyway, with nothing sent anywhere.

type Draft = { s: Record<string, string>; set: (k: string, v: string) => void; on: (k: string, d?: string) => boolean }

type KaraokeStatus = { separator: { available: boolean; usable: boolean; name: string; version: string; source: string }; approximate: boolean; machine: string; usedBytes: number; songs: number }

const megabytes = (n: number) => (n >= 1 << 30 ? t('{size} GB', { size: num(n / (1 << 30), { minimumFractionDigits: 1, maximumFractionDigits: 1 }) }) : t('{size} MB', { size: Math.round(n / (1 << 20)) }))

// A translated sentence with links in it: each {name} in the text is replaced by links[name].
const linked = (text: string, links: Record<string, ReactNode>) => text.split(/(\{\w+\})/).map((part, i) => {
  const m = /^\{(\w+)\}$/.exec(part)
  return m && links[m[1]] !== undefined ? <Fragment key={i}>{links[m[1]]}</Fragment> : part
})

// What the server found to separate songs with, in a sentence.
function karaokeFound(k: KaraokeStatus | null) {
  const how = <HelpLink to="lyrics-karaoke#getting-the-better-version">{t('How to install one')}</HelpLink>
  if (!k) return t('Looking…')
  const sep = k.separator
  if (sep.available && !sep.usable) return t('{name} found, but this machine is too small to run it in a sensible time, so songs use the quick method.', { name: sep.name })
  if (sep.available) {
    const name = sep.name + (sep.version ? ` ${sep.version}` : '')
    return (sep.source === 'config' ? t('{name} is set in the server’s configuration.', { name }) : t('{name} found on this machine.', { name }))
      + ' ' + t('Songs are separated properly, one at a time, when nothing else needs the processor.')
  }
  if (k.approximate) return <>{t('Not installed. Songs use the quick method, which turns the singer down but can’t remove them.')} {how}</>
  return <>{t('Not installed, and FFmpeg is missing too, so karaoke versions can’t be made.')} {how}</>
}

// In Settings, part of the page's draft and saved with it.
export function MusicOnlineSettings({ s, set, on }: Draft) {
  const box = useRef<HTMLDivElement>(null)
  const { hash } = useLocation()
  const [cleared, setCleared] = useState(false)
  useEffect(() => { if (hash === '#lyrics') box.current?.scrollIntoView({ block: 'start' }) }, [hash])
  const useOnline = on('lyricsOnline', '0')
  const [kar, setKar] = useState<KaraokeStatus | null>(null)
  const look = (again: boolean) => { api.get<KaraokeStatus>('/karaoke/status' + (again ? '?look=1' : '')).then(setKar).catch(() => {}) }
  useEffect(() => { look(false) }, [])
  const auto = s.karaokeAuto || 'auto'
  return (
    <div ref={box} id="lyrics">
      <Section title={t('Lyrics and music information')} desc={t('A .lrc file beside a song and the lyrics in its tags are always used first, with nothing sent anywhere. These add what they don’t have.')} actions={<PageHelp to="lyrics-karaoke#finding-lyrics-online" />}>
        <Row label={t('Find lyrics online')} badge={t('No API key needed')} desc={<>{linked(t('Timed lyrics from {LRCLIB}, a free and open database. Only when someone opens the lyrics of a song with none of its own, your server sends LRCLIB that song’s artist, title, album and length. Never a file name, or who’s listening. Answers are kept in the cache on this server, aren’t shared, and don’t go into backups. The words belong to their writers.'), { LRCLIB: <a href="https://lrclib.net" target="_blank" rel="noreferrer">LRCLIB</a> })}</>}>
          <Switch label={t('Find lyrics online')} checked={useOnline} onChange={(v) => set('lyricsOnline', v ? '1' : '0')} />
        </Row>
        <Row label={t('Use LRCLIB’s public service')} dim={!useOnline} desc={t('Turn this off if you only want to use your own server or a key below.')}>
          <Switch label={t('Use LRCLIB’s public service')} disabled={!useOnline} checked={on('lyricsLrclib')} onChange={(v) => set('lyricsLrclib', v ? '1' : '0')} />
        </Row>
        <Row label={t('Your own LRCLIB server')} dim={!useOnline} stack desc={t('Optional. LRCLIB is open source, and a copy can run on your own network, so the look-ups stay at home. It’s asked first.')}>
          <input className="input" disabled={!useOnline} placeholder="http://192.168.1.20:3300" value={s.lyricsMirror || ''} spellCheck={false} autoComplete="off" onChange={(e) => set('lyricsMirror', e.target.value)} aria-label={t('Address of your LRCLIB server')} />
        </Row>
        <Row label={t('Musixmatch key')} dim={!useOnline} stack desc={<>{linked(t('Optional and paid. {Musixmatch} only gives whole lyrics on its paid plans; the free plan is refused and that’s what’s shown. Your key is kept on this server. It’s asked before LRCLIB, and is told the song’s artist, title and length.'), { Musixmatch: <a href="https://developer.musixmatch.com" target="_blank" rel="noreferrer">Musixmatch</a> })}</>}>
          <input className="input" disabled={!useOnline} placeholder={t('API key')} value={s.lyricsMusixmatchKey || ''} spellCheck={false} autoComplete="off" onChange={(e) => set('lyricsMusixmatchKey', e.target.value)} aria-label={t('Musixmatch key')} />
        </Row>
        <Row label={t('Artist biographies and photos')} badge={t('No API key needed')} desc={<>{linked(t('From {Wikipedia}, credited on the artist’s page. When an artist has no {file} in their folder, your server sends Wikipedia the artist’s name. The text is CC BY-SA 4.0, and each picture has its own licence, linked from its article. Nothing is written beside your music.'), { Wikipedia: <a href="https://www.wikipedia.org" target="_blank" rel="noreferrer">Wikipedia</a>, file: <code>artist.jpg</code> })}</>}>
          <Switch label={t('Artist biographies and photos')} checked={on('musicInfoOnline', '0')} onChange={(v) => set('musicInfoOnline', v ? '1' : '0')} />
        </Row>
        <Row label={t('Vocal separation')} badge={kar?.separator.available ? t('Nothing to set up') : undefined} desc={karaokeFound(kar)}>
          <button type="button" className="btn sm" onClick={() => look(true)}>{t('Look again')}</button>
        </Row>
        <Row label={t('Make karaoke versions automatically')} desc={t('When someone opens karaoke on a song, the server makes a version with the singer turned down: a quick one in a few seconds, and a properly separated one behind it if a separator is installed. Nothing is written beside your music.')}>
          <Select value={auto} aria-label={t('Make karaoke versions automatically')} onChange={(e) => set('karaokeAuto', e.target.value)} options={[
            { value: 'auto', label: t('Automatically') },
            { value: 'admin', label: t('Only when an admin asks') },
            { value: 'never', label: t('Never') },
          ]} />
        </Row>
        <Row label={t('Space for karaoke versions')} dim={auto === 'never'} desc={(kar ? tn('Using {size} for {count} song.', 'Using {size} for {count} songs.', kar.songs, { size: megabytes(kar.usedBytes) }) + ' ' : '') + t('When it’s full, the ones sung longest ago are cleared, and made again when someone wants them.')}>
          <Select value={s.karaokeCacheMB || '2048'} aria-label={t('Space for karaoke versions')} onChange={(e) => set('karaokeCacheMB', e.target.value)} options={[
            { value: '512', label: t('{size} MB', { size: 500 }) },
            { value: '2048', label: t('{size} GB', { size: 2 }) },
            { value: '10240', label: t('{size} GB', { size: 10 }) },
            { value: '51200', label: t('{size} GB', { size: 50 }) },
          ]} />
        </Row>
        <Row label={t('Lyrics kept from online')} desc={t('Forget everything fetched so far. Corrections made on this server are kept.')}>
          <button type="button" className="btn sm" disabled={cleared} onClick={() => { api.del('/music/lyrics-cache').then(() => { setCleared(true); toast(t('Lyrics from online forgotten')) }).catch((e: Error) => toast(e.message)) }}>{cleared ? <><IconCheck size={16} /> {t('Forgotten')}</> : t('Forget them')}</button>
        </Row>
      </Section>
    </div>
  )
}

// In first-time setup: the same two questions, in fewer words, saved as they're answered. They read
// what's saved, so coming back to the page shows what was chosen.
export function MusicOnlineStep() {
  const [lyrics, setLyrics] = useState(false)
  const [info, setInfo] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => { setLyrics(s.lyricsOnline === '1'); setInfo(s.musicInfoOnline === '1') }).catch(() => {}) }, [])
  const put = (key: string, on: boolean, then: (v: boolean) => void) => {
    then(on)
    api.put('/settings', { [key]: on ? '1' : '0' }).catch((e: Error) => { then(!on); toast(e.message) })
  }
  return (
    <div className="su-ticks">
      <TickRow kind="switch" checked={lyrics} onChange={(v) => put('lyricsOnline', v, setLyrics)} title={t('Find lyrics online')}
        text={t('From LRCLIB, a free and open database. When someone opens the lyrics of a song with none of its own, your server sends LRCLIB its artist, title, album and length, and keeps the answer on this server. The words belong to their writers.')} />
      <TickRow kind="switch" checked={info} onChange={(v) => put('musicInfoOnline', v, setInfo)} title={t('Artist biographies and photos')}
        text={t('From Wikipedia, credited on the artist’s page. Your server sends Wikipedia each artist’s name.')} />
    </div>
  )
}
