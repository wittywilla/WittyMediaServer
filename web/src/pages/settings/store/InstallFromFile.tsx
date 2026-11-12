// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useRef, useState } from 'react'
import { api, ApiError, fmtDateTime, getToken, serverBase } from '../../../api'
import { IconCheck, IconInfo, IconShield, IconFile } from '../../../icons'
import { toast } from '../../../state'
import { Button, Dialog, Progress } from '../../../ui'
import { ConfirmOwner } from '../ConfirmOwner'
import { OverrideDialog, type OverrideBody } from './OverrideDialog'
import { toDate, uploadView, type UploadResult } from './overrides'
import { mb } from './storeApi'
import './overrides.css'

// Installing a module from a file: a release archive someone built or was given, with its
// checksum list, signature and publisher key if there are any. The server looks at it first and
// says what it found; only then is there anything to install, through the same override dialog as
// any other install the usual rules would refuse. Nothing is kept unless it's installed: closing
// the dialog discards the upload.

type Picked = { archive: File | null; sums: File | null; sig: File | null; key: string }
type Stage = { at: 'choose' } | { at: 'uploading'; sent: number; total: number } | { at: 'found'; r: UploadResult }

const MAX = 512 * 1048576

// Sends the files as a form, reporting how much has gone. Abort stops it.
function upload(p: Picked, onProgress: (sent: number, total: number) => void, signal: AbortSignal): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const x = new XMLHttpRequest()
    const form = new FormData()
    form.append('archive', p.archive!, p.archive!.name)
    if (p.sums) form.append('sums', p.sums, p.sums.name)
    if (p.sig) form.append('sig', p.sig, p.sig.name)
    if (p.key.trim()) form.append('key', p.key.trim())
    x.open('POST', serverBase() + '/api/module-uploads')
    const tok = getToken()
    if (tok) x.setRequestHeader('Authorization', 'Bearer ' + tok)
    x.upload.onprogress = (e) => { if (e.lengthComputable) onProgress(e.loaded, e.total) }
    // A small file goes in one piece with no progress event: once it's all sent, say so, so the
    // page says the server is checking it rather than 0%.
    x.upload.onload = () => { const n = p.archive?.size || 1; onProgress(n, n) }
    x.onerror = () => reject(new ApiError(0, 'Can’t reach your server. Check that it’s running and try again.'))
    x.onabort = () => reject(new DOMException('Stopped', 'AbortError'))
    x.onload = () => {
      let data: any = null
      try { data = x.responseText ? JSON.parse(x.responseText) : null } catch { data = { error: x.responseText } }
      if (x.status >= 200 && x.status < 300 && data?.token) resolve(data as UploadResult)
      else reject(new ApiError(x.status, data?.error || x.statusText || 'The server didn’t accept the file.', data))
    }
    signal.addEventListener('abort', () => x.abort())
    x.send(form)
  })
}

export function InstallFromFile({ onInstalled, label = 'Install from a file…', variant = 'default' }: { onInstalled: () => void; label?: string; variant?: 'default' | 'primary' }) {
  const [open, setOpen] = useState(false)
  const btn = useRef<HTMLButtonElement>(null)
  return (
    <>
      <Button ref={btn} size="sm" variant={variant} icon={<IconFile />} onClick={() => setOpen(true)}>{label}</Button>
      {open && <UploadDialog onClose={() => { setOpen(false); setTimeout(() => btn.current?.focus(), 0) }} onInstalled={onInstalled} />}
    </>
  )
}

