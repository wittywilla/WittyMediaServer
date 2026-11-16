// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { PageHelp } from '../../../components/HelpLink'
import { ApiError } from '../../../api'
import { date, t } from '../../../i18n'
import { BackLink } from '../../../components/common'
import { ConfirmOwner } from '../ConfirmOwner'
import { toast } from '../../../state'
import { ask, Button, Notice, Spinner } from '../../../ui'
import { PageHead, Section } from '../ui'
import SpecPage from '../../../spec/SpecPage'
import { handledIds } from '../../../spec/model'
import { addStoreSpec, otherStoresSpec, type StoreLine } from '../../../spec/specs/store'
import { FailureBody, FailureNotice, useStoreView } from './parts'
import { addStore, checkStore, failureOf, previewStore, removeStore, setStoreOn, type Failure, type StoreInfo, type StorePreview } from './storeApi'

// Stores other than this project's. Adding one is two steps on purpose: the server fetches and
// checks the store and comes back with its key and what it offers, and only then, having read the
// warning, can someone agree. Once added, its key is fixed, and a change is a loud refusal.

export default function OtherStores() {
  const { view, error, reload } = useStoreView()
  const head = (
    <>
      <BackLink to="/settings/modules/store" label={t('Module store')} />
      <PageHead title={t('Other stores')} desc={t('Add a store run by someone else, and see the modules it offers alongside this project’s. Everything from one is labelled, and whether to trust it is up to you.')} actions={<PageHelp to="module-store#other-stores">{t('What to know first')}</PageHelp>} />
    </>
  )
  if (error && !view) return <div className="ms-page st-page">{head}<Notice tone="danger" title={error.title} action={<Button size="sm" onClick={() => reload()}>{t('Try again')}</Button>}>{error.message}</Notice></div>
  if (!view) return <div className="ms-page st-page">{head}<Spinner /></div>
  const others = view.stores.filter((s) => !s.official)
  return (
    <div className="ms-page st-page">
      {head}
      <AddStore onAdded={reload} repairRoute={view.repairRoute} />
      <Stores stores={others} onChanged={reload} repairRoute={view.repairRoute} />
      <Section title={t('Custom modules')} actions={<span className="set-pill">{t('In development')}</span>} desc={t('A store can already offer a different version of a module this server knows, which you choose module by module. Wholly new kinds of module are shown in the store, marked in development, and can’t be installed yet.')} >
        <p className="ms-note">{t('WittyTuner can’t be replaced by anyone’s version, because it’s what refuses encrypted channels, and the server couldn’t tell whether another version still did.')}</p>
      </Section>
    </div>
  )
}

const when = (at: number) => date(at, { dateStyle: 'medium', timeStyle: 'short' })

