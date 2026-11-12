// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { mediaUrl } from '../api'
import { canvasPlays, type CanvasInfo } from './rules'
import { useAlbumCanvas, useCanvas, useTabHidden } from './useCanvas'
import './canvas.css'

// The canvas itself: a silent video looping behind a title, as decoration. It never takes
// focus, a screen reader never hears of it, and the still artwork is what's there until it
// is really playing, and again if it can't. The video is only given its address while it
// would be seen (rules.ts), so one that is scrolled away, in a hidden tab or behind paused
// music is not downloaded, and is let go of when it stops being seen.

export function CanvasVideo({ info, blocked, paused, className }: { info: CanvasInfo | null; blocked: string; paused?: boolean; className?: string }) {
  const box = useRef<HTMLDivElement>(null)
  const video = useRef<HTMLVideoElement>(null)
  const [onScreen, setOnScreen] = useState(false)
  const [showing, setShowing] = useState(false)
  const [broken, setBroken] = useState('')
  const tabHidden = useTabHidden()
  const url = info?.available && info.url ? info.url : ''
  // Loaded while it would be seen. Once it has been, a pause of the music only pauses it:
  // letting go of it and fetching it again for every pause would cost more than it saves.
  const wanted = canvasPlays({ blocked, available: !!url && broken !== url, onScreen, tabHidden: false })
  const plays = canvasPlays({ blocked, available: !!url && broken !== url, onScreen, tabHidden, paused })

  useEffect(() => {
    const el = box.current
    if (!el || typeof IntersectionObserver === 'undefined') { setOnScreen(true); return }
    const io = new IntersectionObserver((entries) => setOnScreen(entries.some((e) => e.isIntersecting)), { rootMargin: '80px' })
    io.observe(el)
    return () => io.disconnect()
  }, [])

  useEffect(() => { setShowing(false) }, [url, wanted])
  useEffect(() => {
    const v = video.current
    if (!v) return
    // Silent before it starts: a browser only lets a video start by itself if it is.
    v.muted = true
    if (plays) void v.play().catch(() => { /* the browser won't start it by itself: the still stays */ })
    else v.pause()
  }, [plays, wanted, url])
  // Leaving: stop the download with the element, which a browser doesn't always do by itself.
  useEffect(() => {
    const v = video.current
    return () => { if (v) { v.pause(); v.removeAttribute('src'); v.load() } }
  }, [wanted, url])

  return (
    <div className={'canvas-loop' + (showing ? ' on' : '') + (className ? ' ' + className : '')} ref={box} aria-hidden="true">
      {wanted && (
        <video ref={video} key={url} src={mediaUrl(url)} muted loop playsInline preload="auto" tabIndex={-1} disablePictureInPicture disableRemotePlayback
          controls={false} onPlaying={() => setShowing(true)} onError={() => { setShowing(false); setBroken(url) }} />
      )}
    </div>
  )
}

// Behind a film's or an episode's page, over its still backdrop.
export function TitleCanvas({ itemId, className }: { itemId: number | undefined; className?: string }) {
  const c = useCanvas(itemId)
  if (!c.info?.available || c.blocked) return null
  return <CanvasVideo info={c.info} blocked={c.blocked} className={className} />
}

// Behind an album's page.
export function AlbumCanvas({ trackIds, className }: { trackIds: number[]; className?: string }) {
  const c = useAlbumCanvas(trackIds)
  if (!c.info?.available || c.blocked) return null
  return <CanvasVideo info={c.info} blocked={c.blocked} className={className} />
}
