import { useEffect, useRef, useState } from 'react'
import { api } from '../../api'
import LegalNote from '../../components/LegalNote'
import { IconCheck } from '../../icons'
import type { ListingStatus } from '../../metadata'
import { Button, Field, Input, LoadError, Notice, Select, Spinner } from '../../ui'
import { LISTING_STATEMENTS, REGIONS } from '../settings/MetadataOptional'
import { PROVIDERS } from './Details'
import { LaterNote, StepActions, StepHead, TickRow, type StepProps } from './parts'

// The optional streaming lists. They stay off unless someone chooses them, reads every statement
// and has a TMDB key, which is what they're looked up with. Leaving them off is the big button.

const guessRegion = () => {
  const r = (() => { try { return new Intl.Locale(navigator.language).region } catch { return undefined } })()
  return r && REGIONS.includes(r) ? r : 'US'
}

export function Listings({ info, next, back }: StepProps) {
  const [s, setS] = useState<ListingStatus | null>(null)
  const [failed, setFailed] = useState(false)
  const [opened, setOpened] = useState(false)
  const [ticked, setTicked] = useState<Set<string>>(new Set())
  const [region, setRegion] = useState(guessRegion)
  const [key, setKey] = useState('')
  const [err, setErr] = useState('')
  const [keyErr, setKeyErr] = useState('')
  const [busy, setBusy] = useState(false)
  const panel = useRef<HTMLDivElement>(null)
  const load = () => { setFailed(false); return api.get<ListingStatus>('/listings/status').then((r) => { setS(r); if (r.enabled) setRegion(r.region) }).catch(() => setFailed(true)) }
  useEffect(() => { void load() }, [])
  useEffect(() => { if (opened) panel.current?.querySelector<HTMLElement>('button, input')?.focus() }, [opened])
  const names = new Intl.DisplayNames(undefined, { type: 'region' })
  const saveKey = async () => {
    if (!key.trim()) { setKeyErr('Paste your key first.'); return }
    setBusy(true); setKeyErr('')
    try {
      const t = await api.post<{ ok: boolean; error?: string }>('/metadata/test', { provider: 'tmdb', key: key.trim(), pin: '' })
      if (!t.ok) { setKeyErr(t.error || 'TMDB didn’t accept that key.'); return }
      await api.put('/settings', { tmdbKey: key.trim() })
      setKey(''); await load()
    } catch (e: any) { setKeyErr(e.message) } finally { setBusy(false) }
  }
  const turnOn = async () => {
    setErr('')
    if (ticked.size !== LISTING_STATEMENTS.length) { setErr('Tick all four statements first. Each one says something you need to know.'); return }
    setBusy(true)
    try { setS(await api.put<ListingStatus>('/listings/settings', { enabled: true, region, accepted: [...ticked] })); setOpened(false) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const turnOff = async () => {
    setBusy(true); setErr('')
    try { setS(await api.put<ListingStatus>('/listings/settings', { enabled: false, region, accepted: [] })); setTicked(new Set()) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const head = <StepHead title="Streaming service lists" lead="Used to finding things by which streaming service has them? This can show which of your own titles a service lists right now. Most people skip this, and it stays off unless you choose it." />
  if (failed) return <section>{head}<LoadError what="This question" onRetry={load} /><StepActions back={back}><Button variant="primary" size="big" onClick={next}>Skip for now</Button></StepActions></section>
  if (!s) return <section>{head}<Spinner /></section>
  return (
    <section>
      {head}
      <Notice title="This doesn’t stream anything from those services">
        It only sorts the files already on your server, using each service’s current list. You don’t need an account with any of them, and you won’t get their shows.
      </Notice>
      {s.enabled && <p className="oobe-added su-gap"><IconCheck size={16} /> On, for {names.of(s.region) || s.region}. Your titles are being checked in the background.</p>}
      {!s.enabled && opened && (
        <div className="su-consent su-gap" ref={panel}>
          {!s.hasKey && (
            <div className="su-keybox">
              <h2 className="oobe-type-head">First, a free key from TMDB</h2>
              <p className="muted oobe-hint">The lists come through TMDB, which gives you a free key: a code that lets your server ask it for things.</p>
              <ol className="op-steps">{PROVIDERS[0].steps.map((x, i) => <li key={i}>{x}</li>)}</ol>
              <Field label="Your TMDB key" error={keyErr}><Input value={key} spellCheck={false} autoComplete="off" onChange={(e) => setKey(e.target.value)} /></Field>
              <Button busy={busy} onClick={saveKey}>Check and save this key</Button>
            </div>
          )}
          {s.hasKey && <p className="oobe-added"><IconCheck size={16} /> Your TMDB key is saved.</p>}
          <h2 className="oobe-type-head su-gap">Please read each of these, and tick it if you accept</h2>
          <div className="su-ticks">
            {LISTING_STATEMENTS.map((a) => <TickRow key={a.id} checked={ticked.has(a.id)} title={a.text} onChange={(v) => setTicked((t) => { const n = new Set(t); if (v) n.add(a.id); else n.delete(a.id); return n })} />)}
          </div>
          <Field label="Your country" hint="Lists differ from one country to the next." className="su-gap">
            <Select value={region} onChange={(e) => setRegion(e.target.value)} options={REGIONS.map((r) => ({ value: r, label: names.of(r) || r }))} />
          </Field>
          {err && <div className="error su-gap" role="alert">{err}</div>}
          <p className="muted su-gap">{s.attribution}</p>
          <div className="su-consent-go">
            <Button variant="primary" busy={busy} onClick={turnOn}>Turn on streaming lists</Button>
            <Button variant="ghost" onClick={() => { setOpened(false); setErr('') }}>Don’t turn on</Button>
          </div>
        </div>
      )}
      <LegalNote kind="listings" newTab className="su-gap" />
      <LaterNote info={info} />
      <StepActions back={back}>
        {s.enabled
          ? <><Button busy={busy} onClick={turnOff}>Turn it off and forget them</Button><Button variant="primary" size="big" onClick={next}>Continue</Button></>
          : <>{!opened && <Button onClick={() => setOpened(true)}>Set this up…</Button>}<Button variant="primary" size="big" onClick={next}>Leave it off and continue</Button></>}
      </StepActions>
    </section>
  )
}
