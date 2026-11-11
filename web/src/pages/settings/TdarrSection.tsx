// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { PageHelp } from '../../components/HelpLink'
import { api } from '../../api'
import { Button, Field, Input, LoadError, Notice, Select, Spinner, toast } from '../../ui'
import { ConfirmOwner, type OwnerAsk } from './ConfirmOwner'
import { Section, Switch } from './ui'
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
  return (
    <Section title="Connect your own Tdarr" actions={<PageHelp to="tdarr" />} desc="If you already use Tdarr, or want spare computers doing the work, this server can watch it and hand it files. It’s optional: everything above works without it.">
      <Notice title="Tdarr is a separate program, and it isn’t open source.">
        <p>It’s made by other people and comes under its own licence, with free and paid tiers. It isn’t included here, and this server doesn’t install it. You download it from tdarr.io and agree to its terms yourself.</p>
        <p><b>About its nodes.</b> A Tdarr node connects to a Tdarr server, and that connection isn’t documented for anything else to use, so nodes can’t connect directly to this server. Attach them to your Tdarr server as usual, and connect that here: you’ll see them below, and what they’re working on shows in the activity panel.</p>
        <p><b>What it decides.</b> Tdarr’s own plugins and flows decide what happens to a file, not the recipes above. If it changes a file’s extension, this server sees a new file on its next scan, and what’s been watched won’t carry over to it.</p>
      </Notice>
      <div className="ltc-pick">
        <Field label="Tdarr server address" hint="Its server port, which is 8266 unless you’ve changed it. Not the web page port.">
          <Input value={url} placeholder="http://192.168.1.20:8266" autoComplete="off" spellCheck={false} onChange={(e) => setUrl(e.target.value)} />
        </Field>
        <Field label="API key" hint={saved.keySet ? 'A key is saved. Type a new one to replace it.' : 'In Tdarr, under Tools, API Keys. Leave empty if its sign-in is off.'}>
          <Input type="password" value={key} autoComplete="new-password" placeholder={saved.keySet ? 'Saved' : ''} onChange={(e) => setKey(e.target.value)} />
        </Field>
      </div>
      <p className="ltc-actions">
        <Button disabled={!url && !saved.url} busy={busy === 'test'} onClick={() => askThen('Try this address?', 'Test the connection', test)}>Test the connection</Button>
        {saved.keySet && <Button variant="ghost" size="sm" disabled={!!busy} onClick={() => askThen('Save this address?', 'Forget the saved key', (s) => save({ clearKey: true }, s))}>Forget the saved key</Button>}
      </p>
      <div aria-live="polite">
        {report && !report.ok && <Notice tone="danger" title="That didn’t work">{report.error}</Notice>}
        {report?.ok && (
          <Notice tone={report.error ? 'warn' : 'ok'} title={`Connected to Tdarr${report.version ? ' ' + report.version : ''}`}>
            {report.error && <p>It answered, but not everything worked: {report.error}</p>}
            {report.nodes.length === 0
              ? <p>No nodes are attached to it right now.</p>
              : <ul className="ltc-list">{report.nodes.map((n) => (
                  <li key={n.id}><b>{n.name}</b>{n.paused ? ', paused' : ''}: {n.workers.filter((w) => w.file).length ? n.workers.filter((w) => w.file).map((w) => `${w.file.split(/[\\/]/).pop()} (${Math.round(w.percent)}%)`).join(', ') : 'nothing to do'}</li>
                ))}</ul>}
            <p>{report.libraries.length} {report.libraries.length === 1 ? 'library' : 'libraries'} set up there.</p>
          </Notice>
        )}
      </div>
      <div className="set-row">
        <div className="set-row-text">
          <div className="set-row-label">Use this connection</div>
          <div className="set-row-desc">Shows what Tdarr is doing in the activity panel, asking it every 20 seconds while something’s going on or someone has the panel open, and every 2 minutes otherwise. Off, nothing contacts it.</div>
        </div>
        <div className="set-row-ctl"><Switch label="Use this connection" checked={enabled} onChange={setEnabled} /></div>
      </div>
      {enabled && libs.length > 0 && (
        <div className="ltc-tdarr-libs">
          <h3>Handing files to Tdarr</h3>
          <p className="ltc-muted">{known.length ? 'Choose which of Tdarr’s libraries each one goes to.' : 'Test the connection to list Tdarr’s libraries.'} If the two machines call a folder by different names, say so, so a file can be named the way Tdarr knows it.</p>
          {libs.map((l) => {
            const m = mapOf(l.id)
            const list = known.some((k) => k.id === m.tdarrId) || !m.tdarrId ? known : [{ id: m.tdarrId, name: m.tdarrId, folder: '' }, ...known]
            return (
              <div key={l.id} className="ltc-tdarr-lib">
                <b>{l.name}</b>
                <div className="ltc-pick">
                  <Field label="Tdarr library">
                    <Select value={m.tdarrId} onChange={(e) => setMap(l.id, { tdarrId: e.target.value })}>
                      <option value="">Not connected</option>
                      {list.map((k) => <option key={k.id} value={k.id}>{k.name}</option>)}
                    </Select>
                  </Field>
                  <Field label="Folder name here"><Input value={m.from} placeholder="/srv/films" spellCheck={false} onChange={(e) => setMap(l.id, { from: e.target.value })} /></Field>
                  <Field label="Folder name in Tdarr"><Input value={m.to} placeholder="/media" spellCheck={false} onChange={(e) => setMap(l.id, { to: e.target.value })} /></Field>
                </div>
                <p className="ltc-actions"><Button size="sm" disabled={!saved.enabled || !saved.mappings.some((x) => x.libraryId === l.id && x.tdarrId)} busy={busy === 'send' + l.id} onClick={() => send(l.id)}>Ask Tdarr to look through this library</Button></p>
              </div>
            )
          })}
        </div>
      )}
      <p className="ltc-actions"><Button variant="primary" disabled={!dirty} busy={busy === 'save'} onClick={() => askThen('Save this address?', 'Save the connection', (s) => save({}, s))}>Save the connection</Button></p>
      {ask && <ConfirmOwner ask={ask} onClose={() => setAsk(null)} />}
    </Section>
  )
}
