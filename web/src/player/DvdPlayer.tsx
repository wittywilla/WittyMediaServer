import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { api, dvdTrackLabel, getPrefs, insertFxOn, langName, mediaUrl, posterUrl, type DiscView, type DvdAction, type DvdButton, type Item, type Presentation, type SubEvent, discName, isBluray } from '../api'
import { setLogSession } from '../errorReport'
import { IconBack, IconClose, IconExpand, IconMenu, IconMini, IconNext, IconPause, IconPlay, IconPrev, IconRemote, IconSettings } from '../icons'
import { usePlayerChrome } from './chrome'
import { Controls, useVideoState } from './Controls'
import { toggleFullscreen, useIdle, useSource } from './hooks'
import InsertDisc from './InsertDisc'
import Remote, { RemoteBar, type RemoteKey } from './Remote'
import TrackPicker, { type Tile } from './TrackPicker'
import ViewSettings from './ViewSettings'
import { discMemory, rememberDisc, useViewPrefs } from './viewPrefs'
import { focusPlayerControls } from '../tv'

type Props = {
  item: Item; mode: 'menu' | 'title'; ttn?: number; chapter?: number; startAt: number | null; insert?: boolean
  menu?: string; menuVts?: number // open this disc menu directly (from the disc page)
}

type Rect = { left: number; top: number; width: number; height: number }
type PickKind = 'audio' | 'subtitle' | 'angle'

// Legacy setting from before the viewing-settings menu: whether the remote was
// shown for discs.
const legacyRemote = () => { try { return localStorage.getItem('wms.remote.dvd') === '1' } catch { return false } }

