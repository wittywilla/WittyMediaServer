// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef } from 'react'
import { api } from '../../api'
import { useSource } from '../../player/hooks'
import { clockOffset, liveStep, type ClockSample } from '../../player/sync'
import { freshGuard, guardStep, type Sample } from '../../player/stallGuard'
import { lendHls, liveNow, liveVideo, liveViewer, setLive, setLiveCatchUp, useLiveState } from './live'
import { preflight, type TuneInfo } from './tvApi'
import { parseCatchupKey } from './guideLogic'
import { autoStep, LADDER, saveStep, savedStep, startStep, lastVia } from './liveAuto'
import { clearStats, frags, noteAuto, noteFrag, noteStall, stalls } from './liveStats'
import { deviceChoice } from '../../player/quality'
import { retune } from './live'

// The TV section's one stream (see live.ts). It's mounted beside the pages rather than in one,
// so the guide, the watch view and full screen all play the same one without starting again.
export default function TvHost() {
  const { key, info, url, ready } = useLiveState()
  const video = useRef<HTMLVideoElement | null>(liveVideo())
  const hls = useSource(video, url, {
    live: true,
    onReady: () => setLive({ ready: true }),
    onError: (m) => setLive({ err: m }),
  })
  useEffect(() => { lendHls(hls); return () => lendHls(null) }, [hls])
  // A stream attached again (back in the TV section within the moment before leaving) is
  // ready again only when it says so, so the page starts it then.
  useEffect(() => () => setLive({ ready: false }), [url])
  // The stream can be refused after it started: a tuner taken for a recording, or a channel
  // that turns out to be encrypted. The server says which.
  useEffect(() => {
    const h = hls.current
    if (!h || !url || !ready) return
    const ERROR = (h.constructor as typeof import('hls.js').default).Events.ERROR
    const onErr = (_: unknown, d: { fatal?: boolean; response?: { code?: number } }) => {
      const code = d.response?.code
      if (d.fatal && code && code !== 503) {
        preflight(url, new AbortController().signal).catch((e) => { setLive({ err: e.message }); h.destroy() })
      }
    }
    h.on(ERROR, onErr)
    return () => { h.off(ERROR, onErr) }
  }, [url, ready, hls])
  // When a programme ends, what's on now: for the banner, and for the slate's card wherever
  // the picture is shown. On a channel that counts watch progress, also when each stretch
  // of it and each break ends, so a break isn't counted as the programme.
  const part = info?.partEnd, programme = info?.now?.end
  const end = part && programme && part < programme ? part : programme
  useEffect(() => {
    // A programme watched from its start stays that programme: there's nothing to move on to.
    if (!key || !end || parseCatchupKey(key)) return
    // An answer that came after the moment it ends (a slow server, or a page in the
    // background whose timers wait) asks again shortly, rather than never.
    const wait = Math.max(end * 1000 - Date.now() + 1500, 5000)
    if (wait > 86400000) return
    const t = setTimeout(() => {
      api.get<TuneInfo>(`/tv/${key}/tune?viewer=${liveViewer()}`).then((i) => { if (liveNow().key === key) setLive({ info: i }) }).catch(() => {})
    }, wait)
    return () => clearTimeout(t)
  }, [key, end])

  // A picture standing still while its sound plays on (a decoder that couldn't start at a tune-in or a
  // programme join, or fell behind): the sound is silenced and the player seeks to where its clock is, which
  // starts both decoders again together. The viewer's own mute comes back with the picture (player/stallGuard).
  useEffect(() => {
    if (!url || !ready) return
    const v = liveVideo()
    let st = freshGuard(), prev: Sample | null = null, mutedBefore = v.muted
    const t = setInterval(() => {
      // A hidden page, or a video parked out of it, shows no pictures on purpose (a channel kept playing in the
      // background is sound alone): that's not a stall.
      const shown = !document.hidden && v.isConnected
      const now: Sample = { t: v.currentTime, frames: v.getVideoPlaybackQuality?.().totalVideoFrames ?? 0, paused: v.paused || v.seeking || !shown, ready: v.readyState >= 2 }
      const r = guardStep(prev, now, st)
      st = r.st
      prev = now
      switch (r.step.kind) {
        case 'silence': mutedBefore = v.muted; v.muted = true; break
        case 'seek': v.currentTime = v.currentTime; break
        case 'restore': v.muted = mutedBefore; break
      }
    }, 250)
    return () => { clearInterval(t); if (st.silenced) v.muted = mutedBefore }
  }, [url, ready])

  // Auto quality (liveAuto.ts): the fragments as they come and the stalls, and a step down when the stream
  // needs more than the link gives or keeps stalling; up a step once the link has had room for a while.
  // Only when the viewer hasn't chosen a quality.
  useEffect(() => {
    const h = hls.current
    if (!h || !url || !ready) return
    const v = liveVideo()
    clearStats()
    const t0 = performance.now()
    const Events = (h.constructor as typeof import('hls.js').default).Events
    type Loaded = { frag: { sn: number | string; cc: number; duration: number; stats: { total: number; loaded: number; loading: { start: number; first: number; end: number } } } }
    const onFrag = (_e: unknown, d: Loaded) => {
      const st = d.frag.stats, l = st.loading
      noteFrag({ at: Math.round(l.start - t0), sn: Number(d.frag.sn), cc: d.frag.cc, bytes: st.total || st.loaded, durationSec: d.frag.duration, loadMs: Math.max(1, l.end - l.start), firstByteMs: Math.max(0, l.first - l.start) })
    }
    let played = false
    const onPlaying = () => { played = true }
    const onWaiting = () => { if (played && !v.seeking) noteStall(Math.round(performance.now() - t0)) }
    h.on(Events.FRAG_LOADED, onFrag as never)
    v.addEventListener('playing', onPlaying)
    v.addEventListener('waiting', onWaiting)
    let changed = performance.now()
    const t = setInterval(() => {
      if (deviceChoice()) return // the viewer's own choice
      const now = performance.now()
      const step = startStep(savedStep(), lastVia())
      const recent = frags.filter((f) => f.at >= changed - t0).slice(-4)
      const look = { frags: recent, stalls: stalls.filter((x) => x.at > now - t0 - 30000).length, sinceChangeSec: (now - changed) / 1000 }
      const say = autoStep(step, look)
      if (say === 'keep') return
      const to = say === 'down' ? Math.min(LADDER.length - 1, step + 1) : Math.max(0, step - 1)
      noteAuto({ at: Math.round(now - t0), from: step, to, why: say === 'down' ? (look.stalls >= 2 ? 'stalls' : 'the link') : 'room' })
      saveStep(to)
      changed = now
      retune()
    }, 3000)
    return () => {
      clearInterval(t)
      h.off(Events.FRAG_LOADED, onFrag as never)
      v.removeEventListener('playing', onPlaying)
      v.removeEventListener('waiting', onWaiting)
    }
  }, [url, ready, hls])

  // Live sync: everyone on a channel at the same moment, "the server's time now, less the
  // hold-back", by the program date-times on its segments. Checked every 2 seconds from what's in
  // hand, with no requests beyond a few for the clock. A pause or going back leaves it be (away),
  // except a pause too short to mean it: a double-click for full screen is two clicks.
  useEffect(() => {
    // A programme from its start is the viewer's own stream: nobody to keep up with.
    if (!url || !ready || parseCatchupKey(key)) return
    const v = liveVideo()
    let on = true, nudging = false, pausedAt = 0, stalledAt = 0, afterStall = false
    // A real stall: waiting for data for over a second while meant to be playing. Only that may
    // let the clock jump (liveStep), once, at the next check after it plays again.
    const onWaiting = () => { if (!v.paused && !v.seeking) stalledAt = Date.now() }
    const onPlaying = () => { if (stalledAt && Date.now() - stalledAt > 1000) afterStall = true; stalledAt = 0 }
    v.addEventListener('waiting', onWaiting)
    v.addEventListener('playing', onPlaying)
    // A silent video paused while the page is hidden was paused by the browser, not the viewer:
    // it starts again when the page is shown (live.ts). With sound, only the viewer pauses it.
    const onPause = () => { if (document.hidden && v.muted) return; pausedAt = Date.now(); setLive({ away: true }) }
    const onPlay = () => { if (pausedAt && Date.now() - pausedAt < 1500) setLive({ away: false }); pausedAt = 0 }
    v.addEventListener('pause', onPause)
    v.addEventListener('play', onPlay)
    void measureClock()
    const t = setInterval(() => {
      const h = hls.current
      const at = h?.playingDate
      if (!on || !h || !at) return
      // Everyone keeps to the server's clock: a library channel is made ahead of it, so to the
      // clock itself; a tuner's comes in as it's broadcast, so three target durations behind it
      // (LiveHoldBack on the server). The playlist's hold-back starts players there.
      const details = h.levels[Math.max(0, h.currentLevel)]?.details
      const target = 3 * (details?.targetduration || 4)
      const lag = liveNow().info?.kind === 'tuner' ? target : 0
      const behind = (Date.now() + clock.offset - lag * 1000 - at.getTime()) / 1000
      v.dataset.behind = behind.toFixed(3) // for tests and anyone curious
      // The nearest a player goes to the newest segment in the playlist (not the end of what it
      // has loaded, which is what a live video's seekable range ends at): three target
      // durations, as HLS asks. A stream running late (a server that can't make it at full
      // speed) can't be caught up, so everyone waits for it there, together.
      const nearest = details?.edge != null ? details.edge - target : v.currentTime
      // How far Back to live would move this player: the watch view shows Live from this, paused or not.
      setLiveCatchUp(Math.max(0, Math.min(nearest, v.currentTime + behind) - v.currentTime))
      if (v.paused || v.seeking || liveNow().away) {
        if (nudging) { v.playbackRate = 1; nudging = false }
        return
      }
      const start = v.seekable.length ? v.seekable.start(0) : v.currentTime
      // Faster only with more than a segment in hand: catching up mustn't run the buffer dry.
      const ahead = v.buffered.length ? v.buffered.end(v.buffered.length - 1) - v.currentTime : 0
      const short = behind > 0 && (v.currentTime >= nearest - 0.5 || ahead < (details?.targetduration || 4))
      const step = short ? { rate: 1 } : liveStep(behind, nudging, afterStall)
      afterStall = false
      if ('seekBy' in step) {
        v.playbackRate = 1
        nudging = false
        v.currentTime = Math.max(start, Math.min(nearest, v.currentTime + step.seekBy))
      } else {
        v.playbackRate = step.rate
        nudging = step.rate !== 1
      }
    }, 2000)
    return () => {
      on = false
      clearInterval(t)
      v.removeEventListener('pause', onPause)
      v.removeEventListener('play', onPlay)
      v.removeEventListener('waiting', onWaiting)
      v.removeEventListener('playing', onPlaying)
      v.playbackRate = 1
      delete v.dataset.behind
      setLiveCatchUp(null)
    }
  }, [url, ready, hls, key])
  return null
}

// How far the server's clock is from this device's, from the best of three timed requests,
// measured again every 10 minutes at most.
const clock = { offset: 0, at: 0 }
async function measureClock() {
  if (clock.at && Date.now() - clock.at < 600000) return
  clock.at = Date.now()
  const samples: ClockSample[] = []
  for (let i = 0; i < 3; i++) {
    const sent = Date.now()
    try {
      const r = await api.get<{ now: number }>('/time')
      samples.push({ sent, server: r.now, got: Date.now() })
    } catch { /* an older server: keep this device's clock */ }
  }
  if (samples.length) clock.offset = clockOffset(samples)
}
