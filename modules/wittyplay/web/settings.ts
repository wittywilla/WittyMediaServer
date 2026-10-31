import { useCallback, useEffect, useRef, useState } from 'react'

// A person's settings, as the server holds them: the same data for this player, the
// settings page and any other client (GET /api/games/settings, docs/GAMING-API.md).

export type Choice = { value: string; label: string }
export type SettingDef = {
  key: string; group: string; section?: string; label: string; help?: string; caution?: string
  kind: 'toggle' | 'choice' | 'slider' | 'color' | 'bezel'; default: unknown
  min?: number; max?: number; step?: number; unit?: string; choices?: Choice[]; shader?: boolean; motion?: boolean
}
export type Preset = { name: string; description?: string; settings?: Record<string, unknown>; coreOptions?: Record<string, string> }
export type Schema = { settings: SettingDef[]; presets: Preset[]; levels: string[]; maxSlot: number; undoSlot: number; prevAutoSlot: number }
export type CustomControl = { label: string; help?: string; type: 'toggle' | 'choice' | 'slider'; option: string; on?: string; off?: string; choices?: Choice[] }
export type Layer = { settings?: Record<string, unknown>; coreOptions?: Record<string, string>; controls?: CustomControl[]; presets?: Preset[] }
export type Resolved = { values: Record<string, unknown>; sources: Record<string, string>; coreOptions: Record<string, string>; coreSources: Record<string, string>; controls: CustomControl[]; presets: Preset[] }
export type Limits = { maxSpeed: number; rewindBytes: number; runAhead: boolean }
export type Issue = { line?: number; column?: number; path?: string; message: string }
export type Report = { errors: Issue[]; ignored: Issue[] }
export type SettingsView = { resolved: Resolved; layers: Record<string, Layer>; limits: Limits; console: string; item: number; ignored?: Issue[] }
export type Level = 'person' | 'console' | 'game' | 'server' | 'serverConsole'
export type OptionInfo = { key: string; label: string; choices: string[]; default: string }
export type CoreOptionsView = { core: string; known: boolean; options: OptionInfo[]; values: Record<string, string>; sources: Record<string, string>; running: Record<string, string>; ignored: Issue[]; controls: CustomControl[] }

// How the panels reach the server. The app provides it.
export interface SettingsHost {
  request<T>(method: string, path: string, body?: unknown): Promise<T>
  upload<T>(path: string, body: Blob): Promise<T>
  mediaUrl(path: string): string
  isAdmin: boolean
}

// Which level a change goes to, by the highest one that applies here.
export const LEVEL_RANK: Record<string, number> = { builtin: 0, server: 1, serverConsole: 2, person: 3, console: 4, game: 5 }

export function levelName(level: string, consoleName?: string): string {
  switch (level) {
    case 'game': return 'this game'
    case 'console': return consoleName ? `all ${consoleName} games` : 'this console'
    case 'person': return 'all your games'
    case 'server': return 'everyone on this server'
    case 'serverConsole': return consoleName ? `everyone’s ${consoleName} games` : 'everyone on this console'
    default: return 'the default'
  }
}

let schemaCache: Promise<Schema> | null = null
export function loadSchema(host: SettingsHost): Promise<Schema> {
  if (!schemaCache) schemaCache = host.request<Schema>('GET', '/games/settings/schema').catch((e) => { schemaCache = null; throw e })
  return schemaCache
}

export type Scope = { console?: string; item?: number }

function query(s: Scope) {
  const q = new URLSearchParams()
  if (s.console) q.set('console', s.console)
  if (s.item) q.set('item', String(s.item))
  const t = q.toString()
  return t ? '?' + t : ''
}

