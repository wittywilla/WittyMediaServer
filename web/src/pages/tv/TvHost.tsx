import { useEffect, useRef } from 'react'
import { useSource } from '../../player/hooks'
import { lendHls, liveVideo, setLive, useLiveState } from './live'
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
  return null
}
