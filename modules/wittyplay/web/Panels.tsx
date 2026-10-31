import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent as RKeyboardEvent } from 'react'
import { reducedMotion } from './display'
import { downloadBlob, fileName, layerDocument, levelName, LEVEL_RANK, pickFile, type Choice, type CoreOptionsView, type CustomControl, type Issue, type Level, type Preset, type Report, type Schema, type SettingDef, type SettingsApi, type SettingsHost, type SettingsView } from './settings'

// The panels of the game menu and of the settings page. They're drawn from what the
// server describes, and every control works with touch, a mouse, arrow keys, a TV remote
// and a controller. Left and right change a value, because up and down are for moving on.

export type LevelChoice = { level: Level; label: string }

// A value and arrows, because a drop-down can't be used from a sofa.
export function Cycler({ label, value, options, onChange, disabled }: { label: string; value: string; options: Choice[]; onChange: (v: string) => void; disabled?: boolean }) {
  const i = Math.max(0, options.findIndex((o) => o.value === value))
  const step = (d: number) => onChange(options[(i + d + options.length) % options.length].value)
  const key = (e: RKeyboardEvent) => {
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      e.preventDefault()
      e.stopPropagation()
      step(e.key === 'ArrowLeft' ? -1 : 1)
    }
  }
  const shown = options.find((o) => o.value === value)?.label ?? value
  return (
    <span className="wp-cycler">
      <button type="button" className="wp-cycler-arrow" aria-hidden="true" tabIndex={-1} disabled={disabled} onClick={() => step(-1)}>‹</button>
      <button type="button" className="wp-cycler-value" disabled={disabled} onClick={() => step(1)} onKeyDown={key} aria-label={`${label}: ${shown}. Left and right change it.`}>{shown}</button>
      <button type="button" className="wp-cycler-arrow" aria-hidden="true" tabIndex={-1} disabled={disabled} onClick={() => step(1)}>›</button>
    </span>
  )
}

export function Slider({ label, value, min, max, step, unit, onChange }: { label: string; value: number; min: number; max: number; step: number; unit?: string; onChange: (v: number) => void }) {
  const clean = (v: number) => Number(Math.max(min, Math.min(max, Math.round(v / step) * step)).toFixed(4))
  // A controller's arrows are made-up key presses, which a slider on its own ignores.
  const key = (e: RKeyboardEvent) => {
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      e.preventDefault()
      e.stopPropagation()
      onChange(clean(value + (e.key === 'ArrowLeft' ? -step : step)))
    }
  }
  const shown = step >= 1 || unit ? `${Number(value.toFixed(2))}${unit || ''}` : value.toFixed(2)
  return (
    <span className="wp-slider">
      <input type="range" aria-label={label} min={min} max={max} step={step} value={value} onChange={(e) => onChange(clean(Number(e.target.value)))} onKeyDown={key} />
      <output>{shown}</output>
    </span>
  )
}

export function Back({ onClick, label = 'Back' }: { onClick: () => void; label?: string }) {
  return <div className="wp-row"><span className="wp-grow" /><button type="button" className="wp-btn" onClick={onClick}>{label}</button></div>
}

// Which level a change applies to.
export function LevelPicker({ levels, level, onChange }: { levels: LevelChoice[]; level: Level; onChange: (l: Level) => void }) {
  if (levels.length < 2) return null
  return (
    <div className="wp-levels" role="group" aria-label="Which games this changes">
      {levels.map((l) => <button key={l.level} type="button" className={'wp-chip' + (l.level === level ? ' on' : '')} aria-pressed={l.level === level} onClick={() => onChange(l.level)}>{l.label}</button>)}
    </div>
  )
}

// Some settings only mean something when another is set.
const shownWhen: Record<string, (v: Record<string, unknown>) => boolean> = {
  'display.aspectCustom': (v) => v['display.aspect'] === 'custom',
  'display.bezelColor': (v) => v['display.bezel'] === 'color',
  'display.bezelImage': (v) => v['display.bezel'] === 'image',
  'display.bezelInset': (v) => v['display.bezel'] !== 'none',
  'osd.corner': (v) => v['osd.on'] === true,
  'osd.size': (v) => v['osd.on'] === true,
  'emulation.rewindSeconds': (v) => v['emulation.rewind'] === true,
  'hotkeys.padModifier': (v) => v['hotkeys.padCombos'] !== false,
}

