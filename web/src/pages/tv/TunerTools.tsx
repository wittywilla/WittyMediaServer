// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmtDateTime } from '../../api'
import { announce } from '../../a11y'
import { IconRefresh } from '../../icons'
import { toast } from '../../state'
import { ask } from '../../ui'
import { PageHead, Section } from '../settings/ui'
import SpecPage from '../../spec/SpecPage'
import { handledIds } from '../../spec/model'
import { duplicatesSpec } from '../../spec/specs/duplicates'
import { forgetLineup, type BulkAction, type DuplicateGroup, type HealthRun, type TunerChannel } from './tvApi'

// Settings → Channels & tuners, for a provider's playlist of thousands: whether each channel
// still opens (tv-channel-health), changing many channels at once, and the same channel listed
// twice made into one, the others its backups, tried in turn when it fails to open (tv-duplicates).

// A channel's health, in a word, and why it failed.
export function ChannelHealth({ c }: { c: TunerChannel }) {
  if (c.health === 'ok') return <small className="tvs-clear" title={c.healthAt ? `Checked ${fmtDateTime(c.healthAt * 1000)}` : undefined}>Opens</small>
  if (c.health === 'failing') return <small className="tvs-lock">Failing{c.healthMsg ? `: ${c.healthMsg}` : ''}{c.healthAt ? <span className="tvs-dim"> · {fmtDateTime(c.healthAt * 1000)}</span> : null}</small>
  return <small className="tvs-dim">Not checked</small>
}

// Which channel this one is a backup of, or how many it has.
export function Merged({ c, all }: { c: TunerChannel; all: TunerChannel[] }) {
  if (c.mergedInto) {
    const main = all.find((x) => x.id === c.mergedInto)
    return <small className="tvs-dim">Backup of {main ? `${main.number} ${main.name}` : 'another channel'}: hidden from the guide</small>
  }
  if (c.backups) return <small className="tvs-dim">{c.backups === 1 ? '1 backup' : `${c.backups} backups`}, tried in turn when it fails to open</small>
  return null
}

const run = (ids: number[], action: BulkAction, extra?: { start?: string; into?: number }) => api.post('/tuner-channels/bulk', { ids, action, ...extra })

// What can be done to the channels ticked, in the order the list shows them.
export function BulkBar({ picked, all, health, merging, onDone, onClear }: { picked: TunerChannel[]; all: TunerChannel[]; health: boolean; merging: boolean; onDone: () => void; onClear: () => void }) {
  const [start, setStart] = useState('')
  const [busy, setBusy] = useState('')
  const ids = picked.map((c) => c.id)
  const n = picked.length
  const word = n === 1 ? '1 channel' : `${n} channels`
  const go = async (action: BulkAction, extra?: { start?: string; into?: number }, done?: string) => {
    setBusy(action)
    try { await run(ids, action, extra); toast(done || 'Done'); announce(done || 'Done'); onDone() } catch (e) { toast((e as Error).message) } finally { setBusy('') }
  }
  const merge = async () => {
    const main = picked[0]
    if (!await ask({ title: `Make ${word} into one?`, body: `${main.number} ${main.name} stays in the guide. The others become its backups, hidden from the guide and tried in the order shown when it fails to open.`, confirm: 'Make them one' })) return
    void go('merge', { into: main.id }, `${main.name} has ${n - 1} ${n - 1 === 1 ? 'backup' : 'backups'}`)
  }
  const anyMerged = picked.some((c) => c.mergedInto || c.backups)
  const others = all.length - n
  return (
    <div className="tvs-toolbar tvs-bulk" role="group" aria-label={`${word} selected`}>
      <b>{word} selected</b>
      <button className="btn sm" disabled={!!busy} onClick={() => void go('enable', undefined, `${word} shown in the guide`)}>Show in guide</button>
      <button className="btn sm" disabled={!!busy} onClick={() => void go('disable', undefined, `${word} hidden from the guide`)}>Hide from guide</button>
      <span className="tvs-bulk-renumber">
        <input className="input tvs-num-in" inputMode="numeric" aria-label="First number" placeholder="From" value={start} onChange={(e) => setStart(e.target.value.replace(/[^0-9.]/g, ''))} />
        <button className="btn sm" disabled={!!busy || !start} onClick={() => void go('renumber', { start }, `${word} numbered from ${start}`)} title="Numbers them from this one upward, in the order shown">Renumber</button>
      </span>
      {merging && n > 1 && <button className="btn sm" disabled={!!busy} onClick={() => void merge()} title={`The first one shown stays, the others become its backups`}>Make into one</button>}
      {merging && anyMerged && <button className="btn sm" disabled={!!busy} onClick={() => void go('unmerge', undefined, 'Separate again')}>Separate</button>}
      {health && <button className="btn sm" disabled={!!busy} onClick={() => void go('check', undefined, `Checking ${word}`)}>{busy === 'check' ? 'Checking…' : 'Check health'}</button>}
      <button className="btn sm ghost" onClick={onClear}>Clear selection</button>
      {others > 0 && <span className="tvs-dim">Renumbering starts at the first one shown.</span>}
    </div>
  )
}

