import { useEffect, useMemo, useRef, useState } from 'react'
import { DisplayEngine, type DisplayMode } from './display'
import { computeLayout } from './layout'
import { CheatsPanel, CorePanel, JsonEditor, PatchesPanel, SettingsPanel, type LevelChoice } from './Panels'
import { levelName, loadSchema, useSettings, type Level, type Schema, type SettingsHost } from './settings'
import './play.css'

// Game settings outside a game: the same panels as the in-game menu, with a live preview
// of a test picture standing in for the game. Everything here is saved on the server, so
// it follows the person to their other devices.

// Our own test picture: colour bars, a gradient, fine stripes and a blocky shape.
function drawTestPicture(c: HTMLCanvasElement) {
  c.width = 256
  c.height = 224
  const g = c.getContext('2d')!
  const bars = ['#ffffff', '#ffff00', '#00ffff', '#00ff00', '#ff00ff', '#ff0000', '#0000ff', '#101010']
  bars.forEach((b, i) => { g.fillStyle = b; g.fillRect(i * 32, 0, 32, 96) })
  for (let x = 0; x < 256; x += 4) { const v = x; g.fillStyle = `rgb(${v},${v},${v})`; g.fillRect(x, 96, 4, 24) }
  for (let y = 120; y < 224; y += 2) { g.fillStyle = y % 4 === 0 ? '#27305c' : '#1a2144'; g.fillRect(0, y, 256, 2) }
  const shape = ['..XXXXXX..', '.XXXXXXXX.', 'XX.XXXX.XX', 'XXXXXXXXXX', 'XXX.XX.XXX', '.XXX..XXX.', '..XXXXXX..', '..X.XX.X..']
  shape.forEach((row, y) => [...row].forEach((ch, x) => { if (ch === 'X') { g.fillStyle = y < 4 ? '#ffd24a' : '#ff7a3c'; g.fillRect(40 + x * 8, 136 + y * 8, 8, 8) } }))
  for (let i = 0; i < 12; i++) { g.fillStyle = i % 2 ? '#7cf0a0' : '#7cc4ff'; g.fillRect(150 + (i % 4) * 22, 136 + Math.floor(i / 4) * 22, 18, 18) }
}

