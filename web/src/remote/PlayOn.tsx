// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// "Play on…": the other devices this account may control, what each is playing, a way to
// start or move something there, and a live remote for the one chosen. The controller's
// side of the Remote Protocol (docs/REMOTE-PROTOCOL.md). The events stream is open only
// while this dialog is.
import { useEffect, useRef, useState } from 'react'
import { ApiError, fmtTime } from '../api'
import { IconBack, IconDevices, IconPlay, IconRemote, IconTv } from '../icons'
import { Button, Dialog, EmptyState, LoadError, Notice, Spinner, Switch } from '../ui'
import { toast } from '../state'
import { canPlayOn, errorText, remoteControls, statusWords } from './capabilities'
import { myDeviceId, ownDeviceId, setTargetEnabled, targetEnabled, type RemoteCommand } from './device'
import DeviceRemote from './DeviceRemote'
import { positionNow } from './position'
import { sendCommand, useDevices, type RemoteDevice, type ServerClock } from './useDevices'
import { t, tx } from '../i18n'
import './remote.css'
import './playon.css'

export { sendCommand, type RemoteDevice } from './useDevices'

// What is open here and could be sent elsewhere: something from its page (with the queue it
// would start, for an album or a season), or what a player is playing now, which is handed
// over at its place. itemKind is the library's kind of the item: it decides which devices
// are offered. Left out, it is a video.
export type PlayOnSource =
  | { kind: 'item'; itemId: number; itemKind?: string; title: string; queue?: number[]; queueIndex?: number }
  | { kind: 'playing'; itemId: number; itemKind?: string; title: string; positionSec: () => number; audio?: string; subtitle?: string; queue?: number[]; queueIndex?: number; stop: () => void }

const kindOf = (s: PlayOnSource) => s.itemKind || 'video'

// What the app's one dialog is asked to open with (RemoteHost): something to play, or a device's remote.
export type PlayOnAsk = { source?: PlayOnSource; remote?: string }

function DeviceRow({ d, source, serverNow, onRemote, onPlayed }: { d: RemoteDevice; source?: PlayOnSource; serverNow: ServerClock; onRemote: () => void; onPlayed: () => void }) {
  const [busy, setBusy] = useState(false)
  const [, setTick] = useState(0)
  useEffect(() => {
    if (d.state.status !== 'playing') return
    const t = setInterval(() => setTick((n) => n + 1), 1000)
    return () => clearInterval(t)
  }, [d.state.status])
  const send = async (cmd: RemoteCommand) => {
    setBusy(true)
    try { return await sendCommand(d.id, cmd) } catch (e) {
      const err = e as ApiError
      toast(errorText(err.data?.code, err.message, d.name, t), { tone: 'warn' })
      return null
    } finally { setBusy(false) }
  }
  const playHere = async () => {
    if (!source) return
    const queue = source.queue && source.queue.length > 1 && d.capabilities.features.includes('queue') ? { queue: source.queue, queueIndex: source.queueIndex || 0 } : null
    if (source.kind === 'item') {
      if (await send(queue ? { type: 'play', ...queue } : { type: 'play', itemId: source.itemId })) { toast(t('Playing on {name}', { name: d.name }), { tone: 'ok' }); onPlayed() }
      return
    }
    // Handing over what's playing here. As a device, the server takes it from this one's
    // own report and stops it here; otherwise it's a play at this place, stopped by hand.
    const me = myDeviceId()
    const res = me
      ? await send({ type: 'cast', from: me })
      : await send({ type: 'play', ...(queue || { itemId: source.itemId }), positionSec: source.positionSec(), audio: source.audio, subtitle: source.subtitle })
    if (!res) return
    if (!me || res.sourceStopped === false) source.stop()
    toast(t('Now playing on {name}', { name: d.name }), { tone: 'ok' })
    onPlayed()
  }
  const c = remoteControls(d)
  const st = d.state
  const words = tx(statusWords(d))
  const what = !d.online || st.status === 'idle' || d.hidden ? words
    : [d.title || words, c.place === 'time' ? t('{time} of {total}', { time: fmtTime(positionNow(st, d.stateAt, serverNow())), total: fmtTime(st.durationSec || 0) }) : '', st.status !== 'playing' ? words.toLowerCase() : ''].filter(Boolean).join(' · ')
  const plays = !!source && canPlayOn(d, kindOf(source))
  return (
    <li className={'rc-device rc-row' + (d.online ? '' : ' off')}>
      <span className="rc-icon" aria-hidden="true">{d.kind === 'tv' ? <IconTv /> : <IconDevices />}</span>
      <div className="rc-what">
        <div className="rc-name">{d.name}{!d.mine && <span className="pill">{d.userName}</span>}</div>
        <div className="rc-state">{what}</div>
        {source && d.online && !plays && <div className="rc-state">{t('It doesn’t play this kind of thing.')}</div>}
      </div>
      <div className="rc-acts">
        {plays && <Button size="sm" variant="primary" busy={busy} icon={<IconPlay />} onClick={playHere}>{source!.kind === 'playing' ? t('Move here') : t('Play here')}</Button>}
        <Button size="sm" icon={<IconRemote />} aria-label={t('Remote for {name}', { name: d.name })} data-device={d.id} disabled={!d.online} onClick={onRemote}>{t('Remote')}</Button>
      </div>
    </li>
  )
}

