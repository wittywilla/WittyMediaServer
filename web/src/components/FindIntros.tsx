// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { taskAction, useActivity, type Task } from '../activity'
import { api } from '../api'
import { HelpLink } from './HelpLink'
import { toast } from '../state'
import { Button, Dialog, Progress, Select } from '../ui'
import './findintros.css'

// Find intros and credits: the episodes of a season are compared by ear and what they share
// is marked (docs/PLAYING.md). An administrator starts it from a show's page, for every
// season or one. It runs as a task, so this shows how far it has got, lets it be stopped,
// and says what it found. Closing the window doesn't stop it.

const RUNNING = new Set<Task['state']>(['queued', 'running', 'paused'])

export default function FindIntros({ showId, title, seasons, season, onClose }: { showId: number; title: string; seasons: { number: number; title: string }[]; season: number; onClose: () => void }) {
  const a = useActivity()
  const [which, setWhich] = useState('all')
  const [taskId, setTaskId] = useState('')
  const [count, setCount] = useState<{ episodes: number; seasons: number } | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  // One run at a time on the whole server: one already under way is shown rather than refused.
  const under = a.tasks.find((t) => t.type === 'intros' && RUNNING.has(t.state))
  const task = (taskId ? a.tasks.find((t) => t.id === taskId) || a.recent.find((t) => t.id === taskId) : undefined) || (!taskId ? under : undefined)
  const running = !!task && RUNNING.has(task.state)
  const mine = !!task && task.item === title
  useEffect(() => { if (task && !taskId) setTaskId(task.id) }, [task, taskId])

  const start = async () => {
    setBusy(true); setErr('')
    try {
      const r = await api.post<{ task: string; episodes: number; seasons: number }>(`/shows/${showId}/detect-intros`, which === 'all' ? {} : { season: Number(which) })
      setTaskId(r.task); setCount({ episodes: r.episodes, seasons: r.seasons })
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const stop = async () => { if (task) try { await taskAction(task.id, 'cancel') } catch (e: any) { setErr(e.message) } }
  const close = () => { if (running || (taskId && !task)) toast('Still looking for intros and credits. Activity shows how far it has got.'); onClose() }
  const again = () => { setTaskId(''); setCount(null); setErr('') }
  const started = !!taskId
  return (
    <Dialog title="Find intros and credits" description={title} onClose={close}
      actions={!started
        ? <><Button variant="ghost" onClick={onClose}>Cancel</Button><Button variant="primary" busy={busy} onClick={start}>Start</Button></>
        : running
          ? <><Button variant="ghost" disabled={!task?.canCancel} onClick={stop}>Stop</Button><Button variant="primary" onClick={close}>Close</Button></>
          : <><Button variant="ghost" onClick={again}>Look again</Button><Button variant="primary" onClick={onClose}>Close</Button></>}>
      <div className="find-intros">
        {!started && (
          <>
            <p>The server listens to the start and end of each episode and marks the stretch a season’s episodes share: the title music as the intro, and the same at the end as the credits. It takes a while, runs quietly in the background, and gives way while someone is watching. <HelpLink to="playing#finding-intros-nothing-marks">How it works</HelpLink></p>
            {seasons.length > 1 && (
              <label className="find-intros-which">Which episodes
                <Select value={which} onChange={(e) => setWhich(e.target.value)}>
                  <option value="all">Every season</option>
                  {seasons.map((s) => <option key={s.number} value={s.number}>{s.title}{s.number === season ? ' (the one shown)' : ''}</option>)}
                </Select>
              </label>
            )}
            <p className="muted">What it finds is trusted less than a file’s chapters and anything you set by hand, so those stay as they are.</p>
          </>
        )}
        {started && !task && (a.loaded && !a.access.visible
          ? <p role="status">It has started{count ? `, on ${count.episodes} episodes` : ''}. Activity is turned off for this account (<Link to="/settings/activity">Settings, Activity</Link>), so how far it has got can’t be shown here.</p>
          : <><Progress label="Finding intros and credits" /><p className="muted" role="status">Starting…</p></>)}
        {task && running && (
          <>
            {!mine && <p className="muted">Intros are already being looked for{task.item ? <> in <b>{task.item}</b></> : ''}. Only one show is done at a time.</p>}
            <Progress label="Finding intros and credits" value={task.total > 0 ? task.done : undefined} max={task.total || 1} showValue />
            <p role="status" className="find-intros-line">{[task.state === 'paused' ? 'Paused' : task.state === 'queued' ? 'Waiting its turn' : task.phase, task.total > 0 ? `${task.done} of ${task.total} episodes listened to` : ''].filter(Boolean).join(' · ')}</p>
          </>
        )}
        {task && !running && (
          <div role="status">
            {task.state === 'done' && <p className="find-intros-result">{task.result || 'Finished.'}</p>}
            {task.state === 'cancelled' && <p>Stopped. What was found before then has been kept.</p>}
            {task.state === 'failed' && <div className="error">It couldn’t finish: {task.error || 'something went wrong'}{task.fix ? ` ${task.fix}` : ''}</div>}
            {task.state === 'done' && <p className="muted">Episodes with an intro now show Skip intro while it plays. Run it again after adding episodes.</p>}
          </div>
        )}
        {!!task?.problems?.length && (
          <ul className="find-intros-problems" aria-label="Episodes that couldn’t be read">
            {task.problems.map((p, i) => <li key={i}>{p.item ? <b>{p.item}: </b> : null}{p.message}</li>)}
          </ul>
        )}
        {err && <div className="error" role="alert">{err}</div>}
      </div>
    </Dialog>
  )
}