// Checking every channel of a tuner, a few at a time, with how far it has got.
export function CheckAll({ tunerId, onDone }: { tunerId: string; onDone: () => void }) {
  const [state, setState] = useState<HealthRun | null>(null)
  useEffect(() => { api.get<HealthRun>(`/tuners/${tunerId}/health`).then(setState).catch(() => {}) }, [tunerId])
  useEffect(() => {
    if (!state?.running) return
    const t = setTimeout(() => api.get<HealthRun>(`/tuners/${tunerId}/health`).then((s) => {
      setState(s)
      if (!s.running) { onDone(); forgetLineup(); toast(`Checked ${s.done} ${s.done === 1 ? 'channel' : 'channels'}: ${s.ok} open, ${s.failing} failing`) }
    }).catch(() => {}), 1500)
    return () => clearTimeout(t)
  }, [state, tunerId]) // eslint-disable-line react-hooks/exhaustive-deps
  const start = async () => { try { setState(await api.post<HealthRun>(`/tuners/${tunerId}/health`)) } catch (e) { toast((e as Error).message) } }
  const stop = async () => { try { await api.del(`/tuners/${tunerId}/health`); setState((s) => (s ? { ...s, running: false } : s)); onDone() } catch (e) { toast((e as Error).message) } }
  const pct = state && state.total ? Math.round((state.done / state.total) * 100) : 0
  return (
    <div className="tvs-toolbar">
      {state?.running ? (
        <>
          <div className="tvs-progress tvs-grow" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label="Checking channels"><i style={{ width: pct + '%' }} /><span>{state.done} of {state.total} checked · {state.ok} open · {state.failing} failing</span></div>
          <button className="btn sm" onClick={() => void stop()}>Stop checking</button>
        </>
      ) : (
        <>
          <button className="btn sm" onClick={() => void start()}><IconRefresh /> Check all channels</button>
          <span className="tvs-dim">{state && state.total ? `Last check: ${state.ok} open, ${state.failing} failing, of ${state.total}.` : 'Opens each channel shown in the guide for a moment, a few at a time, to see which still work.'}</span>
        </>
      )}
    </div>
  )
}

// The same channel listed more than once, across every tuner: each group made one in one press.
export function Duplicates() {
  const [groups, setGroups] = useState<DuplicateGroup[] | null>(null)
  const [err, setErr] = useState('')
  const [main, setMain] = useState<Record<number, number>>({})
  const [busy, setBusy] = useState(-1)
  const load = () => api.get<{ groups: DuplicateGroup[] }>('/tuners/duplicates').then((d) => { setGroups(d.groups || []); setErr('') }).catch((e) => setErr((e as Error).message))
  useEffect(() => { void load() }, [])
  const merge = async (g: DuplicateGroup, i: number) => {
    const into = main[i] ?? g.suggested
    setBusy(i)
    try {
      await api.post('/tuner-channels/bulk', { ids: [into, ...g.channels.map((c) => c.id).filter((x) => x !== into)], action: 'merge', into })
      forgetLineup()
      const kept = g.channels.find((c) => c.id === into)
      toast(`${kept ? kept.name : 'The channel'} has ${g.channels.length - 1} ${g.channels.length - 1 === 1 ? 'backup' : 'backups'}`)
      setGroups((l) => l?.filter((x) => x !== g) || null)
    } catch (e) { toast((e as Error).message) } finally { setBusy(-1) }
  }
  const page = duplicatesSpec(groups, main, busy, err)
  return (
    <>
      <PageHead title="Duplicate channels" desc="The same channel from more than one tuner or playlist. Make each into one: the one you keep stays in the guide, and the others become its backups, tried in turn when it fails to open." actions={<Link className="btn" to="..">Back</Link>} />
      <Section title="Found" actions={<button className="btn sm" onClick={() => void load()}><IconRefresh /> Look again</button>}>
        <SpecPage embedded bare page={page} handled={handledIds(page)} onAction={(id) => {
          const [what, a1, a2] = id.split(':')
          if (what === 'keep') setMain((m) => ({ ...m, [Number(a1)]: Number(a2) }))
          else if (what === 'merge' && groups) void merge(groups[Number(a1)], Number(a1))
        }} />
      </Section>
    </>
  )
}
