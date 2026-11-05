import { useCallback, useEffect, useState } from 'react'
import type Hls from 'hls.js'
import { langName } from '../../api'
import type { TrackOpt } from '../../player/Controls'

// The sound tracks and subtitles a channel's stream carries, for the TV player's menus. A channel
// is one stream for everyone on it, so choosing here changes only what this player plays: other
// languages come as the stream's alternate audio, and subtitles and broadcast captions as text
// tracks on the video. The language chosen is remembered, so the next programme, or the next
// channel, starts in it where it can.

const AUDIO_KEY = 'wms.tv.audioLang'
const SUB_KEY = 'wms.tv.subLang'
const read = (k: string) => { try { return localStorage.getItem(k) || '' } catch { return '' } }
const keep = (k: string, v: string) => { try { localStorage.setItem(k, v) } catch { /* ignore */ } }

const isText = (t: TextTrack) => t.kind === 'subtitles' || t.kind === 'captions'
const textLabel = (t: TextTrack, i: number) =>
  [t.language ? langName(t.language) : '', t.label && t.label !== t.language ? t.label : '', t.kind === 'captions' ? 'CC' : ''].filter(Boolean).join(' · ') || `Subtitles ${i + 1}`

// `ready`: the stream is attached, so hls.js knows what it carries.
export function useLiveTracks(video: HTMLVideoElement | null, hls: () => Hls | null, ready: boolean) {
  const [audio, setAudio] = useState<TrackOpt[]>([])
  const [subtitles, setSubtitles] = useState<TrackOpt[]>([])

  const readAudio = useCallback(() => {
    const h = hls()
    if (!h) { setAudio([]); return }
    setAudio(h.audioTracks.map((t, i) => ({ id: i, label: [t.lang ? langName(t.lang) : '', t.name && t.name !== t.lang ? t.name : ''].filter(Boolean).join(' · ') || `Sound ${i + 1}`, active: i === h.audioTrack })))
  }, [hls])
  const readText = useCallback(() => {
    if (!video) { setSubtitles([]); return }
    const list = Array.from(video.textTracks).filter(isText)
    setSubtitles(list.map((t, i) => ({ id: i, label: textLabel(t, i), active: t.mode === 'showing' })))
  }, [video])

  // What the stream has, as it says, and the remembered languages put back on it.
  useEffect(() => {
    const h = hls()
    if (!h || !ready) { setAudio([]); return }
    const E = (h.constructor as typeof import('hls.js').default).Events
    const onTracks = () => {
      const want = read(AUDIO_KEY)
      const i = want ? h.audioTracks.findIndex((t) => t.lang === want) : -1
      if (i >= 0 && i !== h.audioTrack) h.audioTrack = i
      readAudio()
    }
    h.on(E.AUDIO_TRACKS_UPDATED, onTracks)
    h.on(E.AUDIO_TRACK_SWITCHED, readAudio)
    onTracks()
    return () => { h.off(E.AUDIO_TRACKS_UPDATED, onTracks); h.off(E.AUDIO_TRACK_SWITCHED, readAudio) }
  }, [hls, ready, readAudio])
  useEffect(() => {
    if (!video) return
    const tt = video.textTracks
    const onAdd = (e: TrackEvent) => {
      const t = e.track as TextTrack | null
      const want = read(SUB_KEY)
      if (t && isText(t) && want && t.language === want && !Array.from(tt).some((x) => isText(x) && x.mode === 'showing')) t.mode = 'showing'
      readText()
    }
    tt.addEventListener('addtrack', onAdd)
    tt.addEventListener('removetrack', readText)
    tt.addEventListener('change', readText)
    readText()
    return () => { tt.removeEventListener('addtrack', onAdd); tt.removeEventListener('removetrack', readText); tt.removeEventListener('change', readText) }
  }, [video, ready, readText])

  const chooseAudio = useCallback((id: number) => {
    const h = hls()
    const t = h?.audioTracks[id]
    if (!h || !t) return
    h.audioTrack = id
    keep(AUDIO_KEY, t.lang || '')
    readAudio()
  }, [hls, readAudio])
  // -1 is off.
  const chooseSubtitle = useCallback((id: number) => {
    if (!video) return
    const list = Array.from(video.textTracks).filter(isText)
    list.forEach((t, i) => { t.mode = i === id ? 'showing' : 'disabled' })
    keep(SUB_KEY, id >= 0 ? list[id]?.language || '' : '')
    readText()
  }, [video, readText])
  // The A and S keys: the next of each, round to off for subtitles.
  const nextAudio = () => { if (audio.length > 1) chooseAudio((audio.findIndex((a) => a.active) + 1) % audio.length) }
  const nextSubtitle = () => {
    if (!subtitles.length) return
    const i = subtitles.findIndex((s) => s.active)
    chooseSubtitle(i + 1 >= subtitles.length ? -1 : i + 1)
  }
  return { audio, subtitles, chooseAudio, chooseSubtitle, nextAudio, nextSubtitle }
}
