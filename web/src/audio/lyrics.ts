import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api'

// Lyrics on this side: what the server found for a song, and which line we're on.

export type LyricWord = { t: number; text: string }
export type LyricLine = { t: number; end?: number; text: string; words?: LyricWord[] }
export type Lyrics = {
  itemId: number; source: string; sourceName?: string; creditUrl?: string; synced: boolean; lines: LyricLine[]; plain: string
  instrumental?: boolean; offsetMs: number; online: boolean; canEdit: boolean; tried: boolean; failed?: string; retry?: boolean
  songId?: number; startMs?: number // a music video showing its song's lyrics, and where the song starts in it
}

// A song's lyrics change rarely, so they're kept for as long as this page is open, and
// asked for again only after someone has corrected them. Not when the source couldn't be
// reached: then the next time the song is opened asks again.
const held = new Map<number, Lyrics>()

// The server was busy with another song: ask again after this long, a few times.
const RETRY_MS = 3000
const RETRIES = 4

export function useLyrics(itemId: number | undefined, wanted: boolean) {
  const [lyrics, setLyrics] = useState<Lyrics | null>(null)
  const [loading, setLoading] = useState(false)
  const [failed, setFailed] = useState('')
  const [nonce, setNonce] = useState(0)
  useEffect(() => {
    setFailed('')
    if (!itemId || !wanted) { setLyrics(itemId && held.get(itemId) || null); return }
    const have = held.get(itemId)
    if (have) { setLyrics(have); return }
    let gone = false
    let tries = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    // Moving on to another song gives this one up: a look-up nobody is waiting for
    // mustn't hold a connection the next song needs (issue #17).
    const stop = new AbortController()
    setLoading(true)
    setLyrics(null)
    const ask = () => {
      api.get<Lyrics>(`/music/lyrics/${itemId}?fetch=1`, stop.signal)
        .then((l) => {
          if (gone) return
          if (l.retry && ++tries < RETRIES) { timer = setTimeout(ask, RETRY_MS); return }
          if (!l.failed) held.set(itemId, l)
          setLyrics(l)
          setLoading(false)
        })
        .catch((e: Error) => { if (!gone) { setFailed(e.message); setLoading(false) } })
    }
    ask()
    return () => { gone = true; clearTimeout(timer); stop.abort() }
  }, [itemId, wanted, nonce])
  // A song and its video share lyrics, so a correction to one is forgotten for both.
  const reload = useCallback(() => {
    if (!itemId) return
    const song = held.get(itemId)?.songId || itemId
    for (const [k, v] of held) if (k === itemId || k === song || v.songId === song) held.delete(k)
    setNonce((n) => n + 1)
  }, [itemId])
  const setOffset = useCallback((ms: number) => {
    setLyrics((l) => {
      if (!l) return l
      const next = { ...l, offsetMs: ms }
      held.set(l.itemId, next)
      return next
    })
    if (itemId) saveOffset(itemId, ms)
  }, [itemId])
  return { lyrics, loading, failed, reload, setOffset }
}

// Saved once the buttons have stopped being pressed.
let offsetTimer: ReturnType<typeof setTimeout> | undefined
function saveOffset(id: number, ms: number) {
  clearTimeout(offsetTimer)
  offsetTimer = setTimeout(() => { api.put(`/music/lyrics/${id}/offset`, { offsetMs: ms }).catch(() => {}) }, 500)
}

// The line sung at this moment, or -1 before the first one.
export function lineAt(lines: LyricLine[], ms: number) {
  let lo = 0, hi = lines.length - 1, at = -1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (lines[mid].t <= ms) { at = mid; lo = mid + 1 } else hi = mid - 1
  }
  return at
}

// Follows the music, reading its position every frame while the line is what matters. It
// only causes a new render when the line changes; the rest is left to whoever draws.
export function useLineIndex(lines: LyricLine[], position: () => number, offsetMs: number, playing: boolean) {
  const [index, setIndex] = useState(-1)
  const was = useRef(-1)
  useEffect(() => {
    let raf = 0
    let last = 0
    const step = (now: number) => {
      raf = requestAnimationFrame(step)
      if (now - last < 66) return
      last = now
      const i = lineAt(lines, position() * 1000 - offsetMs)
      if (i !== was.current) { was.current = i; setIndex(i) }
    }
    const once = () => { const i = lineAt(lines, position() * 1000 - offsetMs); if (i !== was.current) { was.current = i; setIndex(i) } }
    once()
    if (!lines.length) return
    // Frames stop by themselves while the page is hidden, and pick up again on the way back.
    if (playing) raf = requestAnimationFrame(step)
    // Paused, a seek still has to move the highlight, without a frame loop running.
    const t = playing ? undefined : setInterval(once, 300)
    return () => { cancelAnimationFrame(raf); clearInterval(t) }
  }, [lines, position, offsetMs, playing])
  return index
}

// Where in a line the singer is, 0 to 1 per word, for the wipe in karaoke. With timed words
// it's exact. Otherwise it's spread by length over the time the line has, which reads well
// for most singing, but a held note will get ahead of it: the display says which it's using.
export function wordTimes(line: LyricLine, nextT: number | undefined): { text: string; from: number; to: number }[] {
  const end = line.end ?? nextT ?? line.t + 5000
  if (line.words?.length) {
    return line.words.map((w, i) => ({ text: w.text, from: w.t, to: line.words![i + 1]?.t ?? Math.min(end, w.t + 1500) }))
  }
  const parts = line.text.split(/(?<=\s)/)
  const singing = Math.min(end - line.t, Math.max(1200, line.text.length * 95))
  const total = parts.reduce((n, p) => n + p.length, 0) || 1
  let at = line.t
  return parts.map((text) => {
    const span = (text.length / total) * singing
    const out = { text, from: at, to: at + span }
    at += span
    return out
  })
}
