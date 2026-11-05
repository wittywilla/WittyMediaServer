import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, ApiError, episodeLabel, fitWidth, langName, mediaUrl, posterUrl, sized, type Item, type PlaybackAnswer, type Show } from '../api'
import { IconBack, IconClose, IconExpand, IconInfo, IconMini, IconNext, IconPause, IconPlay } from '../icons'
import { browserCapabilities } from './capabilities'
import PlayInfoPanel from './PlayInfoPanel'
import { toast } from '../state'
import { usePlayerChrome } from './chrome'
import PickIntroBar from './PickIntroBar'
import { Controls, useVideoState } from './Controls'
import { haveHls, toggleFullscreen, useIdle, useSource, type NetNote } from './hooks'
import { deviceStreaming, mayBeRobust, measureSpeed, rememberSpeed, setDeviceStreaming, type DeviceStreaming } from './streaming'
import Hls from 'hls.js'
import Remote, { DVD_ONLY, RemoteBar, type RemoteKey } from './Remote'
import TrackPicker from './TrackPicker'
import ViewSettings from './ViewSettings'
import { useViewPrefs } from './viewPrefs'
import { getTvMode } from '../tvmode/mode'

haveHls(Hls)

const MODE_WORDS: Record<string, string> = { direct: 'Direct play', remux: 'Rewrapped', transcode: 'Transcoding' }

const legacyRemote = () => { try { return localStorage.getItem('wms.remote.video') === '1' } catch { return false } }

