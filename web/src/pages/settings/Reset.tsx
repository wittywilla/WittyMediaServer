// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { fmtDateTime, api, setToken } from '../../api'
import { Spinner } from '../../components/common'
import { toast } from '../../state'
import { LinkButton, Notice } from '../../ui'
import { PageHead, Section } from './ui'
import { num, t, tk } from '../../i18n'

// Reset: back to a new install, or back to a copy an earlier reset kept. The server
// does it as it starts up, so this page waits for it to come back.

type Copy = { name: string; at: number; bytes: number }
type View = {
  dataDir: string; outside: string[]; blocked?: string; copies: Copy[]; canRestart: boolean; twoFactor: boolean
  pending?: { mode: string }
  last?: { ok: boolean; mode: string; copy?: string; error?: string; notes?: string[]; at: string }
}

const size = (n: number) => (n >= 1 << 30 ? `${num(n / (1 << 30), { minimumFractionDigits: 1, maximumFractionDigits: 1 })} GB` : `${num(Math.max(1, Math.round(n / (1 << 20))))} MB`)

// Waits for the server to go away and come back, then starts the page again.
async function waitForRestart(setStatus: (s: string) => void) {
  const answers = async () => {
    try {
      const r = await fetch('/api/status', { cache: 'no-store' })
      return r.ok
    } catch { return false }
  }
  const started = Date.now()
  let wentAway = false
  while (Date.now() - started < 120000) {
    await new Promise((ok) => setTimeout(ok, 1000))
    const up = await answers()
    if (!up) { wentAway = true; continue }
    if (wentAway || Date.now() - started > 8000) {
      setToken('')
      window.location.href = '/'
      return
    }
  }
  setStatus(t('The server hasn’t come back. Start it again by hand: the reset happens as it starts.'))
}

function Secrets({ twoFactor, password, code, setPassword, setCode }: { twoFactor: boolean; password: string; code: string; setPassword: (s: string) => void; setCode: (s: string) => void }) {
  return (
    <>
      <label className="oobe-field">{t('Your password')}
        <input className="input" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </label>
      {twoFactor && (
        <label className="oobe-field">{t('Code from your authenticator app')}
          <input className="input" inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} />
        </label>
      )}
    </>
  )
}

