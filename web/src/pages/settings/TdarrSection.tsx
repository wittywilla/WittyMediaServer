// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { PageHelp } from '../../components/HelpLink'
import { api } from '../../api'
import { LoadError, Spinner, toast } from '../../ui'
import SpecPage from '../../spec/SpecPage'
import { tdarrSpec, type TdarrView } from '../../spec/specs/tdarr'
import { ConfirmOwner, type OwnerAsk } from './ConfirmOwner'
import { Section } from './ui'
import type { Library, TdarrMapping, TdarrReport, TdarrSettings } from './libraryTranscodingTypes'

// Connecting someone's own Tdarr server. Tdarr is a separate program under its own licence:
// nothing of it is included here, and they install it and agree to its terms themselves.

type Secrets = { password: string; code: string }

export default function TdarrSection({ libs }: { libs: Library[] }) {
  const [ask, setAsk] = useState<OwnerAsk | null>(null)
  const [saved, setSaved] = useState<TdarrSettings | null>(null)
  const [url, setUrl] = useState('')
  const [key, setKey] = useState('')
  const [enabled, setEnabled] = useState(false)
  const [mappings, setMappings] = useState<TdarrMapping[]>([])
  const [report, setReport] = useState<TdarrReport | null>(null)
  const [busy, setBusy] = useState('')
  const [failed, setFailed] = useState(false)
  const adopt = (s: TdarrSettings) => { setSaved(s); setUrl(s.url); setEnabled(s.enabled); setMappings(s.mappings); setKey('') }
  const load = useCallback(() => {
    setFailed(false)
    api.get<TdarrSettings>('/libtranscode/tdarr').then(adopt).catch(() => setFailed(true))
  }, [])
  useEffect(load, [load])
  if (failed) return <Section title="Tdarr"><LoadError what="This section" onRetry={load} /></Section>
  if (!saved) return <Section title="Tdarr"><Spinner /></Section>

  // Talking to a new address asks for the password. The dialog says what was wrong with one.
  const newAddress = !!url.trim() && url !== saved.url
  const test = async (secrets?: Secrets) => {
    setBusy('test')
    try { setReport(await api.post<TdarrReport>('/libtranscode/tdarr/test', { url, apiKey: key, ...secrets })) } catch (e) { if (secrets) throw e; toast((e as Error).message) } finally { setBusy('') }
  }
  const save = async (next: { enabled?: boolean; clearKey?: boolean } = {}, secrets?: Secrets) => {
    setBusy('save')
    try {
      adopt(await api.put<TdarrSettings>('/libtranscode/tdarr', { enabled: next.enabled ?? enabled, url, apiKey: key, clearKey: !!next.clearKey, mappings, ...secrets }))
      toast('Saved')
    } catch (e) { if (secrets) throw e; toast((e as Error).message) } finally { setBusy('') }
  }
  const askThen = (title: string, action: string, run: (secrets?: Secrets) => Promise<void>) => {
    if (!newAddress) { void run(); return }
    setAsk({ title, action, body: <p>This server will talk to an address it hasn’t been talking to, so this asks for your password.</p>, run: (password, code) => run({ password, code }) })
  }
  const send = async (libraryId: number) => {
    setBusy('send' + libraryId)
    try { toast((await api.post<{ message: string }>('/libtranscode/tdarr/send', { libraryId })).message) } catch (e) { toast((e as Error).message) } finally { setBusy('') }
  }
  const mapOf = (id: number) => mappings.find((m) => m.libraryId === id) ?? { libraryId: id, tdarrId: '', from: '', to: '' }
  const setMap = (id: number, patch: Partial<TdarrMapping>) => setMappings([...mappings.filter((m) => m.libraryId !== id), { ...mapOf(id), ...patch }])
  const dirty = url !== saved.url || key !== '' || enabled !== saved.enabled || JSON.stringify(mappings) !== JSON.stringify(saved.mappings)
  const known = report?.libraries ?? []
  const view: TdarrView = {
    url, key, keySet: saved.keySet, enabled, dirty, busy,
    libs: libs.map((l) => { const m = mapOf(l.id); return { id: l.id, name: l.name, tdarrId: m.tdarrId, from: m.from, to: m.to, canSend: saved.enabled && saved.mappings.some((x) => x.libraryId === l.id && x.tdarrId) } }),
    known: known.map((k) => ({ id: k.id, name: k.name })),
    report: report && { ok: report.ok, error: report.error, version: report.version, libraries: report.libraries.length, nodes: report.nodes.map((n) => { const w = n.workers.filter((x) => x.file); return { id: n.id, name: n.name, paused: n.paused, doing: w.length ? w.map((x) => `${x.file.split(/[\\/]/).pop()} (${Math.round(x.percent)}%)`).join(', ') : 'nothing to do' } }) },
  }
  const onField = (id: string, v: string | number | boolean | File) => {
    if (id === 'url') setUrl(String(v)); else if (id === 'key') setKey(String(v)); else if (id === 'enabled') setEnabled(!!v)
    else if (id.startsWith('map:')) { const [, lib, part] = id.split(':'); setMap(Number(lib), part === 'tdarr' ? { tdarrId: String(v) } : part === 'from' ? { from: String(v) } : { to: String(v) }) }
  }
  const onAction = (id: string) => {
    if (id === 'test') askThen('Try this address?', 'Test the connection', test)
    else if (id === 'forget') askThen('Save this address?', 'Forget the saved key', (s) => save({ clearKey: true }, s))
    else if (id === 'save') askThen('Save this address?', 'Save the connection', (s) => save({}, s))
    else if (id.startsWith('send:')) void send(Number(id.slice(5)))
  }
  const handled = new Set(['test', 'forget', 'save', ...libs.map((l) => 'send:' + l.id)])
  return (
    <>
      <div className="spec-page-help"><PageHelp to="tdarr" /></div>
      <SpecPage embedded page={tdarrSpec(view)} handled={handled} onAction={onAction} onField={onField} />
      {ask && <ConfirmOwner ask={ask} onClose={() => setAsk(null)} />}
    </>
  )
}
