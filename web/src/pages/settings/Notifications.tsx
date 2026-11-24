// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { KIND_NAMES, noteChannels, noteChoices, saveNoteChannels, setNoteChoices, testNoteChannels, type NoteChannels, type NoteChoice } from '../../notes'
import { advancedHere } from '../../ui/advanced'
import { Button, Input, LoadError, Notice, Select, Spinner, toast } from '../../ui'
import { useApp } from '../../state'
import { PageHead, Row, Section, Switch } from './ui'
import { t, tk, tx } from '../../i18n'
import '../../components/notes.css'

// What the bell tells this profile about, and for an administrator, where else their notifications may go.
export default function NotificationSettings() {
  const { user } = useApp()
  return (
    <>
      <PageHead title={tk('Notifications')} desc={t('The bell in the top bar: new episodes, finished recordings and updates. Each profile chooses its own.')} />
      <Kinds />
      {user?.isAdmin && <Channels />}
    </>
  )
}

function Kinds() {
  const [list, setList] = useState<NoteChoice[] | null>(null)
  const [failed, setFailed] = useState(false)
  const load = () => { setFailed(false); noteChoices().then(setList).catch(() => setFailed(true)) }
  useEffect(load, [])
  if (failed) return <LoadError what={t('Your choices')} onRetry={load} />
  if (!list) return <Spinner />
  const change = async (c: NoteChoice, on: boolean) => {
    setList(list.map((x) => (x.kind === c.kind ? { ...x, on } : x)))
    try { setList(await setNoteChoices([{ kind: c.kind, on }])) } catch (e) { toast((e as Error).message); load() }
  }
  return (
    <Section title={tk('What you’re told about')} desc={t('Only changes your own profile. Turned off, nothing of that kind is kept for you.')}>
      {list.map((c) => (
        <Row key={c.kind} label={tx(KIND_NAMES[c.kind].name)} desc={tx(KIND_NAMES[c.kind].about)}>
          <Switch checked={c.on} label={tx(KIND_NAMES[c.kind].name)} onChange={(v) => change(c, v)} />
        </Row>
      ))}
    </Section>
  )
}

// Off until set up, and only ever to what the administrator types here.
function Channels() {
  const [v, setV] = useState<NoteChannels | null>(null)
  const [failed, setFailed] = useState(false)
  const [password, setPassword] = useState('')
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null)
  const load = () => { setFailed(false); noteChannels().then(setV).catch(() => setFailed(true)) }
  useEffect(load, [])
  const title = tk('Advanced: send them on')
  const desc = t('Nothing leaves this server unless you set it up here. Your own notifications, the kinds you chose above, can also go to your email or to a webhook of your own.')
  if (!advancedHere()) {
    return <Section title={title} desc={desc}><Row label={t('On a computer')} desc={t('Email and webhooks are set up in a web browser on a computer.')} /></Section>
  }
  if (failed) return <LoadError what={t('These settings')} onRetry={load} />
  if (!v) return null
  const smtp = v.smtp, ch = v.channel
  const setSmtp = (p: Partial<typeof smtp>) => setV({ ...v, smtp: { ...smtp, ...p } })
  const setCh = (p: Partial<typeof ch>) => setV({ ...v, channel: { ...ch, ...p } })
  const save = async () => {
    setBusy(true); setResult(null)
    try {
      const out = await saveNoteChannels({ smtp: { ...smtp, password: password || undefined }, channel: { ...ch, webhookSecret: secret || undefined } })
      setV(out); setPassword(''); setSecret('')
      toast(t('Saved'))
    } catch (e) { setResult({ ok: false, text: (e as Error).message }) } finally { setBusy(false) }
  }
  const test = async () => {
    setBusy(true); setResult(null)
    try { await testNoteChannels(); setResult({ ok: true, text: t('Sent. Check your email or your webhook’s service.') }) } catch (e) { setResult({ ok: false, text: (e as Error).message }) } finally { setBusy(false) }
  }
  return (
    <Section title={title} desc={desc}>
      <Row label={t('Email')} desc={t('Sent through a mail server you have an account with.')}>
        <Switch checked={ch.emailOn} label={t('Email')} onChange={(on) => setCh({ emailOn: on })} />
      </Row>
      <Row label={t('Send to')}><Input type="email" autoComplete="email" value={ch.emailTo} onChange={(e) => setCh({ emailTo: e.target.value })} placeholder="you@example.com" /></Row>
      <Row label={t('Mail server')} desc={t('Its name, as your email provider gives it.')}><Input value={smtp.host} onChange={(e) => setSmtp({ host: e.target.value })} placeholder="smtp.example.com" /></Row>
      <Row label={t('Security')} desc={t('Without encryption, only a mail server on this machine can be used, so a password never crosses your network in the clear.')}>
        <Select value={smtp.security} onChange={(e) => setSmtp({ security: e.target.value as typeof smtp.security, port: e.target.value === 'tls' ? 465 : e.target.value === 'none' ? 25 : 587 })} options={[
          { value: 'starttls', label: t('STARTTLS (usually port 587)') },
          { value: 'tls', label: t('TLS (usually port 465)') },
          { value: 'none', label: t('None: this machine only') },
        ]} />
      </Row>
      <Row label={t('Port')}><Input type="number" min={1} max={65535} value={smtp.port || ''} onChange={(e) => setSmtp({ port: Number(e.target.value) })} /></Row>
      <Row label={t('User name')}><Input autoComplete="off" value={smtp.user} onChange={(e) => setSmtp({ user: e.target.value })} /></Row>
      <Row label={t('Password')} desc={smtp.passwordSet ? t('One is saved. Type a new one to change it.') : undefined}>
        <Input type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={smtp.passwordSet ? '••••••••' : ''} />
      </Row>
      <Row label={t('From address')} desc={t('Leave empty to send from the user name.')}><Input type="email" value={smtp.from} onChange={(e) => setSmtp({ from: e.target.value })} /></Row>

      <Row label={t('Webhook')} desc={t('Each notification is sent as JSON to an address on a service of your own, such as a home automation hub.')}>
        <Switch checked={ch.webhookOn} label={t('Webhook')} onChange={(on) => setCh({ webhookOn: on })} />
      </Row>
      <Row label={t('Webhook address')}><Input type="url" value={ch.webhookUrl} onChange={(e) => setCh({ webhookUrl: e.target.value })} placeholder="https://" /></Row>
      <Row label={t('Secret')} desc={ch.secretSet ? t('One is saved. Type a new one to change it.') : t('Optional. Signs each delivery so your service can tell it came from here.')}>
        <Input type="password" autoComplete="new-password" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={ch.secretSet ? '••••••••' : ''} />
      </Row>
      {result && <Notice tone={result.ok ? 'ok' : 'danger'} title={result.text} />}
      <div className="note-actions">
        <Button variant="primary" disabled={busy} onClick={save}>{t('Save')}</Button>
        <Button disabled={busy || (!ch.emailTo && !ch.webhookUrl)} onClick={test}>{t('Send a test')}</Button>
      </div>
    </Section>
  )
}