function KeyCapture({ def, value, onChange }: { def: SettingDef; value: string; onChange: (v: string) => void }) {
  const [listening, setListening] = useState(false)
  useEffect(() => {
    if (!listening) return
    const key = (e: KeyboardEvent) => {
      e.preventDefault()
      e.stopPropagation()
      setListening(false)
      if (e.code === 'Escape') return
      if (def.choices?.some((c) => c.value === e.code)) onChange(e.code)
    }
    window.addEventListener('keydown', key, true)
    return () => window.removeEventListener('keydown', key, true)
  }, [listening, def, onChange])
  const shown = def.choices?.find((c) => c.value === value)?.label ?? value
  return <button type="button" className="wp-btn wp-keycap" onClick={() => setListening(true)} aria-label={`${def.label}: ${shown || 'not set'}. Press to change.`}>{listening ? 'Press a key…' : shown || 'Not set'}</button>
}

function BezelPicker({ host, value, onChange, say }: { host: SettingsHost; value: number; onChange: (id: number) => void; say: (t: string) => void }) {
  type Bezel = { id: number; name: string; width: number; height: number }
  const [list, setList] = useState<Bezel[]>([])
  useEffect(() => { host.request<Bezel[]>('GET', '/game-bezels').then(setList).catch(() => {}) }, [host])
  const add = async () => {
    const f = await pickFile('image/png,image/jpeg,image/webp')
    if (!f) return
    try {
      const r = await host.upload<{ id: number; bezels: Bezel[] }>(`/game-bezels?name=${encodeURIComponent(f.name)}`, f)
      setList(r.bezels)
      onChange(r.id)
    } catch (e: any) { say(e?.message || 'That image couldn’t be used') }
  }
  const remove = async (id: number) => {
    try {
      await host.request('DELETE', `/game-bezels/${id}`)
      setList((l) => l.filter((b) => b.id !== id))
      if (value === id) onChange(0)
    } catch (e: any) { say(e?.message || 'That didn’t work') }
  }
  return (
    <div className="wp-bezels">
      {list.map((b) => (
        <span key={b.id} className={'wp-bezel-pick' + (b.id === value ? ' on' : '')}>
          <button type="button" onClick={() => onChange(b.id)} aria-pressed={b.id === value} aria-label={`Use ${b.name}`}><img src={host.mediaUrl(`/api/game-bezels/${b.id}`)} alt="" loading="lazy" /></button>
          <button type="button" className="wp-link" onClick={() => void remove(b.id)} aria-label={`Remove ${b.name}`}>Remove</button>
        </span>
      ))}
      <button type="button" className="wp-btn" onClick={() => void add()}>Add an image…</button>
      <p className="wp-muted wp-small">PNG, JPEG or WebP, up to 16 MB. It stays yours: nobody else on this server can see it.</p>
    </div>
  )
}

function SettingControl({ def, value, onChange, host, say }: { def: SettingDef; value: unknown; onChange: (v: unknown) => void; host: SettingsHost; say: (t: string) => void }) {
  switch (def.kind) {
    case 'toggle':
      return <label className="wp-toggle"><input type="checkbox" checked={value === true} onChange={(e) => onChange(e.target.checked)} aria-label={def.label} /><span aria-hidden="true">{value === true ? 'On' : 'Off'}</span></label>
    case 'choice':
      if (def.key.startsWith('hotkeys.key.')) return <KeyCapture def={def} value={String(value ?? '')} onChange={onChange} />
      return <Cycler label={def.label} value={String(value)} options={def.choices || []} onChange={onChange} />
    case 'slider':
      return <Slider label={def.label} value={typeof value === 'number' ? value : 0} min={def.min ?? 0} max={def.max ?? 1} step={def.step ?? 0.1} unit={def.unit} onChange={onChange} />
    case 'color':
      return <input type="color" className="wp-color" aria-label={def.label} value={String(value)} onChange={(e) => onChange(e.target.value)} />
    case 'bezel':
      return <BezelPicker host={host} value={typeof value === 'number' ? value : 0} onChange={onChange} say={say} />
  }
  return null
}

// Why a value is what it is. A setting at a more specific level beats this one, and
// people need to be told, or their change appears to do nothing.
function SourceNote({ source, level, consoleName, onClear }: { source: string; level: Level; consoleName: string; onClear: (at: Level) => void }) {
  const here = source === level
  const higher = (LEVEL_RANK[source] ?? 0) > (LEVEL_RANK[level] ?? 0)
  const canClear = source !== 'builtin'
  if (!canClear) return null
  return (
    <span className={'wp-source' + (higher ? ' wp-source-over' : '')}>
      {here ? `Set for ${levelName(source, consoleName)}` : higher ? `${levelName(source, consoleName)} has its own value, which wins` : `From ${levelName(source, consoleName)}`}
      {(here || higher) && <button type="button" className="wp-link" onClick={() => onClear(source as Level)} aria-label={`Reset this for ${levelName(source, consoleName)}`}>Reset</button>}
    </span>
  )
}

