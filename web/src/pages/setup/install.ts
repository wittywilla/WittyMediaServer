import { ApiError } from '../../api'
import { currentRun, failureOf, installModule, type Failure, type Run } from '../settings/store/storeApi'

// Installing a module the way the store does, from anywhere that isn't the store: the same request,
// the same signature checks, the same run to follow. A run is one module and whatever it's built
// on, and only one goes at a time, so modules wait their turn here.

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

export type InstallResult = { ok: boolean; failure?: Failure }

async function followRun(onProgress?: (r: Run) => void): Promise<Run | undefined> {
  for (;;) {
    await wait(400)
    const r = await currentRun().catch(() => undefined)
    if (r) onProgress?.(r)
    if (!r || r.finished) return r
  }
}

// installAndWait installs one module and what it's built on. A list that has already said what's
// being chosen doesn't need to ask again, so it agrees to bringing the others along.
export async function installAndWait(id: string, name: string, onProgress?: (r: Run) => void): Promise<InstallResult> {
  try {
    await installModule(id, true)
  } catch (e) {
    // Something else is being installed: wait for it, and go again.
    if (e instanceof ApiError && e.status === 409 && e.data?.code !== 'plan') {
      await followRun()
      try { await installModule(id, true) } catch (again) { return { ok: false, failure: failureOf(again, name) } }
    } else return { ok: false, failure: failureOf(e, name) }
  }
  const run = await followRun(onProgress)
  const failed = run?.items.find((x) => x.failure)
  return failed ? { ok: false, failure: failed.failure } : { ok: true }
}
