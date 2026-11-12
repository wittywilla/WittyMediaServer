// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { STATE_NAMES, taskAction, useActivity } from '../activity'
import { Progress, toast } from '../ui'

// Things the server makes for one title while someone waits at its page: a canvas cut from
// a film, a song's stems. Each is a task (docs/TASKS.md), and the title's own listing names
// the task for as long as it runs. So the listing is asked again now and then until the
// task is gone from it, which works for everyone who may start one; the task's own progress
// is shown as well to those who may see tasks.

const POLL_MS = 1500

// useWatched: a listing that is read again while it says something is being made.
// busy says so; changed counts the readings, for whoever wants to act on a new one.
export function useWatched<T>(load: (signal: AbortSignal) => Promise<T>, busy: (v: T) => boolean, key: string) {
  // Kept with the key it was read for: on the way from one title to the next, the last
  // title's listing must not be taken for the new one's.
  const [held, setHeld] = useState<{ key: string; value: T | null; failed: string }>({ key, value: null, failed: '' })
  const [nonce, setNonce] = useState(0)
  const loadRef = useRef(load)
  loadRef.current = load
  const busyRef = useRef(busy)
  busyRef.current = busy
  useEffect(() => {
    if (!key) return
    const stop = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    const ask = () => {
      loadRef.current(stop.signal).then((v) => {
        if (stop.signal.aborted) return
        setHeld({ key, value: v, failed: '' })
        // A hidden tab doesn't keep asking; it asks once on the way back.
        if (busyRef.current(v)) timer = setTimeout(() => { if (document.hidden) document.addEventListener('visibilitychange', ask, { once: true }); else ask() }, POLL_MS)
      }).catch((e: Error) => { if (!stop.signal.aborted) setHeld((h) => ({ key, value: h.key === key ? h.value : null, failed: e.message })) })
    }
    ask()
    return () => { stop.abort(); clearTimeout(timer); document.removeEventListener('visibilitychange', ask) }
  }, [key, nonce])
  const reload = useCallback(() => setNonce((n) => n + 1), [])
  const mine = held.key === key
  return { value: mine ? held.value : null, failed: mine ? held.failed : '', reload }
}

// One task's progress, for whoever is waiting for it. Someone who may not see tasks gets
// a bar that says it is being worked on, and nothing they aren't meant to see.
export function TaskLine({ id, label }: { id: string; label: string }) {
  const a = useActivity()
  const t = a.tasks.find((x) => x.id === id) || a.recent.find((x) => x.id === id)
  const waiting = t?.state === 'queued'
  const words = !t ? 'Working on it…' : waiting ? 'Waiting its turn: the server makes these one at a time.' : t.phase || STATE_NAMES[t.state]
  const stop = () => { taskAction(id, 'cancel').catch((e: Error) => toast(e.message, { tone: 'danger' })) }
  return (
    <div className="job-line" role="status">
      <div className="job-line-top">
        <span>{label}</span>
        {t?.canCancel && !a.access.readOnly && <button type="button" className="btn sm ghost" onClick={stop}>Stop</button>}
      </div>
      <Progress label={label} value={t && t.state === 'running' && t.percent >= 0 ? t.percent : undefined} showValue />
      <span className="job-line-sub">{words}</span>
    </div>
  )
}