export default function PlayOn({ source, remote: remoteFor, onClose }: { source?: PlayOnSource; remote?: string; onClose: () => void }) {
  const { devices, error, off, retry, serverNow } = useDevices()
  const [target, setTarget] = useState(targetEnabled)
  const [chosen, setChosen] = useState(remoteFor || '')
  // Something was started or moved there: its remote is what's wanted next. A dialog opened
  // from a player goes when that player hands over and closes, so the app's own dialog is
  // asked to carry on with the remote.
  const here = useRef(true)
  useEffect(() => { here.current = true; return () => { here.current = false } }, [])
  const played = (id: string) => {
    setChosen(id)
    setTimeout(() => { if (!here.current) window.dispatchEvent(new CustomEvent<PlayOnAsk>('wms-play-on', { detail: { remote: id } })) }, 100)
  }
  // Every device but this one. Another tab of this browser shares its sign-in, and is another device.
  const others = (devices || []).filter((d) => !d.thisSession || d.id !== ownDeviceId())
  const remote = chosen ? others.find((d) => d.id === chosen) : undefined
  // Going between the list and a remote takes the focus along, for a keyboard or a TV remote:
  // to the remote's main key, and back to the row it was opened from.
  const body = useRef<HTMLDivElement | null>(null)
  const cameFrom = useRef('')
  const showing = remote?.id || ''
  useEffect(() => {
    const el = showing
      ? body.current?.querySelector<HTMLElement>('.rr-play') || body.current?.querySelector<HTMLElement>('.rr-key-o') || body.current?.querySelector<HTMLElement>('button')
      : cameFrom.current ? body.current?.querySelector<HTMLElement>(`[data-device="${CSS.escape(cameFrom.current)}"]`) : null
    el?.focus({ preventScroll: true })
    cameFrom.current = showing
  }, [showing])
  // The device whose remote was open has gone: back to the list, which says so.
  useEffect(() => { if (chosen && devices && !remote) setChosen('') }, [chosen, devices, remote])
  if (remote) {
    return (
      <Dialog title={remote.name} onClose={onClose} className="rc-dialog rc-remote">
        <div ref={body} className="rc-body">
          <div><Button size="sm" variant="ghost" icon={<IconBack />} onClick={() => setChosen('')}>{t('All devices')}</Button></div>
          <DeviceRemote d={remote} serverNow={serverNow} />
        </div>
      </Dialog>
    )
  }
  return (
    <Dialog title={t('Play on another device')} onClose={onClose} className="rc-dialog"
      description={source ? (source.kind === 'playing' ? t('Move “{title}” to another device, at the same place.', { title: source.title }) : t('Start “{title}” on another device.', { title: source.title })) : t('Start something on another device from its page, or pick a device here to use this one as its remote.')}>
      {off === 'profile' ? <Notice tone="info" title={t('Playing on other devices is turned off for this profile')}>{t('Whoever manages this profile can turn it on under Settings, Profiles, Limits. Other people can still play on this device.')}</Notice>
        : off ? <Notice tone="info" title={t('Playing on other devices is turned off')}>{t('An administrator can turn it on under Settings, Sharing & photos.')}</Notice>
        : error && !devices ? <LoadError what={t('Your devices')} onRetry={retry} />
        : !devices ? <Spinner />
        : others.length === 0 ? <EmptyState icon={<IconDevices />} title={t('No other devices right now')} text={t('Open the app on another device signed in as you, and it shows here.')} />
        : <div ref={body}><ul className="rc-list">{others.map((d) => <DeviceRow key={d.id} d={d} source={source} serverNow={serverNow} onRemote={() => setChosen(d.id)} onPlayed={() => played(d.id)} />)}</ul></div>}
      {off !== 'server' && (
        <div className="rc-here">
          <div><div className="rc-name" id="rc-here-label">{t('Let other devices play here')}</div><div className="rc-state">{t('This browser shows in the list on your other devices while the app is open.')}</div></div>
          <Switch labelledBy="rc-here-label" checked={target} onChange={(v) => { setTargetEnabled(v); setTarget(v) }} />
        </div>
      )}
    </Dialog>
  )
}
