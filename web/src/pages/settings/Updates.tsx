import { useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api, downloadFile, fmtDateTime } from '../../api'
import { forgetUpdateStatus, loadUpdateStatus, type Installer, type UpdateInfo, type UpdateStatus } from '../../components/UpdateBanner'
import { WebLink } from '../../components/WebLink'
import { GuideBody } from '../../help/GuideBody'
import { parse, type Block } from '../../help/markdown'
import '../../help/help.css'
import { IconCopy, IconDownload, IconRefresh } from '../../icons'
import { toast, useApp } from '../../state'
import { ask } from '../../ui/Overlay'
import { Row, Section, Switch } from './ui'

// Updates: what is installed, what GitHub has, what changed since, and the one way this copy
// updates. A copy that can replace its own files does it with one click: the server verifies the
// release's signature and checksum, swaps the program's files (never the data) and restarts. A copy
// a package or Windows installer owns gets that installer, checked the same way, for the admin to
// run. A container gets the commands that pull the new image.

const ACTIVE = ['downloading', 'verifying', 'installing', 'restarting']
const mb = (n = 0) => (n / 1048576).toFixed(n < 10485760 ? 1 : 0)

export function Updates() {
  const { setUpdate } = useApp()
  const loc = useLocation()
  const nav = useNavigate()
  const [st, setSt] = useState<UpdateStatus | null>(null)
  const [auto, setAuto] = useState(true)
  const [checking, setChecking] = useState(false)
  const [slow, setSlow] = useState(false) // the server has not come back yet
  const from = useRef('')
  const started = useRef(false)
  const show = (n: UpdateStatus) => { setSt(n); setUpdate(n.latest?.version || '') }
  useEffect(() => {
    loadUpdateStatus(true).then(show).catch((e) => toast(e.message))
    api.get<Record<string, string>>('/settings').then((s) => setAuto(s.updateCheck !== '0')).catch(() => {})
  }, [])

  // While installing: follow the progress, then wait for the new version.
  const active = !!st && ACTIVE.includes(st.phase)
  useEffect(() => {
    if (!active || !st) return
    if (!from.current) from.current = st.current
    const begun = Date.now()
    const t = setInterval(async () => {
      try {
        const n = await api.get<UpdateStatus>('/update')
        if (n.current !== from.current) { window.location.reload(); return } // the new version is running
        setSt(n)
      } catch {
        setSt((s) => s && { ...s, phase: 'restarting' }) // unreachable while it restarts
      }
      setSlow(Date.now() - begun > 3 * 60_000)
    }, 1000)
    return () => clearInterval(t)
  }, [active])

  // While the installer downloads: follow it.
  const fetching = st?.installer?.phase === 'downloading' || st?.installer?.phase === 'verifying'
  useEffect(() => {
    if (!fetching) return
    const t = setInterval(() => { api.get<UpdateStatus>('/update').then(setSt).catch(() => {}) }, 1000)
    return () => clearInterval(t)
  }, [fetching])

  const check = async () => {
    setChecking(true)
    try {
      const n = await api.post<UpdateStatus>('/update/check')
      show(n)
      forgetUpdateStatus()
      if (n.checkError) toast(n.checkError)
      else if (!n.latest) toast('You have the newest version')
    } catch (e: any) { toast(e.message) } finally { setChecking(false) }
  }
  const apply = async () => {
    if (!st?.latest || !await ask({ title: `Install WittyMediaServer ${st.latest.version}?`, body: 'The server restarts, which pauses anything playing for a moment. Your library and settings are kept.', confirm: 'Install and restart' })) return
    try { from.current = st.current; setSt(await api.post<UpdateStatus>('/update/apply')) } catch (e: any) { toast(e.message) }
  }
  const fetchInstaller = async () => {
    try { setSt(await api.post<UpdateStatus>('/update/installer')) } catch (e: any) { toast(e.message) }
  }
  const setAutoCheck = async (v: boolean) => {
    setAuto(v)
    try { await api.put('/settings', { updateCheck: v ? '1' : '0' }) } catch (e: any) { setAuto(!v); toast(e.message) }
  }

  // Update from the banner: start the one way this copy updates, once it's known.
  useEffect(() => {
    if (!st?.latest || started.current || !(loc.state as { startUpdate?: boolean } | null)?.startUpdate) return
    started.current = true
    nav(loc.pathname + loc.hash, { replace: true, state: null })
    if (st.method === 'apply' && !active) apply()
    else if (st.method === 'installer' && !st.installer?.phase) fetchInstaller()
  }, [st, loc.state])

  const l = st?.latest
  // The download this copy needs: the package it replaces itself from, or its installer.
  const size = st?.method === 'installer' ? st.installer?.size : st?.method === 'apply' ? l?.size : 0
  return (
    <Section id="updates" title="Updates" desc="New versions of WittyMediaServer are published on GitHub. Each one is signed, and this server installs only releases whose signature and checksum it can verify."
      actions={<button className="btn ghost sm" onClick={check} disabled={checking || active}><IconRefresh /> {checking ? 'Checking…' : 'Check now'}</button>}>
      <Row label="Installed version" desc={st?.checkedAt ? `Last checked ${fmtDateTime(st.checkedAt)}` : 'Not checked yet'}>
        <span className="mod-ver">{st?.current || '…'}</span>
      </Row>
      {st?.withdrawn && <p className="upd-msg warn" role="alert">This version was withdrawn by the project: {st.withdrawn}. Update as soon as a newer one is offered.</p>}
      {st?.checkError && <p className="upd-msg warn">The last check didn’t work: {st.checkError}</p>}
      {st && !l && !st.checkError && !!st.checkedAt && <p className="upd-msg">You have the newest version.</p>}
      {st && l && (
        <div className="upd-new">
          <div className="upd-head">
            <div>
              <b>{l.title || `Version ${l.version}`}</b>
              <span className="muted">Version {l.version}{l.prerelease && ' · pre-release'}{l.published ? ` · ${new Date(l.published).toLocaleDateString()}` : ''}{size ? ` · ${mb(size)} MB` : ''}</span>
            </div>
            {st.method === 'apply' && !active && <button className="btn primary sm" onClick={apply}>Update now</button>}
            {st.method === 'installer' && !st.installer?.phase && <button className="btn primary sm" onClick={fetchInstaller}><IconDownload /> Download the installer</button>}
          </div>
          {st.method !== 'apply' && st.reason && <p className="upd-msg">{st.reason}</p>}
          {st.method === 'installer' && st.installer && <InstallerSteps st={st} installer={st.installer} retry={fetchInstaller} />}
          {st.method === 'docker' && <DockerSteps image={st.image || 'ghcr.io/wittywilla/wittymediaserver:alpha'} />}
          <WhatsNew info={l} />
          <WebLink href={l.url || st.page}>Release page on GitHub ›</WebLink>
        </div>
      )}
      {st && st.phase !== 'idle' && (
        <div className={'upd-progress' + (st.phase === 'failed' ? ' failed' : '')} role="status" aria-live="polite">
          {st.phase === 'downloading' && <>
            <span>Downloading… {mb(st.done)}{st.total ? ` of ${mb(st.total)}` : ''} MB</span>
            <progress max={st.total || undefined} value={st.total ? st.done || 0 : undefined} />
          </>}
          {st.phase === 'verifying' && <span>Checking the signature and checksum…</span>}
          {st.phase === 'installing' && <span>Installing the new files…</span>}
          {st.phase === 'restarting' && <span>{slow
            ? 'The server hasn’t come back yet. If it doesn’t, start WittyMediaServer again (tray icon or Start menu); the new version is installed.'
            : 'Restarting… this page reloads by itself when the new version is running.'}</span>}
          {st.phase === 'failed' && <span>The update didn’t install, and the server keeps running the version it has: {st.error}</span>}
        </div>
      )}
      <Row label="Check for updates automatically" desc="Twice a day the server asks GitHub for its list of releases and shows a dot on Settings when there is a newer one. Nothing about you or your library is sent, and nothing installs without you.">
        <Switch checked={auto} onChange={setAutoCheck} label="Check for updates automatically" />
      </Row>
    </Section>
  )
}

