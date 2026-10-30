import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, episodeLabel, langName, posterUrl, type Item, type PlayInfo, type Show } from '../api'
import { IconBack, IconClose, IconExpand, IconMini, IconNext, IconPause, IconPlay } from '../icons'
import { usePlayerChrome } from './chrome'
import { Controls, useVideoState } from './Controls'
import { toggleFullscreen, useIdle, useSource } from './hooks'
import Remote, { DVD_ONLY, RemoteBar, type RemoteKey } from './Remote'
import TrackPicker from './TrackPicker'
import ViewSettings from './ViewSettings'
import { useViewPrefs } from './viewPrefs'

const legacyRemote = () => { try { return localStorage.getItem('wms.remote.video') === '1' } catch { return false } }

export default function VideoPlayer({ item, startAt }: { item: Item; startAt: number | null }) {
  const nav = useNavigate()
  const chrome = usePlayerChrome()
  const mini = chrome.mini
  const view = useViewPrefs(item.id, legacyRemote())
  const prefs = view.prefs
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [picker, setPicker] = useState<'audio' | 'subtitle' | null>(null)
  const video = useRef<HTMLVideoElement | null>(null)
  const root = useIdle(prefs.hideAfter * 1000, [mini])
  const [info, setInfo] = useState<PlayInfo | null>(null)
  const [audio, setAudio] = useState(0)
  const [sub, setSub] = useState(-1)
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const pendingSeek = useRef<number | null>(startAt ?? (item.progress && !item.progress.watched && item.progress.positionSec > 60 ? item.progress.positionSec : 0))
  const st = useVideoState(video)
  const remoteOn = prefs.remote
  const setRemoteOn = (on: boolean) => view.update({ remote: on }, view.scope)
  const [upNext, setUpNext] = useState<number | null>(null)
  const [next, setNext] = useState<{ item: Item; show?: Show | null; recommended: boolean } | null>(null)
  const [nextDismissed, setNextDismissed] = useState(false)
  const [ended, setEnded] = useState(false)

  // "Watch next": the next episode of this show, or (end of a show / a
  // movie) a recommendation, loaded once the credits are near.
  const nearEnd = (info?.durationSec || item.runtimeSec || 0) > 0 && st.time >= (info?.durationSec || item.runtimeSec) - 30
  useEffect(() => {
    if ((!nearEnd && !ended) || next) return
    if (item.nextId) api.get<Item>(`/items/${item.nextId}`).then((it) => setNext({ item: it, recommended: false })).catch(() => {})
    else api.get<{ item: Item | null; show: Show | null }>(`/items/${item.id}/recommend`).then((r) => { if (r.item) setNext({ item: r.item, show: r.show, recommended: true }) }).catch(() => {})
  }, [nearEnd, ended, next, item.id, item.nextId])

  // Episodes: count down into the next one when this one ends.
  useEffect(() => {
    const v = video.current
    if (!v) return
    const onEnded = () => { setEnded(true); if (item.nextId) setUpNext(8) }
    v.addEventListener('ended', onEnded)
    return () => v.removeEventListener('ended', onEnded)
  }, [item.nextId])
  useEffect(() => {
    if (upNext == null || nextDismissed || mini) return
    if (upNext <= 0) { nav(`/play/${item.nextId}`, { replace: true }); return }
    const t = setTimeout(() => setUpNext(upNext - 1), 1000)
    return () => clearTimeout(t)
  }, [upNext, item.nextId, nav, nextDismissed, mini])

  // "Skip intro" while inside the intro range (chapter or show setting).
  const inIntro = item.introEnd != null && st.time >= (item.introStart || 0) && st.time < item.introEnd - 1

  useEffect(() => {
    setInfo(null)
    api.get<PlayInfo>(`/items/${item.id}/play?audio=${audio}&subtitle=${sub}`).then(setInfo).catch((e) => setErr(e.message))
  }, [item.id, audio, sub])

  // HLS seeking is "seek the playlist"; our server playlist covers the whole
  // file so a plain currentTime assignment works for both modes.
  const seek = useCallback((t: number) => { const v = video.current; if (v) v.currentTime = Math.max(0, Math.min(t, (info?.durationSec || v.duration || t))) }, [info])

  useSource(video, info?.url || null, {
    startAt: () => pendingSeek.current,
    onReady: () => {
      const v = video.current
      if (!v) return
      if (pendingSeek.current && pendingSeek.current > 0 && Math.abs(v.currentTime - pendingSeek.current) > 1) { v.currentTime = pendingSeek.current }
      pendingSeek.current = null
      v.play().catch(() => setNeedClick(true))
    },
    onError: (m) => setErr(m),
  })

  // Progress reporting.
  useEffect(() => {
    const send = () => {
      const v = video.current
      if (!v || !info || v.currentTime < 5) return
      api.post(`/items/${item.id}/progress`, { positionSec: v.currentTime, durationSec: info.durationSec || v.duration }).catch(() => {})
    }
    const t = setInterval(send, 10000)
    window.addEventListener('beforeunload', send)
    return () => { clearInterval(t); window.removeEventListener('beforeunload', send); send() }
  }, [item.id, info])

  // Keyboard and TV remotes (not while minimized: the app owns the keys then).
  useEffect(() => {
    if (mini) return
    const onKey = (e: KeyboardEvent) => {
      const v = video.current
      if (!v || picker || settingsOpen) return
      if (document.querySelector('.modal-backdrop, .scene-panel')) return // a window over the player has the keys
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
      if (e.key.startsWith('Arrow') && (e.target as HTMLElement)?.closest?.('.bottom, .top, .remote, .watch-next')) return
      switch (e.key) {
        case ' ': case 'k': case 'MediaPlayPause': e.preventDefault(); v.paused ? v.play() : v.pause(); break
        case 'ArrowRight': seek(v.currentTime + 10); break
        case 'ArrowLeft': seek(v.currentTime - 10); break
        case 'ArrowUp': v.volume = Math.min(1, v.volume + 0.1); break
        case 'ArrowDown': v.volume = Math.max(0, v.volume - 0.1); break
        case 'f': toggleFullscreen(root.current); break
        case 'm': v.muted = !v.muted; break
        case 'r': setRemoteOn(!remoteOn); break
        case 'Escape': case 'GoBack': case 'BrowserBack': if (!document.fullscreenElement) { e.preventDefault(); chrome.minimize() } break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const switchTrack = (kind: 'audio' | 'sub', id: number) => {
    const v = video.current
    pendingSeek.current = v ? v.currentTime : 0
    kind === 'audio' ? setAudio(id) : setSub(id)
  }
  const duration = info?.durationSec || item.runtimeSec || 0
  // The same remote as for discs; disc-only keys are greyed out.
  const onRemote = (k: RemoteKey) => {
    const v = video.current
    switch (k) {
      case 'eject': chrome.close(); break
      case 'audio': if ((info?.audio.length || 0) > 1) setPicker('audio'); break
      case 'subtitle': if (info?.subtitles.length) setPicker('subtitle'); break
      case 'rew': case 'prev': if (v) seek(v.currentTime - (k === 'prev' ? 60 : 10)); break
      case 'ff': if (v) seek(v.currentTime + 30); break
      case 'next': if (item.nextId) nav(`/play/${item.nextId}`); else if (v) seek(v.currentTime + 60); break
      case 'play': v?.play().catch(() => {}); break
      case 'pause': v?.pause(); break
      case 'stop': if (v) { v.pause(); seek(0) } break
    }
  }
  const remoteEnabled = (k: RemoteKey) => {
    if (DVD_ONLY.includes(k)) return false
    if (k === 'audio') return (info?.audio.length || 0) > 1
    if (k === 'subtitle') return (info?.subtitles.length || 0) > 0
    return true
  }
  const subtitle = [item.showTitle, episodeLabel(item)].filter(Boolean).join(' · ')
  const trackLabel = (t: { language: string; title?: string }) => [langName(t.language), t.title].filter(Boolean).join(' · ')
  const togglePlay = () => { const v = video.current; if (v && !needClick) v.paused ? v.play().catch(() => {}) : v.pause() }
  return (
    <div className={'player' + (mini ? ' mini' : '')} ref={root} tabIndex={-1} onClick={togglePlay}
      onDoubleClick={() => (mini ? chrome.expand() : toggleFullscreen(root.current))}>
      <div className="stage">
        <video ref={video} playsInline preload="auto" style={{ objectFit: prefs.picture === 'fill' ? 'cover' : 'contain' }} />
      </div>
      {st.waiting && !needClick && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && <div className="big-play"><button onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {err && <div className="overlay-msg"><div className="box">Playback failed: {err}<br /><button className="btn" style={{ marginTop: 10 }} onClick={chrome.close}>Back</button></div></div>}
      {mini ? (
        <div className="mini-bar" onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
          <div className="mini-info" onClick={chrome.expand}>
            <div className="mini-title">{item.showTitle || item.title}</div>
            <div className="mini-sub">{item.showTitle ? [episodeLabel(item), item.title].filter(Boolean).join(' · ') : item.year}</div>
          </div>
          <div className="mini-btns">
            <button className="ibtn" onClick={() => onRemote('rew')} title="Back 10 s"><IconBack /></button>
            <button className="ibtn big" onClick={togglePlay} title="Play/Pause">{st.playing ? <IconPause /> : <IconPlay />}</button>
            {item.nextId ? <button className="ibtn" onClick={() => nav(`/play/${item.nextId}`)} title="Next episode"><IconNext /></button> : <span />}
            <button className="ibtn" onClick={chrome.expand} title="Full player"><IconExpand /></button>
            <button className="ibtn" onClick={chrome.close} title="Stop"><IconClose /></button>
          </div>
        </div>
      ) : (
        <>
          <div className="top" onClick={(e) => e.stopPropagation()}>
            <button className="ibtn" onClick={chrome.minimize} title="Back (keeps playing in the mini player)"><IconBack /></button>
            <div className="grow"><h2>{item.title}</h2><div className="sub">{[subtitle || item.year, info?.type === 'direct' ? 'Direct play' : 'Transcoding'].filter(Boolean).join(' · ')}</div></div>
            {item.nextId && <button className="btn sm" onClick={() => nav(`/play/${item.nextId}`, { replace: true })} title="Next episode"><IconNext /> <span className="hide-narrow">Next episode</span></button>}
            <button className="ibtn" onClick={chrome.minimize} title="Mini player"><IconMini /></button>
          </div>
          {remoteOn && (prefs.remoteStyle === 'bar'
            ? <RemoteBar onKey={onRemote} enabled={remoteEnabled} disc={false} />
            : <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label="WittyMedia" />)}
          {inIntro && <button className="btn skip-intro" onClick={(e) => { e.stopPropagation(); seek(item.introEnd!) }}>Skip intro ›</button>}
          {next && (nearEnd || ended) && !nextDismissed && (
            <div className="watch-next" onClick={(e) => e.stopPropagation()}>
              <div className="wn-head">{next.recommended
                ? (next.show ? `Because you watched ${item.showTitle || item.title}` : 'You might also like')
                : upNext != null ? `Next episode in ${upNext}s` : 'Watch next'}</div>
              <div className="wn-body" onClick={() => nav(`/play/${next.item.id}`, { replace: true })}>
                <div className={'wn-thumb' + (next.item.showId != null ? ' wide' : '')}>{next.item.poster && <img src={posterUrl(next.item)} alt="" />}</div>
                <div>
                  <div className="wn-title">{next.show ? next.show.title : next.item.showTitle || next.item.title}</div>
                  <div className="muted" style={{ fontSize: 12 }}>{next.item.showId != null ? [episodeLabel(next.item), next.item.title].filter(Boolean).join(' · ') : next.item.year}</div>
                </div>
              </div>
              <div className="wn-actions">
                <button className="btn sm primary" onClick={() => nav(`/play/${next.item.id}`, { replace: true })}><IconPlay /> Play</button>
                <button className="btn sm ghost" onClick={() => { setNextDismissed(true); setUpNext(null) }}>Dismiss</button>
              </div>
            </div>
          )}
          <Controls video={video} duration={duration} onSeek={seek} onFullscreen={() => toggleFullscreen(root.current)}
            audio={info?.audio.map((a) => ({ id: a.index, label: [langName(a.language), a.title, a.codec?.toUpperCase(), a.channels ? a.channels + 'ch' : ''].filter(Boolean).join(' · '), active: a.index === audio }))}
            subtitles={info?.subtitles.map((s) => ({ id: s.index, label: trackLabel(s), active: s.index === sub }))}
            onAudio={(id) => switchTrack('audio', id)} onSubtitle={(id) => switchTrack('sub', id)} onPick={(k) => { if (k !== 'angle') setPicker(k) }}
            showSeek={prefs.seekBar} remoteOn={remoteOn} onRemote={() => setRemoteOn(!remoteOn)} onSettings={() => setSettingsOpen((o) => !o)} />
          {settingsOpen && <ViewSettings prefs={prefs} scope={view.scope} isDisc={false} onClose={() => setSettingsOpen(false)} onChange={(p) => view.update(p, view.scope)} onScope={view.setScope} />}
          {picker && info && (
            <TrackPicker heading={picker === 'audio' ? 'Audio & language' : 'Subtitles'} onClose={() => setPicker(null)}
              onPick={(id) => switchTrack(picker === 'audio' ? 'audio' : 'sub', id)}
              tiles={picker === 'audio'
                ? info.audio.map((a) => ({ id: a.index, title: langName(a.language), sub: a.title, detail: [a.codec?.toUpperCase(), a.channels ? a.channels + ' ch' : ''].filter(Boolean).join(' · '), active: a.index === audio }))
                : [{ id: -1, title: 'Off', active: sub < 0 }, ...info.subtitles.map((x) => ({ id: x.index, title: langName(x.language), sub: x.title, detail: x.codec?.toUpperCase(), active: x.index === sub }))]} />
          )}
        </>
      )}
    </div>
  )
}
