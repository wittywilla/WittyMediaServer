// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import { LoadError } from '../../components/common'
import { DataPlace, MoveProgress, firstPlace, usePlaceCheck, type Place } from '../../components/DataPlace'
import { loadDataLocation, moveData, size, syncedWords, type DataLocation, type MoveState } from '../../dataLocation'
import { Button, Notice, ask, toast } from '../../ui'
import { ConfirmOwner, type OwnerAsk } from './ConfirmOwner'
import { PageHead, Row, Section } from './ui'
import '../../components/dataPlace.css'

// Settings, Data folder: where the settings, database, artwork and logs are, and moving them. A move
// happens while the server starts again: it copies, checks the copy, and switches, and keeps the old
// copy until it's deleted here.

const FROM: Record<string, string> = {
  pointer: 'Chosen in setup or on this page.',
  default: 'Nobody chose a folder, so it’s this computer’s usual place for an app’s data.',
  install: 'The install folder, found beside the program.',
  flag: 'Set with -data where the server is started.',
  env: 'Set by the WMS_DATA_DIR environment variable.',
  config: 'Set in the config file, wms.json.',
  portable: 'The install folder, as wms.json beside the program asks (portable mode).',
  container: 'The container’s /config volume.',
  service: 'The system service’s folder.',
}

export default function DataFolderSettings() {
  const [view, setView] = useState<DataLocation | null>(null)
  const [failed, setFailed] = useState(false)
  const [place, setPlace] = useState<Place | null>(null)
  const [move, setMove] = useState<MoveState | null>(null)
  const [owner, setOwner] = useState<OwnerAsk | null>(null)
  const load = useCallback(() => {
    setFailed(false)
    loadDataLocation().then((v) => { setView(v); setPlace((p) => p || firstPlace(v)) }).catch(() => setFailed(true))
  }, [])
  useEffect(load, [load])
  const { check, checking } = usePlaceCheck(view?.movable ? view : null, place || { choice: 'documents', path: '' })
  if (!view) return failed ? <LoadError what="This page" onRetry={load} /> : null
  const busy = !!move && move.phase !== 'failed' && move.phase !== 'done'
  const go = async () => {
    if (!place || !check || check.problem) return
    const yes = await ask({
      title: 'Move your data?',
      body: `The server stops what it’s doing and starts again, copies ${size(view.bytes)} to ${check.path}, checks the copy, and uses it from then on. Anyone watching is interrupted for that long. The old copy stays until you delete it here.`,
      confirm: 'Move it',
    })
    if (!yes) return
    try {
      const s = await moveData(place.choice, place.path, setMove)
      if (s.phase === 'done') { toast('Your data is in its new folder'); setPlace(null); load() }
    } catch (e) {
      setMove({ phase: 'failed', error: (e as Error).message })
    }
  }
  const cancel = async () => {
    try { await api.del('/data-location/request'); load() } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }
  const deleteOld = () => setOwner({
    title: 'Delete the old copy?',
    body: <p>What was copied out of <code>{view.old?.from}</code> is deleted. Anything else in that folder stays. This can’t be undone.</p>,
    action: 'Delete it',
    run: async (password, code) => {
      await api.post('/data-location/old/delete', { password, code })
      toast('The old copy is deleted')
      load()
    },
  })
  return (
    <>
      <PageHead title="Data folder" desc="Where this server keeps its settings, its database, artwork, modules and logs. Your media stays where it is." />
      <Section title="Where it is now">
        <Row label="Folder" desc={FROM[view.from] || ''}><code className="port-file">{view.dataDir}</code></Row>
        <Row label="Size" desc="Not counting temporary video, which a move leaves behind.">{`${size(view.bytes)}, ${view.files.toLocaleString()} files`}</Row>
        {view.apart && view.apart.length > 0 && <Row label="Kept apart" desc="Set in wms.json or the environment, so a move leaves them where they are.">{view.apart.map((d) => <code key={d} className="port-file">{d}</code>)}</Row>}
        {view.synced && <Notice tone="warn" title={`Synced by ${view.synced}.`}>{' '}{syncedWords(view.synced)} Move it below.</Notice>}
        {view.last && !view.last.ok && <Notice tone="danger" title="The last move didn’t happen.">{' '}{view.last.error} Your data stayed here.</Notice>}
        {view.last?.ok && view.old && <Notice tone="ok" title="Moved.">{' '}Your data was copied here from <code>{view.old.from}</code> and checked.</Notice>}
        {view.old && (
          <Row label="The old copy" desc="Kept in case you need it. Delete it once you’re happy everything works.">
            <Button variant="danger" onClick={deleteOld}>Delete old copy</Button>
          </Row>
        )}
        {view.pending && <Notice tone="info" title="A move is waiting." action={<Button onClick={cancel}>Cancel it</Button>}>{' '}To {view.pending.to}, the next time the server starts.</Notice>}
      </Section>
      <Section id="data-location" title="Move it" desc="Copy everything to another folder, check the copy, and use it from then on.">
        {!view.movable ? <Notice tone="info">{view.why}</Notice>
          : move ? <><MoveProgress state={move} />{move.phase === 'failed' && <Button onClick={() => setMove(null)}>Choose again</Button>}</>
          : place && <>
            <DataPlace view={view} place={place} onChange={setPlace} check={check} checking={checking} />
            {!view.canRestart && <Notice tone="warn">This server can’t restart itself, and a move happens while it starts again.</Notice>}
            <div className="dp-actions"><Button variant="primary" onClick={go} disabled={!check || !!check.problem || checking || !!check.current || busy || !view.canRestart}>Move data here</Button></div>
          </>}
      </Section>
      {owner && <ConfirmOwner ask={owner} onClose={() => setOwner(null)} />}
    </>
  )
}
