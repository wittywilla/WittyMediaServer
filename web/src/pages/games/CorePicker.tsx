import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { api } from '../../api'
import { Spinner } from '../../components/common'
import { IconCheck, IconInfo } from '../../icons'
import { toast } from '../../state'
import { Sheet } from '../../ui'
import { drawsWords, platformName, type CoreSupport } from './platforms'
import './corepicker.css'

// The core picker: which core plays one game, or a console's games unless a game says otherwise.
// Each core says what it's good at, where it runs and how it draws, and whether it needs a BIOS or
// isn't installed yet. Choosing one that isn't installed installs it, with its progress, and then
// uses it. During a game, choosing another offers to restart on it. It's a sheet: ×, Escape, a
// controller's B or a remote's Back, or a tap outside closes it. Administrators only; the server
// answers 403 to anyone else. The API is docs/GAMING-API.md, "Choosing a core".

export type CoreChoice = {
  id: string; name: string; license: string; nonCommercial?: boolean
  installed: boolean; source?: string; canInstall?: boolean; recommended?: boolean
  strength?: 'accuracy' | 'speed' | 'balanced'; about?: string
  support?: CoreSupport; bios?: { files: string[]; found: boolean }
  starts?: number; failures?: number; seconds?: number; lastError?: string; best?: boolean
}
export type CoreChoices = { console: string; choice: string; consoleDefault?: string; effective: string; canDownload: boolean; choices: CoreChoice[] }

export type CorePickerScope =
  | { kind: 'game'; itemId: number; title: string; consoleShort: string }
  | { kind: 'console'; consoleId: string; consoleShort: string }

// What the picker settled on: the core now chosen, and, during a game, whether to restart on it.
export type CorePicked = { core: string; restart: boolean }

type SaveSlot = { slot: number; core?: string }
type Progress = { received: number; total: number } | null

const STRENGTH: Record<string, string> = { accuracy: 'Accuracy', speed: 'Speed', balanced: 'Balanced' }

const and = (l: string[]) => (l.length < 2 ? l.join('') : `${l.slice(0, -1).join(', ')} and ${l[l.length - 1]}`)
const mb = (n: number) => (n >= 1e6 ? `${(n / 1e6).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1e3))} KB`)
const hours = (sec: number) => (sec < 3600 ? `${Math.max(1, Math.round(sec / 60))} min` : `${(sec / 3600).toFixed(sec < 36000 ? 1 : 0)} h`)

// How it draws here, in a few words: "Drawn by the processor here."
function hereLine(s?: CoreSupport) {
  if (!s) return ''
  const h = s.here
  if (!h.build) return h.note || 'Not published for this server.'
  if (h.draws === 'none') return h.note || 'Can’t draw on this server.'
  return `Here: drawn ${drawsWords[h.draws]}${h.draws === 'gpu' && h.api === 'opengl' ? ' (OpenGL)' : ''}.`
}

// Which server systems it's published for, by name only: "Runs on Windows, Linux and macOS."
function runsLine(s?: CoreSupport) {
  if (!s) return ''
  const names: string[] = []
  for (const p of s.platforms) {
    if (!p.build || p.draws === 'none') continue
    const n = platformName({ os: p.os })
    if (!names.includes(n)) names.push(n)
  }
  return names.length ? `Runs on ${and(names)}.` : ''
}

// Save states only load in the core that made them; the slots say which made each, by its name.
const sameCore = (made: string | undefined, c: CoreChoice) => {
  if (!made) return true
  const a = made.toLowerCase(), b = c.name.toLowerCase()
  return a === b || a.startsWith(b) || b.startsWith(a)
}

