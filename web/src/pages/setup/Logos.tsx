// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { forgetLogos } from '../../components/BrandLogo'
import LegalNote from '../../components/LegalNote'
import { IconCheck } from '../../icons'
import { Button, LoadError, Spinner } from '../../ui'
import { LogoConsentWords } from '../settings/MetadataOptional'
import { t } from '../../i18n'

// One agreement, the same one Settings asks for, covering consoles, game studios, film studios, TV
// networks and channels. No logo comes with the server or in any package. They download only after a yes.
export function Logos() {
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
      setConsent(accepted ? 'yes' : 'no')
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  if (failed) return <LoadError what={t('This question')} onRetry={load} />
  if (consent === null) return <Spinner />
  return (
    <div>
      {consent === 'yes' && <p className="oobe-added su-gap-under"><IconCheck size={16} /> {t('You’ve said yes. Logos are downloading as your libraries are scanned.')}</p>}
      <div className="su-consent">
        <h3 className="oobe-type-head">{t('About these pictures and who owns them')}</h3>
        <LogoConsentWords />
      </div>
      <LegalNote kind="logos" newTab className="su-gap" />
      {err && <div className="error su-gap" role="alert">{err}</div>}
      <div className="su-gap su-row">
        {consent !== 'no' && <Button busy={busy} onClick={() => answer(false)}>{consent === 'yes' ? t('Stop and remove them') : t('No thanks, use plain names')}</Button>}
        {consent !== 'yes' && <Button variant="primary" busy={busy} onClick={() => answer(true)}>{t('I understand: download logos')}</Button>}
      </div>
    </div>
  )
}
