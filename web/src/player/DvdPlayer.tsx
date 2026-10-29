import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, langName, mediaUrl, type DvdAction, type Item, type Presentation } from '../api'
import { IconBack, IconDown, IconLeft, IconMenu, IconPlay, IconRight, IconUp } from '../icons'
import { Controls, useVideoState } from './Controls'
import { toggleFullscreen, useIdle, useSource } from './hooks'

type Props = { item: Item; mode: 'menu' | 'title'; ttn?: number; chapter?: number; startAt: number | null }

type Rect = { left: number; top: number; width: number; height: number }

export default function DvdPlayer({ item, mode, ttn, chapter, startAt }: Props) {
  const nav = useNavigate()
  const root = useIdle(3500, [])
  const stage = useRef<HTMLDivElement | null>(null)
  const video = useRef<HTMLVideoElement | null>(null)
  const [sid, setSid] = useState<string | null>(null)
  const [pres, setPres] = useState<Presentation | null>(null)
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const [rect, setRect] = useState<Rect | null>(null)
  const [selected, setSelected] = useState(0)
  const [showButtons, setShowButtons] = useState(true)
  const [busy, setBusy] = useState(false)
  const pendingSeek = useRef<number | null>(startAt ?? null)
  const stillTimer = useRef<any>(null)
  const serialRef = useRef(0)
  const st = useVideoState(video)
  const resumeUsed = useRef(false)

  // Open the session.
  useEffect(() => {
    let cancelled = false
    api.post<{ sessionId: string; presentation: Presentation }>(`/items/${item.id}/dvd`, { mode, ttn, chapter, wide: true })
      .then((r) => { if (cancelled) return; setSid(r.sessionId); apply(r.presentation) })
      .catch((e) => setErr(e.message))
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.id, mode, ttn, chapter])

  useEffect(() => () => { if (sid) api.del(`/dvd/${sid}`).catch(() => {}) }, [sid])

  const apply = (p: Presentation) => {
    clearTimeout(stillTimer.current)
    serialRef.current = p.serial
    setPres(p)
    setSelected(p.menu?.selected || 0)
    setShowButtons(!(p.menu?.buttonsFromSec && p.menu.buttonsFromSec > 0.3))
    if (p.kind === 'title') {
      // Resume: first title shown after opening in "title" mode with saved progress.
      if (!resumeUsed.current && startAt == null && item.progress && !item.progress.watched && item.progress.positionSec > 60 && item.progress.discTitle === p.title?.ttn) {
        pendingSeek.current = item.progress.positionSec
      } else if (pendingSeek.current == null || resumeUsed.current) {
        pendingSeek.current = p.title?.startSec || 0
      }
      resumeUsed.current = true
    }
    if (p.media?.type === 'still') {
      if (p.media.stillSec >= 0) stillTimer.current = setTimeout(() => act({ type: 'ended' }), Math.max(300, p.media.stillSec * 1000))
    }
  }

  const act = useCallback(async (a: DvdAction) => {
    if (!sid) return
    setBusy(true)
    try {
      const r = await api.post<{ presentation: Presentation }>(`/dvd/${sid}/action`, a)
      apply(r.presentation)
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sid])

  // Media source.
  const mediaUrlStr = pres?.media?.type === 'hls' ? pres.media.url! : null
  useSource(video, mediaUrlStr, {
    onReady: () => {
      const v = video.current
      if (!v) return
      if (pres?.kind === 'title' && pendingSeek.current != null && pendingSeek.current > 0.5) v.currentTime = pendingSeek.current
      pendingSeek.current = null
      v.loop = !!pres?.media?.loop
      v.play().catch(() => setNeedClick(true))
    },
    onError: (m) => setErr(m),
  })

  // Clip ended -> tell the navigator (after any still time).
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
    }
    v.addEventListener('ended', onEnded)
    v.addEventListener('timeupdate', onTime)
    return () => { v.removeEventListener('ended', onEnded); v.removeEventListener('timeupdate', onTime) }
  }, [pres, act, showButtons])

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

  // Displayed video rectangle (letterboxed inside the stage) for the menu layer.
  useLayoutEffect(() => {
    const calc = () => {
      const s = stage.current
      if (!s) return
      const W = s.clientWidth, H = s.clientHeight
      const ar = pres?.menu?.aspect === '4:3' ? 4 / 3 : 16 / 9
      let w = W, h = W / ar
      if (h > H) { h = H; w = H * ar }
      setRect({ left: (W - w) / 2, top: (H - h) / 2, width: w, height: h })
    }
    calc()
    const ro = new ResizeObserver(calc)
    if (stage.current) ro.observe(stage.current)
    return () => ro.disconnect()
  }, [pres?.menu?.aspect, pres?.serial])

  // Keyboard: remote-control style.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const v = video.current
      const inMenu = pres?.kind === 'menu' && (pres.menu?.buttons.length || 0) > 0
      switch (e.key) {
        case 'ArrowUp': case 'ArrowDown': case 'ArrowLeft': case 'ArrowRight':
          if (inMenu) { e.preventDefault(); act({ type: 'key', key: e.key.replace('Arrow', '').toLowerCase() }) }
          else if (v && pres?.kind === 'title') { e.preventDefault(); if (e.key === 'ArrowRight') v.currentTime += 10; if (e.key === 'ArrowLeft') v.currentTime -= 10 }
          break
        case 'Enter': if (inMenu) { e.preventDefault(); act({ type: 'press', button: selected }) } break
        case ' ': case 'k': e.preventDefault(); if (v) v.paused ? v.play() : v.pause(); break
        case 'm': act({ type: 'menu', menu: 'root' }); break
        case 't': act({ type: 'menu', menu: 'title' }); break
        case 'f': toggleFullscreen(root.current); break
        case 'Escape': if (!document.fullscreenElement) nav(-1); break
        case 'PageDown': case 'n': act({ type: 'chapter', chapter: 1 }); break
        case 'PageUp': case 'p': act({ type: 'chapter', chapter: -1 }); break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [pres, selected, act, nav, root])

  const chapters = pres?.title?.chapters || []
  const seek = (t: number) => { const v = video.current; if (v) v.currentTime = Math.max(0, t) }
  const seekChapter = (dir: 1 | -1) => {
    const v = video.current
    if (!v || !chapters.length) return
    const cur = chapters.filter((c) => c <= v.currentTime + 1).length - 1
    const next = Math.min(chapters.length - 1, Math.max(0, cur + dir))
    if (dir === -1 && v.currentTime - chapters[cur] > 3) seek(chapters[cur]); else seek(chapters[next])
  }
  const switchStream = (a: DvdAction) => { const v = video.current; pendingSeek.current = v ? v.currentTime : 0; resumeUsed.current = true; act(a) }

  const scale = useMemo(() => {
    if (!rect || !pres?.menu) return null
    return { x: rect.width / pres.menu.width, y: rect.height / pres.menu.height }
  }, [rect, pres])

  const menu = pres?.menu
  const inMenu = pres?.kind === 'menu' && !!menu && menu.buttons.length > 0 && showButtons
  const selBtn = menu?.buttons.find((b) => b.n === selected)
  const audioOpts = pres?.title?.audio.map((a) => ({ id: a.n, label: [langName(a.language), a.codec?.toUpperCase(), a.channels ? a.channels + 'ch' : ''].filter(Boolean).join(' · '), active: a.n === pres.title!.audioStream }))
  const subOpts = pres?.title?.subtitles.map((s) => ({ id: s.n, label: langName(s.language), active: s.n === pres.title!.subtitleStream }))

  return (
    <div className="player" ref={root} tabIndex={0} onDoubleClick={() => toggleFullscreen(root.current)}
      onClick={() => { const v = video.current; if (v && pres?.kind === 'title' && !needClick) v.paused ? v.play() : v.pause() }}>
      <div className="stage" ref={stage}>
        {pres?.media?.type === 'still'
          ? <img className="still" src={mediaUrl(pres.media.url!)} alt="" />
          : <video ref={video} playsInline preload="auto" />}
        {menu && rect && scale && (
          <div className="dvd-layer" style={{ left: rect.left, top: rect.top, width: rect.width, height: rect.height }}>
            {menu.overlayUrl && <img src={mediaUrl(menu.overlayUrl)} alt="" />}
            {inMenu && menu.highlightUrl && selBtn?.lit && <img key={selected} src={mediaUrl(menu.highlightUrl.replace('{n}', String(selected)))} alt="" />}
            {inMenu && menu.buttons.map((b) => (
              <div key={b.n} title={b.cmd}
                className={'hit' + (b.n === selected ? ' selected' : '') + (!menu.hasHighlights || !b.lit ? ' fallback' : '')}
                style={{ left: b.x1 * scale.x, top: b.y1 * scale.y, width: (b.x2 - b.x1) * scale.x, height: (b.y2 - b.y1) * scale.y }}
                onMouseEnter={() => { if (b.n !== selected) { setSelected(b.n); act({ type: 'select', button: b.n }) } }}
                onClick={(e) => { e.stopPropagation(); act({ type: 'press', button: b.n }) }} />
            ))}
          </div>
        )}
      </div>
      {(st.waiting || busy) && !needClick && pres?.media?.type === 'hls' && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && <div className="big-play"><button onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {err && <div className="overlay-msg"><div className="box">{err}<br /><button className="btn" style={{ marginTop: 10 }} onClick={() => nav(-1)}>Back</button></div></div>}
      {pres?.kind === 'stop' && !err && (
        <div className="overlay-msg"><div className="box">
          <div style={{ marginBottom: 10 }}>{pres.scrambled ? 'This disc is CSS-scrambled and cannot be played.' : pres.reason || 'Playback stopped.'}</div>
          <button className="btn" onClick={(e) => { e.stopPropagation(); act({ type: 'menu', menu: 'title' }) }}>Title menu</button>{' '}
          <button className="btn" onClick={(e) => { e.stopPropagation(); act({ type: 'title', ttn: item.disc?.mainTitle || 1 }) }}>Play movie</button>{' '}
          <button className="btn ghost" onClick={() => nav(-1)}>Back</button>
        </div></div>
      )}
      <div className="top">
        <button className="ibtn" onClick={(e) => { e.stopPropagation(); nav(-1) }}><IconBack /></button>
        <div>
          <h2>{item.title}</h2>
          <div className="sub">{pres?.kind === 'title' ? `Title ${pres.title?.ttn} · ${chapters.length} chapters` : pres?.kind === 'menu' ? 'DVD menu' : ''}</div>
        </div>
      </div>
      {pres?.kind === 'title' && (
        <Controls video={video} duration={pres.title?.durationSec || 0} chapters={chapters} onSeek={seek}
          onPrev={() => seekChapter(-1)} onNext={() => seekChapter(1)}
          audio={audioOpts} subtitles={subOpts}
          onAudio={(id) => switchStream({ type: 'audio', stream: id })} onSubtitle={(id) => switchStream({ type: 'subtitle', stream: Math.max(0, id), on: id >= 0 })}
          onFullscreen={() => toggleFullscreen(root.current)}
          extra={<>
            <button className="btn sm" onClick={() => act({ type: 'menu', menu: 'root' })} title="Disc menu (m)"><IconMenu /> Menu</button>
          </>} />
      )}
      {pres?.kind === 'menu' && (
        <div className="bottom" onClick={(e) => e.stopPropagation()}>
          <div className="ctl">
            <div className="dvd-bar">
              <div className="dpad">
                <span /><button onClick={() => act({ type: 'key', key: 'up' })}><IconUp /></button><span />
                <button onClick={() => act({ type: 'key', key: 'left' })}><IconLeft /></button>
                <button className="ok" onClick={() => act({ type: 'press', button: selected })}>OK</button>
                <button onClick={() => act({ type: 'key', key: 'right' })}><IconRight /></button>
                <span /><button onClick={() => act({ type: 'key', key: 'down' })}><IconDown /></button><span />
              </div>
            </div>
            <div className="spacer" />
            <button className="btn sm" onClick={() => act({ type: 'menu', menu: 'title' })}>Title menu</button>
            <button className="btn sm" onClick={() => act({ type: 'menu', menu: 'root' })}>Root menu</button>
            <button className="btn sm" onClick={() => act({ type: 'resume' })}>Resume</button>
            <button className="btn sm primary" onClick={() => act({ type: 'title', ttn: item.disc?.mainTitle || 1 })}><IconPlay /> Play movie</button>
            {pres.media?.type === 'hls' && !pres.media.loop && pres.media.stillSec !== 0 && <button className="btn sm ghost" onClick={() => act({ type: 'ended' })}>Skip</button>}
            {pres.media?.type === 'hls' && (pres.media.loop || pres.media.stillSec === 0) && menu && menu.buttons.length === 0 && <button className="btn sm ghost" onClick={() => act({ type: 'ended' })}>Skip</button>}
            <button className="ibtn" onClick={() => toggleFullscreen(root.current)} title="Fullscreen">⛶</button>
          </div>
        </div>
      )}
    </div>
  )
}