export default function CorePicker({ scope, onClose, playing, onPicked }: {
  scope: CorePickerScope
  onClose: (picked?: CorePicked) => void
  playing?: boolean // during a game: a change offers to restart on the new core
  onPicked?: (core: string) => void
}) {
  const [info, setInfo] = useState<CoreChoices | null>(null)
  const [failed, setFailed] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState('')
  const [progress, setProgress] = useState<Progress>(null)
  const [restart, setRestart] = useState<CoreChoice | null>(null)
  const [states, setStates] = useState<SaveSlot[]>([])
  const listRef = useRef<HTMLDivElement>(null)
  const alive = useRef(true)
  useEffect(() => () => { alive.current = false }, [])

  const base = scope.kind === 'game' ? `/gaming/games/${scope.itemId}/core` : `/gaming/consoles/${encodeURIComponent(scope.consoleId)}/core`
  const load = useCallback(() => api.get<CoreChoices>(base).then((v) => { if (alive.current) setInfo(v) }).catch((e: any) => { if (alive.current) setFailed(e.message) }), [base])
  useEffect(() => { void load() }, [load])
  useEffect(() => {
    if (scope.kind === 'game') api.get<SaveSlot[]>(`/games/${scope.itemId}/saves`).then((s) => { if (alive.current) setStates(s) }).catch(() => {})
  }, [scope])
  // The controller and the remote start on what's chosen now.
  useEffect(() => {
    if (!info) return
    listRef.current?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus({ preventScroll: false })
  }, [!!info]) // eslint-disable-line react-hooks/exhaustive-deps

  const title = scope.kind === 'game' ? `Core for ${scope.title}` : `Core for ${scope.consoleShort} games`
  const byId = (id: string) => info?.choices.find((c) => c.id === id)

  // Saves the choice. During a game, a change to what plays offers to restart on it.
  const save = async (core: string) => {
    const before = info?.effective
    await api.put(base, { core })
    onPicked?.(core)
    const v = await api.get<CoreChoices>(base)
    if (alive.current) setInfo(v)
    const now = v.choices.find((c) => c.id === v.effective)
    if (playing && now && v.effective !== before) {
      if (alive.current) setRestart(now)
      return
    }
    const c = v.choices.find((x) => x.id === core)
    toast(!core ? (scope.kind === 'game' ? `Plays with the same core as other ${scope.consoleShort} games` : 'The first core installed plays these games') : `${c?.name || core} ${scope.kind === 'game' ? 'plays this game' : `plays ${scope.consoleShort} games`} from the next start`)
  }

  const choose = async (id: string) => {
    if (busy) return
    const c = byId(id)
    if (id && c && !c.installed) {
      if (c.canInstall) setConfirm(id)
      return
    }
    setConfirm('')
    setBusy(id || 'auto')
    try { await save(id) } catch (e: any) { toast(e.message) } finally { if (alive.current) setBusy('') }
  }

  // Install, showing how far the download has got, then use it. The install goes on if the picker
  // is closed meanwhile, and the core is still put to use.
  const install = async (c: CoreChoice) => {
    setBusy(c.id)
    setProgress({ received: 0, total: 0 })
    let polling = true
    const poll = async () => {
      while (polling) {
        try {
          const p = await api.get<{ installing: boolean; received: number; total: number }>(`/gaming/cores/${c.id}/install`)
          if (polling && alive.current && p.installing) setProgress({ received: p.received, total: p.total })
        } catch { /* the install itself says what went wrong */ }
        await new Promise((r) => setTimeout(r, 250))
      }
    }
    void poll()
    try {
      await api.post(`/gaming/cores/${c.id}/install`)
      polling = false
      toast(`${c.name} installed`)
      if (alive.current) setConfirm('')
      await save(c.id)
    } catch (e: any) {
      polling = false
      // A core with no build here says so, naming one that plays the console instead.
      toast(e.message)
    } finally {
      polling = false
      if (alive.current) { setBusy(''); setProgress(null) }
    }
  }

  // Up and Down move between the choices, as in any list of options.
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    const rows = [...(listRef.current?.querySelectorAll<HTMLElement>('.cp-row:not(:disabled)') || [])]
    const i = rows.indexOf(document.activeElement as HTMLElement)
    if (i < 0) return
    const next = rows[i + (e.key === 'ArrowDown' ? 1 : -1)]
    if (next) { e.preventDefault(); next.focus() }
  }

  if (restart) {
    const others = [...new Set(states.filter((s) => s.core && !sameCore(s.core, restart)).map((s) => s.core!))]
    const resumeFrom = states.find((s) => s.slot === 0)
    return (
      <Sheet title={`Restart on ${restart.name}?`} onClose={() => onClose({ core: restart.id, restart: false })} className="cp-sheet"
        actions={<>
          <button type="button" className="btn ghost" onClick={() => onClose({ core: restart.id, restart: false })}>Keep playing</button>
          <button type="button" className="btn primary" autoFocus onClick={() => onClose({ core: restart.id, restart: true })}>Restart now</button>
        </>}>
        <div className="cp-restart">
          <p>{scope.kind === 'game' ? scope.title : 'This game'} will play on <b>{restart.name}</b> from now on. Restart now to switch, or keep playing and it switches the next time the game starts.</p>
          <ul className="gm-checks">
            <li className="ok"><IconCheck size={16} /><span><b>In-game saves stay</b><small>Battery saves and memory cards belong to the game, whichever core plays it.</small></span></li>
            <li className="info"><IconInfo size={16} /><span><b>Save states stay with their core</b><small>{others.length
              ? `Your save states made with ${and(others)}${resumeFrom && !sameCore(resumeFrom.core, restart) ? ', including where you left off,' : ''} only load in ${others.length === 1 ? 'that core' : 'those cores'}. They’re kept for when you switch back.`
              : 'A save state only loads in the core that made it. Those you make now are for this core.'}</small></span></li>
          </ul>
        </div>
      </Sheet>
    )
  }

  const autoSelected = !!info && !info.choice
  // What the first row comes to: the console's default, or else the first core installed.
  const firstInstalled = info?.choices.find((c) => c.installed && c.source !== 'custom')
  const consoleDefault = (info?.consoleDefault && byId(info.consoleDefault)) || undefined
  const autoName = scope.kind === 'game'
    ? `Same as other ${scope.consoleShort} games`
    : 'Automatic'
  const autoSub = scope.kind === 'game'
    ? consoleDefault
      ? `${consoleDefault.name}, chosen for every ${scope.consoleShort} game in Settings, Consoles & cores.`
      : firstInstalled ? `${firstInstalled.name}, the first core installed for ${scope.consoleShort}.` : `The first core installed for ${scope.consoleShort}.`
    : `${firstInstalled ? `${firstInstalled.name}: the` : 'The'} first core installed, in this order. A core that can’t start a game, for want of a BIOS, gives way to the next.`

  return (
    <Sheet title={title} onClose={() => onClose()} className="cp-sheet">
      <p className="cp-intro">A core is the program that plays these games on your server. Every device plays them: the server runs the game and streams it.</p>
      {failed && !info && <p className="error" role="alert">{failed}</p>}
      {!info && !failed && <Spinner />}
      {info && (
        <div className="cp-list" role="radiogroup" aria-label={title} ref={listRef} onKeyDown={onKeyDown}>
          <button type="button" className="cp-row" role="radio" aria-checked={autoSelected} disabled={!!busy} onClick={() => void choose('')}>
            <span className="cp-mark" aria-hidden="true" />
            <span className="cp-main">
              <span className="cp-name"><b>{autoName}</b>{busy === 'auto' && <span className="cp-chip">Saving…</span>}</span>
              <span className="cp-line">{autoSub}</span>
            </span>
          </button>
          {info.choices.map((c) => {
            const selected = info.choice === c.id
            const usable = c.installed || !!c.canInstall
            const state = c.installed ? 'Installed' : c.canInstall ? 'Not installed' : !info.canDownload ? 'No download here' : 'Not on this server'
            const here = hereLine(c.support)
            const runs = runsLine(c.support)
            const installing = busy === c.id && !c.installed
            return (
              <div key={c.id} className={'cp-item' + (selected ? ' on' : '') + (usable ? '' : ' off')}>
                <button type="button" className="cp-row" role="radio" aria-checked={selected} disabled={!usable || (!!busy && busy !== c.id)}
                  aria-describedby={`cp-${c.id}-d`} onClick={() => void choose(c.id)}>
                  <span className="cp-mark" aria-hidden="true" />
                  <span className="cp-main">
                    <span className="cp-name">
                      <b>{c.name}</b>
                      {c.strength && <span className={'cp-chip ' + c.strength}>{STRENGTH[c.strength]}</span>}
                      {info.effective === c.id && <span className="cp-chip now">Plays now</span>}
                      {c.recommended && <span className="cp-chip">Recommended</span>}
                      {c.source === 'custom' && <span className="cp-chip">Custom</span>}
                    </span>
                    <span className="cp-line" id={`cp-${c.id}-d`}>
                      {c.about && <span>{c.about} </span>}
                      <span className="cp-facts">{[here, runs].filter(Boolean).join(' ')}</span>
                    </span>
                    {c.bios && (
                      <span className={'cp-bios' + (c.bios.found ? ' ok' : '')}>
                        {c.bios.found ? <IconCheck size={14} /> : <IconInfo size={14} />}
                        {c.bios.found ? `Needs a BIOS file of your own, and has one (${c.bios.files.join(' or ')}).` : `Needs a BIOS file of your own: ${c.bios.files.join(', ')}. Not found yet.`}
                      </span>
                    )}
                    {(c.starts || 0) > 0 && (
                      <span className="cp-stats">Started {c.starts} {c.starts === 1 ? 'time' : 'times'}, {hours(c.seconds || 0)} played{c.failures ? `, ${c.failures} failed` : ''}{c.best ? '. Has gone best on this game' : ''}.</span>
                    )}
                    <span className="cp-small">{state} · License: {c.license || 'not known'}{c.nonCommercial ? ' (non-commercial)' : ''}</span>
                  </span>
                </button>
                {confirm === c.id && (
                  <div className="cp-install" role="group" aria-label={`Install ${c.name}`}>
                    {installing ? (
                      <>
                        <progress max={progress?.total || undefined} value={progress?.total ? progress.received : undefined} aria-label={`Installing ${c.name}`} />
                        <p className="cp-small" aria-live="polite">{progress?.total ? `Downloading ${c.name}: ${mb(progress.received)} of ${mb(progress.total)}` : `Installing ${c.name}…`}</p>
                      </>
                    ) : (
                      <>
                        <p>Install <b>{c.name}</b> and use it? It downloads from <b>buildbot.libretro.com</b>, the libretro project’s build server, and runs boxed in on this server whenever someone plays. License: {c.license}. It’s its authors’ work and stays a separate program.</p>
                        <div className="cp-actions">
                          <button type="button" className="btn ghost sm" onClick={() => setConfirm('')}>Cancel</button>
                          <button type="button" className="btn primary sm" autoFocus onClick={() => void install(c)}>Install and use</button>
                        </div>
                      </>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
      {info && <p className="cp-foot muted">Switching keeps your in-game saves. A save state only loads in the core that made it.</p>}
    </Sheet>
  )
}