export function SettingsPanel({ schema, api, view, groups, level, levels, onLevel, consoleName, host, say, presets }: {
  schema: Schema; api: SettingsApi; view: SettingsView; groups: string[]; level: Level; levels: LevelChoice[]; onLevel: (l: Level) => void
  consoleName: string; host: SettingsHost; say: (t: string) => void; presets?: Preset[]
}) {
  const v = view.resolved.values
  const defs = schema.settings.filter((d) => groups.includes(d.group) && (shownWhen[d.key]?.(v) ?? true))
  const sections: { title: string; defs: SettingDef[] }[] = []
  for (const d of defs) {
    const t = d.section || ''
    const last = sections[sections.length - 1]
    if (last && last.title === t) last.defs.push(d)
    else sections.push({ title: t, defs: [d] })
  }
  const lim = view.limits
  const note = (d: SettingDef) => {
    if (d.key === 'emulation.ffSpeed') return `This server allows up to ${lim.maxSpeed}×.`
    if (d.key === 'emulation.rewind' && lim.rewindBytes === 0) return 'This server has rewind turned off.'
    if (d.key === 'emulation.runAhead' && !lim.runAhead) return 'This server has run-ahead turned off.'
    if (d.motion && reducedMotion()) return 'Off while your device asks for reduced motion.'
    return ''
  }
  return (
    <>
      <LevelPicker levels={levels} level={level} onChange={onLevel} />
      {presets && presets.length > 0 && (
        <div className="wp-presets" role="group" aria-label="Presets">
          {presets.map((p) => <button key={p.name} type="button" className="wp-chip" title={p.description} onClick={() => api.applyPreset(level, p).then(() => say(`${p.name} applied`), (e) => say(e?.message || 'That didn’t work'))}>{p.name}</button>)}
        </div>
      )}
      {sections.map((s, i) => (
        <section key={i} className="wp-section">
          {s.title && <h3>{s.title}</h3>}
          {s.defs.map((d) => (
            <div key={d.key} className={'wp-setting wp-kind-' + d.kind}>
              <div className="wp-setting-text">
                <b>{d.label}</b>
                {d.help && <small className="wp-muted">{d.help}</small>}
                {d.caution && <small className="wp-caution" role="note"><span aria-hidden="true">⚠ </span>{d.caution}</small>}
                {note(d) && <small className="wp-muted">{note(d)}</small>}
                <SourceNote source={view.resolved.sources[d.key]} level={level} consoleName={consoleName} onClear={(at) => api.set(at, d.key, null)} />
              </div>
              <SettingControl def={d} value={v[d.key]} onChange={(x) => api.set(level, d.key, x)} host={host} say={say} />
            </div>
          ))}
        </section>
      ))}
    </>
  )
}

// ---- core options ----

const optionChoices = (list: string[]): Choice[] => list.map((c) => ({ value: c, label: c }))

