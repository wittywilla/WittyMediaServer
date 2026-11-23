// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What a watch party looks like from the inside, wherever it is shown: the dialog on a
// page (WatchParty.tsx) and the panel in the player (PartyPanel.tsx) both hold this. Before
// there is a party: starting one, joining with a code, answering invitations. In one: its
// code, who is there, the queue, the host's choices, inviting people, and leaving.
//
// Nothing here opens a window of its own. Inviting and "end the party?" are views of this
// same body, so that they show inside a player that fills the screen, where a dialog
// drawn on the page behind it would not.
import { useEffect, useRef, useState } from 'react'
import { api, fmtTime, profileName, type HouseholdProfile, type ShowDetail } from '../api'
import { IconCheck, IconClose, IconCopy, IconDown, IconMore, IconNext, IconPlay, IconPlus, IconPrev, IconUp, IconUsers } from '../icons'
import { Button, Field, IconButton, Input, Notice, Spinner, Switch } from '../ui'
import { toast } from '../state'
import {
  MEMBER_WORDS, createParty, endParty, joinParty, leaveParty, partyHost, partyInfo, partyInvite, partyKick, partyQueue, partySelect, partySet,
  partyStepQueue, useParty, type PartyInfo, type PartyRoom,
} from './party'
import { declineInvite, refreshInvites, useInvites } from './invites'
import { partyKind, queueAdd, queueEditable, queueMove, queuePlayNow, queueRemove, seasonQueue, tidyCode, type QueueEdit } from './partyLogic'
import { t, tn, tx, bidi } from '../i18n'
import './remote.css'
import './party.css'

const say = (e: unknown) => toast((e as Error).message, { tone: 'warn' })

// What a page or a player offers the party: the title it is showing. An episode names its
// show, so that a party started on it carries on through the season; the music player
// hands over its own queue.
export type PartyItem = {
  id: number; title: string; kind: string; showId?: number
  positionSec?: () => number
  queue?: () => { items: number[]; index: number }
}

const DEFAULT_QUEUE = 200

