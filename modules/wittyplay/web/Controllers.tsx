import { useEffect, useRef, useState } from 'react'
import { buttonsOf, padKey, type ControlsData, type KeyMap, type PadBinding, type PadMap, type SaveControl } from './consoles'
import { describeBinding, describeKey, findPress, InputManager, MAX_PLAYERS, snapshotPad, type PadInfo } from './input'

// Which controller is which player, and what each button does. Layouts are saved
// on the server, per console: keyboards for the account, gamepads per model.

type Editing = { kind: 'keys' } | { kind: 'pad'; index: number; id: string; name: string; standard: boolean }
type Props = { input: InputManager; controls: ControlsData; onSave: (s: SaveControl) => Promise<void>; say: (text: string) => void }

// A button rather than a drop-down, so a remote's D-pad never gets stuck in a list.
function PlayerButton({ value, onChange, name }: { value: number; onChange: (n: number) => void; name: string }) {
  return (
    <button type="button" className="wp-btn" aria-label={`${name}: Player ${value + 1}. Press to change.`} onClick={() => onChange((value + 1) % MAX_PLAYERS)}>
      Player {value + 1}
    </button>
  )
}

export function ControllersPanel({ input, controls, onSave, say }: Props) {
  const [pads, setPads] = useState<PadInfo[]>(() => input.listPads())
  const [, tick] = useState(0)
  const [editing, setEditing] = useState<Editing | null>(null)
  useEffect(() => {
    const on = () => setPads(input.listPads())
    const prev = input.onChange
    input.onChange = () => { prev?.(); on() }
    const iv = setInterval(on, 1000) // some browsers don't fire connection events
    return () => { input.onChange = prev; clearInterval(iv) }
  }, [input])

  if (editing) return <RemapScreen editing={editing} controls={controls} onSave={onSave} onDone={() => setEditing(null)} say={say} />
  return (
    <div className="wp-controllers">
      <ul className="wp-list">
        {pads.map((p) => (
          <li key={p.index}>
            <div className="wp-grow"><b>{p.name}</b>
              <small className="wp-muted">{!p.standard && !p.hasMap ? 'This pad’s buttons aren’t in a known order; set them up to be sure they’re right.' : p.hasMap ? 'Your own button layout' : 'Standard layout'}</small></div>
            <PlayerButton value={p.port} name={p.name} onChange={(n) => input.setPadPort(p.index, n)} />
            <button type="button" className="wp-btn" onClick={() => setEditing({ kind: 'pad', index: p.index, id: p.id, name: p.name, standard: p.standard })}>Buttons…</button>
          </li>
        ))}
        {pads.length === 0 && <li className="wp-muted">{window.isSecureContext || typeof navigator.getGamepads === 'function'
          ? 'No gamepad found. Connect one and press a button, or play with the keyboard or touch controls.'
          : 'This browser only offers gamepads on secure (https) pages. Use the keyboard or touch controls here, or open this server over https.'}</li>}
        <li>
          <div className="wp-grow"><b>Keyboard</b><small className="wp-muted">Arrows, Z, X, A, S, Q, W, Enter and Right Shift by default.</small></div>
          <PlayerButton value={input.keyboardPlayer} name="Keyboard" onChange={(n) => { input.setKeyboardPort(n); tick((x) => x + 1) }} />
          <button type="button" className="wp-btn" onClick={() => setEditing({ kind: 'keys' })}>Keys…</button>
        </li>
        <li>
          <div className="wp-grow"><b>Touch controls</b></div>
          <PlayerButton value={input.touchPlayer} name="Touch controls" onChange={(n) => { input.setTouchPort(n); tick((x) => x + 1) }} />
        </li>
      </ul>
    </div>
  )
}

