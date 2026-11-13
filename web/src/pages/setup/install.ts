// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { ApiError } from '../../api'
import { currentRun, failureOf, installModule, type Failure, type Run } from '../settings/store/storeApi'
import { notInstalledWords, olderOf, type OlderVersion } from '../settings/store/olderServer'

// Installing a module the way the store does, from anywhere that isn't the store: the same request,
// the same signature checks, the same run to follow. A run is one module and whatever it's built
// on, and only one goes at a time, so modules wait their turn here.

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

// declined: the only version was made for an older server, and nobody chose Install anyway.
export type InstallResult = { ok: boolean; failure?: Failure; declined?: boolean }

// AskOlder asks a person about versions made for an older server (useOlderAsk), and answers the
// module ids to install anyway.
export type AskOlder = (list: OlderVersion[]) => Promise<string[]>

async function followRun(onProgress?: (r: Run) => void): Promise<Run | undefined> {
  for (;;) {
    await wait(400)
    const r = await currentRun().catch(() => undefined)
    if (r) onProgress?.(r)
    if (!r || r.finished) return r
  }
}

const declined = (list: OlderVersion[]): InstallResult => ({
  ok: false, declined: true,
  failure: { kind: 'older-server', title: 'Not installed',message: list.map(notInstalledWords).join(' '), try: ['Install it later from the module store, once a version made for this server is published.'] },
})

// installAndWait installs one module and what it's built on. A list that has already said what's
// being chosen doesn't need to ask again, so it agrees to bringing the others along. A version made
// for an older server is never sent without confirmOlder: when the server names one, askOlder (the
// pop-up) decides, and with no way to ask, it isn't installed. confirmOlder says the person already
// chose Install anyway for this module.
export async function installAndWait(id: string, name: string, onProgress?: (r: Run) => void, opts?: { confirmOlder?: boolean; askOlder?: AskOlder }): Promise<InstallResult> {
  let confirmOlder = !!opts?.confirmOlder
  const send = () => installModule(id, true, confirmOlder ? { confirmOlder: true } : undefined)
  for (let tries = 0; ; tries++) {
    try {
      await send()
      break
    } catch (e) {
      const older = olderOf(e)
      if (older) {
        if (confirmOlder || !opts?.askOlder || tries > 1) return declined(older)
        const yes = await opts.askOlder(older)
        // A store install is one plan: every version in it made for an older server has to be agreed to.
        if (!older.every((o) => yes.includes(o.module))) return declined(older)
        confirmOlder = true
        continue
      }
      // Something else is being installed: wait for it, and go again.
      if (e instanceof ApiError && e.status === 409 && e.data?.code !== 'plan' && tries === 0) {
        await followRun()
        continue
      }
      return { ok: false, failure: failureOf(e, name) }
    }
  }
  const run = await followRun(onProgress)
  const failed = run?.items.find((x) => x.failure)
  return failed ? { ok: false, failure: failed.failure } : { ok: true }
}