function Start({ item, onDone }: { item?: PartyItem; onDone: () => void }) {
  const [info, setInfo] = useState<PartyInfo | null>(null)
  const [failed, setFailed] = useState('')
  const [everyone, setEveryone] = useState(false)
  const [code, setCode] = useState('')
  const [busy, setBusy] = useState('')
  const [wrong, setWrong] = useState('')
  const invites = useInvites()
  useEffect(() => { partyInfo().then(setInfo).catch((e) => setFailed(e.message)) }, [])
  if (failed) return <Notice tone="warn">{failed}</Notice>
  if (!info) return <Spinner />
  if (!info.enabled) return <Notice tone="info" title={t('Watch parties are turned off')}>{t('An administrator can turn them on under Settings, Sharing & photos.')}</Notice>
  if (info.canJoin === false) return <Notice tone="info" title={t('Watch parties are turned off for this profile')}>{t('Whoever manages this profile can turn them on under Settings, Profiles, Limits.')}</Notice>
  const kind = item ? partyKind(item.kind) : 'none'
  const start = async () => {
    if (!item) return
    setBusy('start')
    try {
      const max = info.limits?.queue || DEFAULT_QUEUE
      let q = { items: [item.id], index: 0 }
      if (item.queue) {
        const mine = item.queue()
        // The songs from this one on, as many as a party holds.
        if (mine.items[mine.index] === item.id) q = { items: mine.items.slice(mine.index, mine.index + max), index: 0 }
      } else if (item.showId != null) {
        // The rest of the season follows by itself. Without the list, the episode alone.
        try { q = { items: seasonQueue((await api.get<ShowDetail>(`/shows/${item.showId}`)).episodes, item.id, max), index: 0 } } catch { /* the one episode */ }
      }
      // The body turns into the room, with its code to give out; the player opens behind it.
      await createParty(q.items, q.index, everyone, item.positionSec?.() || 0)
    } catch (e) { say(e) } finally { setBusy('') }
  }
  const join = async (key: { code: string } | { roomId: string }) => {
    setBusy('join'); setWrong('')
    try { await joinParty(key); refreshInvites(); onDone() } catch (e) { setWrong((e as Error).message) } finally { setBusy('') }
  }
  // Parties this sign-in still has a seat in (from another window, or before a reload).
  const seats = info.rooms.filter((r) => r.memberId)
  return (
    <>
      {item && kind !== 'none' && info.canCreate && (
        <section className="wp-part" aria-labelledby="wp-start">
          <h3 id="wp-start">{t('Start one for “{title}”', { title: item.title })}</h3>
          <p className="muted">
            {kind === 'music' ? t('You get a code to give to the people you want to listen with. It starts paused, so you can wait for them.') : t('You get a code to give to the people you want to watch with. It starts paused, so you can wait for them.')}
            {item.showId != null && ' ' + t('The rest of the season follows by itself.')}
            {item.queue && ' ' + t('The songs queued after this one follow by themselves.')}
          </p>
          <div className="wp-row">
            <span id="wp-everyone">{t('Everyone can play, pause and seek')}</span>
            <Switch labelledBy="wp-everyone" checked={everyone} onChange={setEveryone} />
          </div>
          <Button variant="primary" icon={<IconUsers />} busy={busy === 'start'} disabled={!!busy} onClick={start}>{kind === 'music' ? t('Start a listening party') : t('Start a watch party')}</Button>
        </section>
      )}
      {item && kind === 'none' && <Notice tone="info" title={t('This can’t be kept in step')}>{t('Watch parties here are for films, episodes, other videos and songs. A disc with menus plays its own way on every screen.')}</Notice>}
      {item && kind !== 'none' && !info.canCreate && <Notice tone="info">{t('This account can’t start a watch party. It can join one with a code.')}</Notice>}
      <section className="wp-part" aria-labelledby="wp-join">
        <h3 id="wp-join">{t('Join one')}</h3>
        <form className="wp-join" onSubmit={(e) => { e.preventDefault(); if (code) join({ code }) }}>
          <Field label={t('Code')} hint={t('Six letters and numbers, from whoever started it.')} error={wrong || undefined}>
            <Input value={code} onChange={(e) => setCode(tidyCode(e.target.value).slice(0, 12))} maxLength={16} autoCapitalize="characters" autoComplete="off" autoCorrect="off" spellCheck={false} enterKeyHint="go" className="wp-code-in" />
          </Field>
          <Button type="submit" busy={busy === 'join'} disabled={!!busy || !code}>{t('Join')}</Button>
        </form>
        {(invites.length > 0 || seats.length > 0) && (
          <ul className="rc-list" aria-label={t('Your parties and invitations')}>
            {seats.map((r) => (
              <li key={r.id} className="rc-device">
                <span className="rc-icon" aria-hidden="true"><IconUsers /></span>
                <div className="rc-what"><div className="rc-name">{r.name}</div><div className="rc-state">{[r.hostName && t('Started by {name}', { name: r.hostName }), tn('{count} there', '{count} there', r.members), t('your place is kept')].filter(Boolean).map(bidi).join(' · ')}</div></div>
                <Button size="sm" disabled={!!busy} onClick={() => join({ roomId: r.id })}>{t('Go back in')}</Button>
              </li>
            ))}
            {invites.map((r) => (
              <li key={r.id} className="rc-device">
                <span className="rc-icon" aria-hidden="true"><IconUsers /></span>
                <div className="rc-what"><div className="rc-name">{r.name}</div><div className="rc-state">{[r.hostName && t('{name} invited you', { name: r.hostName }), tn('{count} there', '{count} there', r.members)].filter(Boolean).map(bidi).join(' · ')}</div></div>
                <div className="pp-pair">
                  <Button size="sm" variant="primary" disabled={!!busy} onClick={() => join({ roomId: r.id })}>{t('Accept')}</Button>
                  <Button size="sm" variant="ghost" disabled={!!busy} onClick={() => declineInvite(r.id)}>{t('Decline')}</Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  )
}

// Inviting by name. The names are the ones this account is already shown for switching
// profile (GET /profiles/household), no more: an account that isn't shown that list gives
// out the code instead.
function Invite({ room, sent, onSent, onBack }: { room: PartyRoom; sent: Set<number>; onSent: (ids: number[]) => void; onBack: () => void }) {
  const [people, setPeople] = useState<HouseholdProfile[] | null>(null)
  const [failed, setFailed] = useState('')
  const [chosen, setChosen] = useState<Set<number>>(new Set())
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<{ profiles: HouseholdProfile[] }>('/profiles/household').then((r) => setPeople(r.profiles.filter((p) => !p.current))).catch((e) => setFailed(e.message))
  }, [])
  const here = new Set(room.members.map((m) => m.userId))
  const flip = (id: number) => setChosen((c) => { const n = new Set(c); if (n.has(id)) n.delete(id); else n.add(id); return n })
  const send = async () => {
    setBusy(true)
    try {
      const ids = [...chosen]
      await partyInvite(ids)
      onSent(ids)
      toast(tn('Invitation sent', '{count} invitations sent', ids.length), { tone: 'ok' })
      onBack()
    } catch (e) { say(e) } finally { setBusy(false) }
  }
  return (
    <section className="wp-part pp-view" aria-labelledby="pp-invite-h">
      <h3 id="pp-invite-h">{t('Invite people')}</h3>
      <p className="muted">{t('They’re told that you invited them, not what is playing, and can join without the code. Anyone can still join with the code {code}.', { code: room.code })}</p>
      {failed && <Notice tone="warn">{failed}</Notice>}
      {!people && !failed && <Spinner />}
      {people && people.length === 0 && <Notice tone="info">{t('This account isn’t shown the other profiles on this server. Give people the code instead.')}</Notice>}
      {people && people.length > 0 && (
        <ul className="rc-list pp-people" aria-label={t('Profiles')}>
          {people.map((p) => {
            const there = here.has(p.id), was = sent.has(p.id), on = chosen.has(p.id)
            return (
              <li key={p.id}>
                <button type="button" role="checkbox" aria-checked={there || on} disabled={there || busy} className={'rc-device pp-person' + (on ? ' on' : '')} onClick={() => flip(p.id)}>
                  <span className="pp-check" aria-hidden="true">{(there || on) && <IconCheck />}</span>
                  <span className="rc-what"><span className="rc-name">{profileName(p)}</span><span className="rc-state">{there ? t('Already here') : was ? t('Invited') : p.kind === 'kids' ? t('Kids profile') : p.kind === 'guest' ? t('Guest') : t('Profile')}</span></span>
                </button>
              </li>
            )
          })}
        </ul>
      )}
      <div className="actions">
        <Button variant="ghost" onClick={onBack}>{t('Back')}</Button>
        {people && people.length > 0 && <Button variant="primary" busy={busy} disabled={!chosen.size} onClick={send}>{chosen.size > 1 ? tn('Invite {count} person', 'Invite {count} people', chosen.size) : t('Invite')}</Button>}
      </div>
    </section>
  )
}

function Queue({ room }: { room: PartyRoom }) {
  const { queue, items, state } = room
  const editable = room.canControl && queueEditable(queue)
  const [busy, setBusy] = useState(false)
  const list = useRef<HTMLOListElement | null>(null)
  // A command at a time: each is worked out from the queue as it stands.
  const send = async (fn: () => Promise<boolean>, focus?: string) => {
    if (busy) return
    setBusy(true)
    try {
      if (!await fn()) toast(t('The party changed just then. Have another look.'))
      // The button pressed may have moved or gone: stay on the same row.
      else if (focus) requestAnimationFrame(() => list.current?.querySelector<HTMLElement>(focus)?.focus())
    } catch (e) { say(e) } finally { setBusy(false) }
  }
  const edit = (e: QueueEdit | null, focus?: string) => { if (e) send(() => partyQueue(e), focus) }
  return (
    <section className="wp-part pp-queue-part" aria-labelledby="pp-queue-h">
      <div className="pp-head">
        <h3 id="pp-queue-h">{t('Queue')}</h3>
        {room.canControl && queue.length > 1 && (
          <div className="pp-pair">
            <IconButton label={t('Previous in the queue')} size="sm" disabled={busy || state.index <= 0} onClick={() => send(() => partyStepQueue('previous'))}><IconPrev /></IconButton>
            <IconButton label={t('Next in the queue')} size="sm" disabled={busy || state.index >= queue.length - 1} onClick={() => send(() => partyStepQueue('next'))}><IconNext /></IconButton>
          </div>
        )}
      </div>
      <ol className="pp-queue" ref={list}>
        {queue.map((id, i) => {
          const it = items[i]
          const seen = id > 0 && !!it?.title
          const now = i === state.index
          const name = seen ? it.title! : t('Not available on this account')
          const off = seen && partyKind(it.kind) === 'none'
          return (
            <li key={`${i}-${id}`} className={'pp-q' + (now ? ' now' : '') + (seen ? '' : ' hidden')} aria-current={now ? 'true' : undefined}>
              <span className="pp-q-n" aria-hidden="true">{now ? <IconPlay /> : i + 1}</span>
              {room.canControl && !now
                ? <button type="button" className="pp-q-title" data-q={i} disabled={busy} onClick={() => send(() => partySelect(i))} aria-label={t('Play {name} now', { name })} title={t('Play this now')}>{name}</button>
                : <span className="pp-q-title">{name}</span>}
              <span className="pp-q-note">{now ? t('Playing') : off ? t('Not kept in step here') : seen && it.runtimeSec ? fmtTime(it.runtimeSec) : ''}</span>
              {editable && (
                <span className="pp-q-btns">
                  <IconButton label={t('Move {title} up', { title: name })} size="sm" data-up={i} disabled={busy || i === 0} onClick={() => edit(queueMove(queue, state.index, i, i - 1), `[data-up="${Math.max(1, i - 1)}"]`)}><IconUp /></IconButton>
                  <IconButton label={t('Move {title} down', { title: name })} size="sm" data-down={i} disabled={busy || i === queue.length - 1} onClick={() => edit(queueMove(queue, state.index, i, i + 1), `[data-down="${Math.min(queue.length - 2, i + 1)}"]`)}><IconDown /></IconButton>
                  <IconButton label={t('Take {name} out of the queue', { name })} size="sm" data-out={i} disabled={busy || queue.length < 2} onClick={() => edit(queueRemove(queue, state.index, i), `[data-out="${Math.min(queue.length - 2, i)}"]`)}><IconClose /></IconButton>
                </span>
              )}
            </li>
          )
        })}
      </ol>
      {room.canControl && !editable && queue.length > 0 && <p className="muted pp-small">{t('Something in this queue isn’t available on this account, so this account can step through it but not change it.')}</p>}
    </section>
  )
}

function Room({ room, item, onClose }: { room: PartyRoom; item?: PartyItem; onClose: () => void }) {
  const host = room.hostId === room.you
  const [view, setView] = useState<'main' | 'invite' | 'end'>('main')
  const [more, setMore] = useState('')
  const [invited, setInvited] = useState<Set<number>>(new Set())
  const [max, setMax] = useState(DEFAULT_QUEUE)
  const [busy, setBusy] = useState(false)
  useEffect(() => { partyInfo().then((i) => { if (i.limits?.queue) setMax(i.limits.queue) }).catch(() => {}) }, [])
  const copy = async () => {
    try { await navigator.clipboard.writeText(room.code); toast(t('Code copied'), { tone: 'ok' }) } catch { toast(t('Couldn’t copy it. The code is {code}', { code: room.code })) }
  }
  const leave = () => { leaveParty(); onClose() }
  const end = async () => {
    setBusy(true)
    try { await endParty(); onClose() } catch (e) { say(e) } finally { setBusy(false) }
  }
  if (view === 'invite') return <Invite room={room} sent={invited} onSent={(ids) => setInvited((s) => new Set([...s, ...ids]))} onBack={() => setView('main')} />
  if (view === 'end') {
    return (
      <section className="wp-part pp-view" role="alertdialog" aria-labelledby="pp-end-h" aria-describedby="pp-end-d">
        <h3 id="pp-end-h">{t('End the watch party for everyone?')}</h3>
        <p className="muted" id="pp-end-d">{t('Everyone’s player carries on by itself, out of step.')}</p>
        <div className="actions">
          <Button variant="ghost" autoFocus onClick={() => setView('main')}>{t('Cancel')}</Button>
          <Button variant="danger" busy={busy} onClick={end}>{t('End party')}</Button>
        </div>
      </section>
    )
  }
  const playing = room.items[room.state.index]
  const kind = partyKind(playing?.kind)
  const at = item ? room.queue.indexOf(item.id) : -1
  const offer = item && partyKind(item.kind) !== 'none' && room.canControl && room.state.itemId !== item.id
  const editable = queueEditable(room.queue)
  const change = async (e: QueueEdit | null, done: string, close: boolean) => {
    if (!e) { toast(t('The queue is full.')); return }
    setBusy(true)
    try {
      if (!await partyQueue(e)) toast(t('The party changed just then. Try again.'))
      else { toast(done, { tone: 'ok' }); if (close) onClose() }
    } catch (err) { say(err) } finally { setBusy(false) }
  }
  const playNow = () => {
    if (!item) return
    if (at >= 0) { setBusy(true); partySelect(at).then((ok) => { if (ok) onClose() }).catch(say).finally(() => setBusy(false)); return }
    change(queuePlayNow(room.queue, room.state.index, [item.id], max), t('Playing “{title}” in the party', { title: item.title }), true)
  }
  return (
    <>
      <div className="wp-code">
        <div><div className="wp-code-label">{t('Code')}</div><div className="wp-code-value" aria-label={t('Code {code}', { code: room.code.split('').join(' ') })}>{room.code}</div></div>
        <IconButton label={t('Copy the code')} onClick={copy}><IconCopy /></IconButton>
      </div>
      {room.timeUp
        ? <Notice tone="info" title={t('Your screen time is over')}>{t('You keep your place. The others carry on, and you join in again when you have more time.')}</Notice>
        : room.blocked
        ? <Notice tone="info" title={t('Can’t watch this one')}>{t('What the party is playing now isn’t available on this account. You keep your place, and join in again when the party moves on.')}</Notice>
        : playing?.title && kind === 'none'
          ? <Notice tone="info" title={t('This one isn’t kept in step here')}>{t('The party is on “{title}”, which this app can’t keep in step. You keep your place until the party moves on.', { title: playing.title })}</Notice>
          : playing?.title && <p className="muted pp-now">{kind === 'music' ? t('Listening to {title}', { title: playing.title }) : t('Watching {title}', { title: playing.title })}{room.state.waiting ? ' · ' + t('waiting for everyone to be ready') : room.state.paused ? ' · ' + t('paused') : ''}</p>}
      {offer && (at >= 0 || editable) && (
        <div className="pp-offer" role="group" aria-label={t('“{title}” and the party', { title: item.title })}>
          <Button icon={<IconPlay />} disabled={busy} onClick={playNow}>{t('Play “{title}” in the party now', { title: item.title })}</Button>
          {at < 0 && <Button icon={<IconPlus />} disabled={busy} onClick={() => change(queueAdd(room.queue, room.state.index, [item.id], 'end', max), t('“{title}” is in the party’s queue', { title: item.title }), false)}>{t('Add it to the queue')}</Button>}
        </div>
      )}
      {item && partyKind(item.kind) !== 'none' && !room.canControl && !room.blocked && room.state.itemId !== item.id && <p className="muted pp-small">{t('Only the host chooses what this party plays.')}</p>}
      <section className="wp-part" aria-labelledby="pp-people-h">
        <div className="pp-head">
          <h3 id="pp-people-h">{t('People')}</h3>
          {host && <Button size="sm" icon={<IconPlus />} onClick={() => setView('invite')}>{t('Invite')}</Button>}
        </div>
        <ul className="rc-list">
          {room.members.map((m) => (
            <li key={m.id} className="rc-device pp-member">
              <span className="rc-icon" aria-hidden="true"><IconUsers /></span>
              <div className="rc-what">
                <div className="rc-name">{m.name}{m.you && <span className="pill">{t('You')}</span>}{m.host && <span className="pill">{t('Host')}</span>}</div>
                <div className="rc-state">{tx(MEMBER_WORDS[m.state] || m.state)}</div>
              </div>
              {host && !m.you && <IconButton label={t('More for {name}', { name: m.name })} aria-expanded={more === m.id} onClick={() => setMore(more === m.id ? '' : m.id)}><IconMore /></IconButton>}
              {host && !m.you && more === m.id && (
                <div className="pp-more" role="group" aria-label={t('For {name}', { name: m.name })}>
                  <Button size="sm" onClick={() => { setMore(''); partyHost(m.id).catch(say) }}>{t('Make host')}</Button>
                  <Button size="sm" variant="danger" onClick={() => { setMore(''); partyKick(m.id).catch(say) }}>{t('Remove from the party')}</Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      </section>
      <Queue room={room} />
      {host && (
        <section className="wp-part" aria-label={t('How the party runs')}>
          <div className="wp-row"><span id="wp-pol">{t('Everyone can play, pause and seek')}</span><Switch labelledBy="wp-pol" checked={room.policy === 'everyone'} onChange={(v) => { partySet({ policy: v ? 'everyone' : 'host' }).catch(say) }} /></div>
          <div className="wp-row"><span id="wp-wait">{t('Wait for everyone when someone’s video stalls')}</span><Switch labelledBy="wp-wait" checked={room.waitForAll} onChange={(v) => { partySet({ waitForAll: v }).catch(say) }} /></div>
        </section>
      )}
      <div className="actions">
        <Button onClick={leave}>{t('Leave')}</Button>
        {host && <Button variant="danger" onClick={() => setView('end')}>{t('End party for everyone')}</Button>}
      </div>
    </>
  )
}

// PartyBody: the whole of it. `item` is the title the page or player around it is showing.
export default function PartyBody({ item, onClose }: { item?: PartyItem; onClose: () => void }) {
  const room = useParty()
  return room ? <Room room={room} item={item} onClose={onClose} /> : <Start item={item} onDone={onClose} />
}
