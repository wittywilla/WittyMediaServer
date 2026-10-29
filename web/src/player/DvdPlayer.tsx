import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, dvdTrackLabel, getPrefs, insertFxOn, mediaUrl, posterUrl, type DvdAction, type DvdButton, type Item, type Presentation, type SubEvent } from '../api'
import { IconBack, IconDisc, IconMenu, IconPlay } from '../icons'
import { Controls, useVideoState } from './Controls'
import { toggleFullscreen, useIdle, useSource } from './hooks'
import InsertDisc from './InsertDisc'
import Remote, { useRemoteToggle, type RemoteKey } from './Remote'

type Props = { item: Item; mode: 'menu' | 'title'; ttn?: number; chapter?: number; startAt: number | null; insert?: boolean }

type Rect = { left: number; top: number; width: number; height: number }

export default function DvdPlayer({ item, mode, ttn, chapter, startAt, insert }: Props) {
  const nav = useNavigate()
  const root = useIdle(3500, [])
  const stage = useRef<HTMLDivElement | null>(null)
  const video = useRef<HTMLVideoElement | null>(null)
  const [sid, setSid] = useState<string | null>(null)
  // Timers and the first presentation run before React re-renders with the
  // new session id, so actions read it from a ref.
  const sidRef = useRef<string | null>(null)
  const [pres, setPres] = useState<Presentation | null>(null)
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const [rect, setRect] = useState<Rect | null>(null)
  const [selected, setSelected] = useState(0)
  const [showButtons, setShowButtons] = useState(true)
  const [busy, setBusy] = useState(false)
  const [stopped, setStopped] = useState(false)
  const [inserting, setInserting] = useState(false)
  const [remoteOn, setRemoteOn] = useRemoteToggle('dvd')
  const pendingSeek = useRef<number | null>(startAt ?? null)
  const stillTimer = useRef<any>(null)
  const navTimer = useRef<any>(null)
  const serialRef = useRef(0)
  const urlRef = useRef<string | undefined>(undefined)
  const forcedDone = useRef(0)
  const holdPlay = useRef(false)
  const st = useVideoState(video)
  const resumeUsed = useRef(false)

  // "Insert DVD" effect (per item, falling back to the server default).
  useEffect(() => {
    if (!insert) return
    let cancelled = false
    getPrefs().then((p) => { if (!cancelled && insertFxOn(item, p)) { holdPlay.current = true; setInserting(true) } })
    return () => { cancelled = true }
  }, [insert, item])
  const insertDone = useCallback(() => {
    setInserting(false)
    holdPlay.current = false
    video.current?.play().catch(() => setNeedClick(true))
  }, [])

  // Open the session.
  useEffect(() => {
    let cancelled = false
    api.post<{ sessionId: string; presentation: Presentation }>(`/items/${item.id}/dvd`, { mode, ttn, chapter, wide: true })
      .then((r) => { if (cancelled) return; sidRef.current = r.sessionId; setSid(r.sessionId); apply(r.presentation) })
      .catch((e) => setErr(e.message))
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.id, mode, ttn, chapter])

  useEffect(() => () => { if (sid) api.del(`/dvd/${sid}`).catch(() => {}) }, [sid])

  // freeze keeps the last picture on screen while the next clip loads, so
  // menu transitions never flash black: the current frame (or still image)
  // becomes the video's poster until the new stream has a frame.
  const freeze = () => {
    const v = video.current
    if (!v) return
    if (urlRef.current && pres?.media?.type === 'still') { v.poster = mediaUrl(urlRef.current); return }
    if (v.readyState < 2 || !v.videoWidth) return
    try {
      const c = document.createElement('canvas')
      c.width = v.videoWidth
      c.height = v.videoHeight
      c.getContext('2d')!.drawImage(v, 0, 0)
      v.poster = c.toDataURL('image/jpeg', 0.85)
    } catch { /* ignore */ }
  }

  const apply = (p: Presentation) => {
    // Same screen, same stream: only the highlight (or a message) changed.
    if (p.serial === serialRef.current && p.media?.url === urlRef.current) {
      setPres(p)
      if (p.menu) setSelected(p.menu.selected || 0)
      return
    }
    if (p.media?.url !== urlRef.current) freeze()
    clearTimeout(stillTimer.current)
    clearTimeout(navTimer.current)
    serialRef.current = p.serial
    urlRef.current = p.media?.url
    forcedDone.current = 0
    setStopped(false)
    setPres(p)
    setSelected(p.menu?.selected || 0)
    setShowButtons(!(p.menu?.buttonsFromSec && p.menu.buttonsFromSec > 0.3))
    if (p.kind === 'title') {
      if (!resumeUsed.current && startAt == null && item.progress && !item.progress.watched && item.progress.positionSec > 60 && item.progress.discTitle === p.title?.ttn) {
        pendingSeek.current = item.progress.positionSec
      } else if (resumeUsed.current || pendingSeek.current == null) {
        pendingSeek.current = p.title?.startSec || 0
      }
      resumeUsed.current = true
    }
    if (p.media?.type === 'still' && p.media.stillSec >= 0) {
      stillTimer.current = setTimeout(() => act({ type: 'ended' }), Math.max(300, p.media.stillSec * 1000))
    }
    if (p.timerSec && p.timerSec > 0) navTimer.current = setTimeout(() => act({ type: 'tick' }), p.timerSec * 1000 + 150)
  }

  const act = useCallback(async (a: DvdAction) => {
    const id = sidRef.current
    if (!id) return
    setBusy(true)
    try {
      const r = await api.post<{ presentation: Presentation }>(`/dvd/${id}/action`, a)
      apply(r.presentation)
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Highlight moves are applied locally at once and synced in the background,
  // so the highlight never lags or jumps back on slow connections.
  const selectQuiet = useCallback((n: number) => {
    const id = sidRef.current
    if (!id) return
    const serial = serialRef.current
    api.post<{ presentation: Presentation }>(`/dvd/${id}/action`, { type: 'select', button: n })
      .then((r) => { if (r.presentation.serial !== serial) apply(r.presentation) })
      .catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const pos = () => video.current?.currentTime || 0
  const titleMenu = () => act({ type: 'menu', menu: 'title', positionSec: pres?.kind === 'title' ? pos() : 0 })
  const rootMenu = () => act({ type: 'menu', menu: 'root', positionSec: pres?.kind === 'title' ? pos() : 0 })

  // Media source.
  const mediaUrlStr = pres?.media?.type === 'hls' ? pres.media.url! : null
  useSource(video, mediaUrlStr, {
    onReady: () => {
      const v = video.current
      if (!v) return
      if (pres?.kind === 'title' && pendingSeek.current != null && pendingSeek.current > 0.5) v.currentTime = pendingSeek.current
      pendingSeek.current = null
      v.loop = !!pres?.media?.loop
      if (!holdPlay.current) v.play().catch(() => setNeedClick(true))
    },
    onError: (m) => setErr(m),
  })

  const menu = pres?.menu
  const buttons = menu?.buttons || []
  const pressButton = useCallback((n: number) => act({ type: 'press', button: n }), [act])

  // Clip ended -> tell the navigator (after any still time); forced actions.
  useEffect(() => {
    const v = video.current
    if (!v) return
    const onEnded = () => {
      const p = pres
      if (!p || p.media?.type !== 'hls' || p.media.loop) return
      const serial = p.serial
      const after = p.media.stillSec
      if (after < 0) return // hold the last frame until the viewer acts
      stillTimer.current = setTimeout(() => { if (serialRef.current === serial) act({ type: 'ended' }) }, after * 1000)
    }
    const onTime = () => {
      if (pres?.menu?.buttonsFromSec && !showButtons && v.currentTime >= pres.menu.buttonsFromSec) setShowButtons(true)
      const m = pres?.menu
      if (m?.forcedAction && m.actionAtSec && v.currentTime >= m.actionAtSec && forcedDone.current !== pres!.serial) {
        forcedDone.current = pres!.serial
        pressButton(m.forcedAction)
      }
    }
    v.addEventListener('ended', onEnded)
    v.addEventListener('timeupdate', onTime)
    return () => { v.removeEventListener('ended', onEnded); v.removeEventListener('timeupdate', onTime) }
  }, [pres, act, showButtons, pressButton])

  // Forced action on still menus (no timeupdate there).
  useEffect(() => {
    const m = pres?.menu
    if (pres?.media?.type !== 'still' || !m?.forcedAction || !m.actionAtSec) return
    const serial = pres.serial
    const t = setTimeout(() => { if (serialRef.current === serial) pressButton(m.forcedAction!) }, m.actionAtSec * 1000)
    return () => clearTimeout(t)
  }, [pres, pressButton])

  // Progress reporting for titles.
  useEffect(() => {
    if (pres?.kind !== 'title') return
    const send = () => {
      const v = video.current
      if (!v || v.currentTime < 5 || !pres.title) return
      api.post(`/items/${item.id}/progress`, { positionSec: v.currentTime, durationSec: pres.title.durationSec, discTitle: pres.title.ttn }).catch(() => {})
    }
    const t = setInterval(send, 10000)
    return () => { clearInterval(t); send() }
  }, [pres, item.id])

  // Displayed video rectangle (letterboxed inside the stage) for the overlays.
  const aspect = pres?.kind === 'title' ? pres.title?.aspect : pres?.menu?.aspect
  useLayoutEffect(() => {
    const calc = () => {
      const s = stage.current
      if (!s) return
      const W = s.clientWidth, H = s.clientHeight
      const ar = aspect === '4:3' ? 4 / 3 : 16 / 9
      let w = W, h = W / ar
      if (h > H) { h = H; w = H * ar }
      setRect({ left: (W - w) / 2, top: (H - h) / 2, width: w, height: h })
    }
    calc()
    const ro = new ResizeObserver(calc)
    if (stage.current) ro.observe(stage.current)
    return () => ro.disconnect()
  }, [aspect, pres?.serial])

  const inMenu = pres?.kind === 'menu' && buttons.length > 0 && showButtons

  // Only show the spinner when waiting is noticeable; quick button presses
  // and segment fetches would otherwise make it flash.
  const [spin, setSpin] = useState(false)
  useEffect(() => {
    if (!(st.waiting || busy)) { setSpin(false); return }
    const t = setTimeout(() => setSpin(true), 400)
    return () => clearTimeout(t)
  }, [st.waiting, busy])

  // Remote-style navigation along the disc's own button links.
  const move = useCallback((dir: 'up' | 'down' | 'left' | 'right') => {
    if (!inMenu) return
    const cur = buttons.find((b) => b.n === selected) || buttons[0]
    const next = cur ? (cur as DvdButton)[dir] : 0
    const target = buttons.find((b) => b.n === next)
    if (!target || target.n === cur.n) return
    if (target.auto) { pressButton(target.n); return }
    setSelected(target.n)
    selectQuiet(target.n)
  }, [inMenu, buttons, selected, pressButton, selectQuiet])

  const chapters = pres?.title?.chapters || []
  const seek = (t: number) => { const v = video.current; if (v) v.currentTime = Math.max(0, t) }
  const seekChapter = (dir: 1 | -1) => {
    const v = video.current
    if (!v || !chapters.length) return
    const cur = chapters.filter((c) => c <= v.currentTime + 1).length - 1
    const next = Math.min(chapters.length - 1, Math.max(0, cur + dir))
    if (dir === -1 && v.currentTime - chapters[cur] > 3) seek(chapters[cur]); else seek(chapters[next])
  }
  // Audio needs a new stream: the server restarts it exactly where we are.
  const switchAudio = (n: number) => { const t = pos(); pendingSeek.current = t; act({ type: 'audio', stream: n, positionSec: t }) }
  // Subtitles are drawn here: switching is instant.
  const switchSub = (n: number) => act({ type: 'subtitle', stream: Math.max(0, n), on: n >= 0 })

  const title = pres?.title
  const cycleAudio = () => {
    if (!title || title.audio.length < 2) return
    const i = title.audio.findIndex((a) => a.n === title.audioStream)
    switchAudio(title.audio[(i + 1) % title.audio.length].n)
  }
  const cycleSub = () => {
    if (!title || !title.subtitles.length) return
    const i = title.subtitles.findIndex((s) => s.n === title.subtitleStream)
    switchSub(i + 1 >= title.subtitles.length ? -1 : title.subtitles[i + 1].n)
  }

  const togglePlay = () => { const v = video.current; if (!v) return; if (stopped) { setStopped(false); v.play().catch(() => {}); return } v.paused ? v.play().catch(() => {}) : v.pause() }

  const onRemote = (k: RemoteKey) => {
    const v = video.current
    switch (k) {
      case 'eject': nav(`/item/${item.id}`); break
      case 'topmenu': titleMenu(); break
      case 'menu': rootMenu(); break
      case 'return': act({ type: 'key', key: 'return', positionSec: pres?.kind === 'title' ? pos() : 0 }); break
      case 'up': case 'down': case 'left': case 'right': move(k); break
      case 'enter': if (inMenu && selected) pressButton(selected); break
      case 'audio': cycleAudio(); break
      case 'subtitle': cycleSub(); break
      case 'prev': pres?.kind === 'title' ? seekChapter(-1) : act({ type: 'chapter', chapter: -1 }); break
      case 'next': pres?.kind === 'title' ? seekChapter(1) : act({ type: 'chapter', chapter: 1 }); break
      case 'rew': if (v) v.currentTime = Math.max(0, v.currentTime - 10); break
      case 'ff': if (v) v.currentTime += 30; break
      case 'play': setStopped(false); v?.play().catch(() => {}); break
      case 'pause': v?.pause(); break
      case 'stop': v?.pause(); setStopped(true); break
      default:
        if (k.startsWith('digit')) {
          const d = Number(k.slice(5))
          if (inMenu) { if (buttons.some((b) => b.n === d)) pressButton(d) } else if (d >= 1 && chapters[d - 1] != null) seek(chapters[d - 1])
        }
    }
  }
  const remoteEnabled = (k: RemoteKey) => {
    switch (k) {
      case 'up': case 'down': case 'left': case 'right': case 'enter': return inMenu
      case 'audio': return (title?.audio.length || 0) > 1
      case 'subtitle': return (title?.subtitles.length || 0) > 0
      case 'angle': return false
      case 'rew': case 'ff': case 'play': case 'pause': case 'stop': return pres?.media?.type === 'hls'
      default: return true
    }
  }

  // Keyboard: remote-control style.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const v = video.current
      switch (e.key) {
        case 'ArrowUp': case 'ArrowDown': case 'ArrowLeft': case 'ArrowRight':
          if (inMenu) { e.preventDefault(); move(e.key.replace('Arrow', '').toLowerCase() as any) }
          else if (v && pres?.kind === 'title') { e.preventDefault(); if (e.key === 'ArrowRight') v.currentTime += 10; if (e.key === 'ArrowLeft') v.currentTime -= 10 }
          break
        case 'Enter': if (inMenu) { e.preventDefault(); pressButton(selected) } break
        case ' ': case 'k': e.preventDefault(); togglePlay(); break
        case 'm': rootMenu(); break
        case 't': titleMenu(); break
        case 'r': setRemoteOn(!remoteOn); break
        case 'f': toggleFullscreen(root.current); break
        case 'Escape': if (!document.fullscreenElement) nav(`/item/${item.id}`); break
        case 'PageDown': case 'n': onRemote('next'); break
        case 'PageUp': case 'p': onRemote('prev'); break
        default: if (/^[0-9]$/.test(e.key)) onRemote(('digit' + e.key) as RemoteKey)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const scale = useMemo(() => {
    if (!rect || !pres?.menu) return null
    return { x: rect.width / pres.menu.width, y: rect.height / pres.menu.height }
  }, [rect, pres])

  const audioOpts = title?.audio.map((a) => ({ id: a.n, label: dvdTrackLabel(a), active: a.n === title.audioStream }))
  const subOpts = title?.subtitles.map((s) => ({ id: s.n, label: dvdTrackLabel(s), active: s.n === title.subtitleStream }))

  return (
    <div className="player" ref={root} tabIndex={0} onDoubleClick={() => toggleFullscreen(root.current)}
      onClick={() => { if (pres?.kind === 'title' && !needClick) togglePlay() }}>
      <div className="stage" ref={stage}>
        {/* The video element stays mounted (stills are drawn over it) so the
            picture never blanks between clips. */}
        <video ref={video} playsInline preload="auto" />
        {pres?.media?.type === 'still' && <img className="still" src={mediaUrl(pres.media.url!)} alt="" />}
        {menu && rect && scale && (
          <div className="dvd-layer" style={{ left: rect.left, top: rect.top, width: rect.width, height: rect.height }}>
            {menu.overlayUrl && <img src={mediaUrl(menu.overlayUrl)} alt="" />}
            {/* Every highlight is loaded up front and only shown/hidden, so moving
                the selection never waits for an image (no flicker). */}
            {inMenu && menu.highlightUrl && buttons.filter((b) => b.lit).map((b) => (
              <img key={`${pres!.serial}-${b.n}`} className={'hl' + (b.n === selected ? ' on' : '')} src={mediaUrl(menu.highlightUrl!.replace('{n}', String(b.n)))} alt="" />
            ))}
            {inMenu && buttons.map((b) => (
              <div key={b.n} title={b.cmd}
                className={'hit' + (b.n === selected ? ' selected' : '') + (!menu.hasHighlights || !b.lit ? ' fallback' : '')}
                style={{ left: b.x1 * scale.x, top: b.y1 * scale.y, width: (b.x2 - b.x1) * scale.x, height: (b.y2 - b.y1) * scale.y }}
                onMouseEnter={() => { if (b.n !== selected) { setSelected(b.n); selectQuiet(b.n) } }}
                onClick={(e) => { e.stopPropagation(); pressButton(b.n) }} />
            ))}
          </div>
        )}
        {pres?.kind === 'title' && title && rect && <SubtitleLayer video={video} title={title} rect={rect} />}
      </div>
      {inserting && <InsertDisc poster={item.poster ? posterUrl(item) : undefined} title={item.title} onDone={insertDone} />}
      {spin && !needClick && !inserting && pres?.media?.type === 'hls' && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && !inserting && <div className="big-play"><button onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {stopped && <div className="overlay-msg"><div className="box">Stopped · press Play to continue</div></div>}
      {err && <div className="overlay-msg"><div className="box">{err}<br /><button className="btn" style={{ marginTop: 10 }} onClick={() => nav(`/item/${item.id}`)}>Back</button></div></div>}
      {pres?.kind === 'stop' && !err && (
        <div className="overlay-msg"><div className="box">
          <div style={{ marginBottom: 10 }}>{pres.scrambled ? 'This disc is CSS-scrambled and cannot be played.' : pres.reason || 'Playback stopped.'}</div>
          <button className="btn" onClick={(e) => { e.stopPropagation(); titleMenu() }}>Title menu</button>{' '}
          <button className="btn" onClick={(e) => { e.stopPropagation(); act({ type: 'title', ttn: item.disc?.mainTitle || 1 }) }}>Play movie</button>{' '}
          <button className="btn ghost" onClick={() => nav(`/item/${item.id}`)}>Back</button>
        </div></div>
      )}
      <div className="top" onClick={(e) => e.stopPropagation()}>
        <button className="ibtn" onClick={() => nav(`/item/${item.id}`)} title="Back"><IconBack /></button>
        <div className="grow">
          <h2>{item.title}</h2>
          <div className="sub">{pres?.kind === 'title' ? `Title ${title?.ttn} · ${chapters.length} chapters` : pres?.kind === 'menu' ? (menu?.inTitle ? 'Interactive feature' : 'DVD menu') : ''}</div>
        </div>
        <button className="btn sm" onClick={titleMenu} title="Disc title menu (t)">Title menu</button>
        <button className="btn sm" onClick={rootMenu} title="Menu (m)"><IconMenu /> Menu</button>
        <button className={'btn sm' + (remoteOn ? ' primary' : '')} onClick={() => setRemoteOn(!remoteOn)} title="Show the DVD remote (r)"><IconDisc /> Remote</button>
      </div>
      {remoteOn && <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemoteOn(false)} />}
      {pres?.kind === 'title' && (
        <Controls video={video} duration={title?.durationSec || 0} chapters={chapters} onSeek={seek}
          onPrev={() => seekChapter(-1)} onNext={() => seekChapter(1)}
          audio={audioOpts} subtitles={subOpts}
          onAudio={switchAudio} onSubtitle={switchSub}
          onFullscreen={() => toggleFullscreen(root.current)} />
      )}
      {pres?.kind === 'menu' && (
        <div className="bottom" onClick={(e) => e.stopPropagation()}>
          <div className="ctl">
            <span className="muted" style={{ fontSize: 13 }}>{inMenu ? 'Arrows / mouse to choose · Enter to select' : menu?.buttonsFromSec ? 'Waiting for the menu…' : ''}</span>
            <div className="spacer" />
            <button className="btn sm" onClick={() => act({ type: 'resume' })}>Resume</button>
            <button className="btn sm primary" onClick={() => act({ type: 'title', ttn: item.disc?.mainTitle || 1 })}><IconPlay /> Play movie</button>
            {pres.media?.type === 'hls' && !pres.media.loop && pres.media.stillSec !== 0 && <button className="btn sm ghost" onClick={() => act({ type: 'ended' })}>Skip</button>}
            {pres.media?.type === 'hls' && (pres.media.loop || pres.media.stillSec === 0) && buttons.length === 0 && <button className="btn sm ghost" onClick={() => act({ type: 'ended' })}>Skip</button>}
            <button className="ibtn" onClick={() => toggleFullscreen(root.current)} title="Fullscreen">⛶</button>
          </div>
        </div>
      )}
    </div>
  )
}

// SubtitleLayer draws the disc's own sub-pictures over the video. Events are
// fetched in two-minute windows around the playhead.
function SubtitleLayer({ video, title, rect }: { video: React.RefObject<HTMLVideoElement | null>; title: NonNullable<Presentation['title']>; rect: Rect }) {
  const stream = title.subtitleStream >= 0 ? title.subtitleStream : title.forcedStream
  const forcedOnly = title.subtitleStream < 0
  const cache = useRef(new Map<string, SubEvent[] | 'loading'>())
  const [active, setActive] = useState<SubEvent[]>([])
  useEffect(() => {
    const v = video.current
    if (!v) return
    const W = 120
    const load = (w: number) => {
      const key = `${stream}:${w}`
      if (cache.current.has(key) || w < 0) return
      cache.current.set(key, 'loading')
      api.get<{ events: SubEvent[] }>(`${title.subtitlesUrl.replace(/^\/api/, '')}&stream=${stream}&from=${w * W}&to=${(w + 1) * W}`)
        .then((r) => { cache.current.set(key, r.events); upd() })
        .catch(() => cache.current.delete(key))
    }
    const upd = () => {
      const t = v.currentTime
      const w = Math.floor(t / W)
      load(w)
      if (t - w * W > W * 0.7) load(w + 1)
      const evs = cache.current.get(`${stream}:${w}`)
      const now = Array.isArray(evs) ? evs.filter((e) => e.start <= t && e.end > t && (!forcedOnly || e.forced)) : []
      setActive((prev) => (prev.length === now.length && prev.every((p, i) => p.url === now[i].url) ? prev : now))
    }
    upd()
    v.addEventListener('timeupdate', upd)
    v.addEventListener('seeked', upd)
    return () => { v.removeEventListener('timeupdate', upd); v.removeEventListener('seeked', upd) }
  }, [video, stream, forcedOnly, title.subtitlesUrl])
  if (!active.length || !title.frameW) return null
  const sx = rect.width / title.frameW, sy = rect.height / title.frameH
  return (
    <div className="sub-layer" style={{ left: rect.left, top: rect.top, width: rect.width, height: rect.height }}>
      {active.map((e) => <img key={e.url} src={mediaUrl(e.url)} alt="" style={{ left: e.x * sx, top: e.y * sy, width: e.w * sx, height: e.h * sy }} />)}
    </div>
  )
}
