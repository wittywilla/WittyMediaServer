import { useEffect, useRef, useState } from 'react'
import { PageHelp } from '../../../components/HelpLink'
import { ApiError, fmtDateTime } from '../../../api'
import { BackLink } from '../../../components/common'
import { ConfirmOwner } from '../ConfirmOwner'
import { IconPlus, IconShield, IconTrash } from '../../../icons'
import { toast } from '../../../state'
import { ask, Button, Field, Input, Notice, Spinner, Switch } from '../../../ui'
import { PageHead, Section } from '../ui'
import { FailureBody, FailureNotice, useStoreView } from './parts'
import { addStore, checkStore, failureOf, previewStore, removeStore, setStoreOn, type Failure, type StoreInfo, type StorePreview } from './storeApi'

// Stores other than this project's. Adding one is two steps on purpose: the server fetches and
// checks the store and comes back with its key and what it offers, and only then, having read the
// warning, can someone agree. Once added, its key is fixed, and a change is a loud refusal.

export default function OtherStores() {
  const { view, error, reload } = useStoreView()
  const head = (
    <>
      <BackLink to="/settings/modules/store" label="Module store" />
      <PageHead title="Other stores" desc="Add a store run by someone else, and see the modules it offers alongside this project’s. Everything from one is labelled, and whether to trust it is up to you." actions={<PageHelp to="module-store#other-stores">What to know first</PageHelp>} />
    </>
  )
  if (error && !view) return <div className="ms-page st-page">{head}<Notice tone="danger" title={error.title} action={<Button size="sm" onClick={() => reload()}>Try again</Button>}>{error.message}</Notice></div>
  if (!view) return <div className="ms-page st-page">{head}<Spinner /></div>
  const others = view.stores.filter((s) => !s.official)
  return (
    <div className="ms-page st-page">
      {head}
      <AddStore onAdded={reload} repairRoute={view.repairRoute} />
      <Section title="Stores on this server" desc={others.length ? 'Each one’s key was pinned when it was added.' : undefined}>
        {others.length === 0
          ? <p className="ms-note">You haven’t added any. This server only uses this project’s store.</p>
          : <ul className="st-stores">{others.map((s) => <StoreRow key={s.id} s={s} onChanged={reload} repairRoute={view.repairRoute} />)}</ul>}
      </Section>
      <Section title="Custom modules" actions={<span className="set-pill">In development</span>} desc="A store can already offer a different version of a module this server knows, which you choose module by module. Wholly new kinds of module are shown in the store, marked in development, and can’t be installed yet." >
        <p className="ms-note">WittyTuner can’t be replaced by anyone’s version, because it’s what refuses encrypted channels, and the server couldn’t tell whether another version still did.</p>
      </Section>
    </div>
  )
}

