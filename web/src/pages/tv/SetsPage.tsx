// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState } from 'react'
import MoreInfo from './MoreInfo'
import { Link, Navigate, Route, Routes, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api } from '../../api'
import { announce } from '../../a11y'
import { EmptyState } from '../../components/common'
import { IconClose, IconDown, IconEdit, IconGrip, IconPlus, IconTrash, IconTv, IconUp } from '../../icons'
import { toast } from '../../state'
import { ask, Switch } from '../../ui'
import { useFeatures } from '../../sdui/useSdui'
import { ChannelMark } from './ChannelLogo'
import { addAll, filterChannels, moveItem, removeAll, setNumber, toggleIn, type TvSet } from './guideLogic'
import { surfaceOf, themeRoot, useLook } from './look'
import { getLineup, getSets, type GuideChannel, type SetDetail } from './tvApi'
import VirtualList from './VirtualList'
import './tv.css'
import './tvthemes.css'
import './tvsets.css'
import { num, t, tn } from '../../i18n'

// Sets of channels (tv-sets): the guide's own lists, each in its own order. Anyone signed in can
// make their own; an administrator can share one with the household. /tv/sets lists them and
// puts them in order; /tv/sets/new and /tv/sets/<id> make and change one. docs/CHANNELS.md,
// "Sets of channels", says what they do.

const MAX_CHANNELS = 5000
const MAX_NAME = 40
// The height of a row in the long lists, in pixels: big enough to press, on a phone and with a remote.
const ROW = 52

export default function SetsPage() {
  const look = useLook()
  const features = useFeatures()
  return (
    <div className="ctv ctv-page ctv-list-page ctv-setspage" {...themeRoot(look)}>
      {features === null ? <div className="center" role="status" aria-label={t('Loading')}><div className="spinner" /></div>
        : !features.has('tv-sets') ? (
          <EmptyState icon={<IconTv />} title={t("This server doesn't keep sets of channels")} text={t("Sets of channels need a newer server. The guide's filters still choose which channels it shows.")}
            action={<Link className="ctv-btn" to="/tv">{t('Back to the guide')}</Link>} />
        ) : (
          <Routes>
            <Route index element={<SetList />} />
            <Route path="new" element={<SetEditor />} />
            <Route path=":id" element={<SetEditor />} />
            <Route path="*" element={<Navigate to="/tv/sets" replace />} />
          </Routes>
        )}
    </div>
  )
}

// ------------------------------------------------------------------ the list of sets

