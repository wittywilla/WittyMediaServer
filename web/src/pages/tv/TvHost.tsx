import { useEffect, useRef } from 'react'
import { useSource } from '../../player/hooks'
import { api } from '../../api'
import { lendHls, liveNow, liveVideo, liveViewer, setLive, useLiveState } from './live'
import { preflight, type TuneInfo } from './tvApi'

// The TV section's one stream (see live.ts). It's mounted beside the pages rather than in one,
// so the guide, the watch view and full screen all play the same one without starting again.
export default function TvHost() {
  const { key, info, url, ready } = useLiveState()
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
  // When a programme ends, what's on now: for the banner, and for the slate's card wherever
  // the picture is shown. On a channel that counts watch progress, also when each stretch
  // of it and each break ends, so a break isn't counted as the programme.
  const part = info?.partEnd, programme = info?.now?.end
  const end = part && programme && part < programme ? part : programme
  useEffect(() => {
    if (!key || !end) return
    // An answer that came after the moment it ends (a slow server, or a page in the
    // background whose timers wait) asks again shortly, rather than never.
    const wait = Math.max(end * 1000 - Date.now() + 1500, 5000)
    if (wait > 86400000) return
    const t = setTimeout(() => {
      api.get<TuneInfo>(`/tv/${key}/tune?viewer=${liveViewer()}`).then((i) => { if (liveNow().key === key) setLive({ info: i }) }).catch(() => {})
    }, wait)
    return () => clearTimeout(t)
  }, [key, end])
  return null
}