// The installer, from asking for it to running it.
function InstallerSteps({ st, installer: d, retry }: { st: UpdateStatus; installer: Installer; retry: () => void }) {
  const [saving, setSaving] = useState(false)
  const windows = d.name.endsWith('.msi'), mac = d.name.includes('-macos-')
  const save = async () => {
    setSaving(true)
    try { await downloadFile('/update/installer', d.name) } catch (e: any) { toast(e.message) } finally { setSaving(false) }
  }
  if (d.phase === 'downloading' || d.phase === 'verifying') {
    return (
      <div className="upd-progress" role="status" aria-live="polite">
        <span>{d.phase === 'verifying' ? 'Checking it against the release’s signed checksums…' : `Downloading ${d.name}… ${mb(d.done)}${d.size ? ` of ${mb(d.size)}` : ''} MB`}</span>
        <progress max={d.size || undefined} value={d.size ? d.done || 0 : undefined} />
      </div>
    )
  }
  if (d.phase === 'failed') {
    return (
      <div className="upd-progress failed" role="alert">
        <span>The installer didn’t download, and nothing was kept: {d.error}</span>
        <div><button className="btn sm" onClick={retry}>Try again</button></div>
      </div>
    )
  }
  if (d.phase !== 'ready') return <p className="upd-msg">The installer for this computer is <code>{d.name}</code>{d.size ? ` (${mb(d.size)} MB)` : ''}. It’s checked against the release’s signed checksums before it’s kept.</p>
  return (
    <div className="upd-steps">
      <p className="upd-ok" role="status"><b>Downloaded and checked.</b> <code>{d.name}</code> matches the release’s signed checksum.</p>
      <ol>
        {d.command
          ? <li>On the server’s computer, as its administrator, run:<Command text={d.command} /></li>
          : <li>{windows ? 'On the server’s computer, open the installer. ' : mac ? 'On the Mac, open the installer. ' : 'On the server’s computer, unpack it over the old copy. '}It’s saved there as:<Command text={d.path || d.name} label="Where it is" /></li>}
        <li>Or save it on this device: <button className="btn sm" onClick={save} disabled={saving}><IconDownload /> {saving ? 'Saving…' : 'Save the installer'}</button></li>
        <li>{windows ? 'The installer updates in place: it stops the server, puts the new version in, and starts it again. Your libraries, settings and watch history are kept.' : 'Your libraries, settings and watch history are kept.'} This page shows the new version once it’s running{st.current ? `, instead of ${st.current}` : ''}.</li>
      </ol>
      {windows && <p className="upd-msg">The installer isn’t code-signed yet, so Windows may say it doesn’t recognise it: choose <b>More info</b>, then <b>Run anyway</b>. On a PC with Smart App Control on, Windows blocks installers that aren’t signed; signed releases are on the way.</p>}
    </div>
  )
}

