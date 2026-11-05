import { useEffect, useRef } from 'react'
import { api } from '../../api'
import { useSource } from '../../player/hooks'
import { clockOffset, syncStep, type ClockSample } from '../../player/sync'
import { lendHls, liveNow, liveVideo, setLive, useLiveState } from './live'
import { preflight } from './tvApi'

// The TV section's one stream (see live.ts). It's mounted beside the pages rather than in one,
// so the guide, the watch view and full screen all play the same one without starting again.
export default function TvHost() {
  const { url, ready } = useLiveState()
  const video = useRef<HTMLVideoElement | null>(liveVideo())
  const hls = useSource(video, url, {
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

  // Live sync: everyone on a channel at the same moment, "the server's time now, less the
  // hold-back", by the program date-times on its segments. Checked every 2 seconds from what's in
  // hand, with no requests beyond a few for the clock. A pause or going back leaves it be (away),
  // except a pause too short to mean it: a double-click for full screen is two clicks.
  useEffect(() => {
    if (!url || !ready) return
    const v = liveVideo()
    let on = true, nudging = false, pausedAt = 0
    const onPause = () => { pausedAt = Date.now(); setLive({ away: true }) }
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
      if (v.paused || v.seeking || liveNow().away) {
        if (nudging) { v.playbackRate = 1; nudging = false }
        return
      }
      // The nearest a player goes to the newest segment in the playlist (not the end of what it
      // has loaded, which is what a live video's seekable range ends at): three target
      // durations, as HLS asks. A stream running late (a server that can't make it at full
      // speed) can't be caught up, so everyone waits for it there, together.
      const start = v.seekable.length ? v.seekable.start(0) : v.currentTime
      const nearest = details?.edge != null ? details.edge - target : v.currentTime
      const step = behind > 0 && v.currentTime >= nearest - 0.5 ? { rate: 1 } : syncStep(behind, nudging)
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
      v.playbackRate = 1
      delete v.dataset.behind
    }
  }, [url, ready, hls])
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
