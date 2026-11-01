import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ControllerPicture } from './art'
import type { Background, ControlsData, Target } from './consoles'
import { MAX_PLAYERS, onTV, padSupport, type InputManager, type PadInfo } from './input'
import {
  conflicts, describeKey, describeSource, effectiveKeys, effectiveMap, effectiveOptions, findPress, keyConflicts, noteReleases, rawFromDefaults, resolveProfile, snapshot, standingOf, toStandard,
  type Binding, type ControllerData, type KeyMap, type PadMap, type Profile, type ProfileOptions,
} from './mapping'

// The controller screen: who's on which player, what each button does, and the picture
// behind it all. Everything here works with a D-pad and OK: real buttons in a sensible
// order, steppers instead of sliders and drop-downs, and nothing that needs pointing.

export type ImportResult = { imported: Profile[]; rejected: { index: number; name: string; error: string }[]; notes: string[] }

// What the screen needs from the app. Everything goes through documented routes.
export interface ProfileHost {
  list(): Promise<Profile[]>
  create(p: Partial<Profile>): Promise<Profile>
  update(id: number, p: Profile): Promise<Profile>
  remove(id: number): Promise<void>
  copy(id: number): Promise<Profile>
  setActive(id: number, on: boolean): Promise<Profile>
  exportFile(ids?: number[]): Promise<string>
  importFile(text: string): Promise<ImportResult>
  setBackground(b: Background): Promise<Background>
  uploadBackground(f: File): Promise<Background>
  removeBackground(): Promise<Background>
  backgroundUrl(version: number): string
}

export type PortChoice = { port: number; types: { id: number; name: string }[]; current: number }

type Props = {
  host: ProfileHost
  data: ControlsData
  known: ControllerData | null
  input: InputManager
  consoleName: string
  gameId?: number
  gameTitle?: string
  consolePicker?: React.ReactNode
  devices?: PortChoice[]
  setDevice?: (port: number, id: number) => Promise<void>
  refresh: () => Promise<void> // profiles or the background changed
  onTouch?: () => void
  onClose: () => void
}

type Tab = 'buttons' | 'players' | 'picture'
type Device = number | 'keyboard'

const LISTEN_MS = 7000
const TRY_IDLE_MS = 6000

// While this is held, a pad's buttons mean nothing to the app's own navigation
// (web/src/tv.ts). Let go a moment late, so that what was just pressed isn't also OK.
let tvHolds = 0
function holdTvNavigation(on: boolean) {
  tvHolds += on ? 1 : -1
  if (on) document.documentElement.classList.add('wp-capture')
  else setTimeout(() => { if (tvHolds === 0) document.documentElement.classList.remove('wp-capture') }, 120)
}
const MAX_FILE = 10 << 20
const MAX_IMPORT = 256 << 10

const pct = (v: number) => `${Math.round(v * 100)}%`

// A value changed by pressing + and −, which a remote can do and a slider often can't.
function Stepper({ label, value, text, onChange, min, max, step }: { label: string; value: number; text?: string; onChange: (v: number) => void; min: number; max: number; step: number }) {
  const set = (v: number) => onChange(Math.round(Math.min(max, Math.max(min, v)) / step) * step)
  return (
    <div className="wps-stepper" role="group" aria-label={label}>
      <span className="wp-grow">{label}</span>
      <button type="button" className="wp-btn" disabled={value <= min} aria-label={`Less ${label.toLowerCase()}`} onClick={() => set(value - step)}>−</button>
      <output className="wps-value">{text ?? value}</output>
      <button type="button" className="wp-btn" disabled={value >= max} aria-label={`More ${label.toLowerCase()}`} onClick={() => set(value + step)}>+</button>
    </div>
  )
}

function Toggle({ label, on, onChange, hint }: { label: string; on: boolean; onChange: (v: boolean) => void; hint?: string }) {
  return (
    <button type="button" className={'wps-toggle' + (on ? ' on' : '')} role="switch" aria-checked={on} onClick={() => onChange(!on)}>
      <span className="wp-grow">{label}{hint && <small className="wp-muted"> {hint}</small>}</span>
      <span className="wps-switch" aria-hidden="true">{on ? 'On' : 'Off'}</span>
    </button>
  )
}

export function ControllerStudio(props: Props) {
  const { host, data, input, onClose, refresh } = props
  // Something has to have the focus, or a remote has nothing to move from.
  const root = useRef<HTMLDivElement>(null)
  useEffect(() => { root.current?.querySelector<HTMLElement>('.wps-tabs button[aria-selected=true]')?.focus({ preventScroll: true }) }, [])
  const [tab, setTab] = useState<Tab>('buttons')
  const [note, setNote] = useState('')
  const [bg, setBg] = useState<Background>(data.background)
  useEffect(() => setBg(data.background), [data.background])
  const say = useCallback((t: string) => setNote(t && t[0].toUpperCase() + t.slice(1)), [])
  const tabs: [Tab, string][] = [['buttons', 'Buttons'], ['players', 'Players'], ['picture', 'Background']]

  return (
    <div ref={root} className="wps" role="dialog" aria-label="Controllers">
      <header className="wps-head">
        <h1 className="wps-title">Controllers <span className="wp-muted">· {props.gameTitle || props.consoleName}</span></h1>
        <div className="wps-tabs" role="tablist">
          {tabs.map(([id, name]) => <button key={id} type="button" role="tab" aria-selected={tab === id} className={'wp-btn' + (tab === id ? ' primary' : '')} onClick={() => setTab(id)}>{name}</button>)}
        </div>
        <span className="wp-grow" />
        <button type="button" className="wp-btn" onClick={onClose}>Done</button>
      </header>
      {props.consolePicker}
      <div className="wps-note" role="status" aria-live="polite">{note}</div>
      {tab === 'buttons' && <ButtonsTab {...props} bg={bg} say={say} />}
      {tab === 'players' && <PlayersTab {...props} say={say} />}
      {tab === 'picture' && <PictureTab host={host} bg={bg} setBg={setBg} refresh={refresh} say={say} data={data} input={input} />}
    </div>
  )
}

