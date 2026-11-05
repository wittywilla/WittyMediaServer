import { useEffect, useState } from 'react'
import { api } from '../../api'
import { IconCheck, IconPlus } from '../../icons'
import { Button, LoadError, Spinner } from '../../ui'
import TunerSetup from '../tv/TunerSetup'
import type { Driver, Tuner } from '../tv/tvApi'
import '../tv/tvsettings.css'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'

// Live TV, for a server with WittyTuner: the same steps as Settings, Channels & tuners, Add live
// TV. Leaving it for later changes nothing, so that's the big button until something's added.
export function Tv({ info, next, back }: StepProps) {
  const [drivers, setDrivers] = useState<Driver[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [tuners, setTuners] = useState<Tuner[]>([])
  const [open, setOpen] = useState(false)
  const load = () => {
    setFailed(false)
    api.get<Driver[]>('/tuners/drivers').then(setDrivers).catch(() => setFailed(true))
    api.get<Tuner[]>('/tuners').then(setTuners).catch(() => {})
  }
  useEffect(load, [])
  const head = <StepHead title="Live TV" lead="Add a TV tuner, a provider’s playlist or a single stream, and its channels sit in the guide beside the ones made from your library. Encrypted channels are shown as encrypted and never decrypted. No channel lists or stream addresses come with the server: bring your own." />
  if (failed) return <section>{head}<LoadError what="Live TV" onRetry={load} /><StepActions back={back}><Button variant="primary" size="big" onClick={next}>Skip for now</Button></StepActions></section>
  if (!drivers) return <section>{head}<div className="tvs-toolbar" role="status"><Spinner /> <span className="muted">Asking WittyTuner what this server can use…</span></div></section>
  return (
    <section>
      {head}
      {tuners.length > 0 && (
        <ul className="tvs-list" aria-label="Added">
          {tuners.map((t) => (
            <li key={t.id} className="tvs-item">
              <IconCheck size={16} />
              <div className="tvs-item-text"><b>{t.name}</b><span>{t.channels} {t.channels === 1 ? 'channel' : 'channels'}{t.encrypted ? `, ${t.encrypted} encrypted` : ''}</span></div>
            </li>
          ))}
        </ul>
      )}
      <Button icon={<IconPlus />} onClick={() => setOpen(true)}>{tuners.length ? 'Add another' : 'Add live TV'}</Button>
      {open && <TunerSetup drivers={drivers} inSetup onClose={() => { setOpen(false); load() }} onAdded={() => api.get<Tuner[]>('/tuners').then(setTuners).catch(() => {})} />}
      <LaterNote info={info} />
      <StepActions back={back}>
        <Button variant="primary" size="big" onClick={next}>{tuners.length ? 'Next' : 'Skip for now'}</Button>
      </StepActions>
    </section>
  )
}
