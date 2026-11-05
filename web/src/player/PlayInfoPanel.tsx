import { Dialog, Button } from '../ui'
import { HelpLink } from '../components/HelpLink'
import type { PlaybackAnswer } from '../api'

// "How this is playing": what the file is, what happens to its picture, sound and
// subtitles, and why. The words come from the server, which made the decision.

const LOAD: Record<string, string> = { none: 'Nothing: your player does the work', light: 'Very little', heavy: 'A lot' }

function Fact({ k, v }: { k: string; v?: string }) {
  if (!v) return null
  return <div className="playinfo-fact"><dt>{k}</dt><dd>{v}</dd></div>
}

export default function PlayInfoPanel({ info, netLog, onClose, onFlat3d, flattened, onVideo }: {
  info: PlaybackAnswer
  // What a robust stream's player did while playing, oldest first.
  netLog?: string[]
  onClose: () => void
  onFlat3d: (on: boolean) => void
  flattened: boolean
  onVideo: (index: number) => void
}) {
  const ex = info.explain
  return (
    <Dialog title="How this is playing" onClose={onClose} actions={<Button onClick={onClose}>Close</Button>}>
      <div className="playinfo">
        {ex && <p className="playinfo-summary">{ex.summary} <HelpLink to="playing#quality-direct-play-and-converting" newTab>What this means</HelpLink></p>}
        <dl className="playinfo-facts">
          <Fact k="The file" v={ex?.source} />
          <Fact k="Picture" v={ex?.picture} />
          <Fact k="Sound" v={ex?.sound} />
          <Fact k="Subtitles" v={ex?.subtitles} />
          <Fact k="Work for the server" v={ex?.load ? LOAD[ex.load] : undefined} />
          <Fact k="Streaming" v={ex?.streaming} />
        </dl>
        {netLog && netLog.length > 0 && (
          <>
            <h3 className="playinfo-head">What the player did</h3>
            <ul className="playinfo-reasons">{netLog.map((r, i) => <li key={i}>{r}</li>)}</ul>
          </>
        )}
        {ex && ex.reasons.length > 0 && (
          <>
            <h3 className="playinfo-head">Why it isn’t simpler</h3>
            <ul className="playinfo-reasons">{ex.reasons.map((r) => <li key={r}>{r}</li>)}</ul>
          </>
        )}
        {info.stereo && (
          <div className="playinfo-choice">
            <p>This is a 3D film, stored with both eyes in one frame.</p>
            <Button onClick={() => onFlat3d(!flattened)}>{flattened ? 'Show both eyes again' : 'Show it in 2D'}</Button>
          </div>
        )}
        {info.videos && info.videos.length > 1 && (
          <div className="playinfo-choice">
            <p>This file holds {info.videos.length} video streams.</p>
            <div className="playinfo-videos" role="group" aria-label="Video streams">
              {info.videos.map((v) => (
                <Button key={v.index} variant={(info.videoIndex || 0) === v.index ? 'primary' : undefined} aria-pressed={(info.videoIndex || 0) === v.index} onClick={() => onVideo(v.index)}>
                  {v.title || `Stream ${v.index + 1}`} · {v.width}×{v.height}
                </Button>
              ))}
            </div>
          </div>
        )}
      </div>
    </Dialog>
  )
}
