// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../../api'
import { useModulePages } from '../../moduleBundles'
import { toast } from '../../state'
import { AUTO_THEME, currentThemeId, deviceTheme, loadThemeOverrides, type ThemeMeta } from '../../theme'
import { normalizedColours, overlay, type Colours } from '../../themeColours'
import { Button } from '../../ui/Button'
import { Field, Select } from '../../ui/Field'
import { Notice } from '../../ui/Surface'
import ColourOverrides, { type Look, type Warning } from './ColourOverrides'
import { Section } from './ui'

// The theme engine's colour overrides, for administrators (docs/THEMES.md "The theme
// engine"): single colours of the server's theme changed for everyone who follows it, and
// colours of their own for the pages that belong to one module. Both are saved on the
// server, and show on this page's own app the moment they are.

type Installed = { id: string; name?: string }

// The parts of the app that belong to a module and have pages of their own here.
const KNOWN: Installed[] = [{ id: 'wittytv', name: 'WittyTV' }, { id: 'wittyplay', name: 'WittyPlay' }]

type Engine = {
  // The server's theme, one look or (the automatic theme) two, each with the theme's own colours.
  looks: { scheme: 'dark' | 'light'; own: Colours }[]
  names: string[]
  global: Colours
  modules: Record<string, Colours>
  // What the server said is hard to read, by scope; null when its answer was about another
  // theme (this account's own), so there is nothing of its to show.
  warnings: Record<string, Warning[]> | null
}

// A theme's own colours by name: from the list of themes, or, for a theme from before
// they were listed, read from its stylesheet the way the server reads them.
async function ownColours(id: string, themes: ThemeMeta[]): Promise<Colours> {
  const listed = themes.find((t) => t.id === id)?.colors
  if (listed && Object.keys(listed).length) return normalizedColours(listed)
  const t = await api.get<{ colors?: Colours }>(`/themes/${encodeURIComponent(id)}/tokens.json`)
  const camel = (k: string) => k.replace(/-(.)/g, (_, c: string) => c.toUpperCase())
  return normalizedColours(Object.fromEntries(Object.entries(t.colors || {}).map(([k, v]) => [camel(k), v])))
}

async function loadEngine(): Promise<Engine> {
  const [list, theme] = await Promise.all([api.get<{ themes: ThemeMeta[]; default: string }>('/themes'), loadThemeOverrides()])
  if (!theme) throw new Error('The server’s colours couldn’t be read.')
  const id = currentThemeId(list.default || 'default')
  const scheme = list.themes.find((t) => t.id === id)?.scheme === 'light' ? 'light' : 'dark'
  const looks: Engine['looks'] = id === AUTO_THEME
    ? [{ scheme: 'dark', own: await ownColours('default', list.themes) }, { scheme: 'light', own: await ownColours('light', list.themes) }]
    // A theme that has gone is drawn as the default one, as the server does.
    : [{ scheme, own: await ownColours(id, list.themes).catch(() => ownColours('default', list.themes)) }]
  // The names the server takes are the ones the built-in themes give.
  const names = [...new Set(list.themes.filter((t) => t.builtin).flatMap((t) => Object.keys(t.colors || {})))]
  // An administrator with a theme of their own is answered about that theme, without the
  // server's overrides: those are then read from where they are kept.
  const followsServer = theme.source === 'server'
  const global = followsServer ? theme.overrides || {} : (await api.get<{ theme?: { tokens?: Colours } }>('/sdui/config')).theme?.tokens || {}
  const modules = Object.fromEntries(Object.entries(theme.modules || {}).map(([m, v]) => [m, v.overrides || {}]))
  let warnings: Engine['warnings'] = null
  if (followsServer) {
    warnings = {}
    for (const w of theme.warnings || []) (warnings[w.scope] ||= []).push(w)
  }
  return { looks, names, global: normalizedColours(global), modules, warnings }
}

export default function EngineColours() {
  const [engine, setEngine] = useState<Engine | null>(null)
  const [failed, setFailed] = useState('')
  const [mod, setMod] = useState(KNOWN[0].id)
  const pages = useModulePages()
  const load = useCallback(async () => {
    try { setEngine(await loadEngine()); setFailed('') } catch (e) { setFailed((e as Error).message) }
  }, [])
  useEffect(() => { load() }, [load])

  const mods = useMemo(() => {
    const out = [...KNOWN]
    for (const id of [...pages.map((p) => p.module), ...Object.keys(engine?.modules || {})]) if (!out.some((m) => m.id === id)) out.push({ id })
    return out
  }, [pages, engine])

  // Saving answers with the server's words when it refuses; afterwards the page and the app
  // around it both take what the server now has.
  const saved = async (work: Promise<unknown>, said: string) => { await work; await load(); toast(said, { tone: 'ok' }) }

  const globalLooks: Look[] = useMemo(() => (engine?.looks || []).map((l) => ({ scheme: l.scheme, base: l.own, set: [] })), [engine])
  const moduleLooks: Look[] = useMemo(() => (engine?.looks || []).map((l) => ({ scheme: l.scheme, base: overlay(l.own, engine?.global), set: Object.keys(engine?.global || {}) })), [engine])
  const own = deviceTheme()
  const modSaved = engine?.modules[mod] || {}
  const waiting = failed
    ? <Notice tone="danger" action={<Button size="sm" onClick={load}>Try again</Button>}>{failed}</Notice>
    : <p className="muted">Reading the server’s colours…</p>

  return (
    <>
      <Section title="Colours for everyone" desc="Change single colours of the server’s theme, for everyone who follows it. A colour you leave alone stays the theme’s.">
        {!engine ? waiting : (
          <>
            {own && <Notice>This device has a theme of its own, so it won’t show these colours. Devices that follow the server’s theme do.</Notice>}
            <ColourOverrides looks={globalLooks} names={engine.names} saved={engine.global} checked={engine.warnings ? engine.warnings.global || [] : null}
              hasOwn={Object.keys(engine.global).length > 0} clearLabel="Use the theme’s colours"
              save={(tokens) => saved(api.put('/theme/global', { tokens }), 'Saved for everyone.')}
              clear={() => saved(api.put('/theme/global', { tokens: {} }), 'Everyone has the theme’s colours again.')} />
          </>
        )}
      </Section>
      <Section title="Module colours" desc="Give one module’s pages colours of their own, for everyone. The rest of the app keeps the theme’s colours.">
        {!engine ? waiting : (
          <>
            <div className="form full">
              <Field label="Module" hint={Object.keys(modSaved).length ? 'This module has colours of its own.' : 'This module uses the theme’s colours.'}>
                <Select value={mod} onChange={(e) => setMod(e.target.value)} options={mods.map((m) => ({ value: m.id, label: m.name || m.id }))} />
              </Field>
            </div>
            <ColourOverrides key={mod} looks={moduleLooks} names={engine.names} saved={modSaved} checked={engine.warnings ? engine.warnings['module:' + mod] || [] : null}
              hasOwn={Object.keys(modSaved).length > 0} clearLabel="Use the theme’s colours"
              save={(tokens) => saved(api.put(`/theme/modules/${mod}`, { tokens }), 'Saved for everyone.')}
              clear={() => saved(api.del(`/theme/modules/${mod}`), 'This module uses the theme’s colours again.')} />
          </>
        )}
      </Section>
    </>
  )
}
