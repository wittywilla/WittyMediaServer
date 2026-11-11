// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { announce } from '../a11y'
import { api, mediaUrl, uploadFile } from '../api'
import { HelpLink } from '../components/HelpLink'
import { TaskLine } from '../extras/jobs'
import '../extras/jobs.css'
import { Button, Dialog, Field, Input, Notice, ask, toast } from '../ui'
import { GEN_MAX_SEC, GEN_MIN_SEC, SOURCE_WORDS, generateBody, refusedWords } from './rules'
import { noteCanvasChanged, useCanvas } from './useCanvas'
import './canvas.css'

// A title's canvas, for an administrator: what it has, cutting one from the video (a task
// on the server, shown here until it's done), uploading one, and taking an upload away.

const MAX_UPLOAD = 30 * 1024 * 1024
const BLOCKED_WORDS: Record<string, string> = {
  off: 'You turned moving backgrounds off in Settings, so this device shows the still artwork.',
  motion: 'This device is set to reduce motion, so it shows the still artwork instead.',
  data: 'This device is saving data, so it shows the still artwork instead.',
  away: 'You\'re away from home and streams are limited there, so this device shows the still artwork instead.',
}

export default function CanvasManager({ itemId, title, kind, runtimeSec, picker, onClose }: {
  itemId: number; title: string; kind: 'video' | 'song' | 'disc'; runtimeSec: number
  picker?: ReactNode // an album's page chooses which of its songs this is about
  onClose: () => void
}) {
  const c = useCanvas(itemId, true)
  const info = c.info
  const [start, setStart] = useState('')
  const [length, setLength] = useState('')
  const [fieldErr, setFieldErr] = useState('')
  const [busy, setBusy] = useState<'' | 'generate' | 'upload' | 'remove'>('')
  const file = useRef<HTMLInputElement>(null)
  // Said once when the task ends, and the page behind is told.
  const hadTask = useRef('')
  useEffect(() => {
    if (!info) return
    if (hadTask.current && !info.task) {
      announce(info.error ? 'The canvas couldn\'t be made.' : info.available ? 'The canvas is ready.' : 'The canvas wasn\'t made.')
      noteCanvasChanged(itemId)
    }
    hadTask.current = info.task || ''
  }, [info, itemId])
  useEffect(() => { hadTask.current = ''; setFieldErr('') }, [itemId])

  const done = (words: string) => { toast(words); c.reload(); noteCanvasChanged(itemId) }
  const failed = (e: Error) => toast(e.message, { tone: 'danger' })

  const generate = () => {
    const g = generateBody(start, length, runtimeSec)
    setFieldErr(g.error)
    if (g.error) return
    setBusy('generate')
    api.post(`/media/${itemId}/canvas/generate`, g.body).then(() => c.reload()).catch(failed).finally(() => setBusy(''))
  }
  const upload = (f: File | undefined) => {
    if (!f) return
    // Said here, as a browser can go on sending a big file long after the server has refused it.
    if (f.size > MAX_UPLOAD) { toast('That file is too big: a canvas can be up to 30 MB.', { tone: 'danger' }); return }
    setBusy('upload')
    uploadFile(`/media/${itemId}/canvas`, f, 'PUT').then(() => done('Canvas uploaded')).catch(failed).finally(() => { setBusy(''); if (file.current) file.current.value = '' })
  }
  const remove = async () => {
    if (!await ask({ title: 'Remove this canvas?', body: 'The still artwork is shown again. A canvas file beside the media, if there is one, takes its place.', confirm: 'Remove', danger: true })) return
    setBusy('remove')
    api.del(`/media/${itemId}/canvas`).then(() => done('Canvas removed')).catch(failed).finally(() => setBusy(''))
  }

  const own = info?.source === 'upload' || info?.source === 'generated'
  const facts = info?.available ? [info.width && info.height ? `${info.width}×${info.height}` : '', info.durationMs ? `${(info.durationMs / 1000).toFixed(1)} s` : ''].filter(Boolean).join(' · ') : ''
  return (
    <Dialog title={`Canvas for ${title}`} onClose={onClose} size="md"
      description={<>A short, silent video that loops behind this {kind === 'song' ? 'song' : 'title'}, here and in the apps. <HelpLink to="music#canvas-a-looping-video-behind-the-song" newTab>About canvases</HelpLink></>}
      actions={<Button variant="ghost" onClick={onClose}>Close</Button>}>
      <div className="canvas-admin">
        {picker}
        {!info && !c.failed && <p className="muted" role="status">Looking…</p>}
        {c.failed && <Notice tone="danger">{c.failed}</Notice>}
        {info && (
          <>
            {info.available && info.url ? (
              <div className="canvas-admin-now">
                {/* For looking at, not decoration: it has controls, and starts only when asked. */}
                <video key={info.url} src={mediaUrl(info.url)} muted loop playsInline controls preload="metadata" aria-label={`The canvas for ${title}`} />
                <div className="canvas-admin-facts">
                  <p><b>This {kind === 'song' ? 'song' : 'title'} has a canvas.</b></p>
                  <p className="muted">{SOURCE_WORDS[info.source || ''] || ''}{facts ? ` ${facts}.` : ''}</p>
                  {c.blocked && <p className="muted">{BLOCKED_WORDS[c.blocked]}</p>}
                  {own
                    ? <div className="canvas-admin-row"><Button variant="danger" size="sm" busy={busy === 'remove'} disabled={!!busy || !!info.task} onClick={remove}>Remove</Button></div>
                    : <p className="muted">To take it away, remove the file from its folder. One you upload or cut here is used instead of it.</p>}
                </div>
              </div>
            ) : <p>{refusedWords(info.reason) || `This ${kind === 'song' ? 'song' : 'title'} has no canvas.`}</p>}

            {info.task && <TaskLine id={info.task} label="Cutting a canvas from the video" />}
            {!info.task && info.error && <Notice tone="danger" title="The last one couldn't be made">{info.error}</Notice>}

            <section aria-label="Cut one from the video">
              <h3>Cut one from the video</h3>
              {info.canGenerate ? (
                <>
                  <p className="muted">The server takes a few seconds of the picture, drops the sound, makes it small and fades its end into its start so the loop has no jump. Leave both empty for a quarter of the way in.</p>
                  <div className="canvas-admin-fields">
                    <Field label="Start at" hint="Seconds, or minutes:seconds"><Input inputMode="decimal" value={start} placeholder="A quarter of the way in" onChange={(e) => setStart(e.target.value)} /></Field>
                    <Field label="Length in seconds" hint={`${GEN_MIN_SEC} to ${GEN_MAX_SEC}`}><Input inputMode="decimal" value={length} placeholder="5 to 8" onChange={(e) => setLength(e.target.value)} /></Field>
                  </div>
                  {fieldErr && <div className="error" role="alert">{fieldErr}</div>}
                  <div className="canvas-admin-row"><Button variant="primary" busy={busy === 'generate'} disabled={!!busy || !!info.task} onClick={generate}>{info.available && own ? 'Cut a new one' : 'Cut a canvas'}</Button></div>
                </>
              ) : (
                <p className="muted">{kind === 'song' ? 'A song has no picture to cut from, so a song\'s canvas is always one you supply.'
                  : kind === 'disc' ? 'A canvas can\'t be cut from a disc. Upload one instead.'
                    : 'This server can\'t cut one here: it needs a video file in a library, and an FFmpeg that can encode H.264.'}</p>
              )}
            </section>

            <section aria-label="Upload one">
              <h3>Upload one</h3>
              <p className="muted">An MP4 (H.264) or WebM of up to 30 MB, best kept to 3 to 8 seconds and silent. It's kept in the server's own folder, and is used instead of one beside the media.</p>
              <div className="canvas-admin-row">
                <input ref={file} type="file" accept="video/mp4,video/webm,.mp4,.webm" hidden onChange={(e) => upload(e.target.files?.[0])} />
                <Button busy={busy === 'upload'} disabled={!!busy || !!info.task} onClick={() => file.current?.click()}>Choose a file…</Button>
              </div>
            </section>
          </>
        )}
      </div>
    </Dialog>
  )
}
