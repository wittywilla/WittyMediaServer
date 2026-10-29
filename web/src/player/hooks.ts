import Hls from 'hls.js'
import { useEffect, useRef } from 'react'
import { getToken, mediaUrl, serverBase } from '../api'

// Attaches an HLS (or direct) source to a <video>. Uses hls.js where MSE is
// available and falls back to native HLS (Safari/iOS).
export function useSource(video: React.RefObject<HTMLVideoElement | null>, url: string | null, opts: { onReady?: () => void; onError?: (msg: string) => void } = {}) {
  const hlsRef = useRef<Hls | null>(null)
  const optsRef = useRef(opts)
  optsRef.current = opts
  useEffect(() => {
    const v = video.current
    if (!v || !url) return
    let destroyed = false
    const isHls = url.includes('.m3u8')
    const abs = url.startsWith('http') ? url : serverBase() + url
    if (isHls && Hls.isSupported()) {
      const hls = new Hls({
        xhrSetup: (xhr) => { const t = getToken(); if (t) xhr.setRequestHeader('Authorization', 'Bearer ' + t); xhr.withCredentials = true },
        maxBufferLength: 30, maxMaxBufferLength: 90, backBufferLength: 30,
        fragLoadingTimeOut: 60000, manifestLoadingTimeOut: 20000, fragLoadingMaxRetry: 6, levelLoadingMaxRetry: 6,
        startPosition: -1, lowLatencyMode: false,
      })
      hlsRef.current = hls
      hls.loadSource(abs)
      hls.attachMedia(v)
      hls.on(Hls.Events.MANIFEST_PARSED, () => { if (!destroyed) optsRef.current.onReady?.() })
      hls.on(Hls.Events.ERROR, (_e, data) => {
        if (!data.fatal) return
        if (data.type === Hls.ErrorTypes.NETWORK_ERROR) hls.startLoad()
        else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) hls.recoverMediaError()
        else optsRef.current.onError?.(data.details || 'playback error')
      })
      return () => { destroyed = true; hls.destroy(); hlsRef.current = null }
    }
    v.src = mediaUrl(abs)
    const ready = () => optsRef.current.onReady?.()
    v.addEventListener('loadedmetadata', ready)
    return () => { destroyed = true; v.removeEventListener('loadedmetadata', ready); v.removeAttribute('src'); v.load() }
  }, [url, video])
  return hlsRef
}

// Auto-hide controls after inactivity.
export function useIdle(ms = 3000, deps: unknown[] = []) {
  const ref = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    let t: any
    const show = () => { el.classList.remove('controls-hidden', 'hide-cursor'); clearTimeout(t); t = setTimeout(() => el.classList.add('controls-hidden', 'hide-cursor'), ms) }
    show()
    el.addEventListener('mousemove', show)
    el.addEventListener('touchstart', show, { passive: true })
    el.addEventListener('keydown', show)
    return () => { clearTimeout(t); el.removeEventListener('mousemove', show); el.removeEventListener('touchstart', show); el.removeEventListener('keydown', show) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)
  return ref
}

export function toggleFullscreen(el: HTMLElement | null) {
  if (!el) return
  if (document.fullscreenElement) document.exitFullscreen().catch(() => {})
  else el.requestFullscreen?.().catch(() => {})
}
