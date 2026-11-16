// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Dialog, Button } from '../ui'
import { HelpLink } from '../components/HelpLink'
import type { PlaybackAnswer } from '../api'
import { t, tk, tn, tx } from '../i18n'

// "How this is playing": what the file is, what happens to its picture, sound and
// subtitles, and why. The words come from the server, which made the decision.

const LOAD: Record<string, string> = { none: tk('Nothing: your player does the work'), light: tk('Very little'), heavy: tk('A lot') }

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
    <Dialog title={t('How this is playing')} onClose={onClose} actions={<Button onClick={onClose}>{t('Close')}</Button>}>
      <div className="playinfo">
        {ex && <p className="playinfo-summary">{tx(ex.summary)} <HelpLink to="playing#quality-direct-play-and-converting" newTab>{t('What this means')}</HelpLink></p>}
        <dl className="playinfo-facts">
          <Fact k={t('The file')} v={ex?.source} />
          <Fact k={t('Picture')} v={ex?.picture} />
          <Fact k={t('Sound')} v={ex?.sound} />
          <Fact k={t('Subtitles')} v={ex?.subtitles} />
          <Fact k={t('Work for the server')} v={ex?.load ? tx(LOAD[ex.load]) : undefined} />
          <Fact k={t('Streaming')} v={ex?.streaming} />
        </dl>
        {netLog && netLog.length > 0 && (
          <>
            <h3 className="playinfo-head">{t('What the player did')}</h3>
            <ul className="playinfo-reasons">{netLog.map((r, i) => <li key={i}>{tx(r)}</li>)}</ul>
          </>
        )}
        {ex && ex.reasons.length > 0 && (
          <>
            <h3 className="playinfo-head">{t('Why it isn’t simpler')}</h3>
            <ul className="playinfo-reasons">{ex.reasons.map((r) => <li key={r}>{tx(r)}</li>)}</ul>
          </>
        )}
        {info.stereo && (
          <div className="playinfo-choice">
            <p>{t('This is a 3D film, stored with both eyes in one frame.')}</p>
            <Button onClick={() => onFlat3d(!flattened)}>{flattened ? t('Show both eyes again') : t('Show it in 2D')}</Button>
          </div>
        )}
        {info.videos && info.videos.length > 1 && (
          <div className="playinfo-choice">
            <p>{tn('This file holds {count} video stream.', 'This file holds {count} video streams.', info.videos.length)}</p>
            <div className="playinfo-videos" role="group" aria-label={t('Video streams')}>
              {info.videos.map((v) => (
                <Button key={v.index} variant={(info.videoIndex || 0) === v.index ? 'primary' : undefined} aria-pressed={(info.videoIndex || 0) === v.index} onClick={() => onVideo(v.index)}>
                  {v.title || t('Stream {n}', { n: v.index + 1 })} · {v.width}×{v.height}
                </Button>
              ))}
            </div>
          </div>
        )}
      </div>
    </Dialog>
  )
}