// Each store: its switch, facts and buttons from the spec; its own problem notice and what an action said stay here.
function Stores({ stores, onChanged, repairRoute }: { stores: StoreInfo[]; onChanged: () => void; repairRoute: string }) {
  const [working, setWorking] = useState<string | number>('')
  const [failures, setFailures] = useState<Record<string, Failure>>({})
  const lines: StoreLine[] = stores.map((s) => ({
    id: s.id, name: s.name, host: s.host, enabled: s.enabled, url: s.url, fingerprint: s.fingerprint || '', modules: s.modules,
    added: [s.addedAt ? when(s.addedAt) : '', s.addedBy ? t('by {user}', { user: s.addedBy }) : ''].filter(Boolean).join(' '), checked: s.checkedAt ? when(s.checkedAt) : '',
    private: !!s.private, inUse: s.inUse || [], working: working === s.id,
  }))
  const page = otherStoresSpec(lines)
  const run = async (s: StoreInfo, what: () => Promise<unknown>, done?: string) => {
    setWorking(s.id); setFailures((f) => { const n = { ...f }; delete n[String(s.id)]; return n })
    try { await what(); if (done) toast(done); onChanged() } catch (e) { setFailures((f) => ({ ...f, [String(s.id)]: failureOf(e, s.name) })) } finally { setWorking('') }
  }
  const remove = async (s: StoreInfo) => {
    const using = !s.inUse?.length ? '' : s.inUse.length === 1
      ? t('{names} goes back to the official version, and what came from this store is deleted.', { names: s.inUse[0] })
      : t('{names} go back to the official version, and what came from this store is deleted.', { names: s.inUse.join(', ') })
    if (!await ask({ title: t('Remove {name}?', { name: s.name }), body: [t('This server forgets the store and its key.'), using].filter(Boolean).join(' '), confirm: t('Remove store'), danger: true })) return
    await run(s, () => removeStore(s.id), t('{name} removed', { name: s.name }))
  }
  const act = (id: string) => {
    const m = /^store(.+):(check|remove)$/.exec(id)
    const s = m && stores.find((x) => String(x.id) === m[1])
    if (!m || !s) return
    if (m[2] === 'check') void run(s, () => checkStore(s.id), t('{name} checked', { name: s.name })); else void remove(s)
  }
  const field = (id: string, v: string | number | boolean | File) => {
    const m = /^store(.+):on$/.exec(id)
    const s = m && stores.find((x) => String(x.id) === m[1])
    if (s) { const on = !!v; void run(s, () => setStoreOn(s.id, on), on ? t('{name} turned on', { name: s.name }) : t('{name} turned off. Anything installed from it stays until you remove it.', { name: s.name })) }
  }
  return (
    <>
      {stores.filter((s) => s.problem).map((s) => <FailureNotice key={'p' + s.id} failure={s.problem!} name={s.name} repairRoute={repairRoute} />)}
      {Object.entries(failures).map(([id, f]) => <p key={'f' + id} className="error" role="alert">{f.title}: {f.message}</p>)}
      <SpecPage embedded page={page} handled={handledIds(page)} onAction={act} onField={field} />
    </>
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
      setFailure(failureOf(err, t('That store')))
    } finally { setWorking(false) }
  }
  const agree = async (password: string, code: string) => {
    if (!preview) return
    await addStore(preview.url, preview.key, allowPrivate, true, { password, code })
    toast(t('{name} added', { name: preview.name }))
    setUrl(''); setKey(''); setAllowPrivate(false); reset(); onAdded()
    setTimeout(() => urlField.current?.focus(), 0)
  }
  const cancel = () => { reset(); setTimeout(() => urlField.current?.focus(), 0) }
  const page = addStoreSpec({
    url, key, allowPrivate, needKey, working, trust, failure: failure ? { title: failure.title, message: failure.message } : undefined,
    preview: preview ? { name: preview.name, host: preview.host, count: preview.count, warning: preview.warning, fingerprint: preview.fingerprint, keySource: preview.keySource,
      modules: preview.modules.map((m) => ({ id: m.id, name: m.name, tag: m.kind === 'fork' ? t('A different version of one you know') : m.kind === 'refused' ? t('Can’t be used') : t('In development'), summary: m.summary })) } : undefined,
  })
  const act = (id: string) => { if (id === 'look') { if (!working) { if (allowPrivate) setLookOwner(true); else void fetchPreview() } } else if (id === 'add') setOwner(true); else if (id === 'cancel') cancel() }
  const field = (id: string, v: string | number | boolean | File) => {
    if (id === 'url') { setUrl(String(v)); reset() } else if (id === 'key') { setKey(String(v)); reset() } else if (id === 'private') { setAllowPrivate(!!v); reset() } else if (id === 'trust') setTrust(!!v)
  }
  return (
    <div ref={warned} tabIndex={-1}>
      {failure && <FailureBody failure={failure} name={t('That store')} repairRoute={repairRoute} />}
      <SpecPage embedded page={page} handled={handledIds(page)} onAction={act} onField={field} />
      {lookOwner && <ConfirmOwner onClose={() => setLookOwner(false)} ask={{
        title: t('Look on your own network?'), action: t('Look at this store'),
        body: <p>{t('This server will fetch from an address on your own network, so this asks for your password.')}</p>,
        run: (password, code) => fetchPreview({ password, code }),
      }} />}
      {owner && preview && <ConfirmOwner onClose={() => setOwner(false)} ask={{
        title: t('Add {name}?', { name: preview.name }), action: t('Add this store'),
        body: <p>{t('From now on this store can offer code for this server to run, so this asks for your password.')}</p>,
        run: agree,
      }} />}
    </div>
  )
}