// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { DataPlace, MoveProgress, firstPlace, usePlaceCheck, type Place } from '../../components/DataPlace'
import { loadDataLocation, moveData, type DataLocation, type MoveState } from '../../dataLocation'
import { Button, LoadError, Spinner } from '../../ui'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'
import { t, tx } from '../../i18n'

// Step: where the data is kept. Only on a desktop install that nobody has told yet (the server
// says so: GET /api/data-location, ask). Documents is chosen to begin with. Continuing moves the
// little there is so far, while the server starts again, and goes on from there.
export function Storage({ info, next, back, refresh }: StepProps) {
  const [view, setView] = useState<DataLocation | null>(null)
  const [err, setErr] = useState('')
  const [place, setPlace] = useState<Place | null>(null)
  const [move, setMove] = useState<MoveState | null>(null)
  useEffect(() => {
    loadDataLocation().then((v) => { setView(v); setPlace(firstPlace(v)) }).catch((e) => setErr(tx((e as Error).message)))
  }, [])
  const { check, checking } = usePlaceCheck(view, place || { choice: 'documents', path: '' })
  // Not knowing mustn't stop setup: the data stays where it is, and Settings can move it later.
  if (err) return <><StepHead title={tx(info.label)} lead={err} /><LoadError what={t('Where your data is kept')} onRetry={() => location.reload()} /><StepActions back={back}><Button variant="primary" onClick={next}>{t('Continue')}</Button></StepActions></>
  if (!view || !place) return <Spinner />
  const busy = !!move && move.phase !== 'failed' && move.phase !== 'done'
  const ok = !!check && !check.problem && !checking
  const go = async () => {
    try {
      const s = await moveData(place.choice, place.path, setMove)
      if (s.phase === 'done') {
        await refresh()
        next()
      }
    } catch (e) {
      setMove({ phase: 'failed', error: tx((e as Error).message) })
    }
  }
  return (
    <section>
      <StepHead title={t('Where should WittyMediaServer keep its settings and data?')}
        lead={t('One folder holds your library’s details, accounts, settings, artwork, modules and logs. Your films, music and other media stay where they are.')} />
      {move ? <MoveProgress state={move} /> : <DataPlace view={view} place={place} onChange={setPlace} check={check} checking={checking} />}
      <LaterNote info={info} />
      <StepActions back={busy ? undefined : back} note={!move && check?.current ? t('Nothing needs moving.') : undefined}>
        {move?.phase === 'failed'
          ? <Button variant="primary" onClick={() => setMove(null)}>{t('Choose again')}</Button>
          : <Button variant="primary" onClick={go} disabled={!ok || busy}>{busy ? t('Moving…') : t('Keep it here')}</Button>}
      </StepActions>
    </section>
  )
}