function RemapScreen({ editing, controls, onSave, onDone, say }: { editing: Editing; controls: ControlsData; onSave: Props['onSave']; onDone: () => void; say: Props['say'] }) {
  const buttons = buttonsOf(controls)
  const isPad = editing.kind === 'pad'
  const [padMap, setPadMap] = useState<PadMap>(() => (isPad ? controls.saved.gamepad[padKey(editing.id)] || { ...controls.defaults.gamepad } : {}))
  const [keyMap, setKeyMap] = useState<KeyMap>(() => controls.saved.keyboard || controls.defaults.keyboard)
  const [listening, setListening] = useState<number | null>(null)
  const timer = useRef(0)
  const padIndex = isPad ? editing.index : -1

  const label = (id: number) => buttons.find((c) => c.id === id)?.label || String(id)

  useEffect(() => {
    if (listening === null) return
    say(`Press what should be ${label(listening)}. Escape cancels.`)
    if (isPad) {
      const get = () => (navigator.getGamepads?.() || [])[padIndex]
      const first = get()
      if (!first) return
      const base = snapshotPad(first)
      // On a timer rather than animation frames, which stop when the tab isn't drawing.
      timer.current = window.setInterval(() => {
        const p = get()
        const bind: PadBinding | null = p ? findPress(p, base) : null
        if (bind) {
          setPadMap((m) => ({ ...m, [listening]: bind }))
          say(`${label(listening)} is now ${describeBinding(bind)}`)
          setListening(null)
        }
      }, 16)
      const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); setListening(null) } }
      window.addEventListener('keydown', esc, true)
      return () => { clearInterval(timer.current); window.removeEventListener('keydown', esc, true) }
    }
    const key = (e: KeyboardEvent) => {
      e.preventDefault()
      e.stopPropagation()
      if (e.code !== 'Escape') {
        const name = e.code || e.key // remotes often send no code
        setKeyMap((m) => {
          // One key does one thing: it leaves whatever it was doing before.
          const next: KeyMap = {}
          for (const [k, v] of Object.entries(m)) if (v !== name) next[Number(k)] = v
          next[listening] = name
          return next
        })
        say(`${label(listening)} is now ${describeKey(name)}`)
      }
      setListening(null)
    }
    window.addEventListener('keydown', key, true)
    return () => window.removeEventListener('keydown', key, true)
  }, [listening, isPad, padIndex])

  const save = async () => {
    try {
      await onSave(isPad ? { kind: 'gamepad', key: padKey(editing.id), data: padMap } : { kind: 'keyboard', data: keyMap })
      onDone()
    } catch (e: any) { say(e?.message || 'That couldn’t be saved') }
  }
  const reset = async () => {
    if (isPad) setPadMap({ ...controls.defaults.gamepad })
    else setKeyMap(controls.defaults.keyboard)
    try {
      await onSave(isPad ? { kind: 'gamepad', key: padKey(editing.id), data: null } : { kind: 'keyboard', data: null })
      say('Back to the default buttons')
    } catch (e: any) { say(e?.message || 'That couldn’t be saved') }
  }
  return (
    <div className="wp-remap">
      <h3>{isPad ? editing.name : 'Keyboard'}</h3>
      <ul className="wp-list wp-remap-list">
        {buttons.map((c) => (
          <li key={c.id}>
            <span className="wp-grow">{c.label}</span>
            <span className="wp-muted">{isPad ? describeBinding(padMap[c.id]) : describeKey(keyMap[c.id])}</span>
            <button type="button" className={'wp-btn' + (listening === c.id ? ' primary' : '')} onClick={() => setListening(c.id)} aria-label={`Change ${c.label}`}>
              {listening === c.id ? 'Press…' : 'Change'}
            </button>
          </li>
        ))}
      </ul>
      <div className="wp-row">
        <button type="button" className="wp-btn" onClick={reset}>Reset</button>
        <span className="wp-grow" />
        <button type="button" className="wp-btn" onClick={onDone}>Cancel</button>
        <button type="button" className="wp-btn primary" onClick={save}>Save</button>
      </div>
    </div>
  )
}