// ---- the picture, with whatever's behind it ----

function Stage({ bg, host, children }: { bg: Background; host: ProfileHost; children: React.ReactNode }) {
  return (
    <div className="wps-stage">
      {bg.version > 0 && (
        <>
          <img className="wps-bg" src={host.backgroundUrl(bg.version)} alt="" style={{ objectFit: bg.fit, objectPosition: `${bg.x}% ${bg.y}%` }} />
          <div className="wps-dim" style={{ opacity: bg.dim / 100 }} />
        </>
      )}
      {children}
    </div>
  )
}

const noPadWords = (): string => {
  const s = padSupport()
  if (s === 'insecure') return 'No controller can be seen: this page is on plain http, and most browsers only show gamepads to secure pages. Open this server over https, or at localhost on this computer. Until then the keyboard and touch controls work.'
  if (s === 'blocked') return 'This browser is set not to share controllers with this page. Check its site settings.'
  if (s === 'none') return 'This browser has no way to read gamepads. Try a current version of Chrome, Edge, Firefox or Safari.'
  return 'No controller yet. Connect one and press any button on it: browsers only show a pad once it has been pressed.'
}

// ---- buttons ----

function ButtonsTab({ host, data, known, input, consoleName, gameId, gameTitle, bg, refresh, onTouch, say }: Props & { bg: Background; say: (t: string) => void }) {
  const targets = data.targets
  const targetIds = useMemo(() => new Set(targets.map((t) => t.id)), [targets])
  const [pads, setPads] = useState<PadInfo[]>(() => input.listPads())
  const [device, setDevice] = useState<Device>(() => input.listPads()[0]?.index ?? 'keyboard')
  const [profiles, setProfiles] = useState<Profile[] | null>(null)
  const [selected, setSelected] = useState<number | null>(null) // which profile; null is the built-in default
  const [listening, setListening] = useState<number | null>(null)
  const [walk, setWalk] = useState<number[]>([]) // targets left to set, setting them all in turn
  const [renaming, setRenaming] = useState<string | null>(null)
  const [armed, setArmed] = useState('') // what a second press would do
  const [problems, setProblems] = useState<string[]>([])
  const [focused, setFocused] = useState<number | null>(null)
  const hot = useRef(new Map<number, HTMLButtonElement>())
  const file = useRef<HTMLInputElement>(null)
  const pad = typeof device === 'number' ? pads.find((p) => p.index === device) || null : null
  const isKeys = device === 'keyboard'
  const consoleId = data.console
  const tv = useMemo(onTV, [])

  // Hot-plugging, while this screen is open.
  useEffect(() => {
    const refreshPads = () => setPads(input.listPads())
    const prev = input.onChange
    input.onChange = () => { prev?.(); refreshPads() }
    const iv = setInterval(refreshPads, 1000)
    return () => { input.onChange = prev; clearInterval(iv) }
  }, [input])
  useEffect(() => { if (typeof device === 'number' && !pads.some((p) => p.index === device)) setDevice(pads[0]?.index ?? 'keyboard') }, [pads, device])
  useEffect(() => { if (device === 'keyboard' && pads.length === 1 && listening === null) setDevice(pads[0].index) }, [pads.length]) // eslint-disable-line react-hooks/exhaustive-deps

  const load = useCallback(() => host.list().then(setProfiles).catch((e) => say(e?.message || 'Profiles couldn’t be loaded')), [host, say])
  useEffect(() => { void load() }, [load])

  // The profiles that could apply to this controller here.
  const model = isKeys ? 'keyboard' : pad?.model || ''
  const standing = useMemo(() => {
    const raw = typeof device === 'number' ? navigator.getGamepads?.()[device] : null
    return raw ? standingOf(raw, known, navigator.userAgent) : null
  }, [device, known, pads])
  const rawPad = standing?.standing === 'unknown'
  const fitting = useMemo(() => (profiles || []).filter((p) => p.kind === (isKeys ? 'keyboard' : 'gamepad') && (!p.controller || p.controller === model) && (!p.console || p.console === consoleId) && (!p.game || p.game === gameId) && (isKeys || !!p.raw === rawPad)),
    [profiles, isKeys, model, consoleId, gameId, rawPad])

  // What's in use is what the screen opens on.
  const inUse = useMemo(() => resolveProfile(fitting, isKeys ? 'keyboard' : 'gamepad', model, consoleId, gameId || 0, rawPad), [fitting, isKeys, model, consoleId, gameId, rawPad])
  useEffect(() => { setSelected(inUse?.id ?? null) }, [inUse?.id, device]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (profiles && selected !== null && !fitting.some((p) => p.id === selected)) setSelected(inUse?.id ?? null) }, [fitting]) // eslint-disable-line react-hooks/exhaustive-deps
  const profile = fitting.find((p) => p.id === selected) || null

  const defaults = useMemo<PadMap>(() => {
    const raw = typeof device === 'number' ? navigator.getGamepads?.()[device] : null
    return rawPad && raw ? rawFromDefaults(data.defaults.gamepad, raw) : data.defaults.gamepad
  }, [data.defaults.gamepad, rawPad, device, pads.length])
  const map = useMemo<PadMap>(() => (rawPad && !profile ? defaults : effectiveMap(data.defaults.gamepad, targetIds, profile)), [data.defaults.gamepad, targetIds, profile, rawPad, defaults])
  const keys = useMemo<KeyMap>(() => effectiveKeys(data.defaults.keyboard, targetIds, profile), [data.defaults.keyboard, targetIds, profile])
  const clashes = useMemo(() => (isKeys ? keyConflicts(keys) : conflicts(map)), [isKeys, keys, map])
  const options = effectiveOptions(data.defaults.options, profile)
  const style = pad?.style || 'generic'
  const labelOf = useCallback((id: number) => targets.find((t) => t.id === id)?.label || String(id), [targets])
  const describe = useCallback((t: Target) => (isKeys ? describeKey(keys[t.id]) : describeSource(map[t.id], style, rawPad)), [isKeys, keys, map, style, rawPad])

  // Changes are kept as they're made, and tried on the controller straight away. A
  // queue keeps them in order however fast they come.
  const queue = useRef<Promise<unknown>>(Promise.resolve())
  const timer = useRef(0)
  const pending = useRef<Profile | null>(null)
  const flushSave = useCallback(() => {
    const p = pending.current
    if (!p) return
    pending.current = null
    queue.current = queue.current.then(() => host.update(p.id, p)).then((saved) => {
      // A newer change may be waiting: what's on the screen then stays.
      setProfiles((list) => (list || []).map((x) => (x.id === saved.id && pending.current?.id !== saved.id ? saved : x)))
      return refresh()
    }).catch((e) => say(e?.message || 'That couldn’t be saved'))
  }, [host, refresh, say])
  useEffect(() => () => { clearTimeout(timer.current); flushSave() }, [flushSave])

  // A press can land while the first change is still making the profile. It waits, and
  // changes that profile, rather than making another.
  const creating = useRef<Promise<Profile | null> | null>(null)
  const change = useCallback(async (edit: (p: Profile) => Profile) => {
    let cur = profile
    if (!cur && creating.current) {
      cur = await creating.current
      if (!cur) return
      if (pending.current?.id === cur.id) cur = pending.current
    }
    if (!cur) {
      // The first change to a default makes a profile out of it, so that a person gets
      // what they asked for with nothing else to do first.
      const name = isKeys ? `Keyboard · ${consoleName}` : `${pad?.name || 'Controller'} · ${consoleName}`
      const fresh: Partial<Profile> = {
        name: name.slice(0, 60), kind: isKeys ? 'keyboard' : 'gamepad', controller: isKeys ? 'keyboard' : model, controllerName: pad?.name, console: consoleId, raw: !isKeys && rawPad,
        active: true, map: !isKeys && rawPad ? defaults : {}, keys: {}, options: {},
      }
      const making = host.create(edit(fresh as Profile)).catch((e: any) => { say(e?.message || 'That couldn’t be saved'); return null })
      creating.current = making
      const made = await making
      creating.current = null
      if (!made) return
      setProfiles((l) => [made, ...(l || [])])
      setSelected(made.id)
      if (typeof device === 'number') input.previewProfile(device, made)
      void refresh()
      return
    }
    const next = edit(cur)
    setProfiles((l) => (l || []).map((x) => (x.id === next.id ? next : x)))
    if (typeof device === 'number') input.previewProfile(device, next)
    pending.current = next
    clearTimeout(timer.current)
    timer.current = window.setTimeout(flushSave, 300)
  }, [profile, isKeys, consoleName, pad, model, consoleId, rawPad, defaults, host, device, input, refresh, say, flushSave])

  const setBinding = useCallback((id: number, b: Binding | null) => change((p) => ({ ...p, map: { ...(p.map || {}), [id]: b } })), [change])
  const setKey = useCallback((id: number, code: string) => change((p) => {
    const next: KeyMap = {}
    for (const [k, v] of Object.entries(p.keys || {})) if (v !== code) next[Number(k)] = v // one key does one thing
    return { ...p, keys: { ...next, [id]: code } }
  }), [change])
  const setOption = useCallback((patch: Partial<ProfileOptions>) => change((p) => ({ ...p, options: { ...(p.options || {}), ...patch } })), [change])
  // Listening below outlives a render, and must make its change on the profile as it is by then.
  const setBindingNow = useRef(setBinding)
  setBindingNow.current = setBinding
  const setKeyNow = useRef(setKey)
  setKeyNow.current = setKey

  // The two pairs of face buttons people most often want the other way round.
  const swaps = useMemo(() => (data.pad.art === 'six' ? [] : [[0, 8], [1, 9]].filter(([a, b]) => targetIds.has(a) && targetIds.has(b))), [targetIds, data.pad.art]) // six in two rows have no such pairs
  const swapTargets = (a: number, b: number) => {
    void change((p) => ({ ...p, map: { ...(p.map || {}), [a]: map[b] ?? null, [b]: map[a] ?? null } }))
    say(`Swapped ${labelOf(a)} and ${labelOf(b)}`)
  }

  // ---- listening for a press ----

  useEffect(() => {
    if (listening === null) return
    const what = labelOf(listening)
    say(`Press what you want for ${what}. It stops waiting by itself after a few seconds.`)
    const next = () => setWalk((w) => { const [n, ...rest] = w; setListening(n ?? null); return rest })
    const deadline = window.setTimeout(() => { say(walk.length ? `${what} left as it was` : 'Nothing was pressed, so nothing changed'); next() }, LISTEN_MS)
    const cancel = (e: KeyboardEvent) => {
      if (['Escape', 'GoBack', 'BrowserBack', 'Back', 'XF86Back'].includes(e.key) || (tv && e.key === 'Backspace')) {
        e.preventDefault(); e.stopPropagation()
        setWalk([]); setListening(null)
        say('Nothing changed')
        return true
      }
      return false
    }
    if (isKeys) {
      const key = (e: KeyboardEvent) => {
        if (cancel(e)) return
        e.preventDefault(); e.stopPropagation()
        const code = e.code || e.key // remotes often send no code
        void setKeyNow.current(listening, code)
        say(`${what} is now ${describeKey(code)}`)
        next()
      }
      input.setCapturing(true)
      window.addEventListener('keydown', key, true)
      return () => { clearTimeout(deadline); input.setCapturing(false); window.removeEventListener('keydown', key, true) }
    }
    if (typeof device !== 'number') return () => clearTimeout(deadline)
    const get = () => { try { return navigator.getGamepads?.()[device] ?? null } catch { return null } }
    const first = get()
    if (!first) { clearTimeout(deadline); say('This controller isn’t connected any more'); setListening(null); setWalk([]); return }
    const base = snapshot(first)
    const esc = (e: KeyboardEvent) => { cancel(e) }
    input.setCapturing(true)
    holdTvNavigation(true)
    window.addEventListener('keydown', esc, true)
    let finished = false
    // On a timer rather than animation frames, which stop when the tab isn't drawing.
    const iv = window.setInterval(() => {
      const p = get()
      if (!p || finished) return
      noteReleases(p, base)
      const raw = findPress(p, base)
      if (!raw) return
      const st = standingOf(p, known, navigator.userAgent)
      const b = st.standing === 'unknown' || st.standing === 'standard' ? raw : toStandard(raw, st.layout, p)
      finished = true
      clearInterval(iv)
      clearTimeout(deadline)
      if (!b) { say('That button can’t be used here'); next(); return }
      void setBindingNow.current(listening, b)
      say(`${what} is now ${describeSource(b, style, st.standing === 'unknown')}`)
      // Whatever was pressed mustn't also count as OK when this screen gets it back.
      const wait = window.setInterval(() => {
        const q = get()
        if (q && findPress(q, base)) return
        clearInterval(wait)
        next()
      }, 16)
    }, 16)
    return () => {
      clearInterval(iv)
      clearTimeout(deadline)
      window.removeEventListener('keydown', esc, true)
      input.setCapturing(false)
      holdTvNavigation(false)
    }
  }, [listening, device, isKeys]) // eslint-disable-line react-hooks/exhaustive-deps

  // ---- trying it out ----

  // Normally a pad also moves around this screen, and its right-hand button goes back.
  // This lets someone press everything and see it light up, with nothing else happening.
  // It ends by itself, so that someone with only a controller can't get stuck in it.
  const [trying, setTrying] = useState(false)
  useEffect(() => {
    if (!trying) return
    let last = performance.now()
    input.setCapturing(true)
    holdTvNavigation(true)
    say('Press anything on the controller. This stops after a few seconds of nothing.')
    const iv = window.setInterval(() => {
      const r = typeof device === 'number' ? input.reading(device) : null
      if (r && (r.mask !== 0 || r.lx !== 0 || r.ly !== 0 || r.rx !== 0 || r.ry !== 0)) last = performance.now()
      if (performance.now() - last > TRY_IDLE_MS) setTrying(false)
    }, 200)
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack') { e.preventDefault(); e.stopPropagation(); setTrying(false) } else last = performance.now()
    }
    window.addEventListener('keydown', key, true)
    return () => { clearInterval(iv); window.removeEventListener('keydown', key, true); input.setCapturing(false); holdTvNavigation(false); say('') }
  }, [trying, device, input, say])

  // ---- lighting up as buttons are pressed ----

  useEffect(() => {
    let raf = 0
    const lit = new Set<number>()
    const paint = (id: number, on: boolean) => {
      if (on === lit.has(id)) return
      const el = hot.current.get(id)
      if (on) { lit.add(id); el?.setAttribute('data-on', '') } else { lit.delete(id); el?.removeAttribute('data-on') }
    }
    if (isKeys) {
      const of = new Map(Object.entries(keys).map(([k, v]) => [v, Number(k)]))
      const on = (e: KeyboardEvent) => { const id = of.get(e.code); if (id !== undefined) paint(id, true) }
      const off = (e: KeyboardEvent) => { const id = of.get(e.code); if (id !== undefined) paint(id, false) }
      window.addEventListener('keydown', on)
      window.addEventListener('keyup', off)
      return () => { window.removeEventListener('keydown', on); window.removeEventListener('keyup', off); for (const id of [...lit]) paint(id, false) }
    }
    if (typeof device !== 'number') return
    const prev = input.onFrame
    input.onFrame = () => {
      prev?.()
      if (raf) return
      raf = requestAnimationFrame(() => {
        raf = 0
        const r = input.reading(device)
        if (r) for (let id = 0; id < r.held.length; id++) paint(id, !!r.held[id])
      })
    }
    return () => { input.onFrame = prev; cancelAnimationFrame(raf); for (const id of [...lit]) paint(id, false) }
  }, [input, device, isKeys, keys])

  // ---- profiles ----

  const act = async (fn: () => Promise<unknown>, done?: string) => {
    try {
      await fn()
      if (done) say(done)
      await load()
      await refresh()
    } catch (e: any) { say(e?.message || 'That didn’t work') }
  }
  const scopeEdit = (patch: Partial<Profile>) => change((p) => ({ ...p, ...patch }))
  const newProfile = () => act(async () => {
    const created = await host.create({
      name: `${isKeys ? 'Keyboard' : pad?.name || 'Controller'} · ${consoleName}`.slice(0, 60), kind: isKeys ? 'keyboard' : 'gamepad', controller: isKeys ? 'keyboard' : model, controllerName: pad?.name, console: consoleId,
      raw: !isKeys && rawPad, active: true, map: !isKeys && rawPad ? defaults : {}, keys: {}, options: {},
    })
    setSelected(created.id)
  }, 'New profile made')
  const copyProfile = () => profile && act(async () => { const c = await host.copy(profile.id); setSelected(c.id) }, 'Copied')
  const deleteProfile = () => {
    if (!profile) return
    if (armed !== 'delete') { setArmed('delete'); say(`Press Delete again to remove “${profile.name}”`); return }
    setArmed('')
    void act(() => host.remove(profile.id), 'Deleted')
  }
  const resetProfile = () => {
    if (armed !== 'reset') { setArmed('reset'); say('Press Reset again to put every button back to its default'); return }
    setArmed('')
    if (profile) void change((p) => ({ ...p, map: !isKeys && rawPad ? defaults : {}, keys: {}, options: {} }))
    say('Back to the default buttons')
  }
  const exportProfiles = async (ids?: number[]) => {
    try {
      const text = await host.exportFile(ids)
      const a = document.createElement('a')
      a.href = URL.createObjectURL(new Blob([text], { type: 'application/json' }))
      a.download = 'wittyplay-controller-profiles.json'
      a.click()
      setTimeout(() => URL.revokeObjectURL(a.href), 5000)
      say('Saved to your downloads')
    } catch (e: any) { say(e?.message || 'That didn’t work') }
  }
  const importProfiles = async (f: File | undefined) => {
    if (!f) return
    if (f.size > MAX_IMPORT) { say('This file is too large to be a profile file'); return }
    try {
      const r = await host.importFile(await f.text())
      setProblems([...r.notes, ...r.rejected.map((x) => `“${x.name || 'A profile'}” wasn’t added: ${x.error}.`)])
      say(`Added ${r.imported.length} ${r.imported.length === 1 ? 'profile' : 'profiles'}`)
      await load()
      await refresh()
    } catch (e: any) { say(e?.message || 'That file couldn’t be read') }
  }

  // ---- calibration ----

  const [cal, setCal] = useState<null | { rest: number[]; low: number[]; high: number[] }>(null)
  useEffect(() => {
    if (!cal || typeof device !== 'number') return
    const iv = window.setInterval(() => {
      const p = navigator.getGamepads?.()[device]
      if (!p) return
      setCal((c) => c && { ...c, low: c.low.map((v, i) => Math.min(v, p.axes[i] ?? v)), high: c.high.map((v, i) => Math.max(v, p.axes[i] ?? v)) })
    }, 16)
    return () => clearInterval(iv)
  }, [!!cal, device]) // eslint-disable-line react-hooks/exhaustive-deps
  const startCal = () => {
    const p = typeof device === 'number' ? navigator.getGamepads?.()[device] : null
    if (!p) return
    const rest = Array.from(p.axes)
    setCal({ rest, low: [...rest], high: [...rest] })
    say('Move each stick all the way round, then press Finish. Keep hands off the triggers.')
  }
  const finishCal = () => {
    if (!cal) return
    const out: Record<number, [number, number, number]> = {}
    cal.rest.forEach((c, i) => {
      if (Math.abs(c) > 0.6) return // a trigger or a hat, not a stick
      const low = cal.low[i], high = cal.high[i]
      if (high - c > 0.3 && c - low > 0.3) out[i] = [low, c, high]
    })
    setCal(null)
    if (!Object.keys(out).length) { say('Nothing moved far enough, so nothing changed'); return }
    void setOption({ calibration: out })
    say(`Calibrated ${Object.keys(out).length} axes`)
  }

  // ---- what it looks like ----

  const isDefault = !profile
  const list = (
    <ul className="wps-map" aria-label="Every button, as a list">
      {targets.map((t) => {
        const clash = clashes.get(t.id)
        return (
          <li key={t.id} className={clash ? 'clash' : ''}>
            <span className="wps-map-name">{t.label}</span>
            <span className="wp-muted wps-map-src">{describe(t)}{listening === t.id && ' · waiting…'}</span>
            {clash && (
              <span className="wps-clash">
                Also {clash.map(labelOf).join(', ')}
                {!isKeys && clash.length === 1 && (
                  <button type="button" className="wp-btn wps-small" aria-label={`Swap ${t.label} with ${labelOf(clash[0])}`} onClick={() => {
                    // The other takes what this one had by default, so both end up pressed by something.
                    const a = map[t.id], other = clash[0], def = defaults[t.id] ?? null
                    void change((p) => ({ ...p, map: { ...(p.map || {}), [t.id]: a ?? null, [other]: def } }))
                    say(`${labelOf(other)} moved to ${describeSource(def, style, rawPad)}`)
                  }}>Swap</button>
                )}
              </span>
            )}
            <button type="button" className={'wp-btn wps-small' + (listening === t.id ? ' primary' : '')} aria-label={`Change ${t.label}`} onClick={() => setListening(t.id)} disabled={!isKeys && !pad}>{listening === t.id ? 'Press…' : 'Change'}</button>
            {!isKeys && <button type="button" className="wp-btn wps-small" aria-label={`Clear ${t.label}`} onClick={() => { void setBinding(t.id, null); say(`Nothing presses ${t.label} now`) }} disabled={!map[t.id]}>Clear</button>}
          </li>
        )
      })}
    </ul>
  )
  const focusedTarget = targets.find((t) => t.id === (listening ?? focused))

  return (
    <div className="wps-body-grid">
      <section className="wps-left" aria-label="Picture of the controller">
        <Stage bg={bg} host={host}>
          <ControllerPicture art={data.pad.art} targets={targets} active={listening} describe={describe} refs={hot}
            onPick={(id) => { if (isKeys || pad) setListening(id); else say(noPadWords()) }} onFocusTarget={setFocused} />
          <div className="wps-caption" aria-hidden="true">
            {listening !== null ? <b>Press what you want for {labelOf(listening)}</b> : focusedTarget ? <><b>{focusedTarget.label}</b> <span>{describe(focusedTarget)}</span></> : <span>Choose a button, then press what you want for it.</span>}
          </div>
        </Stage>
        <div className="wps-row">
          <button type="button" className="wp-btn" disabled={(!isKeys && !pad) || listening !== null} onClick={() => { const [n, ...rest] = targets.map((t) => t.id); setWalk(rest); setListening(n) }}>Set them all, one by one</button>
          {listening !== null && <button type="button" className="wp-btn" onClick={() => { setWalk([]); setListening(null); say('Nothing changed') }}>Stop</button>}
          <button type="button" className={'wp-btn' + (trying ? ' primary' : '')} aria-pressed={trying} disabled={!pad && !isKeys} onClick={() => setTrying((t) => !t)}>{trying ? 'Stop trying' : 'Try your controller'}</button>
          {onTouch && <button type="button" className="wp-btn" onClick={onTouch}>Edit touch controls…</button>}
        </div>
        {list}
      </section>

      <section className="wps-right" aria-label="Controller and profile">
        <h3>Controller</h3>
        <div className="wps-choices" role="group" aria-label="Which controller">
          {pads.map((p) => (
            <button key={p.index} type="button" aria-pressed={device === p.index} className={'wp-btn' + (device === p.index ? ' primary' : '')} onClick={() => setDevice(p.index)}>{p.name}</button>
          ))}
          <button type="button" aria-pressed={isKeys} className={'wp-btn' + (isKeys ? ' primary' : '')} onClick={() => setDevice('keyboard')}>Keyboard</button>
        </div>
        {pads.length === 0 && <p className="wp-muted wps-para" role="status">{noPadWords()}</p>}
        {pad && pad.standing === 'known' && <p className="wp-muted wps-para">Recognised, and its buttons were put in order for you. If any are wrong, set them here.</p>}
        {pad && pad.standing === 'unknown' && <p className="wps-para wps-warn">This browser doesn’t say what this controller’s buttons are, so they may be in the wrong places. Choose “Set them all, one by one” to be sure.</p>}

        <h3>Profile</h3>
        <div className="wps-profiles" role="group" aria-label="Which profile">
          <button type="button" aria-pressed={isDefault} className={'wp-btn' + (isDefault ? ' primary' : '')} onClick={() => setSelected(null)}>Default</button>
          {fitting.map((p) => (
            <button key={p.id} type="button" aria-pressed={selected === p.id} className={'wp-btn' + (selected === p.id ? ' primary' : '')} onClick={() => setSelected(p.id)}>
              {p.name}{p.active && <small className="wps-tag">in use</small>}
            </button>
          ))}
        </div>
        <div className="wps-row wps-wrap">
          <button type="button" className="wp-btn" onClick={newProfile}>New</button>
          <button type="button" className="wp-btn" disabled={!profile} onClick={copyProfile}>Copy</button>
          <button type="button" className="wp-btn" disabled={!profile} onClick={() => setRenaming(profile?.name || '')}>Rename</button>
          <button type="button" className="wp-btn" onClick={resetProfile}>Reset</button>
          <button type="button" className="wp-btn" disabled={!profile} onClick={deleteProfile}>{armed === 'delete' ? 'Delete, really' : 'Delete'}</button>
          {!tv && <button type="button" className="wp-btn" disabled={!profiles?.length} onClick={() => void exportProfiles(profile ? [profile.id] : undefined)}>Export</button>}
          {!tv && <button type="button" className="wp-btn" onClick={() => file.current?.click()}>Import</button>}
          <input ref={file} type="file" accept=".json,application/json" hidden aria-label="Import a profile file" onChange={(e) => { void importProfiles(e.target.files?.[0]); e.target.value = '' }} />
        </div>
        {renaming !== null && profile && (
          <form className="wps-row" onSubmit={(e) => { e.preventDefault(); const name = renaming.trim(); if (name) void change((p) => ({ ...p, name })); setRenaming(null) }}>
            <label className="wp-grow">Name <input className="wp-select wps-name" autoFocus maxLength={60} value={renaming} onChange={(e) => setRenaming(e.target.value)} /></label>
            <button type="submit" className="wp-btn primary">Save</button>
          </form>
        )}
        {problems.length > 0 && <ul className="wps-problems" role="alert">{problems.map((p, i) => <li key={i}>{p}</li>)}</ul>}

        {profile && (
          <div className="wps-group">
            <Toggle label="Use this profile" on={profile.active} onChange={(on) => void act(() => host.setActive(profile.id, on), on ? `Using “${profile.name}”` : 'No longer using it')} />
            {!isKeys && !rawPad && <Toggle label="Only this model of controller" hint={pad?.name} on={!!profile.controller} onChange={(on) => void scopeEdit({ controller: on ? model : '', controllerName: on ? pad?.name : undefined })} />}
            <Toggle label={`Only ${consoleName}`} on={!!profile.console} onChange={(on) => void scopeEdit({ console: on ? consoleId : undefined, game: on ? profile.game : undefined })} />
            {!!gameId && <Toggle label="Only this game" hint={gameTitle} on={!!profile.game} onChange={(on) => void scopeEdit({ game: on ? gameId : undefined, console: on ? consoleId : profile.console })} />}
          </div>
        )}
        <p className="wp-muted wps-para">The most exact profile in use wins: one for this game, then this console, then this controller, then the default.</p>

        {!isKeys && swaps.length > 0 && (
          <>
            <h3>Go by the letters</h3>
            <p className="wp-muted wps-para">Buttons are matched by where they are on the pad, which is how games were designed to be held. If you’d rather go by what’s printed on yours, swap them.</p>
            <div className="wps-row wps-wrap">
              {swaps.map(([a, b]) => <button key={a} type="button" className="wp-btn" onClick={() => swapTargets(a, b)}>Swap {labelOf(a)} and {labelOf(b)}</button>)}
            </div>
          </>
        )}

        <h3>How it feels</h3>
        <div className="wps-group">
          {!isKeys && <Stepper label="Stick dead zone" value={Math.round((options.deadzone ?? 0.15) * 100)} text={pct(options.deadzone ?? 0.15)} min={0} max={60} step={5} onChange={(v) => void setOption({ deadzone: v / 100 })} />}
          {!isKeys && <Stepper label="Press point" value={Math.round((options.threshold ?? 0.5) * 100)} text={pct(options.threshold ?? 0.5)} min={10} max={90} step={5} onChange={(v) => void setOption({ threshold: v / 100 })} />}
          {!isKeys && !data.pad.sticks && <Toggle label="Left stick steers the D-pad too" on={options.stickDpad ?? true} onChange={(on) => void setOption({ stickDpad: on })} />}
          {!isKeys && data.pad.sticks && <Toggle label="Left stick steers the D-pad too" on={options.stickDpad ?? false} onChange={(on) => void setOption({ stickDpad: on })} />}
          {!isKeys && pad?.rumble && <Toggle label="Rumble" on={!options.noRumble} onChange={(on) => void setOption({ noRumble: !on })} />}
          {!isKeys && pad?.rumble && <button type="button" className="wp-btn" onClick={() => input.testRumble(device as number)}>Try the rumble</button>}
          {!isKeys && pad && (cal
            ? <button type="button" className="wp-btn primary" onClick={finishCal}>Finish calibrating</button>
            : <div className="wps-row">
                <button type="button" className="wp-btn" onClick={startCal}>Calibrate sticks</button>
                {options.calibration && <button type="button" className="wp-btn" onClick={() => { void setOption({ calibration: undefined }); say('Calibration forgotten') }}>Forget calibration</button>}
              </div>)}
        </div>
        <fieldset className="wps-turbo">
          <legend>Turbo: these repeat while held</legend>
          {targets.filter((t) => t.id < 16 && (t.group === 'face' || t.group === 'shoulder')).map((t) => {
            const on = !!options.turbo?.includes(t.id)
            return <button key={t.id} type="button" aria-pressed={on} className={'wp-btn wps-small' + (on ? ' primary' : '')} onClick={() => void setOption({ turbo: on ? (options.turbo || []).filter((x) => x !== t.id) : [...(options.turbo || []), t.id] })}>{t.label}</button>
          })}
          {!!options.turbo?.length && <Stepper label="Presses a second" value={options.turboHz ?? 12} min={2} max={30} step={2} onChange={(v) => void setOption({ turboHz: v })} />}
        </fieldset>
      </section>
    </div>
  )
}