function StoreRow({ s, onChanged, repairRoute }: { s: StoreInfo; onChanged: () => void; repairRoute: string }) {
  const [working, setWorking] = useState(false)
  const [failure, setFailure] = useState<Failure | null>(null)
  const run = async (what: () => Promise<unknown>, done?: string) => {
    setWorking(true); setFailure(null)
    try { await what(); if (done) toast(done); onChanged() } catch (e) { setFailure(failureOf(e, s.name)) } finally { setWorking(false) }
  }
  const remove = async () => {
    const using = s.inUse?.length ? ` ${s.inUse.join(', ')} ${s.inUse.length === 1 ? 'goes' : 'go'} back to the official version, and what came from this store is deleted.` : ''
    if (!await ask({ title: `Remove ${s.name}?`, body: `This server forgets the store and its key.${using}`, confirm: 'Remove store', danger: true })) return
    await run(() => removeStore(s.id), `${s.name} removed`)
  }
  const keyChanged = s.problem?.kind === 'key-changed'
  return (
    <li className={'st-store' + (s.enabled ? '' : ' off') + (keyChanged ? ' refused' : '')} aria-label={`${s.name}, ${s.host}`}>
      <div className="st-store-head">
        <span className="st-store-name"><IconShield /><b>{s.name}</b><span className="muted">{s.host}</span>{!s.enabled && <span className="ms-tag off">Off</span>}</span>
        <label className="st-store-switch"><span>{s.enabled ? 'On' : 'Off'}</span><Switch label={`Use ${s.name}`} checked={s.enabled} disabled={working} onChange={(on) => run(() => setStoreOn(s.id, on), on ? `${s.name} turned on` : `${s.name} turned off. Anything installed from it stays until you remove it.`)} /></label>
      </div>
      {s.problem && <FailureNotice failure={s.problem} name={s.name} repairRoute={repairRoute} />}
      <dl className="ms-facts">
        <div className="wide"><dt>Address</dt><dd className="mono">{s.url}</dd></div>
        <div className="wide"><dt>Pinned key fingerprint</dt><dd className="mono">{s.fingerprint}</dd></div>
        <div><dt>Modules</dt><dd>{s.modules}</dd></div>
        <div><dt>Added</dt><dd>{s.addedAt ? fmtDateTime(s.addedAt) : ''}{s.addedBy ? ` by ${s.addedBy}` : ''}</dd></div>
        <div><dt>Last checked</dt><dd>{s.checkedAt ? fmtDateTime(s.checkedAt) : 'Not yet'}</dd></div>
        {s.private && <div className="wide"><dt>Network</dt><dd>Allowed on your own network</dd></div>}
        {!!s.inUse?.length && <div className="wide"><dt>In use by</dt><dd>{s.inUse.join(', ')}</dd></div>}
      </dl>
      {failure && <p className="error" role="alert">{failure.title}: {failure.message}</p>}
      <div className="ms-actions">
        <Button size="sm" disabled={working || !s.enabled} onClick={() => run(() => checkStore(s.id), `${s.name} checked`)}>Check now</Button>
        <Button size="sm" variant="ghost" className="danger-hover" icon={<IconTrash />} disabled={working} onClick={remove}>Remove store</Button>
      </div>
    </li>
  )
}