export default function ResetSettings() {
  const [view, setView] = useState<View | null>(null)
  const [mode, setMode] = useState<'keep' | 'delete'>('keep')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [waiting, setWaiting] = useState('')
  const [picked, setPicked] = useState<{ copy: string; action: 'restore' | 'delete' } | null>(null)
  const load = () => api.get<View>('/reset').then(setView).catch((e) => setErr(e.message))
  useEffect(() => { void load() }, [])
  if (!view) return err ? <div className="error" role="alert">{err}</div> : <Spinner />

  const restarted = (r: { restarting: boolean }, what: string) => {
    if (!r.restarting) {
      setWaiting(t('{what} This server can’t restart itself: stop it and start it again, and it happens then.', { what }))
      return
    }
    setWaiting(t('{what} The server is restarting, which can take a minute.', { what }))
    void waitForRestart(setWaiting)
  }
  const go = async (e: React.FormEvent) => {
    e.preventDefault(); setErr(''); setBusy(true)
    try {
      const r = await api.post<{ restarting: boolean }>('/reset', { password, code, confirm: typed, delete: mode === 'delete' })
      restarted(r, mode === 'delete' ? t('Deleting everything.') : t('Resetting.'))
    } catch (x: any) { setErr(x.message); setBusy(false) }
  }
  const onCopy = async (e: React.FormEvent) => {
    e.preventDefault(); if (!picked) return
    setErr(''); setBusy(true)
    try {
      if (picked.action === 'restore') {
        restarted(await api.post<{ restarting: boolean }>('/reset/restore', { password, code, copy: picked.copy }), t('Bringing that copy back.'))
        return
      }
      await api.post('/reset/copies/delete', { password, code, copy: picked.copy })
      toast(t('Copy deleted')); setPicked(null); setPassword(''); setCode(''); setBusy(false); void load()
    } catch (x: any) { setErr(x.message); setBusy(false) }
  }
  const cancel = async () => {
    try { await api.del('/reset/request'); toast(t('The reset was cancelled')); void load() } catch (x: any) { setErr(x.message) }
  }

  if (waiting) {
    return (
      <>
        <PageHead title={tk('Reset this server')} />
        <Section title={tk('Please wait')}><div role="status" className="set-prose"><p>{waiting}</p><p>{t('This page moves on by itself when it’s done.')}</p></div></Section>
      </>
    )
  }
  const deleting = mode === 'delete'
  const ready = password !== '' && typed === 'RESET' && !busy && (!view.twoFactor || code !== '')
  return (
    <>
      <PageHead title={tk('Reset this server')} desc={t('Puts this server back the way it was first installed, so you can set it up from the start.')} />
      {view.last && !view.last.ok && <div className="error" role="alert">{t('The last reset didn’t happen, and nothing was changed: {error}', { error: view.last.error || '' })}</div>}
      {view.last?.ok && (view.last.notes || []).map((n) => <p key={n} className="set-note" role="status">{n}</p>)}
      {view.pending && (
        <div className="set-note bad" role="status">
          {t('A reset is waiting: it happens the next time this server starts.')} <button className="btn sm" onClick={cancel}>{t('Cancel it')}</button>
        </div>
      )}
      <Notice tone="info" title={t('Make a backup first')} action={<LinkButton to="/settings/backup">{t('Go to Backup')}</LinkButton>}>
        {t('A backup is a ZIP you can keep anywhere, and bring back from first-time setup after a reset. Backups already on this server stay through a reset that keeps a copy; deleting everything deletes them too.')}
      </Notice>
      <Section title={tk('What a reset does')} desc={t('Read this first. It applies to everyone who uses this server, not only you.')}>
        <div className="set-prose">
          <p><b>{t('These go away:')}</b></p>
          <ul>
            <li>{t('Every account and profile, and every device’s sign-in. Apps have to connect again.')}</li>
            <li>{t('The list of libraries, and everything known about what’s in them: watch progress, favourites, ratings, playlists, collections and notes.')}</li>
            <li>{t('All settings and API keys, Remote access, TV tuners, channels and scheduled recordings.')}</li>
            <li><b>{t('Game saves and saved states')}</b>{t(', controller profiles, game cores and custom cores.')}</li>
            <li>{t('Installed module versions, logs, reports, and the cache of posters and converted video.')}</li>
            <li><b>{t('Recordings')}</b>{t(', if they’re kept in the data folder, which is where they go unless you chose another.')}</li>
          </ul>
          <p><b>{t('These stay, whichever you choose:')}</b> {t('your media files and game files, wherever they are, anything saved beside them such as posters and info files, the program itself and its config file.')}</p>
          <p>{t('Data folder:')} <code>{view.dataDir}</code></p>
          {view.outside.length > 0 && <p>{t('These folders are set up elsewhere. What’s in them is moved into a')} <code>before-reset</code> {t('folder inside each, and never deleted, so that’s yours to clear:')} {view.outside.map((o) => <code key={o}>{o} </code>)}</p>}
          {!view.canRestart && <p>{t('This server can’t restart itself. Once you’ve asked, stop it and start it again, and the reset happens then.')}</p>}
          <p>{t('Anyone watching or playing now is interrupted. To set the server up again from another device, the setup code is shown on the server’s console.')}</p>
        </div>
      </Section>
      {view.blocked ? (
        <Section title={tk('This server can’t be reset as it’s set up')}><div className="error" role="alert">{view.blocked}</div></Section>
      ) : (
        <Section title={tk('Reset')}>
          <form className="form full" onSubmit={go}>
            <fieldset className="set-fieldset">
              <legend>{t('What happens to what’s there now')}</legend>
              <label className="set-radio"><input type="radio" name="reset-mode" checked={!deleting} onChange={() => setMode('keep')} /> <span><b>{t('Keep a copy')}</b> {t('(recommended). It’s all moved into a folder inside the data folder, and you can bring it back from this page. It still takes up disk space.')}</span></label>
              <label className="set-radio"><input type="radio" name="reset-mode" checked={deleting} onChange={() => setMode('delete')} /> <span><b>{t('Delete everything for good')}</b>{t(', including copies from earlier resets and backups kept on this server. This can’t be undone.')}</span></label>
            </fieldset>
            {deleting && <div className="set-note bad" role="alert">{t('Nothing is kept, and this can’t be undone: game saves, watch progress and any recordings in the data folder are gone for good.')}</div>}
            <Secrets twoFactor={view.twoFactor} password={password} code={code} setPassword={setPassword} setCode={setCode} />
            <label className="oobe-field">{t('Type {word} to confirm', { word: 'RESET' })}
              <input className="input" autoComplete="off" spellCheck={false} value={typed} onChange={(e) => setTyped(e.target.value)} />
            </label>
            {err && !picked && <div className="error" role="alert">{err}</div>}
            <div className="actions"><button className="btn danger" disabled={!ready}>{deleting ? t('Delete everything and reset') : t('Reset this server')}</button></div>
          </form>
        </Section>
      )}
      {view.copies.length > 0 && (
        <Section title={tk('Copies from earlier resets')} desc={t('Bringing one back replaces what’s on this server now. What’s there now is kept as another copy.')}>
          <ul className="set-list">
            {view.copies.map((c) => (
              <li key={c.name}>
                <span><b>{fmtDateTime(c.at * 1000)}</b> <span className="muted">{size(c.bytes)}</span></span>{' '}
                <button className="btn sm" onClick={() => { setErr(''); setPassword(''); setCode(''); setPicked({ copy: c.name, action: 'restore' }) }}>{t('Bring back')}</button>{' '}
                <button className="btn sm ghost" onClick={() => { setErr(''); setPassword(''); setCode(''); setPicked({ copy: c.name, action: 'delete' }) }}>{t('Delete')}</button>
              </li>
            ))}
          </ul>
          {picked && (
            <form className="form full" onSubmit={onCopy}>
              <p>{picked.action === 'restore' ? t('Bring this copy back, and restart the server?') : t('Delete this copy for good? This can’t be undone.')}</p>
              <Secrets twoFactor={view.twoFactor} password={password} code={code} setPassword={setPassword} setCode={setCode} />
              {err && <div className="error" role="alert">{err}</div>}
              <div className="actions">
                <button type="button" className="btn ghost" onClick={() => { setPicked(null); setErr('') }}>{t('Cancel')}</button>
                <button className={'btn ' + (picked.action === 'delete' ? 'danger' : 'primary')} disabled={busy || !password || (view.twoFactor && !code)}>{picked.action === 'restore' ? t('Bring it back') : t('Delete it')}</button>
              </div>
            </form>
          )}
        </Section>
      )}
    </>
  )
}