export default function DvdPlayer({ item, mode, ttn, chapter, startAt, insert, menu: openMenu, menuVts }: Props) {
  const chrome = usePlayerChrome()
  const mini = chrome.mini
  const view = useViewPrefs(item.id, legacyRemote())
  const prefs = view.prefs
  const root = useIdle(prefs.hideAfter * 1000, [mini])
  const stage = useRef<HTMLDivElement | null>(null)
  const video = useRef<HTMLVideoElement | null>(null)
  const [sid, setSid] = useState<string | null>(null)
  // Timers and the first presentation run before React re-renders with the
  // new session id (and presentation), so callbacks read them from refs.
  const sidRef = useRef<string | null>(null)
  const [pres, setPres] = useState<Presentation | null>(null)
  const presRef = useRef<Presentation | null>(null)
  const [err, setErr] = useState('')
  const [needClick, setNeedClick] = useState(false)
  const [rect, setRect] = useState<Rect | null>(null)
  const [selected, setSelected] = useState(0)
  const [showButtons, setShowButtons] = useState(true)
  const [busy, setBusy] = useState(false)
  const [stopped, setStopped] = useState(false)
  const [inserting, setInserting] = useState(false)
  const [insertSound, setInsertSound] = useState(true)
  const [picker, setPicker] = useState<PickKind | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  // A Blu-ray's pop-up menu during the film: shown on the Menu key.
  const [popupOpen, setPopupOpen] = useState(false)
  const [hlFrame, setHlFrame] = useState(0)
  // The previous still image stays up until the next clip has a frame, so
  // pressing a menu button never flashes black.
  const [lingerStill, setLingerStill] = useState<string | null>(null)
  // The last frame of the previous clip, shown over the video until the next
  // clip draws its first picture (see freeze).
  const [frozen, setFrozen] = useState<string | null>(null)
  const [disc, setDisc] = useState<DiscView | null>(null)
  const pendingSeek = useRef<number | null>(startAt ?? null)
  const stillTimer = useRef<any>(null)
  const navTimer = useRef<any>(null)
  const serialRef = useRef(0)
  const urlRef = useRef<string | undefined>(undefined)
  const forcedDone = useRef(0)
  const holdPlay = useRef(false)
  const endedFor = useRef(0) // serial whose clip end was already reported
  const memoryApplied = useRef(false)
  const st = useVideoState(video)
  const resumeUsed = useRef(false)

  // "Insert DVD" effect (per item, falling back to the server default).
  useEffect(() => {
    if (!insert) return
    let cancelled = false
    getPrefs().then((p) => { if (!cancelled && insertFxOn(item)) { holdPlay.current = true; setInsertSound(p.insertSound !== false); setInserting(true) } })
    return () => { cancelled = true }
  }, [insert, item])
  const insertDone = useCallback(() => {
    setInserting(false)
    holdPlay.current = false
    video.current?.play().catch(() => setNeedClick(true))
  }, [])

  // Open the session. Remembered choices go along like a DVD player's
  // memory; the disc's own defaults still apply where nothing was chosen.
  useEffect(() => {
    let cancelled = false
    const mem = discMemory(item.id)
    api.post<{ sessionId: string; presentation: Presentation }>(`/items/${item.id}/dvd`, {
      mode, ttn, chapter, wide: true, menu: mode === 'menu' ? openMenu : undefined, menuVts,
      audioLanguage: mem.audioLanguage, subtitleLanguage: mem.subtitlesOff ? undefined : mem.subtitleLanguage, angle: mem.angle,
    })
      .then((r) => { if (cancelled) return; sidRef.current = r.sessionId; setSid(r.sessionId); apply(r.presentation) })
      .catch((e) => setErr(e.message))
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.id, mode, ttn, chapter, openMenu, menuVts])

  useEffect(() => () => { if (sid) api.del(`/dvd/${sid}`).catch(() => {}) }, [sid])
  useEffect(() => { setLogSession(sid); return () => setLogSession(null) }, [sid])

  // Chapter and title names from the disc page.
  useEffect(() => { api.get<DiscView>(`/items/${item.id}/disc`).then(setDisc).catch(() => {}) }, [item.id])

  // freeze keeps the last picture on screen while the next clip loads. The
  // browser starts the new clip (and its sound) before its decoder has drawn
  // a picture, up to a second later; the frame stays on top until then, so
  // switching clips (menu to menu, a vote to its answer) never flashes black.
  const freeze = () => {
    const v = video.current
    if (!v) return
    const cur = presRef.current
    if (urlRef.current && cur?.media?.type === 'still') { v.poster = mediaUrl(urlRef.current); return }
    if (v.readyState < 2 || !v.videoWidth) return
    try {
      const c = document.createElement('canvas')
      c.width = v.videoWidth
      c.height = v.videoHeight
      c.getContext('2d')!.drawImage(v, 0, 0)
      const frame = c.toDataURL('image/jpeg', 0.85)
      v.poster = frame
      setFrozen(frame)
    } catch { /* ignore */ }
  }

  const apply = (p: Presentation) => {
    const prev = presRef.current
    if (p.pause != null) { const v = video.current; if (v) { if (p.pause) v.pause(); else v.play().catch(() => {}) } }
    // Same screen, same stream: only the highlight (or a message) changed.
    if (p.serial === serialRef.current && p.media?.url === urlRef.current) {
      presRef.current = p
      setPres(p)
      if (p.menu) setSelected(p.menu.selected || 0)
      if (p.menu?.closed) setPopupOpen(false)
      return
    }
    const sameUrl = p.media?.url === urlRef.current
    setPopupOpen(false)
    if (p.media?.url !== urlRef.current) {
      freeze()
      if (prev?.media?.type === 'still' && p.media?.type === 'hls' && prev.media.url) {
        // Preload-free hand-over: keep the old still until the clip shows a frame.
        setLingerStill(prev.media.url)
      }
    }
    clearTimeout(stillTimer.current)
    clearTimeout(navTimer.current)
    serialRef.current = p.serial
    urlRef.current = p.media?.url
    forcedDone.current = 0
    presRef.current = p
    setStopped(false)
    setPres(p)
    setSelected(p.menu?.selected || 0)
    setShowButtons(!(p.menu?.buttonsFromSec && p.menu.buttonsFromSec > 0.3))
    if (p.kind === 'title' && sameUrl && prev?.kind === 'title' && p.title) {
      // The same stream from another place (a pop-up menu chose a chapter): seek.
      const v = video.current
      if (v && Math.abs(v.currentTime - p.title.startSec) > 1.5) v.currentTime = p.title.startSec
    }
    if (p.kind === 'title') {
      if (!resumeUsed.current && startAt == null && chapter == null && item.progress?.resumeSec && item.progress.discTitle === p.title?.ttn) {
        pendingSeek.current = item.progress.resumeSec
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

  // A Blu-ray's button sounds: loaded on first use, then played at once.
  const sounds = useRef(new Map<string, HTMLAudioElement>())
  const playSound = useCallback((id?: number, bdjSound?: boolean) => {
    const tpl = bdjSound && sidRef.current ? `/api/dvd/${sidRef.current}/sound/{id}.wav` : presRef.current?.menu?.soundUrl
    if (!id || !tpl) return
    const url = mediaUrl(tpl.replace('{id}', String(id - 1)))
    let a = sounds.current.get(url)
    if (!a) { a = new Audio(url); sounds.current.set(url, a) }
    a.currentTime = 0
    a.play().catch(() => {})
  }, [])

  // A disc's BD-J applications: their graphics come as pictures whenever they
  // change, and everything else they do arrives with the same message.
  const bdj = pres?.bdj
  const bdjOn = !!bdj
  const [bdjFrame, setBdjFrame] = useState<string | null>(null)
  useEffect(() => {
    if (!bdjOn || !sid) { setBdjFrame(null); return }
    let stop = false, since = -1
    const poll = async () => {
      while (!stop) {
        try {
          const r = await api.get<{ change: number; frameUrl?: string; presentation?: Presentation; sounds?: number[]; pause?: boolean; seek?: number; gone?: boolean }>(`/dvd/${sid}/bdj?since=${since}`)
          if (stop) return
          since = r.change
          if (r.frameUrl) setBdjFrame(r.frameUrl)
          if (r.presentation) apply(r.presentation)
          r.sounds?.forEach((id) => playSound(id + 1, true))
          const v = video.current
          if (v && r.pause != null) { if (r.pause) v.pause(); else v.play().catch(() => {}) }
          if (v && r.seek != null) v.currentTime = r.seek
          if (r.gone) return
        } catch {
          await new Promise((res) => setTimeout(res, 2000))
        }
      }
    }
    poll()
    // The applications keep time by the video, so they hear where it is.
    const tick = setInterval(() => {
      const v = video.current
      if (v && !v.paused) api.post(`/dvd/${sid}/action`, { type: 'tick', positionSec: v.currentTime }).catch(() => {})
    }, 1000)
    return () => { stop = true; clearInterval(tick) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bdjOn, sid])
  const bdjKey = (key: string) => { const id = sidRef.current; if (id) api.post(`/dvd/${id}/action`, { type: 'key', key, positionSec: video.current?.currentTime || 0 }).catch(() => {}) }

  const pos = () => video.current?.currentTime || 0
  const titleMenu = () => act({ type: 'menu', menu: 'title', positionSec: pres?.kind === 'title' ? pos() : 0 })
  const rootMenu = () => act({ type: 'menu', menu: 'root', positionSec: pres?.kind === 'title' ? pos() : 0 })

  // Media source.
  const mediaUrlStr = pres?.media?.type === 'hls' ? pres.media.url! : null
  const hls = useSource(video, mediaUrlStr, {
    logSession: sid,
    startAt: () => (presRef.current?.kind === 'title' ? pendingSeek.current : null),
    onReady: () => {
      const v = video.current
      if (!v) return
      const p = presRef.current
      if (p?.kind === 'title' && pendingSeek.current != null && pendingSeek.current > 0.5 && Math.abs(v.currentTime - pendingSeek.current) > 1) v.currentTime = pendingSeek.current
      pendingSeek.current = null
      v.loop = !!p?.media?.loop
      if (!holdPlay.current) v.play().catch(() => setNeedClick(true))
    },
    onError: (m) => setErr(m),
  })

  // Drop the lingering still and frozen frame once the new clip has drawn a
  // picture: "playing" comes before that, so wait for the first presented
  // frame (with a time limit, in case the browser never reports one).
  useEffect(() => {
    const v = video.current
    if (!v) return
    let limit: ReturnType<typeof setTimeout>
    const clear = () => { clearTimeout(limit); setLingerStill(null); setFrozen(null); v.removeAttribute('poster') }
    const shown = () => {
      clearTimeout(limit)
      limit = setTimeout(clear, 2500)
      if ('requestVideoFrameCallback' in v) v.requestVideoFrameCallback(() => clear())
      else clear()
    }
    v.addEventListener('playing', shown)
    v.addEventListener('seeked', shown)
    return () => { clearTimeout(limit); v.removeEventListener('playing', shown); v.removeEventListener('seeked', shown) }
  }, [])
  useEffect(() => { if (pres?.media?.type !== 'hls') { setLingerStill(null); setFrozen(null) } }, [pres])

  const menu = pres?.menu
  const buttons = menu?.buttons || []
  // Presses carry the playhead: a Blu-ray pop-up menu's buttons read the
  // current chapter and time from the player's registers.
  const pressButton = useCallback((n: number) => {
    playSound(presRef.current?.menu?.buttons.find((b) => b.n === n)?.soundPress)
    return act({ type: 'press', button: n, positionSec: presRef.current?.kind === 'title' ? video.current?.currentTime || 0 : 0 })
  }, [act, playSound])

  // Clip ended -> tell the navigator (after any still time). Also used by the
  // stall watchdog below, so it reports each clip at most once.
  const clipEnded = useCallback(() => {
    const p = presRef.current
    if (!p || p.media?.type !== 'hls' || p.media.loop || endedFor.current === p.serial) return
    const after = p.media.stillSec
    if (after < 0) return // hold the last frame until the viewer acts
    endedFor.current = p.serial
    const serial = p.serial
    stillTimer.current = setTimeout(() => { if (serialRef.current === serial) act({ type: 'ended' }) }, after * 1000)
  }, [act])

  useEffect(() => {
    const v = video.current
    if (!v) return
    const onTime = () => {
      const m = pres?.menu
      // The disc's buttons work from buttonsFromSec until buttonsUntilSec
      // (when its highlight or button selection ends); a looping clip
      // brings them back when it starts over.
      const t = v.currentTime
      const on = (!m?.buttonsFromSec || t >= m.buttonsFromSec) && (!m?.buttonsUntilSec || t < m.buttonsUntilSec)
      if (on !== showButtons && (on || m?.buttonsUntilSec)) setShowButtons(on)
      if (m?.forcedAction && m.actionAtSec && v.currentTime >= m.actionAtSec && forcedDone.current !== pres!.serial) {
        forcedDone.current = pres!.serial
        pressButton(m.forcedAction)
      }
    }
    v.addEventListener('ended', clipEnded)
    v.addEventListener('timeupdate', onTime)
    return () => { v.removeEventListener('ended', clipEnded); v.removeEventListener('timeupdate', onTime) }
  }, [pres, showButtons, pressButton, clipEnded])

  // Stall watchdog. Browsers do not always fire "ended" for HLS streams whose
  // last segment ends a little short of the announced length (the warning
  // screens at the start of many discs), which used to leave the player
  // stuck. Playback that stops moving near the end counts as ended; a stall
  // in the middle gets nudged past the gap.
  useEffect(() => {
    if (pres?.media?.type !== 'hls' || pres.media.loop) return
    let last = -1, still = 0
    const t = setInterval(() => {
      const v = video.current
      if (!v || v.paused || v.ended || holdPlay.current) { still = 0; last = -1; return }
      if (Math.abs(v.currentTime - last) > 0.05) { last = v.currentTime; still = 0; return }
      still++
      const end = pres.kind === 'title' ? (pres.title?.durationSec || v.duration) : (isFinite(v.duration) && v.duration > 0 ? v.duration : pres.media!.durationSec)
      if (still >= 3 && end > 0 && v.currentTime >= end - 2.5) { still = 0; clipEnded() }
      else if (still >= 8) {
        still = 0
        try { v.currentTime = v.currentTime + 0.5; hls.current?.startLoad() } catch { /* ignore */ }
      }
    }, 1000)
    return () => clearInterval(t)
  }, [pres, clipEnded, hls])

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

  // Displayed video rectangle inside the stage, for the overlays. "Fill"
  // zooms features to cover the screen; menus always fit so no button is cut.
  const aspect = pres?.kind === 'title' ? pres.title?.aspect : pres?.menu?.aspect
  const fill = prefs.picture === 'fill' && pres?.kind === 'title'
  useLayoutEffect(() => {
    const calc = () => {
      const s = stage.current
      if (!s) return
      const W = s.clientWidth, H = s.clientHeight
      const ar = aspect === '4:3' ? 4 / 3 : 16 / 9
      let w = W, h = W / ar
      if (fill ? h < H : h > H) { h = H; w = H * ar }
      setRect({ left: (W - w) / 2, top: (H - h) / 2, width: w, height: h })
    }
    calc()
    const ro = new ResizeObserver(calc)
    if (stage.current) ro.observe(stage.current)
    return () => ro.disconnect()
  }, [aspect, pres?.serial, fill])

  const hasPopup = pres?.kind === 'title' && !!menu?.popup && buttons.length > 0
  const inMenu = (pres?.kind === 'menu' && buttons.length > 0 && showButtons) || (hasPopup && popupOpen)

  // Animated highlights (Blu-ray menus): cycle the selected button's frames.
  const selectedBtn = buttons.find((b) => b.n === selected)
  const hlFrames = inMenu && selectedBtn?.frames && selectedBtn.frames > 1 ? selectedBtn.frames : 0
  useEffect(() => {
    setHlFrame(0)
    if (!hlFrames) return
    const t = setInterval(() => setHlFrame((f) => (f + 1) % hlFrames), 1000 / Math.max(1, selectedBtn?.fps || 12))
    return () => clearInterval(t)
  }, [hlFrames, selectedBtn?.fps, selected, pres?.serial])
  const frameUrl = (u: string, f: number) => u + (u.includes('?') ? '&' : '?') + 'f=' + f

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
    if (!target || target.n === cur.n) {
      // Past the disc's own buttons: on to the player's (a remote has no other
      // way to reach Play movie or Resume). Up or down from there comes back.
      focusPlayerControls(dir === 'up' || dir === 'left' ? 'top' : 'first')
      return
    }
    if (target.auto) { pressButton(target.n); return }
    playSound(target.soundSelect)
    setSelected(target.n)
    selectQuiet(target.n)
  }, [inMenu, buttons, selected, pressButton, selectQuiet, playSound])

  // A pop-up closes by itself when the disc says no key has come for long enough.
  useEffect(() => {
    const secs = menu?.userTimeoutSec
    if (!popupOpen || !secs) return
    const t = setTimeout(() => setPopupOpen(false), secs * 1000)
    return () => clearTimeout(t)
  }, [popupOpen, menu?.userTimeoutSec, selected])

  // A menu page's opening transition, picture by picture.
  const [effect, setEffect] = useState<string | null>(null)
  const effectFrames = pres?.kind !== 'title' || popupOpen ? menu?.inEffect : undefined
  useEffect(() => {
    if (!effectFrames?.length) { setEffect(null); return }
    let i = 0, t: ReturnType<typeof setTimeout>
    const step = () => {
      if (i >= effectFrames.length) { setEffect(null); return }
      setEffect(mediaUrl(effectFrames[i].url))
      t = setTimeout(step, Math.max(16, effectFrames[i].sec * 1000))
      i++
    }
    step()
    return () => clearTimeout(t)
  }, [effectFrames])

  useEffect(() => {
    const layer = stage.current?.querySelector('.dvd-layer')
    if (layer?.contains(document.activeElement)) layer.querySelector<HTMLButtonElement>('.hit.selected')?.focus({ preventScroll: true })
  }, [selected])

  const title = pres?.title
  const chapters = title?.chapters || []
  const discTitle = title && disc ? [disc.main, ...disc.extras].find((t) => t?.ttn === title.ttn) : undefined
  const chapterNames = discTitle?.chapters.map((c) => c.label)
  const isFeature = !!title && (disc?.main?.ttn ?? item.disc?.mainTitle) === title.ttn
  const titleName = discTitle?.label || (isFeature ? 'Feature presentation' : title ? `Title ${title.ttn}` : '')

  // What the disc forbids here, as a disc player would (viewers can opt out).
  const locked = (what: string) => prefs.discLocks && !!title?.locks?.includes(what)
  const seek = (t: number) => { const v = video.current; if (v && !locked('seek') && !locked('chapter')) v.currentTime = Math.max(0, t) }
  // Next on the last scene (or in a one-scene clip such as a warning or
  // trailer) moves on to what the disc plays next, like a disc player,
  // instead of starting the scene again.
  const seekChapter = (dir: 1 | -1) => {
    const v = video.current
    if (!v || locked('skip')) return
    // A slideshow's first picture and earlier: the disc's previous item.
    if (dir === -1 && chapters.length <= 1 && v.currentTime < 3) { act({ type: 'key', key: 'prev' }); return }
    const cur = Math.max(0, chapters.filter((c) => c <= v.currentTime + 1).length - 1)
    if (dir === 1 && cur >= chapters.length - 1) { act({ type: 'ended' }); return }
    if (!chapters.length) return
    const next = Math.min(chapters.length - 1, Math.max(0, cur + dir))
    if (dir === -1 && v.currentTime - chapters[cur] > 3) seek(chapters[cur]); else seek(chapters[next])
  }
  // Audio and angles need a new stream: the server restarts it exactly where we are.
  const switchAudio = (n: number) => {
    const t = pos(); pendingSeek.current = t
    const lang = title?.audio.find((a) => a.n === n)?.language
    if (lang) rememberDisc(item.id, { audioLanguage: lang })
    act({ type: 'audio', stream: n, positionSec: t })
  }
  const switchAngle = (n: number) => {
    const t = pos(); pendingSeek.current = t
    rememberDisc(item.id, { angle: n })
    act({ type: 'angle', stream: n, positionSec: t })
  }
  // Subtitles are drawn here: switching is instant.
  const switchSub = (n: number) => {
    const lang = title?.subtitles.find((s) => s.n === n)?.language
    rememberDisc(item.id, n < 0 ? { subtitlesOff: true } : { subtitlesOff: false, subtitleLanguage: lang })
    act({ type: 'subtitle', stream: Math.max(0, n), on: n >= 0 })
  }

  // Memory: the first time the feature starts in this session, put back the
  // tracks the viewer picked last time (if the disc offers them).
  useEffect(() => {
    if (!title || memoryApplied.current) return
    memoryApplied.current = true
    const mem = discMemory(item.id)
    if (mem.subtitlesOff && title.subtitleStream >= 0) act({ type: 'subtitle', stream: 0, on: false })
    else if (!mem.subtitlesOff && mem.subtitleLanguage) {
      const s = title.subtitles.find((x) => x.language === mem.subtitleLanguage && !x.purpose)
      if (s && s.n !== title.subtitleStream) act({ type: 'subtitle', stream: s.n, on: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [title])

  const togglePlay = () => { const v = video.current; if (!v) return; if (stopped) { setStopped(false); v.play().catch(() => {}); return } if (v.paused) v.play().catch(() => {}); else if (!locked('pause')) v.pause() }
  const setRemote = (on: boolean) => view.update({ remote: on }, view.scope)
  const back = () => chrome.minimize()

  const onRemote = (k: RemoteKey) => {
    const v = video.current
    if (bdjOn && (k === 'up' || k === 'down' || k === 'left' || k === 'right' || k === 'enter' || k === 'menu' || k.startsWith('digit'))) { bdjKey(k); return }
    switch (k) {
      case 'eject': chrome.close(); break
      case 'topmenu': titleMenu(); break
      case 'menu': if (hasPopup) setPopupOpen((o) => !o); else rootMenu(); break
      case 'return': if (popupOpen) setPopupOpen(false); else act({ type: 'key', key: 'return', positionSec: pres?.kind === 'title' ? pos() : 0 }); break
      case 'up': case 'down': case 'left': case 'right': move(k); break
      case 'enter': if (inMenu && selected) pressButton(selected); break
      case 'audio': if ((title?.audio.length || 0) > 1) setPicker('audio'); break
      case 'subtitle': if (title?.subtitles.length) setPicker('subtitle'); break
      case 'angle': if ((title?.angles || 1) > 1) setPicker('angle'); break
      case 'prev': pres?.kind === 'title' ? seekChapter(-1) : act({ type: 'chapter', chapter: -1 }); break
      case 'next': pres?.kind === 'title' ? seekChapter(1) : act({ type: 'chapter', chapter: 1 }); break
      case 'rew': if (v && !locked('seek')) v.currentTime = Math.max(0, v.currentTime - 10); break
      case 'ff': if (v && !locked('seek')) v.currentTime += 30; break
      case 'play': setStopped(false); v?.play().catch(() => {}); break
      case 'pause': v?.pause(); break
      case 'stop': v?.pause(); setStopped(true); break
      default:
        if (k.startsWith('digit')) {
          const d = Number(k.slice(5))
          if (inMenu) { const b = buttons.find((x) => (x.num ?? x.n) === d); if (b) pressButton(b.n) } else if (d >= 1 && chapters[d - 1] != null) seek(chapters[d - 1])
        }
    }
  }
  const remoteEnabled = (k: RemoteKey) => {
    switch (k) {
      case 'up': case 'down': case 'left': case 'right': case 'enter': return inMenu
      case 'audio': return (title?.audio.length || 0) > 1
      case 'subtitle': return (title?.subtitles.length || 0) > 0
      case 'angle': return (title?.angles || 1) > 1
      case 'rew': case 'ff': case 'play': case 'pause': case 'stop': return pres?.media?.type === 'hls'
      default: return true
    }
  }

  // The picture and the disc's menus have the arrow keys until the viewer goes
  // to the controls.
  useEffect(() => { if (!mini) root.current?.focus({ preventScroll: true }) }, [mini, root])

  // Keyboard and TV remotes. Not while minimized: the app owns the keys then.
  useEffect(() => {
    if (mini) return
    const onKey = (e: KeyboardEvent) => {
      if (picker || settingsOpen) return
      if (document.querySelector('.modal-backdrop, .scene-panel')) return // a window over the player has the keys
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return
      const v = video.current
      if (bdjOn) {
        // The disc's applications have the remote; F1 to F4 are its colour keys.
        const key = ({ ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right', Enter: 'enter', m: 'menu', ContextMenu: 'menu', F1: 'red', F2: 'green', F3: 'yellow', F4: 'blue' } as Record<string, string>)[e.key]
        if (key && !(e.target as HTMLElement)?.closest?.('.bottom, .top, .remote, .remote-bar, .view-settings')) { e.preventDefault(); bdjKey(key); return }
        if (/^[0-9]$/.test(e.key)) { bdjKey('digit' + e.key); return }
      }
      switch (e.key) {
        case 'ArrowUp': case 'ArrowDown': case 'ArrowLeft': case 'ArrowRight':
          // Focus on the player's own controls: arrows move between them (see tv.ts).
          if ((e.target as HTMLElement)?.closest?.('.bottom, .top, .remote, .remote-bar, .view-settings')) break
          if (inMenu) { e.preventDefault(); move(e.key.replace('Arrow', '').toLowerCase() as any) }
          else if (v && pres?.kind === 'title' && !(e.target as HTMLElement)?.closest?.('.bottom, .top, .remote')) {
          e.preventDefault(); if (!locked('seek')) { if (e.key === 'ArrowRight') v.currentTime += 10; if (e.key === 'ArrowLeft') v.currentTime -= 10 }
          if (e.key === 'ArrowDown') focusPlayerControls()
          if (e.key === 'ArrowUp') focusPlayerControls('top')
          }
          // A menu still loading, a warning screen: nothing to choose yet.
          else if (e.key === 'ArrowDown') focusPlayerControls()
          else if (e.key === 'ArrowUp') focusPlayerControls('top')
          break
        case 'Enter': if (inMenu) { e.preventDefault(); pressButton(selected) } break
        case ' ': case 'k': case 'MediaPlayPause': e.preventDefault(); togglePlay(); break
        case 'm': case 'ContextMenu': if (hasPopup) setPopupOpen((o) => !o); else rootMenu(); break
        case 't': titleMenu(); break
        case 'r': setRemote(!prefs.remote); break
        case 'f': toggleFullscreen(root.current); break
        case 'a': onRemote('audio'); break
        case 's': onRemote('subtitle'); break
        case 'Escape': case 'GoBack': case 'BrowserBack': if (popupOpen) { e.preventDefault(); setPopupOpen(false) } else if (!document.fullscreenElement) { e.preventDefault(); back() } break
        case 'PageDown': case 'n': case 'MediaTrackNext': onRemote('next'); break
        case 'PageUp': case 'p': case 'MediaTrackPrevious': onRemote('prev'); break
        case 'MediaStop': onRemote('stop'); break
        case 'MediaRewind': onRemote('rew'); break
        case 'MediaFastForward': onRemote('ff'); break
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

  // Tiles for the pickers.
  const tiles = (k: PickKind): Tile[] => {
    if (!title) return []
    if (k === 'audio') return title.audio.map((a) => ({
      id: a.n, title: langName(a.language), sub: a.purpose || undefined,
      detail: [a.codec?.toUpperCase(), a.channels ? (a.channels > 2 ? `${a.channels - 1}.1` : a.channels === 2 ? 'Stereo' : 'Mono') : ''].filter(Boolean).join(' · '),
      active: a.n === title.audioStream,
    }))
    if (k === 'subtitle') return [
      { id: -1, title: 'Off', sub: title.forcedStream >= 0 ? (prefs.forcedSubs ? 'Forced captions still show (turn them off in viewing settings)' : 'Forced captions are off too') : undefined, active: title.subtitleStream < 0 },
      ...title.subtitles.map((s) => ({ id: s.n, title: langName(s.language), sub: s.purpose || undefined, active: s.n === title.subtitleStream })),
    ]
    return Array.from({ length: title.angles || 1 }, (_, i) => ({
      id: i + 1, title: `Version ${i + 1}`, sub: `Camera angle ${i + 1}`, active: (title.angle || 1) === i + 1,
    }))
  }
  const pickerText: Record<PickKind, { heading: string; hint?: string }> = {
    audio: { heading: 'Audio & language', hint: 'Changes what you hear. Playback continues from the same moment.' },
    subtitle: { heading: 'Subtitles' },
    angle: {
      heading: 'Video version',
      hint: 'Angles switch the picture, not the sound. Discs use them for alternate camera views or for on-screen text in other languages (for example English or Dutch credits). Your choice is remembered for this disc.',
    },
  }
  const pick = (k: PickKind, id: number) => (k === 'audio' ? switchAudio(id) : k === 'subtitle' ? switchSub(id) : switchAngle(id))

  const audioOpts = title?.audio.map((a) => ({ id: a.n, label: dvdTrackLabel(a), active: a.n === title.audioStream }))
  const subOpts = title?.subtitles.map((s) => ({ id: s.n, label: dvdTrackLabel(s), active: s.n === title.subtitleStream }))
  const objectFit = fill ? 'cover' : 'contain'

  return (
    <div className={'player dvd' + (mini ? ' mini' : '') + (pres?.kind !== 'title' ? ' menu-mode' : '')} ref={root} tabIndex={-1}
      onDoubleClick={() => (mini ? chrome.expand() : toggleFullscreen(root.current))}
      onClick={() => { if (pres?.kind === 'title' && !needClick) togglePlay() }}>
      <div className={'stage' + (prefs.stereo !== 'off' ? ` stereo-${prefs.stereo}` : '')} ref={stage}>
        {/* The video element stays mounted (stills are drawn over it) so the
            picture never blanks between clips. */}
        {/* A BD-J application can shrink the video into a corner of its own menu. */}
        <video ref={video} playsInline preload="auto" style={{ objectFit, opacity: prefs.stereo !== 'off' ? 0 : 1, ...(bdj?.video?.length === 4 && rect ? { position: 'absolute', left: rect.left + bdj.video[0] / 1920 * rect.width, top: rect.top + bdj.video[1] / 1080 * rect.height, width: bdj.video[2] / 1920 * rect.width, height: bdj.video[3] / 1080 * rect.height } : {}) }} />
        {prefs.stereo !== 'off' && rect && <StereoLayer video={video} rect={rect} mode={prefs.stereo} out={prefs.stereoOut} />}
        {pres?.media?.type === 'still' && <img className="still" src={mediaUrl(pres.media.url!)} alt="" />}
        {lingerStill && <img className="still" src={mediaUrl(lingerStill)} alt="" />}
        {frozen && !lingerStill && pres?.media?.type === 'hls' && <img className="still frozen" src={frozen} alt="" style={{ objectFit }} />}
        {menu && rect && scale && (pres?.kind !== 'title' || (hasPopup && popupOpen)) && (
          <div className="dvd-layer" style={{ left: rect.left, top: rect.top, width: rect.width, height: rect.height }}>
            {effect ? <img src={effect} alt="" /> : menu.overlayUrl && <img src={mediaUrl(menu.overlayUrl)} alt="" />}
            {/* Every highlight is loaded up front and only shown/hidden, so moving
                the selection never waits for an image (no flicker). */}
            {inMenu && menu.highlightUrl && buttons.filter((b) => b.lit).map((b) => (
              b.frames && b.frames > 1 && b.n === selected
                ? Array.from({ length: b.frames }, (_, f) => (
                  <img key={`${menu.highlightUrl}-${b.n}-${f}`} className={'hl' + (f === hlFrame ? ' on' : '')} src={frameUrl(mediaUrl(menu.highlightUrl!.replace('{n}', String(b.n))), f)} alt="" />
                ))
                : <img key={`${menu.highlightUrl}-${b.n}`} className={'hl' + (b.n === selected ? ' on' : '')} src={mediaUrl(menu.highlightUrl!.replace('{n}', String(b.n)))} alt="" />
            ))}
            {/* Real buttons (not just hit areas) so screen readers and keyboards
                can reach every menu option; the disc's own artwork is the label. */}
            {inMenu && buttons.map((b, i) => (
              <button key={b.n} type="button" tabIndex={b.n === selected ? 0 : -1} aria-label={`Menu option ${i + 1} of ${buttons.length}`}
                className={'hit' + (b.n === selected ? ' selected' : '') + (!menu.hasHighlights || !b.lit ? ' fallback' : '')}
                style={{ left: b.x1 * scale.x, top: b.y1 * scale.y, width: (b.x2 - b.x1) * scale.x, height: (b.y2 - b.y1) * scale.y }}
                onMouseEnter={() => { if (b.n !== selected) { setSelected(b.n); selectQuiet(b.n) } }}
                onClick={(e) => { e.stopPropagation(); pressButton(b.n) }} />
            ))}
          </div>
        )}
        {bdjOn && bdjFrame && rect && <img className="bdj-layer" src={mediaUrl(bdjFrame)} alt="" style={{ position: 'absolute', left: rect.left, top: rect.top, width: rect.width, height: rect.height, pointerEvents: 'none' }} />}
        {pres?.kind === 'title' && title && rect && <SubtitleLayer video={video} title={title} rect={rect} size={prefs.subtitleSize} forced={prefs.forcedSubs}
          stageH={stage.current?.clientHeight || 0} clear={prefs.remote && prefs.remoteStyle === 'bar' ? 150 : 104} />}
      </div>
      {/* Announces what is on screen to screen readers. */}
      <div className="sr-only" role="status" aria-live="polite">
        {pres?.kind === 'title' ? `Playing ${titleName}` : pres?.kind === 'menu' ? (inMenu ? `Disc menu, ${buttons.length} options. Use the arrow keys and Enter.` : 'Disc menu loading') : ''}
      </div>
      {inserting && !mini && <InsertDisc poster={item.poster ? posterUrl(item) : undefined} title={item.title} label={discName(item)} onDone={insertDone} sound={insertSound} />}
      {spin && !needClick && !inserting && pres?.media?.type === 'hls' && <div className="center-spinner"><div className="spinner" /></div>}
      {needClick && !inserting && <div className="big-play"><button onClick={(e) => { e.stopPropagation(); setNeedClick(false); video.current?.play() }}><IconPlay /></button></div>}
      {stopped && !mini && <div className="overlay-msg"><div className="box">Stopped · press Play to continue</div></div>}
      {err && <div className="overlay-msg"><div className="box">{err}<br /><button className="btn" style={{ marginTop: 10 }} onClick={() => chrome.close()}>Back</button></div></div>}
      {pres?.kind === 'stop' && !err && !mini && (
        <div className="overlay-msg"><div className="box">
          <div style={{ marginBottom: 10 }}>{pres.scrambled && !isBluray(item) ? 'This disc is CSS-scrambled and cannot be played.' : pres.reason || 'Playback stopped.'}</div>
          <button className="btn" onClick={(e) => { e.stopPropagation(); titleMenu() }}>Title menu</button>{' '}
          <button className="btn" onClick={(e) => { e.stopPropagation(); act({ type: 'title', ttn: item.disc?.mainTitle || 1 }) }}>Play movie</button>{' '}
          <button className="btn ghost" onClick={() => chrome.close()}>Back</button>
        </div></div>
      )}

      {mini ? (
        <MiniBar item={item} subtitle={pres?.kind === 'title' ? titleName : `${discName(item)} menu`} playing={st.playing} isTitle={pres?.kind === 'title'}
          onPlay={togglePlay} onPrev={() => onRemote('prev')} onNext={() => onRemote('next')}
          onMenu={rootMenu} onTitleMenu={titleMenu} onExpand={chrome.expand} onClose={chrome.close} />
      ) : (
        <>
          <div className="top" onClick={(e) => e.stopPropagation()}>
            <button className="ibtn" onClick={back} title="Back (playback continues in the mini player)"><IconBack /></button>
            <div className="grow">
              <h1>{item.title}</h1>
              <div className="sub">{pres?.kind === 'title' ? `${titleName} · ${chapters.length} scenes${hasPopup ? (popupOpen ? ' · pop-up menu (Esc closes)' : ' · Menu key: pop-up menu') : ''}` : pres?.kind === 'menu' ? (menu?.inTitle && buttons.length ? 'Interactive feature' : menu?.inTitle ? '' : `${discName(item)} menu`) : ''}</div>
            </div>
            <button className="btn sm hide-narrow" onClick={titleMenu} title="Disc title menu (t)">Title menu</button>
            <button className="btn sm" onClick={rootMenu} title="Menu (m)"><IconMenu /> Menu</button>
            <button className="ibtn" onClick={back} title="Mini player"><IconMini /></button>
          </div>
          {prefs.remote && (prefs.remoteStyle === 'bar'
            ? <RemoteBar onKey={onRemote} enabled={remoteEnabled} />
            : <Remote onKey={onRemote} enabled={remoteEnabled} onClose={() => setRemote(false)} label={isBluray(item) ? 'WittyBluray' : 'WittyDVD'} />)}
          {pres?.kind === 'title' && (
            <Controls disc video={video} duration={title?.durationSec || 0} chapters={chapters} chapterNames={chapterNames} onSeek={seek}
              chapterThumb={title ? (i) => mediaUrl(`/api/items/${item.id}/disc/thumb/${title.ttn}/${discTitle?.chapters[i]?.n ?? i + 1}`) : undefined}
              onPrev={() => seekChapter(-1)} onNext={() => seekChapter(1)}
              audio={audioOpts} subtitles={subOpts} onAudio={switchAudio} onSubtitle={switchSub}
              onPick={setPicker} angles={title?.angles || 1}
              showSeek={prefs.seekBar && !locked('seek')} showChapterMarks={prefs.chapterMarks}
              remoteOn={prefs.remote} onRemote={() => setRemote(!prefs.remote)} onSettings={() => setSettingsOpen((o) => !o)}
              onFullscreen={() => toggleFullscreen(root.current)} />
          )}
          {pres?.kind !== 'title' && (
            <div className="bottom" onClick={(e) => e.stopPropagation()}>
              <div className="ctl">
                <span className="muted hide-narrow" style={{ fontSize: 13 }}>{inMenu ? 'Arrows / mouse to choose · Enter to select' : menu?.buttonsFromSec ? 'Waiting for the menu…' : ''}</span>
                <div className="spacer" />
                <button className="btn sm" onClick={() => act({ type: 'resume' })}>Resume</button>
                <button className="btn sm primary" onClick={() => act({ type: 'title', ttn: item.disc?.mainTitle || 1 })}><IconPlay /> Play movie</button>
                {!bdjOn && pres?.media?.type === 'hls' && !pres.media.loop && pres.media.stillSec !== 0 && <button className="btn sm ghost" onClick={() => act({ type: 'ended' })}>Skip</button>}
                {!bdjOn && pres?.media?.type === 'hls' && (pres.media.loop || pres.media.stillSec === 0) && buttons.length === 0 && <button className="btn sm ghost" onClick={() => act({ type: 'ended' })}>Skip</button>}
                {/* A picture held until the viewer acts, with nothing to press: a player's Play key goes on. */}
                {pres?.media?.type === 'still' && pres.media.stillSec < 0 && buttons.length === 0 && <button className="btn sm ghost" onClick={() => act({ type: 'ended' })}>Continue</button>}
                <button className={'btn sm remote-toggle' + (prefs.remote ? ' primary' : '')} onClick={() => setRemote(!prefs.remote)} aria-pressed={prefs.remote} title="Show or hide the on-screen remote (r)"><IconRemote /> <span>Remote</span></button>
                <button className="ibtn" onClick={() => setSettingsOpen((o) => !o)} title="Viewing settings"><IconSettings /></button>
                <button className="ibtn" onClick={() => toggleFullscreen(root.current)} title="Fullscreen (f)">⛶</button>
              </div>
            </div>
          )}
          {settingsOpen && (
            <ViewSettings prefs={prefs} scope={view.scope} isDisc onClose={() => setSettingsOpen(false)}
              onChange={(p) => view.update(p, view.scope)} onScope={view.setScope} />
          )}
          {picker && title && (
            <TrackPicker heading={pickerText[picker].heading} hint={pickerText[picker].hint} tiles={tiles(picker)}
              onPick={(id) => pick(picker, id)} onClose={() => setPicker(null)} />
          )}
        </>
      )}
    </div>
  )
}

// MiniBar is the control strip of the minimized player: the essentials of a
// disc remote plus restore and stop.
function MiniBar({ item, subtitle, playing, isTitle, onPlay, onPrev, onNext, onMenu, onTitleMenu, onExpand, onClose }: {
  item: Item; subtitle: string; playing: boolean; isTitle: boolean
  onPlay: () => void; onPrev: () => void; onNext: () => void; onMenu: () => void; onTitleMenu: () => void; onExpand: () => void; onClose: () => void
}) {
  const stop = (f: () => void) => (e: React.MouseEvent) => { e.stopPropagation(); f() }
  return (
    <div className="mini-bar" onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}>
      <div className="mini-info" onClick={onExpand}>
        <div className="mini-title">{item.title}</div>
        <div className="mini-sub">{subtitle}</div>
      </div>
      <div className="mini-btns">
        <button className="ibtn" onClick={stop(onPrev)} title="Previous chapter"><IconPrev /></button>
        <button className="ibtn big" onClick={stop(onPlay)} title={isTitle ? 'Play/Pause' : 'Play/Pause menu video'}>{playing ? <IconPause /> : <IconPlay />}</button>
        <button className="ibtn" onClick={stop(onNext)} title="Next chapter"><IconNext /></button>
        <button className="btn sm mini-disc" onClick={stop(onMenu)} title="Disc menu">Menu</button>
        <button className="btn sm mini-disc hide-narrow" onClick={stop(onTitleMenu)} title="Title menu">Title</button>
        <button className="ibtn" onClick={stop(onExpand)} title="Full player"><IconExpand /></button>
        <button className="ibtn" onClick={stop(onClose)} title="Stop"><IconClose /></button>
      </div>
    </div>
  )
}

// SubtitleLayer draws the disc's own sub-pictures over the video. Events are
// fetched in two-minute windows around the playhead. size scales them around
// their bottom centre (bigger captions grow upwards, like a TV setting).
// clear: how much of the player's bottom the controls cover when shown
// (subtitles move up out of their way, and back down when they hide).
// forced: draw the disc's forced captions while subtitles are off.
function SubtitleLayer({ video, title, rect, size = 1, stageH = 0, clear = 0, forced = true }: {
  video: React.RefObject<HTMLVideoElement | null>; title: NonNullable<Presentation['title']>; rect: Rect; size?: number; stageH?: number; clear?: number; forced?: boolean
}) {
  const stream = title.subtitleStream >= 0 ? title.subtitleStream : forced ? title.forcedStream : -1
  const forcedOnly = title.subtitleStream < 0
  const cache = useRef(new Map<string, SubEvent[] | 'loading'>())
  const [active, setActive] = useState<SubEvent[]>([])
  useEffect(() => {
    const v = video.current
    if (!v || stream < 0) { setActive([]); return }
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
      setActive((prev) => (prev.length === now.length && prev.every((p, i) => p.url === now[i].url && p.start === now[i].start) ? prev : now))
    }
    upd()
    v.addEventListener('timeupdate', upd)
    v.addEventListener('seeked', upd)
    return () => { v.removeEventListener('timeupdate', upd); v.removeEventListener('seeked', upd) }
  }, [video, stream, forcedOnly, title.subtitlesUrl])
  if (!active.length || !title.frameW || stream < 0) return null
  const sx = rect.width / title.frameW, sy = rect.height / title.frameH
  const lowest = Math.max(...active.map((e) => (e.y + e.h) * sy))
  const lift = stageH ? Math.max(0, clear - (stageH - rect.top - lowest)) : 0
  return (
    <div className="sub-layer" style={{ left: rect.left, top: rect.top, width: rect.width, height: rect.height, ['--lift' as string]: `${Math.round(lift)}px` }}>
      {active.map((e) => {
        const w = e.w * sx * size, h = e.h * sy * size
        const cx = (e.x + e.w / 2) * sx, bottom = (e.y + e.h) * sy
        if (e.text != null) {
          // Text subtitles: the disc's words and place, in the browser's type.
          return <div key={`${e.start}-${e.y}`} className="sub-text" style={{ position: 'absolute', left: e.x * sx, top: e.y * sy, width: e.w * sx, height: e.h * sy, display: 'flex', whiteSpace: 'pre-line',
            justifyContent: e.halign === 1 ? 'flex-start' : e.halign === 3 ? 'flex-end' : 'center', alignItems: e.valign === 1 ? 'flex-start' : e.valign === 2 ? 'center' : 'flex-end', textAlign: e.halign === 1 ? 'left' : e.halign === 3 ? 'right' : 'center',
            fontSize: (e.size || 42) * sy * size, fontWeight: e.bold ? 700 : 400, fontStyle: e.italic ? 'italic' : undefined, color: e.color || '#fff', textShadow: `0 0 3px ${e.outline || '#000'}, 0 0 3px ${e.outline || '#000'}` }}>{e.text}</div>
        }
        return <img key={e.url} src={mediaUrl(e.url)} alt="" style={{ left: cx - w / 2, top: bottom - h, width: w, height: h }} />
      })}
    </div>
  )
}

// StereoLayer shows 3D video (two views side by side or over-under) as one
// eye's view, or as red/cyan anaglyph, by drawing the hidden video onto a
// canvas frame by frame. Anaglyph needs canvas filters (Chromium, Firefox);
// elsewhere it falls back to one eye.
function StereoLayer({ video, rect, mode, out }: { video: React.RefObject<HTMLVideoElement | null>; rect: Rect; mode: 'sbs' | 'ou'; out: '2d' | 'anaglyph' }) {
  const canvas = useRef<HTMLCanvasElement | null>(null)
  useEffect(() => {
    const v = video.current, c = canvas.current
    if (!v || !c) return
    const ctx = c.getContext('2d')
    if (!ctx) return
    const anaglyph = out === 'anaglyph' && 'filter' in ctx
    let stop = false, raf = 0
    const paint = () => {
      const W = v.videoWidth, H = v.videoHeight
      if (W && H) {
        if (c.width !== W || c.height !== H) { c.width = W; c.height = H }
        const sbs = mode === 'sbs'
        const w = sbs ? W / 2 : W, h = sbs ? H : H / 2
        ctx.globalCompositeOperation = 'source-over'
        ctx.filter = anaglyph ? 'url(#wms-an-left)' : 'none'
        ctx.drawImage(v, 0, 0, w, h, 0, 0, W, H)
        if (anaglyph) {
          ctx.globalCompositeOperation = 'lighter'
          ctx.filter = 'url(#wms-an-right)'
          ctx.drawImage(v, sbs ? w : 0, sbs ? 0 : h, w, h, 0, 0, W, H)
          ctx.globalCompositeOperation = 'source-over'
          ctx.filter = 'none'
        }
      }
    }
    const draw = () => {
      if (stop) return
      paint()
      const rvfc = (v as any).requestVideoFrameCallback
      if (typeof rvfc === 'function') rvfc.call(v, draw); else raf = requestAnimationFrame(draw)
    }
    draw()
    // New frames arrive only while playing; a paused or seeked video is
    // painted once, so switching modes never leaves the picture blank.
    const evs = ['seeked', 'loadeddata', 'pause'] as const
    evs.forEach((e) => v.addEventListener(e, paint))
    return () => { stop = true; cancelAnimationFrame(raf); evs.forEach((e) => v.removeEventListener(e, paint)) }
  }, [video, mode, out])
  return (
    <>
      <svg className="stereo-defs" aria-hidden="true"><defs>
        <filter id="wms-an-left" colorInterpolationFilters="sRGB"><feColorMatrix type="matrix" values="1 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 1 0" /></filter>
        <filter id="wms-an-right" colorInterpolationFilters="sRGB"><feColorMatrix type="matrix" values="0 0 0 0 0  0 1 0 0 0  0 0 1 0 0  0 0 0 1 0" /></filter>
      </defs></svg>
      <canvas ref={canvas} className="stereo" style={{ left: rect.left, top: rect.top, width: rect.width, height: rect.height }} aria-hidden="true" />
    </>
  )
}