function AddStore({ onAdded, repairRoute }: { onAdded: () => void; repairRoute: string }) {
  const [url, setUrl] = useState('')
  const [key, setKey] = useState('')
  const [allowPrivate, setAllowPrivate] = useState(false)
  const [preview, setPreview] = useState<StorePreview | null>(null)
  const [failure, setFailure] = useState<Failure | null>(null)
  const [needKey, setNeedKey] = useState(false)
  const [trust, setTrust] = useState(false)
  const [working, setWorking] = useState(false)
  const [owner, setOwner] = useState(false)
  const [lookOwner, setLookOwner] = useState(false)
  const warned = useRef<HTMLDivElement>(null)
  const urlField = useRef<HTMLInputElement>(null)
  useEffect(() => { if (preview) warned.current?.focus() }, [preview])
  const reset = () => { setPreview(null); setTrust(false); setFailure(null) }
  const fetchPreview = async (secrets?: { password: string; code: string }) => {
    setWorking(true); reset(); setNeedKey(false)
    try { setPreview(await previewStore(url.trim(), key.trim(), allowPrivate, secrets)) } catch (err) {
      if (secrets && err instanceof ApiError && err.status === 403) throw err // the dialog says what was wrong with the password
      if (err instanceof ApiError && err.data?.code === 'need-key') setNeedKey(true)
      setFailure(failureOf(err, 'That store'))
    } finally { setWorking(false) }
  }
  const look = (e: React.FormEvent) => {
    e.preventDefault()
    if (working) return
    if (allowPrivate) setLookOwner(true)
    else void fetchPreview()
  }
  const agree = async (password: string, code: string) => {
    if (!preview) return
    await addStore(preview.url, preview.key, allowPrivate, true, { password, code })
    toast(`${preview.name} added`)
    setUrl(''); setKey(''); setAllowPrivate(false); reset(); onAdded()
    setTimeout(() => urlField.current?.focus(), 0)
  }
  const cancel = () => { reset(); setTimeout(() => urlField.current?.focus(), 0) }
  return (
    <Section title="Add a store" desc="The address of its catalog.json, from whoever runs the store.">
      <form className="st-add" onSubmit={look}>
        <Field label="Address" required hint="It starts with https://, and normally ends in catalog.json.">
          <Input ref={urlField} value={url} onChange={(e) => { setUrl(e.target.value); reset() }} placeholder="https://example.com/modules/catalog.json" spellCheck={false} autoComplete="off" autoCapitalize="off" inputMode="url" />
        </Field>
        <Field label="The store’s public key" hint={needKey ? 'This store doesn’t offer one, so ask its owner.' : 'Optional, and better: if its owner gave you a key, paste it, so the first answer doesn’t have to be taken on trust. Otherwise the store’s own suggestion is shown for you to check.'}>
          <Input className="mono" value={key} onChange={(e) => { setKey(e.target.value); reset() }} placeholder="44 characters" spellCheck={false} autoComplete="off" autoCapitalize="off" />
        </Field>
        <label className="ms-trust st-allow"><input type="checkbox" checked={allowPrivate} onChange={(e) => { setAllowPrivate(e.target.checked); reset() }} /> Allow an address on my own network</label>
        <p className="ms-note st-allow-note">Only for a store you run yourself. It lets this server fetch from your own network for this one store, and over plain http there.</p>
        {failure && <div className="st-add-fail" role="alert"><strong>{failure.title}</strong><p>{failure.message}</p><FailureBody failure={failure} name="That store" repairRoute={repairRoute} /></div>}
        {!preview && <div><Button type="submit" icon={<IconPlus />} aria-busy={working || undefined} disabled={!url.trim()}>Look at this store</Button></div>}
      </form>

      {preview && (
        <div className="warn-box st-warning" role="alert" tabIndex={-1} ref={warned}>
          <h3><IconShield size={20} /> Before you add {preview.name}</h3>
          <p className="muted">From {preview.host}. {preview.count} {preview.count === 1 ? 'module' : 'modules'}.</p>
          {preview.warning.map((w, i) => <p key={i}>{w}</p>)}
          <div className="st-print big">
            <span>{preview.keySource === 'offered' ? 'The key this store offers. Nothing has checked it for you yet.' : 'The key you pasted, which this store’s signature matches.'}</span>
            <code>{preview.fingerprint}</code>
          </div>
          {preview.modules.length > 0 && (
            <details className="ms-more" open={preview.modules.length <= 6}>
              <summary>What it offers</summary>
              <ul className="st-preview-mods">
                {preview.modules.map((m) => (
                  <li key={m.id}>
                    <b>{m.name}</b>
                    <span className="ms-tag warn">{m.kind === 'fork' ? 'A different version of one you know' : m.kind === 'refused' ? 'Can’t be used' : 'In development'}</span>
                    {m.summary && <span className="muted">{m.summary}</span>}
                  </li>
                ))}
              </ul>
            </details>
          )}
          <label className="ms-trust"><input type="checkbox" checked={trust} onChange={(e) => setTrust(e.target.checked)} /> I understand, and I trust whoever runs this store</label>
          <div className="ms-actions">
            <Button variant="danger" disabled={!trust} onClick={() => setOwner(true)}>Add this store</Button>
            <Button variant="ghost" onClick={cancel}>Cancel</Button>
          </div>
        </div>
      )}
      {lookOwner && <ConfirmOwner onClose={() => setLookOwner(false)} ask={{
        title: 'Look on your own network?', action: 'Look at this store',
        body: <p>This server will fetch from an address on your own network, so this asks for your password.</p>,
        run: (password, code) => fetchPreview({ password, code }),
      }} />}
      {owner && preview && <ConfirmOwner onClose={() => setOwner(false)} ask={{
        title: `Add ${preview.name}?`, action: 'Add this store',
        body: <p>From now on this store can offer code for this server to run, so this asks for your password.</p>,
        run: agree,
      }} />}
    </Section>
  )
}