function CustomControlRow({ c, value, known, onChange }: { c: CustomControl; value: string; known: boolean; onChange: (v: string) => void }) {
  if (c.type === 'toggle') {
    const on = c.on ?? 'enabled', off = c.off ?? 'disabled'
    return (
      <div className="wp-setting">
        <div className="wp-setting-text"><b>{c.label}</b>{c.help && <small className="wp-muted">{c.help}</small>}{!known && <small className="wp-caution">This core has no such option, so this does nothing here.</small>}</div>
        <label className="wp-toggle"><input type="checkbox" aria-label={c.label} checked={value === on} onChange={(e) => onChange(e.target.checked ? on : off)} /><span aria-hidden="true">{value === on ? 'On' : 'Off'}</span></label>
      </div>
    )
  }
  const options = c.choices || []
  if (c.type === 'slider') {
    const at = Math.max(0, options.findIndex((o) => o.value === value))
    return (
      <div className="wp-setting wp-kind-slider">
        <div className="wp-setting-text"><b>{c.label}</b>{c.help && <small className="wp-muted">{c.help}</small>}{!known && <small className="wp-caution">This core has no such option, so this does nothing here.</small>}</div>
        <span className="wp-slider">
          <input type="range" aria-label={c.label} min={0} max={options.length - 1} step={1} value={at} onChange={(e) => onChange(options[Number(e.target.value)].value)}
            onKeyDown={(e) => { if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') { e.preventDefault(); e.stopPropagation(); const n = Math.max(0, Math.min(options.length - 1, at + (e.key === 'ArrowLeft' ? -1 : 1))); onChange(options[n].value) } }} />
          <output>{options[at]?.label ?? value}</output>
        </span>
      </div>
    )
  }
  return (
    <div className="wp-setting">
      <div className="wp-setting-text"><b>{c.label}</b>{c.help && <small className="wp-muted">{c.help}</small>}{!known && <small className="wp-caution">This core has no such option, so this does nothing here.</small>}</div>
      <Cycler label={c.label} value={value} options={options} onChange={onChange} />
    </div>
  )
}

export function CorePanel({ host, itemId, api, view, level, levels, onLevel, consoleName, say, onJson }: {
  host: SettingsHost; itemId: number; api: SettingsApi; view: SettingsView; level: Level; levels: LevelChoice[]; onLevel: (l: Level) => void; consoleName: string; say: (t: string) => void; onJson: () => void
}) {
  const [core, setCore] = useState<CoreOptionsView | null>(null)
  const [filter, setFilter] = useState('')
  const load = useCallback(() => host.request<CoreOptionsView>('GET', `/games/${itemId}/core-options`).then(setCore).catch((e) => say(e?.message || 'The core’s options couldn’t be read')), [host, itemId, say])
  useEffect(() => { void load() }, [load])
  const res = view.resolved
  const offered = new Map((core?.options || []).map((o) => [o.key, o]))
  const valueOf = (key: string, fallback: string) => res.coreOptions[key] ?? core?.running[key] ?? fallback
  const list = (core?.options || []).filter((o) => !filter || (o.label + ' ' + o.key).toLowerCase().includes(filter.toLowerCase()))
  if (!core) return <p className="wp-muted" role="status">Asking the core…</p>
  return (
    <>
      <LevelPicker levels={levels} level={level} onChange={onLevel} />
      {!core.known && <p className="wp-muted">This core hasn’t said what options it has. Start the game once and they’ll be listed here.</p>}
      {res.presets.length > 0 && (
        <section className="wp-section">
          <h3>Your presets</h3>
          <div className="wp-presets">{res.presets.map((p) => <button key={p.name} type="button" className="wp-chip" title={p.description} onClick={() => api.applyPreset(level, p).then(() => { say(`${p.name} applied`); void load() }, (e) => say(e?.message || 'That didn’t work'))}>{p.name}</button>)}</div>
        </section>
      )}
      {res.controls.length > 0 && (
        <section className="wp-section">
          <h3>Your controls</h3>
          {res.controls.map((c) => <CustomControlRow key={c.option + c.label} c={c} known={offered.has(c.option) || !core.known} value={valueOf(c.option, offered.get(c.option)?.default ?? '')} onChange={(x) => api.setOption(level, c.option, x)} />)}
        </section>
      )}
      {(core.options.length > 12) && <input className="wp-input" type="search" placeholder="Find an option…" aria-label="Find an option" value={filter} onChange={(e) => setFilter(e.target.value)} />}
      <section className="wp-section">
        {core.known && <h3>{core.core} options</h3>}
        {list.map((o) => {
          const source = res.coreSources[o.key]
          return (
            <div key={o.key} className="wp-setting">
              <div className="wp-setting-text">
                <b>{o.label || o.key}</b>
                <small className="wp-muted wp-mono">{o.key}</small>
                {source && <SourceNote source={source} level={level} consoleName={consoleName} onClear={(at) => api.setOption(at, o.key, null)} />}
              </div>
              <Cycler label={o.label || o.key} value={valueOf(o.key, o.default)} options={optionChoices(o.choices)} onChange={(x) => api.setOption(level, o.key, x)} />
            </div>
          )
        })}
      </section>
      {core.ignored.length > 0 && (
        <section className="wp-section" aria-label="Set but ignored">
          <h3>Set, but ignored by this core</h3>
          <ul className="wp-issues">{core.ignored.map((x, i) => <li key={i}><code>{x.path}</code>: {x.message}</li>)}</ul>
        </section>
      )}
      {core.known && <p className="wp-muted wp-small">A core may only pick up some options the next time the game starts.</p>}
      <button type="button" className="wp-btn" onClick={onJson}>Custom settings file (JSON)…</button>
    </>
  )
}

// ---- the JSON editor ----

const TEMPLATE = `{
  "wittyplay": 1,
  "settings": {
    "display.scanlines": 0.4
  },
  "coreOptions": {
  },
  "controls": [
    { "label": "My control", "type": "toggle", "option": "the_core_option", "on": "enabled", "off": "disabled" }
  ],
  "presets": [
    { "name": "My preset", "settings": { "display.scaling": "pixel" }, "coreOptions": { } }
  ]
}
`

export function JsonEditor({ host, api, view, scope, level, levels, onLevel, consoleName, title, say, itemId }: {
  host: SettingsHost; api: SettingsApi; view: SettingsView; scope: { console?: string; item?: number }; level: Level; levels: LevelChoice[]; onLevel: (l: Level) => void; consoleName: string; title: string; say: (t: string) => void; itemId?: number
}) {
  const [text, setText] = useState(() => layerDocument(view.layers[level]))
  const [report, setReport] = useState<Report | null>(null)
  const [dirty, setDirty] = useState(false)
  const [core, setCore] = useState<CoreOptionsView | null>(null)
  const [filter, setFilter] = useState('')
  const [confirmReset, setConfirmReset] = useState(false)
  const box = useRef<HTMLTextAreaElement>(null)
  const gutter = useRef<HTMLDivElement>(null)
  const seq = useRef(0)

  // Moving to another level shows that level's file.
  useEffect(() => { setText(layerDocument(view.layers[level])); setDirty(false); setReport(null); setConfirmReset(false) }, [level]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (itemId) host.request<CoreOptionsView>('GET', `/games/${itemId}/core-options`).then(setCore).catch(() => {}) }, [host, itemId])

  const check = useCallback(async (src: string) => {
    const n = ++seq.current
    try {
      const r = await host.request<{ report: Report }>('POST', '/games/settings/validate', { document: src, ...scope })
      if (n === seq.current) setReport(r.report)
    } catch { /* it's checked again on saving */ }
  }, [host, scope])
  // Checks follow typing, once it pauses.
  useEffect(() => {
    if (!dirty) return
    const t = setTimeout(() => void check(text), 450)
    return () => clearTimeout(t)
  }, [text, dirty, check])

  const lines = useMemo(() => text.split('\n').length, [text])
  const badLines = useMemo(() => new Set((report?.errors || []).map((e) => e.line)), [report])
  const warnLines = useMemo(() => new Set((report?.ignored || []).map((e) => e.line)), [report])

  const goTo = (issue: Issue) => {
    const el = box.current
    if (!el || !issue.line) return
    let at = 0
    for (let l = 1; l < issue.line; l++) at = text.indexOf('\n', at) + 1
    el.focus()
    el.setSelectionRange(at, Math.max(at, text.indexOf('\n', at)))
    el.scrollTop = (issue.line - 3) * 20
  }

  const save = async () => {
    try {
      const v = await api.saveDocument(level, text)
      setDirty(false)
      setReport({ errors: [], ignored: v.ignored || [] })
      setText(layerDocument(v.layers[level]))
      say('Saved')
    } catch (e: any) {
      if (e?.data?.report) setReport(e.data.report)
      else void check(text)
      say(e?.message || 'That couldn’t be saved')
    }
  }
  const importFile = async () => {
    const f = await pickFile('.json,application/json')
    if (!f) return
    if (f.size > 256 * 1024) { say('That file is too large: 256 KB at most'); return }
    const t = await f.text()
    setText(t)
    setDirty(true)
    void check(t)
    say('Check it, then press Save')
  }
  const exportFile = () => downloadBlob(new Blob([text], { type: 'application/json' }), `${fileName(title)}.wittyplay.json`)
  const insert = (snippet: string) => {
    const el = box.current
    if (!el) return
    const s = el.selectionStart ?? text.length
    const next = text.slice(0, s) + snippet + text.slice(el.selectionEnd ?? s)
    setText(next)
    setDirty(true)
    requestAnimationFrame(() => { el.focus(); el.setSelectionRange(s + snippet.length, s + snippet.length) })
  }
  const issues = [...(report?.errors || []).map((x) => ({ ...x, bad: true })), ...(report?.ignored || []).map((x) => ({ ...x, bad: false }))]
  const options = (core?.options || []).filter((o) => !filter || (o.label + ' ' + o.key).toLowerCase().includes(filter.toLowerCase()))
  return (
    <>
      <LevelPicker levels={levels} level={level} onChange={onLevel} />
      <p className="wp-muted wp-small">The settings for {levelName(level, consoleName)}, as a file you can edit, keep and share. It can only hold settings and core options. It can’t run anything, name files, or reach the network.</p>
      <div className="wp-json">
        <div ref={gutter} className="wp-json-gutter" aria-hidden="true">
          {Array.from({ length: lines }, (_, i) => <div key={i} className={badLines.has(i + 1) ? 'bad' : warnLines.has(i + 1) ? 'warn' : ''}>{i + 1}</div>)}
        </div>
        <textarea ref={box} className="wp-json-text" spellCheck={false} wrap="off" aria-label={`Settings file for ${levelName(level, consoleName)}`} value={text}
          onChange={(e) => { setText(e.target.value); setDirty(true) }}
          onScroll={(e) => { if (gutter.current) gutter.current.scrollTop = e.currentTarget.scrollTop }} />
      </div>
      {issues.length > 0 && (
        <ul className="wp-issues" aria-live="polite">
          {issues.map((x, i) => (
            <li key={i} className={x.bad ? 'bad' : 'warn'}>
              <button type="button" className="wp-link" onClick={() => goTo(x)}>{x.line ? `Line ${x.line}` : 'File'}</button>{' '}
              {x.bad ? '' : 'Ignored: '}{x.message}
            </li>
          ))}
        </ul>
      )}
      {report && issues.length === 0 && <p className="wp-muted wp-small" role="status">No problems found.</p>}
      <div className="wp-row wp-wrap">
        <button type="button" className="wp-btn primary" onClick={() => void save()} disabled={(report?.errors.length || 0) > 0}>Save</button>
        <button type="button" className="wp-btn" onClick={() => void check(text)}>Check</button>
        <button type="button" className="wp-btn" onClick={() => void importFile()}>Open a file…</button>
        <button type="button" className="wp-btn" onClick={exportFile}>Download</button>
        <button type="button" className="wp-btn" onClick={() => { setText(TEMPLATE); setDirty(true) }}>Example</button>
        {!confirmReset
          ? <button type="button" className="wp-btn" onClick={() => setConfirmReset(true)}>Reset…</button>
          : <button type="button" className="wp-btn danger" onClick={() => void api.reset(level).then(() => { setText(layerDocument(undefined)); setDirty(false); setReport(null); setConfirmReset(false); say(`Reset ${levelName(level, consoleName)}`) }, (e) => say(e?.message || 'That didn’t work'))}>Really reset {levelName(level, consoleName)}</button>}
      </div>
      {core && core.options.length > 0 && (
        <details className="wp-section">
          <summary>What {core.core} accepts ({core.options.length})</summary>
          <input className="wp-input" type="search" placeholder="Find an option…" aria-label="Find an option" value={filter} onChange={(e) => setFilter(e.target.value)} />
          <ul className="wp-accepts">
            {options.map((o) => (
              <li key={o.key}>
                <button type="button" className="wp-link wp-mono" onClick={() => insert(`"${o.key}": "${o.default}"`)} aria-label={`Insert ${o.key}`}>{o.key}</button>
                <small className="wp-muted">{o.label}: {o.choices.join(', ')}</small>
              </li>
            ))}
          </ul>
        </details>
      )}
    </>
  )
}

// ---- cheats ----

type Cheat = { label: string; code: string; enabled: boolean }

export function CheatsPanel({ host, itemId, title, say }: { host: SettingsHost; itemId: number; title: string; say: (t: string) => void }) {
  const [list, setList] = useState<Cheat[] | null>(null)
  const [label, setLabel] = useState('')
  const [code, setCode] = useState('')
  useEffect(() => { host.request<Cheat[]>('GET', `/games/${itemId}/cheats`).then(setList).catch(() => setList([])) }, [host, itemId])
  const put = async (next: Cheat[]) => {
    setList(next)
    try { setList(await host.request<Cheat[]>('PUT', `/games/${itemId}/cheats`, { cheats: next })) } catch (e: any) { say(e?.message || 'That couldn’t be saved') }
  }
  const add = () => {
    if (!code.trim() || !list) return
    void put([...list, { label: label.trim() || code.trim(), code: code.trim(), enabled: true }])
    setLabel('')
    setCode('')
  }
  const importFile = async () => {
    const f = await pickFile('.cht,.txt,text/plain')
    if (!f || !list) return
    try {
      const r = await host.upload<{ cheats: Cheat[]; skipped: number }>(`/games/${itemId}/cheats/import`, f)
      await put([...list, ...r.cheats])
      say(`${r.cheats.length} added${r.skipped ? `, ${r.skipped} couldn’t be read` : ''}`)
    } catch (e: any) { say(e?.message || 'That file couldn’t be read') }
  }
  if (!list) return <p className="wp-muted" role="status">Loading…</p>
  return (
    <>
      <p className="wp-muted wp-small">Your own codes, for you only. They go to the core’s cheat system, and not every core has one that works. Nothing comes with this server.</p>
      {list.length === 0 && <p className="wp-muted">No cheats for this game yet.</p>}
      <ul className="wp-list">
        {list.map((c, i) => (
          <li key={i} className="wp-setting">
            <div className="wp-setting-text"><b>{c.label}</b><small className="wp-muted wp-mono">{c.code}</small></div>
            <label className="wp-toggle"><input type="checkbox" checked={c.enabled} aria-label={c.label} onChange={(e) => void put(list.map((x, j) => (j === i ? { ...x, enabled: e.target.checked } : x)))} /><span aria-hidden="true">{c.enabled ? 'On' : 'Off'}</span></label>
            <button type="button" className="wp-link" aria-label={`Remove ${c.label}`} onClick={() => void put(list.filter((_, j) => j !== i))}>Remove</button>
          </li>
        ))}
      </ul>
      <div className="wp-add">
        <input className="wp-input" placeholder="Name" aria-label="Cheat name" value={label} maxLength={120} onChange={(e) => setLabel(e.target.value)} />
        <input className="wp-input wp-mono" placeholder="Code" aria-label="Cheat code" value={code} maxLength={2048} onChange={(e) => setCode(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') add() }} />
        <button type="button" className="wp-btn" disabled={!code.trim()} onClick={add}>Add</button>
      </div>
      <div className="wp-row wp-wrap">
        <button type="button" className="wp-btn" onClick={() => void importFile()}>Add from a file…</button>
        <button type="button" className="wp-btn" disabled={!list.length} onClick={() => downloadBlob(new Blob([JSON.stringify(list, null, 2)], { type: 'application/json' }), `${fileName(title)} cheats.json`)}>Download</button>
      </div>
    </>
  )
}

// ---- patches ----

type Patch = { id: number; name: string; kind: string; size: number; enabled: boolean }

export function PatchesPanel({ host, itemId, say, onRestart }: { host: SettingsHost; itemId: number; say: (t: string) => void; onRestart?: () => void }) {
  const [list, setList] = useState<Patch[] | null>(null)
  const [changed, setChanged] = useState(false)
  useEffect(() => { host.request<Patch[]>('GET', `/games/${itemId}/patches`).then(setList).catch(() => setList([])) }, [host, itemId])
  const add = async () => {
    const f = await pickFile('.ips,.bps')
    if (!f) return
    try {
      setList(await host.upload<Patch[]>(`/games/${itemId}/patches?name=${encodeURIComponent(f.name)}`, f))
      setChanged(true)
    } catch (e: any) { say(e?.message || 'That patch couldn’t be used') }
  }
  const toggle = async (p: Patch, on: boolean) => {
    try {
      await host.request('PUT', `/games/${itemId}/patches/${p.id}`, { enabled: on })
      setList((l) => l!.map((x) => (x.id === p.id ? { ...x, enabled: on } : x)))
      setChanged(true)
    } catch (e: any) { say(e?.message || 'That didn’t work') }
  }
  const remove = async (p: Patch) => {
    try {
      await host.request('DELETE', `/games/${itemId}/patches/${p.id}`)
      setList((l) => l!.filter((x) => x.id !== p.id))
      setChanged(true)
    } catch (e: any) { say(e?.message || 'That didn’t work') }
  }
  if (!list) return <p className="wp-muted" role="status">Loading…</p>
  return (
    <>
      <p className="wp-muted wp-small">IPS and BPS patches of your own, such as translations and fixes. They’re applied to a copy each time the game starts: your game file is never changed, and they’re for you only.</p>
      {list.length === 0 && <p className="wp-muted">No patches for this game yet.</p>}
      <ul className="wp-list">
        {list.map((p) => (
          <li key={p.id} className="wp-setting">
            <div className="wp-setting-text"><b>{p.name}</b><small className="wp-muted">{p.kind.toUpperCase()}, {Math.ceil(p.size / 1024)} KB</small></div>
            <label className="wp-toggle"><input type="checkbox" checked={p.enabled} aria-label={p.name} onChange={(e) => void toggle(p, e.target.checked)} /><span aria-hidden="true">{p.enabled ? 'On' : 'Off'}</span></label>
            <button type="button" className="wp-link" aria-label={`Remove ${p.name}`} onClick={() => void remove(p)}>Remove</button>
          </li>
        ))}
      </ul>
      <button type="button" className="wp-btn" onClick={() => void add()}>Add a patch…</button>
      {changed && (onRestart
        ? <div className="wp-row wp-wrap"><span className="wp-muted wp-small wp-grow">This takes effect when the game next starts.</span><button type="button" className="wp-btn" onClick={onRestart}>Restart the game</button></div>
        : <p className="wp-muted wp-small">This takes effect when the game next starts.</p>)}
    </>
  )
}

// ---- saved states ----

export type SaveSlot = { slot: number; updatedAt: number; size: number; hasThumb: boolean; core?: string }

const withQuery = (u: string, q: string) => u + (u.includes('?') ? '&' : '?') + q

export const slotName = (n: number, schema?: Schema | null) => (n === 0 ? 'Automatic' : n === (schema?.undoSlot ?? 100) ? 'Before the last load' : n === (schema?.prevAutoSlot ?? 101) ? 'Before starting over' : `Slot ${n}`)

export function SlotsPanel({ host, schema, itemId, kind, confirm, act, say, onDone, core }: {
  host: SettingsHost; schema: Schema | null; itemId: number; kind: 'save' | 'load'; confirm: boolean; act: (n: number) => Promise<void>; say: (t: string) => void; onDone: (closed: boolean) => void; core?: string
}) {
  const [slots, setSlots] = useState<SaveSlot[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [arming, setArming] = useState(0)
  const [manage, setManage] = useState(false)
  const load = useCallback(() => host.request<SaveSlot[]>('GET', `/games/${itemId}/saves`).then(setSlots).catch(() => setSlots([])), [host, itemId])
  useEffect(() => { void load() }, [load])
  const go = async (n: number) => {
    const occupied = slots?.some((s) => s.slot === n)
    if (kind === 'save' && occupied && confirm && arming !== n) { setArming(n); return }
    setBusy(true)
    try {
      await act(n)
      say(kind === 'save' ? `Saved to ${slotName(n, schema).toLowerCase()}` : `Loaded ${slotName(n, schema).toLowerCase()}`)
      onDone(true)
    } catch (e: any) { say(e?.message || 'That didn’t work') } finally { setBusy(false) }
  }
  const max = schema?.maxSlot ?? 99
  const used = new Set((slots || []).map((s) => s.slot))
  // Every saved slot, and enough empty ones to always have a few to pick.
  const nums: number[] = []
  if (kind === 'save') {
    let empties = 0
    for (let n = 1; n <= max; n++) {
      if (used.has(n)) nums.push(n)
      else if (empties < 6) { nums.push(n); empties++ }
    }
  } else {
    for (const s of slots || []) nums.push(s.slot)
    nums.sort((a, b) => a - b)
  }
  const importTo = async (n: number) => {
    const f = await pickFile('.wpstate,.state,.state1,.state2,.state3,.bin')
    if (!f) return
    try {
      const r = await host.upload<{ warning?: string }>(`/games/${itemId}/saves/${n}/file`, f)
      say(r.warning || `Imported into ${slotName(n, schema).toLowerCase()}`)
      void load()
    } catch (e: any) { say(e?.message || 'That file couldn’t be used') }
  }
  const remove = async (n: number) => {
    try { await host.request('DELETE', `/games/${itemId}/saves/${n}`); void load() } catch (e: any) { say(e?.message || 'That didn’t work') }
  }
  const nextFree = (() => { for (let n = 1; n <= max; n++) if (!used.has(n)) return n; return 0 })()
  return (
    <>
      <div className="wp-slots">
        {nums.map((n) => {
          const s = slots?.find((x) => x.slot === n)
          const name = slotName(n, schema)
          const when = s ? new Date(s.updatedAt * 1000).toLocaleString() : 'Empty'
          const other = s?.core && core && s.core !== core
          return (
            <div key={n} className="wp-slot-wrap">
              <button type="button" className="wp-slot" disabled={busy} onClick={() => void go(n)} aria-label={`${name}: ${when}${arming === n ? '. Press again to replace it.' : ''}`}>
                {s?.hasThumb ? <img src={withQuery(host.mediaUrl(`/api/games/${itemId}/saves/${n}/thumb`), `v=${s.updatedAt}`)} alt="" /> : <span className="wp-slot-empty" />}
                <b>{arming === n ? 'Replace it?' : name}</b><small className="wp-muted">{when}</small>
                {other && <small className="wp-caution">Made by {s!.core}</small>}
              </button>
              {manage && s && (
                <span className="wp-slot-tools">
                  <a className="wp-link" href={host.mediaUrl(`/api/games/${itemId}/saves/${n}/file`)} download aria-label={`Download ${name}`}>Download</a>
                  <button type="button" className="wp-link" onClick={() => void remove(n)} aria-label={`Delete ${name}`}>Delete</button>
                </span>
              )}
            </div>
          )
        })}
        {kind === 'load' && slots && slots.length === 0 && <p className="wp-muted">No saved states yet.</p>}
      </div>
      <div className="wp-row wp-wrap">
        <button type="button" className="wp-btn" onClick={() => setManage((m) => !m)} aria-pressed={manage}>{manage ? 'Done' : 'Download or delete…'}</button>
        {nextFree > 0 && <button type="button" className="wp-btn" onClick={() => void importTo(nextFree)}>Import a state file…</button>}
        <span className="wp-grow" />
        <button type="button" className="wp-btn" onClick={() => onDone(false)}>Back</button>
      </div>
    </>
  )
}