// ---- players ----

function PlayersTab({ input, devices, setDevice, say }: Props & { say: (t: string) => void }) {
  const [pads, setPads] = useState<PadInfo[]>(() => input.listPads())
  const [, tick] = useState(0)
  useEffect(() => {
    const on = () => { setPads(input.listPads()); tick((x) => x + 1) }
    const prev = input.onChange
    input.onChange = () => { prev?.(); on() }
    const iv = setInterval(on, 1000) // some browsers don't fire connection events
    return () => { input.onChange = prev; clearInterval(iv) }
  }, [input])
  const players = Array.from({ length: MAX_PLAYERS }, (_, i) => i)
  return (
    <div className="wps-players">
      <p className="wp-muted wps-para">Choosing a player that’s taken swaps the two controllers.</p>
      <ul className="wp-list">
        {pads.map((p) => (
          <li key={p.index + p.id}>
            <div className="wp-grow"><b>{p.name}</b>
              <small className="wp-muted">{p.profile ? `Profile: ${p.profile.name}` : p.standing === 'unknown' ? 'Buttons not known: set them up on the Buttons tab.' : p.standing === 'known' ? 'Recognised' : 'Standard buttons'}</small></div>
            <div className="wps-seats" role="group" aria-label={`${p.name}: which player`}>
              {players.map((n) => <button key={n} type="button" aria-pressed={p.port === n} aria-label={`Player ${n + 1}`} className={'wp-btn wps-small' + (p.port === n ? ' primary' : '')} onClick={() => input.setPadPort(p.index, n)}>{n + 1}</button>)}
            </div>
            {p.rumble && <button type="button" className="wp-btn wps-small" onClick={() => input.testRumble(p.index)}>Which one?</button>}
          </li>
        ))}
        {pads.length === 0 && <li className="wp-muted" role="status">{noPadWords()}</li>}
        <li>
          <div className="wp-grow"><b>Keyboard</b></div>
          <div className="wps-seats" role="group" aria-label="Keyboard: which player">
            {players.map((n) => <button key={n} type="button" aria-pressed={input.keyboardPlayer === n} aria-label={`Player ${n + 1}`} className={'wp-btn wps-small' + (input.keyboardPlayer === n ? ' primary' : '')} onClick={() => input.setKeyboardPort(n)}>{n + 1}</button>)}
          </div>
        </li>
        <li>
          <div className="wp-grow"><b>Touch controls</b></div>
          <div className="wps-seats" role="group" aria-label="Touch controls: which player">
            {players.map((n) => <button key={n} type="button" aria-pressed={input.touchPlayer === n} aria-label={`Player ${n + 1}`} className={'wp-btn wps-small' + (input.touchPlayer === n ? ' primary' : '')} onClick={() => input.setTouchPort(n)}>{n + 1}</button>)}
          </div>
        </li>
      </ul>
      {devices && devices.some((d) => d.types.length > 1) && (
        <>
          <h3>What each player has plugged in</h3>
          <p className="wp-muted wps-para">This game’s emulator offers a choice of controller for its ports.</p>
          <ul className="wp-list">
            {devices.filter((d) => d.types.length > 1).map((d) => (
              <li key={d.port}>
                <div className="wp-grow"><b>Player {d.port + 1}</b></div>
                <div className="wps-seats" role="group" aria-label={`Player ${d.port + 1}: kind of controller`}>
                  {d.types.map((t) => <button key={t.id} type="button" aria-pressed={d.current === t.id} className={'wp-btn wps-small' + (d.current === t.id ? ' primary' : '')}
                    onClick={() => void setDevice?.(d.port, t.id).catch((e) => say(e?.message || 'That didn’t work'))}>{t.name}</button>)}
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}

// ---- the background ----

function PictureTab({ host, bg, setBg, refresh, say, data }: { host: ProfileHost; bg: Background; setBg: (b: Background) => void; refresh: () => Promise<void>; say: (t: string) => void; data: ControlsData; input: InputManager }) {
  const file = useRef<HTMLInputElement>(null)
  const hot = useRef(new Map<number, HTMLButtonElement>())
  const [busy, setBusy] = useState(false)
  const timer = useRef(0)
  const tv = useMemo(onTV, [])
  useEffect(() => () => clearTimeout(timer.current), [])
  const change = (patch: Partial<Background>) => {
    const next = { ...bg, ...patch }
    setBg(next)
    clearTimeout(timer.current)
    timer.current = window.setTimeout(() => { host.setBackground(next).then(() => refresh()).catch((e) => say(e?.message || 'That couldn’t be saved')) }, 300)
  }
  const upload = async (f: File | undefined) => {
    if (!f) return
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(f.type)) { say('Use a PNG, JPEG or WebP picture'); return }
    if (f.size > MAX_FILE) { say('The picture must be 10 MB or smaller'); return }
    setBusy(true)
    try {
      setBg(await host.uploadBackground(f))
      await refresh()
      say('Picture added')
    } catch (e: any) { say(e?.message || 'That picture couldn’t be used') } finally { setBusy(false) }
  }
  const remove = async () => {
    try { setBg(await host.removeBackground()); await refresh(); say('Picture removed') } catch (e: any) { say(e?.message || 'That didn’t work') }
  }
  return (
    <div className="wps-body-grid">
      <section className="wps-left" aria-label="How it looks">
        <div inert>
          <Stage bg={bg} host={host}>
            <ControllerPicture art={data.pad.art} targets={data.targets} active={null} describe={() => 'shown as an example'} refs={hot} onPick={() => {}} />
          </Stage>
        </div>
      </section>
      <section className="wps-right">
        <h3>Your own background</h3>
        <p className="wp-muted wps-para">A picture of your own behind the controller, on this screen only. It’s kept on the server for your account, and nobody else can see it. PNG, JPEG or WebP, up to 10 MB.</p>
        <div className="wps-row wps-wrap">
          {!tv && <button type="button" className="wp-btn primary" disabled={busy} onClick={() => file.current?.click()}>{bg.version ? 'Choose another…' : 'Choose a picture…'}</button>}
          {tv && !bg.version && <p className="wp-muted wps-para">Add a picture from a computer or phone, and it follows you here.</p>}
          {bg.version > 0 && <button type="button" className="wp-btn" onClick={remove}>Remove it</button>}
          <input ref={file} type="file" accept="image/png,image/jpeg,image/webp" hidden aria-label="Choose a picture" onChange={(e) => { void upload(e.target.files?.[0]); e.target.value = '' }} />
        </div>
        {bg.version > 0 && (
          <div className="wps-group">
            <Toggle label="Fill the whole area" hint="Otherwise all of it shows" on={bg.fit === 'cover'} onChange={(on) => change({ fit: on ? 'cover' : 'contain' })} />
            <Stepper label="Left to right" value={bg.x} text={`${bg.x}%`} min={0} max={100} step={10} onChange={(x) => change({ x })} />
            <Stepper label="Top to bottom" value={bg.y} text={`${bg.y}%`} min={0} max={100} step={10} onChange={(y) => change({ y })} />
            <Stepper label="Dimming" value={bg.dim} text={`${bg.dim}%`} min={0} max={90} step={10} onChange={(dim) => change({ dim })} />
          </div>
        )}
      </section>
    </div>
  )
}