function UploadDialog({ onClose, onInstalled }: { onClose: () => void; onInstalled: () => void }) {
  const uid = useId()
  const [p, setP] = useState<Picked>({ archive: null, sums: null, sig: null, key: '' })
  const [stage, setStage] = useState<Stage>({ at: 'choose' })
  const [err, setErr] = useState('')
  const [installing, setInstalling] = useState<'override' | 'password' | null>(null)
  const stop = useRef<AbortController | null>(null)
  const token = useRef('')
  const found = useRef<HTMLHeadingElement>(null)
  const installed = useRef(false)

  // Whatever wasn't installed is discarded when the dialog goes, so nothing is left on the server.
  useEffect(() => () => {
    stop.current?.abort()
    if (token.current && !installed.current) void api.del(`/module-uploads/${encodeURIComponent(token.current)}`).catch(() => {})
  }, [])
  useEffect(() => { if (stage.at === 'found') found.current?.focus() }, [stage.at])

  const send = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!p.archive || stage.at === 'uploading') return
    if (p.archive.size > MAX) { setErr(`${p.archive.name} is ${mb(p.archive.size)}. A module’s archive can be at most 512 MB.`); return }
    setErr('')
    const ctl = new AbortController(); stop.current = ctl
    setStage({ at: 'uploading', sent: 0, total: p.archive.size })
    try {
      const r = await upload(p, (sent, total) => setStage({ at: 'uploading', sent, total }), ctl.signal)
      token.current = r.token
      setStage({ at: 'found', r })
    } catch (x) {
      setStage({ at: 'choose' })
      if (!(x instanceof DOMException && x.name === 'AbortError')) setErr(x instanceof Error ? x.message : String(x))
    } finally { stop.current = null }
  }
  const discard = async () => {
    const t = token.current
    token.current = ''
    setStage({ at: 'choose' })
    if (t) { try { await api.del(`/module-uploads/${encodeURIComponent(t)}`); toast('The uploaded file was discarded') } catch { /* it expires by itself */ } }
  }
  const install = async (body: OverrideBody | { password: string; code: string }) => {
    try {
      await api.post(`/module-uploads/${encodeURIComponent(token.current)}/install`, body)
    } catch (x) {
      // An upload that's gone (expired, or the server restarted) can't be installed: say so plainly.
      if (x instanceof ApiError && (x.status === 404 || x.status === 410)) {
        token.current = ''
        setInstalling(null); setStage({ at: 'choose' })
        setErr('This upload has expired (each one is kept for 30 minutes). Choose the file again.')
        return
      }
      throw x
    }
    installed.current = true
    const name = stage.at === 'found' ? stage.r.name || stage.r.id || 'The module' : 'The module'
    toast(`${name} is being installed`)
    onInstalled()
    onClose()
  }

  const v = stage.at === 'found' ? uploadView(stage.r) : null
  const r = stage.at === 'found' ? stage.r : null
  const expires = toDate(r?.expiresAt)
  const pct = stage.at === 'uploading' && stage.total ? Math.round((stage.sent / stage.total) * 100) : 0

  // The go-ahead opens over this dialog, which stays open beneath it: replacing it would close
  // it, and with it the Back entry and the upload this dialog holds.
  const next = installing === 'override' && v?.need && r
    ? <OverrideDialog id={r.id || ''} name={r.name || r.id || 'This module'} need={v.need} onSubmit={install} onClose={() => setInstalling(null)} />
    : installing === 'password' && r
      ? <ConfirmOwner onClose={() => setInstalling(null)} ask={{
        title: `Install ${r.name || r.id}?`, action: 'Install',
        body: <p>This changes which code runs on this server, so it asks for your password.</p>,
        run: (password, code) => install({ password, code }),
      }} />
      : null

  return (<>
    <Dialog title="Install a module from a file" size="md" onClose={onClose}
      actions={stage.at === 'found' ? <>
        <Button variant="ghost" onClick={discard}>Discard</Button>
        {v!.state !== 'refused' && <Button variant={v!.state === 'override' ? 'danger' : 'primary'} onClick={() => setInstalling(v!.state === 'override' ? 'override' : 'password')}>Install…</Button>}
      </> : <>
        <Button variant="ghost" onClick={() => { stop.current?.abort(); onClose() }}>{stage.at === 'uploading' ? 'Stop' : 'Cancel'}</Button>
        <Button variant="primary" type="submit" form={uid + 'f'} disabled={!p.archive} busy={stage.at === 'uploading'}>Upload and check</Button>
      </>}>
      {stage.at !== 'found' && (
        <form id={uid + 'f'} className="up-form" onSubmit={send}>
          <p className="ms-note">The server reads the file and says what it is before anything is installed. A module is someone’s code running on your server, so only install one from someone you trust.</p>
          <label className="up-file">The module’s release archive
            <input type="file" accept=".zip,.tar.gz,.tgz,application/zip,application/gzip" disabled={stage.at === 'uploading'}
              onChange={(e) => { setP((x) => ({ ...x, archive: e.target.files?.[0] || null })); setErr('') }} aria-describedby={uid + 'ah'} />
            <span className="up-hint" id={uid + 'ah'}>As the module’s publisher released it, at most 512 MB.</span>
          </label>
          <fieldset className="up-optional" disabled={stage.at === 'uploading'}>
            <legend>If you have them</legend>
            <label className="up-file">Checksum list
              <input type="file" onChange={(e) => setP((x) => ({ ...x, sums: e.target.files?.[0] || null }))} aria-describedby={uid + 'sh'} />
              <span className="up-hint" id={uid + 'sh'}>Usually SHA256SUMS, from the same release.</span>
            </label>
            <label className="up-file">Signature
              <input type="file" onChange={(e) => setP((x) => ({ ...x, sig: e.target.files?.[0] || null }))} aria-describedby={uid + 'gh'} />
              <span className="up-hint" id={uid + 'gh'}>The .sig file that signs the checksum list.</span>
            </label>
            <label className="set-field">The publisher’s public key
              <input className="input mono" value={p.key} onChange={(e) => setP((x) => ({ ...x, key: e.target.value }))} placeholder="44 characters, from wittymodule keygen" spellCheck={false} autoComplete="off" autoCapitalize="off" />
            </label>
          </fieldset>
          {stage.at === 'uploading' && (
            <div className="up-progress">
              <span role="status" aria-live="polite">{pct < 100 ? `Uploading ${p.archive?.name}: ${pct}%` : 'Uploaded. The server is checking it…'}</span>
              <Progress label={`Uploading ${p.archive?.name}`} value={pct < 100 ? stage.sent : undefined} max={stage.total} />
            </div>
          )}
          {err && <p className="error" role="alert">{err}</p>}
        </form>
      )}

      {v && r && (
        <div className="up-found">
          <h3 ref={found} tabIndex={-1} className="up-title">{v.title}</h3>
          {r.refusal && <p className="error" role="alert">{r.refusal}</p>}
          <dl className="ms-facts">
            {v.facts.map((f) => <div key={f.label}><dt>{f.label}</dt><dd>{f.value}</dd></div>)}
          </dl>
          <p className={'up-sig ' + v.signature.tone}>{v.signature.tone === 'ok' ? <IconCheck size={16} /> : <IconShield size={16} />}<span>{v.signature.words}</span></p>
          {r.fingerprint && <p className="st-print up-print">Key fingerprint <code>{r.fingerprint}</code></p>}
          {r.statusNote && !v.problems.some((x) => x.text.includes(r.statusNote!)) && <p className="warn-box">{r.statusNote}</p>}
          <div>
            <b>What it asks for</b>
            {v.permissions.length ? <ul className="up-problems">{v.permissions.map((x) => <li key={x}>{x}</li>)}</ul> : <p className="ms-note">No special permissions.</p>}
          </div>
          {v.problems.length > 0 && (
            <div>
              <b><IconInfo size={14} /> What’s wrong with it</b>
              <ul className="up-problems">{v.problems.map((x, i) => <li key={i}>{x.text}</li>)}</ul>
            </div>
          )}
          {v.state === 'override' && <p className="ms-note">Installing it needs an override: the next step names each rule it bypasses.</p>}
          {expires && v.state !== 'refused' && <p className="ms-note">Kept until {fmtDateTime(expires.getTime())}, then discarded if it isn’t installed.</p>}
        </div>
      )}
    </Dialog>
    {next}
  </>)
}