function Preview({ values, onMode, host }: { values: Record<string, unknown>; onMode: (m: DisplayMode) => void; host: SettingsHost }) {
  const stage = useRef<HTMLDivElement>(null)
  const pic = useRef<HTMLDivElement>(null)
  const engine = useRef<DisplayEngine | null>(null)
  const [size, setSize] = useState({ w: 480, h: 320 })
  const source = useMemo(() => { const c = document.createElement('canvas'); drawTestPicture(c); return c }, [])
  useEffect(() => {
    const el = stage.current
    if (!el) return
    const ro = new ResizeObserver(() => setSize({ w: el.clientWidth, h: el.clientHeight }))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  useEffect(() => {
    if (!pic.current) return
    source.className = 'wp-media'
    pic.current.appendChild(source)
    const e = new DisplayEngine(pic.current)
    e.onMode = onMode
    e.attach(source)
    engine.current = e
    return () => { e.destroy(); engine.current = null; source.remove() }
  }, [source]) // eslint-disable-line react-hooks/exhaustive-deps
  const bezel = values['display.bezel']
  const rotate = Number(values['display.rotate'] || 0) / 90
  const layout = computeLayout({ areaW: size.w, areaH: size.h, srcW: 256, srcH: 224, aspect: 4 / 3, mode: String(values['display.aspect'] || 'original'), custom: Number(values['display.aspectCustom'] || 0), integer: values['display.integer'] === true, cropX: Number(values['display.cropX'] || 0), cropY: Number(values['display.cropY'] || 0), turns: rotate, inset: bezel === 'none' ? 0 : Number(values['display.bezelInset'] || 0) })
  useEffect(() => { engine.current?.update(values, { width: 256, height: 224 }, { picW: layout.picW, picH: layout.picH }) }, [values, layout.picW, layout.picH])
  const picStyle = { width: layout.picW, height: layout.picH, transform: rotate ? `translate(-50%, -50%) rotate(${rotate * 90}deg)` : 'translate(-50%, -50%)' }
  const mediaStyle = `.wp-preview-stage .wp-media:not(.wp-gl){width:${layout.scaleX * 100}%;height:${layout.scaleY * 100}%;left:${(100 - layout.scaleX * 100) / 2}%;top:${(100 - layout.scaleY * 100) / 2}%;position:absolute}`
  return (
    <div ref={stage} className="wp-preview-stage" aria-label="Preview, with a test picture" role="img"
      style={{ background: bezel === 'color' ? String(values['display.bezelColor'] || '#000') : '#000', backgroundImage: bezel === 'image' && values['display.bezelImage'] ? `url(${JSON.stringify(host.mediaUrl(`/api/game-bezels/${values['display.bezelImage']}`))})` : undefined, backgroundSize: 'cover', backgroundPosition: 'center' }}>
      <style>{mediaStyle}</style>
      <div className="wp-preview-box" style={{ width: layout.boxW, height: layout.boxH }}>
        <div ref={pic} className="wp-pic" style={picStyle} />
      </div>
    </div>
  )
}

type Tab = { id: string; label: string; groups?: string[] }

const TABS: Tab[] = [
  { id: 'display', label: 'Picture', groups: ['display'] },
  { id: 'osd', label: 'On-screen numbers', groups: ['osd'] },
  { id: 'emulation', label: 'Speed & rewind', groups: ['emulation'] },
  { id: 'audio', label: 'Sound', groups: ['audio'] },
  { id: 'states', label: 'Saves', groups: ['states'] },
  { id: 'hotkeys', label: 'Shortcuts', groups: ['hotkeys'] },
  { id: 'core', label: 'Core options' },
  { id: 'cheats', label: 'Cheats' },
  { id: 'patches', label: 'Patches' },
  { id: 'json', label: 'Custom (JSON)' },
]

export function GameSettingsScreen({ host, consoles, itemId, itemTitle, consoleId, onError }: {
  host: SettingsHost; consoles: { id: string; name: string; short: string }[]; itemId?: number; itemTitle?: string; consoleId?: string; onError: (m: string) => void
}) {
  const [schema, setSchema] = useState<Schema | null>(null)
  const [tab, setTab] = useState('display')
  const [chosenConsole, setChosenConsole] = useState(consoleId || '')
  const [level, setLevel] = useState<Level>(itemId ? 'game' : 'person')
  const [mode, setMode] = useState<DisplayMode>('plain')
  useEffect(() => { loadSchema(host).then(setSchema).catch((e) => onError(e?.message || 'Game settings couldn’t be read')) }, [host, onError])
  const scope = useMemo(() => (itemId ? { item: itemId } : chosenConsole ? { console: chosenConsole } : {}), [itemId, chosenConsole])
  const api = useSettings(host, scope, onError)
  const view = api.view
  const consoleName = consoles.find((c) => c.id === (consoleId || chosenConsole))?.short || ''
  const levels: LevelChoice[] = []
  if (itemId) levels.push({ level: 'game', label: 'This game' })
  if (itemId || chosenConsole) levels.push({ level: 'console', label: consoleName ? `All ${consoleName} games` : 'This console' })
  levels.push({ level: 'person', label: 'All my games' })
  if (host.isAdmin) {
    if (itemId || chosenConsole) levels.push({ level: 'serverConsole', label: consoleName ? `Everyone’s ${consoleName} games` : 'Everyone, this console' })
    levels.push({ level: 'server', label: 'Everyone' })
  }
  // A console that's gone from the choices can't be what changes apply to.
  useEffect(() => { if (!levels.some((l) => l.level === level)) setLevel(levels[0].level) }, [chosenConsole]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!schema || !view) return <p className="wp-muted" role="status">Loading…</p>
  const presets = [...schema.presets, ...view.resolved.presets]
  const tabs = TABS.filter((t) => itemId || !['core', 'cheats', 'patches'].includes(t.id))
  const current = tabs.find((t) => t.id === tab) || tabs[0]
  const common = { level, levels, onLevel: setLevel, consoleName, view, api, schema }
  return (
    <div className="wp-screen">
      {!itemId && (
        <label className="wp-screen-console">Settings for
          <select className="wp-select" value={chosenConsole} onChange={(e) => { setChosenConsole(e.target.value); setLevel(e.target.value ? 'console' : 'person') }}>
            <option value="">All my games</option>
            {consoles.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        </label>
      )}
      <div className="wp-tabs" role="tablist" aria-label="Settings">
        {tabs.map((t) => <button key={t.id} type="button" role="tab" aria-selected={t.id === current.id} className={'wp-chip' + (t.id === current.id ? ' on' : '')} onClick={() => setTab(t.id)}>{t.label}</button>)}
      </div>
      <div className={'wp-screen-body' + (current.id === 'display' ? ' with-preview' : '')}>
        {current.id === 'display' && (
          <div className="wp-screen-preview">
            <Preview values={view.resolved.values} onMode={setMode} host={host} />
            <p className="wp-muted wp-small" role="status">{mode === 'plain' ? 'No effects: the picture goes straight to the screen.' : mode === 'shader' ? 'Effects are drawn on this device, which adds almost no delay.' : 'This browser can’t draw every effect, so a simpler version is used.'} Changes are saved for {levelName(level, consoleName)}.</p>
          </div>
        )}
        <div className="wp-screen-panel">
          {current.groups && <SettingsPanel {...common} groups={current.groups} host={host} say={onError} presets={current.id === 'display' ? presets : undefined} />}
          {current.id === 'core' && itemId && <CorePanel {...common} host={host} itemId={itemId} say={onError} onJson={() => setTab('json')} />}
          {current.id === 'cheats' && itemId && <CheatsPanel host={host} itemId={itemId} title={itemTitle || 'game'} say={onError} />}
          {current.id === 'patches' && itemId && <PatchesPanel host={host} itemId={itemId} say={onError} />}
          {current.id === 'json' && <JsonEditor {...common} host={host} scope={scope} title={itemTitle || consoleName || 'games'} say={onError} itemId={itemId} />}
        </div>
      </div>
    </div>
  )
}