// useSettings keeps a view of the settings for one game or console. Changes show at once
// and reach the server a moment later, so that dragging a slider doesn't send a hundred
// requests. A change the server refuses comes back as the server has it.
export function useSettings(host: SettingsHost, scope: Scope, onError: (m: string) => void) {
  const [view, setView] = useState<SettingsView | null>(null)
  const pending = useRef(new Map<Level, { settings: Record<string, unknown>; coreOptions: Record<string, string | null> }>())
  const timer = useRef(0)
  const scopeRef = useRef(scope)
  scopeRef.current = scope

  const reload = useCallback(() => host.request<SettingsView>('GET', '/games/settings' + query(scopeRef.current)).then((v) => { setView(v); return v }), [host])
  useEffect(() => { reload().catch((e) => onError(e?.message || 'Your settings could not be read')) }, [reload, scope.console, scope.item]) // eslint-disable-line react-hooks/exhaustive-deps

  const flush = useCallback(async () => {
    window.clearTimeout(timer.current)
    timer.current = 0
    const batches = [...pending.current.entries()]
    pending.current.clear()
    for (const [level, patch] of batches) {
      try {
        const v = await host.request<SettingsView>('PUT', '/games/settings', { level, ...scopeRef.current, settings: patch.settings, coreOptions: patch.coreOptions })
        if (!pending.current.size) setView(v)
      } catch (e: any) {
        onError(e?.message || 'That setting could not be saved')
        void reload().catch(() => {})
      }
    }
  }, [host, onError, reload])

  // Everything still waiting goes before the screen closes.
  useEffect(() => () => { if (timer.current) void flush() }, [flush])

  const schedule = useCallback(() => {
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => void flush(), 200)
  }, [flush])

  const batch = (level: Level) => {
    let b = pending.current.get(level)
    if (!b) {
      b = { settings: {}, coreOptions: {} }
      pending.current.set(level, b)
    }
    return b
  }

  // set changes a value at a level; null takes it away, so the level below shows through.
  const set = useCallback((level: Level, key: string, value: unknown) => {
    batch(level).settings[key] = value
    setView((v) => {
      if (!v) return v
      const layer = { ...(v.layers[level] || {}), settings: { ...(v.layers[level]?.settings || {}) } }
      if (value === null) delete layer.settings[key]
      else layer.settings[key] = value
      const resolved = { ...v.resolved, values: { ...v.resolved.values }, sources: { ...v.resolved.sources } }
      if (value !== null && LEVEL_RANK[level] >= (LEVEL_RANK[resolved.sources[key]] ?? 0)) {
        resolved.values[key] = value
        resolved.sources[key] = level
      }
      return { ...v, layers: { ...v.layers, [level]: layer }, resolved }
    })
    schedule()
  }, [schedule])

  const setOption = useCallback((level: Level, key: string, value: string | null) => {
    batch(level).coreOptions[key] = value
    setView((v) => {
      if (!v) return v
      const resolved = { ...v.resolved, coreOptions: { ...v.resolved.coreOptions }, coreSources: { ...v.resolved.coreSources } }
      if (value !== null && LEVEL_RANK[level] >= (LEVEL_RANK[resolved.coreSources[key]] ?? 0)) {
        resolved.coreOptions[key] = value
        resolved.coreSources[key] = level
      }
      const layer = { ...(v.layers[level] || {}), coreOptions: { ...(v.layers[level]?.coreOptions || {}) } }
      if (value === null) delete layer.coreOptions[key]
      else layer.coreOptions[key] = value
      return { ...v, layers: { ...v.layers, [level]: layer }, resolved }
    })
    schedule()
  }, [schedule])

  const applyPreset = useCallback(async (level: Level, p: Preset) => {
    await flush()
    const v = await host.request<SettingsView>('PUT', '/games/settings', { level, ...scopeRef.current, settings: p.settings || {}, coreOptions: p.coreOptions || {} })
    setView(v)
    return v
  }, [flush, host])

  const reset = useCallback(async (level: Level) => {
    await flush()
    const v = await host.request<SettingsView>('PUT', '/games/settings', { level, ...scopeRef.current, reset: true })
    setView(v)
  }, [flush, host])

  const saveDocument = useCallback(async (level: Level, document: string) => {
    await flush()
    const v = await host.request<SettingsView>('PUT', '/games/settings', { level, ...scopeRef.current, document })
    setView(v)
    return v
  }, [flush, host])

  return { view, set, setOption, applyPreset, reset, saveDocument, reload, flush }
}

export type SettingsApi = ReturnType<typeof useSettings>

// The file for a level: exactly what a person can edit, save, download and share.
export function layerDocument(l: Layer | undefined): string {
  const doc: Record<string, unknown> = { wittyplay: 1 }
  if (l?.settings && Object.keys(l.settings).length) doc.settings = l.settings
  if (l?.coreOptions && Object.keys(l.coreOptions).length) doc.coreOptions = l.coreOptions
  if (l?.controls?.length) doc.controls = l.controls
  if (l?.presets?.length) doc.presets = l.presets
  return JSON.stringify(doc, null, 2) + '\n'
}

export function downloadBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 30000)
}

// A file chooser, from any button.
export function pickFile(accept: string): Promise<File | null> {
  return new Promise((resolve) => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = accept
    input.onchange = () => resolve(input.files?.[0] || null)
    input.oncancel = () => resolve(null)
    input.click()
  })
}

export const fileName = (t: string) => t.replace(/[^\p{L}\p{N} ._-]+/gu, '').trim().slice(0, 80) || 'game'