export default function VideoPlayer({ item, startAt }: { item: Item; startAt: number | null }) {
  const nav = useNavigate()
  const chrome = usePlayerChrome()
  // A list being played through carries on with its next item.
  const search = new URLSearchParams(useLocation().search)
  const listId = search.get('collection')
  // Choosing where the intro ends, from the edit dialog.
  const pickIntro = search.get('pickIntro')
  const nextHref = `/play/${item.nextId}${listId ? `?collection=${encodeURIComponent(listId)}` : ''}`
  const mini = chrome.mini
  const view = useViewPrefs(item.id, legacyRemote())
  const prefs = view.prefs
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [picker, setPicker] = useState<'audio' | 'subtitle' | null>(null)
  const video = useRef<HTMLVideoElement | null>(null)
  const root = useIdle(prefs.hideAfter * 1000, [mini])
  const [info, setInfo] = useState<PlaybackAnswer | null>(null)
  const [infoOpen, setInfoOpen] = useState(false)
  // What has been asked of the server. Each change is a new request. A subtitle the
  // player draws itself changes with none: that's sub, which starts as the server's
  // choice. restartAt: a rewrapped stream only exists from where it started, so going
  // past what's been made asks for a new one, starting there.
  // reopen counts fresh requests for the same thing at the same place: a robust stream
  // that couldn't go on (the connection was gone, or the server forgot the stream).
  const [want, setWant] = useState<{ audio?: number; burn?: { index: number; styled: boolean } | null; video?: number; flat3d?: boolean; restartAt?: number; convert?: boolean; reopen?: number }>({})
  // What a robust stream's player did: shown for a moment, and listed in the information panel.
  const [note, setNote] = useState<NetNote | null>(null)
  const [netLog, setNetLog] = useState<string[]>([])
  const [streamPref, setStreamPref] = useState<DeviceStreaming>(deviceStreaming)
  const [sub, setSub] = useState<number | null>(null)
  const audio = want.audio ?? info?.audioIndex ?? 0
  const burn = want.burn
  const flat3d = !!want.flat3d
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const pendingSeek = useRef<number | null>(startAt ?? item.progress?.resumeSec ?? 0)
  const st = useVideoState(video)
  const offset = info?.timeOffsetSec || 0
  const now = st.time + offset
  const remoteOn = prefs.remote
  const setRemoteOn = (on: boolean) => view.update({ remote: on }, view.scope)
  const [upNext, setUpNext] = useState<number | null>(null)
  const [next, setNext] = useState<{ item: Item; show?: Show | null; recommended: boolean } | null>(null)
  const [nextDismissed, setNextDismissed] = useState(false)
  const [ended, setEnded] = useState(false)

  // "Watch next": the next episode of this show, or (end of a show / a
  // movie) a recommendation, loaded once the credits are near.
  const nearEnd = (info?.durationSec || item.runtimeSec || 0) > 0 && now >= (info?.durationSec || item.runtimeSec) - 30
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
    if (upNext <= 0) { nav(nextHref, { replace: true }); return }
    const t = setTimeout(() => setUpNext(upNext - 1), 1000)
    return () => clearTimeout(t)
  }, [upNext, item.nextId, nav, nextDismissed, mini])

  // "Skip intro" while inside the intro range (chapter or show setting).
  const inIntro = item.introEnd != null && now >= (item.introStart || 0) && now < item.introEnd - 1

  useEffect(() => {
    setInfo(null)
    let current = true
    let retry = 0
    const start = want.restartAt ?? pendingSeek.current
    const body: Record<string, unknown> = { profile: 'web', capabilities: browserCapabilities(), startSec: start ?? 0, flatten3d: flat3d }
    if (want.audio != null) body.audio = want.audio
    if (burn) { body.subtitle = burn.index; body.burnSubtitle = burn.styled } else if (sub != null) body.subtitle = sub
    if (want.video != null) body.video = want.video
    if (want.convert) body.forceTranscode = true
    const device = deviceStreaming()
    if (device !== 'auto') body.streaming = device
    // A robust stream starts at a rate chosen from what this connection carries.
    const ask = async (wait: number): Promise<void> => {
      if (await mayBeRobust()) body.measuredKbps = await measureSpeed()
      try {
        const a = await api.post<PlaybackAnswer>(`/items/${item.id}/playback`, body)
        if (!current) return
        setInfo(a)
        if (want.reopen) setNote((n) => (n?.kind === 'lost' ? { kind: 'back', text: 'Reconnected, at the same place.' } : n))
        setSub((s) => (s == null ? a.subtitleIndex : s))
        // A stream starts where it was asked to; a file or a full stream seeks once open.
        pendingSeek.current = a.seek === 'restart' ? null : (a.startAtSec || null)
      } catch (e) {
        if (!current) return
        // Picking up after a dropped connection: keep asking, waiting longer each time, rather
        // than stop with an error the moment the connection is still away.
        if (want.reopen && !(e instanceof ApiError && e.status >= 400 && e.status < 500)) {
          setNote({ kind: 'lost', text: 'Connection lost. Trying again…' })
          retry = window.setTimeout(() => { ask(Math.min(wait * 2, 8000)) }, wait)
          return
        }
        setErr((e as Error).message)
      }
    }
    ask(1000)
    return () => { current = false; clearTimeout(retry) }
  }, [item.id, want]) // eslint-disable-line react-hooks/exhaustive-deps

  // Subtitles the player draws: the chosen track, with its cues in the player's time.
  const burned = info?.subtitleDelivery === 'burn'
  const shownSub = info && sub != null && sub >= 0 && !burned ? info.subtitles.find((s) => s.index === sub) : undefined
  const subUrl = shownSub?.url || ''
  useEffect(() => {
    const v = video.current
    if (!v || !subUrl) return
    // Fetched and handed over as a local file: a <track> can't send a sign-in, and an app
    // whose page isn't on the server's own address couldn't load one directly at all.
    let stopped = false
    let track: HTMLTrackElement | null = null
    let local = ''
    fetch(mediaUrl(subUrl)).then((r) => { if (!r.ok) throw new Error(String(r.status)); return r.text() }).then((vtt) => {
      if (stopped) return
      local = URL.createObjectURL(new Blob([vtt], { type: 'text/vtt' }))
      track = document.createElement('track')
      track.kind = 'subtitles'
      track.src = local
      track.srclang = shownSub?.language?.slice(0, 2) || 'und'
      track.label = shownSub?.title || shownSub?.languageName || 'Subtitles'
      track.default = true
      v.appendChild(track)
      track.track.mode = 'showing'
      // Clear of the controls, which sit over the bottom of the picture.
      const t = track
      t.addEventListener('load', () => { for (const c of Array.from(t.track.cues || [])) { const cue = c as VTTCue; if (cue.line === 'auto') cue.line = -3 } })
    }).catch(() => toast('These subtitles couldn’t be loaded.'))
    return () => {
      stopped = true
      if (track) { track.track.mode = 'disabled'; track.remove() }
      if (local) URL.revokeObjectURL(local)
    }
  }, [subUrl]) // eslint-disable-line react-hooks/exhaustive-deps

  // Done with a stream (left the player, another audio or subtitle track):
  // the server can stop converting it now rather than after a while.
  const streamId = info?.streamId || ''
  useEffect(() => {
    if (!streamId) return
    return () => { api.del(`/hls/${streamId}`).catch(() => {}) }
  }, [streamId])

  // A file, or a full stream with its whole length listed ahead, seeks anywhere. A
  // rewrapped stream seeks within what has been made so far, and otherwise starts anew.
  const seek = useCallback((t: number) => {
    const v = video.current
    if (!v) return
    const to = Math.max(0, Math.min(t, (info?.durationSec || v.duration || t)))
    if (info?.seek !== 'restart') { v.currentTime = to; return }
    const local = to - offset
    let made = 0
    try { if (v.seekable.length) made = v.seekable.end(v.seekable.length - 1) } catch { /* not open yet */ }
    if (local >= 0 && local <= made - 1) { v.currentTime = local; return }
    setWant((w) => ({ ...w, restartAt: to }))
  }, [info, offset])

  // Robust streams: the player says what it did, and a stream that can't go on is asked
  // for again at the same place. A file that stalled is asked for as a steadier stream.
  const robust = info?.streaming?.mode === 'robust' ? info.streaming.player : null
  const lastNote = useRef<NetNote['kind'] | null>(null)
  const onNote = useCallback((n: NetNote) => {
    lastNote.current = n.kind
    setNote(n)
    if (n.kind !== 'lost') setNetLog((l) => [...l.slice(-7), n.text])
  }, [])
  useEffect(() => {
    if (!note || note.kind === 'lost') return
    const t = setTimeout(() => setNote(null), 6000)
    return () => clearTimeout(t)
  }, [note])
  const hlsRef = useSource(video, info?.url || null, {
    startAt: () => pendingSeek.current,
    growing: info?.seek === 'restart',
    robust, measuredKbps: info?.streaming?.measuredKbps, startHeight: info?.streaming?.ladder?.[0]?.height, onNote,
    onReopen: (at) => {
      const est = hlsRef.current?.bandwidthEstimate
      if (est) rememberSpeed(est / 1000) // what the connection was doing, for the new stream's start
      pendingSeek.current = at + offset
      const steadier = lastNote.current === 'stall' && info?.decision !== 'transcode'
      setWant((w) => ({ ...w, restartAt: undefined, reopen: (w.reopen || 0) + 1, convert: w.convert || steadier }))
    },
    onReady: () => {
      const v = video.current
      if (!v) return
      if (pendingSeek.current && pendingSeek.current > 0 && Math.abs(v.currentTime - pendingSeek.current) > 1) { v.currentTime = pendingSeek.current }
      pendingSeek.current = null
      v.play().catch(() => setNeedClick(true))
    },
    onError: (m) => { if (!convertInstead()) setErr(m) },
  })

  // Browsers sometimes say they can play what they then can't, or find a file's codecs
  // aren't what its first bytes suggested. Rather than an error, it's converted, once.
  const convertInstead = () => {
    if (want.convert || !info || info.decision === 'transcode') return false
    const v = video.current
    pendingSeek.current = v ? v.currentTime + offset : 0
    setErr('')
    setWant((w) => ({ ...w, restartAt: undefined, convert: true }))
    return true
  }
  useEffect(() => {
    const v = video.current
    if (!v) return
    const onFail = () => {
      if (robust && v.error?.code === MediaError.MEDIA_ERR_NETWORK) return // asked for again at the same place (useSource)
      if (v.error && !convertInstead()) setErr(v.error.message || 'This file can’t be played here.')
    }
    v.addEventListener('error', onFail)
    return () => v.removeEventListener('error', onFail)
  })

  // Progress reporting.
  useEffect(() => {
    const send = () => {
      const v = video.current
      if (!v || !info || v.currentTime + (info.timeOffsetSec || 0) < 5) return
      api.post(`/items/${item.id}/progress`, { positionSec: v.currentTime + (info.timeOffsetSec || 0), durationSec: info.durationSec || v.duration }).catch(() => {})
    }
    const t = setInterval(send, 10000)
    window.addEventListener('beforeunload', send)
    return () => { clearInterval(t); window.removeEventListener('beforeunload', send); send() }
  }, [item.id, info])

  // The picture has the arrow keys until the viewer goes to the controls.
  useEffect(() => { if (!mini) root.current?.focus({ preventScroll: true }) }, [mini, root])

  // Keyboard and TV remotes (not while minimized: the app owns the keys then).
  useEffect(() => {
    if (mini) return
    const onKey = (e: KeyboardEvent) => {
      const v = video.current
      if (!v || picker || settingsOpen || infoOpen) return
      if (document.querySelector('.modal-backdrop, .scene-panel')) return // a window over the player has the keys
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
      if (e.key.startsWith('Arrow') && (e.target as HTMLElement)?.closest?.('.bottom, .top, .remote, .watch-next')) return
      switch (e.key) {
        case ' ': case 'k': case 'MediaPlayPause': e.preventDefault(); v.paused ? v.play() : v.pause(); break
        case 'ArrowRight': seek(v.currentTime + offset + 10); break
        case 'ArrowLeft': seek(v.currentTime + offset - 10); break
        case 'ArrowUp': v.volume = Math.min(1, v.volume + 0.1); break
        case 'ArrowDown': v.volume = Math.max(0, v.volume - 0.1); break
        case 'f': toggleFullscreen(root.current); break
        case 'm': v.muted = !v.muted; break
        case 'r': setRemoteOn(!remoteOn); break
        case 'i': setInfoOpen((o) => !o); break
        // Back leaves the player. In TV mode that stops it, as a TV app does; otherwise it carries on in the corner.
        case 'Escape': case 'GoBack': case 'BrowserBack': if (!document.fullscreenElement) { e.preventDefault(); if (getTvMode()) chrome.close(); else chrome.minimize() } break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  // A new request that keeps the place: a change of track shouldn't lose it.
  const ask = (change: Partial<typeof want>) => {
    const v = video.current
    pendingSeek.current = v ? v.currentTime + offset : 0
    setWant((w) => ({ ...w, restartAt: undefined, ...change }))
  }
  // Styled copies of a subtitle sit in the list as its number plus this.
  const STYLED = 10000
  const switchTrack = (kind: 'audio' | 'sub', id: number) => {
    if (kind === 'audio') { ask({ audio: id }); return }
    if (id >= STYLED) { setSub(id - STYLED); ask({ burn: { index: id - STYLED, styled: true } }); return }
    const delivery = info?.subtitles.find((s) => s.index === id)?.delivery
    setSub(id)
    if (id >= 0 && (delivery === 'burn' || !delivery)) { ask({ burn: { index: id, styled: false } }); return }
    // Text the player draws. A stream that had one drawn in goes back to none.
    if (burned) ask({ burn: null })
  }
  const duration = info?.durationSec || item.runtimeSec || 0
  // The same remote as for discs; disc-only keys are greyed out.
  const onRemote = (k: RemoteKey) => {
    const v = video.current
    switch (k) {
      case 'eject': chrome.close(); break
      case 'audio': if ((info?.audio.length || 0) > 1) setPicker('audio'); break
      case 'subtitle': if (info?.subtitles.length) setPicker('subtitle'); break
      case 'rew': case 'prev': if (v) seek(v.currentTime + offset - (k === 'prev' ? 60 : 10)); break
      case 'ff': if (v) seek(v.currentTime + offset + 30); break
      case 'next': if (item.nextId) nav(nextHref); else if (v) seek(v.currentTime + offset + 60); break
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
      {note && !mini && <div className={'net-note' + (note.kind === 'lost' ? ' lost' : '')} role="status" aria-live="polite">{note.text}</div>}
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
            {item.nextId ? <button className="ibtn" onClick={() => nav(nextHref)} title="Next episode"><IconNext /></button> : <span />}
            <button className="ibtn" onClick={chrome.expand} title="Full player"><IconExpand /></button>
            <button className="ibtn" onClick={chrome.close} title="Stop"><IconClose /></button>
          </div>
        </div>
      ) : (
        <>
          <div className="top" onClick={(e) => e.stopPropagation()}>
            <button className="ibtn" onClick={chrome.minimize} title="Back (keeps playing in the mini player)"><IconBack /></button>
            <div className="grow"><h1>{item.title}</h1><div className="sub">{[subtitle || item.year, info && MODE_WORDS[info.decision]].filter(Boolean).join(' · ')}</div></div>
            {info && <button className="ibtn" onClick={() => setInfoOpen(true)} title="How this is playing (I)" aria-label="How this is playing"><IconInfo /></button>}
            {item.nextId && <button className="btn sm" onClick={() => nav(nextHref, { replace: true })} title="Next episode"><IconNext /> <span className="hide-narrow">Next episode</span></button>}
            <button className="ibtn" onClick={chrome.minimize} title="Mini player"><IconMini /></button>
          </div>
          {remoteOn && (prefs.remoteStyle === 'bar'
            ? <RemoteBar onKey={onRemote} enabled={remoteEnabled} disc={false} />
            : <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} label="WittyMedia" />)}
          {inIntro && <button className="btn skip-intro" onClick={(e) => { e.stopPropagation(); seek(item.introEnd!) }}>Skip intro ›</button>}
          {pickIntro && <PickIntroBar item={item} time={now} mode={pickIntro} />}
          {next && (nearEnd || ended) && !nextDismissed && (
            <div className="watch-next" onClick={(e) => e.stopPropagation()}>
              <div className="wn-head">{next.recommended
                ? (next.show ? `Because you watched ${item.showTitle || item.title}` : 'You might also like')
                : upNext != null ? `Next episode in ${upNext}s` : 'Watch next'}</div>
              <div className="wn-body" onClick={() => nav(`/play/${next.item.id}`, { replace: true })}>
                <div className={'wn-thumb' + (next.item.showId != null ? ' wide' : '')}>{next.item.poster && <img src={sized(posterUrl(next.item), fitWidth(240))} alt="" />}</div>
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
          <Controls video={video} duration={duration} offset={offset} onSeek={seek} onFullscreen={() => toggleFullscreen(root.current)}
            chapters={info?.chapters?.map((c) => c.start)} chapterNames={info?.chapters?.map((c) => c.title)}
            audio={info?.audio.map((a) => ({ id: a.index, label: [langName(a.language), a.title, a.codec?.toUpperCase(), a.channels ? a.channels + 'ch' : ''].filter(Boolean).join(' · '), active: a.index === audio }))}
            subtitles={info?.subtitles.map((s) => ({ id: s.index, label: trackLabel(s), active: s.index === sub }))}
            onAudio={(id) => switchTrack('audio', id)} onSubtitle={(id) => switchTrack('sub', id)} onPick={(k) => { if (k !== 'angle') setPicker(k) }}
            showSeek={prefs.seekBar} remoteOn={remoteOn} onRemote={() => setRemoteOn(!remoteOn)} onSettings={() => setSettingsOpen((o) => !o)} />
          {settingsOpen && <ViewSettings prefs={prefs} scope={view.scope} isDisc={false} onClose={() => setSettingsOpen(false)} onChange={(p) => view.update(p, view.scope)} onScope={view.setScope}
            streaming={streamPref} onStreaming={(v) => { setDeviceStreaming(v); setStreamPref(v); ask({ reopen: (want.reopen || 0) + 1 }) }} />}
          {picker && info && (
            <TrackPicker heading={picker === 'audio' ? 'Audio & language' : 'Subtitles'} onClose={() => setPicker(null)}
              onPick={(id) => switchTrack(picker === 'audio' ? 'audio' : 'sub', id)}
              tiles={picker === 'audio'
                ? info.audio.map((a) => ({ id: a.index, title: langName(a.language), sub: a.title, detail: [a.codec?.toUpperCase(), a.channels ? a.channels + ' ch' : ''].filter(Boolean).join(' · '), active: a.index === audio }))
                : [{ id: -1, title: 'Off', active: sub == null || sub < 0 }, ...info.subtitles.flatMap((x) => {
                  const plain = { id: x.index, title: langName(x.language), sub: x.title, detail: x.codec?.toUpperCase(), active: x.index === sub && !burn?.styled }
                  // ASS can carry placement and styling that plain text loses: keeping it means drawing it into the picture.
                  return x.codec === 'ass' || x.codec === 'ssa' ? [plain, { id: x.index + STYLED, title: langName(x.language), sub: 'With its styling', detail: 'Drawn into the picture', active: x.index === sub && !!burn?.styled }] : [plain]
                })]} />
          )}
          {infoOpen && info && <PlayInfoPanel info={info} netLog={netLog} flattened={flat3d} onClose={() => setInfoOpen(false)} onFlat3d={(on) => ask({ flat3d: on })} onVideo={(i) => ask({ video: i })} />}
        </>
      )}
    </div>
  )
}
