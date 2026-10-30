import { useEffect, useRef, useState } from 'react'
import { controlsOf } from './consoles'
import { describeBinding, describeKey, DEFAULT_KEYS, findPress, InputManager, loadKeyMap, loadPadMap, MAX_PLAYERS, saveKeyMap, savePadMap, snapshotPad, STANDARD_MAP, type KeyMap, type PadBinding, type PadInfo, type PadMap } from './input'

// Which controller is which player, and what each button does. Saved per console:
// keyboards for this browser, gamepads per model.

type Editing = { kind: 'keys' } | { kind: 'pad'; index: number; id: string; name: string; standard: boolean }

export function ControllersPanel({ input, consoleId, onChanged, say }: { input: InputManager; consoleId: string; onChanged: () => void; say: (text: string) => void }) {
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

  if (editing) return <RemapScreen editing={editing} consoleId={consoleId} onDone={() => { setEditing(null); input.reloadMaps(); onChanged() }} say={say} />
  const portSelect = (value: number, onChange: (n: number) => void, label: string) => (
    <select className="wp-select" value={value} aria-label={label} onChange={(e) => { onChange(Number(e.target.value)); tick((n) => n + 1) }}>
      {Array.from({ length: MAX_PLAYERS }, (_, i) => <option key={i} value={i}>Player {i + 1}</option>)}
    </select>
  )
  return (
    <div className="wp-controllers">
      <ul className="wp-list">
        {pads.map((p) => (
          <li key={p.index}>
            <div className="wp-grow"><b>{p.name}</b>
              <small className="wp-muted">{!p.standard && !p.hasMap ? 'This pad’s buttons aren’t in a known order; set them up to be sure they’re right.' : p.hasMap ? 'Your own button layout' : 'Standard layout'}</small></div>
            {portSelect(p.port, (n) => input.setPadPort(p.index, n), `Player for ${p.name}`)}
            <button type="button" className="wp-btn" onClick={() => setEditing({ kind: 'pad', index: p.index, id: p.id, name: p.name, standard: p.standard })}>Buttons…</button>
          </li>
        ))}
        {pads.length === 0 && <li className="wp-muted">{window.isSecureContext || typeof navigator.getGamepads === 'function'
          ? 'No gamepad found. Connect one and press a button, or play with the keyboard or touch controls.'
          : 'This browser only offers gamepads on secure (https) pages. Use the keyboard or touch controls here, or open this server over https.'}</li>}
        <li>
          <div className="wp-grow"><b>Keyboard</b><small className="wp-muted">Arrows, Z, X, A, S, Q, W, Enter and Right Shift by default.</small></div>
          {portSelect(input.keyboardPlayer, (n) => input.setKeyboardPort(n), 'Player for the keyboard')}
          <button type="button" className="wp-btn" onClick={() => setEditing({ kind: 'keys' })}>Keys…</button>
        </li>
        <li>
          <div className="wp-grow"><b>Touch controls</b></div>
          {portSelect(input.touchPlayer, (n) => input.setTouchPort(n), 'Player for the touch controls')}
        </li>
      </ul>
    </div>
  )
}

function RemapScreen({ editing, consoleId, onDone, say }: { editing: Editing; consoleId: string; onDone: () => void; say: (text: string) => void }) {
  const controls = controlsOf(consoleId)
  const isPad = editing.kind === 'pad'
  const [padMap, setPadMap] = useState<PadMap>(() => (isPad ? loadPadMap(consoleId, editing.id) || { ...STANDARD_MAP } : {}))
  const [keyMap, setKeyMap] = useState<KeyMap>(() => loadKeyMap(consoleId))
  const [listening, setListening] = useState<number | null>(null)
  const timer = useRef(0)
  const padIndex = isPad ? editing.index : -1

  const label = (id: number) => controls.find((c) => c.id === id)?.label || String(id)

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
        setKeyMap((m) => {
          // One key does one thing: it leaves whatever it was doing before.
          const next: KeyMap = {}
          for (const [k, v] of Object.entries(m)) if (v !== e.code) next[Number(k)] = v
          next[listening] = e.code
          return next
        })
        say(`${label(listening)} is now ${describeKey(e.code)}`)
      }
      setListening(null)
    }
    window.addEventListener('keydown', key, true)
    return () => window.removeEventListener('keydown', key, true)
  }, [listening, isPad, padIndex])

  const save = () => {
    if (isPad) savePadMap(consoleId, editing.id, padMap)
    else saveKeyMap(consoleId, keyMap)
    onDone()
  }
  const reset = () => {
    if (isPad) { setPadMap({ ...STANDARD_MAP }); savePadMap(consoleId, editing.id, null) }
    else { setKeyMap(DEFAULT_KEYS); saveKeyMap(consoleId, null) }
    say('Back to the default buttons')
  }
  return (
    <div className="wp-remap">
      <h3>{isPad ? editing.name : 'Keyboard'}</h3>
      <ul className="wp-list wp-remap-list">
        {controls.map((c) => (
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
