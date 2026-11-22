// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Fragment, useEffect, useRef, useState, type ReactNode } from 'react'
import { HelpLink, PageHelp } from '../components/HelpLink'
import { Link, useLocation } from 'react-router-dom'
import { api } from '../api'
import { IconCheck } from '../icons'
import { toast } from '../state'
import { Select } from '../ui'
import { Row, Section, Switch } from './settings/ui'
import { TickRow } from './setup/parts'
import { keySaved, SavedKeyField } from '../components/SavedKey'
import { num, t, tk, tn, tx } from '../i18n'

// Lyrics and artist information from online. Both are off until the owner agrees, here or in
// first-time setup, and this is where it says exactly what goes where. What's beside the music
// and in its tags works anyway, with nothing sent anywhere.

type Draft = { s: Record<string, string>; set: (k: string, v: string) => void; on: (k: string, d?: string) => boolean }

type KaraokeStatus = { separator: { available: boolean; usable: boolean; name: string; version: string; source: string }; approximate: boolean; machine: string; usedBytes: number; songs: number }

type SeparatorState = {
  supported: boolean; why: string; task: string; failed: string; phase: string; percent: number
  packages: string[]; uv: string; python: string; model: string; downloadMB: number; cpuDownloadMB: number
  accel: Accel
  // directml: ONNX Runtime's DirectML build comes with an install for a card (64-bit Windows)
  directml?: boolean; directmlPackages?: string[]
  // accel: what songs run on now; auto: what the install chose; choices: what karaokeDevice can be
  installed?: { version: string; model: string; at: string; sizeBytes: number; accel: Accel; auto?: Accel; choices?: Accel[]; device?: string }
}
type Accel = { id: string; name: string; device: string; downloadMB: number; why?: string }
const gb = (mb: number) => t('{size} GB', { size: num(mb / 1000, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) })

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
    return (sep.source === 'config' ? t('{name} is set in the server’s configuration.', { name }) : sep.source === 'installed' ? t('{name} is installed.', { name }) : t('{name} found on this machine.', { name }))
      + ' ' + t('Songs are separated properly, one at a time, when nothing else needs the processor.')
  }
  if (k.approximate) return <>{t('Not installed. Songs use the quick method, which turns the singer down but can’t remove them.')} {how}</>
  return <>{t('Not installed, and FFmpeg is missing too, so karaoke versions can’t be made.')} {how}</>
}

