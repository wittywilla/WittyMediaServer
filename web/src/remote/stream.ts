// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Reading the server's event streams, and the small requests that go with them. Remote
// control and watch parties both listen on one stream and answer with short posts, many of
// them a second; these skip the caches and the error reports the rest of the app's
// requests go through, which are for pages.
import { ApiError, getToken, serverBase } from '../api'

const headers = (json: boolean) => {
  const h: Record<string, string> = {}
  const tok = getToken()
  if (tok) h.Authorization = 'Bearer ' + tok
  if (json) h['Content-Type'] = 'application/json'
  return h
}

// call sends one request and gives back the answer, or throws an ApiError holding the
// server's words and its `code`.
export async function call<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  let res: Response
  try {
    res = await fetch(serverBase() + '/api' + path, { method, headers: headers(body !== undefined), body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin', signal })
  } catch (e) {
    if (signal?.aborted) throw e
    throw new ApiError(0, "Can't reach your server.")
  }
  const text = await res.text()
  let data: any = null
  try { data = text ? JSON.parse(text) : null } catch { data = { error: text } }
  if (!res.ok) {
    // This profile's time is up: the screen-time watcher (shell/ScreenTime.tsx) says so, as
    // it does when a page's own request is refused for it.
    if (res.status === 403 && data?.code === 'screen-time' && data.screenTime) window.dispatchEvent(new CustomEvent('wms:screen-time', { detail: data.screenTime }))
    throw new ApiError(res.status, data?.error || res.statusText, data)
  }
  return data as T
}

// The server says something at least every 25 seconds. A connection a phone or a proxy
// has quietly dropped says nothing at all, so silence is the only way to notice.
const SILENCE = 60000

// readEvents keeps a stream open and hands over each event until the stream ends, the
// signal says stop, or nothing has come for a minute. It resolves when the stream ends
// and throws when it couldn't be opened.
export async function readEvents(path: string, onEvent: (event: string, data: any) => void, signal: AbortSignal): Promise<void> {
  const ctl = new AbortController()
  const stop = () => ctl.abort()
  signal.addEventListener('abort', stop)
  let quiet: ReturnType<typeof setTimeout> | undefined
  const heard = () => { clearTimeout(quiet); quiet = setTimeout(stop, SILENCE) }
  try {
    const res = await fetch(serverBase() + '/api' + path, { headers: headers(false), credentials: 'same-origin', signal: ctl.signal })
    if (!res.ok) {
      let data: any = null
      try { data = await res.json() } catch { /* not JSON */ }
      throw new ApiError(res.status, data?.error || res.statusText, data)
    }
    if (!res.body) throw new ApiError(0, 'This browser can’t read a stream.')
    const reader = res.body.getReader()
    const text = new TextDecoder()
    let buffer = ''
    heard()
    for (;;) {
      const { value, done } = await reader.read()
      if (done) return
      heard()
      buffer += text.decode(value, { stream: true })
      let end: number
      while ((end = buffer.indexOf('\n\n')) >= 0) {
        const frame = buffer.slice(0, end)
        buffer = buffer.slice(end + 2)
        let event = '', data = ''
        for (const line of frame.split('\n')) {
          if (line.startsWith('event: ')) event = line.slice(7)
          else if (line.startsWith('data: ')) data += line.slice(6)
        }
        if (!event || !data) continue
        let msg: any
        try { msg = JSON.parse(data) } catch { continue }
        onEvent(event, msg)
      }
    }
  } catch (e) {
    if (ctl.signal.aborted) return // stopped on purpose, or gone quiet: the caller decides what next
    throw e
  } finally {
    clearTimeout(quiet)
    signal.removeEventListener('abort', stop)
  }
}

export const sleep = (ms: number, signal?: AbortSignal) => new Promise<void>((resolve) => {
  const t = setTimeout(resolve, ms)
  signal?.addEventListener('abort', () => { clearTimeout(t); resolve() }, { once: true })
})

// A clock that doesn't jump when the device's time is corrected, in the wall clock's
// numbers: what timing a round trip needs.
export const steadyNow = () => performance.timeOrigin + performance.now()
