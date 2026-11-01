import { useEffect, useState } from 'react'
import { api } from '../../api'
import { forgetLogos } from '../../components/BrandLogo'
import LegalNote from '../../components/LegalNote'
import { IconCheck } from '../../icons'
import { Button, LoadError, Spinner } from '../../ui'
import { LogoConsentWords } from '../settings/MetadataOptional'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'

// One agreement, the same one Settings asks for, covering consoles, game studios, film studios, TV
// networks and channels. No logo comes with the server or in any package. They download only after a yes.
export function Logos({ info, next, back }: StepProps) {
  const [consent, setConsent] = useState<'yes' | 'no' | '' | null>(null)
  const [failed, setFailed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const load = () => { setFailed(false); api.get<{ consent: 'yes' | 'no' | '' }>('/logos/status').then((r) => setConsent(r.consent)).catch(() => setFailed(true)) }
  useEffect(load, [])
  const answer = async (accepted: boolean) => {
    setBusy(true); setErr('')
    try {
      await api.put('/logos/consent', { accepted, removeDownloaded: !accepted })
      forgetLogos()
      next()
    } catch (e: any) { setErr(e.message); setBusy(false) }
  }
  const head = <StepHead title="Logos for consoles, studios and networks" lead="Would you like real logos on your cards, instead of plain names? This is an optional download, and it’s up to you. Nothing changes if you say no." />
  if (failed) return <section>{head}<LoadError what="This question" onRetry={load} /><StepActions back={back}><Button variant="primary" size="big" onClick={next}>Skip for now</Button></StepActions></section>
  if (consent === null) return <section>{head}<Spinner /></section>
  return (
    <section>
      {head}
      {consent === 'yes' && <p className="oobe-added su-gap-under"><IconCheck size={16} /> You’ve said yes. Logos are downloading as your libraries are scanned.</p>}
      <div className="su-consent">
        <h2 className="oobe-type-head">About these pictures and who owns them</h2>
        <LogoConsentWords />
      </div>
      <LegalNote kind="logos" newTab className="su-gap" />
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <LaterNote info={info} />
      <StepActions back={back}>
        <Button busy={busy} onClick={() => answer(false)}>No thanks, use plain names</Button>
        <Button variant="primary" size="big" busy={busy} onClick={() => answer(true)}>I understand: download logos</Button>
      </StepActions>
    </section>
  )
}
