import { useCallback, useEffect, useRef, useState } from 'react'
import { fmtTime } from '../../api'
import { useLineIndex, useLyrics, type LyricLine } from '../../audio/lyrics'
import { WebLink } from '../../components/WebLink'
import { Button } from '../../ui/Button'

// A music video's lyrics, beside it: its song's when it's tied to one, so what's fetched or
// corrected for either is there for both, and synced lines follow the video. The timing
// starts where the song starts in the video, and each person can nudge it for themselves.

const NO_LINES: LyricLine[] = []

export function VideoLyrics({ videoId, video }: { videoId: number; video: React.MutableRefObject<HTMLVideoElement | null> }) {
  const [open, setOpen] = useState(false)
  const ly = useLyrics(videoId, open)
  const { lyrics, loading, failed, setOffset } = ly
  const [playing, setPlaying] = useState(false)
  useEffect(() => {
    const v = video.current
    if (!v || !open) return
    const on = () => setPlaying(!v.paused)
    on()
    v.addEventListener('play', on)
    v.addEventListener('pause', on)
    return () => { v.removeEventListener('play', on); v.removeEventListener('pause', on) }
  }, [video, open, videoId])
  const position = useCallback(() => video.current?.currentTime || 0, [video])
  const lines = lyrics?.synced ? lyrics.lines : NO_LINES
  const offset = lyrics?.offsetMs || 0
  const start = lyrics?.startMs || 0
  const index = useLineIndex(open ? lines : NO_LINES, position, offset, playing)
  const box = useRef<HTMLOListElement>(null)
  const heldUntil = useRef(0)
  useEffect(() => {
    const list = box.current
    if (!list || index < 0 || performance.now() < heldUntil.current) return
    const el = list.children[index] as HTMLElement | undefined
    if (!el) return
    const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    list.scrollTo({ top: el.offsetTop - list.clientHeight / 2 + el.clientHeight / 2, behavior: reduced ? 'auto' : 'smooth' })
  }, [index])
  const hold = () => { heldUntil.current = performance.now() + 4000 }
  const nudge = (ms: number) => setOffset(Math.max(start - 120000, Math.min(start + 120000, offset + ms)))
  const seek = (ms: number) => { const v = video.current; if (v) { v.currentTime = Math.max(0, ms / 1000); heldUntil.current = 0 } }
  const moved = offset - start
  return (
    <section className="mv-lyrics" aria-label="Lyrics">
      <Button size="sm" variant={open ? 'primary' : undefined} aria-expanded={open} onClick={() => setOpen(!open)}>{open ? 'Hide the lyrics' : 'Lyrics'}</Button>
      {open && (
        <div className="mv-lyrics-body">
          {loading && <p className="muted">Looking for the words…</p>}
          {failed && <p className="muted" role="alert">{failed}</p>}
          {lyrics?.instrumental && <p className="muted">This one has no words.</p>}
          {lyrics && !loading && !lyrics.instrumental && !lyrics.plain && (
            <p className="muted">{lyrics.failed ? 'The lyrics source couldn\'t be reached just now.' : lyrics.songId ? 'No words found for this song.' : 'This video isn\'t tied to a song, and has no lyrics file beside it.'}</p>
          )}
          {lines.length > 0 ? (
            <ol className="mv-lines" ref={box} onWheel={hold} onTouchMove={hold}>
              {lines.map((l, i) => (
                <li key={i} className={i === index ? 'now' : i < index ? 'past' : ''}>
                  <button type="button" aria-current={i === index ? 'true' : undefined} aria-label={`${fmtTime((l.t + offset) / 1000)}: ${l.text || 'a break'}`} onClick={() => seek(l.t + offset)}>
                    {l.text || <span aria-hidden="true">♪</span>}
                  </button>
                </li>
              ))}
            </ol>
          ) : lyrics?.plain ? <pre className="mv-plain" tabIndex={0}>{lyrics.plain}</pre> : null}
          {lyrics && (lyrics.plain || lines.length > 0) && (
            <div className="mv-lyrics-tools">
              {lines.length > 0 && (
                <span className="mv-nudge" role="group" aria-label="Timing">
                  <Button size="sm" variant="ghost" onClick={() => nudge(-250)} title="The words come sooner">Sooner</Button>
                  <span aria-live="polite" className="mv-nudge-v">{moved === 0 ? 'On time' : `${moved > 0 ? '+' : ''}${(moved / 1000).toFixed(2)} s`}</span>
                  <Button size="sm" variant="ghost" onClick={() => nudge(250)} title="The words come later">Later</Button>
                  {moved !== 0 && <Button size="sm" variant="ghost" onClick={() => setOffset(start)}>Reset</Button>}
                </span>
              )}
              {lyrics.sourceName && <span className="muted mv-lyrics-src">{lyrics.creditUrl ? <WebLink href={lyrics.creditUrl}>{lyrics.sourceName}</WebLink> : lyrics.sourceName}</span>}
            </div>
          )}
          {lyrics?.songId && lyrics.canEdit && <p className="muted mv-lyrics-note">These are the song's lyrics: fix them from the song's full-screen player, and the change shows here too.</p>}
        </div>
      )}
    </section>
  )
}
