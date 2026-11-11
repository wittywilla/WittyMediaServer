// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The watch party dialog: starting one, joining one with its code, and, once in one, who
// is there and the host's choices. docs/SYNCPLAY.md is the guide people read.
import { useEffect, useState } from 'react'
import { IconCopy, IconMore, IconPlay, IconUsers } from '../icons'
import { Button, Dialog, Field, IconButton, Input, Menu, Notice, Spinner, Switch, ask } from '../ui'
import { toast } from '../state'
import { MEMBER_WORDS, createParty, endParty, joinParty, leaveParty, partyCommand, partyHost, partyInfo, partyKick, partySet, useParty, type PartyInfo, type PartyRoom } from './party'
import './remote.css'

const say = (e: unknown) => toast((e as Error).message, { tone: 'warn' })

type PartyItem = { id: number; title: string; positionSec?: () => number }

function Start({ item, onDone }: { item?: PartyItem; onDone: () => void }) {
  const [info, setInfo] = useState<PartyInfo | null>(null)
  const [failed, setFailed] = useState('')
  const [everyone, setEveryone] = useState(false)
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState('')
  const [wrong, setWrong] = useState('')
  useEffect(() => { partyInfo().then(setInfo).catch((e) => setFailed(e.message)) }, [])
  if (failed) return <Notice tone="warn">{failed}</Notice>
  if (!info) return <Spinner />
  if (!info.enabled) return <Notice tone="info" title="Watch parties are turned off">An administrator can turn them on under Settings, Sharing &amp; photos.</Notice>
  if (info.canJoin === false) return <Notice tone="info" title="Watch parties are turned off for this profile">Whoever manages this profile can turn them on under Settings, Profiles, Limits.</Notice>
  const start = async () => {
    if (!item) return
    setBusy('start')
    try { await createParty(item.id, everyone, item.positionSec?.() || 0); onDone() } catch (e) { say(e) } finally { setBusy('') }
  }
  const join = async (key: { code: string } | { roomId: string }) => {
    setBusy('join'); setWrong('')
    try { await joinParty(key); onDone() } catch (e) { setWrong((e as Error).message) } finally { setBusy('') }
  }
  const invited = info.rooms.filter((r) => r.invited || r.memberId)
  return (
    <>
      {item && info.canCreate && (
        <section className="wp-part" aria-labelledby="wp-start">
          <h3 id="wp-start">Start one for “{item.title}”</h3>
          <p className="muted">You get a code to give to the people you want to watch with. It starts paused, so you can wait for them.</p>
          <div className="wp-row">
            <span id="wp-everyone">Everyone can play, pause and seek</span>
            <Switch labelledBy="wp-everyone" checked={everyone} onChange={setEveryone} />
          </div>
          <Button variant="primary" icon={<IconUsers />} busy={busy === 'start'} disabled={!!busy} onClick={start}>Start a watch party</Button>
        </section>
      )}
      <section className="wp-part" aria-labelledby="wp-join">
        <h3 id="wp-join">Join one</h3>
        <form className="wp-join" onSubmit={(e) => { e.preventDefault(); if (code.trim()) join({ code }) }}>
          <Field label="Code" hint="Six letters and numbers, from whoever started it." error={wrong || undefined}>
            <Input value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} maxLength={12} autoCapitalize="characters" autoComplete="off" spellCheck={false} className="wp-code-in" />
          </Field>
          <Button type="submit" busy={busy === 'join'} disabled={!!busy || !code.trim()}>Join</Button>
        </form>
        {invited.length > 0 && (
          <ul className="rc-list">
            {invited.map((r) => (
              <li key={r.id} className="rc-device">
                <span className="rc-icon" aria-hidden="true"><IconUsers /></span>
                <div className="rc-what"><div className="rc-name">{r.name}</div><div className="rc-state">{[r.hostName && `Started by ${r.hostName}`, `${r.members} watching`].filter(Boolean).join(' · ')}</div></div>
                <Button size="sm" disabled={!!busy} onClick={() => join({ roomId: r.id })}>{r.memberId ? 'Go back in' : 'Join'}</Button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  )
}

function Room({ room, item, onClose }: { room: PartyRoom; item?: PartyItem; onClose: () => void }) {
  const host = room.hostId === room.you
  const copy = async () => {
    try { await navigator.clipboard.writeText(room.code); toast('Code copied', { tone: 'ok' }) } catch { toast('Couldn’t copy it. The code is ' + room.code) }
  }
  const leave = () => { leaveParty(); onClose() }
  const end = async () => {
    if (!await ask({ title: 'End the watch party for everyone?', body: 'Everyone’s player carries on by itself, out of step.', confirm: 'End party', danger: true })) return
    try { await endParty(); onClose() } catch (e) { say(e) }
  }
  const playing = room.items[room.state.index]
  return (
    <>
      <div className="wp-code">
        <div><div className="wp-code-label">Code</div><div className="wp-code-value" aria-label={`Code ${room.code.split('').join(' ')}`}>{room.code}</div></div>
        <IconButton label="Copy the code" onClick={copy}><IconCopy /></IconButton>
      </div>
      {room.timeUp
        ? <Notice tone="info" title="Your screen time is over">You keep your place. The others carry on, and you join in again when you have more time.</Notice>
        : room.blocked
        ? <Notice tone="info" title="This one isn’t available on this account">You keep your place, and join in again when the party moves on.</Notice>
        : playing?.title && <p className="muted">Watching {playing.title}{room.state.waiting ? ' · waiting for everyone to be ready' : room.state.paused ? ' · paused' : ''}</p>}
      <ul className="rc-list" aria-label="People">
        {room.members.map((m) => (
          <li key={m.id} className="rc-device">
            <span className="rc-icon" aria-hidden="true"><IconUsers /></span>
            <div className="rc-what">
              <div className="rc-name">{m.name}{m.you && <span className="pill">You</span>}{m.host && <span className="pill">Host</span>}</div>
              <div className="rc-state">{MEMBER_WORDS[m.state] || m.state}</div>
            </div>
            {host && !m.you && (
              <Menu label={`More for ${m.name}`} trigger={(p) => <button type="button" {...p} className="btn ghost sm square" aria-label={`More for ${m.name}`}><IconMore /></button>} items={[
                { key: 'host', label: 'Make host', onSelect: () => { partyHost(m.id).catch(say) } },
                { key: 'kick', label: 'Remove from the party', danger: true, onSelect: () => { partyKick(m.id).catch(say) } },
              ]} />
            )}
          </li>
        ))}
      </ul>
      {host && (
        <>
          <div className="wp-row"><span id="wp-pol">Everyone can play, pause and seek</span><Switch labelledBy="wp-pol" checked={room.policy === 'everyone'} onChange={(v) => { partySet({ policy: v ? 'everyone' : 'host' }).catch(say) }} /></div>
          <div className="wp-row"><span id="wp-wait">Wait for everyone when someone’s video stalls</span><Switch labelledBy="wp-wait" checked={room.waitForAll} onChange={(v) => { partySet({ waitForAll: v }).catch(say) }} /></div>
        </>
      )}
      {item && room.canControl && room.state.itemId !== item.id && (
        <Button icon={<IconPlay />} onClick={() => { partyCommand('queue', { items: [item.id], index: 0 }).then(onClose).catch(say) }}>Watch “{item.title}” in the party</Button>
      )}
      <div className="actions">
        <Button onClick={leave}>Leave</Button>
        {host && <Button variant="danger" onClick={end}>End party for everyone</Button>}
      </div>
    </>
  )
}

export default function WatchParty({ item, onClose }: { item?: PartyItem; onClose: () => void }) {
  const room = useParty()
  return (
    <Dialog title={room ? room.name : 'Watch party'} onClose={onClose} className="rc-dialog"
      description={room ? undefined : 'Watch with other people at the same moment, each on your own screen.'}>
      {room ? <Room room={room} item={item} onClose={onClose} /> : <Start item={item} onDone={onClose} />}
    </Dialog>
  )
}