// Demucs with one press: what will be downloaded and from where, said before it starts; progress while it
// runs; and once it's in, how much room it takes and a way to take it out again. Its licenses are listed in
// Settings, About.
function SeparatorInstall({ onChange, s, set }: { onChange: () => void; s: Draft['s']; set: Draft['set'] }) {
  const [st, setSt] = useState<SeparatorState | null>(null)
  const [sure, setSure] = useState(false)
  const [busy, setBusy] = useState(false)
  // The graphics card when there's one Demucs can use: on unless turned off here.
  const [useCard, setUseCard] = useState(true)
  const read = () => api.get<SeparatorState>('/karaoke/separator').then((s) => { setSt(s); return s }).catch(() => null)
  useEffect(() => { void read() }, [])
  // While it installs, look every couple of seconds; when it ends, the page's other lines catch up.
  const running = !!st?.task
  useEffect(() => {
    if (!running) return
    const timer = window.setInterval(() => { void read().then((s) => { if (s && !s.task) onChange() }) }, 2000)
    return () => window.clearInterval(timer)
  }, [running]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!st) return null
  const card = st.accel.id !== 'cpu'
  const willUse = card && useCard
  const download = willUse ? st.downloadMB : st.cpuDownloadMB
  const install = () => { setBusy(true); api.post('/karaoke/separator/install' + (willUse ? '' : '?accel=cpu')).then(() => { setSure(false); return read() }).catch((e: Error) => toast(e.message)).finally(() => setBusy(false)) }
  const remove = () => { setBusy(true); api.del('/karaoke/separator').then(() => { toast(running ? t('Install stopped') : t('Demucs removed')); onChange(); return read() }).catch((e: Error) => toast(e.message)).finally(() => setBusy(false)) }
  if (st.installed) {
    const ins = st.installed
    const choices = ins.choices ?? []
    return (
      <>
        <Row label={t('Demucs for karaoke')} badge={t('Installed')} desc={<>{linked(ins.accel.id !== 'cpu'
          ? t('Demucs {version} with its {model} model, running on {device}, and on the processor for any song the card can’t manage, taking {size} in the server’s own folder. Its licenses, and those of everything installed with it, are in {about}.')
          : t('Demucs {version} with its {model} model, running on {device}, taking {size} in the server’s own folder. Its licenses, and those of everything installed with it, are in {about}.'),
          { version: ins.version, model: ins.model, device: ins.accel.name, size: megabytes(ins.sizeBytes), about: <Link to="/settings/about">{t('Settings, About')}</Link> })}</>}>
          <button type="button" className="btn sm" disabled={busy} onClick={remove}>{t('Remove')}</button>
        </Row>
        {choices.length > 1 && (
          <Row label={t('Runs on')} desc={t('Automatic uses the fastest that passed its test when Demucs was installed. Another choice is used from the next song.')}>
            <Select value={s.karaokeDevice || ins.device || 'auto'} aria-label={t('What Demucs runs on')} onChange={(e) => set('karaokeDevice', e.target.value)} options={[
              { value: 'auto', label: t('Automatic ({device})', { device: ins.auto?.name ?? ins.accel.name }) },
              ...choices.map((c) => ({ value: c.id, label: c.id === 'cpu' ? t('The processor') : c.name })),
            ]} />
          </Row>
        )}
      </>
    )
  }
  if (!st.supported) return <Row label={t('Demucs for karaoke')} desc={tx(st.why)}><span className="muted">{t('Not available here')}</span></Row>
  if (running) {
    return (
      <Row label={t('Demucs for karaoke')} stack desc={<>{st.phase ? tx(st.phase) : t('Starting')}… {t('This takes a while: up to {size} is downloaded. Songs use the quick version until it’s done.', { size: gb(st.downloadMB) })}</>}>
        <div className="sep-progress">
          <progress max={100} value={st.percent} aria-label={t('Installing Demucs')} />
          <button type="button" className="btn sm" disabled={busy} onClick={remove}>{t('Stop')}</button>
        </div>
      </Row>
    )
  }
  return (
    <Row label={t('Demucs for karaoke')} stack desc={<>
      {linked(t('Installs {Demucs} (MIT license), the best of the separators, so karaoke versions are properly separated: the voice taken out cleanly, and drums, bass and the rest as parts of their own.'), { Demucs: <a href="https://github.com/facebookresearch/demucs" target="_blank" rel="noreferrer">Demucs</a> })} {card ? t('This server has {device}, so songs are separated there, in well under a minute each, and on the processor if the card can’t manage one.', { device: st.accel.name }) : <>{st.accel.why ? tx(st.accel.why) : ''} {t('A song takes a few minutes there, one at a time, when nothing else needs it.')}</>}
      {st.failed && <> <strong>{t('The last try failed:')}</strong> {st.failed}</>}
    </>}>
      {!sure ? <button type="button" className="btn sm primary" onClick={() => setSure(true)}>{t('Install Demucs…')}</button> : (
        <div className="sep-confirm" role="group" aria-label={t('Install Demucs')}>
          {card && (
            <label className="sep-card">
              <input type="checkbox" checked={useCard} onChange={(e) => setUseCard(e.target.checked)} />
              <span>{t('Use {device}', { device: st.accel.name })} <small>{t('(on by default; off is a smaller download, {size}, and slower songs)', { size: gb(st.cpuDownloadMB) })}</small></span>
            </label>
          )}
          <p>{t('Your server will download about {size} into its own folder, which backups leave out:', { size: gb(download) })}</p>
          <ul>
            <li>{linked(t('{uv} {version}, an installer (MIT or Apache-2.0), from GitHub, checked against a fingerprint the server carries'), { uv: <a href="https://github.com/astral-sh/uv" target="_blank" rel="noreferrer">uv</a>, version: st.uv })}</li>
            <li>{t('Python {version} (PSF license), through uv, which checks it', { version: st.python })}</li>
            <li>{willUse
              ? (st.accel.id === 'cuda'
                ? t('{packages}: Demucs and PyTorch (BSD license) built for {device}, from PyPI and pytorch.org, with the CUDA libraries it needs (NVIDIA’s license)', { packages: st.packages.join(', '), device: st.accel.name })
                : t('{packages}: Demucs and PyTorch (BSD license) built for {device}, from PyPI and pytorch.org', { packages: st.packages.join(', '), device: st.accel.name }))
              : t('{packages}: Demucs and PyTorch (BSD license) built for the processor, from PyPI and pytorch.org', { packages: st.packages.join(', ') })}</li>
            <li>{t('The {model} model, from Demucs’s makers', { model: st.model })}</li>
            {willUse && st.directml && st.directmlPackages && <li>{st.accel.id === 'dml'
              ? t('{packages}: ONNX Runtime with DirectML (MIT license) and onnx (Apache-2.0), from PyPI, checked against fingerprints the server carries, for this card. The model is converted for it here, on this machine.', { packages: st.directmlPackages.join(', ') })
              : t('{packages}: ONNX Runtime with DirectML (MIT license) and onnx (Apache-2.0), from PyPI, checked against fingerprints the server carries, so DirectML can be chosen later. The model is converted for it here, on this machine.', { packages: st.directmlPackages.join(', ') })}</li>}
          </ul>
          <p>{t('Nothing about you or your music is sent. Every license is listed in Settings, About once it’s in, and Remove takes it all out again.')}</p>
          <div className="sep-buttons">
            <button type="button" className="btn sm primary" disabled={busy} onClick={install}>{t('Download and install')}</button>
            <button type="button" className="btn sm" onClick={() => setSure(false)}>{t('Cancel')}</button>
          </div>
        </div>
      )}
    </Row>
  )
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
      <Section title={tk('Lyrics and music information')} desc={t('A .lrc file beside a song and the lyrics in its tags are always used first, with nothing sent anywhere. These add what they don’t have.')} actions={<PageHelp to="lyrics-karaoke#finding-lyrics-online" />}>
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
          <SavedKeyField name="Musixmatch" saved={keySaved(s, 'lyricsMusixmatchKey')} disabled={!useOnline} placeholder={t('API key')} value={s.lyricsMusixmatchKey || ''} onChange={(v) => set('lyricsMusixmatchKey', v)} label={t('Musixmatch key')} />
        </Row>
        <Row label={t('Artist biographies and photos')} badge={t('No API key needed')} desc={<>{linked(t('From {Wikipedia}, credited on the artist’s page. When an artist has no {file} in their folder, your server sends Wikipedia the artist’s name. The text is CC BY-SA 4.0, and each picture has its own licence, linked from its article. Nothing is written beside your music.'), { Wikipedia: <a href="https://www.wikipedia.org" target="_blank" rel="noreferrer">Wikipedia</a>, file: <code>artist.jpg</code> })}</>}>
          <Switch label={t('Artist biographies and photos')} checked={on('musicInfoOnline', '0')} onChange={(v) => set('musicInfoOnline', v ? '1' : '0')} />
        </Row>
        <Row label={t('Vocal separation')} badge={kar?.separator.available ? t('Nothing to set up') : undefined} desc={karaokeFound(kar)}>
          <button type="button" className="btn sm" onClick={() => look(true)}>{t('Look again')}</button>
        </Row>
        <SeparatorInstall onChange={() => look(true)} s={s} set={set} />
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
        <Row label={t('Separate the whole library ahead of time')} dim={auto === 'never' || !kar?.separator.available} desc={kar?.separator.available
          ? t('Off unless you choose it. Songs are separated one at a time, only while nothing else is being made and nobody is watching or listening to something it would slow, and it stops when the space for karaoke versions is 80% full, so it never clears songs people sang to make room. Without it, a song is separated the first time someone opens karaoke on it.')
          : t('Needs a separator: install Demucs above. Until then a song is made into a karaoke version the first time someone opens karaoke on it.')}>
          <Select value={s.karaokeLibrary || 'off'} aria-label={t('Separate the whole library ahead of time')} disabled={auto === 'never' || !kar?.separator.available} onChange={(e) => set('karaokeLibrary', e.target.value)} options={[
            { value: 'off', label: t('Off') },
            { value: 'idle', label: t('When the server is idle') },
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

// In first-time setup, Extras: the same one-press Demucs install as Settings, on a page of its own. It goes on in the
// background, so the page doesn't hold anyone up, and a choice made here is saved as it's made.
export function SeparatorStep() {
  const [s, setS] = useState<Record<string, string>>({})
  useEffect(() => { api.get<Record<string, string>>('/settings').then(setS).catch(() => {}) }, [])
  const set = (k: string, v: string) => { setS((x) => ({ ...x, [k]: v })); api.put('/settings', { [k]: v }).catch((e: Error) => toast(e.message)) }
  return <div className="set-card su-separator"><SeparatorInstall onChange={() => {}} s={s} set={set} /></div>
}