// A container's way: pull the new image, then recreate the container. The words match the install
// guide's.
function DockerSteps({ image }: { image: string }) {
  return (
    <div className="upd-steps">
      <p>With Docker Compose, in the folder with your <code>docker-compose.yml</code>:</p>
      <Command text={'docker compose pull\ndocker compose up -d'} />
      <p>With <code>docker run</code>: pull the image, then remove and recreate the container with the same command you started it with.</p>
      <Command text={`docker pull ${image}\ndocker rm -f wittymediaserver`} />
      <p className="upd-msg">Your <code>/config</code> volume stays, with your libraries, settings and watch history.</p>
    </div>
  )
}

function Command({ text, label = 'Command' }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try { await navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 1800) } catch { toast('Select the text and copy it') }
  }
  return (
    <div className="help-code upd-command">
      <pre tabIndex={0} role="group" aria-label={label}><code>{text}</code></pre>
      {navigator.clipboard && <button type="button" className="btn sm ghost help-copy" onClick={copy} aria-label={copied ? 'Copied' : 'Copy this'}><IconCopy size={16} /><span>{copied ? 'Copied' : 'Copy'}</span></button>}
    </div>
  )
}

// What changed in every version since this one, newest first, drawn by the help pages' Markdown
// reader: nothing from the notes can act as HTML, and links open outside the app.
function WhatsNew({ info }: { info: UpdateInfo }) {
  const { user } = useApp()
  const changes = useMemo(() => (info.changes?.length ? info.changes : info.notes ? [{ version: info.version, notes: info.notes }] : [])
    .map((c) => ({ ...c, blocks: parse(c.notes) as Block[] })), [info])
  if (!changes.length) return null
  return (
    <div className="upd-whatsnew">
      <h3 className="upd-whatsnew-title">{changes.length > 1 ? `What’s new in the ${changes.length} versions since yours` : 'What’s new'}</h3>
      {changes.map((c, i) => (
        <details key={c.version} open={i === 0} className="upd-version">
          <summary>Version {c.version}</summary>
          <div className="help-prose upd-notes"><GuideBody blocks={c.blocks} signedIn={!!user} idPrefix={`v${c.version.replace(/\W/g, '-')}-`} shift={1} /></div>
        </details>
      ))}
    </div>
  )
}
