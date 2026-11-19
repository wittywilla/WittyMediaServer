// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Fragment, useCallback, useEffect, useState, type ReactNode } from 'react'
import { api } from '../../api'
import { LoadError } from '../../components/common'
import { DataPlace, MoveProgress, firstPlace, usePlaceCheck, type Place } from '../../components/DataPlace'
import { loadDataLocation, moveData, size, syncedWords, type DataLocation, type MoveState } from '../../dataLocation'
import { Button, Notice, ask, toast } from '../../ui'
import { ConfirmOwner, type OwnerAsk } from './ConfirmOwner'
import { PageHead, Row, Section } from './ui'
import { num, t, tk, tn, tx } from '../../i18n'
import '../../components/dataPlace.css'

// Settings, Data folder: where the settings, database, artwork and logs are, and moving them. A move
// happens while the server starts again: it copies, checks the copy, and switches, and keeps the old
// copy until it's deleted here.

const FROM: Record<string, string> = {
  pointer: tk('Chosen in setup or on this page.'),
  default: tk('Nobody chose a folder, so it’s this computer’s usual place for an app’s data.'),
  install: tk('The install folder, found beside the program.'),
  flag: tk('Set with -data where the server is started.'),
  env: tk('Set by the WMS_DATA_DIR environment variable.'),
  config: tk('Set in the config file, wms.json.'),
  portable: tk('The install folder, as wms.json beside the program asks (portable mode).'),
  container: tk('The container’s /config volume.'),
  service: tk('The system service’s folder.'),
}

// A translated sentence with a folder in it: each {name} in the words is swapped for its piece.
function withParts(text: string, parts: Record<string, ReactNode>) {
  return text.split(/(\{\w+\})/).map((bit, i) => {
    const m = /^\{(\w+)\}$/.exec(bit)
    return <Fragment key={i}>{m && m[1] in parts ? parts[m[1]] : bit}</Fragment>
  })
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
  if (!view) return failed ? <LoadError what={t('This page')} onRetry={load} /> : null
  const busy = !!move && move.phase !== 'failed' && move.phase !== 'done'
  const go = async () => {
    if (!place || !check || check.problem) return
    const yes = await ask({
      title: t('Move your data?'),
      body: t('The server stops what it’s doing and starts again, copies {size} to {folder}, checks the copy, and uses it from then on. Anyone watching is interrupted for that long. The old copy stays until you delete it here.', { size: size(view.bytes), folder: check.path }),
      confirm: t('Move it'),
    })
    if (!yes) return
    try {
      const s = await moveData(place.choice, place.path, setMove)
      if (s.phase === 'done') { toast(t('Your data is in its new folder')); setPlace(null); load() }
    } catch (e) {
      setMove({ phase: 'failed', error: tx((e as Error).message) })
    }
  }
  const cancel = async () => {
    try { await api.del('/data-location/request'); load() } catch (e) { toast(tx((e as Error).message), { tone: 'danger' }) }
  }
  const deleteOld = () => setOwner({
    title: t('Delete the old copy?'),
    body: <p>{withParts(t('What was copied out of {folder} is deleted. Anything else in that folder stays. This can’t be undone.'), { folder: <code>{view.old?.from}</code> })}</p>,
    action: t('Delete it'),
    run: async (password, code) => {
      await api.post('/data-location/old/delete', { password, code })
      toast(t('The old copy is deleted'))
      load()
    },
  })
  return (
    <>
      <PageHead title={t('Data folder')} desc={t('Where this server keeps its settings, its database, artwork, modules and logs. Your media stays where it is.')} />
      <Section title={t('Where it is now')}>
        <Row label={t('Folder')} desc={tx(FROM[view.from])}><code className="port-file">{view.dataDir}</code></Row>
        <Row label={t('Size')} desc={t('Not counting temporary video, which a move leaves behind.')}>{tn('{size}, {count} file', '{size}, {count} files', view.files, { size: size(view.bytes), count: num(view.files) })}</Row>
        {view.apart && view.apart.length > 0 && <Row label={t('Kept apart')} desc={t('Set in wms.json or the environment, so a move leaves them where they are.')}>{view.apart.map((d) => <code key={d} className="port-file">{d}</code>)}</Row>}
        {view.synced && <Notice tone="warn" title={t('Synced by {program}.', { program: view.synced })}>{' '}{syncedWords(view.synced)} {t('Move it below.')}</Notice>}
        {view.last && !view.last.ok && <Notice tone="danger" title={t('The last move didn’t happen.')}>{' '}{tx(view.last.error)} {t('Your data stayed here.')}</Notice>}
        {view.last?.ok && view.old && <Notice tone="ok" title={t('Moved.')}>{' '}{withParts(t('Your data was copied here from {folder} and checked.'), { folder: <code>{view.old.from}</code> })}</Notice>}
        {view.old && (
          <Row label={t('The old copy')} desc={t('Kept in case you need it. Delete it once you’re happy everything works.')}>
            <Button variant="danger" onClick={deleteOld}>{t('Delete old copy')}</Button>
          </Row>
        )}
        {view.pending && <Notice tone="info" title={t('A move is waiting.')} action={<Button onClick={cancel}>{t('Cancel it')}</Button>}>{' '}{t('To {folder}, the next time the server starts.', { folder: view.pending.to })}</Notice>}
      </Section>
      <Section id="data-location" title={t('Move it')} desc={t('Copy everything to another folder, check the copy, and use it from then on.')}>
        {!view.movable ? <Notice tone="info">{tx(view.why)}</Notice>
          : move ? <><MoveProgress state={move} />{move.phase === 'failed' && <Button onClick={() => setMove(null)}>{t('Choose again')}</Button>}</>
          : place && <>
            <DataPlace view={view} place={place} onChange={setPlace} check={check} checking={checking} />
            {!view.canRestart && <Notice tone="warn">{t('This server can’t restart itself, and a move happens while it starts again.')}</Notice>}
            <div className="dp-actions"><Button variant="primary" onClick={go} disabled={!check || !!check.problem || checking || !!check.current || busy || !view.canRestart}>{t('Move data here')}</Button></div>
          </>}
      </Section>
      {owner && <ConfirmOwner ask={owner} onClose={() => setOwner(null)} />}
    </>
  )
}