function SetList() {
  const [sets, setSets] = useState<TvSet[] | null>(null)
  const [err, setErr] = useState('')
  const [dragFrom, setDragFrom] = useState<number | null>(null)
  // More info on a set is the first channel's programme on now.
  const [info, setInfo] = useState<{ key: string; at: number } | null>(null)
  const showInfo = async (s: TvSet) => {
    try {
      const g = await api.get<{ channels: { key: string }[] }>(`/guide?hours=1&limit=1&set=${encodeURIComponent(s.id)}`)
      const first = g.channels?.[0]
      if (first) setInfo({ key: first.key, at: Date.now() / 1000 }); else toast(t('That set has no channels yet.'))
    } catch (e) { toast((e as Error).message) }
  }
  const load = () => getSets().then((s) => { setSets(s.sets.filter((x) => x.kind === 'custom')); setErr('') }).catch((e) => setErr((e as Error).message))
  useEffect(() => { void load() }, [])
  // The order is this account's own, whoever made the sets in it.
  const move = async (from: number, to: number) => {
    if (!sets || to < 0 || to >= sets.length || from === to) return
    const next = moveItem(sets, from, to)
    setSets(next)
    announce(t('{name}, now {position} of {count}', { name: sets[from].name, position: to + 1, count: sets.length }))
    try { await api.put('/tv-sets/order', { ids: next.map((s) => s.id) }) } catch (e) { toast((e as Error).message); void load() }
    // Moved with the keyboard, the keyboard stays on what moved.
    requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-set-row="${next[to].id}"] [data-move="${to > from ? 'down' : 'up'}"]`)?.focus())
  }
  const remove = async (s: TvSet) => {
    if (!await ask({ title: t('Delete the set “{name}”?', { name: s.name }), body: s.shared ? t('It goes for everyone in the household. The channels themselves stay.') : t('The channels themselves stay.'), confirm: t('Delete set'), danger: true })) return
    try { await api.del(`/tv-sets/${setNumber(s.id)}`); toast(t('{name} deleted', { name: s.name })); void load() } catch (e) { toast((e as Error).message) }
  }
  return (
    <>
      <header className="ctv-list-head">
        <h1>{t('Sets of channels')}</h1>
        <Link className="ctv-btn sm on" to="/tv/sets/new"><IconPlus size={16} /> {t('New set')}</Link>
        <Link className="ctv-btn sm" to="/tv">{t('Back to the guide')}</Link>
      </header>
      <p className="ctv-list-note ctv-sets-intro">{t("A set is a list of channels in an order of your own, to show in the guide on its own: choose it on the guide's bar, or press C, [ or ] there. The guide on each device remembers the set it last showed.")}</p>
      {err && <div className="ctv-err" role="alert">{err}</div>}
      {sets && sets.length === 0 && <p className="ctv-list-empty">{t('No sets yet. Press New set, or Add to set… on a channel in the guide.')}</p>}
      {sets && sets.length > 0 && (
        <ul className="ctv-list" aria-label={t('Your sets, in the order the guide shows them')}>
          {sets.map((s, i) => (
            <li key={s.id} className={'ctv-rec ctv-setrow' + (dragFrom === i ? ' dragging' : '')} data-set-row={s.id} draggable
              onDragStart={(e) => { setDragFrom(i); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', s.id) }}
              onDragOver={(e) => { if (dragFrom != null) e.preventDefault() }}
              onDrop={(e) => { e.preventDefault(); if (dragFrom != null) void move(dragFrom, i); setDragFrom(null) }}
              onDragEnd={() => setDragFrom(null)}>
              <span className="ctv-grip" aria-hidden="true"><IconGrip size={16} /></span>
              <div className="ctv-rec-main">
                <b>{s.name}</b>
                <span>{tn('{count} channel', '{count} channels', s.count)}{s.shared ? ' · ' + (s.mine ? t('Shared with the household') : t('Shared by an administrator')) : ''}</span>
              </div>
              <button type="button" className="ctv-btn sm" onClick={() => void showInfo(s)} aria-haspopup="dialog" aria-label={t('More info about what is on {name}', { name: s.name })}>{t('More info')}</button>
              <button type="button" className="ctv-btn sm icon" data-move="up" disabled={i === 0} onClick={() => void move(i, i - 1)} aria-label={t('Move {name} up', { name: s.name })}><IconUp size={16} /></button>
              <button type="button" className="ctv-btn sm icon" data-move="down" disabled={i === sets.length - 1} onClick={() => void move(i, i + 1)} aria-label={t('Move {name} down', { name: s.name })}><IconDown size={16} /></button>
              {s.editable && <Link className="ctv-btn sm" to={`/tv/sets/${setNumber(s.id)}`} aria-label={t('Change {name}', { name: s.name })}><IconEdit size={16} /> {t('Change')}</Link>}
              {s.editable && <button type="button" className="ctv-btn sm icon" onClick={() => void remove(s)} aria-label={t('Delete {name}', { name: s.name })}><IconTrash size={16} /></button>}
            </li>
          ))}
        </ul>
      )}
      {info && <MoreInfo channelKey={info.key} at={info.at} onClose={() => setInfo(null)} />}
    </>
  )
}

// ------------------------------------------------------------------ one set

function SetEditor() {
  const { id = 'new' } = useParams()
  const [params] = useSearchParams()
  const nav = useNavigate()
  const look = useLook()
  const isNew = id === 'new'
  const [name, setName] = useState('')
  const [shared, setShared] = useState(false)
  const [channels, setChannels] = useState<string[]>(() => (isNew && params.get('add') ? [params.get('add')!] : []))
  const [editable, setEditable] = useState(true)
  const [canShare, setCanShare] = useState(false)
  const [all, setAll] = useState<GuideChannel[] | null>(null)
  const [loaded, setLoaded] = useState(isNew)
  const [err, setErr] = useState('')
  const [dirty, setDirty] = useState(isNew && !!params.get('add'))
  const [busy, setBusy] = useState(false)
  const [q, setQ] = useState('')
  const [dragFrom, setDragFrom] = useState<number | null>(null)
  const [listH, setListH] = useState(() => Math.max(260, Math.round(window.innerHeight * 0.5)))
  useEffect(() => {
    const on = () => setListH(Math.max(260, Math.round(window.innerHeight * 0.5)))
    window.addEventListener('resize', on)
    return () => window.removeEventListener('resize', on)
  }, [])

  useEffect(() => {
    getSets().then((s) => setCanShare(s.canShare)).catch(() => {})
    getLineup().then(setAll).catch((e) => setErr((e as Error).message))
    if (isNew) return
    api.get<SetDetail>(`/tv-sets/${id}`).then((d) => {
      setName(d.name); setShared(d.shared); setChannels(d.channels); setEditable(d.editable); setLoaded(true)
    }).catch((e) => setErr((e as Error).message))
  }, [id, isNew])

  const byKey = useMemo(() => new Map((all || []).map((c) => [c.key, c])), [all])
  const shown = useMemo(() => filterChannels(all || [], q), [all, q])
  const inSet = useMemo(() => new Set(channels), [channels])
  const change = (next: string[]) => { setChannels(next); setDirty(true) }
  const moveTo = (from: number, to: number) => {
    if (to < 0 || to >= channels.length || from === to) return
    change(moveItem(channels, from, to))
    const c = byKey.get(channels[from])
    announce(t('{name}, now {position} of {count}', { name: c ? c.name : t('Channel'), position: to + 1, count: channels.length }))
  }
  const trimmed = name.trim()
  const tooMany = channels.length > MAX_CHANNELS
  const save = async () => {
    if (!trimmed || trimmed.length > MAX_NAME || tooMany) return
    setBusy(true)
    try {
      const body = { name: trimmed, channels, ...(canShare ? { shared } : {}) }
      if (isNew) await api.post('/tv-sets', body)
      else await api.put(`/tv-sets/${id}`, body)
      setDirty(false)
      toast(t('{name} saved', { name: trimmed }))
      nav('/tv/sets')
    } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  const remove = async () => {
    if (!await ask({ title: t('Delete the set “{name}”?', { name: trimmed || name }), body: shared ? t('It goes for everyone in the household. The channels themselves stay.') : t('The channels themselves stay.'), confirm: t('Delete set'), danger: true })) return
    try { await api.del(`/tv-sets/${id}`); toast(t('{name} deleted', { name: trimmed || name })); nav('/tv/sets') } catch (e) { toast((e as Error).message) }
  }
  const leave = async () => {
    if (dirty && !await ask({ title: t('Leave without saving?'), body: t('The changes to this set are lost.'), confirm: t('Leave'), cancel: t('Keep changing it'), danger: true })) return
    nav('/tv/sets')
  }

  if (err && !loaded) return <><SetHead title={t('Set of channels')} onBack={() => void leave()} /><div className="ctv-err" role="alert">{err}</div></>
  if (!loaded) return <div className="center" role="status" aria-label={t('Loading')}><div className="spinner" /></div>
  if (!editable) {
    return (
      <>
        <SetHead title={name} onBack={() => void leave()} />
        <p className="ctv-list-note">{shared ? t('An administrator shares this set with the household, and only an administrator can change it.') : t('Only whoever made this set can change it.')} {t('You can still show it in the guide, and put it where you like among your sets.')}</p>
      </>
    )
  }

  const on = surfaceOf(look)
  const row = (c: GuideChannel | undefined, key: string) => c ? (
    <>
      <span className="ctv-num">{c.number}</span>
      <span className="ctv-setch-logo"><ChannelMark ch={c} look={look} on={on} /></span>
      <span className="ctv-setch-name"><b>{c.name}</b>{c.group && <small>{c.group}</small>}</span>
    </>
  ) : <span className="ctv-setch-name"><b>{t("A channel you can't see")}</b><small>{key}</small></span>

  return (
    <>
      <SetHead title={isNew ? t('New set of channels') : t('Change “{name}”', { name })} onBack={() => void leave()} />
      <form className="ctv-setform" onSubmit={(e) => { e.preventDefault(); void save() }}>
        <label className="ctv-setfield">{t('Name')}
          <input className="input" value={name} maxLength={MAX_NAME} required autoFocus={isNew} onChange={(e) => { setName(e.target.value); setDirty(true) }} placeholder={t('Kids, Sport, News…')} />
        </label>
        {canShare && (
          <div className="ctv-setfield ctv-setshare">
            <span id="ctv-share-l">{t('Shared with the household')}</span>
            <Switch checked={shared} labelledBy="ctv-share-l" onChange={(v) => { setShared(v); setDirty(true) }} />
            <small>{t('Everyone signed in sees a shared set, each only with the channels they may watch. Only an administrator changes it.')}</small>
          </div>
        )}
        <div className="ctv-setactions">
          <button type="submit" className="ctv-btn on" disabled={busy || !trimmed || tooMany}>{busy ? t('Saving…') : isNew ? t('Make the set') : t('Save')}</button>
          {!isNew && <button type="button" className="ctv-btn" onClick={() => void remove()}><IconTrash size={16} /> {t('Delete set')}</button>}
        </div>
      </form>

      <div className="ctv-setcols">
        <section aria-labelledby="ctv-in-h">
          <h2 className="ctv-list-h2" id="ctv-in-h">{t('In this set, in order')} <small>{num(channels.length)}</small></h2>
          {tooMany && <p className="ctv-rec-err" role="alert">{t('A set can hold up to {count} channels. Take some out to save it.', { count: MAX_CHANNELS })}</p>}
          {channels.length === 0 ? <p className="ctv-list-empty">{t('None yet. Tick channels on the right to put them in.')}</p> : (
            <>
              <p className="ctv-list-note ctv-sets-hint">{t('Drag a channel, or use its arrows, to change the order.')}</p>
              <VirtualList count={channels.length} rowH={ROW} height={listH} label={t('Channels in this set')} className="ctv-setlist"
                onRowKey={(i, e) => {
                  // Alt is taken by the browser; with Shift, the arrows carry the channel.
                  if (e.shiftKey && e.key === 'ArrowUp') { moveTo(i, i - 1); return true }
                  if (e.shiftKey && e.key === 'ArrowDown') { moveTo(i, i + 1); return true }
                  return false
                }}
                render={(i) => {
                  const key = channels[i]
                  const c = byKey.get(key)
                  return (
                    <div className={'ctv-setch' + (dragFrom === i ? ' dragging' : '')} draggable
                      onDragStart={(e) => { setDragFrom(i); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', key) }}
                      onDragOver={(e) => { if (dragFrom != null) e.preventDefault() }}
                      onDrop={(e) => { e.preventDefault(); if (dragFrom != null) moveTo(dragFrom, i); setDragFrom(null) }}
                      onDragEnd={() => setDragFrom(null)}>
                      <span className="ctv-grip" aria-hidden="true"><IconGrip size={16} /></span>
                      <span className="ctv-setch-pos" aria-hidden="true">{i + 1}</span>
                      {row(c, key)}
                      <button type="button" className="ctv-btn sm icon" data-vfocus disabled={i === 0} onClick={() => moveTo(i, i - 1)} aria-label={c ? t('Move {name} up', { name: c.name }) : t('Move channel up')} title={t('Move up (Shift+Up)')}><IconUp size={16} /></button>
                      <button type="button" className="ctv-btn sm icon" disabled={i === channels.length - 1} onClick={() => moveTo(i, i + 1)} aria-label={c ? t('Move {name} down', { name: c.name }) : t('Move channel down')} title={t('Move down (Shift+Down)')}><IconDown size={16} /></button>
                      <button type="button" className="ctv-btn sm icon" onClick={() => change(channels.filter((_, n) => n !== i))} aria-label={c ? t('Take {name} out', { name: c.name }) : t('Take channel out')}><IconClose size={16} /></button>
                    </div>
                  )
                }} />
            </>
          )}
        </section>

        <section aria-labelledby="ctv-pick-h">
          <h2 className="ctv-list-h2" id="ctv-pick-h">{t('Every channel')} <small>{all ? num(all.length) : ''}</small></h2>
          <div className="ctv-setpick-bar">
            <input className="input" type="search" aria-label={t('Find channels, by number, name or group')} placeholder={t('Find by number, name or group')} value={q} onChange={(e) => setQ(e.target.value)} />
            <button type="button" className="ctv-btn sm" disabled={!shown.length} onClick={() => { change(addAll(channels, shown.map((c) => c.key))); announce(t('{count} put in', { count: shown.length })) }}><IconPlus size={16} /> {t('Select all shown')}</button>
            <button type="button" className="ctv-btn sm" disabled={!shown.some((c) => inSet.has(c.key))} onClick={() => { change(removeAll(channels, shown.map((c) => c.key))); announce(t('Taken out')) }}>{t('Clear all shown')}</button>
          </div>
          <div aria-live="polite" className="sr-only">{q ? t('{count} found', { count: shown.length }) : ''}</div>
          {all === null ? <div className="center" role="status" aria-label={t('Loading')}><div className="spinner" /></div>
            : shown.length === 0 ? <p className="ctv-list-empty">{t('No channel has that number, name or group.')}</p> : (
              <VirtualList count={shown.length} rowH={ROW} height={listH} label={t('Every channel')} className="ctv-setlist"
                render={(i) => {
                  const c = shown[i]
                  const has = inSet.has(c.key)
                  return (
                    <label className={'ctv-setch pick' + (has ? ' on' : '')}>
                      <input type="checkbox" data-vfocus checked={has} onChange={(e) => change(toggleIn(channels, c.key, e.target.checked))} aria-label={`${c.number} ${c.name}${c.group ? `, ${c.group}` : ''}`} />
                      {row(c, c.key)}
                    </label>
                  )
                }} />
            )}
        </section>
      </div>
    </>
  )
}

function SetHead({ title, onBack }: { title: string; onBack: () => void }) {
  return (
    <header className="ctv-list-head">
      <h1>{title}</h1>
      <button type="button" className="ctv-btn sm" onClick={onBack}>{t('Back to the sets')}</button>
      <Link className="ctv-btn sm" to="/tv">{t('Back to the guide')}</Link>
    </header>
  )
}
